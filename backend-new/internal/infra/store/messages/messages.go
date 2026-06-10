// Package messages is the orm-backed messagesdomain.Repository: a conversation's content
// journal — the `messages` table (turn records) + `message_blocks` (the Block tree). Both are
// append-only (no deleted_at, D1: a conversation's content is never deleted) and
// workspace-isolated (orm fills/filters workspace_id from ctx via the ,ws tag), so no method
// hand-writes a workspace predicate.
//
// A turn is written in two phases — CreateMessage opens it (and writes a user turn's lone text
// block), FinalizeMessage closes an assistant turn with terminal status + token accounting +
// its blocks — each inside one transaction so the message row and its blocks land atomically.
// Block seq is allocated MAX+1 per conversation inside that transaction; correctness relies on
// chat's per-conversation queue serializing writes (one AI goroutine per conversation), not on
// a DB sequence.
//
// Package messages 是 messagesdomain.Repository 的 orm 实现：一个对话的内容日志——`messages`
// 表（回合记录）+ `message_blocks`（Block 树）。两表皆 append-only（无 deleted_at，D1：对话内容
// 永不删）、按 workspace 隔离（orm 据 ctx 经 ,ws tag 填/过滤），故无方法手写 workspace 谓词。
//
// 回合两段式写——CreateMessage 开（并写 user 回合的单个 text block）、FinalizeMessage 以终态 +
// token 记账 + blocks 收 assistant 回合——各在一个事务内，使 message 行与其 blocks 原子落盘。
// block seq 在该事务内按对话 MAX+1 分配；正确性靠 chat 的 per-conversation 队列串行写
// （每对话一个 AI 协程）、而非 DB 序列。
package messages

import (
	"context"
	"errors"
	"fmt"

	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
)

// Schema is the two tables' DDL, exported as ordered idempotent statements for cmd/server to
// collect via db.Migrate. Both are append-only (no deleted_at, D1). message_blocks' UNIQUE
// (conversation_id, seq) is the seq monotonicity guarantee (idx_blocks_conv_seq); type /
// status / context_role are CHECK-closed so a bad value can never reach the LLM history.
//
// Schema 是两表 DDL，按序幂等导出。两表 append-only（无 deleted_at，D1）。message_blocks 的
// UNIQUE(conversation_id, seq) 即 seq 单调保证（idx_blocks_conv_seq）；type / status /
// context_role 用 CHECK 闭合，坏值永不进 LLM 历史。
var Schema = []string{
	`CREATE TABLE IF NOT EXISTS messages (
		id              TEXT PRIMARY KEY,
		workspace_id    TEXT NOT NULL,
		conversation_id TEXT NOT NULL,
		subagent_id     TEXT NOT NULL DEFAULT '',
		role            TEXT NOT NULL CHECK(role IN ('user','assistant')),
		status          TEXT NOT NULL DEFAULT 'completed' CHECK(status IN ('pending','streaming','completed','error','cancelled')),
		stop_reason     TEXT NOT NULL DEFAULT '',
		error_code      TEXT NOT NULL DEFAULT '',
		error_message   TEXT NOT NULL DEFAULT '',
		input_tokens    INTEGER NOT NULL DEFAULT 0,
		output_tokens   INTEGER NOT NULL DEFAULT 0,
		provider        TEXT NOT NULL DEFAULT '',
		model_id        TEXT NOT NULL DEFAULT '',
		attrs           TEXT NOT NULL DEFAULT 'null',
		created_at      DATETIME NOT NULL,
		updated_at      DATETIME NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_messages_conv ON messages(workspace_id, conversation_id, created_at, id)`,

	`CREATE TABLE IF NOT EXISTS message_blocks (
		id              TEXT PRIMARY KEY,
		workspace_id    TEXT NOT NULL,
		conversation_id TEXT NOT NULL,
		message_id      TEXT NOT NULL,
		parent_block_id TEXT NOT NULL DEFAULT '',
		seq             INTEGER NOT NULL,
		type            TEXT NOT NULL CHECK(type IN ('text','reasoning','tool_call','tool_result','compaction','progress')),
		attrs           TEXT NOT NULL DEFAULT 'null',
		content         TEXT NOT NULL DEFAULT '',
		status          TEXT NOT NULL CHECK(status IN ('pending','streaming','completed','error','cancelled')),
		error           TEXT NOT NULL DEFAULT '',
		context_role    TEXT NOT NULL DEFAULT 'hot' CHECK(context_role IN ('hot','warm','cold','archived')),
		created_at      DATETIME NOT NULL,
		updated_at      DATETIME NOT NULL
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_blocks_conv_seq ON message_blocks(conversation_id, seq)`,
	`CREATE INDEX IF NOT EXISTS idx_blocks_message ON message_blocks(message_id, seq)`,
}

// Store implements messagesdomain.Repository over pkg/orm. It keeps root-bound repos for reads
// and rebuilds tx-bound repos inside Transaction for the atomic two-table writes.
//
// Store 基于 pkg/orm 实现 messagesdomain.Repository。读用根绑定 repo，写在 Transaction 内重建
// tx 绑定 repo 以原子写两表。
type Store struct {
	db     *ormpkg.DB
	msgs   *ormpkg.Repo[messagesdomain.Message]
	blocks *ormpkg.Repo[messagesdomain.Block]
}

// New constructs a Store bound to the messages + message_blocks tables.
//
// New 构造绑定 messages + message_blocks 表的 Store。
func New(db *ormpkg.DB) *Store {
	return &Store{
		db:     db,
		msgs:   ormpkg.For[messagesdomain.Message](db, "messages"),
		blocks: ormpkg.For[messagesdomain.Block](db, "message_blocks"),
	}
}

var _ messagesdomain.Repository = (*Store)(nil)

func (s *Store) CreateMessage(ctx context.Context, m *messagesdomain.Message, blocks []messagesdomain.Block) error {
	return s.db.Transaction(ctx, func(tx *ormpkg.DB) error {
		if err := ormpkg.For[messagesdomain.Message](tx, "messages").Create(ctx, m); err != nil {
			return fmt.Errorf("messagesstore.CreateMessage: insert message: %w", err)
		}
		return insertBlocks(ctx, tx, m, blocks)
	})
}

func (s *Store) FinalizeMessage(ctx context.Context, m *messagesdomain.Message, blocks []messagesdomain.Block) error {
	return s.db.Transaction(ctx, func(tx *ormpkg.DB) error {
		// Partial update of terminal fields only (not Save), so a finalize never touches
		// created_at / role / conversation_id — and Updates' WHERE carries the auto workspace
		// filter, so n==0 means "no such message in this workspace".
		//
		// 仅部分更新终态字段（非 Save），使 finalize 不碰 created_at / role / conversation_id——
		// 且 Updates 的 WHERE 带自动 workspace 过滤，n==0 即「本 workspace 无此 message」。
		n, err := ormpkg.For[messagesdomain.Message](tx, "messages").
			WhereEq("id", m.ID).
			Updates(ctx, map[string]any{
				"status":        m.Status,
				"stop_reason":   m.StopReason,
				"error_code":    m.ErrorCode,
				"error_message": m.ErrorMessage,
				"input_tokens":  m.InputTokens,
				"output_tokens": m.OutputTokens,
				"provider":      m.Provider,
				"model_id":      m.ModelID,
			})
		if err != nil {
			return fmt.Errorf("messagesstore.FinalizeMessage: update message: %w", err)
		}
		if n == 0 {
			return messagesdomain.ErrMessageNotFound
		}
		return insertBlocks(ctx, tx, m, blocks)
	})
}

func (s *Store) GetMessage(ctx context.Context, id string) (*messagesdomain.Message, error) {
	m, err := s.msgs.Get(ctx, id)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, messagesdomain.ErrMessageNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("messagesstore.GetMessage: %w", err)
	}
	if err := s.hydrate(ctx, []*messagesdomain.Message{m}); err != nil {
		return nil, err
	}
	return m, nil
}

// ListMessages returns one keyset page, newest-first (orm Page is DESC on (created_at, id)) —
// the chat-history fetch pattern: load the most recent turns, page backwards for older ones.
// The front end renders chronologically by reversing a page; LoadThread serves the LLM's
// chronological need separately.
//
// ListMessages 返回一页 keyset，最新在前（orm Page 按 (created_at, id) 降序）——chat 历史拉取范式：
// 取最近回合、向后翻更旧。前端按时序渲染时反转一页；LLM 的时序需求由 LoadThread 另行满足。
func (s *Store) ListMessages(ctx context.Context, conversationID, cursor string, limit int) ([]*messagesdomain.Message, string, error) {
	rows, next, err := s.msgs.WhereEq("conversation_id", conversationID).Page(ctx, cursor, limit)
	if err != nil {
		return nil, "", fmt.Errorf("messagesstore.ListMessages: %w", err)
	}
	if err := s.hydrate(ctx, rows); err != nil {
		return nil, "", err
	}
	return rows, next, nil
}

// LoadThread returns the whole conversation oldest-first (Find with an ASC order, not Page) —
// the chronological source chat's LoadHistory composes LLM history from. Unpaginated: a single
// local user's thread fits in memory.
//
// LoadThread 返回整个对话、最旧在前（Find + ASC order，非 Page）——chat 的 LoadHistory 据此组装
// LLM 历史的时序来源。不分页：单用户本地一条线程可装进内存。
func (s *Store) LoadThread(ctx context.Context, conversationID string) ([]*messagesdomain.Message, error) {
	rows, err := s.msgs.WhereEq("conversation_id", conversationID).Order("created_at ASC, id ASC").Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("messagesstore.LoadThread: %w", err)
	}
	if err := s.hydrate(ctx, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// SumTokens totals a conversation's input + output tokens. It loads the turn rows (no blocks)
// and sums in Go — a single conversation's turns fit in memory, and the orm workspace filter
// keeps it scoped. Empty conversation → (0, 0).
//
// SumTokens 求和一个对话的 input + output token。加载回合行（不取 block）在 Go 里累加——单对话回合
// 可装进内存、orm workspace 过滤限定范围。空对话 → (0, 0)。
func (s *Store) SumTokens(ctx context.Context, conversationID string) (int, int, error) {
	rows, err := s.msgs.WhereEq("conversation_id", conversationID).Find(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("messagesstore.SumTokens: %w", err)
	}
	var in, out int
	for _, m := range rows {
		in += m.InputTokens
		out += m.OutputTokens
	}
	return in, out, nil
}

// UpdateBlocksContextRole batch-updates context_role for the given block ids (one statement via
// WhereIn). Mirrors FinalizeMessage's partial Updates (auto workspace filter in the WHERE); the
// stored content is never touched — only the projection role.
//
// UpdateBlocksContextRole 按给定 block id 批量更新 context_role（WhereIn 一条语句）。镜像
// FinalizeMessage 的部分 Updates（WHERE 带自动 workspace 过滤）；落库 content 永不动——只改投影角色。
func (s *Store) UpdateBlocksContextRole(ctx context.Context, blockIDs []string, role string) error {
	if len(blockIDs) == 0 {
		return nil
	}
	ids := make([]any, len(blockIDs))
	for i, id := range blockIDs {
		ids[i] = id
	}
	if _, err := s.blocks.WhereIn("id", ids...).Updates(ctx, map[string]any{"context_role": role}); err != nil {
		return fmt.Errorf("messagesstore.UpdateBlocksContextRole: %w", err)
	}
	return nil
}

// hydrate loads every block of the given messages in one query and attaches each message's
// blocks (seq-ordered) to its Blocks field. A message with no blocks gets a nil slice.
//
// hydrate 一次查出给定 messages 的所有 block，把每个 message 的 block（按 seq 排序）挂到其
// Blocks 字段。无 block 的 message 得 nil 切片。
func (s *Store) hydrate(ctx context.Context, msgs []*messagesdomain.Message) error {
	if len(msgs) == 0 {
		return nil
	}
	ids := make([]any, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	blocks, err := s.blocks.WhereIn("message_id", ids...).Order("seq ASC").Find(ctx)
	if err != nil {
		return fmt.Errorf("messagesstore.hydrate: %w", err)
	}
	byMsg := make(map[string][]messagesdomain.Block, len(msgs))
	for _, b := range blocks {
		byMsg[b.MessageID] = append(byMsg[b.MessageID], *b)
	}
	for _, m := range msgs {
		m.Blocks = byMsg[m.ID]
	}
	return nil
}

// insertBlocks assigns each block a fresh id (if empty), the turn's conversation + message ids,
// a monotonic per-conversation seq, and default status / context_role, then inserts it. It
// mutates the caller's slice in place so the caller sees the assigned ids / seq (orm also fills
// workspace_id + timestamps). The MAX+1 read happens once, before the loop.
//
// insertBlocks 给每个 block 赋新 id（若空）、回合的 conversation + message id、对话内单调 seq、
// 默认 status / context_role，然后插入。原地改 caller 切片，使 caller 看到分配的 id / seq
// （orm 还填 workspace_id + 时间戳）。MAX+1 在循环前读一次。
func insertBlocks(ctx context.Context, tx *ormpkg.DB, m *messagesdomain.Message, blocks []messagesdomain.Block) error {
	if len(blocks) == 0 {
		return nil
	}
	blockRepo := ormpkg.For[messagesdomain.Block](tx, "message_blocks")
	seq, err := nextSeq(ctx, blockRepo, m.ConversationID)
	if err != nil {
		return err
	}
	for i := range blocks {
		if blocks[i].ID == "" {
			blocks[i].ID = idgenpkg.New("blk")
		}
		blocks[i].ConversationID = m.ConversationID
		blocks[i].MessageID = m.ID
		blocks[i].Seq = seq
		seq++
		if blocks[i].Status == "" {
			blocks[i].Status = messagesdomain.StatusCompleted
		}
		if blocks[i].ContextRole == "" {
			blocks[i].ContextRole = messagesdomain.ContextRoleHot
		}
		if err := blockRepo.Create(ctx, &blocks[i]); err != nil {
			return fmt.Errorf("messagesstore: insert block %d: %w", i, err)
		}
	}
	return nil
}

// nextSeq returns MAX(seq)+1 for the conversation (1 when none yet). The orm auto workspace
// filter applies, but conversation_id is globally unique so it only ever sees one workspace's
// rows anyway.
//
// nextSeq 返回该对话的 MAX(seq)+1（无则 1）。orm 自动 workspace 过滤生效，但 conversation_id
// 全局唯一、本就只见一个 workspace 的行。
func nextSeq(ctx context.Context, blockRepo *ormpkg.Repo[messagesdomain.Block], conversationID string) (int64, error) {
	var seqs []int64
	if err := blockRepo.WhereEq("conversation_id", conversationID).Order("seq DESC").Limit(1).Pluck(ctx, "seq", &seqs); err != nil {
		return 0, fmt.Errorf("messagesstore.nextSeq: %w", err)
	}
	if len(seqs) == 0 {
		return 1, nil
	}
	return seqs[0] + 1, nil
}

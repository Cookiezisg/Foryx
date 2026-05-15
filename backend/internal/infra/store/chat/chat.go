// Package chat (infra/store/chat) is the GORM-backed implementation of
// chatdomain.Repository. Combines Message + Block + Attachment ops in
// a single Store.
//
// User scoping: Message + Attachment methods filter by ctx user.
// Block methods do NOT filter — auth lives on the parent Message;
// blocks are written server-side trusted via emitter.
//
// Package chat（infra/store/chat）是 chatdomain.Repository 的 GORM 实现。
// 单一 Store 合并 Message + Block + Attachment 操作。
//
// 用户过滤：Message + Attachment 方法按 ctx 用户过滤；Block 方法不过滤
// ——auth 在父 Message 上，block 写由 emitter server-side 可信。
package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Store is the GORM implementation of chatdomain.Repository.
//
// Store 是 chatdomain.Repository 的 GORM 实现。
type Store struct {
	db *gorm.DB
}

// New constructs a Store bound to the given *gorm.DB.
//
// New 基于给定 *gorm.DB 构造 Store。
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// ── Message ──────────────────────────────────────────────────────────

// SaveMessage upserts a Message row. Blocks are NOT written here —
// they go through SaveBlock / AppendDelta / FinalizeStop via emitter.
// Uses an explicit ON CONFLICT upsert so created_at is only written on
// INSERT and never overwritten by subsequent status updates.
//
// SaveMessage upsert message 行。Blocks 不在此写——经 emitter 走
// SaveBlock / AppendDelta / FinalizeStop。显式 ON CONFLICT upsert 让
// created_at 仅 INSERT 时写，后续 status 更新不覆盖。
func (s *Store) SaveMessage(ctx context.Context, m *chatdomain.Message) error {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	m.UpdatedAt = time.Now().UTC()
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"status", "stop_reason", "error_code", "error_message",
			"input_tokens", "output_tokens", "attrs", "updated_at",
		}),
	}).Create(m).Error
	if err != nil {
		return fmt.Errorf("chatstore.SaveMessage: %w", err)
	}
	return nil
}

// GetMessage fetches a single Message by id (with Blocks attached),
// scoped to the current user. Returns ErrMessageNotFound if no live
// record matches.
//
// GetMessage 按 id 查单 Message（含 Blocks），按当前用户过滤。未命中
// 活跃记录返 ErrMessageNotFound。
func (s *Store) GetMessage(ctx context.Context, id string) (*chatdomain.Message, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var m chatdomain.Message
	err = s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, uid).
		First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, chatdomain.ErrMessageNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("chatstore.GetMessage: %w", err)
	}
	if err := s.attachBlocks(ctx, []*chatdomain.Message{&m}); err != nil {
		return nil, err
	}
	return &m, nil
}

// ListMessagesByConversation returns a cursor-paginated page of messages
// (with Blocks), ordered by created_at ASC.
//
// ListMessagesByConversation 返带 Blocks 的 cursor 分页 message，
// created_at ASC 排序。
func (s *Store) ListMessagesByConversation(ctx context.Context, conversationID string, filter chatdomain.ListFilter) ([]*chatdomain.Message, string, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, "", err
	}
	limit := filter.Limit

	q := s.db.WithContext(ctx).
		Where("conversation_id = ? AND user_id = ?", conversationID, uid)
	if filter.Cursor != "" {
		var c paginationpkg.Cursor
		if err := paginationpkg.DecodeCursor(filter.Cursor, &c); err != nil {
			return nil, "", fmt.Errorf("chatstore.ListMessagesByConversation: %w", err)
		}
		q = q.Where("(created_at, id) > (?, ?)", c.CreatedAt, c.ID)
	}

	var rows []*chatdomain.Message
	if err := q.Order("created_at ASC, id ASC").
		Limit(limit + 1).
		Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("chatstore.ListMessagesByConversation: %w", err)
	}

	var next string
	if len(rows) > limit {
		last := rows[limit-1]
		next, err = paginationpkg.EncodeCursor(paginationpkg.Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return nil, "", fmt.Errorf("chatstore.ListMessagesByConversation cursor: %w", err)
		}
		rows = rows[:limit]
	}

	if err := s.attachBlocks(ctx, rows); err != nil {
		return nil, "", err
	}
	return rows, next, nil
}

// attachBlocks fetches all blocks for the given messages in one query
// (ordered by seq ASC) and distributes them to each Message.Blocks.
// Avoids N+1.
//
// attachBlocks 一次查询取所有相关 blocks（seq ASC）并分配到对应消息
// Blocks——避免 N+1。
func (s *Store) attachBlocks(ctx context.Context, rows []*chatdomain.Message) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]string, len(rows))
	for i, m := range rows {
		ids[i] = m.ID
	}

	var blocks []*chatdomain.Block
	if err := s.db.WithContext(ctx).
		Where("message_id IN ?", ids).
		Order("seq ASC").
		Find(&blocks).Error; err != nil {
		return fmt.Errorf("chatstore.attachBlocks: %w", err)
	}

	blockMap := make(map[string][]chatdomain.Block, len(rows))
	for _, b := range blocks {
		blockMap[b.MessageID] = append(blockMap[b.MessageID], *b)
	}
	for _, m := range rows {
		m.Blocks = blockMap[m.ID]
	}
	return nil
}

// ── Block ────────────────────────────────────────────────────────────

// SaveBlock upserts the row, or replaces it on PK conflict (used at
// block_start with status=streaming, then again at block_stop with
// terminal status). CreatedAt is preserved on conflict; UpdatedAt is
// always written.
//
// SaveBlock upsert 行，PK 冲突时替换（block_start 用 status=streaming
// 写一次，block_stop 用终态再写一次）。冲突时 CreatedAt 保留；
// UpdatedAt 总写。
func (s *Store) SaveBlock(ctx context.Context, b *chatdomain.Block) error {
	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now().UTC()
	}
	b.UpdatedAt = time.Now().UTC()
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"content", "status", "error", "attrs", "updated_at",
		}),
	}).Create(b).Error
	if err != nil {
		return fmt.Errorf("chatstore.SaveBlock: %w", err)
	}
	return nil
}

// AppendDelta atomically appends delta via SQL string-concat. Avoids
// read-modify-write race when many DeltaBlock emits arrive concurrently
// (rare in practice — single chat loop publishes per conversation —
// but the cost is one short SQL statement).
//
// AppendDelta 经 SQL 字符串拼接原子追加。避免 DeltaBlock 并发到达时
// 的 read-modify-write 竞争（实践中罕见——单 chat loop per 对话——
// 但代价仅一条短 SQL）。
func (s *Store) AppendDelta(ctx context.Context, blockID, delta string) error {
	res := s.db.WithContext(ctx).
		Model(&chatdomain.Block{}).
		Where("id = ?", blockID).
		Updates(map[string]any{
			"content":    gorm.Expr("content || ?", delta),
			"updated_at": time.Now().UTC(),
		})
	if res.Error != nil {
		return fmt.Errorf("chatstore.AppendDelta: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return chatdomain.ErrBlockNotFound
	}
	return nil
}

// FinalizeStop updates status + error on blockID. Returns
// ErrBlockNotFound if the row doesn't exist.
//
// FinalizeStop 给 blockID 更新 status + error。行不存在返
// ErrBlockNotFound。
func (s *Store) FinalizeStop(ctx context.Context, blockID, status, errStr string) error {
	res := s.db.WithContext(ctx).
		Model(&chatdomain.Block{}).
		Where("id = ?", blockID).
		Updates(map[string]any{
			"status":     status,
			"error":      errStr,
			"updated_at": time.Now().UTC(),
		})
	if res.Error != nil {
		return fmt.Errorf("chatstore.FinalizeStop: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return chatdomain.ErrBlockNotFound
	}
	return nil
}

// GetBlock returns blockID's row. ErrBlockNotFound when absent.
//
// GetBlock 返 blockID 行。缺失返 ErrBlockNotFound。
func (s *Store) GetBlock(ctx context.Context, blockID string) (*chatdomain.Block, error) {
	var b chatdomain.Block
	err := s.db.WithContext(ctx).Where("id = ?", blockID).First(&b).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, chatdomain.ErrBlockNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("chatstore.GetBlock: %w", err)
	}
	return &b, nil
}

// ListBlocksByConversation returns all blocks of conversationID, seq ASC.
//
// ListBlocksByConversation 返 conversationID 的所有 block，seq ASC。
func (s *Store) ListBlocksByConversation(ctx context.Context, conversationID string) ([]*chatdomain.Block, error) {
	var rows []*chatdomain.Block
	err := s.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("seq ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("chatstore.ListBlocksByConversation: %w", err)
	}
	return rows, nil
}

// ListBlocksByMessage returns all blocks of messageID, seq ASC.
//
// ListBlocksByMessage 返 messageID 的所有 block，seq ASC。
func (s *Store) ListBlocksByMessage(ctx context.Context, messageID string) ([]*chatdomain.Block, error) {
	var rows []*chatdomain.Block
	err := s.db.WithContext(ctx).
		Where("message_id = ?", messageID).
		Order("seq ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("chatstore.ListBlocksByMessage: %w", err)
	}
	return rows, nil
}

// ReplayEventsAfter reconstructs the block-as-events stream for
// conversationID with seq > fromSeq. See chatdomain.Repository.
// ReplayEventsAfter for contract.
//
// ReplayEventsAfter 重构 conversationID 中 seq > fromSeq 的 blocks-
// as-events 流。契约见 chatdomain.Repository.ReplayEventsAfter。
func (s *Store) ReplayEventsAfter(ctx context.Context, conversationID string, fromSeq int64) ([]chatdomain.ReplayEnvelope, error) {
	var rows []*chatdomain.Block
	err := s.db.WithContext(ctx).
		Where("conversation_id = ? AND seq > ?", conversationID, fromSeq).
		Order("seq ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("chatstore.ReplayEventsAfter: %w", err)
	}

	out := make([]chatdomain.ReplayEnvelope, 0, len(rows)*3)
	for _, b := range rows {
		// Attrs is already a map[string]any after the 2026-05 serializer
		// refactor — GORM unmarshalled it from the text column on read.
		// Attrs 2026-05 重构后是 map[string]any (GORM 读列时已 unmarshal)。
		attrs := b.Attrs

		// Reconstruct wire ParentID: empty parent_block_id in DB means
		// "top-level block of the message" → wire ParentID = MessageID.
		// Non-empty → wire ParentID = parent_block_id (nested case).
		//
		// 从 DB 重构 wire ParentID：parent_block_id 空 = 顶层 → wire
		// ParentID = MessageID；非空 = 嵌套 → wire ParentID = parent_block_id。
		parentID := b.ParentBlockID
		if parentID == "" {
			parentID = b.MessageID
		}

		out = append(out, chatdomain.ReplayEnvelope{
			Type: "block_start",
			Seq:  b.Seq,
			Payload: map[string]any{
				"conversationId": conversationID,
				"id":             b.ID,
				"parentId":       parentID,
				"messageId":      b.MessageID,
				"blockType":      b.Type,
				"attrs":          attrs,
			},
		})
		if b.Content != "" {
			out = append(out, chatdomain.ReplayEnvelope{
				Type: "block_delta",
				Seq:  b.Seq,
				Payload: map[string]any{
					"conversationId": conversationID,
					"id":             b.ID,
					"delta":          b.Content,
				},
			})
		}
		stopPayload := map[string]any{
			"conversationId": conversationID,
			"id":             b.ID,
			"status":         b.Status,
		}
		if b.Error != "" {
			stopPayload["error"] = b.Error
		}
		out = append(out, chatdomain.ReplayEnvelope{
			Type:    "block_stop",
			Seq:     b.Seq,
			Payload: stopPayload,
		})
	}
	return out, nil
}

// ── Attachment ───────────────────────────────────────────────────────

// SaveAttachment inserts an Attachment record (write-once).
//
// SaveAttachment 插入 Attachment 记录（一次写）。
func (s *Store) SaveAttachment(ctx context.Context, a *chatdomain.Attachment) error {
	if err := s.db.WithContext(ctx).Create(a).Error; err != nil {
		return fmt.Errorf("chatstore.SaveAttachment: %w", err)
	}
	return nil
}

// GetAttachment fetches an Attachment by id, scoped to the current user.
//
// GetAttachment 按 id 取，按当前用户过滤。
func (s *Store) GetAttachment(ctx context.Context, id string) (*chatdomain.Attachment, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var a chatdomain.Attachment
	err = s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, uid).
		First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("chatstore.GetAttachment: attachment %q not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("chatstore.GetAttachment: %w", err)
	}
	return &a, nil
}

// Compile-time check that *Store satisfies chatdomain.Repository.
//
// 编译期确认 *Store 满足 chatdomain.Repository。
var _ chatdomain.Repository = (*Store)(nil)

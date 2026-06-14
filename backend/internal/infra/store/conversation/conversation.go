// Package conversation is the orm-backed conversationdomain.Repository: a workspace-scoped,
// soft-deleted thread table. Workspace isolation + soft-delete are automatic (orm fills/filters
// from ctx), so no method hand-writes a predicate. List sorts pinned-first then most-recently-
// active, keyset-paginated on (last_message_at, id).
//
// Package conversation 是 conversationdomain.Repository 的 orm 实现：按 workspace、软删的线程表。
// workspace 隔离 + 软删自动（orm 据 ctx 填/过滤），故无方法手写谓词。List 置顶优先再最近活跃、按
// (last_message_at, id) keyset 分页。
package conversation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	conversationdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
)

// Schema is the conversations DDL, exported as ordered idempotent statements for bootstrap to
// apply via db.Migrate. A business/Log table with soft-delete (deleted_at) per D1; the partial
// list index keys the pinned-first, newest-next ordering the frontend renders.
//
// Schema 是 conversations 表 DDL，按序幂等语句导出、由 bootstrap 经 db.Migrate 应用。业务表带
// 软删（deleted_at，D1）；partial 列表索引键住「置顶优先、再最新」的前端渲染顺序。
var Schema = []string{
	`CREATE TABLE IF NOT EXISTS conversations (
		id                       TEXT PRIMARY KEY,
		workspace_id             TEXT NOT NULL,
		title                    TEXT NOT NULL DEFAULT '',
		auto_titled              INTEGER NOT NULL DEFAULT 0,
		system_prompt            TEXT NOT NULL DEFAULT '',
		summary                  TEXT NOT NULL DEFAULT '',
		summary_covers_up_to_seq INTEGER NOT NULL DEFAULT 0,
		attached_documents       TEXT NOT NULL DEFAULT '[]',
		archived                 INTEGER NOT NULL DEFAULT 0,
		pinned                   INTEGER NOT NULL DEFAULT 0,
		model_override           TEXT,
		created_at               DATETIME NOT NULL,
		updated_at               DATETIME NOT NULL,
		last_message_at          DATETIME NOT NULL,
		deleted_at               DATETIME
	)`,
	`CREATE INDEX IF NOT EXISTS idx_conversations_ws_list ON conversations(workspace_id, pinned DESC, last_message_at DESC, id DESC) WHERE deleted_at IS NULL`,
}

// Store implements conversationdomain.Repository over pkg/orm.
//
// Store 基于 pkg/orm 实现 conversationdomain.Repository。
type Store struct {
	db   *ormpkg.DB
	repo *ormpkg.Repo[conversationdomain.Conversation]
}

// New constructs a Store bound to the conversations table.
//
// New 构造绑定 conversations 表的 Store。
func New(db *ormpkg.DB) *Store {
	return &Store{db: db, repo: ormpkg.For[conversationdomain.Conversation](db, "conversations")}
}

var _ conversationdomain.Repository = (*Store)(nil)

func (s *Store) Insert(ctx context.Context, c *conversationdomain.Conversation) error {
	if err := s.repo.Create(ctx, c); err != nil {
		return fmt.Errorf("conversationstore.Insert: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, id string) (*conversationdomain.Conversation, error) {
	c, err := s.repo.Get(ctx, id)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, conversationdomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("conversationstore.Get: %w", err)
	}
	return c, nil
}

func (s *Store) GetBatch(ctx context.Context, ids []string) ([]*conversationdomain.Conversation, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.repo.WhereIn("id", toAny(ids)...).Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("conversationstore.GetBatch: %w", err)
	}
	return rows, nil
}

// List returns one page: pinned-first then most-recently-active, keyset cursor on
// (last_message_at, id). The cursor keys (last_message_at, id) only — the leading pinned partition
// relies on all pins landing on page one (few, single-user), so it never drifts across pages.
// PageKeyset aligns the cursor column with the ORDER BY's last_message_at (the keyset invariant).
//
// List 返一页：置顶优先再最近活跃，游标键 (last_message_at, id)。游标只键 (last_message_at, id)——
// 置顶分区靠「所有置顶都落首页」（少、单用户）故不跨页漂移。PageKeyset 让游标列与 ORDER BY 的
// last_message_at 对齐（keyset 不变量）。
func (s *Store) List(ctx context.Context, filter conversationdomain.ListFilter) ([]*conversationdomain.Conversation, string, error) {
	q := s.repo.Query()
	if filter.Archived == nil {
		q = q.WhereEq("archived", false)
	} else {
		q = q.WhereEq("archived", *filter.Archived)
	}
	if term := strings.TrimSpace(filter.Search); term != "" {
		q = q.Where("title LIKE ?", "%"+term+"%")
	}
	rows, next, err := q.Order("pinned DESC, last_message_at DESC, id DESC").PageKeyset("last_message_at").Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("conversationstore.List: %w", err)
	}
	return rows, next, nil
}

// TouchLastMessage sets last_message_at on one conversation (chat calls it when a message lands).
//
// TouchLastMessage 把某对话的 last_message_at 设为 t（chat 在消息落地时调）。
func (s *Store) TouchLastMessage(ctx context.Context, id string, t time.Time) error {
	if _, err := s.repo.Query().WhereEq("id", id).Updates(ctx, map[string]any{"last_message_at": t}); err != nil {
		return fmt.Errorf("conversationstore.TouchLastMessage: %w", err)
	}
	return nil
}

func (s *Store) Update(ctx context.Context, c *conversationdomain.Conversation) error {
	if err := s.repo.Save(ctx, c); err != nil {
		return fmt.Errorf("conversationstore.Update: %w", err)
	}
	return nil
}

func (s *Store) SoftDelete(ctx context.Context, id string) error {
	found, err := s.repo.Delete(ctx, id)
	if err != nil {
		return fmt.Errorf("conversationstore.SoftDelete: %w", err)
	}
	if !found {
		return conversationdomain.ErrNotFound
	}
	return nil
}

// toAny widens a []string to []any for orm WhereIn variadic args.
//
// toAny 把 []string 拓宽为 []any 以喂 orm WhereIn 变长参数。
func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, v := range ss {
		out[i] = v
	}
	return out
}

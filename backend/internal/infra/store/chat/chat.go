// Package chat (infra/store/chat) is the GORM-backed implementation of
// the domain chat Repository port. Every method scopes queries by the
// userID carried in ctx — callers MUST have run the InjectUserID middleware.
//
// Package chat（infra/store/chat）是 domain chat Repository port 的 GORM 实现。
// 所有方法按 ctx 中的 userID 过滤——调用方必须先经过 InjectUserID 中间件。
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

// Save inserts or updates a Message and replaces its Blocks atomically.
// If m.Blocks is non-empty, existing blocks for this message are deleted and
// the new set is inserted within the same transaction.
//
// Uses an explicit ON CONFLICT upsert so that created_at is only written on
// INSERT and never overwritten by subsequent status updates.
//
// Save 原子地插入或更新 Message，并替换其 Blocks。
// 若 m.Blocks 非空，在同一事务中删除该消息的旧 blocks 并插入新集合。
// 使用显式 ON CONFLICT upsert，保证 created_at 仅在首次 INSERT 时写入，
// 后续状态更新不覆盖。
func (s *Store) Save(ctx context.Context, m *chatdomain.Message) error {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"status", "stop_reason", "input_tokens", "output_tokens",
			}),
		}).Create(m).Error
		if err != nil {
			return fmt.Errorf("chatstore.Save message: %w", err)
		}
		if len(m.Blocks) == 0 {
			return nil
		}
		// Stamp MessageID on every block before insert.
		// 插入前把 MessageID 打在每个 block 上。
		for i := range m.Blocks {
			m.Blocks[i].MessageID = m.ID
		}
		if err := tx.Where("message_id = ?", m.ID).
			Delete(&chatdomain.Block{}).Error; err != nil {
			return fmt.Errorf("chatstore.Save delete old blocks: %w", err)
		}
		if err := tx.Create(&m.Blocks).Error; err != nil {
			return fmt.Errorf("chatstore.Save blocks: %w", err)
		}
		return nil
	})
}

// Get fetches a single Message by id, scoped to the current user.
// Returns ErrMessageNotFound if no live record matches.
//
// Get 按 id 查单条 Message，按当前用户过滤。未命中活跃记录返回 ErrMessageNotFound。
func (s *Store) Get(ctx context.Context, id string) (*chatdomain.Message, error) {
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
		return nil, fmt.Errorf("chatstore.Get: %w", err)
	}
	return &m, nil
}

// ListByConversation returns a cursor-paginated page of messages with their
// Blocks, ordered by created_at ASC (oldest first — chronological).
// Uses a (created_at, id) tuple cursor for stable pagination.
//
// ListByConversation 返回带 Blocks 的 cursor 分页消息，按 created_at ASC 排序。
// 使用 (created_at, id) 元组 cursor 保证分页稳定。
func (s *Store) ListByConversation(ctx context.Context, conversationID string, filter chatdomain.ListFilter) ([]*chatdomain.Message, string, error) {
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
			return nil, "", fmt.Errorf("chatstore.ListByConversation: %w", err)
		}
		q = q.Where("(created_at, id) > (?, ?)", c.CreatedAt, c.ID)
	}

	var rows []*chatdomain.Message
	if err := q.Order("created_at ASC, id ASC").
		Limit(limit + 1).
		Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("chatstore.ListByConversation: %w", err)
	}

	var next string
	if len(rows) > limit {
		last := rows[limit-1]
		next, err = paginationpkg.EncodeCursor(paginationpkg.Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return nil, "", fmt.Errorf("chatstore.ListByConversation cursor: %w", err)
		}
		rows = rows[:limit]
	}

	if err := s.attachBlocks(ctx, rows); err != nil {
		return nil, "", err
	}
	return rows, next, nil
}

// attachBlocks fetches all blocks for the given messages in one query and
// distributes them — avoids N+1 queries.
//
// attachBlocks 一次查询取所有相关 blocks 并分配到对应消息——避免 N+1。
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
		Order("message_id, seq ASC").
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

// SaveAttachment inserts an Attachment record (write-once; files are immutable).
//
// SaveAttachment 插入 Attachment 记录（仅写一次，文件上传后不可变）。
func (s *Store) SaveAttachment(ctx context.Context, a *chatdomain.Attachment) error {
	if err := s.db.WithContext(ctx).Create(a).Error; err != nil {
		return fmt.Errorf("chatstore.SaveAttachment: %w", err)
	}
	return nil
}

// GetAttachment fetches an Attachment by id, scoped to the current user.
//
// GetAttachment 按 id 查 Attachment，按当前用户过滤。
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
// 编译期确认 *Store 满足 chatdomain.Repository。
var _ chatdomain.Repository = (*Store)(nil)

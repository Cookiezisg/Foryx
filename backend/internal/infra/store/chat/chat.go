// Package chat is the GORM-backed chatdomain.Repository (Message + Block + Attachment).
//
// Package chat 是 chatdomain.Repository 的 GORM 实现（Message + Block + Attachment）。
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

// SaveMessage upserts a Message row; created_at preserved on conflict (blocks written elsewhere).
//
// SaveMessage upsert Message 行；冲突时保留 created_at（blocks 走 SaveBlock）。
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

// GetMessage fetches one Message (with Blocks) by id; returns ErrMessageNotFound on miss.
//
// GetMessage 按 id 取 Message（含 Blocks）；未命中返 ErrMessageNotFound。
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

// ListMessagesByConversation returns a cursor-paginated page with Blocks, created_at ASC.
//
// ListMessagesByConversation 返带 Blocks 的分页 messages，created_at ASC。
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

// SaveBlock upserts the row; CreatedAt preserved on PK conflict, UpdatedAt always written.
//
// SaveBlock upsert 行；PK 冲突时 CreatedAt 保留，UpdatedAt 总写。
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

// AppendDelta atomically appends via SQL concat, avoiding read-modify-write races.
//
// AppendDelta 用 SQL 拼接原子追加，避免 read-modify-write 竞争。
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

// UpdateBlockRole sets context_role on blockID; returns ErrBlockNotFound when missing.
//
// UpdateBlockRole 设 blockID 的 context_role；行不存在返 ErrBlockNotFound。
func (s *Store) UpdateBlockRole(ctx context.Context, blockID, role string) error {
	res := s.db.WithContext(ctx).
		Model(&chatdomain.Block{}).
		Where("id = ?", blockID).
		Updates(map[string]any{
			"context_role": role,
			"updated_at":   time.Now().UTC(),
		})
	if res.Error != nil {
		return fmt.Errorf("chatstore.UpdateBlockRole: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return chatdomain.ErrBlockNotFound
	}
	return nil
}

// FinalizeStop updates status + error on blockID; ErrBlockNotFound if missing.
//
// FinalizeStop 更新 blockID 的 status + error；不存在返 ErrBlockNotFound。
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

// GetBlock returns blockID's row; ErrBlockNotFound when absent.
//
// GetBlock 返 blockID 行；缺失返 ErrBlockNotFound。
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

// ReplayEventsAfter reconstructs block-as-events for conversationID with seq > fromSeq.
//
// ReplayEventsAfter 重构 conversationID 中 seq > fromSeq 的 blocks-as-events 流。
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
		attrs := b.Attrs

		// Empty parent_block_id = top-level block → wire ParentID is the MessageID.
		// 空 parent_block_id = 顶层 block → wire ParentID 用 MessageID。
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
		return nil, fmt.Errorf("chatstore.GetAttachment %q: %w", id, chatdomain.ErrAttachmentNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("chatstore.GetAttachment: %w", err)
	}
	return &a, nil
}

var _ chatdomain.Repository = (*Store)(nil)

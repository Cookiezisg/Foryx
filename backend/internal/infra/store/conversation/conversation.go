// Package conversation is the GORM-backed convdomain.Repository.
//
// Package conversation 是 convdomain.Repository 的 GORM 实现。
package conversation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Store is the GORM implementation of convdomain.Repository.
//
// Store 是 convdomain.Repository 的 GORM 实现。
type Store struct {
	db *gorm.DB
}

// New constructs a Store bound to the given *gorm.DB.
//
// New 基于给定 *gorm.DB 构造 Store。
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Save inserts or updates by primary key.
//
// Save 按主键插入或更新。
func (s *Store) Save(ctx context.Context, c *convdomain.Conversation) error {
	if err := s.db.WithContext(ctx).Save(c).Error; err != nil {
		return fmt.Errorf("convstore.Save: %w", err)
	}
	return nil
}

// Get fetches one Conversation by id, scoped to the current user.
//
// Get 按 id 查单条，按当前用户过滤。
func (s *Store) Get(ctx context.Context, id string) (*convdomain.Conversation, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var c convdomain.Conversation
	err = s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, uid).
		First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, convdomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("convstore.Get: %w", err)
	}
	return &c, nil
}

// List returns a page newest-first, using (created_at, id) tuple cursor.
//
// List 返一页（最新优先），(created_at, id) 元组 cursor 稳定分页。
func (s *Store) List(ctx context.Context, filter convdomain.ListFilter) ([]*convdomain.Conversation, string, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, "", err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	q := s.db.WithContext(ctx).Where("user_id = ?", uid)
	if s := strings.TrimSpace(filter.Search); s != "" {
		q = q.Where("title LIKE ?", "%"+s+"%")
	}
	// §17.12: nil filter excludes archived from default list; explicit true/false honors caller.
	// §17.12: filter 为 nil 时默认排除已归档；显式 true/false 按调用方意图过滤。
	if filter.Archived == nil {
		q = q.Where("archived = ?", false)
	} else {
		q = q.Where("archived = ?", *filter.Archived)
	}
	if filter.Cursor != "" {
		var c paginationpkg.Cursor
		if err := paginationpkg.DecodeCursor(filter.Cursor, &c); err != nil {
			return nil, "", fmt.Errorf("convstore.List: %w", err)
		}
		q = q.Where("(created_at, id) < (?, ?)", c.CreatedAt, c.ID)
	}
	var rows []*convdomain.Conversation
	// §15.6: pinned DESC first so favorites bubble to top; cursor still keys (created_at, id)
	// because pinned count ≪ page limit in single-user practice (no cross-page pinned drift).
	// §15.6: 排序加 pinned DESC，置顶项浮顶；cursor 仍仅含 (created_at, id) —— 单用户场景 pinned 数远 < limit，跨页漂移不存在。
	if err := q.Order("pinned DESC, created_at DESC, id DESC").
		Limit(limit + 1).
		Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("convstore.List: %w", err)
	}
	var next string
	if len(rows) > limit {
		last := rows[limit-1]
		next, err = paginationpkg.EncodeCursor(paginationpkg.Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return nil, "", fmt.Errorf("convstore.List: %w", err)
		}
		rows = rows[:limit]
	}
	return rows, next, nil
}

// AnyReferencesApiKey reports whether any conversation.model_override JSON refers
// to this api_key.
//
// AnyReferencesApiKey 报告是否有 conversation.model_override JSON 引用此 api_key。
func (s *Store) AnyReferencesApiKey(ctx context.Context, apiKeyID string) (bool, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return false, fmt.Errorf("convstore.AnyReferencesApiKey: %w", err)
	}
	var count int64
	// SQLite json_extract pulls apiKeyId field from the model_override JSON column.
	//
	// SQLite json_extract 拿出 apiKeyId 字段做匹配。
	if err := s.db.WithContext(ctx).
		Model(&convdomain.Conversation{}).
		Where("user_id = ? AND json_extract(model_override, '$.apiKeyId') = ?", uid, apiKeyID).
		Limit(1).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("convstore.AnyReferencesApiKey: %w", err)
	}
	return count > 0, nil
}

// Delete soft-deletes by id, scoped to the current user.
//
// Delete 按 id 软删除，按当前用户过滤。
func (s *Store) Delete(ctx context.Context, id string) error {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return err
	}
	res := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, uid).
		Delete(&convdomain.Conversation{})
	if res.Error != nil {
		return fmt.Errorf("convstore.Delete: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return convdomain.ErrNotFound
	}
	return nil
}

// Package todo is the GORM-backed tododomain.Repository (conversation-scoped, not user-scoped).
//
// Package todo 是 tododomain.Repository 的 GORM 实现（按 conversation 作用域）。
package todo

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	tododomain "github.com/sunweilin/forgify/backend/internal/domain/todo"
)

// Store is the GORM implementation of tododomain.Repository.
//
// Store 是 tododomain.Repository 的 GORM 实现。
type Store struct {
	db *gorm.DB
}

// New constructs a Store bound to the given *gorm.DB.
//
// New 基于给定 *gorm.DB 构造 Store。
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Create inserts a new todo; GORM errors wrapped, uniqueness conflicts pass via errors.Is.
//
// Create 插入新 todo；GORM 错包装，唯一冲突 errors.Is 透出。
func (s *Store) Create(ctx context.Context, t *tododomain.Todo) error {
	if err := s.db.WithContext(ctx).Create(t).Error; err != nil {
		return fmt.Errorf("todostore.Create: %w", err)
	}
	return nil
}

// Get fetches by id; ErrNotFound when absent or soft-deleted.
//
// Get 按 id 取；不存在或软删返 ErrNotFound。
func (s *Store) Get(ctx context.Context, id string) (*tododomain.Todo, error) {
	var t tododomain.Todo
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, tododomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("todostore.Get: %w", err)
	}
	return &t, nil
}

// ListByConversation returns active todos for a conversation, created_at ASC (creation order).
//
// ListByConversation 返某对话活跃 todo，created_at 升序（创建顺序）。
func (s *Store) ListByConversation(ctx context.Context, conversationID string) ([]*tododomain.Todo, error) {
	var rows []*tododomain.Todo
	err := s.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("created_at ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("todostore.ListByConversation: %w", err)
	}
	return rows, nil
}

// Update writes the todo back via GORM Save (updates all fields including zero values).
//
// Update 用 GORM Save 写回 todo（会更新所有字段含零值）。
func (s *Store) Update(ctx context.Context, t *tododomain.Todo) error {
	if err := s.db.WithContext(ctx).Save(t).Error; err != nil {
		return fmt.Errorf("todostore.Update: %w", err)
	}
	return nil
}

// SoftDelete sets deleted_at; 0-row treated as ErrNotFound (not silent success).
//
// SoftDelete 设 deleted_at；0 行视为 ErrNotFound（不静默成功）。
func (s *Store) SoftDelete(ctx context.Context, id string) error {
	res := s.db.WithContext(ctx).Where("id = ?", id).Delete(&tododomain.Todo{})
	if res.Error != nil {
		return fmt.Errorf("todostore.SoftDelete: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return tododomain.ErrNotFound
	}
	return nil
}

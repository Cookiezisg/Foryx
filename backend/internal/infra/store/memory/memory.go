// Package memory is the GORM-backed memorydomain.Repository (global, no userID scoping).
//
// Package memory 是 memorydomain.Repository 的 GORM 实现（全局，无 userID 作用域）。
package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	memorydomain "github.com/sunweilin/forgify/backend/internal/domain/memory"
)

// Store is the GORM implementation of memorydomain.Repository.
//
// Store 是 memorydomain.Repository 的 GORM 实现。
type Store struct {
	db *gorm.DB
}

// New constructs a Store bound to the given *gorm.DB.
//
// New 基于给定 *gorm.DB 构造 Store。
func New(db *gorm.DB) *Store { return &Store{db: db} }

var _ memorydomain.Repository = (*Store)(nil)

// AutoMigrateModels returns the GORM models to register in db.AutoMigrate.
//
// AutoMigrateModels 返 AutoMigrate 用的 GORM models。
func AutoMigrateModels() []interface{} {
	return []interface{}{&memorydomain.Memory{}}
}

func (s *Store) Save(ctx context.Context, m *memorydomain.Memory) error {
	if err := s.db.WithContext(ctx).Save(m).Error; err != nil {
		if isMemoryDuplicateName(err) {
			return memorydomain.ErrNameConflict
		}
		return fmt.Errorf("memorystore.Save: %w", err)
	}
	return nil
}

func (s *Store) GetByName(ctx context.Context, name string) (*memorydomain.Memory, error) {
	var m memorydomain.Memory
	err := s.db.WithContext(ctx).Where("name = ?", name).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, memorydomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("memorystore.GetByName: %w", err)
	}
	return &m, nil
}

func (s *Store) GetByID(ctx context.Context, id string) (*memorydomain.Memory, error) {
	var m memorydomain.Memory
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, memorydomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("memorystore.GetByID: %w", err)
	}
	return &m, nil
}

func (s *Store) List(ctx context.Context, filter memorydomain.ListFilter) ([]*memorydomain.Memory, error) {
	q := s.db.WithContext(ctx)
	if filter.Type != "" {
		q = q.Where("type = ?", filter.Type)
	}
	if filter.Pinned != nil {
		q = q.Where("pinned = ?", *filter.Pinned)
	}
	var rows []*memorydomain.Memory
	if err := q.Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("memorystore.List: %w", err)
	}
	return rows, nil
}

func (s *Store) ListPinned(ctx context.Context) ([]*memorydomain.Memory, error) {
	var rows []*memorydomain.Memory
	if err := s.db.WithContext(ctx).
		Where("pinned = ?", true).
		Order("updated_at DESC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("memorystore.ListPinned: %w", err)
	}
	return rows, nil
}

func (s *Store) ListForIndex(ctx context.Context, limit int) ([]*memorydomain.Memory, error) {
	if limit <= 0 {
		limit = 200
	}
	var rows []*memorydomain.Memory
	// Non-pinned only (pinned injected in full elsewhere); sort accessed_at/access_count/updated_at DESC.
	// 仅非 pinned（pinned 全文已注入）；按 accessed_at/access_count/updated_at DESC 排序。
	if err := s.db.WithContext(ctx).
		Where("pinned = ?", false).
		Order("accessed_at IS NULL, accessed_at DESC, access_count DESC, updated_at DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("memorystore.ListForIndex: %w", err)
	}
	return rows, nil
}

func (s *Store) MarkAccessed(ctx context.Context, name string) error {
	now := time.Now().UTC()
	res := s.db.WithContext(ctx).
		Model(&memorydomain.Memory{}).
		Where("name = ?", name).
		Updates(map[string]any{
			"accessed_at":  &now,
			"access_count": gorm.Expr("access_count + 1"),
		})
	if res.Error != nil {
		return fmt.Errorf("memorystore.MarkAccessed: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return memorydomain.ErrNotFound
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, name string) error {
	res := s.db.WithContext(ctx).
		Where("name = ?", name).
		Delete(&memorydomain.Memory{})
	if res.Error != nil {
		return fmt.Errorf("memorystore.Delete: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return memorydomain.ErrNotFound
	}
	return nil
}

func isMemoryDuplicateName(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") &&
		(strings.Contains(msg, "memories.name") || strings.Contains(msg, "idx_memories_name_active"))
}

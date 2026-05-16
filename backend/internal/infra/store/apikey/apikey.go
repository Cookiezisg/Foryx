// Package apikey is the GORM-backed Repository port for domain apikey, scoped by ctx userID.
//
// Package apikey 是 domain apikey Repository 的 GORM 实现，按 ctx userID 过滤。
package apikey

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Store is the GORM implementation of ports.Repository for APIKey.
//
// Store 是 APIKey Repository 的 GORM 实现。
type Store struct {
	db *gorm.DB
}

// New constructs a Store bound to the given *gorm.DB.
//
// New 基于给定 *gorm.DB 构造 Store。
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Get fetches one APIKey by id; returns ErrNotFound if no live row matches.
//
// Get 按 id 取单条 APIKey；未命中返回 ErrNotFound。
func (s *Store) Get(ctx context.Context, id string) (*apikeydomain.APIKey, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var k apikeydomain.APIKey
	err = s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, uid).
		First(&k).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apikeydomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("apikeystore.Get: %w", err)
	}
	return &k, nil
}

// List returns a page of keys; tuple cursor (created_at, id) keeps pagination stable.
//
// List 分页返回 keys；(created_at, id) 元组 cursor 保证稳定分页。
func (s *Store) List(ctx context.Context, filter apikeydomain.ListFilter) ([]*apikeydomain.APIKey, string, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, "", err
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	q := s.db.WithContext(ctx).Where("user_id = ?", uid)
	if filter.Provider != "" {
		q = q.Where("provider = ?", filter.Provider)
	}
	if filter.Cursor != "" {
		var c paginationpkg.Cursor
		if err := paginationpkg.DecodeCursor(filter.Cursor, &c); err != nil {
			return nil, "", fmt.Errorf("apikeystore.List: %w", err)
		}
		q = q.Where("(created_at, id) < (?, ?)", c.CreatedAt, c.ID)
	}

	var rows []*apikeydomain.APIKey
	if err := q.Order("created_at DESC, id DESC").
		Limit(limit + 1).
		Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("apikeystore.List: %w", err)
	}

	var next string
	if len(rows) > limit {
		last := rows[limit-1]
		next, err = paginationpkg.EncodeCursor(paginationpkg.Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return nil, "", fmt.Errorf("apikeystore.List: %w", err)
		}
		rows = rows[:limit]
	}
	return rows, next, nil
}

// GetByProvider picks the most usable key by (ok > pending, last_tested DESC, created DESC).
//
// GetByProvider 按 (ok 优先, last_tested DESC, created DESC) 挑最合适的 key。
func (s *Store) GetByProvider(ctx context.Context, provider string) (*apikeydomain.APIKey, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var k apikeydomain.APIKey
	err = s.db.WithContext(ctx).
		Where("user_id = ? AND provider = ?", uid, provider).
		Order("CASE WHEN test_status = 'ok' THEN 0 ELSE 1 END").
		Order("last_tested_at DESC").
		Order("created_at DESC").
		First(&k).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apikeydomain.ErrNotFoundForProvider
	}
	if err != nil {
		return nil, fmt.Errorf("apikeystore.GetByProvider: %w", err)
	}
	return &k, nil
}

// Save upserts on primary key; caller must set k.UserID first.
//
// Save 按主键 upsert；调用方需先设置 k.UserID。
func (s *Store) Save(ctx context.Context, k *apikeydomain.APIKey) error {
	if err := s.db.WithContext(ctx).Save(k).Error; err != nil {
		return fmt.Errorf("apikeystore.Save: %w", err)
	}
	return nil
}

// Delete soft-deletes by id; returns ErrNotFound on no-match (so retries don't silently succeed).
//
// Delete 按 id 软删除；未命中返回 ErrNotFound（避免静默成功）。
func (s *Store) Delete(ctx context.Context, id string) error {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return err
	}
	res := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, uid).
		Delete(&apikeydomain.APIKey{})
	if res.Error != nil {
		return fmt.Errorf("apikeystore.Delete: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apikeydomain.ErrNotFound
	}
	return nil
}

// UpdateTestResult writes only test_status / test_error / last_tested_at / models_found.
//
// UpdateTestResult 仅写测试相关字段，避免整行往返。
func (s *Store) UpdateTestResult(ctx context.Context, id, status, errMsg string, models []string) error {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	modelsJSON := "[]"
	if len(models) > 0 {
		b, _ := json.Marshal(models)
		modelsJSON = string(b)
	}
	res := s.db.WithContext(ctx).
		Model(&apikeydomain.APIKey{}).
		Where("id = ? AND user_id = ?", id, uid).
		Updates(map[string]any{
			"test_status":    status,
			"test_error":     errMsg,
			"last_tested_at": &now,
			"models_found":   modelsJSON,
		})
	if res.Error != nil {
		return fmt.Errorf("apikeystore.UpdateTestResult: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apikeydomain.ErrNotFound
	}
	return nil
}

// Package apikey (infra/store/apikey) is the GORM-backed implementation
// of the domain apikey Repository port. Every method scopes queries by
// the userID carried in ctx — callers MUST have run the InjectUserID
// middleware.
//
// The package shares its name with domain/apikey by design; external
// callers alias at import: `apikeystore "…/infra/store/apikey"`.
//
// Package apikey（infra/store/apikey）是 domain apikey Repository port
// 的 GORM 实现。所有方法按 ctx 中的 userID 过滤——调用方必须先经过
// InjectUserID 中间件。
//
// 本包与 domain/apikey 同名是刻意的；外部调用方 import 时起别名，
// 例如 `apikeystore "…/infra/store/apikey"`。
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
// Store 是 APIKey 的 ports.Repository 的 GORM 实现。
type Store struct {
	db *gorm.DB
}

// New constructs a Store bound to the given *gorm.DB.
//
// New 基于给定 *gorm.DB 构造 Store。
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Get fetches a single APIKey by id, scoped to the caller. Returns
// apikeydomain.ErrNotFound if no live row matches.
//
// Get 按 id 查单条 APIKey，按调用者过滤。未命中活跃行返回 apikeydomain.ErrNotFound。
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

// List returns a page of keys for the caller, ordered created_at DESC
// with id as tiebreaker. Uses a tuple cursor over (created_at, id) so
// pagination is stable even across identical timestamps.
//
// List 返回调用者的一页 Key，按 created_at DESC（以 id 作 tiebreaker）排序。
// cursor 是 (created_at, id) 元组，保证时间戳相同也稳定分页。
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
		// Tuple comparison: rows strictly "older" than the cursor position.
		// 元组比较：严格比 cursor 位置"更旧"的行。
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

// GetByProvider picks the most suitable live APIKey for (user, provider).
// Ordering:
//  1. test_status = 'ok' preferred
//  2. last_tested_at DESC (NULLs last)
//  3. created_at DESC
//
// Returns apikeydomain.ErrNotFoundForProvider if no row exists.
//
// GetByProvider 为 (user, provider) 挑选**最适合**的活跃 APIKey。排序：
//  1. test_status = 'ok' 优先
//  2. last_tested_at DESC（NULL 排最后）
//  3. created_at DESC
//
// 未命中返回 apikeydomain.ErrNotFoundForProvider。
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

// Save upserts on primary key. Caller must have set k.UserID (usually
// from ctx) before calling.
//
// Save 按主键 upsert。调用方需先设置 k.UserID（通常从 ctx 取）。
func (s *Store) Save(ctx context.Context, k *apikeydomain.APIKey) error {
	if err := s.db.WithContext(ctx).Save(k).Error; err != nil {
		return fmt.Errorf("apikeystore.Save: %w", err)
	}
	return nil
}

// Delete soft-deletes by id, scoped to the caller. Returns apikeydomain.ErrNotFound
// when no live row was matched (so retries don't silently succeed).
//
// Delete 按 id 软删除，按调用者过滤。未命中活跃行返回 apikeydomain.ErrNotFound
// （让重试不会静默成功）。
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

// UpdateTestResult writes only test_status / test_error / last_tested_at.
// Used by Service.Test and MarkInvalid to avoid round-tripping the whole row.
//
// UpdateTestResult 仅写 test_status / test_error / last_tested_at。
// Service.Test 和 MarkInvalid 用来避免整行往返。
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

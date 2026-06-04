// Package apikey is the orm-backed implementation of apikeydomain.Repository.
// Workspace isolation is automatic (orm fills/filters workspace_id), so no
// method writes a hand-rolled `WHERE workspace_id = ?`.
//
// Package apikey 是 apikeydomain.Repository 的 orm 实现。workspace 隔离由 orm 自动完成
// （填/过滤 workspace_id），故无方法手写 `WHERE workspace_id = ?`。
package apikey

import (
	"context"
	"errors"
	"fmt"
	"time"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
)

// Schema is the api_keys DDL — exported as ordered idempotent statements for
// cmd/server to collect and apply via db.Migrate. display_name is unique per
// workspace (partial index, active rows only) so a soft-deleted name frees up.
//
// Schema 是 api_keys 表 DDL，按序幂等语句导出，由 cmd/server 汇总经 db.Migrate 应用。
// display_name 按 workspace 唯一（partial index 仅活跃行），软删名可重用。
var Schema = []string{
	`CREATE TABLE IF NOT EXISTS api_keys (
		id             TEXT PRIMARY KEY,
		workspace_id   TEXT NOT NULL,
		provider       TEXT NOT NULL,
		display_name   TEXT NOT NULL DEFAULT '',
		key_encrypted  TEXT NOT NULL,
		key_masked     TEXT NOT NULL DEFAULT '',
		base_url       TEXT NOT NULL DEFAULT '',
		api_format     TEXT NOT NULL DEFAULT '',
		test_status    TEXT NOT NULL DEFAULT 'pending',
		test_error     TEXT NOT NULL DEFAULT '',
		test_response  TEXT NOT NULL DEFAULT '',
		last_tested_at DATETIME,
		created_at     DATETIME NOT NULL,
		updated_at     DATETIME NOT NULL,
		deleted_at     DATETIME
	)`,
	`CREATE INDEX IF NOT EXISTS idx_api_keys_ws_provider ON api_keys(workspace_id, provider) WHERE deleted_at IS NULL`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_ws_displayname ON api_keys(workspace_id, display_name) WHERE deleted_at IS NULL`,
}

// Store implements apikeydomain.Repository over pkg/orm.
//
// Store 基于 pkg/orm 实现 apikeydomain.Repository。
type Store struct {
	repo *ormpkg.Repo[apikeydomain.APIKey]
}

// New builds a Store bound to the api_keys table.
//
// New 构造绑定 api_keys 表的 Store。
func New(db *ormpkg.DB) *Store {
	return &Store{repo: ormpkg.For[apikeydomain.APIKey](db, "api_keys")}
}

var _ apikeydomain.Repository = (*Store)(nil)

func (s *Store) Get(ctx context.Context, id string) (*apikeydomain.APIKey, error) {
	k, err := s.repo.Get(ctx, id)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, apikeydomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("apikeystore.Get: %w", err)
	}
	return k, nil
}

// List pages keys newest-first; an optional provider filter narrows the set.
//
// List 按最新优先分页；可选 provider 过滤收窄结果。
func (s *Store) List(ctx context.Context, filter apikeydomain.ListFilter) ([]*apikeydomain.APIKey, string, error) {
	q := s.repo.Query()
	if filter.Provider != "" {
		q = q.WhereEq("provider", filter.Provider)
	}
	rows, next, err := q.Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("apikeystore.List: %w", err)
	}
	return rows, next, nil
}

// Save upserts; a duplicate display_name (UNIQUE index) surfaces as
// ErrDisplayNameConflict — the orm gateway translated the SQLite violation.
//
// Save upsert；重名 display_name（UNIQUE 索引）冒泡为 ErrDisplayNameConflict——orm 网关已译。
func (s *Store) Save(ctx context.Context, k *apikeydomain.APIKey) error {
	if err := s.repo.Save(ctx, k); err != nil {
		if errors.Is(err, ormpkg.ErrConflict) {
			return apikeydomain.ErrDisplayNameConflict
		}
		return fmt.Errorf("apikeystore.Save: %w", err)
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	found, err := s.repo.Delete(ctx, id)
	if err != nil {
		return fmt.Errorf("apikeystore.Delete: %w", err)
	}
	if !found {
		return apikeydomain.ErrNotFound
	}
	return nil
}

// UpdateTestResult writes only the probe-outcome fields (orm bumps updated_at).
//
// UpdateTestResult 仅写探测结果字段（orm 自动 bump updated_at）。
func (s *Store) UpdateTestResult(ctx context.Context, id, status, errMsg, response string) error {
	now := time.Now().UTC()
	n, err := s.repo.WhereEq("id", id).Updates(ctx, map[string]any{
		"test_status":    status,
		"test_error":     errMsg,
		"test_response":  response,
		"last_tested_at": &now,
	})
	if err != nil {
		return fmt.Errorf("apikeystore.UpdateTestResult: %w", err)
	}
	if n == 0 {
		return apikeydomain.ErrNotFound
	}
	return nil
}

// ListProbed returns the probe archive (provider + status + raw response) of
// every active key in the workspace, for the model module to parse.
//
// ListProbed 返回本 workspace 每把活跃 key 的探测档案（provider + 状态 + 原始返回），供 model 解析。
func (s *Store) ListProbed(ctx context.Context) ([]apikeydomain.ProbedKey, error) {
	rows, err := s.repo.Query().Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("apikeystore.ListProbed: %w", err)
	}
	out := make([]apikeydomain.ProbedKey, 0, len(rows))
	for _, k := range rows {
		out = append(out, apikeydomain.ProbedKey{
			ID:           k.ID,
			DisplayName:  k.DisplayName,
			Provider:     k.Provider,
			TestStatus:   k.TestStatus,
			TestResponse: k.TestResponse,
		})
	}
	return out, nil
}

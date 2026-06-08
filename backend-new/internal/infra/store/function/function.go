// Package function is the orm-backed implementation of functiondomain.Repository:
// functions (soft-deleted) + function_versions (append-only, cap-trimmed) +
// function_executions (append-only log). Workspace isolation is automatic (orm fills
// /filters workspace_id from ctx via the ,ws tag), so no method hand-writes a
// workspace predicate.
//
// Package function 是 functiondomain.Repository 的 orm 实现：functions（软删）+
// function_versions（只增、按上限裁剪）+ function_executions（只增 log）。workspace 隔离
// 自动（orm 据 ctx 经 ,ws tag 填/过滤 workspace_id），故无方法手写 workspace 谓词。
package function

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
)

// Schema is the function tables' DDL, exported as ordered idempotent statements for
// cmd/server to collect and apply via db.Migrate. functions has a partial-UNIQUE name
// (freed on soft-delete); versions are UNIQUE(function_id, version); executions are an
// append-only log (no deleted_at — D1) with CHECK-constrained status / triggered_by.
//
// Schema 是 function 三表 DDL，按序幂等导出，由 cmd/server 汇总经 db.Migrate 应用。functions
// 用 partial-UNIQUE name（软删后释放）；versions UNIQUE(function_id, version)；executions 是
// 只增 log（无 deleted_at——D1），status / triggered_by 带 CHECK。
var Schema = []string{
	`CREATE TABLE IF NOT EXISTS functions (
		id                TEXT PRIMARY KEY,
		workspace_id      TEXT NOT NULL,
		name              TEXT NOT NULL,
		description       TEXT NOT NULL DEFAULT '',
		tags              TEXT NOT NULL DEFAULT '[]',
		active_version_id TEXT NOT NULL DEFAULT '',
		created_at        DATETIME NOT NULL,
		updated_at        DATETIME NOT NULL,
		deleted_at        DATETIME
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_functions_ws_name ON functions(workspace_id, name) WHERE deleted_at IS NULL`,
	`CREATE INDEX IF NOT EXISTS idx_functions_ws_created ON functions(workspace_id, created_at DESC, id DESC) WHERE deleted_at IS NULL`,

	`CREATE TABLE IF NOT EXISTS function_versions (
		id                        TEXT PRIMARY KEY,
		workspace_id              TEXT NOT NULL,
		function_id               TEXT NOT NULL,
		version                   INTEGER NOT NULL,
		code                      TEXT NOT NULL DEFAULT '',
		inputs                    TEXT NOT NULL DEFAULT '[]',
		outputs                   TEXT NOT NULL DEFAULT '[]',
		dependencies              TEXT NOT NULL DEFAULT '[]',
		python_version            TEXT NOT NULL DEFAULT '',
		env_id                    TEXT NOT NULL DEFAULT '',
		env_status                TEXT NOT NULL DEFAULT 'pending',
		env_error                 TEXT NOT NULL DEFAULT '',
		env_synced_at             DATETIME,
		change_reason             TEXT NOT NULL DEFAULT '',
		forged_in_conversation_id TEXT,
		created_at                DATETIME NOT NULL,
		updated_at                DATETIME NOT NULL
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_fnv_function_version ON function_versions(function_id, version)`,
	`CREATE INDEX IF NOT EXISTS idx_fnv_function_created ON function_versions(function_id, created_at DESC, id DESC)`,

	`CREATE TABLE IF NOT EXISTS function_executions (
		id              TEXT PRIMARY KEY,
		workspace_id    TEXT NOT NULL,
		function_id     TEXT NOT NULL,
		version_id      TEXT NOT NULL,
		status          TEXT NOT NULL CHECK (status IN ('ok','failed','cancelled','timeout')),
		triggered_by    TEXT NOT NULL CHECK (triggered_by IN ('chat','agent','workflow','manual')),
		input           TEXT NOT NULL DEFAULT '{}',
		output          TEXT,
		error_message   TEXT NOT NULL DEFAULT '',
		elapsed_ms      INTEGER NOT NULL DEFAULT 0,
		started_at      DATETIME NOT NULL,
		ended_at        DATETIME NOT NULL,
		conversation_id TEXT NOT NULL DEFAULT '',
		message_id      TEXT NOT NULL DEFAULT '',
		tool_call_id    TEXT NOT NULL DEFAULT '',
		flowrun_id      TEXT NOT NULL DEFAULT '',
		flowrun_node_id TEXT NOT NULL DEFAULT '',
		created_at      DATETIME NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_fne_ws_function ON function_executions(workspace_id, function_id, created_at DESC, id DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_fne_ws_conversation ON function_executions(workspace_id, conversation_id) WHERE conversation_id != ''`,
	`CREATE INDEX IF NOT EXISTS idx_fne_ws_flowrun ON function_executions(workspace_id, flowrun_id) WHERE flowrun_id != ''`,
}

// Store implements functiondomain.Repository over pkg/orm.
//
// Store 基于 pkg/orm 实现 functiondomain.Repository。
type Store struct {
	db    *ormpkg.DB
	fns   *ormpkg.Repo[functiondomain.Function]
	vers  *ormpkg.Repo[functiondomain.Version]
	execs *ormpkg.Repo[functiondomain.Execution]
}

// New constructs a Store bound to the three function tables.
//
// New 构造绑定 function 三表的 Store。
func New(db *ormpkg.DB) *Store {
	return &Store{
		db:    db,
		fns:   ormpkg.For[functiondomain.Function](db, "functions"),
		vers:  ormpkg.For[functiondomain.Version](db, "function_versions"),
		execs: ormpkg.For[functiondomain.Execution](db, "function_executions"),
	}
}

var _ functiondomain.Repository = (*Store)(nil)

// --- functions -------------------------------------------------------------

func (s *Store) SaveFunction(ctx context.Context, f *functiondomain.Function) error {
	if err := s.fns.Save(ctx, f); err != nil {
		if errors.Is(err, ormpkg.ErrConflict) {
			return functiondomain.ErrDuplicateName
		}
		return fmt.Errorf("functionstore.SaveFunction: %w", err)
	}
	return nil
}

func (s *Store) GetFunction(ctx context.Context, id string) (*functiondomain.Function, error) {
	f, err := s.fns.Get(ctx, id)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, functiondomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("functionstore.GetFunction: %w", err)
	}
	return f, nil
}

func (s *Store) GetFunctionByName(ctx context.Context, name string) (*functiondomain.Function, error) {
	f, err := s.fns.WhereEq("name", name).First(ctx)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, functiondomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("functionstore.GetFunctionByName: %w", err)
	}
	return f, nil
}

func (s *Store) GetFunctionsByIDs(ctx context.Context, ids []string) ([]*functiondomain.Function, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.fns.WhereIn("id", toAny(ids)...).Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("functionstore.GetFunctionsByIDs: %w", err)
	}
	byID := make(map[string]*functiondomain.Function, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	out := make([]*functiondomain.Function, 0, len(ids))
	for _, id := range ids {
		if r, ok := byID[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Store) ListFunctions(ctx context.Context, filter functiondomain.ListFilter) ([]*functiondomain.Function, string, error) {
	rows, next, err := s.fns.Query().Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("functionstore.ListFunctions: %w", err)
	}
	return rows, next, nil
}

func (s *Store) ListAllFunctions(ctx context.Context) ([]*functiondomain.Function, error) {
	rows, err := s.fns.Order("created_at DESC, id DESC").Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("functionstore.ListAllFunctions: %w", err)
	}
	return rows, nil
}

func (s *Store) DeleteFunction(ctx context.Context, id string) error {
	ok, err := s.fns.Delete(ctx, id) // soft-delete (functions has deleted_at)
	if err != nil {
		return fmt.Errorf("functionstore.DeleteFunction: %w", err)
	}
	if !ok {
		return functiondomain.ErrNotFound
	}
	return nil
}

func (s *Store) SetActiveVersion(ctx context.Context, functionID, versionID string) error {
	n, err := s.fns.WhereEq("id", functionID).Update(ctx, "active_version_id", versionID)
	if err != nil {
		return fmt.Errorf("functionstore.SetActiveVersion: %w", err)
	}
	if n == 0 {
		return functiondomain.ErrNotFound
	}
	return nil
}

// --- versions --------------------------------------------------------------

func (s *Store) SaveVersion(ctx context.Context, v *functiondomain.Version) error {
	if err := s.vers.Save(ctx, v); err != nil {
		return fmt.Errorf("functionstore.SaveVersion: %w", err)
	}
	return nil
}

func (s *Store) GetVersion(ctx context.Context, versionID string) (*functiondomain.Version, error) {
	v, err := s.vers.Get(ctx, versionID)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, functiondomain.ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("functionstore.GetVersion: %w", err)
	}
	return v, nil
}

func (s *Store) GetVersionByNumber(ctx context.Context, functionID string, versionN int) (*functiondomain.Version, error) {
	v, err := s.vers.WhereEq("function_id", functionID).WhereEq("version", versionN).First(ctx)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, functiondomain.ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("functionstore.GetVersionByNumber: %w", err)
	}
	return v, nil
}

func (s *Store) ListVersions(ctx context.Context, functionID string, filter functiondomain.VersionListFilter) ([]*functiondomain.Version, string, error) {
	rows, next, err := s.vers.WhereEq("function_id", functionID).Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("functionstore.ListVersions: %w", err)
	}
	return rows, next, nil
}

func (s *Store) MaxVersionNumber(ctx context.Context, functionID string) (int, error) {
	var nums []int
	if err := s.vers.WhereEq("function_id", functionID).Order("version DESC").Limit(1).Pluck(ctx, "version", &nums); err != nil {
		return 0, fmt.Errorf("functionstore.MaxVersionNumber: %w", err)
	}
	if len(nums) == 0 {
		return 0, nil
	}
	return nums[0], nil
}

// UpdateVersionEnv writes env terminal state + the (possibly corrected) dep list. deps
// is JSON-marshalled here because Updates passes raw values straight to the driver
// (the orm only serialises ,json fields on Create/Save, not on a fields-map update).
//
// UpdateVersionEnv 写 env 终态 + （可能被修正的）依赖列表。deps 在此手工 JSON 序列化，因为
// Updates 把原始值直送 driver（orm 仅在 Create/Save 序列化 ,json 字段，不在 fields-map 更新时）。
func (s *Store) UpdateVersionEnv(ctx context.Context, versionID, envStatus, envError string, deps []string, syncedAt *time.Time) error {
	if deps == nil {
		deps = []string{}
	}
	depsJSON, err := json.Marshal(deps)
	if err != nil {
		return fmt.Errorf("functionstore.UpdateVersionEnv: marshal deps: %w", err)
	}
	fields := map[string]any{
		"env_status":    envStatus,
		"env_error":     envError,
		"dependencies":  string(depsJSON),
		"env_synced_at": syncedAt,
	}
	n, err := s.vers.WhereEq("id", versionID).Updates(ctx, fields)
	if err != nil {
		return fmt.Errorf("functionstore.UpdateVersionEnv: %w", err)
	}
	if n == 0 {
		return functiondomain.ErrVersionNotFound
	}
	return nil
}

// TrimOldestVersions hard-deletes versions below the keep-th newest version number,
// always sparing the function's active version (which may be old after a revert).
//
// TrimOldestVersions 硬删低于第 keep 新版本号的版本，始终放过 function 的 active 版本
// （revert 后它可能很老）。
func (s *Store) TrimOldestVersions(ctx context.Context, functionID string, keep int) error {
	if keep <= 0 {
		keep = functiondomain.VersionCap
	}
	var nums []int
	if err := s.vers.WhereEq("function_id", functionID).Order("version DESC").Pluck(ctx, "version", &nums); err != nil {
		return fmt.Errorf("functionstore.TrimOldestVersions: %w", err)
	}
	if len(nums) <= keep {
		return nil
	}
	cutoff := nums[keep-1] // keep versions with number >= cutoff (the keep newest)

	f, err := s.fns.Get(ctx, functionID)
	if err != nil {
		return fmt.Errorf("functionstore.TrimOldestVersions: load active: %w", err)
	}
	if _, err := s.vers.
		WhereEq("function_id", functionID).
		Where("version < ?", cutoff).
		Where("id != ?", f.ActiveVersionID).
		Delete(ctx); err != nil { // hard-delete: function_versions has no deleted_at
		return fmt.Errorf("functionstore.TrimOldestVersions: %w", err)
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

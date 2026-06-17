// Package handler is the orm-backed implementation of handlerdomain.Repository:
// handlers (soft-deleted, with encrypted config) + handler_versions (append-only,
// cap-trimmed) + handler_calls (append-only log). Workspace isolation is automatic
// (orm ,ws tag).
//
// Package handler 是 handlerdomain.Repository 的 orm 实现：handlers（软删，带加密 config）+
// handler_versions（只增、按上限裁剪）+ handler_calls（只增 log）。workspace 隔离自动（orm ,ws tag）。
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	handlerdomain "github.com/sunweilin/anselm/backend/internal/domain/handler"
	ormpkg "github.com/sunweilin/anselm/backend/internal/pkg/orm"
)

// Schema is the handler tables' DDL (idempotent, ordered) for bootstrap to collect via
// db.Migrate. handlers carry config_encrypted (init-args values); versions are
// UNIQUE(handler_id, version); calls are an append-only log (no deleted_at — D1).
//
// Schema 是 handler 三表 DDL（幂等、按序），由 bootstrap 汇总经 db.Migrate 应用。handlers 带
// config_encrypted（init-args 值）；versions UNIQUE(handler_id, version)；calls 只增 log（无 deleted_at——D1）。
var Schema = []string{
	`CREATE TABLE IF NOT EXISTS handlers (
		id                TEXT PRIMARY KEY,
		workspace_id      TEXT NOT NULL,
		name              TEXT NOT NULL,
		description       TEXT NOT NULL DEFAULT '',
		tags              TEXT NOT NULL DEFAULT '[]',
		active_version_id TEXT NOT NULL DEFAULT '',
		config_encrypted  TEXT NOT NULL DEFAULT '',
		created_at        DATETIME NOT NULL,
		updated_at        DATETIME NOT NULL,
		deleted_at        DATETIME
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_handlers_ws_name ON handlers(workspace_id, name) WHERE deleted_at IS NULL`,
	`CREATE INDEX IF NOT EXISTS idx_handlers_ws_created ON handlers(workspace_id, created_at DESC, id DESC) WHERE deleted_at IS NULL`,

	`CREATE TABLE IF NOT EXISTS handler_versions (
		id                        TEXT PRIMARY KEY,
		workspace_id              TEXT NOT NULL,
		handler_id                TEXT NOT NULL,
		version                   INTEGER NOT NULL,
		imports                   TEXT NOT NULL DEFAULT '',
		init_body                 TEXT NOT NULL DEFAULT '',
		shutdown_body             TEXT NOT NULL DEFAULT '',
		methods                   TEXT NOT NULL DEFAULT '[]',
		init_args_schema          TEXT NOT NULL DEFAULT '[]',
		dependencies              TEXT NOT NULL DEFAULT '[]',
		python_version            TEXT NOT NULL DEFAULT '',
		env_id                    TEXT NOT NULL DEFAULT '',
		env_status                TEXT NOT NULL DEFAULT 'pending',
		env_error                 TEXT NOT NULL DEFAULT '',
		env_synced_at             DATETIME,
		change_reason             TEXT NOT NULL DEFAULT '',
		built_in_conversation_id TEXT,
		created_at                DATETIME NOT NULL,
		updated_at                DATETIME NOT NULL
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_hdv_handler_version ON handler_versions(handler_id, version)`,
	`CREATE INDEX IF NOT EXISTS idx_hdv_handler_created ON handler_versions(handler_id, created_at DESC, id DESC)`,

	`CREATE TABLE IF NOT EXISTS handler_calls (
		id              TEXT PRIMARY KEY,
		workspace_id    TEXT NOT NULL,
		handler_id      TEXT NOT NULL,
		version_id      TEXT NOT NULL,
		method          TEXT NOT NULL,
		status          TEXT NOT NULL CHECK (status IN ('ok','failed','cancelled','timeout')),
		triggered_by    TEXT NOT NULL CHECK (triggered_by IN ('chat','agent','workflow','manual')),
		input           TEXT NOT NULL DEFAULT '{}',
		output          TEXT,
		error_message   TEXT NOT NULL DEFAULT '',
		logs            TEXT NOT NULL DEFAULT '',
		elapsed_ms      INTEGER NOT NULL DEFAULT 0,
		started_at      DATETIME NOT NULL,
		ended_at        DATETIME NOT NULL,
		instance_id     TEXT NOT NULL DEFAULT '',
		conversation_id TEXT NOT NULL DEFAULT '',
		message_id      TEXT NOT NULL DEFAULT '',
		tool_call_id    TEXT NOT NULL DEFAULT '',
		flowrun_id      TEXT NOT NULL DEFAULT '',
		flowrun_node_id TEXT NOT NULL DEFAULT '',
		created_at      DATETIME NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_hcl_ws_handler ON handler_calls(workspace_id, handler_id, created_at DESC, id DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_hcl_ws_conversation ON handler_calls(workspace_id, conversation_id) WHERE conversation_id != ''`,
	`CREATE INDEX IF NOT EXISTS idx_hcl_ws_flowrun ON handler_calls(workspace_id, flowrun_id) WHERE flowrun_id != ''`,
}

// Store implements handlerdomain.Repository over pkg/orm.
type Store struct {
	db    *ormpkg.DB
	hdls  *ormpkg.Repo[handlerdomain.Handler]
	vers  *ormpkg.Repo[handlerdomain.Version]
	calls *ormpkg.Repo[handlerdomain.Call]
}

// New constructs a Store bound to the three handler tables.
func New(db *ormpkg.DB) *Store {
	return &Store{
		db:    db,
		hdls:  ormpkg.For[handlerdomain.Handler](db, "handlers"),
		vers:  ormpkg.For[handlerdomain.Version](db, "handler_versions"),
		calls: ormpkg.For[handlerdomain.Call](db, "handler_calls"),
	}
}

var _ handlerdomain.Repository = (*Store)(nil)

// --- handlers --------------------------------------------------------------

func (s *Store) SaveHandler(ctx context.Context, h *handlerdomain.Handler) error {
	if err := s.hdls.Save(ctx, h); err != nil {
		if errors.Is(err, ormpkg.ErrConflict) {
			return handlerdomain.ErrDuplicateName
		}
		return fmt.Errorf("handlerstore.SaveHandler: %w", err)
	}
	return nil
}

func (s *Store) GetHandler(ctx context.Context, id string) (*handlerdomain.Handler, error) {
	h, err := s.hdls.Get(ctx, id)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, handlerdomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("handlerstore.GetHandler: %w", err)
	}
	return h, nil
}

func (s *Store) GetHandlerByName(ctx context.Context, name string) (*handlerdomain.Handler, error) {
	h, err := s.hdls.WhereEq("name", name).First(ctx)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, handlerdomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("handlerstore.GetHandlerByName: %w", err)
	}
	return h, nil
}

func (s *Store) GetHandlersByIDs(ctx context.Context, ids []string) ([]*handlerdomain.Handler, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.hdls.WhereIn("id", toAny(ids)...).Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("handlerstore.GetHandlersByIDs: %w", err)
	}
	byID := make(map[string]*handlerdomain.Handler, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	out := make([]*handlerdomain.Handler, 0, len(ids))
	for _, id := range ids {
		if r, ok := byID[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Store) ListHandlers(ctx context.Context, filter handlerdomain.ListFilter) ([]*handlerdomain.Handler, string, error) {
	rows, next, err := s.hdls.Query().Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("handlerstore.ListHandlers: %w", err)
	}
	return rows, next, nil
}

func (s *Store) ListAllHandlers(ctx context.Context) ([]*handlerdomain.Handler, error) {
	rows, err := s.hdls.Order("created_at DESC, id DESC").Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("handlerstore.ListAllHandlers: %w", err)
	}
	return rows, nil
}

func (s *Store) DeleteHandler(ctx context.Context, id string) error {
	ok, err := s.hdls.Delete(ctx, id) // soft-delete
	if err != nil {
		return fmt.Errorf("handlerstore.DeleteHandler: %w", err)
	}
	if !ok {
		return handlerdomain.ErrNotFound
	}
	return nil
}

func (s *Store) SetActiveVersion(ctx context.Context, handlerID, versionID string) error {
	n, err := s.hdls.WhereEq("id", handlerID).Update(ctx, "active_version_id", versionID)
	if err != nil {
		return fmt.Errorf("handlerstore.SetActiveVersion: %w", err)
	}
	if n == 0 {
		return handlerdomain.ErrNotFound
	}
	return nil
}

// --- encrypted config ------------------------------------------------------

func (s *Store) GetConfigEncrypted(ctx context.Context, handlerID string) (string, error) {
	var vals []string
	if err := s.hdls.WhereEq("id", handlerID).Limit(1).Pluck(ctx, "config_encrypted", &vals); err != nil {
		return "", fmt.Errorf("handlerstore.GetConfigEncrypted: %w", err)
	}
	if len(vals) == 0 {
		return "", handlerdomain.ErrNotFound
	}
	return vals[0], nil
}

func (s *Store) UpdateConfigEncrypted(ctx context.Context, handlerID, ciphertext string) error {
	n, err := s.hdls.WhereEq("id", handlerID).Update(ctx, "config_encrypted", ciphertext)
	if err != nil {
		return fmt.Errorf("handlerstore.UpdateConfigEncrypted: %w", err)
	}
	if n == 0 {
		return handlerdomain.ErrNotFound
	}
	return nil
}

func (s *Store) ClearConfig(ctx context.Context, handlerID string) error {
	n, err := s.hdls.WhereEq("id", handlerID).Update(ctx, "config_encrypted", "")
	if err != nil {
		return fmt.Errorf("handlerstore.ClearConfig: %w", err)
	}
	if n == 0 {
		return handlerdomain.ErrNotFound
	}
	return nil
}

// --- versions --------------------------------------------------------------

func (s *Store) SaveVersion(ctx context.Context, v *handlerdomain.Version) error {
	if err := s.vers.Save(ctx, v); err != nil {
		return fmt.Errorf("handlerstore.SaveVersion: %w", err)
	}
	return nil
}

func (s *Store) GetVersion(ctx context.Context, versionID string) (*handlerdomain.Version, error) {
	v, err := s.vers.Get(ctx, versionID)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, handlerdomain.ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("handlerstore.GetVersion: %w", err)
	}
	return v, nil
}

func (s *Store) GetVersionByNumber(ctx context.Context, handlerID string, versionN int) (*handlerdomain.Version, error) {
	v, err := s.vers.WhereEq("handler_id", handlerID).WhereEq("version", versionN).First(ctx)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, handlerdomain.ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("handlerstore.GetVersionByNumber: %w", err)
	}
	return v, nil
}

func (s *Store) ListVersions(ctx context.Context, handlerID string, filter handlerdomain.VersionListFilter) ([]*handlerdomain.Version, string, error) {
	rows, next, err := s.vers.WhereEq("handler_id", handlerID).Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("handlerstore.ListVersions: %w", err)
	}
	return rows, next, nil
}

func (s *Store) MaxVersionNumber(ctx context.Context, handlerID string) (int, error) {
	var nums []int
	if err := s.vers.WhereEq("handler_id", handlerID).Order("version DESC").Limit(1).Pluck(ctx, "version", &nums); err != nil {
		return 0, fmt.Errorf("handlerstore.MaxVersionNumber: %w", err)
	}
	if len(nums) == 0 {
		return 0, nil
	}
	return nums[0], nil
}

func (s *Store) UpdateVersionEnv(ctx context.Context, versionID, envStatus, envError string, deps []string, syncedAt *time.Time) error {
	if deps == nil {
		deps = []string{}
	}
	depsJSON, err := json.Marshal(deps)
	if err != nil {
		return fmt.Errorf("handlerstore.UpdateVersionEnv: marshal deps: %w", err)
	}
	n, err := s.vers.WhereEq("id", versionID).Updates(ctx, map[string]any{
		"env_status":    envStatus,
		"env_error":     envError,
		"dependencies":  string(depsJSON),
		"env_synced_at": syncedAt,
	})
	if err != nil {
		return fmt.Errorf("handlerstore.UpdateVersionEnv: %w", err)
	}
	if n == 0 {
		return handlerdomain.ErrVersionNotFound
	}
	return nil
}

// TrimOldestVersions hard-deletes versions below the keep-th newest, sparing the active.
//
// TrimOldestVersions 硬删低于第 keep 新版本号的版本，放过 active 版本。
func (s *Store) TrimOldestVersions(ctx context.Context, handlerID string, keep int) error {
	if keep <= 0 {
		keep = handlerdomain.VersionCap
	}
	var nums []int
	if err := s.vers.WhereEq("handler_id", handlerID).Order("version DESC").Pluck(ctx, "version", &nums); err != nil {
		return fmt.Errorf("handlerstore.TrimOldestVersions: %w", err)
	}
	if len(nums) <= keep {
		return nil
	}
	cutoff := nums[keep-1]

	h, err := s.hdls.Get(ctx, handlerID)
	if err != nil {
		return fmt.Errorf("handlerstore.TrimOldestVersions: load active: %w", err)
	}
	if _, err := s.vers.
		WhereEq("handler_id", handlerID).
		Where("version < ?", cutoff).
		Where("id != ?", h.ActiveVersionID).
		Delete(ctx); err != nil { // hard-delete: handler_versions has no deleted_at
		return fmt.Errorf("handlerstore.TrimOldestVersions: %w", err)
	}
	return nil
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, v := range ss {
		out[i] = v
	}
	return out
}

// CreateWithVersion inserts the entity row and its v1 in ONE transaction: a
// create either fully lands or fully doesn't — no versionless entity row on a mid-write failure.
//
// CreateWithVersion 在单事务内插入实体行与其 v1：create 要么完整落地、要么完全不落
// ——中途失败不留无版本实体行。
func (s *Store) CreateWithVersion(ctx context.Context, e *handlerdomain.Handler, v *handlerdomain.Version) error {
	return s.db.Transaction(ctx, func(tx *ormpkg.DB) error {
		if err := ormpkg.For[handlerdomain.Handler](tx, "handlers").Save(ctx, e); err != nil {
			if errors.Is(err, ormpkg.ErrConflict) {
				return handlerdomain.ErrDuplicateName
			}
			return fmt.Errorf("handlerstore.CreateWithVersion: entity: %w", err)
		}
		if err := ormpkg.For[handlerdomain.Version](tx, "handler_versions").Save(ctx, v); err != nil {
			return fmt.Errorf("handlerstore.CreateWithVersion: version: %w", err)
		}
		return nil
	})
}

// SaveVersionAndActivate inserts a new version and moves the active pointer in ONE transaction:
// an edit either fully lands or fully doesn't — no orphan version + stale pointer.
//
// SaveVersionAndActivate 在单事务内插入新版本并移动 active 指针：edit 要么完整生效、
// 要么完全不生效——不留孤儿版本 + 旧指针。
func (s *Store) SaveVersionAndActivate(ctx context.Context, v *handlerdomain.Version, h *handlerdomain.Handler) error {
	return s.db.Transaction(ctx, func(tx *ormpkg.DB) error {
		if err := ormpkg.For[handlerdomain.Version](tx, "handler_versions").Save(ctx, v); err != nil {
			return fmt.Errorf("handlerstore.SaveVersionAndActivate: version: %w", err)
		}
		// Persist the row (active pointer AND meta) in the same tx: a set_meta op carried by the
		// edit must land too, else the handler keeps its old name/description while a new version
		// is active — the edit silently loses the rename. h was just read by the caller, so Save
		// upserts the existing row.
		//
		// 同事务持久化整行（active 指针 + meta）：edit 带的 set_meta 也必须落，否则 handler 保留旧名/旧述
		// 而新版本已 active——edit 静默丢了改名。h 是调用方刚读出的，Save 对已存在行做 upsert。
		h.ActiveVersionID = v.ID
		if err := ormpkg.For[handlerdomain.Handler](tx, "handlers").Save(ctx, h); err != nil {
			return fmt.Errorf("handlerstore.SaveVersionAndActivate: handler: %w", err)
		}
		return nil
	})
}

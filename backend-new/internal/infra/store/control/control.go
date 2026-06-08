// Package control is the orm-backed implementation of controldomain.Repository:
// control_logics (soft-deleted) + control_logic_versions (append-only, cap-trimmed).
// Workspace isolation is automatic (orm fills/filters workspace_id from ctx via the ,ws
// tag), so no method hand-writes a workspace predicate.
//
// Package control 是 controldomain.Repository 的 orm 实现：control_logics（软删）+
// control_logic_versions（只增、按上限裁剪）。workspace 隔离自动（orm 据 ctx 经 ,ws tag 填/
// 过滤 workspace_id），故无方法手写 workspace 谓词。
package control

import (
	"context"
	"errors"
	"fmt"

	controldomain "github.com/sunweilin/forgify/backend/internal/domain/control"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
)

// Schema is the control tables' DDL, exported as ordered idempotent statements for
// cmd/server to collect and apply via db.Migrate. control_logics has a partial-UNIQUE
// name (freed on soft-delete); versions are UNIQUE(control_id, version) and append-only
// (no deleted_at — cap-trimmed by hard delete).
//
// Schema 是 control 两表 DDL，按序幂等导出，由 cmd/server 汇总经 db.Migrate 应用。control_logics
// 用 partial-UNIQUE name（软删后释放）；versions UNIQUE(control_id, version)、只增（无 deleted_at
// ——按上限硬删裁剪）。
var Schema = []string{
	`CREATE TABLE IF NOT EXISTS control_logics (
		id                TEXT PRIMARY KEY,
		workspace_id      TEXT NOT NULL,
		name              TEXT NOT NULL,
		description       TEXT NOT NULL DEFAULT '',
		active_version_id TEXT NOT NULL DEFAULT '',
		created_at        DATETIME NOT NULL,
		updated_at        DATETIME NOT NULL,
		deleted_at        DATETIME
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_control_logics_ws_name ON control_logics(workspace_id, name) WHERE deleted_at IS NULL`,
	`CREATE INDEX IF NOT EXISTS idx_control_logics_ws_created ON control_logics(workspace_id, created_at DESC, id DESC) WHERE deleted_at IS NULL`,

	`CREATE TABLE IF NOT EXISTS control_logic_versions (
		id                        TEXT PRIMARY KEY,
		workspace_id              TEXT NOT NULL,
		control_id                TEXT NOT NULL,
		version                   INTEGER NOT NULL,
		input_schema              TEXT NOT NULL DEFAULT '[]',
		branches                  TEXT NOT NULL DEFAULT '[]',
		change_reason             TEXT NOT NULL DEFAULT '',
		forged_in_conversation_id TEXT,
		created_at                DATETIME NOT NULL,
		updated_at                DATETIME NOT NULL
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_ctlv_control_version ON control_logic_versions(control_id, version)`,
	`CREATE INDEX IF NOT EXISTS idx_ctlv_control_created ON control_logic_versions(control_id, created_at DESC, id DESC)`,
}

// Store implements controldomain.Repository over pkg/orm.
//
// Store 基于 pkg/orm 实现 controldomain.Repository。
type Store struct {
	db   *ormpkg.DB
	ctls *ormpkg.Repo[controldomain.ControlLogic]
	vers *ormpkg.Repo[controldomain.Version]
}

// New constructs a Store bound to the two control tables.
//
// New 构造绑定 control 两表的 Store。
func New(db *ormpkg.DB) *Store {
	return &Store{
		db:   db,
		ctls: ormpkg.For[controldomain.ControlLogic](db, "control_logics"),
		vers: ormpkg.For[controldomain.Version](db, "control_logic_versions"),
	}
}

var _ controldomain.Repository = (*Store)(nil)

// --- control logics --------------------------------------------------------

func (s *Store) SaveControl(ctx context.Context, c *controldomain.ControlLogic) error {
	if err := s.ctls.Save(ctx, c); err != nil {
		if errors.Is(err, ormpkg.ErrConflict) {
			return controldomain.ErrDuplicateName
		}
		return fmt.Errorf("controlstore.SaveControl: %w", err)
	}
	return nil
}

func (s *Store) GetControl(ctx context.Context, id string) (*controldomain.ControlLogic, error) {
	c, err := s.ctls.Get(ctx, id)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, controldomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("controlstore.GetControl: %w", err)
	}
	return c, nil
}

func (s *Store) GetControlsByIDs(ctx context.Context, ids []string) ([]*controldomain.ControlLogic, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.ctls.WhereIn("id", toAny(ids)...).Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("controlstore.GetControlsByIDs: %w", err)
	}
	byID := make(map[string]*controldomain.ControlLogic, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	out := make([]*controldomain.ControlLogic, 0, len(ids))
	for _, id := range ids {
		if r, ok := byID[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Store) ListControls(ctx context.Context, filter controldomain.ListFilter) ([]*controldomain.ControlLogic, string, error) {
	rows, next, err := s.ctls.Query().Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("controlstore.ListControls: %w", err)
	}
	return rows, next, nil
}

func (s *Store) ListAllControls(ctx context.Context) ([]*controldomain.ControlLogic, error) {
	rows, err := s.ctls.Order("created_at DESC, id DESC").Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("controlstore.ListAllControls: %w", err)
	}
	return rows, nil
}

func (s *Store) DeleteControl(ctx context.Context, id string) error {
	ok, err := s.ctls.Delete(ctx, id) // soft-delete (control_logics has deleted_at)
	if err != nil {
		return fmt.Errorf("controlstore.DeleteControl: %w", err)
	}
	if !ok {
		return controldomain.ErrNotFound
	}
	return nil
}

func (s *Store) SetActiveVersion(ctx context.Context, controlID, versionID string) error {
	n, err := s.ctls.WhereEq("id", controlID).Update(ctx, "active_version_id", versionID)
	if err != nil {
		return fmt.Errorf("controlstore.SetActiveVersion: %w", err)
	}
	if n == 0 {
		return controldomain.ErrNotFound
	}
	return nil
}

// --- versions --------------------------------------------------------------

func (s *Store) SaveVersion(ctx context.Context, v *controldomain.Version) error {
	if err := s.vers.Save(ctx, v); err != nil {
		return fmt.Errorf("controlstore.SaveVersion: %w", err)
	}
	return nil
}

func (s *Store) GetVersion(ctx context.Context, versionID string) (*controldomain.Version, error) {
	v, err := s.vers.Get(ctx, versionID)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, controldomain.ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("controlstore.GetVersion: %w", err)
	}
	return v, nil
}

func (s *Store) GetVersionByNumber(ctx context.Context, controlID string, versionN int) (*controldomain.Version, error) {
	v, err := s.vers.WhereEq("control_id", controlID).WhereEq("version", versionN).First(ctx)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, controldomain.ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("controlstore.GetVersionByNumber: %w", err)
	}
	return v, nil
}

func (s *Store) ListVersions(ctx context.Context, controlID string, filter controldomain.VersionListFilter) ([]*controldomain.Version, string, error) {
	rows, next, err := s.vers.WhereEq("control_id", controlID).Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("controlstore.ListVersions: %w", err)
	}
	return rows, next, nil
}

func (s *Store) MaxVersionNumber(ctx context.Context, controlID string) (int, error) {
	var nums []int
	if err := s.vers.WhereEq("control_id", controlID).Order("version DESC").Limit(1).Pluck(ctx, "version", &nums); err != nil {
		return 0, fmt.Errorf("controlstore.MaxVersionNumber: %w", err)
	}
	if len(nums) == 0 {
		return 0, nil
	}
	return nums[0], nil
}

// TrimOldestVersions hard-deletes versions below the keep-th newest version number,
// always sparing the control logic's active version (which may be old after a revert).
//
// TrimOldestVersions 硬删低于第 keep 新版本号的版本，始终放过 control 逻辑的 active 版本
// （revert 后它可能很老）。
func (s *Store) TrimOldestVersions(ctx context.Context, controlID string, keep int) error {
	if keep <= 0 {
		keep = controldomain.VersionCap
	}
	var nums []int
	if err := s.vers.WhereEq("control_id", controlID).Order("version DESC").Pluck(ctx, "version", &nums); err != nil {
		return fmt.Errorf("controlstore.TrimOldestVersions: %w", err)
	}
	if len(nums) <= keep {
		return nil
	}
	cutoff := nums[keep-1] // keep versions with number >= cutoff (the keep newest)

	c, err := s.ctls.Get(ctx, controlID)
	if err != nil {
		return fmt.Errorf("controlstore.TrimOldestVersions: load active: %w", err)
	}
	if _, err := s.vers.
		WhereEq("control_id", controlID).
		Where("version < ?", cutoff).
		Where("id != ?", c.ActiveVersionID).
		Delete(ctx); err != nil { // hard-delete: control_logic_versions has no deleted_at
		return fmt.Errorf("controlstore.TrimOldestVersions: %w", err)
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

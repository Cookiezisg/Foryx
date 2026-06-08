// Package approval is the orm-backed implementation of approvaldomain.Repository:
// approval_forms (soft-deleted) + approval_form_versions (append-only, cap-trimmed).
// Workspace isolation is automatic (orm fills/filters workspace_id from ctx via the ,ws
// tag), so no method hand-writes a workspace predicate. Prefix apf_/apfv_ — NOT apv_
// (which is the `approvals` runtime table, 波次 4).
//
// Package approval 是 approvaldomain.Repository 的 orm 实现：approval_forms（软删）+
// approval_form_versions（只增、按上限裁剪）。workspace 隔离自动（orm 据 ctx 经 ,ws tag 填/过滤
// workspace_id），故无方法手写 workspace 谓词。前缀 apf_/apfv_——**非** apv_（那是 `approvals`
// 运行时表，波次 4）。
package approval

import (
	"context"
	"errors"
	"fmt"

	approvaldomain "github.com/sunweilin/forgify/backend/internal/domain/approval"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
)

// Schema is the approval-form tables' DDL, exported as ordered idempotent statements for
// cmd/server to collect and apply via db.Migrate. approval_forms has a partial-UNIQUE name
// (freed on soft-delete); versions are UNIQUE(approval_id, version) and append-only.
//
// Schema 是审批表两表 DDL，按序幂等导出，由 cmd/server 汇总经 db.Migrate 应用。approval_forms 用
// partial-UNIQUE name（软删后释放）；versions UNIQUE(approval_id, version)、只增。
var Schema = []string{
	`CREATE TABLE IF NOT EXISTS approval_forms (
		id                TEXT PRIMARY KEY,
		workspace_id      TEXT NOT NULL,
		name              TEXT NOT NULL,
		description       TEXT NOT NULL DEFAULT '',
		active_version_id TEXT NOT NULL DEFAULT '',
		created_at        DATETIME NOT NULL,
		updated_at        DATETIME NOT NULL,
		deleted_at        DATETIME
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_approval_forms_ws_name ON approval_forms(workspace_id, name) WHERE deleted_at IS NULL`,
	`CREATE INDEX IF NOT EXISTS idx_approval_forms_ws_created ON approval_forms(workspace_id, created_at DESC, id DESC) WHERE deleted_at IS NULL`,

	`CREATE TABLE IF NOT EXISTS approval_form_versions (
		id                        TEXT PRIMARY KEY,
		workspace_id              TEXT NOT NULL,
		approval_id               TEXT NOT NULL,
		version                   INTEGER NOT NULL,
		template                  TEXT NOT NULL DEFAULT '',
		allow_reason              INTEGER NOT NULL DEFAULT 0,
		timeout                   TEXT NOT NULL DEFAULT '',
		timeout_behavior          TEXT NOT NULL DEFAULT '',
		change_reason             TEXT NOT NULL DEFAULT '',
		forged_in_conversation_id TEXT,
		created_at                DATETIME NOT NULL,
		updated_at                DATETIME NOT NULL
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_apfv_approval_version ON approval_form_versions(approval_id, version)`,
	`CREATE INDEX IF NOT EXISTS idx_apfv_approval_created ON approval_form_versions(approval_id, created_at DESC, id DESC)`,
}

// Store implements approvaldomain.Repository over pkg/orm.
//
// Store 基于 pkg/orm 实现 approvaldomain.Repository。
type Store struct {
	db    *ormpkg.DB
	forms *ormpkg.Repo[approvaldomain.ApprovalForm]
	vers  *ormpkg.Repo[approvaldomain.Version]
}

// New constructs a Store bound to the two approval-form tables.
//
// New 构造绑定审批表两表的 Store。
func New(db *ormpkg.DB) *Store {
	return &Store{
		db:    db,
		forms: ormpkg.For[approvaldomain.ApprovalForm](db, "approval_forms"),
		vers:  ormpkg.For[approvaldomain.Version](db, "approval_form_versions"),
	}
}

var _ approvaldomain.Repository = (*Store)(nil)

// --- approval forms --------------------------------------------------------

func (s *Store) SaveForm(ctx context.Context, f *approvaldomain.ApprovalForm) error {
	if err := s.forms.Save(ctx, f); err != nil {
		if errors.Is(err, ormpkg.ErrConflict) {
			return approvaldomain.ErrDuplicateName
		}
		return fmt.Errorf("approvalstore.SaveForm: %w", err)
	}
	return nil
}

func (s *Store) GetForm(ctx context.Context, id string) (*approvaldomain.ApprovalForm, error) {
	f, err := s.forms.Get(ctx, id)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, approvaldomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("approvalstore.GetForm: %w", err)
	}
	return f, nil
}

func (s *Store) GetFormsByIDs(ctx context.Context, ids []string) ([]*approvaldomain.ApprovalForm, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.forms.WhereIn("id", toAny(ids)...).Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("approvalstore.GetFormsByIDs: %w", err)
	}
	byID := make(map[string]*approvaldomain.ApprovalForm, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	out := make([]*approvaldomain.ApprovalForm, 0, len(ids))
	for _, id := range ids {
		if r, ok := byID[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Store) ListForms(ctx context.Context, filter approvaldomain.ListFilter) ([]*approvaldomain.ApprovalForm, string, error) {
	rows, next, err := s.forms.Query().Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("approvalstore.ListForms: %w", err)
	}
	return rows, next, nil
}

func (s *Store) ListAllForms(ctx context.Context) ([]*approvaldomain.ApprovalForm, error) {
	rows, err := s.forms.Order("created_at DESC, id DESC").Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("approvalstore.ListAllForms: %w", err)
	}
	return rows, nil
}

func (s *Store) DeleteForm(ctx context.Context, id string) error {
	ok, err := s.forms.Delete(ctx, id) // soft-delete (approval_forms has deleted_at)
	if err != nil {
		return fmt.Errorf("approvalstore.DeleteForm: %w", err)
	}
	if !ok {
		return approvaldomain.ErrNotFound
	}
	return nil
}

func (s *Store) SetActiveVersion(ctx context.Context, formID, versionID string) error {
	n, err := s.forms.WhereEq("id", formID).Update(ctx, "active_version_id", versionID)
	if err != nil {
		return fmt.Errorf("approvalstore.SetActiveVersion: %w", err)
	}
	if n == 0 {
		return approvaldomain.ErrNotFound
	}
	return nil
}

// --- versions --------------------------------------------------------------

func (s *Store) SaveVersion(ctx context.Context, v *approvaldomain.Version) error {
	if err := s.vers.Save(ctx, v); err != nil {
		return fmt.Errorf("approvalstore.SaveVersion: %w", err)
	}
	return nil
}

func (s *Store) GetVersion(ctx context.Context, versionID string) (*approvaldomain.Version, error) {
	v, err := s.vers.Get(ctx, versionID)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, approvaldomain.ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("approvalstore.GetVersion: %w", err)
	}
	return v, nil
}

func (s *Store) GetVersionByNumber(ctx context.Context, formID string, versionN int) (*approvaldomain.Version, error) {
	v, err := s.vers.WhereEq("approval_id", formID).WhereEq("version", versionN).First(ctx)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, approvaldomain.ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("approvalstore.GetVersionByNumber: %w", err)
	}
	return v, nil
}

func (s *Store) ListVersions(ctx context.Context, formID string, filter approvaldomain.VersionListFilter) ([]*approvaldomain.Version, string, error) {
	rows, next, err := s.vers.WhereEq("approval_id", formID).Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("approvalstore.ListVersions: %w", err)
	}
	return rows, next, nil
}

func (s *Store) MaxVersionNumber(ctx context.Context, formID string) (int, error) {
	var nums []int
	if err := s.vers.WhereEq("approval_id", formID).Order("version DESC").Limit(1).Pluck(ctx, "version", &nums); err != nil {
		return 0, fmt.Errorf("approvalstore.MaxVersionNumber: %w", err)
	}
	if len(nums) == 0 {
		return 0, nil
	}
	return nums[0], nil
}

// TrimOldestVersions hard-deletes versions below the keep-th newest version number, always
// sparing the form's active version (which may be old after a revert).
//
// TrimOldestVersions 硬删低于第 keep 新版本号的版本，始终放过审批表的 active 版本（revert 后它
// 可能很老）。
func (s *Store) TrimOldestVersions(ctx context.Context, formID string, keep int) error {
	if keep <= 0 {
		keep = approvaldomain.VersionCap
	}
	var nums []int
	if err := s.vers.WhereEq("approval_id", formID).Order("version DESC").Pluck(ctx, "version", &nums); err != nil {
		return fmt.Errorf("approvalstore.TrimOldestVersions: %w", err)
	}
	if len(nums) <= keep {
		return nil
	}
	cutoff := nums[keep-1]

	f, err := s.forms.Get(ctx, formID)
	if err != nil {
		return fmt.Errorf("approvalstore.TrimOldestVersions: load active: %w", err)
	}
	if _, err := s.vers.
		WhereEq("approval_id", formID).
		Where("version < ?", cutoff).
		Where("id != ?", f.ActiveVersionID).
		Delete(ctx); err != nil { // hard-delete: approval_form_versions has no deleted_at
		return fmt.Errorf("approvalstore.TrimOldestVersions: %w", err)
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

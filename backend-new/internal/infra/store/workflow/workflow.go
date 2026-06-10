// Package workflow is the orm-backed implementation of workflowdomain.Repository:
// workflows (soft-deleted) + workflow_versions (append-only, cap-trimmed). Workspace
// isolation is automatic (orm fills/filters workspace_id from ctx via the ,ws tag), so no
// method hand-writes a workspace predicate.
//
// Package workflow 是 workflowdomain.Repository 的 orm 实现：workflows（软删）+
// workflow_versions（只增、按上限裁剪）。workspace 隔离自动（orm 据 ctx 经 ,ws tag 填/过滤
// workspace_id），故无方法手写 workspace 谓词。
package workflow

import (
	"context"
	"errors"
	"fmt"

	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
)

// Schema is the workflow tables' DDL, exported as ordered idempotent statements for
// cmd/server to collect and apply via db.Migrate. workflows has a partial-UNIQUE name
// (freed on soft-delete) plus CHECK-constrained lifecycle_state / concurrency; versions are
// UNIQUE(workflow_id, version) and append-only (no deleted_at — cap-trimmed by hard delete).
//
// Schema 是 workflow 两表 DDL，按序幂等导出，由 cmd/server 汇总经 db.Migrate 应用。workflows 用
// partial-UNIQUE name（软删后释放）+ CHECK 约束的 lifecycle_state / concurrency；versions
// UNIQUE(workflow_id, version)、只增（无 deleted_at——按上限硬删裁剪）。
var Schema = []string{
	`CREATE TABLE IF NOT EXISTS workflows (
		id                TEXT PRIMARY KEY,
		workspace_id      TEXT NOT NULL,
		name              TEXT NOT NULL,
		description       TEXT NOT NULL DEFAULT '',
		tags              TEXT NOT NULL DEFAULT '[]',
		active            INTEGER NOT NULL DEFAULT 0,
		lifecycle_state   TEXT NOT NULL DEFAULT 'inactive' CHECK (lifecycle_state IN ('active','draining','inactive')),
		concurrency       TEXT NOT NULL DEFAULT 'serial' CHECK (concurrency IN ('serial','Skip','BufferOne','BufferAll','AllowAll')),
		needs_attention   INTEGER NOT NULL DEFAULT 0,
		attention_reason  TEXT NOT NULL DEFAULT '',
		last_action_by    TEXT NOT NULL DEFAULT 'user',
		active_version_id TEXT NOT NULL DEFAULT '',
		created_at        DATETIME NOT NULL,
		updated_at        DATETIME NOT NULL,
		deleted_at        DATETIME
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_workflows_ws_name ON workflows(workspace_id, name) WHERE deleted_at IS NULL`,
	`CREATE INDEX IF NOT EXISTS idx_workflows_ws_created ON workflows(workspace_id, created_at DESC, id DESC) WHERE deleted_at IS NULL`,
	`CREATE INDEX IF NOT EXISTS idx_workflows_ws_active ON workflows(workspace_id, active) WHERE deleted_at IS NULL AND active = 1`,

	`CREATE TABLE IF NOT EXISTS workflow_versions (
		id                        TEXT PRIMARY KEY,
		workspace_id              TEXT NOT NULL,
		workflow_id               TEXT NOT NULL,
		version                   INTEGER NOT NULL,
		graph                     TEXT NOT NULL DEFAULT '{}',
		change_reason             TEXT NOT NULL DEFAULT '',
		forged_in_conversation_id TEXT,
		created_at                DATETIME NOT NULL,
		updated_at                DATETIME NOT NULL
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_wfv_workflow_version ON workflow_versions(workflow_id, version)`,
	`CREATE INDEX IF NOT EXISTS idx_wfv_workflow_created ON workflow_versions(workflow_id, created_at DESC, id DESC)`,
}

// Store implements workflowdomain.Repository over pkg/orm.
//
// Store 基于 pkg/orm 实现 workflowdomain.Repository。
type Store struct {
	db   *ormpkg.DB
	wfs  *ormpkg.Repo[workflowdomain.Workflow]
	vers *ormpkg.Repo[workflowdomain.Version]
}

// New constructs a Store bound to the two workflow tables.
//
// New 构造绑定 workflow 两表的 Store。
func New(db *ormpkg.DB) *Store {
	return &Store{
		db:   db,
		wfs:  ormpkg.For[workflowdomain.Workflow](db, "workflows"),
		vers: ormpkg.For[workflowdomain.Version](db, "workflow_versions"),
	}
}

var _ workflowdomain.Repository = (*Store)(nil)

// --- workflows -------------------------------------------------------------

func (s *Store) SaveWorkflow(ctx context.Context, w *workflowdomain.Workflow) error {
	if err := s.wfs.Save(ctx, w); err != nil {
		if errors.Is(err, ormpkg.ErrConflict) {
			return workflowdomain.ErrDuplicateName
		}
		return fmt.Errorf("workflowstore.SaveWorkflow: %w", err)
	}
	return nil
}

func (s *Store) GetWorkflow(ctx context.Context, id string) (*workflowdomain.Workflow, error) {
	w, err := s.wfs.Get(ctx, id)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, workflowdomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("workflowstore.GetWorkflow: %w", err)
	}
	return w, nil
}

func (s *Store) GetWorkflowByName(ctx context.Context, name string) (*workflowdomain.Workflow, error) {
	w, err := s.wfs.WhereEq("name", name).First(ctx)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, workflowdomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("workflowstore.GetWorkflowByName: %w", err)
	}
	return w, nil
}

func (s *Store) GetWorkflowsByIDs(ctx context.Context, ids []string) ([]*workflowdomain.Workflow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.wfs.WhereIn("id", toAny(ids)...).Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("workflowstore.GetWorkflowsByIDs: %w", err)
	}
	byID := make(map[string]*workflowdomain.Workflow, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	out := make([]*workflowdomain.Workflow, 0, len(ids))
	for _, id := range ids {
		if r, ok := byID[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Store) ListWorkflows(ctx context.Context, filter workflowdomain.ListFilter) ([]*workflowdomain.Workflow, string, error) {
	rows, next, err := s.wfs.Query().Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("workflowstore.ListWorkflows: %w", err)
	}
	return rows, next, nil
}

func (s *Store) ListAllWorkflows(ctx context.Context) ([]*workflowdomain.Workflow, error) {
	rows, err := s.wfs.Order("created_at DESC, id DESC").Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("workflowstore.ListAllWorkflows: %w", err)
	}
	return rows, nil
}

func (s *Store) ListActiveWorkflows(ctx context.Context) ([]*workflowdomain.Workflow, error) {
	rows, err := s.wfs.WhereEq("active", true).Order("created_at DESC, id DESC").Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("workflowstore.ListActiveWorkflows: %w", err)
	}
	return rows, nil
}

func (s *Store) DeleteWorkflow(ctx context.Context, id string) error {
	ok, err := s.wfs.Delete(ctx, id) // soft-delete (workflows has deleted_at)
	if err != nil {
		return fmt.Errorf("workflowstore.DeleteWorkflow: %w", err)
	}
	if !ok {
		return workflowdomain.ErrNotFound
	}
	return nil
}

func (s *Store) SetActiveVersion(ctx context.Context, workflowID, versionID string) error {
	n, err := s.wfs.WhereEq("id", workflowID).Update(ctx, "active_version_id", versionID)
	if err != nil {
		return fmt.Errorf("workflowstore.SetActiveVersion: %w", err)
	}
	if n == 0 {
		return workflowdomain.ErrNotFound
	}
	return nil
}

// UpdateWorkflowMeta writes the lifecycle/concurrency/attention columns in one update; a nil
// field in MetaUpdate is omitted. An empty patch is a no-op (no row touched, no error).
//
// UpdateWorkflowMeta 一次更新写 lifecycle/concurrency/attention 列；MetaUpdate 中 nil 字段省略。
// 空 patch 为 no-op（不碰行、不报错）。
func (s *Store) UpdateWorkflowMeta(ctx context.Context, workflowID string, upd workflowdomain.MetaUpdate) error {
	fields := map[string]any{}
	if upd.Active != nil {
		fields["active"] = *upd.Active
	}
	if upd.LifecycleState != nil {
		fields["lifecycle_state"] = *upd.LifecycleState
	}
	if upd.Concurrency != nil {
		fields["concurrency"] = *upd.Concurrency
	}
	if upd.NeedsAttention != nil {
		fields["needs_attention"] = *upd.NeedsAttention
	}
	if upd.AttentionReason != nil {
		fields["attention_reason"] = *upd.AttentionReason
	}
	if upd.LastActionBy != nil {
		fields["last_action_by"] = *upd.LastActionBy
	}
	if len(fields) == 0 {
		return nil
	}
	n, err := s.wfs.WhereEq("id", workflowID).Updates(ctx, fields)
	if err != nil {
		return fmt.Errorf("workflowstore.UpdateWorkflowMeta: %w", err)
	}
	if n == 0 {
		return workflowdomain.ErrNotFound
	}
	return nil
}

// MarkInactiveIfDraining flips draining→inactive (+ active=false) conditionally on the row still
// being draining. 0 rows matched (already off draining / gone) is the expected no-op of an
// idempotent reconcile, NOT ErrNotFound.
//
// MarkInactiveIfDraining 条件把 draining→inactive（+ active=false），条件是行仍 draining。0 行匹配
// （已离开 draining / 不存在）是幂等 reconcile 的预期 no-op，**非** ErrNotFound。
func (s *Store) MarkInactiveIfDraining(ctx context.Context, workflowID string) error {
	_, err := s.wfs.WhereEq("id", workflowID).WhereEq("lifecycle_state", workflowdomain.LifecycleDraining).Updates(ctx, map[string]any{
		"lifecycle_state": workflowdomain.LifecycleInactive,
		"active":          false,
	})
	if err != nil {
		return fmt.Errorf("workflowstore.MarkInactiveIfDraining: %w", err)
	}
	return nil
}

// --- versions --------------------------------------------------------------

func (s *Store) SaveVersion(ctx context.Context, v *workflowdomain.Version) error {
	if err := s.vers.Save(ctx, v); err != nil {
		return fmt.Errorf("workflowstore.SaveVersion: %w", err)
	}
	return nil
}

func (s *Store) GetVersion(ctx context.Context, versionID string) (*workflowdomain.Version, error) {
	v, err := s.vers.Get(ctx, versionID)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, workflowdomain.ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("workflowstore.GetVersion: %w", err)
	}
	return v, nil
}

func (s *Store) GetVersionByNumber(ctx context.Context, workflowID string, versionN int) (*workflowdomain.Version, error) {
	v, err := s.vers.WhereEq("workflow_id", workflowID).WhereEq("version", versionN).First(ctx)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, workflowdomain.ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("workflowstore.GetVersionByNumber: %w", err)
	}
	return v, nil
}

func (s *Store) ListVersions(ctx context.Context, workflowID string, filter workflowdomain.VersionListFilter) ([]*workflowdomain.Version, string, error) {
	rows, next, err := s.vers.WhereEq("workflow_id", workflowID).Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("workflowstore.ListVersions: %w", err)
	}
	return rows, next, nil
}

func (s *Store) MaxVersionNumber(ctx context.Context, workflowID string) (int, error) {
	var nums []int
	if err := s.vers.WhereEq("workflow_id", workflowID).Order("version DESC").Limit(1).Pluck(ctx, "version", &nums); err != nil {
		return 0, fmt.Errorf("workflowstore.MaxVersionNumber: %w", err)
	}
	if len(nums) == 0 {
		return 0, nil
	}
	return nums[0], nil
}

// TrimOldestVersions hard-deletes versions below the keep-th newest version number, always
// sparing the workflow's active version (which may be old after a revert).
//
// TrimOldestVersions 硬删低于第 keep 新版本号的版本，始终放过 workflow 的 active 版本（revert 后
// 它可能很老）。
func (s *Store) TrimOldestVersions(ctx context.Context, workflowID string, keep int) error {
	if keep <= 0 {
		keep = workflowdomain.VersionCap
	}
	var nums []int
	if err := s.vers.WhereEq("workflow_id", workflowID).Order("version DESC").Pluck(ctx, "version", &nums); err != nil {
		return fmt.Errorf("workflowstore.TrimOldestVersions: %w", err)
	}
	if len(nums) <= keep {
		return nil
	}
	cutoff := nums[keep-1] // keep versions with number >= cutoff (the keep newest)

	w, err := s.wfs.Get(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("workflowstore.TrimOldestVersions: load active: %w", err)
	}
	if _, err := s.vers.
		WhereEq("workflow_id", workflowID).
		Where("version < ?", cutoff).
		Where("id != ?", w.ActiveVersionID).
		Delete(ctx); err != nil { // hard-delete: workflow_versions has no deleted_at
		return fmt.Errorf("workflowstore.TrimOldestVersions: %w", err)
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

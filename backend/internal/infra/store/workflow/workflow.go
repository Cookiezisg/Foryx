// Package workflow is the GORM-backed workflowdomain.Repository, scoped by ctx userID.
//
// Package workflow 是 workflowdomain.Repository 的 GORM 实现，按 ctx userID 过滤。
package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Store is the GORM implementation of workflowdomain.Repository.
//
// Store 是 workflowdomain.Repository 的 GORM 实现。
type Store struct {
	db *gorm.DB
}

// New constructs a Store bound to the given *gorm.DB.
//
// New 基于给定 *gorm.DB 构造 Store。
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

var _ workflowdomain.Repository = (*Store)(nil)

// AutoMigrateModels returns the GORM models to register in db.AutoMigrate.
//
// AutoMigrateModels 返 AutoMigrate 用的 GORM models。
func AutoMigrateModels() []interface{} {
	return []interface{}{
		&workflowdomain.Workflow{},
		&workflowdomain.Version{},
	}
}

// SaveWorkflow upserts by PK; partial-UNIQUE name violation maps to ErrDuplicateName.
//
// SaveWorkflow 按主键 upsert；name partial UNIQUE 违反返 ErrDuplicateName。
func (s *Store) SaveWorkflow(ctx context.Context, w *workflowdomain.Workflow) error {
	if err := s.db.WithContext(ctx).Save(w).Error; err != nil {
		if isWorkflowDuplicateName(err) {
			return workflowdomain.ErrDuplicateName
		}
		return fmt.Errorf("workflowstore.SaveWorkflow: %w", err)
	}
	return nil
}

// GetWorkflow fetches by id; ErrNotFound on miss.
//
// GetWorkflow 按 id 查；未命中返 ErrNotFound。
func (s *Store) GetWorkflow(ctx context.Context, id string) (*workflowdomain.Workflow, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var w workflowdomain.Workflow
	res := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, uid).First(&w)
	if errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return nil, workflowdomain.ErrNotFound
	}
	if res.Error != nil {
		return nil, fmt.Errorf("workflowstore.GetWorkflow: %w", res.Error)
	}
	return &w, nil
}

// GetWorkflowByName fetches by name; ErrNotFound on miss.
//
// GetWorkflowByName 按 name 查；未命中返 ErrNotFound。
func (s *Store) GetWorkflowByName(ctx context.Context, name string) (*workflowdomain.Workflow, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var w workflowdomain.Workflow
	res := s.db.WithContext(ctx).Where("user_id = ? AND name = ?", uid, name).First(&w)
	if errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return nil, workflowdomain.ErrNotFound
	}
	if res.Error != nil {
		return nil, fmt.Errorf("workflowstore.GetWorkflowByName: %w", res.Error)
	}
	return &w, nil
}

// GetWorkflowsByIDs batch fetches by id slice, preserving input order; misses skipped.
//
// GetWorkflowsByIDs 批量按 id 查，保留输入顺序；未命中跳过。
func (s *Store) GetWorkflowsByIDs(ctx context.Context, ids []string) ([]*workflowdomain.Workflow, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []*workflowdomain.Workflow{}, nil
	}
	var rows []workflowdomain.Workflow
	if err := s.db.WithContext(ctx).Where("user_id = ? AND id IN ?", uid, ids).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("workflowstore.GetWorkflowsByIDs: %w", err)
	}
	byID := make(map[string]*workflowdomain.Workflow, len(rows))
	for i := range rows {
		byID[rows[i].ID] = &rows[i]
	}
	out := make([]*workflowdomain.Workflow, 0, len(ids))
	for _, id := range ids {
		if w := byID[id]; w != nil {
			out = append(out, w)
		}
	}
	return out, nil
}

// ListWorkflows returns a cursor-paginated page; updated_at DESC, id DESC.
//
// ListWorkflows 返分页；updated_at DESC + id DESC；EnabledOnly 过滤 disabled。
func (s *Store) ListWorkflows(ctx context.Context, filter workflowdomain.ListFilter) ([]*workflowdomain.Workflow, string, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, "", err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	tx := s.db.WithContext(ctx).Where("user_id = ?", uid)
	if filter.EnabledOnly {
		tx = tx.Where("enabled = ?", true)
	}
	if filter.Cursor != "" {
		var c paginationpkg.Cursor
		if err := paginationpkg.DecodeCursor(filter.Cursor, &c); err != nil {
			return nil, "", fmt.Errorf("workflowstore.ListWorkflows: %w", err)
		}
		tx = tx.Where("(updated_at, id) < (?, ?)", c.CreatedAt, c.ID)
	}
	var rows []workflowdomain.Workflow
	if err := tx.Order("updated_at DESC, id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("workflowstore.ListWorkflows: %w", err)
	}
	next := ""
	if len(rows) > limit {
		last := rows[limit-1]
		var err error
		next, err = paginationpkg.EncodeCursor(paginationpkg.Cursor{CreatedAt: last.UpdatedAt, ID: last.ID})
		if err != nil {
			return nil, "", fmt.Errorf("workflowstore.ListWorkflows: %w", err)
		}
		rows = rows[:limit]
	}
	out := make([]*workflowdomain.Workflow, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, next, nil
}

// ListAllWorkflows returns every live workflow for current user (no pagination).
//
// ListAllWorkflows 返当前用户全部活跃 workflow（无分页）。
func (s *Store) ListAllWorkflows(ctx context.Context) ([]*workflowdomain.Workflow, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var rows []workflowdomain.Workflow
	if err := s.db.WithContext(ctx).Where("user_id = ?", uid).
		Order("updated_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("workflowstore.ListAllWorkflows: %w", err)
	}
	out := make([]*workflowdomain.Workflow, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, nil
}

// DeleteWorkflow soft-deletes by id; ErrNotFound on miss.
//
// DeleteWorkflow 按 id 软删；未命中返 ErrNotFound。
func (s *Store) DeleteWorkflow(ctx context.Context, id string) error {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return err
	}
	res := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, uid).
		Delete(&workflowdomain.Workflow{})
	if res.Error != nil {
		return fmt.Errorf("workflowstore.DeleteWorkflow: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return workflowdomain.ErrNotFound
	}
	return nil
}

// SetActiveVersion atomically updates Workflow.ActiveVersionID (accept / revert flows).
//
// SetActiveVersion 原子更新 ActiveVersionID（accept / revert 用）。
func (s *Store) SetActiveVersion(ctx context.Context, workflowID, versionID string) error {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return err
	}
	res := s.db.WithContext(ctx).Model(&workflowdomain.Workflow{}).
		Where("id = ? AND user_id = ?", workflowID, uid).
		Update("active_version_id", versionID)
	if res.Error != nil {
		return fmt.Errorf("workflowstore.SetActiveVersion: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return workflowdomain.ErrNotFound
	}
	return nil
}

// SetNeedsAttention atomically updates NeedsAttention + AttentionReason.
//
// SetNeedsAttention 原子更新 NeedsAttention + AttentionReason。
func (s *Store) SetNeedsAttention(ctx context.Context, workflowID string, needs bool, reason string) error {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return err
	}
	res := s.db.WithContext(ctx).Model(&workflowdomain.Workflow{}).
		Where("id = ? AND user_id = ?", workflowID, uid).
		Updates(map[string]any{
			"needs_attention":  needs,
			"attention_reason": reason,
		})
	if res.Error != nil {
		return fmt.Errorf("workflowstore.SetNeedsAttention: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return workflowdomain.ErrNotFound
	}
	return nil
}

// AnyReferencesApiKey reports whether any workflow_version.graph JSON contains
// a node.modelOverride.apiKeyId equal to apiKeyID. Scope is current user via
// workflows.user_id join (workflow_versions has no user_id column of its own).
//
// AnyReferencesApiKey 报告是否有 workflow_version.graph 里的
// node.modelOverride.apiKeyId 等于 apiKeyID；通过 workflows.user_id JOIN 限定当前用户
// （workflow_versions 表本身没有 user_id 列）。
func (s *Store) AnyReferencesApiKey(ctx context.Context, apiKeyID string) (bool, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return false, fmt.Errorf("workflowstore.AnyReferencesApiKey: %w", err)
	}
	pattern := `%"apiKeyId":"` + apiKeyID + `"%`
	var count int64
	if err := s.db.WithContext(ctx).
		Table("workflow_versions").
		Joins("JOIN workflows ON workflows.id = workflow_versions.workflow_id").
		Where("workflows.user_id = ? AND workflows.deleted_at IS NULL AND workflow_versions.graph LIKE ?", uid, pattern).
		Limit(1).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("workflowstore.AnyReferencesApiKey: %w", err)
	}
	return count > 0, nil
}

// SaveVersion upserts a Version by primary key.
//
// SaveVersion 按主键 upsert Version。
func (s *Store) SaveVersion(ctx context.Context, v *workflowdomain.Version) error {
	if err := s.db.WithContext(ctx).Save(v).Error; err != nil {
		return fmt.Errorf("workflowstore.SaveVersion: %w", err)
	}
	return nil
}

// GetVersion fetches by version id; ErrVersionNotFound if absent.
//
// GetVersion 按 version id 查；未命中返 ErrVersionNotFound。
func (s *Store) GetVersion(ctx context.Context, versionID string) (*workflowdomain.Version, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, err
	}
	var v workflowdomain.Version
	res := s.db.WithContext(ctx).Where("id = ?", versionID).First(&v)
	if errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return nil, workflowdomain.ErrVersionNotFound
	}
	if res.Error != nil {
		return nil, fmt.Errorf("workflowstore.GetVersion: %w", res.Error)
	}
	if err := s.assertVersionUser(ctx, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// GetVersionByNumber fetches by workflow id + integer version.
//
// GetVersionByNumber 按 workflow + 整数版本查。
func (s *Store) GetVersionByNumber(ctx context.Context, workflowID string, versionN int) (*workflowdomain.Version, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var wf workflowdomain.Workflow
	if err := s.db.WithContext(ctx).Select("id").Where("id = ? AND user_id = ?", workflowID, uid).
		First(&wf).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, workflowdomain.ErrNotFound
		}
		return nil, fmt.Errorf("workflowstore.GetVersionByNumber: %w", err)
	}
	var v workflowdomain.Version
	res := s.db.WithContext(ctx).Where("workflow_id = ? AND version = ?", workflowID, versionN).First(&v)
	if errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return nil, workflowdomain.ErrVersionNotFound
	}
	if res.Error != nil {
		return nil, fmt.Errorf("workflowstore.GetVersionByNumber: %w", res.Error)
	}
	return &v, nil
}

// ListVersions returns cursor-paginated versions for a workflow, newest first.
//
// ListVersions 返某 workflow 版本的分页（新→旧）；可按 status 过滤。
func (s *Store) ListVersions(ctx context.Context, workflowID string, filter workflowdomain.VersionListFilter) ([]*workflowdomain.Version, string, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, "", err
	}
	var wf workflowdomain.Workflow
	if err := s.db.WithContext(ctx).Select("id").Where("id = ? AND user_id = ?", workflowID, uid).
		First(&wf).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", workflowdomain.ErrNotFound
		}
		return nil, "", fmt.Errorf("workflowstore.ListVersions: %w", err)
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	tx := s.db.WithContext(ctx).Where("workflow_id = ?", workflowID)
	if filter.Status != "" {
		tx = tx.Where("status = ?", filter.Status)
	}
	if filter.Cursor != "" {
		var c paginationpkg.Cursor
		if err := paginationpkg.DecodeCursor(filter.Cursor, &c); err != nil {
			return nil, "", fmt.Errorf("workflowstore.ListVersions: %w", err)
		}
		tx = tx.Where("(created_at, id) < (?, ?)", c.CreatedAt, c.ID)
	}
	var rows []workflowdomain.Version
	if err := tx.Order("created_at DESC, id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("workflowstore.ListVersions: %w", err)
	}
	next := ""
	if len(rows) > limit {
		last := rows[limit-1]
		var err error
		next, err = paginationpkg.EncodeCursor(paginationpkg.Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return nil, "", fmt.Errorf("workflowstore.ListVersions: %w", err)
		}
		rows = rows[:limit]
	}
	out := make([]*workflowdomain.Version, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, next, nil
}

// GetPending returns the active pending version (at most one); ErrPendingNotFound if none.
//
// GetPending 返当前 pending（至多一个）；无返 ErrPendingNotFound。
func (s *Store) GetPending(ctx context.Context, workflowID string) (*workflowdomain.Version, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var wf workflowdomain.Workflow
	if err := s.db.WithContext(ctx).Select("id").Where("id = ? AND user_id = ?", workflowID, uid).
		First(&wf).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, workflowdomain.ErrNotFound
		}
		return nil, fmt.Errorf("workflowstore.GetPending: %w", err)
	}
	var v workflowdomain.Version
	res := s.db.WithContext(ctx).Where("workflow_id = ? AND status = ?",
		workflowID, workflowdomain.StatusPending).First(&v)
	if errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return nil, workflowdomain.ErrPendingNotFound
	}
	if res.Error != nil {
		return nil, fmt.Errorf("workflowstore.GetPending: %w", res.Error)
	}
	return &v, nil
}

// UpdateVersionStatus transitions status; versionN non-nil iff transitioning to accepted.
//
// UpdateVersionStatus 状态机转换；转 accepted 时 versionN 非 nil。
func (s *Store) UpdateVersionStatus(ctx context.Context, versionID, status string, versionN *int) error {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return err
	}
	updates := map[string]any{"status": status}
	if versionN != nil {
		updates["version"] = *versionN
	}
	res := s.db.WithContext(ctx).Model(&workflowdomain.Version{}).
		Where("id = ?", versionID).Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("workflowstore.UpdateVersionStatus: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return workflowdomain.ErrVersionNotFound
	}
	return nil
}

// HardDeleteVersion physically deletes one Version row by ID.
//
// HardDeleteVersion 按 ID 物理删 Version 行。
func (s *Store) HardDeleteVersion(ctx context.Context, versionID string) error {
	if err := s.db.WithContext(ctx).Where("id = ?", versionID).
		Delete(&workflowdomain.Version{}).Error; err != nil {
		return fmt.Errorf("workflowstore.HardDeleteVersion: %w", err)
	}
	return nil
}

// HardDeleteOldestAccepted keeps `keep` newest accepted versions, hard-deletes the rest.
//
// HardDeleteOldestAccepted 保留 keep 个最新 accepted 版本，其余物理删。
func (s *Store) HardDeleteOldestAccepted(ctx context.Context, workflowID string, keep int) error {
	if keep <= 0 {
		keep = workflowdomain.AcceptedVersionCap
	}
	var ids []string
	if err := s.db.WithContext(ctx).
		Model(&workflowdomain.Version{}).
		Where("workflow_id = ? AND status = ?", workflowID, workflowdomain.StatusAccepted).
		Order("created_at DESC, id DESC").
		Offset(keep).
		Pluck("id", &ids).Error; err != nil {
		return fmt.Errorf("workflowstore.HardDeleteOldestAccepted: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}
	if err := s.db.WithContext(ctx).
		Where("id IN ?", ids).
		Delete(&workflowdomain.Version{}).Error; err != nil {
		return fmt.Errorf("workflowstore.HardDeleteOldestAccepted: %w", err)
	}
	return nil
}

func (s *Store) assertVersionUser(ctx context.Context, v *workflowdomain.Version) error {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return err
	}
	var wf workflowdomain.Workflow
	res := s.db.WithContext(ctx).Select("id", "user_id").
		Where("id = ?", v.WorkflowID).First(&wf)
	if errors.Is(res.Error, gorm.ErrRecordNotFound) || wf.UserID != uid {
		return workflowdomain.ErrVersionNotFound
	}
	if res.Error != nil {
		return fmt.Errorf("workflowstore.assertVersionUser: %w", res.Error)
	}
	return nil
}

func isWorkflowDuplicateName(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if !strings.Contains(msg, "UNIQUE constraint failed") {
		return false
	}
	return strings.Contains(msg, "idx_workflows_user_name_active") ||
		strings.Contains(msg, "workflows.user_id, workflows.name") ||
		strings.Contains(msg, "workflows.name, workflows.user_id")
}

// Package function (infra/store/function) is the GORM-backed implementation
// of the domain function Repository port. All methods scope by the userID
// carried in ctx — callers MUST have run the InjectUserID middleware.
//
// The package shares its name with domain/function by design; external
// callers alias at import: `functionstore "…/infra/store/function"`.
//
// Package function (infra/store/function) 是 domain function Repository
// 的 GORM 实现。所有方法按 ctx userID 过滤;调用方先跑 InjectUserID 中间件。
// 包名跟 domain/function 同名是刻意的;外部 import 起别名 functionstore。
package function

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Store is the GORM implementation of functiondomain.Repository.
//
// Store 是 functiondomain.Repository 的 GORM 实现。
type Store struct {
	db *gorm.DB
}

// New constructs a Store bound to the given *gorm.DB.
//
// New 基于给定 *gorm.DB 构造 Store。
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Compile-time interface assertion.
//
// 编译期接口兼容性断言。
var _ functiondomain.Repository = (*Store)(nil)

// AutoMigrateModels returns the GORM models to register in db.AutoMigrate
// (called from cmd/server/main.go).
//
// AutoMigrateModels 返回 cmd/server/main.go 注册 AutoMigrate 用的 GORM models。
func AutoMigrateModels() []interface{} {
	return []interface{}{
		&functiondomain.Function{},
		&functiondomain.Version{},
		&functiondomain.Execution{},
	}
}

// ── Function CRUD ────────────────────────────────────────────────────────────

// SaveFunction inserts or updates by primary key. UNIQUE violation on
// (user_id, name) WHERE deleted_at IS NULL is translated to ErrDuplicateName.
//
// SaveFunction 按主键插入或更新;name 重复(partial UNIQUE)返 ErrDuplicateName。
func (s *Store) SaveFunction(ctx context.Context, f *functiondomain.Function) error {
	if err := s.db.WithContext(ctx).Save(f).Error; err != nil {
		if isFunctionDuplicateName(err) {
			return functiondomain.ErrDuplicateName
		}
		return fmt.Errorf("functionstore.SaveFunction: %w", err)
	}
	return nil
}

// GetFunction fetches by id, scoped to caller. ErrNotFound on miss.
//
// GetFunction 按 id 查,按调用者过滤;未命中返 ErrNotFound。
func (s *Store) GetFunction(ctx context.Context, id string) (*functiondomain.Function, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var f functiondomain.Function
	err = s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, uid).
		First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, functiondomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("functionstore.GetFunction: %w", err)
	}
	return &f, nil
}

// GetFunctionByName fetches by name (scoped to caller) — for create-time
// duplicate check (race with concurrent Save still caught by partial UNIQUE).
//
// GetFunctionByName 按 name 查;create 时查重名用(竞态由 partial UNIQUE 兜底)。
func (s *Store) GetFunctionByName(ctx context.Context, name string) (*functiondomain.Function, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var f functiondomain.Function
	err = s.db.WithContext(ctx).
		Where("name = ? AND user_id = ?", name, uid).
		First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, functiondomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("functionstore.GetFunctionByName: %w", err)
	}
	return &f, nil
}

// GetFunctionsByIDs batch fetches by id slice, preserving input order (used
// by search after LLM returns ranked IDs).
//
// GetFunctionsByIDs 按 id 切片批量查,保持输入顺序。
func (s *Store) GetFunctionsByIDs(ctx context.Context, ids []string) ([]*functiondomain.Function, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}

	var rows []*functiondomain.Function
	if err := s.db.WithContext(ctx).
		Where("id IN ? AND user_id = ?", ids, uid).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("functionstore.GetFunctionsByIDs: %w", err)
	}

	// Preserve input order
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

// ListFunctions returns a cursor-paginated page of live functions for the
// caller. Tuple cursor (created_at, id) for stable pagination across identical
// timestamps.
//
// ListFunctions 返当前用户活跃 function 的 cursor 分页;tuple cursor 保证稳定。
func (s *Store) ListFunctions(ctx context.Context, filter functiondomain.ListFilter) ([]*functiondomain.Function, string, error) {
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

	q := s.db.WithContext(ctx).Where("user_id = ?", uid)
	if filter.Cursor != "" {
		var c paginationpkg.Cursor
		if err := paginationpkg.DecodeCursor(filter.Cursor, &c); err != nil {
			return nil, "", fmt.Errorf("functionstore.ListFunctions: %w", err)
		}
		q = q.Where("(created_at, id) < (?, ?)", c.CreatedAt, c.ID)
	}

	var rows []*functiondomain.Function
	if err := q.Order("created_at DESC, id DESC").
		Limit(limit + 1).
		Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("functionstore.ListFunctions: %w", err)
	}

	var next string
	if len(rows) > limit {
		last := rows[limit-1]
		next, err = paginationpkg.EncodeCursor(paginationpkg.Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return nil, "", fmt.Errorf("functionstore.ListFunctions: %w", err)
		}
		rows = rows[:limit]
	}
	return rows, next, nil
}

// ListAllFunctions returns all live functions for caller (no pagination).
// Used by SearchFunction (LLM ranking) and CatalogSource.ListItems.
//
// ListAllFunctions 返当前用户全部活跃 function(无分页);SearchFunction +
// CatalogSource 用。
func (s *Store) ListAllFunctions(ctx context.Context) ([]*functiondomain.Function, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}

	var rows []*functiondomain.Function
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", uid).
		Order("created_at DESC, id DESC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("functionstore.ListAllFunctions: %w", err)
	}
	return rows, nil
}

// DeleteFunction soft-deletes by id, scoped to caller. ErrNotFound if no live
// row matched (so retries don't silently succeed).
//
// DeleteFunction 软删,按调用者过滤;未命中返 ErrNotFound(让重试不静默成功)。
func (s *Store) DeleteFunction(ctx context.Context, id string) error {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return err
	}
	res := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, uid).
		Delete(&functiondomain.Function{})
	if res.Error != nil {
		return fmt.Errorf("functionstore.DeleteFunction: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return functiondomain.ErrNotFound
	}
	return nil
}

// SetActiveVersion updates Function.ActiveVersionID atomically. Used by
// accept-pending / revert flows.
//
// SetActiveVersion 原子更新 ActiveVersionID(accept / revert 用)。
func (s *Store) SetActiveVersion(ctx context.Context, functionID, versionID string) error {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return err
	}
	res := s.db.WithContext(ctx).
		Model(&functiondomain.Function{}).
		Where("id = ? AND user_id = ?", functionID, uid).
		Update("active_version_id", versionID)
	if res.Error != nil {
		return fmt.Errorf("functionstore.SetActiveVersion: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return functiondomain.ErrNotFound
	}
	return nil
}

// ── Versions ─────────────────────────────────────────────────────────────────

// SaveVersion inserts or updates a FunctionVersion by primary key.
//
// SaveVersion 按主键 upsert FunctionVersion。
func (s *Store) SaveVersion(ctx context.Context, v *functiondomain.Version) error {
	if err := s.db.WithContext(ctx).Save(v).Error; err != nil {
		return fmt.Errorf("functionstore.SaveVersion: %w", err)
	}
	return nil
}

// GetVersion fetches by version id. ErrVersionNotFound on miss.
//
// GetVersion 按 version id 查;未命中返 ErrVersionNotFound。
func (s *Store) GetVersion(ctx context.Context, versionID string) (*functiondomain.Version, error) {
	var v functiondomain.Version
	err := s.db.WithContext(ctx).Where("id = ?", versionID).First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, functiondomain.ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("functionstore.GetVersion: %w", err)
	}
	return &v, nil
}

// GetVersionByNumber fetches an accepted version by (function_id, version_int).
// Used by revert flow.
//
// GetVersionByNumber 按 (function_id, version 整数) 查 accepted 版本;revert 用。
func (s *Store) GetVersionByNumber(ctx context.Context, functionID string, versionN int) (*functiondomain.Version, error) {
	var v functiondomain.Version
	err := s.db.WithContext(ctx).
		Where("function_id = ? AND status = ? AND version = ?", functionID, functiondomain.StatusAccepted, versionN).
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, functiondomain.ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("functionstore.GetVersionByNumber: %w", err)
	}
	return &v, nil
}

// ListVersions returns cursor-paginated versions for a function, newest first.
// Filter.Status filters by 'pending' / 'accepted' / 'rejected' if non-empty.
//
// ListVersions 返某 function 版本 cursor 分页(新→旧);可按 status 过滤。
func (s *Store) ListVersions(ctx context.Context, functionID string, filter functiondomain.VersionListFilter) ([]*functiondomain.Version, string, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	q := s.db.WithContext(ctx).Where("function_id = ?", functionID)
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.Cursor != "" {
		var c paginationpkg.Cursor
		if err := paginationpkg.DecodeCursor(filter.Cursor, &c); err != nil {
			return nil, "", fmt.Errorf("functionstore.ListVersions: %w", err)
		}
		q = q.Where("(created_at, id) < (?, ?)", c.CreatedAt, c.ID)
	}

	var rows []*functiondomain.Version
	if err := q.Order("created_at DESC, id DESC").
		Limit(limit + 1).
		Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("functionstore.ListVersions: %w", err)
	}

	var next string
	if len(rows) > limit {
		last := rows[limit-1]
		cur, err := paginationpkg.EncodeCursor(paginationpkg.Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return nil, "", fmt.Errorf("functionstore.ListVersions: %w", err)
		}
		next = cur
		rows = rows[:limit]
	}
	return rows, next, nil
}

// GetPending returns the active pending version for a function. ErrPendingNotFound
// if none. There SHOULD be at most one pending per function — service-layer
// invariant (DB doesn't enforce uniqueness across pending).
//
// GetPending 返某 function 的活动 pending 版本(应至多一个,service 层保证);
// 无则返 ErrPendingNotFound。
func (s *Store) GetPending(ctx context.Context, functionID string) (*functiondomain.Version, error) {
	var v functiondomain.Version
	err := s.db.WithContext(ctx).
		Where("function_id = ? AND status = ?", functionID, functiondomain.StatusPending).
		Order("created_at DESC, id DESC").
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, functiondomain.ErrPendingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("functionstore.GetPending: %w", err)
	}
	return &v, nil
}

// UpdateVersionStatus transitions a version's status. When transitioning to
// accepted, versionN must be non-nil. For pending / rejected pass nil.
//
// UpdateVersionStatus 状态机转换。转 accepted 时 versionN 非 nil;其他传 nil。
func (s *Store) UpdateVersionStatus(ctx context.Context, versionID, status string, versionN *int) error {
	updates := map[string]any{"status": status}
	if versionN != nil {
		updates["version"] = *versionN
	} else {
		updates["version"] = nil
	}
	res := s.db.WithContext(ctx).
		Model(&functiondomain.Version{}).
		Where("id = ?", versionID).
		Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("functionstore.UpdateVersionStatus: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return functiondomain.ErrVersionNotFound
	}
	return nil
}

// UpdateVersionEnv writes the env_* fields atomically. Called by sandbox sync.
//
// UpdateVersionEnv 原子写 env_* 字段(sandbox sync 用)。
func (s *Store) UpdateVersionEnv(ctx context.Context, versionID, envStatus, envError, envSyncStage, envSyncDetail string, syncedAt *time.Time) error {
	updates := map[string]any{
		"env_status":      envStatus,
		"env_error":       envError,
		"env_sync_stage":  envSyncStage,
		"env_sync_detail": envSyncDetail,
		"env_synced_at":   syncedAt, // *time.Time — nil writes NULL
	}
	res := s.db.WithContext(ctx).
		Model(&functiondomain.Version{}).
		Where("id = ?", versionID).
		Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("functionstore.UpdateVersionEnv: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return functiondomain.ErrVersionNotFound
	}
	return nil
}

// HardDeleteVersion physically deletes one Version row by ID. function_versions
// has no soft-delete column so this is an unconditional DELETE.
//
// HardDeleteVersion 按 ID 物理删 Version 行(function_versions 无软删列)。
func (s *Store) HardDeleteVersion(ctx context.Context, versionID string) error {
	if err := s.db.WithContext(ctx).
		Where("id = ?", versionID).
		Delete(&functiondomain.Version{}).Error; err != nil {
		return fmt.Errorf("functionstore.HardDeleteVersion: %w", err)
	}
	return nil
}

// HardDeleteOldestAccepted keeps `keep` newest accepted versions per function
// and HARD-deletes the rest (Version table has no soft-delete column). Called
// from service layer after each new accept.
//
// HardDeleteOldestAccepted 保留 keep 个最新 accepted 版本,其余 hard delete。
func (s *Store) HardDeleteOldestAccepted(ctx context.Context, functionID string, keep int) error {
	if keep <= 0 {
		keep = functiondomain.AcceptedVersionCap
	}

	var ids []string
	if err := s.db.WithContext(ctx).
		Model(&functiondomain.Version{}).
		Where("function_id = ? AND status = ?", functionID, functiondomain.StatusAccepted).
		Order("created_at DESC, id DESC").
		Offset(keep).
		Pluck("id", &ids).Error; err != nil {
		return fmt.Errorf("functionstore.HardDeleteOldestAccepted: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}

	if err := s.db.WithContext(ctx).
		Where("id IN ?", ids).
		Delete(&functiondomain.Version{}).Error; err != nil {
		return fmt.Errorf("functionstore.HardDeleteOldestAccepted: %w", err)
	}
	return nil
}

// isFunctionDuplicateName detects SQLite UNIQUE constraint violation on the
// functions partial UNIQUE index (idx_functions_user_name_active in
// schema_extras). modernc.org/sqlite errors contain the literal
// "UNIQUE constraint failed" + table name.
//
// isFunctionDuplicateName 检测 functions 表 partial UNIQUE 违反。
func isFunctionDuplicateName(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") &&
		strings.Contains(msg, "functions")
}

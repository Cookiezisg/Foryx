// Package handler is the GORM-backed handlerdomain.Repository, scoped by ctx userID.
//
// Package handler 是 handlerdomain.Repository 的 GORM 实现，按 ctx userID 过滤。
package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Store is the GORM implementation of handlerdomain.Repository.
//
// Store 是 handlerdomain.Repository 的 GORM 实现。
type Store struct {
	db *gorm.DB
}

// New constructs a Store bound to the given *gorm.DB.
//
// New 基于给定 *gorm.DB 构造 Store。
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

var _ handlerdomain.Repository = (*Store)(nil)

// AutoMigrateModels returns the GORM models to register in db.AutoMigrate.
//
// AutoMigrateModels 返 AutoMigrate 用的 GORM models。
func AutoMigrateModels() []interface{} {
	return []interface{}{
		&handlerdomain.Handler{},
		&handlerdomain.Version{},
		&handlerdomain.Call{},
	}
}

// SaveHandler upserts by PK; partial-UNIQUE name violation maps to ErrDuplicateName.
//
// SaveHandler 按主键 upsert；name partial UNIQUE 违反返 ErrDuplicateName。
func (s *Store) SaveHandler(ctx context.Context, h *handlerdomain.Handler) error {
	if err := s.db.WithContext(ctx).Save(h).Error; err != nil {
		if isHandlerDuplicateName(err) {
			return handlerdomain.ErrDuplicateName
		}
		return fmt.Errorf("handlerstore.SaveHandler: %w", err)
	}
	return nil
}

// GetHandler fetches by id; ErrNotFound on miss.
//
// GetHandler 按 id 查；未命中返 ErrNotFound。
func (s *Store) GetHandler(ctx context.Context, id string) (*handlerdomain.Handler, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var h handlerdomain.Handler
	err = s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, uid).
		First(&h).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, handlerdomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("handlerstore.GetHandler: %w", err)
	}
	return &h, nil
}

// GetHandlerByName fetches by name for create-time dup check (race caught by partial UNIQUE).
//
// GetHandlerByName 按 name 查（create 查重；竞态由 partial UNIQUE 兜底）。
func (s *Store) GetHandlerByName(ctx context.Context, name string) (*handlerdomain.Handler, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var h handlerdomain.Handler
	err = s.db.WithContext(ctx).
		Where("name = ? AND user_id = ?", name, uid).
		First(&h).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, handlerdomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("handlerstore.GetHandlerByName: %w", err)
	}
	return &h, nil
}

// GetHandlersByIDs batch fetches by id slice, preserving input order.
//
// GetHandlersByIDs 按 id 切片批量查，保持输入顺序。
func (s *Store) GetHandlersByIDs(ctx context.Context, ids []string) ([]*handlerdomain.Handler, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}

	var rows []*handlerdomain.Handler
	if err := s.db.WithContext(ctx).
		Where("id IN ? AND user_id = ?", ids, uid).
		Find(&rows).Error; err != nil {
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

// ListHandlers returns cursor-paginated handlers for caller; (created_at, id) DESC.
//
// ListHandlers 返当前用户活跃 handler 分页（新→旧）。
func (s *Store) ListHandlers(ctx context.Context, filter handlerdomain.ListFilter) ([]*handlerdomain.Handler, string, error) {
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
			return nil, "", fmt.Errorf("handlerstore.ListHandlers: %w", err)
		}
		q = q.Where("(created_at, id) < (?, ?)", c.CreatedAt, c.ID)
	}

	var rows []*handlerdomain.Handler
	if err := q.Order("created_at DESC, id DESC").
		Limit(limit + 1).
		Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("handlerstore.ListHandlers: %w", err)
	}

	var next string
	if len(rows) > limit {
		last := rows[limit-1]
		next, err = paginationpkg.EncodeCursor(paginationpkg.Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return nil, "", fmt.Errorf("handlerstore.ListHandlers: %w", err)
		}
		rows = rows[:limit]
	}
	return rows, next, nil
}

// ListAllHandlers returns all live handlers for caller (no pagination).
//
// ListAllHandlers 返当前用户全部活跃 handler（无分页）。
func (s *Store) ListAllHandlers(ctx context.Context) ([]*handlerdomain.Handler, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}

	var rows []*handlerdomain.Handler
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", uid).
		Order("created_at DESC, id DESC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("handlerstore.ListAllHandlers: %w", err)
	}
	return rows, nil
}

// DeleteHandler soft-deletes by id; ErrNotFound on miss.
//
// DeleteHandler 按 id 软删；未命中返 ErrNotFound。
func (s *Store) DeleteHandler(ctx context.Context, id string) error {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return err
	}
	res := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, uid).
		Delete(&handlerdomain.Handler{})
	if res.Error != nil {
		return fmt.Errorf("handlerstore.DeleteHandler: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return handlerdomain.ErrNotFound
	}
	return nil
}

// SetActiveVersion atomically updates Handler.ActiveVersionID (accept / revert flows).
//
// SetActiveVersion 原子更新 ActiveVersionID（accept / revert 用）。
func (s *Store) SetActiveVersion(ctx context.Context, handlerID, versionID string) error {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return err
	}
	res := s.db.WithContext(ctx).
		Model(&handlerdomain.Handler{}).
		Where("id = ? AND user_id = ?", handlerID, uid).
		Update("active_version_id", versionID)
	if res.Error != nil {
		return fmt.Errorf("handlerstore.SetActiveVersion: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return handlerdomain.ErrNotFound
	}
	return nil
}

// SaveVersion upserts by primary key.
//
// SaveVersion 按主键 upsert。
func (s *Store) SaveVersion(ctx context.Context, v *handlerdomain.Version) error {
	if err := s.db.WithContext(ctx).Save(v).Error; err != nil {
		return fmt.Errorf("handlerstore.SaveVersion: %w", err)
	}
	return nil
}

// GetVersion fetches by version id.
//
// GetVersion 按 version id 查。
func (s *Store) GetVersion(ctx context.Context, versionID string) (*handlerdomain.Version, error) {
	var v handlerdomain.Version
	err := s.db.WithContext(ctx).Where("id = ?", versionID).First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, handlerdomain.ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("handlerstore.GetVersion: %w", err)
	}
	return &v, nil
}

// GetVersionByNumber fetches accepted version by (handler_id, version int).
//
// GetVersionByNumber 按 (handler_id, version 整数) 查 accepted 版本。
func (s *Store) GetVersionByNumber(ctx context.Context, handlerID string, versionN int) (*handlerdomain.Version, error) {
	var v handlerdomain.Version
	err := s.db.WithContext(ctx).
		Where("handler_id = ? AND status = ? AND version = ?", handlerID, handlerdomain.StatusAccepted, versionN).
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, handlerdomain.ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("handlerstore.GetVersionByNumber: %w", err)
	}
	return &v, nil
}

// ListVersions returns cursor-paginated versions for a handler, newest first.
//
// ListVersions 返某 handler 版本的分页（新→旧）。
func (s *Store) ListVersions(ctx context.Context, handlerID string, filter handlerdomain.VersionListFilter) ([]*handlerdomain.Version, string, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	q := s.db.WithContext(ctx).Where("handler_id = ?", handlerID)
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.Cursor != "" {
		var c paginationpkg.Cursor
		if err := paginationpkg.DecodeCursor(filter.Cursor, &c); err != nil {
			return nil, "", fmt.Errorf("handlerstore.ListVersions: %w", err)
		}
		q = q.Where("(created_at, id) < (?, ?)", c.CreatedAt, c.ID)
	}

	var rows []*handlerdomain.Version
	if err := q.Order("created_at DESC, id DESC").
		Limit(limit + 1).
		Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("handlerstore.ListVersions: %w", err)
	}

	var next string
	if len(rows) > limit {
		last := rows[limit-1]
		cur, err := paginationpkg.EncodeCursor(paginationpkg.Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return nil, "", fmt.Errorf("handlerstore.ListVersions: %w", err)
		}
		next = cur
		rows = rows[:limit]
	}
	return rows, next, nil
}

// GetPending returns the active pending version; ErrPendingNotFound if none.
//
// GetPending 返活动 pending；无则 ErrPendingNotFound。
func (s *Store) GetPending(ctx context.Context, handlerID string) (*handlerdomain.Version, error) {
	var v handlerdomain.Version
	err := s.db.WithContext(ctx).
		Where("handler_id = ? AND status = ?", handlerID, handlerdomain.StatusPending).
		Order("created_at DESC, id DESC").
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, handlerdomain.ErrPendingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("handlerstore.GetPending: %w", err)
	}
	return &v, nil
}

// UpdateVersionStatus transitions status; versionN non-nil iff transitioning to accepted.
//
// UpdateVersionStatus 状态机转换；转 accepted 时 versionN 非 nil。
func (s *Store) UpdateVersionStatus(ctx context.Context, versionID, status string, versionN *int) error {
	updates := map[string]any{"status": status}
	if versionN != nil {
		updates["version"] = *versionN
	} else {
		updates["version"] = nil
	}
	res := s.db.WithContext(ctx).
		Model(&handlerdomain.Version{}).
		Where("id = ?", versionID).
		Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("handlerstore.UpdateVersionStatus: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return handlerdomain.ErrVersionNotFound
	}
	return nil
}

// UpdateVersionEnv atomically writes all env_* fields.
//
// UpdateVersionEnv 原子写 env_* 字段。
func (s *Store) UpdateVersionEnv(ctx context.Context, versionID, envStatus, envError, envSyncStage, envSyncDetail string, syncedAt *time.Time) error {
	updates := map[string]any{
		"env_status":      envStatus,
		"env_error":       envError,
		"env_sync_stage":  envSyncStage,
		"env_sync_detail": envSyncDetail,
		"env_synced_at":   syncedAt,
	}
	res := s.db.WithContext(ctx).
		Model(&handlerdomain.Version{}).
		Where("id = ?", versionID).
		Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("handlerstore.UpdateVersionEnv: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return handlerdomain.ErrVersionNotFound
	}
	return nil
}

// HardDeleteVersion physically deletes one Version row (no soft-delete column).
//
// HardDeleteVersion 按 ID 物理删 Version 行（无软删列）。
func (s *Store) HardDeleteVersion(ctx context.Context, versionID string) error {
	if err := s.db.WithContext(ctx).
		Where("id = ?", versionID).
		Delete(&handlerdomain.Version{}).Error; err != nil {
		return fmt.Errorf("handlerstore.HardDeleteVersion: %w", err)
	}
	return nil
}

// HardDeleteOldestAccepted keeps `keep` newest accepted versions, hard-deletes the rest.
//
// HardDeleteOldestAccepted 保留 keep 个最新 accepted，其余物理删。
func (s *Store) HardDeleteOldestAccepted(ctx context.Context, handlerID string, keep int) error {
	if keep <= 0 {
		keep = handlerdomain.AcceptedVersionCap
	}

	var ids []string
	if err := s.db.WithContext(ctx).
		Model(&handlerdomain.Version{}).
		Where("handler_id = ? AND status = ?", handlerID, handlerdomain.StatusAccepted).
		Order("created_at DESC, id DESC").
		Offset(keep).
		Pluck("id", &ids).Error; err != nil {
		return fmt.Errorf("handlerstore.HardDeleteOldestAccepted: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}

	if err := s.db.WithContext(ctx).
		Where("id IN ?", ids).
		Delete(&handlerdomain.Version{}).Error; err != nil {
		return fmt.Errorf("handlerstore.HardDeleteOldestAccepted: %w", err)
	}
	return nil
}

// UpdateConfigEncrypted writes the AES-GCM ciphertext blob (repo opaque to ciphertext).
//
// UpdateConfigEncrypted 写 AES-GCM 密文 blob（repo 对密文不透明）。
func (s *Store) UpdateConfigEncrypted(ctx context.Context, handlerID, ciphertext string) error {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return err
	}
	res := s.db.WithContext(ctx).
		Model(&handlerdomain.Handler{}).
		Where("id = ? AND user_id = ?", handlerID, uid).
		Update("config_encrypted", ciphertext)
	if res.Error != nil {
		return fmt.Errorf("handlerstore.UpdateConfigEncrypted: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return handlerdomain.ErrNotFound
	}
	return nil
}

// ClearConfig wipes ConfigEncrypted back to unconfigured state.
//
// ClearConfig 清 ConfigEncrypted（回未配置态）。
func (s *Store) ClearConfig(ctx context.Context, handlerID string) error {
	return s.UpdateConfigEncrypted(ctx, handlerID, "")
}

// GetConfigEncrypted returns the raw ciphertext ("" if unconfigured).
//
// GetConfigEncrypted 返原始密文（"" = 未配置）。
func (s *Store) GetConfigEncrypted(ctx context.Context, handlerID string) (string, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return "", err
	}
	var ciphertext string
	err = s.db.WithContext(ctx).
		Model(&handlerdomain.Handler{}).
		Select("config_encrypted").
		Where("id = ? AND user_id = ?", handlerID, uid).
		Scan(&ciphertext).Error
	if err != nil {
		return "", fmt.Errorf("handlerstore.GetConfigEncrypted: %w", err)
	}
	// Scan leaves zero on no-row — explicit existence check distinguishes missing vs empty.
	// Scan 无行不报错——单独 count 查区分"无行" vs "有行但密文空"。
	var count int64
	if err := s.db.WithContext(ctx).
		Model(&handlerdomain.Handler{}).
		Where("id = ? AND user_id = ?", handlerID, uid).
		Count(&count).Error; err != nil {
		return "", fmt.Errorf("handlerstore.GetConfigEncrypted: count: %w", err)
	}
	if count == 0 {
		return "", handlerdomain.ErrNotFound
	}
	return ciphertext, nil
}

func isHandlerDuplicateName(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") &&
		strings.Contains(msg, "handlers")
}

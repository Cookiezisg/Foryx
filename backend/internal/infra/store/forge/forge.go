// Package forge (infra/store/forge) is the GORM-backed implementation of the
// domain forge Repository port. Every method scopes queries to the userID
// carried in ctx — callers MUST have run the InjectUserID middleware.
//
// The package shares its name with domain/forge and app/forge by design;
// external callers alias at import: `forgestore "…/infra/store/forge"`.
//
// Package forge（infra/store/forge）是 domain forge Repository port 的 GORM 实现。
// 所有方法按 ctx 中的 userID 过滤——调用方必须先经过 InjectUserID 中间件。
//
// 本包与 domain/forge、app/forge 同名是刻意的；外部调用方 import 时起别名，
// 如 `forgestore "…/infra/store/forge"`。
package forge

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Store is the GORM implementation of forgedomain.Repository.
//
// Store 是 forgedomain.Repository 的 GORM 实现。
type Store struct {
	db *gorm.DB
}

// New constructs a Store bound to the given *gorm.DB.
//
// New 基于给定 *gorm.DB 构造 Store。
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// ── Forge CRUD ─────────────────────────────────────────────────────────────────

// SaveForge inserts or updates a Forge by primary key.
//
// SaveForge 按主键插入或更新 Forge。
func (s *Store) SaveForge(ctx context.Context, f *forgedomain.Forge) error {
	if err := s.db.WithContext(ctx).Save(f).Error; err != nil {
		return fmt.Errorf("forgestore.SaveForge: %w", err)
	}
	return nil
}

// GetForge fetches a single live Forge by id for the current user.
//
// GetForge 按 id 查当前用户的单条活跃 Forge。
func (s *Store) GetForge(ctx context.Context, id string) (*forgedomain.Forge, error) {
	userID, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var f forgedomain.Forge
	err = s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, forgedomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("forgestore.GetForge: %w", err)
	}
	return &f, nil
}

// GetForgesByIDs fetches multiple live Forges by id slice, preserving the
// input order. IDs that don't exist or belong to another user are silently omitted.
//
// GetForgesByIDs 按 id 切片批量查活跃 Forge，保持输入顺序。
// 不存在或属于其他用户的 ID 静默忽略。
func (s *Store) GetForgesByIDs(ctx context.Context, ids []string) ([]*forgedomain.Forge, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	userID, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var rows []*forgedomain.Forge
	if err = s.db.WithContext(ctx).
		Where("id IN ? AND user_id = ?", ids, userID).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("forgestore.GetForgesByIDs: %w", err)
	}
	idx := make(map[string]*forgedomain.Forge, len(rows))
	for _, r := range rows {
		idx[r.ID] = r
	}
	ordered := make([]*forgedomain.Forge, 0, len(ids))
	for _, id := range ids {
		if f, ok := idx[id]; ok {
			ordered = append(ordered, f)
		}
	}
	return ordered, nil
}

// ListForges returns a cursor-paginated page of live forges for the current user,
// ordered by created_at DESC with id as tiebreaker.
//
// ListForges 返回当前用户活跃 forge 的 cursor 分页结果，按 created_at DESC 排序。
func (s *Store) ListForges(ctx context.Context, filter forgedomain.ListFilter) ([]*forgedomain.Forge, string, error) {
	userID, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, "", err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	q := s.db.WithContext(ctx).Where("user_id = ?", userID)
	if filter.Cursor != "" {
		var c paginationpkg.Cursor
		if err := paginationpkg.DecodeCursor(filter.Cursor, &c); err != nil {
			return nil, "", fmt.Errorf("forgestore.ListForges: %w", err)
		}
		q = q.Where("(created_at, id) < (?, ?)", c.CreatedAt, c.ID)
	}
	var rows []*forgedomain.Forge
	if err = q.Order("created_at DESC, id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("forgestore.ListForges: %w", err)
	}
	var next string
	if len(rows) > limit {
		last := rows[limit-1]
		next, err = paginationpkg.EncodeCursor(paginationpkg.Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return nil, "", fmt.Errorf("forgestore.ListForges: %w", err)
		}
		rows = rows[:limit]
	}
	return rows, next, nil
}

// ListAllForges returns all live forges for the current user without pagination.
//
// ListAllForges 返回当前用户全部活跃 forge，不分页。
func (s *Store) ListAllForges(ctx context.Context) ([]*forgedomain.Forge, error) {
	userID, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var rows []*forgedomain.Forge
	if err = s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("forgestore.ListAllForges: %w", err)
	}
	return rows, nil
}

// DeleteForge soft-deletes a forge by id for the current user.
//
// DeleteForge 软删除当前用户的指定 forge。
func (s *Store) DeleteForge(ctx context.Context, id string) error {
	userID, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if err = s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&forgedomain.Forge{}).Error; err != nil {
		return fmt.Errorf("forgestore.DeleteForge: %w", err)
	}
	return nil
}

// UpdateForgeActiveVersion sets forge.ActiveVersionID for the current
// user's forge. Used by AcceptPending after promoting a pending version
// to accepted, and by RevertToVersion after switching back to an older
// version. No-op (no error) if the forge id doesn't match — service
// layer is responsible for validating the forge exists first.
//
// UpdateForgeActiveVersion 设当前用户某 forge 的 ActiveVersionID。
// AcceptPending 把 pending 提升 accepted 后调；RevertToVersion 切回旧版本
// 后调。forge id 不匹配时静默 no-op——service 层负责前置校验 forge 存在。
func (s *Store) UpdateForgeActiveVersion(ctx context.Context, forgeID, versionID string) error {
	userID, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if err = s.db.WithContext(ctx).
		Model(&forgedomain.Forge{}).
		Where("id = ? AND user_id = ?", forgeID, userID).
		Update("active_version_id", versionID).Error; err != nil {
		return fmt.Errorf("forgestore.UpdateForgeActiveVersion: %w", err)
	}
	return nil
}

// ── Versions (including pending) ──────────────────────────────────────────────

// SaveVersion inserts a ForgeVersion record.
//
// SaveVersion 插入一条 ForgeVersion 记录。
func (s *Store) SaveVersion(ctx context.Context, v *forgedomain.ForgeVersion) error {
	if err := s.db.WithContext(ctx).Create(v).Error; err != nil {
		return fmt.Errorf("forgestore.SaveVersion: %w", err)
	}
	return nil
}

// GetVersion fetches the accepted ForgeVersion with the given version number.
//
// GetVersion 查询指定版本号的已接受版本记录。
func (s *Store) GetVersion(ctx context.Context, forgeID string, version int) (*forgedomain.ForgeVersion, error) {
	userID, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var v forgedomain.ForgeVersion
	err = s.db.WithContext(ctx).
		Where("forge_id = ? AND user_id = ? AND version = ? AND status = ?",
			forgeID, userID, version, forgedomain.VersionStatusAccepted).
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, forgedomain.ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("forgestore.GetVersion: %w", err)
	}
	return &v, nil
}

// GetActivePending returns the pending ForgeVersion for the forge.
//
// GetActivePending 返回 forge 当前的 pending ForgeVersion。
func (s *Store) GetActivePending(ctx context.Context, forgeID string) (*forgedomain.ForgeVersion, error) {
	userID, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var v forgedomain.ForgeVersion
	err = s.db.WithContext(ctx).
		Where("forge_id = ? AND user_id = ? AND status = ?",
			forgeID, userID, forgedomain.VersionStatusPending).
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, forgedomain.ErrPendingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("forgestore.GetActivePending: %w", err)
	}
	return &v, nil
}

// ListAcceptedVersions returns all accepted versions for a forge, newest first.
//
// ListAcceptedVersions 返回 forge 所有已接受版本，最新在前。
func (s *Store) ListAcceptedVersions(ctx context.Context, forgeID string) ([]*forgedomain.ForgeVersion, error) {
	userID, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var rows []*forgedomain.ForgeVersion
	if err = s.db.WithContext(ctx).
		Where("forge_id = ? AND user_id = ? AND status = ?",
			forgeID, userID, forgedomain.VersionStatusAccepted).
		Order("version DESC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("forgestore.ListAcceptedVersions: %w", err)
	}
	return rows, nil
}

// UpdateVersionStatus updates the status and optionally the version number.
//
// UpdateVersionStatus 更新 status 字段，可选分配版本号。
func (s *Store) UpdateVersionStatus(ctx context.Context, id, status string, version *int) error {
	updates := map[string]any{"status": status}
	if version != nil {
		updates["version"] = *version
	}
	if err := s.db.WithContext(ctx).
		Model(&forgedomain.ForgeVersion{}).
		Where("id = ?", id).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("forgestore.UpdateVersionStatus: %w", err)
	}
	return nil
}

// CountAcceptedVersions returns the number of accepted versions for a forge.
//
// CountAcceptedVersions 返回 forge 已接受版本数。
func (s *Store) CountAcceptedVersions(ctx context.Context, forgeID string) (int64, error) {
	var n int64
	if err := s.db.WithContext(ctx).Model(&forgedomain.ForgeVersion{}).
		Where("forge_id = ? AND status = ?", forgeID, forgedomain.VersionStatusAccepted).
		Count(&n).Error; err != nil {
		return 0, fmt.Errorf("forgestore.CountAcceptedVersions: %w", err)
	}
	return n, nil
}

// DeleteOldestAcceptedVersion hard-deletes the accepted version with the
// lowest version number for the given forge.
//
// DeleteOldestAcceptedVersion 硬删除指定 forge 版本号最小的已接受版本。
func (s *Store) DeleteOldestAcceptedVersion(ctx context.Context, forgeID string) error {
	var v forgedomain.ForgeVersion
	err := s.db.WithContext(ctx).
		Where("forge_id = ? AND status = ?", forgeID, forgedomain.VersionStatusAccepted).
		Order("version ASC").
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("forgestore.DeleteOldestAcceptedVersion: find: %w", err)
	}
	if err = s.db.WithContext(ctx).Delete(&v).Error; err != nil {
		return fmt.Errorf("forgestore.DeleteOldestAcceptedVersion: delete: %w", err)
	}
	return nil
}

// GetVersionByID fetches a ForgeVersion by primary key for the current
// user — works for pending / accepted / rejected without needing the
// version number. Used by sandbox sync flow which only carries the
// version's UUID.
//
// GetVersionByID 按主键查 ForgeVersion（按当前用户过滤）——pending /
// accepted / rejected 都可，不需版本号。供沙箱 sync 流使用。
func (s *Store) GetVersionByID(ctx context.Context, versionID string) (*forgedomain.ForgeVersion, error) {
	userID, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var v forgedomain.ForgeVersion
	err = s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", versionID, userID).
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, forgedomain.ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("forgestore.GetVersionByID: %w", err)
	}
	return &v, nil
}

// UpdateVersionEnvStatus updates EnvStatus + EnvError + EnvSyncedAt
// atomically. errMsg should be "" except when status == EnvStatusFailed;
// EnvSyncedAt is set to time.Now().UTC() when status transitions to
// EnvStatusReady, and cleared otherwise (so a re-sync from "ready" → "syncing"
// resets the prior success timestamp).
//
// UpdateVersionEnvStatus 原子更新 EnvStatus + EnvError + EnvSyncedAt。
// errMsg 仅在 status == EnvStatusFailed 时填；状态转 EnvStatusReady 时
// EnvSyncedAt 设为 time.Now().UTC()，其他状态清空（"ready" → "syncing"
// 重 sync 时重置前次成功时间戳）。
func (s *Store) UpdateVersionEnvStatus(ctx context.Context, versionID, status, errMsg string) error {
	updates := map[string]any{
		"env_status": status,
		"env_error":  errMsg,
	}
	if status == forgedomain.EnvStatusReady {
		updates["env_synced_at"] = time.Now().UTC()
	} else {
		updates["env_synced_at"] = nil
	}
	if err := s.db.WithContext(ctx).
		Model(&forgedomain.ForgeVersion{}).
		Where("id = ?", versionID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("forgestore.UpdateVersionEnvStatus: %w", err)
	}
	return nil
}

// UpdateVersionEnvProgress writes EnvSyncStage + EnvSyncDetail during
// active sync. Called from app/forge.Service's OnProgress callback fed
// to sandbox.Sync.
//
// UpdateVersionEnvProgress 在 sync 期间写 EnvSyncStage + EnvSyncDetail。
// 由 app/forge.Service 喂给 sandbox.Sync 的 OnProgress callback 调。
func (s *Store) UpdateVersionEnvProgress(ctx context.Context, versionID, stage, detail string) error {
	if err := s.db.WithContext(ctx).
		Model(&forgedomain.ForgeVersion{}).
		Where("id = ?", versionID).
		Updates(map[string]any{
			"env_sync_stage":  stage,
			"env_sync_detail": detail,
		}).Error; err != nil {
		return fmt.Errorf("forgestore.UpdateVersionEnvProgress: %w", err)
	}
	return nil
}

// UpdateVersionEnvID changes a non-accepted ForgeVersion's EnvID. The
// query refuses to mutate accepted rows — accepted history is immutable.
// Service layer should call GetVersionByID first to check status before
// invoking this; the WHERE clause is the safety net.
//
// UpdateVersionEnvID 改非 accepted ForgeVersion 的 EnvID。查询拒绝改
// accepted 行——已接受历史不可变。service 层应先 GetVersionByID 查 status
// 再调；WHERE 子句是兜底保险。
func (s *Store) UpdateVersionEnvID(ctx context.Context, versionID, envID string) error {
	if err := s.db.WithContext(ctx).
		Model(&forgedomain.ForgeVersion{}).
		Where("id = ? AND status != ?", versionID, forgedomain.VersionStatusAccepted).
		Update("env_id", envID).Error; err != nil {
		return fmt.Errorf("forgestore.UpdateVersionEnvID: %w", err)
	}
	return nil
}

// ListEnvIDsForForge returns the distinct non-empty EnvIDs in use across
// the forge's ForgeVersion rows, ordered most-recently-referenced first.
// The recency key is MAX(updated_at) per EnvID, so an old EnvID reused
// by a fresh pending counts as recent. trimEnvBuffer drops the tail
// past MaxEnvIDsPerForge.
//
// ListEnvIDsForForge 返回某 forge 全部 ForgeVersion 行用到的不重复非空
// EnvID，按最近引用排序。recency key 是 per-EnvID 的 MAX(updated_at)——
// 老 EnvID 被新 pending 复用也算最近。trimEnvBuffer 砍掉
// MaxEnvIDsPerForge 之外的尾部。
func (s *Store) ListEnvIDsForForge(ctx context.Context, forgeID string) ([]string, error) {
	userID, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	// Pluck only the env_id column — the MAX(updated_at) is used purely for
	// ORDER BY and never returned to Go. Avoids modernc.org/sqlite's habit
	// of giving aggregate columns string types that can't scan into time.Time.
	//
	// 只 Pluck env_id 列——MAX(updated_at) 仅用于 ORDER BY，不返 Go。
	// 避开 modernc.org/sqlite 把聚合列给成 string 类型而无法 scan 进 time.Time 的坑。
	var out []string
	if err = s.db.WithContext(ctx).
		Model(&forgedomain.ForgeVersion{}).
		Where("forge_id = ? AND user_id = ? AND env_id != ''", forgeID, userID).
		Group("env_id").
		Order("MAX(updated_at) DESC").
		Pluck("env_id", &out).Error; err != nil {
		return nil, fmt.Errorf("forgestore.ListEnvIDsForForge: %w", err)
	}
	return out, nil
}

// ── Test cases ────────────────────────────────────────────────────────────────

// SaveTestCase inserts a ForgeTestCase.
//
// SaveTestCase 插入 ForgeTestCase。
func (s *Store) SaveTestCase(ctx context.Context, tc *forgedomain.ForgeTestCase) error {
	if err := s.db.WithContext(ctx).Create(tc).Error; err != nil {
		return fmt.Errorf("forgestore.SaveTestCase: %w", err)
	}
	return nil
}

// GetTestCase fetches a test case by id.
//
// GetTestCase 按 id 查测试用例。
func (s *Store) GetTestCase(ctx context.Context, id string) (*forgedomain.ForgeTestCase, error) {
	userID, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var tc forgedomain.ForgeTestCase
	err = s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&tc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, forgedomain.ErrTestCaseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("forgestore.GetTestCase: %w", err)
	}
	return &tc, nil
}

// ListTestCases returns all test cases for the given forge, ordered by created_at ASC.
//
// ListTestCases 返回指定 forge 所有测试用例，按 created_at ASC 排序。
func (s *Store) ListTestCases(ctx context.Context, forgeID string) ([]*forgedomain.ForgeTestCase, error) {
	userID, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var rows []*forgedomain.ForgeTestCase
	if err = s.db.WithContext(ctx).
		Where("forge_id = ? AND user_id = ?", forgeID, userID).
		Order("created_at ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("forgestore.ListTestCases: %w", err)
	}
	return rows, nil
}

// DeleteTestCase hard-deletes a test case by id.
//
// DeleteTestCase 硬删除测试用例。
func (s *Store) DeleteTestCase(ctx context.Context, id string) error {
	userID, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if err = s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&forgedomain.ForgeTestCase{}).Error; err != nil {
		return fmt.Errorf("forgestore.DeleteTestCase: %w", err)
	}
	return nil
}

// ── Executions (unified run + test history) ───────────────────────────────────

// SaveExecution inserts a ForgeExecution record.
//
// SaveExecution 插入一条 ForgeExecution 记录。
func (s *Store) SaveExecution(ctx context.Context, e *forgedomain.ForgeExecution) error {
	if err := s.db.WithContext(ctx).Create(e).Error; err != nil {
		return fmt.Errorf("forgestore.SaveExecution: %w", err)
	}
	return nil
}

// ListExecutions returns a cursor-paginated page of execution records matching
// the filter. Order: BatchID set → created_at ASC (single batch in run order);
// otherwise created_at DESC. Pagination uses (created_at, id) tuple via the
// shared paginationpkg.Cursor.
//
// ListExecutions 返回匹配 filter 的执行记录 cursor 分页结果。排序：指定 BatchID
// 时按 created_at ASC（单批次按运行顺序）；否则 created_at DESC。分页用 (created_at, id)
// 元组，通过共享的 paginationpkg.Cursor。
func (s *Store) ListExecutions(ctx context.Context, filter forgedomain.ExecutionFilter) ([]*forgedomain.ForgeExecution, string, error) {
	userID, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, "", err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = paginationpkg.DefaultLimit
	}

	q := s.db.WithContext(ctx).Where("user_id = ?", userID)
	if filter.ForgeID != "" {
		q = q.Where("forge_id = ?", filter.ForgeID)
	}
	if filter.Kind != "" {
		q = q.Where("kind = ?", filter.Kind)
	}
	if filter.BatchID != "" {
		q = q.Where("batch_id = ?", filter.BatchID)
	}
	if filter.TestCaseID != "" {
		q = q.Where("test_case_id = ?", filter.TestCaseID)
	}
	if filter.ConversationID != "" {
		q = q.Where("conversation_id = ?", filter.ConversationID)
	}
	if filter.MessageID != "" {
		q = q.Where("message_id = ?", filter.MessageID)
	}
	if filter.ToolCallID != "" {
		q = q.Where("tool_call_id = ?", filter.ToolCallID)
	}

	// Cursor predicate flips with sort direction: DESC uses (c, id) <;
	// ASC uses (c, id) >.
	//
	// cursor 谓词随排序方向反转：DESC 用 (c, id) <；ASC 用 (c, id) >。
	asc := filter.BatchID != ""
	if filter.Cursor != "" {
		var c paginationpkg.Cursor
		if err := paginationpkg.DecodeCursor(filter.Cursor, &c); err != nil {
			return nil, "", fmt.Errorf("forgestore.ListExecutions: %w", err)
		}
		if asc {
			q = q.Where("(created_at, id) > (?, ?)", c.CreatedAt, c.ID)
		} else {
			q = q.Where("(created_at, id) < (?, ?)", c.CreatedAt, c.ID)
		}
	}
	if asc {
		q = q.Order("created_at ASC, id ASC")
	} else {
		q = q.Order("created_at DESC, id DESC")
	}

	var rows []*forgedomain.ForgeExecution
	if err = q.Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("forgestore.ListExecutions: %w", err)
	}
	var next string
	if len(rows) > limit {
		last := rows[limit-1]
		next, err = paginationpkg.EncodeCursor(paginationpkg.Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return nil, "", fmt.Errorf("forgestore.ListExecutions: %w", err)
		}
		rows = rows[:limit]
	}
	return rows, next, nil
}

// CountExecutions returns the total execution count for a forge (across all kinds).
//
// CountExecutions 返回 forge 执行记录总数（所有 kind 合计）。
func (s *Store) CountExecutions(ctx context.Context, forgeID string) (int64, error) {
	var n int64
	if err := s.db.WithContext(ctx).Model(&forgedomain.ForgeExecution{}).
		Where("forge_id = ?", forgeID).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("forgestore.CountExecutions: %w", err)
	}
	return n, nil
}

// DeleteOldestExecution hard-deletes the oldest execution record for a forge.
//
// DeleteOldestExecution 硬删除 forge 最早的执行记录。
func (s *Store) DeleteOldestExecution(ctx context.Context, forgeID string) error {
	var e forgedomain.ForgeExecution
	err := s.db.WithContext(ctx).
		Where("forge_id = ?", forgeID).
		Order("created_at ASC").
		First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("forgestore.DeleteOldestExecution: find: %w", err)
	}
	if err = s.db.WithContext(ctx).Delete(&e).Error; err != nil {
		return fmt.Errorf("forgestore.DeleteOldestExecution: delete: %w", err)
	}
	return nil
}

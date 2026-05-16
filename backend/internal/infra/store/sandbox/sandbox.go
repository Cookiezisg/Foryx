// Package sandbox is the GORM-backed sandboxdomain.Repository (system-level, no userID scope).
//
// Package sandbox 是 sandboxdomain.Repository 的 GORM 实现（系统级，无 userID 作用域）。
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

// Store is the GORM implementation of sandboxdomain.Repository.
//
// Store 是 sandboxdomain.Repository 的 GORM 实现。
type Store struct {
	db *gorm.DB
}

// New constructs a Store bound to the given *gorm.DB.
//
// New 基于给定 *gorm.DB 构造 Store。
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// CreateRuntime inserts a new Runtime row; UNIQUE(kind, version) collisions surface raw.
//
// CreateRuntime 插入新 Runtime 行；UNIQUE(kind, version) 冲突直接上抛。
func (s *Store) CreateRuntime(ctx context.Context, r *sandboxdomain.Runtime) error {
	if err := s.db.WithContext(ctx).Create(r).Error; err != nil {
		return fmt.Errorf("sandboxstore.CreateRuntime: %w", err)
	}
	return nil
}

// GetRuntime fetches a single Runtime by id; returns gorm.ErrRecordNotFound on miss.
//
// GetRuntime 按 id 查单条 Runtime；未命中返 gorm.ErrRecordNotFound。
func (s *Store) GetRuntime(ctx context.Context, id string) (*sandboxdomain.Runtime, error) {
	var r sandboxdomain.Runtime
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, gorm.ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sandboxstore.GetRuntime: %w", err)
	}
	return &r, nil
}

// FindRuntime looks up by exact (kind, version) UNIQUE pair; gorm.ErrRecordNotFound if missing.
//
// FindRuntime 按精确 (kind, version) UNIQUE 对查；未装返 gorm.ErrRecordNotFound。
func (s *Store) FindRuntime(ctx context.Context, kind, version string) (*sandboxdomain.Runtime, error) {
	var r sandboxdomain.Runtime
	err := s.db.WithContext(ctx).
		Where("kind = ? AND version = ?", kind, version).
		First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, gorm.ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sandboxstore.FindRuntime: %w", err)
	}
	return &r, nil
}

// ListRuntimes returns all installed runtimes, ordered by kind then version.
//
// ListRuntimes 返所有已装 runtime，按 kind 再 version 排序。
func (s *Store) ListRuntimes(ctx context.Context) ([]*sandboxdomain.Runtime, error) {
	var rows []*sandboxdomain.Runtime
	if err := s.db.WithContext(ctx).
		Order("kind, version").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("sandboxstore.ListRuntimes: %w", err)
	}
	return rows, nil
}

// UpdateRuntime persists changes to an existing Runtime row by primary key.
//
// UpdateRuntime 按主键持久化 Runtime 修改。
func (s *Store) UpdateRuntime(ctx context.Context, r *sandboxdomain.Runtime) error {
	if err := s.db.WithContext(ctx).Save(r).Error; err != nil {
		return fmt.Errorf("sandboxstore.UpdateRuntime: %w", err)
	}
	return nil
}

// DeleteRuntime hard-deletes a Runtime row by id (caller ensures no Env still references it).
//
// DeleteRuntime 按 id 硬删 Runtime 行（调用方需确保无 Env 引用）。
func (s *Store) DeleteRuntime(ctx context.Context, id string) error {
	if err := s.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&sandboxdomain.Runtime{}).Error; err != nil {
		return fmt.Errorf("sandboxstore.DeleteRuntime: %w", err)
	}
	return nil
}

// CreateEnv inserts a new Env row; UNIQUE(owner_kind, owner_id) collisions surface raw.
//
// CreateEnv 插入新 Env 行；UNIQUE(owner_kind, owner_id) 冲突直接上抛。
func (s *Store) CreateEnv(ctx context.Context, e *sandboxdomain.Env) error {
	if err := s.db.WithContext(ctx).Create(e).Error; err != nil {
		return fmt.Errorf("sandboxstore.CreateEnv: %w", err)
	}
	return nil
}

// GetEnv fetches a single Env by id; ErrEnvNotFound on miss.
//
// GetEnv 按 id 查 Env；未命中返 ErrEnvNotFound。
func (s *Store) GetEnv(ctx context.Context, id string) (*sandboxdomain.Env, error) {
	var e sandboxdomain.Env
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, sandboxdomain.ErrEnvNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sandboxstore.GetEnv: %w", err)
	}
	return &e, nil
}

// FindEnvByOwner looks up by (owner_kind, owner_id) UNIQUE pair; ErrEnvNotFound on miss.
//
// FindEnvByOwner 按 (owner_kind, owner_id) UNIQUE 对查；未命中返 ErrEnvNotFound。
func (s *Store) FindEnvByOwner(ctx context.Context, ownerKind, ownerID string) (*sandboxdomain.Env, error) {
	var e sandboxdomain.Env
	err := s.db.WithContext(ctx).
		Where("owner_kind = ? AND owner_id = ?", ownerKind, ownerID).
		First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, sandboxdomain.ErrEnvNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sandboxstore.FindEnvByOwner: %w", err)
	}
	return &e, nil
}

// ListEnvsByRuntime returns all envs referencing the given runtime (Runtime GC dependency check).
//
// ListEnvsByRuntime 返所有引用此 runtime 的 env（Runtime GC 依赖检查）。
func (s *Store) ListEnvsByRuntime(ctx context.Context, runtimeID string) ([]*sandboxdomain.Env, error) {
	var rows []*sandboxdomain.Env
	if err := s.db.WithContext(ctx).
		Where("runtime_id = ?", runtimeID).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("sandboxstore.ListEnvsByRuntime: %w", err)
	}
	return rows, nil
}

// ListEnvsByOwnerKind returns all envs owned by a given kind, last_used_at DESC.
//
// ListEnvsByOwnerKind 返某 kind 拥有的所有 env，按 last_used_at DESC 排序。
func (s *Store) ListEnvsByOwnerKind(ctx context.Context, ownerKind string) ([]*sandboxdomain.Env, error) {
	var rows []*sandboxdomain.Env
	if err := s.db.WithContext(ctx).
		Where("owner_kind = ?", ownerKind).
		Order("last_used_at DESC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("sandboxstore.ListEnvsByOwnerKind: %w", err)
	}
	return rows, nil
}

// UpdateEnv persists changes to an existing Env row by primary key.
//
// UpdateEnv 按主键持久化 Env 修改。
func (s *Store) UpdateEnv(ctx context.Context, e *sandboxdomain.Env) error {
	if err := s.db.WithContext(ctx).Save(e).Error; err != nil {
		return fmt.Errorf("sandboxstore.UpdateEnv: %w", err)
	}
	return nil
}

// DeleteEnv hard-deletes an Env row by id.
//
// DeleteEnv 按 id 硬删 Env 行。
func (s *Store) DeleteEnv(ctx context.Context, id string) error {
	if err := s.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&sandboxdomain.Env{}).Error; err != nil {
		return fmt.Errorf("sandboxstore.DeleteEnv: %w", err)
	}
	return nil
}

// TotalSizeBytes returns the sum of size_bytes across runtimes + envs.
//
// TotalSizeBytes 返 runtimes + envs 两表 size_bytes 之和。
func (s *Store) TotalSizeBytes(ctx context.Context) (int64, error) {
	var rtTotal, envTotal int64
	if err := s.db.WithContext(ctx).
		Model(&sandboxdomain.Runtime{}).
		Select("COALESCE(SUM(size_bytes), 0)").
		Scan(&rtTotal).Error; err != nil {
		return 0, fmt.Errorf("sandboxstore.TotalSizeBytes: runtimes: %w", err)
	}
	if err := s.db.WithContext(ctx).
		Model(&sandboxdomain.Env{}).
		Select("COALESCE(SUM(size_bytes), 0)").
		Scan(&envTotal).Error; err != nil {
		return 0, fmt.Errorf("sandboxstore.TotalSizeBytes: envs: %w", err)
	}
	return rtTotal + envTotal, nil
}

// ListEnvsLastUsedBefore returns envs with last_used_at < t, ordered ASC (oldest first).
//
// ListEnvsLastUsedBefore 返 last_used_at 早于 t 的 env（最旧的先）。
func (s *Store) ListEnvsLastUsedBefore(ctx context.Context, t time.Time) ([]*sandboxdomain.Env, error) {
	var rows []*sandboxdomain.Env
	if err := s.db.WithContext(ctx).
		Where("last_used_at < ?", t).
		Order("last_used_at ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("sandboxstore.ListEnvsLastUsedBefore: %w", err)
	}
	return rows, nil
}

// SetEnvRunningPID records a long-lived process PID for boot-time leak scan.
//
// SetEnvRunningPID 记录长生命周期进程 PID，给启动扫描留 manifest 痕迹。
func (s *Store) SetEnvRunningPID(ctx context.Context, envID string, pid int) error {
	if err := s.db.WithContext(ctx).
		Model(&sandboxdomain.Env{}).
		Where("id = ?", envID).
		Update("running_pid", pid).Error; err != nil {
		return fmt.Errorf("sandboxstore.SetEnvRunningPID %s: %w", envID, err)
	}
	return nil
}

// ClearEnvRunningPID resets running_pid to 0 on graceful process exit.
//
// ClearEnvRunningPID 进程优雅退出时把 running_pid 重置为 0。
func (s *Store) ClearEnvRunningPID(ctx context.Context, envID string) error {
	if err := s.db.WithContext(ctx).
		Model(&sandboxdomain.Env{}).
		Where("id = ?", envID).
		Update("running_pid", 0).Error; err != nil {
		return fmt.Errorf("sandboxstore.ClearEnvRunningPID %s: %w", envID, err)
	}
	return nil
}

// ListEnvsWithRunningPID returns envs with running_pid > 0 (boot-time leak scan source).
//
// ListEnvsWithRunningPID 返 running_pid > 0 的 env（启动扫描遍历目标）。
func (s *Store) ListEnvsWithRunningPID(ctx context.Context) ([]*sandboxdomain.Env, error) {
	var rows []*sandboxdomain.Env
	if err := s.db.WithContext(ctx).
		Where("running_pid > ?", 0).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("sandboxstore.ListEnvsWithRunningPID: %w", err)
	}
	return rows, nil
}

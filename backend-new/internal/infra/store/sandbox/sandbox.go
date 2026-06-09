// Package sandbox is the orm-backed implementation of sandboxdomain.Repository.
// The two manifest tables (sandbox_runtimes + sandbox_envs) are system-level: a
// runtime is a machine-global interpreter/image and an env is isolated through
// its globally-unique owner id, so neither carries workspace_id and the orm
// applies no workspace filter (meta.ws == nil). Rows are hard-deleted (no
// deleted_at) — an evicted env/runtime keeps no tombstone.
//
// Package sandbox 是 sandboxdomain.Repository 的 orm 实现。两张 manifest 表
// （sandbox_runtimes + sandbox_envs）系统级：runtime 是全机解释器/镜像，env 通过全局唯一
// 的 owner id 隔离，故都无 workspace_id、orm 不施加 workspace 过滤（meta.ws == nil）。
// 行物理删除（无 deleted_at）——淘汰的 env/runtime 不留墓碑。
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
)

// Schema is the manifest DDL, exported as ordered idempotent statements for
// cmd/server to collect and apply via db.Migrate. CHECK constraints pin
// owner_kind / status to the domain enums (physical guard against a code bug
// writing a junk value).
//
// Schema 是 manifest 表 DDL，按序幂等语句导出，由 cmd/server 汇总经 db.Migrate 应用。
// CHECK 把 owner_kind / status 钉死到 domain 枚举（物理兜底，防代码 bug 写脏值）。
var Schema = []string{
	`CREATE TABLE IF NOT EXISTS sandbox_runtimes (
		id           TEXT PRIMARY KEY,
		kind         TEXT NOT NULL,
		version      TEXT NOT NULL,
		path         TEXT NOT NULL,
		size_bytes   INTEGER NOT NULL DEFAULT 0,
		installed_at DATETIME NOT NULL,
		updated_at   DATETIME NOT NULL
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_sandbox_runtimes_kind_version ON sandbox_runtimes(kind, version)`,

	`CREATE TABLE IF NOT EXISTS sandbox_envs (
		id           TEXT PRIMARY KEY,
		owner_kind   TEXT NOT NULL CHECK(owner_kind IN ('function','handler','mcp','skill','conversation','attachment')),
		owner_id     TEXT NOT NULL,
		owner_name   TEXT NOT NULL DEFAULT '',
		runtime_id   TEXT NOT NULL,
		deps         TEXT NOT NULL DEFAULT '[]',
		path         TEXT NOT NULL,
		size_bytes   INTEGER NOT NULL DEFAULT 0,
		status       TEXT NOT NULL DEFAULT 'ready' CHECK(status IN ('installing','ready','failed')),
		error_msg    TEXT NOT NULL DEFAULT '',
		created_at   DATETIME NOT NULL,
		last_used_at DATETIME NOT NULL,
		updated_at   DATETIME NOT NULL,
		running_pid  INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_sandbox_envs_owner ON sandbox_envs(owner_kind, owner_id)`,
	`CREATE INDEX IF NOT EXISTS idx_sandbox_envs_runtime ON sandbox_envs(runtime_id)`,
	`CREATE INDEX IF NOT EXISTS idx_sandbox_envs_running_pid ON sandbox_envs(running_pid)`,
}

// Store implements sandboxdomain.Repository over pkg/orm with one Repo per table.
//
// Store 基于 pkg/orm 实现 sandboxdomain.Repository，每张表一个 Repo。
type Store struct {
	runtimes *ormpkg.Repo[sandboxdomain.Runtime]
	envs     *ormpkg.Repo[sandboxdomain.Env]
}

// New builds a Store bound to the two manifest tables.
//
// New 构造绑定两张 manifest 表的 Store。
func New(db *ormpkg.DB) *Store {
	return &Store{
		runtimes: ormpkg.For[sandboxdomain.Runtime](db, "sandbox_runtimes"),
		envs:     ormpkg.For[sandboxdomain.Env](db, "sandbox_envs"),
	}
}

var _ sandboxdomain.Repository = (*Store)(nil)

// ---- runtimes ----

func (s *Store) CreateRuntime(ctx context.Context, r *sandboxdomain.Runtime) error {
	if err := s.runtimes.Create(ctx, r); err != nil {
		return fmt.Errorf("sandboxstore.CreateRuntime: %w", err)
	}
	return nil
}

func (s *Store) GetRuntime(ctx context.Context, id string) (*sandboxdomain.Runtime, error) {
	r, err := s.runtimes.Get(ctx, id)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, sandboxdomain.ErrRuntimeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sandboxstore.GetRuntime: %w", err)
	}
	return r, nil
}

func (s *Store) FindRuntime(ctx context.Context, kind, version string) (*sandboxdomain.Runtime, error) {
	r, err := s.runtimes.WhereEq("kind", kind).WhereEq("version", version).First(ctx)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, sandboxdomain.ErrRuntimeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sandboxstore.FindRuntime: %w", err)
	}
	return r, nil
}

func (s *Store) ListRuntimes(ctx context.Context) ([]*sandboxdomain.Runtime, error) {
	rows, err := s.runtimes.Order("kind, version").Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("sandboxstore.ListRuntimes: %w", err)
	}
	return rows, nil
}

// DeleteRuntime hard-deletes by id; the app layer has already confirmed no env
// still references it.
//
// DeleteRuntime 按 id 物理删；app 层已确认无 env 引用。
func (s *Store) DeleteRuntime(ctx context.Context, id string) error {
	if _, err := s.runtimes.Delete(ctx, id); err != nil {
		return fmt.Errorf("sandboxstore.DeleteRuntime: %w", err)
	}
	return nil
}

// ---- envs ----

func (s *Store) CreateEnv(ctx context.Context, e *sandboxdomain.Env) error {
	if err := s.envs.Create(ctx, e); err != nil {
		return fmt.Errorf("sandboxstore.CreateEnv: %w", err)
	}
	return nil
}

func (s *Store) GetEnv(ctx context.Context, id string) (*sandboxdomain.Env, error) {
	e, err := s.envs.Get(ctx, id)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, sandboxdomain.ErrEnvNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sandboxstore.GetEnv: %w", err)
	}
	return e, nil
}

func (s *Store) FindEnvByOwner(ctx context.Context, ownerKind, ownerID string) (*sandboxdomain.Env, error) {
	e, err := s.envs.WhereEq("owner_kind", ownerKind).WhereEq("owner_id", ownerID).First(ctx)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, sandboxdomain.ErrEnvNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sandboxstore.FindEnvByOwner: %w", err)
	}
	return e, nil
}

func (s *Store) ListEnvsByRuntime(ctx context.Context, runtimeID string) ([]*sandboxdomain.Env, error) {
	rows, err := s.envs.WhereEq("runtime_id", runtimeID).Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("sandboxstore.ListEnvsByRuntime: %w", err)
	}
	return rows, nil
}

func (s *Store) ListEnvsByOwnerKind(ctx context.Context, ownerKind string) ([]*sandboxdomain.Env, error) {
	rows, err := s.envs.WhereEq("owner_kind", ownerKind).Order("last_used_at DESC").Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("sandboxstore.ListEnvsByOwnerKind: %w", err)
	}
	return rows, nil
}

func (s *Store) UpdateEnv(ctx context.Context, e *sandboxdomain.Env) error {
	if err := s.envs.Save(ctx, e); err != nil {
		return fmt.Errorf("sandboxstore.UpdateEnv: %w", err)
	}
	return nil
}

func (s *Store) DeleteEnv(ctx context.Context, id string) error {
	if _, err := s.envs.Delete(ctx, id); err != nil {
		return fmt.Errorf("sandboxstore.DeleteEnv: %w", err)
	}
	return nil
}

// TotalSizeBytes sums size_bytes across both tables. Runtimes are a handful and
// envs a few dozen on a single machine, so summing in Go beats reaching past the
// orm for a SQL aggregate.
//
// TotalSizeBytes 汇总两表 size_bytes。单机 runtime 寥寥、env 几十，Go 层求和优于为 SQL
// 聚合绕过 orm。
func (s *Store) TotalSizeBytes(ctx context.Context) (int64, error) {
	rts, err := s.runtimes.Query().Find(ctx)
	if err != nil {
		return 0, fmt.Errorf("sandboxstore.TotalSizeBytes: runtimes: %w", err)
	}
	envs, err := s.envs.Query().Find(ctx)
	if err != nil {
		return 0, fmt.Errorf("sandboxstore.TotalSizeBytes: envs: %w", err)
	}
	var total int64
	for _, r := range rts {
		total += r.SizeBytes
	}
	for _, e := range envs {
		total += e.SizeBytes
	}
	return total, nil
}

func (s *Store) ListEnvsLastUsedBefore(ctx context.Context, t time.Time) ([]*sandboxdomain.Env, error) {
	rows, err := s.envs.Where("last_used_at < ?", t).Order("last_used_at ASC").Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("sandboxstore.ListEnvsLastUsedBefore: %w", err)
	}
	return rows, nil
}

// ---- running-pid leak tracking ----

func (s *Store) SetEnvRunningPID(ctx context.Context, envID string, pid int) error {
	if _, err := s.envs.WhereEq("id", envID).Updates(ctx, map[string]any{"running_pid": pid}); err != nil {
		return fmt.Errorf("sandboxstore.SetEnvRunningPID %s: %w", envID, err)
	}
	return nil
}

func (s *Store) ClearEnvRunningPID(ctx context.Context, envID string) error {
	if _, err := s.envs.WhereEq("id", envID).Updates(ctx, map[string]any{"running_pid": 0}); err != nil {
		return fmt.Errorf("sandboxstore.ClearEnvRunningPID %s: %w", envID, err)
	}
	return nil
}

func (s *Store) ListEnvsWithRunningPID(ctx context.Context) ([]*sandboxdomain.Env, error) {
	rows, err := s.envs.Where("running_pid > ?", 0).Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("sandboxstore.ListEnvsWithRunningPID: %w", err)
	}
	return rows, nil
}

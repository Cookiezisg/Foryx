// Package sandbox is the domain layer for PluginSandbox v2 runtime/env manifest.
//
// Package sandbox 是 PluginSandbox v2 的 domain 层（runtime/env 清单）。
package sandbox

import (
	"context"
	"errors"
	"io"
	"time"
)

const (
	OwnerKindFunction     = "function"
	OwnerKindHandler      = "handler"
	OwnerKindMCP          = "mcp"
	OwnerKindSkill        = "skill"
	OwnerKindConversation = "conversation"
)

// Owner identifies an Env's owner; ID semantics depend on Kind (see sandbox.md §3).
//
// Owner 标识 Env 所有者；ID 语义依 Kind，conversation 用 `_` 不用 `:`（PATH 分隔符）。
type Owner struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// Runtime is one installed (kind, version) on disk; UNIQUE(kind, version).
//
// Runtime 是磁盘上一份已装的 (kind, version)；UNIQUE(kind, version)，all-or-nothing 插入。
type Runtime struct {
	ID          string    `gorm:"primaryKey;type:text"                                            json:"id"`
	Kind        string    `gorm:"not null;type:text;uniqueIndex:uniq_sr_kind_version,priority:1"   json:"kind"`
	Version     string    `gorm:"not null;type:text;uniqueIndex:uniq_sr_kind_version,priority:2"   json:"version"`
	Path        string    `gorm:"not null;type:text"                                               json:"path"`
	SizeBytes   int64     `json:"sizeBytes"`
	InstalledAt time.Time `json:"installedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (Runtime) TableName() string { return "sandbox_runtimes" }

const (
	EnvStatusInstalling = "installing"
	EnvStatusReady      = "ready"
	EnvStatusFailed     = "failed"
)

// Env is one per-owner package-isolation directory bound to a Runtime; UNIQUE(owner_kind, owner_id).
//
// Env 是 per-owner 包隔离目录，绑定一个 Runtime；UNIQUE(owner_kind, owner_id)。
type Env struct {
	ID         string    `gorm:"primaryKey;type:text"                                      json:"id"`
	OwnerKind  string    `gorm:"not null;type:text;uniqueIndex:uniq_se_owner,priority:1;index:idx_se_owner,priority:1;check:owner_kind IN ('function','handler','mcp','skill','conversation')" json:"ownerKind"`
	OwnerID    string    `gorm:"not null;type:text;uniqueIndex:uniq_se_owner,priority:2;index:idx_se_owner,priority:2" json:"ownerId"`
	OwnerName  string    `gorm:"type:text"                                                 json:"ownerName,omitempty"`
	RuntimeID  string    `gorm:"not null;type:text;index"                                  json:"runtimeId"`
	Deps       []string  `gorm:"serializer:json"                                           json:"deps"`
	Path       string    `gorm:"not null;type:text"                                        json:"path"`
	SizeBytes  int64     `json:"sizeBytes"`
	Status     string    `gorm:"not null;type:text;default:ready;check:status IN ('installing','ready','failed')" json:"status"`
	ErrorMsg   string    `gorm:"type:text"                                                 json:"errorMsg,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsedAt time.Time `gorm:"index"                                                     json:"lastUsedAt"`
	UpdatedAt  time.Time `json:"updatedAt"`

	// RunningPID > 0 means a long-lived process from this env was alive at last manifest write.
	// RunningPID > 0 表上次 manifest 写时该 env 有长生命周期进程；Bootstrap 启动扫并杀残留。
	RunningPID int `gorm:"column:running_pid;default:0;index" json:"runningPid,omitempty"`
}

func (Env) TableName() string { return "sandbox_envs" }

// RuntimeSpec describes a runtime requirement; empty Version resolves to default.
//
// RuntimeSpec 描述 runtime 需求；Version 空时解析默认版本。
type RuntimeSpec struct {
	Kind    string `json:"kind"`
	Version string `json:"version,omitempty"`
}

// EnvSpec describes an owner's env; Deps follow runtime native pkg manager (pip/npm/cargo).
//
// EnvSpec 描述 owner 的 env；Deps 按 runtime 原生包管理器格式。
type EnvSpec struct {
	Runtime RuntimeSpec `json:"runtime"`
	Deps    []string    `json:"deps,omitempty"`
}

// SpawnOpts is one spawn order; LongLived flips return between ExecutionResult and LongLivedHandle.
//
// SpawnOpts 是一份 spawn 指令；LongLived 切换返回类型。
type SpawnOpts struct {
	Cmd       string            `json:"cmd"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Stdin     []byte            `json:"-"`
	Timeout   time.Duration     `json:"timeoutMs,omitempty"`
	LongLived bool              `json:"longLived,omitempty"`
}

type ExecutionResult struct {
	Ok       bool          `json:"ok"`
	Stdout   []byte        `json:"-"`
	Stderr   []byte        `json:"-"`
	ExitCode int           `json:"exitCode"`
	Duration time.Duration `json:"durationMs"`
}

// LongLivedHandle: caller owns lifecycle; must Wait or Kill to release.
//
// LongLivedHandle：调用方拥有生命周期，须 Wait 或 Kill 释放。
type LongLivedHandle interface {
	Stdin() io.WriteCloser
	Stdout() io.ReadCloser
	Stderr() io.ReadCloser
	Wait() error
	Kill() error
	PID() int
}

// ProgressFunc reports install/sync progress; percent 0-100 (-1 = unknown).
//
// ProgressFunc 报装机/同步进度；percent 0-100，-1 表未知。
type ProgressFunc func(stage, message string, percent int)

var (
	ErrRuntimeNotSupported  = errors.New("sandbox: runtime kind not registered")
	ErrRuntimeInstallFailed = errors.New("sandbox: runtime install failed")
	ErrEnvNotFound          = errors.New("sandbox: env not found")
	ErrEnvCreateFailed      = errors.New("sandbox: env create failed")
	ErrDepInstallFailed     = errors.New("sandbox: dependency install failed")
	ErrSpawnFailed          = errors.New("sandbox: spawn process failed")
	ErrSpawnTimeout         = errors.New("sandbox: spawn process timeout")
	ErrEnvInUse             = errors.New("sandbox: env in use; cannot destroy")

	// ErrInvalidOwnerID is returned when owner.ID contains PATH-meta / shell meta / whitespace / NUL.
	// ErrInvalidOwnerID 在 owner.ID 含 PATH-meta / shell-meta / 空白 / NUL 时返回。
	ErrInvalidOwnerID = errors.New("sandbox: owner.ID contains PATH-meta or whitespace character")

	ErrCmdRequired = errors.New("sandbox: SpawnOpts.Cmd is required")

	// ErrDockerNotInstalled: docker binary missing; Forgify cannot install it (root/admin required).
	// ErrDockerNotInstalled：docker 不在 PATH；Forgify 不能代装（需 root/admin）。
	ErrDockerNotInstalled = errors.New("sandbox: docker not installed")

	// ErrDockerDaemonDown: docker on PATH but daemon not responding.
	// ErrDockerDaemonDown：docker 在 PATH 但 daemon 不响应。
	ErrDockerDaemonDown = errors.New("sandbox: docker daemon not responding")
)

// Repository is the persistence contract for sandbox manifest tables.
//
// Repository 是 sandbox manifest 表（sandbox_runtimes + sandbox_envs）的持久化契约。
type Repository interface {
	CreateRuntime(ctx context.Context, r *Runtime) error
	GetRuntime(ctx context.Context, id string) (*Runtime, error)
	FindRuntime(ctx context.Context, kind, version string) (*Runtime, error)
	ListRuntimes(ctx context.Context) ([]*Runtime, error)
	UpdateRuntime(ctx context.Context, r *Runtime) error
	DeleteRuntime(ctx context.Context, id string) error

	CreateEnv(ctx context.Context, e *Env) error
	GetEnv(ctx context.Context, id string) (*Env, error)
	FindEnvByOwner(ctx context.Context, ownerKind, ownerID string) (*Env, error)
	ListEnvsByRuntime(ctx context.Context, runtimeID string) ([]*Env, error)
	ListEnvsByOwnerKind(ctx context.Context, ownerKind string) ([]*Env, error)
	UpdateEnv(ctx context.Context, e *Env) error
	DeleteEnv(ctx context.Context, id string) error

	TotalSizeBytes(ctx context.Context) (int64, error)
	ListEnvsLastUsedBefore(ctx context.Context, t time.Time) ([]*Env, error)

	SetEnvRunningPID(ctx context.Context, envID string, pid int) error
	ClearEnvRunningPID(ctx context.Context, envID string) error
	ListEnvsWithRunningPID(ctx context.Context) ([]*Env, error)
}

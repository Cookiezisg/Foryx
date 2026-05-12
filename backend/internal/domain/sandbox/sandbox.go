// Package sandbox is the domain layer for PluginSandbox v2 — the runtime /
// env manifest shared by forge, mcp, skill, and conversation scratch envs.
// See documents/version-1.2/service-design-documents/sandbox.md for design.
//
// Package sandbox 是 PluginSandbox v2 的 domain 层——forge / mcp / skill /
// conversation scratch env 共用的 runtime / env 清单。
// 设计见 documents/version-1.2/service-design-documents/sandbox.md。
package sandbox

import (
	"context"
	"errors"
	"io"
	"time"
)

// OwnerKind enumerates env owner types. Stable strings — used as DB / JSON
// values; renames require migration. OwnerKindSkill is reserved for the
// skill domain when its sandbox integration ships (no current producer).
//
// OwnerKind 枚举 env 所有者类型。稳定字符串——直接作 DB / JSON 值；改名需迁移。
// OwnerKindSkill 预留给 skill domain 接 sandbox 时用（暂无生产 producer）。
const (
	OwnerKindFunction     = "function"
	OwnerKindHandler      = "handler"
	OwnerKindMCP          = "mcp"
	OwnerKindSkill        = "skill"
	OwnerKindConversation = "conversation"
)

// Owner identifies an Env's owner. ID semantics depend on Kind:
//   - function:     "<functionID>_<envID>" (per-function envID buffer for revert)
//   - handler:      "<handlerID>_<envID>" (same shape as function)
//   - mcp:          server name (e.g. "playwright")
//   - skill:        skill name
//   - conversation: "<conv_id>_<runtime_kind>" (use `_`, not `:` —
//     POSIX PATH separator would break runtime PATH resolution; the
//     EnsureEnv validator rejects any owner.ID containing `:`)
// Name is UI-only, not part of identity.
//
// Owner 标识 Env 所有者。ID 语义依 Kind（见上；conversation 用 `_` 不用 `:`，
// `:` 是 POSIX PATH 分隔符会破坏运行期 PATH 解析；EnsureEnv 校验器拒绝
// owner.ID 含 `:`）。Name 仅 UI 用，不参与身份。
type Owner struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// Runtime is one installed (kind, version) on disk; UNIQUE(kind, version).
// Default version selection is owned by RuntimeInstaller.ResolveDefault
// (a constant baked into MiseInstaller at construction); no DB column /
// query backs it.
// Insertion is all-or-nothing: EnsureRuntime only inserts after Install
// succeeds — failed installs leave no row.
//
// Runtime 是磁盘上一份已装的 (kind, version)；UNIQUE(kind, version)。
// 默认版本选择由 RuntimeInstaller.ResolveDefault 拥有（构造时固化的常量）；
// 无 DB 列 / query 支持。
// all-or-nothing 插入：EnsureRuntime 仅在 Install 成功后插行——失败不留行。
type Runtime struct {
	ID          string    `gorm:"primaryKey;type:text"                                            json:"id"` // sr_<16hex>
	Kind        string    `gorm:"not null;type:text;uniqueIndex:uniq_sr_kind_version,priority:1"   json:"kind"`
	Version     string    `gorm:"not null;type:text;uniqueIndex:uniq_sr_kind_version,priority:2"   json:"version"`
	Path        string    `gorm:"not null;type:text"                                               json:"path"`
	SizeBytes   int64     `json:"sizeBytes"`
	InstalledAt time.Time `json:"installedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (Runtime) TableName() string { return "sandbox_runtimes" }

// EnvStatus enumerates Env install lifecycle. DB CHECK enforces the whitelist;
// renames require migration.
//
// EnvStatus 枚举 Env 装机生命周期。DB CHECK 强制白名单；改名需迁移。
const (
	EnvStatusInstalling = "installing"
	EnvStatusReady      = "ready"
	EnvStatusFailed     = "failed"
)

// Env is one per-owner package-isolation directory bound to a Runtime.
// UNIQUE(owner_kind, owner_id). Path is relative to sandbox envs root.
// ErrorMsg holds failure text when Status="failed".
//
// Env 是 per-owner 包隔离目录，绑定一个 Runtime。UNIQUE(owner_kind, owner_id)。
// Path 相对 sandbox envs 根目录。Status="failed" 时 ErrorMsg 存失败文本。
type Env struct {
	ID         string    `gorm:"primaryKey;type:text"                                      json:"id"` // se_<16hex>
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

	// RunningPID > 0 = a long-lived process from this env was alive at last
	// manifest write. Service.Bootstrap scans these on boot and kills survivors
	// (Layer B leak prevention; covers app crashes bypassing graceful Shutdown).
	// Explicit column:running_pid — GORM default would produce "running_p_id"
	// (doesn't recognise PID as an acronym).
	//
	// RunningPID > 0 = 上次 manifest 写时该 env 有长生命周期进程活着。
	// Service.Bootstrap 启动扫 + 杀残留（层 B leak 防御；防 app crash 绕过优雅 Shutdown）。
	// 显式 column:running_pid——GORM 默认会转成 "running_p_id"（不认 PID 缩写）。
	RunningPID int `gorm:"column:running_pid;default:0;index" json:"runningPid,omitempty"`
}

func (Env) TableName() string { return "sandbox_envs" }

// RuntimeSpec describes a runtime requirement. Empty Version = kind's default
// (resolved via RuntimeInstaller.ResolveDefault).
//
// RuntimeSpec 描述 runtime 需求。Version 空 = 该 kind 默认版本
// （RuntimeInstaller.ResolveDefault 解析）。
type RuntimeSpec struct {
	Kind    string `json:"kind"`
	Version string `json:"version,omitempty"`
}

// EnvSpec describes an owner's env. Deps follow the runtime's native package
// manager (pip / npm / cargo).
//
// EnvSpec 描述 owner 的 env。Deps 按 runtime 原生包管理器（pip / npm / cargo）。
type EnvSpec struct {
	Runtime RuntimeSpec `json:"runtime"`
	Deps    []string    `json:"deps,omitempty"`
}

// SpawnOpts is one spawn order. LongLived=false → ExecutionResult (one-shot);
// LongLived=true → LongLivedHandle (caller drives stdin/stdout/wait).
//
// SpawnOpts 是一份 spawn 指令。LongLived=false → ExecutionResult（一次性）；
// LongLived=true → LongLivedHandle（调用方驱 stdin/stdout/wait）。
type SpawnOpts struct {
	Cmd       string            `json:"cmd"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Stdin     []byte            `json:"-"`
	Timeout   time.Duration     `json:"timeoutMs,omitempty"`
	LongLived bool              `json:"longLived,omitempty"`
}

// ExecutionResult is the one-shot Spawn return.
// ExecutionResult 是一次性 Spawn 的返回。
type ExecutionResult struct {
	Ok       bool          `json:"ok"`
	Stdout   []byte        `json:"-"`
	Stderr   []byte        `json:"-"`
	ExitCode int           `json:"exitCode"`
	Duration time.Duration `json:"durationMs"`
}

// LongLivedHandle: caller owns the lifecycle, must Wait or Kill to release.
//
// LongLivedHandle：调用方拥有生命周期，必须 Wait 或 Kill 释放。
type LongLivedHandle interface {
	Stdin() io.WriteCloser
	Stdout() io.ReadCloser
	Stderr() io.ReadCloser
	Wait() error
	Kill() error
	PID() int
}

// ProgressFunc reports install / sync progress. percent 0-100 (or -1 unknown).
//
// ProgressFunc 报装机/同步进度。percent 0-100（未知 -1）。
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
	// ErrInvalidOwnerID is returned when a caller-supplied owner.ID
	// contains characters that would break PATH-prepend resolution
	// (POSIX/Windows path separators, shell metacharacters, whitespace,
	// NUL). Defense-in-depth against bash auto-route regression — see
	// the B1 fix in commit 3cdf18a + sandbox.md §3.
	//
	// ErrInvalidOwnerID 在调用方传的 owner.ID 含会破坏 PATH 前置的字符
	// （POSIX/Windows 分隔符、shell 元字符、空白、NUL）时返回。防 bash
	// auto-route 回归（commit 3cdf18a 修过的 B1）+ sandbox.md §3。
	ErrInvalidOwnerID = errors.New("sandbox: owner.ID contains PATH-meta or whitespace character")
	// ErrCmdRequired is returned when SpawnOpts.Cmd is empty. Spawn
	// callers should always provide a concrete Cmd; explicit sentinel
	// instead of panic preserves the existing test contract
	// (TestServiceSpawn_EmptyCmd_Errors).
	//
	// ErrCmdRequired 在 SpawnOpts.Cmd 为空时返回。Spawn 调用方应当总是给
	// 具体 Cmd；用 sentinel 而非 panic 保留既有测试契约。
	ErrCmdRequired = errors.New("sandbox: SpawnOpts.Cmd is required")
	// ErrDockerNotInstalled = `docker` binary not on PATH. Forgify cannot
	// install Docker for the user (system service, requires root/admin) —
	// caller should surface a platform-specific install URL.
	//
	// ErrDockerNotInstalled = `docker` 二进制不在 PATH。Forgify 不能替用户装
	// Docker（系统服务，要 root/admin）—— 调用方应给平台对应安装链接。
	ErrDockerNotInstalled = errors.New("sandbox: docker not installed")
	// ErrDockerDaemonDown = `docker` is on PATH but the daemon is not
	// responding. Typically Docker Desktop not running on Mac/Win, or
	// `systemctl status docker` says inactive on Linux.
	//
	// ErrDockerDaemonDown = `docker` 在 PATH 但 daemon 不响应。通常 Mac/Win
	// 上 Docker Desktop 没启，或 Linux 上 `systemctl status docker` inactive。
	ErrDockerDaemonDown = errors.New("sandbox: docker daemon not responding")
)

// Repository is the persistence contract for sandbox manifest tables
// (sandbox_runtimes + sandbox_envs). Implemented by infra/store/sandbox.
//
// Repository 是 sandbox manifest 表（sandbox_runtimes + sandbox_envs）的
// 持久化契约。由 infra/store/sandbox 实现。
type Repository interface {
	// Runtime CRUD
	CreateRuntime(ctx context.Context, r *Runtime) error
	GetRuntime(ctx context.Context, id string) (*Runtime, error)
	FindRuntime(ctx context.Context, kind, version string) (*Runtime, error)
	ListRuntimes(ctx context.Context) ([]*Runtime, error)
	UpdateRuntime(ctx context.Context, r *Runtime) error
	DeleteRuntime(ctx context.Context, id string) error

	// Env CRUD
	CreateEnv(ctx context.Context, e *Env) error
	GetEnv(ctx context.Context, id string) (*Env, error)
	FindEnvByOwner(ctx context.Context, ownerKind, ownerID string) (*Env, error)
	ListEnvsByRuntime(ctx context.Context, runtimeID string) ([]*Env, error)
	ListEnvsByOwnerKind(ctx context.Context, ownerKind string) ([]*Env, error)
	UpdateEnv(ctx context.Context, e *Env) error
	DeleteEnv(ctx context.Context, id string) error

	// Aggregate — UI disk usage + GC candidate selection.
	TotalSizeBytes(ctx context.Context) (int64, error)
	ListEnvsLastUsedBefore(ctx context.Context, t time.Time) ([]*Env, error)

	// Layer B leak prevention — track + scan running PIDs.
	SetEnvRunningPID(ctx context.Context, envID string, pid int) error
	ClearEnvRunningPID(ctx context.Context, envID string) error
	ListEnvsWithRunningPID(ctx context.Context) ([]*Env, error)
}

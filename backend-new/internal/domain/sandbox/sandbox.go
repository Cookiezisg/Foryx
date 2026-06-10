// Package sandbox is the domain layer for the isolated plugin runtime: it models
// the on-disk Runtime/Env manifest plus the install/spawn contracts that let user
// code (function/handler/mcp/skill/conversation) run in a per-owner isolated
// Python/Node environment or a Docker container.
//
// Package sandbox 是隔离插件运行时的 domain 层：建模磁盘上的 Runtime/Env 清单，
// 以及让用户代码（function/handler/mcp/skill/conversation）在 per-owner 隔离的
// Python/Node 环境或 Docker 容器中运行的 install/spawn 契约。
package sandbox

import (
	"context"
	"io"
	"time"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
)

// Owner kinds — the entity types that can own an isolated env. function/handler/mcp/skill/
// conversation own a per-entity env for user code; attachment owns a single shared, machine-global
// env (fixed owner id) holding the document-extraction toolchain (pdfplumber / python-docx …).
//
// Owner kind —— 可拥有隔离 env 的实体类型。function/handler/mcp/skill/conversation 各自一个
// per-entity env 跑用户代码；attachment 拥有一个共享、全机唯一的 env（固定 owner id），装文档抽取
// 工具链（pdfplumber / python-docx …）。
const (
	OwnerKindFunction     = "function"
	OwnerKindHandler      = "handler"
	OwnerKindMCP          = "mcp"
	OwnerKindSkill        = "skill"
	OwnerKindConversation = "conversation"
	OwnerKindAttachment   = "attachment"
)

// Owner identifies an Env's owner. ID becomes a directory name and joins PATH at
// spawn time, so it must be free of PATH/shell metacharacters (validated in app).
//
// Owner 标识 Env 所有者。ID 会成为目录名并在 spawn 时进入 PATH，故必须不含
// PATH/shell 元字符（在 app 层校验）。
type Owner struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// Runtime is one installed (kind, version) on disk — a shared interpreter
// (python/node) or a pulled Docker image. UNIQUE(kind, version). Shared across
// every workspace: no workspace_id, because an interpreter/image is a
// machine-global resource that would only be wastefully duplicated per workspace.
//
// Runtime 是磁盘上一份已装的 (kind, version)——共享解释器（python/node）或已拉取的
// Docker 镜像。UNIQUE(kind, version)。跨所有 workspace 共享：无 workspace_id，因为
// 解释器/镜像是全机资源，按 workspace 复制只会浪费。
type Runtime struct {
	ID          string    `db:"id,pk"               json:"id"`
	Kind        string    `db:"kind"                json:"kind"`
	Version     string    `db:"version"             json:"version"`
	Path        string    `db:"path"                json:"path"` // python/node: dir rel sandboxRoot; docker: image ref
	SizeBytes   int64     `db:"size_bytes"          json:"sizeBytes"`
	InstalledAt time.Time `db:"installed_at,created" json:"installedAt"`
	UpdatedAt   time.Time `db:"updated_at,updated"  json:"updatedAt"`
}

// Env status values.
const (
	EnvStatusInstalling = "installing"
	EnvStatusReady      = "ready"
	EnvStatusFailed     = "failed"
)

// Env is a per-owner package-isolation environment bound to a Runtime: a venv /
// node_modules dir for python/node, or a logical binding to a Docker image.
// UNIQUE(owner_kind, owner_id). It is isolated per workspace transitively through
// the owner id (fn_xxx / mcp owner ids are globally unique and workspace-owned),
// so it carries no workspace_id column of its own.
//
// Env 是绑定一个 Runtime 的 per-owner 包隔离环境：python/node 的 venv / node_modules
// 目录，或对 Docker 镜像的逻辑绑定。UNIQUE(owner_kind, owner_id)。通过 owner id 间接
// 按 workspace 隔离（fn_xxx / mcp owner id 全局唯一且归属某 workspace），故自身无
// workspace_id 列。
type Env struct {
	ID         string    `db:"id,pk"               json:"id"`
	OwnerKind  string    `db:"owner_kind"          json:"ownerKind"`
	OwnerID    string    `db:"owner_id"            json:"ownerId"`
	OwnerName  string    `db:"owner_name"          json:"ownerName,omitempty"`
	RuntimeID  string    `db:"runtime_id"          json:"runtimeId"`
	Deps       []string  `db:"deps,json"           json:"deps"`
	Path       string    `db:"path"                json:"path"`
	SizeBytes  int64     `db:"size_bytes"          json:"sizeBytes"`
	Status     string    `db:"status"              json:"status"`
	ErrorMsg   string    `db:"error_msg"           json:"errorMsg,omitempty"`
	CreatedAt  time.Time `db:"created_at,created"  json:"createdAt"`
	LastUsedAt time.Time `db:"last_used_at"        json:"lastUsedAt"`
	UpdatedAt  time.Time `db:"updated_at,updated"  json:"updatedAt"`

	// RunningPID > 0 means a long-lived process from this env was alive at the
	// last manifest write; the boot scan kills survivors from a prior crash.
	//
	// RunningPID > 0 表上次 manifest 写时该 env 有长生命周期进程；启动扫描杀掉上次
	// 崩溃残留。
	RunningPID int `db:"running_pid" json:"runningPid,omitempty"`
}

// RuntimeSpec describes a runtime requirement; empty Version resolves to the
// kind's default. For docker, Version is the image ref (e.g. ghcr.io/org/img:tag).
//
// RuntimeSpec 描述 runtime 需求；Version 空时解析该 kind 的默认版本。docker 的 Version
// 是镜像 ref（如 ghcr.io/org/img:tag）。
type RuntimeSpec struct {
	Kind    string `json:"kind"`
	Version string `json:"version,omitempty"`
}

// EnvSpec describes an owner's env; Deps follow the runtime's native package
// manager (pip/npm). Docker envs carry no deps — the image is self-contained.
//
// EnvSpec 描述 owner 的 env；Deps 按 runtime 原生包管理器格式（pip/npm）。docker env
// 无 deps——镜像自包含。
type EnvSpec struct {
	Runtime RuntimeSpec `json:"runtime"`
	Deps    []string    `json:"deps,omitempty"`
}

// SpawnOpts is one spawn order; LongLived flips the return between
// ExecutionResult and LongLivedHandle.
//
// SpawnOpts 是一份 spawn 指令；LongLived 切换返回类型。
type SpawnOpts struct {
	Cmd       string            `json:"cmd"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Stdin     []byte            `json:"-"`
	Timeout   time.Duration     `json:"timeoutMs,omitempty"`
	LongLived bool              `json:"longLived,omitempty"`
	// StreamErr (optional) tees the child's stderr to a live sink as it runs — the seam a tool
	// (e.g. run_function) uses to stream the function's print() output as progress. nil = capture only.
	//
	// StreamErr（可选）把子进程 stderr 实时 tee 到 sink——工具（如 run_function）据此把函数 print() 输出作为
	// 进度流出的接缝。nil = 仅捕获。
	StreamErr io.Writer `json:"-"`
}

// ExecutionResult is a finished one-shot spawn; Ok=false means a non-zero exit
// (an infra failure surfaces as a Go error instead).
//
// ExecutionResult 是一次性 spawn 的结果；Ok=false 表非零退出（基础设施失败改为 Go error）。
type ExecutionResult struct {
	Ok       bool          `json:"ok"`
	Stdout   []byte        `json:"-"`
	Stderr   []byte        `json:"-"`
	ExitCode int           `json:"exitCode"`
	Duration time.Duration `json:"durationMs"`
}

// LongLivedHandle hands lifecycle ownership to the caller, who must Wait or Kill.
//
// LongLivedHandle 把生命周期交给调用方，须 Wait 或 Kill 释放。
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

// Sentinel errors. Built with errorsdomain.New so transport reads Kind→HTTP +
// stable wire code directly (§S20). KindBadGateway (502) is the canonical class
// for "an upstream tool we shelled out to failed".
//
// Sentinel 错误。用 errorsdomain.New 构造，使 transport 直接读 Kind→HTTP + 稳定 wire
// code（§S20）。KindBadGateway (502) 是"外包给的上游工具失败"的标准类别。
var (
	ErrRuntimeNotSupported  = errorsdomain.New(errorsdomain.KindUnprocessable, "SANDBOX_RUNTIME_NOT_SUPPORTED", "runtime kind not registered")
	ErrRuntimeInstallFailed = errorsdomain.New(errorsdomain.KindBadGateway, "SANDBOX_RUNTIME_INSTALL_FAILED", "runtime install failed")
	// ErrRuntimeNotFound is an internal lookup miss (EnsureRuntime consumes it to
	// decide "not installed → install"); it does not normally reach HTTP.
	//
	// ErrRuntimeNotFound 是内部查找未命中（EnsureRuntime 据此判断"未装→去装"），
	// 通常不冒泡到 HTTP。
	ErrRuntimeNotFound    = errorsdomain.New(errorsdomain.KindNotFound, "SANDBOX_RUNTIME_NOT_FOUND", "runtime not found")
	ErrEnvNotFound        = errorsdomain.New(errorsdomain.KindNotFound, "SANDBOX_ENV_NOT_FOUND", "env not found")
	ErrEnvCreateFailed    = errorsdomain.New(errorsdomain.KindBadGateway, "SANDBOX_ENV_CREATE_FAILED", "env create failed")
	ErrDepInstallFailed   = errorsdomain.New(errorsdomain.KindBadGateway, "SANDBOX_DEP_INSTALL_FAILED", "dependency install failed")
	ErrSpawnFailed        = errorsdomain.New(errorsdomain.KindBadGateway, "SANDBOX_SPAWN_FAILED", "spawn process failed")
	ErrSpawnTimeout       = errorsdomain.New(errorsdomain.KindGatewayTimeout, "SANDBOX_SPAWN_TIMEOUT", "spawn process timeout")
	ErrEnvInUse           = errorsdomain.New(errorsdomain.KindConflict, "SANDBOX_ENV_IN_USE", "env in use; cannot destroy")
	ErrInvalidOwnerID     = errorsdomain.New(errorsdomain.KindInvalid, "SANDBOX_INVALID_OWNER_ID", "owner id contains PATH-meta or whitespace character")
	ErrCmdRequired        = errorsdomain.New(errorsdomain.KindInvalid, "SANDBOX_CMD_REQUIRED", "spawn cmd is required")
	ErrDockerNotInstalled = errorsdomain.New(errorsdomain.KindUnprocessable, "SANDBOX_DOCKER_NOT_INSTALLED", "docker not installed")
	ErrDockerDaemonDown   = errorsdomain.New(errorsdomain.KindUnavailable, "SANDBOX_DOCKER_DAEMON_DOWN", "docker daemon not responding")
)

// Repository is the persistence contract for the sandbox manifest tables
// (sandbox_runtimes + sandbox_envs). System-level: no workspace scope.
//
// Repository 是 sandbox manifest 表（sandbox_runtimes + sandbox_envs）的持久化契约。
// 系统级：无 workspace 作用域。
type Repository interface {
	CreateRuntime(ctx context.Context, r *Runtime) error
	GetRuntime(ctx context.Context, id string) (*Runtime, error)
	FindRuntime(ctx context.Context, kind, version string) (*Runtime, error)
	ListRuntimes(ctx context.Context) ([]*Runtime, error)
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

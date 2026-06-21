// Package handler is the domain layer for user-built stateful Python classes (the
// second Quadrinity element). Unlike a function (stateless, fresh process per call), a
// handler runs as ONE long-lived resident process per handler — like an MCP server:
// spawned at boot (or when first configured), kept alive across every call so self.xxx
// state persists, restarted on edit / config-change / crash, gracefully shut down only
// on app exit. All callers (chat / agent / workflow) share that single instance.
//
// The version model mirrors function: a linear append-only line of Versions with a
// free-moving ActiveVersionID pointer — no pending/accept state machine.
//
// Package handler 是用户构建的有状态 Python 类的 domain 层（Quadrinity 第二元）。不同于
// function（无状态、每调用全新进程），handler 以**每 handler 一个常驻长跑进程**运行——像 MCP
// server：开局（或首次配齐）spawn，跨每次调用保活使 self.xxx 状态留存，edit / 改 config / crash
// 时重启，仅退出软件优雅关闭。所有调用方（chat / agent / workflow）共享这单一实例。
//
// 版本模型镜像 function：线性只增的 Version 线 + 自由 active 指针，无 pending/accept。
package handler

import (
	"time"

	errorspkg "github.com/sunweilin/anselm/backend/internal/pkg/errors"
)

// Handler is the Definition entity; class code / methods / init-args schema / deps live
// on the active Version. ConfigEncrypted holds the init-args VALUES (encrypted at rest).
//
// Handler 是 Definition 实体；类代码 / methods / init-args schema / deps 在 active Version。
// ConfigEncrypted 存 init-args 的值（加密存盘）。
type Handler struct {
	ID              string     `db:"id,pk"               json:"id"`
	WorkspaceID     string     `db:"workspace_id,ws"     json:"-"`
	Name            string     `db:"name"                json:"name"`
	Description     string     `db:"description"         json:"description"`
	Tags            []string   `db:"tags,json"           json:"tags"`
	ActiveVersionID string     `db:"active_version_id"   json:"activeVersionId"`
	ConfigEncrypted string     `db:"config_encrypted"    json:"-"`
	CreatedAt       time.Time  `db:"created_at,created"  json:"createdAt"`
	UpdatedAt       time.Time  `db:"updated_at,updated"  json:"updatedAt"`
	DeletedAt       *time.Time `db:"deleted_at,deleted"  json:"-"`

	// Computed (db:"-") — attached by Service.Get.
	// 计算字段（db:"-"）——由 Service.Get 附上。
	ActiveVersion *Version `db:"-" json:"activeVersion,omitempty"`
	ConfigState   string   `db:"-" json:"configState,omitempty"`   // unconfigured / partially_configured / ready
	MissingConfig []string `db:"-" json:"missingConfig,omitempty"` // required init-args still unset
	RuntimeState  string   `db:"-" json:"runtimeState,omitempty"`  // running / stopped / crashed (resident instance)
}

// DefaultPythonVersion is used when a handler omits set_python_version.
//
// DefaultPythonVersion 在未设 set_python_version 时使用。
const DefaultPythonVersion = "3.12"

// Version is one immutable snapshot of a handler's class (parts) + interface + deps.
// Version is a monotonic counter assigned at write time (max+1).
//
// Version 是 handler 类（各部分）+ 接口 + 依赖的不可变快照。Version 是写入时分配的单调号（max+1）。
type Version struct {
	ID                    string        `db:"id,pk"                      json:"id"`
	WorkspaceID           string        `db:"workspace_id,ws"            json:"-"`
	HandlerID             string        `db:"handler_id"                 json:"handlerId"`
	Version               int           `db:"version"                    json:"version"`
	Imports               string        `db:"imports"                    json:"imports"`
	InitBody              string        `db:"init_body"                  json:"initBody"`
	ShutdownBody          string        `db:"shutdown_body"              json:"shutdownBody"`
	Methods               []MethodSpec  `db:"methods,json"               json:"methods"`
	InitArgsSchema        []InitArgSpec `db:"init_args_schema,json"      json:"initArgsSchema"`
	Dependencies          []string      `db:"dependencies,json"          json:"dependencies"`
	PythonVersion         string        `db:"python_version"             json:"pythonVersion"`
	EnvID                 string        `db:"env_id"                     json:"envId"`
	EnvStatus             string        `db:"env_status"                 json:"envStatus"`
	EnvError              string        `db:"env_error"                  json:"envError,omitempty"`
	EnvSyncedAt           *time.Time    `db:"env_synced_at"              json:"envSyncedAt,omitempty"`
	ChangeReason          string        `db:"change_reason"              json:"changeReason,omitempty"`
	BuiltInConversationID *string       `db:"built_in_conversation_id"  json:"builtInConversationId,omitempty"`
	CreatedAt             time.Time     `db:"created_at,created"         json:"createdAt"`
	UpdatedAt             time.Time     `db:"updated_at,updated"         json:"updatedAt"`
}

// Env status values (mirror sandbox env lifecycle, surfaced on the version row).
//
// Env 状态值（镜像 sandbox env 生命周期，挂在版本行上）。
const (
	EnvStatusPending = "pending"
	EnvStatusSyncing = "syncing"
	EnvStatusReady   = "ready"
	EnvStatusFailed  = "failed"
)

// ConfigState reflects how complete the encrypted init-args config is vs the schema.
//
// ConfigState 反映加密 init-args config 相对 schema 的完整度。
const (
	ConfigStateUnconfigured        = "unconfigured"
	ConfigStatePartiallyConfigured = "partially_configured"
	ConfigStateReady               = "ready"
)

// RuntimeState reflects the resident instance's liveness (computed from the manager).
//
// RuntimeState 反映常驻实例的存活（由 manager 计算）。
const (
	RuntimeStateStopped = "stopped" // no live instance (not spawned / config incomplete)
	RuntimeStateRunning = "running" // live and healthy
	RuntimeStateCrashed = "crashed" // process died; next call respawns
)

var (
	ErrNotFound      = errorspkg.New(errorspkg.KindNotFound, "HANDLER_NOT_FOUND", "handler not found")
	ErrDuplicateName = errorspkg.New(errorspkg.KindConflict, "HANDLER_NAME_DUPLICATE", "handler name already exists")
	// ErrInvalidCallStatus: a list filter passed a status outside CallStatuses — 422 with the allowed set
	// in Details so the caller self-corrects instead of silently getting an empty page (F168-M2).
	// ErrInvalidCallStatus：list 过滤传了 CallStatuses 外的状态——返 422、Details 带合法集自纠（F168-M2）。
	ErrInvalidCallStatus   = errorspkg.New(errorspkg.KindUnprocessable, "HANDLER_CALL_INVALID_STATUS", "handler call status filter must be one of: ok, failed, cancelled, timeout")
	ErrVersionNotFound     = errorspkg.New(errorspkg.KindNotFound, "HANDLER_VERSION_NOT_FOUND", "handler version not found")
	ErrVersionConflict     = errorspkg.New(errorspkg.KindConflict, "HANDLER_VERSION_CONFLICT", "handler version already exists (concurrent edit)")
	ErrCallNotFound        = errorspkg.New(errorspkg.KindNotFound, "HANDLER_CALL_NOT_FOUND", "handler call not found")
	ErrMethodNotFound      = errorspkg.New(errorspkg.KindNotFound, "HANDLER_METHOD_NOT_FOUND", "handler method not found")
	ErrNoActiveVersion     = errorspkg.New(errorspkg.KindUnprocessable, "HANDLER_NO_ACTIVE_VERSION", "handler has no active version")
	ErrEnvNotReady         = errorspkg.New(errorspkg.KindUnprocessable, "HANDLER_ENV_NOT_READY", "handler env not ready")
	ErrConfigIncomplete    = errorspkg.New(errorspkg.KindUnprocessable, "HANDLER_CONFIG_INCOMPLETE", "handler config incomplete (required init args unset)")
	ErrOpInvalid           = errorspkg.New(errorspkg.KindUnprocessable, "HANDLER_OP_INVALID", "invalid build op")
	ErrInvalidName         = errorspkg.New(errorspkg.KindInvalid, "HANDLER_INVALID_NAME", "invalid handler name (lowercase alphanumeric + dashes/underscores, 1-64 chars)")
	ErrInvalidCode         = errorspkg.New(errorspkg.KindUnprocessable, "HANDLER_INVALID_CODE", "handler class code invalid")
	ErrSandboxUnavailable  = errorspkg.New(errorspkg.KindUnavailable, "HANDLER_SANDBOX_UNAVAILABLE", "sandbox runtime unavailable")
	ErrInstanceSpawnFailed = errorspkg.New(errorspkg.KindBadGateway, "HANDLER_INSTANCE_SPAWN_FAILED", "handler instance spawn failed")
	ErrInstanceCrashed     = errorspkg.New(errorspkg.KindBadGateway, "HANDLER_CRASHED", "handler instance crashed")
	ErrInstanceRPCTimeout  = errorspkg.New(errorspkg.KindGatewayTimeout, "HANDLER_RPC_TIMEOUT", "handler instance RPC timeout")
	ErrConfigDecryptFailed = errorspkg.New(errorspkg.KindInternal, "HANDLER_CONFIG_DECRYPT_FAILED", "handler config decrypt failed")
)

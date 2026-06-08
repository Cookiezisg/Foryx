// Package function is the domain layer for user-forged Python sandbox functions:
// stateless code that runs in a fresh, isolated sandbox process per call (the first
// Quadrinity element). A Function owns an append-only line of Versions (each an
// immutable code+deps snapshot); ActiveVersionID is a free-moving pointer at the
// version currently in effect. There is NO pending/accept state machine — every edit
// writes a new version and takes effect immediately; revert just moves the pointer.
//
// Package function 是用户锻造的 Python 沙箱函数的 domain 层：每次调用在全新隔离沙箱进程里跑
// 的无状态代码（Quadrinity 第一元）。Function 持一条只增的 Version 线（每个是不可变的代码+依赖
// 快照）；ActiveVersionID 是指向当前生效版本的可自由移动指针。**无 pending/accept 状态机**——
// 每次编辑写新版本并立即生效；revert 只移指针。
package function

import (
	"time"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
)

// Function is a user-forged function; its code lives on the active Version, not here.
//
// Function 是用户锻造的 function；代码在 active Version 上，不在本表。
type Function struct {
	ID              string     `db:"id,pk"               json:"id"`
	WorkspaceID     string     `db:"workspace_id,ws"     json:"-"`
	Name            string     `db:"name"                json:"name"`
	Description     string     `db:"description"         json:"description"`
	Tags            []string   `db:"tags,json"           json:"tags"`
	ActiveVersionID string     `db:"active_version_id"   json:"activeVersionId"`
	CreatedAt       time.Time  `db:"created_at,created"  json:"createdAt"`
	UpdatedAt       time.Time  `db:"updated_at,updated"  json:"updatedAt"`
	DeletedAt       *time.Time `db:"deleted_at,deleted"  json:"-"`

	// ActiveVersion is a computed (non-column) field attached by Service.Get so a
	// reader sees the function's current code + env state in one round-trip.
	//
	// ActiveVersion 是计算字段（非列），由 Service.Get 附上，使读者一趟拿到当前代码 + env 状态。
	ActiveVersion *Version `db:"-" json:"activeVersion,omitempty"`
}

// DefaultPythonVersion is used when a function omits set_python_version.
//
// DefaultPythonVersion 在未设 set_python_version 时使用。
const DefaultPythonVersion = "3.12"

// Version is one immutable snapshot of a function's code + interface + deps. Version
// is a monotonic counter assigned at write time (max+1) — never reassigned, never
// renumbered. EnvID anchors the sandbox env materialized for this exact dep set.
//
// Version 是函数代码+接口+依赖的一份不可变快照。Version 是写入时分配的单调号（max+1）——绝不
// 重分配、绝不重排号。EnvID 锚定为这组确切依赖物化的 sandbox env。
type Version struct {
	ID                     string          `db:"id,pk"                      json:"id"`
	WorkspaceID            string          `db:"workspace_id,ws"            json:"-"`
	FunctionID             string          `db:"function_id"                json:"functionId"`
	Version                int             `db:"version"                    json:"version"`
	Code                   string          `db:"code"                       json:"code"`
	Inputs                 []schemapkg.Field `db:"inputs,json"             json:"inputs"`
	Outputs                []schemapkg.Field `db:"outputs,json"            json:"outputs"`
	Dependencies           []string        `db:"dependencies,json"          json:"dependencies"`
	PythonVersion          string          `db:"python_version"             json:"pythonVersion"`
	EnvID                  string          `db:"env_id"                     json:"envId"`
	EnvStatus              string          `db:"env_status"                 json:"envStatus"`
	EnvError               string          `db:"env_error"                  json:"envError,omitempty"`
	EnvSyncedAt            *time.Time      `db:"env_synced_at"              json:"envSyncedAt,omitempty"`
	ChangeReason           string          `db:"change_reason"              json:"changeReason,omitempty"`
	ForgedInConversationID *string         `db:"forged_in_conversation_id"  json:"forgedInConversationId,omitempty"`
	CreatedAt              time.Time       `db:"created_at,created"         json:"createdAt"`
	UpdatedAt              time.Time       `db:"updated_at,updated"         json:"updatedAt"`
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

var (
	// ErrNotFound: function id miss (scoped to workspace).
	//
	// ErrNotFound：function id 未命中（按 workspace 隔离）。
	ErrNotFound = errorsdomain.New(errorsdomain.KindNotFound, "FUNCTION_NOT_FOUND", "function not found")

	// ErrDuplicateName: a live function already owns this name in the workspace.
	//
	// ErrDuplicateName：workspace 内已有同名活跃 function。
	ErrDuplicateName = errorsdomain.New(errorsdomain.KindConflict, "FUNCTION_NAME_DUPLICATE", "function name already exists")

	// ErrVersionNotFound: version id / number miss.
	//
	// ErrVersionNotFound：version id / 号未命中。
	ErrVersionNotFound = errorsdomain.New(errorsdomain.KindNotFound, "FUNCTION_VERSION_NOT_FOUND", "function version not found")

	// ErrNoActiveVersion: function has no active version to run.
	//
	// ErrNoActiveVersion：function 无 active 版本可运行。
	ErrNoActiveVersion = errorsdomain.New(errorsdomain.KindUnprocessable, "FUNCTION_NO_ACTIVE_VERSION", "function has no active version")

	// ErrEnvNotReady: the version's sandbox env could not be built (deps won't install
	// even after the fix loop), so it cannot run.
	//
	// ErrEnvNotReady：版本的 sandbox env 建不起来（修复循环后依赖仍装不上），无法运行。
	ErrEnvNotReady = errorsdomain.New(errorsdomain.KindUnprocessable, "FUNCTION_ENV_NOT_READY", "function env not ready")

	// ErrSandboxUnavailable: sandbox runtime not ready (cannot materialize venv).
	//
	// ErrSandboxUnavailable：sandbox runtime 未就绪（无法物化 venv）。
	ErrSandboxUnavailable = errorsdomain.New(errorsdomain.KindUnavailable, "FUNCTION_SANDBOX_UNAVAILABLE", "sandbox runtime unavailable")

	// ErrOpInvalid: a forge op is malformed or leaves the draft invalid.
	//
	// ErrOpInvalid：锻造 op 畸形，或应用后草稿非法。
	ErrOpInvalid = errorsdomain.New(errorsdomain.KindUnprocessable, "FUNCTION_OP_INVALID", "invalid forge op")

	// ErrInvalidCode: final draft validation failed (no def, empty code, blacklisted import).
	//
	// ErrInvalidCode：草稿终校验失败（无 def、空代码、黑名单 import）。
	ErrInvalidCode = errorsdomain.New(errorsdomain.KindUnprocessable, "FUNCTION_INVALID_CODE", "function code invalid")

	// ErrExecutionNotFound: execution id miss.
	//
	// ErrExecutionNotFound：execution id 未命中。
	ErrExecutionNotFound = errorsdomain.New(errorsdomain.KindNotFound, "FUNCTION_EXECUTION_NOT_FOUND", "function execution not found")
)

// Package function is the domain layer for user-forged Python sandbox
// functions: Function entity, FunctionVersion (versioned snapshot with code
// + parameters + deps), Execution (D22 execution log row), sentinels,
// Repository port (→ infra/store/function).
//
// See documents/version-1.2/adhoc-topic-documents/forge_redesign/02-function.md
// for the full spec and 08-executions.md for the shared execution log schema.
//
// Package function 是用户锻造的 Python 沙箱函数的 domain 层:Function 实体、
// FunctionVersion(带 code+parameters+deps 的版本快照)、Execution(D22
// 执行日志行)、sentinel、Repository port(→ infra/store/function)。
package function

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Function is a user-forged Python sandbox function entity. Owned by one
// user; name unique while not soft-deleted (partial UNIQUE on
// (user_id, name) WHERE deleted_at IS NULL — added via schema_extras).
//
// Code / parameters / return_schema / deps live on FunctionVersion (versioned
// snapshots). Computed fields (Pending / EnvStatus / ...) are populated by
// service layer's attachPending / attachActiveEnv hooks from the version row.
//
// Function 是用户锻造的 Python 沙箱函数实体。属一个用户;未软删时 name 唯一。
// 代码 / 参数 / 返回 schema / 依赖在 FunctionVersion 上(版本快照)。
// 计算字段由 service 层从 active version 填。
type Function struct {
	ID              string         `gorm:"primaryKey;type:text" json:"id"`
	UserID          string         `gorm:"not null;index:idx_functions_user_id;type:text" json:"userId"`
	Name            string         `gorm:"not null;type:text" json:"name"`
	Description     string         `gorm:"type:text;default:''" json:"description"`
	Tags            []string       `gorm:"serializer:json;type:text;default:'[]'" json:"tags"`
	ActiveVersionID string         `gorm:"type:text;default:''" json:"activeVersionId"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`

	// Computed fields (gorm:"-") populated by service-layer attach hooks
	// after Repository read. Never directly serialized from DB.
	//
	// 计算字段(gorm:"-")由 service 层 attach 钩子在 Repository 读出后填。
	Pending       *Version   `gorm:"-" json:"pending,omitempty"`
	EnvStatus     string     `gorm:"-" json:"envStatus,omitempty"`
	EnvError      string     `gorm:"-" json:"envError,omitempty"`
	EnvSyncedAt   *time.Time `gorm:"-" json:"envSyncedAt,omitempty"`
	EnvSyncStage  string     `gorm:"-" json:"envSyncStage,omitempty"`
	EnvSyncDetail string     `gorm:"-" json:"envSyncDetail,omitempty"`
}

// TableName fixes the table name so GORM does not pluralize it elsewhere.
//
// TableName 显式指定表名(防 GORM 复数化漂移)。
func (Function) TableName() string { return "functions" }

// Sentinel errors. Wire codes registered in transport/httpapi/response/errmap.go.
// errors.Is must unwrap through fmt.Errorf("functionstore.Method: %w", err)
// chains back to these sentinels (§S16 error wrap discipline).
//
// 哨兵错误。HTTP wire code 在 errmap.go 登记。errors.Is 必须能从最外层
// fmt.Errorf 包装 unwrap 到这里的 sentinel(§S16)。
var (
	ErrNotFound             = errors.New("function: not found")
	ErrDuplicateName        = errors.New("function: duplicate name")
	ErrVersionNotFound      = errors.New("function: version not found")
	ErrPendingNotFound      = errors.New("function: pending not found")
	ErrPendingConflict      = errors.New("function: pending conflict")
	ErrRunFailed            = errors.New("function: run failed")
	ErrASTParseError        = errors.New("function: AST parse error")
	ErrNoActiveVersion      = errors.New("function: no active version")
	ErrEnvNotReady          = errors.New("function: env not ready")
	ErrEnvFailed            = errors.New("function: env failed")
	ErrDependencyResolution = errors.New("function: dependency resolution failed")
	ErrSandboxUnavailable   = errors.New("function: sandbox unavailable")
	ErrOpInvalid            = errors.New("function: op invalid")
)

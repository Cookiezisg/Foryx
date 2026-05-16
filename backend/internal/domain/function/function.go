// Package function is the domain layer for user-forged Python sandbox functions.
//
// Package function 是用户锻造的 Python 沙箱函数的 domain 层。
package function

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Function is a user-forged Python sandbox function; code lives on FunctionVersion.
//
// Function 是用户锻造的 Python 沙箱函数实体，代码在 FunctionVersion 版本快照。
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

	// Computed fields (gorm:"-") filled by service-layer attach hooks after Repository read.
	// 计算字段（gorm:"-"）由 service attach 钩子填，不直接从 DB 反序列化。
	Pending       *Version   `gorm:"-" json:"pending,omitempty"`
	EnvStatus     string     `gorm:"-" json:"envStatus,omitempty"`
	EnvError      string     `gorm:"-" json:"envError,omitempty"`
	EnvSyncedAt   *time.Time `gorm:"-" json:"envSyncedAt,omitempty"`
	EnvSyncStage  string     `gorm:"-" json:"envSyncStage,omitempty"`
	EnvSyncDetail string     `gorm:"-" json:"envSyncDetail,omitempty"`
}

func (Function) TableName() string { return "functions" }

var (
	ErrNotFound             = errors.New("function: not found")
	ErrDuplicateName        = errors.New("function: duplicate name")
	ErrVersionNotFound      = errors.New("function: version not found")
	ErrPendingNotFound      = errors.New("function: pending not found")
	ErrRunFailed            = errors.New("function: run failed")
	ErrASTParseError        = errors.New("function: AST parse error")
	ErrNoActiveVersion      = errors.New("function: no active version")
	ErrEnvNotReady          = errors.New("function: env not ready")
	ErrEnvFailed            = errors.New("function: env failed")
	ErrDependencyResolution = errors.New("function: dependency resolution failed")
	ErrSandboxUnavailable   = errors.New("function: sandbox unavailable")
	ErrOpInvalid            = errors.New("function: op invalid")
)

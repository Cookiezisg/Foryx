// Package handler defines the stateful-Python-class Handler domain (Definition + Instance two-tier).
//
// Package handler 定义有状态 Python 类的 Handler domain（Definition + Instance 二层）。
package handler

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Handler is the Definition entity; code / methods / init_args schema / deps live on Version.
//
// Handler 是 Definition 实体；代码/methods/init_args schema/deps 在 Version 上。
type Handler struct {
	ID              string         `gorm:"primaryKey;type:text" json:"id"`
	UserID          string         `gorm:"not null;index:idx_handlers_user_id;type:text" json:"userId"`
	Name            string         `gorm:"not null;type:text" json:"name"`
	Description     string         `gorm:"type:text;default:''" json:"description"`
	Tags            []string       `gorm:"serializer:json;type:text;default:'[]'" json:"tags"`
	ActiveVersionID string         `gorm:"type:text;default:''" json:"activeVersionId"`
	ConfigEncrypted string         `gorm:"type:text;default:''" json:"-"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`

	// Computed fields (gorm:"-") populated by service.attachComputed.
	// 计算字段（gorm:"-"），由 service.attachComputed 填。
	Pending       *Version   `gorm:"-" json:"pending,omitempty"`
	EnvStatus     string     `gorm:"-" json:"envStatus,omitempty"`
	EnvError      string     `gorm:"-" json:"envError,omitempty"`
	EnvSyncedAt   *time.Time `gorm:"-" json:"envSyncedAt,omitempty"`
	EnvSyncStage  string     `gorm:"-" json:"envSyncStage,omitempty"`
	EnvSyncDetail string     `gorm:"-" json:"envSyncDetail,omitempty"`
	ConfigState   string     `gorm:"-" json:"configState,omitempty"`
	LiveInstances int        `gorm:"-" json:"liveInstances,omitempty"`
}

func (Handler) TableName() string { return "handlers" }

var (
	ErrNotFound            = errors.New("handler: not found")
	ErrDuplicateName       = errors.New("handler: duplicate name")
	ErrMethodNotFound      = errors.New("handler: method not found")
	ErrVersionNotFound     = errors.New("handler: version not found")
	ErrPendingNotFound     = errors.New("handler: pending not found")
	ErrInstanceSpawnFailed = errors.New("handler: instance spawn failed")
	ErrInstanceCrashed     = errors.New("handler: instance crashed")
	ErrInstanceRPCTimeout  = errors.New("handler: instance RPC timeout")
	ErrInstanceNotFound    = errors.New("handler: instance not found")
	ErrNoActiveVersion     = errors.New("handler: no active version")
	ErrEnvNotReady         = errors.New("handler: env not ready")
	ErrEnvFailed           = errors.New("handler: env failed")
	ErrSandboxUnavailable  = errors.New("handler: sandbox unavailable")
	ErrOpInvalid           = errors.New("handler: op invalid")
	ErrASTParseError       = errors.New("handler: AST parse error")
	ErrConfigIncomplete    = errors.New("handler: config incomplete")
	ErrConfigInvalid       = errors.New("handler: config invalid")
	ErrConfigDecryptFailed = errors.New("handler: config decrypt failed")
)

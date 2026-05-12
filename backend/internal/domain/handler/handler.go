// Package handler defines the Handler domain — stateful Python class with
// methods, Definition + Instance two-tier, instance lifetime caller-owned
// (chat=per-call / workflow=per-run / test=per-test / session=explicit).
// Trinity second leg per forge_redesign Plan 02. See:
//
//   - documents/version-1.2/service-design-documents/handler.md (created with Plan 02)
//   - documents/version-1.2/adhoc-topic-documents/forge_redesign/03-handler.md
//
// Package handler 定义 Handler domain —— 有状态 Python 类 + 多 method,
// Definition + Instance 二层。Instance lifetime 由 caller-context 决定:
// chat = 单调用,workflow = run 全程,test = test 全程,session = 显式
// acquire/release。Trinity 第二条腿(forge_redesign Plan 02)。
package handler

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Handler is the Definition entity (per-user, partial UNIQUE on name while
// not soft-deleted). Code / methods / init_args schema / deps live on Version.
// Config(init_args values, AES-GCM 加密)is per-Definition.
//
// Handler 是 Definition 实体。代码 / methods / init_args schema / deps 在
// Version 上;Config(init_args 实际值,AES-GCM 加密)per-Definition 一份。
type Handler struct {
	ID              string         `gorm:"primaryKey;type:text" json:"id"`
	UserID          string         `gorm:"not null;index:idx_handlers_user_id;type:text" json:"userId"`
	Name            string         `gorm:"not null;type:text" json:"name"`
	Description     string         `gorm:"type:text;default:''" json:"description"`
	Tags            []string       `gorm:"serializer:json;type:text;default:'[]'" json:"tags"`
	ActiveVersionID string         `gorm:"type:text;default:''" json:"activeVersionId"`
	ConfigEncrypted string         `gorm:"type:text;default:''" json:"-"` // AES-GCM 密文;json:"-" 永不返
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`

	// Computed fields(gorm:"-") populated by service.attachComputed.
	// 计算字段(service.attachComputed 填)。
	Pending       *Version   `gorm:"-" json:"pending,omitempty"`
	EnvStatus     string     `gorm:"-" json:"envStatus,omitempty"`
	EnvError      string     `gorm:"-" json:"envError,omitempty"`
	EnvSyncedAt   *time.Time `gorm:"-" json:"envSyncedAt,omitempty"`
	EnvSyncStage  string     `gorm:"-" json:"envSyncStage,omitempty"`
	EnvSyncDetail string     `gorm:"-" json:"envSyncDetail,omitempty"`
	ConfigState   string     `gorm:"-" json:"configState,omitempty"` // ready / partially_configured / unconfigured
	LiveInstances int        `gorm:"-" json:"liveInstances,omitempty"` // in-memory registry count
}

func (Handler) TableName() string { return "handlers" }

// ── Sentinels ─────────────────────────────────────────────────────────────────

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

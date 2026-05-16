package handler

import "time"

const (
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusRejected = "rejected"
)

const (
	EnvStatusPending = "pending"
	EnvStatusSyncing = "syncing"
	EnvStatusReady   = "ready"
	EnvStatusFailed  = "failed"
	EnvStatusEvicted = "evicted"
)

const (
	ConfigStateUnconfigured        = "unconfigured"
	ConfigStatePartiallyConfigured = "partially_configured"
	ConfigStateReady               = "ready"
)

const DefaultPythonVersion = ">=3.12"

const AcceptedVersionCap = 50

// Version is a HandlerVersion snapshot of class code-parts + methods + init args + env state.
//
// Version 是 HandlerVersion 快照：class 各部分代码 + methods + init_args schema + env 状态。
type Version struct {
	ID        string `gorm:"primaryKey;type:text" json:"id"`
	HandlerID string `gorm:"not null;index:idx_handler_versions_handler_id;type:text" json:"handlerId"`
	Status    string `gorm:"not null;check:status IN ('pending','accepted','rejected');type:text;default:'pending'" json:"status"`
	Version   *int   `gorm:"type:integer" json:"version,omitempty"`

	Imports      string `gorm:"type:text;default:''" json:"imports"`
	InitBody     string `gorm:"type:text;default:''" json:"initBody"`
	ShutdownBody string `gorm:"type:text;default:''" json:"shutdownBody"`

	Methods        []MethodSpec  `gorm:"serializer:json;type:text;default:'[]'" json:"methods"`
	InitArgsSchema []InitArgSpec `gorm:"serializer:json;type:text;default:'[]'" json:"initArgsSchema"`
	Dependencies   []string      `gorm:"serializer:json;type:text;default:'[]'" json:"dependencies"`
	PythonVersion  string        `gorm:"type:text;default:''" json:"pythonVersion"`

	EnvID         string     `gorm:"index:idx_handler_versions_env_id;type:text;default:''" json:"envId"`
	EnvStatus     string     `gorm:"type:text;default:'pending'" json:"envStatus"`
	EnvError      string     `gorm:"type:text;default:''" json:"envError"`
	EnvSyncedAt   *time.Time `json:"envSyncedAt,omitempty"`
	EnvSyncStage  string     `gorm:"type:text;default:''" json:"envSyncStage"`
	EnvSyncDetail string     `gorm:"type:text;default:''" json:"envSyncDetail"`

	ChangeReason string    `gorm:"type:text;default:''" json:"changeReason"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func (Version) TableName() string { return "handler_versions" }

package function

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

const DefaultPythonVersion = ">=3.12"

// Version is a snapshot of code+parameters+return_schema+deps for one Function.
//
// Version 是 Function 在某时刻 code/parameters/return_schema/deps 的快照。
type Version struct {
	ID            string          `gorm:"primaryKey;type:text" json:"id"`
	FunctionID    string          `gorm:"not null;index:idx_function_versions_function_id;type:text" json:"functionId"`
	Status        string          `gorm:"not null;check:status IN ('pending','accepted','rejected');type:text;default:'pending'" json:"status"`
	Version       *int            `gorm:"type:integer" json:"version,omitempty"`
	Code          string          `gorm:"type:text;default:''" json:"code"`
	Parameters    []ParameterSpec `gorm:"serializer:json;type:text;default:'[]'" json:"parameters"`
	ReturnSchema  map[string]any  `gorm:"serializer:json;type:text;default:'{}'" json:"returnSchema"`
	Dependencies  []string        `gorm:"serializer:json;type:text;default:'[]'" json:"dependencies"`
	PythonVersion string          `gorm:"type:text;default:''" json:"pythonVersion"`
	EnvID         string          `gorm:"index:idx_function_versions_env_id;type:text;default:''" json:"envId"`
	EnvStatus     string          `gorm:"type:text;default:'pending'" json:"envStatus"`
	EnvError      string          `gorm:"type:text;default:''" json:"envError"`
	EnvSyncedAt   *time.Time      `json:"envSyncedAt,omitempty"`
	EnvSyncStage  string          `gorm:"type:text;default:''" json:"envSyncStage"`
	EnvSyncDetail string          `gorm:"type:text;default:''" json:"envSyncDetail"`
	ChangeReason  string          `gorm:"type:text;default:''" json:"changeReason"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

func (Version) TableName() string { return "function_versions" }

// ParameterSpec describes one declared input parameter (LLM self-reports via set_parameters op).
//
// ParameterSpec 描述声明的一个入参（LLM 通过 set_parameters op 自报）。
type ParameterSpec struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
	Default     any    `json:"default,omitempty"`
	Enum        []any  `json:"enum,omitempty"`
}

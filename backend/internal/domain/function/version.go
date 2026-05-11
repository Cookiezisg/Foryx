package function

import "time"

// Version-status enum. DB enforces via CHECK on Status column (see GORM tag).
//
// Version 状态枚举,DB CHECK 强制(GORM tag 上)。
const (
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusRejected = "rejected"
)

// Env-status enum. Sandbox v2 venv sync lifecycle for this version.
// V1 keeps this as a whitelist enforced at app layer (no DB CHECK).
//
// Env 同步状态枚举。Sandbox v2 venv 生命周期。V1 在 app 层校验白名单。
const (
	EnvStatusPending = "pending"
	EnvStatusSyncing = "syncing"
	EnvStatusReady   = "ready"
	EnvStatusFailed  = "failed"
	EnvStatusEvicted = "evicted"
)

// DefaultPythonVersion is the fallback when LLM does not specify
// python_version at forge time (PEP 440 spec).
//
// DefaultPythonVersion 是 LLM 不指定 python_version 时的回退(PEP 440)。
const DefaultPythonVersion = ">=3.12"

// Version is a snapshot of code + parameters + return_schema + deps for one
// Function at one point in time. status=accepted has a sequential Version
// integer; pending / rejected have Version=nil.
//
// EnvID = sha256(deps + python_version) — Functions with identical deps
// share venvs on disk (uv hardlink wheel cache lets multi-function disk ≈ 1×).
//
// Version 是 Function 在某时刻的 code / parameters / return_schema / deps
// 快照。accepted 时 Version 递增整数;pending/rejected 时 Version=nil。
// EnvID = sha256(deps + python_version),同 deps 跨 Function 共享 venv。
type Version struct {
	ID            string `gorm:"primaryKey;type:text" json:"id"`
	FunctionID    string `gorm:"not null;index:idx_function_versions_function_id;type:text" json:"functionId"`
	Status        string `gorm:"not null;check:status IN ('pending','accepted','rejected');type:text;default:'pending'" json:"status"`
	Version       *int   `gorm:"type:integer" json:"version,omitempty"` // NULL on pending / rejected
	Code          string `gorm:"type:text;default:''" json:"code"`
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

// TableName fixes the table name to keep it explicit through plural rules.
//
// TableName 显式指定表名。
func (Version) TableName() string { return "function_versions" }

// ParameterSpec describes one declared input parameter (LLM self-reports via
// set_parameters op; backend validates parameters match the Python function
// signature — D14).
//
// ParameterSpec 描述声明的一个入参(LLM 通过 set_parameters op 自报;
// 后端校验跟 Python 函数签名一致 — D14)。
type ParameterSpec struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // string | number | integer | boolean | object | array
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
	Default     any    `json:"default,omitempty"`
	Enum        []any  `json:"enum,omitempty"`
}

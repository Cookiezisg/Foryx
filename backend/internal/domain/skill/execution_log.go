// execution_log.go — per-skill Execution log (D22). One row per
// Service.Activate completion. Schema follows spec/08-executions.md §2
// (common 16) + §4.4 (skill-specific skill_name / skill_version SHA256 /
// fork_depth / substitutions JSON).
//
// execution_log.go —— per-skill Execution 日志(D22)。每 Service.Activate
// 终态写一行;schema 对齐 spec 08 §2 + §4.4。

package skill

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Execution status (4 terminal) + trigger source (4) — aligned with the
// other D22 tables (function_executions / handler_calls / mcp_calls).
//
// Execution 状态(4 终态)+ 触发源(4)对齐其他 D22 表。
const (
	ExecutionStatusOK        = "ok"
	ExecutionStatusFailed    = "failed"
	ExecutionStatusCancelled = "cancelled"
	ExecutionStatusTimeout   = "timeout"

	TriggeredByChat     = "chat"
	TriggeredByWorkflow = "workflow"
	TriggeredByHTTP     = "http"
	TriggeredByTest     = "test"
)

// Execution is one Service.Activate terminal record. Common 16 + 4 skill-
// specific (skill_name / skill_version / fork_depth / substitutions).
//
// Execution 是 Service.Activate 一次终态;16 通用 + 4 skill 专属。
type Execution struct {
	// Common 16
	ID             string         `gorm:"primaryKey;type:text" json:"id"`
	UserID         string         `gorm:"not null;index:idx_ske_user_id;type:text" json:"userId"`
	Status         string         `gorm:"not null;check:status IN ('ok','failed','cancelled','timeout');type:text" json:"status"`
	TriggeredBy    string         `gorm:"not null;check:triggered_by IN ('chat','workflow','http','test');type:text" json:"triggeredBy"`
	Input          map[string]any `gorm:"serializer:json;type:text;default:'{}'" json:"input"`
	Output         any            `gorm:"serializer:json;type:text" json:"output,omitempty"`
	ErrorCode      string         `gorm:"type:text;default:''" json:"errorCode,omitempty"`
	ErrorMessage   string         `gorm:"type:text;default:''" json:"errorMessage,omitempty"`
	ElapsedMs      int64          `gorm:"not null;default:0" json:"elapsedMs"`
	StartedAt      time.Time      `gorm:"not null;index:idx_ske_started_at,sort:desc" json:"startedAt"`
	EndedAt        time.Time      `gorm:"not null" json:"endedAt"`
	ConversationID string         `gorm:"type:text;default:'';index:idx_ske_conv,priority:1" json:"conversationId,omitempty"`
	MessageID      string         `gorm:"type:text;default:'';index:idx_ske_conv,priority:2" json:"messageId,omitempty"`
	ToolCallID     string         `gorm:"type:text;default:''" json:"toolCallId,omitempty"`
	FlowrunID      string         `gorm:"type:text;default:'';index:idx_ske_flowrun,priority:1" json:"flowrunId,omitempty"`
	FlowrunNodeID  string         `gorm:"type:text;default:''" json:"flowrunNodeId,omitempty"`

	// Skill-specific (spec 08 §4.4)
	SkillName     string         `gorm:"not null;type:text;index:idx_ske_skill,priority:1" json:"skillName"`
	SkillVersion  string         `gorm:"type:text;default:''" json:"skillVersion,omitempty"` // SHA256 of SKILL.md
	ForkDepth     int            `gorm:"not null;default:0" json:"forkDepth"`                // 0=inline, ≥1=fork nesting
	Substitutions map[string]any `gorm:"serializer:json;type:text;default:'{}'" json:"substitutions"`

	CreatedAt time.Time      `gorm:"index:idx_ske_skill,priority:2,sort:desc" json:"createdAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName pins the table name.
//
// TableName 显式指定表名。
func (Execution) TableName() string { return "skill_executions" }

// ExecutionFilter is the query shape for ListExecutions.
//
// ExecutionFilter 查询形状。
type ExecutionFilter struct {
	SkillName      string
	Status         string
	ConversationID string
	FlowrunID      string
	ForkDepth      *int // nil = any
	Cursor         string
	Limit          int
}

// ExecutionAggregates is the rollup returned alongside a list page.
//
// ExecutionAggregates 分页结果的聚合。
type ExecutionAggregates struct {
	OKCount        int   `json:"okCount"`
	FailedCount    int   `json:"failedCount"`
	CancelledCount int   `json:"cancelledCount"`
	TimeoutCount   int   `json:"timeoutCount"`
	AvgElapsedMs   int64 `json:"avgElapsedMs"`
	P95ElapsedMs   int64 `json:"p95ElapsedMs"`
}

// ErrExecutionNotFound is returned by GetExecutionByID when missing.
//
// ErrExecutionNotFound GetExecutionByID 未命中时返。
var ErrExecutionNotFound = errors.New("skill: execution not found")

// ExecutionRepository is the persistence port for skill_executions.
//
// ExecutionRepository 是 skill_executions 持久化端口。
type ExecutionRepository interface {
	SaveExecution(ctx context.Context, e *Execution) error
	GetExecutionByID(ctx context.Context, id string) (*Execution, error)
	ListExecutions(ctx context.Context, filter ExecutionFilter) ([]*Execution, string, error)
	ComputeAggregates(ctx context.Context, filter ExecutionFilter) (ExecutionAggregates, error)
}

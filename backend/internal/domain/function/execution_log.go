package function

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

const (
	ExecutionStatusOK        = "ok"
	ExecutionStatusFailed    = "failed"
	ExecutionStatusCancelled = "cancelled"
	ExecutionStatusTimeout   = "timeout"
)

const (
	TriggeredByChat     = "chat"
	TriggeredByWorkflow = "workflow"
	TriggeredByHTTP     = "http"
	TriggeredByTest     = "test"
)

// Execution is one terminal record of a Service.RunFunction call.
//
// Execution 是 Service.RunFunction 完成后的终态记录。
type Execution struct {
	ID             string         `gorm:"primaryKey;type:text" json:"id"`
	UserID         string         `gorm:"not null;index:idx_fne_user_id;type:text" json:"userId"`
	Status         string         `gorm:"not null;check:status IN ('ok','failed','cancelled','timeout');type:text" json:"status"`
	TriggeredBy    string         `gorm:"not null;check:triggered_by IN ('chat','workflow','http','test');type:text" json:"triggeredBy"`
	Input          map[string]any `gorm:"serializer:json;type:text;default:'{}'" json:"input"`
	Output         any            `gorm:"serializer:json;type:text" json:"output,omitempty"`
	ErrorCode      string         `gorm:"type:text;default:''" json:"errorCode,omitempty"`
	ErrorMessage   string         `gorm:"type:text;default:''" json:"errorMessage,omitempty"`
	ElapsedMs      int64          `gorm:"not null;default:0" json:"elapsedMs"`
	StartedAt      time.Time      `gorm:"not null;index:idx_fne_started_at,sort:desc" json:"startedAt"`
	EndedAt        time.Time      `gorm:"not null" json:"endedAt"`
	ConversationID string         `gorm:"type:text;default:'';index:idx_fne_conv,priority:1" json:"conversationId,omitempty"`
	MessageID      string         `gorm:"type:text;default:'';index:idx_fne_conv,priority:2" json:"messageId,omitempty"`
	ToolCallID     string         `gorm:"type:text;default:''" json:"toolCallId,omitempty"`
	FlowrunID      string         `gorm:"type:text;default:'';index:idx_fne_flowrun,priority:1" json:"flowrunId,omitempty"`
	FlowrunNodeID  string         `gorm:"type:text;default:''" json:"flowrunNodeId,omitempty"`

	FunctionID    string `gorm:"not null;type:text;index:idx_fne_function,priority:1" json:"functionId"`
	VersionID     string `gorm:"not null;type:text" json:"versionId"`
	PythonVersion string `gorm:"type:text;default:''" json:"pythonVersion,omitempty"`

	CreatedAt time.Time      `gorm:"index:idx_fne_function,priority:2,sort:desc" json:"createdAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Execution) TableName() string { return "function_executions" }

type ExecutionFilter struct {
	FunctionID     string
	VersionID      string
	Status         string
	ConversationID string
	FlowrunID      string
	Since          *time.Time
	Until          *time.Time
	Limit          int
	Cursor         string
}

// ExecutionAggregates is the rollup returned alongside a page of executions.
//
// ExecutionAggregates 是分页结果旁的聚合，便于一眼看 ok/failed 比例。
type ExecutionAggregates struct {
	OKCount        int   `json:"okCount"`
	FailedCount    int   `json:"failedCount"`
	CancelledCount int   `json:"cancelledCount"`
	TimeoutCount   int   `json:"timeoutCount"`
	AvgElapsedMs   int64 `json:"avgElapsedMs"`
	P95ElapsedMs   int64 `json:"p95ElapsedMs"`
}

var ErrExecutionNotFound = errors.New("function: execution not found")

// ExecutionRepository extends Repository with execution-log methods.
//
// ExecutionRepository 是 Repository 的扩展接口，带 execution log 方法。
type ExecutionRepository interface {
	SaveExecution(ctx context.Context, e *Execution) error
	GetExecutionByID(ctx context.Context, id string) (*Execution, error)
	ListExecutions(ctx context.Context, filter ExecutionFilter) ([]*Execution, string, error)
	ComputeAggregates(ctx context.Context, filter ExecutionFilter) (ExecutionAggregates, error)
}

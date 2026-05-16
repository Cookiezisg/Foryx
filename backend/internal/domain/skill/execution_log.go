package skill

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

	TriggeredByChat     = "chat"
	TriggeredByWorkflow = "workflow"
	TriggeredByHTTP     = "http"
	TriggeredByTest     = "test"
)

// Execution is one Service.Activate terminal record.
//
// Execution 是 Service.Activate 一次终态记录。
type Execution struct {
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

	SkillName     string         `gorm:"not null;type:text;index:idx_ske_skill,priority:1" json:"skillName"`
	SkillVersion  string         `gorm:"type:text;default:''" json:"skillVersion,omitempty"`
	ForkDepth     int            `gorm:"not null;default:0" json:"forkDepth"`
	Substitutions map[string]any `gorm:"serializer:json;type:text;default:'{}'" json:"substitutions"`

	CreatedAt time.Time      `gorm:"index:idx_ske_skill,priority:2,sort:desc" json:"createdAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Execution) TableName() string { return "skill_executions" }

type ExecutionFilter struct {
	SkillName      string
	Status         string
	ConversationID string
	FlowrunID      string
	ForkDepth      *int
	Cursor         string
	Limit          int
}

type ExecutionAggregates struct {
	OKCount        int   `json:"okCount"`
	FailedCount    int   `json:"failedCount"`
	CancelledCount int   `json:"cancelledCount"`
	TimeoutCount   int   `json:"timeoutCount"`
	AvgElapsedMs   int64 `json:"avgElapsedMs"`
	P95ElapsedMs   int64 `json:"p95ElapsedMs"`
}

var ErrExecutionNotFound = errors.New("skill: execution not found")

// ExecutionRepository is the persistence port for skill_executions.
//
// ExecutionRepository 是 skill_executions 的持久化端口。
type ExecutionRepository interface {
	SaveExecution(ctx context.Context, e *Execution) error
	GetExecutionByID(ctx context.Context, id string) (*Execution, error)
	ListExecutions(ctx context.Context, filter ExecutionFilter) ([]*Execution, string, error)
	ComputeAggregates(ctx context.Context, filter ExecutionFilter) (ExecutionAggregates, error)
}

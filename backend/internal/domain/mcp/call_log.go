package mcp

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

const (
	CallStatusOK        = "ok"
	CallStatusFailed    = "failed"
	CallStatusCancelled = "cancelled"
	CallStatusTimeout   = "timeout"
)

const (
	TriggeredByChat     = "chat"
	TriggeredByWorkflow = "workflow"
	TriggeredByHTTP     = "http"
	TriggeredByTest     = "test"
)

// Call is one terminal record of a Service.CallTool invocation.
//
// Call 是 Service.CallTool 完成后的终态记录。
type Call struct {
	ID             string         `gorm:"primaryKey;type:text" json:"id"`
	UserID         string         `gorm:"not null;index:idx_mcl_user_id;type:text" json:"userId"`
	Status         string         `gorm:"not null;check:status IN ('ok','failed','cancelled','timeout');type:text" json:"status"`
	TriggeredBy    string         `gorm:"not null;check:triggered_by IN ('chat','workflow','http','test');type:text" json:"triggeredBy"`
	Input          map[string]any `gorm:"serializer:json;type:text;default:'{}'" json:"input"`
	Output         any            `gorm:"serializer:json;type:text" json:"output,omitempty"`
	ErrorCode      string         `gorm:"type:text;default:''" json:"errorCode,omitempty"`
	ErrorMessage   string         `gorm:"type:text;default:''" json:"errorMessage,omitempty"`
	ElapsedMs      int64          `gorm:"not null;default:0" json:"elapsedMs"`
	StartedAt      time.Time      `gorm:"not null;index:idx_mcl_started_at,sort:desc" json:"startedAt"`
	EndedAt        time.Time      `gorm:"not null" json:"endedAt"`
	ConversationID string         `gorm:"type:text;default:'';index:idx_mcl_conv,priority:1" json:"conversationId,omitempty"`
	MessageID      string         `gorm:"type:text;default:'';index:idx_mcl_conv,priority:2" json:"messageId,omitempty"`
	ToolCallID     string         `gorm:"type:text;default:''" json:"toolCallId,omitempty"`
	FlowrunID      string         `gorm:"type:text;default:'';index:idx_mcl_flowrun,priority:1" json:"flowrunId,omitempty"`
	FlowrunNodeID  string         `gorm:"type:text;default:''" json:"flowrunNodeId,omitempty"`

	ServerName    string `gorm:"not null;type:text;index:idx_mcl_server,priority:1" json:"serverName"`
	ToolName      string `gorm:"not null;type:text" json:"toolName"`
	ServerVersion string `gorm:"type:text;default:''" json:"serverVersion,omitempty"`

	CreatedAt time.Time      `gorm:"index:idx_mcl_server,priority:2,sort:desc" json:"createdAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Call) TableName() string { return "mcp_calls" }

type CallFilter struct {
	ServerName     string
	ToolName       string
	Status         string
	ConversationID string
	FlowrunID      string
	Cursor         string
	Limit          int
}

type CallAggregates struct {
	OKCount        int   `json:"okCount"`
	FailedCount    int   `json:"failedCount"`
	CancelledCount int   `json:"cancelledCount"`
	TimeoutCount   int   `json:"timeoutCount"`
	AvgElapsedMs   int64 `json:"avgElapsedMs"`
	P95ElapsedMs   int64 `json:"p95ElapsedMs"`
}

var ErrCallNotFound = errors.New("mcp: call not found")

// CallRepository is the persistence port for mcp_calls (decoupled from in-memory registry).
//
// CallRepository 是 mcp_calls 持久化端口，与 in-memory server registry 解耦。
type CallRepository interface {
	SaveCall(ctx context.Context, c *Call) error
	GetCallByID(ctx context.Context, id string) (*Call, error)
	ListCalls(ctx context.Context, filter CallFilter) ([]*Call, string, error)
	ComputeAggregates(ctx context.Context, filter CallFilter) (CallAggregates, error)
}

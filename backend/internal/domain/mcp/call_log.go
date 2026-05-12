// call_log.go — per-MCP-server Call log (D22). One row written terminally
// per Service.CallTool — schema follows spec/08-executions.md §2 (common
// 16 fields) + §4.3 (mcp-specific server_name / tool_name / server_version).
//
// call_log.go —— per-MCP-server Call 日志(D22)。每 Service.CallTool 终态
// 写一行;schema 对齐 spec 08 §2 + §4.3。

package mcp

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Call status enumeration (4 terminal states — aligns with function /
// handler execution log).
//
// Call 状态枚举(4 终态)。
const (
	CallStatusOK        = "ok"
	CallStatusFailed    = "failed"
	CallStatusCancelled = "cancelled"
	CallStatusTimeout   = "timeout"
)

// Call trigger sources (4 fixed values).
//
// Call 触发源。
const (
	TriggeredByChat     = "chat"
	TriggeredByWorkflow = "workflow"
	TriggeredByHTTP     = "http"
	TriggeredByTest     = "test"
)

// Call is one terminal record of a Service.CallTool. Same 16 common
// fields as function_executions / handler_calls plus 3 mcp-specific.
//
// Call 是 Service.CallTool 一次终态记录。16 通用 + 3 mcp 专属。
type Call struct {
	// Common 16
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

	// MCP-specific (spec 08 §4.3)
	ServerName    string `gorm:"not null;type:text;index:idx_mcl_server,priority:1" json:"serverName"`
	ToolName      string `gorm:"not null;type:text" json:"toolName"`
	ServerVersion string `gorm:"type:text;default:''" json:"serverVersion,omitempty"`

	CreatedAt time.Time      `gorm:"index:idx_mcl_server,priority:2,sort:desc" json:"createdAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName pins the table name (matches function_executions pattern).
//
// TableName 显式指定表名。
func (Call) TableName() string { return "mcp_calls" }

// CallFilter is the query shape for ListCalls.
//
// CallFilter 查询形状。
type CallFilter struct {
	ServerName     string
	ToolName       string
	Status         string
	ConversationID string
	FlowrunID      string
	Cursor         string
	Limit          int
}

// CallAggregates is the rollup returned alongside a list page.
//
// CallAggregates 分页结果的聚合。
type CallAggregates struct {
	OKCount        int   `json:"okCount"`
	FailedCount    int   `json:"failedCount"`
	CancelledCount int   `json:"cancelledCount"`
	TimeoutCount   int   `json:"timeoutCount"`
	AvgElapsedMs   int64 `json:"avgElapsedMs"`
	P95ElapsedMs   int64 `json:"p95ElapsedMs"`
}

// ErrCallNotFound is returned by GetCallByID when the row is missing.
//
// ErrCallNotFound GetCallByID 未命中时返。
var ErrCallNotFound = errors.New("mcp: call not found")

// CallRepository is the persistence port for mcp_calls. Separate from
// the runtime mcp.Service registry — D22 writes go to GORM, the runtime
// list of servers stays in-memory.
//
// CallRepository 是 mcp_calls 持久化端口;跟 runtime in-memory server
// registry 解耦。
type CallRepository interface {
	SaveCall(ctx context.Context, c *Call) error
	GetCallByID(ctx context.Context, id string) (*Call, error)
	ListCalls(ctx context.Context, filter CallFilter) ([]*Call, string, error)
	ComputeAggregates(ctx context.Context, filter CallFilter) (CallAggregates, error)
}

package handler

import (
	"context"
	"time"
)

// Call status values.
//
// Call 状态值。
const (
	CallStatusOK        = "ok"
	CallStatusFailed    = "failed"
	CallStatusCancelled = "cancelled"
	CallStatusTimeout   = "timeout"
)

// CallStatuses is the closed call-status enum — used to reject illegal list-filter values (an
// out-of-set status would otherwise silently match zero rows, F168-M2).
//
// CallStatuses 是调用状态封闭集——用于拒非法 list 过滤值（非集内状态否则静默匹配 0 行，F168-M2）。
var CallStatuses = []string{CallStatusOK, CallStatusFailed, CallStatusCancelled, CallStatusTimeout}

// TriggeredBy values record which execution body invoked the method (same axis as
// function: who ran it, not how the request arrived).
//
// TriggeredBy 记录哪个执行体调用了方法（与 function 同轴：谁在跑，非请求怎么来的）。
const (
	TriggeredByChat     = "chat"
	TriggeredByAgent    = "agent"
	TriggeredByWorkflow = "workflow"
	TriggeredByManual   = "manual"
)

// IsValidTrigger reports whether s is a known trigger source (CHECK-constraint aligned).
//
// IsValidTrigger 报 s 是否已知触发来源（与 CHECK 约束对齐）。
func IsValidTrigger(s string) bool {
	switch s {
	case TriggeredByChat, TriggeredByAgent, TriggeredByWorkflow, TriggeredByManual:
		return true
	}
	return false
}

// Call is one terminal audit record of a method invocation. Log table: append-only,
// never soft- or hard-deleted (D1).
//
// Call 是一次方法调用的终态审计记录。Log 表：只增，绝不软删/硬删（D1）。
type Call struct {
	ID             string         `db:"id,pk"               json:"id"`
	WorkspaceID    string         `db:"workspace_id,ws"     json:"-"`
	HandlerID      string         `db:"handler_id"          json:"handlerId"`
	VersionID      string         `db:"version_id"          json:"versionId"`
	Method         string         `db:"method"              json:"method"`
	Status         string         `db:"status"              json:"status"`
	TriggeredBy    string         `db:"triggered_by"        json:"triggeredBy"`
	Input          map[string]any `db:"input,json"          json:"input"`
	Output         any            `db:"output,json"         json:"output,omitempty"`
	ErrorMessage   string         `db:"error_message"       json:"errorMessage,omitempty"`
	Logs           string         `db:"logs"                json:"logs,omitempty"`
	ElapsedMs      int64          `db:"elapsed_ms"          json:"elapsedMs"`
	StartedAt      time.Time      `db:"started_at"          json:"startedAt"`
	EndedAt        time.Time      `db:"ended_at"            json:"endedAt"`
	InstanceID     string         `db:"instance_id"         json:"instanceId,omitempty"`
	ConversationID string         `db:"conversation_id"     json:"conversationId,omitempty"`
	MessageID      string         `db:"message_id"          json:"messageId,omitempty"`
	ToolCallID     string         `db:"tool_call_id"        json:"toolCallId,omitempty"`
	FlowrunID      string         `db:"flowrun_id"          json:"flowrunId,omitempty"`
	FlowrunNodeID  string         `db:"flowrun_node_id"     json:"flowrunNodeId,omitempty"`
	CreatedAt      time.Time      `db:"created_at,created"  json:"createdAt"`
}

// CallFilter scopes a call-log query; empty fields are not constrained.
//
// CallFilter 约束 call-log 查询；空字段不约束。
type CallFilter struct {
	HandlerID      string
	VersionID      string
	Method         string
	Status         string
	TriggeredBy    string
	ConversationID string
	FlowrunID      string
	Cursor         string
	Limit          int
}

// CallAggregates is the ok / not-ok rollup beside a page of calls.
//
// CallAggregates 是分页旁的 ok / 非 ok 汇总。
type CallAggregates struct {
	OKCount     int `json:"okCount"`
	FailedCount int `json:"failedCount"`
}

// CallRepository is the call-log slice of Repository.
//
// CallRepository 是 Repository 的 call-log 切片。
type CallRepository interface {
	SaveCall(ctx context.Context, c *Call) error
	GetCallByID(ctx context.Context, id string) (*Call, error)
	ListCalls(ctx context.Context, filter CallFilter) ([]*Call, string, error)
	ComputeCallAggregates(ctx context.Context, filter CallFilter) (CallAggregates, error)
}

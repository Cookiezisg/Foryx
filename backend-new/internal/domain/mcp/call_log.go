package mcp

import (
	"context"
	"encoding/json"
	"time"
)

// Call status values (mirror handler 1:1).
//
// Call 状态值（与 handler 1:1）。
const (
	CallStatusOK        = "ok"
	CallStatusFailed    = "failed"
	CallStatusCancelled = "cancelled"
	CallStatusTimeout   = "timeout"
)

// TriggeredBy records which execution body invoked the tool (same axis as handler: who ran it).
// The dynamic chat tool passes "" and the service derives chat/agent from ctx; HTTP, the workflow
// dispatcher and the sensor poller set theirs explicitly.
//
// TriggeredBy 记录哪个执行体调用了工具（与 handler 同轴：谁在跑）。chat 动态工具传 ""、service 从 ctx 推
// chat/agent；HTTP、workflow dispatcher、sensor 轮询各自显式设。
const (
	CallTriggeredByChat     = "chat"
	CallTriggeredByAgent    = "agent"
	CallTriggeredByWorkflow = "workflow"
	CallTriggeredByManual   = "manual"
)

// IsValidCallTrigger reports whether s is a known trigger source (CHECK-constraint aligned).
//
// IsValidCallTrigger 报 s 是否已知触发来源（与 CHECK 约束对齐）。
func IsValidCallTrigger(s string) bool {
	switch s {
	case CallTriggeredByChat, CallTriggeredByAgent, CallTriggeredByWorkflow, CallTriggeredByManual:
		return true
	}
	return false
}

// Call is one terminal audit record of an MCP tool invocation (SSE-C C4: MCP joins function /
// handler / agent in having a durable execution log). Log table: append-only, never soft- or
// hard-deleted (D1). Leaner than handler_calls on purpose: MCP servers are version-less (no
// version_id) and externally owned (no instance_id — stderr has its own ring).
//
// Call 是一次 MCP 工具调用的终态审计记录（SSE-C C4：MCP 与 function/handler/agent 一样有耐久执行日志）。
// Log 表：只增，绝不软删/硬删（D1）。刻意比 handler_calls 精简：MCP server 无版本（无 version_id）、外部
// 所有（无 instance_id——stderr 自有 ring）。
type Call struct {
	ID             string          `db:"id,pk"               json:"id"`
	WorkspaceID    string          `db:"workspace_id,ws"     json:"-"`
	ServerID       string          `db:"server_id"           json:"serverId"`
	Tool           string          `db:"tool"                json:"tool"`
	Status         string          `db:"status"              json:"status"`
	TriggeredBy    string          `db:"triggered_by"        json:"triggeredBy"`
	Input          json.RawMessage `db:"input,json"          json:"input,omitempty"`
	Output         string          `db:"output"              json:"output,omitempty"`
	ErrorMessage   string          `db:"error_message"       json:"errorMessage,omitempty"`
	ElapsedMs      int64           `db:"elapsed_ms"          json:"elapsedMs"`
	StartedAt      time.Time       `db:"started_at"          json:"startedAt"`
	EndedAt        time.Time       `db:"ended_at"            json:"endedAt"`
	ConversationID string          `db:"conversation_id"     json:"conversationId,omitempty"`
	MessageID      string          `db:"message_id"          json:"messageId,omitempty"`
	ToolCallID     string          `db:"tool_call_id"        json:"toolCallId,omitempty"`
	CreatedAt      time.Time       `db:"created_at,created"  json:"createdAt"`
}

// CallFilter scopes a call-log query; empty fields are not constrained.
//
// CallFilter 约束 call-log 查询；空字段不约束。
type CallFilter struct {
	ServerID    string
	Tool        string
	Status      string
	TriggeredBy string
	Cursor      string
	Limit       int
}

// CallRepository is the call-log slice of Repository (Save on every invocation; Get fetches one
// for triage; List feeds the server panel's run history).
//
// CallRepository 是 Repository 的 call-log 切片（每次调用 Save；Get 取一条供 triage；List 喂 server 面板的运行历史）。
type CallRepository interface {
	SaveCall(ctx context.Context, c *Call) error
	GetCall(ctx context.Context, id string) (*Call, error)
	ListCalls(ctx context.Context, filter CallFilter) ([]*Call, string, error)
}

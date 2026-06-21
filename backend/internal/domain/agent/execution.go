package agent

import (
	"context"
	"encoding/json"
	"time"
)

// Execution status values (mirror function 1:1).
//
// Execution 状态值（与 function 1:1）。
const (
	ExecutionStatusOK        = "ok"
	ExecutionStatusFailed    = "failed"
	ExecutionStatusCancelled = "cancelled"
	ExecutionStatusTimeout   = "timeout"
)

// ExecutionStatuses is the closed execution-status enum — used to reject illegal list-filter values
// (an out-of-set status would otherwise silently match zero rows, F168-M2).
//
// ExecutionStatuses 是执行状态封闭集——用于拒非法 list 过滤值（非集内状态否则静默匹配 0 行，F168-M2）。
var ExecutionStatuses = []string{ExecutionStatusOK, ExecutionStatusFailed, ExecutionStatusCancelled, ExecutionStatusTimeout}

// TriggeredBy records which execution body invoked the agent — the axis is "who ran it", not
// "how the request arrived". There is no "agent" trigger: an agent cannot invoke another agent.
//
// TriggeredBy 记录哪个执行体调用了 agent——轴是「谁在跑」，非「请求怎么来的」。无 "agent" 触发：
// 员工不调员工。
const (
	TriggeredByChat     = "chat"     // LLM ran invoke_agent inside a user conversation
	TriggeredByWorkflow = "workflow" // a workflow agent node invoked it
	TriggeredByManual   = "manual"   // the user ran it by hand (REST :invoke)
)

// IsValidTrigger reports whether s is a known trigger source (CHECK-constraint aligned).
//
// IsValidTrigger 报 s 是否已知触发来源（与 CHECK 约束对齐）。
func IsValidTrigger(s string) bool {
	switch s {
	case TriggeredByChat, TriggeredByWorkflow, TriggeredByManual:
		return true
	}
	return false
}

// Execution is one terminal audit record of an InvokeAgent call. This is a log table:
// append-only, never soft- or hard-deleted (D1) — hence no DeletedAt column.
//
// Execution 是 InvokeAgent 一次完成的终态审计记录。这是 log 表：只增，绝不软删/硬删（D1）——
// 故无 DeletedAt 列。
type Execution struct {
	ID          string         `db:"id,pk"               json:"id"`
	WorkspaceID string         `db:"workspace_id,ws"     json:"-"`
	AgentID     string         `db:"agent_id"            json:"agentId"`
	VersionID   string         `db:"version_id"          json:"versionId"`
	ModelID     string         `db:"model_id"            json:"modelId,omitempty"` // which model actually ran
	Status      string         `db:"status"              json:"status"`
	TriggeredBy string         `db:"triggered_by"        json:"triggeredBy"`
	Input       map[string]any `db:"input,json"          json:"input"`
	Output      any            `db:"output,json"         json:"output,omitempty"`
	// Transcript is the agent's full block sequence (text / reasoning / tool_call / tool_result
	// across steps) serialized as JSON — the durable, self-contained record of the run. The chat
	// stream nests these blocks live under the invoke_agent tool_call; on reload the front end
	// rehydrates them from here (agent runs persist HERE, not in the shared message_blocks table).
	//
	// Transcript 是 agent 的完整 block 序列（跨步的 text/reasoning/tool_call/tool_result）序列化为 JSON
	// ——本次运行的耐久、自包含记录。chat 流把这些 block 实时嵌在 invoke_agent tool_call 下；reload 时前端从
	// 此处重水合（agent 运行落在**这里**，不与共享的 message_blocks 表公用）。
	Transcript     json.RawMessage `db:"transcript,json"     json:"transcript,omitempty"`
	ErrorMessage   string          `db:"error_message"       json:"errorMessage,omitempty"`
	ElapsedMs      int64           `db:"elapsed_ms"          json:"elapsedMs"`
	StartedAt      time.Time       `db:"started_at"          json:"startedAt"`
	EndedAt        time.Time       `db:"ended_at"            json:"endedAt"`
	ConversationID string          `db:"conversation_id"     json:"conversationId,omitempty"`
	MessageID      string          `db:"message_id"          json:"messageId,omitempty"`
	ToolCallID     string          `db:"tool_call_id"        json:"toolCallId,omitempty"`
	FlowrunID      string          `db:"flowrun_id"          json:"flowrunId,omitempty"`
	FlowrunNodeID  string          `db:"flowrun_node_id"     json:"flowrunNodeId,omitempty"`
	CreatedAt      time.Time       `db:"created_at,created"  json:"createdAt"`
}

// ExecutionFilter scopes an execution-log query; empty fields are not constrained.
//
// ExecutionFilter 约束 execution-log 查询；空字段不约束。
type ExecutionFilter struct {
	AgentID        string
	VersionID      string
	Status         string
	TriggeredBy    string
	ConversationID string
	FlowrunID      string
	Cursor         string
	Limit          int
}

// ExecutionAggregates is the slim ok/failed split beside a page of executions (mirrors function:
// no p95/avg — nobody consumed them, and the LLM reads elapsedMs off individual rows).
//
// ExecutionAggregates 是分页旁的精简 ok/failed 计数（对齐 function：无 p95/avg——无人消费，LLM
// 在意时自读单行 elapsedMs）。
type ExecutionAggregates struct {
	OKCount     int `json:"okCount"`
	FailedCount int `json:"failedCount"`
}

// ExecutionRepository is the execution-log slice of Repository.
//
// ExecutionRepository 是 Repository 的 execution-log 切片。
type ExecutionRepository interface {
	SaveExecution(ctx context.Context, e *Execution) error
	GetExecutionByID(ctx context.Context, id string) (*Execution, error)
	ListExecutions(ctx context.Context, filter ExecutionFilter) ([]*Execution, string, error)
	ComputeExecutionAggregates(ctx context.Context, filter ExecutionFilter) (ExecutionAggregates, error)
}

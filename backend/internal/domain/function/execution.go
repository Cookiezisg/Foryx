package function

import (
	"context"
	"time"
)

// Execution status values.
//
// Execution 状态值。
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

// TriggeredBy values record which execution body invoked the function. The axis is
// "who ran it" (an execution body), not "how the request arrived":
//   - chat:     LLM ran run_function inside a user conversation
//   - agent:    an agent run invoked it
//   - workflow: a workflow node invoked it
//   - manual:   the user ran it by hand from the editor (REST :run)
//
// TriggeredBy 记录哪个执行体调用了 function。轴是「谁在跑」（执行体），非「请求怎么来的」。
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

// ExecutionResult is the in-memory outcome of one sandbox Run (not persisted directly;
// recordExecution maps it into an Execution row). Logs is the function's own print()/
// debug output (capped head+tail by the adapter) — returned to the caller too, so the
// LLM's run_function tool_result carries what the code printed, not just its return value.
//
// ExecutionResult 是单次 sandbox Run 的内存结果（不直接持久化；recordExecution 映射成 Execution 行）。
// Logs 是函数自己的 print()/调试输出（adapter 做头尾限长）——也返回给调用方，使 LLM 的 run_function
// tool_result 带上代码打印的内容、而不只是返回值。
type ExecutionResult struct {
	OK        bool   `json:"ok"`
	Output    any    `json:"output"`
	ErrorMsg  string `json:"errorMsg"`
	ElapsedMs int64  `json:"elapsedMs"`
	Logs      string `json:"logs,omitempty"`
}

// Execution is one terminal audit record of a RunFunction call. This is a log table:
// append-only, never soft- or hard-deleted (D1).
//
// Execution 是 RunFunction 一次完成的终态审计记录。这是 log 表：只增，绝不软删/硬删（D1）。
type Execution struct {
	ID             string         `db:"id,pk"               json:"id"`
	WorkspaceID    string         `db:"workspace_id,ws"     json:"-"`
	FunctionID     string         `db:"function_id"         json:"functionId"`
	VersionID      string         `db:"version_id"          json:"versionId"`
	Status         string         `db:"status"              json:"status"`
	TriggeredBy    string         `db:"triggered_by"        json:"triggeredBy"`
	Input          map[string]any `db:"input,json"          json:"input"`
	Output         any            `db:"output,json"         json:"output,omitempty"`
	ErrorMessage   string         `db:"error_message"       json:"errorMessage,omitempty"`
	Logs           string         `db:"logs"                json:"logs,omitempty"`
	ElapsedMs      int64          `db:"elapsed_ms"          json:"elapsedMs"`
	StartedAt      time.Time      `db:"started_at"          json:"startedAt"`
	EndedAt        time.Time      `db:"ended_at"            json:"endedAt"`
	ConversationID string         `db:"conversation_id"     json:"conversationId,omitempty"`
	MessageID      string         `db:"message_id"          json:"messageId,omitempty"`
	ToolCallID     string         `db:"tool_call_id"        json:"toolCallId,omitempty"`
	FlowrunID      string         `db:"flowrun_id"          json:"flowrunId,omitempty"`
	FlowrunNodeID  string         `db:"flowrun_node_id"     json:"flowrunNodeId,omitempty"`
	CreatedAt      time.Time      `db:"created_at,created"  json:"createdAt"`
}

// ExecutionFilter scopes an execution-log query; empty fields are not constrained.
//
// ExecutionFilter 约束 execution-log 查询；空字段不约束。
type ExecutionFilter struct {
	FunctionID     string
	VersionID      string
	Status         string
	TriggeredBy    string
	ConversationID string
	FlowrunID      string
	Cursor         string
	Limit          int
}

// ExecutionAggregates is a slim rollup beside a page of executions — just the ok vs
// not-ok split for a status badge. (No p95 / avg: nobody consumed them, and the LLM
// reads elapsedMs off individual rows when it cares.)
//
// ExecutionAggregates 是分页旁的精简汇总——只 ok / 非 ok 计数供状态徽标。（无 p95/avg：无人
// 消费，LLM 在意时自己读单行 elapsedMs。）
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

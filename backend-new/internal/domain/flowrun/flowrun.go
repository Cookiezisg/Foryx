// Package flowrun is the domain layer for one workflow execution's DURABLE STATE — the
// truth a crash recovers from. It is NOT a forgeable entity (no catalog / relation / version);
// it is a runtime log written by the scheduler (M4.3) as it interprets a pinned graph.
//
// The model is node-result MEMOIZATION (DBOS / Conductor style), NOT an event-sourcing journal
// (Temporal style): there is no user code to replay, only a graph interpreter whose entire state
// is "which (node, iteration) completed with what result". That lives in flowrun_nodes — the one
// truth table. Re-running the interpreter (crash recovery / :replay) is idempotent because a
// completed row is copied, never re-executed (record-once on UNIQUE(flowrun_id,node_id,iteration)).
//
// Package flowrun 是「一次 workflow 执行的持久化状态」的 domain 层——崩溃从这里恢复。它不是可锻造
// 实体（无 catalog/relation/版本），是 scheduler（M4.3）解释钉死的图时写的运行时日志。
//
// 模型是**节点结果记忆化**（DBOS/Conductor 式），不是事件溯源日志（Temporal 式）：没有用户代码可
// 重放，只有图解释器，其全部状态 = 「哪些 (节点,轮次) 完成了、result 是啥」——这住在 flowrun_nodes
// 这张唯一真相表里。重跑解释器（崩溃恢复 / :replay）幂等，因为 completed 行被抄、绝不重跑
// （record-once 落在 UNIQUE(flowrun_id,node_id,iteration) 上）。
package flowrun

import (
	"time"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
)

// Run statuses. A run is running until terminal; "等人审批" is a NODE state (NodeParked), not a
// run state — "which runs await a human" is derived from parked flowrun_nodes rows, not a header
// column. cancelled is the kill terminal: a user hard-stopped the workflow (kill_workflow / :kill),
// distinct from failed (an activity errored) — it carries no engine fault, the run was simply
// terminated by hand.
//
// Run 状态。run 终态前一直 running；「等人审批」是**节点**状态（NodeParked）、不是 run 状态——
// 「哪些 run 在等人」从 parked 的 flowrun_nodes 行派生，不在头上冗余。cancelled 是 kill 终态：用户
// 硬停了 workflow（kill_workflow / :kill），区别于 failed（activity 出错）——它不带引擎故障，run 只是被手动终止。
const (
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// Node statuses. Rows are written TERMINAL-ONLY (no transient "running" row): an action runs and
// completes within one synchronous advance() pass, so there is no mid-flight node state to persist
// — a crash before the row is written simply re-runs (at-least-once, see doc 21 §8). parked is the
// one non-terminal state: an approval writes it before suspending, then a decision flips it to
// completed (first-wins conditional update).
//
// Node 状态。行只写**终态**（无瞬时 running 行）：action 在一次同步 advance() 内跑完，无中途节点态
// 可存——写行前崩溃就重跑（at-least-once，doc 21 §8）。parked 是唯一非终态：approval 挂起前写它，
// 决策再把它翻成 completed（first-wins 条件更新）。
const (
	NodeCompleted = "completed"
	NodeFailed    = "failed"
	NodeParked    = "parked"
)

// Result keys — the per-kind shape of FlowRunNode.Result (doc 21 §3.2). control/approval results
// are structured (a port/decision drives routing + carried data); action/agent results are the
// raw callable/agent output stored as-is.
//
// Result keys —— FlowRunNode.Result 的 per-kind 形状（doc 21 §3.2）。control/approval 的 result 有
// 结构（port/decision 驱动路由 + 携带数据）；action/agent 的 result 是 callable/agent 原始输出原样存。
const (
	// ResultKeyPort: a control node's chosen routing port, stored under this RESERVED key ALONGSIDE
	// the chosen branch's emitted fields (which are stored FLAT) — so downstream reads
	// gate.<emitField> directly (doc 20 §5.4: "下游按名读 gate.feedback") while the interpreter reads
	// gate.__port for routing. The double-underscore avoids colliding with a user emit field.
	// ResultKeyPort：control 节点选中的路由 port，存在这个**保留**键下，与选中分支 emit 的字段（扁平存）
	// 并列——故下游直接读 gate.<emit字段>（doc 20 §5.4），解释器读 gate.__port 路由。双下划线避免撞 emit 字段。
	ResultKeyPort     = "__port"   // control: chosen branch port (reserved routing key)
	ResultKeyDecision = "decision" // approval: yes | no (also downstream-readable)
	ResultKeyReason   = "reason"   // approval: human reason (optional)
	ResultKeyRendered = "rendered" // approval (parked): the rendered markdown for the inbox UI
)

// ControlResult builds a control node's memoized result: the chosen branch's emitted fields FLAT
// (so downstream reads gate.feedback, doc 20 §5.4) plus the reserved __port routing key.
//
// ControlResult 构造 control 节点的记忆化 result：选中分支 emit 的字段**扁平**（下游读 gate.feedback，
// doc 20 §5.4）+ 保留的 __port 路由键。
func ControlResult(port string, emit map[string]any) map[string]any {
	out := make(map[string]any, len(emit)+1)
	for k, v := range emit {
		out[k] = v
	}
	out[ResultKeyPort] = port
	return out
}

// ApprovalDecision builds a decided approval node's result. decision ∈ {yes,no}; reason may be "".
//
// ApprovalDecision 构造已决策 approval 节点的 result。decision ∈ {yes,no}；reason 可空。
func ApprovalDecision(decision, reason string) map[string]any {
	return map[string]any{ResultKeyDecision: decision, ResultKeyReason: reason}
}

// FlowRun is the execution header: the FROZEN topology (VersionID) + the FROZEN referenced-entity
// versions (PinnedRefs) an interpreter walks, plus status + replay bookkeeping. Pinning is the
// two locks that make replay deterministic: a mid-run edit to the workflow or any referenced
// entity cannot change a running flow (doc 21 §6 boundary 1). This is a Log table — NO soft delete
// (D1).
//
// FlowRun 是执行头：钉死的拓扑（VersionID）+ 钉死的引用实体版本（PinnedRefs），加状态 + replay 记账。
// pin 是让重放确定的两把锁：运行中编辑 workflow 或任何引用实体都改不动在途 run（doc 21 §6 边界一）。
// 这是 Log 表——无软删（D1）。
type FlowRun struct {
	ID          string            `db:"id,pk"               json:"id"`
	WorkspaceID string            `db:"workspace_id,ws"     json:"-"`
	WorkflowID  string            `db:"workflow_id"         json:"workflowId"`
	VersionID   string            `db:"version_id"          json:"versionId"`           // pinned wfv_ (graph topology)
	PinnedRefs  map[string]string `db:"pinned_refs,json"    json:"pinnedRefs"`          // BuildPinClosure {entity_id: active_version_id}
	TriggerID   string            `db:"trigger_id"          json:"triggerId,omitempty"` // entry trg_ (empty for a manual :trigger)
	FiringID    string            `db:"firing_id"           json:"firingId,omitempty"`  // source trf_ (single-tx claim)
	Status      string            `db:"status"              json:"status"`              // running | completed | failed
	ReplayCount int               `db:"replay_count"        json:"replayCount"`         // :replay increments; NOT a generation
	Error       string            `db:"error"               json:"error,omitempty"`     // terminal-failed reason
	StartedAt   time.Time         `db:"started_at,created"  json:"startedAt"`
	CompletedAt *time.Time        `db:"completed_at"        json:"completedAt,omitempty"`
	UpdatedAt   time.Time         `db:"updated_at,updated"  json:"updatedAt"`
}

// FlowRunNode is ★the truth: one (node, iteration) of a run with its memoized result. action /
// agent / control / approval each write their own row. UNIQUE(flowrun_id, node_id, iteration) is
// the record-once key — INSERT OR IGNORE makes the first write win (replay copies, never
// re-executes; approval first-wins falls out of it). Log table — NO soft delete (D1).
//
// FlowRunNode 是★真相：一个 run 的某 (节点,轮次) 及其记忆化 result。action/agent/control/approval
// 各写自己的行。UNIQUE(flowrun_id,node_id,iteration) 是 record-once 键——INSERT OR IGNORE 让首写赢
// （重放抄、绝不重跑；approval first-wins 由它落出）。Log 表——无软删（D1）。
type FlowRunNode struct {
	ID          string         `db:"id,pk"               json:"id"`
	WorkspaceID string         `db:"workspace_id,ws"     json:"-"`
	FlowRunID   string         `db:"flowrun_id"          json:"flowrunId"`
	NodeID      string         `db:"node_id"             json:"nodeId"`    // graph-local id (= the downstream reference name)
	Iteration   int            `db:"iteration"           json:"iteration"` // loop turn, 0-based
	Kind        string         `db:"kind"                json:"kind"`      // trigger|action|agent|control|approval
	Ref         string         `db:"ref"                 json:"ref"`       // pinned entity ref (audit)
	Status      string         `db:"status"              json:"status"`    // completed | failed | parked
	Result      map[string]any `db:"result,json"         json:"result"`    // per-kind shape (Result keys)
	Error       string         `db:"error"               json:"error,omitempty"`
	CreatedAt   time.Time      `db:"created_at,created"  json:"createdAt"`             // terminal write / park time
	CompletedAt *time.Time     `db:"completed_at"        json:"completedAt,omitempty"` // nil while parked
	UpdatedAt   time.Time      `db:"updated_at,updated"  json:"updatedAt"`
}

var (
	// ErrNotFound: flowrun id miss (scoped to workspace).
	// ErrNotFound：flowrun id 未命中（按 workspace 隔离）。
	ErrNotFound = errorsdomain.New(errorsdomain.KindNotFound, "FLOWRUN_NOT_FOUND", "flowrun not found")

	// ErrNotReplayable: :replay called on a run that is not in a failed state (nothing to fix).
	// ErrNotReplayable：对非 failed 状态的 run 调 :replay（没坏东西可修）。
	ErrNotReplayable = errorsdomain.New(errorsdomain.KindUnprocessable, "FLOWRUN_NOT_REPLAYABLE", "flowrun is not in a replayable (failed) state")

	// ErrNodeNotParked: an approval decision targeted a node that is not awaiting a signal (already
	// decided / timed out / never parked) — the first-wins loser, surfaced as a clean 422.
	// ErrNodeNotParked：审批决策指向一个不在等信号的节点（已决/已超时/从未 park）——first-wins 的输家，
	// 以干净 422 上呈。
	ErrNodeNotParked = errorsdomain.New(errorsdomain.KindUnprocessable, "FLOWRUN_APPROVAL_NOT_PARKED", "approval node is not awaiting a decision")

	// ErrInvalidEntry: a manual :trigger named an entry node that is missing or not a trigger, or
	// omitted entryNode for a graph with multiple trigger nodes (ambiguous). Details carry the reason.
	// ErrInvalidEntry：手动 :trigger 指定的 entry 节点缺失/非 trigger，或多 trigger 图未指定 entryNode
	// （歧义）。details 带原因。
	ErrInvalidEntry = errorsdomain.New(errorsdomain.KindUnprocessable, "FLOWRUN_INVALID_ENTRY", "invalid or ambiguous trigger entry node")

	// ErrInvalidDecision: an approval decision was neither "yes" nor "no".
	// ErrInvalidDecision：审批决策既非 "yes" 也非 "no"。
	ErrInvalidDecision = errorsdomain.New(errorsdomain.KindUnprocessable, "FLOWRUN_INVALID_DECISION", "approval decision must be 'yes' or 'no'")
)

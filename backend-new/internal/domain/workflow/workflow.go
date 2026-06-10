// Package workflow is the domain layer for workflow graph entities (wf_): the
// orchestrator of the Quadrinity — "function 范式套一张图". A Workflow owns an
// append-only line of immutable graph Versions; ActiveVersionID is a free-moving
// pointer at the version currently in effect. Like function/control it has NO
// pending/accept state machine — every edit writes a new version and takes effect
// immediately; revert just moves the pointer.
//
// A Version's Graph is a static DAG-with-back-edges of typed Nodes (trigger / action /
// agent / control / approval) wired by Edges. The graph is the orchestration recipe; it
// references other entities by id (trg_/fn_/hd_/mcp:/ag_/ctl_/apf_) and wires their I/O
// with bare CEL in each node's Input map. This package STORES + VALIDATES + PINS that
// graph — it does NOT execute it: the durable interpreter (later wave) imports the same
// pure helpers (ValidateGraph, BackEdges) and walks the pinned version. CEL is NOT
// compiled here (domain must not import cel-go, 原则 #3) — the app layer compiles every
// node.Input value via pkg/cel at create/edit time.
//
// Package workflow 是 workflow 图实体（wf_）的 domain 层：Quadrinity 的编排者——「function
// 范式套一张图」。Workflow 持一条只增的不可变图 Version 线；ActiveVersionID 是指向当前生效版本
// 的可自由移动指针。与 function/control 同：**无 pending/accept 状态机**——每次编辑写新版本并
// 立即生效；revert 只移指针。
//
// Version 的 Graph 是一张静态「DAG + 回边」的有类型 Node（trigger / action / agent / control /
// approval）图，由 Edge 接线。图是编排配方；它按 id 引用其它实体（trg_/fn_/hd_/mcp:/ag_/ctl_/
// apf_），并用每个 node.Input 里的裸 CEL 接线其 I/O。本包 STORE + VALIDATE + PIN 这张图——不执行
// 它：durable 解释器（后续波次）import 同一批纯 helper（ValidateGraph、BackEdges）并走 pin 的
// 版本。CEL 不在此编译（domain 不准 import cel-go，原则 #3）——app 层 create/edit 时用 pkg/cel
// 编译每个 node.Input 值。
package workflow

import (
	"time"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
)

// Workflow is a workflow graph entity; its graph lives on the active Version, not here.
// The lifecycle/concurrency/attention columns govern how the (future) durable scheduler
// treats it — they live on the header because they outlive any single graph version.
//
// Workflow 是一个 workflow 图实体；图在 active Version 上，不在本表。lifecycle/concurrency/
// attention 列治理（未来）durable 调度器如何对待它——放在头上，因为它们比任何单个图版本更长寿。
type Workflow struct {
	ID              string     `db:"id,pk"               json:"id"`
	WorkspaceID     string     `db:"workspace_id,ws"     json:"-"`
	Name            string     `db:"name"                json:"name"`
	Description     string     `db:"description"         json:"description"`
	Tags            []string   `db:"tags,json"           json:"tags"`
	Active          bool       `db:"active"              json:"active"`
	LifecycleState  string     `db:"lifecycle_state"     json:"lifecycleState"`
	Concurrency     string     `db:"concurrency"         json:"concurrency"`
	NeedsAttention  bool       `db:"needs_attention"     json:"needsAttention"`
	AttentionReason string     `db:"attention_reason"    json:"attentionReason,omitempty"`
	LastActionBy    string     `db:"last_action_by"      json:"lastActionBy"`
	ActiveVersionID string     `db:"active_version_id"   json:"activeVersionId"`
	CreatedAt       time.Time  `db:"created_at,created"  json:"createdAt"`
	UpdatedAt       time.Time  `db:"updated_at,updated"  json:"updatedAt"`
	DeletedAt       *time.Time `db:"deleted_at,deleted"  json:"-"`

	// ActiveVersion is a computed (non-column) field attached by Service.Get so a reader
	// sees the current graph in one round-trip.
	//
	// ActiveVersion 是计算字段（非列），由 Service.Get 附上，使读者一趟拿到当前图。
	ActiveVersion *Version `db:"-" json:"activeVersion,omitempty"`
}

// Version is one immutable snapshot of a workflow's graph. Version is a monotonic counter
// assigned at write time (max+1) — never reassigned, never renumbered. Graph is the JSON
// blob of Graph; GraphParsed is the decoded form, attached by the app on read.
//
// Version 是 workflow 图的一份不可变快照。Version 是写入时分配的单调号（max+1）——绝不重分配、
// 绝不重排号。Graph 是 Graph 的 JSON blob；GraphParsed 是解码形式，由 app 读时附上。
type Version struct {
	ID                     string    `db:"id,pk"                     json:"id"`
	WorkspaceID            string    `db:"workspace_id,ws"           json:"-"`
	WorkflowID             string    `db:"workflow_id"               json:"workflowId"`
	Version                int       `db:"version"                   json:"version"`
	Graph                  string    `db:"graph"                     json:"graph"`
	ChangeReason           string    `db:"change_reason"             json:"changeReason,omitempty"`
	ForgedInConversationID *string   `db:"forged_in_conversation_id" json:"forgedInConversationId,omitempty"`
	CreatedAt              time.Time `db:"created_at,created"        json:"createdAt"`
	UpdatedAt              time.Time `db:"updated_at,updated"        json:"updatedAt"`

	// GraphParsed is the decoded Graph (non-column), attached by Service.Get / app pipeline
	// so a reader sees nodes+edges without re-parsing the JSON blob.
	//
	// GraphParsed 是解码后的 Graph（非列），由 Service.Get / app 流程附上，使读者无需重解 JSON
	// 即见 nodes+edges。
	GraphParsed *Graph `db:"-" json:"graphParsed,omitempty"`
}

// Graph is the orchestration recipe: typed nodes wired by edges. It is a DAG except where
// a control/approval node closes a structured loop (a reducible back edge — see BackEdges).
//
// Graph 是编排配方：有类型 node 由 edge 接线。除非 control/approval 节点闭合一个结构化循环
// （可归约回边——见 BackEdges），否则是 DAG。
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Node kinds. Each kind references exactly one entity family by ref-prefix (see RefPrefix*).
//
// Node 类型。每种按 ref 前缀（见 RefPrefix*）恰引用一个实体族。
const (
	NodeKindTrigger  = "trigger"  // trg_ — the graph's entry signal source; Input empty
	NodeKindAction   = "action"   // fn_ / hd_<id>.method / mcp:server/tool — a durable activity
	NodeKindAgent    = "agent"    // ag_ — a configured LLM worker
	NodeKindControl  = "control"  // ctl_ — routing logic; FromPort selects a branch
	NodeKindApproval = "approval" // apf_ — a human approval gate; FromPort ∈ {yes,no}
)

// Ref prefixes per kind. action accepts any of the three activity prefixes.
//
// 各类型 ref 前缀。action 接受三种 activity 前缀之一。
const (
	RefPrefixTrigger  = "trg_"
	RefPrefixFunction = "fn_"
	RefPrefixHandler  = "hd_"
	RefPrefixMCP      = "mcp:"
	RefPrefixAgent    = "ag_"
	RefPrefixControl  = "ctl_"
	RefPrefixApproval = "apf_"
)

// Approval ports — the two structural outcomes an approval node routes on.
//
// approval 端口——approval 节点路由的两个结构性结局。
const (
	ApprovalPortYes = "yes"
	ApprovalPortNo  = "no"
)

// Node is one graph vertex. ID is graph-local (also the name this node's result is
// referenced by in downstream Input CEL). Kind selects the entity family; Ref is the
// referenced entity (resolved to an active version at pin time, NOT here). Input maps each
// declared field of the referenced entity to a bare CEL expression over upstream node
// results — ALL kinds use this (a trigger node has none). Retry tunes an action's durable
// retry; Pos/Notes are authoring-only.
//
// Node 是图的一个顶点。ID 是图内局部（也是下游 Input CEL 里引用本节点结果的名字）。Kind 选实体
// 族；Ref 是被引用实体（pin 时解析为 active 版本，不在此）。Input 把被引用实体的每个声明字段映射到
// 一条读上游节点结果的裸 CEL——所有类型都用它（trigger 节点没有）。Retry 调 action 的 durable
// 重试；Pos/Notes 仅供编排。
type Node struct {
	ID    string            `json:"id"`
	Kind  string            `json:"kind"`
	Ref   string            `json:"ref"`
	Input map[string]string `json:"input,omitempty"`
	Retry *RetryConfig      `json:"retry,omitempty"`
	Pos   *Position         `json:"pos,omitempty"`
	Notes string            `json:"notes,omitempty"`
}

// Edge is one directed wire From → To. FromPort selects a branch on a control/approval
// source (empty for other kinds); the durable interpreter routes a control branch / an
// approval outcome to the edge whose FromPort matches.
//
// Edge 是一条有向连线 From → To。FromPort 在 control/approval 源上选分支（其它类型为空）；durable
// 解释器把 control 分支 / approval 结局路由到 FromPort 匹配的边。
type Edge struct {
	ID       string `json:"id"`
	From     string `json:"from"`
	FromPort string `json:"fromPort,omitempty"`
	To       string `json:"to"`
}

// RetryConfig tunes an action node's durable retry policy (interpreted later by the
// scheduler; stored verbatim here).
//
// RetryConfig 调 action 节点的 durable 重试策略（由调度器后续解释；此处原样存储）。
type RetryConfig struct {
	MaxAttempts int    `json:"maxAttempts"`
	Backoff     string `json:"backoff,omitempty"`
	DelayMs     int    `json:"delayMs,omitempty"`
}

// Position is the node's canvas coordinate (authoring metadata; ignored by execution).
//
// Position 是节点画布坐标（编排元数据；执行忽略）。
type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Lifecycle states govern scheduler participation. active accepts triggers; draining
// finishes in-flight runs but starts no new ones; inactive is fully parked.
//
// Lifecycle 状态治理调度参与。active 接受触发；draining 跑完在途、不再起新；inactive 完全停泊。
const (
	LifecycleActive   = "active"
	LifecycleDraining = "draining"
	LifecycleInactive = "inactive"
)

// IsValidLifecycle reports whether s is one of the three lifecycle states.
//
// IsValidLifecycle 报告 s 是否三种 lifecycle 状态之一。
func IsValidLifecycle(s string) bool {
	switch s {
	case LifecycleActive, LifecycleDraining, LifecycleInactive:
		return true
	}
	return false
}

// Concurrency policies decide what the scheduler does when a run is requested while one is
// already in flight. serial waits; Skip drops the new request; BufferOne keeps only the
// latest pending; BufferAll queues every request; AllowAll runs them concurrently.
//
// Concurrency 策略决定有运行在途时再请求运行调度器怎么办。serial 等待；Skip 丢弃新请求；BufferOne
// 仅留最新待处理；BufferAll 全排队；AllowAll 并发跑。
const (
	ConcurrencySerial    = "serial"
	ConcurrencySkip      = "Skip"
	ConcurrencyBufferOne = "BufferOne"
	ConcurrencyBufferAll = "BufferAll"
	ConcurrencyAllowAll  = "AllowAll"
)

// IsValidConcurrency reports whether s is one of the five concurrency policies.
//
// IsValidConcurrency 报告 s 是否五种 concurrency 策略之一。
func IsValidConcurrency(s string) bool {
	switch s {
	case ConcurrencySerial, ConcurrencySkip, ConcurrencyBufferOne, ConcurrencyBufferAll, ConcurrencyAllowAll:
		return true
	}
	return false
}

// LastActionBy distinguishes a user-initiated state change from a system one (e.g. the
// scheduler auto-parking a workflow that lost its trigger).
//
// LastActionBy 区分用户发起的状态变更与系统发起的（如调度器自动停泊丢了 trigger 的 workflow）。
const (
	ActorUser   = "user"
	ActorSystem = "system"
)

var (
	// ErrNotFound: workflow id miss (scoped to workspace).
	// ErrNotFound：workflow id 未命中（按 workspace 隔离）。
	ErrNotFound = errorsdomain.New(errorsdomain.KindNotFound, "WORKFLOW_NOT_FOUND", "workflow not found")

	// ErrDuplicateName: a live workflow already owns this name in the workspace.
	// ErrDuplicateName：workspace 内已有同名活跃 workflow。
	ErrDuplicateName = errorsdomain.New(errorsdomain.KindConflict, "WORKFLOW_NAME_DUPLICATE", "workflow name already exists")

	// ErrVersionNotFound: version id / number miss.
	// ErrVersionNotFound：version id / 号未命中。
	ErrVersionNotFound = errorsdomain.New(errorsdomain.KindNotFound, "WORKFLOW_VERSION_NOT_FOUND", "workflow version not found")

	// ErrNoActiveVersion: workflow has no active version (graph) yet.
	// ErrNoActiveVersion：workflow 尚无 active 版本（图）。
	ErrNoActiveVersion = errorsdomain.New(errorsdomain.KindUnprocessable, "WORKFLOW_NO_ACTIVE_VERSION", "workflow has no active version")

	// ErrInvalidGraph: the graph failed structural validation (shape / wiring / cycles /
	// ports). The descriptive cause rides in details["reason"].
	// ErrInvalidGraph：图未过结构校验（形状 / 接线 / 环 / 端口）。具体原因在 details["reason"]。
	ErrInvalidGraph = errorsdomain.New(errorsdomain.KindUnprocessable, "WORKFLOW_INVALID_GRAPH", "workflow graph is invalid")

	// ErrInvalidOps: a graph op is malformed or leaves the graph invalid.
	// ErrInvalidOps：图 op 畸形，或应用后图非法。
	ErrInvalidOps = errorsdomain.New(errorsdomain.KindUnprocessable, "WORKFLOW_INVALID_OPS", "invalid workflow ops")

	// ErrRefNotFound: a node Ref does not resolve, or its resolved kind/port/method
	// mismatches (raised by the app CapabilityCheck against the resolver).
	// ErrRefNotFound：某 node Ref 解析不到，或解析出的 kind/port/method 不符（由 app CapabilityCheck
	// 据 resolver 抛出）。
	ErrRefNotFound = errorsdomain.New(errorsdomain.KindUnprocessable, "WORKFLOW_REF_NOT_FOUND", "workflow node ref not found or mismatched")

	// ErrInvalidLifecycle: an illegal lifecycle value or transition.
	// ErrInvalidLifecycle：非法 lifecycle 值或转换。
	ErrInvalidLifecycle = errorsdomain.New(errorsdomain.KindUnprocessable, "WORKFLOW_INVALID_LIFECYCLE", "invalid workflow lifecycle state or transition")

	// ErrNoTriggerEntry: an execution-lifecycle action (activate / stage) needs the workflow's entry
	// trigger node(s) to bind a listener, but the active graph has none (a manual-only graph can only
	// be :trigger-ed by hand, never armed to listen).
	// ErrNoTriggerEntry：执行生命周期动作（activate / stage）要靠 workflow 的入口 trigger 节点挂监听，但
	// active 图里没有（纯手动图只能手动 :trigger、无从挂监听）。
	ErrNoTriggerEntry = errorsdomain.New(errorsdomain.KindUnprocessable, "WORKFLOW_NO_TRIGGER_ENTRY", "workflow has no entry trigger node to listen on")

	// ErrAlreadyActive: stage (one-shot arm) was called on a workflow that is already active — it is
	// continuously listening, so a one-shot arm is meaningless (deactivate it first to stage).
	// ErrAlreadyActive：对已 active 的 workflow 调 stage（一次性待命）——它已在持续监听，一次性待命无意义
	// （先 deactivate 再 stage）。
	ErrAlreadyActive = errorsdomain.New(errorsdomain.KindConflict, "WORKFLOW_ALREADY_ACTIVE", "workflow is already active; deactivate before staging a one-shot run")
)

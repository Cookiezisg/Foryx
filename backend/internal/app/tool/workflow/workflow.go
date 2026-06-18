// Package workflow provides the LLM system tools for the user's workflow library. Two groups:
// the BUILD/QUERY tools that edit the graph — search / get / create / edit / revert / delete /
// capability_check (build.go, query.go); and the EXECUTION-LIFECYCLE tools that drive its runtime —
// trigger / stage / activate / deactivate / kill (exec.go, D1, over the durable scheduler + trigger
// binder). All are lazy tools (Toolset.Lazy) — surfaced via search_tools, not resident.
//
// Package workflow 提供操作用户 workflow 库的 LLM system tool。两组：编辑图的 BUILD/QUERY 工具——
// search / get / create / edit / revert / delete / capability_check（build.go, query.go）；驱动其运行时的
// 执行生命周期工具——trigger / stage / activate / deactivate / kill（exec.go，D1，基于 durable 调度器 +
// trigger binder）。全是懒加载工具（Toolset.Lazy）——经 search_tools 浮现、非常驻。
package workflow

import (
	schedulerapp "github.com/sunweilin/anselm/backend/internal/app/scheduler"
	searchapp "github.com/sunweilin/anselm/backend/internal/app/search"
	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	workflowapp "github.com/sunweilin/anselm/backend/internal/app/workflow"
)

// WorkflowTools constructs the workflow system tools over the app service; sched is the
// durable scheduler's read surface backing the run-observability tools (runs.go).
//
// WorkflowTools 基于 app service 构造 workflow system tool；sched 是 durable 调度器的读取面，
// 支撑运行可观测工具（runs.go）。
func WorkflowTools(svc *workflowapp.Service, content *searchapp.Service, sched *schedulerapp.Service) []toolapp.Tool {
	return []toolapp.Tool{
		&SearchWorkflow{svc: svc, content: content},
		&GetWorkflow{svc: svc},
		&CreateWorkflow{svc: svc},
		&EditWorkflow{svc: svc},
		&RevertWorkflow{svc: svc},
		&DeleteWorkflow{svc: svc},
		&CapabilityCheckWorkflow{svc: svc},
		// execution lifecycle (D1)
		&TriggerWorkflow{svc: svc},
		&StageWorkflow{svc: svc},
		&ActivateWorkflow{svc: svc},
		&DeactivateWorkflow{svc: svc},
		&KillWorkflow{svc: svc},
		// run observability — read back what the lifecycle verbs started
		// 运行可观测——把生命周期动词启动的东西读回来
		&GetFlowrun{sched: sched},
		&SearchFlowruns{sched: sched},
		// durable recovery — re-run a failed run from where it broke (clears failed nodes, keeps memoized)
		// durable 恢复——从断点重跑失败 run（清 failed 节点、留记忆化）
		&ReplayFlowrun{sched: sched},
	}
}

// opsDoc documents the graph-edit op shapes shared by create_workflow / edit_workflow.
//
// opsDoc 记录 create_workflow / edit_workflow 共用的图编辑 op 形状。
const opsDoc = `OP SHAPES (each has an "op" discriminator):
  {"op":"set_meta", "name":"snake_case", "description":"one line", "tags":["..."], "concurrency":"serial|skip|buffer_one|replace|allow_all"}
      // concurrency = overlap policy when a fire arrives while a run is in flight: serial (queue the new one, run after the current), skip (drop the new one), buffer_one (queue but keep only the LATEST — older waiting fires are superseded), replace (gracefully cancel the in-flight run and run the new one instead), allow_all (run concurrently). Default serial.
  {"op":"add_node", "node":{"id":"<graphLocalId>", "kind":"trigger|action|agent|control|approval", "ref":"<entityRef>", "input":{"<field>":"<bareCEL>"}}}
  {"op":"update_node", "id":"<nodeId>", "patch":{...partial node fields, merged...}}
  {"op":"delete_node", "id":"<nodeId>"}   // cascades: its edges are removed too
  {"op":"add_edge", "edge":{"id":"<edgeId>", "from":"<nodeId>", "to":"<nodeId>", "fromPort":"<branch>"}}
  {"op":"update_edge", "id":"<edgeId>", "patch":{...}}
  {"op":"delete_edge", "id":"<edgeId>"}

NODE KINDS & REF PREFIXES: trigger→trg_, action→fn_ | hd_<id>.method | mcp:server/tool, agent→ag_, control→ctl_, approval→apf_.
A node's "input" wires each field to a bare CEL expression that reads UPSTREAM NODES' RESULTS BY NODE ID — "<upstreamNodeId>.<field>", e.g. "start.amount" or "check_amount.score". There is NO payload/ctx/input root in a node's input CEL; address the producing node directly. A trigger node has no input. A referenced field must be present on EVERY branch path that can reach this node — a key absent on a taken branch (e.g. an upstream that emits it only on success) fails the WHOLE run fail-fast, and capability_check does NOT catch it.
NODE RESULT SHAPES — what "<nodeId>.<field>" can read from each kind:
  • trigger  → the fire payload's fields (e.g. start.amount).
  • action   → a function's declared outputs / a handler method's return / an mcp tool's result.
  • control  → the chosen branch's emit fields (flattened) plus "__port" (the branch name taken).
  • approval → {decision: "yes"|"no", reason} ONLY — an approval does NOT pass its input through. To use the original data downstream (e.g. the amount), read it from an upstream node like "start.amount", NOT from the approval node.
  • agent    → if it declared "outputs", those structured fields; if "outputs" is empty (a free-form answer), a single field "text" — read as <nodeId>.text.
  ↳ SCHEMA-LESS result (ANY callable kind): a function / handler / mcp tool / agent that returns a bare string|number|array — NOT an object — lands under ONE field "text"; read it as <nodeId>.text (e.g. a schema-less summarizer agent → summarize.text, an mcp text tool → echo.text). capability_check cannot see this key (no declared schema), so wire it from this doc — not by trial-and-error on a failed run.
fromPort is required on an edge leaving a control node (a branch name) or an approval node (yes|no), and must be absent otherwise.
The graph must have ≥1 trigger, no orphan nodes, and any loop must be closed by a control or approval branch (a back edge).
LOOP STATE: a back edge re-runs the downstream nodes each iteration; a node reads its loop-internal predecessor's result, which on iteration N≥1 is the PRIOR turn's value. On the FIRST turn that predecessor has not run yet, so to carry/accumulate state you MUST guard the read: "has(loopNode.field) ? loopNode.field : seedNode.field" — initialise from a pre-loop node on turn 1, then accumulate from the loop node after. Carry the state forward on the back edge via the control's emit (e.g. a control whose emit is {count: input.count + 1}, with a downstream node reading "has(theControl.count) ? theControl.count : start.count"). A bare unguarded read of a not-yet-run loop node fails on turn 1; guard it with has().`

// Package workflow provides the LLM system tools for the user's workflow library. Two groups:
// the FORGE/QUERY tools that edit the graph — search / get / create / edit / revert / delete /
// capability_check (forge.go, query.go); and the EXECUTION-LIFECYCLE tools that drive its runtime —
// trigger / stage / activate / deactivate / kill (exec.go, D1, over the durable scheduler + trigger
// binder). All are lazy tools (Toolset.Lazy) — surfaced via search_tools, not resident.
//
// Package workflow 提供操作用户 workflow 库的 LLM system tool。两组：编辑图的 FORGE/QUERY 工具——
// search / get / create / edit / revert / delete / capability_check（forge.go, query.go）；驱动其运行时的
// 执行生命周期工具——trigger / stage / activate / deactivate / kill（exec.go，D1，基于 durable 调度器 +
// trigger binder）。全是懒加载工具（Toolset.Lazy）——经 search_tools 浮现、非常驻。
package workflow

import (
	"encoding/json"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
)

// WorkflowTools constructs the workflow system tools over the app service.
//
// WorkflowTools 基于 app service 构造 workflow system tool。
func WorkflowTools(svc *workflowapp.Service) []toolapp.Tool {
	return []toolapp.Tool{
		&SearchWorkflow{svc: svc},
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
	}
}

// opsDoc documents the graph-edit op shapes shared by create_workflow / edit_workflow.
//
// opsDoc 记录 create_workflow / edit_workflow 共用的图编辑 op 形状。
const opsDoc = `OP SHAPES (each has an "op" discriminator):
  {"op":"set_meta", "name":"snake_case", "description":"one line", "tags":["..."]}
  {"op":"add_node", "node":{"id":"<graphLocalId>", "kind":"trigger|action|agent|control|approval", "ref":"<entityRef>", "input":{"<field>":"<bareCEL>"}}}
  {"op":"update_node", "id":"<nodeId>", "patch":{...partial node fields, merged...}}
  {"op":"delete_node", "id":"<nodeId>"}   // cascades: its edges are removed too
  {"op":"add_edge", "edge":{"id":"<edgeId>", "from":"<nodeId>", "to":"<nodeId>", "fromPort":"<branch>"}}
  {"op":"update_edge", "id":"<edgeId>", "patch":{...}}
  {"op":"delete_edge", "id":"<edgeId>"}

NODE KINDS & REF PREFIXES: trigger→trg_, action→fn_ | hd_<id>.method | mcp:server/tool, agent→ag_, control→ctl_, approval→apf_.
A node's "input" wires each field to a bare CEL expression over upstream results (payload/ctx for a trigger's signal, input for node-fed data). A trigger node has no input.
fromPort is required on an edge leaving a control node (a branch name) or an approval node (yes|no), and must be absent otherwise.
The graph must have ≥1 trigger, no orphan nodes, and any loop must be closed by a control or approval branch (a back edge).`

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	forgepkg "github.com/sunweilin/forgify/backend/internal/pkg/forge"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

type CreateWorkflow struct {
	svc   *workflowapp.Service
	forge forgepkg.Publisher
}

func (t *CreateWorkflow) Name() string { return "create_workflow" }

func (t *CreateWorkflow) Description() string {
	return `Create a new workflow by applying a sequence of ops. V1 auto-accepts the created workflow as v1.

MINIMAL COMPLETE EXAMPLE — manual trigger → one function → done:
  ops = [
    {"op":"set_meta", "name":"daily_report", "description":"Generate and email"},
    {"op":"add_node", "node":{"id":"t1", "type":"trigger", "config":{"kind":"manual"}}},
    {"op":"add_node", "node":{"id":"f1", "type":"function", "config":{"functionId":"fn_xxx"}}},
    {"op":"add_edge", "edge":{"from":"t1", "to":"f1"}}
  ]
DAG terminates implicitly after f1 — DO NOT add an "end" / "output" / "finish"
node, those types do not exist. trigger node has no incoming edges; any node
with no outgoing edge is a leaf.

OP CHEATSHEET (write each op as a JSON object with the listed shape):

  {"op":"set_meta", "name":"...", "description":"...", "tags":[...]}
  {"op":"add_node", "node":{"id":"n1", "type":"trigger|function|handler|mcp|skill|llm|http|condition|loop|parallel|approval|wait|variable", "config":{...}}}
  {"op":"add_edge", "edge":{"from":"<sourceNodeId>", "to":"<targetNodeId>", "fromPort":"<port>"}}
  {"op":"set_variable", "variable":{"name":"...", "type":"...", "default":...}}

WORKFLOW GRAPH RULES:
  - Need at least one trigger node. Workflow ENDS when the DAG runs out of edges (no "end" node type — don't add one).
  - Plain capability nodes (function/handler/mcp/skill/llm/http) reference existing entities via config.functionId / handlerName / serverName / skillName.
  - Edges connect node IDs directly (no dots, no port-in-id).

BRANCHING NODES (require fromPort on outgoing edges):
  - approval node → fromPort must be "approved" or "rejected"
  - loop node    → fromPort must be "iterate" or "done"
  - condition node → fromPort must be one of the case names declared in the node's config.cases

  Example approval routing:
    {"op":"add_node", "node":{"id":"a1", "type":"approval", "config":{"prompt":"OK to proceed?"}}}
    {"op":"add_edge", "edge":{"from":"a1", "to":"on_ok",     "fromPort":"approved"}}
    {"op":"add_edge", "edge":{"from":"a1", "to":"on_cancel", "fromPort":"rejected"}}

SINGLE-OUTPUT NODES (trigger / function / handler / mcp / skill / llm / http / wait / variable / parallel):
  - fromPort must be empty/omitted on their outgoing edges.

The schema rejects mismatches at create time — if your edge violates these rules you'll get WORKFLOW_OP_INVALID with the specific reason.`
}

func (t *CreateWorkflow) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"ops": {
				"type": "array",
				"description": "Sequence of ops. Each op is one of: set_meta / add_node / add_edge / set_variable / update_node / update_edge / delete_node / delete_edge / unset_variable. See tool description for exact shapes.",
				"items": {"type": "object"}
			},
			"changeReason": {"type": "string", "description": "One-line reason for this workflow creation"}
		},
		"required": ["ops"]
	}`)
}

func (t *CreateWorkflow) IsReadOnly() bool        { return false }
func (t *CreateWorkflow) NeedsReadFirst() bool    { return false }
func (t *CreateWorkflow) RequiresWorkspace() bool { return false }

func (t *CreateWorkflow) ValidateInput(json.RawMessage) error { return nil }
func (t *CreateWorkflow) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *CreateWorkflow) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Ops          json.RawMessage `json:"ops"`
		ChangeReason string          `json:"changeReason"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("create_workflow: bad args: %w", err)
	}
	ops, err := workflowapp.ParseOps(args.Ops)
	if err != nil {
		return "", fmt.Errorf("create_workflow: %w", err)
	}

	em := eventlogpkg.From(ctx)
	progID := em.StartBlock(ctx, eventlogdomain.BlockTypeProgress, map[string]any{
		"stage": "applying ops",
		"count": len(ops),
	})
	defer em.StopBlock(ctx, progID, eventlogdomain.StatusCompleted, nil)

	w, v, err := t.svc.Create(ctx, workflowapp.CreateInput{
		Ops:             ops,
		ChangeReason:    args.ChangeReason,
		ProgressBlockID: progID,
	})
	if err != nil {
		em.StopBlock(ctx, progID, eventlogdomain.StatusError, err)
		// We don't know the workflow ID (Create failed before persistence),
		// so we can't publish forge_started/completed for this failure path.
		// Caller sees the wrapped err via the tool_result.
		// Create 失败前无 workflow ID,无法发 forge 事件,err 经 tool_result 抛。
		return "", fmt.Errorf("create_workflow: %w", err)
	}

	scope := eventlogdomain.Scope{Kind: eventlogdomain.KindWorkflow, ID: w.ID}
	convID, _ := reqctxpkg.GetConversationID(ctx)
	toolCallID, _ := reqctxpkg.GetToolCallID(ctx)
	t.forge.PublishStarted(ctx, scope, forgedomain.OperationCreate, convID, toolCallID)
	t.forge.PublishCompleted(ctx, scope, forgedomain.CompletedOK, v.ID, "", 1, nil)

	versionN := 1
	if v.Version != nil {
		versionN = *v.Version
	}
	out := map[string]any{
		"id":         w.ID,
		"name":       w.Name,
		"versionId":  v.ID,
		"version":    versionN,
		"status":     v.Status,
		"opsApplied": len(ops),
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

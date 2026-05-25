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

type EditWorkflow struct {
	svc   *workflowapp.Service
	forge forgepkg.Publisher
}

func (t *EditWorkflow) Name() string { return "edit_workflow" }

func (t *EditWorkflow) Description() string {
	return `Edit a workflow by applying ops. Repeated edits while a pending exists rewrite the same pending row (iterate-same-pending — no conflict error). User must accept or reject the pending result.

add_node / add_edge / set_meta / set_variable: same shapes as create_workflow (see that tool for node types, branching/port rules, and loop body format).

EDIT-ONLY OPS:
  {"op":"update_node", "nodeId":"<nodeId>", "patch":{...}}  // RFC 7396 JSON Merge Patch
  {"op":"delete_node", "nodeId":"<nodeId>"}                 // cascades incident edges
  {"op":"update_edge", "edgeId":"<edgeId>", "patch":{...}}
  {"op":"delete_edge", "edgeId":"<edgeId>"}
  {"op":"unset_variable", "name":"..."}

Schema validates after every batch — violations return WORKFLOW_OP_INVALID with the specific reason.`
}

func (t *EditWorkflow) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string"},
			"ops": {
				"type": "array",
				"description": "Sequence of ops to apply on top of current pending (or active if no pending)",
				"items": {"type": "object"}
			},
			"changeReason": {"type": "string", "description": "One-line reason"}
		},
		"required": ["id", "ops"]
	}`)
}

func (t *EditWorkflow) IsReadOnly() bool        { return false }
func (t *EditWorkflow) NeedsReadFirst() bool    { return false }
func (t *EditWorkflow) RequiresWorkspace() bool { return false }

func (t *EditWorkflow) ValidateInput(json.RawMessage) error { return nil }
func (t *EditWorkflow) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *EditWorkflow) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ID           string          `json:"id"`
		Ops          json.RawMessage `json:"ops"`
		ChangeReason string          `json:"changeReason"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("edit_workflow: bad args: %w", err)
	}
	if args.ID == "" {
		return "", fmt.Errorf("edit_workflow: id required")
	}
	ops, err := workflowapp.ParseOps(args.Ops)
	if err != nil {
		return "", fmt.Errorf("edit_workflow: %w", err)
	}

	em := eventlogpkg.From(ctx)
	progID := em.StartBlock(ctx, eventlogdomain.BlockTypeProgress, map[string]any{
		"stage":      "applying ops",
		"count":      len(ops),
		"workflowId": args.ID,
	})
	defer em.StopBlock(ctx, progID, eventlogdomain.StatusCompleted, nil)

	scope := eventlogdomain.Scope{Kind: eventlogdomain.KindWorkflow, ID: args.ID}
	convID, _ := reqctxpkg.GetConversationID(ctx)
	toolCallID, _ := reqctxpkg.GetToolCallID(ctx)
	t.forge.PublishStarted(ctx, scope, forgedomain.OperationEdit, convID, toolCallID)

	v, err := t.svc.Edit(ctx, workflowapp.EditInput{
		ID:              args.ID,
		Ops:             ops,
		ChangeReason:    args.ChangeReason,
		ProgressBlockID: progID,
	})
	if err != nil {
		em.StopBlock(ctx, progID, eventlogdomain.StatusError, err)
		t.forge.PublishCompleted(ctx, scope, forgedomain.CompletedFailed, "", "", 0, err)
		return "", fmt.Errorf("edit_workflow: %w", err)
	}
	t.forge.PublishCompleted(ctx, scope, forgedomain.CompletedOK, v.ID, "", 1, nil)

	out := map[string]any{
		"pendingId":  v.ID,
		"opsApplied": len(ops),
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	forgepkg "github.com/sunweilin/forgify/backend/internal/pkg/forge"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

type DeleteWorkflow struct {
	svc   *workflowapp.Service
	forge forgepkg.Publisher
}

func (t *DeleteWorkflow) Name() string { return "delete_workflow" }

func (t *DeleteWorkflow) Description() string {
	return "Soft-delete a workflow. The historical versions remain in DB " +
		"but the workflow disappears from listings + LLM search. Triggers " +
		"that referenced it stop firing."
}

func (t *DeleteWorkflow) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string", "description": "Workflow ID (wf_xxx)"}
		},
		"required": ["id"]
	}`)
}

func (t *DeleteWorkflow) IsReadOnly() bool        { return false }
func (t *DeleteWorkflow) NeedsReadFirst() bool    { return false }
func (t *DeleteWorkflow) RequiresWorkspace() bool { return false }

func (t *DeleteWorkflow) ValidateInput(json.RawMessage) error { return nil }
func (t *DeleteWorkflow) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *DeleteWorkflow) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("delete_workflow: bad args: %w", err)
	}
	if args.ID == "" {
		return "", fmt.Errorf("delete_workflow: id required")
	}

	scope := eventlogdomain.Scope{Kind: eventlogdomain.KindWorkflow, ID: args.ID}
	convID, _ := reqctxpkg.GetConversationID(ctx)
	toolCallID, _ := reqctxpkg.GetToolCallID(ctx)
	t.forge.PublishStarted(ctx, scope, forgedomain.OperationDelete, convID, toolCallID)

	if err := t.svc.Delete(ctx, args.ID); err != nil {
		t.forge.PublishCompleted(ctx, scope, forgedomain.CompletedFailed, "", "", 0, err)
		return "", fmt.Errorf("delete_workflow: %w", err)
	}
	t.forge.PublishCompleted(ctx, scope, forgedomain.CompletedOK, "", "", 0, nil)

	out := map[string]any{"deleted": true, "id": args.ID}
	b, _ := json.Marshal(out)
	return string(b), nil
}

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

type RevertWorkflow struct {
	svc   *workflowapp.Service
	forge forgepkg.Publisher
}

func (t *RevertWorkflow) Name() string { return "revert_workflow" }

func (t *RevertWorkflow) Description() string {
	return "Revert a workflow's active version to a previously accepted " +
		"version number. Soft action — the historical version remains in " +
		"the version list; revert just flips the active pointer."
}

func (t *RevertWorkflow) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string", "description": "Workflow ID (wf_xxx)"},
			"targetVersion": {"type": "integer", "description": "Accepted version number to flip active to"}
		},
		"required": ["id", "targetVersion"]
	}`)
}

func (t *RevertWorkflow) IsReadOnly() bool        { return false }
func (t *RevertWorkflow) NeedsReadFirst() bool    { return false }
func (t *RevertWorkflow) RequiresWorkspace() bool { return false }

func (t *RevertWorkflow) ValidateInput(json.RawMessage) error { return nil }
func (t *RevertWorkflow) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *RevertWorkflow) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ID            string `json:"id"`
		TargetVersion int    `json:"targetVersion"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("revert_workflow: bad args: %w", err)
	}
	if args.ID == "" {
		return "", fmt.Errorf("revert_workflow: id required")
	}
	if args.TargetVersion <= 0 {
		return "", fmt.Errorf("revert_workflow: targetVersion must be >= 1")
	}

	scope := eventlogdomain.Scope{Kind: eventlogdomain.KindWorkflow, ID: args.ID}
	convID, _ := reqctxpkg.GetConversationID(ctx)
	toolCallID, _ := reqctxpkg.GetToolCallID(ctx)
	t.forge.PublishStarted(ctx, scope, forgedomain.OperationRevert, convID, toolCallID)

	v, err := t.svc.Revert(ctx, args.ID, args.TargetVersion)
	if err != nil {
		t.forge.PublishCompleted(ctx, scope, forgedomain.CompletedFailed, "", "", 0, err)
		return "", fmt.Errorf("revert_workflow: %w", err)
	}
	versionN := args.TargetVersion
	if v.Version != nil {
		versionN = *v.Version
	}
	t.forge.PublishCompleted(ctx, scope, forgedomain.CompletedOK, v.ID, "", 1, nil)

	out := map[string]any{
		"activeVersionId": v.ID,
		"version":         versionN,
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

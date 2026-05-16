package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

type GetWorkflow struct {
	svc *workflowapp.Service
}

func (t *GetWorkflow) Name() string { return "get_workflow" }

func (t *GetWorkflow) Description() string {
	return "Get full details of a workflow including the parsed graph " +
		"(nodes / edges / variables) of the active version and the pending " +
		"version if one exists. Use before edit_workflow to inspect the " +
		"current shape."
}

func (t *GetWorkflow) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string", "description": "Workflow ID (wf_xxx)"}
		},
		"required": ["id"]
	}`)
}

func (t *GetWorkflow) IsReadOnly() bool        { return true }
func (t *GetWorkflow) NeedsReadFirst() bool    { return false }
func (t *GetWorkflow) RequiresWorkspace() bool { return false }

func (t *GetWorkflow) ValidateInput(json.RawMessage) error { return nil }
func (t *GetWorkflow) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *GetWorkflow) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("get_workflow: bad args: %w", err)
	}
	if args.ID == "" {
		return "", fmt.Errorf("get_workflow: id required")
	}
	w, err := t.svc.Get(ctx, args.ID)
	if err != nil {
		return "", fmt.Errorf("get_workflow: %w", err)
	}
	var active map[string]any
	if w.ActiveVersionID != "" {
		v, err := t.svc.GetVersion(ctx, w.ActiveVersionID)
		if err == nil && v != nil {
			active = map[string]any{
				"id":           v.ID,
				"version":      v.Version,
				"graph":        v.GraphParsed,
				"changeReason": v.ChangeReason,
			}
		}
	}
	out := map[string]any{
		"workflow":      w,
		"activeVersion": active,
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

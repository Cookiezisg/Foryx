package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
)

type GetWorkflowExecution struct {
	repo flowrundomain.Repository
}

func (t *GetWorkflowExecution) Name() string { return "get_workflow_execution" }

func (t *GetWorkflowExecution) Description() string {
	return "Get one workflow node execution by id, with full input/output JSON, error, timing, and attempts."
}

func (t *GetWorkflowExecution) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {"id": {"type": "string"}},
		"required": ["id"]
	}`)
}

func (t *GetWorkflowExecution) IsReadOnly() bool        { return true }
func (t *GetWorkflowExecution) NeedsReadFirst() bool    { return false }
func (t *GetWorkflowExecution) RequiresWorkspace() bool { return false }
func (t *GetWorkflowExecution) ValidateInput(json.RawMessage) error { return nil }
func (t *GetWorkflowExecution) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *GetWorkflowExecution) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct{ ID string `json:"id"` }
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("get_workflow_execution: bad args: %w", err)
	}
	if args.ID == "" {
		return "", fmt.Errorf("get_workflow_execution: id required")
	}
	node, err := t.repo.GetNode(ctx, args.ID)
	if err != nil {
		return "", fmt.Errorf("get_workflow_execution: %w", err)
	}
	_ = flowrundomain.NodeStatusOK // keep import explicit
	b, _ := json.Marshal(node)
	return string(b), nil
}

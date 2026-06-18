package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	workflowapp "github.com/sunweilin/anselm/backend/internal/app/workflow"
)

// --- capability_check_workflow ---------------------------------------------

type CapabilityCheckWorkflow struct{ svc *workflowapp.Service }

func (t *CapabilityCheckWorkflow) Name() string { return "capability_check_workflow" }

func (t *CapabilityCheckWorkflow) Description() string {
	return "Validate a workflow's active graph: structural soundness plus, when the capability catalog is wired, whether every referenced entity (trigger / function / handler / mcp / agent / control / approval) exists, has an active version, and exposes the ports/methods the graph uses. Returns a report listing any problems — use it before activating a workflow. It does NOT validate node-input DATAFLOW: a node's input CEL may read an upstream field that isn't emitted on every branch reaching it (or a schema-less node's runtime-only `.text` key), and a missing key fails the WHOLE run fail-fast — so a green report still needs one trigger_workflow to confirm the data wiring."
}

func (t *CapabilityCheckWorkflow) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["workflowId"],
		"properties": {"workflowId": {"type": "string"}}
	}`)
}

func (t *CapabilityCheckWorkflow) ValidateInput(args json.RawMessage) error {
	var a struct {
		WorkflowID string `json:"workflowId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("capability_check_workflow: bad args: %w", err)
	}
	if a.WorkflowID == "" {
		return ErrWorkflowIDRequired
	}
	return nil
}

func (t *CapabilityCheckWorkflow) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		WorkflowID string `json:"workflowId"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("capability_check_workflow: bad args: %w", err)
	}
	rep, err := t.svc.CapabilityCheckByID(ctx, args.WorkflowID)
	if err != nil {
		return "", fmt.Errorf("capability_check_workflow: %w", err)
	}
	return toolapp.ToJSON(map[string]any{
		"id":                args.WorkflowID,
		"ok":                rep.OK(),
		"structurallyValid": rep.StructurallyValid,
		"resolved":          rep.Resolved,
		"problems":          rep.Problems,
	}), nil
}

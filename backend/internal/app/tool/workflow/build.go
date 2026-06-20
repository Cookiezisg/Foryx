package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	workflowapp "github.com/sunweilin/anselm/backend/internal/app/workflow"
	relationdomain "github.com/sunweilin/anselm/backend/internal/domain/relation"
	workflowdomain "github.com/sunweilin/anselm/backend/internal/domain/workflow"
)

// --- create_workflow -------------------------------------------------------

type CreateWorkflow struct{ svc *workflowapp.Service }

func (t *CreateWorkflow) Name() string { return "create_workflow" }

func (t *CreateWorkflow) Description() string {
	return "Build a new workflow graph from ops; v1 takes effect immediately (no separate accept step). The new workflow starts deactivated — activate it once its graph is sound. Provide the name and an ops array that builds at least a trigger node.\n\n" + opsDoc
}

func (t *CreateWorkflow) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["name", "ops"],
		"properties": {
			"name": {"type": "string", "description": "Unique workflow name."},
			"description": {"type": "string"},
			"tags": {"type": "array", "items": {"type": "string"}},
			"ops": {"type": "array", "description": "Graph-edit ops; each has an 'op' discriminator.", "items": {"type": "object"}},
			"changeReason": {"type": "string", "description": "One-line reason for this creation."}
		}
	}`)
}

func (t *CreateWorkflow) ValidateInput(args json.RawMessage) error {
	var a struct {
		Name string            `json:"name"`
		Ops  []json.RawMessage `json:"ops"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("create_workflow: bad args: %w", err)
	}
	if a.Name == "" {
		return ErrNameRequired
	}
	if len(a.Ops) == 0 {
		return ErrOpsRequired
	}
	return nil
}

func (t *CreateWorkflow) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Name         string          `json:"name"`
		Description  string          `json:"description"`
		Tags         []string        `json:"tags"`
		Ops          json.RawMessage `json:"ops"`
		ChangeReason string          `json:"changeReason"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("create_workflow: bad args: %w", err)
	}
	ops, err := workflowdomain.ParseOps(args.Ops)
	if err != nil {
		return "", fmt.Errorf("create_workflow: %w", err)
	}
	w, v, err := t.svc.Create(ctx, workflowapp.CreateInput{
		Name: args.Name, Description: args.Description, Tags: args.Tags, Ops: ops, ChangeReason: args.ChangeReason,
	})
	if err != nil {
		return "", fmt.Errorf("create_workflow: %w", err)
	}
	return toolapp.ToJSON(map[string]any{"id": w.ID, "versionId": v.ID, "version": v.Version, "active": w.Active, "lifecycleState": w.LifecycleState}), nil
}

// --- edit_workflow ---------------------------------------------------------

type EditWorkflow struct{ svc *workflowapp.Service }

func (t *EditWorkflow) Name() string { return "edit_workflow" }

func (t *EditWorkflow) Description() string {
	return "Edit a workflow: apply ops on top of its active graph, producing a new version that takes effect immediately. Same op shapes as create_workflow. The ops array must be non-empty. Use revert_workflow to switch the active version to an older one.\n\n" + opsDoc
}

func (t *EditWorkflow) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["workflowId", "ops"],
		"properties": {
			"workflowId": {"type": "string"},
			"ops": {"type": "array", "description": "Graph-edit ops (non-empty).", "items": {"type": "object"}},
			"changeReason": {"type": "string", "description": "One-line reason for this edit."}
		}
	}`)
}

func (t *EditWorkflow) ValidateInput(args json.RawMessage) error {
	var a struct {
		WorkflowID string            `json:"workflowId"`
		Ops        []json.RawMessage `json:"ops"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("edit_workflow: bad args: %w", err)
	}
	if a.WorkflowID == "" {
		return ErrWorkflowIDRequired
	}
	if len(a.Ops) == 0 {
		return ErrOpsRequired
	}
	return nil
}

func (t *EditWorkflow) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		WorkflowID   string          `json:"workflowId"`
		Ops          json.RawMessage `json:"ops"`
		ChangeReason string          `json:"changeReason"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("edit_workflow: bad args: %w", err)
	}
	ops, err := workflowdomain.ParseOps(args.Ops)
	if err != nil {
		return "", fmt.Errorf("edit_workflow: %w", err)
	}
	v, err := t.svc.Edit(ctx, workflowapp.EditInput{ID: args.WorkflowID, Ops: ops, ChangeReason: args.ChangeReason})
	if err != nil {
		return "", fmt.Errorf("edit_workflow: %w", err)
	}
	return toolapp.ToJSON(map[string]any{"id": args.WorkflowID, "versionId": v.ID, "version": v.Version}), nil
}

// --- revert_workflow -------------------------------------------------------

type RevertWorkflow struct{ svc *workflowapp.Service }

func (t *RevertWorkflow) Name() string { return "revert_workflow" }

func (t *RevertWorkflow) Description() string {
	return "Switch a workflow's active graph version to an existing version by its number. This only moves the active pointer — newer versions are kept in history and can be switched back to."
}

func (t *RevertWorkflow) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["workflowId", "version"],
		"properties": {
			"workflowId": {"type": "string"},
			"version": {"type": "integer", "description": "The version number to make active."}
		}
	}`)
}

func (t *RevertWorkflow) ValidateInput(args json.RawMessage) error {
	var a struct {
		WorkflowID string `json:"workflowId"`
		Version    int    `json:"version"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("revert_workflow: bad args: %w", err)
	}
	if a.WorkflowID == "" {
		return ErrWorkflowIDRequired
	}
	if a.Version <= 0 {
		return ErrVersionPositive
	}
	return nil
}

func (t *RevertWorkflow) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		WorkflowID string `json:"workflowId"`
		Version    int    `json:"version"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("revert_workflow: bad args: %w", err)
	}
	v, err := t.svc.Revert(ctx, args.WorkflowID, args.Version)
	if err != nil {
		return "", fmt.Errorf("revert_workflow: %w", err)
	}
	return toolapp.ToJSON(map[string]any{"id": args.WorkflowID, "activeVersionId": v.ID, "version": v.Version}), nil
}

// --- delete_workflow -------------------------------------------------------

type DeleteWorkflow struct {
	svc  *workflowapp.Service
	deps toolapp.DependentCounter
}

func (t *DeleteWorkflow) Name() string { return "delete_workflow" }

func (t *DeleteWorkflow) Description() string {
	return "Delete a workflow and all its graph versions. This is not reversible. The result reports how many other entities referenced it (and may now fail) — to check dependents BEFORE deleting, use get_relations."
}

func (t *DeleteWorkflow) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["workflowId"],
		"properties": {"workflowId": {"type": "string"}}
	}`)
}

func (t *DeleteWorkflow) ValidateInput(args json.RawMessage) error {
	var a struct {
		WorkflowID string `json:"workflowId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("delete_workflow: bad args: %w", err)
	}
	if a.WorkflowID == "" {
		return ErrWorkflowIDRequired
	}
	return nil
}

func (t *DeleteWorkflow) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		WorkflowID string `json:"workflowId"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("delete_workflow: bad args: %w", err)
	}
	deps := toolapp.DependentCount(ctx, t.deps, relationdomain.EntityKindWorkflow, args.WorkflowID)
	if err := t.svc.Delete(ctx, args.WorkflowID); err != nil {
		return "", fmt.Errorf("delete_workflow: %w", err)
	}
	return toolapp.ToJSON(toolapp.AnnotateDependents(map[string]any{"id": args.WorkflowID, "deleted": true}, deps)), nil
}

package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	approvalapp "github.com/sunweilin/anselm/backend/internal/app/approval"
	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	relationdomain "github.com/sunweilin/anselm/backend/internal/domain/relation"
	schemapkg "github.com/sunweilin/anselm/backend/internal/pkg/schema"
)

// --- create_approval -------------------------------------------------------

type CreateApproval struct{ svc *approvalapp.Service }

func (t *CreateApproval) Name() string { return "create_approval" }

func (t *CreateApproval) Description() string {
	return "Create an approval-form entity that a workflow approval node references: a markdown prompt `template` (with `{{ input.* }}` interpolation over the inputs the workflow node feeds, e.g. `批准对 {{ input.user }} 的退款 {{ input.amount }} 元?`) which renders into a human-readable decision point, plus decision rules. `template` is REQUIRED — a button with no explanation is meaningless. `allowReason` toggles an optional free-text note. `timeout` (a duration like `30d` / `2h`; empty = never times out) and `timeoutBehavior` (reject|approve|fail; required when timeout is set) govern what happens if nobody responds. The node has fixed yes/no exits the graph wires to downstream nodes. Its downstream result is {decision: \"yes\"|\"no\", reason} ONLY — an approval does NOT pass its input through, so a downstream node needing the original data (e.g. the amount) must read it from an upstream node, not from the approval node."
}

func (t *CreateApproval) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["name", "template"],
		"properties": {
			"name": {"type": "string", "description": "Unique name within the workspace."},
			"description": {"type": "string", "description": "One line on what this approval decides."},
			"inputs": {"type": "array", "description": "Declared inputs the workflow node feeds (template reads input.*): each {name, type, description}.", "items": {"type": "object"}},
			"template": {"type": "string", "description": "Markdown prompt with {{ input.* }} interpolation; shown to the user so they know what they're approving."},
			"allowReason": {"type": "boolean", "description": "Allow an optional free-text note when deciding."},
			"timeout": {"type": "string", "description": "Duration like 30d / 2h; empty = never times out."},
			"timeoutBehavior": {"type": "string", "enum": ["reject", "approve", "fail"], "description": "What happens on timeout; required when timeout is set."},
			"changeReason": {"type": "string"}
		}
	}`)
}

func (t *CreateApproval) ValidateInput(args json.RawMessage) error {
	var a struct {
		Name     string `json:"name"`
		Template string `json:"template"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("create_approval: bad args: %w", err)
	}
	if strings.TrimSpace(a.Name) == "" {
		return ErrNameRequired
	}
	if strings.TrimSpace(a.Template) == "" {
		return ErrTemplateRequired
	}
	return nil
}

func (t *CreateApproval) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Name            string            `json:"name"`
		Description     string            `json:"description"`
		Inputs          []schemapkg.Field `json:"inputs"`
		Template        string            `json:"template"`
		AllowReason     bool              `json:"allowReason"`
		Timeout         string            `json:"timeout"`
		TimeoutBehavior string            `json:"timeoutBehavior"`
		ChangeReason    string            `json:"changeReason"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("create_approval: bad args: %w", err)
	}
	f, v, err := t.svc.Create(ctx, approvalapp.CreateInput{
		Name: args.Name, Description: args.Description, Inputs: args.Inputs, Template: args.Template,
		AllowReason: args.AllowReason, Timeout: args.Timeout, TimeoutBehavior: args.TimeoutBehavior,
		ChangeReason: args.ChangeReason,
	})
	if err != nil {
		return "", fmt.Errorf("create_approval: %w", err)
	}
	return toolapp.ToJSON(map[string]any{"id": f.ID, "name": f.Name, "activeVersionId": v.ID, "version": v.Version}), nil
}

// --- edit_approval ---------------------------------------------------------

type EditApproval struct{ svc *approvalapp.Service }

func (t *EditApproval) Name() string { return "edit_approval" }

func (t *EditApproval) Description() string {
	return "Replace an approval form's template + decision rules with a new set, writing a new version that takes effect immediately (revert can switch back). Pass the COMPLETE form (template + rules), not a delta — same shape and rules as create_approval."
}

func (t *EditApproval) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["approvalId", "template"],
		"properties": {
			"approvalId": {"type": "string"},
			"inputs": {"type": "array", "description": "Declared inputs (template reads input.*): each {name, type, description}.", "items": {"type": "object"}},
			"template": {"type": "string", "description": "Markdown prompt with {{ input.* }} interpolation."},
			"allowReason": {"type": "boolean"},
			"timeout": {"type": "string", "description": "Duration like 30d / 2h; empty = never."},
			"timeoutBehavior": {"type": "string", "enum": ["reject", "approve", "fail"]},
			"changeReason": {"type": "string"}
		}
	}`)
}

func (t *EditApproval) ValidateInput(args json.RawMessage) error {
	var a struct {
		ApprovalID string `json:"approvalId"`
		Template   string `json:"template"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("edit_approval: bad args: %w", err)
	}
	if a.ApprovalID == "" {
		return ErrApprovalIDRequired
	}
	if strings.TrimSpace(a.Template) == "" {
		return ErrTemplateRequired
	}
	return nil
}

func (t *EditApproval) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ApprovalID      string            `json:"approvalId"`
		Inputs          []schemapkg.Field `json:"inputs"`
		Template        string            `json:"template"`
		AllowReason     bool              `json:"allowReason"`
		Timeout         string            `json:"timeout"`
		TimeoutBehavior string            `json:"timeoutBehavior"`
		ChangeReason    string            `json:"changeReason"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("edit_approval: bad args: %w", err)
	}
	v, err := t.svc.Edit(ctx, approvalapp.EditInput{
		ID: args.ApprovalID, Inputs: args.Inputs, Template: args.Template, AllowReason: args.AllowReason,
		Timeout: args.Timeout, TimeoutBehavior: args.TimeoutBehavior, ChangeReason: args.ChangeReason,
	})
	if err != nil {
		return "", fmt.Errorf("edit_approval: %w", err)
	}
	return toolapp.ToJSON(map[string]any{"id": args.ApprovalID, "activeVersionId": v.ID, "version": v.Version}), nil
}

// --- revert_approval -------------------------------------------------------

type RevertApproval struct{ svc *approvalapp.Service }

func (t *RevertApproval) Name() string { return "revert_approval" }

func (t *RevertApproval) Description() string {
	return "Switch an approval form's active version to an existing version by its number. Only moves the active pointer — newer versions are kept in history and can be switched back to. Note: name, description and tags are NOT versioned (they live on the approval), so a revert restores only the versioned snapshot (template + decision rules) and leaves name/description/tags unchanged — use edit_approval set_meta to also change those."
}

func (t *RevertApproval) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["approvalId", "version"],
		"properties": {
			"approvalId": {"type": "string"},
			"version": {"type": "integer", "description": "The version number to make active."}
		}
	}`)
}

func (t *RevertApproval) ValidateInput(args json.RawMessage) error {
	var a struct {
		ApprovalID string `json:"approvalId"`
		Version    int    `json:"version"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("revert_approval: bad args: %w", err)
	}
	if a.ApprovalID == "" {
		return ErrApprovalIDRequired
	}
	if a.Version <= 0 {
		return ErrVersionPositive
	}
	return nil
}

func (t *RevertApproval) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ApprovalID string `json:"approvalId"`
		Version    int    `json:"version"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("revert_approval: bad args: %w", err)
	}
	v, err := t.svc.Revert(ctx, args.ApprovalID, args.Version)
	if err != nil {
		return "", fmt.Errorf("revert_approval: %w", err)
	}
	return toolapp.ToJSON(map[string]any{"id": args.ApprovalID, "activeVersionId": v.ID, "version": v.Version}), nil
}

// --- delete_approval -------------------------------------------------------

type DeleteApproval struct {
	svc  *approvalapp.Service
	deps toolapp.DependentCounter
}

func (t *DeleteApproval) Name() string { return "delete_approval" }

func (t *DeleteApproval) Description() string {
	return "Delete an approval form and all its versions. Not reversible. Workflows that reference it will fail their capability check until repointed. The result reports how many entities referenced it — to check dependents BEFORE deleting, use get_relations."
}

func (t *DeleteApproval) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["approvalId"],
		"properties": {"approvalId": {"type": "string"}}
	}`)
}

func (t *DeleteApproval) ValidateInput(args json.RawMessage) error {
	var a struct {
		ApprovalID string `json:"approvalId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("delete_approval: bad args: %w", err)
	}
	if a.ApprovalID == "" {
		return ErrApprovalIDRequired
	}
	return nil
}

func (t *DeleteApproval) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ApprovalID string `json:"approvalId"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("delete_approval: bad args: %w", err)
	}
	deps := toolapp.DependentCount(ctx, t.deps, relationdomain.EntityKindApproval, args.ApprovalID)
	if err := t.svc.Delete(ctx, args.ApprovalID); err != nil {
		return "", fmt.Errorf("delete_approval: %w", err)
	}
	return toolapp.ToJSON(toolapp.AnnotateDependents(map[string]any{"id": args.ApprovalID, "deleted": true}, deps)), nil
}

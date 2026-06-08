package control

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	controlapp "github.com/sunweilin/forgify/backend/internal/app/control"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
)

// --- create_control --------------------------------------------------------

type CreateControl struct{ svc *controlapp.Service }

func (t *CreateControl) Name() string { return "create_control" }

func (t *CreateControl) Description() string {
	return "Create a control-logic entity: an ordered list of routing branches that a workflow control node references. Each branch has a `port` (the exit name the graph wires to a downstream node), a `when` (a boolean CEL guard over `payload`/`ctx`; branches are evaluated top-to-bottom and the FIRST whose when is true wins), and an optional `emit` (a field→CEL map that reshapes the downstream payload; omit to pass the payload through unchanged). The LAST branch MUST be `when: \"true\"` as the catch-all. CEL reads payload/ctx only — no side effects, no now(). A port may wire back to an upstream node to form a loop; use emit to carry loop state (e.g. attempt + 1)."
}

func (t *CreateControl) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["name", "branches"],
		"properties": {
			"name": {"type": "string", "description": "Unique name within the workspace."},
			"description": {"type": "string", "description": "One line on what this routing logic decides."},
			"inputSchema": {"type": "array", "description": "Declared inputs the workflow node feeds (when/emit read input.*): each {name, type, description}.", "items": {"type": "object"}},
			"branches": {
				"type": "array",
				"description": "Ordered branches; first true when wins; the last branch must be when:\"true\".",
				"items": {
					"type": "object",
					"required": ["port", "when"],
					"properties": {
						"port": {"type": "string", "description": "Named outcome the workflow routes on (an edge's fromPort matches it)."},
						"when": {"type": "string", "description": "Boolean CEL guard over input.*, e.g. input.score >= 0.9."},
						"emit": {"type": "object", "description": "Optional field->CEL map building this branch's output, e.g. {\"attempt\": \"input.attempt + 1\"}.", "additionalProperties": {"type": "string"}}
					}
				}
			},
			"changeReason": {"type": "string"}
		}
	}`)
}

func (t *CreateControl) ValidateInput(args json.RawMessage) error {
	var a struct {
		Name     string      `json:"name"`
		Branches []branchArg `json:"branches"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("create_control: bad args: %w", err)
	}
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("create_control: name is required")
	}
	if len(a.Branches) == 0 {
		return fmt.Errorf("create_control: at least one branch is required")
	}
	return nil
}

func (t *CreateControl) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Name         string            `json:"name"`
		Description  string            `json:"description"`
		InputSchema  []schemapkg.Field `json:"inputSchema"`
		Branches     []branchArg       `json:"branches"`
		ChangeReason string            `json:"changeReason"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("create_control: bad args: %w", err)
	}
	c, v, err := t.svc.Create(ctx, controlapp.CreateInput{
		Name: args.Name, Description: args.Description, InputSchema: args.InputSchema,
		Branches: toBranches(args.Branches), ChangeReason: args.ChangeReason,
	})
	if err != nil {
		return "", fmt.Errorf("create_control: %w", err)
	}
	return toJSON(map[string]any{"id": c.ID, "name": c.Name, "activeVersionId": v.ID, "version": v.Version}), nil
}

// --- edit_control ----------------------------------------------------------

type EditControl struct{ svc *controlapp.Service }

func (t *EditControl) Name() string { return "edit_control" }

func (t *EditControl) Description() string {
	return "Replace a control logic's branches with a new ordered set, writing a new version that takes effect immediately (revert can switch back). Pass the COMPLETE branch list (not a delta) — same branch shape and catch-all rule as create_control."
}

func (t *EditControl) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["controlId", "branches"],
		"properties": {
			"controlId": {"type": "string"},
			"inputSchema": {"type": "array", "description": "Declared inputs (when/emit read input.*): each {name, type, description}.", "items": {"type": "object"}},
			"branches": {
				"type": "array",
				"description": "The complete new ordered branch list; last must be when:\"true\".",
				"items": {
					"type": "object",
					"required": ["port", "when"],
					"properties": {
						"port": {"type": "string"},
						"when": {"type": "string"},
						"emit": {"type": "object", "additionalProperties": {"type": "string"}}
					}
				}
			},
			"changeReason": {"type": "string"}
		}
	}`)
}

func (t *EditControl) ValidateInput(args json.RawMessage) error {
	var a struct {
		ControlID string      `json:"controlId"`
		Branches  []branchArg `json:"branches"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("edit_control: bad args: %w", err)
	}
	if a.ControlID == "" {
		return fmt.Errorf("edit_control: controlId is required")
	}
	if len(a.Branches) == 0 {
		return fmt.Errorf("edit_control: branches is required (the complete new set)")
	}
	return nil
}

func (t *EditControl) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ControlID    string            `json:"controlId"`
		InputSchema  []schemapkg.Field `json:"inputSchema"`
		Branches     []branchArg       `json:"branches"`
		ChangeReason string            `json:"changeReason"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("edit_control: bad args: %w", err)
	}
	v, err := t.svc.Edit(ctx, controlapp.EditInput{
		ID: args.ControlID, InputSchema: args.InputSchema, Branches: toBranches(args.Branches), ChangeReason: args.ChangeReason,
	})
	if err != nil {
		return "", fmt.Errorf("edit_control: %w", err)
	}
	return toJSON(map[string]any{"id": args.ControlID, "activeVersionId": v.ID, "version": v.Version}), nil
}

// --- revert_control --------------------------------------------------------

type RevertControl struct{ svc *controlapp.Service }

func (t *RevertControl) Name() string { return "revert_control" }

func (t *RevertControl) Description() string {
	return "Switch a control logic's active version to an existing version by its number. This only moves the active pointer — newer versions are kept in history and can be switched back to."
}

func (t *RevertControl) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["controlId", "version"],
		"properties": {
			"controlId": {"type": "string"},
			"version": {"type": "integer", "description": "The version number to make active."}
		}
	}`)
}

func (t *RevertControl) ValidateInput(args json.RawMessage) error {
	var a struct {
		ControlID string `json:"controlId"`
		Version   int    `json:"version"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("revert_control: bad args: %w", err)
	}
	if a.ControlID == "" {
		return fmt.Errorf("revert_control: controlId is required")
	}
	if a.Version <= 0 {
		return fmt.Errorf("revert_control: version must be a positive integer")
	}
	return nil
}

func (t *RevertControl) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ControlID string `json:"controlId"`
		Version   int    `json:"version"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("revert_control: bad args: %w", err)
	}
	v, err := t.svc.Revert(ctx, args.ControlID, args.Version)
	if err != nil {
		return "", fmt.Errorf("revert_control: %w", err)
	}
	return toJSON(map[string]any{"id": args.ControlID, "activeVersionId": v.ID, "version": v.Version}), nil
}

// --- delete_control --------------------------------------------------------

type DeleteControl struct{ svc *controlapp.Service }

func (t *DeleteControl) Name() string { return "delete_control" }

func (t *DeleteControl) Description() string {
	return "Delete a control logic and all its versions. Not reversible. Workflows that reference it will fail their capability check until repointed."
}

func (t *DeleteControl) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["controlId"],
		"properties": {"controlId": {"type": "string"}}
	}`)
}

func (t *DeleteControl) ValidateInput(args json.RawMessage) error {
	var a struct {
		ControlID string `json:"controlId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("delete_control: bad args: %w", err)
	}
	if a.ControlID == "" {
		return fmt.Errorf("delete_control: controlId is required")
	}
	return nil
}

func (t *DeleteControl) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ControlID string `json:"controlId"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("delete_control: bad args: %w", err)
	}
	if err := t.svc.Delete(ctx, args.ControlID); err != nil {
		return "", fmt.Errorf("delete_control: %w", err)
	}
	return toJSON(map[string]any{"id": args.ControlID, "deleted": true}), nil
}

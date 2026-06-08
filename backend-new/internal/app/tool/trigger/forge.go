package trigger

import (
	"context"
	"encoding/json"
	"fmt"

	triggerapp "github.com/sunweilin/forgify/backend/internal/app/trigger"
	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
)

// --- create_trigger --------------------------------------------------------

type CreateTrigger struct{ svc *triggerapp.Service }

func (t *CreateTrigger) Name() string { return "create_trigger" }

func (t *CreateTrigger) Description() string {
	return "Create a trigger — a signal source that fires the workflows listening to it. " +
		"kind + config:\n" +
		"• cron — config.expression (5-field cron, e.g. \"0 9 * * *\").\n" +
		"• webhook — config.path (mount subpath); optional config.secret (+ signatureAlgo \"hmac-sha256-hex\" for HMAC).\n" +
		"• fsnotify — config.path (absolute dir/file to watch); optional config.events [create|modify|delete|rename|chmod] and config.pattern (glob).\n" +
		"• sensor — periodically invokes a function/handler and fires when a CEL condition holds: config.targetKind (function|handler), config.targetId, config.method (handler only), config.intervalSec (≥5), config.condition (CEL bool over `payload` = the return value), config.output (CEL building the fire payload). For stateful/incremental probing bind a handler method (the resident process keeps its own cursor).\n" +
		"A trigger only runs while an active workflow references it."
}

func (t *CreateTrigger) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["name", "kind", "config"],
		"properties": {
			"name": {"type": "string", "description": "Unique display name."},
			"description": {"type": "string"},
			"kind": {"type": "string", "enum": ["cron", "webhook", "fsnotify", "sensor"]},
			"config": {"type": "object", "description": "Source-specific settings; see the tool description per kind."},
			"outputs": {"type": "array", "description": "Declared payload fields this trigger delivers to listening workflows: each {name, type, description}.", "items": {"type": "object"}}
		}
	}`)
}

func (t *CreateTrigger) ValidateInput(args json.RawMessage) error {
	var a struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("create_trigger: bad args: %w", err)
	}
	if a.Name == "" {
		return fmt.Errorf("create_trigger: name is required")
	}
	if !triggerdomain.IsValidKind(a.Kind) {
		return fmt.Errorf("create_trigger: kind must be one of cron/webhook/fsnotify/sensor")
	}
	return nil
}

func (t *CreateTrigger) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Kind        string            `json:"kind"`
		Config      map[string]any    `json:"config"`
		Outputs     []schemapkg.Field `json:"outputs"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("create_trigger: bad args: %w", err)
	}
	tr, err := t.svc.Create(ctx, triggerapp.CreateInput{
		Name: args.Name, Description: args.Description, Kind: args.Kind, Config: args.Config, Outputs: args.Outputs,
	})
	if err != nil {
		return "", fmt.Errorf("create_trigger: %w", err)
	}
	return toJSON(tr), nil
}

// --- edit_trigger ----------------------------------------------------------

type EditTrigger struct{ svc *triggerapp.Service }

func (t *EditTrigger) Name() string { return "edit_trigger" }

func (t *EditTrigger) Description() string {
	return "Edit a trigger's name / description / config (kind is immutable — to change source kind, delete and recreate). If the trigger is currently live, the new config takes effect immediately. Pass only the fields you want to change."
}

func (t *EditTrigger) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["triggerId"],
		"properties": {
			"triggerId": {"type": "string"},
			"name": {"type": "string"},
			"description": {"type": "string"},
			"config": {"type": "object", "description": "Full replacement config for the trigger's kind."},
			"outputs": {"type": "array", "description": "Declared payload fields delivered to workflows: each {name, type, description}.", "items": {"type": "object"}}
		}
	}`)
}

func (t *EditTrigger) ValidateInput(args json.RawMessage) error {
	var a struct {
		TriggerID string `json:"triggerId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("edit_trigger: bad args: %w", err)
	}
	if a.TriggerID == "" {
		return fmt.Errorf("edit_trigger: triggerId is required")
	}
	return nil
}

func (t *EditTrigger) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		TriggerID   string         `json:"triggerId"`
		Name        *string        `json:"name"`
		Description *string        `json:"description"`
		Config      map[string]any    `json:"config"`
		Outputs     []schemapkg.Field `json:"outputs"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("edit_trigger: bad args: %w", err)
	}
	tr, err := t.svc.Edit(ctx, args.TriggerID, triggerapp.EditInput{
		Name: args.Name, Description: args.Description, Config: args.Config, Outputs: args.Outputs,
	})
	if err != nil {
		return "", fmt.Errorf("edit_trigger: %w", err)
	}
	return toJSON(tr), nil
}

// --- delete_trigger --------------------------------------------------------

type DeleteTrigger struct{ svc *triggerapp.Service }

func (t *DeleteTrigger) Name() string { return "delete_trigger" }

func (t *DeleteTrigger) Description() string {
	return "Delete a trigger (soft-delete). Stops its listener and removes its relation edges. Workflows that referenced it stop receiving its signal."
}

func (t *DeleteTrigger) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["triggerId"],
		"properties": {"triggerId": {"type": "string"}}
	}`)
}

func (t *DeleteTrigger) ValidateInput(args json.RawMessage) error {
	var a struct {
		TriggerID string `json:"triggerId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("delete_trigger: bad args: %w", err)
	}
	if a.TriggerID == "" {
		return fmt.Errorf("delete_trigger: triggerId is required")
	}
	return nil
}

func (t *DeleteTrigger) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		TriggerID string `json:"triggerId"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("delete_trigger: bad args: %w", err)
	}
	if err := t.svc.Delete(ctx, args.TriggerID); err != nil {
		return "", fmt.Errorf("delete_trigger: %w", err)
	}
	return toJSON(map[string]any{"deleted": true, "triggerId": args.TriggerID}), nil
}

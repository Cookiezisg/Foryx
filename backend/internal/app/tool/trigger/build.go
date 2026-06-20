package trigger

import (
	"context"
	"encoding/json"
	"fmt"

	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	triggerapp "github.com/sunweilin/anselm/backend/internal/app/trigger"
	relationdomain "github.com/sunweilin/anselm/backend/internal/domain/relation"
	triggerdomain "github.com/sunweilin/anselm/backend/internal/domain/trigger"
	schemapkg "github.com/sunweilin/anselm/backend/internal/pkg/schema"
)

// --- create_trigger --------------------------------------------------------

type CreateTrigger struct{ svc *triggerapp.Service }

func (t *CreateTrigger) Name() string { return "create_trigger" }

func (t *CreateTrigger) Description() string {
	return "Create a trigger — a signal source that fires the workflows listening to it. The trigger node's " +
		"result (what a downstream node reads by node id, e.g. start.path) IS the FIRE PAYLOAD listed per kind:\n" +
		"• cron — config.expression (5-field cron, e.g. \"0 9 * * *\"). Fire payload: {firedAt}. get_trigger returns a computed nextFireAt for cron triggers (the next scheduled fire), so read it instead of computing the schedule yourself.\n" +
		"• webhook — config.path is the mount SUBpath, NOT the full URL: callers POST to /api/v1/webhooks/{triggerId}/{config.path} (triggerId = the id THIS call returns). AUTH (optional, tell whoever wires the caller the EXACT header or signed POSTs get 401): config.secret with NO signatureAlgo = plain shared-secret, caller sends header `X-Webhook-Secret: <secret>` OR query `?token=<secret>`; config.secret + config.signatureAlgo \"hmac-sha256-hex\" = HMAC, caller sends header `X-Hub-Signature-256: sha256=<lowercase-hex hmac_sha256(rawBody, secret)>` (rename the header via config.signatureHeader). Fire payload: {firedAt, method, path, headers, body (the POSTed JSON, parsed) | bodyRaw (the raw string when the body is not JSON)}.\n" +
		"• fsnotify — config.path (absolute dir/file to watch); optional config.events [create|modify|delete|rename|chmod] and config.pattern (glob). Fire payload: {firedAt, path, eventKind} — eventKind is one of those same lowercase tokens (combined events join with \"|\", e.g. \"create|modify\").\n" +
		"• sensor — periodically invokes a function/handler/mcp tool and fires when a CEL condition holds: config.targetKind (function|handler|mcp), config.targetId, config.method (required for handler/mcp — the method/tool name), config.intervalSec (≥5), config.condition (CEL bool over `payload` = the return value), config.output (CEL building the fire payload — YOU define its shape here, so that is the fire payload). LEVEL-TRIGGERED: it fires on EVERY interval the condition holds, not just on a false→true flip — a sustained bad state keeps firing each poll (the listening workflow's concurrency policy bounds the overlap; the default `serial` queues them, so set `skip`/`buffer_one` if you only want one run at a time). There is no built-in edge-trigger/dedup; if you need fire-once-per-transition, track the prior state inside a handler's condition. For stateful/incremental probing bind a handler method (the resident process keeps its own cursor).\n" +
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
			"outputs": {"type": "array", "description": "Declared payload fields delivered to listening workflows: each {name, type, description}. ONLY needed for sensor (describe what config.output emits); for cron/webhook/fsnotify it is set automatically from the kind's fixed fire payload and any value you pass is ignored.", "items": {"type": "object"}}
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
		return ErrNameRequired
	}
	if !triggerdomain.IsValidKind(a.Kind) {
		return triggerdomain.ErrInvalidKind
	}
	return nil
}

func (t *CreateTrigger) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
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
	return toolapp.ToJSON(tr), nil
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
			"outputs": {"type": "array", "description": "Declared payload fields delivered to workflows: each {name, type, description}. ONLY needed for sensor; for cron/webhook/fsnotify it is set automatically from the kind's fixed fire payload (any value passed is ignored).", "items": {"type": "object"}}
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
		return ErrTriggerIDRequired
	}
	return nil
}

func (t *EditTrigger) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		TriggerID   string            `json:"triggerId"`
		Name        *string           `json:"name"`
		Description *string           `json:"description"`
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
	return toolapp.ToJSON(tr), nil
}

// --- delete_trigger --------------------------------------------------------

type DeleteTrigger struct {
	svc  *triggerapp.Service
	deps toolapp.DependentCounter
}

func (t *DeleteTrigger) Name() string { return "delete_trigger" }

func (t *DeleteTrigger) Description() string {
	return "Delete a trigger (soft-delete). Stops its listener and removes its relation edges. Workflows that referenced it stop receiving its signal. The result reports how many entities referenced it — to check dependents BEFORE deleting, use get_relations."
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
		return ErrTriggerIDRequired
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
	deps := toolapp.DependentCount(ctx, t.deps, relationdomain.EntityKindTrigger, args.TriggerID)
	if err := t.svc.Delete(ctx, args.TriggerID); err != nil {
		return "", fmt.Errorf("delete_trigger: %w", err)
	}
	return toolapp.ToJSON(toolapp.AnnotateDependents(map[string]any{"deleted": true, "triggerId": args.TriggerID}, deps)), nil
}

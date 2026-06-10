package handler

import (
	"context"
	"encoding/json"
	"fmt"

	envfixapp "github.com/sunweilin/forgify/backend/internal/app/envfix"
	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
)

// --- create_handler --------------------------------------------------------

type CreateHandler struct{ svc *handlerapp.Service }

func (t *CreateHandler) Name() string { return "create_handler" }

func (t *CreateHandler) Description() string {
	return `Forge a new stateful handler (a Python class that stays resident across calls, so self.xxx persists — for DB connections, API sessions, caches). v1 takes effect immediately (no separate accept). Required ops: set_meta + at least one add_method. The class is assembled as HandlerImpl with __init__(self, ...initArgs), shutdown(self), and your methods.

OP SHAPES:
  {"op":"set_meta", "name":"snake_case", "description":"one line", "tags":["..."]}
  {"op":"set_imports", "imports":"import requests"}
  {"op":"set_init", "initBody":"self.session = requests.Session()"}     — __init__ body (after init args)
  {"op":"set_shutdown", "shutdownBody":"self.session.close()"}          — cleanup on stop/restart
  {"op":"set_init_args_schema", "args":[{"name":"api_key","type":"string","required":true,"sensitive":true}]}
  {"op":"add_method", "method":{"name":"fetch","inputs":[{"name":"url","type":"string"}],"outputs":[{"name":"body","type":"object"}],"body":"return self.session.get(url).json()","streaming":false}}
  {"op":"update_method", "name":"fetch", "patch":{"description":"..."}}  — RFC 7396 merge patch
  {"op":"delete_method", "name":"fetch"}
  {"op":"set_dependencies", "dependencies":["requests==2.31"]}
  {"op":"set_python_version", "version":"3.12"}

init_args (secrets like api_key) are NOT set here — the user fills them via the config; mark sensitive:true to encrypt at rest. A method body that yields {"progress": ...} streams progress. The instance starts once config is complete; failed dependency installs auto-fix (≤3) with an LLM.`
}

func (t *CreateHandler) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["ops"],
		"properties": {
			"ops": {"type": "array", "description": "Forge ops; each has an 'op' discriminator + op-specific fields.", "items": {"type": "object"}},
			"changeReason": {"type": "string", "description": "One-line reason for this creation."}
		}
	}`)
}

func (t *CreateHandler) ValidateInput(args json.RawMessage) error {
	var a struct {
		Ops []json.RawMessage `json:"ops"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("create_handler: bad args: %w", err)
	}
	if len(a.Ops) == 0 {
		return fmt.Errorf("create_handler: ops is required (non-empty)")
	}
	return nil
}

func (t *CreateHandler) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Ops          json.RawMessage `json:"ops"`
		ChangeReason string          `json:"changeReason"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("create_handler: bad args: %w", err)
	}
	ops, err := handlerapp.ParseOps(args.Ops)
	if err != nil {
		return "", fmt.Errorf("create_handler: %w", err)
	}
	sink := newForgeSink(ctx)
	defer sink.Close()
	h, v, err := t.svc.Create(ctx, handlerapp.CreateInput{Ops: ops, ChangeReason: args.ChangeReason, Progress: sink})
	if err != nil {
		return "", fmt.Errorf("create_handler: %w", err)
	}
	return toJSON(forgeOutput(h.ID, v, len(ops), sink.attempts)), nil
}

// --- edit_handler ----------------------------------------------------------

type EditHandler struct{ svc *handlerapp.Service }

func (t *EditHandler) Name() string { return "edit_handler" }

func (t *EditHandler) Description() string {
	return `Edit a handler: apply ops on top of its active version, producing a new version that takes effect immediately — the resident instance is restarted to load the new code. Same op shapes as create_handler. Empty ops rebuilds the environment + restarts. Use revert_handler to switch to an older version, restart_handler to just reset a misbehaving instance.`
}

func (t *EditHandler) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["handlerId", "ops"],
		"properties": {
			"handlerId": {"type": "string"},
			"ops": {"type": "array", "description": "Forge ops (empty array = rebuild env + restart).", "items": {"type": "object"}},
			"changeReason": {"type": "string", "description": "One-line reason for this edit."}
		}
	}`)
}

func (t *EditHandler) ValidateInput(args json.RawMessage) error {
	var a struct {
		HandlerID string          `json:"handlerId"`
		Ops       json.RawMessage `json:"ops"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("edit_handler: bad args: %w", err)
	}
	if a.HandlerID == "" {
		return fmt.Errorf("edit_handler: handlerId is required")
	}
	return nil
}

func (t *EditHandler) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		HandlerID    string          `json:"handlerId"`
		Ops          json.RawMessage `json:"ops"`
		ChangeReason string          `json:"changeReason"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("edit_handler: bad args: %w", err)
	}
	var ops []handlerapp.Op
	if len(args.Ops) > 0 {
		parsed, perr := handlerapp.ParseOps(args.Ops)
		if perr != nil {
			return "", fmt.Errorf("edit_handler: %w", perr)
		}
		ops = parsed
	}
	sink := newForgeSink(ctx)
	defer sink.Close()
	v, err := t.svc.Edit(ctx, handlerapp.EditInput{ID: args.HandlerID, Ops: ops, ChangeReason: args.ChangeReason, Progress: sink})
	if err != nil {
		return "", fmt.Errorf("edit_handler: %w", err)
	}
	return toJSON(forgeOutput(args.HandlerID, v, len(ops), sink.attempts)), nil
}

func forgeOutput(handlerID string, v *handlerdomain.Version, opsApplied int, attempts []envfixapp.Attempt) map[string]any {
	out := map[string]any{
		"id":         handlerID,
		"versionId":  v.ID,
		"version":    v.Version,
		"envStatus":  v.EnvStatus,
		"opsApplied": opsApplied,
	}
	if v.EnvError != "" {
		out["envError"] = v.EnvError
	}
	if len(attempts) > 1 {
		out["envFixAttempts"] = attempts
	}
	return out
}

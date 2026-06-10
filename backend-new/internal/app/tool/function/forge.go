package function

import (
	"context"
	"encoding/json"
	"fmt"

	envfixapp "github.com/sunweilin/forgify/backend/internal/app/envfix"
	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
)

// --- create_function -------------------------------------------------------

type CreateFunction struct{ svc *functionapp.Service }

func (t *CreateFunction) Name() string { return "create_function" }

func (t *CreateFunction) Description() string {
	return `Forge a new Python function from ops; v1 takes effect immediately (no separate accept step). Required ops: set_meta, set_code. Optional: set_inputs, set_outputs, set_dependencies, set_python_version.

OP SHAPES (exact field names):
  {"op":"set_meta", "name":"snake_case_name", "description":"one line", "tags":["..."]}
  {"op":"set_code", "code":"def main(x: str) -> dict:\n    return {\"y\": x}"}
  {"op":"set_inputs", "inputs":[{"name":"x","type":"string","description":"..."}]}
  {"op":"set_outputs", "outputs":[{"name":"y","type":"string","description":"..."}]}
  {"op":"set_dependencies", "dependencies":["requests==2.31","pandas"]}
  {"op":"set_python_version", "version":"3.12"}

Field type is one of: string, number, boolean, object, array (a coarse hint; nested shapes are read with CEL at runtime, not declared here).

The function is a stateless def(**kwargs) -> JSON-serialisable value, run in a fresh isolated process per call. If the dependency install fails, the platform auto-revises the deps with an LLM and retries (≤3); the result reports envStatus + any envFixAttempts. Pass credentials via arguments, never hard-code them.`
}

func (t *CreateFunction) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["ops"],
		"properties": {
			"ops": {"type": "array", "description": "Forge ops; each has an 'op' discriminator + op-specific fields.", "items": {"type": "object"}},
			"changeReason": {"type": "string", "description": "One-line reason for this creation."}
		}
	}`)
}

func (t *CreateFunction) ValidateInput(args json.RawMessage) error {
	var a struct {
		Ops []json.RawMessage `json:"ops"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("create_function: bad args: %w", err)
	}
	if len(a.Ops) == 0 {
		return fmt.Errorf("create_function: ops is required (non-empty)")
	}
	return nil
}

func (t *CreateFunction) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Ops          json.RawMessage `json:"ops"`
		ChangeReason string          `json:"changeReason"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("create_function: bad args: %w", err)
	}
	ops, err := functionapp.ParseOps(args.Ops)
	if err != nil {
		return "", fmt.Errorf("create_function: %w", err)
	}
	sink := newForgeSink(ctx)
	defer sink.Close()
	f, v, err := t.svc.Create(ctx, functionapp.CreateInput{Ops: ops, ChangeReason: args.ChangeReason, Progress: sink})
	if err != nil {
		return "", fmt.Errorf("create_function: %w", err)
	}
	return toJSON(forgeOutput(f.ID, v, len(ops), sink.attempts)), nil
}

// --- edit_function ---------------------------------------------------------

type EditFunction struct{ svc *functionapp.Service }

func (t *EditFunction) Name() string { return "edit_function" }

func (t *EditFunction) Description() string {
	return `Edit a function: apply ops on top of its active version, producing a new version that takes effect immediately. Same op shapes as create_function. Pass an empty ops array to just rebuild the active version's environment (retry a failed dependency install). Use revert_function to switch the active version to an older one.`
}

func (t *EditFunction) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["functionId", "ops"],
		"properties": {
			"functionId": {"type": "string"},
			"ops": {"type": "array", "description": "Forge ops (empty array = rebuild env only).", "items": {"type": "object"}},
			"changeReason": {"type": "string", "description": "One-line reason for this edit."}
		}
	}`)
}

func (t *EditFunction) ValidateInput(args json.RawMessage) error {
	var a struct {
		FunctionID string          `json:"functionId"`
		Ops        json.RawMessage `json:"ops"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("edit_function: bad args: %w", err)
	}
	if a.FunctionID == "" {
		return fmt.Errorf("edit_function: functionId is required")
	}
	return nil
}

func (t *EditFunction) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		FunctionID   string          `json:"functionId"`
		Ops          json.RawMessage `json:"ops"`
		ChangeReason string          `json:"changeReason"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("edit_function: bad args: %w", err)
	}
	var ops []functionapp.Op
	if len(args.Ops) > 0 {
		parsed, perr := functionapp.ParseOps(args.Ops)
		if perr != nil {
			return "", fmt.Errorf("edit_function: %w", perr)
		}
		ops = parsed
	}
	sink := newForgeSink(ctx)
	defer sink.Close()
	v, err := t.svc.Edit(ctx, functionapp.EditInput{ID: args.FunctionID, Ops: ops, ChangeReason: args.ChangeReason, Progress: sink})
	if err != nil {
		return "", fmt.Errorf("edit_function: %w", err)
	}
	return toJSON(forgeOutput(args.FunctionID, v, len(ops), sink.attempts)), nil
}

// forgeOutput is the shared create/edit result envelope: identity + env outcome +
// (when the fix loop ran more than once) the attempt history.
//
// forgeOutput 是 create/edit 共享的结果信封：身份 + env 结果 +（修复循环跑过一次以上时）尝试历史。
func forgeOutput(functionID string, v *functiondomain.Version, opsApplied int, attempts []envfixapp.Attempt) map[string]any {
	out := map[string]any{
		"id":         functionID,
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

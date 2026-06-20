package function

import (
	"context"
	"encoding/json"
	"fmt"

	functionapp "github.com/sunweilin/anselm/backend/internal/app/function"
	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	relationdomain "github.com/sunweilin/anselm/backend/internal/domain/relation"
)

// --- revert_function -------------------------------------------------------

type RevertFunction struct{ svc *functionapp.Service }

func (t *RevertFunction) Name() string { return "revert_function" }

func (t *RevertFunction) Description() string {
	return "Switch a function's active version to an existing version by its number. This only moves the active pointer — newer versions are kept in history and can be switched back to. Note: name, description and tags are NOT versioned (they live on the function), so a revert restores only the versioned snapshot (code/inputs/outputs/dependencies) and leaves name/description/tags unchanged — use edit_function set_meta to also change those."
}

func (t *RevertFunction) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["functionId", "version"],
		"properties": {
			"functionId": {"type": "string"},
			"version": {"type": "integer", "description": "The version number to make active."}
		}
	}`)
}

func (t *RevertFunction) ValidateInput(args json.RawMessage) error {
	var a struct {
		FunctionID string `json:"functionId"`
		Version    int    `json:"version"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("revert_function: bad args: %w", err)
	}
	if a.FunctionID == "" {
		return ErrFunctionIDRequired
	}
	if a.Version <= 0 {
		return ErrVersionPositive
	}
	return nil
}

func (t *RevertFunction) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		FunctionID string `json:"functionId"`
		Version    int    `json:"version"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("revert_function: bad args: %w", err)
	}
	v, err := t.svc.Revert(ctx, args.FunctionID, args.Version)
	if err != nil {
		return "", fmt.Errorf("revert_function: %w", err)
	}
	return toolapp.ToJSON(map[string]any{"id": args.FunctionID, "activeVersionId": v.ID, "version": v.Version}), nil
}

// --- delete_function -------------------------------------------------------

type DeleteFunction struct {
	svc  *functionapp.Service
	deps toolapp.DependentCounter
}

func (t *DeleteFunction) Name() string { return "delete_function" }

func (t *DeleteFunction) Description() string {
	return "Delete a function and all its versions and sandbox environments. This is not reversible. The result reports how many other entities referenced it (and may now fail) — to see what depends on something BEFORE deleting, use get_relations."
}

func (t *DeleteFunction) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["functionId"],
		"properties": {"functionId": {"type": "string"}}
	}`)
}

func (t *DeleteFunction) ValidateInput(args json.RawMessage) error {
	var a struct {
		FunctionID string `json:"functionId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("delete_function: bad args: %w", err)
	}
	if a.FunctionID == "" {
		return ErrFunctionIDRequired
	}
	return nil
}

func (t *DeleteFunction) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		FunctionID string `json:"functionId"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("delete_function: bad args: %w", err)
	}
	// Count dependents BEFORE the delete — the purge erases the edges, so reading after is too late.
	// 删**前**数依赖——purge 会抹掉边，删后再读已晚。
	deps := toolapp.DependentCount(ctx, t.deps, relationdomain.EntityKindFunction, args.FunctionID)
	if err := t.svc.Delete(ctx, args.FunctionID); err != nil {
		return "", fmt.Errorf("delete_function: %w", err)
	}
	return toolapp.ToJSON(toolapp.AnnotateDependents(map[string]any{"id": args.FunctionID, "deleted": true}, deps)), nil
}

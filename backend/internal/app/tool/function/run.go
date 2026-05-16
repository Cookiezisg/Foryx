package function

import (
	"context"
	"encoding/json"
	"fmt"

	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
)

type RunFunction struct {
	svc *functionapp.Service
}

func (t *RunFunction) Name() string { return "run_function" }

func (t *RunFunction) Description() string {
	return "Execute a function with given arguments. Returns the result object containing " +
		"ok / output / errorMsg / elapsedMs. If the function's env is not yet ready, " +
		"sync stage progress streams as deltas under a progress block. Cancellation is " +
		"caller-driven (no per-call timeout knob): if the user cancels the chat turn the " +
		"sandbox process tree is killed via ctx propagation."
}

func (t *RunFunction) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"functionId": {"type": "string", "description": "Function ID (fn_xxx)"},
			"version":    {"type": "string", "description": "Optional version ID (fnv_xxx); omit for active"},
			"args":       {"type": "object", "description": "Kwargs passed to the user's def"}
		},
		"required": ["functionId", "args"]
	}`)
}

func (t *RunFunction) IsReadOnly() bool        { return false }
func (t *RunFunction) NeedsReadFirst() bool    { return false }
func (t *RunFunction) RequiresWorkspace() bool { return false }

func (t *RunFunction) ValidateInput(json.RawMessage) error { return nil }
func (t *RunFunction) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *RunFunction) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		FunctionID string         `json:"functionId"`
		Version    string         `json:"version"`
		Args       map[string]any `json:"args"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("run_function: bad args: %w", err)
	}
	if args.FunctionID == "" {
		return "", fmt.Errorf("run_function: functionId required")
	}

	em := eventlogpkg.From(ctx)
	progID := em.StartBlock(ctx, eventlogdomain.BlockTypeProgress, map[string]any{
		"stage":      "executing",
		"functionId": args.FunctionID,
	})

	res, err := t.svc.RunFunction(ctx, functionapp.RunInput{
		FunctionID: args.FunctionID,
		VersionID:  args.Version,
		Input:      args.Args,
	})
	if err != nil {
		em.StopBlock(ctx, progID, eventlogdomain.StatusError, err)
		return "", fmt.Errorf("run_function: %w", err)
	}
	em.StopBlock(ctx, progID, eventlogdomain.StatusCompleted, nil)

	b, _ := json.Marshal(res)
	return string(b), nil
}

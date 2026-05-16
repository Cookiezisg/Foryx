package function

import (
	"context"
	"encoding/json"
	"fmt"

	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

type GetFunction struct {
	svc *functionapp.Service
}

func (t *GetFunction) Name() string { return "get_function" }

func (t *GetFunction) Description() string {
	return "Get the full details of a specific function including code, parameters, " +
		"dependencies, and pending version (if any). Use this to verify a candidate " +
		"function before running it or before editing."
}

func (t *GetFunction) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string", "description": "The function ID (fn_xxx) to retrieve"}
		},
		"required": ["id"]
	}`)
}

func (t *GetFunction) IsReadOnly() bool        { return true }
func (t *GetFunction) NeedsReadFirst() bool    { return false }
func (t *GetFunction) RequiresWorkspace() bool { return false }

func (t *GetFunction) ValidateInput(json.RawMessage) error { return nil }
func (t *GetFunction) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *GetFunction) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("get_function: bad args: %w", err)
	}
	if args.ID == "" {
		return "", fmt.Errorf("get_function: id required")
	}
	f, err := t.svc.Get(ctx, args.ID)
	if err != nil {
		return "", fmt.Errorf("get_function: %w", err)
	}
	b, _ := json.Marshal(f)
	return string(b), nil
}

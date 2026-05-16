package function

import (
	"context"
	"encoding/json"
	"fmt"

	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

type DeleteFunction struct {
	svc *functionapp.Service
}

func (t *DeleteFunction) Name() string { return "delete_function" }

func (t *DeleteFunction) Description() string {
	return "Soft-delete a function. Any workflows referencing it will be marked " +
		"needs_attention until the user remediates."
}

func (t *DeleteFunction) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string", "description": "Function ID to delete"}
		},
		"required": ["id"]
	}`)
}

func (t *DeleteFunction) IsReadOnly() bool        { return false }
func (t *DeleteFunction) NeedsReadFirst() bool    { return false }
func (t *DeleteFunction) RequiresWorkspace() bool { return false }

func (t *DeleteFunction) ValidateInput(json.RawMessage) error { return nil }
func (t *DeleteFunction) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *DeleteFunction) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("delete_function: bad args: %w", err)
	}
	if args.ID == "" {
		return "", fmt.Errorf("delete_function: id required")
	}
	if err := t.svc.Delete(ctx, args.ID); err != nil {
		return "", fmt.Errorf("delete_function: %w", err)
	}
	b, _ := json.Marshal(map[string]any{"deleted": true, "id": args.ID})
	return string(b), nil
}

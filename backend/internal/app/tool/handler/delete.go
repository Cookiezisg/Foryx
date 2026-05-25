package handler

import (
	"context"
	"encoding/json"
	"fmt"

	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

type DeleteHandler struct {
	svc *handlerapp.Service
}

func (t *DeleteHandler) Name() string { return "delete_handler" }

func (t *DeleteHandler) Description() string {
	return "Soft-delete a handler. Destroys all live instances; workflows referencing it become needs_attention."
}

func (t *DeleteHandler) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string"}
		},
		"required": ["id"]
	}`)
}

func (t *DeleteHandler) IsReadOnly() bool        { return false }
func (t *DeleteHandler) NeedsReadFirst() bool    { return false }
func (t *DeleteHandler) RequiresWorkspace() bool { return false }

func (t *DeleteHandler) ValidateInput(json.RawMessage) error { return nil }
func (t *DeleteHandler) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *DeleteHandler) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("delete_handler: bad args: %w", err)
	}
	if args.ID == "" {
		return "", fmt.Errorf("delete_handler: id required")
	}
	if err := t.svc.Delete(ctx, args.ID); err != nil {
		return "", fmt.Errorf("delete_handler: %w", err)
	}
	b, _ := json.Marshal(map[string]any{"deleted": true, "id": args.ID})
	return string(b), nil
}

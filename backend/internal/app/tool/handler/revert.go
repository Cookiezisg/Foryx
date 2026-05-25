package handler

import (
	"context"
	"encoding/json"
	"fmt"

	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

type RevertHandler struct {
	svc *handlerapp.Service
}

func (t *RevertHandler) Name() string { return "revert_handler" }

func (t *RevertHandler) Description() string {
	return "Point a handler's active version at a previously-accepted version number."
}

func (t *RevertHandler) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string"},
			"targetVersion": {"type": "integer"}
		},
		"required": ["id", "targetVersion"]
	}`)
}

func (t *RevertHandler) IsReadOnly() bool        { return false }
func (t *RevertHandler) NeedsReadFirst() bool    { return false }
func (t *RevertHandler) RequiresWorkspace() bool { return false }

func (t *RevertHandler) ValidateInput(json.RawMessage) error { return nil }
func (t *RevertHandler) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *RevertHandler) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ID            string `json:"id"`
		TargetVersion int    `json:"targetVersion"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("revert_handler: bad args: %w", err)
	}
	if args.ID == "" || args.TargetVersion <= 0 {
		return "", fmt.Errorf("revert_handler: id + targetVersion required")
	}
	v, err := t.svc.Revert(ctx, args.ID, args.TargetVersion)
	if err != nil {
		return "", fmt.Errorf("revert_handler: %w", err)
	}
	out := map[string]any{"versionId": v.ID, "targetVersion": v.Version}
	b, _ := json.Marshal(out)
	return string(b), nil
}

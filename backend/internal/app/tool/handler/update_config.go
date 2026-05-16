package handler

import (
	"context"
	"encoding/json"
	"fmt"

	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

type UpdateHandlerConfig struct {
	svc *handlerapp.Service
}

func (t *UpdateHandlerConfig) Name() string { return "update_handler_config" }

func (t *UpdateHandlerConfig) Description() string {
	return "Set or update init_args values for a handler (e.g. DB connection " +
		"strings, API keys). Values are merged into stored config (nil deletes a " +
		"key) and encrypted at rest. Sensitive values per the handler's schema " +
		"are masked in get_handler / search_handler responses — they are NEVER " +
		"echoed in tool results, including this one. Returns the new configState."
}

func (t *UpdateHandlerConfig) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id":     {"type": "string", "description": "Handler ID"},
			"config": {"type": "object", "description": "Partial config (init_args values; null deletes a key)"}
		},
		"required": ["id", "config"]
	}`)
}

func (t *UpdateHandlerConfig) IsReadOnly() bool        { return false }
func (t *UpdateHandlerConfig) NeedsReadFirst() bool    { return false }
func (t *UpdateHandlerConfig) RequiresWorkspace() bool { return false }

func (t *UpdateHandlerConfig) ValidateInput(json.RawMessage) error { return nil }
func (t *UpdateHandlerConfig) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *UpdateHandlerConfig) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ID     string         `json:"id"`
		Config map[string]any `json:"config"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("update_handler_config: bad args: %w", err)
	}
	if args.ID == "" {
		return "", fmt.Errorf("update_handler_config: id required")
	}
	if err := t.svc.UpdateConfig(ctx, args.ID, args.Config); err != nil {
		return "", fmt.Errorf("update_handler_config: %w", err)
	}

	// Recompute configState — needs active version's schema.
	// 重算 configState — 需 active 版本的 schema。
	h, err := t.svc.Get(ctx, args.ID)
	if err != nil {
		return "", fmt.Errorf("update_handler_config: get after update: %w", err)
	}
	out := map[string]any{
		"updated":     true,
		"configState": h.ConfigState,
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

package handler

import (
	"context"
	"encoding/json"
	"fmt"

	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

type GetHandler struct {
	svc *handlerapp.Service
}

func (t *GetHandler) Name() string { return "get_handler" }

func (t *GetHandler) Description() string {
	return "Get full details of a handler: methods, init_args schema, configState " +
		"(ready / partially_configured / unconfigured), pending version if any, and " +
		"a masked view of stored config (sensitive values replaced with ********)."
}

func (t *GetHandler) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string", "description": "Handler ID (hd_xxx)"}
		},
		"required": ["id"]
	}`)
}

func (t *GetHandler) IsReadOnly() bool        { return true }
func (t *GetHandler) NeedsReadFirst() bool    { return false }
func (t *GetHandler) RequiresWorkspace() bool { return false }

func (t *GetHandler) ValidateInput(json.RawMessage) error { return nil }
func (t *GetHandler) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *GetHandler) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("get_handler: bad args: %w", err)
	}
	if args.ID == "" {
		return "", fmt.Errorf("get_handler: id required")
	}
	h, err := t.svc.Get(ctx, args.ID)
	if err != nil {
		return "", fmt.Errorf("get_handler: %w", err)
	}

	// Active version for methods + init_args.
	// active 版本拿 methods + init_args schema。
	var active map[string]any
	if h.ActiveVersionID != "" {
		v, err := t.svc.GetVersion(ctx, h.ActiveVersionID)
		if err == nil && v != nil {
			masked, _ := t.svc.MaskedConfig(ctx, args.ID, v.InitArgsSchema)
			active = map[string]any{
				"id":             v.ID,
				"version":        v.Version,
				"methods":        v.Methods,
				"initArgsSchema": v.InitArgsSchema,
				"dependencies":   v.Dependencies,
				"pythonVersion":  v.PythonVersion,
				"envStatus":      v.EnvStatus,
				"envError":       v.EnvError,
				"maskedConfig":   masked,
			}
		}
	}

	out := map[string]any{
		"handler":       h,
		"activeVersion": active,
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

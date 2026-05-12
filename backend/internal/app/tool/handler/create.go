// create.go — create_handler system tool: applies ops to build a new Handler
// with auto-accepted v1. Streams 1 progress delta per op.

package handler

import (
	"context"
	"encoding/json"
	"fmt"

	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
)

type CreateHandler struct {
	svc *handlerapp.Service
}

func (t *CreateHandler) Name() string { return "create_handler" }

func (t *CreateHandler) Description() string {
	return "Create a new handler by applying a sequence of method-level ops. " +
		"Common ops: set_meta (name + description), set_imports (top-level imports), " +
		"set_init (__init__ body), set_init_args_schema (one entry per init arg, " +
		"mark sensitive=true for secrets), add_method (one Python method spec + body), " +
		"set_dependencies. v1 is auto-accepted; user must configure init_args (via " +
		"update_handler_config) before call_handler can succeed."
}

func (t *CreateHandler) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"ops": {"type": "array", "items": {"type": "object"}, "description": "Method-level ops"},
			"changeReason": {"type": "string", "description": "One-line reason"}
		},
		"required": ["ops"]
	}`)
}

func (t *CreateHandler) IsReadOnly() bool        { return false }
func (t *CreateHandler) NeedsReadFirst() bool    { return false }
func (t *CreateHandler) RequiresWorkspace() bool { return false }

func (t *CreateHandler) ValidateInput(json.RawMessage) error { return nil }
func (t *CreateHandler) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
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

	em := eventlogpkg.From(ctx)
	progID := em.StartBlock(ctx, eventlogdomain.BlockTypeProgress, map[string]any{
		"stage": "applying ops", "count": len(ops),
	})
	defer em.StopBlock(ctx, progID, eventlogdomain.StatusCompleted, nil)

	h, v, err := t.svc.Create(ctx, handlerapp.CreateInput{
		Ops:             ops,
		ChangeReason:    args.ChangeReason,
		ProgressBlockID: progID,
	})
	if err != nil {
		em.StopBlock(ctx, progID, eventlogdomain.StatusError, err)
		return "", fmt.Errorf("create_handler: %w", err)
	}

	out := map[string]any{
		"id":         h.ID,
		"versionId":  v.ID,
		"version":    v.Version,
		"status":     v.Status,
		"envStatus":  v.EnvStatus,
		"envError":   v.EnvError,
		"opsApplied": len(ops),
		"note":       "Use update_handler_config to set init_args before call_handler.",
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

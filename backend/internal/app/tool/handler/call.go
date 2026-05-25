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

type CallHandler struct {
	svc *handlerapp.Service
}

func (t *CallHandler) Name() string { return "call_handler" }

func (t *CallHandler) Description() string {
	return "Invoke a method on a handler. Chat-scope calls are per-call (spawn → run → destroy). " +
		"Streaming methods (Python yield) emit progress deltas; the final return is the tool_result. " +
		"Fails if configState != ready — set init_args via update_handler_config first."
}

func (t *CallHandler) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"handlerName": {"type": "string", "description": "Handler name (preferred)"},
			"handlerId":   {"type": "string", "description": "Handler ID; alternative to name"},
			"method":      {"type": "string", "description": "Method name on the class"},
			"args":        {"type": "object", "description": "Kwargs for the method"}
		},
		"required": ["method"]
	}`)
}

func (t *CallHandler) IsReadOnly() bool        { return false }
func (t *CallHandler) NeedsReadFirst() bool    { return false }
func (t *CallHandler) RequiresWorkspace() bool { return false }

func (t *CallHandler) ValidateInput(json.RawMessage) error { return nil }
func (t *CallHandler) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *CallHandler) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		HandlerName string         `json:"handlerName"`
		HandlerID   string         `json:"handlerId"`
		Method      string         `json:"method"`
		Args        map[string]any `json:"args"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("call_handler: bad args: %w", err)
	}
	if args.HandlerName == "" && args.HandlerID == "" {
		return "", fmt.Errorf("call_handler: handlerName or handlerId required")
	}
	if args.Method == "" {
		return "", fmt.Errorf("call_handler: method required")
	}

	// Allocate a progress block to which the streaming yields will append.
	// 起 progress block 接 yield 流。
	em := eventlogpkg.From(ctx)
	progID := em.StartBlock(ctx, eventlogdomain.BlockTypeProgress, map[string]any{
		"stage":  "calling",
		"method": args.Method,
	})
	onProgress := func(p any) {
		raw, _ := json.Marshal(p)
		em.DeltaBlock(ctx, progID, string(raw)+"\n")
	}

	result, err := t.svc.Call(ctx, handlerapp.CallInput{
		HandlerName: args.HandlerName,
		HandlerID:   args.HandlerID,
		Method:      args.Method,
		Args:        args.Args,
		// chat scope → per-call lifetime (spawn-method-destroy)
		// chat scope → per-call lifetime
		Owner:      handlerapp.Owner{Kind: "chat"},
		OnProgress: onProgress,
	})
	if err != nil {
		em.StopBlock(ctx, progID, eventlogdomain.StatusError, err)
		return "", fmt.Errorf("call_handler: %w", err)
	}
	em.StopBlock(ctx, progID, eventlogdomain.StatusCompleted, nil)

	b, _ := json.Marshal(map[string]any{"result": result})
	return string(b), nil
}

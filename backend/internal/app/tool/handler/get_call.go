package handler

import (
	"context"
	"encoding/json"
	"fmt"

	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

type GetHandlerCall struct {
	svc *handlerapp.Service
}

func (t *GetHandlerCall) Name() string { return "get_handler_call" }

func (t *GetHandlerCall) Description() string {
	return "Full detail of one handler call: complete input/output (truncated at 4KB) plus computed hints (outputEmpty, significantlySlower) for diagnosis."
}

func (t *GetHandlerCall) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string"}
		},
		"required": ["id"]
	}`)
}

func (t *GetHandlerCall) IsReadOnly() bool        { return true }
func (t *GetHandlerCall) NeedsReadFirst() bool    { return false }
func (t *GetHandlerCall) RequiresWorkspace() bool { return false }

func (t *GetHandlerCall) ValidateInput(json.RawMessage) error { return nil }
func (t *GetHandlerCall) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *GetHandlerCall) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("get_handler_call: bad args: %w", err)
	}
	if args.ID == "" {
		return "", fmt.Errorf("get_handler_call: id required")
	}
	detail, err := t.svc.GetCallDetail(ctx, args.ID)
	if err != nil {
		return "", fmt.Errorf("get_handler_call: %w", err)
	}

	inputTrunc := truncateJSON(detail.Input, 4096)
	outputTrunc := truncateJSON(detail.Output, 4096)
	inputTruncated := false
	if raw, e := json.Marshal(detail.Input); e == nil && len(raw) > 4096 {
		inputTruncated = true
	}
	outputTruncated := false
	if raw, e := json.Marshal(detail.Output); e == nil && len(raw) > 4096 {
		outputTruncated = true
	}

	out := map[string]any{
		"id":              detail.ID,
		"status":          detail.Status,
		"triggeredBy":     detail.TriggeredBy,
		"handlerId":       detail.HandlerID,
		"versionId":       detail.VersionID,
		"method":          detail.Method,
		"instanceId":      detail.InstanceID,
		"ownerKind":       detail.OwnerKind,
		"ownerId":         detail.OwnerID,
		"input":           json.RawMessage(orFallback(inputTrunc, "null")),
		"output":          json.RawMessage(orFallback(outputTrunc, "null")),
		"inputTruncated":  inputTruncated,
		"outputTruncated": outputTruncated,
		"errorMessage":    detail.ErrorMessage,
		"elapsedMs":       detail.ElapsedMs,
		"startedAt":       detail.StartedAt,
		"endedAt":         detail.EndedAt,
		"conversationId":  detail.ConversationID,
		"messageId":       detail.MessageID,
		"toolCallId":      detail.ToolCallID,
		"flowrunId":       detail.FlowrunID,
		"hints":           detail.Hints,
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

func orFallback(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

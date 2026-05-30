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
	return "Full detail of one handler call: complete input/output (256KB cap) plus computed hints (outputEmpty, significantlySlower) for diagnosis."
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

	// 256KB defensive cap (raised from 4KB); boundedJSON keeps the envelope valid
	// even when it caps — the old sliced-RawMessage returned malformed JSON / empty.
	//
	// 256KB 防御上限(从 4KB 抬高)；boundedJSON 即使截断也保 envelope 合法
	//（旧切片 RawMessage 返畸形 JSON / 空）。
	const getCallMaxBytes = 256 * 1024
	inputVal, inputTruncated := boundedJSON(detail.Input, getCallMaxBytes)
	outputVal, outputTruncated := boundedJSON(detail.Output, getCallMaxBytes)

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
		"input":           inputVal,
		"output":          outputVal,
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

// boundedJSON renders a value for the call detail: valid json.RawMessage within
// limit, else a truncated STRING (valid envelope — a sliced RawMessage is
// malformed JSON and makes the whole result fail to marshal). bool = truncated.
//
// boundedJSON 为 call 详情渲染值：未超 limit 返合法 json.RawMessage，超长返截断
// STRING（envelope 内合法——切片 RawMessage 是畸形 JSON，会让整个结果 marshal 失败）。
func boundedJSON(v any, limit int) (any, bool) {
	if v == nil {
		return json.RawMessage("null"), false
	}
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null"), false
	}
	if len(b) <= limit {
		return json.RawMessage(b), false
	}
	return fmt.Sprintf("%s…[truncated, %d total bytes]", b[:limit], len(b)), true
}

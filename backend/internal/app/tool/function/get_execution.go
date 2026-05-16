package function

import (
	"context"
	"encoding/json"
	"fmt"

	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

type GetFunctionExecution struct {
	svc *functionapp.Service
}

func (t *GetFunctionExecution) Name() string { return "get_function_execution" }

func (t *GetFunctionExecution) Description() string {
	return "Get full details of one function execution by id, including complete input + " +
		"output (truncated at 4KB) and machine-computed hints (outputEmpty, " +
		"significantlySlower) for fast diagnosis."
}

func (t *GetFunctionExecution) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string", "description": "Execution id (fne_xxx)"}
		},
		"required": ["id"]
	}`)
}

func (t *GetFunctionExecution) IsReadOnly() bool        { return true }
func (t *GetFunctionExecution) NeedsReadFirst() bool    { return false }
func (t *GetFunctionExecution) RequiresWorkspace() bool { return false }

func (t *GetFunctionExecution) ValidateInput(json.RawMessage) error { return nil }
func (t *GetFunctionExecution) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *GetFunctionExecution) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("get_function_execution: bad args: %w", err)
	}
	if args.ID == "" {
		return "", fmt.Errorf("get_function_execution: id required")
	}
	detail, err := t.svc.GetExecutionDetail(ctx, args.ID)
	if err != nil {
		return "", fmt.Errorf("get_function_execution: %w", err)
	}

	// 4KB truncation for input/output per spec/08-executions.md §7.1.
	// Reuses the helper in search_executions.go.
	//
	// 4KB 截断 input/output(spec §7.1)。复用 search_executions.go 的 helper。
	inputTrunc := truncateJSON(detail.Input, 4096)
	outputTrunc := truncateJSON(detail.Output, 4096)
	// Detect truncation by re-marshaling raw and comparing byte length.
	inputTruncated := false
	if rawIn, e := json.Marshal(detail.Input); e == nil && len(rawIn) > 4096 {
		inputTruncated = true
	}
	outputTruncated := false
	if rawOut, e := json.Marshal(detail.Output); e == nil && len(rawOut) > 4096 {
		outputTruncated = true
	}

	out := map[string]any{
		"id":              detail.ID,
		"status":          detail.Status,
		"triggeredBy":     detail.TriggeredBy,
		"functionId":      detail.FunctionID,
		"versionId":       detail.VersionID,
		"pythonVersion":   detail.PythonVersion,
		"input":           json.RawMessage(orFallback(inputTrunc, "null")),
		"output":          json.RawMessage(orFallback(outputTrunc, "null")),
		"inputTruncated":  inputTruncated,
		"outputTruncated": outputTruncated,
		"errorCode":       detail.ErrorCode,
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

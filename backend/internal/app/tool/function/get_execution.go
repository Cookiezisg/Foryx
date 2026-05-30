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
	return "Full detail of one execution: input + output (256KB cap) plus diagnostic hints (outputEmpty, significantlySlower)."
}

func (t *GetFunctionExecution) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string"}
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

	// 256KB defensive cap (raised from 4KB): the user explicitly asked for THIS
	// execution, so stop semantically truncating it small. boundedJSON keeps the
	// envelope valid even when it caps — the old sliced-RawMessage produced
	// malformed JSON and an empty result on large rows. 08-executions.md §7.1.
	//
	// 256KB 防御上限(从 4KB 抬高)：用户明确要看这条执行，不再语义截小。boundedJSON
	// 即使截断也保 envelope 合法(旧的切片 RawMessage 是畸形 JSON、大行返空)。
	const getExecMaxBytes = 256 * 1024
	inputVal, inputTruncated := boundedJSON(detail.Input, getExecMaxBytes)
	outputVal, outputTruncated := boundedJSON(detail.Output, getExecMaxBytes)

	out := map[string]any{
		"id":              detail.ID,
		"status":          detail.Status,
		"triggeredBy":     detail.TriggeredBy,
		"functionId":      detail.FunctionID,
		"versionId":       detail.VersionID,
		"pythonVersion":   detail.PythonVersion,
		"input":           inputVal,
		"output":          outputVal,
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

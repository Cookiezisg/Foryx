package function

import (
	"context"
	"encoding/json"
	"fmt"

	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// --- run_function ----------------------------------------------------------

type RunFunction struct{ svc *functionapp.Service }

func (t *RunFunction) Name() string { return "run_function" }

func (t *RunFunction) Description() string {
	return "Run a function with keyword arguments; returns {ok, output, errorMsg, elapsedMs, logs} — logs carries the function's print()/debug output. Omit version to run the active version. Each run is recorded — inspect history with search_function_executions / get_function_execution."
}

func (t *RunFunction) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["functionId", "args"],
		"properties": {
			"functionId": {"type": "string"},
			"args": {"type": "object", "description": "Keyword arguments passed to the function."},
			"version": {"type": "integer", "description": "Optional version number; omit for the active version."}
		}
	}`)
}

func (t *RunFunction) ValidateInput(args json.RawMessage) error {
	var a struct {
		FunctionID string `json:"functionId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("run_function: bad args: %w", err)
	}
	if a.FunctionID == "" {
		return ErrFunctionIDRequired
	}
	return nil
}

func (t *RunFunction) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		FunctionID string         `json:"functionId"`
		Args       map[string]any `json:"args"`
		Version    int            `json:"version"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("run_function: bad args: %w", err)
	}

	versionID := ""
	if args.Version > 0 {
		v, err := t.svc.GetVersionByNumber(ctx, args.FunctionID, args.Version)
		if err != nil {
			return "", fmt.Errorf("run_function: %w", err)
		}
		versionID = v.ID
	}

	res, err := t.svc.RunFunction(ctx, functionapp.RunInput{
		FunctionID:  args.FunctionID,
		VersionID:   versionID,
		Input:       args.Args,
		TriggeredBy: triggerFromCtx(ctx),
	})
	if err != nil {
		return "", fmt.Errorf("run_function: %w", err)
	}
	return toolapp.ToJSON(res), nil
}

// triggerFromCtx derives the execution body: a subagent context means an agent run,
// otherwise a normal chat turn. (Workflow runs call Service.RunFunction directly, not
// via this tool, and set workflow themselves.)
//
// triggerFromCtx 推导执行体：有 subagent 上下文即 agent 运行，否则普通 chat 回合。（workflow
// 直接调 Service.RunFunction、不经此工具，自设 workflow。）
func triggerFromCtx(ctx context.Context) string {
	if _, ok := reqctxpkg.GetSubagentID(ctx); ok {
		return functiondomain.TriggeredByAgent
	}
	return functiondomain.TriggeredByChat
}

// --- search_function_executions --------------------------------------------

type SearchFunctionExecutions struct{ svc *functionapp.Service }

func (t *SearchFunctionExecutions) Name() string { return "search_function_executions" }

func (t *SearchFunctionExecutions) Description() string {
	return "List a function's execution history (most recent first) with an ok/failed rollup. Filter by status (ok|failed|cancelled|timeout) or version id. Use get_function_execution on an id for the full record including logs."
}

func (t *SearchFunctionExecutions) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["functionId"],
		"properties": {
			"functionId": {"type": "string"},
			"status": {"type": "string", "description": "Optional: ok | failed | cancelled | timeout."},
			"versionId": {"type": "string", "description": "Optional version id filter."},
			"limit": {"type": "integer", "description": "Page size (default 50)."},
			"cursor": {"type": "string", "description": "Opaque pagination cursor."}
		}
	}`)
}

func (t *SearchFunctionExecutions) ValidateInput(args json.RawMessage) error {
	var a struct {
		FunctionID string `json:"functionId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("search_function_executions: bad args: %w", err)
	}
	if a.FunctionID == "" {
		return ErrFunctionIDRequired
	}
	return nil
}

func (t *SearchFunctionExecutions) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		FunctionID string `json:"functionId"`
		Status     string `json:"status"`
		VersionID  string `json:"versionId"`
		Limit      int    `json:"limit"`
		Cursor     string `json:"cursor"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("search_function_executions: bad args: %w", err)
	}
	res, err := t.svc.SearchExecutions(ctx, functiondomain.ExecutionFilter{
		FunctionID: args.FunctionID,
		Status:     args.Status,
		VersionID:  args.VersionID,
		Limit:      args.Limit,
		Cursor:     args.Cursor,
	})
	if err != nil {
		return "", fmt.Errorf("search_function_executions: %w", err)
	}
	return toolapp.ToJSON(res), nil
}

// --- get_function_execution ------------------------------------------------

type GetFunctionExecution struct{ svc *functionapp.Service }

func (t *GetFunctionExecution) Name() string { return "get_function_execution" }

func (t *GetFunctionExecution) Description() string {
	return "Get one execution record (input, output, error, logs, timing) by its id. logs carries the function's print()/debug output."
}

func (t *GetFunctionExecution) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["executionId"],
		"properties": {"executionId": {"type": "string"}}
	}`)
}

func (t *GetFunctionExecution) ValidateInput(args json.RawMessage) error {
	var a struct {
		ExecutionID string `json:"executionId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("get_function_execution: bad args: %w", err)
	}
	if a.ExecutionID == "" {
		return ErrExecutionIDRequired
	}
	return nil
}

func (t *GetFunctionExecution) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ExecutionID string `json:"executionId"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("get_function_execution: bad args: %w", err)
	}
	e, err := t.svc.GetExecution(ctx, args.ExecutionID)
	if err != nil {
		return "", fmt.Errorf("get_function_execution: %w", err)
	}
	return toolapp.ToJSON(e), nil
}

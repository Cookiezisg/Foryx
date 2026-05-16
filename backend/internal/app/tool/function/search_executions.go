package function

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
)

type SearchFunctionExecutions struct {
	svc *functionapp.Service
}

func (t *SearchFunctionExecutions) Name() string { return "search_function_executions" }

func (t *SearchFunctionExecutions) Description() string {
	return "Search the function execution log. Filter by functionId / versionId / status " +
		"(ok|failed|cancelled|timeout) / conversationId / flowrunId / since-until ISO8601. " +
		"Returns previews (200-byte input/output snippets) + aggregates (ok/failed/cancelled/" +
		"timeout counts + avg/p95 elapsed_ms). Use get_function_execution to drill into a single " +
		"row by id when you need the full input + output."
}

func (t *SearchFunctionExecutions) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"functionId":     {"type": "string", "description": "Filter to one function"},
			"versionId":      {"type": "string", "description": "Filter to one version"},
			"status":         {"type": "string", "enum": ["ok","failed","cancelled","timeout"]},
			"conversationId": {"type": "string"},
			"flowrunId":      {"type": "string"},
			"since":          {"type": "string", "description": "ISO8601 lower bound on startedAt"},
			"until":          {"type": "string", "description": "ISO8601 upper bound on startedAt"},
			"limit":          {"type": "integer", "description": "Max rows (1-200, default 50)"},
			"cursor":         {"type": "string", "description": "Opaque pagination token from prior call"}
		}
	}`)
}

func (t *SearchFunctionExecutions) IsReadOnly() bool        { return true }
func (t *SearchFunctionExecutions) NeedsReadFirst() bool    { return false }
func (t *SearchFunctionExecutions) RequiresWorkspace() bool { return false }

func (t *SearchFunctionExecutions) ValidateInput(json.RawMessage) error { return nil }
func (t *SearchFunctionExecutions) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *SearchFunctionExecutions) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		FunctionID     string `json:"functionId"`
		VersionID      string `json:"versionId"`
		Status         string `json:"status"`
		ConversationID string `json:"conversationId"`
		FlowrunID      string `json:"flowrunId"`
		Since          string `json:"since"`
		Until          string `json:"until"`
		Limit          int    `json:"limit"`
		Cursor         string `json:"cursor"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("search_function_executions: bad args: %w", err)
	}
	filter := functiondomain.ExecutionFilter{
		FunctionID:     args.FunctionID,
		VersionID:      args.VersionID,
		Status:         args.Status,
		ConversationID: args.ConversationID,
		FlowrunID:      args.FlowrunID,
		Limit:          args.Limit,
		Cursor:         args.Cursor,
	}
	if args.Since != "" {
		ts, err := time.Parse(time.RFC3339, args.Since)
		if err != nil {
			return "", fmt.Errorf("search_function_executions: since not RFC3339: %w", err)
		}
		filter.Since = &ts
	}
	if args.Until != "" {
		ts, err := time.Parse(time.RFC3339, args.Until)
		if err != nil {
			return "", fmt.Errorf("search_function_executions: until not RFC3339: %w", err)
		}
		filter.Until = &ts
	}
	res, err := t.svc.SearchExecutions(ctx, filter)
	if err != nil {
		return "", fmt.Errorf("search_function_executions: %w", err)
	}

	// Build previews: 200-byte snippets of input/output so the LLM sees scale
	// before pulling the full row.
	//
	// 构 200 字节 input/output 预览,LLM 一眼看规模再 drill。
	type previewRow struct {
		ID             string `json:"id"`
		Status         string `json:"status"`
		StartedAt      string `json:"startedAt"`
		ElapsedMs      int64  `json:"elapsedMs"`
		FunctionID     string `json:"functionId"`
		VersionID      string `json:"versionId"`
		InputPreview   string `json:"inputPreview"`
		OutputPreview  string `json:"outputPreview"`
		ErrorMessage   string `json:"errorMessage,omitempty"`
		ConversationID string `json:"conversationId,omitempty"`
	}
	previews := make([]previewRow, 0, len(res.Executions))
	for _, e := range res.Executions {
		previews = append(previews, previewRow{
			ID: e.ID, Status: e.Status, StartedAt: e.StartedAt.Format(time.RFC3339),
			ElapsedMs: e.ElapsedMs, FunctionID: e.FunctionID, VersionID: e.VersionID,
			InputPreview:   truncateJSON(e.Input, 200),
			OutputPreview:  truncateJSON(e.Output, 200),
			ErrorMessage:   e.ErrorMessage,
			ConversationID: e.ConversationID,
		})
	}

	out := map[string]any{
		"count":      res.Count,
		"executions": previews,
		"hasMore":    res.HasMore,
		"nextCursor": res.NextCursor,
		"aggregates": res.Aggregates,
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// truncateJSON marshals v to compact JSON and truncates to max bytes (with
// an ellipsis suffix if cut). Returns "" for nil.
//
// truncateJSON 把 v 序列化为紧凑 JSON 截到 max 字节(超长加…)。
func truncateJSON(v any, max int) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "…"
}

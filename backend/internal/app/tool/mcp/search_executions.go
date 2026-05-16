package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
)

type SearchMCPCalls struct {
	repo mcpdomain.CallRepository
}

func (t *SearchMCPCalls) Name() string { return "search_mcp_calls" }

func (t *SearchMCPCalls) Description() string {
	return "Search MCP tool call history. Filter by serverName / toolName / status / " +
		"conversationId / flowrunId. Returns previews (200-byte input/output snippets) + " +
		"aggregates (ok/failed/cancelled/timeout counts + avg/p95 elapsed_ms)."
}

func (t *SearchMCPCalls) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"serverName":     {"type": "string"},
			"toolName":       {"type": "string"},
			"status":         {"type": "string", "enum": ["ok","failed","cancelled","timeout"]},
			"conversationId": {"type": "string"},
			"flowrunId":      {"type": "string"},
			"limit":          {"type": "integer"},
			"cursor":         {"type": "string"}
		}
	}`)
}

func (t *SearchMCPCalls) IsReadOnly() bool        { return true }
func (t *SearchMCPCalls) NeedsReadFirst() bool    { return false }
func (t *SearchMCPCalls) RequiresWorkspace() bool { return false }
func (t *SearchMCPCalls) ValidateInput(json.RawMessage) error { return nil }
func (t *SearchMCPCalls) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *SearchMCPCalls) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ServerName, ToolName, Status, ConversationID, FlowrunID, Cursor string
		Limit                                                           int
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("search_mcp_calls: bad args: %w", err)
	}
	filter := mcpdomain.CallFilter{
		ServerName: args.ServerName, ToolName: args.ToolName,
		Status: args.Status, ConversationID: args.ConversationID,
		FlowrunID: args.FlowrunID, Limit: args.Limit, Cursor: args.Cursor,
	}
	rows, next, err := t.repo.ListCalls(ctx, filter)
	if err != nil {
		return "", fmt.Errorf("search_mcp_calls: %w", err)
	}
	agg, _ := t.repo.ComputeAggregates(ctx, filter)

	type preview struct {
		ID, Status, ServerName, ToolName, StartedAt, ErrorMessage string
		ElapsedMs                                                 int64
		InputPreview, OutputPreview                               string
	}
	previews := make([]preview, 0, len(rows))
	for _, r := range rows {
		previews = append(previews, preview{
			ID: r.ID, Status: r.Status,
			ServerName: r.ServerName, ToolName: r.ToolName,
			StartedAt: r.StartedAt.Format(time.RFC3339), ElapsedMs: r.ElapsedMs,
			ErrorMessage:  r.ErrorMessage,
			InputPreview:  truncateJSON(r.Input, 200),
			OutputPreview: truncateJSON(r.Output, 200),
		})
	}
	// Deterministic ordering for testability (newest first by StartedAt).
	sort.Slice(previews, func(i, j int) bool { return previews[i].StartedAt > previews[j].StartedAt })

	resp := map[string]any{
		"count":      len(previews),
		"calls":      previews,
		"nextCursor": next,
		"hasMore":    next != "",
		"aggregates": agg,
	}
	b, _ := json.Marshal(resp)
	return string(b), nil
}

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

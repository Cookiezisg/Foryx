package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
)

type SearchWorkflowExecutions struct {
	repo flowrundomain.Repository
}

func (t *SearchWorkflowExecutions) Name() string { return "search_workflow_executions" }

func (t *SearchWorkflowExecutions) Description() string {
	return "Search workflow node-execution history. Filters: flowrunId, nodeType, status, conversationId. Returns 200-byte input/output previews; use get_workflow_execution for full detail."
}

func (t *SearchWorkflowExecutions) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"flowrunId":      {"type": "string"},
			"nodeType":       {"type": "string"},
			"status":         {"type": "string", "enum": ["ok","failed","cancelled","timeout","skipped"]},
			"conversationId": {"type": "string"},
			"limit":          {"type": "integer"},
			"cursor":         {"type": "string"}
		}
	}`)
}

func (t *SearchWorkflowExecutions) IsReadOnly() bool        { return true }
func (t *SearchWorkflowExecutions) NeedsReadFirst() bool    { return false }
func (t *SearchWorkflowExecutions) RequiresWorkspace() bool { return false }
func (t *SearchWorkflowExecutions) ValidateInput(json.RawMessage) error { return nil }
func (t *SearchWorkflowExecutions) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *SearchWorkflowExecutions) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		FlowrunID      string `json:"flowrunId"`
		NodeType       string `json:"nodeType"`
		Status         string `json:"status"`
		ConversationID string `json:"conversationId"`
		Limit          int    `json:"limit"`
		Cursor         string `json:"cursor"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("search_workflow_executions: bad args: %w", err)
	}
	rows, next, err := t.repo.ListNodes(ctx, flowrundomain.NodeFilter{
		FlowrunID:      args.FlowrunID,
		NodeType:       args.NodeType,
		Status:         args.Status,
		ConversationID: args.ConversationID,
		Limit:          args.Limit,
		Cursor:         args.Cursor,
	})
	if err != nil {
		return "", fmt.Errorf("search_workflow_executions: %w", err)
	}

	type preview struct {
		ID, Status, NodeID, NodeType, StartedAt, ErrorMessage, FlowrunID string
		ElapsedMs                                                        int64
		Attempts                                                         int
		InputPreview, OutputPreview                                      string
	}
	out := make([]preview, 0, len(rows))
	for _, r := range rows {
		out = append(out, preview{
			ID: r.ID, Status: r.Status,
			NodeID: r.NodeID, NodeType: r.NodeType,
			StartedAt:     r.StartedAt.Format(time.RFC3339),
			ElapsedMs:     r.ElapsedMs,
			Attempts:      r.Attempts,
			InputPreview:  truncateJSON(r.Input, 200),
			OutputPreview: truncateJSON(r.Output, 200),
			ErrorMessage:  r.ErrorMessage,
			FlowrunID:     r.FlowrunID,
		})
	}
	resp := map[string]any{
		"count":      len(out),
		"executions": out,
		"nextCursor": next,
		"hasMore":    next != "",
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

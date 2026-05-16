package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
)

type SearchHandlerCalls struct {
	svc *handlerapp.Service
}

func (t *SearchHandlerCalls) Name() string { return "search_handler_calls" }

func (t *SearchHandlerCalls) Description() string {
	return "Search the handler call log. Filter by handlerId / versionId / method / " +
		"instanceId / ownerKind / status (ok|failed|cancelled|timeout) / conversationId / " +
		"flowrunId / since-until ISO8601. Returns 200-byte previews + aggregates (ok/" +
		"failed/cancelled/timeout counts + avg/p95 elapsed_ms). Use get_handler_call to " +
		"drill into full input/output by id."
}

func (t *SearchHandlerCalls) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"handlerId":      {"type": "string"},
			"versionId":      {"type": "string"},
			"method":         {"type": "string"},
			"instanceId":     {"type": "string"},
			"ownerKind":      {"type": "string"},
			"status":         {"type": "string", "enum": ["ok","failed","cancelled","timeout"]},
			"conversationId": {"type": "string"},
			"flowrunId":      {"type": "string"},
			"since":          {"type": "string"},
			"until":          {"type": "string"},
			"limit":          {"type": "integer"},
			"cursor":         {"type": "string"}
		}
	}`)
}

func (t *SearchHandlerCalls) IsReadOnly() bool        { return true }
func (t *SearchHandlerCalls) NeedsReadFirst() bool    { return false }
func (t *SearchHandlerCalls) RequiresWorkspace() bool { return false }

func (t *SearchHandlerCalls) ValidateInput(json.RawMessage) error { return nil }
func (t *SearchHandlerCalls) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *SearchHandlerCalls) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		HandlerID      string `json:"handlerId"`
		VersionID      string `json:"versionId"`
		Method         string `json:"method"`
		InstanceID     string `json:"instanceId"`
		OwnerKind      string `json:"ownerKind"`
		Status         string `json:"status"`
		ConversationID string `json:"conversationId"`
		FlowrunID      string `json:"flowrunId"`
		Since          string `json:"since"`
		Until          string `json:"until"`
		Limit          int    `json:"limit"`
		Cursor         string `json:"cursor"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("search_handler_calls: bad args: %w", err)
	}
	filter := handlerdomain.CallFilter{
		HandlerID:      args.HandlerID,
		VersionID:      args.VersionID,
		Method:         args.Method,
		InstanceID:     args.InstanceID,
		OwnerKind:      args.OwnerKind,
		Status:         args.Status,
		ConversationID: args.ConversationID,
		FlowrunID:      args.FlowrunID,
		Limit:          args.Limit,
		Cursor:         args.Cursor,
	}
	if args.Since != "" {
		ts, err := time.Parse(time.RFC3339, args.Since)
		if err != nil {
			return "", fmt.Errorf("search_handler_calls: since not RFC3339: %w", err)
		}
		filter.Since = &ts
	}
	if args.Until != "" {
		ts, err := time.Parse(time.RFC3339, args.Until)
		if err != nil {
			return "", fmt.Errorf("search_handler_calls: until not RFC3339: %w", err)
		}
		filter.Until = &ts
	}
	res, err := t.svc.SearchCalls(ctx, filter)
	if err != nil {
		return "", fmt.Errorf("search_handler_calls: %w", err)
	}

	type previewRow struct {
		ID             string `json:"id"`
		Status         string `json:"status"`
		StartedAt      string `json:"startedAt"`
		ElapsedMs      int64  `json:"elapsedMs"`
		HandlerID      string `json:"handlerId"`
		VersionID      string `json:"versionId"`
		Method         string `json:"method"`
		InstanceID     string `json:"instanceId,omitempty"`
		OwnerKind      string `json:"ownerKind,omitempty"`
		InputPreview   string `json:"inputPreview"`
		OutputPreview  string `json:"outputPreview"`
		ErrorMessage   string `json:"errorMessage,omitempty"`
		ConversationID string `json:"conversationId,omitempty"`
	}
	previews := make([]previewRow, 0, len(res.Calls))
	for _, c := range res.Calls {
		previews = append(previews, previewRow{
			ID: c.ID, Status: c.Status, StartedAt: c.StartedAt.Format(time.RFC3339),
			ElapsedMs: c.ElapsedMs, HandlerID: c.HandlerID, VersionID: c.VersionID,
			Method: c.Method, InstanceID: c.InstanceID, OwnerKind: c.OwnerKind,
			InputPreview:   truncateJSON(c.Input, 200),
			OutputPreview:  truncateJSON(c.Output, 200),
			ErrorMessage:   c.ErrorMessage,
			ConversationID: c.ConversationID,
		})
	}

	out := map[string]any{
		"count":      res.Count,
		"calls":      previews,
		"hasMore":    res.HasMore,
		"nextCursor": res.NextCursor,
		"aggregates": res.Aggregates,
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// truncateJSON marshals v to compact JSON, truncated to max bytes. Same as
// function's variant.
//
// truncateJSON marshal v compact JSON 截 max(跟 function 同)。
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

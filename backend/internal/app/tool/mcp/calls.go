package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
)

// calls.go gives the LLM the mcp_calls read surface — every other executable kind
// (function / handler / agent / trigger / flowrun) already pairs its execution log with
// search/get tools; MCP recorded calls but offered no way to read them back.
//
// calls.go 给 LLM 开 mcp_calls 读取面——其余可执行体（function/handler/agent/trigger/flowrun）
// 的执行日志都配了 search/get 工具；MCP 一直在记账却没有读回的口。

// --- search_mcp_calls --------------------------------------------------------

type SearchMCPCalls struct{ svc *mcpapp.Service }

func (t *SearchMCPCalls) Name() string { return "search_mcp_calls" }

func (t *SearchMCPCalls) Description() string {
	return "List an MCP server's tool-call history (most recent first) with an ok/failed rollup. Filter by tool name or status (ok|failed|cancelled|timeout). Use get_mcp_call on an id for the full record including logs."
}

func (t *SearchMCPCalls) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["serverId"],
		"properties": {
			"serverId": {"type": "string"},
			"tool": {"type": "string", "description": "Optional tool-name filter."},
			"status": {"type": "string", "description": "Optional: ok | failed | cancelled | timeout."},
			"limit": {"type": "integer", "description": "Page size (default 50)."},
			"cursor": {"type": "string", "description": "Opaque pagination cursor."}
		}
	}`)
}

func (t *SearchMCPCalls) ValidateInput(args json.RawMessage) error {
	var a struct {
		ServerID string `json:"serverId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("search_mcp_calls: bad args: %w", err)
	}
	if a.ServerID == "" {
		return ErrServerIDRequired
	}
	return nil
}

func (t *SearchMCPCalls) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ServerID string `json:"serverId"`
		Tool     string `json:"tool"`
		Status   string `json:"status"`
		Limit    int    `json:"limit"`
		Cursor   string `json:"cursor"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("search_mcp_calls: bad args: %w", err)
	}
	res, err := t.svc.SearchCalls(ctx, mcpdomain.CallFilter{
		ServerID: args.ServerID,
		Tool:     args.Tool,
		Status:   args.Status,
		Limit:    args.Limit,
		Cursor:   args.Cursor,
	})
	if err != nil {
		return "", fmt.Errorf("search_mcp_calls: %w", err)
	}
	return toolapp.ToJSON(res), nil
}

// --- get_mcp_call ------------------------------------------------------------

type GetMCPCall struct{ svc *mcpapp.Service }

func (t *GetMCPCall) Name() string { return "get_mcp_call" }

func (t *GetMCPCall) Description() string {
	return "Get one MCP tool-call record (input, output, error, logs, timing) by its id."
}

func (t *GetMCPCall) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["callId"],
		"properties": {"callId": {"type": "string"}}
	}`)
}

func (t *GetMCPCall) ValidateInput(args json.RawMessage) error {
	var a struct {
		CallID string `json:"callId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("get_mcp_call: bad args: %w", err)
	}
	if a.CallID == "" {
		return ErrCallIDRequired
	}
	return nil
}

func (t *GetMCPCall) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		CallID string `json:"callId"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("get_mcp_call: bad args: %w", err)
	}
	c, err := t.svc.GetCall(ctx, args.CallID)
	if err != nil {
		return "", fmt.Errorf("get_mcp_call: %w", err)
	}
	return toolapp.ToJSON(c), nil
}

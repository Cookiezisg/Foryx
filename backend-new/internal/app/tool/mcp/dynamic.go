package mcp

import (
	"context"
	"encoding/json"

	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	mcpinfra "github.com/sunweilin/forgify/backend/internal/infra/mcp"
)

// dynamicTool wraps one tool of one installed server as a standard tool.Tool. Name is
// "mcp__<server>__<tool>" (LLM tool names disallow ':'); Parameters is the server's own
// inputSchema verbatim; Execute forwards to Service.CallTool bound to the server's mcp_ id.
// danger is the LLM's per-call self-report — this adapter carries zero danger logic (S18).
//
// dynamicTool 把某 server 的一个工具包成标准 tool.Tool。Name 是 "mcp__<server>__<tool>"
// （LLM tool 名不许冒号）；Parameters 原样用 server 的 inputSchema；Execute 转发到绑定该 server
// mcp_ id 的 Service.CallTool。danger 由 LLM 逐次自报——本适配器零 danger 逻辑（S18）。
type dynamicTool struct {
	serverID    string // mcp_ id (closure-bound; Execute routes by it, not by name)
	serverName  string
	toolName    string
	description string
	schema      json.RawMessage
	svc         *mcpapp.Service
}

var _ toolapp.Tool = (*dynamicTool)(nil)

func (t *dynamicTool) Name() string                { return "mcp__" + t.serverName + "__" + t.toolName }
func (t *dynamicTool) Description() string         { return t.description }
func (t *dynamicTool) Parameters() json.RawMessage { return t.schema }

// ValidateInput defers to the MCP server's own validation (the upstream tool checks args).
//
// ValidateInput 交给 MCP server 自身校验（上游工具检查 args）。
func (t *dynamicTool) ValidateInput(json.RawMessage) error { return nil }

func (t *dynamicTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	// Stream the MCP server's progress notifications (if it emits any) live under this tool_call;
	// the final tool result is still the return value. nil-safe off a streamed turn (no-op → plain call).
	//
	// 把 MCP server 的进度通知（若发）实时流在本 tool_call 下；最终结果仍是返回值。非流式 turn 下 nil 安全
	// （no-op → 普通调用）。
	prog := loopapp.ToolProgress(ctx)
	defer prog.Close()
	ctx = mcpinfra.WithProgress(ctx, prog.Print)
	return t.svc.CallTool(ctx, t.serverID, t.toolName, json.RawMessage(argsJSON))
}

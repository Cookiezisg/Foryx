package scheduler

import (
	"context"
	"encoding/json"
	"fmt"

	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
)

// MCPDispatcher bridges workflow mcp nodes to mcpapp.Service.CallTool.
//
// MCPDispatcher 把 workflow mcp 节点桥接到 mcpapp.CallTool。
type MCPDispatcher struct {
	svc *mcpapp.Service
}

// NewMCPDispatcher constructs MCPDispatcher.
//
// NewMCPDispatcher 构造 MCPDispatcher。
func NewMCPDispatcher(svc *mcpapp.Service) *MCPDispatcher {
	return &MCPDispatcher{svc: svc}
}

// Dispatch reads serverName + tool + args and invokes the MCP tool.
//
// Dispatch 读 serverName + tool + args 并调用 MCP tool。
func (d *MCPDispatcher) Dispatch(ctx context.Context, in DispatchInput) DispatchOutput {
	serverName, _ := in.Node.Config["serverName"].(string)
	tool, _ := in.Node.Config["tool"].(string)
	if serverName == "" {
		return DispatchOutput{Error: fmt.Errorf("mcp node %q: serverName required", in.Node.ID)}
	}
	if tool == "" {
		return DispatchOutput{Error: fmt.Errorf("mcp node %q: tool required", in.Node.ID)}
	}
	args, _ := in.Node.Config["args"].(map[string]any)
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return DispatchOutput{Error: fmt.Errorf("mcp node %q: marshal args: %w", in.Node.ID, err)}
	}

	result, err := d.svc.CallTool(ctx, serverName, tool, argsJSON)
	if err != nil {
		return DispatchOutput{Error: err}
	}
	return DispatchOutput{Outputs: map[string]any{"out": result}}
}

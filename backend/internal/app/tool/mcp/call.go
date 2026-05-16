package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
)


var (
	// ErrEmptyServer: server missing or empty.
	//
	// ErrEmptyServer：server 缺失或为空。
	ErrEmptyServer = errors.New("server is required and must be non-empty")

	// ErrEmptyTool: tool missing or empty.
	//
	// ErrEmptyTool：tool 缺失或为空。
	ErrEmptyTool = errors.New("tool is required and must be non-empty")
)


const callMCPDescription = `Invoke a specific tool on a specific MCP server. Pair with search_mcp_tools: discover the tool + its inputSchema, then call_mcp_tool with matching server / tool / args. The args object must conform to the tool's inputSchema. On failure the result string carries the reason so you can adjust args, pick a different tool, or stop.`

var callMCPSchema = json.RawMessage(`{
	"type": "object",
	"required": ["server", "tool", "args"],
	"properties": {
		"server": {
			"type": "string",
			"description": "MCP server name (e.g. 'github', 'playwright', 'sqlite')."
		},
		"tool": {
			"type": "string",
			"description": "Tool name as returned by search_mcp (no 'mcp__' prefix)."
		},
		"args": {
			"type": "object",
			"description": "Arguments matching the tool's inputSchema. Use {} when the tool takes no arguments.",
			"additionalProperties": true
		}
	}
}`)


// CallMCP implements the call_mcp system tool.
//
// CallMCP 是 call_mcp 系统工具的实现。
type CallMCP struct {
	svc *mcpapp.Service
}

func (t *CallMCP) Name() string                { return "call_mcp_tool" }
func (t *CallMCP) Description() string         { return callMCPDescription }
func (t *CallMCP) Parameters() json.RawMessage { return callMCPSchema }

func (t *CallMCP) IsReadOnly() bool        { return false }
func (t *CallMCP) NeedsReadFirst() bool    { return false }
func (t *CallMCP) RequiresWorkspace() bool { return false }


func (t *CallMCP) ValidateInput(args json.RawMessage) error {
	var a struct {
		Server string `json:"server"`
		Tool   string `json:"tool"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("call_mcp.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.Server) == "" {
		return ErrEmptyServer
	}
	if strings.TrimSpace(a.Tool) == "" {
		return ErrEmptyTool
	}
	return nil
}

func (t *CallMCP) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}


// Execute dispatches to Service.CallTool; known sentinels map to LLM-friendly strings.
//
// Execute 派发到 Service.CallTool；已知 sentinel 映射到 LLM 友好字符串。
func (t *CallMCP) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Server string          `json:"server"`
		Tool   string          `json:"tool"`
		Args   json.RawMessage `json:"args"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("call_mcp.Execute: parse args: %w", err)
	}

	out, err := t.svc.CallTool(ctx, args.Server, args.Tool, args.Args)
	if err != nil {
		return mapCallToolErrorToFriendly(args.Server, args.Tool, err), nil
	}
	return out, nil
}

// mapCallToolErrorToFriendly converts mcpdomain sentinels to LLM-readable strings; unknown errors pass through.
//
// mapCallToolErrorToFriendly 把 mcpdomain sentinel 转成 LLM 可读字符串；未知错误原样透传。
func mapCallToolErrorToFriendly(server, tool string, err error) string {
	switch {
	case errors.Is(err, mcpdomain.ErrServerNotFound):
		return fmt.Sprintf("MCP server %q is not configured. Use search_mcp_tools to see available servers, or install %q via install_mcp_server.", server, server)
	case errors.Is(err, mcpdomain.ErrServerNotConnected):
		return fmt.Sprintf("MCP server %q is not connected.", server)
	case errors.Is(err, mcpdomain.ErrToolNotFound):
		return fmt.Sprintf("MCP tool %q does not exist on server %q. Use search_mcp_tools to discover the correct tool name.", tool, server)
	case errors.Is(err, mcpdomain.ErrToolCallTimeout):
		return fmt.Sprintf("MCP call %s/%s timed out. Try a more specific query or break the work into smaller steps.", server, tool)
	case errors.Is(err, mcpdomain.ErrToolCallFailed):
		return fmt.Sprintf("MCP call %s/%s failed: %s", server, tool, err.Error())
	default:
		return fmt.Sprintf("call_mcp %s/%s failed: %s", server, tool, err.Error())
	}
}


var _ toolapp.Tool = (*CallMCP)(nil)

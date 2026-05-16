// Package mcp provides MCP system tools — search/call installed servers + the curated marketplace flow.
//
// Package mcp 提供 MCP 系统工具——搜索 / 调用已装 server + curated marketplace 流程。
package mcp

import (
	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
)

// MCPTools constructs the MCP system tools sharing one Service.
//
// MCPTools 用一个 Service 构造 MCP 系统工具。
func MCPTools(svc *mcpapp.Service) []toolapp.Tool {
	return []toolapp.Tool{
		&SearchMCP{svc: svc},
		&CallMCP{svc: svc},
		&ListMCPMarketplace{svc: svc},
		&InstallMCPServer{svc: svc},
		&UninstallMCPServer{svc: svc},
	}
}

// MCPCallLogTools constructs the call-log tools (search/get) wired with the Repository.
//
// MCPCallLogTools 用 Repository 装配调用日志工具。
func MCPCallLogTools(repo mcpdomain.CallRepository) []toolapp.Tool {
	return []toolapp.Tool{
		&SearchMCPCalls{repo: repo},
		&GetMCPCall{repo: repo},
	}
}

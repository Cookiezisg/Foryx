// searchrouter.go — adapter that lets WebSearch (app/tool/web) route to
// the duckduckgo-search MCP server when it's installed and connected.
// Lives in app/mcp so the MCPSearchRouter port (defined in app/tool/web)
// is satisfied via composition without web ↔ mcp mutual import.
//
// Hardcoded server name "duckduckgo-search" matches the marketplace
// registry entry; the tool name "search" matches what
// duckduckgo-mcp-server exposes. Future: capability-based discovery
// (mcp.RegistryEntry.Capabilities = ["search"]) when there's a second
// search MCP in play.
//
// searchrouter.go ——让 WebSearch（app/tool/web）在 duckduckgo-search MCP
// server 已装且连接时路由过去的 adapter。住 app/mcp 让 web 包定义的
// MCPSearchRouter 端口经组合满足，避免 web ↔ mcp 互相 import。
//
// server 名 "duckduckgo-search" 硬编码匹配 marketplace 条目；tool 名 "search"
// 匹配 duckduckgo-mcp-server 暴露的工具。将来真有第二个搜索 MCP 时再升级到
// capability-based 发现（mcp.RegistryEntry.Capabilities = ["search"]）。
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
)

// V1SearchServerName is the hardcoded MCP server name we route WebSearch
// to. Lives next to the registry entry so renaming surfaces here too.
//
// V1SearchServerName 是 WebSearch 路由的硬编码 MCP server 名。住此处让
// 改名时此处也暴露。
const V1SearchServerName = "duckduckgo-search"

// V1SearchToolName is the tool the duckduckgo-search MCP server exposes.
// duckduckgo-mcp-server (Python) names it "search".
//
// V1SearchToolName 是 duckduckgo-search MCP server 暴露的工具名。
// duckduckgo-mcp-server（Python）命名为 "search"。
const V1SearchToolName = "search"

// ErrSearchServerUnavailable is returned when no MCP server matching
// V1SearchServerName is configured or its current Status is not
// callable. Web tool's MCPSearchRouter port wraps this into its own
// ErrMCPSearchUnavailable so web doesn't import mcp domain types.
//
// ErrSearchServerUnavailable 在无配置 / 不可调用时返。web 工具的
// MCPSearchRouter 端口包成自己的 ErrMCPSearchUnavailable，避免 web 导入
// mcp domain 类型。
var ErrSearchServerUnavailable = errors.New("mcp: duckduckgo-search server not configured or not connected")

// SearchRouter wraps Service.CallTool to satisfy app/tool/web's
// MCPSearchRouter port. Constructed in main.go; web tool gets the
// interface, never the *mcp.Service struct.
//
// SearchRouter 包 Service.CallTool 满足 app/tool/web 的 MCPSearchRouter
// 端口。main.go 构造；web 工具拿接口，不见 *mcp.Service。
type SearchRouter struct {
	svc *Service
}

// NewSearchRouter constructs a router around svc.
//
// NewSearchRouter 围 svc 构造路由。
func NewSearchRouter(svc *Service) *SearchRouter {
	return &SearchRouter{svc: svc}
}

// CallSearchTool checks duckduckgo-search server is callable, then sends
// {query, max_results} via Service.CallTool. Returns the raw tool result
// string for the caller to JSON-parse.
//
// CallSearchTool 检查 duckduckgo-search server 可调，然后经 Service.CallTool
// 发 {query, max_results}。返原始 tool result string 供调用方 JSON 解析。
func (r *SearchRouter) CallSearchTool(ctx context.Context, query string, limit int) (string, error) {
	st, err := r.svc.GetServer(ctx, V1SearchServerName)
	if err != nil {
		// ErrServerNotFound = not configured at all.
		// ErrServerNotFound = 完全未配置。
		return "", ErrSearchServerUnavailable
	}
	if !mcpdomain.IsCallable(st.Status) {
		return "", fmt.Errorf("mcpapp.SearchRouter.CallSearchTool: %w (status=%s)", ErrSearchServerUnavailable, st.Status)
	}
	// Marshal of {string, int} map literal cannot fail at runtime; ignore err.
	// 字符串/int 字面量 map Marshal 不会运行时失败；忽略 err。
	args, _ := json.Marshal(map[string]any{
		"query":       query,
		"max_results": limit,
	})
	return r.svc.CallTool(ctx, V1SearchServerName, V1SearchToolName, args)
}

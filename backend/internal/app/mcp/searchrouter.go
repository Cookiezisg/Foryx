package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
)

const (
	V1SearchServerName = "duckduckgo-search"
	V1SearchToolName   = "search"
)

// ErrSearchServerUnavailable signals the search server is absent or unreachable.
//
// ErrSearchServerUnavailable 表示搜索 server 未配置或不可调用。
var ErrSearchServerUnavailable = errors.New("mcp: duckduckgo-search server not configured or not connected")

// SearchRouter wraps Service.CallTool for app/tool/web's MCPSearchRouter port.
//
// SearchRouter 包装 Service.CallTool 以满足 app/tool/web 的 MCPSearchRouter 端口。
type SearchRouter struct {
	svc *Service
}

// NewSearchRouter constructs a router around svc.
//
// NewSearchRouter 围绕 svc 构造 SearchRouter。
func NewSearchRouter(svc *Service) *SearchRouter {
	return &SearchRouter{svc: svc}
}

// CallSearchTool routes a search query to the duckduckgo-search MCP server.
//
// CallSearchTool 把查询转发到 duckduckgo-search MCP server。
func (r *SearchRouter) CallSearchTool(ctx context.Context, query string, limit int) (string, error) {
	st, err := r.svc.GetServer(ctx, V1SearchServerName)
	if err != nil {
		return "", ErrSearchServerUnavailable
	}
	if !mcpdomain.IsCallable(st.Status) {
		return "", fmt.Errorf("mcpapp.SearchRouter.CallSearchTool: %w (status=%s)", ErrSearchServerUnavailable, st.Status)
	}
	args, _ := json.Marshal(map[string]any{
		"query":       query,
		"max_results": limit,
	})
	return r.svc.CallTool(ctx, V1SearchServerName, V1SearchToolName, args)
}

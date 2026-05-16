// Package web provides network-facing system tools (WebFetch + WebSearch) sharing an SSRF guard.
//
// Package web 提供网络相关 system tool（WebFetch + WebSearch），共用 SSRF 守卫。
package web

import (
	"net/http"

	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// WebTools constructs the web system tools; mcpRouter is optional (nil disables MCP routing).
//
// WebTools 构造 web system tool；mcpRouter 可空（nil 关闭 MCP 路由）。
func WebTools(
	picker modeldomain.ModelPicker,
	keys apikeydomain.KeyProvider,
	factory *llminfra.Factory,
	mcpRouter MCPSearchRouter,
	log *zap.Logger,
) []toolapp.Tool {
	return []toolapp.Tool{
		newWebFetch(picker, keys, factory),
		newWebSearch(keys, mcpRouter, log),
	}
}

func newWebFetch(
	picker modeldomain.ModelPicker,
	keys apikeydomain.KeyProvider,
	factory *llminfra.Factory,
) *WebFetch {
	return &WebFetch{
		picker:  picker,
		keys:    keys,
		factory: factory,
	}
}

func newWebSearch(keys apikeydomain.KeyProvider, mcpRouter MCPSearchRouter, log *zap.Logger) *WebSearch {
	return &WebSearch{
		httpClient: &http.Client{Timeout: searchTimeout},
		keys:       keys,
		mcpRouter:  mcpRouter,
		log:        log,
	}
}

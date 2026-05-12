package router

import (
	"net/http"

	handlershttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/handlers"
	middlewarehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/middleware"
)

// New builds the complete HTTP handler: routes + middleware chain +
// 404 fallback. main.go calls this once and hands the result to http.Server.
//
// Chain order on the wire (outermost first):
//
//	Recover → RequestLogger → CORS → InjectLocale → InjectUserID → mux
//
// Recover outermost catches any inner panic. RequestLogger next so the
// access log captures 500s from Recover. CORS / locale / userID are
// innermost so preflight OPTIONS (terminates inside CORS) doesn't need them.
//
// New 构造完整的 HTTP handler：路由 + 中间件链 + 404 兜底。
// main.go 只调一次，结果交给 http.Server。
//
// 链序（从外到内）：Recover → RequestLogger → CORS → InjectLocale →
// InjectUserID → mux。Recover 在最外层捕 panic；RequestLogger 在其内层
// 让 Recover 写的 500 也能被日志记录；CORS/locale/userID 在最内层，
// 因 preflight OPTIONS 在 CORS 层就结束，不需要它们。
func New(deps Deps) http.Handler {
	mux := http.NewServeMux()

	// Each handler registers its own routes.
	// 每个 handler 注册自己的路由。
	handlershttpapi.NewHealthHandler().Register(mux)
	handlershttpapi.NewProvidersHandler().Register(mux)
	if deps.APIKeyService != nil {
		handlershttpapi.NewAPIKeyHandler(deps.APIKeyService, deps.Log).Register(mux)
	}
	if deps.ModelService != nil {
		handlershttpapi.NewModelConfigHandler(deps.ModelService, deps.Log).Register(mux)
	}
	if deps.ConversationService != nil {
		handlershttpapi.NewConversationHandler(deps.ConversationService, deps.Log).Register(mux)
	}
	if deps.FunctionService != nil {
		handlershttpapi.NewFunctionHandler(deps.FunctionService, deps.Log).Register(mux)
	}
	if deps.HandlerService != nil {
		handlershttpapi.NewHandlerHandler(deps.HandlerService, deps.Log).Register(mux)
	}
	if deps.WorkflowService != nil {
		handlershttpapi.NewWorkflowHandler(deps.WorkflowService, deps.Log).Register(mux)
	}
	if deps.ChatService != nil {
		handlershttpapi.NewChatHandler(deps.ChatService, deps.Log).Register(mux)
	}
	if deps.EventLogBridge != nil {
		handlershttpapi.NewEventLogHandler(deps.EventLogBridge, deps.BlockV2Repo, deps.Log).Register(mux)
	}
	if deps.NotificationsBridge != nil {
		handlershttpapi.NewNotificationsHandler(deps.NotificationsBridge, deps.Log).Register(mux)
	}
	if deps.ForgeBridge != nil {
		handlershttpapi.NewForgeHandler(deps.ForgeBridge, deps.Log).Register(mux)
	}
	if deps.AskService != nil {
		handlershttpapi.NewAnswerHandler(deps.AskService, deps.Log).Register(mux)
	}
	if deps.SandboxService != nil {
		handlershttpapi.NewSandboxHandler(deps.SandboxService, deps.Log).Register(mux)
	}
	// SubagentService no longer registers HTTP routes — sub-run data lives
	// in the unified messages/message_blocks tables and is observed via
	// the eventlog SSE stream + standard chat message endpoints.
	//
	// SubagentService 不再注册 HTTP 路由——sub-run 数据在统一 messages/
	// message_blocks 表，经 eventlog SSE 流 + 标准 chat message 端点观测。
	_ = deps.SubagentService
	if deps.MCPService != nil {
		handlershttpapi.NewMCPHandler(deps.MCPService, deps.Log).Register(mux)
	}
	if deps.SkillService != nil {
		handlershttpapi.NewSkillsHandler(deps.SkillService, deps.Log).Register(mux)
	}
	if deps.CatalogService != nil {
		handlershttpapi.NewCatalogHandler(deps.CatalogService, deps.Log).Register(mux)
	}
	if deps.Dev {
		handlershttpapi.NewDevHandler(deps.DB, deps.LogBroadcaster, deps.CollectionsDir, deps.IntegrationDir, deps.ForgifyHome, deps.Port, deps.Tools, deps.LLMFactory, deps.ShellManager, deps.Log).Register(mux)
	}

	// 404 fallback — must be last so specific routes take precedence.
	// 404 兜底——必须最后，让具体路由优先。
	mux.HandleFunc("/", middlewarehttpapi.NotFound)

	return applyChain(mux, deps)
}

// applyChain wraps h with the full middleware chain. Inside-out: the
// outermost middleware (Recover) is applied last so it runs first per request.
//
// applyChain 用完整中间件链包裹 h。从内向外：最外层中间件（Recover）
// 最后应用，因而在每次请求中最先运行。
func applyChain(h http.Handler, deps Deps) http.Handler {
	h = middlewarehttpapi.InjectUserID(h) // innermost / 最内层
	h = middlewarehttpapi.InjectLocale(h)
	h = middlewarehttpapi.CORS(middlewarehttpapi.DefaultCORSConfig())(h)
	h = middlewarehttpapi.RequestLogger(deps.Log)(h)
	h = middlewarehttpapi.Recover(deps.Log)(h) // outermost / 最外层
	return h
}

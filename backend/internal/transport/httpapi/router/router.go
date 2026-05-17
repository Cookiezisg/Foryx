package router

import (
	"net/http"

	handlershttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/handlers"
	middlewarehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/middleware"
)

// New assembles routes + middleware (Recover → Logger → CORS → Locale → UserID → mux).
//
// New 装配路由 + 中间件链(Recover → Logger → CORS → Locale → UserID → mux)。
func New(deps Deps) http.Handler {
	mux := deps.Mux
	if mux == nil {
		mux = http.NewServeMux()
	}

	handlershttpapi.NewHealthHandler().Register(mux)
	handlershttpapi.NewProvidersHandler().Register(mux)
	if deps.APIKeyService != nil {
		handlershttpapi.NewAPIKeyHandler(deps.APIKeyService, deps.Log).Register(mux)
	}
	if deps.ModelService != nil {
		handlershttpapi.NewModelConfigHandler(deps.ModelService, deps.Log).Register(mux)
	}
	if deps.ConversationService != nil {
		// ChatService is the TokenSummer (V1.2 §4.1); nil-tolerant.
		// ChatService 即 TokenSummer（§4.1）；nil 安全。
		var tokens handlershttpapi.TokenSummer
		if deps.ChatService != nil {
			tokens = deps.ChatService
		}
		convH := handlershttpapi.NewConversationHandler(deps.ConversationService, tokens, deps.Log)
		// §18.2 system prompt preview: ChatService implements SystemPromptPreviewer; nil-safe.
		if deps.ChatService != nil {
			convH.SetSystemPromptPreviewer(deps.ChatService)
		}
		convH.Register(mux)
	}
	if deps.FunctionService != nil {
		handlershttpapi.NewFunctionHandler(deps.FunctionService, deps.Log).Register(mux)
	}
	if deps.HandlerService != nil {
		handlershttpapi.NewHandlerHandler(deps.HandlerService, deps.Log).Register(mux)
	}
	var wfH *handlershttpapi.WorkflowHandler
	if deps.WorkflowService != nil {
		wfH = handlershttpapi.NewWorkflowHandler(deps.WorkflowService, deps.Log)
		wfH.Register(mux)
	}
	if deps.FlowRunRepo != nil {
		frH := handlershttpapi.NewFlowRunHandler(deps.FlowRunRepo, deps.SchedulerService, deps.TriggerService, deps.Log)
		frH.Register(mux)
		if wfH != nil {
			wfH.AttachFlowRunHandler(frH)
		}
	}
	if deps.ChatService != nil {
		handlershttpapi.NewChatHandler(deps.ChatService, deps.Log).Register(mux)
		// /api/v1/usage piggy-backs on chat's SumTokens methods (V1.2 §4.2).
		// /api/v1/usage 复用 chat 的 SumTokens 方法（§4.2）。
		handlershttpapi.NewUsageHandler(deps.ChatService, deps.Log).Register(mux)
	}
	// V1.2 §3 final-sweep — permissions + settings 5 endpoints.
	// 全 nil 时 group 整组跳。
	if deps.SettingsService != nil && deps.PermGate != nil {
		handlershttpapi.NewPermissionsHandler(
			deps.SettingsService, deps.PermGate, deps.SettingsPath, deps.Tools, deps.Log,
		).Register(mux)
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
	if deps.MemoryService != nil {
		handlershttpapi.NewMemoryHandler(deps.MemoryService, deps.Log).Register(mux)
	}
	if deps.DocumentService != nil {
		handlershttpapi.NewDocumentHandler(deps.DocumentService, deps.Log).Register(mux)
	}
	if deps.UserService != nil {
		handlershttpapi.NewUsersHandler(deps.UserService, deps.Log).Register(mux)
	}
	// §4.8 context-stats — needs conv + tokensummer; nil-safe degradation.
	if deps.ConversationService != nil {
		handlershttpapi.NewContextStatsHandler(
			deps.ConversationService,
			deps.CatalogService,
			deps.MemoryService,
			deps.DocumentService,
			deps.ChatService,
			deps.Log,
		).Register(mux)
	}
	// §4.5 metrics: register when at least one execution-log repo is wired.
	if deps.FunctionExecRepo != nil || deps.HandlerCallRepo != nil || deps.MCPCallRepo != nil || deps.SkillExecRepo != nil {
		handlershttpapi.NewMetricsHandler(
			deps.FunctionExecRepo, deps.HandlerCallRepo, deps.MCPCallRepo, deps.SkillExecRepo,
			deps.Log,
		).Register(mux)
	}
	if deps.Dev {
		handlershttpapi.NewDevHandler(deps.DB, deps.LogBroadcaster, deps.CollectionsDir, deps.IntegrationDir, deps.ForgifyHome, deps.Port, deps.Tools, deps.LLMFactory, deps.ShellManager, deps.Log).Register(mux)
		// §18.1 prompt inventory — dev-only audit endpoint.
		handlershttpapi.NewPromptsHandler(deps.Tools, deps.SubagentRegistry, deps.Log).Register(mux)
	}

	mux.HandleFunc("/", middlewarehttpapi.NotFound)

	return applyChain(mux, deps)
}

func applyChain(h http.Handler, deps Deps) http.Handler {
	// Multi-user middleware reads X-Forgify-User-ID; if UserService nil (early boot / tests), falls back to legacy default.
	// 多用户中间件读 X-Forgify-User-ID;UserService nil（早期 boot / 测试）走 legacy 默认。
	if deps.UserService != nil {
		h = middlewarehttpapi.InjectUserIDWith(deps.UserService)(h)
	} else {
		h = middlewarehttpapi.InjectUserID(h)
	}
	h = middlewarehttpapi.InjectLocale(h)
	h = middlewarehttpapi.CORS(middlewarehttpapi.DefaultCORSConfig())(h)
	h = middlewarehttpapi.RequestLogger(deps.Log)(h)
	h = middlewarehttpapi.Recover(deps.Log)(h)
	return h
}

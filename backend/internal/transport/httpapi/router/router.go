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
		handlershttpapi.NewConversationHandler(deps.ConversationService, deps.Log).Register(mux)
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
	if deps.Dev {
		handlershttpapi.NewDevHandler(deps.DB, deps.LogBroadcaster, deps.CollectionsDir, deps.IntegrationDir, deps.ForgifyHome, deps.Port, deps.Tools, deps.LLMFactory, deps.ShellManager, deps.Log).Register(mux)
	}

	mux.HandleFunc("/", middlewarehttpapi.NotFound)

	return applyChain(mux, deps)
}

func applyChain(h http.Handler, deps Deps) http.Handler {
	h = middlewarehttpapi.InjectUserID(h)
	h = middlewarehttpapi.InjectLocale(h)
	h = middlewarehttpapi.CORS(middlewarehttpapi.DefaultCORSConfig())(h)
	h = middlewarehttpapi.RequestLogger(deps.Log)(h)
	h = middlewarehttpapi.Recover(deps.Log)(h)
	return h
}

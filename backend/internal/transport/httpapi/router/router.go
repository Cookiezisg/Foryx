package router

import (
	"net/http"
	"strings"

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

	rec := NewRecorder(mux)

	handlershttpapi.NewHealthHandler().Register(rec)
	handlershttpapi.NewProvidersHandler().Register(rec)
	handlershttpapi.NewScenariosHandler().Register(rec)
	if deps.APIKeyService != nil {
		handlershttpapi.NewAPIKeyHandler(deps.APIKeyService, deps.Log).Register(rec)
	}
	if deps.ModelService != nil {
		handlershttpapi.NewModelConfigHandler(deps.ModelService, deps.Log).Register(rec)
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
		convH.Register(rec)
	}
	if deps.FunctionService != nil {
		fh := handlershttpapi.NewFunctionHandler(deps.FunctionService, deps.Log)
		fh.SetSpawner(deps.AskAISpawner)
		fh.Register(rec)
	}
	if deps.HandlerService != nil {
		hh := handlershttpapi.NewHandlerHandler(deps.HandlerService, deps.Log)
		hh.SetSpawner(deps.AskAISpawner)
		hh.Register(rec)
	}
	var wfH *handlershttpapi.WorkflowHandler
	if deps.WorkflowService != nil {
		wfH = handlershttpapi.NewWorkflowHandler(deps.WorkflowService, deps.Log)
		wfH.SetSpawner(deps.AskAISpawner)
		wfH.Register(rec)
	}
	if deps.FlowRunRepo != nil {
		frH := handlershttpapi.NewFlowRunHandler(deps.FlowRunRepo, deps.SchedulerService, deps.TriggerService, deps.Log)
		frH.SetAskAI(deps.AskAISpawner, deps.WorkflowService)
		frH.Register(rec)
		if wfH != nil {
			wfH.AttachFlowRunHandler(frH)
		}
	}
	if deps.ChatService != nil {
		handlershttpapi.NewChatHandler(deps.ChatService, deps.Log).Register(rec)
		// /api/v1/usage piggy-backs on chat's SumTokens methods (V1.2 §4.2).
		// /api/v1/usage 复用 chat 的 SumTokens 方法（§4.2）。
		handlershttpapi.NewUsageHandler(deps.ChatService, deps.Log).Register(rec)
	}
	// V1.2 §3 final-sweep — permissions + settings 5 endpoints.
	// 全 nil 时 group 整组跳。
	if deps.SettingsService != nil && deps.PermGate != nil {
		handlershttpapi.NewPermissionsHandler(
			deps.SettingsService, deps.PermGate, deps.SettingsPath, deps.Tools, deps.Log,
		).Register(rec)
	}
	if deps.EventLogBridge != nil {
		handlershttpapi.NewEventLogHandler(deps.EventLogBridge, deps.BlockV2Repo, deps.Log).Register(rec)
	}
	if deps.NotificationsBridge != nil {
		handlershttpapi.NewNotificationsHandler(deps.NotificationsBridge, deps.Log).Register(rec)
	}
	if deps.ForgeBridge != nil {
		handlershttpapi.NewForgeHandler(deps.ForgeBridge, deps.Log).Register(rec)
	}
	if deps.AskService != nil {
		handlershttpapi.NewAnswerHandler(deps.AskService, deps.Log).Register(rec)
	}
	if deps.SandboxService != nil {
		handlershttpapi.NewSandboxHandler(deps.SandboxService, deps.Log).Register(rec)
	}
	_ = deps.SubagentService
	if deps.MCPService != nil {
		handlershttpapi.NewMCPHandler(deps.MCPService, deps.Log).Register(rec)
	}
	if deps.SkillService != nil {
		handlershttpapi.NewSkillsHandler(deps.SkillService, deps.Log).Register(rec)
	}
	if deps.CatalogService != nil {
		handlershttpapi.NewCatalogHandler(deps.CatalogService, deps.Log).Register(rec)
	}
	if deps.MemoryService != nil {
		handlershttpapi.NewMemoryHandler(deps.MemoryService, deps.Log).Register(rec)
	}
	if deps.DocumentService != nil {
		dh := handlershttpapi.NewDocumentHandler(deps.DocumentService, deps.Log)
		dh.SetSpawner(deps.AskAISpawner)
		dh.Register(rec)
	}
	if deps.RelationService != nil {
		handlershttpapi.NewRelationHandler(deps.RelationService, deps.Log).Register(rec)
	}
	if deps.UserService != nil {
		handlershttpapi.NewUsersHandler(deps.UserService, deps.Log).Register(rec)
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
		).Register(rec)
	}
	// §4.5 metrics: register when at least one execution-log repo is wired.
	if deps.FunctionExecRepo != nil || deps.HandlerCallRepo != nil || deps.MCPCallRepo != nil || deps.SkillExecRepo != nil {
		handlershttpapi.NewMetricsHandler(
			deps.FunctionExecRepo, deps.HandlerCallRepo, deps.MCPCallRepo, deps.SkillExecRepo,
			deps.Log,
		).Register(rec)
	}
	if deps.Dev {
		handlershttpapi.NewDevHandler(deps.DB, deps.LogBroadcaster, deps.TestendDir, deps.ForgifyHome, deps.Port, deps.LLMFactory, deps.ShellManager, deps.Log, NewRecorderAdapter(rec)).Register(rec)
		// §18.1 prompt inventory — dev-only audit endpoint.
		handlershttpapi.NewPromptsHandler(deps.Tools, deps.SubagentRegistry, deps.Log).Register(rec)
	}

	mux.HandleFunc("/", middlewarehttpapi.NotFound)

	return applyChain(mux, deps)
}

// requireUserExempt wraps RequireUser around all /api/v1/* routes EXCEPT:
//   - /api/v1/users (onboarding must call POST /users before any user exists)
//   - /api/v1/health (liveness probe)
//   - /api/v1/providers + /api/v1/scenarios (static metadata; onboarding's
//     provider grid + Config Model tab need them readable pre-user)
//   - non-/api/v1/* paths (let mux handle NotFound / static assets / etc.)
//
// requireUserExempt:/api/v1/users、/health、/providers、/scenarios 不走
// RequireUser;前两者 onboarding 创号用,后两者是静态白名单,onboarding
// 还没建 user 时也得能拉到 provider/scenario 列表。
// 非 /api/v1/* 路径(如 NotFound)也放过,让 mux 处理。
func requireUserExempt(next http.Handler) http.Handler {
	guarded := middlewarehttpapi.RequireUser(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if !strings.HasPrefix(p, "/api/v1/") ||
			strings.HasPrefix(p, "/api/v1/users") ||
			p == "/api/v1/health" ||
			p == "/api/v1/providers" ||
			p == "/api/v1/scenarios" {
			next.ServeHTTP(w, r)
			return
		}
		guarded.ServeHTTP(w, r)
	})
}

func applyChain(h http.Handler, deps Deps) http.Handler {
	// IdentifyUser stamps ctx with X-Forgify-User-ID (validated) or leaves
	// ctx empty; RequireUser 401s if no user in ctx. /users CRUD and
	// /health are exempt — they must work pre-onboarding.
	//
	// Explicit nil-interface assignment dodges the typed-nil gotcha: a
	// nil *userapp.Service stuffed into a UserResolver interface compares
	// != nil, so we'd skip the "no resolver" branch and panic in .Get.
	//
	// IdentifyUser 校验 header 后入 ctx;RequireUser 强制非空(401);
	// /users 与 /health 例外,onboarding 前必须可达。显式 nil 接口
	// 赋值避开 Go typed-nil 坑。
	var resolver middlewarehttpapi.UserResolver
	if deps.UserService != nil {
		resolver = deps.UserService
	}
	h = requireUserExempt(h)
	h = middlewarehttpapi.IdentifyUser(resolver)(h)
	h = middlewarehttpapi.InjectLocale(h)
	h = middlewarehttpapi.CORS(middlewarehttpapi.DefaultCORSConfig())(h)
	h = middlewarehttpapi.RequestLogger(deps.Log)(h)
	h = middlewarehttpapi.Recover(deps.Log)(h)
	return h
}

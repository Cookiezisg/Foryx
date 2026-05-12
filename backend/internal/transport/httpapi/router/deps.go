// Package router assembles HTTP routes and the middleware chain into a
// single http.Handler.
//
// Package router 把 HTTP 路由和中间件链组装成一个 http.Handler。
package router

import (
	"go.uber.org/zap"
	"gorm.io/gorm"

	apikeyapp "github.com/sunweilin/forgify/backend/internal/app/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	notificationsdomain "github.com/sunweilin/forgify/backend/internal/domain/notifications"
	askapp "github.com/sunweilin/forgify/backend/internal/app/ask"
	chatapp "github.com/sunweilin/forgify/backend/internal/app/chat"
	convapp "github.com/sunweilin/forgify/backend/internal/app/conversation"
	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	catalogapp "github.com/sunweilin/forgify/backend/internal/app/catalog"
	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	skillapp "github.com/sunweilin/forgify/backend/internal/app/skill"
	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	shelltool "github.com/sunweilin/forgify/backend/internal/app/tool/shell"
	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	loggerinfra "github.com/sunweilin/forgify/backend/internal/infra/logger"
)

// Deps bundles everything the HTTP transport layer needs. Constructed
// once in main.go and handed to router.New. Per-domain service fields
// are nil-tolerant — router.New only registers a domain's routes when
// its service is non-nil, so integration tests can stay narrow.
//
// Deps 聚合 HTTP transport 层需要的全部依赖。main.go 里一次性构造后
// 交给 router.New。各 domain 的 service 字段容忍 nil——router.New 仅在
// service 非 nil 时注册对应路由，让集成测试可保持窄切片。
type Deps struct {
	Log *zap.Logger

	// APIKeyService implements CRUD + KeyProvider for /api/v1/api-keys/*.
	// APIKeyService 为 /api/v1/api-keys/* 提供 CRUD + KeyProvider。
	APIKeyService *apikeyapp.Service

	// ModelService implements CRUD + ModelPicker for /api/v1/model-configs/*.
	// ModelService 为 /api/v1/model-configs/* 提供 CRUD + ModelPicker。
	ModelService *modelapp.Service

	// ConversationService implements CRUD for /api/v1/conversations/*.
	// ConversationService 为 /api/v1/conversations/* 提供 CRUD。
	ConversationService *convapp.Service

	// FunctionService manages the user's Python function library (CRUD,
	// versions, sandbox execution). Forge_redesign trinity domain — replaces
	// the prior ForgeService.
	//
	// FunctionService 管理用户的 Python function 库(CRUD、版本、沙箱执行)。
	// forge_redesign trinity 域,替代历史 ForgeService。
	FunctionService *functionapp.Service

	// HandlerService manages the user's Python handler library (CRUD,
	// versions, sandbox-spawned long-lived instances, AES-GCM init args
	// config). Trinity second leg.
	//
	// HandlerService 管理用户 Python handler 库(CRUD、版本、sandbox 起的
	// 长跑 instance、AES-GCM init args config)。Trinity 第二条腿。
	HandlerService *handlerapp.Service

	// WorkflowService manages the user's DAG workflow library (CRUD,
	// versions, ops engine, validation). Plan 04 / trinity third leg.
	// Trigger + execution plane live in Plan 05 (scheduler / trigger /
	// flowrun).
	//
	// WorkflowService 管理用户 DAG workflow 库(CRUD、版本、ops 引擎、校验)。
	// Plan 04 / trinity 第三条腿;trigger + 执行在 Plan 05。
	WorkflowService *workflowapp.Service

	// ChatService implements messaging, attachment upload, and Agent streaming.
	// ChatService 实现消息收发、附件上传和 Agent 流式输出。
	ChatService *chatapp.Service

	// EventLogBridge is the recursive-event-log Bridge backing the new
	// /api/v1/eventlog SSE endpoint. Phase 1 wiring: present alongside
	// EventsBridge but no producer publishes to it yet (Phase 2 cuts
	// chat / subagent / tools over).
	//
	// EventLogBridge 是递归事件日志 Bridge，背后 /api/v1/eventlog SSE 端点。
	// Phase 1 接线：与 EventsBridge 并存，暂无 producer 推（Phase 2 切
	// chat / subagent / tools）。
	EventLogBridge eventlogdomain.Bridge

	// BlockV2Repo backs the /api/v1/conversations/{id}/eventlog HTTP
	// refetch endpoint. Optional — when nil only the SSE stream is
	// served (no history refetch).
	//
	// BlockV2Repo 给 /api/v1/conversations/{id}/eventlog HTTP refetch
	// 端点。可选——nil 时只服务 SSE 流（无历史 refetch）。
	BlockV2Repo chatdomain.Repository

	// NotificationsBridge backs the per-user /api/v1/notifications SSE
	// endpoint (entity-update stream: conversation rename, todo CRUD,
	// trinity entity actions, future mcp/skill/system events).
	//
	// NotificationsBridge 支撑 per-user /api/v1/notifications SSE 端点
	// (entity-update 流:conv 改名、todo CRUD、trinity entity action、未来
	// mcp/skill/系统事件)。
	NotificationsBridge notificationsdomain.Bridge

	// ForgeBridge backs the per-user /api/v1/forge SSE endpoint
	// (trinity-forging progress stream: forge_started / forge_op_applied /
	// forge_env_attempt / forge_completed). Optional — nil disables the
	// /api/v1/forge route.
	//
	// ForgeBridge 支撑 per-user /api/v1/forge SSE 端点(trinity 锻造进度流);
	// 可选,nil 禁 /api/v1/forge 路由。
	ForgeBridge forgedomain.Bridge

	// AskService routes user answers from POST /api/v1/conversations/{id}/answers
	// back to the AskUserQuestion tool that is currently blocking on Wait.
	//
	// AskService 把 POST /api/v1/conversations/{id}/answers 收到的用户答案
	// 路由回正在 Wait 阻塞的 AskUserQuestion 工具。
	AskService *askapp.Service

	// SandboxService backs the /api/v1/sandbox/* admin/debug endpoints
	// (runtime + env listing, manual GC, bootstrap status / retry,
	// per-conversation scratch env management).
	//
	// SandboxService 支持 /api/v1/sandbox/* 管理/debug 端点（runtime + env
	// 列表、手动 GC、bootstrap 状态/重试、per-conversation scratch env 管理）。
	SandboxService *sandboxapp.Service

	// SubagentService backs the /api/v1/subagent-* observability endpoints
	// (run lists, run detail, message replay, type catalog). The Subagent
	// SYSTEM TOOL is what the LLM uses to spawn runs; these endpoints are
	// the UI/inspection surface.
	//
	// SubagentService 支持 /api/v1/subagent-* 观测端点（run 列表、run 详情、
	// message 回放、类型目录）。LLM 用 Subagent 系统工具 spawn run；这些
	// 端点是 UI / 检查面。
	SubagentService *subagentapp.Service

	// MCPService backs the /api/v1/mcp-* + /api/v1/mcp-registry endpoints
	// (server CRUD / import / reconnect / health-check / registry list /
	// install). The search_mcp + call_mcp SYSTEM TOOLS are what the LLM
	// uses to discover + invoke MCP tools at runtime; these endpoints are
	// the UI's configuration + observability surface.
	//
	// MCPService 支持 /api/v1/mcp-* + /api/v1/mcp-registry 端点（server CRUD
	// / import / reconnect / health-check / registry list / install）。
	// search_mcp + call_mcp 系统工具供 LLM 运行时发现+调用 MCP 工具；这些
	// 端点是 UI 的配置+观测面。
	MCPService *mcpapp.Service

	// SkillService backs the /api/v1/skills/* endpoints (CRUD + body
	// fetch + drag-import + manual rescan + manual invoke). The
	// search_skills + activate_skill SYSTEM TOOLS are what the LLM uses
	// at runtime; these endpoints are the UI configuration + observation
	// surface. The 1s polling loop (also owned by skillapp) keeps the
	// in-memory cache live as the user edits ~/.forgify/skills/.
	//
	// SkillService 支持 /api/v1/skills/* 端点（CRUD + body 取 + 拖入 +
	// 手动重扫 + 手动 invoke）。search_skills + activate_skill 系统工具
	// 供 LLM 运行时；这些端点是 UI 配置+观测面。1s 轮询（也由 skillapp
	// 持）让用户编辑 ~/.forgify/skills/ 时内存 cache 实时更新。
	SkillService *skillapp.Service

	// CatalogService backs the /api/v1/catalog + /api/v1/catalog:refresh
	// endpoints (debug GET + UI 'Refresh now'). The Capability Catalog
	// is mostly an internal component (its real consumer is chat's
	// system-prompt assembly via SetSystemPromptProvider); these
	// endpoints exist so testend / UI can inspect + force-rebuild the
	// summary without waiting for the 1s polling tick.
	//
	// CatalogService 支持 /api/v1/catalog + /api/v1/catalog:refresh 端点
	// （debug GET + UI "立即刷新"）。Catalog 多为内部组件（真正消费者是
	// chat 经 SetSystemPromptProvider 装 system prompt）；这两个端点让
	// testend / UI 能查看 + 强制重建 summary 不等 1s 轮询。
	CatalogService *catalogapp.Service

	// ── Dev-only fields (nil/zero when Dev=false) ─────────────────────────────

	// Dev enables the /dev/* route group (static files, logs SSE, SQL, collections).
	// Dev 启用 /dev/* 路由组（静态文件、日志 SSE、SQL、集合）。
	Dev bool

	// DB is the raw GORM handle used by the /dev/sql endpoint.
	// DB 是 /dev/sql 端点使用的原始 GORM 句柄。
	DB *gorm.DB

	// LogBroadcaster fans backend log entries to /dev/logs SSE subscribers.
	// LogBroadcaster 把后端日志条目扇出给 /dev/logs SSE 订阅者。
	LogBroadcaster *loggerinfra.LogBroadcaster

	// CollectionsDir is the filesystem path to testend/collections/*.yaml.
	// CollectionsDir 是 testend/collections/*.yaml 的文件系统路径。
	CollectionsDir string

	// IntegrationDir is the filesystem path to the testend/ directory,
	// served as static files under /dev/static/.
	// IntegrationDir 是 testend/ 目录的文件系统路径，
	// 以 /dev/static/ 对外提供静态文件服务。
	IntegrationDir string

	// ForgifyHome is the resolved root holding mcp.json / skills/ /
	// .catalog.json. dev → <data-dir>/.forgify (so make clear wipes it),
	// prod → ~/.forgify. Used by /dev/info + /dev/forgify-home.
	//
	// ForgifyHome 是解析后根（含 mcp.json / skills/ / .catalog.json）。
	// dev → <data-dir>/.forgify（make clear 一并清），prod → ~/.forgify。
	// 给 /dev/info + /dev/forgify-home 用。
	ForgifyHome string

	// Port is the actual TCP port the server is listening on.
	// Used by the collections test runner to call back into the local backend.
	// Port 是服务器实际监听的 TCP 端口。
	// 供集合测试运行器回调本地后端使用。
	Port int

	// Tools is the list of system tools registered with the agent, exposed
	// for direct invocation via /dev/invoke (dev mode only).
	// Tools 是注册到 agent 的 system tool 列表，在 dev 模式下可通过
	// /dev/invoke 直接调用。
	Tools []toolapp.Tool

	// LLMFactory is the shared LLM client factory; passed to DevHandler
	// so the /dev/mock-llm/* endpoints can talk to the singleton
	// MockClient (TE-4b). Production main.go always wires it; dev
	// mode is the only consumer right now.
	//
	// LLMFactory 共享 LLM client 工厂；传给 DevHandler 让
	// /dev/mock-llm/* 端点能通到 MockClient 单例（TE-4b）。生产 main.go
	// 永远接；当前仅 dev mode 消费。
	LLMFactory *llminfra.Factory

	// ShellManager exposes the Bash background-process registry to the
	// /dev/bash-processes endpoint (TE-12). Lets testend list every
	// long-running child the LLM has spawned with run_in_background:true.
	// Nil-tolerant: when unset DevHandler simply doesn't register the route.
	//
	// ShellManager 把 Bash 后台进程注册表暴露给 /dev/bash-processes 端点
	// （TE-12）。让 testend 列每一个 LLM 用 run_in_background:true spawn
	// 的长跑子进程。容忍 nil。
	ShellManager *shelltool.ProcessManager
}

// Package router assembles HTTP routes and the middleware chain into a
// single http.Handler.
//
// Package router 把 HTTP 路由和中间件链组装成一个 http.Handler。
package router

import (
	"go.uber.org/zap"
	"gorm.io/gorm"

	apikeyapp "github.com/sunweilin/forgify/backend/internal/app/apikey"
	askapp "github.com/sunweilin/forgify/backend/internal/app/ask"
	chatapp "github.com/sunweilin/forgify/backend/internal/app/chat"
	convapp "github.com/sunweilin/forgify/backend/internal/app/conversation"
	forgeapp "github.com/sunweilin/forgify/backend/internal/app/forge"
	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	eventsdomain "github.com/sunweilin/forgify/backend/internal/domain/events"
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

	// ForgeService manages the user's Python forge library (CRUD, versions, sandbox execution).
	// ForgeService 管理用户的 Python 工具库（CRUD、版本、沙箱执行）。
	ForgeService *forgeapp.Service

	// ChatService implements messaging, attachment upload, and Agent streaming.
	// ChatService 实现消息收发、附件上传和 Agent 流式输出。
	ChatService *chatapp.Service

	// EventsBridge is the in-process pub-sub bus, shared between ChatService
	// (publisher) and the SSE handler (subscriber).
	//
	// EventsBridge 是进程内发布-订阅总线，由 ChatService（发布方）
	// 和 SSE handler（订阅方）共享。
	EventsBridge eventsdomain.Bridge

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
}

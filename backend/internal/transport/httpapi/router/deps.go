// Package router assembles HTTP routes and middleware into one http.Handler.
//
// Package router 把 HTTP 路由 + 中间件链组装成一个 http.Handler。
package router

import (
	"net/http"

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
	memoryapp "github.com/sunweilin/forgify/backend/internal/app/memory"
	permgateapp "github.com/sunweilin/forgify/backend/internal/app/tool/permissionsgate"
	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	skillapp "github.com/sunweilin/forgify/backend/internal/app/skill"
	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	shelltool "github.com/sunweilin/forgify/backend/internal/app/tool/shell"
	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	schedulerapp "github.com/sunweilin/forgify/backend/internal/app/scheduler"
	triggerapp "github.com/sunweilin/forgify/backend/internal/app/trigger"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	loggerinfra "github.com/sunweilin/forgify/backend/internal/infra/logger"
	handlershttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/handlers"
)

// SettingsServicePort is the contract the permissions HTTP handler
// needs from infra/settings.Service. Defined here as type alias-shape
// to avoid pulling infra into transport.
//
// SettingsServicePort 是 permissions HTTP handler 从 infra/settings.Service
// 要的契约。本地定义形状避免 transport 引 infra。
type SettingsServicePort = handlershttpapi.SettingsService

// Deps bundles HTTP-transport dependencies; per-domain fields are nil-tolerant.
//
// Deps 聚合 HTTP transport 层依赖;各 domain service 字段容忍 nil。
type Deps struct {
	Log *zap.Logger

	APIKeyService       *apikeyapp.Service
	ModelService        *modelapp.Service
	ConversationService *convapp.Service
	FunctionService     *functionapp.Service
	HandlerService      *handlerapp.Service
	WorkflowService     *workflowapp.Service

	FlowRunRepo      flowrundomain.Repository
	SchedulerService *schedulerapp.Service
	TriggerService   *triggerapp.Service

	Mux *http.ServeMux

	ChatService         *chatapp.Service
	EventLogBridge      eventlogdomain.Bridge
	BlockV2Repo         chatdomain.Repository
	NotificationsBridge notificationsdomain.Bridge
	ForgeBridge         forgedomain.Bridge
	AskService          *askapp.Service
	SandboxService      *sandboxapp.Service
	SubagentService     *subagentapp.Service
	MCPService          *mcpapp.Service
	SkillService        *skillapp.Service
	CatalogService      *catalogapp.Service
	MemoryService       *memoryapp.Service

	// V1.2 §3 final-sweep — permissions + hooks.
	// SettingsService 持 settings.json snapshot；SettingsPath 给 PUT 写；
	// PermGate 给 /permissions/test 用。三者捆绑出现，nil 时本组 5 端点跳。
	SettingsService SettingsServicePort
	SettingsPath    string
	PermGate        *permgateapp.Gate

	// Dev enables the /dev/* route group; below fields populate only when Dev=true.
	//
	// Dev 启用 /dev/* 路由组;下列字段仅 Dev=true 时填充。
	Dev bool

	DB             *gorm.DB
	LogBroadcaster *loggerinfra.LogBroadcaster
	CollectionsDir string
	IntegrationDir string
	ForgifyHome    string
	Port           int
	Tools          []toolapp.Tool
	LLMFactory     *llminfra.Factory
	ShellManager   *shelltool.ProcessManager
}

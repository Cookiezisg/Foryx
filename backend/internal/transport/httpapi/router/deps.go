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
)

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

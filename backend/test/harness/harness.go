//go:build pipeline

// Package harness boots the production DI graph against in-memory SQLite + httptest for pipeline tests.
//
// Package harness 用内存 SQLite + httptest 启生产 DI 图给 pipeline 测试。
package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"gorm.io/gorm"

	apikeyapp "github.com/sunweilin/forgify/backend/internal/app/apikey"
	askapp "github.com/sunweilin/forgify/backend/internal/app/ask"
	catalogapp "github.com/sunweilin/forgify/backend/internal/app/catalog"
	chatapp "github.com/sunweilin/forgify/backend/internal/app/chat"
	contextmgrapp "github.com/sunweilin/forgify/backend/internal/app/contextmgr"
	hooksapp "github.com/sunweilin/forgify/backend/internal/app/hooks"
	permgateapp "github.com/sunweilin/forgify/backend/internal/app/tool/permissionsgate"
	convapp "github.com/sunweilin/forgify/backend/internal/app/conversation"
	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
	documentapp "github.com/sunweilin/forgify/backend/internal/app/document"
	memoryapp "github.com/sunweilin/forgify/backend/internal/app/memory"
	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	skillapp "github.com/sunweilin/forgify/backend/internal/app/skill"
	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	todoapp "github.com/sunweilin/forgify/backend/internal/app/todo"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	asktool "github.com/sunweilin/forgify/backend/internal/app/tool/ask"
	fstool "github.com/sunweilin/forgify/backend/internal/app/tool/filesystem"
	functiontool "github.com/sunweilin/forgify/backend/internal/app/tool/function"
	handlertool "github.com/sunweilin/forgify/backend/internal/app/tool/handler"
	mcptool "github.com/sunweilin/forgify/backend/internal/app/tool/mcp"
	documenttool "github.com/sunweilin/forgify/backend/internal/app/tool/document"
	memorytool "github.com/sunweilin/forgify/backend/internal/app/tool/memory"
	searchtool "github.com/sunweilin/forgify/backend/internal/app/tool/search"
	shelltool "github.com/sunweilin/forgify/backend/internal/app/tool/shell"
	skilltool "github.com/sunweilin/forgify/backend/internal/app/tool/skill"
	subagenttool "github.com/sunweilin/forgify/backend/internal/app/tool/subagent"
	todotool "github.com/sunweilin/forgify/backend/internal/app/tool/todo"
	webtool "github.com/sunweilin/forgify/backend/internal/app/tool/web"
	workflowtool "github.com/sunweilin/forgify/backend/internal/app/tool/workflow"
	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	schedulerapp "github.com/sunweilin/forgify/backend/internal/app/scheduler"
	triggerapp "github.com/sunweilin/forgify/backend/internal/app/trigger"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	documentdomain "github.com/sunweilin/forgify/backend/internal/domain/document"
	memorydomain "github.com/sunweilin/forgify/backend/internal/domain/memory"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	tododomain "github.com/sunweilin/forgify/backend/internal/domain/todo"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	cryptoinfra "github.com/sunweilin/forgify/backend/internal/infra/crypto"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	eventloginfra "github.com/sunweilin/forgify/backend/internal/infra/eventlog"
	forgeinfra "github.com/sunweilin/forgify/backend/internal/infra/forge"
	mcpinfra "github.com/sunweilin/forgify/backend/internal/infra/mcp"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	settingsinfra "github.com/sunweilin/forgify/backend/internal/infra/settings"
	notificationsinfra "github.com/sunweilin/forgify/backend/internal/infra/notifications"
	sandboxinfra "github.com/sunweilin/forgify/backend/internal/infra/sandbox"
	apikeystore "github.com/sunweilin/forgify/backend/internal/infra/store/apikey"
	chatstore "github.com/sunweilin/forgify/backend/internal/infra/store/chat"
	convstore "github.com/sunweilin/forgify/backend/internal/infra/store/conversation"
	functionstore "github.com/sunweilin/forgify/backend/internal/infra/store/function"
	handlerstore "github.com/sunweilin/forgify/backend/internal/infra/store/handler"
	documentstore "github.com/sunweilin/forgify/backend/internal/infra/store/document"
	memorystore "github.com/sunweilin/forgify/backend/internal/infra/store/memory"
	modelstore "github.com/sunweilin/forgify/backend/internal/infra/store/model"
	sandboxstore "github.com/sunweilin/forgify/backend/internal/infra/store/sandbox"
	todostore "github.com/sunweilin/forgify/backend/internal/infra/store/todo"
	workflowstore "github.com/sunweilin/forgify/backend/internal/infra/store/workflow"
	flowrunstore "github.com/sunweilin/forgify/backend/internal/infra/store/flowrun"
	mcpcallstore "github.com/sunweilin/forgify/backend/internal/infra/store/mcpcalls"
	skillexecstore "github.com/sunweilin/forgify/backend/internal/infra/store/skillexec"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	forgepkg "github.com/sunweilin/forgify/backend/internal/pkg/forge"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
	routerhttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/router"
)

// Option configures the Harness at construction time.
//
// Option 构造时配置 Harness。
type Option func(*options)

type options struct {
	fakeLLMBaseURL  string
	curatedRegistry bool
	sandboxDataDir  string
}

// WithFakeLLMBaseURL routes the injected apikey to a fake LLM server.
//
// WithFakeLLMBaseURL 把 apikey BaseURL 指向 fake LLM server。
func WithFakeLLMBaseURL(url string) Option {
	return func(o *options) { o.fakeLLMBaseURL = url }
}

// WithCuratedRegistry swaps the in-memory test registry for the production CuratedRegistrySource.
//
// WithCuratedRegistry 把测试 registry 换成生产 CuratedRegistrySource。
func WithCuratedRegistry() Option {
	return func(o *options) { o.curatedRegistry = true }
}

// WithSandboxDataDir overrides the per-test t.TempDir() to share mise/npm/uv caches across runs.
//
// WithSandboxDataDir 用指定目录共享 mise/npm/uv 缓存，避免每测重下载。
func WithSandboxDataDir(dir string) Option {
	return func(o *options) { o.sandboxDataDir = dir }
}

// Harness is a booted in-process backend driveable over HTTP.
//
// Harness 是可通过 HTTP 驱动的 in-process 后端。
type Harness struct {
	t      *testing.T
	server *httptest.Server
	log    *zap.Logger

	fakeLLMBaseURL string

	DB                  *gorm.DB
	EventLogBridge      *eventloginfra.Bridge
	NotificationsBridge *notificationsinfra.Bridge
	NotificationsPub    notificationspkg.Publisher
	ForgeBridge         *forgeinfra.Bridge
	ForgePub            forgepkg.Publisher
	ChatEmitter         eventlogpkg.Emitter
	Sandbox             *sandboxapp.Service
	MCP                 *mcpapp.Service
	Skill               *skillapp.Service
	Catalog             *catalogapp.Service
	Memory              *memoryapp.Service
	Document            *documentapp.Service
	ContextManager      *contextmgrapp.Manager
	Settings            *settingsinfra.Service
	SettingsPath        string // path to settings.json for tests to write rules
	PermGate            *permgateapp.Gate
	HookRunner          *hooksapp.Runner

	APIKey       *apikeyapp.Service
	Model        *modelapp.Service
	Conversation *convapp.Service
	Function     *functionapp.Service
	Handler      *handlerapp.Service
	Workflow     *workflowapp.Service
	Scheduler    *schedulerapp.Service
	Trigger      *triggerapp.Service
	FlowRunRepo  flowrundomain.Repository
	Chat         *chatapp.Service
	Tools        []toolapp.Tool
}

// New boots a fresh harness with in-memory SQLite + httptest server; cleanup is registered on t.
//
// New 启动内存 SQLite + httptest 的 harness，清理挂到 t。
func New(t *testing.T, opts ...Option) *Harness {
	t.Helper()

	cfg := &options{}
	for _, o := range opts {
		o(cfg)
	}

	log := zaptest.NewLogger(t)

	gdb, err := dbinfra.Open(dbinfra.Config{DataDir: ""})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		if err := dbinfra.Close(gdb); err != nil {
			t.Logf("close db: %v", err)
		}
	})

	// SQLite :memory: per-connection isolation; force single connection so goroutines share tables.
	if sqlDB, err := gdb.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}

	if err := dbinfra.Migrate(gdb,
		&apikeydomain.APIKey{},
		&modeldomain.ModelConfig{},
		&convdomain.Conversation{},
		&chatdomain.Message{},
		&chatdomain.Block{},
		&chatdomain.Attachment{},
		&functiondomain.Function{},
		&functiondomain.Version{},
		&functiondomain.Execution{},
		&handlerdomain.Handler{},
		&handlerdomain.Version{},
		&handlerdomain.Call{},
		&workflowdomain.Workflow{},
		&workflowdomain.Version{},
		&flowrundomain.FlowRun{},
		&flowrundomain.Node{},
		&mcpdomain.Call{},
		&skilldomain.Execution{},
		&sandboxdomain.Runtime{},
		&sandboxdomain.Env{},
		&tododomain.Todo{},
		&memorydomain.Memory{},
		&documentdomain.Document{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Deterministic test fingerprint so harness can decrypt keys it inserted.
	encryptor, err := cryptoinfra.NewAESGCMEncryptor(
		cryptoinfra.DeriveKey("forgify-pipeline-test-fingerprint"),
	)
	if err != nil {
		t.Fatalf("build encryptor: %v", err)
	}

	apikeyService := apikeyapp.NewService(
		apikeystore.New(gdb),
		encryptor,
		apikeyapp.NewHTTPTester(nil),
		log,
	)
	modelService := modelapp.NewService(modelstore.New(gdb), apikeyService, log)

	llmFactory := llminfra.NewFactory()

	// PluginSandbox v2 rooted at per-test tempdir; Bootstrap failure → degraded mode.
	var dataDir string
	if cfg.sandboxDataDir != "" {
		dataDir = cfg.sandboxDataDir
	} else {
		dataDir = t.TempDir()
	}
	eventLogBridge := eventloginfra.NewBridge(log)
	notificationsBridge := notificationsinfra.NewBridge(log)
	notificationsPub := notificationspkg.New(notificationsBridge, log)
	forgeBridge := forgeinfra.NewBridge(log)
	forgePub := forgepkg.New(forgeBridge, log)

	sandboxRepo := sandboxstore.New(gdb)
	sandboxSvc := sandboxapp.New(sandboxRepo, dataDir, notificationsPub, log)
	if err := sandboxSvc.Bootstrap(context.Background()); err != nil {
		t.Logf("sandbox v2 bootstrap failed: %v (degraded mode active; runtime ops will fail)", err)
	}
	registerSandboxStack(sandboxSvc)
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := sandboxSvc.Shutdown(shutdownCtx); err != nil {
			t.Logf("sandbox shutdown: %v", err)
		}
	})

	convService := convapp.NewService(convstore.New(gdb), notificationsPub, log)

	functionService := functionapp.NewService(
		functionstore.New(gdb),
		functionapp.NewSandboxAdapter(sandboxSvc, dataDir),
		notificationsPub,
		log,
	)

	handlerService := handlerapp.NewService(
		handlerstore.New(gdb),
		handlerapp.NewSandboxAdapter(sandboxSvc, dataDir),
		handlerapp.DefaultClientFactory,
		encryptor,
		notificationsPub,
		log,
	)
	t.Cleanup(func() {
		handlerService.Shutdown(context.Background())
	})

	// Skill + MCP backfilled below once those services exist.
	workflowChecker := &workflowapp.ProductionChecker{
		Function: functionService,
		Handler:  handlerService,
	}
	workflowService := workflowapp.NewService(
		workflowstore.New(gdb),
		workflowChecker,
		notificationsPub,
		log,
	)

	chatRepo := chatstore.New(gdb)
	chatEmitter := eventlogpkg.New(eventLogBridge, chatRepo, log)
	chatService := chatapp.NewService(
		chatRepo,
		convstore.New(gdb),
		modelService,
		apikeyService,
		llmFactory,
		chatEmitter,
		notificationsPub,
		dataDir,
		log,
	)

	pathGuard := pathguardpkg.NewDefault()

	tools := functiontool.FunctionTools(
		functionService, modelService, apikeyService, llmFactory, forgePub, log,
	)
	tools = append(tools, handlertool.HandlerTools(
		handlerService, modelService, apikeyService, llmFactory, forgePub, log,
	)...)
	tools = append(tools, workflowtool.WorkflowTools(
		workflowService, forgePub, log,
	)...)
	tools = append(tools, fstool.FilesystemTools(pathGuard)...)
	tools = append(tools, searchtool.SearchTools(pathGuard, log)...)
	// nil MCP router in harness = BYOK + Bing CN only; tests that need MCP construct WebSearch directly.
	tools = append(tools, webtool.WebTools(modelService, apikeyService, llmFactory, nil, log)...)
	shells := shelltool.NewShellTools(sandboxSvc)
	t.Cleanup(shells.Manager.Stop)
	tools = append(tools, shells.Tools...)
	todoService := todoapp.NewService(todostore.New(gdb), notificationsPub, log)
	tools = append(tools, todotool.TodoTools(todoService)...)
	askService := askapp.NewService()
	tools = append(tools, asktool.AskTools(askService)...)

	memoryService := memoryapp.New(memorystore.New(gdb), notificationsPub, log)
	tools = append(tools, memorytool.MemoryTools(memoryService)...)

	documentService := documentapp.New(documentstore.New(gdb), notificationsPub, log)
	tools = append(tools, documenttool.DocumentTools(documentService)...)

	subagentService := subagentapp.New(
		chatRepo,
		subagentapp.NewRegistry(),
		modelService,
		apikeyService,
		llmFactory,
		log,
	)
	tools = append(tools, subagenttool.SubagentTools(subagentService)...)
	subagentService.SetTools(tools)

	// per-test tempdir for mcp.json + in-memory registry (curated swapped in via WithCuratedRegistry).
	mcpConfigPath := filepath.Join(dataDir, "mcp.json")
	var mcpRegistrySource mcpdomain.RegistrySource
	if cfg.curatedRegistry {
		mcpRegistrySource = mcpinfra.NewCuratedRegistrySource()
	} else {
		mcpRegistrySource = newTestRegistrySource(
			mcpdomain.RegistryEntry{
				Name:        "everything",
				Description: "MCP protocol reference test server (used by D9 pipeline tests).",
				Runtime:     "node",
				InstallCmd: mcpdomain.InstallCmd{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-everything"},
				},
				Category: "browser",
				Tier:     0,
			},
			mcpdomain.RegistryEntry{
				Name:        "sqlite-test",
				Description: "Sample server with a required arg, for install error-path tests.",
				Runtime:     "python",
				InstallCmd: mcpdomain.InstallCmd{
					Command: "uvx",
					Args:    []string{"mcp-server-sqlite", "--db-path", "${dbPath}"},
				},
				RequiredArgs: []mcpdomain.ArgRequirement{
					{Name: "dbPath", Description: "Path to the SQLite db file", Type: "path"},
				},
				Category: "database",
				Tier:     3,
			},
		)
	}
	mcpService := mcpapp.New(
		mcpConfigPath,
		mcpRegistrySource,
		sandboxSvc,
		modelService,
		apikeyService,
		llmFactory,
		notificationsPub,
		log,
	)
	if err := mcpService.Start(context.Background()); err != nil {
		t.Logf("mcp start: %v (continuing — pipeline tests that need it will skip)", err)
	}
	tools = append(tools, mcptool.MCPTools(mcpService)...)

	// per-test tempdir SkillsDir; tests seed and either call Scan or rely on 1s polling.
	skillsDir := filepath.Join(dataDir, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	skillService := skillapp.New(
		skillsDir,
		subagentService,
		modelService,
		apikeyService,
		llmFactory,
		notificationsPub,
		log,
	)
	if err := skillService.Start(context.Background()); err != nil {
		t.Logf("skill start: %v", err)
	}
	t.Cleanup(skillService.Stop)
	tools = append(tools, skilltool.SkillTools(skillService)...)

	// per-test cache; no SetGenerator → mechanical-fallback only (avoids FIFO script queue contention).
	catalogCachePath := filepath.Join(dataDir, ".catalog.json")
	catalogService := catalogapp.New(catalogCachePath, notificationsPub, log)
	catalogService.RegisterSource(functionService.AsCatalogSource())
	catalogService.RegisterSource(handlerService.AsCatalogSource())
	catalogService.RegisterSource(skillService.AsCatalogSource())
	catalogService.RegisterSource(mcpService.AsCatalogSource())
	catalogService.RegisterSource(documentService.AsCatalogSource())
	if err := catalogService.Start(context.Background()); err != nil {
		t.Logf("catalog start: %v", err)
	}
	// Stop must drain before t.TempDir RemoveAll to avoid "directory not empty" race.
	t.Cleanup(catalogService.Stop)
	chatService.SetSystemPromptProvider(catalogService)
	chatService.SetMemoryProvider(memoryService)

	cheapLLMResolver := func(ctx context.Context) (llminfra.Client, string, string, string, error) {
		bundle, err := llmclientpkg.ResolveForWebSummary(ctx, modelService, apikeyService, llmFactory)
		if err != nil {
			return nil, "", "", "", err
		}
		return bundle.Client, bundle.ModelID, bundle.Key, bundle.BaseURL, nil
	}
	contextManager := contextmgrapp.New(
		chatRepo, convstore.New(gdb), chatEmitter, notificationsPub, cheapLLMResolver, log)
	chatService.SetContextCompactor(contextManager)

	// V1.2 §3 final-sweep — permissions + hooks. Test harness points at
	// a per-test settings.json under t.TempDir() (created lazily); tests
	// write a custom file when they need rules.
	// V1.2 §3 ——permissions + hooks。test harness 指向 t.TempDir()/
	// settings.json（懒建）；测试需规则时自写文件。
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	settingsService := settingsinfra.New(settingsPath, log)
	if err := settingsService.Start(context.Background()); err != nil {
		t.Logf("settings start failed (continuing with empty defaults): %v", err)
	}
	t.Cleanup(settingsService.Close)
	permGate := permgateapp.New(settingsService)
	hookRunner := hooksapp.New(settingsService, log)
	chatService.SetPermissionsAndHooks(permGate, hookRunner)

	workflowChecker.Skill = skillService
	workflowChecker.MCP = mcpService

	flowrunRepo := flowrunstore.New(gdb)
	mcpCallRepo := mcpcallstore.New(gdb)
	skillExecRepo := skillexecstore.New(gdb)
	mcpService.SetCallRepo(mcpCallRepo)
	skillService.SetExecRepo(skillExecRepo)

	httpMux := http.NewServeMux()
	triggerService := triggerapp.New(httpMux, log)
	schedulerService := schedulerapp.NewService(flowrunRepo, workflowService, notificationsPub, log)
	triggerService.SetScheduler(schedulerService)
	t.Cleanup(triggerService.Shutdown)

	router := schedulerapp.NewRouter()
	router.Set(workflowdomain.NodeTypeTrigger, schedulerapp.NewTriggerDispatcher())
	router.Set(workflowdomain.NodeTypeFunction, schedulerapp.NewFunctionDispatcher(functionService))
	router.Set(workflowdomain.NodeTypeHandler, schedulerapp.NewHandlerDispatcher(handlerService))
	router.Set(workflowdomain.NodeTypeMCP, schedulerapp.NewMCPDispatcher(mcpService))
	router.Set(workflowdomain.NodeTypeSkill, schedulerapp.NewSkillDispatcher(skillService))
	router.Set(workflowdomain.NodeTypeLLM, schedulerapp.NewLLMDispatcher(nil))
	router.Set(workflowdomain.NodeTypeHTTP, schedulerapp.NewHTTPDispatcher(nil))
	router.Set(workflowdomain.NodeTypeCondition, schedulerapp.NewConditionDispatcher())
	router.Set(workflowdomain.NodeTypeLoop, schedulerapp.NewLoopDispatcher())
	router.Set(workflowdomain.NodeTypeParallel, schedulerapp.NewParallelDispatcher())
	router.Set(workflowdomain.NodeTypeApproval, schedulerapp.NewApprovalDispatcher())
	router.Set(workflowdomain.NodeTypeWait, schedulerapp.NewWaitDispatcher())
	router.Set(workflowdomain.NodeTypeVariable, schedulerapp.NewVariableDispatcher())
	schedulerService.SetRouter(router)

	tools = append(tools, workflowtool.WorkflowExecutionTools(flowrunRepo)...)
	tools = append(tools, mcptool.MCPCallLogTools(mcpCallRepo)...)
	tools = append(tools, skilltool.SkillExecutionTools(skillExecRepo)...)

	chatService.SetTools(tools)

	handler := routerhttpapi.New(routerhttpapi.Deps{
		Log:                 log,
		APIKeyService:       apikeyService,
		ModelService:        modelService,
		ConversationService: convService,
		FunctionService:     functionService,
		HandlerService:      handlerService,
		WorkflowService:     workflowService,
		FlowRunRepo:         flowrunRepo,
		SchedulerService:    schedulerService,
		TriggerService:      triggerService,
		Mux:                 httpMux,
		ChatService:         chatService,
		EventLogBridge:      eventLogBridge,
		BlockV2Repo:         chatRepo,
		NotificationsBridge: notificationsBridge,
		ForgeBridge:         forgeBridge,
		AskService:          askService,
		SandboxService:      sandboxSvc,
		SubagentService:     subagentService,
		MCPService:          mcpService,
		SkillService:        skillService,
		CatalogService:      catalogService,
		MemoryService:       memoryService,
		DocumentService:     documentService,
		SettingsService:     settingsService,
		SettingsPath:        settingsPath,
		PermGate:            permGate,
		Dev:                 false,
		Tools:               tools,
		LLMFactory:          llmFactory,
		ShellManager:        shells.Manager,
		DB:                  gdb,
		Port:                0,
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &Harness{
		t:                   t,
		server:              srv,
		log:                 log,
		fakeLLMBaseURL:      cfg.fakeLLMBaseURL,
		DB:                  gdb,
		EventLogBridge:      eventLogBridge,
		NotificationsBridge: notificationsBridge,
		NotificationsPub:    notificationsPub,
		ForgeBridge:         forgeBridge,
		ForgePub:            forgePub,
		ChatEmitter:         chatEmitter,
		Sandbox:             sandboxSvc,
		MCP:                 mcpService,
		Skill:               skillService,
		Catalog:             catalogService,
		APIKey:              apikeyService,
		Model:               modelService,
		Conversation:        convService,
		Function:            functionService,
		Handler:             handlerService,
		Workflow:            workflowService,
		Scheduler:           schedulerService,
		Trigger:             triggerService,
		FlowRunRepo:         flowrunRepo,
		Chat:                chatService,
		Memory:              memoryService,
		Document:            documentService,
		ContextManager:      contextManager,
		Settings:            settingsService,
		SettingsPath:        settingsPath,
		PermGate:            permGate,
		HookRunner:          hookRunner,
		Tools:               tools,
	}
}

// registerSandboxStack mirrors cmd/server/main.go::registerSandboxStack.
//
// registerSandboxStack 镜像 main.go 的同名 helper。
func registerSandboxStack(svc *sandboxapp.Service) {
	miseBin := svc.MiseBin()
	if miseBin == "" {
		return
	}
	for kind, defaultVer := range map[string]string{
		"python": "3.12",
		"node":   "22",
		"uv":     "0.11.4",
	} {
		svc.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, kind, defaultVer))
	}
	svc.RegisterEnvManager(sandboxinfra.NewPythonEnvManager(svc))
	svc.RegisterEnvManager(sandboxinfra.NewNodeEnvManager())
}

// URL returns the test server's base URL.
//
// URL 返回 test server base URL。
func (h *Harness) URL() string { return h.server.URL }

// HTTPClient returns a client for short-lived requests; SSE uses SubscribeSSE.
//
// Timeout sized for slowest legitimate sync path: first-ever function POST
// triggers mise to fetch + install Python runtime (15-25s typical, up to ~40s
// under load / cold disk). 30s here previously caused flake when env sync
// raced the deadline — bumped to 120s with 4x safety margin.
//
// HTTPClient 返回短请求 client;SSE 走 SubscribeSSE。
// timeout 按最慢合法同步路径设:首次 function POST 触发 mise 下载装 Python
// (典型 15-25s,负载 / 冷盘下到 ~40s)。原 30s 偶发卡 env sync 死线,改 120s。
func (h *Harness) HTTPClient() *http.Client {
	return &http.Client{Timeout: 120 * time.Second}
}

// PostJSON POSTs body as JSON and decodes into out; fatals on non-2xx.
//
// PostJSON POST JSON 解到 out，非 2xx 直接 fatal。
func (h *Harness) PostJSON(path string, body, out any) *http.Response {
	h.t.Helper()
	return h.requestJSON("POST", path, body, out)
}

// GetJSON GETs path and decodes into out.
//
// GetJSON GET path 解到 out。
func (h *Harness) GetJSON(path string, out any) *http.Response {
	h.t.Helper()
	return h.requestJSON("GET", path, nil, out)
}

// PatchJSON PATCHes body to path and decodes into out.
//
// PatchJSON PATCH body 到 path 解到 out。
func (h *Harness) PatchJSON(path string, body, out any) *http.Response {
	h.t.Helper()
	return h.requestJSON("PATCH", path, body, out)
}

// Delete DELETEs path; fatals on non-2xx.
//
// Delete DELETE path，非 2xx 直接 fatal。
func (h *Harness) Delete(path string) *http.Response {
	h.t.Helper()
	return h.requestJSON("DELETE", path, nil, nil)
}

func (h *Harness) requestJSON(method, path string, body, out any) *http.Response {
	h.t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			h.t.Fatalf("marshal %s %s body: %v", method, path, err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, h.server.URL+path, rdr)
	if err != nil {
		h.t.Fatalf("build %s %s: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.HTTPClient().Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		h.t.Fatalf("%s %s: status %d: %s", method, path, resp.StatusCode, raw)
	}
	if out != nil {
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			h.t.Fatalf("%s %s: decode response: %v", method, path, err)
		}
	} else {
		_ = resp.Body.Close()
	}
	return resp
}

// RequireDeepSeekKey returns DEEPSEEK_API_KEY from env or skips the test.
//
// RequireDeepSeekKey 返回 env 中的 DEEPSEEK_API_KEY，缺则 skip。
func RequireDeepSeekKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("DEEPSEEK_API_KEY")
	if key == "" {
		t.Skip("DEEPSEEK_API_KEY not set; skipping (run via `make test-pipeline` to load .env)")
	}
	return key
}


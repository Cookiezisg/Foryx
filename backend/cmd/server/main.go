// Command server boots the Forgify backend: logger, DB, HTTP router, and graceful shutdown.
//
// Command server 启动 Forgify 后端：logger、DB、HTTP 路由、优雅关闭。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	apikeyapp "github.com/sunweilin/forgify/backend/internal/app/apikey"
	askapp "github.com/sunweilin/forgify/backend/internal/app/ask"
	chatapp "github.com/sunweilin/forgify/backend/internal/app/chat"
	convapp "github.com/sunweilin/forgify/backend/internal/app/conversation"
	documentapp "github.com/sunweilin/forgify/backend/internal/app/document"
	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
	memoryapp "github.com/sunweilin/forgify/backend/internal/app/memory"
	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	skillapp "github.com/sunweilin/forgify/backend/internal/app/skill"
	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	todoapp "github.com/sunweilin/forgify/backend/internal/app/todo"
	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	schedulerapp "github.com/sunweilin/forgify/backend/internal/app/scheduler"
	triggerapp "github.com/sunweilin/forgify/backend/internal/app/trigger"
	catalogapp "github.com/sunweilin/forgify/backend/internal/app/catalog"
	contextmgrapp "github.com/sunweilin/forgify/backend/internal/app/contextmgr"
	hooksapp "github.com/sunweilin/forgify/backend/internal/app/hooks"
	permgateapp "github.com/sunweilin/forgify/backend/internal/app/tool/permissionsgate"
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
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	documentdomain "github.com/sunweilin/forgify/backend/internal/domain/document"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	tododomain "github.com/sunweilin/forgify/backend/internal/domain/todo"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	memorydomain "github.com/sunweilin/forgify/backend/internal/domain/memory"
	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	cryptoinfra "github.com/sunweilin/forgify/backend/internal/infra/crypto"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	eventloginfra "github.com/sunweilin/forgify/backend/internal/infra/eventlog"
	forgeinfra "github.com/sunweilin/forgify/backend/internal/infra/forge"
	notificationsinfra "github.com/sunweilin/forgify/backend/internal/infra/notifications"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	forgepkg "github.com/sunweilin/forgify/backend/internal/pkg/forge"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	loggerinfra "github.com/sunweilin/forgify/backend/internal/infra/logger"
	mcpinfra     "github.com/sunweilin/forgify/backend/internal/infra/mcp"
	sandboxinfra "github.com/sunweilin/forgify/backend/internal/infra/sandbox"
	apikeystore "github.com/sunweilin/forgify/backend/internal/infra/store/apikey"
	chatstore "github.com/sunweilin/forgify/backend/internal/infra/store/chat"
	convstore "github.com/sunweilin/forgify/backend/internal/infra/store/conversation"
	documentstore "github.com/sunweilin/forgify/backend/internal/infra/store/document"
	functionstore "github.com/sunweilin/forgify/backend/internal/infra/store/function"
	handlerstore "github.com/sunweilin/forgify/backend/internal/infra/store/handler"
	memorystore "github.com/sunweilin/forgify/backend/internal/infra/store/memory"
	modelstore "github.com/sunweilin/forgify/backend/internal/infra/store/model"
	sandboxstore "github.com/sunweilin/forgify/backend/internal/infra/store/sandbox"
	todostore "github.com/sunweilin/forgify/backend/internal/infra/store/todo"
	workflowstore "github.com/sunweilin/forgify/backend/internal/infra/store/workflow"
	flowrunstore "github.com/sunweilin/forgify/backend/internal/infra/store/flowrun"
	mcpcallstore "github.com/sunweilin/forgify/backend/internal/infra/store/mcpcalls"
	skillexecstore "github.com/sunweilin/forgify/backend/internal/infra/store/skillexec"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	settingsinfra "github.com/sunweilin/forgify/backend/internal/infra/settings"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
	routerhttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/router"
)

func main() {
	port := flag.Int("port", 0, "HTTP port (0 = pick a free port, print it)")
	dataDir := flag.String("data-dir", "", "Data directory (empty = os.TempDir)")
	dev := flag.Bool("dev", false, "Development mode (colored console logs + /dev/* routes)")
	collectionsDir := flag.String("collections-dir", "../testend/collections", "Path to YAML test collections (dev mode)")
	integrationDir := flag.String("integration-dir", "../testend", "Path to testend/ directory served at /dev/static/ (dev mode)")
	forgifyHome := flag.String("forgify-home", "",
		"User-level config root holding mcp.json / skills/ / .catalog.json. "+
			"Default: <data-dir>/.forgify in --dev mode, ~/.forgify otherwise.")
	flag.Parse()

	// Dev mode roots forgifyHome under data-dir so `make clear` wipes mcp.json/skills/catalog cache.
	homeRoot := *forgifyHome
	if homeRoot == "" {
		if *dev && *dataDir != "" {
			homeRoot = filepath.Join(*dataDir, ".forgify")
		} else if h, err := os.UserHomeDir(); err == nil && h != "" {
			homeRoot = filepath.Join(h, ".forgify")
		} else {
			homeRoot = ".forgify"
		}
	}

	var broadcaster *loggerinfra.LogBroadcaster
	var logExtras []zapcore.Core
	if *dev {
		broadcaster = loggerinfra.NewLogBroadcaster()
		logExtras = []zapcore.Core{broadcaster}
	}

	log, err := loggerinfra.New(*dev, logExtras...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync() //nolint:errcheck

	gdb, err := dbinfra.Open(dbinfra.Config{DataDir: *dataDir})
	if err != nil {
		log.Error("open db", zap.Error(err))
		os.Exit(1)
	}
	defer func() {
		if err := dbinfra.Close(gdb); err != nil {
			log.Warn("close db", zap.Error(err))
		}
	}()

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
		log.Error("migrate db", zap.Error(err))
		os.Exit(1)
	}

	fingerprint, err := cryptoinfra.MachineFingerprint()
	if err != nil {
		log.Error("machine fingerprint", zap.Error(err))
		os.Exit(1)
	}
	encryptor, err := cryptoinfra.NewAESGCMEncryptor(cryptoinfra.DeriveKey(fingerprint))
	if err != nil {
		log.Error("build encryptor", zap.Error(err))
		os.Exit(1)
	}
	apikeyService := apikeyapp.NewService(
		apikeystore.New(gdb),
		encryptor,
		apikeyapp.NewHTTPTester(nil),
		log,
	)

	modelService := modelapp.NewService(modelstore.New(gdb), apikeyService, log)

	llmFactory := llminfra.NewFactory()

	// Dev mode enables LLM call tracing for testend's Wire tab replay.
	if *dev {
		llmFactory.SetTracer(llminfra.NewTraceRecorder())
		log.Info("LLM trace recorder enabled (--dev) — testend Wire tab will replay every Stream call")
	}

	eventLogBridge := eventloginfra.NewBridge(log)
	notificationsBridge := notificationsinfra.NewBridge(log)
	notificationsPub := notificationspkg.New(notificationsBridge, log)
	forgeBridge := forgeinfra.NewBridge(log)
	forgePub := forgepkg.New(forgeBridge, log)
	convService := convapp.NewService(convstore.New(gdb), notificationsPub, log)

	// PluginSandbox v2 bootstrap: extract embedded mise binary; failure flips degraded mode (non-fatal).
	sandboxRepo := sandboxstore.New(gdb)
	sandboxSvc := sandboxapp.New(sandboxRepo, *dataDir, notificationsPub, log)
	if err := sandboxSvc.Bootstrap(context.Background()); err != nil {
		log.Warn("sandbox v2 bootstrap failed (degraded mode active; runtime ops will fail)",
			zap.Error(err))
	}
	registerSandboxStack(sandboxSvc, log)

	functionService := functionapp.NewService(
		functionstore.New(gdb),
		functionapp.NewSandboxAdapter(sandboxSvc, *dataDir),
		notificationsPub,
		log,
	)

	handlerService := handlerapp.NewService(
		handlerstore.New(gdb),
		handlerapp.NewSandboxAdapter(sandboxSvc, *dataDir),
		handlerapp.DefaultClientFactory,
		encryptor,
		notificationsPub,
		log,
	)

	// workflowChecker.Skill / .MCP are filled below once those services exist.
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
		*dataDir,
		log,
	)

	pathGuard := pathguardpkg.NewDefault()

	// MCP Service constructed before WebTools so WebSearch can route through duckduckgo-search MCP.
	mcpConfigPath := filepath.Join(homeRoot, "mcp.json")
	mcpRegistrySource := mcpinfra.NewCuratedRegistrySource()
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

	tools := functiontool.FunctionTools(
		functionService,
		modelService,
		apikeyService,
		llmFactory,
		forgePub,
		log,
	)
	tools = append(tools, handlertool.HandlerTools(
		handlerService,
		modelService,
		apikeyService,
		llmFactory,
		forgePub,
		log,
	)...)
	tools = append(tools, workflowtool.WorkflowTools(
		workflowService,
		forgePub,
		log,
	)...)
	tools = append(tools, fstool.FilesystemTools(pathGuard)...)
	tools = append(tools, searchtool.SearchTools(pathGuard, log)...)
	tools = append(tools, webtool.WebTools(modelService, apikeyService, llmFactory, mcpapp.NewSearchRouter(mcpService), log)...)
	shells := shelltool.NewShellTools(sandboxSvc)
	defer shells.Manager.Stop()
	tools = append(tools, shells.Tools...)

	todoService := todoapp.NewService(todostore.New(gdb), notificationsPub, log)
	tools = append(tools, todotool.TodoTools(todoService)...)
	askService := askapp.NewService()
	tools = append(tools, asktool.AskTools(askService)...)

	memoryService := memoryapp.New(memorystore.New(gdb), notificationsPub, log)
	tools = append(tools, memorytool.MemoryTools(memoryService)...)

	documentService := documentapp.New(documentstore.New(gdb), notificationsPub, log)
	tools = append(tools, documenttool.DocumentTools(documentService)...)

	// SubagentTool is appended after Service construction; SetTools runs after the slice is finalized.
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

	// §S9 detached ctx: inject DefaultLocalUserID so boot publishStatus can write.
	mcpBootCtx := reqctxpkg.SetUserID(context.Background(), reqctxpkg.DefaultLocalUserID)
	if err := mcpService.Start(mcpBootCtx); err != nil {
		log.Warn("mcp start partial failure (some servers may be unreachable)", zap.Error(err))
	}
	tools = append(tools, mcptool.MCPTools(mcpService)...)

	skillService := skillapp.New(
		filepath.Join(homeRoot, "skills"),
		subagentService,
		modelService,
		apikeyService,
		llmFactory,
		notificationsPub,
		log,
	)
	skillBootCtx := reqctxpkg.SetUserID(context.Background(), reqctxpkg.DefaultLocalUserID)
	if err := skillService.Start(skillBootCtx); err != nil {
		log.Warn("skill start failed (continuing with empty cache)", zap.Error(err))
	}
	tools = append(tools, skilltool.SkillTools(skillService)...)

	catalogService := catalogapp.New(filepath.Join(homeRoot, ".catalog.json"), notificationsPub, log)
	catalogService.SetGenerator(catalogapp.NewLLMGenerator(modelService, apikeyService, llmFactory, log))
	// Sources here = "things the LLM can call from chat as capabilities":
	// function (run_function), handler (call_handler), skill (invoke), mcp
	// (server tools). Workflows are intentionally NOT in coverage — they
	// are user-triggered (trigger_workflow fires a job; not a capability
	// the LLM picks based on intent matching).
	//
	// 此处 sources = LLM 从 chat 可调用的能力:function (run_function) /
	// handler (call_handler) / skill (invoke) / mcp (server tools)。
	// workflow 故意不在 coverage——它是用户触发(trigger_workflow 启动 job,
	// 不是 LLM 按意图匹配挑用的能力)。
	catalogService.RegisterSource(functionService.AsCatalogSource())
	catalogService.RegisterSource(handlerService.AsCatalogSource())
	catalogService.RegisterSource(skillService.AsCatalogSource())
	catalogService.RegisterSource(mcpService.AsCatalogSource())
	catalogService.RegisterSource(documentService.AsCatalogSource())
	if err := catalogService.Start(context.Background()); err != nil {
		log.Warn("catalog start failed (continuing without catalog injection)", zap.Error(err))
	}
	chatService.SetSystemPromptProvider(catalogService)
	chatService.SetMemoryProvider(memoryService)

	// V1.2 §3 final-sweep — permissions + hooks.
	// settings.json lives at <homeRoot>/settings.json; gate reads via
	// SettingsService snapshot; HookRunner consumes the same snapshot.
	// V1.2 §3 final-sweep —— permissions + hooks。settings.json 在
	// <homeRoot>/settings.json；gate 经 SettingsService 快照读；
	// HookRunner 共用此快照。
	settingsService := settingsinfra.New(filepath.Join(homeRoot, "settings.json"), log)
	if err := settingsService.Start(context.Background()); err != nil {
		log.Warn("settings start failed (continuing with last good snapshot)", zap.Error(err))
	}
	permGate := permgateapp.New(settingsService)
	hookRunner := hooksapp.New(settingsService, log)
	chatService.SetPermissionsAndHooks(permGate, hookRunner)
	settingsPath := filepath.Join(homeRoot, "settings.json")

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

	workflowChecker.Skill = skillService
	workflowChecker.MCP = mcpService

	flowrunRepo := flowrunstore.New(gdb)
	mcpCallRepo := mcpcallstore.New(gdb)
	skillExecRepo := skillexecstore.New(gdb)
	mcpService.SetCallRepo(mcpCallRepo)
	skillService.SetExecRepo(skillExecRepo)

	// Build mux up-front so trigger.webhook can register sub-paths on the same ServeMux.
	httpMux := http.NewServeMux()
	triggerService := triggerapp.New(httpMux, log)
	schedulerService := schedulerapp.NewService(
		flowrunRepo,
		workflowService,
		notificationsPub,
		log,
	)
	triggerService.SetScheduler(schedulerService)

	router := schedulerapp.NewRouter()
	router.Set(workflowdomain.NodeTypeTrigger, schedulerapp.NewTriggerDispatcher())
	router.Set(workflowdomain.NodeTypeFunction, schedulerapp.NewFunctionDispatcher(functionService))
	router.Set(workflowdomain.NodeTypeHandler, schedulerapp.NewHandlerDispatcher(handlerService))
	router.Set(workflowdomain.NodeTypeMCP, schedulerapp.NewMCPDispatcher(mcpService))
	router.Set(workflowdomain.NodeTypeSkill, schedulerapp.NewSkillDispatcher(skillService))
	router.Set(workflowdomain.NodeTypeLLM, schedulerapp.NewLLMDispatcher(nil)) // TODO: E15 LLMCaller adapter
	router.Set(workflowdomain.NodeTypeHTTP, schedulerapp.NewHTTPDispatcher(nil))
	router.Set(workflowdomain.NodeTypeCondition, schedulerapp.NewConditionDispatcher())
	router.Set(workflowdomain.NodeTypeLoop, schedulerapp.NewLoopDispatcher())
	router.Set(workflowdomain.NodeTypeParallel, schedulerapp.NewParallelDispatcher())
	router.Set(workflowdomain.NodeTypeApproval, schedulerapp.NewApprovalDispatcher())
	router.Set(workflowdomain.NodeTypeWait, schedulerapp.NewWaitDispatcher())
	router.Set(workflowdomain.NodeTypeVariable, schedulerapp.NewVariableDispatcher())
	schedulerService.SetRouter(router)

	if err := schedulerService.RehydrateOnBoot(context.Background(), ""); err != nil {
		log.Warn("scheduler rehydrate failed (paused runs may need manual resume)", zap.Error(err))
	}

	tools = append(tools, workflowtool.WorkflowExecutionTools(flowrunRepo)...)
	tools = append(tools, mcptool.MCPCallLogTools(mcpCallRepo)...)
	tools = append(tools, skilltool.SkillExecutionTools(skillExecRepo)...)

	chatService.SetTools(tools)

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		log.Error("listen", zap.Error(err))
		os.Exit(1)
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port

	// Electron reads this line from stdout to discover the port.
	fmt.Printf("BACKEND_PORT=%d\n", actualPort)

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
		Dev:                 *dev,
		Tools:               tools,
		LLMFactory:          llmFactory,
		ShellManager:        shells.Manager,
		DB:                  gdb,
		LogBroadcaster:      broadcaster,
		CollectionsDir:      *collectionsDir,
		IntegrationDir:      *integrationDir,
		ForgifyHome:         homeRoot,
		Port:                actualPort,
	})

	// srvBaseCtx ancestors every r.Context(); cancel before srv.Shutdown to unblock SSE handlers.
	srvBaseCtx, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()

	srv := &http.Server{
		Handler:     handler,
		ReadTimeout: 15 * time.Second,
		// WriteTimeout=0: SSE streams may run for minutes.
		IdleTimeout: 60 * time.Second,
		BaseContext: func(_ net.Listener) context.Context { return srvBaseCtx },
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("serve", zap.Error(err))
			stop()
		}
	}()
	log.Info("backend started", zap.Int("port", actualPort), zap.Bool("dev", *dev))

	<-ctx.Done()
	log.Info("shutdown requested")

	cancelBase()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown", zap.Error(err))
	}

	handlerService.Shutdown(shutdownCtx)
}

// registerSandboxStack registers Marketplace V3 runtimes (python/node/uv) and env managers.
//
// registerSandboxStack 注册 Marketplace V3 用到的 runtime（python/node/uv）和 env manager。
// uv pinned to 0.11.4 (0.11.9 lacks GitHub artifact attestation mise verifies).
func registerSandboxStack(svc *sandboxapp.Service, _ *zap.Logger) {
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


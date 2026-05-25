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
	askaiapp "github.com/sunweilin/forgify/backend/internal/app/askai"
	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	skillapp "github.com/sunweilin/forgify/backend/internal/app/skill"
	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	todoapp "github.com/sunweilin/forgify/backend/internal/app/todo"
	userapp "github.com/sunweilin/forgify/backend/internal/app/user"
	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	schedulerapp "github.com/sunweilin/forgify/backend/internal/app/scheduler"
	triggerapp "github.com/sunweilin/forgify/backend/internal/app/trigger"
	catalogapp "github.com/sunweilin/forgify/backend/internal/app/catalog"
	contextmgrapp "github.com/sunweilin/forgify/backend/internal/app/contextmgr"
	hooksapp "github.com/sunweilin/forgify/backend/internal/app/hooks"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
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
	toolsettool "github.com/sunweilin/forgify/backend/internal/app/tool/toolset"
	webtool "github.com/sunweilin/forgify/backend/internal/app/tool/web"
	workflowtool "github.com/sunweilin/forgify/backend/internal/app/tool/workflow"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	documentdomain "github.com/sunweilin/forgify/backend/internal/domain/document"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	relationdomain "github.com/sunweilin/forgify/backend/internal/domain/relation"
	relationapp "github.com/sunweilin/forgify/backend/internal/app/relation"
	relationstore "github.com/sunweilin/forgify/backend/internal/infra/store/relation"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	tododomain "github.com/sunweilin/forgify/backend/internal/domain/todo"
	userdomain "github.com/sunweilin/forgify/backend/internal/domain/user"
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
	userpathpkg "github.com/sunweilin/forgify/backend/internal/pkg/userpath"
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
	userstore "github.com/sunweilin/forgify/backend/internal/infra/store/user"
	workflowstore "github.com/sunweilin/forgify/backend/internal/infra/store/workflow"
	flowrunstore "github.com/sunweilin/forgify/backend/internal/infra/store/flowrun"
	mcpcallstore "github.com/sunweilin/forgify/backend/internal/infra/store/mcpcalls"
	mcphealthstore "github.com/sunweilin/forgify/backend/internal/infra/store/mcphealth"
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
		&userdomain.User{},
		&relationdomain.Relation{},
		&mcpdomain.HealthSnapshot{},
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
	userService := userapp.NewService(userstore.New(gdb), log)
	// Legacy on-disk paths: pre-multi-user installs stored mcp.json / skills
	// / .catalog.json / settings.json under users/local-user/. We keep
	// reading from / writing to that directory for backward compatibility
	// with existing installs. The literal "local-user" is now ONLY a stable
	// directory name — no auth semantics. Fresh installs use the same path
	// (catalog cache, mcp + skill services not yet per-user; deferred).
	//
	// 历史磁盘路径:老的单用户数据放在 users/local-user/;保留以兼容现有
	// 安装。"local-user" 现在仅是稳定目录名,不再有 auth 语义。
	const legacyDefaultUserDir = "local-user"
	if err := userpathpkg.MigrateLegacy(homeRoot, legacyDefaultUserDir,
		"mcp.json", "skills", ".catalog.json", "settings.json"); err != nil {
		log.Warn("legacy path migration", zap.Error(err))
	}
	defaultUserHome, err := userpathpkg.UserHome(homeRoot, legacyDefaultUserDir)
	if err != nil {
		log.Error("user home init", zap.Error(err))
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
	convService.SetKeyProvider(apikeyService) // §12.3 enable ModelOverride 422 validation

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
	// V1.2 multi-user: paths scoped to default user's home (~/.forgify/users/local-user/). Per-user
	// switching today reads from default user's bucket; rebuilding services per-user is V1.5.
	// V1.2 多用户：路径 scope 到默认用户主目录 (~/.forgify/users/local-user/)。
	// 切换 user 时今天仍读默认 user 桶；运行时按 user 重建 service 留 V1.5。
	mcpConfigPath := filepath.Join(defaultUserHome, "mcp.json")
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
	askService.SetNotifications(notificationsPub)
	tools = append(tools, asktool.AskTools(askService)...)

	memoryService := memoryapp.New(memorystore.New(gdb), notificationsPub, log)
	tools = append(tools, memorytool.MemoryTools(memoryService)...)

	documentService := documentapp.New(documentstore.New(gdb), notificationsPub, log)
	tools = append(tools, documenttool.DocumentTools(documentService)...)

	// SubagentTool is appended after Service construction; SetTools runs after the slice is finalized.
	subagentRegistry := subagentapp.NewRegistry()
	subagentService := subagentapp.New(
		chatRepo,
		subagentRegistry,
		modelService,
		apikeyService,
		llmFactory,
		log,
	)
	tools = append(tools, subagenttool.SubagentTools(subagentService)...)
	subagentService.SetTools(tools)

	// §S9 detached ctx: boot publishStatus needs a user id in ctx. mcp +
	// skill aren't per-user yet — they share the legacyDefaultUserDir on
	// disk, so use that same string as the ctx user id. When mcp/skill
	// move to true per-user storage, replace with iterate-users pattern.
	//
	// §S9 detached ctx:启动时 publishStatus 需要 user id;mcp+skill 还
	// 没真正 per-user,沿用 legacyDefaultUserDir 作为 ctx user id。
	mcpBootCtx := reqctxpkg.SetUserID(context.Background(), legacyDefaultUserDir)
	if err := mcpService.Start(mcpBootCtx); err != nil {
		log.Warn("mcp start partial failure (some servers may be unreachable)", zap.Error(err))
	}
	tools = append(tools, mcptool.MCPTools(mcpService)...)

	skillService := skillapp.New(
		filepath.Join(defaultUserHome, "skills"),
		subagentService,
		modelService,
		apikeyService,
		llmFactory,
		notificationsPub,
		log,
	)
	skillBootCtx := reqctxpkg.SetUserID(context.Background(), legacyDefaultUserDir)
	if err := skillService.Start(skillBootCtx); err != nil {
		log.Warn("skill start failed (continuing with empty cache)", zap.Error(err))
	}
	tools = append(tools, skilltool.SkillTools(skillService)...)

	catalogService := catalogapp.New(log)
	// Sources = all LLM-callable capabilities: function / handler / skill / mcp /
	// workflow / document. The menu renderer shows each group's invoke tool so the
	// LLM knows exactly which tool-call to emit.
	//
	// sources = 全部 LLM 可调能力：function / handler / skill / mcp / workflow / document。
	// menu 渲染带 invokeTool，让 LLM 确知发哪个 tool-call。
	catalogService.RegisterSource(functionService.AsCatalogSource())
	catalogService.RegisterSource(handlerService.AsCatalogSource())
	catalogService.RegisterSource(skillService.AsCatalogSource())
	catalogService.RegisterSource(mcpService.AsCatalogSource())
	catalogService.RegisterSource(workflowService.AsCatalogSource())
	catalogService.RegisterSource(documentService.AsCatalogSource())
	chatService.SetSystemPromptProvider(catalogService)
	chatService.SetMemoryProvider(memoryService)
	chatService.SetDocumentResolver(documentService)
	chatService.RegisterMentionResolver(documentService.AsMentionResolver())
	chatService.RegisterMentionResolver(functionService.AsMentionResolver())
	chatService.RegisterMentionResolver(handlerService.AsMentionResolver())
	chatService.RegisterMentionResolver(workflowService.AsMentionResolver())

	// V1.2 §3 final-sweep — permissions + hooks.
	// settings.json lives at <homeRoot>/settings.json; gate reads via
	// SettingsService snapshot; HookRunner consumes the same snapshot.
	// V1.2 §3 final-sweep —— permissions + hooks。settings.json 在
	// <homeRoot>/settings.json；gate 经 SettingsService 快照读；
	// HookRunner 共用此快照。
	settingsService := settingsinfra.New(filepath.Join(defaultUserHome, "settings.json"), log)
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
	workflowChecker.Document = documentService

	// Relation domain — live-derived cross-entity edge graph. Wire AFTER all source
	// domain services are constructed (so we can pass readers), THEN call each
	// source's SetRelationSyncer to inject the cycle-broken hook port.
	//
	// Relation domain —— live-derived 跨实体关系图。所有 source 服务装配完后再装
	// (可传 reader)；然后逐个 SetRelationSyncer 反注入避循环依赖。
	relationService := relationapp.NewService(relationapp.Config{
		Repo:               relationstore.New(gdb),
		FunctionReader:     functionService,
		HandlerReader:      handlerService,
		WorkflowReader:     workflowService,
		DocumentReader:     documentService,
		SkillReader:        skillService,
		McpReader:          mcpService,
		ConversationReader: convService,
		Log:                log,
	})
	workflowService.SetRelationSyncer(relationService)
	functionService.SetRelationSyncer(relationService)
	handlerService.SetRelationSyncer(relationService)
	documentService.SetRelationSyncer(relationService)
	convService.SetRelationSyncer(relationService)
	mcpService.SetRelationSyncer(relationService)
	skillService.SetRelationSyncer(relationService)

	// V1.2 §17 askai spawner: shared infrastructure for forge :iterate +
	// flowrun :triage endpoints. Creates user-visible conversation, system-
	// prompts it, sends initial user message, returns conversationId.
	//
	// V1.2 §17 askai spawner：forge :iterate + flowrun :triage 端点共享基础设施。
	// 起用户可见对话、注入 system prompt、发首个用户消息、返 conversationId。
	askaiSpawner := askaiapp.New(convService, chatService, log)

	flowrunRepo := flowrunstore.New(gdb)
	mcpCallRepo := mcpcallstore.New(gdb)
	mcpHealthRepo := mcphealthstore.New(gdb)
	skillExecRepo := skillexecstore.New(gdb)
	mcpService.SetCallRepo(mcpCallRepo)
	mcpService.SetHealthHistoryRepo(mcpHealthRepo)
	skillService.SetExecRepo(skillExecRepo)
	// §4.5 metrics dashboard reuses these execution-log repos.
	functionExecRepo := functionstore.New(gdb)
	handlerCallRepo := handlerstore.New(gdb)

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
	router.Set(workflowdomain.NodeTypeLLM, schedulerapp.NewLLMDispatcher(
		schedulerapp.NewDefaultLLMCaller(modelService, apikeyService, llmFactory),
		documentService,
	))
	router.Set(workflowdomain.NodeTypeAgent, schedulerapp.NewAgentDispatcher(
		modelService, apikeyService, llmFactory,
		documentService, func() []toolapp.Tool { return tools }, log,
	))
	router.Set(workflowdomain.NodeTypeHTTP, schedulerapp.NewHTTPDispatcher(nil))
	router.Set(workflowdomain.NodeTypeCondition, schedulerapp.NewConditionDispatcher())
	router.Set(workflowdomain.NodeTypeLoop, schedulerapp.NewLoopDispatcher(schedulerService))
	router.Set(workflowdomain.NodeTypeParallel, schedulerapp.NewParallelDispatcher())
	router.Set(workflowdomain.NodeTypeApproval, schedulerapp.NewApprovalDispatcher())
	router.Set(workflowdomain.NodeTypeWait, schedulerapp.NewWaitDispatcher())
	router.Set(workflowdomain.NodeTypeVariable, schedulerapp.NewVariableDispatcher())
	schedulerService.SetRouter(router)

	// §multi-user: rehydrate paused FlowRuns for every user, not just default.
	// §multi-user: 给每个 user 都 rehydrate paused FlowRun，不止默认 user。
	if users, err := userService.List(context.Background()); err == nil {
		for _, u := range users {
			if err := schedulerService.RehydrateOnBoot(context.Background(), u.ID); err != nil {
				log.Warn("scheduler rehydrate failed (paused runs may need manual resume)",
					zap.String("user_id", u.ID), zap.Error(err))
			}
		}
	} else {
		log.Warn("rehydrate: skipped (user list failed)", zap.Error(err))
	}

	tools = append(tools, workflowtool.WorkflowExecutionTools(flowrunRepo)...)
	tools = append(tools, workflowtool.WorkflowTriggerTool(schedulerService)...)
	tools = append(tools, mcptool.MCPCallLogTools(mcpCallRepo)...)
	tools = append(tools, skilltool.SkillExecutionTools(skillExecRepo)...)

	// Partition into Resident + Lazy groups; activate_tools is injected as RESIDENT.
	// T8 will switch host.Tools() to return only the activated subset; today All() = full set.
	//
	// 分拆为 Resident + Lazy 组；activate_tools 注入为 RESIDENT。
	// T8 将把 host.Tools() 改成只返已激活子集；目前 All() = 全集。
	ts := buildToolset(tools)
	ts.Resident = append(ts.Resident, toolsettool.NewActivateTools(ts))
	chatService.SetToolset(ts)
	tools = ts.All()

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
		RelationService:     relationService,
		AskAISpawner:        askaiSpawner,
		UserService:         userService,
		FunctionExecRepo:    functionExecRepo,
		HandlerCallRepo:     handlerCallRepo,
		MCPCallRepo:         mcpCallRepo,
		SkillExecRepo:       skillExecRepo,
		SettingsService:     settingsService,
		SettingsPath:        settingsPath,
		PermGate:            permGate,
		Dev:                 *dev,
		Tools:               tools,
		SubagentRegistry:    subagentRegistry,
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

	// §13.4: drain background polling goroutines so SIGTERM doesn't orphan them until OS reaps.
	// §13.4: 关停 polling goroutine,避免 SIGTERM 时 OS 兜底回收。
	skillService.Stop()
	if err := mcpService.Stop(shutdownCtx); err != nil {
		log.Warn("mcp stop", zap.Error(err))
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

// lazyGroups is the closed name→category mapping for tools that belong to lazy groups.
//
// lazyGroups 是 lazy 工具 Name()→category 的封闭映射。
var lazyGroups = map[string]string{
	// function group
	"create_function":           "function",
	"edit_function":             "function",
	"delete_function":           "function",
	"revert_function":           "function",
	"get_function":              "function",
	"get_function_execution":    "function",
	"search_function_executions": "function",
	// handler group
	"create_handler":       "handler",
	"edit_handler":         "handler",
	"delete_handler":       "handler",
	"revert_handler":       "handler",
	"get_handler":          "handler",
	"update_handler_config": "handler",
	"get_handler_call":     "handler",
	"search_handler_calls": "handler",
	// workflow group
	"create_workflow":            "workflow",
	"edit_workflow":              "workflow",
	"delete_workflow":            "workflow",
	"revert_workflow":            "workflow",
	"get_workflow":               "workflow",
	"get_workflow_execution":     "workflow",
	"search_workflow_executions": "workflow",
	"trigger_workflow":           "workflow",
	// mcp group
	"call_mcp_tool":        "mcp",
	"install_mcp_server":   "mcp",
	"uninstall_mcp_server": "mcp",
	"list_mcp_marketplace": "mcp",
	"get_mcp_call":         "mcp",
	"search_mcp_calls":     "mcp",
	// document group
	"create_document":  "document",
	"edit_document":    "document",
	"delete_document":  "document",
	"move_document":    "document",
	"read_document":    "document",
	"list_documents":   "document",
	"search_documents": "document",
	// skill group
	"get_skill_execution":    "skill",
	"search_skill_executions": "skill",
}

// residentToolNames is the closed whitelist of tools that are always-present (resident).
// activate_tools is NOT listed here: it is appended to ts.Resident after buildToolset returns,
// so it never passes through this function.
//
// residentToolNames 是常驻工具的封闭白名单。
// activate_tools 不在此列，因为它在 buildToolset 返回后才追加。
var residentToolNames = map[string]bool{
	"search_function":  true,
	"search_handler":   true,
	"search_workflow":  true,
	"search_skills":    true,
	"search_mcp_tools": true,
	"run_function":     true,
	"call_handler":     true,
	"Read":             true,
	"Write":            true,
	"Edit":             true,
	"Grep":             true,
	"Glob":             true,
	"Bash":             true,
	"BashOutput":       true,
	"KillShell":        true,
	"WebSearch":        true,
	"WebFetch":         true,
	"AskUserQuestion":  true,
	"TodoCreate":       true,
	"TodoUpdate":       true,
	"TodoList":         true,
	"TodoGet":          true,
	"read_memory":      true,
	"write_memory":     true,
	"forget_memory":    true,
	"activate_skill":   true,
	"Subagent":         true,
}

// buildToolset partitions all assembled tools into Resident + Lazy groups using two closed maps.
// A tool whose Name() appears in lazyGroups is placed in that lazy category.
// A tool whose Name() appears in residentToolNames is placed in Resident.
// Any Name() absent from both maps causes a panic at startup — misconfiguration must not be silent.
// Note: activate_tools is injected into ts.Resident by the caller after this function returns,
// so it does not need to appear in either map.
//
// buildToolset 用两张封闭表把工具分入 Resident + Lazy 组。
// 两张表都没有的 Name() 会在启动时 panic——错误配置不得静默。
func buildToolset(all []toolapp.Tool) toolapp.Toolset {
	ts := toolapp.Toolset{
		Lazy: make(map[string][]toolapp.Tool),
	}
	for _, t := range all {
		name := t.Name()
		if cat, ok := lazyGroups[name]; ok {
			ts.Lazy[cat] = append(ts.Lazy[cat], t)
		} else if residentToolNames[name] {
			ts.Resident = append(ts.Resident, t)
		} else {
			panic(fmt.Sprintf("buildToolset: tool %q is not classified — add it to lazyGroups or residentToolNames", name))
		}
	}
	return ts
}


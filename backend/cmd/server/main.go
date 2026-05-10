// Command server boots the Forgify backend: logger, DB, HTTP router with
// middleware chain, and graceful shutdown.
//
// Command server 启动 Forgify 后端：logger、DB、带中间件链的 HTTP 路由、优雅关闭。
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
	forgeapp "github.com/sunweilin/forgify/backend/internal/app/forge"
	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	skillapp "github.com/sunweilin/forgify/backend/internal/app/skill"
	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	todoapp "github.com/sunweilin/forgify/backend/internal/app/todo"
	catalogapp "github.com/sunweilin/forgify/backend/internal/app/catalog"
	asktool "github.com/sunweilin/forgify/backend/internal/app/tool/ask"
	fstool "github.com/sunweilin/forgify/backend/internal/app/tool/filesystem"
	forgetool "github.com/sunweilin/forgify/backend/internal/app/tool/forge"
	mcptool "github.com/sunweilin/forgify/backend/internal/app/tool/mcp"
	searchtool "github.com/sunweilin/forgify/backend/internal/app/tool/search"
	shelltool "github.com/sunweilin/forgify/backend/internal/app/tool/shell"
	skilltool "github.com/sunweilin/forgify/backend/internal/app/tool/skill"
	subagenttool "github.com/sunweilin/forgify/backend/internal/app/tool/subagent"
	todotool "github.com/sunweilin/forgify/backend/internal/app/tool/todo"
	webtool "github.com/sunweilin/forgify/backend/internal/app/tool/web"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	tododomain "github.com/sunweilin/forgify/backend/internal/domain/todo"
	cryptoinfra "github.com/sunweilin/forgify/backend/internal/infra/crypto"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	eventloginfra "github.com/sunweilin/forgify/backend/internal/infra/eventlog"
	notificationsinfra "github.com/sunweilin/forgify/backend/internal/infra/notifications"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	loggerinfra "github.com/sunweilin/forgify/backend/internal/infra/logger"
	mcpinfra     "github.com/sunweilin/forgify/backend/internal/infra/mcp"
	sandboxinfra "github.com/sunweilin/forgify/backend/internal/infra/sandbox"
	apikeystore "github.com/sunweilin/forgify/backend/internal/infra/store/apikey"
	chatstore "github.com/sunweilin/forgify/backend/internal/infra/store/chat"
	convstore "github.com/sunweilin/forgify/backend/internal/infra/store/conversation"
	forgestore "github.com/sunweilin/forgify/backend/internal/infra/store/forge"
	modelstore "github.com/sunweilin/forgify/backend/internal/infra/store/model"
	sandboxstore "github.com/sunweilin/forgify/backend/internal/infra/store/sandbox"
	todostore "github.com/sunweilin/forgify/backend/internal/infra/store/todo"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
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

	// Resolve forgifyHome. In dev mode we root it under data-dir so
	// `make clear` (which rm -rf's data-dir) wipes mcp.json + skills +
	// catalog cache too — dev sessions stay isolated from the user's
	// real ~/.forgify/ install. Prod / Wails use real ~/.forgify/.
	//
	// 解析 forgifyHome。dev 模式下根到 data-dir 让 `make clear`（rm -rf
	// data-dir）连带清 mcp.json + skills + catalog cache——dev session 与
	// 真用户 ~/.forgify/ 隔离。Prod / Wails 走真 ~/.forgify/。
	homeRoot := *forgifyHome
	if homeRoot == "" {
		if *dev && *dataDir != "" {
			homeRoot = filepath.Join(*dataDir, ".forgify")
		} else if h, err := os.UserHomeDir(); err == nil && h != "" {
			homeRoot = filepath.Join(h, ".forgify")
		} else {
			homeRoot = ".forgify" // working-dir fallback
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
		&chatdomain.Block{}, // message_blocks table (event-log协议 unified shape)
		&chatdomain.Attachment{},
		&forgedomain.Forge{},
		&forgedomain.ForgeVersion{},
		&forgedomain.ForgeTestCase{},
		&forgedomain.ForgeExecution{},
		&sandboxdomain.Runtime{},
		&sandboxdomain.Env{},
		&tododomain.Todo{},
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

	modelService := modelapp.NewService(modelstore.New(gdb), log)

	llmFactory := llminfra.NewFactory()

	// TE-5a: in --dev mode, enable LLM call tracing so testend's Wire
	// tab can replay every Stream() call (request + emitted events
	// + final text + elapsed). Production unchanged — tracer stays
	// nil so Build returns the raw provider client unwrapped.
	//
	// TE-5a：--dev 模式启 LLM 调用 tracing 让 testend Wire tab 能 replay
	// 每个 Stream() 调用（请求 + 发出的 events + 最终文字 + 耗时）。生产
	// 不动——tracer 保持 nil 让 Build 返不带包装的原 provider client。
	if *dev {
		llmFactory.SetTracer(llminfra.NewTraceRecorder())
		log.Info("LLM trace recorder enabled (--dev) — testend Wire tab will replay every Stream call")
	}

	// forgeLLMClient satisfies forgeapp.LLMClient for GenerateTestCases
	// (non-streaming JSON calls only).
	//
	// forgeLLMClient 满足 forgeapp.LLMClient 接口，仅用于 GenerateTestCases
	// 的非流式 JSON 调用。
	forgeLLM := &forgeLLMClientAdapter{
		picker:  modelService,
		keys:    apikeyService,
		factory: llmFactory,
	}
	eventLogBridge := eventloginfra.NewBridge(log)
	notificationsBridge := notificationsinfra.NewBridge(log)
	notificationsPub := notificationspkg.New(notificationsBridge, log)
	convService := convapp.NewService(convstore.New(gdb), notificationsPub, log)

	// PluginSandbox v2 — unified runtime/env service. Bootstrap extracts
	// the embedded mise binary; failure flips degraded mode (chat-only
	// path stays alive) but is non-fatal. After Bootstrap we register
	// installers + env managers covering all v1 supported runtimes.
	//
	// PluginSandbox v2 ——统一 runtime/env 服务。Bootstrap 解 embed mise；
	// 失败翻 degraded mode（chat-only 路径保活）但不致命。Bootstrap 后注册
	// 覆盖所有 v1 支持 runtime 的 installer + env manager。
	sandboxRepo := sandboxstore.New(gdb)
	sandboxSvc := sandboxapp.New(sandboxRepo, *dataDir, notificationsPub, log)
	if err := sandboxSvc.Bootstrap(context.Background()); err != nil {
		log.Warn("sandbox v2 bootstrap failed (degraded mode active; runtime ops will fail)",
			zap.Error(err))
	}
	registerSandboxStack(sandboxSvc, log)

	forgeService := forgeapp.NewService(
		forgestore.New(gdb),
		forgeapp.NewSandboxAdapter(sandboxSvc, *dataDir),
		forgeLLM,
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

	// PathGuard for filesystem tools — denies a curated list of sensitive
	// paths (~/.ssh / ~/.aws / ~/.gnupg / ~/.netrc / ~/.config/git-credentials
	// / ~/.forgify/ / system paths). See pkg/pathguard for the full list and
	// 02-tools-deep/03-shell.md decision D5 for why we use a thin deny-list
	// rather than OS-level sandboxing.
	//
	// 文件系统 tool 的 PathGuard——拒绝精选的敏感路径清单。详见 pkg/pathguard
	// 与 02-tools-deep/03-shell.md 决策 D5。
	pathGuard := pathguardpkg.NewDefault()

	// MCP Service constructed here (before WebTools) so the
	// WebSearch tool can route through the duckduckgo-search MCP server
	// when installed. mcpService.Start is called later (right after the
	// tool slice is finalized) — the SearchRouter wrapper checks server
	// status at call time, so pre-Start invocations safely fall through
	// to Bing CN.
	//
	// MCP Service 构造提前到 WebTools 之前，让 WebSearch 在 duckduckgo-search
	// MCP server 已装时能路由过去。mcpService.Start 留在 tool 切片定稿后调
	// ——SearchRouter 在调用时查 server 状态，pre-Start 调用安全降级到 Bing CN。
	mcpConfigPath := filepath.Join(homeRoot, "mcp.json")
	// Marketplace V3 (2026-05-08, post-curation): the upstream
	// registry.modelcontextprotocol.io has 5000+ entries of mixed quality
	// (mostly broken, abandoned, or API-key-required). We replaced it with
	// a hardcoded curated catalog of 21 high-value MCP servers verified to
	// install + run out of the box. No HTTP, no flakiness, predictable list.
	//
	// Marketplace V3：上游 registry 5000+ 条多数不可用，换成 21 条精选
	// hardcoded 目录，每条都验证过装上即可用。无 HTTP / 无抖动 / 列表稳定。
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

	tools := forgetool.ForgeTools(
		forgeService,
		chatRepo,
		modelService,
		apikeyService,
		llmFactory,
		log,
	)
	tools = append(tools, fstool.FilesystemTools(pathGuard)...)
	tools = append(tools, searchtool.SearchTools(pathGuard, log)...)
	tools = append(tools, webtool.WebTools(modelService, apikeyService, llmFactory, mcpapp.NewSearchRouter(mcpService), log)...)
	shells := shelltool.NewShellTools(sandboxSvc)
	defer shells.Manager.Stop() // graceful shutdown: kill any background children
	tools = append(tools, shells.Tools...)

	todoService := todoapp.NewService(todostore.New(gdb), notificationsPub, log)
	tools = append(tools, todotool.TodoTools(todoService)...)
	askService := askapp.NewService()
	tools = append(tools, asktool.AskTools(askService)...)

	// Subagent: Service holds back-refs to the global tool list, so it's
	// constructed before the SubagentTool is appended; tools.SetTools is
	// called after the slice is finalized so Service.filterTools sees
	// every other tool. The structural recursion defense in Service
	// .filterTools strips the SubagentTool itself before passing the
	// list to a sub-runner — the sub-LLM physically can't see "Subagent".
	//
	// Subagent：Service 持全局 tool 列表反向引用——构造在 SubagentTool 加入
	// 之前；slice 终稿后再 SetTools 让 filterTools 看到所有其他 tool。
	// Service.filterTools 的结构性防递归会在传给 sub-runner 前剥掉 SubagentTool
	// 自身——sub-LLM 物理看不到 "Subagent"。
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

	// MCP Start: load ~/.forgify/mcp.json + parallel-Connect all configured
	// servers (30s per-server handshake timeout; per-server failures captured
	// in ServerStatus, don't block boot). Service struct itself was constructed
	// above (so WebSearch could see the SearchRouter); Start runs here once
	// the full tool slice is being assembled.
	//
	// MCP Start：加载 ~/.forgify/mcp.json + 并发 Connect 所有 server（30s 握手
	// 超时；per-server 失败记到 ServerStatus，不挡 boot）。Service struct 在
	// 上方已构造（让 WebSearch 能见 SearchRouter）；tool 切片装配中跑 Start。
	if err := mcpService.Start(context.Background()); err != nil {
		log.Warn("mcp start partial failure (some servers may be unreachable)", zap.Error(err))
	}
	tools = append(tools, mcptool.MCPTools(mcpService)...)

	// Skill: scan ~/.forgify/skills/ for any installed Anthropic Agent
	// Skills + start the 1s polling loop for live rescan on user edits.
	// Same boot-don't-block discipline as MCP — empty skills dir is the
	// typical first-launch case.
	//
	// Skill：扫 ~/.forgify/skills/ 把已装 Agent Skill 元数据缓存好 + 启
	// 1s 轮询让用户编辑时实时重扫。同 MCP 不挡 boot 纪律——首次启动 skills
	// 目录通常为空。
	skillService := skillapp.New(
		filepath.Join(homeRoot, "skills"),
		subagentService,
		modelService,
		apikeyService,
		llmFactory,
		notificationsPub,
		log,
	)
	if err := skillService.Start(context.Background()); err != nil {
		log.Warn("skill start failed (continuing with empty cache)", zap.Error(err))
	}
	tools = append(tools, skilltool.SkillTools(skillService)...)

	// Capability Catalog: subscribes to forge / skill / mcp via the
	// CatalogSource port, polls every 1s with fingerprint short-circuit,
	// regenerates the system-prompt summary via LLM (3-attempt retry +
	// mechanical fallback) when descriptions change. The summary is
	// what teaches the LLM "what categories of capabilities you have +
	// when to prefer one over another". subagent NOT registered — its
	// own tool description already enumerates subagent types
	// (catalog.md §1).
	//
	// Catalog 订阅 forge / skill / mcp 经 CatalogSource 接口，每 1s
	// 轮询 fingerprint 短路，description 变时经 LLM regen system-prompt
	// summary（3 attempt + mechanical fallback）。summary 教 LLM "你有
	// 哪些类目能力 + 何时优先何者"。subagent 不注册——其 tool description
	// 已枚举 subagent 类型（catalog.md §1）。
	catalogService := catalogapp.New(filepath.Join(homeRoot, ".catalog.json"), notificationsPub, log)
	catalogService.SetGenerator(catalogapp.NewLLMGenerator(modelService, apikeyService, llmFactory, log))
	catalogService.RegisterSource(forgeService.AsCatalogSource())
	catalogService.RegisterSource(skillService.AsCatalogSource())
	catalogService.RegisterSource(mcpService.AsCatalogSource())
	if err := catalogService.Start(context.Background()); err != nil {
		log.Warn("catalog start failed (continuing without catalog injection)", zap.Error(err))
	}
	chatService.SetSystemPromptProvider(catalogService)

	chatService.SetTools(tools)

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		log.Error("listen", zap.Error(err))
		os.Exit(1)
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port

	// Electron reads this line from stdout to discover the port.
	// Electron 从 stdout 读取此行发现端口。
	fmt.Printf("BACKEND_PORT=%d\n", actualPort)

	handler := routerhttpapi.New(routerhttpapi.Deps{
		Log:                 log,
		APIKeyService:       apikeyService,
		ModelService:        modelService,
		ConversationService: convService,
		ForgeService:        forgeService,
		ChatService:         chatService,
		EventLogBridge:      eventLogBridge,
		BlockV2Repo:         chatRepo,
		NotificationsBridge: notificationsBridge,
		AskService:          askService,
		SandboxService:      sandboxSvc,
		SubagentService:     subagentService,
		MCPService:          mcpService,
		SkillService:        skillService,
		CatalogService:      catalogService,
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

	// srvBaseCtx is the ancestor of every request's r.Context(). On Ctrl+C
	// we cancel it BEFORE srv.Shutdown so SSE handlers (eventlog /
	// notifications) selecting on r.Context().Done() unblock instantly —
	// otherwise srv.Shutdown waits until the 5s timeout because stdlib
	// doesn't cancel request contexts on Shutdown.
	//
	// srvBaseCtx 是所有 r.Context() 的祖先。Ctrl+C 时在 srv.Shutdown 前
	// 先 cancel 它，让 SSE handler（eventlog/notifications）的 select
	// 立刻解开——否则 stdlib 不会因 Shutdown 主动 cancel request ctx，
	// 长连接 SSE 会撑满 5 秒超时。
	srvBaseCtx, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()

	srv := &http.Server{
		Handler:     handler,
		ReadTimeout: 15 * time.Second,
		// WriteTimeout=0: SSE streams may run for minutes.
		// WriteTimeout=0：SSE 流可能持续几分钟。
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

	// Unblock SSE handlers first, then graceful Shutdown waits idle
	// connections to drain. Without cancelBase() Shutdown burns the
	// full 5s every Ctrl+C.
	//
	// 先解开 SSE handler，再 graceful Shutdown 等空闲连接 drain。
	// 不 cancelBase() 的话每次 Ctrl+C 都吃满 5s 超时。
	cancelBase()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown", zap.Error(err))
	}
}

// forgeLLMClientAdapter satisfies forgeapp.LLMClient using infra/llm.
// Used only for non-streaming calls (GenerateTestCases).
//
// forgeLLMClientAdapter 用 infra/llm 满足 forgeapp.LLMClient 接口，
// 仅用于非流式调用（GenerateTestCases）。
type forgeLLMClientAdapter struct {
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
}

func (c *forgeLLMClientAdapter) Generate(ctx context.Context, prompt string) (string, error) {
	bc, err := llmclientpkg.Resolve(ctx, c.picker, c.keys, c.factory)
	if err != nil {
		return "", fmt.Errorf("forgeLLMClient: %w", err)
	}
	return llminfra.Generate(ctx, bc.Client, llminfra.Request{
		ModelID:  bc.ModelID,
		Key:      bc.Key,
		BaseURL:  bc.BaseURL,
		Messages: []llminfra.LLMMessage{{Role: llminfra.RoleUser, Content: prompt}},
	})
}

// registerSandboxStack wires the v1 PluginSandbox runtime/env matrix —
// 4 installer kinds + 11 env managers — onto the freshly-bootstrapped
// service. Order doesn't matter; idempotent re-registration is fine.
//
// Kept as a top-level helper rather than inlined in main() so the
// service-construction block stays scannable and the registry table
// reads as one coherent block.
//
// registerSandboxStack 把 curated marketplace 用到的 runtime/env 挂到 service。
// 砍剩 python + node + uv（Marketplace V3 = npm + pypi only），其他语言 / docker /
// playwright 已删除（curated 21 条目都是 npx / uvx 起的纯 stdio server）。
//
// 提为顶层 helper 让 service 构造段易读。
func registerSandboxStack(svc *sandboxapp.Service, _ *zap.Logger) {
	miseBin := svc.MiseBin()
	if miseBin == "" {
		// Bootstrap failed — nothing to register that depends on mise.
		// Bootstrap 失败——依赖 mise 的不注册。
		return
	}

	// Mise-managed runtimes — npm + pypi only (Marketplace V3).
	// uv pin: 0.11.9 ships without the GitHub artifact attestation mise
	// verifies — install fails on "expected workflow .../release.yml,
	// found certificate: None". 0.11.4 is last known-good.
	//
	// Mise 管的 runtime——curated marketplace 仅 npm + pypi。uv 钉 0.11.4：
	// 0.11.9 缺 mise 校验的 GitHub attestation。
	for kind, defaultVer := range map[string]string{
		"python": "3.12",
		"node":   "22",
		"uv":     "0.11.4",
	} {
		svc.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, kind, defaultVer))
	}

	// Env managers — only python + node (Marketplace V3 runtimes).
	// Env manager——仅 python + node。
	svc.RegisterEnvManager(sandboxinfra.NewPythonEnvManager(svc))
	svc.RegisterEnvManager(sandboxinfra.NewNodeEnvManager())
}


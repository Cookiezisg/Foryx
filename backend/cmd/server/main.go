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
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	apikeyapp "github.com/sunweilin/forgify/backend/internal/app/apikey"
	askapp "github.com/sunweilin/forgify/backend/internal/app/ask"
	chatapp "github.com/sunweilin/forgify/backend/internal/app/chat"
	convapp "github.com/sunweilin/forgify/backend/internal/app/conversation"
	forgeapp "github.com/sunweilin/forgify/backend/internal/app/forge"
	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	taskapp "github.com/sunweilin/forgify/backend/internal/app/task"
	asktool "github.com/sunweilin/forgify/backend/internal/app/tool/ask"
	fstool "github.com/sunweilin/forgify/backend/internal/app/tool/filesystem"
	forgetool "github.com/sunweilin/forgify/backend/internal/app/tool/forge"
	searchtool "github.com/sunweilin/forgify/backend/internal/app/tool/search"
	shelltool "github.com/sunweilin/forgify/backend/internal/app/tool/shell"
	tasktool "github.com/sunweilin/forgify/backend/internal/app/tool/task"
	webtool "github.com/sunweilin/forgify/backend/internal/app/tool/web"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	taskdomain "github.com/sunweilin/forgify/backend/internal/domain/task"
	cryptoinfra "github.com/sunweilin/forgify/backend/internal/infra/crypto"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	memoryinfra "github.com/sunweilin/forgify/backend/internal/infra/events/memory"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	loggerinfra "github.com/sunweilin/forgify/backend/internal/infra/logger"
	sandboxinfra "github.com/sunweilin/forgify/backend/internal/infra/sandbox"
	apikeystore "github.com/sunweilin/forgify/backend/internal/infra/store/apikey"
	chatstore "github.com/sunweilin/forgify/backend/internal/infra/store/chat"
	convstore "github.com/sunweilin/forgify/backend/internal/infra/store/conversation"
	forgestore "github.com/sunweilin/forgify/backend/internal/infra/store/forge"
	modelstore "github.com/sunweilin/forgify/backend/internal/infra/store/model"
	sandboxstore "github.com/sunweilin/forgify/backend/internal/infra/store/sandbox"
	taskstore "github.com/sunweilin/forgify/backend/internal/infra/store/task"
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
	flag.Parse()

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
		&chatdomain.Block{}, // message_blocks table (chat infra refactor)
		&chatdomain.Attachment{},
		&forgedomain.Forge{},
		&forgedomain.ForgeVersion{},
		&forgedomain.ForgeTestCase{},
		&forgedomain.ForgeExecution{},
		&sandboxdomain.Runtime{},
		&sandboxdomain.Env{},
		&taskdomain.Task{},
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
	convService := convapp.NewService(convstore.New(gdb), log)

	llmFactory := llminfra.NewFactory()

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
	eventsBridge := memoryinfra.NewBridge(log)

	// PluginSandbox v2 — unified runtime/env service. Bootstrap extracts
	// the embedded mise binary; failure flips degraded mode (chat-only
	// path stays alive) but is non-fatal. After Bootstrap we register
	// installers + env managers covering all v1 supported runtimes.
	//
	// PluginSandbox v2 ——统一 runtime/env 服务。Bootstrap 解 embed mise；
	// 失败翻 degraded mode（chat-only 路径保活）但不致命。Bootstrap 后注册
	// 覆盖所有 v1 支持 runtime 的 installer + env manager。
	sandboxRepo := sandboxstore.New(gdb)
	sandboxSvc := sandboxapp.New(sandboxRepo, *dataDir, log)
	if err := sandboxSvc.Bootstrap(context.Background()); err != nil {
		log.Warn("sandbox v2 bootstrap failed (degraded mode active; runtime ops will fail)",
			zap.Error(err))
	}
	registerSandboxStack(sandboxSvc)

	forgeService := forgeapp.NewService(
		forgestore.New(gdb),
		forgeapp.NewSandboxAdapter(sandboxSvc, *dataDir),
		forgeLLM,
		eventsBridge,
		log,
	)

	chatRepo := chatstore.New(gdb)
	chatService := chatapp.NewService(
		chatRepo,
		convstore.New(gdb),
		modelService,
		apikeyService,
		llmFactory,
		eventsBridge,
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

	tools := forgetool.ForgeTools(
		forgeService,
		chatRepo,
		modelService,
		apikeyService,
		llmFactory,
	)
	tools = append(tools, fstool.FilesystemTools(pathGuard)...)
	tools = append(tools, searchtool.SearchTools(pathGuard)...)
	tools = append(tools, webtool.WebTools(modelService, apikeyService, llmFactory)...)
	shells := shelltool.NewShellTools()
	defer shells.Manager.Stop() // graceful shutdown: kill any background children
	tools = append(tools, shells.Tools...)

	taskService := taskapp.NewService(taskstore.New(gdb), eventsBridge, log)
	tools = append(tools, tasktool.TaskTools(taskService)...)
	askService := askapp.NewService()
	tools = append(tools, asktool.AskTools(askService)...)
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
		EventsBridge:        eventsBridge,
		AskService:          askService,
		Dev:                 *dev,
		Tools:               tools,
		DB:                  gdb,
		LogBroadcaster:      broadcaster,
		CollectionsDir:      *collectionsDir,
		IntegrationDir:      *integrationDir,
		Port:                actualPort,
	})

	srv := &http.Server{
		Handler:     handler,
		ReadTimeout: 15 * time.Second,
		// WriteTimeout=0: SSE streams may run for minutes.
		// WriteTimeout=0：SSE 流可能持续几分钟。
		IdleTimeout: 60 * time.Second,
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
// registerSandboxStack 把 v1 PluginSandbox runtime/env 矩阵——4 installer
// kind + 11 env manager——挂到刚 bootstrap 的 service 上。顺序无关，重复
// 注册幂等。
//
// 提为顶层 helper 而非内联 main() 让 service 构造段易读，注册表作整段连贯。
func registerSandboxStack(svc *sandboxapp.Service) {
	miseBin := svc.MiseBin()
	if miseBin == "" {
		// Bootstrap failed — nothing to register that depends on mise.
		// Static binary installer is mise-independent but skipped too;
		// degraded mode means runtime ops fail uniformly.
		//
		// Bootstrap 失败——依赖 mise 的不注册。Static binary installer 不依赖
		// mise 但也跳过；degraded mode 让 runtime ops 一致 fail。
		return
	}

	// Mise-managed runtimes (7 main langs + 5 support tools).
	// Mise 管的 runtime（7 主流语言 + 5 支持工具）。
	for kind, defaultVer := range map[string]string{
		"python": "3.12",
		"node":   "22",
		"rust":   "stable",
		"java":   "21",
		"go":     "1.22",
		"ruby":   "3.3",
		"php":    "8.3",
		// Support tools — used by EnvManagers via ToolRegistry.
		// 支持工具——EnvManager 通过 ToolRegistry 用。
		"uv":       "",
		"pnpm":     "",
		"maven":    "",
		"bundler":  "",
		"composer": "",
	} {
		svc.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, kind, defaultVer))
	}

	// Specialty installers.
	// 专用 installer。
	svc.RegisterInstaller(sandboxinfra.NewDotnetInstaller("8.0"))
	// PlaywrightInstaller takes a CLI path; resolved per-call inside the
	// EnvManager so we don't register it as a global RuntimeInstaller —
	// PlaywrightEnvManager owns the install logic via its Node delegate.
	// PlaywrightInstaller 接 CLI 路径；EnvManager 内 per-call 解析所以不
	// 当全局 RuntimeInstaller 注册——PlaywrightEnvManager 通过其 Node 委托
	// 拥有 install 逻辑。

	// Env managers covering all 11 v1 supported runtimes.
	// 覆盖所有 11 个 v1 支持 runtime 的 env manager。
	svc.RegisterEnvManager(sandboxinfra.NewPythonEnvManager(svc))
	svc.RegisterEnvManager(sandboxinfra.NewNodeEnvManager(svc))
	svc.RegisterEnvManager(sandboxinfra.NewRustEnvManager())
	svc.RegisterEnvManager(sandboxinfra.NewGoEnvManager())
	svc.RegisterEnvManager(sandboxinfra.NewJavaEnvManager(svc))
	svc.RegisterEnvManager(sandboxinfra.NewRubyEnvManager(svc))
	svc.RegisterEnvManager(sandboxinfra.NewPHPEnvManager(svc))
	svc.RegisterEnvManager(sandboxinfra.NewDotnetEnvManager())
	svc.RegisterEnvManager(sandboxinfra.NewPlaywrightEnvManager(
		sandboxinfra.NewNodeEnvManager(svc), svc.SandboxRoot()))
	// Static binary EnvManager: one per static-binary plugin family
	// (registered alongside its matching StaticBinaryInstaller). v1
	// ships none — this is scaffolding for future plugins like
	// GitHub MCP that ship pre-built binaries.
	//
	// Static binary EnvManager：每个 static-binary plugin family 一个
	// （跟匹配的 StaticBinaryInstaller 一起注册）。v1 不发——给未来发
	// 预构建二进制的 plugin（如 GitHub MCP）做的脚手架。

	// GenericEnvManager: fallback for long-tail mise-installable languages
	// that don't have a dedicated EnvManager. v1 doesn't pre-register
	// any specific kind — main.go could add `NewGenericEnvManager("elixir")`
	// etc. as needed.
	//
	// GenericEnvManager：mise 可装的长尾语言无专用 EnvManager 的兜底。v1
	// 不预注册具体 kind——main.go 按需加 NewGenericEnvManager("elixir") 等。
}

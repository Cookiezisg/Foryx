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
	chatapp "github.com/sunweilin/forgify/backend/internal/app/chat"
	convapp "github.com/sunweilin/forgify/backend/internal/app/conversation"
	forgeapp "github.com/sunweilin/forgify/backend/internal/app/forge"
	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	forgetool "github.com/sunweilin/forgify/backend/internal/app/tool/forge"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
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
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
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

	// Sandbox: bundled-uv + bundled-Python + per-EnvID venv runtime. Bootstrap
	// from $FORGIFY_DEV_RESOURCES (dev) — prod cmd/desktop will instead
	// extract embed.FS into a temp dir and pass that path here. Failure to
	// bootstrap is logged but not fatal: backend stays up; forge operations
	// return ErrSandboxUnavailable until resources arrive.
	//
	// 沙箱：捆绑 uv + 捆绑 Python + 每 EnvID 一个 venv 的运行时。从
	// $FORGIFY_DEV_RESOURCES（dev）bootstrap——prod cmd/desktop 把 embed.FS
	// 解到临时目录后把路径传进来。bootstrap 失败仅 log 不致命：backend 仍
	// 起来；forge 操作返 ErrSandboxUnavailable 直到资源就位。
	sandbox := sandboxinfra.New(sandboxinfra.Config{
		DataDir:       *dataDir,
		DefaultPython: forgedomain.DefaultPythonVersion,
		Logger:        log,
	})
	if resourceDir := os.Getenv("FORGIFY_DEV_RESOURCES"); resourceDir != "" {
		if err := sandbox.Bootstrap(context.Background(), resourceDir); err != nil {
			log.Warn("sandbox.Bootstrap failed (forge ops will be unavailable)",
				zap.String("resource_dir", resourceDir),
				zap.Error(err))
		}
	} else {
		log.Warn("FORGIFY_DEV_RESOURCES not set; forge sandbox will be unavailable. Run `make download-resources` to enable forge ops.")
	}

	forgeService := forgeapp.NewService(
		forgestore.New(gdb),
		sandbox,
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

	forgeTools := forgetool.ForgeTools(
		forgeService,
		chatRepo,
		modelService,
		apikeyService,
		llmFactory,
	)
	chatService.SetTools(forgeTools)

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
		Dev:                 *dev,
		Tools:               forgeTools,
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

// Package bootstrap is the composition root: the one place allowed to import across every app
// and infra package. Build wires the SQLite DB, all stores, infra singletons, the 21 app
// Services, every cross-Service adapter (see resolvers/dispatch/refresolver/renderers/sensor),
// the tool set, the HTTP router, and the boot/shutdown lifecycle into a single *App. Nothing
// imports bootstrap, so there is no dependency cycle. cmd/server/main.go is a thin shell over it.
//
// Package bootstrap 是 composition root：唯一允许横跨所有 app/infra 包 import 的地方。Build 把 SQLite
// DB、所有 store、infra 单例、21 个 app Service、每个跨 Service 适配器、工具集、HTTP router、boot/
// shutdown 生命周期焊成一个 *App。无人 import bootstrap，故无依赖环。main.go 是它的薄壳。
package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	loggerinfra "github.com/sunweilin/forgify/backend/internal/infra/logger"
	handlershttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/handlers"
	routerhttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/router"
)

// Config parameterizes Build. DataDir empty → in-memory DB (tests). Addr defaults to :8080.
// Fingerprint is the machine-stable seed for the at-rest encryption key (api-key & mcp secrets).
//
// Config 参数化 Build。DataDir 空 → 内存 DB（测试）。Addr 默认 :8080。Fingerprint 是落盘加密密钥
// （api-key & mcp 密文）的机器稳定种子。
type Config struct {
	DataDir     string
	Addr        string
	Fingerprint string
	Dev         bool
}

// App is the assembled application: the HTTP handler plus the boot/shutdown lifecycle for the
// background-owning Services (sandbox runtime, handler/mcp processes, trigger listeners, the
// scheduler firing-drain ticker).
//
// App 是装配好的应用：HTTP handler + 持后台工作的 Service 的 boot/shutdown 生命周期。
type App struct {
	Handler  http.Handler
	Addr     string
	log      *zap.Logger
	svc      *services
	tickStop context.CancelFunc
}

const drainInterval = 5 * time.Second

// Build assembles the whole backend. The returned App is ready to serve immediately (health works
// before Boot); call Boot to start background work and Shutdown to stop it.
//
// Build 装配整个后端。返回的 App 立即可服务（Boot 前 health 即通）；调 Boot 启后台、Shutdown 停。
func Build(cfg Config) (*App, error) {
	log, err := loggerinfra.New(cfg.Dev)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: logger: %w", err)
	}
	database, err := openDB(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	enc, err := newEncryptor(cfg.Fingerprint, cfg.DataDir)
	if err != nil {
		return nil, err
	}

	st := buildStores(database, enc, cfg.DataDir)
	inf := infra{factory: llminfra.NewFactory(), encryptor: enc}
	bus := newBuses()

	// One mux: trigger registers webhook routes on it; the 24 resource handlers register theirs;
	// then Chain wraps it with the middleware stack (workspace identify/require, locale, cors…).
	mux := http.NewServeMux()
	svc := buildServices(st, inf, bus, mux, cfg.DataDir, log)
	registerHandlers(mux, svc, bus, log)

	addr := cfg.Addr
	if addr == "" {
		addr = ":8080"
	}
	return &App{
		Handler: routerhttpapi.Chain(mux, log, svc.workspace),
		Addr:    addr,
		log:     log,
		svc:     svc,
	}, nil
}

// registerHandlers constructs each resource handler over its Service and registers its routes on
// the shared mux, plus the static health probe (exempt from RequireWorkspace).
//
// registerHandlers 用各自 Service 构造每个资源 handler 并把路由注册到共享 mux，外加静态 health 探针。
func registerHandlers(mux *http.ServeMux, s *services, bus buses, log *zap.Logger) {
	mux.HandleFunc("GET /api/v1/health", handleHealth)

	regs := []interface {
		Register(handlershttpapi.Registrar)
	}{
		handlershttpapi.NewWorkspacesHandler(s.workspace, log),
		handlershttpapi.NewAPIKeyHandler(s.apikey, log),
		handlershttpapi.NewModelCapabilitiesHandler(s.modelCaps, log),
		handlershttpapi.NewScenariosHandler(),
		handlershttpapi.NewRelationHandler(s.relation, log),
		handlershttpapi.NewCatalogHandler(s.catalog, log),
		handlershttpapi.NewNotificationHandler(s.notification, log),
		handlershttpapi.NewStreamHandler(bus.messages, bus.entities, bus.notifications, log),
		handlershttpapi.NewMemoryHandler(s.memory, log),
		handlershttpapi.NewSandboxHandler(s.sandbox, log),
		handlershttpapi.NewDocumentHandler(s.document, s.aispawn, log),
		handlershttpapi.NewTodoHandler(s.todo, log),
		handlershttpapi.NewAttachmentHandler(s.attachment, log),
		handlershttpapi.NewFunctionHandler(s.function, s.aispawn, log),
		handlershttpapi.NewHandlerHandler(s.handler, s.aispawn, log),
		handlershttpapi.NewAgentHandler(s.agent, s.aispawn, log),
		handlershttpapi.NewTriggerHandler(s.trigger, s.aispawn, log),
		handlershttpapi.NewMCPHandler(s.mcp, log),
		handlershttpapi.NewSkillHandler(s.skill, log),
		handlershttpapi.NewControlHandler(s.control, s.aispawn, log),
		handlershttpapi.NewApprovalHandler(s.approval, s.aispawn, log),
		handlershttpapi.NewWorkflowHandler(s.workflow, s.aispawn, log),
		handlershttpapi.NewFlowrunHandler(s.scheduler, log),
		handlershttpapi.NewConversationHandler(s.conversation, log),
		handlershttpapi.NewChatHandler(s.chat, log),
		handlershttpapi.NewTriageHandler(s.aispawn, log),
	}
	for _, h := range regs {
		h.Register(mux)
	}
}

// handleHealth reports liveness as the N1 success envelope.
//
// handleHealth 以 N1 成功 envelope 返回存活状态。
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"data":{"status":"ok"}}`))
}

// Boot starts background work: sandbox runtime bootstrap + env-manager registration, resident
// handler & mcp processes, trigger listeners, scheduler crash-recovery, and the firing-drain
// ticker. Each step is best-effort logged — a single subsystem failing to boot degrades that
// feature, never the whole server.
//
// Boot 启后台工作：sandbox runtime bootstrap + env manager 注册、常驻 handler & mcp 进程、trigger
// listener、scheduler 崩溃恢复、firing-drain ticker。每步 best-effort 记日志——单子系统 boot 失败只
// 降级该功能，绝不拖垮整个 server。
func (a *App) Boot(ctx context.Context) {
	if err := a.svc.sandbox.Bootstrap(ctx); err != nil {
		a.log.Warn("bootstrap: sandbox bootstrap failed (runtimes degraded)", zap.Error(err))
	}
	registerSandboxStack(a.svc.sandbox)
	a.svc.sandbox.RestoreOrCleanupOnBoot(ctx)
	a.svc.handler.Boot(ctx)
	a.svc.mcp.Boot(ctx)
	a.svc.trigger.Start()
	if err := a.svc.scheduler.Recover(ctx); err != nil {
		a.log.Warn("bootstrap: scheduler recover failed", zap.Error(err))
	}
	// D1: the trigger listen-registry is in-memory, so re-engage the listener for every active
	// workflow (the "replay active references on boot" the trigger lifecycle expects). Same ctx
	// workspace as handler/mcp Boot above.
	//
	// D1：trigger 监听注册表是内存的，故为每个 active workflow 重挂监听（trigger 生命周期期待的「boot 重放
	// active 引用」）。与上面 handler/mcp Boot 同一 ctx workspace。
	if err := a.svc.workflow.ReattachActive(ctx); err != nil {
		a.log.Warn("bootstrap: workflow reattach-active failed", zap.Error(err))
	}

	// Firing-drain ticker: trigger listeners persist Firings to the durable inbox; the scheduler
	// claims + advances them here on a fixed cadence, and sweeps approval/timer timeouts.
	tickCtx, stop := context.WithCancel(context.Background())
	a.tickStop = stop
	go a.drainLoop(tickCtx)
}

// drainLoop periodically drains pending firings and checks timeouts until the app shuts down.
//
// drainLoop 周期 drain 待处理 firing + 检查超时，直到 app 关停。
func (a *App) drainLoop(ctx context.Context) {
	t := time.NewTicker(drainInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if err := a.svc.scheduler.DrainFirings(ctx); err != nil {
				a.log.Warn("bootstrap: drain firings", zap.Error(err))
			}
			if err := a.svc.scheduler.CheckTimeouts(ctx, now.UTC()); err != nil {
				a.log.Warn("bootstrap: check timeouts", zap.Error(err))
			}
		}
	}
}

// Shutdown stops background work in reverse dependency order. ctx bounds the graceful drain.
//
// Shutdown 逆依赖序停后台工作。ctx 限定优雅排空时间。
func (a *App) Shutdown(ctx context.Context) {
	if a.tickStop != nil {
		a.tickStop()
	}
	a.svc.trigger.Shutdown()
	a.svc.chat.Shutdown()
	a.svc.mcp.Shutdown(ctx)
	a.svc.handler.Shutdown(ctx)
	_ = a.log.Sync()
}

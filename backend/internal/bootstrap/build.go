// Package bootstrap is the composition root: the one place allowed to import across every app
// and infra package. Build wires the SQLite DB, all stores, infra singletons, the 28 app
// Services, every cross-Service adapter (see resolvers/dispatch/refresolver/renderers/sensor),
// the tool set, the HTTP router, and the boot/shutdown lifecycle into a single *App. Nothing
// imports bootstrap, so there is no dependency cycle. cmd/server/main.go is a thin shell over it.
//
// Package bootstrap 是 composition root：唯一允许横跨所有 app/infra 包 import 的地方。Build 把 SQLite
// DB、所有 store、infra 单例、28 个 app Service、每个跨 Service 适配器、工具集、HTTP router、boot/
// shutdown 生命周期焊成一个 *App。无人 import bootstrap，故无依赖环。main.go 是它的薄壳。
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"go.uber.org/zap"

	settingsapp "github.com/sunweilin/anselm/backend/internal/app/settings"
	llminfra "github.com/sunweilin/anselm/backend/internal/infra/llm"
	loggerinfra "github.com/sunweilin/anselm/backend/internal/infra/logger"
	ormpkg "github.com/sunweilin/anselm/backend/internal/pkg/orm"
	reqctxpkg "github.com/sunweilin/anselm/backend/internal/pkg/reqctx"
	handlershttpapi "github.com/sunweilin/anselm/backend/internal/transport/httpapi/handlers"
	routerhttpapi "github.com/sunweilin/anselm/backend/internal/transport/httpapi/router"
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
	Handler     http.Handler
	Addr        string
	log         *zap.Logger
	svc         *services
	db          *ormpkg.DB
	tickStop    context.CancelFunc
	drainDone   chan struct{}      // closed when drainLoop returns; Shutdown waits on it. drainLoop 退出时关闭，Shutdown 等它。
	timeoutStop context.CancelFunc // stops the independent timeout-sweep loop (F174: decoupled from drain so a slow node can't starve approval timeouts). 停独立超时扫描循环。
	timeoutDone chan struct{}      // closed when timeoutLoop returns. timeoutLoop 退出时关闭。
}

const drainInterval = 5 * time.Second

// drainShutdownGrace bounds how long Shutdown lets an in-flight workflow Advance finish its current
// node before interrupting it (R3 option C — clean durability for fast nodes, bounded shutdown for
// slow). Kept under shutdownGrace so the rest of the ordered shutdown keeps budget.
//
// drainShutdownGrace 限 Shutdown 给在飞 Advance 跑完当前节点的时长，超则打断（R3 选项 C——快节点干净、慢节点有界）。
// 留在 shutdownGrace 之下，使关停其余步骤仍有预算。
const drainShutdownGrace = 6 * time.Second

// Build assembles the whole backend. The returned App is ready to serve immediately (health works
// before Boot); call Boot to start background work and Shutdown to stop it.
//
// Build 装配整个后端。返回的 App 立即可服务（Boot 前 health 即通）；调 Boot 启后台、Shutdown 停。
func Build(cfg Config) (*App, error) {
	log, err := loggerinfra.New(cfg.Dev, filepath.Join(cfg.DataDir, "logs"))
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

	// settings.json (limits) loads before services so every consumer's first read sees
	// user-tuned values; a malformed file fails boot loudly.
	// settings.json（limits）先于服务加载，使所有消费方首读即见用户调校值；坏文件大声喊停。
	settingsSvc, err := settingsapp.Load(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: %w", err)
	}

	st := buildStores(database, enc, cfg.DataDir)
	inf := infra{factory: llminfra.NewFactory(), encryptor: enc}
	bus := newBuses()

	// One mux: trigger registers webhook routes on it; the 28 resource handlers register theirs;
	// then Chain wraps it with the middleware stack (workspace identify/require, locale, cors…).
	mux := http.NewServeMux()
	svc := buildServices(st, inf, bus, mux, cfg.DataDir, log)
	svc.settings = settingsSvc
	registerHandlers(mux, svc, bus, log)
	registerDebug(mux, cfg.Dev, log) // dev-only /debug/pprof + /debug/stats (observability)

	addr := cfg.Addr
	if addr == "" {
		addr = ":8080"
	}
	return &App{
		Handler: routerhttpapi.Chain(mux, log, svc.workspace),
		Addr:    addr,
		log:     log,
		svc:     svc,
		db:      database,
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
		handlershttpapi.NewSearchHandler(s.search, log),
		handlershttpapi.NewAPIKeyHandler(s.apikey, log),
		handlershttpapi.NewModelCapabilitiesHandler(s.modelCaps, log),
		handlershttpapi.NewScenariosHandler(),
		handlershttpapi.NewRelationHandler(s.relation, log),
		handlershttpapi.NewCatalogHandler(s.catalog, log),
		handlershttpapi.NewNotificationHandler(s.notification, log),
		handlershttpapi.NewStreamHandler(bus.messages, bus.entities, bus.notifications, log),
		handlershttpapi.NewMemoryHandler(s.memory, log),
		handlershttpapi.NewSandboxHandler(s.sandbox, log),
		handlershttpapi.NewLimitsHandler(s.settings, log),
		handlershttpapi.NewSystemHandler(s.settings, log),
		handlershttpapi.NewFreetierHandler(s.freetierQuota, log),
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

// shutdownGrace bounds the whole graceful drain (HTTP + background + DB).
//
// shutdownGrace 限定整个优雅排空（HTTP + 后台 + DB）。
const shutdownGrace = 10 * time.Second

// Serve owns the entire server lifecycle and blocks until ctx is cancelled (the entry shell wires
// SIGINT/SIGTERM to it) or the listener fails. The graceful-shutdown ORDER is a backend concern, not
// the shell's, and it must be exactly this — otherwise it is NOT graceful:
//
//  1. cancel the base request context FIRST — every request derives from it, so the frontend's three
//     resident SSE streams (never idle) end at once. Without this, http.Shutdown would block the full
//     grace window waiting for those connections to go idle (they never do).
//  2. http.Shutdown — now drains instantly (only short requests remain).
//  3. App.Shutdown — stop background work, then close the DB last.
//
// Returns the listener error, or nil on a clean signal-triggered stop.
//
// Serve 拥有整个服务生命周期，阻塞到 ctx 取消（入口壳把 SIGINT/SIGTERM 接到它）或 listener 失败。优雅关停的
// **顺序**是后端的事、不是壳的事，且必须正是这个顺序——否则就不优雅：① 先取消 base 请求 ctx——每个请求都从它派
// 生，故前端三条常驻 SSE 流（永不 idle）一起结束；否则 http.Shutdown 会干等满整个 grace 窗口等这些永不 idle 的
// 连接。② http.Shutdown——这下瞬间排空（只剩短请求）。③ App.Shutdown——停后台、最后关 DB。
func (a *App) Serve(ctx context.Context) error {
	a.Boot(context.Background())

	baseCtx, cancelBase := context.WithCancel(context.Background())
	srv := &http.Server{
		Addr:        a.Addr,
		Handler:     a.Handler,
		BaseContext: func(net.Listener) context.Context { return baseCtx },
	}

	serveErr := make(chan error, 1)
	go func() {
		a.log.Info("serving", zap.String("addr", a.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	var listenErr error
	select {
	case <-ctx.Done(): // SIGINT/SIGTERM
	case listenErr = <-serveErr:
	}

	sctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	a.log.Info("shutting down gracefully")
	cancelBase() // 1. end resident SSE streams so HTTP can drain
	if err := srv.Shutdown(sctx); err != nil {
		a.log.Warn("bootstrap: http shutdown", zap.Error(err))
	}
	a.Shutdown(sctx) // 2. stop background work + close DB
	return listenErr
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
	a.svc.trigger.Start()
	// search index worker + per-workspace reconcile (self-healing for dropped events /
	// crashes / schema bumps); never blocks boot.
	// 搜索索引 worker + 逐 workspace 对账（丢事件/崩溃/schema 升版的自愈）；绝不阻塞 boot。
	if workspaces, err := a.svc.workspace.List(ctx); err == nil {
		ids := make([]string, 0, len(workspaces))
		for _, w := range workspaces {
			ids = append(ids, w.ID)
		}
		a.svc.search.Start(ids)
	} else {
		a.log.Warn("bootstrap: list workspaces for search start", zap.Error(err))
		a.svc.search.Start(nil)
	}
	// Start the Advance worker pool BEFORE Recover so recovered runs resume ON the pool (off this boot
	// goroutine): a slow recovered node must not block boot, and pooled phase-2 Advance is the whole
	// point of F174. Recover enqueues; the workers drive concurrently with the rest of boot below.
	// 在 Recover **之前**启动 Advance worker 池，使恢复的 run 在池上恢复（脱离 boot goroutine）：慢的恢复
	// 节点不该卡 boot，池化的阶段 2 Advance 正是 F174 的目的。Recover 入队；worker 与下面的 boot 余下部分并发驱动。
	a.svc.scheduler.StartPool()
	if err := a.svc.scheduler.Recover(ctx); err != nil {
		a.log.Warn("bootstrap: scheduler recover failed", zap.Error(err))
	}
	// Background entry points run OFF any request, so ctx carries no workspace — but
	// handler/mcp Boot and ReattachActive read workspace-scoped tables (the orm ,ws filter
	// would reject a bare ctx with MISSING_WORKSPACE_ID). The ONE convention for background
	// work: seed a Detached workspace ctx per workspace and replay the entry point in each
	// (same family as Recover's per-run seeding and onReport's Detached(wsID)).
	//
	// 后台入口在任何请求之外跑，ctx 不带 workspace——而 handler/mcp Boot 与 ReattachActive 读
	// workspace 隔离表（orm 的 ,ws 过滤会以 MISSING_WORKSPACE_ID 拒裸 ctx）。后台工作的唯一惯例：
	// 逐 workspace 种 Detached ctx、在每个里重放入口（与 Recover 的 per-run 播种、onReport 的
	// Detached(wsID) 同族）。
	a.forEachWorkspace(ctx, func(wsCtx context.Context) {
		a.svc.handler.Boot(wsCtx)
		a.svc.mcp.Boot(wsCtx)
		// Backfill the built-in free-tier credential for every existing workspace (idempotent: a
		// no-op where it already exists; self-heals a workspace whose prior install failed). New
		// workspaces created after boot are covered by the workspace OnCreated hook. Best-effort —
		// EnsureForWorkspace always returns nil, a degraded free tier never blocks boot.
		// 为每个已存在 workspace 回填内置免费档凭证（幂等：已存在即 no-op；自愈上次 install 失败的）。
		// boot 后新建的由 workspace OnCreated 钩子覆盖。best-effort——EnsureForWorkspace 恒返 nil，
		// 降级的免费档绝不挂 boot。
		a.svc.freetier.EnsureForWorkspace(wsCtx)
		// Reconcile turns orphaned mid-stream by a hard crash (messages' scheduler.Recover
		// counterpart): pending/streaming rows become cancelled so the UI never shows a
		// forever-spinning bubble.
		// 对账被硬崩溃卡在流式中的孤儿回合（messages 版 scheduler.Recover）：pending/streaming 行
		// 置 cancelled，UI 不再出现永久转圈气泡。
		a.svc.chat.SweepOrphans(wsCtx)
		// D1: the trigger listen-registry is in-memory, so re-engage the listener for every
		// active workflow ("replay active references on boot").
		// D1：trigger 监听注册表是内存的，为每个 active workflow 重挂监听（boot 重放 active 引用）。
		if err := a.svc.workflow.ReattachActive(wsCtx); err != nil {
			a.log.Warn("bootstrap: workflow reattach-active failed", zap.Error(err))
		}
	})

	// Firing-drain ticker: trigger listeners persist Firings to the durable inbox; the scheduler claims
	// them here on a fixed cadence and enqueues each onto the Advance pool. The approval/timer timeout
	// sweep runs on its OWN ticker (F174) so a saturated pool can never starve approval-timeout settling
	// — they used to share the drain closure, where a slow Advance blocked CheckTimeouts.
	// firing-drain ticker：trigger 监听把 Firing 落到耐久收件箱；scheduler 在此按固定节律 claim 并把每条入队到
	// Advance 池。审批/计时超时扫描跑在**自己**的 ticker 上（F174），故满载的池绝不饿死审批超时结算——它们原来
	// 共用 drain 闭包、慢 Advance 阻塞 CheckTimeouts。
	tickCtx, stop := context.WithCancel(context.Background())
	a.tickStop = stop
	a.drainDone = make(chan struct{})
	go a.drainLoop(tickCtx)
	timeoutCtx, tstop := context.WithCancel(context.Background())
	a.timeoutStop = tstop
	a.timeoutDone = make(chan struct{})
	go a.timeoutLoop(timeoutCtx)
}

// forEachWorkspace runs fn once per workspace, each in a Detached ctx seeded with that
// workspace's id. The workspaces table is global (no ,ws column), so listing works on a bare
// ctx; everything inside fn is then properly isolated. Listing fresh per call keeps a
// workspace created after boot participating in the next tick.
//
// forEachWorkspace 对每个 workspace 跑一次 fn，各自在种了该 workspace id 的 Detached ctx 里。
// workspaces 表是全局表（无 ,ws 列），裸 ctx 可列；fn 内部随之正确隔离。每次调用现列，使 boot 后
// 新建的 workspace 在下一个 tick 即参与。
func (a *App) forEachWorkspace(ctx context.Context, fn func(wsCtx context.Context)) {
	workspaces, err := a.svc.workspace.List(ctx)
	if err != nil {
		a.log.Warn("bootstrap: list workspaces for background work", zap.Error(err))
		return
	}
	for _, ws := range workspaces {
		fn(reqctxpkg.Detached(ws.ID))
	}
}

// drainLoop periodically claims pending firings (enqueuing each onto the Advance pool) until the app
// shuts down — per workspace per tick (the firings table is workspace-scoped). Now FAST: it only
// claims + enqueues, never executes a node, so a slow run can't stall it (F174). Timeouts are swept by
// timeoutLoop, not here.
//
// drainLoop 周期 claim 待处理 firing（把每条入队到 Advance 池）直到 app 关停——每 tick 逐 workspace
// （firings 表按 workspace 隔离）。现在**很快**：只 claim + 入队、绝不执行节点，故慢 run 卡不住它（F174）。
// 超时由 timeoutLoop 扫描、不在此处。
func (a *App) drainLoop(ctx context.Context) {
	defer close(a.drainDone)
	t := time.NewTicker(drainInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.forEachWorkspace(ctx, func(wsCtx context.Context) {
				if err := a.svc.scheduler.DrainFirings(wsCtx); err != nil {
					a.log.Warn("bootstrap: drain firings", zap.Error(err))
				}
			})
		}
	}
}

// timeoutLoop sweeps approval/timer timeouts on its own ticker, decoupled from drainLoop (F174) so a
// saturated Advance pool can never delay approval-timeout settling — it only resolves parked nodes
// (pure DB) and ENQUEUES any re-drive, never executing a node inline. Per workspace per tick (parked-
// nodes table is workspace-scoped; CheckTimeouts' contract is "the caller ticks it per workspace").
//
// timeoutLoop 在自己的 ticker 上扫描审批/计时超时，与 drainLoop 解耦（F174），故满载的 Advance 池绝不延迟
// 审批超时结算——它只结算 parked 节点（纯 DB）并**入队**重驱动、绝不内联执行节点。每 tick 逐 workspace
// （parked-nodes 表按 workspace 隔离；CheckTimeouts 契约就是「调用方逐 workspace tick」）。
func (a *App) timeoutLoop(ctx context.Context) {
	defer close(a.timeoutDone)
	t := time.NewTicker(drainInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			a.forEachWorkspace(ctx, func(wsCtx context.Context) {
				if err := a.svc.scheduler.CheckTimeouts(wsCtx, now.UTC()); err != nil {
					a.log.Warn("bootstrap: check timeouts", zap.Error(err))
				}
			})
		}
	}
}

// Shutdown stops everything in reverse dependency order, then closes the DB last. ctx bounds the
// graceful drain. Order: stop the firing-drain ticker (no new runs) → bounded-grace the in-flight
// workflow Advance to finish its current node, then cancel every in-flight Advance + wait for the
// drain loop to return (R3, option C — so nothing keeps spawning or races db.Close) → trigger
// listeners → chat queues → mcp / handler resident processes → sandbox (kills any remaining spawned
// long-lived handles its consumers didn't) → shell background jobs (run_in_background children + their
// trees, R1) → flush logs → close the DB (checkpoints the SQLite WAL). Each step is best-effort logged
// so one stuck subsystem cannot block the rest.
//
// Shutdown 逆依赖序停一切、最后关 DB。ctx 限优雅排空。顺序：停 firing-drain ticker（不再起新 run）→
// 给在飞 Advance 有限宽限跑完当前节点、再取消所有在飞 Advance + 等 drain 循环返回（R3 选项 C——免其继续
// spawn 或撞 db.Close）→ trigger listener → chat 队列 → mcp / handler 常驻进程 → sandbox（杀消费者没杀干净的
// spawned long-lived handle）→ shell 后台任务（run_in_background 子进程 + 整树，R1）→ flush 日志 → 关 DB
// （checkpoint SQLite WAL）。每步 best-effort 记日志，一个卡死子系统不拖垮其余。
func (a *App) Shutdown(ctx context.Context) {
	if a.tickStop != nil {
		a.tickStop() // no new firing drains → no new enqueues from the drain ticker
	}
	if a.timeoutStop != nil {
		a.timeoutStop() // no new timeout sweeps → no new enqueues from the timeout ticker
	}
	// R3 (option C), F174 pool: the drain/timeout tickers stop FEEDING the pool; their loops return
	// fast (they only claim + enqueue now). Then give the in-flight POOL workers a bounded grace to
	// finish their CURRENT node — record-once makes a completed node durable, so the run resumes cleanly
	// next boot. If the grace expires, cancel every in-flight Advance (pooled AND manual :trigger) so it
	// can't keep spawning subprocesses or race db.Close, then WAIT for the pool workers to exit BEFORE
	// db.Close (StopPool's WaitGroup) — drainDone alone no longer means "all Advance done", it only means
	// the drain ticker stopped. The interrupted run records failed (:replay-able).
	// R3 选项 C + F174 池：drain/timeout ticker 停止**喂**池；其循环快速返回（现在只 claim+入队）。再给在飞的
	// **池 worker** 有限宽限跑完当前节点（record-once 持久化、下次 boot 干净续）。宽限超时则取消所有在飞 Advance
	// （池上 + 手动 :trigger），免其继续 spawn 子进程或撞 db.Close，再**等池 worker 退出**才 db.Close（StopPool 的
	// WaitGroup）——drainDone 单独已不再表示「所有 Advance 完」、只表示 drain ticker 停了。被打断的 run 记 failed、可 :replay。
	// Bound BOTH waits by the shutdown ctx: the loops return fast now (claim + enqueue only, F174), but a
	// wedged DB op inside forEachWorkspace must never turn SIGTERM into a SIGKILL (the F101 shutdown-hang
	// symptom). If a loop overruns the grace we proceed anyway — the pool's send is panic-safe (sendJob),
	// so a still-feeding loop racing StopPool can no longer crash the process; its late enqueue is dropped
	// and the run resumes next boot.
	// 两个等待都受 shutdown ctx 上界约束：循环现在很快返回（只 claim+enqueue，F174），但 forEachWorkspace 里
	// 卡死的 DB 操作绝不能把 SIGTERM 拖成 SIGKILL（F101 关停挂起症状）。循环超出宽限则照常往下走——池的发送已
	// panic-safe（sendJob），仍在喂的循环撞上 StopPool 不再崩进程，其迟到入队被丢、run 下次 boot 续。
	if a.drainDone != nil {
		select {
		case <-a.drainDone: // drain ticker loop returned (fast — claim + enqueue only)
		case <-ctx.Done():
			a.log.Warn("bootstrap: drain loop did not return within shutdown grace; proceeding")
		}
	}
	if a.timeoutDone != nil {
		select {
		case <-a.timeoutDone: // timeout ticker loop returned
		case <-ctx.Done():
			a.log.Warn("bootstrap: timeout loop did not return within shutdown grace; proceeding")
		}
	}
	a.svc.scheduler.WaitPoolDrained(ctx, drainShutdownGrace) // bounded grace for in-flight nodes to finish cleanly
	a.svc.scheduler.Shutdown()                               // cancel every still-in-flight Advance (pooled + manual :trigger)
	a.svc.scheduler.StopPool()                               // close the queue + wait for workers to exit before db.Close
	a.svc.trigger.Shutdown()
	a.svc.chat.Shutdown()
	a.svc.search.Close(ctx) // bounded by the shutdown ctx — a first-demand model download can't stall shutdown (R14)
	a.svc.mcp.Shutdown(ctx)
	a.svc.handler.Shutdown(ctx)
	if err := a.svc.sandbox.Shutdown(ctx); err != nil {
		a.log.Warn("bootstrap: sandbox shutdown", zap.Error(err))
	}
	a.svc.shellMgr.Stop() // reap run_in_background children + their whole process trees (R1)
	_ = a.log.Sync()
	if err := a.db.Close(); err != nil {
		a.log.Warn("bootstrap: db close", zap.Error(err))
	}
}

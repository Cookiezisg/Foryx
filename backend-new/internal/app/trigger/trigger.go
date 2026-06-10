// Package trigger (app) is the trigger entity surface: CRUD + the reference-counted listen
// registry (a trigger's listener runs only while ≥1 active workflow references it) + fan-out
// of fires into durable Firings + the per-action Activation log. It owns four source
// listeners (cron/webhook/fsnotify/sensor) behind one report callback. The claim of Firings
// into flowruns is the scheduler's job (波次 4).
//
// Package trigger（app）是 trigger 实体入口：CRUD + 引用计数监听表（listener 仅在 ≥1 个 active
// workflow 引用时运行）+ 把 fire 扇成 durable Firing + 逐动作 Activation 日志。它在一个 report
// 回调后持有 4 个 source listener。Firing→flowrun 的 claim 是 scheduler 的事（波次 4）。
package trigger

import (
	"net/http"
	"sync"

	"go.uber.org/zap"

	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
	triggerinfra "github.com/sunweilin/forgify/backend/internal/infra/trigger"
	croninfra "github.com/sunweilin/forgify/backend/internal/infra/trigger/cron"
	fsnotifyinfra "github.com/sunweilin/forgify/backend/internal/infra/trigger/fsnotify"
	sensorinfra "github.com/sunweilin/forgify/backend/internal/infra/trigger/sensor"
	webhookinfra "github.com/sunweilin/forgify/backend/internal/infra/trigger/webhook"
)

// listenEntry is the in-memory registration for one trigger whose listener is hot: which
// workspace it belongs to, its source kind, the set of workflows referencing it, and the subset
// of those that are ONE-SHOT (staged via AttachOnce) — auto-detached after their single fire.
//
// listenEntry 是某个 listener 正热的 trigger 的内存注册：所属 workspace、source 种类、引用它的 workflow 集、
// 以及其中**一次性**（经 AttachOnce 试运行）的子集——单次扇出后自动 Detach。
type listenEntry struct {
	workspaceID string
	kind        string
	workflows   map[string]bool
	once        map[string]bool // workflowID → drop after one fire (stage_workflow)
}

// Service is the unified trigger surface.
//
// Service 是统一的 trigger 入口。
type Service struct {
	repo triggerdomain.Repository

	cron     triggerinfra.Listener
	webhook  triggerinfra.Listener
	fsnotify triggerinfra.Listener
	sensor   triggerinfra.Listener

	mu        sync.RWMutex
	listeners map[string]*listenEntry // key: triggerID

	relations RelationSyncer
	entities  streamdomain.Bridge // entities stream (SSE-C); nil → no trigger-panel firing feed
	log       *zap.Logger
}

// SetEntitiesBridge installs the entities stream post-construction (SSE-C): every fan-out emits a
// fire signal scoped to the trigger, so the trigger panel shows firings live.
//
// SetEntitiesBridge 装配后装入 entities 流（SSE-C）：每次扇出发一条 trigger scope 的 fire 信号，使 trigger
// 面板实时显示触发。
func (s *Service) SetEntitiesBridge(b streamdomain.Bridge) { s.entities = b }

// NewService constructs the Service and wires the four listeners to s.onReport. mux is shared
// with the HTTP server (webhook routes mount on it); invoker resolves sensor targets
// (function/handler), injected at boot.
//
// NewService 构造 Service 并把 4 个 listener 接到 s.onReport。mux 与 HTTP server 共享（webhook 路由挂其上）；
// invoker 解析 sensor 目标（function/handler），boot 注入。
func NewService(repo triggerdomain.Repository, mux *http.ServeMux, invoker sensorinfra.SensorInvoker, log *zap.Logger) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	s := &Service{
		repo:      repo,
		listeners: make(map[string]*listenEntry),
		log:       log.Named("triggerapp"),
	}
	s.cron = croninfra.New(log, s.onReport)
	s.webhook = webhookinfra.New(mux, log, s.onReport)
	s.fsnotify = fsnotifyinfra.New(log, s.onReport)
	s.sensor = sensorinfra.New(invoker, log, s.onReport)
	return s
}

// SetRelationSyncer attaches the relation syncer post-construction (avoids a DI cycle).
//
// SetRelationSyncer 构造后注入 relation syncer（避开 DI 循环）。
func (s *Service) SetRelationSyncer(r RelationSyncer) { s.relations = r }

// listenerFor returns the listener for a source kind; nil for an unknown kind (callers guard).
//
// listenerFor 返回某 source 种类的 listener；未知 kind 返 nil（调用方守卫）。
func (s *Service) listenerFor(kind string) triggerinfra.Listener {
	switch kind {
	case triggerdomain.KindCron:
		return s.cron
	case triggerdomain.KindWebhook:
		return s.webhook
	case triggerdomain.KindFsnotify:
		return s.fsnotify
	case triggerdomain.KindSensor:
		return s.sensor
	}
	return nil
}

// Start boots all listeners (cron starts its scheduler; push listeners no-op). Call once at boot.
//
// Start 启动所有 listener（cron 启调度器；push 型 no-op）。boot 调一次。
func (s *Service) Start() {
	s.cron.Start()
	s.webhook.Start()
	s.fsnotify.Start()
	s.sensor.Start()
}

// Shutdown stops all listeners; call at process exit.
//
// Shutdown 停止所有 listener；进程退出调。
func (s *Service) Shutdown() {
	s.cron.Stop()
	s.webhook.Stop()
	s.fsnotify.Stop()
	s.sensor.Stop()
}

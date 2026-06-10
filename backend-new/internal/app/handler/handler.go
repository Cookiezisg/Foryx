// Package handler (app layer) orchestrates the handler domain: forging class versions
// from ops, encrypted init-args config, env materialization (via app/envfix), and the
// MCP-style resident-instance lifecycle (boot / restart / shutdown). The relation /
// catalog / mention adapters mirror function.
//
// Version model is method-A (linear, append-only, free active pointer — no accept), same
// as function. The lifecycle is the only real addition: one long-lived process per
// handler, spawned at boot / first call, restarted on edit / config-change / crash,
// gracefully shut down on app exit; all callers share it (true shared state).
//
// Package handler（app 层）编排 handler domain：ops 锻造类版本、加密 init-args config、env 物化
// （经 app/envfix）、MCP 式常驻实例生命周期（boot / restart / shutdown）。relation / catalog /
// mention 适配器镜像 function。版本模型同 function（方案 A）。
package handler

import (
	"context"
	"io"
	"time"

	"go.uber.org/zap"

	envfixapp "github.com/sunweilin/forgify/backend/internal/app/envfix"
	cryptodomain "github.com/sunweilin/forgify/backend/internal/domain/crypto"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	notificationdomain "github.com/sunweilin/forgify/backend/internal/domain/notification"
	relationdomain "github.com/sunweilin/forgify/backend/internal/domain/relation"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	handlerinfra "github.com/sunweilin/forgify/backend/internal/infra/handler"
)

// SandboxRunner is the long-lived spawn + cleanup surface (env materialization goes
// through envfix.Provisioner, not here). Wired over sandboxapp.Service at boot.
//
// SandboxRunner 是长跑 spawn + 清理面（env 物化走 envfix.Provisioner，不在此）。boot 时基于
// sandboxapp.Service 装配。
type SandboxRunner interface {
	// Ready reports whether the sandbox runtime is bootstrapped.
	Ready() bool

	// Spawn writes the version's user_handler.py (classCode) + driver.py, then starts the
	// long-lived `python driver.py` process in owner's env.
	//
	// Spawn 写版本的 user_handler.py（classCode）+ driver.py，再在 owner 的 env 里起长跑
	// `python driver.py` 进程。
	Spawn(ctx context.Context, owner sandboxdomain.Owner, handlerID, versionID, classCode string) (sandboxdomain.LongLivedHandle, error)

	// Destroy removes every env owned by the handler and its on-disk code dir.
	Destroy(ctx context.Context, handlerID string) error
}

// ClientFactory wraps subprocess pipes into a handlerinfra.Client (overridable in tests).
//
// ClientFactory 把子进程 pipe 包成 handlerinfra.Client（测试可替）。
type ClientFactory func(stdin io.WriteCloser, stdout io.Reader, log *zap.Logger) handlerinfra.Client

// DefaultClientFactory wraps handlerinfra.New.
func DefaultClientFactory(stdin io.WriteCloser, stdout io.Reader, log *zap.Logger) handlerinfra.Client {
	return handlerinfra.New(stdin, stdout, log)
}

// RelationSyncer is the slice of relationapp.Service handler consumes (nil-tolerant).
type RelationSyncer interface {
	SyncIncoming(ctx context.Context, toKind, toID string, kindScope []string, edges []relationdomain.SyncEdge) error
	PurgeEntity(ctx context.Context, kind, id string) error
}

// Service orchestrates the handler domain.
type Service struct {
	repo        handlerdomain.Repository
	provisioner *envfixapp.Provisioner
	runner      SandboxRunner
	clientFact  ClientFactory
	encryptor   cryptodomain.Encryptor
	manager     *instanceManager
	notif       notificationdomain.Emitter // nil-tolerant
	relations   RelationSyncer             // nil disables relation hooks
	entities    streamdomain.Bridge        // entities stream (SSE-C); nil → no entity-panel run terminal
	log         *zap.Logger
}

// SetEntitiesBridge installs the entities stream post-construction (SSE-C): Call tees a streaming
// method's yields onto the handler's run terminal for the entity panel, regardless of caller.
//
// SetEntitiesBridge 装配后装入 entities 流（SSE-C）：Call 把流式 method 的 yield tee 到 handler 的 run 终端
// 供实体面板，与谁触发无关。
func (s *Service) SetEntitiesBridge(b streamdomain.Bridge) { s.entities = b }

// NewService wires the service; nil repo / provisioner / runner / encryptor / log is a
// wiring bug (log degrades to no-op).
//
// NewService 装配 service；nil repo / provisioner / runner / encryptor / log 是装配 bug
// （log 退化为 no-op）。
func NewService(
	repo handlerdomain.Repository,
	provisioner *envfixapp.Provisioner,
	runner SandboxRunner,
	encryptor cryptodomain.Encryptor,
	clientFact ClientFactory,
	notif notificationdomain.Emitter,
	log *zap.Logger,
) *Service {
	if repo == nil {
		panic("handlerapp.NewService: repo is nil")
	}
	if provisioner == nil {
		panic("handlerapp.NewService: provisioner is nil")
	}
	if runner == nil {
		panic("handlerapp.NewService: runner is nil")
	}
	if encryptor == nil {
		panic("handlerapp.NewService: encryptor is nil")
	}
	if clientFact == nil {
		clientFact = DefaultClientFactory
	}
	if log == nil {
		log = zap.NewNop()
	}
	s := &Service{repo: repo, provisioner: provisioner, runner: runner, clientFact: clientFact, encryptor: encryptor, notif: notif, log: log}
	s.manager = newInstanceManager(s.spawnInstance, log)
	return s
}

// SetRelationSyncer installs the relation Service post-construction (avoids an init cycle).
func (s *Service) SetRelationSyncer(r RelationSyncer) { s.relations = r }

// Boot eagerly spawns a resident instance for every handler in the ctx's workspace whose
// active version is env-ready and config-complete (MCP-style "always-on from start").
// Best-effort: a handler that fails to spawn just stays stopped (next call retries).
//
// Boot 为 ctx workspace 内每个 active 版本 env-ready 且 config 完整的 handler 预先起常驻实例
// （MCP 式"开局就在线"）。best-effort：起不来的就停着（下次调用重试）。
func (s *Service) Boot(ctx context.Context) {
	handlers, err := s.repo.ListAllHandlers(ctx)
	if err != nil {
		s.log.Warn("handlerapp.Boot: list handlers failed", zap.Error(err))
		return
	}
	for _, h := range handlers {
		if h.ActiveVersionID == "" {
			continue
		}
		if _, gerr := s.manager.Get(ctx, h.ID); gerr != nil {
			s.log.Info("handlerapp.Boot: handler not started (likely needs config / env)",
				zap.String("handlerId", h.ID), zap.Error(gerr))
		}
	}
}

// Shutdown gracefully stops every resident instance (app exit).
//
// Shutdown 优雅停所有常驻实例（退出软件）。
func (s *Service) Shutdown(ctx context.Context) { s.manager.StopAll(ctx) }

// envOwner is the sandbox owner key for a version's env.
//
// envOwner 是某版本 env 的 sandbox owner key。
func envOwner(handlerID, envID string) sandboxdomain.Owner {
	return sandboxdomain.Owner{Kind: sandboxdomain.OwnerKindHandler, ID: handlerID + "_" + envID}
}

// ensureEnv materializes v's env via the envfix loop and writes terminal state + deps
// back. Returns whether the env ended ready.
//
// ensureEnv 经 envfix 循环物化 v 的 env，写回终态 + deps。返回 env 是否就绪。
func (s *Service) ensureEnv(ctx context.Context, v *handlerdomain.Version, sink envfixapp.Sink) (ready bool, errMsg string) {
	_ = s.repo.UpdateVersionEnv(ctx, v.ID, handlerdomain.EnvStatusSyncing, "", v.Dependencies, nil)

	res := s.provisioner.Provision(ctx, envfixapp.Request{
		Owner:   envOwner(v.HandlerID, v.EnvID),
		Runtime: sandboxdomain.RuntimeSpec{Kind: "python", Version: v.PythonVersion},
		Deps:    v.Dependencies,
		Sink:    sink,
	})

	now := time.Now().UTC()
	if res.OK {
		_ = s.repo.UpdateVersionEnv(ctx, v.ID, handlerdomain.EnvStatusReady, "", res.FinalDeps, &now)
		v.Dependencies = res.FinalDeps
		v.EnvStatus = handlerdomain.EnvStatusReady
		v.EnvError = ""
		v.EnvSyncedAt = &now
		return true, ""
	}
	errMsg = lastEnvError(res.History)
	_ = s.repo.UpdateVersionEnv(ctx, v.ID, handlerdomain.EnvStatusFailed, errMsg, res.FinalDeps, &now)
	v.Dependencies = res.FinalDeps
	v.EnvStatus = handlerdomain.EnvStatusFailed
	v.EnvError = errMsg
	v.EnvSyncedAt = &now
	return false, errMsg
}

func lastEnvError(history []envfixapp.Attempt) string {
	if len(history) == 0 {
		return "env install failed"
	}
	return history[len(history)-1].Error
}

// publish emits a handler lifecycle notification; nil emitter is a no-op.
func (s *Service) publish(ctx context.Context, action, handlerID string, extra map[string]any) {
	if s.notif == nil {
		return
	}
	payload := map[string]any{"handlerId": handlerID}
	for k, v := range extra {
		payload[k] = v
	}
	if err := s.notif.Emit(ctx, "handler."+action, payload); err != nil {
		s.log.Warn("handlerapp.publish: emit failed", zap.String("action", action), zap.Error(err))
	}
}

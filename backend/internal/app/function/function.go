// Package function (app layer) orchestrates the function domain: forging versions from
// ops, materializing each version's sandbox env (delegated to app/envfix, which adds
// the LLM dep-fix loop), running code, and the relation / catalog / mention adapters.
//
// The version model is a linear, append-only history with a free-moving ActiveVersionID
// pointer — no pending/accept state machine. Create/edit write a new version and take
// effect immediately; revert just moves the pointer.
//
// Package function（app 层）编排 function domain：从 ops 锻造版本、物化每个版本的 sandbox env
// （委托 app/envfix，它加 LLM 改依赖循环）、运行代码、relation / catalog / mention 适配器。
//
// 版本模型是线性、只增的历史 + 可自由移动的 ActiveVersionID 指针——无 pending/accept 状态机。
// create/edit 写新版本并立即生效；revert 只移指针。
package function

import (
	"context"
	"time"

	"go.uber.org/zap"

	entitystreamapp "github.com/sunweilin/forgify/backend/internal/app/entitystream"
	envfixapp "github.com/sunweilin/forgify/backend/internal/app/envfix"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	notificationdomain "github.com/sunweilin/forgify/backend/internal/domain/notification"
	relationdomain "github.com/sunweilin/forgify/backend/internal/domain/relation"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	searchdomain "github.com/sunweilin/forgify/backend/internal/domain/search"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
)

// SandboxRunner is the execution + cleanup surface function needs from the sandbox
// (env materialization is NOT here — that goes through envfix.Provisioner). Wired over
// sandboxapp.Service at boot.
//
// SandboxRunner 是 function 需要的执行 + 清理面（env 物化不在此——走 envfix.Provisioner）。
// boot 时基于 sandboxapp.Service 装配。
type SandboxRunner interface {
	// Ready reports whether the sandbox runtime is bootstrapped.
	//
	// Ready 报 sandbox runtime 是否已 bootstrap。
	Ready() bool

	// Run writes the version's main.py and spawns it in owner's env; a non-zero exit
	// becomes ExecutionResult{OK:false}, an infra failure a Go error.
	//
	// Run 写版本 main.py 并在 owner 的 env 里 spawn；非零退出返 ExecutionResult{OK:false}，
	// 基础设施失败返 Go error。
	Run(ctx context.Context, owner sandboxdomain.Owner, functionID, versionID, code string, input map[string]any) (*functiondomain.ExecutionResult, error)

	// Destroy removes every env owned by the function plus its on-disk code dir.
	//
	// Destroy 删除 function 拥有的所有 env 与盘上代码目录。
	Destroy(ctx context.Context, functionID string) error
}

// RelationSyncer is the slice of relationapp.Service function consumes (nil-tolerant).
//
// RelationSyncer 是 function 消费的 relationapp.Service 切片（允许 nil）。
type RelationSyncer interface {
	SyncIncoming(ctx context.Context, toKind, toID string, kindScope []string, edges []relationdomain.SyncEdge) error
	PurgeEntity(ctx context.Context, kind, id string) error
}

// Service orchestrates the function domain.
//
// Service 编排 function domain。
type Service struct {
	repo        functiondomain.Repository
	search      searchdomain.Notifier // nil → search indexing disabled. nil → 不接搜索索引。
	provisioner *envfixapp.Provisioner
	runner      SandboxRunner
	notif       notificationdomain.Emitter // nil-tolerant
	relations   RelationSyncer             // nil disables relation hooks
	entities    streamdomain.Bridge        // nil → no panel env terminal. nil → 无面板 env 终端。
	log         *zap.Logger
}

// SetEntitiesBridge installs the entities stream post-construction (SSE-C): every env
// materialization tees its attempt lines to the function's forge terminal, so the panel
// shows progress no matter which entry (HTTP editor / chat forge / run rebuild) paid for it.
//
// SetEntitiesBridge 装配后装入 entities 流（SSE-C）：每次 env 物化把尝试行 tee 到 function
// 的锻造终端——不论哪个入口（HTTP 编辑器/chat 锻造/run 重建）买单，面板都看得到进度。
func (s *Service) SetEntitiesBridge(b streamdomain.Bridge) { s.entities = b }

// NewService wires the service; nil repo / provisioner / runner / log is a wiring bug.
//
// NewService 装配 service；nil repo / provisioner / runner / log 是装配 bug。
func NewService(
	repo functiondomain.Repository,
	provisioner *envfixapp.Provisioner,
	runner SandboxRunner,
	notif notificationdomain.Emitter,
	log *zap.Logger,
) *Service {
	if repo == nil {
		panic("functionapp.NewService: repo is nil")
	}
	if provisioner == nil {
		panic("functionapp.NewService: provisioner is nil")
	}
	if runner == nil {
		panic("functionapp.NewService: runner is nil")
	}
	if log == nil {
		log = zap.NewNop()
	}
	return &Service{repo: repo, provisioner: provisioner, runner: runner, notif: notif, log: log}
}

// SetRelationSyncer installs the relation Service post-construction (avoids an init cycle).
//
// SetRelationSyncer 装配后注入 relation Service（避 init 环）。
func (s *Service) SetRelationSyncer(r RelationSyncer) { s.relations = r }

// envOwner is the sandbox owner key for a version's env: function kind + composite
// (functionID_envID) so every version's venv is distinct and addressable.
//
// envOwner 是某版本 env 的 sandbox owner key：function kind + 复合 (functionID_envID)，
// 使每个版本的 venv 各自独立且可寻址。
func envOwner(functionID, envID string) sandboxdomain.Owner {
	return sandboxdomain.Owner{Kind: sandboxdomain.OwnerKindFunction, ID: functionID + "_" + envID}
}

// ensureEnv materializes v's env via the envfix loop, writes the terminal env state +
// (fix-corrected) deps back to the row and mirrors them onto v, and returns whether the
// env ended ready. It never errors on a build failure — that is a state the caller
// surfaces (Create/Edit tolerate it; Run treats not-ready as ErrNoActiveVersion's kin).
//
// ensureEnv 经 envfix 循环物化 v 的 env，把终态 + （修复后）deps 写回行并镜像到 v，返回 env
// 是否就绪。它从不因构建失败报错——那是调用方上呈的状态（Create/Edit 容忍；Run 视未就绪为错）。
func (s *Service) ensureEnv(ctx context.Context, v *functiondomain.Version, sink envfixapp.Sink) (ready bool, errMsg string) {
	_ = s.repo.UpdateVersionEnv(ctx, v.ID, functiondomain.EnvStatusSyncing, "", v.Dependencies, nil)

	// Tee attempts to the panel's forge terminal regardless of caller — the HTTP editor
	// path used to build in silence while chat forge streamed (AC follow-up).
	// 把尝试行 tee 到面板锻造终端、不分调用方——此前 HTTP 编辑器路径静默构建而 chat 锻造有流。
	term := entitystreamapp.New(ctx, s.entities, streamdomain.Scope{Kind: streamdomain.KindFunction, ID: v.FunctionID}, entitystreamapp.NodeForge, nil)
	defer term.Close("completed", nil)
	sink = envfixapp.MultiSink(sink, envfixapp.NewWriterSink(term))

	res := s.provisioner.Provision(ctx, envfixapp.Request{
		Owner:   envOwner(v.FunctionID, v.EnvID),
		Runtime: sandboxdomain.RuntimeSpec{Kind: "python", Version: v.PythonVersion},
		Deps:    v.Dependencies,
		Sink:    sink,
	})

	now := time.Now().UTC()
	if res.OK {
		_ = s.repo.UpdateVersionEnv(ctx, v.ID, functiondomain.EnvStatusReady, "", res.FinalDeps, &now)
		v.Dependencies = res.FinalDeps
		v.EnvStatus = functiondomain.EnvStatusReady
		v.EnvError = ""
		v.EnvSyncedAt = &now
		return true, ""
	}

	errMsg = lastEnvError(res.History)
	_ = s.repo.UpdateVersionEnv(ctx, v.ID, functiondomain.EnvStatusFailed, errMsg, res.FinalDeps, &now)
	v.Dependencies = res.FinalDeps
	v.EnvStatus = functiondomain.EnvStatusFailed
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

// publish emits a function lifecycle notification; nil emitter is a no-op.
//
// publish 发一条 function 生命周期通知；nil emitter 为 no-op。
func (s *Service) publish(ctx context.Context, action, functionID string, extra map[string]any) {
	s.notifySearch(ctx, functionID)
	if s.notif == nil {
		return
	}
	payload := map[string]any{"functionId": functionID}
	for k, v := range extra {
		payload[k] = v
	}
	if err := s.notif.Emit(ctx, "function."+action, payload); err != nil {
		s.log.Warn("functionapp.publish: emit failed", zap.String("action", action), zap.Error(err))
	}
}

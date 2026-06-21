// Package envfix provisions a sandbox env for a set of (runtime, deps) and, when
// the install fails, asks a utility LLM to revise the dependency list and retries
// — a self-healing provisioning loop shared by every entity that owns a sandbox env
// (function / handler).
//
// It is deliberately stream-agnostic: progress is surfaced through a caller-supplied
// Sink (the tool layer pushes it onto an SSE stream; HTTP callers pass nil and the
// loop runs silently). The package never imports a stream/eventlog dependency.
//
// Package envfix 把一组 (runtime, deps) 物化成 sandbox env；装失败时让 utility LLM
// 改依赖列表并重试——一个自愈配置循环，被所有持有 sandbox env 的实体共用
// （function / handler）。
//
// 它刻意与流解耦：进度经调用方提供的 Sink 暴露（tool 层把它推到 SSE 流；HTTP 调用方
// 传 nil，循环静默跑）。本包绝不 import 任何 stream/eventlog 依赖。
package envfix

import (
	"context"

	"go.uber.org/zap"

	apikeydomain "github.com/sunweilin/anselm/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/anselm/backend/internal/domain/model"
	sandboxdomain "github.com/sunweilin/anselm/backend/internal/domain/sandbox"
	llminfra "github.com/sunweilin/anselm/backend/internal/infra/llm"
)

// DefaultMaxAttempts caps total install attempts (1 initial + LLM-suggested retries).
//
// DefaultMaxAttempts 是装环境总次数上限（1 次初始 + LLM 修复重试）。
const DefaultMaxAttempts = 3

// SandboxPort is the minimal sandbox surface envfix needs (DIP: defined here, so
// envfix does not import app/sandbox; sandboxapp.Service satisfies it structurally).
// EnsureEnv is idempotent on identical deps and rebuilds on changed deps — exactly
// the retry semantics the fix loop relies on.
//
// SandboxPort 是 envfix 需要的最小 sandbox 面（DIP：定义在此，故 envfix 不 import
// app/sandbox；sandboxapp.Service 结构化满足它）。EnsureEnv 对相同 deps 幂等、对变化的
// deps 重建——正是 fix loop 依赖的重试语义。
type SandboxPort interface {
	EnsureEnv(ctx context.Context, owner sandboxdomain.Owner, spec sandboxdomain.EnvSpec, stream sandboxdomain.ProgressFunc) (*sandboxdomain.Env, error)
}

// Sink receives per-attempt progress. The tool layer implements it to push onto an
// SSE stream; a nil Sink (HTTP / test) is treated as a no-op.
//
// Sink 收每次尝试的进度。tool 层实现它推到 SSE 流；nil（HTTP / 测试）视为 no-op。
type Sink interface {
	// OnAttempt fires once per install attempt with its terminal result.
	//
	// OnAttempt 每次装环境尝试结束触发一次，带终态结果。
	OnAttempt(a Attempt)

	// OnFixing fires before each LLM dep-repair, naming the attempt number it feeds.
	//
	// OnFixing 在每次 LLM 改依赖前触发，标出它要喂的尝试号。
	OnFixing(attempt int)
}

// Attempt records one install attempt; Error holds the captured stderr tail on failure.
//
// Attempt 记一次装环境；失败时 Error 存捕获的 stderr 尾部。
type Attempt struct {
	Number int      `json:"attempt"`
	Deps   []string `json:"deps"`
	OK     bool     `json:"ok"`
	Error  string   `json:"error,omitempty"`
}

// Request is one provision order. Sink may be nil. MaxAttempts <= 0 → DefaultMaxAttempts.
//
// Request 是一份物化指令。Sink 可空。MaxAttempts <= 0 → DefaultMaxAttempts。
type Request struct {
	Owner       sandboxdomain.Owner
	Runtime     sandboxdomain.RuntimeSpec
	Deps        []string
	MaxAttempts int
	Sink        Sink
}

// Result is the terminal outcome. FinalDeps is the dep list that succeeded (or the
// last tried on failure); callers persist it back onto the version so a later run
// reproduces the working env.
//
// Result 是终态。FinalDeps 是成功的依赖列表（失败则最后一次尝试的）；调用方把它回写到
// 版本，使后续运行复现可用 env。
type Result struct {
	OK           bool
	FinalDeps    []string
	AttemptsUsed int
	History      []Attempt
}

// Provisioner runs the install→fix→retry loop. All dependencies are interfaces /
// factories injected at construction.
//
// Provisioner 跑 装→修→重试 循环。所有依赖在构造时以接口 / 工厂注入。
type Provisioner struct {
	sandbox SandboxPort
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
	log     *zap.Logger
}

// NewProvisioner wires the provisioner; nil sandbox / picker / keys / factory is a
// wiring bug and panics. A nil logger degrades to a no-op logger.
//
// NewProvisioner 装配 provisioner；sandbox / picker / keys / factory 为 nil 是装配
// bug，直接 panic。nil logger 退化为 no-op logger。
func NewProvisioner(
	sandbox SandboxPort,
	picker modeldomain.ModelPicker,
	keys apikeydomain.KeyProvider,
	factory *llminfra.Factory,
	log *zap.Logger,
) *Provisioner {
	if sandbox == nil {
		panic("envfix.NewProvisioner: sandbox is nil")
	}
	if picker == nil {
		panic("envfix.NewProvisioner: picker is nil")
	}
	if keys == nil {
		panic("envfix.NewProvisioner: keys is nil")
	}
	if factory == nil {
		panic("envfix.NewProvisioner: factory is nil")
	}
	if log == nil {
		log = zap.NewNop()
	}
	return &Provisioner{sandbox: sandbox, picker: picker, keys: keys, factory: factory, log: log}
}

// Provision installs the env, and on failure asks the utility LLM to revise deps and
// retries up to MaxAttempts. It never returns a Go error: an infra failure or a
// missing utility model simply ends the loop with OK=false and the last stderr in
// History — the caller surfaces that to the authoring LLM, which can fix the code.
//
// Provision 装 env，失败则让 utility LLM 改依赖并重试至 MaxAttempts。它从不返回 Go error：
// 基础设施失败或未配 utility 模型只是以 OK=false 结束循环、最后一段 stderr 留在 History——
// 由调用方上呈给编写 LLM 自行改代码。
func (p *Provisioner) Provision(ctx context.Context, req Request) Result {
	max := req.MaxAttempts
	if max <= 0 {
		max = DefaultMaxAttempts
	}
	sink := resolveSink(req.Sink)
	deps := append([]string(nil), req.Deps...)
	history := make([]Attempt, 0, max)

	for attempt := 1; attempt <= max; attempt++ {
		err := p.install(ctx, req.Owner, req.Runtime, deps)
		a := Attempt{Number: attempt, Deps: deps, OK: err == nil}
		if err != nil {
			a.Error = err.Error()
		}
		history = append(history, a)
		sink.OnAttempt(a)

		if err == nil {
			return Result{OK: true, FinalDeps: deps, AttemptsUsed: attempt, History: history}
		}

		// Out of retries — stop. The caller surfaces the failure.
		// 重试名额用尽 — 停。失败由调用方上呈。
		if attempt == max {
			break
		}

		// Ask the utility LLM for a revised dep list. A resolve / call failure
		// (e.g. no utility model configured) ends the loop gracefully rather than
		// erroring — there is simply no auto-fix available.
		// 让 utility LLM 给修正依赖。解析 / 调用失败（如未配 utility 模型）优雅结束
		// 循环、不报错——只是没有自动修复可用。
		sink.OnFixing(attempt + 1)
		newDeps, fixErr := p.suggestDeps(ctx, deps, a.Error, history)
		if fixErr != nil {
			p.log.Warn("envfix: dep repair unavailable; stopping retries",
				zap.String("ownerId", req.Owner.ID), zap.Error(fixErr))
			break
		}
		// Reject a "fix" that DROPS a declared package — i.e. shrinks the list below the count the user
		// ORIGINALLY requested. Renaming a typo or loosening a version keeps the count; silently removing a
		// required package only makes the install error vanish while producing a green env that is MISSING
		// the package — a false-ready signal that defers the failure to a runtime ModuleNotFoundError and
		// throws away what the user declared. Keep the env FAILED with the real install error instead, so
		// the run-time guard (FUNCTION_ENV_NOT_READY) stays reachable and the declared deps are preserved (F148).
		// 拒绝「丢包」式修复——即修正后列表比用户**最初**声明的更短。改拼写/松版本会保持包数；静默删掉必需包只是让
		// 装错消失、却得到**缺包的绿 env**——假就绪信号，把失败推迟到运行时 ModuleNotFoundError 且丢掉用户所声明。
		// 改为保持 env **失败** + 真实装错，使运行时门控（FUNCTION_ENV_NOT_READY）仍可达、声明的 deps 不丢（F148）。
		if len(newDeps) < len(req.Deps) {
			p.log.Warn("envfix: rejecting a dep-dropping fix (would discard a declared package and false-ready the env)",
				zap.String("ownerId", req.Owner.ID), zap.Int("declared", len(req.Deps)), zap.Int("suggested", len(newDeps)))
			break
		}
		deps = newDeps
	}

	last := history[len(history)-1]
	return Result{OK: false, FinalDeps: last.Deps, AttemptsUsed: len(history), History: history}
}

// install materializes the env for one dep set. Progress flows through the env's own
// status SSE (sandbox publishes it); envfix passes a no-op ProgressFunc so it never
// depends on stream wiring here.
//
// install 物化一组 deps 的 env。细粒度进度走 env 自身的状态 SSE（sandbox 发布）；envfix
// 传 no-op ProgressFunc，使此处不依赖任何流接线。
func (p *Provisioner) install(ctx context.Context, owner sandboxdomain.Owner, runtime sandboxdomain.RuntimeSpec, deps []string) error {
	spec := sandboxdomain.EnvSpec{Runtime: runtime, Deps: deps}
	_, err := p.sandbox.EnsureEnv(ctx, owner, spec, func(string, string, int) {})
	return err
}

func resolveSink(s Sink) Sink {
	if s == nil {
		return noopSink{}
	}
	return s
}

type noopSink struct{}

func (noopSink) OnAttempt(Attempt) {}
func (noopSink) OnFixing(int)      {}

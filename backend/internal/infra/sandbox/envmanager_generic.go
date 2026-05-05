// envmanager_generic.go — fallback EnvManager for runtimes without a
// dedicated implementation (mise's long-tail 600+ languages: Erlang /
// Elixir / Lua / Crystal / Zig / Deno / etc.) and for owners that don't
// need package isolation (raw conversation scratch where the LLM just
// wants a clean cwd).
//
// What it does:  mkdir <envPath>, return.
// What it does NOT do: install any deps, set any env vars, build any
// venv-equivalent. The caller (or the LLM via Bash) runs the language's
// native package manager in the env's cwd; whatever convention that
// language follows (Cargo.toml, mix.exs, deno.json, ...) is the user's
// problem.
//
// Why this is acceptable for v1:
//
//   - Cross-env disk sharing for these langs is the language's own
//     package manager problem, not ours. Cargo / Hex / Mix / etc. each
//     have their own caches and we don't fight them.
//   - LLM in conversation Bash usually wants to run a one-off command
//     ("show me the AST of this Elixir file") not build a long-lived
//     env. mkdir + cwd is sufficient.
//   - When a long-tail language proves popular, swap GenericEnvManager
//     for a dedicated EnvManager (one new file, one main.go registration
//     line) without touching any other code.
//
// envmanager_generic.go ——通用兜底 EnvManager。给没有专用实现的 runtime
// 用（mise 长尾 600+ 语言：Erlang / Elixir / Lua / Crystal / Zig / Deno 等），
// 以及不需要包隔离的 owner（裸 conversation scratch，LLM 只想要个干净 cwd）。
//
// 干啥：mkdir <envPath>，返。
// 不干啥：装任何 deps、设任何 env var、建任何 venv 等价物。调用方（或 LLM
// via Bash）在 env cwd 跑该语言原生包管理器；该语言按啥惯例（Cargo.toml /
// mix.exs / deno.json / ...）是用户自己的事。
//
// 为什么 v1 这样可以：
//
//   - 这些语言的跨 env 磁盘共享是该语言包管理器自己的事，我们不掺和。
//     Cargo / Hex / Mix 等各有自己缓存，我们不跟它们打架。
//   - LLM 在 conversation Bash 通常想跑一次性命令（"给我这个 Elixir 文件的
//     AST"）不是建长生命周期 env。mkdir + cwd 足够。
//   - 某长尾语言变流行时，把 GenericEnvManager 换成专用 EnvManager（一个新
//     文件 + main.go 一行注册）就行，其他代码不动。

package sandbox

import (
	"context"
	"fmt"
	"os"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

// GenericEnvManager satisfies sandboxdomain.EnvManager as a no-op fallback
// for arbitrary runtime kinds. The constructor accepts a kind tag so a
// single struct can be registered against multiple long-tail kinds
// (NewGenericEnvManager("elixir"), NewGenericEnvManager("zig"), etc.).
//
// GenericEnvManager 满足 sandboxdomain.EnvManager 作任意 runtime kind 的
// no-op fallback。构造接 kind tag，让单个 struct 能注册到多个长尾 kind
// （NewGenericEnvManager("elixir") / NewGenericEnvManager("zig") 等）。
type GenericEnvManager struct {
	kind string
}

// NewGenericEnvManager constructs a fallback manager for the given kind.
//
// NewGenericEnvManager 构造给定 kind 的兜底 manager。
func NewGenericEnvManager(kind string) *GenericEnvManager {
	return &GenericEnvManager{kind: kind}
}

// Kind reports the dispatch key supplied at construction.
//
// Kind 报告构造时提供的派发键。
func (g *GenericEnvManager) Kind() string { return g.kind }

// CreateEnv mkdirs the env directory and returns. No package.json /
// pyproject.toml / Cargo.toml is written — the language's own tooling
// initializes its own scaffolding when the LLM (or user) runs the first
// command from this cwd.
//
// CreateEnv mkdir env 目录后返。不写 package.json / pyproject.toml /
// Cargo.toml——LLM（或用户）从该 cwd 跑第一个命令时该语言自己 tooling 会
// 初始化自己 scaffolding。
func (g *GenericEnvManager) CreateEnv(ctx context.Context, runtimePath, envPath string) error {
	if err := os.MkdirAll(envPath, 0o755); err != nil {
		return fmt.Errorf("sandbox.GenericEnvManager.CreateEnv %s: mkdir: %w (env: %w)", g.kind, err, sandboxdomain.ErrEnvCreateFailed)
	}
	return nil
}

// InstallDeps is a no-op. Generic EnvManager doesn't know how to install
// packages for arbitrary languages — caller (or LLM via Bash) runs the
// language's package manager themselves. Returns nil (not an error)
// because empty deps is valid; caller must not pass non-empty deps to
// the generic manager (it would silently drop them).
//
// InstallDeps no-op。Generic EnvManager 不知任意语言怎么装包——调用方
// （或 LLM via Bash）自己跑该语言包管理器。返 nil（不是错），因空 deps
// 是合法；调用方不该向 generic manager 传非空 deps（会被静默丢）。
func (g *GenericEnvManager) InstallDeps(ctx context.Context, runtimePath, envPath string, deps []string, stream sandboxdomain.ProgressFunc) error {
	return nil
}

// InstallExtras is a no-op. Same rationale as InstallDeps.
//
// InstallExtras no-op。同 InstallDeps 理由。
func (g *GenericEnvManager) InstallExtras(ctx context.Context, runtimePath, envPath string, extras []string, stream sandboxdomain.ProgressFunc) error {
	return nil
}

// EnvBin returns runtimePath/<binName> — for generic kinds, "binary"
// just means the runtime's main interpreter/compiler executable, not a
// venv-shimmed copy. Caller is expected to spawn from envPath as cwd
// with runtimePath in PATH.
//
// EnvBin 返 runtimePath/<binName>——generic kind 的 "binary" 仅指 runtime
// 主解释器/编译器，不是 venv-shim 的 copy。调用方应以 envPath 当 cwd
// 起子进程，runtimePath 在 PATH 里。
func (g *GenericEnvManager) EnvBin(envPath, binName string) string {
	return binName
}

// EnvDir returns envPath unchanged.
//
// EnvDir 原样返 envPath。
func (g *GenericEnvManager) EnvDir(envPath string) string { return envPath }

// Open/closed extension ports — adding a new runtime kind = one
// RuntimeInstaller + EnvManager pair + one main.go registration. Sandbox
// core unchanged.

package sandbox

import "context"

// RuntimeInstaller is the install + locate contract for one runtime kind.
// One per kind; registered at boot (main.go).
//
// RuntimeInstaller 是单个 runtime kind 的装机 + 定位契约。
// 每 kind 一个；启动时（main.go）注册。
type RuntimeInstaller interface {
	// Kind must match RuntimeSpec.Kind and the kind column in sandbox_runtimes.
	// Kind 必须与 RuntimeSpec.Kind 和 sandbox_runtimes.kind 一致。
	Kind() string

	// Install installs version under sandboxRoot and returns the install dir's
	// path RELATIVE to sandboxRoot (stored in Runtime.Path). Installer chooses
	// the layout (e.g. mise: "mise-data/installs/<kind>/<version>"). stream
	// gets progress updates; pass nil to skip. Returns ErrRuntimeInstallFailed
	// (wrapping stderr) on failure.
	//
	// Install 把 version 装到 sandboxRoot 下，返相对 sandboxRoot 的安装目录路径
	// （存进 Runtime.Path）。Installer 自选 layout（如 mise:
	// "mise-data/installs/<kind>/<version>"）。stream 接进度，传 nil 跳过。
	// 失败返 ErrRuntimeInstallFailed（含 stderr）。
	Install(ctx context.Context, version, sandboxRoot string, stream ProgressFunc) (relPath string, err error)

	// Locate returns the absolute path to the runtime's primary executable
	// for an installed (version, sandboxRoot) pair.
	//
	// Locate 返回已装 (version, sandboxRoot) 主可执行绝对路径。
	Locate(version, sandboxRoot string) (binPath string, err error)

	// ResolveDefault returns the kind's default version (used when
	// EnvSpec.Runtime.Version is empty).
	//
	// ResolveDefault 返该 kind 默认版本（EnvSpec.Runtime.Version 为空时用）。
	ResolveDefault(ctx context.Context) (string, error)

	// NormalizeVersion canonicalizes a version spec into the concrete form
	// stored in sandbox_runtimes.version. Used to dedupe rows that would
	// otherwise differ only in spec syntax (e.g. mise: `>=3.12` and `3.12`
	// both install python 3.12.x — same install, should share one row).
	// Default impl can return version unchanged when no normalization is
	// needed.
	//
	// NormalizeVersion 把 version 规范化为存进 sandbox_runtimes.version 的
	// 具体形态。用于去重——`>=3.12` 与 `3.12` 在 mise 都装同一份 python,
	// 不该建两行。无需归一化时直接返原值即可。
	NormalizeVersion(version string) string
}

// EnvManager is the per-owner env build contract for one runtime kind.
// Paired with the matching RuntimeInstaller.
//
// EnvManager 是单个 runtime kind 的 per-owner env 构建契约。
// 与对应 RuntimeInstaller 配对。
type EnvManager interface {
	// Kind matches RuntimeInstaller.Kind() — same key dispatches both.
	// Kind 与 RuntimeInstaller.Kind() 一致——同 key 派发两者。
	Kind() string

	// CreateEnv materialises an empty isolation env at envPath against the
	// runtime at runtimePath. Idempotent — existing env returns nil.
	//
	// CreateEnv 在 envPath 物化空隔离 env，针对 runtimePath 的 runtime。
	// 幂等——已存在返 nil。
	CreateEnv(ctx context.Context, runtimePath, envPath string) error

	// InstallDeps installs deps via the runtime's native package manager
	// (uv / pnpm / cargo / ...). ErrDepInstallFailed (wrapping stderr) on failure.
	//
	// InstallDeps 通过 runtime 原生包管理器装 deps（uv / pnpm / cargo / ...）。
	// 失败返 ErrDepInstallFailed（含 stderr）。
	InstallDeps(ctx context.Context, runtimePath, envPath string, deps []string, stream ProgressFunc) error

	// EnvBin returns the absolute path of binName inside envPath
	// (e.g. "<envPath>/.venv/bin/python").
	//
	// EnvBin 返 envPath 内 binName 的绝对路径（如 "<envPath>/.venv/bin/python"）。
	EnvBin(envPath, binName string) string

	// EnvDir returns the env's primary directory (typically Spawn cwd).
	// EnvDir 返 env 主目录（通常作 Spawn cwd）。
	EnvDir(envPath string) string
}

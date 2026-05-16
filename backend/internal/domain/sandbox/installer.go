package sandbox

import "context"

// RuntimeInstaller is the install + locate contract for one runtime kind.
//
// RuntimeInstaller 是单个 runtime kind 的装机 + 定位契约。
type RuntimeInstaller interface {
	Kind() string

	// Install installs version under sandboxRoot; returns relPath stored in Runtime.Path.
	//
	// Install 把 version 装到 sandboxRoot；返相对 sandboxRoot 的 relPath（存进 Runtime.Path）。
	Install(ctx context.Context, version, sandboxRoot string, stream ProgressFunc) (relPath string, err error)

	Locate(version, sandboxRoot string) (binPath string, err error)
	ResolveDefault(ctx context.Context) (string, error)

	// NormalizeVersion canonicalizes a version spec to the concrete form stored in sandbox_runtimes.
	//
	// NormalizeVersion 把 version 规范化为 sandbox_runtimes.version 的具体形态用于去重。
	NormalizeVersion(version string) string
}

// EnvManager is the per-owner env build contract paired with a RuntimeInstaller.
//
// EnvManager 是 per-owner env 构建契约，与同 kind 的 RuntimeInstaller 配对。
type EnvManager interface {
	Kind() string

	// CreateEnv materialises an empty isolation env at envPath; idempotent.
	//
	// CreateEnv 在 envPath 物化空隔离 env；幂等，已存在返 nil。
	CreateEnv(ctx context.Context, runtimePath, envPath string) error

	// InstallDeps installs deps via runtime native pkg manager; ErrDepInstallFailed on failure.
	//
	// InstallDeps 经 runtime 原生包管理器装 deps；失败返 ErrDepInstallFailed（含 stderr）。
	InstallDeps(ctx context.Context, runtimePath, envPath string, deps []string, stream ProgressFunc) error

	EnvBin(envPath, binName string) string
	EnvDir(envPath string) string
}

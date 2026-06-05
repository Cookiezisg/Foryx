package sandbox

import "context"

// RuntimeInstaller is the install + locate contract for one runtime kind.
//
// RuntimeInstaller 是单个 runtime kind 的装机 + 定位契约。
type RuntimeInstaller interface {
	Kind() string

	// Install installs version under sandboxRoot and returns the relPath stored
	// in Runtime.Path. For docker this pulls the image and returns the image ref.
	//
	// Install 把 version 装到 sandboxRoot 并返回存进 Runtime.Path 的 relPath。docker
	// 则拉取镜像并返回镜像 ref。
	Install(ctx context.Context, version, sandboxRoot string, stream ProgressFunc) (relPath string, err error)

	// Locate returns the primary binary's absolute path. For docker it returns
	// the image ref (there is no host-side binary).
	//
	// Locate 返回主 binary 绝对路径。docker 返回镜像 ref（宿主侧无 binary）。
	Locate(version, sandboxRoot string) (binPath string, err error)

	ResolveDefault(ctx context.Context) (string, error)

	// NormalizeVersion canonicalizes a version spec to the form stored in
	// sandbox_runtimes.version, so equivalent specs share one runtime row.
	//
	// NormalizeVersion 把 version 规范化为 sandbox_runtimes.version 的形态，使等价 spec
	// 共用一个 runtime 行。
	NormalizeVersion(version string) string
}

// EnvManager is the per-owner env build + exec contract, paired with a
// RuntimeInstaller of the same kind.
//
// EnvManager 是与同 kind RuntimeInstaller 配对的 per-owner env 构建 + 执行契约。
type EnvManager interface {
	Kind() string

	// CreateEnv materializes an empty isolation env at envPath; idempotent.
	// Docker is a no-op — the image is the environment.
	//
	// CreateEnv 在 envPath 物化空隔离 env；幂等。docker 为 no-op——镜像即环境。
	CreateEnv(ctx context.Context, runtimePath, envPath string) error

	// InstallDeps installs deps via the runtime's native package manager and
	// returns ErrDepInstallFailed (wrapping stderr) on failure. Docker is a no-op.
	//
	// InstallDeps 经 runtime 原生包管理器装 deps，失败返 ErrDepInstallFailed（含
	// stderr）。docker 为 no-op。
	InstallDeps(ctx context.Context, runtimePath, envPath string, deps []string, stream ProgressFunc) error

	// ResolveExec assembles the host command that runs opts.Cmd inside this env.
	// python/node resolve a binary inside envPath (a bare cmd → <env>/bin/<cmd>,
	// an absolute/relative cmd passes through); docker wraps opts.Cmd in
	// `docker run --rm -i <image>` where image = runtimeRef. It returns the host
	// cmd, the full arg list, and the working directory.
	//
	// ResolveExec 组装在该 env 内运行 opts.Cmd 的宿主命令。python/node 解析 envPath 内
	// 的 binary（裸 cmd → <env>/bin/<cmd>，绝对/相对 cmd 原样透传）；docker 把 opts.Cmd
	// 包进 `docker run --rm -i <image>`（image = runtimeRef）。返回宿主 cmd、完整 arg
	// 列表与工作目录。
	ResolveExec(runtimeRef, envPath string, opts SpawnOpts) (cmd string, args []string, cwd string)
}

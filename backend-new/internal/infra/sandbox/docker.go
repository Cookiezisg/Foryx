package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

// dockerBin is the docker CLI on PATH. Forgify cannot install docker (it needs
// root/admin), so the docker runtime probes the host daemon and shells out to it.
//
// dockerBin 是 PATH 上的 docker CLI。Forgify 不能装 docker（需 root/admin），故 docker
// runtime 探测宿主 daemon 并外包给它。
const dockerBin = "docker"

// DockerInstaller is the RuntimeInstaller for docker-image MCP servers. Unlike
// mise it installs nothing on disk: "install" = confirm the daemon is reachable,
// then `docker pull <image>`. The returned ref (stored in Runtime.Path) is the
// image ref itself.
//
// DockerInstaller 是 docker 镜像型 MCP server 的 RuntimeInstaller。不同于 mise，它不在
// 磁盘装任何东西："install" = 确认 daemon 可达，然后 `docker pull <image>`。返回的 ref
// （存进 Runtime.Path）就是镜像 ref 本身。
type DockerInstaller struct{}

var _ sandboxdomain.RuntimeInstaller = (*DockerInstaller)(nil)

func NewDockerInstaller() *DockerInstaller { return &DockerInstaller{} }

func (d *DockerInstaller) Kind() string { return "docker" }

// Install pulls the image after confirming the daemon is up; returns the image
// ref verbatim. version is the image ref (e.g. ghcr.io/org/img:tag).
//
// Install 在确认 daemon 在线后拉取镜像；原样返回镜像 ref。version 即镜像 ref
// （如 ghcr.io/org/img:tag）。
func (d *DockerInstaller) Install(ctx context.Context, version, _ string, stream sandboxdomain.ProgressFunc) (string, error) {
	if err := probeDocker(ctx); err != nil {
		return "", err
	}
	if version == "" {
		return "", fmt.Errorf("sandbox.DockerInstaller.Install: %w: empty image ref", sandboxdomain.ErrRuntimeInstallFailed)
	}
	cmd := exec.CommandContext(ctx, dockerBin, "pull", version)
	if err := RunWithStderrCapture(cmd, stream,
		sandboxdomain.ErrRuntimeInstallFailed,
		fmt.Sprintf("sandbox.DockerInstaller.Install %s", version)); err != nil {
		return "", err
	}
	return version, nil
}

// Locate returns the image ref unchanged — there is no host-side binary.
//
// Locate 原样返回镜像 ref——宿主侧无 binary。
func (d *DockerInstaller) Locate(version, _ string) (string, error) { return version, nil }

// ResolveDefault has no meaning for docker — the image ref is always explicit.
//
// ResolveDefault 对 docker 无意义——镜像 ref 总是显式给出。
func (d *DockerInstaller) ResolveDefault(ctx context.Context) (string, error) { return "", nil }

// NormalizeVersion keeps the image ref verbatim (a tag/digest is already canonical).
//
// NormalizeVersion 原样保留镜像 ref（tag/digest 已是规范形）。
func (d *DockerInstaller) NormalizeVersion(version string) string { return version }

// probeDocker classifies host docker state: a missing binary →
// ErrDockerNotInstalled; a present binary whose daemon is unreachable →
// ErrDockerDaemonDown.
//
// probeDocker 分类宿主 docker 状态：缺 binary → ErrDockerNotInstalled；有 binary 但
// daemon 不可达 → ErrDockerDaemonDown。
func probeDocker(ctx context.Context) error {
	if _, err := exec.LookPath(dockerBin); err != nil {
		return fmt.Errorf("sandbox.probeDocker: %w", sandboxdomain.ErrDockerNotInstalled)
	}
	// `docker info` round-trips to the daemon; a non-zero exit means it's down.
	// `docker info` 往返 daemon；非零退出表示 daemon 不可用。
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, dockerBin, "info", "--format", "{{.ServerVersion}}")
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sandbox.probeDocker: %w (%s)", sandboxdomain.ErrDockerDaemonDown, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// DockerEnvManager is the EnvManager for docker-image envs. CreateEnv/InstallDeps
// are no-ops — a pulled image is the whole environment; ResolveExec wraps the
// command in `docker run --rm -i <image>`.
//
// DockerEnvManager 是 docker 镜像型 env 的 EnvManager。CreateEnv/InstallDeps 为 no-op
// ——已拉取的镜像即整个环境；ResolveExec 把命令包进 `docker run --rm -i <image>`。
type DockerEnvManager struct{}

var _ sandboxdomain.EnvManager = (*DockerEnvManager)(nil)

func NewDockerEnvManager() *DockerEnvManager { return &DockerEnvManager{} }

func (d *DockerEnvManager) Kind() string { return "docker" }

// CreateEnv is a no-op — a pulled image needs no per-owner materialization.
//
// CreateEnv 为 no-op——已拉取的镜像无需 per-owner 物化。
func (d *DockerEnvManager) CreateEnv(context.Context, string, string) error { return nil }

// InstallDeps is a no-op — a docker image bundles its own dependencies.
//
// InstallDeps 为 no-op——docker 镜像自带依赖。
func (d *DockerEnvManager) InstallDeps(context.Context, string, string, []string, sandboxdomain.ProgressFunc) error {
	return nil
}

// ResolveExec wraps opts.Cmd in `docker run --rm -i [-e K=V ...] <image> [cmd]
// [args]`, where image = runtimeRef. opts.Env is forwarded as -e flags (a host
// process env would not reach inside the container), sorted for determinism.
// envPath is unused (the container is the env); cwd is empty (the workdir lives
// inside the container). Container lifecycle refinement (graceful stop, orphan
// reclaim) is deferred to the mcp module.
//
// ResolveExec 把 opts.Cmd 包进 `docker run --rm -i [-e K=V ...] <image> [cmd] [args]`
// （image = runtimeRef）。opts.Env 以 -e 注入（宿主进程 env 进不了容器），排序保证确定
// 性。envPath 不用（容器即 env）；cwd 空（工作目录在容器内）。容器生命周期精细化（优雅
// 停止、孤儿回收）留给 mcp 模块。
func (d *DockerEnvManager) ResolveExec(runtimeRef, _ string, opts sandboxdomain.SpawnOpts) (cmd string, args []string, cwd string) {
	runArgs := []string{"run", "--rm", "-i"}
	if len(opts.Env) > 0 {
		keys := make([]string, 0, len(opts.Env))
		for k := range opts.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			runArgs = append(runArgs, "-e", k+"="+opts.Env[k])
		}
	}
	runArgs = append(runArgs, runtimeRef)
	if opts.Cmd != "" {
		runArgs = append(runArgs, opts.Cmd)
	}
	runArgs = append(runArgs, opts.Args...)
	return dockerBin, runArgs, ""
}

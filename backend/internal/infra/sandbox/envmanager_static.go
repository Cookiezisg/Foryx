// envmanager_static.go — EnvManager paired with StaticBinaryInstaller.
//
// Static-binary plugins have nothing to install per env: the binary
// itself is the entire payload. CreateEnv mkdirs an empty env (Spawn
// uses it as cwd / log / state directory); InstallDeps and InstallExtras
// are no-ops.
//
// EnvBin returns the binary path that StaticBinaryInstaller.Install
// produced — caller passes the binary's filename as binName so we can
// reach back into <sandboxRoot>/static-binaries/<kind>/<binName>. Note
// this requires the EnvManager to know sandboxRoot (passed at
// construction).
//
// envmanager_static.go ——与 StaticBinaryInstaller 配对的 EnvManager。
//
// Static-binary plugin 没 per env 要装的东西：binary 本身就是全部 payload。
// CreateEnv mkdir 空 env（Spawn 用它当 cwd / log / state 目录）；
// InstallDeps / InstallExtras no-op。
//
// EnvBin 返 StaticBinaryInstaller.Install 产出的 binary 路径——调用方传
// binary 文件名作 binName，让我们能拿到 <sandboxRoot>/static-binaries/<kind>/<binName>。
// 这需要 EnvManager 知道 sandboxRoot（构造时传）。

package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

// StaticBinaryEnvManager satisfies sandboxdomain.EnvManager for plugins
// installed via StaticBinaryInstaller. One instance per kind, paired
// with a matching installer.
//
// StaticBinaryEnvManager 满足 sandboxdomain.EnvManager 给经
// StaticBinaryInstaller 装的 plugin。每 kind 一个实例，与匹配 installer 配对。
type StaticBinaryEnvManager struct {
	kind        string
	sandboxRoot string // absolute path to <dataDir>/sandbox/, needed for EnvBin lookup
}

// NewStaticBinaryEnvManager constructs a manager for the given kind.
// sandboxRoot must match what StaticBinaryInstaller was given.
//
// NewStaticBinaryEnvManager 构造给定 kind 的 manager。sandboxRoot 必须匹配
// StaticBinaryInstaller 收到的值。
func NewStaticBinaryEnvManager(kind, sandboxRoot string) *StaticBinaryEnvManager {
	return &StaticBinaryEnvManager{kind: kind, sandboxRoot: sandboxRoot}
}

// Kind reports the dispatch tag — must match StaticBinaryInstaller(kind).
//
// Kind 报告派发 tag——必须匹配 StaticBinaryInstaller(kind)。
func (s *StaticBinaryEnvManager) Kind() string { return s.kind }

// CreateEnv mkdirs the env directory and returns. Used by Spawn as cwd /
// scratch space for the plugin to write logs / state files into.
//
// CreateEnv mkdir env 目录后返。Spawn 用作 cwd / scratch 给 plugin 写
// log / state 文件。
func (s *StaticBinaryEnvManager) CreateEnv(ctx context.Context, runtimePath, envPath string) error {
	if err := os.MkdirAll(envPath, 0o755); err != nil {
		return fmt.Errorf("sandbox.StaticBinaryEnvManager.CreateEnv %s: %w (env: %w)", s.kind, err, sandboxdomain.ErrEnvCreateFailed)
	}
	return nil
}

// InstallDeps is a no-op — static binaries are self-contained.
//
// InstallDeps no-op——static 二进制自包含。
func (s *StaticBinaryEnvManager) InstallDeps(ctx context.Context, runtimePath, envPath string, deps []string, stream sandboxdomain.ProgressFunc) error {
	return nil
}

// InstallExtras is a no-op for the same reason.
//
// InstallExtras 同理 no-op。
func (s *StaticBinaryEnvManager) InstallExtras(ctx context.Context, runtimePath, envPath string, extras []string, stream sandboxdomain.ProgressFunc) error {
	return nil
}

// EnvBin returns the absolute path to the static binary installed by the
// paired StaticBinaryInstaller. Note this path is shared across all envs
// of this kind (the binary itself lives outside per-env dirs); per-env
// "binaries" don't make sense for static-binary plugins.
//
// EnvBin 返 StaticBinaryInstaller 装的 static 二进制绝对路径。注意该路径
// 在该 kind 所有 env 间共享（二进制本体位于 per-env 目录之外）；static-binary
// plugin 的 per-env "binary" 不适用。
func (s *StaticBinaryEnvManager) EnvBin(envPath, binName string) string {
	return filepath.Join(s.sandboxRoot, staticBinariesSubdir, s.kind, binName)
}

// EnvDir returns the env path unchanged — used as Spawn cwd.
//
// EnvDir 原样返 envPath——Spawn 用作 cwd。
func (s *StaticBinaryEnvManager) EnvDir(envPath string) string { return envPath }

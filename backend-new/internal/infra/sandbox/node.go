package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

// NodeEnvManager is the npm-backed EnvManager for Node plugin envs.
//
// NodeEnvManager 是基于 npm 的 Node plugin env 管理器。
type NodeEnvManager struct{}

var _ sandboxdomain.EnvManager = (*NodeEnvManager)(nil)

// NewNodeEnvManager constructs the manager (npm comes bundled with node@22).
//
// NewNodeEnvManager 构造 manager（npm 随 node@22 自带）。
func NewNodeEnvManager() *NodeEnvManager {
	return &NodeEnvManager{}
}

func (n *NodeEnvManager) Kind() string { return "node" }

// CreateEnv writes a minimal package.json so subsequent npm commands anchor; idempotent.
//
// CreateEnv 写最小 package.json 让后续 npm 命令有锚；幂等。
func (n *NodeEnvManager) CreateEnv(ctx context.Context, runtimePath, envPath string) error {
	pkgJSON := filepath.Join(envPath, "package.json")
	if _, err := os.Stat(pkgJSON); err == nil {
		return nil
	}
	if err := os.MkdirAll(envPath, 0o755); err != nil {
		return fmt.Errorf("sandbox.NodeEnvManager.CreateEnv: mkdir env: %w", err)
	}
	manifest := map[string]any{
		"name":    "forgify-env-" + filepath.Base(envPath),
		"version": "0.0.0",
		"private": true,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("sandbox.NodeEnvManager.CreateEnv: marshal pkg: %w", err)
	}
	if err := os.WriteFile(pkgJSON, data, 0o644); err != nil {
		return fmt.Errorf("sandbox.NodeEnvManager.CreateEnv: write pkg: %w: %w", sandboxdomain.ErrEnvCreateFailed, err)
	}
	return nil
}

// InstallDeps runs `npm install ...` from envPath using the runtime's bundled npm.
//
// InstallDeps 在 envPath 跑 `npm install ...`，使用 runtime 自带 npm。
func (n *NodeEnvManager) InstallDeps(ctx context.Context, runtimePath, envPath string, deps []string, stream sandboxdomain.ProgressFunc) error {
	if len(deps) == 0 {
		return nil
	}
	npmBin := filepath.Join(runtimePath, "bin", "npm")
	if runtime.GOOS == "windows" {
		npmBin = filepath.Join(runtimePath, "npm.cmd")
	}
	args := append([]string{"install"}, deps...)
	cmd := exec.CommandContext(ctx, npmBin, args...)
	cmd.Dir = envPath
	return RunWithStderrCapture(cmd, stream,
		sandboxdomain.ErrDepInstallFailed,
		fmt.Sprintf("sandbox.NodeEnvManager.InstallDeps %v", deps))
}

// ResolveExec resolves opts.Cmd. node's BUNDLED runners (npx/npm/node — in the runtime's own
// bin/, e.g. how mcp servers launch: `npx -y <pkg>`) resolve against the ABSOLUTE runtime
// install dir (runtimeRef). Other bare names resolve inside the env's node_modules/.bin. A
// path-like cmd passes through. cwd = envPath.
//
// ResolveExec 解析 opts.Cmd。node 自带 runner（npx/npm/node——在 runtime 自己的 bin/，如 mcp
// server 用的 `npx -y <pkg>`）按绝对 runtime install dir（runtimeRef）解析；其它裸名按 env 的
// node_modules/.bin；路径式 cmd 原样透传。cwd = envPath。
func (n *NodeEnvManager) ResolveExec(runtimeRef string, envPath string, opts sandboxdomain.SpawnOpts) (cmd string, args []string, cwd string) {
	cmd = opts.Cmd
	switch {
	case isNodeRuntimeTool(cmd):
		cmd = runtimeToolPath(runtimeRef, cmd)
	case isBareCommand(cmd):
		cmd = n.binPath(envPath, cmd)
	}
	return cmd, opts.Args, envPath
}

// isNodeRuntimeTool reports whether cmd is a tool bundled with the node runtime itself (in
// <runtime>/bin), not an env dependency. npx is how mcp node servers launch.
//
// isNodeRuntimeTool 报告 cmd 是否为 node runtime 自带工具（在 <runtime>/bin），而非 env 依赖。
// npx 是 mcp node server 的启动方式。
func isNodeRuntimeTool(cmd string) bool {
	switch cmd {
	case "npx", "npm", "node", "corepack":
		return true
	}
	return false
}

// runtimeToolPath returns <runtimeRef>/bin/<name> (win: <name>.cmd, except node → node.exe).
//
// runtimeToolPath 返 <runtimeRef>/bin/<name>（win：<name>.cmd，node 例外→ node.exe）。
func runtimeToolPath(runtimeRef, name string) string {
	if runtime.GOOS == "windows" {
		if name == "node" {
			name += ".exe"
		} else {
			name += ".cmd"
		}
	}
	return filepath.Join(runtimeRef, "bin", name)
}

// binPath returns the absolute path to a binary inside node_modules/.bin/.
//
// binPath 返 env node_modules/.bin/ 内某 binary 的绝对路径。
func (n *NodeEnvManager) binPath(envPath, binName string) string {
	if runtime.GOOS == "windows" && filepath.Ext(binName) == "" {
		binName += ".cmd"
	}
	return filepath.Join(envPath, "node_modules", ".bin", binName)
}

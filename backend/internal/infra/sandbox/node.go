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


// EnvBin returns the absolute path to a binary inside node_modules/.bin/.
//
// EnvBin 返 env node_modules/.bin/ 内某 binary 的绝对路径。
func (n *NodeEnvManager) EnvBin(envPath, binName string) string {
	if runtime.GOOS == "windows" && filepath.Ext(binName) == "" {
		binName += ".cmd"
	}
	return filepath.Join(envPath, "node_modules", ".bin", binName)
}

func (n *NodeEnvManager) EnvDir(envPath string) string { return envPath }

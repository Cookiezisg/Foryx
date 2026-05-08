// envmanager_node.go — npm-backed EnvManager for Node plugin envs.
//
// Marketplace V3 ships only ~21 curated MCP servers, all stdio + npm —
// the cross-env hardlink dedup npm offered isn't worth the extra
// installer / global store. We use vanilla npm (bundled with node@22)
// to install per-env into <envPath>/node_modules/.
//
// envmanager_node.go ——基于 npm 的 Node plugin env EnvManager。
//
// Marketplace V3 仅 21 条 curated MCP server、全 stdio+npm——npm 的跨 env
// hardlink 共享对 21 条无价值，多带个 installer 不值。改用 node@22 自带的
// npm，per-env 装到 <envPath>/node_modules/。

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

// NodeEnvManager satisfies sandboxdomain.EnvManager for Node.
//
// NodeEnvManager 满足 sandboxdomain.EnvManager 的 Node 实现。
type NodeEnvManager struct{}

// NewNodeEnvManager constructs the manager. No deps — npm comes
// bundled with node@22 (resolved per-call from the runtime path).
//
// NewNodeEnvManager 构造 manager。无依赖——npm 随 node@22 自带（每次调用
// 从 runtime 路径解析）。
func NewNodeEnvManager() *NodeEnvManager {
	return &NodeEnvManager{}
}

// Kind reports the dispatch key — must match MiseInstaller("node").
//
// Kind 报告派发键——必须匹配 MiseInstaller("node")。
func (n *NodeEnvManager) Kind() string { return "node" }

// CreateEnv writes a minimal package.json to envPath so subsequent
// `npm install` / `npm add` commands have an anchor. Idempotent — already-
// existing package.json returns nil.
//
// CreateEnv 在 envPath 写最小 package.json，让后续 `npm install` /
// `npm add` 有锚点。幂等——已存在的 package.json 返 nil。
func (n *NodeEnvManager) CreateEnv(ctx context.Context, runtimePath, envPath string) error {
	pkgJSON := filepath.Join(envPath, "package.json")
	if _, err := os.Stat(pkgJSON); err == nil {
		return nil
	}
	if err := os.MkdirAll(envPath, 0o755); err != nil {
		return fmt.Errorf("sandbox.NodeEnvManager.CreateEnv: mkdir env: %w", err)
	}
	// Minimal package.json — name derived from envPath, private to prevent
	// accidental publish, no scripts/deps so npm has nothing to interpret
	// at install time other than what we explicitly add.
	//
	// 最小 package.json——name 从 envPath 派生，private 防误发，无 scripts/deps
	// 让 npm 在 install 时只解释我们显式 add 的东西。
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
		return fmt.Errorf("sandbox.NodeEnvManager.CreateEnv: write pkg: %w (env: %w)", err, sandboxdomain.ErrEnvCreateFailed)
	}
	return nil
}

// InstallDeps runs `npm install <deps...>` from envPath using the
// runtime's bundled npm. Marketplace V3 stripped npm from the
// installer registry (curated 21 entries don't need cross-env hardlink
// dedup), so we use vanilla npm — node@22 ships it.
//
// InstallDeps 在 envPath 跑 `npm install <deps...>`，用 runtime 自带的
// npm。Marketplace V3 把 npm 从 installer 移除（curated 21 条不需要 N
// env 共享 hardlink 优化），改用 node@22 自带的 npm。
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

// InstallExtras is a no-op for Node — Node plugins declare runtime deps
// only. Browser binary downloads (Playwright's chromium) live in the
// dedicated PlaywrightEnvManager which orchestrates `playwright install`
// after the npm package itself is in node_modules.
//
// InstallExtras Node 上是 no-op——Node plugin 只声明 runtime deps。浏览器
// 二进制下载（Playwright 的 chromium）在专用的 PlaywrightEnvManager 里编排，
// 在 npm 包本身已进 node_modules 之后跑 `playwright install`。
func (n *NodeEnvManager) InstallExtras(ctx context.Context, runtimePath, envPath string, extras []string, stream sandboxdomain.ProgressFunc) error {
	return nil
}

// EnvBin returns the absolute path to a binary inside the env's
// node_modules/.bin/ shim directory (npm/npm convention). On Windows
// npm generates *.cmd / *.ps1 wrappers; we tack on .cmd if the caller
// did not provide an extension.
//
// EnvBin 返 env 的 node_modules/.bin/ shim 目录中某 binary 绝对路径
// （npm/npm 约定）。Windows 上 npm 生成 *.cmd / *.ps1 包装；调用方
// 未传扩展名时加 .cmd。
func (n *NodeEnvManager) EnvBin(envPath, binName string) string {
	if runtime.GOOS == "windows" && filepath.Ext(binName) == "" {
		binName += ".cmd"
	}
	return filepath.Join(envPath, "node_modules", ".bin", binName)
}

// EnvDir returns the env root.
//
// EnvDir 返 env 根目录。
func (n *NodeEnvManager) EnvDir(envPath string) string { return envPath }

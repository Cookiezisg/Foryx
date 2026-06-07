package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

// PythonEnvManager is the uv-backed EnvManager for Python plugin envs.
//
// PythonEnvManager 是基于 uv 的 Python plugin env 管理器。
type PythonEnvManager struct {
	tools sandboxdomain.ToolRegistry
}

var _ sandboxdomain.EnvManager = (*PythonEnvManager)(nil)

// NewPythonEnvManager constructs the manager (tools resolves uv lazily).
//
// NewPythonEnvManager 构造 manager（tools 懒解析 uv）。
func NewPythonEnvManager(tools sandboxdomain.ToolRegistry) *PythonEnvManager {
	return &PythonEnvManager{tools: tools}
}

func (p *PythonEnvManager) Kind() string { return "python" }

// CreateEnv runs `uv venv` at <envPath>/.venv; idempotent.
//
// CreateEnv 在 <envPath>/.venv 跑 `uv venv`；幂等。
func (p *PythonEnvManager) CreateEnv(ctx context.Context, runtimePath, envPath string) error {
	venvDir := filepath.Join(envPath, ".venv")
	if _, err := os.Stat(venvDir); err == nil {
		return nil
	}
	if err := os.MkdirAll(envPath, 0o755); err != nil {
		return fmt.Errorf("sandbox.PythonEnvManager.CreateEnv: mkdir env: %w", err)
	}
	uvBin, err := p.tools.EnsureTool(ctx, "uv", "")
	if err != nil {
		return fmt.Errorf("sandbox.PythonEnvManager.CreateEnv: locate uv: %w", err)
	}
	cmd := exec.CommandContext(ctx, uvBin, "venv", "--python", runtimePath, venvDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sandbox.PythonEnvManager.CreateEnv %s: %w: %w (uv output: %s)",
			venvDir, sandboxdomain.ErrEnvCreateFailed, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// InstallDeps runs `uv pip install ...`; stream fires for each stderr line.
//
// InstallDeps 跑 `uv pip install ...`；stream 在每行 stderr 触发。
func (p *PythonEnvManager) InstallDeps(ctx context.Context, runtimePath, envPath string, deps []string, stream sandboxdomain.ProgressFunc) error {
	if len(deps) == 0 {
		return nil
	}
	uvBin, err := p.tools.EnsureTool(ctx, "uv", "")
	if err != nil {
		return fmt.Errorf("sandbox.PythonEnvManager.InstallDeps: locate uv: %w", err)
	}
	venvPython := filepath.Join(envPath, ".venv", venvBinSubdir(), pythonExe())
	args := append([]string{"pip", "install", "--python", venvPython}, deps...)
	cmd := exec.CommandContext(ctx, uvBin, args...)
	return RunWithStderrCapture(cmd, stream,
		sandboxdomain.ErrDepInstallFailed,
		fmt.Sprintf("sandbox.PythonEnvManager.InstallDeps %v", deps))
}

// ResolveExec resolves opts.Cmd. The uv RUNNER (uvx/uv — how mcp python servers launch:
// `uvx <pkg>`) lives in the aqua-installed uv tool dir, resolved via the ToolRegistry (NOT the
// python runtime/env). The env was created with uv, so uv is already installed → EnsureTool is
// a fast lookup. Other bare names resolve inside the env's venv; a path-like cmd passes through.
//
// ResolveExec 解析 opts.Cmd。uv runner（uvx/uv——mcp python server 的启动方式 `uvx <pkg>`）在
// aqua 装的 uv 工具目录，经 ToolRegistry 解析（不在 python runtime/env）。env 用 uv 建的，故 uv 已装
// → EnsureTool 是快速查找。其它裸名按 env 的 venv；路径式 cmd 原样透传。
func (p *PythonEnvManager) ResolveExec(_ string, envPath string, opts sandboxdomain.SpawnOpts) (cmd string, args []string, cwd string) {
	cmd = opts.Cmd
	switch {
	case isPythonRuntimeTool(cmd):
		if uvBin, err := p.tools.EnsureTool(context.Background(), "uv", ""); err == nil {
			name := cmd
			if runtime.GOOS == "windows" {
				name += ".exe"
			}
			cmd = filepath.Join(filepath.Dir(uvBin), name) // uvx ships beside uv
		}
	case isBareCommand(cmd):
		cmd = p.binPath(envPath, cmd)
	}
	return cmd, opts.Args, envPath
}

// isPythonRuntimeTool reports whether cmd is the uv package runner (uvx/uv), which lives in the
// aqua-installed uv dir, not the python env's venv. uvx is how mcp python servers launch.
//
// isPythonRuntimeTool 报告 cmd 是否为 uv 包运行器（uvx/uv），在 aqua 装的 uv 目录、不在 python env
// 的 venv。uvx 是 mcp python server 的启动方式。
func isPythonRuntimeTool(cmd string) bool {
	switch cmd {
	case "uvx", "uv":
		return true
	}
	return false
}

// binPath returns the absolute path to a binary inside the env's venv.
//
// binPath 返 env venv 内某 binary 的绝对路径。
func (p *PythonEnvManager) binPath(envPath, binName string) string {
	if runtime.GOOS == "windows" && filepath.Ext(binName) == "" {
		binName += ".exe"
	}
	return filepath.Join(envPath, ".venv", venvBinSubdir(), binName)
}

func venvBinSubdir() string {
	if runtime.GOOS == "windows" {
		return "Scripts"
	}
	return "bin"
}

func pythonExe() string {
	if runtime.GOOS == "windows" {
		return "python.exe"
	}
	return "python"
}

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

// ResolveExec resolves opts.Cmd to a binary inside the venv (a bare name →
// <env>/.venv/bin/<cmd>; a path-like cmd passes through), with cwd = envPath.
// runtimeRef is unused — the venv already pins its interpreter.
//
// ResolveExec 把 opts.Cmd 解析为 venv 内 binary（裸名 → <env>/.venv/bin/<cmd>；
// 路径式 cmd 原样透传），cwd = envPath。runtimeRef 不用——venv 已钉死解释器。
func (p *PythonEnvManager) ResolveExec(_ string, envPath string, opts sandboxdomain.SpawnOpts) (cmd string, args []string, cwd string) {
	cmd = opts.Cmd
	if isBareCommand(cmd) {
		cmd = p.binPath(envPath, cmd)
	}
	return cmd, opts.Args, envPath
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

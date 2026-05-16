package function

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

// SandboxAdapter satisfies function.Sandbox by delegating runtime/env work to sandboxapp.Service.
//
// SandboxAdapter 把 runtime/env 工作委托给 sandboxapp.Service 满足 function.Sandbox。
type SandboxAdapter struct {
	svc             *sandboxapp.Service
	functionDataDir string

	pythonPathOnce sync.Once
	pythonPath     string
}

var _ Sandbox = (*SandboxAdapter)(nil)

// NewSandboxAdapter wires the adapter to a sandbox service and the function data dir.
//
// NewSandboxAdapter 把 adapter 绑到 sandbox service 与 function 数据根目录。
func NewSandboxAdapter(svc *sandboxapp.Service, functionDataDir string) *SandboxAdapter {
	return &SandboxAdapter{svc: svc, functionDataDir: functionDataDir}
}

// PythonPath lazy-resolves the bundled Python interpreter; returns "" on EnsureTool failure.
//
// PythonPath 懒解析捆绑 Python 解释器，EnsureTool 失败返 ""。
func (a *SandboxAdapter) PythonPath() string {
	a.pythonPathOnce.Do(func() {
		path, err := a.svc.EnsureTool(context.Background(), "python", "")
		if err != nil {
			return
		}
		a.pythonPath = path
	})
	return a.pythonPath
}

// Sync materializes the venv via Service.EnsureEnv.
//
// Sync 通过 Service.EnsureEnv 物化 venv。
func (a *SandboxAdapter) Sync(ctx context.Context, req SyncRequest) error {
	owner := sandboxdomain.Owner{
		Kind: sandboxdomain.OwnerKindFunction,
		ID:   req.FunctionID + "_" + req.EnvID,
	}
	spec := sandboxdomain.EnvSpec{
		Runtime: sandboxdomain.RuntimeSpec{Kind: "python", Version: req.PythonVersion},
		Deps:    req.Dependencies,
	}
	var stream sandboxdomain.ProgressFunc
	if req.OnProgress != nil {
		stream = func(stage, message string, _ int) {
			req.OnProgress(stage, message)
		}
	}
	if _, err := a.svc.EnsureEnv(ctx, owner, spec, stream); err != nil {
		return &SyncError{Cause: err, Stderr: err.Error()}
	}
	return nil
}

// Run writes main.py + driver and spawns "python main.py"; non-zero exit becomes ExecutionResult{OK:false}.
//
// Run 写 main.py + driver 并 spawn "python main.py"，非零退出返 ExecutionResult{OK:false} 而非 Go error。
func (a *SandboxAdapter) Run(ctx context.Context, req RunRequest) (*functiondomain.ExecutionResult, error) {
	verDir := a.versionDir(req.FunctionID, req.VersionID)
	if err := os.MkdirAll(verDir, 0o755); err != nil {
		return nil, fmt.Errorf("functionapp.SandboxAdapter.Run: mkdir verDir: %w", err)
	}

	funcName := req.EntryFunction
	if funcName == "" {
		var err error
		funcName, err = extractFuncName(req.Code)
		if err != nil {
			return nil, fmt.Errorf("functionapp.SandboxAdapter.Run: %w", err)
		}
	}
	mainPy := filepath.Join(verDir, "main.py")
	if err := writeAtomic(mainPy, []byte(req.Code+buildDriver(funcName)), 0o644); err != nil {
		return nil, fmt.Errorf("functionapp.SandboxAdapter.Run: write main.py: %w", err)
	}

	inputJSON, err := json.Marshal(req.Input)
	if err != nil {
		return nil, fmt.Errorf("functionapp.SandboxAdapter.Run: marshal input: %w", err)
	}

	owner := sandboxdomain.Owner{
		Kind: sandboxdomain.OwnerKindFunction,
		ID:   req.FunctionID + "_" + req.EnvID,
	}
	res, spawnErr := a.svc.Spawn(ctx, owner, sandboxdomain.SpawnOpts{
		Cmd:   "python",
		Args:  []string{mainPy},
		Stdin: inputJSON,
	})
	if spawnErr != nil {
		return nil, fmt.Errorf("functionapp.SandboxAdapter.Run: %w", spawnErr)
	}

	out := &functiondomain.ExecutionResult{ElapsedMs: res.Duration.Milliseconds()}
	if !res.Ok {
		msg := strings.TrimSpace(string(res.Stderr))
		if msg == "" {
			msg = fmt.Sprintf("python exit %d", res.ExitCode)
		}
		out.OK = false
		out.ErrorMsg = msg
		return out, nil
	}
	var output any
	if err := json.Unmarshal(res.Stdout, &output); err != nil {
		output = strings.TrimSpace(string(res.Stdout))
	}
	out.OK = true
	out.Output = output
	return out, nil
}

// WriteCodeFile updates main.py for a version without touching its venv.
//
// WriteCodeFile 重写 version 的 main.py，不动 venv。
func (a *SandboxAdapter) WriteCodeFile(ctx context.Context, functionID, versionID, code, entryFunction string) error {
	verDir := a.versionDir(functionID, versionID)
	if err := os.MkdirAll(verDir, 0o755); err != nil {
		return fmt.Errorf("functionapp.SandboxAdapter.WriteCodeFile: mkdir: %w", err)
	}
	funcName := entryFunction
	if funcName == "" {
		var err error
		funcName, err = extractFuncName(code)
		if err != nil {
			return fmt.Errorf("functionapp.SandboxAdapter.WriteCodeFile: %w", err)
		}
	}
	mainPy := filepath.Join(verDir, "main.py")
	return writeAtomic(mainPy, []byte(code+buildDriver(funcName)), 0o644)
}

// Destroy removes every env owned by this function and its on-disk versions dir.
//
// Destroy 删除该 function 的所有 env 与盘上 versions 目录。
func (a *SandboxAdapter) Destroy(ctx context.Context, functionID string) error {
	envs, err := a.svc.ListEnvs(ctx, sandboxdomain.OwnerKindFunction)
	if err != nil {
		return fmt.Errorf("functionapp.SandboxAdapter.Destroy: list envs: %w", err)
	}
	prefix := functionID + "_"
	for _, e := range envs {
		if !strings.HasPrefix(e.OwnerID, prefix) {
			continue
		}
		owner := sandboxdomain.Owner{Kind: sandboxdomain.OwnerKindFunction, ID: e.OwnerID}
		if err := a.svc.Destroy(ctx, owner); err != nil {
			return fmt.Errorf("functionapp.SandboxAdapter.Destroy %s: %w", owner.ID, err)
		}
	}
	functionDir := filepath.Join(a.functionDataDir, "functions", functionID)
	if err := os.RemoveAll(functionDir); err != nil {
		return fmt.Errorf("functionapp.SandboxAdapter.Destroy: rm %s: %w", functionDir, err)
	}
	return nil
}

// DestroyEnv removes a single (functionID, envID) env.
//
// DestroyEnv 删除单个 (functionID, envID) env。
func (a *SandboxAdapter) DestroyEnv(ctx context.Context, functionID, envID string) error {
	owner := sandboxdomain.Owner{
		Kind: sandboxdomain.OwnerKindFunction,
		ID:   functionID + "_" + envID,
	}
	return a.svc.Destroy(ctx, owner)
}

func (a *SandboxAdapter) versionDir(functionID, versionID string) string {
	return filepath.Join(a.functionDataDir, "functions", functionID, "versions", versionID)
}

const driverTemplate = `

if __name__ == "__main__":
    import json as _json, sys as _sys
    _input = _json.load(_sys.stdin)
    _result = {FUNC_NAME}(**_input)
    print(_json.dumps(_result))
`

func buildDriver(funcName string) string {
	return strings.Replace(driverTemplate, "{FUNC_NAME}", funcName, 1)
}

func extractFuncName(code string) (string, error) {
	for line := range strings.SplitSeq(code, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "def ") {
			continue
		}
		rest := strings.TrimPrefix(trimmed, "def ")
		if idx := strings.IndexAny(rest, "(: "); idx > 0 {
			return rest[:idx], nil
		}
	}
	return "", fmt.Errorf("no function definition found in code")
}

// writeAtomic write-then-rename is concurrency-safe — each invocation
// gets a unique temp filename via os.CreateTemp so parallel writers don't
// collide on the same `<path>.tmp` (which would cause one rename to find
// its source already moved by the other).
//
// writeAtomic 写后 rename，**并发安全**——每次调用经 os.CreateTemp 取唯一
// 临时文件名，避免多 goroutine 撞同一个 `<path>.tmp`（否则一方 rename 时
// 源已被另一方移走，导致 no such file or directory）。
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir, base := filepath.Split(path)
	f, err := os.CreateTemp(dir, base+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

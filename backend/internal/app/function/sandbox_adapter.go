// sandbox_adapter.go — bridges the function.Sandbox port to sandboxapp.Service.
//
// Owner.ID convention: "<functionID>_<envID>" — mirrors forge's per-entity
// EnvID buffer (lets us keep 2-3 EnvIDs per function for fast revert without
// re-syncing). Layout matches forge:
//
//     <dataDir>/functions/<functionID>/versions/<versionID>/main.py
//
// The adapter owns the file layout (driver template + main.py write); the
// sandbox v2 service only knows about runtime + env materialization.
//
// Per forge_redesign D5, this duplicates ~150 lines of forge's adapter rather
// than extracting to a shared pkg — once handler ships its own sandbox
// adapter (Plan 02) we'll re-evaluate whether the count justifies a `pkg/
// pyrunner` (handlers are stateful, file layout differs, so probably not).
//
// sandbox_adapter.go —— 桥接 function.Sandbox 端口到 sandboxapp.Service。
//
// Owner.ID = "<functionID>_<envID>",镜像 forge per-entity EnvID buffer。布局:
//     <dataDir>/functions/<functionID>/versions/<versionID>/main.py
//
// adapter 拥有文件布局(driver 模板 + main.py 写);v2 sandbox 只管 runtime +
// env 物化。
//
// per forge_redesign D5,这里有意复制 forge 的 ~150 行不抽公共 pkg——handler
// 跑出 sandbox adapter(Plan 02)之后再看是否值得抽 pkg/pyrunner(handler 是
// 有状态的,布局不一样,大概率不抽)。

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

// SandboxAdapter satisfies the function.Sandbox interface by delegating
// runtime/env management to sandboxapp.Service while owning the function-
// specific file layout.
//
// SandboxAdapter 通过委托 sandboxapp.Service 管 runtime/env 满足
// function.Sandbox 接口,同时拥有 function 专属文件布局。
type SandboxAdapter struct {
	svc             *sandboxapp.Service
	functionDataDir string // <dataDir> — adapter writes versions/<vID>/main.py under <dataDir>/functions/

	pythonPathOnce sync.Once
	pythonPath     string
}

// Static-assert SandboxAdapter implements Sandbox.
var _ Sandbox = (*SandboxAdapter)(nil)

// NewSandboxAdapter wires the adapter to a sandbox service + the data
// directory under which function versions live.
//
// NewSandboxAdapter 把 adapter 跟 sandbox service + function 版本根目录绑起来。
func NewSandboxAdapter(svc *sandboxapp.Service, functionDataDir string) *SandboxAdapter {
	return &SandboxAdapter{svc: svc, functionDataDir: functionDataDir}
}

// PythonPath lazy-resolves the bundled Python interpreter on first call,
// caching for process lifetime. Returns "" on EnsureTool failure — the caller
// (Service.validate) treats "" as "AST parse unavailable" and degrades.
//
// PythonPath 首次调用懒解析捆绑 Python 解释器,结果缓存到进程生命周期。
// EnsureTool 失败返 "",调用方降级处理。
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

// Sync materializes the venv via Service.EnsureEnv. Owner.Kind = "function".
//
// Sync 通过 Service.EnsureEnv 物化 venv。Owner.Kind = "function"。
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

// Run writes main.py + driver to <dataDir>/functions/<fnID>/versions/<vID>/,
// then spawns "python main.py" via Service.Spawn piping the input JSON to
// stdin. Non-zero exit returns ExecutionResult{OK:false, ErrorMsg:stderr}
// (NOT a Go error) — LLM sees the failure as a tool_result, not infra error.
//
// Run 把 main.py + driver 写到 <dataDir>/functions/<fnID>/versions/<vID>/,
// 经 Service.Spawn 跑 "python main.py" 把 input pipe 到 stdin。非零退出返
// ExecutionResult{OK:false, ErrorMsg:stderr}(不是 Go error)——LLM 看到
// 失败为 tool_result 而非基础设施错。
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
// WriteCodeFile 写 version 的 main.py 不动 venv。
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

// Destroy removes every env owned by this function and the function's
// versions directory on disk.
//
// Destroy 删该 function 所有 env + 盘上 versions 目录。
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
// DestroyEnv 删单个 (functionID, envID) env。
func (a *SandboxAdapter) DestroyEnv(ctx context.Context, functionID, envID string) error {
	owner := sandboxdomain.Owner{
		Kind: sandboxdomain.OwnerKindFunction,
		ID:   functionID + "_" + envID,
	}
	return a.svc.Destroy(ctx, owner)
}

// versionDir returns the absolute path where main.py lives for a
// (functionID, versionID) pair.
//
// versionDir 返 (functionID, versionID) 对的 main.py 绝对路径。
func (a *SandboxAdapter) versionDir(functionID, versionID string) string {
	return filepath.Join(a.functionDataDir, "functions", functionID, "versions", versionID)
}

// ── helpers (mirrored from forge sandbox adapter per D5) ────────

// driverTemplate wraps the user function with a stdin → kwargs → stdout JSON
// shim so Run can pipe input/output without the user code knowing.
//
// driverTemplate 把用户函数包以 stdin → kwargs → stdout JSON shim。
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

// extractFuncName parses the first `def <name>` line from the code.
//
// extractFuncName 从代码第一个 `def <name>` 行解析名字。
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

// writeAtomic writes data via tmp + rename so readers never see a half-written
// file.
//
// writeAtomic tmp + rename 原子写。
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// sandbox_adapter.go — bridges the forge.Sandbox port to the new
// sandboxapp.Service. Forge service code is unchanged; this adapter
// translates each forge call into the appropriate Service operation.
//
// Owner.ID convention: "<forgeID>_<envID>". Sandbox.md §5 suggests using
// just envID as the owner key (so multiple forges sharing the same deps
// share a single env), but that breaks v1 forge's per-forge N=3 EnvID
// buffer for quick revert. Adapter keeps v1 behavior — one env per
// (forge, deps) pair — and revisits the shared-env optimization in v2
// after dogfooding shows the disk impact.
//
// This file owns:
//   - main.py + driver template authoring (used to live in
//     infra/sandbox/run.go; the v2 sandbox doesn't manage forge file
//     layout, so the adapter takes ownership)
//   - extractFuncName helper (same)
//   - writeAtomic helper (same)
//
// sandbox_adapter.go ——把 forge.Sandbox 端口桥接到新 sandboxapp.Service。
// forge service 代码不变；adapter 把每个 forge 调用翻译为对应 Service 操作。
//
// Owner.ID 约定 "<forgeID>_<envID>"。sandbox.md §5 建议仅用 envID 作 owner
// key（多 forge 共享相同 deps 共享一份 env），但这破坏 v1 forge 的 per-forge
// N=3 EnvID buffer 快速 revert。adapter 保 v1 行为——一个 env 对应一个
// (forge, deps)——v2 dogfooding 显示磁盘影响后再考虑共享 env 优化。
//
// 本文件拥有：main.py + driver 模板（曾在 infra/sandbox/run.go；v2 sandbox
// 不管 forge 文件布局，adapter 接管）+ extractFuncName + writeAtomic helper。

package forge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

// SandboxAdapter satisfies the forge.Sandbox interface by delegating to
// sandboxapp.Service for runtime/env management while owning the forge-
// specific file layout (versions/<vID>/main.py).
//
// SandboxAdapter 通过委托 sandboxapp.Service 管 runtime/env 满足
// forge.Sandbox 接口，同时拥有 forge 专属文件布局（versions/<vID>/main.py）。
type SandboxAdapter struct {
	svc          *sandboxapp.Service
	forgeDataDir string // <dataDir> — adapter writes versions/<vID>/main.py under <forgeDataDir>/forges/

	pythonPathOnce sync.Once
	pythonPath     string
}

// NewSandboxAdapter wires the adapter to a sandbox service + the data
// directory under which forge versions live. forgeDataDir is typically
// the top-level dataDir; the adapter appends "/forges/<id>/versions/<id>/".
//
// NewSandboxAdapter 把 adapter 跟 sandbox service + forge versions 的根
// 目录绑起来。forgeDataDir 通常是顶层 dataDir；adapter 拼
// "/forges/<id>/versions/<id>/"。
func NewSandboxAdapter(svc *sandboxapp.Service, forgeDataDir string) *SandboxAdapter {
	return &SandboxAdapter{svc: svc, forgeDataDir: forgeDataDir}
}

// PythonPath lazy-resolves the bundled Python interpreter on first call,
// caching the result for the process lifetime. Returns "" if EnsureTool
// fails — the caller (forge.parse) treats "" as "AST parse unavailable"
// and degrades gracefully.
//
// PythonPath 首次调用时懒解析捆绑 Python 解释器，结果缓存到进程生命周期。
// EnsureTool 失败返 ""——调用方（forge.parse）把 "" 当 "AST parse 不可用"
// 优雅降级。
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

// Sync materializes the venv via Service.EnsureEnv. Owner is
// (forge, "<forgeID>_<envID>"). OnProgress is wrapped so the v1 two-arg
// callback bridges to the v2 three-arg ProgressFunc (percent stays -1).
//
// Sync 通过 Service.EnsureEnv 物化 venv。Owner 是 (forge, "<forgeID>_<envID>")。
// OnProgress 包装一层，让 v1 两参 callback 桥接到 v2 三参 ProgressFunc
// （percent 保 -1）。
func (a *SandboxAdapter) Sync(ctx context.Context, req SyncRequest) error {
	owner := sandboxdomain.Owner{
		Kind: sandboxdomain.OwnerKindForge,
		ID:   req.ForgeID + "_" + req.EnvID,
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
		// Wrap as SyncError so forge service can errors.As + extract stderr.
		// 包装为 SyncError 让 forge service 能 errors.As + 提 stderr。
		return &SyncError{Cause: err, Stderr: err.Error()}
	}
	return nil
}

// Run writes main.py to <forgeDataDir>/forges/<fID>/versions/<vID>/, then
// spawns "python <main.py>" via Service.Spawn with the input piped to
// stdin. The non-zero exit / Ok=false case is translated to a forge
// ExecutionResult with ErrorMsg, NOT a Go error (matches v1 sandbox
// behavior — the LLM sees the failure as a tool_result, not an
// infrastructure error).
//
// Run 把 main.py 写到 <forgeDataDir>/forges/<fID>/versions/<vID>/，再通过
// Service.Spawn 起 "python <main.py>" 把 input pipe 到 stdin。非零退出 /
// Ok=false 翻译为 forge ExecutionResult + ErrorMsg，**不**返 Go error
// （跟 v1 sandbox 行为一致——LLM 看到失败为 tool_result 而非基础设施错）。
func (a *SandboxAdapter) Run(ctx context.Context, req RunRequest) (*forgedomain.ExecutionResult, error) {
	verDir := a.versionDir(req.ForgeID, req.VersionID)
	if err := os.MkdirAll(verDir, 0o755); err != nil {
		return nil, fmt.Errorf("forgeapp.SandboxAdapter.Run: mkdir verDir: %w", err)
	}

	funcName := req.EntryFunction
	if funcName == "" {
		var err error
		funcName, err = extractFuncName(req.Code)
		if err != nil {
			return nil, fmt.Errorf("forgeapp.SandboxAdapter.Run: %w", err)
		}
	}
	mainPy := filepath.Join(verDir, "main.py")
	if err := writeAtomic(mainPy, []byte(req.Code+buildDriver(funcName)), 0o644); err != nil {
		return nil, fmt.Errorf("forgeapp.SandboxAdapter.Run: write main.py: %w", err)
	}

	inputJSON, err := json.Marshal(req.Input)
	if err != nil {
		return nil, fmt.Errorf("forgeapp.SandboxAdapter.Run: marshal input: %w", err)
	}

	owner := sandboxdomain.Owner{
		Kind: sandboxdomain.OwnerKindForge,
		ID:   req.ForgeID + "_" + req.EnvID,
	}
	res, spawnErr := a.svc.Spawn(ctx, owner, sandboxdomain.SpawnOpts{
		Cmd:   "python",
		Args:  []string{mainPy},
		Stdin: inputJSON,
	})
	if spawnErr != nil {
		// Infrastructure failure (env missing, OS denied exec, etc.) — this
		// IS an error path the forge service should bubble up.
		// 基础设施失败（env 缺、OS 拒 exec 等）——forge service 该上抛的错。
		return nil, fmt.Errorf("forgeapp.SandboxAdapter.Run: %w", spawnErr)
	}

	out := &forgedomain.ExecutionResult{ElapsedMs: res.Duration.Milliseconds()}
	if !res.Ok {
		msg := strings.TrimSpace(string(res.Stderr))
		if msg == "" {
			msg = fmt.Sprintf("python exit %d", res.ExitCode)
		}
		out.OK = false
		out.ErrorMsg = msg
		return out, nil
	}
	// Stdout is JSON by convention (driver template wraps the user
	// function's return value); raw string fallback for forges that
	// printed something else.
	//
	// stdout 按约定是 JSON（driver 模板包用户函数返回值）；非 JSON 退回
	// raw string（forge 误打印别的）。
	var output any
	if err := json.Unmarshal(res.Stdout, &output); err != nil {
		output = strings.TrimSpace(string(res.Stdout))
	}
	out.OK = true
	out.Output = output
	return out, nil
}

// WriteCodeFile updates main.py for a version without touching its venv.
// Used when EnvID is unchanged but code changed (caller skips Sync).
//
// WriteCodeFile 写 version 的 main.py 不动 venv。EnvID 不变只代码变时用
// （调用方跳过 Sync）。
func (a *SandboxAdapter) WriteCodeFile(ctx context.Context, forgeID, versionID, code, entryFunction string) error {
	verDir := a.versionDir(forgeID, versionID)
	if err := os.MkdirAll(verDir, 0o755); err != nil {
		return fmt.Errorf("forgeapp.SandboxAdapter.WriteCodeFile: mkdir: %w", err)
	}
	funcName := entryFunction
	if funcName == "" {
		var err error
		funcName, err = extractFuncName(code)
		if err != nil {
			return fmt.Errorf("forgeapp.SandboxAdapter.WriteCodeFile: %w", err)
		}
	}
	mainPy := filepath.Join(verDir, "main.py")
	return writeAtomic(mainPy, []byte(code+buildDriver(funcName)), 0o644)
}

// Destroy removes every env owned by this forge and the forge's versions
// directory on disk. Iterates env manifest looking for "<forgeID>:" prefix
// because Service.Destroy is keyed by full Owner.
//
// Destroy 删该 forge 所有 env + 盘上 versions 目录。遍历 env manifest 找
// "<forgeID>:" 前缀因 Service.Destroy 按完整 Owner 索引。
func (a *SandboxAdapter) Destroy(ctx context.Context, forgeID string) error {
	envs, err := a.svc.ListEnvs(ctx, sandboxdomain.OwnerKindForge)
	if err != nil {
		return fmt.Errorf("forgeapp.SandboxAdapter.Destroy: list envs: %w", err)
	}
	prefix := forgeID + "_"
	for _, e := range envs {
		if !strings.HasPrefix(e.OwnerID, prefix) {
			continue
		}
		owner := sandboxdomain.Owner{Kind: sandboxdomain.OwnerKindForge, ID: e.OwnerID}
		if err := a.svc.Destroy(ctx, owner); err != nil {
			return fmt.Errorf("forgeapp.SandboxAdapter.Destroy %s: %w", owner.ID, err)
		}
	}
	forgeDir := filepath.Join(a.forgeDataDir, "forges", forgeID)
	if err := os.RemoveAll(forgeDir); err != nil {
		return fmt.Errorf("forgeapp.SandboxAdapter.Destroy: rm %s: %w", forgeDir, err)
	}
	return nil
}

// DestroyEnv removes a single (forgeID, envID) env, used by trimEnvBuffer
// when the N=3 history evicts an old EnvID.
//
// DestroyEnv 删单个 (forgeID, envID) env，trimEnvBuffer 在 N=3 历史驱逐
// 旧 EnvID 时用。
func (a *SandboxAdapter) DestroyEnv(ctx context.Context, forgeID, envID string) error {
	owner := sandboxdomain.Owner{
		Kind: sandboxdomain.OwnerKindForge,
		ID:   forgeID + "_" + envID,
	}
	return a.svc.Destroy(ctx, owner)
}

// versionDir returns the absolute path where main.py lives for a
// (forgeID, versionID) pair.
//
// versionDir 返 (forgeID, versionID) 对的 main.py 绝对路径。
func (a *SandboxAdapter) versionDir(forgeID, versionID string) string {
	return filepath.Join(a.forgeDataDir, "forges", forgeID, "versions", versionID)
}

// ── helpers (copied from v1 infra/sandbox/run.go and sync.go) ────────

// driverTemplate wraps the user function with a stdin → kwargs → stdout
// JSON shim so Run can pipe input/output without the user code knowing.
//
// driverTemplate 把用户函数包以 stdin → kwargs → stdout JSON shim，让 Run
// 不让用户代码知就能 pipe input/output。
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

// extractFuncName parses the first `def <name>` line from the code as a
// fallback when the caller didn't precompute via AST.
//
// extractFuncName 从代码第一个 `def <name>` 行解析名字，调用方未通过 AST
// 预计算时兜底。
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

// writeAtomic writes data via tmp + rename so readers never see a half-
// written file.
//
// writeAtomic 通过 tmp + rename 写文件——读取方永远看不到半成品。
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

package function

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	entitystreamapp "github.com/sunweilin/forgify/backend/internal/app/entitystream"
	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
)

// SandboxAdapter satisfies SandboxRunner by delegating spawn + cleanup to sandboxapp
// .Service and writing each version's main.py under the function data dir. Env
// materialization is NOT here — that goes through envfix.Provisioner (whose SandboxPort
// is sandboxapp.Service directly).
//
// SandboxAdapter 把 spawn + 清理委托 sandboxapp.Service、把每个版本 main.py 写到 function
// 数据根目录，满足 SandboxRunner。env 物化不在此——走 envfix.Provisioner（其 SandboxPort 直接
// 是 sandboxapp.Service）。
type SandboxAdapter struct {
	svc      *sandboxapp.Service
	dataDir  string
	entities streamdomain.Bridge // entities stream (SSE-C); nil → no entity-panel run terminal
}

// NewSandboxAdapter binds the adapter to a sandbox service + the function data root. entities (the
// entities stream Bridge, nil-tolerant) carries a run's live stderr to the function panel's terminal
// regardless of who triggered the run (chat / REST / workflow / sensor).
//
// NewSandboxAdapter 把 adapter 绑到 sandbox service + function 数据根。entities（entities 流 Bridge，允许
// nil）把一次运行的实时 stderr 送到 function 面板终端——不论谁触发（chat / REST / workflow / sensor）。
func NewSandboxAdapter(svc *sandboxapp.Service, dataDir string, entities streamdomain.Bridge) *SandboxAdapter {
	return &SandboxAdapter{svc: svc, dataDir: dataDir, entities: entities}
}

var _ SandboxRunner = (*SandboxAdapter)(nil)

func (a *SandboxAdapter) Ready() bool { return a.svc.IsReady() }

// Run writes main.py (code + stdin/stdout driver) and spawns it in owner's venv. A
// non-zero exit becomes ExecutionResult{OK:false}; an infra failure (incl. evicted env)
// a Go error.
//
// Run 写 main.py（代码 + stdin/stdout driver）并在 owner 的 venv 里 spawn。非零退出返
// ExecutionResult{OK:false}；基础设施失败（含被驱逐的 env）返 Go error。
func (a *SandboxAdapter) Run(ctx context.Context, owner sandboxdomain.Owner, functionID, versionID, code string, input map[string]any) (*functiondomain.ExecutionResult, error) {
	funcName := entryFuncName(code)
	if funcName == "" {
		return nil, fmt.Errorf("functionapp.SandboxAdapter.Run: no top-level def in code")
	}
	verDir := a.versionDir(functionID, versionID)
	if err := os.MkdirAll(verDir, 0o755); err != nil {
		return nil, fmt.Errorf("functionapp.SandboxAdapter.Run: mkdir: %w", err)
	}
	mainPy := filepath.Join(verDir, "main.py")
	if err := writeAtomic(mainPy, []byte(code+buildDriver(funcName)), 0o644); err != nil {
		return nil, fmt.Errorf("functionapp.SandboxAdapter.Run: write main.py: %w", err)
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("functionapp.SandboxAdapter.Run: marshal input: %w", err)
	}

	// Tee the function's own print() output (the driver routes it to stderr; the JSON result still
	// lands on clean stdout) to BOTH: the messages stream under the run_function tool_call (chat
	// view, ToolProgress) AND the entities stream's run terminal scoped to this function (panel
	// view, all callers). Both nil-safe.
	//
	// 把函数自己的 print() 输出（driver 引到 stderr；JSON 结果仍走干净 stdout）**双写**：messages 流 run_function
	// tool_call 下（对话视图，ToolProgress）+ entities 流锚到本 function 的 run 终端（面板视图，全 caller）。皆 nil 安全。
	prog := loopapp.ToolProgress(ctx)
	defer prog.Close()
	runTerm := entitystreamapp.New(ctx, a.entities, streamdomain.Scope{Kind: streamdomain.KindFunction, ID: functionID}, entitystreamapp.NodeRun, nil)
	res, spawnErr := a.svc.Spawn(ctx, owner, sandboxdomain.SpawnOpts{
		Cmd:       "python",
		Args:      []string{mainPy},
		Stdin:     inputJSON,
		StreamErr: io.MultiWriter(prog, runTerm),
	})
	if spawnErr != nil {
		runTerm.Close("error", nil)
		return nil, fmt.Errorf("functionapp.SandboxAdapter.Run: %w", spawnErr)
	}
	if res.Ok {
		runTerm.Close("completed", nil)
	} else {
		runTerm.Close("error", nil)
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
		output = strings.TrimSpace(string(res.Stdout)) // non-JSON stdout → return as string
	}
	out.OK = true
	out.Output = output
	return out, nil
}

// Destroy removes every env owned by the function and its on-disk code dir.
//
// Destroy 删除 function 拥有的所有 env 与盘上代码目录。
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
		if err := a.svc.Destroy(ctx, sandboxdomain.Owner{Kind: sandboxdomain.OwnerKindFunction, ID: e.OwnerID}); err != nil {
			return fmt.Errorf("functionapp.SandboxAdapter.Destroy %s: %w", e.OwnerID, err)
		}
	}
	if err := os.RemoveAll(filepath.Join(a.dataDir, "functions", functionID)); err != nil {
		return fmt.Errorf("functionapp.SandboxAdapter.Destroy: rm code dir: %w", err)
	}
	return nil
}

func (a *SandboxAdapter) versionDir(functionID, versionID string) string {
	return filepath.Join(a.dataDir, "functions", functionID, "versions", versionID)
}

// driverTemplate redirects the function's stdout to stderr for the duration of the call, then
// prints the JSON result to the real stdout. This keeps stdout a clean single JSON document (so
// res.Stdout parses) AND routes the function's own print()s to stderr — which the tool layer
// streams live as progress under the run_function tool_call. (Before this, a print() corrupted the
// result by interleaving on stdout.)
//
// driverTemplate 在调用期间把函数 stdout 重定向到 stderr，再把 JSON 结果打到真正的 stdout。这既让
// stdout 保持单一干净 JSON（res.Stdout 可解析），又把函数自己的 print() 引到 stderr——工具层将其作为
// run_function tool_call 下的实时进度流出。（此前 print() 会在 stdout 上交错、破坏结果。）
const driverTemplate = `

if __name__ == "__main__":
    import json as _json, sys as _sys
    _input = _json.load(_sys.stdin)
    _real_stdout = _sys.stdout
    _sys.stdout = _sys.stderr
    try:
        _result = {FUNC_NAME}(**_input)
    finally:
        _sys.stdout = _real_stdout
    print(_json.dumps(_result))
`

func buildDriver(funcName string) string {
	return strings.Replace(driverTemplate, "{FUNC_NAME}", funcName, 1)
}

// writeAtomic writes via a unique temp file + rename so concurrent writers never collide.
//
// writeAtomic 经唯一临时文件 + rename 写入，并发写不撞。
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

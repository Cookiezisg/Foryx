// run.go: Sandbox.Run executes a forge in its EnvID's venv via `uv run
// --no-sync`. Each invocation writes (or overwrites) the per-version
// main.py from req.Code with a small `if __name__ == "__main__"` driver
// appended, pipes the JSON-encoded input to stdin, and returns
// ExecutionResult parsed from stdout.
//
// Forge convention: the user's Python code defines exactly one function;
// the driver reads JSON from stdin, calls that function with the JSON keys
// as kwargs, and prints the JSON-serialized return value to stdout. A
// non-JSON return falls back to a raw string output. A Python exception
// makes the subprocess exit non-zero, captured into ErrorMsg with ok=false.
//
// No timeout is enforced here — the sandbox does not arbitrate forge
// runtime. ctx-cancel kills the entire process tree (including any
// subprocess Python forks) via setupProcessGroup + killProcessGroup +
// Cmd.Cancel callback.
//
// run.go：Sandbox.Run 通过 `uv run --no-sync` 在 EnvID 的 venv 中执行
// forge。每次调用从 req.Code 写（或覆盖）per-version main.py，附加 driver
// 块（if __name__ == "__main__"），通过 stdin 喂 JSON 编码的 input，
// 解析 stdout 成 ExecutionResult。
//
// Forge 约定：用户 Python 代码定义恰好一个函数；driver 从 stdin 读 JSON，
// 把 JSON 键当 kwargs 调函数，把 JSON 序列化的返回值打印到 stdout。
// 非 JSON 返回退回 raw 字符串。Python 异常让子进程非零 exit，stderr 进
// ErrorMsg，ok=false。
//
// 沙箱不强加 timeout——上游 ctx 决定运行时长。ctx-cancel 通过
// setupProcessGroup + killProcessGroup + Cmd.Cancel 杀整个进程树。

package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
)

// RunRequest is one execute-this-forge order.
//
// RunRequest 是一份"执行这个 forge"的指令。
type RunRequest struct {
	ForgeID       string
	VersionID     string
	EnvID         string
	Code          string
	EntryFunction string // optional; sandbox falls back to first `def` if empty
	Input         map[string]any
}

// Run executes the forge code in its venv. The Python subprocess receives
// the JSON-encoded input on stdin and prints the JSON-encoded result to
// stdout. ctx-cancel kills the whole tree.
//
// Run 在 forge 的 venv 中执行代码。Python 子进程通过 stdin 拿 JSON 编码的
// input，把 JSON 编码的结果写到 stdout。ctx-cancel 杀整个进程树。
func (s *Sandbox) Run(ctx context.Context, req RunRequest) (*forgedomain.ExecutionResult, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}

	envD := envDir(s.cfg.DataDir, req.ForgeID, req.EnvID)
	verD := versionDir(s.cfg.DataDir, req.ForgeID, req.VersionID)

	funcName := req.EntryFunction
	if funcName == "" {
		var err error
		funcName, err = extractFuncName(req.Code)
		if err != nil {
			return nil, fmt.Errorf("sandbox.Run: %w", err)
		}
	}
	fullCode := req.Code + buildDriver(funcName)

	if err := os.MkdirAll(verD, 0o755); err != nil {
		return nil, fmt.Errorf("sandbox.Run: mkdir version dir: %w", err)
	}
	if err := writeAtomic(filepath.Join(verD, "main.py"), []byte(fullCode), 0o644); err != nil {
		return nil, fmt.Errorf("sandbox.Run: write main.py: %w", err)
	}

	inputJSON, err := json.Marshal(req.Input)
	if err != nil {
		return nil, fmt.Errorf("sandbox.Run: marshal input: %w", err)
	}

	cmd := exec.CommandContext(ctx, s.UVPath(),
		"run",
		"--project", envD,
		"--no-sync",
		"python", filepath.Join(verD, "main.py"),
	)
	cmd.Env = s.withUVEnv()
	cmd.Stdin = bytes.NewReader(inputJSON)

	setupProcessGroup(cmd)
	cmd.Cancel = func() error { return killProcessGroup(cmd) }

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	start := time.Now()
	stdout, runErr := cmd.Output()
	elapsed := time.Since(start).Milliseconds()

	if runErr != nil {
		// subprocess failed — venv missing / Python exception / ctx
		// cancel / etc. stderr has the gory details for the LLM via
		// tool_result. We deliberately do NOT do "venv missing → trigger
		// resync" self-healing here — that's punted to the LLM
		// (sandbox iter §11.1).
		//
		// 子进程失败——venv 不存在 / Python 异常 / ctx 取消等。stderr 有
		// 详细信息供 LLM via tool_result。**不**做"venv 不存在 → 触发 resync"
		// 自愈——punt 给 LLM（沙箱迭代 §11.1）。
		msg := stderr.String()
		if msg == "" {
			msg = runErr.Error()
		}
		return &forgedomain.ExecutionResult{
			OK:        false,
			ErrorMsg:  strings.TrimSpace(msg),
			ElapsedMs: elapsed,
		}, nil
	}

	// Forge printed something to stdout. Forge convention is JSON; if it's
	// not parseable JSON, fall back to the raw trimmed string so the LLM
	// still sees output (e.g. forge that prints debug text by mistake).
	//
	// Forge 在 stdout 输出了内容。约定是 JSON；不是合法 JSON 时退回 raw
	// 字符串，让 LLM 仍能看到（如 forge 错误打印 debug 文本）。
	var output any
	if err := json.Unmarshal(stdout, &output); err != nil {
		output = strings.TrimSpace(string(stdout))
	}

	return &forgedomain.ExecutionResult{
		OK:        true,
		Output:    output,
		ElapsedMs: elapsed,
	}, nil
}

// WriteCodeFile writes (or overwrites) main.py for a version without
// touching its venv. Service layer calls this when EnvID is unchanged but
// code changed (deps not modified, only logic), so we skip the expensive
// uv sync entirely.
//
// WriteCodeFile 写（或覆盖）某 version 的 main.py，不动 venv。service 层
// 在 EnvID 没变只代码变了时调（deps 没改，只改逻辑），跳过昂贵的 uv sync。
func (s *Sandbox) WriteCodeFile(ctx context.Context, forgeID, versionID, code, entryFunction string) error {
	if err := s.ensureReady(); err != nil {
		return err
	}

	funcName := entryFunction
	if funcName == "" {
		var err error
		funcName, err = extractFuncName(code)
		if err != nil {
			return fmt.Errorf("sandbox.WriteCodeFile: %w", err)
		}
	}

	fullCode := code + buildDriver(funcName)
	verD := versionDir(s.cfg.DataDir, forgeID, versionID)
	if err := os.MkdirAll(verD, 0o755); err != nil {
		return fmt.Errorf("sandbox.WriteCodeFile: mkdir: %w", err)
	}
	return writeAtomic(filepath.Join(verD, "main.py"), []byte(fullCode), 0o644)
}

// driverTemplate is appended to user code to bridge stdin → function →
// stdout. {FUNC_NAME} is replaced at runtime with the parsed function name.
//
// driverTemplate 追加到用户代码末尾，桥接 stdin → 函数 → stdout。
// {FUNC_NAME} 运行时替换为解析出的函数名。
const driverTemplate = `

if __name__ == "__main__":
    import json as _json, sys as _sys
    _input = _json.load(_sys.stdin)
    _result = {FUNC_NAME}(**_input)
    print(_json.dumps(_result))
`

// buildDriver returns the driver block with funcName substituted in.
//
// buildDriver 返回替换 funcName 后的 driver 块。
func buildDriver(funcName string) string {
	return strings.Replace(driverTemplate, "{FUNC_NAME}", funcName, 1)
}

// extractFuncName parses the first `def <name>` line from user code.
// Used as fallback when RunRequest.EntryFunction is empty (caller didn't
// pre-parse via AST). The forgeapp layer should normally pass a precise
// name from ASTParser instead.
//
// extractFuncName 从用户代码中解析第一个 `def <name>` 行。
// RunRequest.EntryFunction 为空时兜底；forgeapp 层正常情况应从 ASTParser
// 拿到精确名传入。
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

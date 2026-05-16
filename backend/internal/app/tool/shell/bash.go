package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	installprogresspkg "github.com/sunweilin/forgify/backend/internal/pkg/installprogress"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)


const (
	defaultTimeoutMS = 120_000
	maxTimeoutMS     = 600_000
	outputCapBytes   = 256 * 1024
)

var (
	// ErrEmptyCommand: command missing or empty.
	//
	// ErrEmptyCommand：command 缺失或为空。
	ErrEmptyCommand = errors.New("command is required and must be non-empty")

	// ErrInvalidTimeout: timeout outside [0, maxTimeoutMS].
	//
	// ErrInvalidTimeout：timeout 不在 [0, maxTimeoutMS]。
	ErrInvalidTimeout = fmt.Errorf("timeout must be between 0 and %d ms", maxTimeoutMS)
)


const bashDescription = `Run a shell command on the user's machine.

Usage:
- ` + "`command`" + ` is the shell command. macOS/Linux: ` + "`/bin/sh -c`" + `; Windows: ` + "`cmd.exe /c`" + `. Examples: "ls -la", "git status", "go test ./...".
- ` + "`run_in_background: true`" + ` spawns without waiting and returns a bash_id; poll with BashOutput, terminate with KillShell.
- ` + "`timeout`" + ` (ms, foreground only) defaults to 120000; max 600000. Longer tasks should use background mode.
- The conversation has a tracked working directory: ` + "`cd <path>`" + ` as the entire command updates it; chained ` + "`cd ... && ...`" + ` does not (matches subshell semantics).
- Combined stdout+stderr is returned, capped at 256 KB. Exit code appears in a status footer.

Sandbox auto-routing (Python and Node only):
- Commands invoking ` + "`python`" + `, ` + "`pip`" + `, ` + "`uv`" + `, ` + "`virtualenv`" + `, ` + "`pipenv`" + `, ` + "`poetry`" + `, ` + "`node`" + `, ` + "`npm`" + `, ` + "`npx`" + `, ` + "`yarn`" + `, ` + "`pnpm`" + ` execute inside a per-conversation isolated environment. Detection covers nested forms (` + "`bash -c \"pip install ...\"`" + `, ` + "`env VAR=val python ...`" + `, path-prefixed binaries, ` + "`cd && python`" + ` chains, ` + "`which python3`" + `).
- Other languages run on the host. The router cannot see through ` + "`eval`" + `, ` + "`source`" + `, or dynamic ` + "`$(...)`" + `; write runtime commands directly.`

var bashSchema = json.RawMessage(`{
	"type": "object",
	"required": ["command"],
	"properties": {
		"command": {
			"type": "string",
			"description": "Shell command to execute (POSIX sh)."
		},
		"run_in_background": {
			"type": "boolean",
			"default": false,
			"description": "If true, spawn without waiting and return a bash_id for BashOutput / KillShell."
		},
		"timeout": {
			"type": "number",
			"description": "Foreground timeout in milliseconds (default 120000, hard max 600000). Ignored in background mode."
		}
	}
}`)


type bashArgs struct {
	Command    string `json:"command"`
	Background bool   `json:"run_in_background"`
	Timeout    int    `json:"timeout"`
}


// Bash implements the Bash system tool; mgr is shared with BashOutput + KillShell, sandbox is optional auto-route.
//
// Bash 是 Bash 系统工具的实现；mgr 与 BashOutput + KillShell 共享，sandbox 是可选自动路由目标。
type Bash struct {
	mgr     *ProcessManager
	sandbox *sandboxapp.Service
}

func (t *Bash) Name() string                { return "Bash" }
func (t *Bash) Description() string         { return bashDescription }
func (t *Bash) Parameters() json.RawMessage { return bashSchema }

func (t *Bash) IsReadOnly() bool        { return false }
func (t *Bash) NeedsReadFirst() bool    { return false }
func (t *Bash) RequiresWorkspace() bool { return false }

// ValidateInput rejects empty commands and out-of-range timeouts.
//
// ValidateInput 拒绝空命令和越界 timeout。
func (t *Bash) ValidateInput(args json.RawMessage) error {
	var a bashArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("Bash.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.Command) == "" {
		return ErrEmptyCommand
	}
	if a.Timeout < 0 || a.Timeout > maxTimeoutMS {
		return ErrInvalidTimeout
	}
	return nil
}

func (t *Bash) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}


// Execute dispatches cd-only / background / foreground branches.
//
// Execute 分派 cd-only / 后台 / 前台分支。
func (t *Bash) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args bashArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("Bash.Execute: %w", err)
	}
	cmdText := strings.TrimSpace(args.Command)

	if target, ok := parseCDOnly(cmdText); ok {
		return t.handleCD(ctx, target)
	}

	cwd := resolveCwd(ctx)

	// Sandbox prep failures must surface to LLM; falling through to system shell would feed it stale facts (§S3).
	// sandbox 准备失败必须抛给 LLM；落到系统 shell 会让 LLM 拿到假事实（违 §S3）。
	extraPath, autoRouteErr := t.maybeAutoRoute(ctx, cmdText)
	if autoRouteErr != nil {
		return formatAutoRouteError(autoRouteErr), nil
	}

	if args.Background {
		return t.runBackground(ctx, cmdText, cwd, extraPath)
	}

	timeoutMS := args.Timeout
	if timeoutMS == 0 {
		timeoutMS = defaultTimeoutMS
	}
	return t.runForeground(ctx, cmdText, cwd, extraPath, time.Duration(timeoutMS)*time.Millisecond)
}

// formatAutoRouteError turns a sandbox-prep failure into a tool result with exit -1 and a clear footer.
//
// formatAutoRouteError 把 sandbox 准备失败转成 exit -1 + 明确 footer 的 tool result。
func formatAutoRouteError(err error) string {
	body := err.Error()
	return formatForegroundResult(body, -1, "sandbox auto-route failed")
}

// maybeAutoRoute returns PATH-prepend dirs for runtime commands; nil/nil for non-runtime; nil/err when sandbox unavailable.
//
// maybeAutoRoute 返 runtime 命令的 PATH 前置目录；非 runtime 返 nil/nil；sandbox 不可用返 nil/err。
func (t *Bash) maybeAutoRoute(ctx context.Context, command string) ([]string, error) {
	kind := detectRuntime(command)
	if kind == "" {
		return nil, nil
	}
	if t.sandbox == nil {
		return nil, fmt.Errorf("sandbox unavailable for %s: runtime not configured", kind)
	}
	if !t.sandbox.IsReady() {
		bootErr := t.sandbox.BootstrapError()
		if bootErr != nil {
			return nil, fmt.Errorf("sandbox unavailable for %s: bootstrap failed: %s", kind, bootErr.Error())
		}
		return nil, fmt.Errorf("sandbox unavailable for %s: bootstrap incomplete", kind)
	}
	convID, ok := reqctxpkg.GetConversationID(ctx)
	if !ok || convID == "" {
		return nil, fmt.Errorf("sandbox unavailable for %s: no conversation context", kind)
	}
	// owner.ID uses "_" (not ":") to avoid POSIX PATH segment split since the ID becomes a literal dir name.
	// owner.ID 用 "_" 而非 ":" 防 POSIX PATH 分割（ID 直接当目录名）。
	owner := sandboxdomain.Owner{
		Kind: sandboxdomain.OwnerKindConversation,
		ID:   convID + "_" + kind,
		Name: fmt.Sprintf("Conv %s scratch (%s)", convID, kind),
	}
	// Wrap with installprogresspkg.Run so install progress streams under the in-flight Bash tool_call.
	// 用 installprogresspkg.Run 包，让装包进度挂在当前 Bash tool_call 父下。
	env, err := installprogresspkg.Run(ctx,
		map[string]any{
			"stage":   "preparing-runtime",
			"runtime": kind,
			"owner":   owner.ID,
		},
		func(progress sandboxdomain.ProgressFunc) (*sandboxdomain.Env, error) {
			return t.sandbox.EnsureEnv(ctx, owner, sandboxdomain.EnvSpec{
				Runtime: sandboxdomain.RuntimeSpec{Kind: kind},
			}, progress)
		})
	if err != nil {
		return nil, fmt.Errorf("sandbox unavailable for %s: env install failed: %s", kind, err.Error())
	}
	envPath := filepath.Join(t.sandbox.SandboxRoot(), env.Path)
	return envBinDirsForKind(envPath, kind), nil
}


// parseCDOnly returns (target, true) for `cd` or `cd <path>` only; target empty means home.
//
// parseCDOnly 仅识别 `cd` 或 `cd <path>`；target 为空意味着 home。
func parseCDOnly(cmd string) (string, bool) {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "cd" {
		return "", true
	}
	if !strings.HasPrefix(trimmed, "cd ") && !strings.HasPrefix(trimmed, "cd\t") {
		return "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "cd"))
	if strings.ContainsAny(rest, "&|;<>`$") {
		return "", false
	}
	if len(rest) >= 2 && (rest[0] == '"' || rest[0] == '\'') && rest[0] == rest[len(rest)-1] {
		rest = rest[1 : len(rest)-1]
	}
	return rest, true
}

// handleCD validates the target dir, updates AgentState, and returns confirmation.
//
// handleCD 校验目标目录 / 更新 AgentState / 返确认。
func (t *Bash) handleCD(ctx context.Context, target string) (string, error) {
	if target == "" {
		if h, err := os.UserHomeDir(); err == nil {
			target = h
		} else {
			return "Cannot resolve home directory: " + err.Error(), nil
		}
	}
	current := resolveCwd(ctx)
	if !filepath.IsAbs(target) {
		target = filepath.Join(current, target)
	}
	target = filepath.Clean(target)

	info, err := os.Stat(target)
	if err != nil {
		return fmt.Sprintf("cd: %s: %v", target, err), nil
	}
	if !info.IsDir() {
		return fmt.Sprintf("cd: not a directory: %s", target), nil
	}

	state, ok := reqctxpkg.GetAgentState(ctx)
	if !ok {
		return "cd: agent state missing — cwd not persisted across calls. Subsequent commands will use the process default cwd.", nil
	}
	state.SetCwd(target)
	return "Changed working directory to " + target, nil
}

// resolveCwd returns conversation cwd; falls back to process cwd when AgentState is missing.
//
// resolveCwd 返对话级 cwd；AgentState 缺失时回落到进程 cwd。
func resolveCwd(ctx context.Context) string {
	if state, ok := reqctxpkg.GetAgentState(ctx); ok {
		if c := state.Cwd(); c != "" {
			return c
		}
	}
	if c, err := os.Getwd(); err == nil {
		return c
	}
	return "/"
}


// runForeground execs command with a wall-clock timeout and returns formatted combined stdout+stderr (capped).
//
// runForeground 带墙钟超时执行 command，返合并 stdout+stderr（截断）。
// 返格式化结果。
func (t *Bash) runForeground(ctx context.Context, command, cwd string, extraPath []string, timeout time.Duration) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := buildShellCmd(runCtx, command, cwd, extraPath)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	output := capOutput(buf.Bytes())

	switch {
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		return formatForegroundResult(output, -1, fmt.Sprintf("command timed out after %s", timeout)), nil
	case errors.Is(runCtx.Err(), context.Canceled):
		// Parent ctx cancel branch — without it Go would report SIGKILL as "exec failed: signal: killed".
		// 父 ctx 取消分支——不加这支会报 SIGKILL 为 "exec failed: signal: killed" 误导 LLM。
		return formatForegroundResult(output, -1, "cancelled"), nil
	case err != nil:
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return formatForegroundResult(output, exitErr.ExitCode(), ""), nil
		}
		return formatForegroundResult(output, -1, "exec failed: "+err.Error()), nil
	}
	return formatForegroundResult(output, 0, ""), nil
}

// formatForegroundResult assembles body + status footer; footer always included for clarity.
//
// formatForegroundResult 拼正文 + 状态 footer；footer 始终带，避免歧义。
func formatForegroundResult(output string, exitCode int, note string) string {
	var sb strings.Builder
	sb.WriteString(output)
	if !strings.HasSuffix(output, "\n") {
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	if note != "" {
		fmt.Fprintf(&sb, "[%s]\n", note)
	}
	fmt.Fprintf(&sb, "[exit code: %d]", exitCode)
	return sb.String()
}

// capOutput trims to outputCapBytes and annotates the truncation count.
//
// capOutput 截到 outputCapBytes 并标注截断字节数。
func capOutput(b []byte) string {
	if len(b) <= outputCapBytes {
		return string(b)
	}
	dropped := len(b) - outputCapBytes
	return fmt.Sprintf("...[truncated %d bytes from start]\n", dropped) + string(b[len(b)-outputCapBytes:])
}


// runBackground starts the command detached so it outlives the chat turn; reaped via KillShell or shutdown Stop().
//
// runBackground 用 detached ctx 启动，让子进程 outlive 单次 chat turn；清理走 KillShell 或关停 Stop()。
func (t *Bash) runBackground(ctx context.Context, command, cwd string, extraPath []string) (string, error) {
	convID, _ := reqctxpkg.GetConversationID(ctx)
	cmd := buildShellCmd(context.Background(), command, cwd, extraPath)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Sprintf("Failed to open stdout pipe: %v", err), nil
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Sprintf("Failed to open stderr pipe: %v", err), nil
	}

	if err := cmd.Start(); err != nil {
		return fmt.Sprintf("Failed to start background command: %v", err), nil
	}

	proc := &BgProcess{
		ConvID:    convID,
		Command:   command,
		Cmd:       cmd,
		StartedAt: time.Now(),
		status:    StatusRunning,
	}
	t.mgr.Register(proc)

	// Concurrent pumps prevent deadlock from a noisy process filling one pipe.
	// 并发 pump 防一根管满死锁。
	var pumpWG sync.WaitGroup
	pumpWG.Add(2)
	go pumpReader(&pumpWG, proc, stdout)
	go pumpReader(&pumpWG, proc, stderr)

	go func() {
		pumpWG.Wait()
		err := cmd.Wait()
		switch {
		case err == nil:
			proc.markFinished(StatusExited, 0)
		default:
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				if exitErr.ProcessState != nil && exitErr.ProcessState.Exited() {
					proc.markFinished(StatusExited, exitErr.ExitCode())
				} else {
					proc.markFinished(StatusKilled, -1)
				}
			} else {
				proc.markErrored(err)
			}
		}
	}()

	return fmt.Sprintf(
		"Started background command (bash_id=%s): %s\nUse BashOutput with this bash_id to poll new output, or KillShell to terminate.",
		proc.ID, command,
	), nil
}

func pumpReader(wg *sync.WaitGroup, proc *BgProcess, r io.Reader) {
	defer wg.Done()
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			proc.appendOutput(buf[:n])
		}
		if err != nil {
			return
		}
	}
}


// buildShellCmd builds *exec.Cmd; Unix uses /bin/sh -c, Windows uses cmd.exe /c (not PowerShell).
//
// buildShellCmd 构造 *exec.Cmd；Unix 用 /bin/sh -c，Windows 用 cmd.exe /c（不用 PowerShell）。
func buildShellCmd(ctx context.Context, command, cwd string, extraPath []string) *exec.Cmd {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	}
	cmd.Dir = cwd
	cmd.Env = prependPath(os.Environ(), extraPath)
	return cmd
}


var _ toolapp.Tool = (*Bash)(nil)

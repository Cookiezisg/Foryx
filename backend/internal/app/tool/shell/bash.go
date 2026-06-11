package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	errorspkg "github.com/sunweilin/forgify/backend/internal/pkg/errors"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
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
	ErrEmptyCommand = errorspkg.New(errorspkg.KindInvalid, "SHELL_EMPTY_COMMAND", "command is required and must be non-empty")

	// ErrInvalidTimeout: timeout outside [0, maxTimeoutMS].
	//
	// ErrInvalidTimeout：timeout 不在 [0, maxTimeoutMS]。
	ErrInvalidTimeout = errorspkg.New(errorspkg.KindInvalid, "SHELL_INVALID_TIMEOUT", fmt.Sprintf("timeout must be between 0 and %d ms", maxTimeoutMS))
)

const bashDescription = `Run a shell command (POSIX sh on Unix, cmd.exe /c on Windows). Output is combined stdout+stderr, capped at 256KB, with an exit-code footer. There is no persistent working directory — pass absolute paths, or prefix a single command with "cd /abs/dir && ..." (cd does NOT carry across calls). Set run_in_background for long-running commands, then poll with BashOutput and stop with KillShell.`

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

// Bash implements the Bash system tool; mgr is shared with BashOutput + KillShell.
//
// Bash 是 Bash 系统工具的实现；mgr 与 BashOutput + KillShell 共享。
type Bash struct{ mgr *ProcessManager }

func (t *Bash) Name() string                { return "Bash" }
func (t *Bash) Description() string         { return bashDescription }
func (t *Bash) Parameters() json.RawMessage { return bashSchema }

// ValidateInput rejects empty commands and out-of-range timeouts.
//
// ValidateInput 拒绝空命令和越界 timeout。
func (t *Bash) ValidateInput(args json.RawMessage) error {
	var a bashArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("Bash: bad args: %w", err)
	}
	if strings.TrimSpace(a.Command) == "" {
		return ErrEmptyCommand
	}
	if a.Timeout < 0 || a.Timeout > maxTimeoutMS {
		return ErrInvalidTimeout
	}
	return nil
}

// Execute hard-blocks catastrophic commands, then dispatches background / foreground.
//
// Execute 先硬拦截灾难命令，再分派后台 / 前台。
func (t *Bash) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args bashArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("Bash: %w", err)
	}
	cmdText := strings.TrimSpace(args.Command)

	if reason, blocked := checkDangerous(cmdText); blocked {
		return formatForegroundResult("", -1, "blocked: "+reason+" (refused; rephrase if intentional)"), nil
	}

	if args.Background {
		return t.runBackground(cmdText)
	}

	timeoutMS := args.Timeout
	if timeoutMS == 0 {
		timeoutMS = defaultTimeoutMS
	}
	return t.runForeground(ctx, cmdText, time.Duration(timeoutMS)*time.Millisecond)
}

// runForeground execs command with a wall-clock timeout and returns formatted combined
// stdout+stderr (capped). No cwd — the child inherits the backend process's directory.
//
// runForeground 带墙钟超时执行 command，返合并 stdout+stderr（截断）。无 cwd——子进程继承
// 后端进程的目录。
func (t *Bash) runForeground(ctx context.Context, command string, timeout time.Duration) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := buildShellCmd(runCtx, command)

	// Tee combined output to BOTH the result buffer (the final tool_result the LLM reads) AND a
	// live progress stream, so the user watches stdout/stderr scroll in real time under the Bash
	// tool_call. ToolProgress is nil-safe: off a streamed chat turn (REST / tests) it is a no-op
	// and only buf is written.
	//
	// 把合并输出**同时**写结果 buf（LLM 读的最终 tool_result）+ 实时 progress 流，使用户在 Bash
	// tool_call 下实时看 stdout/stderr 滚动。ToolProgress nil 安全：不在流式 chat turn（REST/测试）则
	// no-op、只写 buf。
	prog := loopapp.ToolProgress(ctx)
	defer prog.Close()
	var buf bytes.Buffer
	w := io.MultiWriter(&buf, prog)
	cmd.Stdout = w
	cmd.Stderr = w

	err := cmd.Run()
	output := capOutput(buf.Bytes())

	switch {
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		return formatForegroundResult(output, -1, fmt.Sprintf("command timed out after %s", timeout)), nil
	case errors.Is(runCtx.Err(), context.Canceled):
		// Parent ctx cancel — without this branch Go reports SIGKILL as "exec failed: signal: killed".
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

// capOutput trims to outputCapBytes (keeping the tail) and annotates the dropped count.
//
// capOutput 截到 outputCapBytes（保留尾部）并标注丢弃字节数。
func capOutput(b []byte) string {
	if len(b) <= outputCapBytes {
		return string(b)
	}
	dropped := len(b) - outputCapBytes
	return fmt.Sprintf("...[truncated %d bytes from start]\n", dropped) + string(b[len(b)-outputCapBytes:])
}

// runBackground starts the command detached so it outlives the chat turn; reaped via
// KillShell or shutdown Stop().
//
// runBackground 用 detached ctx 启动，让子进程 outlive 单次 chat turn；清理走 KillShell 或
// 关停 Stop()。
func (t *Bash) runBackground(command string) (string, error) {
	cmd := buildShellCmd(context.Background(), command)

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

// buildShellCmd builds *exec.Cmd; Unix uses /bin/sh -c, Windows uses cmd.exe /c (not
// PowerShell). No Dir is set (no cwd) and Env is inherited from the backend process.
//
// buildShellCmd 构造 *exec.Cmd；Unix 用 /bin/sh -c，Windows 用 cmd.exe /c（不用 PowerShell）。
// 不设 Dir（无 cwd），Env 继承后端进程。
func buildShellCmd(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/c", command)
	}
	return exec.CommandContext(ctx, "/bin/sh", "-c", command)
}

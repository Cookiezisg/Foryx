package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

// checkBinaryExists guards against env corruption — a dangling symlink left
// behind by a mise runtime upgrade (mise relocates runtimes/python/3.12.5 to
// 3.12.6 and the venv's python link goes dangling). os.Stat follows symlinks, so
// a dangling link returns ENOENT here; returning ErrEnvNotFound lets the app
// layer lazily rebuild the env and retry once. Non-absolute commands rely on
// PATH resolution — exec handles those.
//
// checkBinaryExists 防 env 腐坏——mise 升级后留下的 dangling symlink（mise 把
// runtimes/python/3.12.5 迁到 3.12.6，venv 里的 python link 悬空）。os.Stat 跟 link，
// dangling 返 ENOENT；返 ErrEnvNotFound 让 app 层懒重建 env 并重试一次。非绝对路径走
// PATH 解析，由 exec 处理。
func checkBinaryExists(cmd string) error {
	if cmd == "" || !filepath.IsAbs(cmd) {
		return nil
	}
	if _, err := os.Stat(cmd); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("sandbox: binary missing at %s (dangling symlink? runtime relocated?): %w",
				cmd, sandboxdomain.ErrEnvNotFound)
		}
		return fmt.Errorf("sandbox: stat binary %s: %v: %w", cmd, err, sandboxdomain.ErrSpawnFailed)
	}
	return nil
}

// isBareCommand reports whether cmd is a plain binary name (no path separators or
// leading ~ / .), which an EnvManager should resolve inside the env; a path-like
// cmd runs as-is.
//
// isBareCommand 报告 cmd 是否为裸 binary 名（无路径分隔符、无前缀 ~ / .），应由
// EnvManager 在 env 内解析；路径式 cmd 原样运行。
func isBareCommand(cmd string) bool {
	if cmd == "" {
		return false
	}
	return !filepath.IsAbs(cmd) &&
		!strings.ContainsAny(cmd, `/\`) &&
		!strings.HasPrefix(cmd, "~") &&
		!strings.HasPrefix(cmd, ".")
}

// SpawnOptions carries fully resolved inputs to SpawnOnce / SpawnLongLived.
//
// SpawnOptions 是 SpawnOnce / SpawnLongLived 的全解析入参。
type SpawnOptions struct {
	Cmd   string
	Args  []string
	Cwd   string
	Env   []string
	Stdin []byte
	// StreamErr (optional) receives stderr live as it is produced, in addition to the captured
	// buffer — the seam a tool uses to stream a child's progress output. nil = capture only.
	//
	// StreamErr（可选）在捕获 buffer 之外实时接收 stderr——工具据此流式推子进程进度输出的接缝。nil = 仅捕获。
	StreamErr io.Writer
}

// SpawnOnce runs cmd to completion; a non-zero exit → Ok=false, an infra failure
// → Go error.
//
// SpawnOnce 跑命令到结束；非零退出返 Ok=false，基础设施失败才返 Go error。
func SpawnOnce(ctx context.Context, opts SpawnOptions) (*sandboxdomain.ExecutionResult, error) {
	if err := checkBinaryExists(opts.Cmd); err != nil {
		return nil, fmt.Errorf("sandbox.SpawnOnce: %w", err)
	}
	cmd := exec.CommandContext(ctx, opts.Cmd, opts.Args...)
	cmd.Dir = opts.Cwd
	if len(opts.Env) > 0 {
		cmd.Env = opts.Env
	}
	if len(opts.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(opts.Stdin)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	// Tee stderr to the live sink (if any) so a tool can stream the child's progress output while
	// still capturing the full buffer for the result.
	//
	// 把 stderr 同时喂实时 sink（若有），使工具能流式推子进程进度输出，同时仍捕获完整 buffer 供结果用。
	if opts.StreamErr != nil {
		cmd.Stderr = io.MultiWriter(&stderr, opts.StreamErr)
	} else {
		cmd.Stderr = &stderr
	}

	setupProcessGroup(cmd)
	cmd.Cancel = func() error { return killProcessGroup(cmd) }

	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)

	result := &sandboxdomain.ExecutionResult{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		Duration: elapsed,
	}

	if runErr == nil {
		result.Ok = true
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		result.Ok = false
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}

	if errors.Is(runErr, context.DeadlineExceeded) {
		return result, fmt.Errorf("sandbox.SpawnOnce: %w", sandboxdomain.ErrSpawnTimeout)
	}
	return result, fmt.Errorf("sandbox.SpawnOnce: %w (cause: %w)", sandboxdomain.ErrSpawnFailed, runErr)
}

// SpawnLongLived starts cmd, wires stdio pipes, and returns a handle; the caller
// must Wait or Kill.
//
// SpawnLongLived 启动命令并布 stdio 管道，返 handle；调用方必须 Wait 或 Kill。
func SpawnLongLived(ctx context.Context, opts SpawnOptions) (sandboxdomain.LongLivedHandle, error) {
	if err := checkBinaryExists(opts.Cmd); err != nil {
		return nil, fmt.Errorf("sandbox.SpawnLongLived: %w", err)
	}
	cmd := exec.CommandContext(ctx, opts.Cmd, opts.Args...)
	cmd.Dir = opts.Cwd
	if len(opts.Env) > 0 {
		cmd.Env = opts.Env
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("sandbox.SpawnLongLived: stdin pipe: %w (spawn: %w)", err, sandboxdomain.ErrSpawnFailed)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("sandbox.SpawnLongLived: stdout pipe: %w (spawn: %w)", err, sandboxdomain.ErrSpawnFailed)
	}
	stderrR, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("sandbox.SpawnLongLived: stderr pipe: %w (spawn: %w)", err, sandboxdomain.ErrSpawnFailed)
	}

	setupProcessGroup(cmd)
	cmd.Cancel = func() error { return killProcessGroup(cmd) }

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderrR.Close()
		return nil, fmt.Errorf("sandbox.SpawnLongLived: start: %w (spawn: %w)", err, sandboxdomain.ErrSpawnFailed)
	}

	return &longLivedHandle{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderrR,
	}, nil
}

type longLivedHandle struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
}

func (h *longLivedHandle) Stdin() io.WriteCloser { return h.stdin }
func (h *longLivedHandle) Stdout() io.ReadCloser { return h.stdout }
func (h *longLivedHandle) Stderr() io.ReadCloser { return h.stderr }

func (h *longLivedHandle) Wait() error { return h.cmd.Wait() }

func (h *longLivedHandle) Kill() error { return killProcessGroup(h.cmd) }

func (h *longLivedHandle) PID() int {
	if h.cmd.Process == nil {
		return 0
	}
	return h.cmd.Process.Pid
}

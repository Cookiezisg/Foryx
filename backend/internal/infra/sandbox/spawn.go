package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

// SpawnOptions carries fully resolved inputs to SpawnOnce / SpawnLongLived.
//
// SpawnOptions 是 SpawnOnce / SpawnLongLived 的全解析入参。
type SpawnOptions struct {
	Cmd   string
	Args  []string
	Cwd   string
	Env   []string
	Stdin []byte
}

// SpawnOnce runs cmd to completion; non-zero exit → Ok=false, infra failure → Go error.
//
// SpawnOnce 跑命令到结束；非零退出返 Ok=false，基础设施失败才返 Go error。
func SpawnOnce(ctx context.Context, opts SpawnOptions) (*sandboxdomain.ExecutionResult, error) {
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
	cmd.Stderr = &stderr

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

// SpawnLongLived starts cmd, wires stdio pipes, and returns a handle; caller must Wait or Kill.
//
// SpawnLongLived 启动命令并布 stdio 管道，返 handle；调用方必须 Wait 或 Kill。
func SpawnLongLived(ctx context.Context, opts SpawnOptions) (sandboxdomain.LongLivedHandle, error) {
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

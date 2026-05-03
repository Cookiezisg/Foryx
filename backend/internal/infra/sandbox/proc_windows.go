//go:build windows

// proc_windows.go: process tree management for Windows. Windows has no
// process group concept; we use `taskkill /T /F /PID <pid>` as a simple
// fallback that handles the common case (uv → pip → python). A proper
// Job Object implementation (CreateJobObject + JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
// + AssignProcessToJobObject) would be cleaner — sandbox iteration doc §12
// risk table flags this as a v2 upgrade. For MVP, taskkill is enough.
//
// proc_windows.go：Windows 进程树管理。Windows 没"进程组"概念，
// 我们用 `taskkill /T /F /PID <pid>` 作简单 fallback 处理常见场景
// （uv → pip → python）。Job Object 实现（CreateJobObject +
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE + AssignProcessToJobObject）更干净
// ——沙箱迭代 §12 风险表标为 v2 升级。MVP 用 taskkill 够。

package sandbox

import (
	"fmt"
	"os/exec"
	"strconv"
)

// setupProcessGroup is a no-op on Windows. taskkill /T below handles the
// tree without any per-process setup.
//
// setupProcessGroup Windows 上 no-op。taskkill /T 不需 per-process 设置。
func setupProcessGroup(cmd *exec.Cmd) {
	// Nothing to set up — taskkill /T below kills the tree.
}

// killProcessGroup runs `taskkill /T /F /PID <pid>` to terminate cmd's
// entire process tree. /T = tree (includes all descendants); /F = force
// (no graceful shutdown chance). Returns nil if the process never started.
//
// killProcessGroup 跑 `taskkill /T /F /PID <pid>` 终止 cmd 的整个进程树。
// /T = 树（含所有后代）；/F = 强制（不给优雅关闭机会）。进程未启动返 nil。
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	pid := strconv.Itoa(cmd.Process.Pid)
	out, err := exec.Command("taskkill", "/T", "/F", "/PID", pid).CombinedOutput()
	if err != nil {
		return fmt.Errorf("taskkill: %w (output: %s)", err, out)
	}
	return nil
}

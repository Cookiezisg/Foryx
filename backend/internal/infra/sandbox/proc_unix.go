//go:build !windows

// proc_unix.go: process tree management for unix-like systems via process
// groups. setupProcessGroup tells the kernel to put the child in its own
// pgrp; killProcessGroup signals the whole group via -pid (negative pid
// targets the group rather than a single process).
//
// proc_unix.go：unix 系统下通过进程组管理进程树。setupProcessGroup 让
// 内核把子进程放在独立 pgrp；killProcessGroup 通过 -pid 给整组发信号
// （负 pid 指向进程组而非单进程）。

package sandbox

import (
	"os/exec"
	"syscall"
)

// setupProcessGroup configures cmd so its child runs in its own process
// group. Must be called before cmd.Start().
//
// setupProcessGroup 配 cmd 让子进程跑在独立进程组。必须在 cmd.Start() 前调。
func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends SIGKILL to cmd's entire process group. Used as the
// cmd.Cancel callback for ctx-cancel propagation. Returns nil if the
// process never started (cmd.Process == nil).
//
// killProcessGroup 给 cmd 的整个进程组发 SIGKILL。作 cmd.Cancel callback
// 传播 ctx 取消。进程未启动（cmd.Process == nil）返 nil。
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

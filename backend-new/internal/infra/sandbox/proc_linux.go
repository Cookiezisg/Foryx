//go:build linux

package sandbox

import (
	"os/exec"
	"syscall"
)

// setupProcessGroup puts cmd's child in its own group + Pdeathsig=SIGTERM; call before Start.
//
// setupProcessGroup 让 cmd 的 child 在独立进程组并设 Pdeathsig=SIGTERM；Start 前调。
func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}
}

// killProcessGroup sends SIGKILL to the entire process group via -pid.
//
// killProcessGroup 给整个进程组发 SIGKILL（负 pid）。
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

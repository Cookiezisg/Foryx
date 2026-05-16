// Package shell provides shell-execution system tools (Bash / BashOutput / KillShell).
//
// Package shell 提供 shell 执行系统工具（Bash / BashOutput / KillShell）。
package shell

import (
	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

// ShellTools bundles shell system tools sharing one ProcessManager; caller calls Manager.Stop() on shutdown.
//
// ShellTools 共享一份 ProcessManager 的 shell system tool 集合；调用方关停时调 Manager.Stop()。
type ShellTools struct {
	Manager *ProcessManager
	Tools   []toolapp.Tool
}

// NewShellTools wires Bash + BashOutput + KillShell; pass nil sandbox to keep Bash on plain system shell.
//
// NewShellTools 装配 Bash + BashOutput + KillShell；sandbox 传 nil 走 plain system shell。
func NewShellTools(sandbox *sandboxapp.Service) *ShellTools {
	mgr := NewProcessManager()
	return &ShellTools{
		Manager: mgr,
		Tools: []toolapp.Tool{
			&Bash{mgr: mgr, sandbox: sandbox},
			&BashOutput{mgr: mgr},
			&KillShell{mgr: mgr},
		},
	}
}

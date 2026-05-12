// sandbox_types.go — request value types for the handler.Sandbox port.
//
// Per D-redo-8 (forge_redesign 2026-05-12) each HandlerVersion owns its own
// venv keyed by a freshly-generated EnvID (`hdenv_<16hex>`), 1:1 with the
// Version row but **independent from VersionID**. EnvID is handler-local
// nomenclature; sandbox treats it as opaque. SpawnRequest is handler-specific
// (handler is the first trinity domain that needs long-lived subprocess spawn).
//
// sandbox_types.go —— handler.Sandbox 端口的请求值类型。
//
// 按 D-redo-8(forge_redesign 2026-05-12),每个 HandlerVersion 独立持有 venv,
// EnvID(`hdenv_<16hex>`)在 Version 创建时新生成,跟 VersionID 1:1 但**独立**。
// EnvID 是 handler 自己的命名空间。

package handler

// SyncRequest is one materialize-this-EnvID order. Same shape as function's.
//
// SyncRequest 物化 EnvID 指令(跟 function 同形)。
type SyncRequest struct {
	HandlerID     string
	VersionID     string
	EnvID         string
	Dependencies  []string
	PythonVersion string
	OnProgress    func(stage, detail string)
}

// SpawnRequest is one start-long-lived-subprocess order. The Sandbox spawns
// a python process running the driver script (which imports HandlerImpl from
// user_handler.py).
//
// SpawnRequest 起长跑子进程指令。Sandbox 跑 python driver(import user_handler 中的 HandlerImpl)。
type SpawnRequest struct {
	HandlerID string
	VersionID string
	EnvID     string
	// Env vars passed to subprocess (PYTHONPATH etc.). System sets these;
	// user init_args go via the protocol init message, NOT env.
	//
	// Env 给子进程的环境变量(PYTHONPATH 等);user init_args 走协议 init
	// 消息,不走 env。
	Env map[string]string
}

// SyncError wraps a venv-build failure so Service can errors.As + extract
// stderr text into Version.EnvError.
//
// SyncError 包装 venv 构建失败,Service 经 errors.As + 提 stderr 到 EnvError。
type SyncError struct {
	Cause  error
	Stderr string
}

func (e *SyncError) Error() string { return e.Stderr }
func (e *SyncError) Unwrap() error { return e.Cause }

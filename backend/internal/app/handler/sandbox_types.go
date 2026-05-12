// sandbox_types.go — request value types for the handler.Sandbox port.
//
// Per D-redo-8 (forge_redesign 2026-05-12) EnvID = VersionID (1:1 per version,
// no cross-version sharing) — the prior sha256(deps, python) hash + sharing
// logic was removed; each HandlerVersion row owns its own venv keyed by
// Version.ID. SpawnRequest is handler-specific (handler is the first trinity
// domain that needs long-lived subprocess spawn).
//
// sandbox_types.go —— handler.Sandbox 端口的请求值类型。
//
// 按 D-redo-8(forge_redesign 2026-05-12),EnvID = VersionID(每版本独立 venv,
// 跨版本不共享)——sha256(deps, python) 哈希共享逻辑已删。SpawnRequest 是
// handler 专属(handler 是第一个需要长跑子进程的 trinity 域)。

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

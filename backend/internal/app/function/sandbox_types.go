// sandbox_types.go — request value types for the function.Sandbox port.
//
// Per D-redo-8 (forge_redesign 2026-05-12) EnvID = VersionID (1:1 per version,
// no cross-version sharing) — the prior sha256(deps, python) hash + sharing
// logic was removed; each Version row now owns its own venv keyed by Version.ID.
//
// sandbox_types.go —— function.Sandbox 端口的请求值类型。
//
// 按 D-redo-8(forge_redesign 2026-05-12),EnvID = VersionID(每版本独立 venv,
// 跨版本不共享)——原本 sha256(deps, python) 哈希共享逻辑已删,每个 Version 行
// 拥有自己的 venv,key 即 Version.ID。

package function

// SyncRequest is one materialize-this-EnvID order. The Sandbox implementation
// creates a venv keyed by EnvID under the function's own dir, installs
// Dependencies via uv pip, and reports per-stage progress via OnProgress.
//
// SyncRequest 是一份"物化这个 EnvID"的指令。Sandbox 实现按 EnvID 在 function
// 自己的 dir 下建 venv,通过 uv pip 装 Dependencies,per-stage 进度通过
// OnProgress 报。
type SyncRequest struct {
	FunctionID    string
	VersionID     string // EnvID == VersionID per D-redo-8; kept distinct in the struct for log clarity
	EnvID         string
	Dependencies  []string
	PythonVersion string
	OnProgress    func(stage, detail string)
}

// RunRequest is one execute-this-function order.
//
// RunRequest 是一份"执行这个 function"的指令。
type RunRequest struct {
	FunctionID    string
	VersionID     string
	EnvID         string
	Code          string
	EntryFunction string // optional; sandbox falls back to first `def` if empty
	Input         map[string]any
}

// SyncError wraps a venv-build failure (e.g. uv pip stderr) so the function
// service can errors.As + extract the captured stderr text into the
// FunctionVersion.EnvError column. Adapter implementations populate this when
// the underlying tool reports a failure.
//
// SyncError 包装 venv 构建失败(如 uv pip stderr),让 function service 能
// errors.As + 把捕获的 stderr 文本提取到 FunctionVersion.EnvError 列。
// adapter 实现在底层工具报错时填这个。
type SyncError struct {
	Cause  error
	Stderr string
}

func (e *SyncError) Error() string { return e.Stderr }
func (e *SyncError) Unwrap() error { return e.Cause }

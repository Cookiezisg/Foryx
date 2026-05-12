// sandbox_types.go — request value types for the function.Sandbox port.
//
// Per D-redo-8 (forge_redesign 2026-05-12) each FunctionVersion owns its own
// venv keyed by a freshly-generated EnvID (`fnenv_<16hex>`), 1:1 with the
// Version row but **independent from VersionID**. EnvID is function-local
// nomenclature: the sandbox treats it as an opaque string. Other sandbox
// consumers (handler, chat tool calls, mcp, ...) generate EnvIDs in their
// own naming spaces — sandbox stays neutral.
//
// sandbox_types.go —— function.Sandbox 端口的请求值类型。
//
// 按 D-redo-8(forge_redesign 2026-05-12),每个 FunctionVersion 独立持有一个
// venv,EnvID(`fnenv_<16hex>`)在 Version 创建时新生成,跟 VersionID 1:1 但
// **独立**。EnvID 是 function 自己的命名空间;sandbox 当不透明 string,跟其他
// 消费者(handler / chat tool / mcp ...)的 EnvID 命名互不干扰。

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
	VersionID     string // log/diagnostic field; EnvID is the venv key, not this
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

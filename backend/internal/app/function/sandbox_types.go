package function

// SyncRequest is one materialize-this-EnvID order.
//
// SyncRequest 是一份「物化这个 EnvID」的指令。
type SyncRequest struct {
	FunctionID    string
	VersionID     string
	EnvID         string
	Dependencies  []string
	PythonVersion string
	OnProgress    func(stage, detail string)
}

// RunRequest is one execute-this-function order.
//
// RunRequest 是一份「执行这个 function」的指令。
type RunRequest struct {
	FunctionID    string
	VersionID     string
	EnvID         string
	Code          string
	EntryFunction string
	Input         map[string]any
}

// SyncError carries the captured stderr from a venv-build failure for errors.As use.
//
// SyncError 携带 venv 构建失败的 stderr，供 errors.As 提取。
type SyncError struct {
	Cause  error
	Stderr string
}

func (e *SyncError) Error() string { return e.Stderr }
func (e *SyncError) Unwrap() error { return e.Cause }

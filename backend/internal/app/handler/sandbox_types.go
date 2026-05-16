package handler

// SyncRequest is one materialize-this-EnvID order.
//
// SyncRequest 是一份物化 EnvID 的指令。
type SyncRequest struct {
	HandlerID     string
	VersionID     string
	EnvID         string
	Dependencies  []string
	PythonVersion string
	OnProgress    func(stage, detail string)
}

// SpawnRequest is one start-long-lived-subprocess order.
//
// SpawnRequest 是一份启动长跑子进程的指令。
type SpawnRequest struct {
	HandlerID string
	VersionID string
	EnvID     string
	Env       map[string]string
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

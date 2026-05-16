package function

// ExecutionResult is the outcome of a single sandbox Run call for a function.
//
// ExecutionResult 是单次 sandbox Run function 的执行结果。
type ExecutionResult struct {
	OK        bool   `json:"ok"`
	Output    any    `json:"output"`
	ErrorMsg  string `json:"errorMsg"`
	ElapsedMs int64  `json:"elapsedMs"`
}

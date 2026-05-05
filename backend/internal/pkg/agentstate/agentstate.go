// Package agentstate carries per-conversation state shared across tool
// invocations (must-Read-first SeenFiles, Bash cwd). Lives in pkg/ (not
// app/chat/) so pkg/reqctx can ferry the pointer through ctx without cycles.
//
// Package agentstate 持有跨 tool 调用的对话级状态（must-Read-first 的 SeenFiles、
// Bash cwd）。放在 pkg/ 是为了让 pkg/reqctx 通过 ctx 转运指针不形成循环。
package agentstate

import "sync"

// AgentState is the per-conversation shared state for tool invocations.
// Methods are concurrency-safe.
//
// AgentState 是 tool 调用的对话级共享状态。方法并发安全。
type AgentState struct {
	// SeenFiles maps absolute path → file size at Read time. Edit/Write
	// check membership for must-Read-first; size detects external mods.
	//
	// SeenFiles：绝对路径 → Read 时文件 size。Edit/Write 检查 membership
	// 以强制 must-Read-first；size 用于检测外部修改。
	SeenFiles sync.Map // string → int64

	// cwd: empty = "use process cwd" (Bash resolves lazily, so zero-value AgentState works).
	// cwd: 空 = "用进程 cwd"（Bash 懒解析，零值 AgentState 即可用）。
	cwdMu sync.Mutex
	cwd   string
}

// MarkRead records path as Read this conversation with its current size.
//
// MarkRead 记录 path 在本对话中已 Read，并存当前 size。
func (s *AgentState) MarkRead(path string, size int64) {
	s.SeenFiles.Store(path, size)
}

// WasRead returns the size recorded at first MarkRead, or false if absent.
// A current-vs-recorded size mismatch can indicate external modification.
//
// WasRead 返回首次 MarkRead 时记录的 size；缺失返 false。
// 当前与记录 size 不一致可能意味着外部修改。
func (s *AgentState) WasRead(path string) (int64, bool) {
	v, ok := s.SeenFiles.Load(path)
	if !ok {
		return 0, false
	}
	return v.(int64), true
}

// Cwd returns the tracked Bash working directory; "" means "use process cwd".
//
// Cwd 返回追踪的 Bash 工作目录；"" 表示"用进程 cwd"。
func (s *AgentState) Cwd() string {
	s.cwdMu.Lock()
	defer s.cwdMu.Unlock()
	return s.cwd
}

// SetCwd updates the tracked working directory (called when Bash detects an
// entire-command `cd <path>`).
//
// SetCwd 更新追踪的工作目录（Bash 识别整条命令为 `cd <path>` 时调用）。
func (s *AgentState) SetCwd(path string) {
	s.cwdMu.Lock()
	defer s.cwdMu.Unlock()
	s.cwd = path
}

// Package agentstate carries per-conversation state shared across tool invocations.
//
// Package agentstate 持有跨 tool 调用的对话级共享状态。
package agentstate

import "sync"

// AgentState is the per-conversation shared state for tool invocations; methods are concurrency-safe.
//
// AgentState 是 tool 调用的对话级共享状态，方法并发安全。
type AgentState struct {
	SeenFiles sync.Map // string → int64

	cwdMu sync.Mutex
	cwd   string

	activeSkill activeSkillSlot

	groupMu         sync.Mutex
	activatedGroups map[string]bool
}

// MarkRead records path as Read this conversation with its current size.
//
// MarkRead 记录 path 在本对话中已 Read，并存当前 size。
func (s *AgentState) MarkRead(path string, size int64) {
	s.SeenFiles.Store(path, size)
}

// WasRead returns the size recorded at first MarkRead; a mismatch with current size hints at external mod.
//
// WasRead 返回首次 MarkRead 记录的 size；与当前 size 不符可能是外部改了。
func (s *AgentState) WasRead(path string) (int64, bool) {
	v, ok := s.SeenFiles.Load(path)
	if !ok {
		return 0, false
	}
	return v.(int64), true
}

// Cwd returns the tracked Bash working directory; "" means use process cwd.
//
// Cwd 返回追踪的 Bash 工作目录；"" 表示用进程 cwd。
func (s *AgentState) Cwd() string {
	s.cwdMu.Lock()
	defer s.cwdMu.Unlock()
	return s.cwd
}

// SetCwd updates the tracked working directory (called when Bash detects a bare `cd <path>`).
//
// SetCwd 更新追踪的工作目录（Bash 识别整条命令为 `cd <path>` 时调用）。
func (s *AgentState) SetCwd(path string) {
	s.cwdMu.Lock()
	defer s.cwdMu.Unlock()
	s.cwd = path
}

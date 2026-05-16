// Package memory provides system tools for the LLM to manage cross-conversation long-term memory.
//
// Package memory 提供管理跨对话长期记忆的 system tool。
package memory

import (
	memoryapp "github.com/sunweilin/forgify/backend/internal/app/memory"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

// MemoryTools constructs the memory system tools wired against one Service.
//
// MemoryTools 用一个 Service 构造 memory system tool。
func MemoryTools(svc *memoryapp.Service) []toolapp.Tool {
	return []toolapp.Tool{
		&ReadMemory{svc: svc},
		&WriteMemory{svc: svc},
		&ForgetMemory{svc: svc},
	}
}

var (
	_ toolapp.Tool = (*ReadMemory)(nil)
	_ toolapp.Tool = (*WriteMemory)(nil)
	_ toolapp.Tool = (*ForgetMemory)(nil)
)

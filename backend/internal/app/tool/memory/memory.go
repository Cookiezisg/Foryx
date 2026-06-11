// Package memory provides the LLM system tools for cross-conversation long-term
// memory: read / write / forget. These are thin adapters over memoryapp.Service —
// no domain / store / handler / DDL / HTTP — implementing only the app/tool 5-method
// contract. They are lazy tools (Toolset.Lazy), surfaced via search_tools.
//
// The memory index is already injected into the system prompt by
// memory.ForSystemPrompt (pinned in full, others as a name+description list), so the
// LLM rarely needs a "list" tool — read_memory loads the full body of a non-pinned one.
//
// Package memory 提供跨对话长期记忆的 LLM system tool：read / write / forget。它们是
// memoryapp.Service 之上的薄适配器——无 domain/store/handler/DDL/HTTP——只实现 app/tool
// 的 5 方法契约。是懒加载工具（Toolset.Lazy），经 search_tools 浮现。记忆目录已由
// memory.ForSystemPrompt 注入 system prompt（pinned 全文、其余 name+description 列表），
// 故几乎不需要 list 工具——read_memory 加载某条非 pinned 的全文。
package memory

import (
	memoryapp "github.com/sunweilin/forgify/backend/internal/app/memory"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	errorspkg "github.com/sunweilin/forgify/backend/internal/pkg/errors"
)

// Input-validation sentinels shared across the memory tools' ValidateInput (presence
// checks). errorspkg.New like every sentinel; surfaced to the LLM as a tool-result string.
//
// memory 工具 ValidateInput 的输入校验 sentinel（必填检查）。同所有 sentinel 用 errorspkg.New；
// 经 tool-result 串给 LLM。
var (
	ErrEmptyName        = errorspkg.New(errorspkg.KindInvalid, "MEMORY_EMPTY_NAME", "name is required")
	ErrEmptyDescription = errorspkg.New(errorspkg.KindInvalid, "MEMORY_EMPTY_DESCRIPTION", "description is required")
	ErrEmptyContent     = errorspkg.New(errorspkg.KindInvalid, "MEMORY_EMPTY_CONTENT", "content is required")
)

// MemoryTools constructs the memory system tools over one Service.
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

// Package todo provides the 4 system tools the LLM uses to manage its
// per-conversation todo list: TodoCreate, TodoList, TodoGet, TodoUpdate.
//
// Imported as `todotool` per §S13 nested sub-package alias rule.
//
// All four tools share one *todoapp.Service which owns scoping (every
// call resolves the conversation_id from ctx), persistence, and SSE
// publishing. The tools themselves stay thin — JSON in, JSON out.
//
// Package todo 提供 4 个 LLM 用于管理对话级 TODO 列表的 system tool：
// TodoCreate / TodoList / TodoGet / TodoUpdate。
//
// 按 §S13 嵌套子包别名规则导入为 `todotool`。
//
// 4 个工具共享一份 *todoapp.Service——它负责作用域（每次调用从 ctx 解析
// conversation_id）、持久化与 SSE 推送。工具本身保持薄：JSON in / JSON out。
package todo

import (
	todoapp "github.com/sunweilin/forgify/backend/internal/app/todo"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

// TodoTools constructs the 4 todo system tools wired against one Service.
//
// TodoTools 用一个 Service 构造 4 个 todo system tool。
func TodoTools(svc *todoapp.Service) []toolapp.Tool {
	return []toolapp.Tool{
		&TodoCreate{svc: svc},
		&TodoList{svc: svc},
		&TodoGet{svc: svc},
		&TodoUpdate{svc: svc},
	}
}

// ── Compile-time checks ───────────────────────────────────────────────────────

var (
	_ toolapp.Tool = (*TodoCreate)(nil)
	_ toolapp.Tool = (*TodoList)(nil)
	_ toolapp.Tool = (*TodoGet)(nil)
	_ toolapp.Tool = (*TodoUpdate)(nil)
)

// Package task provides the 4 system tools the LLM uses to manage its
// per-conversation task list: TaskCreate, TaskList, TaskGet, TaskUpdate.
//
// Imported as `tasktool` per §S13 nested sub-package alias rule.
//
// All four tools share one *taskapp.Service which owns scoping (every
// call resolves the conversation_id from ctx), persistence, and SSE
// publishing. The tools themselves stay thin — JSON in, JSON out.
//
// Package task 提供 4 个 LLM 用于管理对话级任务列表的 system tool：
// TaskCreate / TaskList / TaskGet / TaskUpdate。
//
// 按 §S13 嵌套子包别名规则导入为 `tasktool`。
//
// 4 个工具共享一份 *taskapp.Service——它负责作用域（每次调用从 ctx 解析
// conversation_id）、持久化与 SSE 推送。工具本身保持薄：JSON in / JSON out。
package task

import (
	taskapp "github.com/sunweilin/forgify/backend/internal/app/task"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

// TaskTools constructs the 4 task system tools wired against one Service.
//
// TaskTools 用一个 Service 构造 4 个 task system tool。
func TaskTools(svc *taskapp.Service) []toolapp.Tool {
	return []toolapp.Tool{
		&TaskCreate{svc: svc},
		&TaskList{svc: svc},
		&TaskGet{svc: svc},
		&TaskUpdate{svc: svc},
	}
}

// ── Compile-time checks ───────────────────────────────────────────────────────

var (
	_ toolapp.Tool = (*TaskCreate)(nil)
	_ toolapp.Tool = (*TaskList)(nil)
	_ toolapp.Tool = (*TaskGet)(nil)
	_ toolapp.Tool = (*TaskUpdate)(nil)
)

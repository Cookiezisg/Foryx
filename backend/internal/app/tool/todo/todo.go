// Package todo provides system tools for managing the per-conversation todo list.
//
// Package todo 提供管理对话级 TODO 列表的 system tool。
package todo

import (
	todoapp "github.com/sunweilin/forgify/backend/internal/app/todo"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

// TodoTools constructs the todo system tools wired against one Service.
//
// TodoTools 用一个 Service 构造 todo system tool。
func TodoTools(svc *todoapp.Service) []toolapp.Tool {
	return []toolapp.Tool{
		&TodoCreate{svc: svc},
		&TodoList{svc: svc},
		&TodoGet{svc: svc},
		&TodoUpdate{svc: svc},
	}
}


var (
	_ toolapp.Tool = (*TodoCreate)(nil)
	_ toolapp.Tool = (*TodoList)(nil)
	_ toolapp.Tool = (*TodoGet)(nil)
	_ toolapp.Tool = (*TodoUpdate)(nil)
)

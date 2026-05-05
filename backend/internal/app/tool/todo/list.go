// list.go — TodoList system tool: read every active todo in the current
// conversation, ordered by creation time. Used by the LLM to decide what
// to do next.
//
// list.go — TodoList 系统工具：读取当前对话所有活跃 todo，按创建时间排序；
// LLM 用来决定下一步做什么。
package todo

import (
	"context"
	"encoding/json"
	"fmt"

	todoapp "github.com/sunweilin/forgify/backend/internal/app/todo"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

const todoListDescription = `List every todo on the current conversation's todo list.

Usage:
- Returns a JSON array of todos, each with id / subject / status / activeForm / etc.
- Todos are ordered by created_at ascending so you see them in the order they were added.
- Soft-deleted todos are excluded.
- Use this to decide which todo to work on next or to summarise progress to the user.`

var todoListSchema = json.RawMessage(`{
	"type": "object",
	"properties": {}
}`)

// TodoList implements the TodoList system tool.
//
// TodoList struct 是 TodoList 系统工具。
type TodoList struct {
	svc *todoapp.Service
}

func (t *TodoList) Name() string                { return "TodoList" }
func (t *TodoList) Description() string         { return todoListDescription }
func (t *TodoList) Parameters() json.RawMessage { return todoListSchema }

func (t *TodoList) IsReadOnly() bool        { return true }
func (t *TodoList) NeedsReadFirst() bool    { return false }
func (t *TodoList) RequiresWorkspace() bool { return false }

func (t *TodoList) ValidateInput(_ json.RawMessage) error { return nil }

func (t *TodoList) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

// Execute pulls the conversation's todos and returns them as JSON.
//
// Execute 拉取对话 todo 并返 JSON。
func (t *TodoList) Execute(ctx context.Context, _ string) (string, error) {
	todos, err := t.svc.List(ctx)
	if err != nil {
		return classifyTodoErr(err, "list"), nil
	}
	out := struct {
		Total int `json:"total"`
		Todos any `json:"todos"`
	}{
		Total: len(todos),
		Todos: todos,
	}
	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", fmt.Errorf("TodoList.Execute: marshal: %w", err)
	}
	return string(body), nil
}

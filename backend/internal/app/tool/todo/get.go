// get.go — TodoGet system tool: fetch a single todo by ID from the
// current conversation.
//
// get.go — TodoGet 系统工具：按 ID 从当前对话取单条 todo。
package todo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	todoapp "github.com/sunweilin/forgify/backend/internal/app/todo"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

const todoGetDescription = `Fetch one todo by ID from the current conversation's todo list.

Usage:
- ` + "`todo_id`" + ` is the ID returned by TodoCreate (or seen in TodoList output).
- Returns the todo as JSON, or a not-found message if the ID does not belong to this conversation.`

var todoGetSchema = json.RawMessage(`{
	"type": "object",
	"required": ["todo_id"],
	"properties": {
		"todo_id": {
			"type": "string",
			"description": "ID of the todo to fetch (e.g. td_abc123…)."
		}
	}
}`)

// TodoGet implements the TodoGet system tool.
//
// TodoGet struct 是 TodoGet 系统工具。
type TodoGet struct {
	svc *todoapp.Service
}

func (t *TodoGet) Name() string                { return "TodoGet" }
func (t *TodoGet) Description() string         { return todoGetDescription }
func (t *TodoGet) Parameters() json.RawMessage { return todoGetSchema }

func (t *TodoGet) IsReadOnly() bool        { return true }
func (t *TodoGet) NeedsReadFirst() bool    { return false }
func (t *TodoGet) RequiresWorkspace() bool { return false }

func (t *TodoGet) ValidateInput(args json.RawMessage) error {
	var a struct {
		TodoID string `json:"todo_id"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("TodoGet.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.TodoID) == "" {
		return errors.New("todo_id is required")
	}
	return nil
}

func (t *TodoGet) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *TodoGet) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		TodoID string `json:"todo_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("TodoGet.Execute: %w", err)
	}
	got, err := t.svc.Get(ctx, args.TodoID)
	if err != nil {
		return classifyTodoErr(err, "get"), nil
	}
	return marshalIndent(got)
}

// Package todo provides the todo_write resident tool — the checklist's ONLY write path.
// The HTTP board is read-only by design ("写入是 LLM 专属") and the per-turn reminder
// only renders what was written, so without this tool the whole todo entity is inert.
//
// Package todo 提供 todo_write 常驻工具——清单的**唯一**写入口。HTTP 看板按设计只读
// （「写入是 LLM 专属」），每轮 reminder 只渲染已写内容，没有这个工具整个 todo 实体是死的。
package todo

import (
	"context"
	"encoding/json"
	"fmt"

	todoapp "github.com/sunweilin/forgify/backend/internal/app/todo"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	tododomain "github.com/sunweilin/forgify/backend/internal/domain/todo"
)

// TodoTools constructs the todo tool group (resident — planning must not need discovery).
//
// TodoTools 构造 todo 工具组（常驻——规划不该先经发现）。
func TodoTools(svc *todoapp.Service) []toolapp.Tool {
	return []toolapp.Tool{&TodoWrite{svc: svc}}
}

type TodoWrite struct{ svc *todoapp.Service }

func (t *TodoWrite) Name() string { return "todo_write" }

func (t *TodoWrite) Description() string {
	return "Replace your ENTIRE task checklist for this conversation (wholesale write — always send the full list, not a diff). Use it to plan multi-step work and to mark progress: statuses are pending | in_progress | completed. Pass an empty items array to clear the list. The user sees the board live."
}

func (t *TodoWrite) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["items"],
		"properties": {
			"items": {
				"type": "array",
				"description": "The complete checklist (max 64). Replaces whatever was there.",
				"items": {
					"type": "object",
					"required": ["content", "status"],
					"properties": {
						"content": {"type": "string", "description": "Imperative task description."},
						"activeForm": {"type": "string", "description": "Present-continuous label shown while in_progress."},
						"status": {"type": "string", "description": "pending | in_progress | completed."}
					}
				}
			}
		}
	}`)
}

func (t *TodoWrite) ValidateInput(args json.RawMessage) error {
	var a struct {
		Items *[]tododomain.Item `json:"items"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("todo_write: bad args: %w", err)
	}
	if a.Items == nil {
		return tododomain.ErrItemsRequired
	}
	return nil
}

func (t *TodoWrite) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Items []tododomain.Item `json:"items"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("todo_write: bad args: %w", err)
	}
	rendered, err := t.svc.Write(ctx, args.Items)
	if err != nil {
		return "", fmt.Errorf("todo_write: %w", err)
	}
	return rendered, nil
}

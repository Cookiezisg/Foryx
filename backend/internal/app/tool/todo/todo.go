// Package todo provides the todo_write (the checklist's ONLY write path) + todo_read (the
// read-back path) resident tools. The HTTP board is read-only by design ("写入是 LLM 专属") and
// the per-turn reminder suppresses a fully-completed list, so without todo_read the agent
// confabulated when asked to list its todos after finishing them (F39).
//
// Package todo 提供 todo_write（清单**唯一**写入口）+ todo_read（读回路径）常驻工具。HTTP
// 看板按设计只读（「写入是 LLM 专属」），每轮 reminder 抑制全完成清单，故没有 todo_read 时
// agent 在完成 todo 后被问列清单会编造（F39）。
package todo

import (
	"context"
	"encoding/json"
	"fmt"

	todoapp "github.com/sunweilin/anselm/backend/internal/app/todo"
	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	tododomain "github.com/sunweilin/anselm/backend/internal/domain/todo"
)

// TodoTools constructs the todo tool group (resident — planning AND read-back must not need
// discovery; a read tool gated behind search_tools would defeat its purpose).
//
// TodoTools 构造 todo 工具组（常驻——规划与读回都不该先经发现；读工具若藏在 search_tools 后面就失去意义）。
func TodoTools(svc *todoapp.Service) []toolapp.Tool {
	return []toolapp.Tool{&TodoWrite{svc: svc}, &TodoRead{svc: svc}}
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

// TodoRead reads back the current conversation's checklist INCLUDING completed items. The gap it
// fills (F39): the per-turn reminder suppresses a fully-completed list, and there was no read path,
// so asked to "list my todos" after finishing them the agent answered from memory and confabulated.
// No-arg, read-only; reuses the same render() todo_write echoes.
//
// TodoRead 读回当前对话清单**含已完成项**。它填的缺口（F39）：每轮 reminder 抑制全完成清单、又
// 无读路径，故 agent 完成后被问「列出我的 todo」时凭记忆作答、编造。无参、只读；复用 todo_write 同款 render()。
type TodoRead struct{ svc *todoapp.Service }

func (t *TodoRead) Name() string { return "todo_read" }

func (t *TodoRead) Description() string {
	return "Read back your current task checklist for THIS conversation, INCLUDING completed items. Use it when asked to list or recall your todos — read the saved list rather than relying on memory (a fully-completed list is not shown in the per-turn reminder). No arguments."
}

func (t *TodoRead) Parameters() json.RawMessage {
	return json.RawMessage(`{"type": "object", "properties": {}}`)
}

// ValidateInput accepts any input — todo_read takes no arguments.
//
// ValidateInput 接受任意输入——todo_read 无参。
func (t *TodoRead) ValidateInput(_ json.RawMessage) error { return nil }

func (t *TodoRead) Execute(ctx context.Context, _ string) (string, error) {
	rendered, err := t.svc.ReadRendered(ctx)
	if err != nil {
		return "", fmt.Errorf("todo_read: %w", err)
	}
	return rendered, nil
}

// update.go — TodoUpdate system tool: change a todo's status or other
// fields. Pointer fields in the schema map to "set" semantics — omit a
// field to leave it unchanged. Status transitions are validated against
// the whitelist; status:"deleted" soft-deletes via the Service.
//
// update.go — TodoUpdate 系统工具：改 todo 状态或其他字段。schema 字段缺
// 即"不变"；status 按白名单校验；status:"deleted" 走 Service 软删。
package todo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	todoapp "github.com/sunweilin/forgify/backend/internal/app/todo"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	tododomain "github.com/sunweilin/forgify/backend/internal/domain/todo"
)

const todoUpdateDescription = `Update a todo's status or other fields.

Usage:
- ` + "`todo_id`" + ` is the ID returned by TodoCreate (or seen in TodoList).
- Provide only the fields you want to change; omitted fields stay as-is.
- ` + "`status`" + ` transitions: pending → in_progress → completed. Use "deleted" to remove a todo; the deletion broadcasts an SSE update so any UI drops it.
- ` + "`subject`" + ` / ` + "`description`" + ` / ` + "`active_form`" + ` / ` + "`owner`" + ` are simple replacements.
- ` + "`blocked_by`" + ` replaces the entire dependency list (pass [] to clear).
- Returns the updated todo as JSON.`

var todoUpdateSchema = json.RawMessage(`{
	"type": "object",
	"required": ["todo_id"],
	"properties": {
		"todo_id": {
			"type": "string",
			"description": "ID of the todo to update."
		},
		"subject": {
			"type": "string",
			"description": "New imperative title (must be non-empty if provided)."
		},
		"description": {
			"type": "string",
			"description": "New context note (empty string clears it)."
		},
		"active_form": {
			"type": "string",
			"description": "New present-continuous form."
		},
		"status": {
			"type": "string",
			"enum": ["pending", "in_progress", "completed", "deleted"],
			"description": "New status. \"deleted\" soft-deletes the todo."
		},
		"owner": {
			"type": "string",
			"description": "New owner identifier."
		},
		"blocked_by": {
			"type": "array",
			"items": {"type": "string"},
			"description": "Replacement list of todo IDs that must complete before this one starts. Pass [] to clear."
		}
	}
}`)

// TodoUpdate implements the TodoUpdate system tool.
//
// TodoUpdate struct 是 TodoUpdate 系统工具。
type TodoUpdate struct {
	svc *todoapp.Service
}

func (t *TodoUpdate) Name() string                { return "TodoUpdate" }
func (t *TodoUpdate) Description() string         { return todoUpdateDescription }
func (t *TodoUpdate) Parameters() json.RawMessage { return todoUpdateSchema }

func (t *TodoUpdate) IsReadOnly() bool        { return false }
func (t *TodoUpdate) NeedsReadFirst() bool    { return false }
func (t *TodoUpdate) RequiresWorkspace() bool { return false }

// ValidateInput rejects empty todo_id and out-of-whitelist status.
//
// ValidateInput 拒空 todo_id 与白名单外 status。
func (t *TodoUpdate) ValidateInput(args json.RawMessage) error {
	var a updateRaw
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("TodoUpdate.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.TodoID) == "" {
		return errors.New("todo_id is required")
	}
	if a.Status != nil && !tododomain.IsValidStatus(*a.Status) {
		return tododomain.ErrInvalidStatus
	}
	return nil
}

func (t *TodoUpdate) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

// updateRaw is the JSON payload shape; pointer fields encode "set vs
// leave unchanged" semantics.
//
// updateRaw 是 JSON 载荷形态；指针字段编码"设值 vs 不变"语义。
type updateRaw struct {
	TodoID      string    `json:"todo_id"`
	Subject     *string   `json:"subject"`
	Description *string   `json:"description"`
	ActiveForm  *string   `json:"active_form"`
	Status      *string   `json:"status"`
	Owner       *string   `json:"owner"`
	BlockedBy   *[]string `json:"blocked_by"`
}

// Execute applies the partial update via Service. status:"deleted"
// triggers Service.Delete instead so the soft-delete + final-snapshot
// SSE broadcast happens in one place.
//
// Execute 通过 Service 应用部分更新；status:"deleted" 走 Service.Delete
// 让软删 + 最终 SSE 广播集中一处。
func (t *TodoUpdate) Execute(ctx context.Context, argsJSON string) (string, error) {
	var raw updateRaw
	if err := json.Unmarshal([]byte(argsJSON), &raw); err != nil {
		return "", fmt.Errorf("TodoUpdate.Execute: %w", err)
	}

	// Special case: status:"deleted" → Service.Delete (sets deleted_at).
	// 特例：status:"deleted" → Service.Delete（置 deleted_at）。
	if raw.Status != nil && *raw.Status == tododomain.StatusDeleted {
		if err := t.svc.Delete(ctx, raw.TodoID); err != nil {
			return classifyTodoErr(err, "delete"), nil
		}
		return fmt.Sprintf(`{"deleted":true,"id":%q}`, raw.TodoID), nil
	}

	updated, err := t.svc.Update(ctx, raw.TodoID, todoapp.UpdateInput{
		Subject:     raw.Subject,
		Description: raw.Description,
		ActiveForm:  raw.ActiveForm,
		Status:      raw.Status,
		Owner:       raw.Owner,
		BlockedBy:   raw.BlockedBy,
	})
	if err != nil {
		return classifyTodoErr(err, "update"), nil
	}
	return marshalIndent(updated)
}

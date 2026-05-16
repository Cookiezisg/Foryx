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

const todoUpdateDescription = `Update a todo's status or other fields. Omitted fields stay as-is. ` + "`status`" + ` transitions: pending → in_progress → completed; use "deleted" to remove a todo. ` + "`blocked_by`" + ` replaces the entire dependency list (pass [] to clear). Returns the updated todo as JSON, or ` + "`{deleted, id}`" + ` for a deletion.`

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
// TodoUpdate 是 TodoUpdate 系统工具的实现。
type TodoUpdate struct {
	svc *todoapp.Service
}

func (t *TodoUpdate) Name() string                { return "TodoUpdate" }
func (t *TodoUpdate) Description() string         { return todoUpdateDescription }
func (t *TodoUpdate) Parameters() json.RawMessage { return todoUpdateSchema }

func (t *TodoUpdate) IsReadOnly() bool        { return false }
func (t *TodoUpdate) NeedsReadFirst() bool    { return false }
func (t *TodoUpdate) RequiresWorkspace() bool { return false }

// ValidateInput rejects empty todo_id and status outside the whitelist.
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

type updateRaw struct {
	TodoID      string    `json:"todo_id"`
	Subject     *string   `json:"subject"`
	Description *string   `json:"description"`
	ActiveForm  *string   `json:"active_form"`
	Status      *string   `json:"status"`
	Owner       *string   `json:"owner"`
	BlockedBy   *[]string `json:"blocked_by"`
}

// Execute applies the partial update; status:"deleted" routes to Service.Delete.
//
// Execute 应用部分更新；status:"deleted" 走 Service.Delete。
func (t *TodoUpdate) Execute(ctx context.Context, argsJSON string) (string, error) {
	var raw updateRaw
	if err := json.Unmarshal([]byte(argsJSON), &raw); err != nil {
		return "", fmt.Errorf("TodoUpdate.Execute: %w", err)
	}

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

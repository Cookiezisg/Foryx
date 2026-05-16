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

const todoCreateDescription = `Create a new todo on the current conversation's todo list. New todos start with status "pending"; move them via TodoUpdate. Returns the new todo as JSON including the assigned id (use that for follow-up TodoUpdate calls).`

var todoCreateSchema = json.RawMessage(`{
	"type": "object",
	"required": ["subject"],
	"properties": {
		"subject": {
			"type": "string",
			"description": "Imperative one-line title (e.g. \"Run tests\")."
		},
		"description": {
			"type": "string",
			"description": "Longer note for context."
		},
		"active_form": {
			"type": "string",
			"description": "Present continuous form (e.g. \"Running tests\")."
		},
		"blocked_by": {
			"type": "array",
			"items": {"type": "string"},
			"description": "Todo IDs that must complete before this one starts."
		}
	}
}`)

// TodoCreate implements the TodoCreate system tool.
//
// TodoCreate 是 TodoCreate 系统工具的实现。
type TodoCreate struct {
	svc *todoapp.Service
}

func (t *TodoCreate) Name() string                { return "TodoCreate" }
func (t *TodoCreate) Description() string         { return todoCreateDescription }
func (t *TodoCreate) Parameters() json.RawMessage { return todoCreateSchema }

func (t *TodoCreate) IsReadOnly() bool        { return false }
func (t *TodoCreate) NeedsReadFirst() bool    { return false }
func (t *TodoCreate) RequiresWorkspace() bool { return false }

// ValidateInput rejects empty subject pre-Execute.
//
// ValidateInput 在 Execute 前拒绝空 subject。
func (t *TodoCreate) ValidateInput(args json.RawMessage) error {
	var a struct {
		Subject string `json:"subject"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("TodoCreate.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.Subject) == "" {
		return tododomain.ErrSubjectRequired
	}
	return nil
}

func (t *TodoCreate) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

// Execute creates the todo via Service and returns the new entity as JSON.
//
// Execute 通过 Service 创建 todo，返新 entity 的 JSON。
func (t *TodoCreate) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Subject     string   `json:"subject"`
		Description string   `json:"description"`
		ActiveForm  string   `json:"active_form"`
		BlockedBy   []string `json:"blocked_by"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("TodoCreate.Execute: %w", err)
	}
	created, err := t.svc.Create(ctx, todoapp.CreateInput{
		Subject:     args.Subject,
		Description: args.Description,
		ActiveForm:  args.ActiveForm,
		BlockedBy:   args.BlockedBy,
	})
	if err != nil {
		return classifyTodoErr(err, "create"), nil
	}
	return marshalIndent(created)
}


// classifyTodoErr converts Service errors to LLM-friendly strings; sentinels become recoverable hints.
//
// classifyTodoErr 把 Service 错转友好字符串；sentinel 给可恢复提示。
func classifyTodoErr(err error, op string) string {
	switch {
	case errors.Is(err, tododomain.ErrNotFound):
		return "Todo not found in this conversation."
	case errors.Is(err, tododomain.ErrSubjectRequired):
		return "Todo subject is required and must be non-empty."
	case errors.Is(err, tododomain.ErrInvalidStatus):
		return "Invalid status. Allowed: pending, in_progress, completed, deleted."
	}
	return fmt.Sprintf("Todo %s failed: %s", op, err.Error())
}

func marshalIndent(v any) (string, error) {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}
	return string(body), nil
}

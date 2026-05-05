// create.go — TodoCreate system tool: add a new todo to the current
// conversation's todo list. Returns the freshly-minted Todo as JSON so
// the LLM has the assigned ID handy for follow-up TodoUpdate calls.
//
// create.go — TodoCreate 系统工具：往当前对话的 TODO 列表加一条；
// 返回新铸 Todo 的 JSON，让 LLM 后续 TodoUpdate 时直接用上分配的 ID。
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

const todoCreateDescription = `Create a new todo on the current conversation's todo list.

Usage:
- Use this when planning multi-step work the user can watch progress on.
- ` + "`subject`" + ` is the imperative verb-first title (e.g. "Run tests", "Fix login bug").
- ` + "`description`" + ` (optional) is a longer note for context.
- ` + "`active_form`" + ` (optional) is the present-continuous form shown in the UI's "in_progress" spinner (e.g. "Running tests").
- ` + "`blocked_by`" + ` (optional) is a list of todo IDs that must complete before this one can start.
- New todos start in status "pending". Use TodoUpdate to move them to "in_progress" / "completed".
- The returned JSON includes the assigned todo ID — keep it for follow-up TodoUpdate calls.`

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
// TodoCreate struct 是 TodoCreate 系统工具。
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

// Execute creates the todo via Service and returns the new entity as
// indented JSON.
//
// Execute 通过 Service 创建 todo，返新 entity 的缩进 JSON。
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

// ── shared helpers ───────────────────────────────────────────────────────────

// classifyTodoErr converts a Service error into an LLM-friendly string.
// Sentinels become recoverable hints; anything else surfaces with a
// generic prefix so the LLM doesn't latch onto wrapping noise.
//
// classifyTodoErr 把 Service 错转友好字符串。Sentinel 给可恢复提示；其他
// 走通用前缀，避免 LLM 抓到包装噪声。
func classifyTodoErr(err error, op string) string {
	switch {
	case errors.Is(err, tododomain.ErrNotFound):
		return "Todo not found in this conversation."
	case errors.Is(err, tododomain.ErrSubjectRequired):
		return "Todo subject is required and must be non-empty."
	case errors.Is(err, tododomain.ErrInvalidStatus):
		return "Invalid status. Allowed: pending, in_progress, completed, deleted."
	}
	return fmt.Sprintf("Todo %s failed: %v", op, err)
}

// marshalIndent emits the entity as pretty-printed JSON the LLM can
// quote back if useful.
//
// marshalIndent 输出 entity 的缩进 JSON，方便 LLM 引用。
func marshalIndent(v any) (string, error) {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}
	return string(body), nil
}

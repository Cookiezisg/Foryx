// Package ask provides the AskUserQuestion system tool that pauses the LLM agent loop until the user answers.
//
// Package ask 提供 AskUserQuestion 系统工具：暂停 LLM agent 循环直到用户回答。
package ask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	askapp "github.com/sunweilin/forgify/backend/internal/app/ask"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// defaultTimeout is a 7-day zombie guard; UX should not depend on it firing.
//
// defaultTimeout 是 7 天 zombie 守卫，真用户 UX 不依赖于其触发。
const defaultTimeout = 7 * 24 * time.Hour

var (
	// ErrEmptyQuestion: question missing or empty.
	//
	// ErrEmptyQuestion：question 缺失或为空。
	ErrEmptyQuestion = errors.New("question is required and must be non-empty")
)

const askDescription = `Pause the agent loop and ask the user a question. Returns the user's answer as free-form text.

WHEN TO USE:
- Use when you genuinely need user input that you can't infer.
- For open-ended questions (e.g. "what's your account name?"), leave ` + "`options`" + ` empty — the UI shows a free-form input.
- For structured choice (e.g. "which DB are you using?"), provide ` + "`options`" + ` as quick-pick buttons. Users may still type a free-form answer instead.
- If the user "skips" (clicks the skip button on the frontend), you'll get the literal string "(user skipped)" back — treat it as "user wants you to continue with reasonable defaults".

The tool blocks until the user responds (no practical timeout — backend uses 7 days as a zombie guard, not a UX deadline).`

var askSchema = json.RawMessage(`{
	"type": "object",
	"required": ["question"],
	"properties": {
		"question": {
			"type": "string",
			"description": "The question text shown to the user. Be concise — one short paragraph."
		},
		"options": {
			"type": "array",
			"items": {"type": "string"},
			"description": "OPTIONAL. List of suggested quick-pick answers. Leave empty / omit for open-ended questions where you want a free-form reply. The user is never restricted to these — they may type any reply or click 'skip'."
		}
	}
}`)

// AskUserQuestion implements the AskUserQuestion system tool.
//
// AskUserQuestion 是 AskUserQuestion 系统工具的实现。
type AskUserQuestion struct {
	svc     *askapp.Service
	timeout time.Duration
}

// AskTools constructs the ask system tools sharing one Service.
//
// AskTools 用一个 Service 构造 ask 系统工具。
func AskTools(svc *askapp.Service) []toolapp.Tool {
	return []toolapp.Tool{
		&AskUserQuestion{svc: svc, timeout: defaultTimeout},
	}
}

func (t *AskUserQuestion) Name() string                { return "AskUserQuestion" }
func (t *AskUserQuestion) Description() string         { return askDescription }
func (t *AskUserQuestion) Parameters() json.RawMessage { return askSchema }

func (t *AskUserQuestion) IsReadOnly() bool        { return true }
func (t *AskUserQuestion) NeedsReadFirst() bool    { return false }
func (t *AskUserQuestion) RequiresWorkspace() bool { return false }

// ValidateInput rejects empty question pre-Execute.
//
// ValidateInput 在 Execute 前拒绝空 question。
func (t *AskUserQuestion) ValidateInput(args json.RawMessage) error {
	var a struct {
		Question string `json:"question"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("AskUserQuestion.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.Question) == "" {
		return ErrEmptyQuestion
	}
	return nil
}

func (t *AskUserQuestion) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

// Execute registers the pending question on the Service and blocks until answer or timeout.
//
// Execute 注册 pending 问题到 Service 并阻塞直到答案到达或超时。
func (t *AskUserQuestion) Execute(ctx context.Context, argsJSON string) (string, error) {
	callID, _ := reqctxpkg.GetToolCallID(ctx)
	if callID == "" {
		return "Cannot ask the user: tool runtime is not properly initialized.", nil
	}
	answer, err := t.svc.Wait(ctx, callID, t.timeout)
	switch {
	case errors.Is(err, askapp.ErrTimeout):
		return "User did not respond within the timeout.", nil
	case errors.Is(err, context.Canceled):
		return "Question cancelled by the user.", nil
	case err != nil:
		return fmt.Sprintf("Asking the user failed: %s", err.Error()), nil
	}
	return answer, nil
}

var _ toolapp.Tool = (*AskUserQuestion)(nil)

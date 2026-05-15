// Package ask provides the AskUserQuestion system tool. It pauses the
// LLM agent loop until the user answers via the
// POST /api/v1/conversations/{id}/answers endpoint, then returns the
// answer as the tool's tool_result so the LLM can continue.
//
// Imported as `asktool` per §S13 nested sub-package alias rule.
//
// The question itself ships with the standard chat.message SSE stream —
// the AskUserQuestion tool_call block carries `question` and optional
// `options` in its arguments map, and the UI renders the prompt off
// that. Decision D11: no separate event family.
//
// Package ask 提供 AskUserQuestion 系统工具：暂停 LLM agent 循环，直到
// 用户经 POST /api/v1/conversations/{id}/answers 回答，然后把答案作为
// tool_result 返回让 LLM 继续。
//
// 按 §S13 嵌套子包别名规则导入为 `asktool`。
//
// 问题本身坐 chat.message SSE 流——AskUserQuestion tool_call block 的
// arguments 里带 `question` 与可选 `options`，UI 据此渲染。决策 D11：
// 不新建事件家族。
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

// ── Limits & defaults ─────────────────────────────────────────────────────────

const (
	// defaultTimeout is the wall-clock the tool blocks waiting for an
	// answer. 2026-05 #6 redesign: 7 days as a "basically forever" zombie
	// guard. Chat UX should NOT depend on this firing — user might step
	// away for hours / overnight, that's fine. The frontend's Composer
	// state machine (Wails / V2 testend) handles "show pending awareness"
	// + smart-route incoming messages as answers.
	//
	// 2026-05 #6 重构:超时 5min → 7 天(基本=不限,只防 zombie 进程)。
	// 真用户 UX 不该依赖超时——用户可能离屏过夜。前端 Composer 状态机
	// 负责"等答题"提示 + 自动 route 新消息当答案。
	defaultTimeout = 7 * 24 * time.Hour
)

// ── Validation sentinels ──────────────────────────────────────────────────────

var (
	// ErrEmptyQuestion: question missing or empty.
	// ErrEmptyQuestion：question 缺失或为空。
	ErrEmptyQuestion = errors.New("question is required and must be non-empty")
)

// ── Description & schema ──────────────────────────────────────────────────────

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

// ── Tool struct & 9 methods ───────────────────────────────────────────────────

// AskUserQuestion implements the AskUserQuestion system tool.
//
// AskUserQuestion struct 是 AskUserQuestion 系统工具。
type AskUserQuestion struct {
	svc     *askapp.Service
	timeout time.Duration // overridable for tests
}

// AskTools constructs the ask system tools sharing one Service.
//
// AskTools 用一个 Service 构造 ask 系统工具。
func AskTools(svc *askapp.Service) []toolapp.Tool {
	return []toolapp.Tool{
		&AskUserQuestion{svc: svc, timeout: defaultTimeout},
	}
}

// Identity --------------------------------------------------------------------

func (t *AskUserQuestion) Name() string                { return "AskUserQuestion" }
func (t *AskUserQuestion) Description() string         { return askDescription }
func (t *AskUserQuestion) Parameters() json.RawMessage { return askSchema }

// Static metadata -------------------------------------------------------------

func (t *AskUserQuestion) IsReadOnly() bool        { return true }
func (t *AskUserQuestion) NeedsReadFirst() bool    { return false }
func (t *AskUserQuestion) RequiresWorkspace() bool { return false }

// ── Args-dependent hooks ──────────────────────────────────────────────────────

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

// ── Execute ───────────────────────────────────────────────────────────────────

// Execute pulls the LLM-assigned tool_call_id from ctx, registers a
// pending question with the Service, and blocks until either the answer
// arrives or the timeout elapses. The chat.message SSE stream already
// carries the tool_call block (with `question` + `options` in
// arguments), so the UI sees the prompt without any new event type.
//
// Failure paths return LLM-friendly strings (not Go errors) so the LLM
// can read the situation and reword / retry.
//
// Execute 从 ctx 取 LLM 分配的 tool_call_id，注册 pending 问题，阻塞
// 直至答案到达或超时。chat.message SSE 已携带 tool_call block（arguments
// 含 question/options），UI 无需新事件就能看到提示。
//
// 失败路径返友好字符串（非 Go err），让 LLM 看清情况并改述 / 重试。
func (t *AskUserQuestion) Execute(ctx context.Context, argsJSON string) (string, error) {
	callID, _ := reqctxpkg.GetToolCallID(ctx)
	if callID == "" {
		// Caller-side defect (no tool_call_id in ctx). Keep the LLM-
		// facing text generic — the LLM cannot do anything about it,
		// and operator sees the actual stack via the executeTool warn log.
		// 调用方 defect（ctx 缺 tool_call_id）；LLM 无法处理，保持通用
		// 文本，operator 可经 executeTool warn log 看到栈。
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

// ── Compile-time checks ───────────────────────────────────────────────────────

var _ toolapp.Tool = (*AskUserQuestion)(nil)

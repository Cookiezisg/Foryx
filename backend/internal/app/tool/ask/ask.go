// Package ask is the ask_user tool (R0064): it lets an agent pause and ask the human a question
// when it needs information or a decision only the user can give. It is an ordinary tool — no
// special loop support — whose Execute blocks on the humanloop broker until the user answers, then
// returns that answer as the tool_result. In a non-interactive context (no broker in ctx — a
// workflow / sensor run) it reports that asking isn't available, so the model adapts.
//
// Package ask 是 ask_user 工具（R0064）：当 agent 需要只有用户能给的信息或决定时，让它暂停发问。它是个普通工具
// ——无特殊 loop 支持——其 Execute 阻塞在 humanloop broker 上直到用户回答，再把答案当 tool_result 返回。在非交互
// 语境（ctx 无 broker——workflow / sensor 运行）它报告无法发问，使模型自适应。
package ask

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	humanloopapp "github.com/sunweilin/forgify/backend/internal/app/humanloop"
	errorspkg "github.com/sunweilin/forgify/backend/internal/pkg/errors"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Input sentinels — errorspkg.New like every sentinel; surfaced to the LLM as a tool-result string.
//
// 输入 sentinel——同所有 sentinel 用 errorspkg.New；经 tool-result 串给 LLM。
var (
	ErrMessageRequired   = errorspkg.New(errorspkg.KindInvalid, "ASK_MESSAGE_REQUIRED", "message is required")
	ErrNoInteractiveUser = errorspkg.New(errorspkg.KindUnavailable, "ASK_NO_INTERACTIVE_USER", "ask_user is only available in an interactive conversation; proceed without asking")
)

// Tool is the ask_user tool. It carries no state — the broker arrives via ctx.
//
// Tool 是 ask_user 工具。无状态——broker 经 ctx 到达。
type Tool struct{}

func New() *Tool { return &Tool{} }

func (*Tool) Name() string { return "ask_user" }

func (*Tool) Description() string {
	return "Ask the user a question and wait for their answer. Use this only when you genuinely need " +
		"information or a decision that only the user can provide — do not use it for things you can " +
		"figure out yourself. Provide `options` to offer a multiple-choice prompt. Returns the user's answer."
}

func (*Tool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"message": {"type": "string", "description": "The question to ask the user."},
			"options": {"type": "array", "items": {"type": "string"}, "description": "Optional choices to offer."}
		},
		"required": ["message"]
	}`)
}

type input struct {
	Message string   `json:"message"`
	Options []string `json:"options,omitempty"`
}

func (*Tool) ValidateInput(raw json.RawMessage) error {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return fmt.Errorf("ask_user.ValidateInput: %w", err)
	}
	if strings.TrimSpace(in.Message) == "" {
		return ErrMessageRequired
	}
	return nil
}

// Execute surfaces the question via the humanloop broker and BLOCKS until the user accepts (→ the
// answer) or declines (→ a re-route hint). The broker flows in through ctx; without it asking has
// no audience. A cancelled run unblocks with the ctx error.
//
// Execute 经 humanloop broker 露出问题并**阻塞**至用户 accept（→答案）或 decline（→改道提示）。broker 经 ctx
// 流入；没有它则发问无对象。运行被取消时以 ctx 错解阻。
func (*Tool) Execute(ctx context.Context, args string) (string, error) {
	var in input
	_ = json.Unmarshal([]byte(args), &in)

	broker := humanloopapp.From(ctx)
	if broker == nil {
		// Non-interactive run (workflow / sensor / standalone): there is no user to ask.
		// 非交互运行（workflow / sensor / standalone）：无用户可问。
		return "", ErrNoInteractiveUser
	}

	convID, _ := reqctxpkg.GetConversationID(ctx)
	tcID, _ := reqctxpkg.GetToolCallID(ctx)
	prompt, _ := json.Marshal(in)

	resp, err := broker.Request(ctx, humanloopapp.Request{
		ToolCallID:     tcID,
		Kind:           humanloopapp.KindAsk,
		Tool:           "ask_user",
		ConversationID: convID,
		Prompt:         prompt,
	})
	if err != nil {
		return "", err // ctx cancelled — the run is aborting
	}
	// Fail-safe: only an explicit accept yields the answer; decline / anything else re-routes.
	//
	// fail-safe：只有显式 accept 才给答案；decline / 其它都改道。
	if resp.Action != humanloopapp.DecisionAccept {
		return humanloopapp.DeclineFeedback, nil
	}
	answer := strings.TrimSpace(resp.Answer)
	if answer == "" {
		return "(the user submitted an empty answer)", nil
	}
	return answer, nil
}

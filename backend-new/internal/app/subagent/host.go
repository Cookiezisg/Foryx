package subagent

import (
	"context"

	"go.uber.org/zap"

	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// subagentHost is one Spawn's loop.Host — a hybrid: like agentHost its history is just the task
// prompt and its tools are a fixed whitelist, but like chatHost its WriteFinalize persists the
// turn (a sub-message tagged SubagentID) and pushes message_stop, on a detached context. It does
// NOT implement AutoActivator / ReminderProvider / StepRecorder (static tools, no live todo, no
// durable replay).
//
// subagentHost 是一次 Spawn 的 loop.Host——混血：像 agentHost 其历史就是任务 prompt、工具是固定白
// 名单，但像 chatHost 其 WriteFinalize 落盘回合（带 SubagentID 的 sub-message）+ 推 message_stop、
// 在 detached context 上。不实现 AutoActivator / ReminderProvider / StepRecorder（静态工具、无 live
// todo、无持久重放）。
type subagentHost struct {
	svc            *Service
	conversationID string
	subMsg         *messagesdomain.Message // mutated + persisted by WriteFinalize
	userPrompt     string
	systemPrompt   string
	tools          []toolapp.Tool
}

var _ loopapp.Host = (*subagentHost)(nil)

// LoadHistory seeds the loop with just the task prompt (an isolated run — no parent thread).
//
// LoadHistory 只用任务 prompt 起始（隔离运行——无父线程）。
func (h *subagentHost) LoadHistory(_ context.Context) ([]llminfra.LLMMessage, error) {
	return []llminfra.LLMMessage{{Role: llminfra.RoleUser, Content: h.userPrompt}}, nil
}

// Tools returns the type-filtered static whitelist (no lazy / search_tools dance — a subagent is
// focused and short-lived).
//
// Tools 返回按类型过滤的静态白名单（无 lazy / search_tools 周旋——subagent 聚焦短命）。
func (h *subagentHost) Tools(_ context.Context) []toolapp.Tool { return h.tools }

// WriteFinalize lands the subagent's turn as a sub-message (SubagentID already set) with its
// blocks, and pushes message_stop. Detached (background + re-seeded workspace/conversation) for
// the same orphan-avoidance reason as chat: a cancelled subagent must still reach a terminal
// state. The final answer (result.LastMessage) is returned by Spawn and becomes the spawning
// tool_call's tool_result — that, not this sub-message, is what the parent's LLM sees.
//
// WriteFinalize 把 subagent 回合作为 sub-message（SubagentID 已设）连同 blocks 落盘、推 message_stop。
// detached（background + 重埋 workspace/conversation），与 chat 同样防孤儿：被取消的 subagent 仍须
// 抵达终态。最终答案（result.LastMessage）由 Spawn 返回、成为派它的 tool_call 的 tool_result——父的
// LLM 看的是那个、而非这条 sub-message。
func (h *subagentHost) WriteFinalize(ctx context.Context, blocks []messagesdomain.Block, status, stopReason, errCode, errMsg string, in, out int) {
	wsID, _ := reqctxpkg.GetWorkspaceID(ctx)
	dctx := reqctxpkg.SetWorkspaceID(context.Background(), wsID)
	dctx = reqctxpkg.SetConversationID(dctx, h.conversationID)

	h.subMsg.Status = status
	h.subMsg.StopReason = stopReason
	h.subMsg.ErrorCode = errCode
	h.subMsg.ErrorMessage = errMsg
	h.subMsg.InputTokens = in
	h.subMsg.OutputTokens = out

	if err := h.svc.deps.Messages.FinalizeMessage(dctx, h.subMsg, blocks); err != nil {
		h.svc.log.Warn("subagentapp.WriteFinalize: persist failed",
			zap.String("subMessageId", h.subMsg.ID), zap.Error(err))
	}
	h.svc.emitMessageStop(dctx, h.conversationID, h.subMsg)
}

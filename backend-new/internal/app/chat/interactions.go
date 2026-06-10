package chat

import (
	"context"
	"strings"

	"go.uber.org/zap"

	entitystreamapp "github.com/sunweilin/forgify/backend/internal/app/entitystream"
	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Resolution verbs live in loopapp (the shared HITL protocol home); re-exported here so chat code
// reads naturally.
//
// 决议动词在 loopapp（共享 HITL 协议的家）；此处转出使 chat 代码读着自然。
const (
	ResolveApprove = loopapp.ResolveApprove
	ResolveDeny    = loopapp.ResolveDeny
	ResolveAccept  = loopapp.ResolveAccept
	ResolveDecline = loopapp.ResolveDecline
	ResolveCancel  = loopapp.ResolveCancel
)

// Errors for the resolve path (bubble to HTTP via errorsdomain → Kind→status + wire code).
//
// resolve 路径错误（经 errorsdomain 冒泡 HTTP → Kind→status + wire code）。
var (
	ErrNoPendingInteraction = errorsdomain.New(errorsdomain.KindNotFound, "NO_PENDING_INTERACTION", "no pending interaction with that tool call id in this conversation")
	ErrInteractionPending   = errorsdomain.New(errorsdomain.KindConflict, "INTERACTION_PENDING", "this conversation is waiting on a pending interaction; resolve it before sending")
	ErrBadResolveAction     = errorsdomain.New(errorsdomain.KindInvalid, "BAD_RESOLVE_ACTION", "unknown resolve action for this interaction kind")
)

// allowsToolForConversation reports whether the conversation has session-whitelisted a tool
// (always-allow), so the danger park is skipped. D1 has no whitelist (always false → every
// dangerous call parks); D4 wires the per-conversation set populated by the approve-always action.
//
// allowsToolForConversation 报告本对话是否会话白名单了某工具（always-allow）以跳过 danger park。D1 无白名单
// （恒 false → 每个危险调用都 park）；D4 接由 approve-always 动作填充的 per-conversation 集合。
func (s *Service) allowsToolForConversation(_ /*conversationID*/, _ /*name*/ string) bool {
	return false
}

// ResolveInteraction resolves one pending human interaction in a conversation's parked turn
// (R0064): it fills the pending tool_result for toolCallID per the action, and — once the parked
// turn has no other pending interaction — flips it to completed (or cancelled) and drives a
// continuation turn. Resolution-as-data-into-a-continuation is the industry-standard durable-HITL
// resume (Temporal signal / Restate awakeable / OpenAI state.approve); execute-then-record the
// approved danger tool so the continuation re-reads it from history rather than re-executing.
//
// ResolveInteraction 决议一个对话 parked 回合里的一条待决人机交互（R0064）：按动作填 toolCallID 的 pending
// tool_result，待该 parked 回合无其它待决交互时翻为 completed（或 cancelled）并驱动续跑回合。决议作为 data 注入
// 续跑是业界标准 durable-HITL resume；批准的 danger 工具 execute-then-record，使续跑从历史重读而非重执行。
func (s *Service) ResolveInteraction(ctx context.Context, conversationID, toolCallID, action, answer string) error {
	parked, err := s.messages.GetParkedMessage(ctx, conversationID)
	if err != nil {
		return ErrNoPendingInteraction
	}
	tcBlock, pending := findInteraction(parked, toolCallID)
	if tcBlock == nil || pending == nil {
		return ErrNoPendingInteraction
	}
	kind, _ := pending.Attrs["park"].(string)

	// Cancel abandons the whole turn: fill EVERY pending result as cancelled, flip the turn to
	// cancelled, no continuation (mirrors the existing per-turn Cancel).
	//
	// Cancel 放弃整个回合：把所有 pending 填为 cancelled、回合翻 cancelled、不续跑（镜像现有逐回合 Cancel）。
	if action == ResolveCancel {
		for i := range parked.Blocks {
			b := &parked.Blocks[i]
			if b.Type == messagesdomain.BlockTypeToolResult && b.Status == messagesdomain.StatusPending {
				const cancelled = "The user cancelled this request."
				_ = s.messages.ResolveToolResult(ctx, b.ID, cancelled, messagesdomain.StatusCancelled, "")
				s.closeInteractionNode(ctx, conversationID, b.ID, cancelled, messagesdomain.StatusCancelled)
			}
		}
		return s.messages.SetMessageStatus(ctx, parked.ID, messagesdomain.StatusCancelled, messagesdomain.StopReasonCancelled)
	}

	content, status, errMsg, err := s.resolveOne(ctx, conversationID, parked.ID, kind, action, answer, tcBlock)
	if err != nil {
		return err
	}
	if err := s.messages.ResolveToolResult(ctx, pending.ID, content, status, errMsg); err != nil {
		return err
	}
	s.closeInteractionNode(ctx, conversationID, pending.ID, content, status)

	// More pending in this turn (a step with several gated calls)? Wait for them all.
	//
	// 本回合还有 pending（一步多个被门调用）？等全部决议。
	if hasOtherPending(parked, pending.ID) {
		return nil
	}
	// All resolved → the turn is no longer parked; drive the continuation.
	//
	// 全决议 → 回合不再 parked；驱动续跑。
	if err := s.messages.SetMessageStatus(ctx, parked.ID, messagesdomain.StatusCompleted, messagesdomain.StopReasonEndTurn); err != nil {
		return err
	}
	s.driveContinuation(ctx, conversationID)
	return nil
}

// resolveOne computes the (content, status, error) for one interaction's tool_result. danger:
// approve runs the gated tool now (execute-then-record), deny feeds the denial back. ask: D2.
//
// resolveOne 算一条交互 tool_result 的 (content, status, error)。danger：approve 此刻跑被门工具
// （execute-then-record），deny 把拒绝反馈回模型。ask：D2。
func (s *Service) resolveOne(ctx context.Context, conversationID, parkedMsgID, kind, action, answer string, tcBlock *messagesdomain.Block) (content, status, errMsg string, err error) {
	switch kind {
	case loopapp.ParkKindDanger:
		switch action {
		case ResolveApprove:
			out, exErr := s.executeApprovedTool(ctx, conversationID, parkedMsgID, tcBlock)
			if exErr != "" {
				return out, messagesdomain.StatusError, exErr, nil
			}
			return out, messagesdomain.StatusCompleted, "", nil
		case ResolveDeny:
			return "The user denied running this tool. Do not retry it unless the user explicitly asks.", messagesdomain.StatusCompleted, "", nil
		}
	case loopapp.ParkKindAsk:
		switch action {
		case ResolveAccept:
			ans := strings.TrimSpace(answer)
			if ans == "" {
				ans = "(the user submitted an empty answer)"
			}
			return ans, messagesdomain.StatusCompleted, "", nil
		case ResolveDecline:
			return "The user declined to answer this question. Proceed without it or ask differently.", messagesdomain.StatusCompleted, "", nil
		}
	}
	return "", "", "", ErrBadResolveAction
}

// executeApprovedTool runs the gated tool at resolve time on a detached context that keeps the
// workspace + nests progress under the original tool_call (the messages + entities bridges stream
// it live). The stripped args ride the persisted tool_call block's Content. Returns (output,
// errMsg); errMsg != "" → the call failed (recorded as an error tool_result, the model adapts).
//
// executeApprovedTool 在 resolve 时跑被门工具，用保留 workspace 的 detached ctx，并把进度嵌在原 tool_call 下
// （messages + entities bridge 实时流）。剥离后的 args 随持久化的 tool_call 块 Content。返 (output, errMsg)；
// errMsg != "" → 调用失败（记为 error tool_result，模型自适应）。
func (s *Service) executeApprovedTool(ctx context.Context, conversationID, parkedMsgID string, tcBlock *messagesdomain.Block) (string, string) {
	name, _ := tcBlock.Attrs["tool"].(string)
	tool := s.findTool(name)
	if tool == nil {
		return "", "approved tool " + name + " is not available"
	}
	wsID, _ := reqctxpkg.GetWorkspaceID(ctx)
	ectx := reqctxpkg.SetWorkspaceID(context.Background(), wsID)
	ectx = reqctxpkg.SetConversationID(ectx, conversationID)
	ectx = reqctxpkg.SetMessageID(ectx, parkedMsgID)
	ectx = reqctxpkg.SetToolCallID(ectx, tcBlock.ID)
	ectx = loopapp.WithBridge(ectx, s.deps.Bridge)
	ectx = entitystreamapp.WithBridge(ectx, s.deps.EntitiesBridge)
	out, err := tool.Execute(ectx, tcBlock.Content)
	if err != nil {
		if out != "" {
			return out, err.Error()
		}
		return err.Error(), err.Error()
	}
	return out, ""
}

// driveContinuation opens a fresh assistant turn and enqueues it: a vanilla loop.Run that re-reads
// the now-resolved history (the filled tool_result memoized in it) and continues the ReAct. A
// best-effort enqueue — a full queue (a racing Send) just drops the continuation; the parked turn
// is already resolved so nothing is lost but the auto-resume.
//
// driveContinuation 开一个新 assistant 回合并入队：vanilla loop.Run 重读已决议的历史（填好的 tool_result 在其中
// 记忆化）继续 ReAct。best-effort 入队——队列满（竞态 Send）就丢续跑；parked 回合已决议、除自动续跑外无损失。
func (s *Service) driveContinuation(ctx context.Context, conversationID string) {
	wsID, _ := reqctxpkg.GetWorkspaceID(ctx)
	asstMsg := &messagesdomain.Message{
		ID:             idgenpkg.New("msg"),
		ConversationID: conversationID,
		Role:           messagesdomain.RoleAssistant,
		Status:         messagesdomain.StatusStreaming,
	}
	dctx := reqctxpkg.SetWorkspaceID(context.Background(), wsID)
	dctx = reqctxpkg.SetConversationID(dctx, conversationID)
	if err := s.messages.CreateMessage(dctx, asstMsg, nil); err != nil {
		s.log.Warn("chatapp.driveContinuation: open turn failed", zap.String("conversationId", conversationID), zap.Error(err))
		return
	}
	s.emitMessageStart(dctx, conversationID, asstMsg.ID)
	t := task{assistantMsgID: asstMsg.ID, workspaceID: wsID, locale: reqctxpkg.GetLocale(ctx)}
	if err := s.enqueue(conversationID, t); err != nil {
		asstMsg.Status = messagesdomain.StatusError
		asstMsg.StopReason = messagesdomain.StopReasonError
		asstMsg.ErrorCode = "STREAM_IN_PROGRESS"
		_ = s.messages.FinalizeMessage(dctx, asstMsg, nil)
	}
}

// findTool resolves a tool by name across the resident + lazy sets (for execute-at-resolve, which
// runs outside the loop's per-step Tools()).
//
// findTool 在 resident + lazy 集合里按名解析工具（供 execute-at-resolve，它在 loop 的每步 Tools() 之外跑）。
func (s *Service) findTool(name string) toolapp.Tool {
	ts := s.deps.Toolset
	for _, t := range ts.Resident {
		if t.Name() == name {
			return t
		}
	}
	for _, t := range ts.Lazy {
		if t.Name() == name {
			return t
		}
	}
	if s.searchTool != nil && s.searchTool.Name() == name {
		return s.searchTool
	}
	return nil
}

// findInteraction locates the tool_call block (id == toolCallID) and its pending tool_result child
// in a parked turn.
//
// findInteraction 在 parked 回合里定位 tool_call 块（id == toolCallID）及其 pending tool_result 子块。
func findInteraction(m *messagesdomain.Message, toolCallID string) (tcBlock, pending *messagesdomain.Block) {
	for i := range m.Blocks {
		b := &m.Blocks[i]
		if b.ID == toolCallID && b.Type == messagesdomain.BlockTypeToolCall {
			tcBlock = b
		}
		if b.ParentBlockID == toolCallID && b.Type == messagesdomain.BlockTypeToolResult && b.Status == messagesdomain.StatusPending {
			pending = b
		}
	}
	return
}

// hasOtherPending reports whether the turn has a pending tool_result other than exceptID (the
// in-memory copy of the just-resolved one still reads pending).
//
// hasOtherPending 报告回合是否有除 exceptID 外的 pending tool_result（刚决议那条的内存副本仍读 pending）。
func hasOtherPending(m *messagesdomain.Message, exceptID string) bool {
	for i := range m.Blocks {
		b := &m.Blocks[i]
		if b.Type == messagesdomain.BlockTypeToolResult && b.Status == messagesdomain.StatusPending && b.ID != exceptID {
			return true
		}
	}
	return false
}

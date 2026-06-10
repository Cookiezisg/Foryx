package chat

import (
	"context"
	"time"

	"go.uber.org/zap"

	entitystreamapp "github.com/sunweilin/forgify/backend/internal/app/entitystream"
	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// idleTimeout reclaims a conversation's drain goroutine + queue after this long with no task, so
// dormant conversations cost nothing. A new Send re-creates the queue on demand.
//
// idleTimeout 在无任务这么久后回收对话的抽取 goroutine + 队列，使休眠对话零成本。新 Send 按需重建队列。
const idleTimeout = 5 * time.Minute

// runQueue is the conversation's single drain goroutine — it serializes generations (one
// assistant turn at a time, which makes per-conversation block seq allocation race-free). It
// self-destructs after idleTimeout with no task, deregistering from s.queues; a task that races
// in during teardown re-registers the queue and keeps it alive.
//
// runQueue 是对话的单抽取 goroutine——串行化生成（同时一个 assistant 回合，这使 per-conversation
// block seq 分配无竞争）。无任务 idleTimeout 后自毁、从 s.queues 注销；拆卸期竞态进来的任务会重新
// 注册队列并保活。
func (s *Service) runQueue(conversationID string, q *convQueue) {
	defer s.wg.Done()
	idle := time.NewTimer(idleTimeout)
	defer idle.Stop()

	for {
		select {
		case t := <-q.ch:
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			s.processTask(conversationID, q, t)
			idle.Reset(idleTimeout)

		case <-idle.C:
			s.queues.Delete(conversationID)
			// A task may have been offered between the timer firing and the Delete; if so,
			// re-register and serve it. Otherwise the goroutine exits.
			//
			// 任务可能在 timer 触发与 Delete 之间被投递；若是，重新注册并服务。否则 goroutine 退出。
			select {
			case t := <-q.ch:
				s.queues.Store(conversationID, q)
				s.processTask(conversationID, q, t)
				idle.Reset(idleTimeout)
			default:
				return
			}
		}
	}
}

// processTask runs one assistant generation. It rebuilds a fresh context (the Send context is
// long gone) carrying the per-run identity + AgentState + the live stream bridge + a cancel the
// cancel endpoint (R0056) can trigger, resolves the conversation's model, builds the system
// prompt, and runs the ReAct loop. The host's WriteFinalize persists + streams the terminal turn,
// so processTask discards the loop Result.
//
// processTask 跑一次 assistant 生成。它重建新 context（Send context 早已消失），携带 per-run 身份 +
// AgentState + live 流桥 + cancel 端点（R0056）可触发的 cancel，解析对话模型、拼 system prompt、跑
// ReAct 循环。host 的 WriteFinalize 落盘 + 推流终态，故 processTask 丢弃 loop Result。
func (s *Service) processTask(conversationID string, q *convQueue, t task) {
	base := reqctxpkg.SetWorkspaceID(context.Background(), t.workspaceID)
	base = reqctxpkg.SetLocale(base, t.locale)
	base = reqctxpkg.SetConversationID(base, conversationID)
	base = reqctxpkg.SetMessageID(base, t.assistantMsgID)
	base = reqctxpkg.WithAgentState(base, q.agentState)
	base = loopapp.WithBridge(base, s.deps.Bridge)
	// SSE-C: seed the entities Bridge so the loop mirrors a forge tool_call's content delta onto the
	// entities stream (the entity panel fills in live as the LLM forges).
	//
	// SSE-C：种 entities Bridge，使 loop 把 forge tool_call 的内容 delta 镜像到 entities 流（LLM 锻造时实体面板实时填充）。
	base = entitystreamapp.WithBridge(base, s.deps.EntitiesBridge)

	ctx, cancel := context.WithCancel(base)
	q.mu.Lock()
	q.cancel = cancel
	q.mu.Unlock()
	defer cancel()

	conv, err := s.deps.Conversations.Get(ctx, conversationID)
	if err != nil {
		s.failTurn(ctx, conversationID, t.assistantMsgID, "INTERNAL_ERROR", "load conversation: "+err.Error())
		return
	}

	bundle, err := s.deps.Resolver.ResolveChat(ctx, conv.ModelOverride)
	if err != nil {
		s.failTurn(ctx, conversationID, t.assistantMsgID, "LLM_RESOLVE_ERROR", err.Error())
		return
	}

	host := &chatHost{
		svc:            s,
		conversationID: conversationID,
		assistantMsgID: t.assistantMsgID,
		assistantMsg: &messagesdomain.Message{
			ID:             t.assistantMsgID,
			ConversationID: conversationID,
			Role:           messagesdomain.RoleAssistant,
			Provider:       bundle.Provider,        // provenance: which provider produced this turn
			ModelID:        bundle.Request.ModelID, // provenance: which model
		},
		caps:                 bundle.Caps,
		summary:              conv.Summary,
		summaryCoversUpToSeq: conv.SummaryCoversUpToSeq,
	}

	req := bundle.Request
	req.System = s.buildSystemPrompt(ctx, conv)

	// loop.Run always ends with exactly one host.WriteFinalize (persist + message_stop), so the
	// Result is redundant here.
	//
	// loop.Run 总以恰一次 host.WriteFinalize（落盘 + message_stop）收尾，故此处 Result 冗余。
	loopapp.Run(ctx, host, bundle.Client, req, s.maxSteps, s.log)

	// After the first turn of an untitled conversation, auto-title it in the background
	// (best-effort, detached). conv is the pre-turn snapshot, so its empty title is exactly the
	// "first turn" signal.
	//
	// 无标题对话首回合后，后台自动起标题（best-effort，detached）。conv 是回合前快照，其空标题正是
	// 「首回合」信号。
	s.maybeAutoTitle(conv, t.workspaceID)

	// Compact older history if this turn pushed the context near the model's window. Synchronous
	// here (inside the per-conversation queue slot, so the next turn's LoadHistory can't race the
	// summary/role writes), on a detached context so a cancelled turn still compacts. Best-effort:
	// a failure is non-fatal — the next turn just re-checks.
	//
	// 若本回合把上下文逼近模型 window 则压缩旧历史。此处同步（在 per-conversation queue 槽内，故下回合
	// LoadHistory 不与 summary/角色写竞态），detached context 使被取消的回合仍压缩。best-effort：失败
	// 非致命——下回合再查。
	if s.deps.Compactor != nil {
		cctx := reqctxpkg.SetWorkspaceID(context.Background(), t.workspaceID)
		if err := s.deps.Compactor.MaybeCompact(cctx, conversationID); err != nil {
			s.log.Warn("chatapp: compaction failed (non-fatal)", zap.Error(err))
		}
	}
}

// failTurn marks an assistant turn terminal-error before the loop ever runs (model resolve or
// conversation load failed) and pushes message_stop, so the streaming bubble never hangs. Runs
// on a detached context for the same reason WriteFinalize does.
//
// failTurn 在 loop 还没跑就把 assistant 回合标记为终态错误（模型解析或对话加载失败）并推
// message_stop，使流式气泡不挂死。出于与 WriteFinalize 相同的理由在 detached context 上跑。
func (s *Service) failTurn(ctx context.Context, conversationID, msgID, code, msg string) {
	wsID, _ := reqctxpkg.GetWorkspaceID(ctx)
	dctx := reqctxpkg.SetWorkspaceID(context.Background(), wsID)
	dctx = reqctxpkg.SetConversationID(dctx, conversationID)

	m := &messagesdomain.Message{
		ID:             msgID,
		ConversationID: conversationID,
		Role:           messagesdomain.RoleAssistant,
		Status:         messagesdomain.StatusError,
		StopReason:     messagesdomain.StopReasonError,
		ErrorCode:      code,
		ErrorMessage:   msg,
	}
	if err := s.messages.FinalizeMessage(dctx, m, nil); err != nil {
		s.log.Warn("chatapp.failTurn: finalize failed", zap.String("messageId", msgID), zap.Error(err))
	}
	s.emitMessageStop(dctx, conversationID, m)
}

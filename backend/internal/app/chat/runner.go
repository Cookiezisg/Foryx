package chat

import (
	"context"
	"time"

	"go.uber.org/zap"

	entitystreamapp "github.com/sunweilin/forgify/backend/internal/app/entitystream"
	humanloopapp "github.com/sunweilin/forgify/backend/internal/app/humanloop"
	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	limitspkg "github.com/sunweilin/forgify/backend/internal/pkg/limits"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// idleTimeout reclaims a conversation's drain goroutine + queue after this long with no task, so
// dormant conversations cost nothing. A new Send re-creates the queue on demand.
//
// idleTimeout 在无任务这么久后回收对话的抽取 goroutine + 队列，使休眠对话零成本。新 Send 按需重建队列。
const idleTimeout = 5 * time.Minute

// runQueue is the conversation's single drain goroutine — it serializes generations (one
// assistant turn at a time, which makes per-conversation block seq allocation race-free). It
// self-destructs after idleTimeout with no task, deregistering from s.queues. The exit decision
// happens under q.mu together with setting q.dead: enqueue sends under the same lock after
// checking dead, so a task either lands before the final drain (and is served) or sees dead and
// re-creates the queue — no task can be stranded in a dead channel. s.stop short-circuits the
// loop at shutdown (the running turn was already cancelled by Shutdown).
//
// runQueue 是对话的单抽取 goroutine——串行化生成（同时一个 assistant 回合，这使 per-conversation
// block seq 分配无竞争）。无任务 idleTimeout 后自毁、从 s.queues 注销。退出判定与设 q.dead 在
// q.mu 下原子完成：enqueue 在同一锁下查 dead 再投，task 要么落在最终抽干之前（被服务）、要么看见
// dead 重建队列——不可能滞留死 channel。s.stop 在关停时短路循环（在跑回合已被 Shutdown 取消）。
func (s *Service) runQueue(conversationID string, q *convQueue) {
	defer s.wg.Done()
	idle := time.NewTimer(idleTimeout)
	defer idle.Stop()

	for {
		select {
		case <-s.stop:
			return

		case t := <-q.ch:
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			// running brackets processTask: once dequeued the task is invisible in the
			// channel, so without the flag a concurrent Send would queue behind the user's
			// back instead of getting STREAM_IN_PROGRESS.
			// running 括住 processTask：task 一被取走在 channel 里就不可见，没有该标志，
			// 并发 Send 会背着用户排队而非收到 STREAM_IN_PROGRESS。
			q.setRunning(true)
			s.processTask(conversationID, q, t)
			q.setRunning(false)
			idle.Reset(idleTimeout)

		case <-idle.C:
			// Atomic teardown: mark dead + deregister + final drain all under q.mu, so a
			// concurrent enqueue (which sends under q.mu) cannot slip a task in after the drain.
			//
			// 原子拆卸：标 dead + 注销 + 最终抽干全在 q.mu 下，使并发 enqueue（同锁下投递）不可能
			// 在抽干之后塞进 task。
			q.mu.Lock()
			select {
			case t := <-q.ch:
				q.mu.Unlock()
				q.setRunning(true)
				s.processTask(conversationID, q, t)
				q.setRunning(false)
				idle.Reset(idleTimeout)
				continue
			default:
			}
			q.dead = true
			s.queues.Delete(conversationID)
			q.mu.Unlock()
			return
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
	base := reqctxpkg.Detached(t.workspaceID)
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
	// R0064: seed the human-in-the-loop broker so the loop's danger gate + ask_user can block for a
	// human decision (and it flows into any nested agent run via ctx). Cancel (below) unblocks them.
	//
	// R0064：种人在环 broker，使 loop 的 danger 门 + ask_user 能阻塞等人决定（经 ctx 流入任何嵌套 agent 运行）。
	// 下方 cancel 解阻它们。
	base = humanloopapp.WithBroker(base, s.broker)

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
	// Result is redundant here. maxSteps is read live (not captured at construction) so a
	// PATCH /limits hot-swap takes effect on the next turn — like every other limits consumer.
	//
	// loop.Run 总以恰一次 host.WriteFinalize（落盘 + message_stop）收尾，故此处 Result 冗余。
	// maxSteps 实时读取（非构造时捕获），故 PATCH /limits 热换下一回合即生效——与其余 limits 消费方一致。
	loopapp.Run(ctx, host, bundle.Client, req, limitspkg.Current().Agent.MaxSteps, s.log)

	// The user-visible turn is over (finalized + message_stop) — release the in-flight gate
	// BEFORE the synchronous tail: compaction below can be a real LLM call lasting seconds,
	// and a follow-up Send right after the reply must queue (slot), not bounce with 409.
	// 用户可见回合已结束（已落盘 + message_stop）——在同步尾活前释放在飞门：下面的压缩可能是
	// 数秒级的真 LLM 调用，回复刚完用户接着发的消息应进槽排队、而不是被 409 弹回。
	q.setRunning(false)

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
		cctx := reqctxpkg.Detached(t.workspaceID)
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
	dctx := reqctxpkg.Detached(wsID)
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

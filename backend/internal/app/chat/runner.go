// runner.go — Queue management, processTask, and the per-turn handoff to
// loop.Run. The ReAct mechanics (stream / tool dispatch / history extension /
// finalize cadence) live in internal/app/loop. This file owns chat-specific
// concerns: queueing, model resolution, system prompt, autoTitle.
//
// runner.go — 队列管理、processTask 与每回合交付给 loop.Run 的入口。ReAct
// 机制（流 / 工具调度 / 历史扩展 / 终态节奏）在 internal/app/loop。本文件
// 只持 chat 专属：队列、模型解析、system prompt、autoTitle。
package chat

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"

	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	agentstatepkg "github.com/sunweilin/forgify/backend/internal/pkg/agentstate"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// maxSteps caps the ReAct loop to prevent runaway tool-calling cycles.
// maxSteps 限制 ReAct 循环次数，防止工具调用无限循环。
const maxSteps = 20

// ── Queue / worker ────────────────────────────────────────────────────────────

func (s *Service) getOrCreateQueue(conversationID string) *convQueue {
	q := &convQueue{
		ch:         make(chan queuedTask, queueCapacity),
		agentState: &agentstatepkg.AgentState{},
	}
	actual, loaded := s.queues.LoadOrStore(conversationID, q)
	if loaded {
		return actual.(*convQueue)
	}
	go s.runQueue(conversationID, q)
	return q
}

func (s *Service) runQueue(conversationID string, q *convQueue) {
	const idleTimeout = 5 * time.Minute
	timer := time.NewTimer(idleTimeout)
	defer func() {
		timer.Stop()
		s.queues.Delete(conversationID)
	}()
	for {
		select {
		case task := <-q.ch:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			s.processTask(conversationID, q, task)
			timer.Reset(idleTimeout)
		case <-timer.C:
			return
		}
	}
}

// ── processTask ───────────────────────────────────────────────────────────────

func (s *Service) processTask(conversationID string, q *convQueue, task queuedTask) {
	ctx := task.ctx

	agentCtx, cancel := context.WithCancel(ctx)
	q.mu.Lock()
	q.cancel = cancel
	q.mu.Unlock()
	defer func() {
		cancel()
		q.mu.Lock()
		q.cancel = nil
		q.mu.Unlock()
	}()
	agentCtx = reqctxpkg.WithConversationID(agentCtx, conversationID)
	agentCtx = reqctxpkg.WithAgentState(agentCtx, q.agentState)
	agentCtx = eventlogpkg.With(agentCtx, s.emitter)

	// Pre-allocate the assistant msgID so the event-log message_start
	// (below) can be emitted with a stable ID before LLM resolution; pre-
	// LLM errors then have a valid msgID to attach the message_stop to.
	//
	// 预分配 assistant msgID，让事件日志 message_start（下文）在 LLM 解析前
	// 即可用稳定 ID 发出；LLM 前错误也有合法 msgID 挂 message_stop。
	msgID := newMsgID()
	agentCtx = reqctxpkg.WithMessageID(agentCtx, msgID)

	// Event-log: open the assistant message slot on the new bridge so
	// streamLLM-emitted block_start events have a valid parent. Top-level
	// assistant message (parent_block_id="").
	//
	// 事件日志：在新 bridge 上开 assistant message 槽，让 streamLLM 推的
	// block_start 有合法 parent。顶层 assistant message（parent_block_id=""）。
	s.emitter.EmitMessageStart(agentCtx, msgID, chatdomain.RoleAssistant, "", nil)

	bc, err := llmclientpkg.Resolve(agentCtx, s.modelPicker, s.keyProvider, s.llmFactory)
	if err != nil {
		code := "LLM_PROVIDER_ERROR"
		switch {
		case errors.Is(err, llmclientpkg.ErrPickModel):
			code = "MODEL_NOT_CONFIGURED"
		case errors.Is(err, llmclientpkg.ErrResolveCreds):
			code = "API_KEY_PROVIDER_NOT_FOUND"
		case errors.Is(err, llmclientpkg.ErrBuildClient):
			code = "LLM_BUILD_FAILED"
		}
		s.emitFatalError(agentCtx, task.conv, task.uid, msgID, code, err.Error())
		return
	}

	baseReq := llminfra.Request{
		ModelID: bc.ModelID,
		Key:     bc.Key,
		BaseURL: bc.BaseURL,
		System:  s.buildSystemPrompt(agentCtx, task.conv),
		// loop.Run fills baseReq.Tools from host.Tools().
	}

	host := &chatHost{
		svc:       s,
		convID:    task.conv.ID,
		uid:       task.uid,
		msgID:     msgID,
		userMsgID: task.userMsgID,
	}
	result := loopapp.Run(agentCtx, host, bc.Client, baseReq, maxSteps, s.log)

	s.log.Info("agent run complete",
		zap.String("conversation_id", task.conv.ID),
		zap.String("stop_reason", result.StopReason),
		zap.Int("input_tokens", result.TokensIn),
		zap.Int("output_tokens", result.TokensOut))

	if task.conv.Title == "" && !task.conv.AutoTitled {
		go s.autoTitle(context.Background(), task.conv, task.uid, result.LastMessage)
	}
}

// emitFatalError persists a stub assistant message with status=error and
// emits message_stop on the event-log bridge so the SSE stream's
// streaming bubble closes. Used for failures before the LLM stream begins
// (model not configured, key resolution failed).
//
// emitFatalError 落库 status=error 的 stub assistant 消息并发 message_stop
// 关闭 SSE 流上的 streaming bubble。供 LLM 流开始前的失败使用（模型未配置、
// key 解析失败）。
func (s *Service) emitFatalError(
	ctx context.Context,
	conv *convdomain.Conversation,
	uid, msgID, code, message string,
) {
	s.log.Error("chat fatal error",
		zap.String("conversation_id", conv.ID),
		zap.String("code", code), zap.String("message", message))

	// Detached saveCtx: a cancelled upstream stream must not block the
	// terminal write OR the message_stop emit. Re-stamp uid + convID so
	// the saved row keeps ownership and the emit can find its convID
	// (Emitter.StopMessage requires conversationID via reqctx; ctx may
	// have it but saveCtx is fresh, so we put it back). Mirrors host.go
	// ::WriteFinalize for the regular finalize path.
	//
	// Detached saveCtx：上游 cancel 不能挡终态写入或 message_stop emit。
	// 重打 uid + convID——落库行保留 owner，且 emit 能从 reqctx 找到
	// convID（Emitter.StopMessage 经 reqctx 要 conversationID；ctx 可能
	// 有，但 saveCtx 是新建的，所以重塞）。镜像 host.go::WriteFinalize 的
	// 正常终态路径。
	saveCtx := reqctxpkg.SetUserID(context.Background(), uid)
	saveCtx = reqctxpkg.WithConversationID(saveCtx, conv.ID)
	msg := buildMessage(msgID, conv.ID, uid,
		chatdomain.StatusError, chatdomain.StopReasonError,
		code, message, 0, 0)
	if err := s.repo.SaveMessage(saveCtx, msg); err != nil {
		s.log.Error("CRITICAL: fatal-error stub message persist failed — message lost",
			zap.String("msg_id", msgID), zap.Error(err))
	}

	s.emitter.StopMessage(saveCtx, msgID, eventlogdomain.StatusError,
		chatdomain.StopReasonError, code, message, 0, 0)
}

// ── System prompt & helpers ───────────────────────────────────────────────────

func (s *Service) buildSystemPrompt(ctx context.Context, conv *convdomain.Conversation) string {
	var sb strings.Builder
	sb.WriteString("You are Forgify, an AI assistant that helps users build tools, automate workflows, and work with data.")
	if conv.SystemPrompt != "" {
		sb.WriteString("\n\n")
		sb.WriteString(conv.SystemPrompt)
	}
	// Capability Catalog block (D8): teaches the LLM what categories of
	// capabilities exist + when to prefer one over another. Skipped
	// silently when no provider is wired (unit tests / no-LLM-key envs)
	// or when the provider returns an empty string (boot window before
	// the first Refresh tick completes).
	//
	// Capability Catalog 段（D8）：教 LLM 有哪些类目能力 + 何时优先何者。
	// 无 provider（单测 / 无 LLM key）或返空（首 Refresh 完成前 boot 窗
	// 口）静默跳。
	if s.catalog != nil {
		if catalogText := s.catalog.GetForSystemPrompt(); catalogText != "" {
			sb.WriteString("\n\n")
			sb.WriteString(catalogText)
		}
	}
	// Multi-agent forging block (Plan 06 F2 + D21 教学):tells the main chat
	// LLM when to spawn parallel forger sub-agents vs. forging in-place;
	// reminds it that sub-agents can't touch workflow ops, so workflow
	// assembly + trigger are always the main agent's responsibility.
	//
	// Multi-agent forging 段(F2 + D21):告主 chat LLM 何时并发 spawn forger
	// 子 agent;sub-agent 无 workflow ops,装配 + 触发归主 agent。
	sb.WriteString("\n\n")
	sb.WriteString(multiAgentForgingPromptSection)
	if reqctxpkg.GetLocale(ctx) == reqctxpkg.LocaleZhCN {
		sb.WriteString("\n\nPlease respond in Chinese (Simplified) unless the user writes in another language.")
	}
	return sb.String()
}

// autoTitle picks a short title via LLM, persists, and publishes a
// `conversation` notification (entity snapshot) so all open UI windows
// see the new name. Best-effort: any failure aborts silently.
//
// autoTitle 通过 LLM 取一个短标题，持久化后发 `conversation` 通知
// （entity snapshot）让所有打开的 UI 窗口看到新名字。Best-effort：失败
// 静默退出。
func (s *Service) autoTitle(ctx context.Context, conv *convdomain.Conversation, uid, assistantContent string) {
	titleCtx := reqctxpkg.SetUserID(ctx, uid)
	bc, err := llmclientpkg.Resolve(titleCtx, s.modelPicker, s.keyProvider, s.llmFactory)
	if err != nil {
		return
	}

	tCtx, cancel := context.WithTimeout(titleCtx, 10*time.Second)
	defer cancel()

	req := llminfra.Request{
		ModelID: bc.ModelID, Key: bc.Key, BaseURL: bc.BaseURL,
		System: "Generate a short conversation title (5 words or fewer). Reply with ONLY the title, no punctuation.\n只返回标题本身，不超过 10 个字，不加标点。",
		Messages: []llminfra.LLMMessage{
			{Role: llminfra.RoleUser, Content: "Assistant said: " + truncate(assistantContent, 300)},
		},
	}
	title, err := llminfra.Generate(tCtx, bc.Client, req)
	if err != nil || title == "" {
		return
	}
	conv.Title = strings.TrimSpace(title)
	conv.AutoTitled = true
	if err := s.convRepo.Save(titleCtx, conv); err != nil {
		s.log.Warn("auto-title save failed", zap.Error(err))
		return
	}
	// Slim payload (D-redo-22): action + new title + autoTitled flag so
	// the sidebar can re-render the title cell without a GET round-trip.
	// 瘦身 payload;侧栏看到新 title + autoTitled 标记即可,不用 GET。
	s.notifications.Publish(titleCtx, "conversation", conv.ID,
		map[string]any{"action": "auto_titled", "title": conv.Title, "autoTitled": true},
		conv.ID)
	s.log.Info("auto-title generated",
		zap.String("conversation_id", conv.ID), zap.String("title", conv.Title))
}

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

const maxSteps = 20

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

	// Pre-allocate msgID so pre-LLM errors can attach message_stop to a stable ID.
	msgID := newMsgID()
	agentCtx = reqctxpkg.WithMessageID(agentCtx, msgID)

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

	// Compaction runs synchronously before autoTitle so the fake LLM FIFO is deterministic.
	if s.compactor != nil {
		compactCtx := reqctxpkg.SetUserID(context.Background(), task.uid)
		compactCtx = reqctxpkg.WithConversationID(compactCtx, task.conv.ID)
		if err := s.compactor.MaybeCompact(compactCtx, task.conv.ID); err != nil {
			s.log.Warn("contextmgr.MaybeCompact failed (non-fatal)",
				zap.String("conv", task.conv.ID), zap.Error(err))
		}
	}

	if task.conv.Title == "" && !task.conv.AutoTitled {
		go s.autoTitle(context.Background(), task.conv, task.uid, result.LastMessage)
	}
}

// emitFatalError persists an error stub Message and emits message_stop to close the SSE bubble.
//
// emitFatalError 落库 error 占位 Message 并推 message_stop 关闭 SSE bubble。
func (s *Service) emitFatalError(
	ctx context.Context,
	conv *convdomain.Conversation,
	uid, msgID, code, message string,
) {
	s.log.Error("chat fatal error",
		zap.String("conversation_id", conv.ID),
		zap.String("code", code), zap.String("message", message))

	// Detached saveCtx mirrors host.WriteFinalize: upstream cancel must not block terminal write.
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

func (s *Service) buildSystemPrompt(ctx context.Context, conv *convdomain.Conversation) string {
	var sb strings.Builder
	sb.WriteString("You are Forgify, an AI assistant that helps users build tools, automate workflows, and work with data.")
	if conv.SystemPrompt != "" {
		sb.WriteString("\n\n")
		sb.WriteString(conv.SystemPrompt)
	}
	if s.catalog != nil {
		if catalogText := s.catalog.GetForSystemPrompt(); catalogText != "" {
			sb.WriteString("\n\n")
			sb.WriteString(catalogText)
		}
	}
	if s.memory != nil {
		if memoryText := s.memory.ForSystemPrompt(ctx); memoryText != "" {
			sb.WriteString("\n\n")
			sb.WriteString(memoryText)
		}
	}
	sb.WriteString("\n\n")
	sb.WriteString(multiAgentForgingPromptSection)
	if reqctxpkg.GetLocale(ctx) == reqctxpkg.LocaleZhCN {
		sb.WriteString("\n\nPlease respond in Chinese (Simplified) unless the user writes in another language.")
	}
	return sb.String()
}

// autoTitle generates a short title via LLM, persists, and publishes a notification (best-effort).
//
// autoTitle 经 LLM 生成短标题、持久化、发 conversation 通知（失败静默）。
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
	s.notifications.Publish(titleCtx, "conversation", conv.ID,
		map[string]any{"action": "auto_titled", "title": conv.Title, "autoTitled": true},
		conv.ID)
	s.log.Info("auto-title generated",
		zap.String("conversation_id", conv.ID), zap.String("title", conv.Title))
}

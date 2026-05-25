package chat

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"

	documentapp "github.com/sunweilin/forgify/backend/internal/app/document"
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

// maxTurnDuration is the hard wall-clock cap on one chat agent turn.
// Beyond this, agentCtx is cancelled; loop.Run observes cancellation
// and exits at the next iteration boundary, host.WriteFinalize lands
// status="cancelled" stop_reason="timeout". Protects against (a) a
// runaway tool that never returns, (b) an LLM stream that hangs on a
// dead socket past the HTTP timeout, (c) infinite tool-call loops.
//
// maxTurnDuration 是单 chat agent turn 的硬墙钟上限。超时 agentCtx 取消，
// loop.Run 下一步退出，host.WriteFinalize 落 status=cancelled
// stop_reason=timeout。防 (a) 永不返回的 tool，(b) socket 死后挂死的
// LLM stream，(c) 无限 tool-call 循环。10min 取自"单轮工作流复杂任务
// 合理上限"经验值；超过基本是 bug 而非用户期望。
const maxTurnDuration = 10 * time.Minute

func (s *Service) processTask(conversationID string, q *convQueue, task queuedTask) {
	ctx := task.ctx

	agentCtx, cancel := context.WithTimeout(ctx, maxTurnDuration)
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

	// §12.3 per-conv override: conv.ModelOverride beats user's chat-scenario default.
	// §12.3 对话级 override：conv.ModelOverride 优先于 user 的 chat scenario 默认。
	bc, err := llmclientpkg.ResolveWithOverride(agentCtx, task.conv.ModelOverride, s.modelPicker, s.keyProvider, s.llmFactory)
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
		provider:  bc.Provider,
		modelID:   bc.ModelID,
	}
	// Install V1.2 §3 interceptor (permissions gate + Pre/PostToolUse).
	// Nil when SetPermissionsAndHooks wasn't called → loop sees noop.
	// 装 V1.2 §3 interceptor。未 SetPermissionsAndHooks 时为 nil，loop 走 noop。
	if s.interceptor != nil {
		agentCtx = loopapp.WithInterceptor(agentCtx, s.interceptor)
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
	// emitFatalError fires before bundle.Provider/ModelID are known
	// (resolve failed), so leave them empty — usage aggregation drops
	// zero-token rows anyway.
	// emitFatalError 在 bundle.Provider/ModelID 已知前触发（resolve 已
	// 失败），留空——usage 聚合本就丢 0 token 行。
	msg := buildMessage(msgID, conv.ID, uid,
		chatdomain.StatusError, chatdomain.StopReasonError,
		code, message, 0, 0, "", "")
	if err := s.repo.SaveMessage(saveCtx, msg); err != nil {
		s.log.Error("CRITICAL: fatal-error stub message persist failed — message lost",
			zap.String("msg_id", msgID), zap.Error(err))
	}

	s.emitter.StopMessage(saveCtx, msgID, eventlogdomain.StatusError,
		chatdomain.StopReasonError, code, message, 0, 0)
}

// PromptSection is one named segment in the chat system prompt; sections concatenate via separator into the wire prompt.
//
// PromptSection 是 chat system prompt 的一段；按顺序拼接为最终 wire prompt。
type PromptSection struct {
	Name    string `json:"name"`    // "base" / "user_systemPrompt" / "catalog" / "memory" / "documents" / "multi_agent_forging" / "locale_hint"
	Content string `json:"content"`
}

// SystemPromptSections returns the per-conv assembled prompt as ordered named sections (cache-friendly order: static-first, dynamic-last).
//
// SystemPromptSections 返按 cache-friendly 顺序（静态前 / 动态后）排好的命名段；外部预览端点直接消费。
func (s *Service) SystemPromptSections(ctx context.Context, conv *convdomain.Conversation) []PromptSection {
	out := make([]PromptSection, 0, 9)
	out = append(out, PromptSection{Name: "base", Content: chatBasePrompt})
	out = append(out, PromptSection{Name: "tool_conventions", Content: toolConventionsSection})
	out = append(out, PromptSection{Name: "multi_agent_forging", Content: multiAgentForgingPromptSection})

	if s.catalog != nil {
		if catalogText := s.catalog.GetForSystemPrompt(ctx); catalogText != "" {
			out = append(out, PromptSection{Name: "catalog", Content: catalogText})
		}
	}
	if s.memory != nil {
		if memoryText := s.memory.ForSystemPrompt(ctx); memoryText != "" {
			out = append(out, PromptSection{Name: "memory", Content: memoryText})
		}
	}
	if s.documents != nil && len(conv.AttachedDocuments) > 0 {
		docs, err := s.documents.ResolveAttached(ctx, conv.AttachedDocuments)
		if err != nil {
			s.log.Warn("chat.SystemPromptSections: ResolveAttached failed",
				zap.String("conv_id", conv.ID), zap.Error(err))
		} else if len(docs) > 0 {
			out = append(out, PromptSection{Name: "documents", Content: documentapp.RenderAttachedAsXML(docs)})
		}
	}
	if conv.SystemPrompt != "" {
		out = append(out, PromptSection{Name: "user_systemPrompt", Content: conv.SystemPrompt})
	}
	if reqctxpkg.GetLocale(ctx) == reqctxpkg.LocaleZhCN {
		out = append(out, PromptSection{Name: "locale_hint",
			Content: "Please respond in Chinese (Simplified) unless the user writes in another language."})
	}
	return out
}

// chatBasePrompt is the identity line prepended to every chat system prompt.
//
// chatBasePrompt 是每轮 chat system prompt 的身份开头。
const chatBasePrompt = "You are Forgify, an AI assistant that helps users build tools, automate workflows, and work with data."

// toolConventionsSection teaches the LLM the three standard tool fields once here
// instead of duplicating the explanation across every tool schema.
//
// toolConventionsSection 把三个标准注入字段的说明集中在此，避免在 64 个 tool schema 里重复。
const toolConventionsSection = `Every tool call accepts three standard fields:
- summary (required): one sentence on what you're doing and why.
- destructive (optional): true if the call may be irreversible (delete, force-push, writes to external state); the user sees a warning.
- execution_group (optional, int): calls sharing a group run in parallel; different groups run in ascending order. Group only calls with no interdependence and no shared state. Omit when unsure.`

// BasePromptText / MultiAgentForgingPromptText / ToolConventionsText expose static chat prompt segments to the §18 inventory endpoint.
//
// BasePromptText / MultiAgentForgingPromptText / ToolConventionsText 把静态段暴露给 §18 prompt 总览端点。
func BasePromptText() string              { return chatBasePrompt }
func MultiAgentForgingPromptText() string { return multiAgentForgingPromptSection }
func ToolConventionsText() string         { return toolConventionsSection }

func (s *Service) buildSystemPrompt(ctx context.Context, conv *convdomain.Conversation) string {
	return AssemblePromptSections(s.SystemPromptSections(ctx, conv))
}

// AssemblePromptSections wraps each section in <section name="..."> markers so the LLM (and the preview UI) can see boundaries.
//
// AssemblePromptSections 把每段用 <section name="..."> 包起来，LLM 与预览 UI 都能看到边界。
func AssemblePromptSections(sections []PromptSection) string {
	var sb strings.Builder
	for i, sec := range sections {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("<section name=\"")
		sb.WriteString(sec.Name)
		sb.WriteString("\">\n")
		sb.WriteString(sec.Content)
		sb.WriteString("\n</section>")
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

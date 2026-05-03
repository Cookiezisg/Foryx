// runner.go — Queue management, processTask, and the main ReAct agent loop.
// One worker goroutine per conversation drains tasks sequentially; each task
// runs agentRun until the LLM stops calling tools or the step limit is reached.
//
// SSE: this file is the single source of truth for chat.message snapshot
// publishing. See publishMessageSnapshot for the helper; streamLLM /
// runTools call it indirectly via parentBlocks-aware closures.
//
// runner.go — 队列管理、processTask 和主 ReAct agent 循环。
// 每个 conversation 一个 worker goroutine 顺序消费任务；每个任务运行
// agentRun，直到 LLM 不再调用工具或达到步骤上限。
//
// SSE：本文件是 chat.message 快照推送的唯一事实源。helper 见 publishMessageSnapshot；
// streamLLM / runTools 通过感知 parentBlocks 的闭包间接调用。
package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	eventsdomain "github.com/sunweilin/forgify/backend/internal/domain/events"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// ── Queue / worker ────────────────────────────────────────────────────────────

func (s *Service) getOrCreateQueue(conversationID string) *convQueue {
	q := &convQueue{ch: make(chan queuedTask, queueCapacity)}
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

	// Per-task cancellable context so Cancel() can stop the running agent.
	// 每任务可取消 context，供 Cancel() 停止运行中的 agent。
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

	// Allocate the assistant msgID up front so pre-LLM errors can be
	// emitted as a stub assistant Message (entity-state SSE — every
	// chat.message event must carry a real Message).
	//
	// 预先分配 assistant msgID，让 LLM 调用前的错误也能以 stub 消息发出
	// （entity-state SSE 要求每个 chat.message 都承载真实 Message）。
	msgID := newMsgID()

	bc, err := llmclientpkg.Resolve(agentCtx, s.modelPicker, s.keyProvider, s.llmFactory)
	if err != nil {
		// Map per-step failure to its user-facing error code so the UI can
		// distinguish "no model configured" from "no API key" from a generic
		// upstream provider error.
		//
		// 把分步失败映射到对外错误码，让 UI 能区分"未配模型"/"无 API key"/
		// 通用上游错误。
		code := "LLM_PROVIDER_ERROR"
		switch {
		case errors.Is(err, llmclientpkg.ErrPickModel):
			code = "MODEL_NOT_CONFIGURED"
		case errors.Is(err, llmclientpkg.ErrResolveCreds):
			code = "API_KEY_PROVIDER_NOT_FOUND"
		}
		s.emitFatalError(agentCtx, task.conv, task.uid, msgID, code, err.Error())
		return
	}

	baseReq := llminfra.Request{
		ModelID: bc.ModelID,
		Key:     bc.Key,
		BaseURL: bc.BaseURL,
		System:  s.buildSystemPrompt(agentCtx, task.conv),
		Tools:   toolapp.ToLLMDefs(s.tools),
	}
	s.agentRun(agentCtx, task.uid, task.conv, task.userMsgID, msgID, bc.Client, baseReq)
}

// ── agentRun ──────────────────────────────────────────────────────────────────

// maxSteps caps the ReAct loop to prevent runaway tool-calling cycles.
// maxSteps 限制 ReAct 循环次数，防止工具调用无限循环。
const maxSteps = 20

// agentRun runs the ReAct loop for one user turn. It calls streamLLM to get
// the LLM's response, executes any tool calls, persists checkpoints + final
// state, and publishes a chat.message snapshot at every observable change.
//
// agentRun 为一次用户回合运行 ReAct 循环。调用 streamLLM 获取 LLM 回复，
// 执行工具调用，持久化 checkpoint 与最终状态，并在每个可观察变化点推送
// chat.message 快照。
func (s *Service) agentRun(
	ctx context.Context,
	uid string,
	conv *convdomain.Conversation,
	userMsgID, msgID string,
	client llminfra.Client,
	baseReq llminfra.Request,
) {
	history, err := s.buildHistory(ctx, conv.ID, userMsgID)
	if err != nil {
		s.emitFatalError(ctx, conv, uid, msgID, "INTERNAL_ERROR", err.Error())
		return
	}

	var (
		allBlocks    []chatdomain.Block
		totalInput   int
		totalOutput  int
		stopReason   = chatdomain.StopReasonEndTurn
		errCode      string
		errMessage   string
		finalWritten bool
	)

	// Initial publish — opens the assistant slot in the UI before any tokens
	// arrive. Status=streaming, blocks empty.
	//
	// 初始发布——在任何 token 到达前先打开前端的 assistant 槽位。status=streaming，blocks 空。
	s.publishMessageSnapshot(ctx, msgID, conv.ID, uid, nil,
		chatdomain.StatusStreaming, "", "", "", 0, 0)

	for step := range maxSteps {
		req := baseReq
		req.Messages = history

		aBlocks, toolCalls, sr, em, iT, oT := s.streamLLM(ctx, client, req, conv.ID, msgID, uid, allBlocks)
		allBlocks = append(allBlocks, aBlocks...)
		totalInput += iT
		totalOutput += oT
		if sr != "" {
			stopReason = sr
		}

		if stopReason == chatdomain.StopReasonCancelled || stopReason == chatdomain.StopReasonError {
			status := chatdomain.StatusCancelled
			if stopReason == chatdomain.StopReasonError {
				status = chatdomain.StatusError
				errCode = "LLM_STREAM_ERROR"
				errMessage = em
			}
			s.writeAndPublish(ctx, msgID, conv.ID, uid, allBlocks, status, stopReason,
				errCode, errMessage, totalInput, totalOutput, true)
			finalWritten = true
			break
		}

		if len(toolCalls) == 0 {
			// No tool calls — LLM produced its final answer.
			// 无工具调用——LLM 产生最终答案。
			s.writeAndPublish(ctx, msgID, conv.ID, uid, allBlocks, chatdomain.StatusCompleted, stopReason,
				"", "", totalInput, totalOutput, true)
			finalWritten = true
			break
		}

		rBlocks := s.runTools(ctx, toolCalls, conv.ID, msgID, uid, allBlocks)
		allBlocks = append(allBlocks, rBlocks...)

		// Streaming checkpoint — non-fatal write + snapshot publish.
		// streaming checkpoint——非致命落盘 + 快照推送。
		s.writeAndPublish(ctx, msgID, conv.ID, uid, allBlocks, chatdomain.StatusStreaming, "",
			"", "", totalInput, totalOutput, false)

		history, err = extendHistory(history, aBlocks, rBlocks)
		if err != nil {
			s.log.Error("extend history failed",
				zap.String("conversation_id", conv.ID), zap.Error(err))
			stopReason = chatdomain.StopReasonError
			errCode = "HISTORY_EXTEND_FAILED"
			errMessage = err.Error()
			s.writeAndPublish(ctx, msgID, conv.ID, uid, allBlocks, chatdomain.StatusError, stopReason,
				errCode, errMessage, totalInput, totalOutput, true)
			finalWritten = true
			break
		}
		// TODO: context compaction — history = s.compactor.MaybeCompact(ctx, history)

		s.log.Debug("react step complete",
			zap.Int("step", step), zap.String("conversation_id", conv.ID))
	}

	if !finalWritten {
		// Step limit reached.
		// 达到步骤上限。
		stopReason = chatdomain.StopReasonMaxTokens
		s.writeAndPublish(ctx, msgID, conv.ID, uid, allBlocks, chatdomain.StatusCompleted, stopReason,
			"", "", totalInput, totalOutput, true)
	}

	s.log.Info("agent run complete",
		zap.String("conversation_id", conv.ID),
		zap.String("stop_reason", stopReason),
		zap.Int("input_tokens", totalInput),
		zap.Int("output_tokens", totalOutput))

	if conv.Title == "" && !conv.AutoTitled {
		go s.autoTitle(context.Background(), conv, uid, extractTextContent(allBlocks))
	}
}

// ── Persistence + snapshot publishing ─────────────────────────────────────────

// writeAndPublish persists the assistant message with its blocks AND publishes
// a chat.message snapshot. fatal=true means a write failure would lose the
// final state — we publish the snapshot even on save failure so the UI sees
// the error state. fatal=false (streaming checkpoints) only warns on failure.
//
// writeAndPublish 持久化 assistant 消息及 blocks，并推送 chat.message 快照。
// fatal=true 表示写失败会丢终态——即便保存失败也照样推快照让 UI 感知错误。
// fatal=false（streaming checkpoint）失败只 warn。
func (s *Service) writeAndPublish(
	ctx context.Context,
	msgID, convID, uid string,
	blocks []chatdomain.Block,
	status, stopReason string,
	errorCode, errorMessage string,
	inputTokens, outputTokens int,
	fatal bool,
) {
	saveCtx := ctx
	if fatal {
		// Fresh context: a cancelled stream must not block the terminal write.
		// 新 context：已取消的流不能阻止终态写入。
		saveCtx = reqctxpkg.SetUserID(context.Background(), uid)
	}

	msg := buildMessage(msgID, convID, uid, blocks, status, stopReason,
		errorCode, errorMessage, inputTokens, outputTokens)

	if err := s.repo.Save(saveCtx, msg); err != nil {
		if fatal {
			s.log.Error("CRITICAL: final assistant message persist failed — message lost",
				zap.String("msg_id", msgID), zap.String("conversation_id", convID), zap.Error(err))
			// Still publish so UI sees something — overlay the persistence
			// failure as the new error reason.
			//
			// 即便如此也要推快照让 UI 看到——把持久化失败覆盖为新的 error 原因。
			msg = buildMessage(msgID, convID, uid, blocks, chatdomain.StatusError, chatdomain.StopReasonError,
				"INTERNAL_ERROR", "failed to save assistant message to database",
				inputTokens, outputTokens)
		} else {
			s.log.Warn("streaming checkpoint persist failed, continuing",
				zap.String("msg_id", msgID), zap.Error(err))
		}
	}

	s.bridge.Publish(ctx, convID, eventsdomain.ChatMessage{Message: msg})
}

// publishMessageSnapshot emits a chat.message event without persisting.
// Used for in-flight streaming updates where the DB write happens at
// well-defined checkpoints.
//
// publishMessageSnapshot 推送 chat.message 事件但不落库。供流式中间更新使用，
// DB 写入只在明确的 checkpoint 发生。
func (s *Service) publishMessageSnapshot(
	ctx context.Context,
	msgID, convID, uid string,
	blocks []chatdomain.Block,
	status, stopReason string,
	errorCode, errorMessage string,
	inputTokens, outputTokens int,
) {
	msg := buildMessage(msgID, convID, uid, blocks, status, stopReason,
		errorCode, errorMessage, inputTokens, outputTokens)
	s.bridge.Publish(ctx, convID, eventsdomain.ChatMessage{Message: msg})
}

// emitFatalError persists a stub assistant message with status=error and
// publishes its chat.message snapshot. Used for failures that occur before
// the LLM stream begins (model not configured, key resolution failed, etc.).
//
// emitFatalError 落库一条 status=error 的 stub assistant 消息并推送快照。
// 供 LLM 流开始前的失败使用（模型未配置、key 解析失败等）。
func (s *Service) emitFatalError(
	ctx context.Context,
	conv *convdomain.Conversation,
	uid, msgID, code, message string,
) {
	s.log.Error("chat fatal error",
		zap.String("conversation_id", conv.ID),
		zap.String("code", code), zap.String("message", message))
	s.writeAndPublish(ctx, msgID, conv.ID, uid, nil,
		chatdomain.StatusError, chatdomain.StopReasonError,
		code, message, 0, 0, true)
}

// buildMessage constructs an assistant Message struct ready for persistence
// or SSE publish. Blocks are stamped with msgID + sequential seq.
//
// buildMessage 构造可直接落库或推 SSE 的 assistant Message。Blocks 被打上
// msgID 与连续 seq。
func buildMessage(
	msgID, convID, uid string,
	blocks []chatdomain.Block,
	status, stopReason, errorCode, errorMessage string,
	inputTokens, outputTokens int,
) *chatdomain.Message {
	return &chatdomain.Message{
		ID:             msgID,
		ConversationID: convID,
		UserID:         uid,
		Role:           chatdomain.RoleAssistant,
		Status:         status,
		StopReason:     stopReason,
		ErrorCode:      errorCode,
		ErrorMessage:   errorMessage,
		InputTokens:    inputTokens,
		OutputTokens:   outputTokens,
		Blocks:         stampBlocks(blocks, msgID),
		UpdatedAt:      time.Now().UTC(),
	}
}

// stampBlocks assigns global seq and messageID to every block before a DB write.
// stampBlocks 在写 DB 前为每个 block 打上全局 seq 和 messageID。
func stampBlocks(blocks []chatdomain.Block, msgID string) []chatdomain.Block {
	stamped := make([]chatdomain.Block, len(blocks))
	copy(stamped, blocks)
	for i := range stamped {
		stamped[i].MessageID = msgID
		stamped[i].Seq = i
		if stamped[i].ID == "" {
			stamped[i].ID = newBlockID()
		}
	}
	return stamped
}

// ── System prompt & helpers ───────────────────────────────────────────────────

func (s *Service) buildSystemPrompt(ctx context.Context, conv *convdomain.Conversation) string {
	var sb strings.Builder
	sb.WriteString("You are Forgify, an AI assistant that helps users build tools, automate workflows, and work with data.")
	if conv.SystemPrompt != "" {
		sb.WriteString("\n\n")
		sb.WriteString(conv.SystemPrompt)
	}
	if reqctxpkg.GetLocale(ctx) == reqctxpkg.LocaleZhCN {
		sb.WriteString("\n\nPlease respond in Chinese (Simplified) unless the user writes in another language.")
	}
	return sb.String()
}

// autoTitle picks a short title via LLM and persists + publishes a Conversation
// snapshot when successful. Best-effort: any failure aborts silently.
//
// autoTitle 通过 LLM 取一个短标题，成功则持久化并推送 Conversation 快照。
// best-effort：任何失败静默退出。
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
	s.bridge.Publish(titleCtx, conv.ID, eventsdomain.Conversation{Conversation: conv})
	s.log.Info("auto-title generated",
		zap.String("conversation_id", conv.ID), zap.String("title", conv.Title))
}

// extractTextContent returns the last text block's content from a block slice.
// Used to seed auto-titling after the agent run completes.
//
// extractTextContent 从 block 列表中返回最后一个 text block 的内容。
// 用于 agent 运行完成后提供自动命名的素材。
func extractTextContent(blocks []chatdomain.Block) string {
	var last string
	for _, b := range blocks {
		if b.Type == chatdomain.BlockTypeText {
			var d chatdomain.TextData
			if json.Unmarshal([]byte(b.Data), &d) == nil {
				last = d.Text
			}
		}
	}
	return last
}

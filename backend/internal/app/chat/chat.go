// Package chat (app/chat) orchestrates the chat pipeline: queueing,
// attachment handling, auto-titling, and SSE event publishing. The ReAct
// engine itself lives in internal/app/loop — chat and subagent are the
// current callers; future phases (Phase 4 workflow LLM nodes) will join.
// Owns no SQL — persistence is delegated to infra/store/chat.
//
// Concurrency: each conversation has a convQueue with a buffered task
// channel; one worker goroutine drains it sequentially, so messages within
// one conversation execute one at a time in order.
//
// Package chat（app/chat）编排聊天管线：队列、附件处理、自动命名、SSE 推送。
// ReAct 引擎本身在 internal/app/loop——chat 与 subagent 是当前调用方；未来
// Phase（Phase 4 workflow LLM 节点）会加入。不含 SQL，持久化委托给
// infra/store/chat。
//
// Files:
//
//	chat.go     — public API (Send, Cancel, ListMessages, UploadAttachment)
//	runner.go   — queue, processTask → loop.Run, autoTitle, system prompt
//	host.go     — chatHost implements loop.Host (persists Message rows + emits message_stop on the event-log bridge)
//	history.go  — buildHistory + buildUserLLMMessage + attachment resolve
//	util.go     — ID generators, file helpers, truncate
package chat

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	agentstatepkg "github.com/sunweilin/forgify/backend/internal/pkg/agentstate"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// queueCapacity is the maximum number of messages that can queue behind
// the currently running Agent for one conversation.
//
// queueCapacity 是单个 conversation 在当前 Agent 之后最多排队的消息数。
const queueCapacity = 5

// convQueue manages sequential Agent execution for one conversation.
// agentState carries cross-tool state (most notably SeenFiles for the
// must-Read-first guard); it lives as long as the queue itself, so it
// is GC'd together with the conversation when the idle timer fires.
//
// convQueue 管理单个 conversation 的顺序 Agent 执行。
// agentState 携带跨 tool 状态（最重要的是 must-Read-first 守卫用的 SeenFiles）；
// 生命周期跟 queue 同步，conversation idle 触发清队列时一并 GC。
type convQueue struct {
	ch         chan queuedTask
	mu         sync.Mutex
	cancel     context.CancelFunc // nil when idle
	agentState *agentstatepkg.AgentState
}

// queuedTask is one pending chat turn waiting to be processed.
//
// queuedTask 是等待处理的一次对话请求。
type queuedTask struct {
	ctx       context.Context
	conv      *convdomain.Conversation
	uid       string
	userMsgID string // ID of the user message that triggered this task
}

// Service orchestrates LLM calls, attachment handling, and SSE event publishing.
//
// Service 编排 LLM 调用、附件处理和 SSE 事件推送。
type Service struct {
	repo          chatdomain.Repository
	convRepo      convdomain.Repository
	modelPicker   modeldomain.ModelPicker
	keyProvider   apikeydomain.KeyProvider
	llmFactory    *llminfra.Factory
	tools         []toolapp.Tool
	emitter       eventlogpkg.Emitter        // event-log emit (chat / block lifecycle)
	notifications notificationspkg.Publisher // global notifications (autoTitle / etc.)
	dataDir       string
	log           *zap.Logger
	queues        sync.Map // conversationID → *convQueue

	// catalog (optional) provides the Capability Catalog summary that
	// gets prepended to every system prompt. Nil-tolerant: when not
	// wired (unit tests, environments without the catalog subsystem),
	// the system prompt skips the catalog block. Set via
	// SetSystemPromptProvider after construction (post-injection avoids
	// a circular dep — catalog imports chat would create one).
	//
	// catalog（可选）提供 Capability Catalog summary，前置每个 system
	// prompt。容忍 nil（单测、无 catalog 环境跳）。SetSystemPromptProvider
	// 后置注入避循环依赖（catalog import chat 就会循环）。
	catalog catalogdomain.SystemPromptProvider
}

// NewService wires Service dependencies. Panics on nil logger.
//
// emitter is the event-log Emitter for chat / block lifecycle.
// notifications is the global notifications Publisher for entity
// updates (autoTitle conversation rename, etc.). Either can be nil →
// no-op fallback (used by tests that don't exercise the SSE paths).
//
// NewService 装配依赖。nil logger 立刻 panic。
//
// emitter 是 chat / block 生命周期的事件日志 Emitter。notifications 是
// entity 更新（autoTitle / 等）的全局通知 Publisher。任一可 nil → no-op
// 回退（不练 SSE 的测试用）。
func NewService(
	repo chatdomain.Repository,
	convRepo convdomain.Repository,
	modelPicker modeldomain.ModelPicker,
	keyProvider apikeydomain.KeyProvider,
	llmFactory *llminfra.Factory,
	emitter eventlogpkg.Emitter,
	notifications notificationspkg.Publisher,
	dataDir string,
	log *zap.Logger,
) *Service {
	if log == nil {
		panic("chat.NewService: logger is nil")
	}
	if dataDir == "" {
		dataDir = filepath.Join(os.TempDir(), "forgify")
	}
	if emitter == nil {
		emitter = eventlogpkg.From(context.Background()) // no-op fallback
	}
	if notifications == nil {
		notifications = notificationspkg.New(nil, log) // no-op fallback
	}
	return &Service{
		repo:          repo,
		convRepo:      convRepo,
		modelPicker:   modelPicker,
		keyProvider:   keyProvider,
		llmFactory:    llmFactory,
		emitter:       emitter,
		notifications: notifications,
		dataDir:       dataDir,
		log:           log,
	}
}

// emitUserMessage publishes the user message_start + each block (with
// content delta) + message_stop to the new event-log bridge as a self-
// contained burst. User messages are not streamed (saved synchronously
// in Send), so all events fire at once. Block content is the raw text
// — no JSON wrapper. Attachments live in Message.Attrs (not blocks).
//
// Best-effort: any failure logs and continues.
//
// emitUserMessage 把 user message_start + 每个 block（含 content delta）
// + message_stop 一次性 burst 推。user message 不是流式（Send 中同步落库），
// 全部事件一次性发完。Block content 是裸文本——无 JSON 包装。Attachments
// 在 Message.Attrs（不是 blocks）。
//
// Best-effort：失败 log 后继续。
func (s *Service) emitUserMessage(ctx context.Context, msg *chatdomain.Message) {
	em := s.emitter
	em.EmitMessageStart(ctx, msg.ID, msg.Role, "", nil)
	for _, b := range msg.Blocks {
		em.EmitBlockStart(ctx, b.ID, msg.ID, msg.ID, b.Type, nil)
		if b.Content != "" {
			em.DeltaBlock(ctx, b.ID, b.Content)
		}
		em.StopBlock(ctx, b.ID, eventlogdomain.StatusCompleted, nil)
	}
	em.StopMessage(ctx, msg.ID, eventlogdomain.StatusCompleted, "", "", "", 0, 0)
}

// SetTools injects system tools into the ReAct Agent.
// Safe to call before any conversation starts.
//
// SetTools 将 system tools 注入 ReAct Agent，在任何对话启动前调用均安全。
func (s *Service) SetTools(tools []toolapp.Tool) {
	s.tools = tools
}

// SetSystemPromptProvider plugs the Capability Catalog (or any
// implementation of catalogdomain.SystemPromptProvider) so its summary
// gets prepended to every conversation's system prompt. Safe to leave
// nil — buildSystemPrompt skips the catalog block when not wired.
// Call after main.go constructs the catalog Service.
//
// SetSystemPromptProvider 接 Capability Catalog（或任何
// catalogdomain.SystemPromptProvider 实现）让其 summary 前置每个对话
// system prompt。留 nil 安全——buildSystemPrompt 在未接时跳。main.go 构
// 造 catalog Service 后调。
func (s *Service) SetSystemPromptProvider(p catalogdomain.SystemPromptProvider) {
	s.catalog = p
}

// SendInput is the payload for Service.Send.
//
// SendInput 是 Service.Send 的请求载荷。
type SendInput struct {
	Content       string
	AttachmentIDs []string
}

// UploadAttachment copies fileBytes to the data directory, stores metadata
// in DB, and returns the created Attachment.
//
// UploadAttachment 把 fileBytes 复制到 data 目录，把元数据存入 DB，返回创建好的 Attachment。
func (s *Service) UploadAttachment(ctx context.Context, fileBytes []byte, mimeType, fileName string) (*chatdomain.Attachment, error) {
	if int64(len(fileBytes)) > chatdomain.MaxAttachmentBytes {
		return nil, chatdomain.ErrAttachmentTooLarge
	}
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, fmt.Errorf("chat.Service.UploadAttachment: %w", err)
	}

	id := newAttachmentID()
	ext := filepath.Ext(fileName)
	storageDir := filepath.Join(s.dataDir, "attachments", id)
	storagePath := filepath.Join(storageDir, "original"+ext)

	if err := os.MkdirAll(storageDir, 0750); err != nil {
		return nil, fmt.Errorf("chat.Service.UploadAttachment: mkdir: %w", err)
	}
	if err := os.WriteFile(storagePath, fileBytes, 0640); err != nil {
		return nil, fmt.Errorf("chat.Service.UploadAttachment: write: %w", err)
	}

	a := &chatdomain.Attachment{
		ID:          id,
		UserID:      uid,
		FileName:    fileName,
		MimeType:    mimeType,
		SizeBytes:   int64(len(fileBytes)),
		StoragePath: storagePath,
	}
	if err := s.repo.SaveAttachment(ctx, a); err != nil {
		if cleanErr := os.RemoveAll(storageDir); cleanErr != nil {
			s.log.Warn("failed to clean up attachment directory after save error",
				zap.String("dir", storageDir), zap.Error(cleanErr))
		}
		return nil, fmt.Errorf("chat.Service.UploadAttachment: %w", err)
	}
	return a, nil
}

// Send saves the user message and enqueues an Agent task. Returns
// immediately with the user message ID (202 semantics). Returns
// ErrStreamInProgress only when the queue is full.
//
// User message text → single text block (emitted via emitUserMessage,
// which dual-writes to message_blocks). Attachments → Message.Attrs
// JSON ({"attachments": [...]}), NOT blocks. UI reads attrs for the
// chip rendering above the message text.
//
// Send 保存用户消息并把 Agent 任务加入队列，立刻返回用户消息 ID
// （202 语义）。仅在队列已满时返回 ErrStreamInProgress。
//
// 用户文本 → 单 text block（经 emitUserMessage 发，自动 dual-write 到
// message_blocks）。附件 → Message.Attrs JSON ({"attachments": [...]})，
// 非 block。UI 读 attrs 渲染文本上方的附件 chip。
func (s *Service) Send(ctx context.Context, conversationID string, in SendInput) (string, error) {
	conv, err := s.convRepo.Get(ctx, conversationID)
	if err != nil {
		return "", fmt.Errorf("chat.Service.Send: %w", err)
	}
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return "", fmt.Errorf("chat.Service.Send: %w", err)
	}

	// Resolve attachments → AttachmentRef list for Message.Attrs.
	// 解析附件 → AttachmentRef 列表填 Message.Attrs。
	attrs := map[string]any{}
	if len(in.AttachmentIDs) > 0 {
		refs := make([]chatdomain.AttachmentRef, 0, len(in.AttachmentIDs))
		for _, attID := range in.AttachmentIDs {
			att, err := s.repo.GetAttachment(ctx, attID)
			if err != nil {
				return "", fmt.Errorf("chat.Service.Send: attachment %q: %w", attID, err)
			}
			refs = append(refs, chatdomain.AttachmentRef{
				AttachmentID: attID, FileName: att.FileName, MimeType: att.MimeType,
			})
		}
		attrs["attachments"] = refs
	}
	// 2026-05: Attrs is map[string]any (GORM serializer:json handles storage)
	// — pass the map directly without intermediate JSON marshal here.
	// 2026-05: Attrs 直接是 map[string]any (GORM serializer 处理列存),不再
	// 手动 marshal。
	var attrsField map[string]any
	if len(attrs) > 0 {
		attrsField = attrs
	}

	// Build single text block (or empty Blocks if user sent attachments only).
	// emitUserMessage hardcodes status=completed for the SSE stop emit;
	// SaveMessage doesn't write block rows; so Status / CreatedAt fields
	// here are unread. Keep only the fields consumed downstream.
	//
	// 建单 text block（或仅附件时 Blocks 为空）。emitUserMessage 写死 SSE stop
	// 的 status=completed；SaveMessage 不写 block 行；所以这里 Status / CreatedAt
	// 字段无人读，只留下游真正消费的字段。
	var blocks []chatdomain.Block
	if in.Content != "" {
		blocks = append(blocks, chatdomain.Block{
			ID:      newBlockID(),
			Type:    eventlogdomain.BlockTypeText,
			Content: in.Content,
		})
	}

	msgID := newMsgID()
	userMsg := &chatdomain.Message{
		ID:             msgID,
		ConversationID: conversationID,
		UserID:         uid,
		Role:           chatdomain.RoleUser,
		Status:         chatdomain.StatusCompleted,
		Attrs:          attrsField,
		Blocks:         blocks,
	}
	if err := s.repo.SaveMessage(ctx, userMsg); err != nil {
		return "", fmt.Errorf("chat.Service.Send: %w", err)
	}

	// Event-log: emit the user message burst. Bridge needs conversationID
	// via reqctx; ctx from the HTTP layer doesn't carry it, so we stamp
	// here. (No need to also stamp the emitter — emitUserMessage uses
	// s.emitter directly, not eventlogpkg.From(ctx).)
	//
	// 事件日志：burst 推 user message。Bridge 经 reqctx 取 conversationID；
	// HTTP 层 ctx 不带，这里打。（不必再塞 emitter——emitUserMessage 直接
	// 用 s.emitter，不走 eventlogpkg.From(ctx)。）
	emitCtx := reqctxpkg.WithConversationID(ctx, conversationID)
	s.emitUserMessage(emitCtx, userMsg)

	agentCtx := reqctxpkg.SetUserID(context.Background(), uid)
	agentCtx = reqctxpkg.SetLocale(agentCtx, reqctxpkg.GetLocale(ctx))

	q := s.getOrCreateQueue(conversationID)
	task := queuedTask{ctx: agentCtx, conv: conv, uid: uid, userMsgID: msgID}
	select {
	case q.ch <- task:
	default:
		return "", chatdomain.ErrStreamInProgress
	}

	s.log.Info("chat task enqueued",
		zap.String("conversation_id", conversationID),
		zap.String("user_message_id", msgID),
		zap.Int("queue_depth", len(q.ch)))
	return msgID, nil
}


// Cancel stops the currently running Agent and drains any pending tasks.
//
// Cancel 停止当前正在运行的 Agent 并清空队列中待处理的任务。
func (s *Service) Cancel(_ context.Context, conversationID string) error {
	v, ok := s.queues.Load(conversationID)
	if !ok {
		return chatdomain.ErrStreamNotFound
	}
	q := v.(*convQueue)
	q.mu.Lock()
	cancel := q.cancel
	q.mu.Unlock()
	if cancel == nil {
		return chatdomain.ErrStreamNotFound
	}
	cancel()
	for {
		select {
		case <-q.ch:
		default:
			return nil
		}
	}
}

// ListMessages returns a paginated list of messages (with Blocks) for the conversation.
//
// ListMessages 返回对话的分页消息列表（含 Blocks）。
func (s *Service) ListMessages(ctx context.Context, conversationID string, filter chatdomain.ListFilter) ([]*chatdomain.Message, string, error) {
	return s.repo.ListMessagesByConversation(ctx, conversationID, filter)
}

// Package chat orchestrates the chat pipeline: queueing, attachments, auto-title, SSE.
//
// Package chat 编排聊天管线：队列、附件、自动命名、SSE 推送。
package chat

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	hooksapp "github.com/sunweilin/forgify/backend/internal/app/hooks"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	permgate "github.com/sunweilin/forgify/backend/internal/app/tool/permissionsgate"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
	memorydomain "github.com/sunweilin/forgify/backend/internal/domain/memory"
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

const queueCapacity = 5

// convQueue serialises Agent runs for one conversation; agentState lives with the queue.
//
// convQueue 串行化单 conversation 的 Agent 执行；agentState 与 queue 同生命周期。
type convQueue struct {
	ch         chan queuedTask
	mu         sync.Mutex
	cancel     context.CancelFunc
	agentState *agentstatepkg.AgentState
}

type queuedTask struct {
	ctx       context.Context
	conv      *convdomain.Conversation
	uid       string
	userMsgID string
}

// Service orchestrates LLM calls, attachments, and SSE publishing.
//
// Service 编排 LLM 调用、附件处理、SSE 推送。
type Service struct {
	repo          chatdomain.Repository
	convRepo      convdomain.Repository
	modelPicker   modeldomain.ModelPicker
	keyProvider   apikeydomain.KeyProvider
	llmFactory    *llminfra.Factory
	tools         []toolapp.Tool
	emitter       eventlogpkg.Emitter
	notifications notificationspkg.Publisher
	dataDir       string
	log           *zap.Logger
	queues        sync.Map

	catalog     catalogdomain.SystemPromptProvider
	memory      memorydomain.SystemPromptProvider
	compactor   ContextCompactor
	interceptor *toolInterceptor
}

// ContextCompactor is the chat-side port for app/contextmgr.Manager.
//
// ContextCompactor 是 app/contextmgr.Manager 的 chat 侧端口。
type ContextCompactor interface {
	MaybeCompact(ctx context.Context, convID string) error
	Calibrate(convID string, actualInputTokens, ourEstimate int)
}

// NewService wires Service dependencies; panics on nil logger; nil emitter/notifications → no-op.
//
// NewService 装配依赖；nil logger panic；nil emitter / notifications → no-op 回退。
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
		emitter = eventlogpkg.From(context.Background())
	}
	if notifications == nil {
		notifications = notificationspkg.New(nil, log)
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

// emitUserMessage bursts message_start + block lifecycle + message_stop for a synchronous user message.
//
// emitUserMessage 一次性 burst 推用户 message_start + block 生命周期 + message_stop。
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
//
// SetTools 注入 system tools 到 ReAct Agent。
func (s *Service) SetTools(tools []toolapp.Tool) {
	s.tools = tools
}

// SetSystemPromptProvider plugs the Capability Catalog; nil-tolerant.
//
// SetSystemPromptProvider 接 Capability Catalog；留 nil 安全。
func (s *Service) SetSystemPromptProvider(p catalogdomain.SystemPromptProvider) {
	s.catalog = p
}

// SetMemoryProvider plugs the long-term memory provider; nil-tolerant.
//
// SetMemoryProvider 接长期 memory provider；留 nil 安全。
func (s *Service) SetMemoryProvider(p memorydomain.SystemPromptProvider) {
	s.memory = p
}

// SetContextCompactor plugs the conversation-level token compactor; nil-tolerant.
//
// SetContextCompactor 接对话级 token 压缩器；留 nil 安全。
func (s *Service) SetContextCompactor(c ContextCompactor) {
	s.compactor = c
}

// SetPermissionsAndHooks plugs the V1.2 §3 permissions gate + hook runner.
// Either arg may be nil → that stage skipped. Call after main constructs
// the gate / runner; chat.runAgent installs the resulting interceptor
// on agentCtx so loop.runOneTool consumes it.
//
// SetPermissionsAndHooks 接 V1.2 §3 permissions gate + hook runner。
// 任一为 nil → 对应阶段跳。main 构造完调；chat.runAgent 把生成的
// interceptor 装到 agentCtx 让 loop.runOneTool 消费。
func (s *Service) SetPermissionsAndHooks(gate *permgate.Gate, hooks *hooksapp.Runner) {
	s.interceptor = newToolInterceptor(gate, hooks, s.log)
}

type SendInput struct {
	Content       string
	AttachmentIDs []string
}

// UploadAttachment copies bytes to data dir, persists metadata, returns the Attachment.
//
// UploadAttachment 把字节落盘到 data 目录、存元数据、返回 Attachment。
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

// Send persists the user message + enqueues an Agent task (202 semantics).
//
// Send 保存用户消息并入队 Agent 任务（202 语义）；队列满返 ErrStreamInProgress。
func (s *Service) Send(ctx context.Context, conversationID string, in SendInput) (string, error) {
	if strings.TrimSpace(in.Content) == "" && len(in.AttachmentIDs) == 0 {
		return "", fmt.Errorf("chat.Service.Send: %w", chatdomain.ErrEmptyContent)
	}
	conv, err := s.convRepo.Get(ctx, conversationID)
	if err != nil {
		return "", fmt.Errorf("chat.Service.Send: %w", err)
	}
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return "", fmt.Errorf("chat.Service.Send: %w", err)
	}

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
	var attrsField map[string]any
	if len(attrs) > 0 {
		attrsField = attrs
	}

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

	// Bridge needs conversationID via reqctx; HTTP-layer ctx lacks it.
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


// Cancel stops the running Agent and drains pending tasks.
//
// Cancel 停止运行中 Agent 并清空队列。
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

// ListMessages returns a paginated message list (with Blocks) for a conversation.
//
// ListMessages 返回对话的分页消息列表（含 Blocks）。
func (s *Service) ListMessages(ctx context.Context, conversationID string, filter chatdomain.ListFilter) ([]*chatdomain.Message, string, error) {
	return s.repo.ListMessagesByConversation(ctx, conversationID, filter)
}

// SumTokensForConversation returns aggregate {input, output, total}
// for convID under the ctx user. Used by GET /conversations/{id} +
// /api/v1/usage (V1.2 §4.1/§4.2).
//
// SumTokensForConversation 返 ctx 用户在 convID 下的 {input, output, total}
// 聚合。GET /conversations/{id} + /api/v1/usage 用（§4.1/§4.2）。
func (s *Service) SumTokensForConversation(ctx context.Context, convID string) (chatdomain.TokensUsed, error) {
	return s.repo.SumTokensByConversation(ctx, convID)
}

// SumTokensByPeriod groups SUM(input/output) by (provider, modelId) in
// [since, until) for the ctx user. Zero-value bounds skip.
//
// SumTokensByPeriod 按 (provider, modelId) 分组聚合 ctx 用户 [since, until)
// 区间。零值端跳过。
func (s *Service) SumTokensByPeriod(ctx context.Context, since, until time.Time) ([]chatdomain.TokensByModel, error) {
	return s.repo.SumTokensByPeriod(ctx, since, until)
}

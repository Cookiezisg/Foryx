// Package conversation owns the conversation CRUD Service (create / list / get / update /
// delete) for chat-thread containers. Workspace isolation is automatic at the orm layer, so the
// Service holds no workspace id. Lifecycle changes broadcast via notification.Emitter
// (conversation.<action>); relation hydrate + edge purge live in relations.go. The chat runtime
// reads SystemPrompt / AttachedDocuments / ModelOverride from the record — this layer only
// persists them.
//
// Package conversation 持有对话线程容器的 CRUD Service（建/列/取/改/删）。workspace 隔离在 orm 层
// 自动完成，故 Service 不持 workspace id。生命周期变更经 notification.Emitter（conversation.<动作>）
// 广播；relation hydrate + 边清理在 relations.go。chat 运行时从记录读 SystemPrompt /
// AttachedDocuments / ModelOverride——本层只持久化它们。
package conversation

import (
	"context"
	"maps"
	"strings"
	"time"

	"go.uber.org/zap"

	conversationdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	notificationdomain "github.com/sunweilin/forgify/backend/internal/domain/notification"
	searchdomain "github.com/sunweilin/forgify/backend/internal/domain/search"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
)

// Re-export the domain payload types so handlers depend on the app package only.
//
// 复用 domain 载荷类型，使 handler 只依赖 app 包。
type (
	ListFilter  = conversationdomain.ListFilter
	UpdateInput = conversationdomain.UpdateInput
)

// Service is the conversation CRUD application façade.
//
// Service 是对话 CRUD 应用 façade。
type Service struct {
	repo    conversationdomain.Repository
	search  searchdomain.Notifier // nil → search indexing disabled. nil → 不接搜索索引。
	emitter notificationdomain.Emitter
	log     *zap.Logger

	// relations is the optional relation hook; nil disables edge purge + the Namer is harmless.
	// relations 是可选 relation 钩子；nil 时禁用边清理、Namer 仍无害。
	relations RelationSyncer

	// canceler is the optional generation hook (chatapp, injected post-build like relations):
	// Delete cancels any in-flight generation so a deleted conversation can't keep burning
	// tokens and streaming into the void. nil → deletion alone.
	//
	// canceler 是可选生成钩子（chatapp，与 relations 同款后注入）：Delete 取消在途生成，使已删
	// 对话不再烧 token、不再对空推流。nil → 只删除。
	canceler GenerationCanceler

	// querier is the optional in-flight-generation reader (chatapp, injected post-build): Get/List
	// derive each row's IsGenerating from it so a freshly-connected client cold-starts its live
	// activity dots. Symmetric to canceler — same DIP port that breaks the chat↔conversation cycle.
	// nil → IsGenerating stays false.
	//
	// querier 是可选在途生成读取器（chatapp，后注入）：Get/List 据它派生每行 IsGenerating，使刚连上的
	// 客户端冷启动活动圆点。与 canceler 对称——同款 DIP 端口破 chat↔conversation 环。nil → IsGenerating 恒 false。
	querier GeneratingQuerier
}

// GenerationCanceler stops a conversation's in-flight generation (chatapp.Service satisfies it).
//
// GenerationCanceler 停止对话的在途生成（chatapp.Service 满足之）。
type GenerationCanceler interface {
	Cancel(ctx context.Context, conversationID string) error
}

// SetGenerationCanceler injects the chat cancel hook after construction (bootstrap breaks the
// chat→conversation→chat cycle this way, same as SetRelationSyncer).
//
// SetGenerationCanceler 构造后注入 chat cancel 钩子（bootstrap 以此破 chat→conversation→chat
// 环，与 SetRelationSyncer 同款）。
func (s *Service) SetGenerationCanceler(c GenerationCanceler) { s.canceler = c }

// GeneratingQuerier reports whether a conversation has an in-flight assistant turn (chatapp.Service
// satisfies it via its per-conversation queue registry).
//
// GeneratingQuerier 报告某对话是否有在途 assistant 回合（chatapp.Service 经其 per-conversation 队列
// 登记满足之）。
type GeneratingQuerier interface {
	IsGenerating(conversationID string) bool
}

// SetGeneratingQuerier injects the chat generation-state reader post-construction (same cycle-break
// as SetGenerationCanceler).
//
// SetGeneratingQuerier 构造后注入 chat 生成态读取器（与 SetGenerationCanceler 同款破环）。
func (s *Service) SetGeneratingQuerier(q GeneratingQuerier) { s.querier = q }

// markGenerating fills the derived IsGenerating flag on each row from the chat registry (no-op when
// the querier is unwired). Pure in-memory reads — no DB/IO — so it is cheap even per-row in List.
//
// markGenerating 据 chat 登记给每行填派生 IsGenerating 标志（querier 未接时 no-op）。纯内存读、无
// DB/IO，故即便 List 逐行也廉价。
func (s *Service) markGenerating(rows ...*conversationdomain.Conversation) {
	if s.querier == nil {
		return
	}
	for _, c := range rows {
		if c != nil {
			c.IsGenerating = s.querier.IsGenerating(c.ID)
		}
	}
}

// New constructs a Service; panics on nil logger. A nil emitter disables notifications (best-effort).
//
// New 构造 Service；nil logger panic。emitter 为 nil 时禁用通知（best-effort）。
func NewService(repo conversationdomain.Repository, emitter notificationdomain.Emitter, log *zap.Logger) *Service {
	if log == nil {
		panic("conversationapp.New: nil logger")
	}
	return &Service{repo: repo, emitter: emitter, log: log}
}

// SetRelationSyncer installs the relation Service post-construction (avoids an init cycle:
// relation needs conversation's Namer, conversation needs relation's syncer).
//
// SetRelationSyncer 装配后注入 relation Service（避免 init 环：relation 要 conversation 的 Namer，
// conversation 要 relation 的 syncer）。
func (s *Service) SetRelationSyncer(r RelationSyncer) { s.relations = r }

// Create makes a new conversation with the given title (may be empty → chat auto-titles later).
//
// Create 创建一个新对话，title 可为空（→ chat 后续自动命名）。
func (s *Service) Create(ctx context.Context, title string) (*conversationdomain.Conversation, error) {
	return s.CreateWithSystemPrompt(ctx, title, "")
}

// CreateWithSystemPrompt creates a thread pre-stamped with a system-prompt section — used by the
// ask-ai / triage spawner so the LLM sees entity context from turn 1.
//
// CreateWithSystemPrompt 创建带 system prompt 的新对话——ask-ai / triage 用，LLM 从第 1 轮起
// 就看到 entity 上下文。
func (s *Service) CreateWithSystemPrompt(ctx context.Context, title, systemPrompt string) (*conversationdomain.Conversation, error) {
	c := &conversationdomain.Conversation{
		ID:           idgenpkg.New("cv"),
		Title:        strings.TrimSpace(title),
		SystemPrompt: systemPrompt,
		// Seed recency to creation time so a brand-new (message-less) thread sorts by when it was
		// opened until chat bumps it on the first message (last_message_at is NOT NULL).
		// 用创建时间种 recency，使全新（无消息）线程在 chat 首条消息刷新前按开启时间排序（NOT NULL）。
		LastMessageAt: time.Now().UTC(),
	}
	if err := s.repo.Insert(ctx, c); err != nil {
		return nil, err
	}
	s.emit(ctx, c.ID, "created", map[string]any{"title": c.Title})
	return c, nil
}

// Get fetches one conversation by id (workspace-scoped by the orm layer).
//
// Get 按 id 取一条对话（orm 层按 workspace 过滤）。
func (s *Service) Get(ctx context.Context, id string) (*conversationdomain.Conversation, error) {
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	s.markGenerating(c)
	return c, nil
}

// List returns a page of conversations, each row's derived IsGenerating filled from the chat
// registry (recency-sorted by last_message_at in the store).
//
// List 返回对话的一页，每行派生 IsGenerating 据 chat 登记填（store 按 last_message_at 最近活跃排序）。
func (s *Service) List(ctx context.Context, filter ListFilter) ([]*conversationdomain.Conversation, string, error) {
	rows, next, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, "", err
	}
	s.markGenerating(rows...)
	return rows, next, nil
}

// TouchLastMessage records that a message just landed in a conversation — chat calls it on each
// user turn so the list re-sorts by recent activity. A single cheap UPDATE; best-effort (a failed
// touch only mis-sorts the list, never blocks the turn).
//
// TouchLastMessage 记一条消息刚落入对话——chat 每个用户回合调，使列表按最近活跃重排。一次廉价 UPDATE；
// best-effort（touch 失败只是列表排序略偏，绝不阻塞回合）。
func (s *Service) TouchLastMessage(ctx context.Context, id string, t time.Time) error {
	return s.repo.TouchLastMessage(ctx, id, t)
}

// Update applies a PATCH (nil = leave; for ModelOverride nil = leave, &nil = clear, &(&ref) = set).
//
// Update 部分更新（nil = 不动；ModelOverride nil = 不动、&nil = 清除、&(&ref) = 设置）。
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*conversationdomain.Conversation, error) {
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	action := "updated"
	if in.Title != nil {
		c.Title = strings.TrimSpace(*in.Title)
	}
	if in.SystemPrompt != nil {
		c.SystemPrompt = *in.SystemPrompt
	}
	if in.AttachedDocuments != nil {
		c.AttachedDocuments = *in.AttachedDocuments
	}
	if in.Archived != nil && c.Archived != *in.Archived {
		c.Archived = *in.Archived
		if c.Archived {
			action = "archived"
		} else {
			action = "unarchived"
		}
	}
	if in.Pinned != nil && c.Pinned != *in.Pinned {
		c.Pinned = *in.Pinned
		if c.Pinned {
			action = "pinned"
		} else {
			action = "unpinned"
		}
	}
	if in.ModelOverride != nil {
		ref := *in.ModelOverride // *ModelRef; nil = clear
		if err := validateModelOverride(ref); err != nil {
			return nil, err
		}
		c.ModelOverride = ref
		action = "model_override"
	}
	if err := s.repo.Update(ctx, c); err != nil {
		return nil, err
	}
	s.emit(ctx, c.ID, action, map[string]any{"title": c.Title, "archived": c.Archived, "pinned": c.Pinned})
	return c, nil
}

// Delete soft-deletes a conversation and purges its relation edges.
//
// Delete 软删对话并清除其 relation 边。
// SetAutoTitle sets a conversation's auto-generated title (chat's auto-title). It writes
// both Title and AutoTitled — a path PATCH deliberately doesn't expose (auto-title is chat-owned)
// — and emits conversation.auto_titled. A title that already exists (user-set or previously
// auto-titled) is left untouched, so a manual rename is never clobbered.
//
// SetAutoTitle 设置对话的自动生成标题（chat auto-title）。写 Title + AutoTitled——PATCH 故意
// 不暴露的路径（auto-title 由 chat 专写）——并发 conversation.auto_titled。已存在的标题（用户设或已
// 自动标题）不动，故手动改名永不被覆盖。
func (s *Service) SetAutoTitle(ctx context.Context, id, title string) error {
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if c.AutoTitled || strings.TrimSpace(c.Title) != "" {
		return nil
	}
	c.Title = strings.TrimSpace(title)
	c.AutoTitled = true
	if err := s.repo.Update(ctx, c); err != nil {
		return err
	}
	s.emit(ctx, c.ID, "auto_titled", map[string]any{"title": c.Title})
	return nil
}

// SetSummary writes the compaction summary + its watermark (app/contextmgr). A PATCH-invisible
// path (only the compactor writes it). The watermark `coversUpToSeq` is the max block seq the
// summary now folds in, so the next compaction summarizes only `(coversUpToSeq, …]` — the
// idempotent re-summarization guard. Emits conversation.compacted.
//
// SetSummary 写压缩摘要 + 其水位线（app/contextmgr）。PATCH 不暴露的路径（只压缩器写）。水位
// `coversUpToSeq` 是摘要现已并入的最大 block seq，故下次压缩只摘要 `(coversUpToSeq, …]`——幂等
// 重摘守卫。发 conversation.compacted。
func (s *Service) SetSummary(ctx context.Context, id, summary string, coversUpToSeq int64) error {
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	c.Summary = summary
	c.SummaryCoversUpToSeq = coversUpToSeq
	if err := s.repo.Update(ctx, c); err != nil {
		return err
	}
	s.emit(ctx, c.ID, "compacted", map[string]any{"coversUpToSeq": coversUpToSeq, "summaryBytes": len(summary)})
	return nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	// Stop any in-flight generation first: a deleted conversation must not keep calling the
	// LLM or streaming to a thread nobody can see.
	//
	// 先停在途生成：已删对话不该继续调 LLM、不该往没人能看的线程推流。
	if s.canceler != nil {
		if err := s.canceler.Cancel(ctx, id); err != nil {
			s.log.Warn("conversation.Delete: cancel generation failed", zap.String("id", id), zap.Error(err))
		}
	}
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		return err
	}
	s.emit(ctx, id, "deleted", nil)
	s.purgeRelations(ctx, id)
	return nil
}

// emit raises a conversation.<action> notification (persisted + SSE signal); nil emitter is a
// best-effort no-op, a soft-fail logs but never blocks the mutation.
//
// emit 发一条 conversation.<动作> 通知（持久化 + SSE signal）；nil emitter 即 best-effort no-op，
// 软失败只 log、绝不挡 mutation。
func (s *Service) emit(ctx context.Context, convID, action string, extra map[string]any) {
	s.notifySearch(ctx, convID)
	if s.emitter == nil {
		return
	}
	payload := map[string]any{"conversationId": convID}
	maps.Copy(payload, extra)
	if err := s.emitter.Emit(ctx, "conversation."+action, payload); err != nil {
		s.log.Warn("conversation emit failed",
			zap.String("conversationId", convID), zap.String("action", action), zap.Error(err))
	}
}

// validateModelOverride requires both apiKeyId and modelId when an override is set; mirrors
// agent — structural only, no key-existence probe (resolved, possibly failing gracefully, at chat time).
//
// validateModelOverride 在设了 override 时要求 apiKeyId 和 modelId 都非空；照 agent——仅结构、不探
// key 存在性（在 chat 时解析，可优雅失败）。
func validateModelOverride(o *modeldomain.ModelRef) error {
	if o == nil {
		return nil
	}
	if strings.TrimSpace(o.APIKeyID) == "" || strings.TrimSpace(o.ModelID) == "" {
		return conversationdomain.ErrInvalidModelOverride
	}
	return nil
}

// Unarchive clears the archived flag (no-op when already active) — chat's auto-unarchive on
// Send: messaging an archived thread implicitly brings it back.
//
// Unarchive 清除归档标志（已活跃则 no-op）——chat Send 的自动解档：给归档线程发消息即隐式唤回。
func (s *Service) Unarchive(ctx context.Context, id string) error {
	f := false
	_, err := s.Update(ctx, id, UpdateInput{Archived: &f})
	return err
}

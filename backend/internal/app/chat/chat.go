// Package chat is the conversation engine: it turns a user message into a persisted turn,
// drives a ReAct loop (app/loop) over the workspace's tools, streams the assistant turn live
// (messages stream), and persists the result. It is the hub of wave 5 — wiring the already-built
// conversation / messages / loop / tool / attachment / memory / document / catalog / todo / model
// pieces into one dialogue turn — but owns none of them: every dependency arrives through a port
// (DIP), so chat stays testable with a fake LLM and the real wiring lands in M7.
//
// Built across M5.2's chat sub-rounds: R0055 = engine core (chatHost / convQueue / Send /
// System Prompt / SSE message node / model resolve); R0056 = HTTP handler + Cancel + mention
// (registry / freeze-on-send / render); R0057 = auto-title + usage + system-prompt-preview.
//
// Package chat 是对话引擎：把用户消息变成持久化回合、在工作区工具上驱动 ReAct 循环（app/loop）、
// 实时推 assistant 回合（messages 流）、落盘结果。它是波次 5 的枢纽——把已建的 conversation /
// messages / loop / tool / attachment / memory / document / catalog / todo / model 拧成一个对话
// 回合——但一个都不拥有：每个依赖都经端口注入（DIP），故 chat 用 fake LLM 即可测，真实装配在 M7。
//
// 跨 M5.2 chat 子轮建成：R0055 = 引擎核心（chatHost / convQueue / Send / System Prompt / SSE
// message 节点 / model resolve）；R0056 = HTTP handler + Cancel + mention（注册表 / freeze / 渲染）；
// R0057 = auto-title + usage + system-prompt-preview。
package chat

import (
	"context"
	"strings"
	"sync"

	"go.uber.org/zap"

	humanloopapp "github.com/sunweilin/forgify/backend/internal/app/humanloop"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	toolsetpkg "github.com/sunweilin/forgify/backend/internal/app/tool/toolset"
	conversationdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	documentdomain "github.com/sunweilin/forgify/backend/internal/domain/document"
	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	notificationdomain "github.com/sunweilin/forgify/backend/internal/domain/notification"
	searchdomain "github.com/sunweilin/forgify/backend/internal/domain/search"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	agentstatepkg "github.com/sunweilin/forgify/backend/internal/pkg/agentstate"
	errorspkg "github.com/sunweilin/forgify/backend/internal/pkg/errors"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// attrAttachments is the Message.Attrs key under which Send snapshots a user turn's attachment
// ids; LoadHistory reads it back to render the multimodal content parts.
//
// attrAttachments 是 Send 把 user 回合附件 id 快照进 Message.Attrs 的键；LoadHistory 读回以渲染
// 多模态内容部件。
const attrAttachments = "attachments"

// queueCapacity is 1: the single slot only decouples Send from the drain goroutine's pickup.
// "One assistant turn in flight per conversation" is a hard contract — a Send while a turn is
// running (q.running) or already buffered (slot taken) rejects with STREAM_IN_PROGRESS instead
// of queueing behind the user's back.
//
// queueCapacity 为 1：单槽只为解耦 Send 与抽取 goroutine 的取走。「每对话同时只飞一个 assistant
// 回合」是硬契约——回合在跑（q.running）或已有一个在槽里时再 Send 直接 STREAM_IN_PROGRESS 拒绝，
// 不背着用户排队。
const queueCapacity = 1

// Errors that bubble to HTTP (R0056 handler maps them). Defined here (chat has no domain package
// — messages is the neutral content model) via errorspkg so they carry a Kind→status + a
// stable wire code, per S20. The wire codes are already registered in error-codes.md §2.4.
//
// 冒泡到 HTTP 的错误（R0056 handler 映射）。在此定义（chat 无 domain 包——messages 是中立内容
// 模型），经 errorspkg 带 Kind→status + 稳定 wire code（S20）。wire code 已登记 error-codes §2.4。
var (
	ErrEmptyContent     = errorspkg.New(errorspkg.KindInvalid, "EMPTY_CONTENT", "message has no text and no attachments")
	ErrStreamInProgress = errorspkg.New(errorspkg.KindConflict, "STREAM_IN_PROGRESS", "this conversation already has an assistant turn running")
)

// ----- DIP ports: chat depends on capabilities, never on concrete app packages. -----
// ----- DIP 端口：chat 依赖能力、不依赖具体 app 包。-----

// ConversationReader loads a conversation's thread-level config (system prompt, summary,
// attached docs, model override). The conversationapp.Service satisfies it structurally.
//
// ConversationReader 读对话线程级配置。conversationapp.Service 结构化满足。
type ConversationReader interface {
	Get(ctx context.Context, id string) (*conversationdomain.Conversation, error)
	// Unarchive clears the archived flag — Send auto-unarchives (messaging an archived thread
	// implicitly brings it back, review PD-2).
	//
	// Unarchive 清归档标志——Send 自动解档（给归档线程发消息即隐式唤回，评审 PD-2）。
	Unarchive(ctx context.Context, id string) error
}

// ContentCapabilities is what the resolved model can natively ingest — supplied by the resolver
// (from the model catalog), consumed by the attachment renderer to decide image_url vs text and
// native PDF vs sandbox-extracted text. chat owns the type so neither side imports the other.
//
// ContentCapabilities 是解析出的模型能原生吞下什么——由 resolver 给（取自 model 目录）、被附件
// 渲染器消费以决定 image_url vs 文字、原生 PDF vs sandbox 抽文本。chat 拥有该类型，两侧互不 import。
type ContentCapabilities struct {
	Vision     bool
	NativeDocs bool
}

// Bundle is a ready-to-run LLM client + a pre-filled base Request (ModelID/Key/BaseURL/Options)
// + the model's content capabilities. Mirrors agentapp.LLMBundle but adds Caps (chat renders
// attachments per the active model).
//
// Bundle 是即用 LLM client + 预填 base Request + 模型内容能力。对标 agentapp.LLMBundle，多 Caps
// （chat 按当前模型渲染附件）。
type Bundle struct {
	Client   llminfra.Client
	Request  llminfra.Request
	Caps     ContentCapabilities
	Provider string // which provider produced the turn (message provenance)
}

// ModelResolver turns the conversation's model override (nil = workspace dialogue default) into
// a runnable Bundle. The M7 adapter does model.Resolve(dialogue, override, picker) → credentials
// → factory.Build, mirroring agent's runLoop.
//
// ModelResolver 把对话的 model 覆盖（nil = workspace dialogue 默认）解析为可运行 Bundle。M7 适配器
// 做 model.Resolve(dialogue, override, picker) → credentials → factory.Build，对标 agent runLoop。
type ModelResolver interface {
	ResolveChat(ctx context.Context, override *modeldomain.ModelRef) (Bundle, error)
	// ResolveUtility resolves the workspace's utility model (a small, cheap model) for
	// background chores like auto-title — the M7 adapter does model.Resolve(ScenarioUtility, …).
	//
	// ResolveUtility 解析 workspace 的 utility 模型（小而廉价）供 auto-title 等后台杂活——M7 适配器做
	// model.Resolve(ScenarioUtility, …)。
	ResolveUtility(ctx context.Context) (Bundle, error)
}

// ConversationTitler writes a conversation's auto-generated title (auto-title, R0057). The
// conversationapp.Service satisfies it; it deliberately never clobbers a user-set title.
//
// ConversationTitler 写对话的自动生成标题（auto-title）。conversationapp.Service 满足之；它绝不
// 覆盖用户已设标题。
type ConversationTitler interface {
	SetAutoTitle(ctx context.Context, conversationID, title string) error
}

// AttachmentRenderer turns a user turn's attachment ids into neutral multimodal content parts,
// gated by the model's capabilities. The attachmentapp.Service satisfies it (adapter converts
// ContentCapabilities → attachment.Capabilities).
//
// AttachmentRenderer 把 user 回合的附件 id 渲成中立多模态内容部件，按模型能力门控。
// attachmentapp.Service 满足之（适配器转 ContentCapabilities → attachment.Capabilities）。
type AttachmentRenderer interface {
	ToContentParts(ctx context.Context, ids []string, caps ContentCapabilities) ([]llminfra.ContentPart, error)
}

// MemoryProvider / CatalogProvider / DocumentRenderer / TodoReminder are the System Prompt and
// live-reminder sources, each a one-method projection of an already-built service.
//
// MemoryProvider / CatalogProvider / DocumentRenderer / TodoReminder 是 System Prompt 与 live
// reminder 的来源，各是某已建 service 的单方法投影。
type (
	MemoryProvider interface {
		ForSystemPrompt(ctx context.Context) string
	}
	CatalogProvider interface {
		GetForSystemPrompt(ctx context.Context) string
	}
	DocumentRenderer interface {
		RenderAttached(ctx context.Context, atts []documentdomain.AttachedDocument) (string, error)
	}
	TodoReminder interface {
		SystemReminder(ctx context.Context) (string, bool)
	}
)

// Deps are chat's injected collaborators (DIP). Grouped so New stays readable; M7 fills the real
// implementations, tests fill fakes. A nil optional provider degrades that System Prompt section
// to empty (chat never hard-requires memory/catalog/documents to be wired).
//
// Deps 是 chat 注入的协作者（DIP）。分组使 New 可读；M7 填真实现、测试填 fake。可选 provider 为
// nil 时该 System Prompt 段降级为空（chat 不硬要求 memory/catalog/documents 接线）。
type Deps struct {
	Conversations  ConversationReader
	Resolver       ModelResolver
	Attachments    AttachmentRenderer
	Toolset        toolapp.Toolset
	Memory         MemoryProvider
	Catalog        CatalogProvider
	Documents      DocumentRenderer
	Todo           TodoReminder
	Bridge         streamdomain.Bridge        // messages stream instance; nil → no live push (REST history still works)
	EntitiesBridge streamdomain.Bridge        // entities stream (SSE-C): loop mirrors forge tool_call deltas here; nil → no entity-panel live fill
	Titler         ConversationTitler         // auto-title writer (R0057); nil → no auto-titling
	Notifier       notificationdomain.Emitter // auto-title notification (R0057); nil → no notify
	Compactor      Compactor                  // context compaction (R0059); nil → no compaction
}

// Compactor compacts a conversation when it nears the model's context window (contextmgr M5.3).
// chat calls it at the tail of a turn, on a detached context inside the per-conversation queue
// slot (so it can't race the next turn's history load). nil → compaction is off.
//
// Compactor 在对话逼近模型 context window 时压缩它（contextmgr M5.3）。chat 在回合尾、per-conversation
// queue 槽内的 detached context 上调用（故与下回合历史加载无竞态）。nil → 关闭压缩。
type Compactor interface {
	MaybeCompact(ctx context.Context, conversationID string) error
}

// Service is the chat engine. messages is the persistence (R0054); the rest arrive via Deps.
// queues holds one convQueue per active conversation; wg tracks their goroutines for shutdown.
//
// Service 是 chat 引擎。messages 是持久化（R0054）；其余经 Deps。queues 每活跃对话一个 convQueue；
// wg 追踪其 goroutine 供关停。
type Service struct {
	messages         messagesdomain.Repository
	search           searchdomain.Notifier // nil → search indexing disabled. nil → 不接搜索索引。
	deps             Deps
	searchTool       toolapp.Tool                                         // search_tools, built once from Deps.Toolset.Lazy; resident in every turn
	mentionResolvers map[mentiondomain.MentionType]mentiondomain.Resolver // @-mention resolvers, registered per type at M7
	log              *zap.Logger

	// broker is the human-in-the-loop broker (R0064): seeded into every turn's ctx so the loop's
	// danger gate + the ask_user tool can block for a human decision, resolved via ResolveInteraction.
	//
	// broker 是人在环 broker（R0064）：seed 进每个回合的 ctx，使 loop 的 danger 门 + ask_user 工具能阻塞等人
	// 决定，经 ResolveInteraction 决议。
	broker *humanloopapp.Broker

	queues sync.Map // conversationID → *convQueue
	wg     sync.WaitGroup
	stop   chan struct{} // closed by Shutdown: short-circuits every runQueue loop
}

// New constructs the chat Service. nil messages / log is a wiring bug; Deps fields may be nil
// (optional providers degrade gracefully). search_tools is built from the lazy partition so the
// LLM can pull a lazy tool's full schema on demand.
//
// New 构造 chat Service。nil messages / log 是装配 bug；Deps 字段可为 nil（可选 provider 优雅降级）。
// search_tools 由 lazy 划分构造，使 LLM 按需拉取 lazy 工具完整 schema。
func New(messages messagesdomain.Repository, deps Deps, log *zap.Logger) *Service {
	if messages == nil || log == nil {
		panic("chatapp.New: nil messages repository or logger")
	}
	s := &Service{
		messages:         messages,
		deps:             deps,
		searchTool:       toolsetpkg.NewSearchTools(deps.Toolset.Lazy),
		mentionResolvers: map[mentiondomain.MentionType]mentiondomain.Resolver{},
		log:              log,
		stop:             make(chan struct{}),
	}
	s.broker = humanloopapp.New(s.interactionSurface)
	return s
}

// SendInput is the user's turn: text plus referenced attachment ids. Mentions are deferred to
// R0056 (the resolver registry + <mentions> rendering); the field is reserved so the Send API is
// stable across the two sub-rounds.
//
// SendInput 是用户回合：文本 + 引用的附件 id。Mentions 留 R0056（resolver 注册表 + <mentions> 渲染）；
// 字段预留使 Send API 在两个子轮间稳定。
type SendInput struct {
	Content       string
	AttachmentIDs []string
	Mentions      []mentiondomain.MentionInput // @-references, frozen to content snapshots at send time
}

// Send persists the user turn, opens an assistant turn (streaming), emits message_start, and
// enqueues the generation — returning the assistant message id immediately (202 semantics; the
// turn streams over the messages SSE). STREAM_IN_PROGRESS if the conversation is already running.
//
// Send 落用户回合、开 assistant 回合（streaming）、发 message_start、入队生成——立即返回 assistant
// message id（202 语义；回合经 messages SSE 流式）。对话已在跑则 STREAM_IN_PROGRESS。
func (s *Service) Send(ctx context.Context, conversationID string, in SendInput) (string, error) {
	// Trimmed gate: a whitespace-only message is as empty as "" — it would persist a blank
	// user turn and bill a model call for nothing.
	// trim 后再判：纯空白消息等同空串——否则落一条空白 user 回合、为空内容白付一次模型调用。
	if strings.TrimSpace(in.Content) == "" && len(in.AttachmentIDs) == 0 {
		return "", ErrEmptyContent
	}
	// Existence gate: without it a Send to a deleted / unknown conversation persists an orphan
	// user turn (and an assistant row) that only fails later inside processTask — 404 first.
	//
	// 存在性闸：没有它，发往已删/未知对话的 Send 会先落孤儿 user 回合（和 assistant 行），到
	// processTask 里才失败——先 404。
	conv, err := s.deps.Conversations.Get(ctx, conversationID)
	if err != nil {
		return "", err
	}
	// Auto-unarchive (PD-2): sending to an archived thread implicitly brings it back. Soft-fail
	// — a failed flag flip must not block the message itself.
	//
	// 自动解档（PD-2）：给归档线程发消息即隐式唤回。软失败——标志翻转失败不挡消息本身。
	if conv.Archived {
		if err := s.deps.Conversations.Unarchive(ctx, conversationID); err != nil {
			s.log.Warn("chatapp.Send: auto-unarchive failed", zap.String("conversationId", conversationID), zap.Error(err))
		}
	}

	// Persist the user turn (one text block + attachment ids snapshotted in Attrs) and echo it
	// to the stream so other clients see it immediately.
	//
	// 落用户回合（一个 text block + 附件 id 快照进 Attrs）并回显到流，使其他客户端立即看到。
	userMsg := &messagesdomain.Message{
		ID:             idgenpkg.New("msg"),
		ConversationID: conversationID,
		Role:           messagesdomain.RoleUser,
		Status:         messagesdomain.StatusCompleted,
	}
	attrs := map[string]any{}
	if len(in.AttachmentIDs) > 0 {
		attrs[attrAttachments] = in.AttachmentIDs
	}
	if snaps := s.resolveMentions(ctx, in.Mentions); len(snaps) > 0 {
		attrs[attrMentions] = snaps // freeze-on-send: snapshot @-mentioned entities' content now
	}
	if len(attrs) > 0 {
		userMsg.Attrs = attrs
	}
	var userBlocks []messagesdomain.Block
	if in.Content != "" {
		userBlocks = []messagesdomain.Block{{Type: messagesdomain.BlockTypeText, Content: in.Content}}
	}
	if err := s.messages.CreateMessage(ctx, userMsg, userBlocks); err != nil {
		return "", err
	}
	s.notifySearchMessage(ctx, conversationID, userMsg.ID)
	s.emitUserMessage(ctx, conversationID, userMsg, in.Content)

	// Open the assistant turn (streaming, no blocks yet) to mint its id for the live stream
	// anchor and reqctx seed, then emit message_start.
	//
	// 开 assistant 回合（streaming、暂无 block）以 mint id 作流锚点 + reqctx 种子，再发 message_start。
	asstMsg := &messagesdomain.Message{
		ID:             idgenpkg.New("msg"),
		ConversationID: conversationID,
		Role:           messagesdomain.RoleAssistant,
		Status:         messagesdomain.StatusStreaming,
	}
	if err := s.messages.CreateMessage(ctx, asstMsg, nil); err != nil {
		return "", err
	}
	s.emitMessageStart(ctx, conversationID, asstMsg.ID)

	// Enqueue: carries the per-run identity the detached queue goroutine needs (the Send ctx is
	// gone by the time the turn runs).
	//
	// 入队：携带脱离的队列 goroutine 所需的 per-run 身份（回合运行时 Send ctx 已消失）。
	wsID, _ := reqctxpkg.GetWorkspaceID(ctx)
	t := task{
		assistantMsgID: asstMsg.ID,
		workspaceID:    wsID,
		locale:         reqctxpkg.GetLocale(ctx),
	}
	if err := s.enqueue(conversationID, t); err != nil {
		// Roll the assistant turn to error so it isn't a permanent streaming orphan.
		// 把 assistant 回合落为 error，使其不成永久 streaming 孤儿。
		asstMsg.Status = messagesdomain.StatusError
		asstMsg.StopReason = messagesdomain.StopReasonError
		asstMsg.ErrorCode = "STREAM_IN_PROGRESS"
		_ = s.messages.FinalizeMessage(ctx, asstMsg, nil)
		return "", err
	}
	return asstMsg.ID, nil
}

// task is one queued generation. It carries only the per-run identity (assistant message id +
// workspace + locale); everything else (conversation config, model, tools) is re-derived in
// processTask from the conversation id (the queue's key).
//
// task 是一次入队生成。只携带 per-run 身份（assistant message id + workspace + locale）；其余
// （对话配置、模型、工具）在 processTask 据对话 id（队列键）重新求得。
type task struct {
	assistantMsgID string
	workspaceID    string
	locale         reqctxpkg.Locale
}

// convQueue serializes one conversation's generations: a single goroutine drains a small buffered
// channel, so only one assistant turn runs at a time (which makes the per-conversation seq
// allocation race-free, R0054). agentState is shared across the conversation's turns; cancel
// holds the running turn's cancel func (the cancel endpoint, R0056, calls it).
//
// convQueue 串行化一个对话的生成：单 goroutine 抽干小缓冲 channel，故同一时刻只跑一个 assistant 回合
// （这使 per-conversation seq 分配无竞争，R0054）。agentState 跨对话的回合共享；cancel 持运行中回合的
// cancel func（cancel 端点 R0056 调它）。
type convQueue struct {
	ch         chan task
	agentState *agentstatepkg.AgentState
	mu         sync.Mutex
	cancel     context.CancelFunc
	dead       bool // set under mu by runQueue at teardown; enqueue re-creates on sight
	running    bool // set under mu by runQueue around processTask; enqueue rejects while true. 运行中标志，enqueue 见之即拒。
}

// setRunning flips the in-flight flag under q.mu (enqueue reads it to enforce the
// one-turn-in-flight contract — the dequeued task is no longer visible in the channel).
//
// setRunning 在 q.mu 下翻转在飞标志（enqueue 读它执行「同时一回合」契约——被取走的 task
// 在 channel 里已不可见）。
func (q *convQueue) setRunning(v bool) {
	q.mu.Lock()
	q.running = v
	q.mu.Unlock()
}

// enqueue gets-or-creates the conversation's queue and offers the task; a full buffer means a
// turn is already running (or backlogged) → STREAM_IN_PROGRESS. The send happens under q.mu
// after checking q.dead: runQueue's teardown (mark dead + final drain) runs under the same
// lock, so a task can never land in a channel nobody will ever read — it either arrives before
// the final drain or sees dead and retries on a fresh queue.
//
// enqueue 取/建对话队列并投递 task；缓冲满 = 已有回合在跑（或积压）→ STREAM_IN_PROGRESS。投递在
// q.mu 下、先查 q.dead：runQueue 的拆卸（标 dead + 最终抽干）在同一锁下进行，故 task 不可能落进
// 永远无人读的 channel——要么赶在最终抽干前到达、要么看见 dead 在新队列重试。
func (s *Service) enqueue(conversationID string, t task) error {
	for {
		q := s.getOrCreateQueue(conversationID)
		q.mu.Lock()
		if q.dead {
			q.mu.Unlock()
			continue // teardown won the race — the map entry is gone; build a fresh queue
		}
		if q.running {
			q.mu.Unlock()
			return ErrStreamInProgress
		}
		select {
		case q.ch <- t:
			q.mu.Unlock()
			return nil
		default:
			q.mu.Unlock()
			return ErrStreamInProgress
		}
	}
}

// getOrCreateQueue atomically returns the conversation's queue, starting its drain goroutine on
// first use. The idle goroutine self-destructs after idleTimeout (runner.go), reclaiming memory
// for dormant conversations.
//
// getOrCreateQueue 原子返回对话队列，首次使用时启动其抽取 goroutine。空闲 goroutine 在 idleTimeout
// 后自毁（runner.go），为休眠对话回收内存。
func (s *Service) getOrCreateQueue(conversationID string) *convQueue {
	if existing, ok := s.queues.Load(conversationID); ok {
		return existing.(*convQueue)
	}
	q := &convQueue{ch: make(chan task, queueCapacity), agentState: agentstatepkg.New()}
	actual, loaded := s.queues.LoadOrStore(conversationID, q)
	if loaded {
		return actual.(*convQueue)
	}
	s.wg.Add(1)
	go s.runQueue(conversationID, q)
	return q
}

// Shutdown stops all conversation goroutines promptly: it cancels every running turn (loop
// aborts; WriteFinalize lands a cancelled terminal on its detached context — no streaming
// orphan), closes s.stop so every runQueue exits on its next select instead of waiting out the
// 5-minute idle timer, then waits. Queued-but-unstarted tasks are NOT finalized here — the boot
// sweep (SweepOrphans) reconciles their streaming rows on next start, same as a hard crash.
//
// Shutdown 立即停下所有对话 goroutine：先取消每个在跑回合（loop 中断；WriteFinalize 在 detached
// context 落 cancelled 终态——无 streaming 孤儿），再 close s.stop 使每个 runQueue 在下次 select
// 即退（而非等满 5 分钟 idle timer），最后等待。已入队未开始的 task 不在此落账——下次启动的
// boot 对账（SweepOrphans）收拾其 streaming 行，与硬崩溃同一路径。
func (s *Service) Shutdown() {
	close(s.stop)
	s.queues.Range(func(_, v any) bool {
		q := v.(*convQueue)
		q.mu.Lock()
		if q.cancel != nil {
			q.cancel()
		}
		q.mu.Unlock()
		return true
	})
	s.wg.Wait()
}

// ListMessages returns one keyset page of a conversation's turns (each with its blocks) for the
// REST history endpoint — a thin pass-through to the messages store (N4 pagination, newest-first).
//
// ListMessages 返回一个对话回合的一页 keyset（每条带 blocks）给 REST 历史端点——薄转 messages
// store（N4 分页、最新在前）。
func (s *Service) ListMessages(ctx context.Context, conversationID, cursor string, limit int) ([]*messagesdomain.Message, string, error) {
	return s.messages.ListMessages(ctx, conversationID, cursor, limit)
}

// SystemPromptPreview builds the system prompt a turn in this conversation would receive — the
// GET /system-prompt-preview endpoint (transparency / debugging). Reuses buildSystemPrompt; no
// model is resolved (the prompt doesn't depend on the model).
//
// SystemPromptPreview 构建本对话一个回合会收到的 system prompt——GET /system-prompt-preview 端点
// （透明度 / 调试）。复用 buildSystemPrompt；不解析模型（prompt 不依赖模型）。
func (s *Service) SystemPromptPreview(ctx context.Context, conversationID string) (string, error) {
	conv, err := s.deps.Conversations.Get(ctx, conversationID)
	if err != nil {
		return "", err
	}
	return s.buildSystemPrompt(ctx, conv), nil
}

// Usage returns a conversation's total input + output token cost across all turns — the
// GET /usage endpoint (the tokensUsed the conversation detail shows).
//
// Usage 返回一个对话所有回合的 input + output token 总成本——GET /usage 端点（对话详情的 tokensUsed）。
func (s *Service) Usage(ctx context.Context, conversationID string) (inputTokens, outputTokens int, err error) {
	return s.messages.SumTokens(ctx, conversationID)
}

// Cancel stops a conversation's generation (the DELETE stream endpoint, R0056): it triggers the
// running turn's context cancel — loop's stream aborts and WriteFinalize lands a cancelled
// terminal on its detached context — and drains any queued-but-unstarted turns, finalizing each
// as cancelled so none becomes a streaming orphan. No active queue → a graceful no-op.
//
// Cancel 停止一个对话的生成（DELETE stream 端点）：触发运行回合的 context cancel——loop 流式中断、
// WriteFinalize 在其 detached context 落 cancelled 终态——并清空已入队未开始的回合，逐个落
// cancelled 终态使无 streaming 孤儿。无活跃队列 → 优雅 no-op。
func (s *Service) Cancel(_ context.Context, conversationID string) error {
	v, ok := s.queues.Load(conversationID)
	if !ok {
		return nil
	}
	q := v.(*convQueue)

	q.mu.Lock()
	cancel := q.cancel
	q.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	// Drain queued-but-unstarted turns and finalize each as cancelled (they hold a streaming
	// assistant row from Send that would otherwise hang forever).
	//
	// 清空已入队未开始的回合并逐个落 cancelled（它们持 Send 建的 streaming assistant 行，否则永挂）。
	for {
		select {
		case t := <-q.ch:
			s.finalizeCancelled(conversationID, t.assistantMsgID, t.workspaceID)
		default:
			return nil
		}
	}
}

// finalizeCancelled marks a never-started assistant turn cancelled + pushes message_stop, on a
// detached context (same orphan-avoidance discipline as WriteFinalize).
//
// finalizeCancelled 把一个从未开始的 assistant 回合标 cancelled + 推 message_stop，在 detached
// context 上（与 WriteFinalize 同一防孤儿纪律）。
func (s *Service) finalizeCancelled(conversationID, msgID, workspaceID string) {
	dctx := reqctxpkg.Detached(workspaceID)
	dctx = reqctxpkg.SetConversationID(dctx, conversationID)
	m := &messagesdomain.Message{
		ID:             msgID,
		ConversationID: conversationID,
		Role:           messagesdomain.RoleAssistant,
		Status:         messagesdomain.StatusCancelled,
		StopReason:     messagesdomain.StopReasonCancelled,
	}
	if err := s.messages.FinalizeMessage(dctx, m, nil); err != nil {
		s.log.Warn("chatapp.finalizeCancelled: finalize failed", zap.String("messageId", msgID), zap.Error(err))
	}
	s.emitMessageStop(dctx, conversationID, m)
}

// SweepOrphans force-finalizes turns left in a non-terminal status by a hard crash (or a
// shutdown that outpaced the queue) — called once per workspace at boot, the messages
// counterpart of scheduler.Recover. Detached/graceful paths never need it; death does.
//
// SweepOrphans 强制收尾被硬崩溃（或快过队列的关停）留在非终态的回合——boot 时每 workspace 调一次，
// 是 scheduler.Recover 的 messages 对应物。detached/优雅路径用不到它；进程死亡用得到。
func (s *Service) SweepOrphans(ctx context.Context) {
	n, err := s.messages.SweepNonTerminal(ctx)
	if err != nil {
		s.log.Warn("chatapp.SweepOrphans failed", zap.Error(err))
		return
	}
	if n > 0 {
		s.log.Info("chatapp: swept orphaned non-terminal turns", zap.Int("count", n))
	}
}

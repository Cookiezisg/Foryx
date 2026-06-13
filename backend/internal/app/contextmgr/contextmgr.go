// Package contextmgr is the conversation-compaction engine (M5.3): when a thread approaches the
// model's context window it compacts older history so the conversation keeps fitting. It is the
// PRODUCE side — the consume side (loop.BlocksToAssistantLLM dropping archived/compaction +
// projecting warm/cold; chat.LoadHistory prepending the summary and dropping seq ≤ watermark) is
// already wired. A turn-boundary, two-step pipeline (gentle→aggressive, industry-standard):
//
//	① demote old tool_results (LLM-free): newest stay hot, then warm (preview), then cold
//	   (placeholder). Often enough on its own — tool outputs dominate token usage.
//	② if still over budget, summarize the oldest span once (utility model), fold it into the
//	   conversation summary incrementally, and advance the watermark.
//
// Trigger uses the real persisted InputTokens of the last turn (ground truth, no estimator);
// the step-① gate uses a cheap bytes/4 estimate. The watermark (summary_covers_up_to_seq) is the
// idempotency key: re-summarization only covers (watermark, …], and a crash between writing the
// summary and flipping the archived flag can't double-count (LoadHistory drops by watermark).
//
// Package contextmgr 是对话压缩引擎（M5.3）：线程逼近模型 context window 时压缩旧历史，使对话持续
// 装得下。它是**生产侧**——消费侧（loop.BlocksToAssistantLLM 丢 archived/compaction + 投影
// warm/cold；chat.LoadHistory 前置 summary + 丢 seq ≤ 水位）已接好。回合边界、两步管线
// （gentle→aggressive，业界标准）：① demote 旧 tool_result（免 LLM：最新留 hot、再 warm 预览、再
// cold 占位符；常就够——工具输出占 token 大头）；② 仍超预算则单次摘要最旧 span（utility 模型），增量
// 并入对话 summary、推进水位。触发用末回合**真实** InputTokens（真相源、免估算器）；步①闸用
// bytes/4 廉价估算。水位（summary_covers_up_to_seq）是幂等键：重摘只覆盖 (水位, …]，写 summary 与翻
// archived 标记间崩溃也不重复计数（LoadHistory 按水位丢弃）。
package contextmgr

import (
	"context"

	"go.uber.org/zap"

	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	limitspkg "github.com/sunweilin/forgify/backend/internal/pkg/limits"
)

const (
	// limitspkg.Current().Context.TriggerRatio compacts when the last turn's real input tokens reach this fraction of the
	// input budget (window − maxOutput). 0.80 sits in the 75–90% band recommended for quality
	// (context rot sets in before the hard limit); the 20% slack is the compaction headroom.
	//
	// limitspkg.Current().Context.TriggerRatio：末回合真实 input token 达 input 预算（window − maxOutput）此比例时压缩。0.80 在
	// 业界推荐的 75–90% 区间（硬上限前已现 context rot）；20% 余量即压缩 headroom。

	// recentTurns most recent messages are never touched (verbatim floor — the actual current
	// task must always be present unsummarized).
	//
	// 最近 recentTurns 条 message 永不动（逐字底线——当前任务必须始终未摘要在场）。
	recentTurns = 4

	// Among non-protected tool_results (newest-first), the first recentTRHot stay hot, the next
	// warmZone become warm (truncated preview), the rest cold (placeholder). A tool_result only
	// ever ages hot→warm→cold (its newness rank only grows), so demotion never promotes.
	//
	// 非保护 tool_result 中（新→旧），前 recentTRHot 留 hot、接着 warmZone 个转 warm（截断预览）、其余
	// cold（占位符）。tool_result 只会 hot→warm→cold 老化（新近名次只增），故 demote 绝不升级。
	recentTRHot = 4
	warmZone    = 8

	// bytesPerToken is the cheap estimate ratio for the step-① gate only (the trigger uses real
	// tokens). ~4 bytes/token is the common heuristic; it under-counts the system prompt (absent
	// here), but the gate is self-correcting — if demotion wasn't enough, the next turn's real
	// usage re-triggers.
	//
	// bytesPerToken 仅供步①闸的廉价估算（触发用真实 token）。~4 字节/token 是通用近似；它漏算 system
	// prompt（此处无），但闸自校正——demote 不够则下回合真实用量再触发。
	bytesPerToken = 4

	// maxBlockExcerptBytes caps each block's contribution to the summary prompt (a single huge
	// block can't blow the summarizer's own context).
	//
	// maxBlockExcerptBytes 限每个 block 进摘要 prompt 的量（单个巨大 block 不能冲爆摘要器自己的上下文）。
	maxBlockExcerptBytes = 1500

	// warmPreviewBytes mirrors loop's warm projection, so the gate estimate matches what the LLM
	// will actually receive.
	//
	// warmPreviewBytes 镜像 loop 的 warm 投影，使闸估算与 LLM 实收一致。
	warmPreviewBytes = 200
)

// Bundle is a ready utility-model client + pre-filled base Request (self-contained — contextmgr
// doesn't import chatapp). The summary call sets Request.System + Messages and runs llm.Generate.
//
// Bundle 是即用 utility 模型 client + 预填 base Request（自包含——contextmgr 不引 chatapp）。摘要调用
// 设 Request.System + Messages 后跑 llm.Generate。
type Bundle struct {
	Client  llminfra.Client
	Request llminfra.Request
}

// ----- DIP ports -----

// ConversationSummary reads/writes a conversation's running summary + watermark. Narrow (no
// domain type leak); the M7 adapter wraps conversation.Service.
//
// ConversationSummary 读写一个对话的滚动 summary + 水位。窄口（不泄漏 domain 类型）；M7 适配器包
// conversation.Service。
type ConversationSummary interface {
	GetSummary(ctx context.Context, conversationID string) (summary string, coversUpToSeq int64, err error)
	SetSummary(ctx context.Context, conversationID, summary string, coversUpToSeq int64) error
}

// UtilityResolver yields the workspace utility model (small/cheap) for the summary call (same
// model auto-title uses).
//
// UtilityResolver 给出 workspace utility 模型（小/廉价）供摘要调用（与 auto-title 同模型）。
type UtilityResolver interface {
	ResolveUtility(ctx context.Context) (Bundle, error)
}

// WindowResolver gives a model's context window + max output tokens (from llminfra.ModelInfo).
// (0, 0) when unknown → compaction is skipped (don't compact without a known budget).
//
// WindowResolver 给出一个模型的 context window + max output token（取自 llminfra.ModelInfo）。
// 未知时 (0, 0) → 跳过压缩（不知预算不压）。
type WindowResolver interface {
	ContextBudget(ctx context.Context, provider, modelID string) (window, maxOutput int)
}

// Deps are contextmgr's injected collaborators (DIP).
//
// Deps 是 contextmgr 注入的协作者（DIP）。
type Deps struct {
	Messages      messagesdomain.Repository
	Conversations ConversationSummary
	Resolver      UtilityResolver
	Windows       WindowResolver
}

// Service compacts conversations.
//
// Service 压缩对话。
type Service struct {
	deps Deps
	log  *zap.Logger
}

// New constructs the Service. nil log → no-op.
//
// New 构造 Service。nil log → no-op。
func New(deps Deps, log *zap.Logger) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	return &Service{deps: deps, log: log.Named("contextmgr")}
}

// MaybeCompact compacts the conversation if its last turn's real input tokens crossed the
// trigger. Best-effort + idempotent: chat calls it on a detached context at the tail of a turn
// (inside the per-conversation queue slot, so no race with the next turn's history load); a
// returned error is logged non-fatally by the caller. Under threshold / unknown window / nothing
// to compact → returns nil without writing.
//
// MaybeCompact 在对话末回合真实 input token 越过触发线时压缩。best-effort + 幂等：chat 在回合尾、
// detached context 上调用（在 per-conversation queue 槽内，与下回合历史加载无竞态）；返回的 error 由
// 调用方非致命记录。未达阈值 / window 未知 / 无可压 → 返 nil 不写。
func (s *Service) MaybeCompact(ctx context.Context, conversationID string) error {
	thread, err := s.deps.Messages.LoadThread(ctx, conversationID)
	if err != nil {
		return err
	}
	last := lastMeasuredTurn(thread)
	if last == nil {
		return nil // no completed turn with token accounting yet
	}
	window, maxOutput := s.deps.Windows.ContextBudget(ctx, last.Provider, last.ModelID)
	inputBudget := window - maxOutput
	if window <= 0 || inputBudget <= 0 {
		return nil // unknown budget — don't compact blind
	}
	if last.InputTokens < int(limitspkg.Current().Context.TriggerRatio*float64(inputBudget)) {
		return nil // under threshold
	}

	summary, watermark, err := s.deps.Conversations.GetSummary(ctx, conversationID)
	if err != nil {
		return err
	}
	protectedFrom := max(0, len(thread)-recentTurns)

	// ① demote old tool_results (LLM-free); mutates thread roles in place + persists.
	// ① demote 旧 tool_result（免 LLM）；原地改 thread 角色 + 落盘。
	s.demote(ctx, thread, protectedFrom)

	// Gate: if the projected size is now under the trigger, demotion sufficed — skip the LLM.
	// 闸：投影大小已低于触发线则 demote 足够——跳过 LLM。
	if s.estimateTokens(thread, summary) < int(limitspkg.Current().Context.TriggerRatio*float64(inputBudget)) {
		return nil
	}

	// ② summarize the oldest non-protected span into the running summary.
	// ② 把最旧的非保护 span 摘要并入滚动 summary。
	return s.summarize(ctx, conversationID, thread, protectedFrom, summary, watermark)
}

// lastMeasuredTurn returns the most recent assistant turn carrying real token accounting (its
// InputTokens = the full context the provider billed for that request = current context size).
//
// lastMeasuredTurn 返回最近一个带真实 token 记账的 assistant 回合（其 InputTokens = provider 为该
// 请求计费的完整上下文 = 当前上下文大小）。
func lastMeasuredTurn(thread []*messagesdomain.Message) *messagesdomain.Message {
	for i := len(thread) - 1; i >= 0; i-- {
		m := thread[i]
		if m.SubagentID == "" && m.Role == messagesdomain.RoleAssistant && m.InputTokens > 0 {
			return m
		}
	}
	return nil
}

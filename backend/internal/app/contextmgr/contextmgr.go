// Package contextmgr runs conversation-level token compaction after each AI turn.
//
// Package contextmgr 在每个 AI turn 后跑对话级 token 压缩。
package contextmgr

import (
	"context"
	"errors"
	"sync"

	"go.uber.org/zap"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	modelcapspkg "github.com/sunweilin/forgify/backend/internal/pkg/modelcaps"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
)

// ErrCompactFailed is the manual-trigger failure sentinel; auto path swallows + retries.
//
// ErrCompactFailed 手动触发失败的 sentinel；自动路径吞错后重试。
var ErrCompactFailed = errors.New("contextmgr: compact failed")

// LLMResolver builds a cheap LLM bundle for summary generation (injection point for fake LLMs in tests).
// thinking may be nil (= auto); compact.go sets Request.Thinking so adapters can encode it.
//
// LLMResolver 为摘要生成构造便宜 LLM bundle（测试可注入 fake）。
// thinking 可为 nil（= auto）；compact.go 把它赋给 Request.Thinking。
type LLMResolver func(ctx context.Context) (client llminfra.Client, modelID, key, baseURL string, thinking *llminfra.ThinkingSpec, err error)

// Thresholds controls when compaction kicks in.
//
// Thresholds 控压缩触发点。
type Thresholds struct {
	Soft           float64
	Hard           float64
	RecentTurns    int
	RecentTRKeep   int
	WarmCutoff     int
	MaxSummaryRune int
}

func DefaultThresholds() Thresholds {
	return Thresholds{
		Soft:           0.70,
		Hard:           0.85,
		RecentTurns:    3,
		RecentTRKeep:   5,
		WarmCutoff:     15,
		MaxSummaryRune: 6000,
	}
}

// CapabilityResolver returns the effective model capability for (provider, modelID).
// Used by estimate to read the real context window instead of a static fallback.
//
// CapabilityResolver 返回 (provider, modelID) 的有效模型能力，用于 estimate 读真实窗口。
type CapabilityResolver func(ctx context.Context, provider, modelID string) modelcapspkg.Cap

// Manager orchestrates demotion + compaction; MaybeCompact is the only entry point.
//
// Manager 编排降级 + 压缩；MaybeCompact 是唯一入口。
type Manager struct {
	chatRepo   chatdomain.Repository
	convRepo   convdomain.Repository
	emitter    eventlogpkg.Emitter
	notif      notificationspkg.Publisher
	resolveLLM LLMResolver
	capFor     CapabilityResolver
	log        *zap.Logger
	thr        Thresholds

	calMu sync.Mutex
	cal   map[string]float64

	nilCapOnce sync.Once
}

// New wires Manager dependencies; panics on nil log; nil resolveLLM disables fullCompact.
//
// New 装配依赖；nil log panic；nil resolveLLM 关 fullCompact，仅降级。
func New(
	chatRepo chatdomain.Repository,
	convRepo convdomain.Repository,
	emitter eventlogpkg.Emitter,
	notif notificationspkg.Publisher,
	resolveLLM LLMResolver,
	log *zap.Logger,
) *Manager {
	if log == nil {
		panic("contextmgr.New: logger is nil")
	}
	if notif == nil {
		notif = notificationspkg.New(nil, log)
	}
	return &Manager{
		chatRepo:   chatRepo,
		convRepo:   convRepo,
		emitter:    emitter,
		notif:      notif,
		resolveLLM: resolveLLM,
		log:        log.Named("contextmgr"),
		thr:        DefaultThresholds(),
		cal:        map[string]float64{},
	}
}

// SetThresholds overrides defaults; safe before first MaybeCompact.
//
// SetThresholds 覆盖默认值；首次 MaybeCompact 前安全。
func (m *Manager) SetThresholds(t Thresholds) { m.thr = t }

// SetCapabilityResolver injects the per-(provider,model) window resolver; nil disables window-aware sizing.
//
// SetCapabilityResolver 注入 per-(provider,model) 窗口解析器；nil 时退回保守默认。
func (m *Manager) SetCapabilityResolver(r CapabilityResolver) { m.capFor = r }

// MaybeCompact runs the demote-or-compact dispatch once; auto-trigger path swallows errors.
// provider and modelID identify the model in use so the correct context window is applied.
//
// MaybeCompact 跑一次降级/压缩派发；auto-trigger 路径吞错。provider/modelID 用于读取真实窗口。
func (m *Manager) MaybeCompact(ctx context.Context, convID, provider, modelID string) error {
	if convID == "" {
		return nil
	}
	conv, err := m.convRepo.Get(ctx, convID)
	if err != nil {
		m.log.Warn("MaybeCompact: load conv failed", zap.String("conv", convID), zap.Error(err))
		return nil
	}
	blocks, err := m.chatRepo.ListBlocksByConversation(ctx, convID)
	if err != nil {
		m.log.Warn("MaybeCompact: list blocks failed", zap.String("conv", convID), zap.Error(err))
		return nil
	}
	usable, used := m.estimate(ctx, conv, blocks, provider, modelID)
	if usable <= 0 {
		return nil
	}
	ratio := float64(used) / float64(usable)
	m.log.Debug("MaybeCompact ratio",
		zap.String("conv", convID),
		zap.String("provider", provider),
		zap.String("model", modelID),
		zap.Int("used", used),
		zap.Int("usable", usable),
		zap.Float64("ratio", ratio))

	if ratio < m.thr.Soft {
		return nil
	}

	demoted := m.demoteOldBlocks(ctx, blocks)
	if demoted > 0 {
		m.log.Info("contextmgr demoted blocks",
			zap.String("conv", convID),
			zap.Int("demoted_count", demoted),
			zap.Float64("ratio_before", ratio))
	}

	// Re-estimate after demotion since cold blocks barely count.
	if demoted > 0 {
		blocks, _ = m.chatRepo.ListBlocksByConversation(ctx, convID)
		_, used = m.estimate(ctx, conv, blocks, provider, modelID)
		ratio = float64(used) / float64(usable)
	}

	if ratio < m.thr.Hard {
		return nil
	}

	if m.resolveLLM == nil {
		m.log.Debug("MaybeCompact: hard threshold reached but no LLM resolver wired; skipping fullCompact",
			zap.String("conv", convID))
		return nil
	}
	if err := m.fullCompact(ctx, conv, blocks, used, usable); err != nil {
		m.log.Warn("fullCompact failed (will retry next turn)",
			zap.String("conv", convID), zap.Error(err))
		return nil
	}
	return nil
}

// ForceCompact runs fullCompact regardless of thresholds; surfaces ErrCompactFailed on failure.
//
// ForceCompact 无视阈值强跑 fullCompact；失败返 ErrCompactFailed。
func (m *Manager) ForceCompact(ctx context.Context, convID, provider, modelID string) error {
	if m.resolveLLM == nil {
		return ErrCompactFailed
	}
	conv, err := m.convRepo.Get(ctx, convID)
	if err != nil {
		return err
	}
	blocks, err := m.chatRepo.ListBlocksByConversation(ctx, convID)
	if err != nil {
		return err
	}
	usable, used := m.estimate(ctx, conv, blocks, provider, modelID)
	if err := m.fullCompact(ctx, conv, blocks, used, usable); err != nil {
		return ErrCompactFailed
	}
	return nil
}

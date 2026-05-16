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
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
)

// ErrCompactFailed is the manual-trigger failure sentinel; auto path swallows + retries.
//
// ErrCompactFailed 手动触发失败的 sentinel；自动路径吞错后重试。
var ErrCompactFailed = errors.New("contextmgr: compact failed")

// LLMResolver builds a cheap LLM bundle for summary generation (injection point for fake LLMs in tests).
//
// LLMResolver 为摘要生成构造便宜 LLM bundle（测试可注入 fake）。
type LLMResolver func(ctx context.Context) (client llminfra.Client, modelID, key, baseURL string, err error)

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

// Manager orchestrates demotion + compaction; MaybeCompact is the only entry point.
//
// Manager 编排降级 + 压缩；MaybeCompact 是唯一入口。
type Manager struct {
	chatRepo   chatdomain.Repository
	convRepo   convdomain.Repository
	emitter    eventlogpkg.Emitter
	notif      notificationspkg.Publisher
	resolveLLM LLMResolver
	log        *zap.Logger
	thr        Thresholds

	calMu sync.Mutex
	cal   map[string]float64
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

// MaybeCompact runs the demote-or-compact dispatch once; auto-trigger path swallows errors.
//
// MaybeCompact 跑一次降级/压缩派发；auto-trigger 路径吞错。
func (m *Manager) MaybeCompact(ctx context.Context, convID string) error {
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
	usable, used := m.estimate(conv, blocks)
	if usable <= 0 {
		return nil
	}
	ratio := float64(used) / float64(usable)
	m.log.Debug("MaybeCompact ratio",
		zap.String("conv", convID),
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
		_, used = m.estimate(conv, blocks)
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
func (m *Manager) ForceCompact(ctx context.Context, convID string) error {
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
	usable, used := m.estimate(conv, blocks)
	if err := m.fullCompact(ctx, conv, blocks, used, usable); err != nil {
		return ErrCompactFailed
	}
	return nil
}

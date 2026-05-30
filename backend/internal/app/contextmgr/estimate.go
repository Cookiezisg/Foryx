package contextmgr

import (
	"context"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	modelcapspkg "github.com/sunweilin/forgify/backend/internal/pkg/modelcaps"
	"github.com/sunweilin/forgify/backend/internal/pkg/tokencount"
)

// conservativeDefault is the fallback cap when no resolver is wired; matches modelcaps.fallback.
//
// conservativeDefault 是未注入 resolver 时的兜底 cap，值与 modelcaps.fallback 对齐。
var conservativeDefault = modelcapspkg.Cap{ContextWindow: 32_768, MaxOutput: 8_192}

// estimate computes (usable, used) for the conv using the injected capability resolver.
// provider/modelID come from the chat runner's resolved bundle so the real window is used.
//
// estimate 用注入的 capability resolver 算 (usable, used)；provider/modelID 来自 runner
// 解析的 bundle，确保使用真实窗口而不是硬编码兜底。
func (m *Manager) estimate(ctx context.Context, conv *convdomain.Conversation, blocks []*chatdomain.Block, provider, modelID string) (usable, used int) {
	var cap modelcapspkg.Cap
	if m.capFor != nil {
		cap = m.capFor(ctx, provider, modelID)
	} else {
		// No capability resolver wired → every model is sized as the conservative
		// 32K/8K fallback, which silently compacts a 200K/1M model far too early.
		// Warn once so this wiring regression is visible (must never happen in prod).
		//
		// 未注入 capability resolver → 所有模型按 32K/8K 兜底，会把 200K/1M 模型远
		// 过早压缩。警告一次让此装配回归可见（生产不应发生）。
		m.nilCapOnce.Do(func() {
			m.log.Warn("contextmgr: no CapabilityResolver wired; using conservative 32K/8K fallback — large models will compact far too early")
		})
		cap = conservativeDefault
	}
	usable = cap.UsableInput()

	used = tokencount.Estimate(conv.Summary)
	for _, b := range blocks {
		used += m.projectedTokens(b)
	}
	used = int(float64(used) * m.calibrationFor(conv.ID))
	return
}

// projectedTokens returns the block's projected token cost under its ContextRole.
//
// projectedTokens 返 block 按 ContextRole 投影后的 token 消耗。
func (m *Manager) projectedTokens(b *chatdomain.Block) int {
	if b == nil {
		return 0
	}
	switch b.ContextRole {
	case ContextRoleArchived:
		return 0
	case ContextRoleCold:
		return 20
	case ContextRoleWarm:
		preview := b.Content
		if len(preview) > WarmPreviewBytes {
			preview = preview[:WarmPreviewBytes]
		}
		return tokencount.Estimate(preview) + 12
	default:
		// compaction blocks' content also lives in conv.Summary — skip to avoid double-count.
		if b.Type == eventlogdomain.BlockTypeCompaction {
			return 0
		}
		return tokencount.Estimate(b.Content)
	}
}

// Calibrate updates per-conv calibration ratio from a real LLM usage observation.
//
// Calibrate 用真实 LLM usage 更新 per-conv 校准比例。
func (m *Manager) Calibrate(convID string, actualInputTokens, ourEstimate int) {
	if convID == "" || actualInputTokens <= 0 || ourEstimate <= 0 {
		return
	}
	fresh := tokencount.Calibrate(actualInputTokens, ourEstimate)
	m.calMu.Lock()
	defer m.calMu.Unlock()
	m.cal[convID] = tokencount.MergeCalibration(m.cal[convID], fresh)
}

func (m *Manager) calibrationFor(convID string) float64 {
	m.calMu.Lock()
	defer m.calMu.Unlock()
	if r, ok := m.cal[convID]; ok && r > 0 {
		return r
	}
	return 1.0
}


package contextmgr

import (
	"strings"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	"github.com/sunweilin/forgify/backend/internal/pkg/modelmeta"
	"github.com/sunweilin/forgify/backend/internal/pkg/tokencount"
)

// estimate computes (usable, used) for the conv; used = summary + projected blocks × calibration.
//
// estimate 算 (usable, used)；used = summary + 按 role 投影的 block × 校准。
func (m *Manager) estimate(conv *convdomain.Conversation, blocks []*chatdomain.Block) (usable, used int) {
	// conversation row lacks provider/model; conservative fallback corrected by Calibrate next turn.
	meta := modelmeta.Lookup("", "")
	usable = meta.UsableInput()

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

// MessageRoleFromBlock maps an event-log block to its LLM history role.
//
// MessageRoleFromBlock 把事件日志 block 映射到 LLM history 的 role。
func MessageRoleFromBlock(b *chatdomain.Block) string {
	if b == nil {
		return ""
	}
	if b.Type == eventlogdomain.BlockTypeToolResult {
		return "tool"
	}
	return "assistant"
}

// projectedTokensSnapshot exposes per-role token counts for diagnostics.
//
// projectedTokensSnapshot 暴露按 role 的 token 切分供诊断用。
func (m *Manager) projectedTokensSnapshot(blocks []*chatdomain.Block) map[string]int {
	out := map[string]int{}
	for _, b := range blocks {
		role := strings.ToLower(b.ContextRole)
		if role == "" {
			role = ContextRoleHot
		}
		out[role] += m.projectedTokens(b)
	}
	return out
}

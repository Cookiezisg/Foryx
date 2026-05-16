package contextmgr

import (
	"context"

	"go.uber.org/zap"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
)

const (
	ContextRoleHot      = "hot"
	ContextRoleWarm     = "warm"
	ContextRoleCold     = "cold"
	ContextRoleArchived = "archived"
)

const WarmPreviewBytes = 200

// demoteOldBlocks updates context_role on past-window tool_result blocks; returns rows changed.
//
// demoteOldBlocks 给超出保留窗口的 tool_result 更新 context_role；返实际改动数。
func (m *Manager) demoteOldBlocks(ctx context.Context, blocks []*chatdomain.Block) int {
	var (
		toolResultIdx int
		changed       int
	)
	for i := len(blocks) - 1; i >= 0; i-- {
		b := blocks[i]
		if b == nil {
			continue
		}
		if b.ContextRole == ContextRoleArchived {
			continue
		}
		if pinned, _ := b.Attrs["pinned"].(bool); pinned {
			continue
		}
		if m.isWithinRecentTurns(blocks, i) {
			continue
		}
		if b.Type != eventlogdomain.BlockTypeToolResult {
			continue
		}
		toolResultIdx++
		var newRole string
		switch {
		case toolResultIdx <= m.thr.RecentTRKeep:
			continue
		case toolResultIdx <= m.thr.WarmCutoff:
			newRole = ContextRoleWarm
		default:
			newRole = ContextRoleCold
		}
		if b.ContextRole == newRole {
			continue
		}
		if err := m.chatRepo.UpdateBlockRole(ctx, b.ID, newRole); err != nil {
			m.log.Warn("demote: UpdateBlockRole failed",
				zap.String("block_id", b.ID),
				zap.String("from", b.ContextRole),
				zap.String("to", newRole),
				zap.Error(err))
			continue
		}
		b.ContextRole = newRole
		changed++
	}
	return changed
}

// isWithinRecentTurns returns true when blocks[i] belongs to one of the latest RecentTurns messages.
//
// isWithinRecentTurns 报告 blocks[i] 是否属于最近 RecentTurns 个 message。
func (m *Manager) isWithinRecentTurns(blocks []*chatdomain.Block, i int) bool {
	if m.thr.RecentTurns <= 0 {
		return false
	}
	seen := map[string]struct{}{}
	for j := len(blocks) - 1; j >= 0; j-- {
		seen[blocks[j].MessageID] = struct{}{}
		if len(seen) >= m.thr.RecentTurns {
			break
		}
	}
	if _, ok := seen[blocks[i].MessageID]; ok {
		return true
	}
	return false
}

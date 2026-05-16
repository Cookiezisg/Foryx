package contextmgr

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
)

// fullCompact runs one LLM summarisation pass; failure emits the block in error state.
//
// fullCompact 跑一次 LLM 摘要；失败仍以 error 状态 emit 该 block。
func (m *Manager) fullCompact(
	ctx context.Context,
	conv *convdomain.Conversation,
	blocks []*chatdomain.Block,
	usedBefore, usable int,
) error {
	candidates := m.collectArchiveCandidates(blocks, conv.SummaryCoversUpToSeq)
	if len(candidates) == 0 {
		m.log.Debug("fullCompact: no archive candidates",
			zap.String("conv", conv.ID),
			zap.Int64("cover_seq", conv.SummaryCoversUpToSeq))
		return nil
	}

	// Mint a virtual system message to host the compaction block (cleaner than retro-emitting).
	msgID := idgenpkg.New("msg")
	attrs := map[string]any{"kind": "compaction"}
	m.emitter.EmitMessageStart(ctx, msgID, "system", "", attrs)

	if err := m.chatRepo.SaveMessage(ctx, &chatdomain.Message{
		ID:             msgID,
		ConversationID: conv.ID,
		UserID:         conv.UserID,
		Role:           "system",
		Status:         chatdomain.StatusStreaming,
		Attrs:          attrs,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}); err != nil {
		m.log.Warn("fullCompact: SaveMessage failed", zap.Error(err))
	}

	firstSeq := candidates[0].Seq
	lastSeq := candidates[len(candidates)-1].Seq

	blockAttrs := map[string]any{
		"coversFromSeq":   firstSeq,
		"coversToSeq":     lastSeq,
		"blocksArchived":  len(candidates),
		"generatedBy":     "contextmgr",
	}
	// Top-level block: parentID must equal messageID (EmitBlockStart rejects empty parentID).
	blockID := idgenpkg.New("blk")
	m.emitter.EmitBlockStart(ctx, blockID, msgID, msgID, eventlogdomain.BlockTypeCompaction, blockAttrs)

	client, modelID, key, baseURL, err := m.resolveLLM(ctx)
	if err != nil {
		m.finishCompactionWithError(ctx, msgID, blockID,
			fmt.Errorf("contextmgr.fullCompact: resolveLLM: %w", err))
		return err
	}

	prompt := buildCompactPrompt(conv.Summary, candidates, conv.SummaryCoversUpToSeq)
	llmCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	newSummary, err := llminfra.Generate(llmCtx, client, llminfra.Request{
		ModelID: modelID, Key: key, BaseURL: baseURL,
		System: compactSystemPrompt,
		Messages: []llminfra.LLMMessage{
			{Role: llminfra.RoleUser, Content: prompt},
		},
	})
	if err != nil {
		m.finishCompactionWithError(ctx, msgID, blockID,
			fmt.Errorf("contextmgr.fullCompact: LLM: %w", err))
		return err
	}
	if newSummary == "" {
		m.finishCompactionWithError(ctx, msgID, blockID,
			fmt.Errorf("contextmgr.fullCompact: empty summary from LLM"))
		return fmt.Errorf("contextmgr.fullCompact: empty summary")
	}

	m.emitter.DeltaBlock(ctx, blockID, newSummary)
	m.emitter.StopBlock(ctx, blockID, eventlogdomain.StatusCompleted, nil)

	m.emitter.StopMessage(ctx, msgID, eventlogdomain.StatusCompleted,
		chatdomain.StopReasonEndTurn, "", "", 0, 0)
	if err := m.chatRepo.SaveMessage(ctx, &chatdomain.Message{
		ID:             msgID,
		ConversationID: conv.ID,
		UserID:         conv.UserID,
		Role:           "system",
		Status:         chatdomain.StatusCompleted,
		StopReason:     chatdomain.StopReasonEndTurn,
		Attrs:          attrs,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}); err != nil {
		m.log.Warn("fullCompact: SaveMessage final failed", zap.Error(err))
	}

	conv.Summary = newSummary
	conv.SummaryCoversUpToSeq = lastSeq
	if err := m.convRepo.Save(ctx, conv); err != nil {
		m.log.Error("fullCompact: convRepo.Save failed", zap.Error(err))
		return fmt.Errorf("contextmgr.fullCompact: convRepo.Save: %w", err)
	}
	archivedCount := 0
	for _, b := range candidates {
		if err := m.chatRepo.UpdateBlockRole(ctx, b.ID, ContextRoleArchived); err != nil {
			m.log.Warn("fullCompact: UpdateBlockRole archive failed",
				zap.String("block_id", b.ID), zap.Error(err))
			continue
		}
		archivedCount++
	}

	m.notif.Publish(ctx, "compaction", conv.ID, map[string]any{
		"action":         "completed",
		"coversToSeq":    lastSeq,
		"blocksArchived": archivedCount,
		"summaryBytes":   len(newSummary),
	}, conv.ID)

	m.log.Info("compaction complete",
		zap.String("conv", conv.ID),
		zap.Int("blocks_archived", archivedCount),
		zap.Int64("covers_to_seq", lastSeq),
		zap.Int("summary_bytes", len(newSummary)),
		zap.Int("used_before", usedBefore),
		zap.Int("usable", usable))
	return nil
}

// collectArchiveCandidates returns archivable blocks in seq-ascending order.
//
// collectArchiveCandidates 返可 archive 的 block，按 seq 升序。
func (m *Manager) collectArchiveCandidates(blocks []*chatdomain.Block, coverSeq int64) []*chatdomain.Block {
	out := make([]*chatdomain.Block, 0, len(blocks))
	for i, b := range blocks {
		if b == nil {
			continue
		}
		if b.Seq <= coverSeq {
			continue
		}
		if b.ContextRole == ContextRoleArchived {
			continue
		}
		if pinned, _ := b.Attrs["pinned"].(bool); pinned {
			continue
		}
		if b.Type == eventlogdomain.BlockTypeCompaction {
			continue
		}
		if m.isWithinRecentTurns(blocks, i) {
			continue
		}
		out = append(out, b)
	}
	return out
}

func (m *Manager) finishCompactionWithError(ctx context.Context, msgID, blockID string, err error) {
	m.log.Warn("compaction failed",
		zap.String("msg_id", msgID),
		zap.String("block_id", blockID),
		zap.Error(err))
	m.emitter.StopBlock(ctx, blockID, eventlogdomain.StatusError, err)
	m.emitter.StopMessage(ctx, msgID, eventlogdomain.StatusError,
		chatdomain.StopReasonError, "COMPACTION_FAILED", err.Error(), 0, 0)
}

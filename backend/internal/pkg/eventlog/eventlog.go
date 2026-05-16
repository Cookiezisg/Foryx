// Package eventlog provides ergonomic ctx-scoped helpers around the eventlog Bridge.
//
// Package eventlog 在 eventlog Bridge 之上提供 ctx-scoped 的 Emitter helper。
package eventlog

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Emitter is the high-level emit API; methods read conversationID + parent linkage from ctx.
//
// Emitter 是高层 emit API，方法从 ctx 读 conversationID + 父链。
type Emitter interface {
	StopMessage(ctx context.Context, msgID, status, stopReason, errCode, errMsg string, inputTokens, outputTokens int)
	StartBlock(ctx context.Context, blockType string, attrs map[string]any) string
	EmitMessageStart(ctx context.Context, id, role, parentBlockID string, attrs map[string]any)
	EmitBlockStart(ctx context.Context, id, parentID, messageID, blockType string, attrs map[string]any)
	DeltaBlock(ctx context.Context, blockID, delta string)
	StopBlock(ctx context.Context, blockID, status string, err error)
}

// New constructs an Emitter; repo is optional dual-write target for block rows (nil-safe).
//
// New 构造 Emitter；repo 是 block 行可选双写目标（nil 安全）。
func New(bridge eventlogdomain.Bridge, repo chatdomain.Repository, log *zap.Logger) Emitter {
	if log == nil {
		log = zap.NewNop()
	}
	return &emitter{
		bridge: bridge,
		repo:   repo,
		log:    log.Named("eventlog.emitter"),
	}
}

type emitter struct {
	bridge eventlogdomain.Bridge
	repo   chatdomain.Repository
	log    *zap.Logger
}

func (em *emitter) requireConv(ctx context.Context, op string) (string, bool) {
	convID, ok := reqctxpkg.GetConversationID(ctx)
	if !ok {
		em.log.Warn("emit skipped: no conversationID in ctx",
			zap.String("op", op))
		return "", false
	}
	return convID, true
}

func (em *emitter) publish(ctx context.Context, convID string, e eventlogdomain.Event) (int64, bool) {
	env, err := em.bridge.Publish(ctx, e)
	if err != nil {
		fields := []zap.Field{
			zap.String("type", e.EventType()),
			zap.String("conversationId", convID),
			zap.Error(err),
		}
		switch {
		case errors.Is(err, eventlogdomain.ErrInvalidEvent):
			em.log.Error("emit failed: invalid event (producer bug)", fields...)
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			em.log.Debug("emit skipped: ctx cancelled", fields...)
		default:
			em.log.Warn("emit failed", fields...)
		}
		return 0, false
	}
	return env.Seq, true
}

func (em *emitter) saveBlockRow(ctx context.Context, convID, id, parentID, messageID, blockType string, attrs map[string]any, seq int64) {
	if em.repo == nil {
		return
	}
	parentBlock := ""
	if parentID != messageID {
		parentBlock = parentID
	}
	now := time.Now().UTC()
	if err := em.repo.SaveBlock(ctx, &chatdomain.Block{
		ID:             id,
		ConversationID: convID,
		MessageID:      messageID,
		ParentBlockID:  parentBlock,
		Seq:            seq,
		Type:           blockType,
		Attrs:          attrs,
		Status:         eventlogdomain.StatusStreaming,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		em.log.Warn("blockV2 dual-write failed (block_start)",
			zap.String("blockId", id), zap.Error(err))
	}
}

func (em *emitter) StopMessage(ctx context.Context, msgID, status, stopReason, errCode, errMsg string, inputTokens, outputTokens int) {
	convID, ok := em.requireConv(ctx, "StopMessage")
	if !ok {
		return
	}
	em.publish(ctx, convID, eventlogdomain.MessageStop{
		ConversationID: convID,
		ID:             msgID,
		Status:         status,
		StopReason:     stopReason,
		ErrorCode:      errCode,
		ErrorMessage:   errMsg,
		InputTokens:    inputTokens,
		OutputTokens:   outputTokens,
	})
}

func (em *emitter) StartBlock(ctx context.Context, blockType string, attrs map[string]any) string {
	convID, ok := em.requireConv(ctx, "StartBlock")
	if !ok {
		return ""
	}
	parentID, _ := reqctxpkg.GetParentBlockID(ctx)
	if parentID == "" {
		parentID, _ = reqctxpkg.GetMessageID(ctx)
	}
	msgID, _ := reqctxpkg.GetMessageID(ctx)
	if parentID == "" || msgID == "" {
		em.log.Warn("emit skipped: missing parent or message in ctx",
			zap.String("op", "StartBlock"),
			zap.String("blockType", blockType))
		return ""
	}
	blockID := idgenpkg.New("blk")
	seq, ok := em.publish(ctx, convID, eventlogdomain.BlockStart{
		ConversationID: convID,
		ID:             blockID,
		ParentID:       parentID,
		MessageID:      msgID,
		BlockType:      blockType,
		Attrs:          attrs,
	})
	if ok {
		em.saveBlockRow(ctx, convID, blockID, parentID, msgID, blockType, attrs, seq)
	}
	return blockID
}

func (em *emitter) EmitMessageStart(ctx context.Context, id, role, parentBlockID string, attrs map[string]any) {
	convID, ok := em.requireConv(ctx, "EmitMessageStart")
	if !ok {
		return
	}
	if id == "" || role == "" {
		em.log.Warn("emit skipped: empty id or role",
			zap.String("op", "EmitMessageStart"))
		return
	}
	em.publish(ctx, convID, eventlogdomain.MessageStart{
		ConversationID: convID,
		ID:             id,
		ParentBlockID:  parentBlockID,
		Role:           role,
		Attrs:          attrs,
	})
}

func (em *emitter) EmitBlockStart(ctx context.Context, id, parentID, messageID, blockType string, attrs map[string]any) {
	convID, ok := em.requireConv(ctx, "EmitBlockStart")
	if !ok {
		return
	}
	if id == "" || parentID == "" || messageID == "" {
		em.log.Warn("emit skipped: empty id / parent / message",
			zap.String("op", "EmitBlockStart"),
			zap.String("blockType", blockType))
		return
	}
	seq, ok := em.publish(ctx, convID, eventlogdomain.BlockStart{
		ConversationID: convID,
		ID:             id,
		ParentID:       parentID,
		MessageID:      messageID,
		BlockType:      blockType,
		Attrs:          attrs,
	})
	if ok {
		em.saveBlockRow(ctx, convID, id, parentID, messageID, blockType, attrs, seq)
	}
}

func (em *emitter) DeltaBlock(ctx context.Context, blockID, delta string) {
	convID, ok := em.requireConv(ctx, "DeltaBlock")
	if !ok {
		return
	}
	if blockID == "" {
		return
	}
	em.publish(ctx, convID, eventlogdomain.BlockDelta{
		ConversationID: convID,
		ID:             blockID,
		Delta:          delta,
	})
	if em.repo != nil {
		if err := em.repo.AppendDelta(ctx, blockID, delta); err != nil {
			em.log.Warn("blockV2 dual-write failed (delta)",
				zap.String("blockId", blockID), zap.Error(err))
		}
	}
}

func (em *emitter) StopBlock(ctx context.Context, blockID, status string, err error) {
	convID, ok := em.requireConv(ctx, "StopBlock")
	if !ok {
		return
	}
	if blockID == "" {
		return
	}
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	em.publish(ctx, convID, eventlogdomain.BlockStop{
		ConversationID: convID,
		ID:             blockID,
		Status:         status,
		Error:          errStr,
	})
	// Use Background so caller-cancel can't leave block stuck at streaming (§S9 / §S21).
	// 用 Background 防止 caller-cancel 让 block 卡 streaming（§S9 / §S21）。
	if em.repo != nil {
		if e := em.repo.FinalizeStop(context.Background(), blockID, status, errStr); e != nil {
			em.log.Warn("blockV2 dual-write failed (stop)",
				zap.String("blockId", blockID), zap.Error(e))
		}
	}
}

type emitterKey struct{}

// With returns a copy of ctx carrying em.
//
// With 返回携带 em 的 ctx 拷贝。
func With(ctx context.Context, em Emitter) context.Context {
	return context.WithValue(ctx, emitterKey{}, em)
}

// From returns the Emitter stored in ctx, or a no-op Emitter if absent.
//
// From 返回 ctx 中的 Emitter，缺失则返 no-op。
func From(ctx context.Context) Emitter {
	em, ok := ctx.Value(emitterKey{}).(Emitter)
	if !ok || em == nil {
		return noopEmitter{}
	}
	return em
}

type noopEmitter struct{}

func (noopEmitter) StopMessage(context.Context, string, string, string, string, string, int, int) {
}
func (noopEmitter) StartBlock(context.Context, string, map[string]any) string                 { return "" }
func (noopEmitter) EmitMessageStart(context.Context, string, string, string, map[string]any)  {}
func (noopEmitter) EmitBlockStart(context.Context, string, string, string, string, map[string]any) {
}
func (noopEmitter) DeltaBlock(context.Context, string, string)       {}
func (noopEmitter) StopBlock(context.Context, string, string, error) {}

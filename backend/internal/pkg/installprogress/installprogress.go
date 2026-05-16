// Package installprogress wraps a sandbox install with an eventlog progress block in chat flow.
//
// Package installprogress 在 chat flow 下用 eventlog progress block 包 sandbox install。
package installprogress

import (
	"context"
	"fmt"
	"strings"

	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Run wraps fn with a progress block in chat flow; outside chat flow the callback is a no-op.
//
// Run 在 chat flow 下用 progress block 包 fn；非 chat flow 下回调是 no-op。
func Run[T any](
	ctx context.Context,
	attrs map[string]any,
	fn func(progress sandboxdomain.ProgressFunc) (T, error),
) (T, error) {
	progressCb := newProgressCallback(ctx, attrs)
	progressCb.emitStartLine(attrs)
	out, err := fn(progressCb.cb)
	progressCb.close(ctx, err)
	return out, err
}

type progressCallback struct {
	em      eventlogpkg.Emitter
	blockID string
}

func newProgressCallback(ctx context.Context, attrs map[string]any) *progressCallback {
	if !inChatFlow(ctx) {
		return &progressCallback{}
	}
	em := eventlogpkg.From(ctx)
	if em == nil {
		return &progressCallback{}
	}
	blockID := em.StartBlock(ctx, eventlogdomain.BlockTypeProgress, attrs)
	return &progressCallback{em: em, blockID: blockID}
}

func (p *progressCallback) cb(stage, message string, percent int) {
	if p.blockID == "" {
		return
	}
	p.em.DeltaBlock(context.Background(), p.blockID, formatProgressLine(stage, message, percent))
}

// emitStartLine pushes a synthetic "[starting]" delta so a silent installer still shows progress.
//
// emitStartLine 推一条合成 "[starting]" delta，避免静默 installer 让块空白。
func (p *progressCallback) emitStartLine(attrs map[string]any) {
	if p.blockID == "" {
		return
	}
	rt, _ := attrs["runtime"].(string)
	srv, _ := attrs["server"].(string)
	var line string
	switch {
	case rt != "" && srv != "":
		line = fmt.Sprintf("[starting] %s for %s\n", rt, srv)
	case rt != "":
		line = fmt.Sprintf("[starting] %s runtime\n", rt)
	default:
		line = "[starting] sandbox install\n"
	}
	p.em.DeltaBlock(context.Background(), p.blockID, line)
}

func (p *progressCallback) close(_ context.Context, err error) {
	if p.blockID == "" {
		return
	}
	status := eventlogdomain.StatusCompleted
	if err != nil {
		p.em.DeltaBlock(context.Background(), p.blockID, fmt.Sprintf("[error] %v\n", err))
		status = eventlogdomain.StatusError
	} else {
		p.em.DeltaBlock(context.Background(), p.blockID, "[done] install complete\n")
	}
	// Use Background so caller-cancel can't leave block stuck at streaming (§S9).
	// 用 Background 防止 caller-cancel 让 block 卡 streaming（§S9）。
	p.em.StopBlock(context.Background(), p.blockID, status, err)
}

// inChatFlow reports whether ctx carries both a conversationId AND a parent block.
//
// inChatFlow 报告 ctx 是否同时带 conversationId 和 parent block。
func inChatFlow(ctx context.Context) bool {
	if convID, ok := reqctxpkg.GetConversationID(ctx); !ok || convID == "" {
		return false
	}
	if parent, ok := reqctxpkg.GetParentBlockID(ctx); !ok || parent == "" {
		return false
	}
	return true
}

func formatProgressLine(stage, message string, percent int) string {
	var sb strings.Builder
	if stage != "" {
		sb.WriteString("[")
		sb.WriteString(stage)
		sb.WriteString("] ")
	}
	sb.WriteString(message)
	if percent >= 0 {
		fmt.Fprintf(&sb, " (%d%%)", percent)
	}
	sb.WriteString("\n")
	return sb.String()
}

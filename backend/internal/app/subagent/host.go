package subagent

import (
	"context"
	"time"

	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// subagentHost satisfies loop.Host; one instance per Spawn.
//
// subagentHost 满足 loop.Host，每次 Spawn 一份实例。
type subagentHost struct {
	svc           *Service
	subMsgID      string
	parentConvID  string
	parentBlockID string
	uid           string
	typeName      string
	maxTurns      int
	tools         []toolapp.Tool
	userPrompt    string
	systemPrompt  string
}

// LoadHistory returns the seed history: just the user prompt.
//
// LoadHistory 返回种子历史：仅 user prompt。
func (h *subagentHost) LoadHistory(_ context.Context) ([]llminfra.LLMMessage, error) {
	return []llminfra.LLMMessage{
		{Role: llminfra.RoleUser, Content: h.userPrompt},
	}, nil
}

// Tools returns the per-spawn filtered tool list.
//
// Tools 返回 per-spawn 过滤后的 tool 列表。
func (h *subagentHost) Tools() []toolapp.Tool {
	return h.tools
}

// WriteFinalize persists the sub-Message row and emits message_stop using detached ctx.
//
// WriteFinalize 用 detached ctx 持久化 sub-Message 并发 message_stop。
func (h *subagentHost) WriteFinalize(ctx context.Context, blocks []chatdomain.Block, status, stopReason, errCode, errMsg string, in, out int) {
	saveCtx := context.Background()
	if uid, err := reqctxpkg.RequireUserID(ctx); err == nil {
		saveCtx = reqctxpkg.SetUserID(saveCtx, uid)
	} else if h.uid != "" {
		saveCtx = reqctxpkg.SetUserID(saveCtx, h.uid)
	}

	attrs := map[string]any{
		"kind":     "subagent_run",
		"type":     h.typeName,
		"runId":    h.subMsgID,
		"maxTurns": h.maxTurns,
	}

	msg := &chatdomain.Message{
		ID:             h.subMsgID,
		ConversationID: h.parentConvID,
		UserID:         h.uid,
		ParentBlockID:  h.parentBlockID,
		Role:           chatdomain.RoleAssistant,
		Status:         status,
		StopReason:     stopReason,
		ErrorCode:      errCode,
		ErrorMessage:   errMsg,
		InputTokens:    in,
		OutputTokens:   out,
		Attrs:          attrs,
		UpdatedAt:      time.Now().UTC(),
	}
	if err := h.svc.chatRepo.SaveMessage(saveCtx, msg); err != nil {
		h.svc.log.Error("CRITICAL: subagent terminal Message write failed",
			zap.String("sub_msg_id", h.subMsgID), zap.Error(err))
	}

	em := eventlogpkg.From(ctx)
	em.StopMessage(saveCtx, h.subMsgID, h.mapEventLogStatus(status),
		stopReason, errCode, errMsg, in, out)

	_ = blocks
}

func (h *subagentHost) mapEventLogStatus(s string) string {
	switch s {
	case chatdomain.StatusStreaming:
		return eventlogdomain.StatusStreaming
	case chatdomain.StatusError:
		return eventlogdomain.StatusError
	case chatdomain.StatusCancelled:
		return eventlogdomain.StatusCancelled
	case chatdomain.StatusCompleted, chatdomain.StatusPending:
		return eventlogdomain.StatusCompleted
	default:
		h.svc.log.Warn("subagent.host.mapEventLogStatus: unknown chatdomain status; mapped to Completed",
			zap.String("status", s), zap.String("sub_msg_id", h.subMsgID))
		return eventlogdomain.StatusCompleted
	}
}

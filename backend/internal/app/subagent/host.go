// host.go — subagentHost satisfies loop.Host for a sub-run. The
// sub-run's transcript (text / reasoning / tool_call / tool_result
// blocks) is written real-time via the eventlog Emitter (no
// subagent_messages table — sub blocks live in the unified
// `message_blocks` table, parented by sub-message-block placeholder
// emitted in Service.Spawn).
//
// Lifecycle:
//
//	loop.Run → host.LoadHistory  → seed: [user prompt]
//	         → loop iterations    (stream emits + tool emits real-time)
//	         → host.WriteFinalize → write sub-Message row to messages
//	                                table + emit message_stop
//
// host.go ——subagentHost 满足 sub-run 的 loop.Host。sub-run 的转录
// （text / reasoning / tool_call / tool_result block）由 eventlog
// Emitter 实时写（无 subagent_messages 表——sub block 都在统一
// `message_blocks` 表，挂在 Service.Spawn 推的 sub message-block 占
// 位下）。
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

// subagentHost satisfies loop.Host. One instance per Spawn — owns the
// sub-message ID + run metadata for the terminal Message write.
//
// subagentHost 满足 loop.Host。每次 Spawn 一份——持 sub-message ID +
// run 元数据给终态 Message 写入用。
type subagentHost struct {
	svc          *Service
	subMsgID     string // sub-Message ID (event-log协议 + messages 表 PK)
	parentConvID string // sub-Message.ConversationID（与父对话相同）
	parentBlockID string // 占位 message-block ID（Spawn 已推 BlockStart）
	uid          string
	typeName     string
	maxTurns     int
	tools        []toolapp.Tool
	userPrompt   string
	systemPrompt string
}

// LoadHistory returns the seed history: just the user prompt. The
// system prompt is supplied via baseReq.System (not Messages).
//
// LoadHistory 返种子历史：仅 user prompt。System prompt 走 baseReq.System
// 不进 Messages。
func (h *subagentHost) LoadHistory(_ context.Context) ([]llminfra.LLMMessage, error) {
	return []llminfra.LLMMessage{
		{Role: llminfra.RoleUser, Content: h.userPrompt},
	}, nil
}

// Tools returns the per-spawn filtered tool list.
//
// Tools 返 per-spawn 过滤后 tool 列表。
func (h *subagentHost) Tools() []toolapp.Tool {
	return h.tools
}

// WriteFinalize persists the sub-Message row to the messages table with
// parent_block_id + attrs (kind=subagent_run + metadata) + emits
// message_stop on the eventlog Bridge. Uses detached ctx for the
// persist write so a cancelled parent doesn't lose the terminal record.
//
// WriteFinalize 把 sub-Message 行写到 messages 表（含 parent_block_id
// + attrs，kind=subagent_run + 元数据）+ 给 eventlog Bridge 发
// message_stop。持久化用 detached ctx 防 parent cancel 丢终态。
func (h *subagentHost) WriteFinalize(ctx context.Context, blocks []chatdomain.Block, status, stopReason, errCode, errMsg string, in, out int) {
	saveCtx := context.Background()
	if uid, err := reqctxpkg.RequireUserID(ctx); err == nil {
		saveCtx = reqctxpkg.SetUserID(saveCtx, uid)
	} else if h.uid != "" {
		saveCtx = reqctxpkg.SetUserID(saveCtx, h.uid)
	}

	// 2026-05: Attrs is map[string]any (GORM serializer:json) — no marshal.
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

	// emit message_stop on eventlog (block_stop for the placeholder
	// message-block fires from Service.Spawn after loop.Run returns).
	// Use saveCtx (detached) instead of ctx so a parent-cancel race
	// between SaveMessage above and StopMessage emit doesn't leave the
	// frontend with a sub-message stuck in `streaming` until reload.
	// Same §S9 reasoning as chat/host.go::WriteFinalize::StopMessage.
	//
	// 给 eventlog 发 message_stop（占位 message-block 的 block_stop 由
	// Service.Spawn 在 loop.Run 返后发）。用 saveCtx（detached）而非
	// ctx——父 cancel 在 SaveMessage 与 StopMessage 之间触发会让前端
	// 的 sub-message 卡在 streaming 直到刷新。同 chat/host.go 的 §S9
	// 逻辑。
	em := eventlogpkg.From(ctx)
	em.StopMessage(saveCtx, h.subMsgID, h.mapEventLogStatus(status),
		stopReason, errCode, errMsg, in, out)

	// blocks param is unused — sub blocks were emitted in real-time and
	// written to message_blocks via the Emitter dual-write.
	//
	// blocks 参数未用——sub blocks 已实时 emit 并经 Emitter dual-write
	// 写到 message_blocks。
	_ = blocks
}

// mapEventLogStatus translates chatdomain.Status* → eventlogdomain.Status*.
// Default branch logs Warn so chatdomain status drift surfaces in operator
// audit trail. Mirror of chat/host.go::mapEventLogStatus (commit e5b65fa).
//
// mapEventLogStatus 翻译 chatdomain.Status* → eventlogdomain.Status*。
// default 分支 Warn log 让 chatdomain 增加新 Status* 但未更新此 switch
// 的漂移可见。同 chat/host.go::mapEventLogStatus（commit e5b65fa）。
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

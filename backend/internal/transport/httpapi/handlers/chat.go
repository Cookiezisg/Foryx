package handlers

import (
	"net/http"

	"go.uber.org/zap"

	chatapp "github.com/sunweilin/forgify/backend/internal/app/chat"
	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// ChatHandler serves the chat engine's 3 endpoints: send a message (202, streams over the
// messages SSE), list a conversation's history (paged), and cancel the running turn (204). The
// assistant turn itself is delivered over the messages stream, not this REST surface.
//
// ChatHandler 提供 chat 引擎 3 端点：发消息（202，经 messages SSE 流式）、列对话历史（分页）、取消
// 运行回合（204）。assistant 回合本身经 messages 流交付、不在此 REST 面。
type ChatHandler struct {
	svc *chatapp.Service
	log *zap.Logger
}

// NewChatHandler constructs the handler.
//
// NewChatHandler 构造 handler。
func NewChatHandler(svc *chatapp.Service, log *zap.Logger) *ChatHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &ChatHandler{svc: svc, log: log.Named("handlers.chat")}
}

// Register wires the endpoints onto mux.
//
// Register 把端点挂到 mux。
func (h *ChatHandler) Register(mux Registrar) {
	mux.HandleFunc("POST /api/v1/conversations/{id}/messages", h.Send)
	mux.HandleFunc("GET /api/v1/conversations/{id}/messages", h.List)
	mux.HandleFunc("POST /api/v1/conversations/{idAction}", h.postAction) // :cancel(N5/MD5——取消在途生成是动作、非删子资源)
	mux.HandleFunc("GET /api/v1/conversations/{id}/system-prompt-preview", h.SystemPromptPreview)
	mux.HandleFunc("GET /api/v1/conversations/{id}/usage", h.Usage)
	mux.HandleFunc("GET /api/v1/conversations/{id}/interactions", h.ListInteractions)
	mux.HandleFunc("POST /api/v1/conversations/{id}/interactions/{toolCallId}", h.ResolveInteraction)
}

// sendMessageRequest is the user turn: text + referenced attachments + @-mentions.
//
// sendMessageRequest 是用户回合：文本 + 引用附件 + @ mention。
type sendMessageRequest struct {
	Content       string                       `json:"content"`
	AttachmentIDs []string                     `json:"attachmentIds"`
	Mentions      []mentiondomain.MentionInput `json:"mentions"`
}

// Send accepts a user turn and starts the generation: 202 Accepted + the assistant message id;
// the turn streams over the messages SSE. EMPTY_CONTENT (400) / STREAM_IN_PROGRESS (409) bubble
// from the service.
//
// Send 接受用户回合并启动生成：202 Accepted + assistant message id；回合经 messages SSE 流式。
// EMPTY_CONTENT (400) / STREAM_IN_PROGRESS (409) 从 service 冒泡。
func (h *ChatHandler) Send(w http.ResponseWriter, r *http.Request) {
	var req sendMessageRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	msgID, err := h.svc.Send(r.Context(), r.PathValue("id"), chatapp.SendInput{
		Content:       req.Content,
		AttachmentIDs: req.AttachmentIDs,
		Mentions:      req.Mentions,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusAccepted, map[string]string{"id": msgID}) // 异步动作返新资源 id 统一 {id}(MD3)
}

// List returns one keyset page of the conversation's history (newest-first), each message with
// its blocks. N4 pagination via ?cursor & ?limit.
//
// List 返回对话历史的一页 keyset（最新在前），每条带 blocks。N4 分页经 ?cursor & ?limit。
func (h *ChatHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := responsehttpapi.ParsePage(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	items, next, err := h.svc.ListMessages(r.Context(), r.PathValue("id"), p.Cursor, p.Limit)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, items, next, next != "")
}

// postAction dispatches the conversation-level action POST /conversations/{id}:cancel.
//
// postAction 派发对话级动作 POST /conversations/{id}:cancel。
func (h *ChatHandler) postAction(w http.ResponseWriter, r *http.Request) {
	id, action, ok := idAndAction(r, "idAction")
	if !ok || action != "cancel" {
		http.NotFound(w, r)
		return
	}
	// Cancel stops the conversation's running turn (204). Graceful no-op when nothing runs.
	// Cancel 停止对话运行中的回合（204）。无运行回合时优雅 no-op。
	if err := h.svc.Cancel(r.Context(), id); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// SystemPromptPreview returns the system prompt a turn in this conversation would receive — a
// transparency / debugging aid (R0057).
//
// SystemPromptPreview 返回本对话一个回合会收到的 system prompt——透明度 / 调试辅助（R0057）。
func (h *ChatHandler) SystemPromptPreview(w http.ResponseWriter, r *http.Request) {
	prompt, err := h.svc.SystemPromptPreview(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]string{"systemPrompt": prompt})
}

// ListInteractions returns the human interactions this conversation is currently awaiting — the
// front end's reconnect/refresh re-sync (the live surface signal is ephemeral, R0064).
//
// ListInteractions 返回本对话当前在等的人机交互——前端重连/刷新的重新同步（live surface signal 是 ephemeral，R0064）。
func (h *ChatHandler) ListInteractions(w http.ResponseWriter, r *http.Request) {
	responsehttpapi.Success(w, http.StatusOK, h.svc.PendingInteractions(r.Context(), r.PathValue("id")))
}

// ResolveInteraction delivers a human decision (approve / approve_always / deny / accept / decline)
// to a tool blocked awaiting input in this conversation; 202 (the gated tool resumes + streams over
// the messages SSE). NO_PENDING_INTERACTION (404) if nothing is waiting on that tool_call (R0064).
//
// ResolveInteraction 把人的决定（approve / approve_always / deny / accept / decline）送给本对话中阻塞等输入的
// 工具；202（被门工具续跑 + 经 messages SSE 流式）。该 tool_call 无等待项则 NO_PENDING_INTERACTION (404)。
func (h *ChatHandler) ResolveInteraction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"` // approve | approve_always | deny | accept | decline
		Answer string `json:"answer"` // ask accept: the user's answer
	}
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	if err := h.svc.ResolveInteraction(r.Context(), r.PathValue("toolCallId"), req.Action, req.Answer); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w) // 纯状态变更、无新产物(MD4)
}

// Usage returns a conversation's total token cost (the tokensUsed the detail view shows, R0057).
//
// Usage 返回一个对话的 token 总成本（详情视图显示的 tokensUsed，R0057）。
func (h *ChatHandler) Usage(w http.ResponseWriter, r *http.Request) {
	in, out, err := h.svc.Usage(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]int{
		"inputTokens":  in,
		"outputTokens": out,
		"totalTokens":  in + out,
	})
}

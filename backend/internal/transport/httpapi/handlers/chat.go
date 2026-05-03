// chat.go — HTTP handlers for chat endpoints: attachment upload, message
// send/list/cancel, and the SSE event stream.
//
// chat.go — 聊天端点的 HTTP handler：附件上传、消息发送/列表/取消、SSE 事件流。
package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"go.uber.org/zap"

	chatapp "github.com/sunweilin/forgify/backend/internal/app/chat"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventsdomain "github.com/sunweilin/forgify/backend/internal/domain/events"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// ChatHandler serves the 5 chat endpoints.
//
// ChatHandler 提供 5 个聊天端点。
type ChatHandler struct {
	svc    *chatapp.Service
	bridge eventsdomain.Bridge
	log    *zap.Logger
}

// NewChatHandler wires the handler dependencies.
//
// NewChatHandler 装配 handler 依赖。
func NewChatHandler(svc *chatapp.Service, bridge eventsdomain.Bridge, log *zap.Logger) *ChatHandler {
	return &ChatHandler{svc: svc, bridge: bridge, log: log}
}

// Register attaches chat routes.
//
// Register 挂载聊天路由。
func (h *ChatHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/attachments", h.UploadAttachment)
	mux.HandleFunc("POST /api/v1/conversations/{id}/messages", h.SendMessage)
	mux.HandleFunc("DELETE /api/v1/conversations/{id}/stream", h.CancelStream)
	mux.HandleFunc("GET /api/v1/conversations/{id}/messages", h.ListMessages)
	mux.HandleFunc("GET /api/v1/events", h.EventsSSE)
}

// ── POST /api/v1/attachments ─────────────────────────────────────────────────

// UploadAttachment: POST /api/v1/attachments → 201.
//
// UploadAttachment：POST /api/v1/attachments → 201。
func (h *ChatHandler) UploadAttachment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(chatdomain.MaxAttachmentBytes); err != nil {
		responsehttpapi.FromDomainError(w, h.log, fmt.Errorf("%w: %v", chatdomain.ErrAttachmentTooLarge, err))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, fmt.Errorf("%w: missing file field", chatdomain.ErrAttachmentParseFailed))
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, fmt.Errorf("%w: read failed", chatdomain.ErrAttachmentParseFailed))
		return
	}

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	att, err := h.svc.UploadAttachment(r.Context(), data, mimeType, header.Filename)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, att)
}

// ── POST /api/v1/conversations/{id}/messages ─────────────────────────────────

type sendMessageRequest struct {
	Content       string   `json:"content"`
	AttachmentIDs []string `json:"attachmentIds"`
}

// SendMessage: POST /api/v1/conversations/{id}/messages → 202.
//
// SendMessage：POST /api/v1/conversations/{id}/messages → 202。
func (h *ChatHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req sendMessageRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	msgID, err := h.svc.Send(r.Context(), id, chatapp.SendInput{
		Content:       req.Content,
		AttachmentIDs: req.AttachmentIDs,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusAccepted, map[string]string{"messageId": msgID})
}

// ── DELETE /api/v1/conversations/{id}/stream ─────────────────────────────────

// CancelStream: DELETE /api/v1/conversations/{id}/stream → 204.
//
// CancelStream：DELETE /api/v1/conversations/{id}/stream → 204。
func (h *ChatHandler) CancelStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.svc.Cancel(r.Context(), id); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

// ── GET /api/v1/conversations/{id}/messages ──────────────────────────────────

// ListMessages: GET /api/v1/conversations/{id}/messages → 200 paged.
//
// ListMessages：GET /api/v1/conversations/{id}/messages → 200 分页。
func (h *ChatHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := paginationpkg.Parse(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	items, next, err := h.svc.ListMessages(r.Context(), id, chatdomain.ListFilter{
		Cursor: p.Cursor,
		Limit:  p.Limit,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, items, next, next != "")
}

// ── GET /api/v1/events ───────────────────────────────────────────────────────

// EventsSSE: GET /api/v1/events?conversationId=xxx → SSE stream.
// Each event carries a per-connection sequence ID for debugging; server-side
// replay is not supported (in-memory bridge has no history buffer). Standard
// SSE setup (headers, keep-alive, ctx-driven shutdown) is delegated to
// responsehttpapi.StreamSSE.
//
// EventsSSE：GET /api/v1/events?conversationId=xxx → SSE 流。
// 每个事件带连接内自增 ID（供调试）；不支持服务端 replay（内存 bridge 无历史
// 缓冲）。SSE 标准设置（header、keep-alive、ctx 驱动退出）委托给
// responsehttpapi.StreamSSE。
func (h *ChatHandler) EventsSSE(w http.ResponseWriter, r *http.Request) {
	conversationID := r.URL.Query().Get("conversationId")
	if conversationID == "" {
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "conversationId is required", nil)
		return
	}

	ch, unsub := h.bridge.Subscribe(r.Context(), conversationID)
	defer unsub()

	var eventID int
	responsehttpapi.StreamSSE(w, r, nil, ch,
		func(out io.Writer, event eventsdomain.Event) error {
			eventID++
			data, err := json.Marshal(event)
			if err != nil {
				h.log.Warn("SSE marshal failed", zap.Error(err))
				return err
			}
			_, err = fmt.Fprintf(out, "id: %d\nevent: %s\ndata: %s\n\n", eventID, event.EventName(), data)
			return err
		},
	)
}

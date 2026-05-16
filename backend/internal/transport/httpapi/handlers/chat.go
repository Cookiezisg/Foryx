package handlers

import (
	"fmt"
	"io"
	"net/http"

	"go.uber.org/zap"

	chatapp "github.com/sunweilin/forgify/backend/internal/app/chat"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// ChatHandler serves attachment + message HTTP endpoints.
//
// ChatHandler 提供附件 + 消息 HTTP 端点。
type ChatHandler struct {
	svc *chatapp.Service
	log *zap.Logger
}

func NewChatHandler(svc *chatapp.Service, log *zap.Logger) *ChatHandler {
	return &ChatHandler{svc: svc, log: log}
}

func (h *ChatHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/attachments", h.UploadAttachment)
	mux.HandleFunc("POST /api/v1/conversations/{id}/messages", h.SendMessage)
	mux.HandleFunc("DELETE /api/v1/conversations/{id}/stream", h.CancelStream)
	mux.HandleFunc("GET /api/v1/conversations/{id}/messages", h.ListMessages)
}

func (h *ChatHandler) UploadAttachment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(chatdomain.MaxAttachmentBytes); err != nil {
		responsehttpapi.FromDomainError(w, h.log, fmt.Errorf("handlers.UploadAttachment: parseMultipart: %w (%v)", chatdomain.ErrAttachmentTooLarge, err))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, fmt.Errorf("handlers.UploadAttachment: missing file field: %w", chatdomain.ErrAttachmentParseFailed))
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, fmt.Errorf("handlers.UploadAttachment: read failed: %w (%v)", chatdomain.ErrAttachmentParseFailed, err))
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

type sendMessageRequest struct {
	Content       string   `json:"content"`
	AttachmentIDs []string `json:"attachmentIds"`
}

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

func (h *ChatHandler) CancelStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.svc.Cancel(r.Context(), id); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

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

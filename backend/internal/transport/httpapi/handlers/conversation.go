package handlers

import (
	"net/http"

	"go.uber.org/zap"

	convapp "github.com/sunweilin/forgify/backend/internal/app/conversation"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// ConversationHandler serves the 5 /api/v1/conversations/* endpoints.
//
// ConversationHandler 提供 /api/v1/conversations/* 的 5 个端点。
type ConversationHandler struct {
	svc *convapp.Service
	log *zap.Logger
}

func NewConversationHandler(svc *convapp.Service, log *zap.Logger) *ConversationHandler {
	return &ConversationHandler{svc: svc, log: log}
}

func (h *ConversationHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/conversations", h.Create)
	mux.HandleFunc("GET /api/v1/conversations", h.List)
	mux.HandleFunc("GET /api/v1/conversations/{id}", h.Get)
	mux.HandleFunc("PATCH /api/v1/conversations/{id}", h.Rename)
	mux.HandleFunc("DELETE /api/v1/conversations/{id}", h.Delete)
}

type createConvRequest struct {
	Title string `json:"title"`
}

// updateConvRequest uses pointer fields so absent vs explicit-clear are distinct.
//
// updateConvRequest 用指针字段区分"未传"和"传空"。
type updateConvRequest struct {
	Title        *string `json:"title,omitempty"`
	SystemPrompt *string `json:"systemPrompt,omitempty"`
}

func (h *ConversationHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createConvRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	c, err := h.svc.Create(r.Context(), req.Title)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, c)
}

func (h *ConversationHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := paginationpkg.Parse(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	items, next, err := h.svc.List(r.Context(), convdomain.ListFilter{
		Cursor: p.Cursor,
		Limit:  p.Limit,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, items, next, next != "")
}

func (h *ConversationHandler) Get(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, c)
}

// Rename is a partial-update PATCH accepting title and/or systemPrompt.
//
// Rename 是部分更新 PATCH,接 title 和/或 systemPrompt。
func (h *ConversationHandler) Rename(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateConvRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	c, err := h.svc.Update(r.Context(), id, req.Title, req.SystemPrompt)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusOK, c)
}

func (h *ConversationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

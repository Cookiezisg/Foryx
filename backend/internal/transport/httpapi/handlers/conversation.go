package handlers

import (
	"context"
	"net/http"

	"go.uber.org/zap"

	convapp "github.com/sunweilin/forgify/backend/internal/app/conversation"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// TokenSummer reports aggregate token usage for a conversation. Injected
// into ConversationHandler.Get so the response can carry tokensUsed
// without an extra request (V1.2 §4.1).
//
// TokenSummer 报告 conversation 聚合 token 使用量。注入 Get 让响应直接
// 携带 tokensUsed，省一次请求（V1.2 §4.1）。
type TokenSummer interface {
	SumTokensForConversation(ctx context.Context, convID string) (chatdomain.TokensUsed, error)
}

// ConversationHandler serves the 5 /api/v1/conversations/* endpoints.
//
// ConversationHandler 提供 /api/v1/conversations/* 的 5 个端点。
type ConversationHandler struct {
	svc    *convapp.Service
	tokens TokenSummer // optional; nil → omit tokensUsed from response
	log    *zap.Logger
}

func NewConversationHandler(svc *convapp.Service, tokens TokenSummer, log *zap.Logger) *ConversationHandler {
	return &ConversationHandler{svc: svc, tokens: tokens, log: log}
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

// convWithTokens embeds the Conversation entity flat and tacks on
// aggregated tokensUsed (V1.2 §4.1). When TokenSummer is unwired we
// just omit the field.
//
// convWithTokens 平铺 Conversation 实体 + 附加 tokensUsed 聚合（§4.1）。
// 未接 TokenSummer 时省略字段。
type convWithTokens struct {
	*convdomain.Conversation
	TokensUsed *chatdomain.TokensUsed `json:"tokensUsed,omitempty"`
}

func (h *ConversationHandler) Get(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	resp := convWithTokens{Conversation: c}
	if h.tokens != nil {
		if t, sumErr := h.tokens.SumTokensForConversation(r.Context(), c.ID); sumErr == nil {
			resp.TokensUsed = &t
		} else {
			// Soft-fail: log + ship the conv without tokensUsed; we don't
			// want token-sum hiccups to make the conv unfetchable.
			// 软失败：log + 不带 tokensUsed 返；不让 token 求和拖死 conv 取。
			h.log.Warn("conversation.Get: SumTokens failed (non-fatal)",
				zap.String("conv_id", c.ID), zap.Error(sumErr))
		}
	}
	responsehttpapi.Success(w, http.StatusOK, resp)
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

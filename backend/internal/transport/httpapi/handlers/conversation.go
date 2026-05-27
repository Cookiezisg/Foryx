package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"go.uber.org/zap"

	chatapp "github.com/sunweilin/forgify/backend/internal/app/chat"
	convapp "github.com/sunweilin/forgify/backend/internal/app/conversation"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	documentdomain "github.com/sunweilin/forgify/backend/internal/domain/document"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	tokencountpkg "github.com/sunweilin/forgify/backend/internal/pkg/tokencount"
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

// SystemPromptPreviewer returns the assembled system prompt sections for a conversation (§18.2).
//
// SystemPromptPreviewer 返某 conversation 拼装好的 system prompt 段（§18.2）。
type SystemPromptPreviewer interface {
	SystemPromptSections(ctx context.Context, conv *convdomain.Conversation) []chatapp.PromptSection
}

// ConversationHandler serves the 5 /api/v1/conversations/* endpoints.
//
// ConversationHandler 提供 /api/v1/conversations/* 的 5 个端点。
type ConversationHandler struct {
	svc             *convapp.Service
	tokens          TokenSummer            // optional; nil → omit tokensUsed from response
	promptPreviewer SystemPromptPreviewer  // optional; nil → no /system-prompt-preview endpoint
	log             *zap.Logger
}

func NewConversationHandler(svc *convapp.Service, tokens TokenSummer, log *zap.Logger) *ConversationHandler {
	return &ConversationHandler{svc: svc, tokens: tokens, log: log}
}

// SetSystemPromptPreviewer enables the §18.2 preview endpoint; call during DI wire-up.
//
// SetSystemPromptPreviewer 启 §18.2 预览端点；装配阶段调一次。
func (h *ConversationHandler) SetSystemPromptPreviewer(p SystemPromptPreviewer) {
	h.promptPreviewer = p
}

func (h *ConversationHandler) Register(mux Registrar) {
	mux.HandleFunc("POST /api/v1/conversations", h.Create)
	mux.HandleFunc("GET /api/v1/conversations", h.List)
	mux.HandleFunc("GET /api/v1/conversations/{id}", h.Get)
	mux.HandleFunc("PATCH /api/v1/conversations/{id}", h.Rename)
	mux.HandleFunc("DELETE /api/v1/conversations/{id}", h.Delete)
	if h.promptPreviewer != nil {
		mux.HandleFunc("GET /api/v1/conversations/{id}/system-prompt-preview", h.SystemPromptPreview)
	}
}

type createConvRequest struct {
	Title string `json:"title"`
}

// updateConvRequest uses pointer fields so absent vs explicit-clear are distinct.
//
// updateConvRequest 用指针字段区分"未传"和"传空"。
type updateConvRequest struct {
	Title             *string                              `json:"title,omitempty"`
	SystemPrompt      *string                              `json:"systemPrompt,omitempty"`
	AttachedDocuments *[]documentdomain.AttachedDocument   `json:"attachedDocuments,omitempty"`
	Archived          *bool                                `json:"archived,omitempty"`
	Pinned            *bool                                `json:"pinned,omitempty"`
	// ModelOverride: 缺字段 = 不变；显式 null = 清除；显式 object = 设置。需 hasModelOverride 区分缺/null。
	ModelOverride     *modeldomain.ModelRef                `json:"modelOverride,omitempty"`
	HasModelOverride  bool                                 `json:"-"`
}

// UnmarshalJSON detects whether `modelOverride` was present as a key
// (vs absent), to distinguish "leave unchanged" from "explicitly clear to null".
//
// UnmarshalJSON 检测 `modelOverride` 是否在 JSON 中出现（区分"未传"与"显式 null 清除"）。
func (r *updateConvRequest) UnmarshalJSON(data []byte) error {
	type raw updateConvRequest
	if err := json.Unmarshal(data, (*raw)(r)); err != nil {
		return err
	}
	// Second pass: detect key presence on the raw map.
	// 二次扫描：探测 raw map 上 key 是否存在。
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err == nil {
		_, r.HasModelOverride = m["modelOverride"]
	}
	return nil
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
	// §17.12 archived filter: nil = exclude archived (default), "true" / "false" = explicit.
	// §17.12 archived 过滤：缺省排除已归档；显式 "true"/"false" 按值过滤。
	var archived *bool
	if v := r.URL.Query().Get("archived"); v != "" {
		b := v == "true" || v == "1"
		archived = &b
	}
	items, next, err := h.svc.List(r.Context(), convdomain.ListFilter{
		Cursor:   p.Cursor,
		Limit:    p.Limit,
		Search:   r.URL.Query().Get("search"),
		Archived: archived,
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
	in := convapp.UpdateInput{
		Title:             req.Title,
		SystemPrompt:      req.SystemPrompt,
		AttachedDocuments: req.AttachedDocuments,
		Archived:          req.Archived,
		Pinned:            req.Pinned,
	}
	// §12.3 modelOverride: present in JSON (even as null) → set pointer-to-pointer for tristate.
	// §12.3 modelOverride：JSON 中出现（含 null）→ 用 **ptr 三态。
	if req.HasModelOverride {
		in.ModelOverride = &req.ModelOverride
	}
	c, err := h.svc.Update(r.Context(), id, in)
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

// SystemPromptPreview returns the assembled system prompt for one conv,
// broken down by section so users can see exactly what's sent to the LLM (§18.2).
//
// SystemPromptPreview 返该 conv 拼装好的 system prompt，按段拆解（§18.2）。
func (h *ConversationHandler) SystemPromptPreview(w http.ResponseWriter, r *http.Request) {
	conv, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	sections := h.promptPreviewer.SystemPromptSections(r.Context(), conv)
	assembled := chatapp.AssemblePromptSections(sections)
	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"conversationId": conv.ID,
		"sections":       sections,
		"assembled":      assembled,
		"totalLength":    len(assembled),
		"totalTokensEst": tokencountpkg.Estimate(assembled),
	})
}

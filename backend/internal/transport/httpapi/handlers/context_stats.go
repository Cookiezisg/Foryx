package handlers

import (
	"context"
	"net/http"

	"go.uber.org/zap"

	convapp "github.com/sunweilin/forgify/backend/internal/app/conversation"
	documentapp "github.com/sunweilin/forgify/backend/internal/app/document"
	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
	documentdomain "github.com/sunweilin/forgify/backend/internal/domain/document"
	memorydomain "github.com/sunweilin/forgify/backend/internal/domain/memory"
	tokencountpkg "github.com/sunweilin/forgify/backend/internal/pkg/tokencount"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// ContextStatsHandler exposes /api/v1/conversations/{id}/context-stats (§4.8).
// Estimates token budget per static section so the user can see what's
// eating context before LLM history is even loaded.
//
// ContextStatsHandler 暴露 /api/v1/conversations/{id}/context-stats (§4.8)。
// 估算静态各段 token 占用,用户能看见 history 还没装就吃掉多少 context。
type ContextStatsHandler struct {
	conv     *convapp.Service
	catalog  catalogdomain.SystemPromptProvider
	memory   memorydomain.SystemPromptProvider
	document *documentapp.Service
	tokens   TokenSummer
	log      *zap.Logger
}

func NewContextStatsHandler(
	conv *convapp.Service,
	catalog catalogdomain.SystemPromptProvider,
	memory memorydomain.SystemPromptProvider,
	document *documentapp.Service,
	tokens TokenSummer,
	log *zap.Logger,
) *ContextStatsHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &ContextStatsHandler{
		conv: conv, catalog: catalog, memory: memory,
		document: document, tokens: tokens,
		log: log.Named("handlers.context_stats"),
	}
}

func (h *ContextStatsHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/conversations/{id}/context-stats", h.Get)
}

type sectionStat struct {
	Section   string `json:"section"`
	Chars     int    `json:"chars"`
	EstTokens int    `json:"estTokens"`
}

func (h *ContextStatsHandler) Get(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	if h.conv == nil {
		responsehttpapi.Error(w, http.StatusServiceUnavailable, "CONTEXT_STATS_UNAVAILABLE",
			"conversation service not wired", nil)
		return
	}
	c, err := h.conv.Get(r.Context(), convID)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}

	sections := []sectionStat{}
	addSection := func(name, text string) {
		if text == "" {
			return
		}
		sections = append(sections, sectionStat{
			Section:   name,
			Chars:     len(text),
			EstTokens: tokencountpkg.Estimate(text),
		})
	}

	// Static sections — sources of the system prompt.
	// 静态段:system prompt 来源各段。
	if h.catalog != nil {
		addSection("catalogSummary", h.catalog.GetForSystemPrompt(r.Context()))
	}
	if h.memory != nil {
		addSection("memorySection", h.memory.ForSystemPrompt(r.Context()))
	}
	if c.SystemPrompt != "" {
		addSection("conversationSystemPrompt", c.SystemPrompt)
	}
	if h.document != nil && len(c.AttachedDocuments) > 0 {
		docsXML, err := renderAttachedXML(r.Context(), h.document, c.AttachedDocuments)
		if err != nil {
			h.log.Warn("context-stats: render attached docs failed",
				zap.String("conv_id", convID), zap.Error(err))
		}
		addSection("attachedDocuments", docsXML)
	}

	// History totals from existing TokenSummer (no need to walk messages).
	//
	// History 总量经现有 TokenSummer (省遍历 messages)。
	var historyInput, historyOutput int
	if h.tokens != nil {
		if t, err := h.tokens.SumTokensForConversation(r.Context(), convID); err == nil {
			historyInput = t.Input
			historyOutput = t.Output
		}
	}

	totalStatic := 0
	totalChars := 0
	for _, s := range sections {
		totalStatic += s.EstTokens
		totalChars += s.Chars
	}

	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"conversationId": convID,
		"sections":       sections,
		"static": map[string]int{
			"chars":     totalChars,
			"estTokens": totalStatic,
		},
		"history": map[string]int{
			"inputTokens":  historyInput,
			"outputTokens": historyOutput,
		},
		"note": "estTokens is a rough char-based estimate (CJK 1tok/char, ascii bytes/4); use for budgeting not billing",
	})
}

func renderAttachedXML(ctx context.Context, svc *documentapp.Service, atts []documentdomain.AttachedDocument) (string, error) {
	docs, err := svc.ResolveAttached(ctx, atts)
	if err != nil {
		return "", err
	}
	return documentapp.RenderAttachedAsXML(docs), nil
}

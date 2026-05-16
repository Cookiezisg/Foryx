// usage.go — GET /api/v1/usage: per-conversation or per-period token +
// cost aggregation (V1.2 §4.2 final-sweep). One endpoint, two query
// modes — `?conversationId=cv_xxx` scopes to one conv; `?period=day|
// week|month|all` aggregates across the user's whole history.
//
// usage.go ——GET /api/v1/usage：per-conversation 或 per-period 的 token
// + cost 聚合（V1.2 §4.2）。一个端点两种 query 模式：?conversationId=
// 按对话；?period=day|week|month|all 按时间区间。
package handlers

import (
	"context"
	"net/http"
	"time"

	"go.uber.org/zap"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	"github.com/sunweilin/forgify/backend/internal/pkg/llmcost"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// UsageProvider exposes the two token-aggregation calls the handler
// needs. chatapp.Service implements it.
//
// UsageProvider 暴露 handler 需要的两个 token 聚合调用，chatapp.Service 实现。
type UsageProvider interface {
	SumTokensForConversation(ctx context.Context, convID string) (chatdomain.TokensUsed, error)
	SumTokensByPeriod(ctx context.Context, since, until time.Time) ([]chatdomain.TokensByModel, error)
}

// UsageHandler serves GET /api/v1/usage.
//
// UsageHandler 提供 GET /api/v1/usage。
type UsageHandler struct {
	provider UsageProvider
	log      *zap.Logger
}

func NewUsageHandler(provider UsageProvider, log *zap.Logger) *UsageHandler {
	return &UsageHandler{provider: provider, log: log}
}

func (h *UsageHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/usage", h.Get)
}

// modelBreakdownRow is one row of the usage response — broken down by
// (provider, modelId) so the UI can show which model burned the most.
//
// modelBreakdownRow 是 usage 响应一行——按 (provider, modelId) 拆，UI
// 显示哪个 model 烧得多。
type modelBreakdownRow struct {
	Provider        string  `json:"provider"`
	ModelID         string  `json:"modelId"`
	InputTokens     int     `json:"inputTokens"`
	OutputTokens    int     `json:"outputTokens"`
	TotalTokens     int     `json:"totalTokens"`
	CostEstimateUsd float64 `json:"costEstimateUsd"`
	CostKnown       bool    `json:"costKnown"`
}

type usageResponse struct {
	Scope           string              `json:"scope"`
	ConversationID  string              `json:"conversationId,omitempty"`
	Period          *periodWindow       `json:"period,omitempty"`
	InputTokens     int                 `json:"inputTokens"`
	OutputTokens    int                 `json:"outputTokens"`
	TotalTokens     int                 `json:"totalTokens"`
	CostEstimateUsd float64             `json:"costEstimateUsd"`
	ByModel         []modelBreakdownRow `json:"byModel"`
	Note            string              `json:"note,omitempty"`
}

type periodWindow struct {
	Since string `json:"since"`
	Until string `json:"until"`
}

// Get dispatches based on query params:
//   - ?conversationId=cv_xxx → per-conv scope (cost unavailable without
//     model breakdown; we omit cost in this mode and only return totals)
//   - ?period=day|week|month|all (default: all) → per-period scope with
//     full model breakdown + cost estimate
//
// Get 按 query 参分派：conversationId 仅返 totals（per-conv 没有 model
// 拆，无 cost）；period 模式有完整 model 拆 + cost 估算。
func (h *UsageHandler) Get(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if convID := q.Get("conversationId"); convID != "" {
		t, err := h.provider.SumTokensForConversation(r.Context(), convID)
		if err != nil {
			responsehttpapi.FromDomainError(w, h.log, err)
			return
		}
		responsehttpapi.Success(w, http.StatusOK, usageResponse{
			Scope:          "conversation",
			ConversationID: convID,
			InputTokens:    t.Input,
			OutputTokens:   t.Output,
			TotalTokens:    t.Total,
			ByModel:        []modelBreakdownRow{},
			Note:           "per-conversation totals; use ?period= for cost estimate by model.",
		})
		return
	}

	since, until, periodLabel := parsePeriod(q.Get("period"))
	rows, err := h.provider.SumTokensByPeriod(r.Context(), since, until)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}

	var totalIn, totalOut int
	var totalCost float64
	breakdown := make([]modelBreakdownRow, 0, len(rows))
	for _, row := range rows {
		_, known := llmcost.Lookup(row.Provider, row.ModelID)
		cost := llmcost.Estimate(row.Provider, row.ModelID, row.Input, row.Output)
		breakdown = append(breakdown, modelBreakdownRow{
			Provider:        coalesceProvider(row.Provider),
			ModelID:         coalesceModelID(row.ModelID),
			InputTokens:     row.Input,
			OutputTokens:    row.Output,
			TotalTokens:     row.Input + row.Output,
			CostEstimateUsd: cost,
			CostKnown:       known,
		})
		totalIn += row.Input
		totalOut += row.Output
		totalCost += cost
	}

	resp := usageResponse{
		Scope:           "period",
		Period:          &periodWindow{Since: formatPeriodTime(since), Until: formatPeriodTime(until)},
		InputTokens:     totalIn,
		OutputTokens:    totalOut,
		TotalTokens:     totalIn + totalOut,
		CostEstimateUsd: totalCost,
		ByModel:         breakdown,
		Note: "Cost estimates are based on public per-model pricing snapshots; treat as ballpark. " +
			"Period: " + periodLabel + ".",
	}
	responsehttpapi.Success(w, http.StatusOK, resp)
}

// parsePeriod maps the ?period= label to (since, until, label). "all"
// (or empty) returns zero times so the SQL skips bounds.
//
// parsePeriod 把 ?period= 映射到 (since, until, label)。"all" / 空返
// 零值，SQL 跳掉时间约束。
func parsePeriod(label string) (since, until time.Time, displayLabel string) {
	now := time.Now().UTC()
	switch label {
	case "day":
		return now.Add(-24 * time.Hour), now, "last 24h"
	case "week":
		return now.AddDate(0, 0, -7), now, "last 7 days"
	case "month":
		return now.AddDate(0, -1, 0), now, "last 30 days"
	case "", "all":
		return time.Time{}, time.Time{}, "all time"
	default:
		// Unknown label — treat as "all" and surface the choice in note
		// rather than 400 (zero risk of wedging clients on a typo).
		// 未知 label 当 all 处理 + 在 note 提示；不返 400 防止打字错卡客户端。
		return time.Time{}, time.Time{}, "all time (unknown period " + label + ")"
	}
}

func formatPeriodTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func coalesceProvider(p string) string {
	if p == "" {
		return "(unknown)"
	}
	return p
}

func coalesceModelID(m string) string {
	if m == "" {
		return "(unknown)"
	}
	return m
}

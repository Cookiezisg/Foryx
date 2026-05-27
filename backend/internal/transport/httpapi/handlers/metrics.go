package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// MetricsHandler exposes /api/v1/metrics/* dashboards aggregated from the
// 4 D22 execution-log tables (function_executions / handler_calls /
// mcp_calls / skill_executions). §4.5.
//
// MetricsHandler 暴露 /api/v1/metrics/* 看板,从 4 张 D22 execution-log 表聚合(§4.5)。
type MetricsHandler struct {
	functionExec functiondomain.ExecutionRepository
	handlerCall  handlerdomain.CallRepository
	mcpCall      mcpdomain.CallRepository
	skillExec    skilldomain.ExecutionRepository
	log          *zap.Logger
}

func NewMetricsHandler(
	functionExec functiondomain.ExecutionRepository,
	handlerCall handlerdomain.CallRepository,
	mcpCall mcpdomain.CallRepository,
	skillExec skilldomain.ExecutionRepository,
	log *zap.Logger,
) *MetricsHandler {
	return &MetricsHandler{
		functionExec: functionExec,
		handlerCall:  handlerCall,
		mcpCall:      mcpCall,
		skillExec:    skillExec,
		log:          log,
	}
}

func (h *MetricsHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/metrics/tools", h.Tools)
}

// Tools returns aggregated metrics for the 4 tool families within the
// since window (default 7d). Per-tool granularity (group by FunctionID
// etc.) deferred until a real consumer needs it.
//
// Tools 返 4 类工具家族在 since 窗口内的聚合(默认 7 天)。
// per-tool 粒度(按 FunctionID group by)等真消费方再上。
func (h *MetricsHandler) Tools(w http.ResponseWriter, r *http.Request) {
	since := parseSinceDuration(r.URL.Query().Get("since"), 7*24*time.Hour)
	cutoff := time.Now().Add(-since).UTC()

	type bucket struct {
		Source        string  `json:"source"`
		OKCount       int     `json:"okCount"`
		FailedCount   int     `json:"failedCount"`
		CancelledCnt  int     `json:"cancelledCount"`
		TimeoutCount  int     `json:"timeoutCount"`
		TotalCount    int     `json:"totalCount"`
		SuccessRatePc float64 `json:"successRatePercent"`
		AvgElapsedMs  int64   `json:"avgElapsedMs"`
		P95ElapsedMs  int64   `json:"p95ElapsedMs"`
	}

	out := []bucket{}

	if h.functionExec != nil {
		if agg, err := h.functionExec.ComputeAggregates(r.Context(), functiondomain.ExecutionFilter{Since: &cutoff}); err != nil {
			h.log.Warn("metrics: function exec aggregates failed", zap.Error(err))
		} else {
			out = append(out, makeBucket("function", agg.OKCount, agg.FailedCount, agg.CancelledCount, agg.TimeoutCount, agg.AvgElapsedMs, agg.P95ElapsedMs))
		}
	}
	if h.handlerCall != nil {
		if agg, err := h.handlerCall.ComputeCallAggregates(r.Context(), handlerdomain.CallFilter{Since: &cutoff}); err != nil {
			h.log.Warn("metrics: handler call aggregates failed", zap.Error(err))
		} else {
			out = append(out, makeBucket("handler", agg.OKCount, agg.FailedCount, agg.CancelledCount, agg.TimeoutCount, agg.AvgElapsedMs, agg.P95ElapsedMs))
		}
	}
	if h.mcpCall != nil {
		if agg, err := h.mcpCall.ComputeAggregates(r.Context(), mcpdomain.CallFilter{Since: &cutoff}); err != nil {
			h.log.Warn("metrics: mcp call aggregates failed", zap.Error(err))
		} else {
			out = append(out, makeBucket("mcp", agg.OKCount, agg.FailedCount, agg.CancelledCount, agg.TimeoutCount, agg.AvgElapsedMs, agg.P95ElapsedMs))
		}
	}
	if h.skillExec != nil {
		if agg, err := h.skillExec.ComputeAggregates(r.Context(), skilldomain.ExecutionFilter{Since: &cutoff}); err != nil {
			h.log.Warn("metrics: skill exec aggregates failed", zap.Error(err))
		} else {
			out = append(out, makeBucket("skill", agg.OKCount, agg.FailedCount, agg.CancelledCount, agg.TimeoutCount, agg.AvgElapsedMs, agg.P95ElapsedMs))
		}
	}

	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"since":   cutoff,
		"until":   time.Now().UTC(),
		"window":  since.String(),
		"buckets": out,
	})
}

func makeBucket(source string, okN, failN, cancN, timeoutN int, avg, p95 int64) struct {
	Source        string  `json:"source"`
	OKCount       int     `json:"okCount"`
	FailedCount   int     `json:"failedCount"`
	CancelledCnt  int     `json:"cancelledCount"`
	TimeoutCount  int     `json:"timeoutCount"`
	TotalCount    int     `json:"totalCount"`
	SuccessRatePc float64 `json:"successRatePercent"`
	AvgElapsedMs  int64   `json:"avgElapsedMs"`
	P95ElapsedMs  int64   `json:"p95ElapsedMs"`
} {
	total := okN + failN + cancN + timeoutN
	rate := 0.0
	if total > 0 {
		rate = float64(okN) / float64(total) * 100.0
	}
	return struct {
		Source        string  `json:"source"`
		OKCount       int     `json:"okCount"`
		FailedCount   int     `json:"failedCount"`
		CancelledCnt  int     `json:"cancelledCount"`
		TimeoutCount  int     `json:"timeoutCount"`
		TotalCount    int     `json:"totalCount"`
		SuccessRatePc float64 `json:"successRatePercent"`
		AvgElapsedMs  int64   `json:"avgElapsedMs"`
		P95ElapsedMs  int64   `json:"p95ElapsedMs"`
	}{
		Source:        source,
		OKCount:       okN,
		FailedCount:   failN,
		CancelledCnt:  cancN,
		TimeoutCount:  timeoutN,
		TotalCount:    total,
		SuccessRatePc: rate,
		AvgElapsedMs:  avg,
		P95ElapsedMs:  p95,
	}
}

// parseSinceDuration accepts "7d" / "24h" / "30m" / pure seconds; falls back to default.
//
// parseSinceDuration 接受 "7d" / "24h" / "30m" / 纯秒数;失败用 default。
func parseSinceDuration(raw string, def time.Duration) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	if strings.HasSuffix(raw, "d") {
		if n, err := strconv.Atoi(strings.TrimSuffix(raw, "d")); err == nil && n > 0 {
			return time.Duration(n) * 24 * time.Hour
		}
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	return def
}

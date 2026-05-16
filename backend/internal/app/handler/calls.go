package handler

import (
	"context"
	"fmt"

	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// SearchCallsResult is the response shape for SearchCalls.
//
// SearchCallsResult 是 SearchCalls 的响应形状。
type SearchCallsResult struct {
	Count      int                            `json:"count"`
	Calls      []*handlerdomain.Call          `json:"calls"`
	NextCursor string                         `json:"nextCursor,omitempty"`
	HasMore    bool                           `json:"hasMore"`
	Aggregates handlerdomain.CallAggregates   `json:"aggregates"`
}

// CallDetail wraps a Call with machine-derived hints.
//
// CallDetail 把 Call 与机器计算的 hints 一起返回。
type CallDetail struct {
	*handlerdomain.Call
	Hints CallHints `json:"hints"`
}

// CallHints flags non-obvious signals.
//
// CallHints 标记非显然的信号。
type CallHints struct {
	OutputEmpty         bool `json:"outputEmpty"`
	SignificantlySlower bool `json:"significantlySlower"`
}

// SearchCalls runs a paginated call-log query with aggregates.
//
// SearchCalls 跑分页 call-log 查询并附聚合。
func (s *Service) SearchCalls(ctx context.Context, filter handlerdomain.CallFilter) (*SearchCallsResult, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("handlerapp.SearchCalls: %w", err)
	}
	rows, next, err := s.repo.ListCalls(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.SearchCalls: %w", err)
	}
	agg, err := s.repo.ComputeCallAggregates(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.SearchCalls: aggregates: %w", err)
	}
	return &SearchCallsResult{
		Count:      len(rows),
		Calls:      rows,
		NextCursor: next,
		HasMore:    next != "",
		Aggregates: agg,
	}, nil
}

// GetCallDetail returns one call row with computed hints.
//
// GetCallDetail 返单 call 行加计算 hints。
func (s *Service) GetCallDetail(ctx context.Context, id string) (*CallDetail, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("handlerapp.GetCallDetail: %w", err)
	}
	row, err := s.repo.GetCallByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.GetCallDetail: %w", err)
	}
	hints := buildCallHints(ctx, s, row)
	return &CallDetail{Call: row, Hints: hints}, nil
}

func buildCallHints(ctx context.Context, s *Service, c *handlerdomain.Call) CallHints {
	h := CallHints{}
	switch v := c.Output.(type) {
	case nil:
		h.OutputEmpty = true
	case string:
		if v == "" {
			h.OutputEmpty = true
		}
	case []any:
		if len(v) == 0 {
			h.OutputEmpty = true
		}
	case map[string]any:
		if len(v) == 0 {
			h.OutputEmpty = true
		}
	}
	agg, err := s.repo.ComputeCallAggregates(ctx, handlerdomain.CallFilter{HandlerID: c.HandlerID, Method: c.Method})
	if err == nil && agg.AvgElapsedMs > 0 && c.ElapsedMs > 3*agg.AvgElapsedMs {
		h.SignificantlySlower = true
	}
	return h
}

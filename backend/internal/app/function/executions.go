package function

import (
	"context"
	"fmt"

	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// SearchExecutionsResult is the response shape for SearchExecutions.
//
// SearchExecutionsResult 是 SearchExecutions 的响应形状。
type SearchExecutionsResult struct {
	Count      int                                  `json:"count"`
	Executions []*functiondomain.Execution          `json:"executions"`
	NextCursor string                               `json:"nextCursor,omitempty"`
	HasMore    bool                                 `json:"hasMore"`
	Aggregates functiondomain.ExecutionAggregates   `json:"aggregates"`
}

// ExecutionDetail is the raw Execution row plus machine-computed hints.
//
// ExecutionDetail 是原始 Execution 行加机器计算的 hints。
type ExecutionDetail struct {
	*functiondomain.Execution
	Hints ExecutionHints `json:"hints"`
}

// ExecutionHints flags non-obvious signals so the LLM doesn't have to recompute.
//
// ExecutionHints 标记 LLM 不必重算的信号。
type ExecutionHints struct {
	OutputEmpty           bool `json:"outputEmpty"`
	SignificantlySlower   bool `json:"significantlySlower"`
}

// SearchExecutions runs a paginated execution-log query with aggregates.
//
// SearchExecutions 跑分页 execution-log 查询并附聚合。
func (s *Service) SearchExecutions(ctx context.Context, filter functiondomain.ExecutionFilter) (*SearchExecutionsResult, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("functionapp.SearchExecutions: %w", err)
	}
	rows, next, err := s.repo.ListExecutions(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("functionapp.SearchExecutions: %w", err)
	}
	agg, err := s.repo.ComputeAggregates(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("functionapp.SearchExecutions: aggregates: %w", err)
	}
	return &SearchExecutionsResult{
		Count:      len(rows),
		Executions: rows,
		NextCursor: next,
		HasMore:    next != "",
		Aggregates: agg,
	}, nil
}

// GetExecutionDetail returns one row with machine-derived hints attached.
//
// GetExecutionDetail 返单行加 machine 算的 hints。
func (s *Service) GetExecutionDetail(ctx context.Context, id string) (*ExecutionDetail, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("functionapp.GetExecutionDetail: %w", err)
	}
	row, err := s.repo.GetExecutionByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("functionapp.GetExecutionDetail: %w", err)
	}
	hints := buildHints(ctx, s, row)
	return &ExecutionDetail{Execution: row, Hints: hints}, nil
}

func buildHints(ctx context.Context, s *Service, e *functiondomain.Execution) ExecutionHints {
	h := ExecutionHints{}
	switch v := e.Output.(type) {
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
	agg, err := s.repo.ComputeAggregates(ctx, functiondomain.ExecutionFilter{FunctionID: e.FunctionID})
	if err == nil && agg.P95ElapsedMs > 0 {
		if e.ElapsedMs > 3*agg.AvgElapsedMs && agg.AvgElapsedMs > 0 {
			h.SignificantlySlower = true
		}
	}
	return h
}

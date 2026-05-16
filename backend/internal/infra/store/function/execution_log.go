package function

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"gorm.io/gorm"

	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// SaveExecution inserts one Execution row.
//
// SaveExecution 插入一行 Execution。
func (s *Store) SaveExecution(ctx context.Context, e *functiondomain.Execution) error {
	if err := s.db.WithContext(ctx).Create(e).Error; err != nil {
		return fmt.Errorf("functionstore.SaveExecution: %w", err)
	}
	return nil
}

// GetExecutionByID returns one execution by id; ErrExecutionNotFound if absent.
//
// GetExecutionByID 按 id 取 execution；未命中返 ErrExecutionNotFound。
func (s *Store) GetExecutionByID(ctx context.Context, id string) (*functiondomain.Execution, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, fmt.Errorf("functionstore.GetExecutionByID: %w", err)
	}
	var row functiondomain.Execution
	err = s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, uid).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, functiondomain.ErrExecutionNotFound
		}
		return nil, fmt.Errorf("functionstore.GetExecutionByID: %w", err)
	}
	return &row, nil
}

// ListExecutions returns cursor-paginated executions newest-first matching filter.
//
// ListExecutions 返按 filter 过滤的分页（新→旧）。
func (s *Store) ListExecutions(ctx context.Context, filter functiondomain.ExecutionFilter) ([]*functiondomain.Execution, string, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("functionstore.ListExecutions: %w", err)
	}
	q := s.applyExecutionFilter(s.db.WithContext(ctx).Model(&functiondomain.Execution{}), uid, filter)

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	if filter.Cursor != "" {
		var cur paginationpkg.Cursor
		if err := paginationpkg.DecodeCursor(filter.Cursor, &cur); err != nil {
			return nil, "", fmt.Errorf("functionstore.ListExecutions: cursor: %w", err)
		}
		q = q.Where("(started_at, id) < (?, ?)", cur.CreatedAt, cur.ID)
	}

	var rows []*functiondomain.Execution
	if err := q.Order("started_at DESC, id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("functionstore.ListExecutions: %w", err)
	}

	var nextCursor string
	if len(rows) > limit {
		last := rows[limit-1]
		cur, encErr := paginationpkg.EncodeCursor(paginationpkg.Cursor{
			CreatedAt: last.StartedAt,
			ID:        last.ID,
		})
		if encErr != nil {
			return nil, "", fmt.Errorf("functionstore.ListExecutions: encode cursor: %w", encErr)
		}
		nextCursor = cur
		rows = rows[:limit]
	}
	return rows, nextCursor, nil
}

// ComputeAggregates returns rollup counts + p95 (in-memory from a 1000-row slice).
//
// ComputeAggregates 返 filter 匹配行的聚合 + p95（1000 行切片内存求百分位）。
func (s *Store) ComputeAggregates(ctx context.Context, filter functiondomain.ExecutionFilter) (functiondomain.ExecutionAggregates, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return functiondomain.ExecutionAggregates{}, fmt.Errorf("functionstore.ComputeAggregates: %w", err)
	}

	type countsRow struct {
		OK        int
		Failed    int
		Cancelled int
		Timeout   int
		AvgMs     float64
	}
	var counts countsRow
	q := s.applyExecutionFilter(s.db.WithContext(ctx).Model(&functiondomain.Execution{}), uid, filter)
	if err := q.Select(
		`SUM(CASE WHEN status = 'ok'        THEN 1 ELSE 0 END) AS ok,
		 SUM(CASE WHEN status = 'failed'    THEN 1 ELSE 0 END) AS failed,
		 SUM(CASE WHEN status = 'cancelled' THEN 1 ELSE 0 END) AS cancelled,
		 SUM(CASE WHEN status = 'timeout'   THEN 1 ELSE 0 END) AS timeout,
		 COALESCE(AVG(elapsed_ms), 0)                          AS avg_ms`,
	).Scan(&counts).Error; err != nil {
		return functiondomain.ExecutionAggregates{}, fmt.Errorf("functionstore.ComputeAggregates: %w", err)
	}

	var elapsedMsList []int64
	q2 := s.applyExecutionFilter(s.db.WithContext(ctx).Model(&functiondomain.Execution{}), uid, filter)
	if err := q2.Order("elapsed_ms ASC").Limit(1000).Pluck("elapsed_ms", &elapsedMsList).Error; err != nil {
		return functiondomain.ExecutionAggregates{}, fmt.Errorf("functionstore.ComputeAggregates: p95: %w", err)
	}

	agg := functiondomain.ExecutionAggregates{
		OKCount:        counts.OK,
		FailedCount:    counts.Failed,
		CancelledCount: counts.Cancelled,
		TimeoutCount:   counts.Timeout,
		AvgElapsedMs:   int64(counts.AvgMs),
	}
	if len(elapsedMsList) > 0 {
		sort.Slice(elapsedMsList, func(i, j int) bool { return elapsedMsList[i] < elapsedMsList[j] })
		idx := (len(elapsedMsList) * 95) / 100
		if idx >= len(elapsedMsList) {
			idx = len(elapsedMsList) - 1
		}
		agg.P95ElapsedMs = elapsedMsList[idx]
	}
	return agg, nil
}

func (s *Store) applyExecutionFilter(q *gorm.DB, uid string, filter functiondomain.ExecutionFilter) *gorm.DB {
	q = q.Where("user_id = ?", uid)
	if filter.FunctionID != "" {
		q = q.Where("function_id = ?", filter.FunctionID)
	}
	if filter.VersionID != "" {
		q = q.Where("version_id = ?", filter.VersionID)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.ConversationID != "" {
		q = q.Where("conversation_id = ?", filter.ConversationID)
	}
	if filter.FlowrunID != "" {
		q = q.Where("flowrun_id = ?", filter.FlowrunID)
	}
	if filter.Since != nil {
		q = q.Where("started_at >= ?", *filter.Since)
	}
	if filter.Until != nil {
		q = q.Where("started_at <= ?", *filter.Until)
	}
	return q
}

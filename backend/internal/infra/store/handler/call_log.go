package handler

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"gorm.io/gorm"

	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// SaveCall inserts one Call row.
//
// SaveCall 插入一行 Call。
func (s *Store) SaveCall(ctx context.Context, c *handlerdomain.Call) error {
	if err := s.db.WithContext(ctx).Create(c).Error; err != nil {
		return fmt.Errorf("handlerstore.SaveCall: %w", err)
	}
	return nil
}

// GetCallByID returns one call by id (scoped to caller).
//
// GetCallByID 按 id 取 call（按调用者过滤）。
func (s *Store) GetCallByID(ctx context.Context, id string) (*handlerdomain.Call, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, fmt.Errorf("handlerstore.GetCallByID: %w", err)
	}
	var row handlerdomain.Call
	err = s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, uid).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, handlerdomain.ErrCallNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("handlerstore.GetCallByID: %w", err)
	}
	return &row, nil
}

// ListCalls returns cursor-paginated calls newest-first matching filter.
//
// ListCalls 返按 filter 过滤的分页（新→旧）。
func (s *Store) ListCalls(ctx context.Context, filter handlerdomain.CallFilter) ([]*handlerdomain.Call, string, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("handlerstore.ListCalls: %w", err)
	}
	q := s.applyCallFilter(s.db.WithContext(ctx).Model(&handlerdomain.Call{}), uid, filter)

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
			return nil, "", fmt.Errorf("handlerstore.ListCalls: cursor: %w", err)
		}
		q = q.Where("(started_at, id) < (?, ?)", cur.CreatedAt, cur.ID)
	}

	var rows []*handlerdomain.Call
	if err := q.Order("started_at DESC, id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("handlerstore.ListCalls: %w", err)
	}

	var nextCursor string
	if len(rows) > limit {
		last := rows[limit-1]
		cur, encErr := paginationpkg.EncodeCursor(paginationpkg.Cursor{
			CreatedAt: last.StartedAt,
			ID:        last.ID,
		})
		if encErr != nil {
			return nil, "", fmt.Errorf("handlerstore.ListCalls: encode cursor: %w", encErr)
		}
		nextCursor = cur
		rows = rows[:limit]
	}
	return rows, nextCursor, nil
}

// ComputeCallAggregates returns rollup counts + p95 elapsed.
//
// ComputeCallAggregates 返聚合 count + p95。
func (s *Store) ComputeCallAggregates(ctx context.Context, filter handlerdomain.CallFilter) (handlerdomain.CallAggregates, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return handlerdomain.CallAggregates{}, fmt.Errorf("handlerstore.ComputeCallAggregates: %w", err)
	}

	type countsRow struct {
		OK        int
		Failed    int
		Cancelled int
		Timeout   int
		AvgMs     float64
	}
	var counts countsRow
	q := s.applyCallFilter(s.db.WithContext(ctx).Model(&handlerdomain.Call{}), uid, filter)
	if err := q.Select(
		`SUM(CASE WHEN status = 'ok'        THEN 1 ELSE 0 END) AS ok,
		 SUM(CASE WHEN status = 'failed'    THEN 1 ELSE 0 END) AS failed,
		 SUM(CASE WHEN status = 'cancelled' THEN 1 ELSE 0 END) AS cancelled,
		 SUM(CASE WHEN status = 'timeout'   THEN 1 ELSE 0 END) AS timeout,
		 COALESCE(AVG(elapsed_ms), 0)                          AS avg_ms`,
	).Scan(&counts).Error; err != nil {
		return handlerdomain.CallAggregates{}, fmt.Errorf("handlerstore.ComputeCallAggregates: %w", err)
	}

	var elapsedMsList []int64
	q2 := s.applyCallFilter(s.db.WithContext(ctx).Model(&handlerdomain.Call{}), uid, filter)
	if err := q2.Order("elapsed_ms ASC").Limit(1000).Pluck("elapsed_ms", &elapsedMsList).Error; err != nil {
		return handlerdomain.CallAggregates{}, fmt.Errorf("handlerstore.ComputeCallAggregates: p95: %w", err)
	}

	agg := handlerdomain.CallAggregates{
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

func (s *Store) applyCallFilter(q *gorm.DB, uid string, filter handlerdomain.CallFilter) *gorm.DB {
	q = q.Where("user_id = ?", uid)
	if filter.HandlerID != "" {
		q = q.Where("handler_id = ?", filter.HandlerID)
	}
	if filter.VersionID != "" {
		q = q.Where("version_id = ?", filter.VersionID)
	}
	if filter.Method != "" {
		q = q.Where("method = ?", filter.Method)
	}
	if filter.InstanceID != "" {
		q = q.Where("instance_id = ?", filter.InstanceID)
	}
	if filter.OwnerKind != "" {
		q = q.Where("owner_kind = ?", filter.OwnerKind)
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

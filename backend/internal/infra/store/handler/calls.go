package handler

import (
	"context"
	"errors"
	"fmt"
	"slices"

	handlerdomain "github.com/sunweilin/anselm/backend/internal/domain/handler"
	ormpkg "github.com/sunweilin/anselm/backend/internal/pkg/orm"
)

func (s *Store) SaveCall(ctx context.Context, c *handlerdomain.Call) error {
	if err := s.calls.Create(ctx, c); err != nil {
		return fmt.Errorf("handlerstore.SaveCall: %w", err)
	}
	return nil
}

func (s *Store) GetCallByID(ctx context.Context, id string) (*handlerdomain.Call, error) {
	c, err := s.calls.Get(ctx, id)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, handlerdomain.ErrCallNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("handlerstore.GetCallByID: %w", err)
	}
	return c, nil
}

func (s *Store) ListCalls(ctx context.Context, filter handlerdomain.CallFilter) ([]*handlerdomain.Call, string, error) {
	// Reject an out-of-enum status loudly (422) instead of silently matching zero rows (F168-M2).
	// 非枚举状态大声拒（422），而非静默匹配 0 行（F168-M2）。
	if filter.Status != "" && !slices.Contains(handlerdomain.CallStatuses, filter.Status) {
		return nil, "", handlerdomain.ErrInvalidCallStatus.WithDetails(map[string]any{"allowed": handlerdomain.CallStatuses, "got": filter.Status})
	}
	rows, next, err := s.callFilterQuery(filter, true).Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("handlerstore.ListCalls: %w", err)
	}
	// Lists travel light: logs ride only the single-record Get (see functionstore).
	// 列表轻装：logs 只随单条 Get（同 functionstore）。
	for _, c := range rows {
		c.Logs = ""
	}
	return rows, next, nil
}

// ComputeCallAggregates returns the ok / not-ok split over the filter (status ignored
// for the rollup).
//
// ComputeCallAggregates 返过滤集的 ok / 非 ok 计数（汇总忽略 status）。
func (s *Store) ComputeCallAggregates(ctx context.Context, filter handlerdomain.CallFilter) (handlerdomain.CallAggregates, error) {
	total, err := s.callFilterQuery(filter, false).Count(ctx)
	if err != nil {
		return handlerdomain.CallAggregates{}, fmt.Errorf("handlerstore.ComputeCallAggregates: total: %w", err)
	}
	ok, err := s.callFilterQuery(filter, false).WhereEq("status", handlerdomain.CallStatusOK).Count(ctx)
	if err != nil {
		return handlerdomain.CallAggregates{}, fmt.Errorf("handlerstore.ComputeCallAggregates: ok: %w", err)
	}
	return handlerdomain.CallAggregates{OKCount: int(ok), FailedCount: int(total - ok)}, nil
}

func (s *Store) callFilterQuery(filter handlerdomain.CallFilter, includeStatus bool) *ormpkg.Query[handlerdomain.Call] {
	q := s.calls.Query()
	if filter.HandlerID != "" {
		q = q.WhereEq("handler_id", filter.HandlerID)
	}
	if filter.VersionID != "" {
		q = q.WhereEq("version_id", filter.VersionID)
	}
	if filter.Method != "" {
		q = q.WhereEq("method", filter.Method)
	}
	if includeStatus && filter.Status != "" {
		q = q.WhereEq("status", filter.Status)
	}
	if filter.TriggeredBy != "" {
		q = q.WhereEq("triggered_by", filter.TriggeredBy)
	}
	if filter.ConversationID != "" {
		q = q.WhereEq("conversation_id", filter.ConversationID)
	}
	if filter.FlowrunID != "" {
		q = q.WhereEq("flowrun_id", filter.FlowrunID)
	}
	return q
}

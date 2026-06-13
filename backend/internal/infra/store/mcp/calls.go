package mcp

import (
	"context"
	"errors"
	"fmt"

	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
)

func (s *Store) SaveCall(ctx context.Context, c *mcpdomain.Call) error {
	if err := s.calls.Create(ctx, c); err != nil {
		return fmt.Errorf("mcpstore.SaveCall: %w", err)
	}
	return nil
}

func (s *Store) GetCall(ctx context.Context, id string) (*mcpdomain.Call, error) {
	c, err := s.calls.Get(ctx, id)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, mcpdomain.ErrCallNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("mcpstore.GetCall: %w", err)
	}
	return c, nil
}

func (s *Store) ListCalls(ctx context.Context, filter mcpdomain.CallFilter) ([]*mcpdomain.Call, string, error) {
	rows, next, err := s.callFilterQuery(filter, true).Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("mcpstore.ListCalls: %w", err)
	}
	// Lists travel light: logs ride only the single-record Get (see functionstore).
	// 列表轻装：logs 只随单条 Get（同 functionstore）。
	for _, c := range rows {
		c.Logs = ""
	}
	return rows, next, nil
}

// ComputeCallAggregates returns the ok / not-ok split over the filter (status ignored for
// the rollup — the badge always shows both halves of the matched set; same as handler).
//
// ComputeCallAggregates 返过滤集的 ok / 非 ok 计数（汇总忽略 status——徽标总显示两半；同 handler）。
func (s *Store) ComputeCallAggregates(ctx context.Context, filter mcpdomain.CallFilter) (mcpdomain.CallAggregates, error) {
	total, err := s.callFilterQuery(filter, false).Count(ctx)
	if err != nil {
		return mcpdomain.CallAggregates{}, fmt.Errorf("mcpstore.ComputeCallAggregates: total: %w", err)
	}
	ok, err := s.callFilterQuery(filter, false).WhereEq("status", mcpdomain.CallStatusOK).Count(ctx)
	if err != nil {
		return mcpdomain.CallAggregates{}, fmt.Errorf("mcpstore.ComputeCallAggregates: ok: %w", err)
	}
	return mcpdomain.CallAggregates{OKCount: int(ok), FailedCount: int(total - ok)}, nil
}

// callFilterQuery builds the shared WHERE set; includeStatus=false drops the status
// predicate (the rollup ignores it).
//
// callFilterQuery 构建共享 WHERE 集；includeStatus=false 去掉 status 谓词（汇总忽略它）。
func (s *Store) callFilterQuery(filter mcpdomain.CallFilter, includeStatus bool) *ormpkg.Query[mcpdomain.Call] {
	q := s.calls.Query()
	if filter.ServerID != "" {
		q = q.WhereEq("server_id", filter.ServerID)
	}
	if filter.Tool != "" {
		q = q.WhereEq("tool", filter.Tool)
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

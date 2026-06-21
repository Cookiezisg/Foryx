package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"

	agentdomain "github.com/sunweilin/anselm/backend/internal/domain/agent"
	ormpkg "github.com/sunweilin/anselm/backend/internal/pkg/orm"
)

func (s *Store) SaveExecution(ctx context.Context, e *agentdomain.Execution) error {
	if err := s.execs.Create(ctx, e); err != nil {
		return fmt.Errorf("agentstore.SaveExecution: %w", err)
	}
	return nil
}

func (s *Store) GetExecutionByID(ctx context.Context, id string) (*agentdomain.Execution, error) {
	e, err := s.execs.Get(ctx, id)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, agentdomain.ErrExecutionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("agentstore.GetExecutionByID: %w", err)
	}
	return e, nil
}

func (s *Store) ListExecutions(ctx context.Context, filter agentdomain.ExecutionFilter) ([]*agentdomain.Execution, string, error) {
	// Reject an out-of-enum status loudly (422) instead of silently matching zero rows (F168-M2).
	// 非枚举状态大声拒（422），而非静默匹配 0 行（F168-M2）。
	if filter.Status != "" && !slices.Contains(agentdomain.ExecutionStatuses, filter.Status) {
		return nil, "", agentdomain.ErrInvalidExecutionStatus.WithDetails(map[string]any{"allowed": agentdomain.ExecutionStatuses, "got": filter.Status})
	}
	rows, next, err := s.execFilterQuery(filter, true).Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("agentstore.ListExecutions: %w", err)
	}
	return rows, next, nil
}

// ComputeExecutionAggregates returns the ok / not-ok split over the filter (status filter is ignored for
// the rollup — the badge always shows both halves of the matched set).
//
// ComputeExecutionAggregates 返过滤集的 ok / 非 ok 计数（汇总忽略 status 过滤——徽标总显示匹配集两半）。
func (s *Store) ComputeExecutionAggregates(ctx context.Context, filter agentdomain.ExecutionFilter) (agentdomain.ExecutionAggregates, error) {
	total, err := s.execFilterQuery(filter, false).Count(ctx)
	if err != nil {
		return agentdomain.ExecutionAggregates{}, fmt.Errorf("agentstore.ComputeExecutionAggregates: total: %w", err)
	}
	ok, err := s.execFilterQuery(filter, false).WhereEq("status", agentdomain.ExecutionStatusOK).Count(ctx)
	if err != nil {
		return agentdomain.ExecutionAggregates{}, fmt.Errorf("agentstore.ComputeExecutionAggregates: ok: %w", err)
	}
	return agentdomain.ExecutionAggregates{
		OKCount:     int(ok),
		FailedCount: int(total - ok),
	}, nil
}

// execFilterQuery builds a fresh query applying the filter predicates. includeStatus controls
// whether the status predicate is applied (aggregates omit it to compute the ok-vs-rest rollup
// over the whole matched set).
//
// execFilterQuery 构造套用过滤谓词的新 query。includeStatus 控制是否套 status 谓词（聚合不套）。
func (s *Store) execFilterQuery(filter agentdomain.ExecutionFilter, includeStatus bool) *ormpkg.Query[agentdomain.Execution] {
	q := s.execs.Query()
	if filter.AgentID != "" {
		q = q.WhereEq("agent_id", filter.AgentID)
	}
	if filter.VersionID != "" {
		q = q.WhereEq("version_id", filter.VersionID)
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

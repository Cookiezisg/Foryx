package function

import (
	"context"
	"errors"
	"fmt"
	"slices"

	functiondomain "github.com/sunweilin/anselm/backend/internal/domain/function"
	ormpkg "github.com/sunweilin/anselm/backend/internal/pkg/orm"
)

func (s *Store) SaveExecution(ctx context.Context, e *functiondomain.Execution) error {
	if err := s.execs.Create(ctx, e); err != nil {
		return fmt.Errorf("functionstore.SaveExecution: %w", err)
	}
	return nil
}

func (s *Store) GetExecutionByID(ctx context.Context, id string) (*functiondomain.Execution, error) {
	e, err := s.execs.Get(ctx, id)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, functiondomain.ErrExecutionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("functionstore.GetExecutionByID: %w", err)
	}
	return e, nil
}

func (s *Store) ListExecutions(ctx context.Context, filter functiondomain.ExecutionFilter) ([]*functiondomain.Execution, string, error) {
	// Reject an out-of-enum status loudly (422) instead of silently matching zero rows (F168-M2).
	// 非枚举状态大声拒（422），而非静默匹配 0 行（F168-M2）。
	if filter.Status != "" && !slices.Contains(functiondomain.ExecutionStatuses, filter.Status) {
		return nil, "", functiondomain.ErrInvalidExecutionStatus.WithDetails(map[string]any{"allowed": functiondomain.ExecutionStatuses, "got": filter.Status})
	}
	q := s.execFilterQuery(filter, true)
	rows, next, err := q.Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("functionstore.ListExecutions: %w", err)
	}
	// Lists travel light: logs (up to 64KiB/row) ride only the single-record Get —
	// a 50-row page carrying them would blow the LLM tool-result cap mid-JSON.
	// 列表轻装：logs（每行至多 64KiB）只随单条 Get——50 行的页带上它会把 LLM
	// tool-result 上限撑爆在 JSON 半中腰。
	for _, e := range rows {
		e.Logs = ""
	}
	return rows, next, nil
}

// ComputeExecutionAggregates returns the ok / not-ok split over the filter (status filter is
// ignored for the rollup — the badge always shows both halves of the matched set).
//
// ComputeExecutionAggregates 返过滤集的 ok / 非 ok 计数（汇总忽略 status 过滤——徽标总显示匹配集两半）。
func (s *Store) ComputeExecutionAggregates(ctx context.Context, filter functiondomain.ExecutionFilter) (functiondomain.ExecutionAggregates, error) {
	total, err := s.execFilterQuery(filter, false).Count(ctx)
	if err != nil {
		return functiondomain.ExecutionAggregates{}, fmt.Errorf("functionstore.ComputeExecutionAggregates: total: %w", err)
	}
	ok, err := s.execFilterQuery(filter, false).WhereEq("status", functiondomain.ExecutionStatusOK).Count(ctx)
	if err != nil {
		return functiondomain.ExecutionAggregates{}, fmt.Errorf("functionstore.ComputeExecutionAggregates: ok: %w", err)
	}
	return functiondomain.ExecutionAggregates{
		OKCount:     int(ok),
		FailedCount: int(total - ok),
	}, nil
}

// execFilterQuery builds a fresh query applying the filter predicates. includeStatus
// controls whether the status predicate is applied (aggregates omit it to compute the
// ok-vs-rest rollup over the whole matched set).
//
// execFilterQuery 构造一个套用过滤谓词的新 query。includeStatus 控制是否套 status 谓词
// （聚合不套，以在整个匹配集上算 ok-vs-其余）。
func (s *Store) execFilterQuery(filter functiondomain.ExecutionFilter, includeStatus bool) *ormpkg.Query[functiondomain.Execution] {
	q := s.execs.Query()
	if filter.FunctionID != "" {
		q = q.WhereEq("function_id", filter.FunctionID)
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

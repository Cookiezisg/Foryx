package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	agentdomain "github.com/sunweilin/forgify/backend/internal/domain/agent"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
)

func (s *Store) SaveExecution(ctx context.Context, e *agentdomain.Execution) error {
	if err := s.execs.Create(ctx, e); err != nil {
		return fmt.Errorf("agentstore.SaveExecution: %w", err)
	}
	return nil
}

// UpdateExecution rewrites a parked run's terminal fields in place when it resumes (R0064) —
// status / output / transcript / error / elapsed / ended_at by id (partial Updates, auto workspace
// filter in the WHERE). Never an append: the transcript column is overwritten with the full
// resumed sequence. output is JSON-marshalled (the `,json` column); a nil-result row stays 'null'.
//
// UpdateExecution 在 parked 运行恢复时原地重写终态字段（R0064）——按 id 更新 status / output / transcript /
// error / elapsed / ended_at（部分 Updates，WHERE 带自动 workspace 过滤）。非追加：transcript 列被恢复后的完整
// 序列覆盖。output 走 JSON marshal（`,json` 列）；nil 结果保持 'null'。
func (s *Store) UpdateExecution(ctx context.Context, e *agentdomain.Execution) error {
	output, err := json.Marshal(e.Output)
	if err != nil {
		return fmt.Errorf("agentstore.UpdateExecution: marshal output: %w", err)
	}
	transcript := e.Transcript
	if len(transcript) == 0 {
		transcript = json.RawMessage("[]")
	}
	n, err := s.execs.WhereEq("id", e.ID).Updates(ctx, map[string]any{
		"status":        e.Status,
		"output":        string(output),
		"transcript":    string(transcript),
		"error_message": e.ErrorMessage,
		"elapsed_ms":    e.ElapsedMs,
		"ended_at":      e.EndedAt,
	})
	if err != nil {
		return fmt.Errorf("agentstore.UpdateExecution: %w", err)
	}
	if n == 0 {
		return agentdomain.ErrExecutionNotFound
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
	rows, next, err := s.execFilterQuery(filter, true).Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("agentstore.ListExecutions: %w", err)
	}
	return rows, next, nil
}

// ComputeAggregates returns the ok / not-ok split over the filter (status filter is ignored for
// the rollup — the badge always shows both halves of the matched set).
//
// ComputeAggregates 返过滤集的 ok / 非 ok 计数（汇总忽略 status 过滤——徽标总显示匹配集两半）。
func (s *Store) ComputeAggregates(ctx context.Context, filter agentdomain.ExecutionFilter) (agentdomain.ExecutionAggregates, error) {
	total, err := s.execFilterQuery(filter, false).Count(ctx)
	if err != nil {
		return agentdomain.ExecutionAggregates{}, fmt.Errorf("agentstore.ComputeAggregates: total: %w", err)
	}
	ok, err := s.execFilterQuery(filter, false).WhereEq("status", agentdomain.ExecutionStatusOK).Count(ctx)
	if err != nil {
		return agentdomain.ExecutionAggregates{}, fmt.Errorf("agentstore.ComputeAggregates: ok: %w", err)
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

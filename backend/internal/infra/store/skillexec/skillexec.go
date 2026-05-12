// Package skillexec (infra/store/skillexec) is the GORM-backed
// implementation of skilldomain.ExecutionRepository. Separate package
// from skill's existing infra (filesystem-backed skills/ scanner).
//
// Importers alias as `skillexecstore`.
//
// Package skillexec(infra/store/skillexec)是 skill_executions 表的 GORM
// 实现;跟 skill 现有 fs 扫描 infra 分包。
package skillexec

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"gorm.io/gorm"

	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Store is the GORM implementation of skilldomain.ExecutionRepository.
//
// Store 是 skilldomain.ExecutionRepository 的 GORM 实现。
type Store struct{ db *gorm.DB }

// New constructs a Store.
//
// New 构造 Store。
func New(db *gorm.DB) *Store { return &Store{db: db} }

// Compile-time interface assertion.
//
// 编译期断言。
var _ skilldomain.ExecutionRepository = (*Store)(nil)

// AutoMigrateModels returns the GORM models to register.
//
// AutoMigrateModels 返 AutoMigrate model。
func AutoMigrateModels() []interface{} {
	return []interface{}{&skilldomain.Execution{}}
}

func (s *Store) SaveExecution(ctx context.Context, e *skilldomain.Execution) error {
	if err := s.db.WithContext(ctx).Create(e).Error; err != nil {
		return fmt.Errorf("skillexecstore.SaveExecution: %w", err)
	}
	return nil
}

func (s *Store) GetExecutionByID(ctx context.Context, id string) (*skilldomain.Execution, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, fmt.Errorf("skillexecstore.GetExecutionByID: %w", err)
	}
	var row skilldomain.Execution
	res := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, uid).First(&row)
	if errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return nil, skilldomain.ErrExecutionNotFound
	}
	if res.Error != nil {
		return nil, fmt.Errorf("skillexecstore.GetExecutionByID: %w", res.Error)
	}
	return &row, nil
}

func (s *Store) ListExecutions(ctx context.Context, filter skilldomain.ExecutionFilter) ([]*skilldomain.Execution, string, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("skillexecstore.ListExecutions: %w", err)
	}
	q := s.applyFilter(s.db.WithContext(ctx).Model(&skilldomain.Execution{}), uid, filter)

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
			return nil, "", fmt.Errorf("skillexecstore.ListExecutions: cursor: %w", err)
		}
		q = q.Where("(started_at, id) < (?, ?)", cur.CreatedAt, cur.ID)
	}
	var rows []*skilldomain.Execution
	if err := q.Order("started_at DESC, id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("skillexecstore.ListExecutions: %w", err)
	}
	next := ""
	if len(rows) > limit {
		last := rows[limit-1]
		cur, encErr := paginationpkg.EncodeCursor(paginationpkg.Cursor{
			CreatedAt: last.StartedAt, ID: last.ID,
		})
		if encErr != nil {
			return nil, "", fmt.Errorf("skillexecstore.ListExecutions: encode cursor: %w", encErr)
		}
		next = cur
		rows = rows[:limit]
	}
	return rows, next, nil
}

func (s *Store) ComputeAggregates(ctx context.Context, filter skilldomain.ExecutionFilter) (skilldomain.ExecutionAggregates, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return skilldomain.ExecutionAggregates{}, fmt.Errorf("skillexecstore.ComputeAggregates: %w", err)
	}
	type countsRow struct {
		OK, Failed, Cancelled, Timeout int
		AvgMs                          float64
	}
	var counts countsRow
	q := s.applyFilter(s.db.WithContext(ctx).Model(&skilldomain.Execution{}), uid, filter)
	if err := q.Select(
		`SUM(CASE WHEN status='ok'        THEN 1 ELSE 0 END) AS ok,
		 SUM(CASE WHEN status='failed'    THEN 1 ELSE 0 END) AS failed,
		 SUM(CASE WHEN status='cancelled' THEN 1 ELSE 0 END) AS cancelled,
		 SUM(CASE WHEN status='timeout'   THEN 1 ELSE 0 END) AS timeout,
		 COALESCE(AVG(elapsed_ms), 0)                       AS avg_ms`,
	).Scan(&counts).Error; err != nil {
		return skilldomain.ExecutionAggregates{}, fmt.Errorf("skillexecstore.ComputeAggregates: %w", err)
	}
	var msList []int64
	q2 := s.applyFilter(s.db.WithContext(ctx).Model(&skilldomain.Execution{}), uid, filter)
	if err := q2.Order("elapsed_ms ASC").Limit(1000).Pluck("elapsed_ms", &msList).Error; err != nil {
		return skilldomain.ExecutionAggregates{}, fmt.Errorf("skillexecstore.ComputeAggregates: p95: %w", err)
	}
	agg := skilldomain.ExecutionAggregates{
		OKCount: counts.OK, FailedCount: counts.Failed,
		CancelledCount: counts.Cancelled, TimeoutCount: counts.Timeout,
		AvgElapsedMs: int64(counts.AvgMs),
	}
	if len(msList) > 0 {
		sort.Slice(msList, func(i, j int) bool { return msList[i] < msList[j] })
		idx := (len(msList) * 95) / 100
		if idx >= len(msList) {
			idx = len(msList) - 1
		}
		agg.P95ElapsedMs = msList[idx]
	}
	return agg, nil
}

func (s *Store) applyFilter(q *gorm.DB, uid string, filter skilldomain.ExecutionFilter) *gorm.DB {
	q = q.Where("user_id = ?", uid)
	if filter.SkillName != "" {
		q = q.Where("skill_name = ?", filter.SkillName)
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
	if filter.ForkDepth != nil {
		q = q.Where("fork_depth = ?", *filter.ForkDepth)
	}
	return q
}

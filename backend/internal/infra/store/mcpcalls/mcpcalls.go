// Package mcpcalls (infra/store/mcpcalls) is the GORM-backed implementation
// of mcpdomain.CallRepository. Separate package from infra/store/mcp would
// be — but mcp's existing infra (configfile-backed mcp.json) isn't under
// infra/store. Putting D22 call log alone here is the cleanest split.
//
// Importers alias as `mcpcallstore`.
//
// Package mcpcalls(infra/store/mcpcalls)是 mcp_calls 表的 GORM 实现。
// MCP 现有 infra 走配置文件,call log 单独 GORM 表放此包。
package mcpcalls

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"gorm.io/gorm"

	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Store is the GORM implementation of mcpdomain.CallRepository.
//
// Store 是 mcpdomain.CallRepository 的 GORM 实现。
type Store struct{ db *gorm.DB }

// New constructs a Store.
//
// New 构造 Store。
func New(db *gorm.DB) *Store { return &Store{db: db} }

// Compile-time interface assertion.
//
// 编译期断言。
var _ mcpdomain.CallRepository = (*Store)(nil)

// AutoMigrateModels returns the GORM models to register.
//
// AutoMigrateModels 返回 AutoMigrate 用的 model。
func AutoMigrateModels() []interface{} {
	return []interface{}{&mcpdomain.Call{}}
}

// SaveCall inserts one Call row (terminal write).
//
// SaveCall 写一行 Call(终态)。
func (s *Store) SaveCall(ctx context.Context, c *mcpdomain.Call) error {
	if err := s.db.WithContext(ctx).Create(c).Error; err != nil {
		return fmt.Errorf("mcpcallstore.SaveCall: %w", err)
	}
	return nil
}

// GetCallByID returns one call, user-scoped. ErrCallNotFound on miss.
//
// GetCallByID 按 id 取,过滤当前用户;未命中返 ErrCallNotFound。
func (s *Store) GetCallByID(ctx context.Context, id string) (*mcpdomain.Call, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, fmt.Errorf("mcpcallstore.GetCallByID: %w", err)
	}
	var row mcpdomain.Call
	res := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, uid).First(&row)
	if errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return nil, mcpdomain.ErrCallNotFound
	}
	if res.Error != nil {
		return nil, fmt.Errorf("mcpcallstore.GetCallByID: %w", res.Error)
	}
	return &row, nil
}

// ListCalls returns cursor-paginated calls newest-first.
//
// ListCalls 返 filter 过滤的 cursor 分页(新→旧)。
func (s *Store) ListCalls(ctx context.Context, filter mcpdomain.CallFilter) ([]*mcpdomain.Call, string, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("mcpcallstore.ListCalls: %w", err)
	}
	q := s.applyFilter(s.db.WithContext(ctx).Model(&mcpdomain.Call{}), uid, filter)

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
			return nil, "", fmt.Errorf("mcpcallstore.ListCalls: cursor: %w", err)
		}
		q = q.Where("(started_at, id) < (?, ?)", cur.CreatedAt, cur.ID)
	}
	var rows []*mcpdomain.Call
	if err := q.Order("started_at DESC, id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("mcpcallstore.ListCalls: %w", err)
	}
	next := ""
	if len(rows) > limit {
		last := rows[limit-1]
		cur, encErr := paginationpkg.EncodeCursor(paginationpkg.Cursor{
			CreatedAt: last.StartedAt, ID: last.ID,
		})
		if encErr != nil {
			return nil, "", fmt.Errorf("mcpcallstore.ListCalls: encode cursor: %w", encErr)
		}
		next = cur
		rows = rows[:limit]
	}
	return rows, next, nil
}

// ComputeAggregates returns the standard 4-status counts + avg/p95
// elapsed for the filter.
//
// ComputeAggregates 返 4 状态 count + avg/p95 elapsed。
func (s *Store) ComputeAggregates(ctx context.Context, filter mcpdomain.CallFilter) (mcpdomain.CallAggregates, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return mcpdomain.CallAggregates{}, fmt.Errorf("mcpcallstore.ComputeAggregates: %w", err)
	}
	type countsRow struct {
		OK, Failed, Cancelled, Timeout int
		AvgMs                          float64
	}
	var counts countsRow
	q := s.applyFilter(s.db.WithContext(ctx).Model(&mcpdomain.Call{}), uid, filter)
	if err := q.Select(
		`SUM(CASE WHEN status='ok'        THEN 1 ELSE 0 END) AS ok,
		 SUM(CASE WHEN status='failed'    THEN 1 ELSE 0 END) AS failed,
		 SUM(CASE WHEN status='cancelled' THEN 1 ELSE 0 END) AS cancelled,
		 SUM(CASE WHEN status='timeout'   THEN 1 ELSE 0 END) AS timeout,
		 COALESCE(AVG(elapsed_ms), 0)                       AS avg_ms`,
	).Scan(&counts).Error; err != nil {
		return mcpdomain.CallAggregates{}, fmt.Errorf("mcpcallstore.ComputeAggregates: %w", err)
	}

	var msList []int64
	q2 := s.applyFilter(s.db.WithContext(ctx).Model(&mcpdomain.Call{}), uid, filter)
	if err := q2.Order("elapsed_ms ASC").Limit(1000).Pluck("elapsed_ms", &msList).Error; err != nil {
		return mcpdomain.CallAggregates{}, fmt.Errorf("mcpcallstore.ComputeAggregates: p95: %w", err)
	}
	agg := mcpdomain.CallAggregates{
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

func (s *Store) applyFilter(q *gorm.DB, uid string, filter mcpdomain.CallFilter) *gorm.DB {
	q = q.Where("user_id = ?", uid)
	if filter.ServerName != "" {
		q = q.Where("server_name = ?", filter.ServerName)
	}
	if filter.ToolName != "" {
		q = q.Where("tool_name = ?", filter.ToolName)
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
	return q
}

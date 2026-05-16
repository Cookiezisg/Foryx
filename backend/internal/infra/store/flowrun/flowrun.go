// Package flowrun is the GORM-backed flowrundomain.Repository, scoped by ctx userID.
//
// Package flowrun 是 flowrundomain.Repository 的 GORM 实现，按 ctx userID 过滤。
package flowrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Store is the GORM implementation of flowrundomain.Repository.
//
// Store 是 flowrundomain.Repository 的 GORM 实现。
type Store struct {
	db *gorm.DB
}

// New constructs a Store bound to the given *gorm.DB.
//
// New 基于给定 *gorm.DB 构造 Store。
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

var _ flowrundomain.Repository = (*Store)(nil)

// AutoMigrateModels returns the GORM models to register in db.AutoMigrate.
//
// AutoMigrateModels 返回 AutoMigrate 用的 GORM models。
func AutoMigrateModels() []interface{} {
	return []interface{}{
		&flowrundomain.FlowRun{},
		&flowrundomain.Node{},
	}
}

// Create persists a fresh FlowRun row.
//
// Create 写新 FlowRun 行。
func (s *Store) Create(ctx context.Context, run *flowrundomain.FlowRun) error {
	if err := s.db.WithContext(ctx).Create(run).Error; err != nil {
		return fmt.Errorf("flowrunstore.Create: %w", err)
	}
	return nil
}

// Get fetches by id, scoped to caller.
//
// Get 按 id 查，按调用者过滤。
func (s *Store) Get(ctx context.Context, id string) (*flowrundomain.FlowRun, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var run flowrundomain.FlowRun
	res := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, uid).First(&run)
	if errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return nil, flowrundomain.ErrNotFound
	}
	if res.Error != nil {
		return nil, fmt.Errorf("flowrunstore.Get: %w", res.Error)
	}
	return &run, nil
}

// List paginates by filter; order started_at DESC, id DESC.
//
// List 按 filter 分页；started_at DESC + id DESC 排序。
func (s *Store) List(ctx context.Context, filter flowrundomain.ListFilter) ([]*flowrundomain.FlowRun, string, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, "", err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	tx := s.db.WithContext(ctx).Where("user_id = ?", uid)
	if filter.WorkflowID != "" {
		tx = tx.Where("workflow_id = ?", filter.WorkflowID)
	}
	if filter.Status != "" {
		tx = tx.Where("status = ?", filter.Status)
	}
	if filter.TriggerKind != "" {
		tx = tx.Where("trigger_kind = ?", filter.TriggerKind)
	}
	if filter.Cursor != "" {
		var c paginationpkg.Cursor
		if err := paginationpkg.DecodeCursor(filter.Cursor, &c); err != nil {
			return nil, "", fmt.Errorf("flowrunstore.List: %w", err)
		}
		tx = tx.Where("(started_at, id) < (?, ?)", c.CreatedAt, c.ID)
	}
	var rows []flowrundomain.FlowRun
	if err := tx.Order("started_at DESC, id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("flowrunstore.List: %w", err)
	}
	next := ""
	if len(rows) > limit {
		last := rows[limit-1]
		var encErr error
		next, encErr = paginationpkg.EncodeCursor(paginationpkg.Cursor{CreatedAt: last.StartedAt, ID: last.ID})
		if encErr != nil {
			return nil, "", fmt.Errorf("flowrunstore.List: %w", encErr)
		}
		rows = rows[:limit]
	}
	out := make([]*flowrundomain.FlowRun, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, next, nil
}

// UpdateStatus transitions status atomically; writes ended_at/elapsed_ms/output/error on terminal.
//
// UpdateStatus 原子状态机转换；转终态时同时写 ended_at/elapsed_ms/output/error。
func (s *Store) UpdateStatus(ctx context.Context, runID, status string, output any, errCode, errMsg string, endedAt *time.Time, elapsedMs int64) error {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return err
	}
	updates := map[string]any{"status": status}
	if endedAt != nil {
		updates["ended_at"] = *endedAt
	}
	if elapsedMs > 0 {
		updates["elapsed_ms"] = elapsedMs
	}
	if output != nil {
		// GORM Updates(map) bypasses serializer:json — marshal manually so the column gets a string.
		// GORM Updates(map) 不走 serializer:json，手动 marshal 成 string。
		b, mErr := json.Marshal(output)
		if mErr != nil {
			return fmt.Errorf("flowrunstore.UpdateStatus: marshal output: %w", mErr)
		}
		updates["output"] = string(b)
	}
	if errCode != "" {
		updates["error_code"] = errCode
	}
	if errMsg != "" {
		updates["error_message"] = errMsg
	}
	res := s.db.WithContext(ctx).Model(&flowrundomain.FlowRun{}).
		Where("id = ? AND user_id = ?", runID, uid).Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("flowrunstore.UpdateStatus: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return flowrundomain.ErrNotFound
	}
	return nil
}

// SetPausedState writes paused_state JSON blob (marshal manually since Update bypasses serializer).
//
// SetPausedState 写 paused_state JSON（手动 marshal，Update 不走 serializer）。
func (s *Store) SetPausedState(ctx context.Context, runID string, ps *flowrundomain.PausedState) error {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return err
	}
	b, mErr := json.Marshal(ps)
	if mErr != nil {
		return fmt.Errorf("flowrunstore.SetPausedState: marshal: %w", mErr)
	}
	res := s.db.WithContext(ctx).Model(&flowrundomain.FlowRun{}).
		Where("id = ? AND user_id = ?", runID, uid).
		Update("paused_state", string(b))
	if res.Error != nil {
		return fmt.Errorf("flowrunstore.SetPausedState: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return flowrundomain.ErrNotFound
	}
	return nil
}

// ClearPausedState nullifies the paused_state column.
//
// ClearPausedState 清 paused_state 列。
func (s *Store) ClearPausedState(ctx context.Context, runID string) error {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return err
	}
	res := s.db.WithContext(ctx).Model(&flowrundomain.FlowRun{}).
		Where("id = ? AND user_id = ?", runID, uid).
		Update("paused_state", gorm.Expr("NULL"))
	if res.Error != nil {
		return fmt.Errorf("flowrunstore.ClearPausedState: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return flowrundomain.ErrNotFound
	}
	return nil
}

// ListPaused returns all paused FlowRuns for current user (boot rehydrate entry point).
//
// ListPaused 返当前用户所有 paused FlowRun（boot rehydrate 入口）。
func (s *Store) ListPaused(ctx context.Context) ([]*flowrundomain.FlowRun, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var rows []flowrundomain.FlowRun
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND status = ?", uid, flowrundomain.StatusPaused).
		Order("started_at ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("flowrunstore.ListPaused: %w", err)
	}
	out := make([]*flowrundomain.FlowRun, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, nil
}

// CountRunning returns the running FlowRun count for a workflow (serial concurrency check).
//
// CountRunning 返 workflow 的 running FlowRun 数（serial 并发检查用）。
func (s *Store) CountRunning(ctx context.Context, workflowID string) (int, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return 0, err
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&flowrundomain.FlowRun{}).
		Where("user_id = ? AND workflow_id = ? AND status = ?",
			uid, workflowID, flowrundomain.StatusRunning).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("flowrunstore.CountRunning: %w", err)
	}
	return int(count), nil
}

// HardDeleteOldest keeps `keep` newest FlowRuns per workflow and physically deletes the rest.
//
// HardDeleteOldest 保留 keep 个最新，物理删其余。
func (s *Store) HardDeleteOldest(ctx context.Context, workflowID string, keep int) error {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if keep <= 0 {
		keep = flowrundomain.DefaultRetentionLimit
	}
	var ids []string
	if err := s.db.WithContext(ctx).
		Model(&flowrundomain.FlowRun{}).
		Where("user_id = ? AND workflow_id = ?", uid, workflowID).
		Order("created_at DESC, id DESC").
		Offset(keep).
		Pluck("id", &ids).Error; err != nil {
		return fmt.Errorf("flowrunstore.HardDeleteOldest: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}
	if err := s.db.WithContext(ctx).
		Where("id IN ?", ids).
		Delete(&flowrundomain.FlowRun{}).Error; err != nil {
		return fmt.Errorf("flowrunstore.HardDeleteOldest: %w", err)
	}
	return nil
}

// CreateNode persists a Node row (terminal write).
//
// CreateNode 写 Node 行（终态写）。
func (s *Store) CreateNode(ctx context.Context, node *flowrundomain.Node) error {
	if err := s.db.WithContext(ctx).Create(node).Error; err != nil {
		return fmt.Errorf("flowrunstore.CreateNode: %w", err)
	}
	return nil
}

// GetNode fetches a Node by id.
//
// GetNode 按 id 取 Node。
func (s *Store) GetNode(ctx context.Context, id string) (*flowrundomain.Node, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var node flowrundomain.Node
	res := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, uid).First(&node)
	if errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return nil, flowrundomain.ErrNodeNotFound
	}
	if res.Error != nil {
		return nil, fmt.Errorf("flowrunstore.GetNode: %w", res.Error)
	}
	return &node, nil
}

// ListNodes paginates Nodes by filter; order started_at ASC (chronological).
//
// ListNodes 按 filter 分页；started_at ASC 排序（时间顺序）。
func (s *Store) ListNodes(ctx context.Context, filter flowrundomain.NodeFilter) ([]*flowrundomain.Node, string, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, "", err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	tx := s.db.WithContext(ctx).Where("user_id = ?", uid)
	if filter.FlowrunID != "" {
		tx = tx.Where("flowrun_id = ?", filter.FlowrunID)
	}
	if filter.NodeType != "" {
		tx = tx.Where("node_type = ?", filter.NodeType)
	}
	if filter.Status != "" {
		tx = tx.Where("status = ?", filter.Status)
	}
	if filter.ConversationID != "" {
		tx = tx.Where("conversation_id = ?", filter.ConversationID)
	}
	if filter.Cursor != "" {
		var c paginationpkg.Cursor
		if err := paginationpkg.DecodeCursor(filter.Cursor, &c); err != nil {
			return nil, "", fmt.Errorf("flowrunstore.ListNodes: %w", err)
		}
		tx = tx.Where("(started_at, id) > (?, ?)", c.CreatedAt, c.ID)
	}
	var rows []flowrundomain.Node
	if err := tx.Order("started_at ASC, id ASC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("flowrunstore.ListNodes: %w", err)
	}
	next := ""
	if len(rows) > limit {
		last := rows[limit-1]
		var encErr error
		next, encErr = paginationpkg.EncodeCursor(paginationpkg.Cursor{CreatedAt: last.StartedAt, ID: last.ID})
		if encErr != nil {
			return nil, "", fmt.Errorf("flowrunstore.ListNodes: %w", encErr)
		}
		rows = rows[:limit]
	}
	out := make([]*flowrundomain.Node, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, next, nil
}

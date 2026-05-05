// Package subagent (infra/store/subagent) is the GORM implementation of
// the domain subagent Repository port. Two tables: subagent_runs (the
// per-spawn ledger) + subagent_messages (per-message rows linked by
// SubagentRunID + Seq composite index).
//
// AppendMessage assigns Seq inside a transaction (SELECT MAX(seq)+1)
// so concurrent appends within one run can't collide. UpdateMessage
// rewrites Blocks of an existing row by ID, preserving Seq for the
// streaming-refinement use case (sub-runner growing a single assistant
// message across multiple LLM events).
//
// Package subagent (infra/store/subagent) 是 domain subagent Repository
// port 的 GORM 实现。两表：subagent_runs（每次 spawn 总账）+
// subagent_messages（按 SubagentRunID + Seq 复合索引的消息行）。
//
// AppendMessage 在事务内 SELECT MAX(seq)+1 分配 Seq，单 run 并发 append
// 不撞号。UpdateMessage 按 ID 重写已有行的 Blocks，保留 Seq——sub-runner
// 把同一条 assistant 消息跨多个 LLM 事件流式增长。
package subagent

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	subagentdomain "github.com/sunweilin/forgify/backend/internal/domain/subagent"
)

// Store is the GORM implementation of subagentdomain.Repository.
//
// Store 是 subagentdomain.Repository 的 GORM 实现。
type Store struct {
	db *gorm.DB
}

// New constructs a Store bound to the given *gorm.DB.
//
// New 基于给定 *gorm.DB 构造 Store。
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// ── SubagentRun ──────────────────────────────────────────────────────

// CreateRun inserts a new subagent_runs row.
//
// CreateRun 插入新的 subagent_runs 行。
func (s *Store) CreateRun(ctx context.Context, r *subagentdomain.SubagentRun) error {
	if err := s.db.WithContext(ctx).Create(r).Error; err != nil {
		return fmt.Errorf("subagentstore.CreateRun: %w", err)
	}
	return nil
}

// GetRun fetches by id; returns ErrTypeNotFound-cousin sentinel via
// gorm.ErrRecordNotFound when absent.
//
// GetRun 按 id 取；不存在时返 gorm.ErrRecordNotFound（service 层映射
// 到具体业务 sentinel）。
func (s *Store) GetRun(ctx context.Context, id string) (*subagentdomain.SubagentRun, error) {
	var r subagentdomain.SubagentRun
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&r).Error
	if err != nil {
		// Surface the canonical gorm.ErrRecordNotFound unwrapped so callers
		// can errors.Is it; non-NotFound errors get a wrap.
		// 直接透出 gorm.ErrRecordNotFound 让调用方 errors.Is；其他错 wrap。
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("subagentstore.GetRun: %w", err)
	}
	return &r, nil
}

// UpdateRun writes a SubagentRun back. GORM Save updates all persistent
// columns (gorm:"-" transient fields are skipped automatically).
//
// UpdateRun 写回 SubagentRun。GORM Save 更新所有持久列（gorm:"-" 瞬时
// 字段自动跳过）。
func (s *Store) UpdateRun(ctx context.Context, r *subagentdomain.SubagentRun) error {
	if err := s.db.WithContext(ctx).Save(r).Error; err != nil {
		return fmt.Errorf("subagentstore.UpdateRun: %w", err)
	}
	return nil
}

// ListRunsByConversation returns runs spawned from the given conversation,
// newest first (UI history list ordering).
//
// ListRunsByConversation 返某对话发起的所有 run，按时间倒序（UI 历史列表
// 顺序）。
func (s *Store) ListRunsByConversation(ctx context.Context, conversationID string) ([]*subagentdomain.SubagentRun, error) {
	var rows []*subagentdomain.SubagentRun
	err := s.db.WithContext(ctx).
		Where("parent_conversation_id = ?", conversationID).
		Order("started_at DESC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("subagentstore.ListRunsByConversation: %w", err)
	}
	return rows, nil
}

// ── SubagentMessage ──────────────────────────────────────────────────

// AppendMessage assigns Seq atomically (SELECT COALESCE(MAX(seq),-1)+1
// inside a single transaction) before inserting, so concurrent appends
// within one run don't collide.
//
// AppendMessage 在事务内原子分配 Seq（SELECT COALESCE(MAX(seq),-1)+1）后
// 插入；单 run 并发 append 不撞号。
func (s *Store) AppendMessage(ctx context.Context, m *subagentdomain.SubagentMessage) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var nextSeq int
		// COALESCE handles the empty-table case (MAX over zero rows is NULL).
		// COALESCE 处理空表情况（零行的 MAX 是 NULL）。
		err := tx.Raw(
			`SELECT COALESCE(MAX(seq), -1) + 1 FROM subagent_messages WHERE subagent_run_id = ?`,
			m.SubagentRunID,
		).Scan(&nextSeq).Error
		if err != nil {
			return fmt.Errorf("subagentstore.AppendMessage: next seq: %w", err)
		}
		m.Seq = nextSeq
		if err := tx.Create(m).Error; err != nil {
			return fmt.Errorf("subagentstore.AppendMessage: insert: %w", err)
		}
		return nil
	})
}

// UpdateMessage rewrites an existing message row's Blocks (and other
// non-Seq fields) by ID. Used when sub-runner streams refinements into
// the same assistant message across LLM events.
//
// UpdateMessage 按 ID 重写已有消息行的 Blocks（及其他非 Seq 字段）。
// sub-runner 把同一条 assistant 消息跨多个 LLM 事件流式精化时调用。
func (s *Store) UpdateMessage(ctx context.Context, m *subagentdomain.SubagentMessage) error {
	if err := s.db.WithContext(ctx).Save(m).Error; err != nil {
		return fmt.Errorf("subagentstore.UpdateMessage: %w", err)
	}
	return nil
}

// ListMessagesByRun returns all messages in a run, ordered by Seq —
// the canonical replay order for UI history.
//
// ListMessagesByRun 返某 run 的全部消息，按 Seq 排序——UI 历史回放的规范
// 顺序。
func (s *Store) ListMessagesByRun(ctx context.Context, runID string) ([]*subagentdomain.SubagentMessage, error) {
	var rows []*subagentdomain.SubagentMessage
	err := s.db.WithContext(ctx).
		Where("subagent_run_id = ?", runID).
		Order("seq ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("subagentstore.ListMessagesByRun: %w", err)
	}
	return rows, nil
}

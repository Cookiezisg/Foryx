// Package trigger is the GORM-backed durable trigger layer: the firings inbox + schedules +
// polling cursors (17 §1, Theme 3). Persist-before-act: a firing is written before any flowrun
// starts, and claimed in a single transaction (ADR-021) so there is never a claimed-but-no-flowrun
// strand.
//
// Package trigger 是 durable 触发层 store:收件箱 + 调度 + polling 游标;先持久化再动作。
package trigger

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
)

// ErrFiringNotPending means the firing was already claimed/started/terminal — the claim lost the
// race (idempotent: the winner already created the flowrun).
var ErrFiringNotPending = errors.New("triggerstore: firing not pending")

type Store struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Store { return &Store{db: db} }

// AutoMigrateModels returns the durable-trigger models to register in db.AutoMigrate.
//
// AutoMigrateModels 返 AutoMigrate 用的 durable 触发 model。
func AutoMigrateModels() []interface{} {
	return []interface{}{
		&triggerdomain.TriggerSchedule{},
		&triggerdomain.TriggerFiring{},
		&triggerdomain.PollingState{},
	}
}

// AppendFiring writes a pending firing. UNIQUE(workflow_id, trigger_node_id, dedup_key) makes
// re-materialization (crash between append→bump, catchup re-compute) idempotent: a duplicate
// returns the existing firing (not-lost + not-duplicated, A-3).
//
// AppendFiring 写一条 pending firing;dedup_key UNIQUE 让重复材化幂等(不丢且不重)。
func (s *Store) AppendFiring(ctx context.Context, f *triggerdomain.TriggerFiring) (*triggerdomain.TriggerFiring, error) {
	if f.ID == "" {
		f.ID = idgenpkg.New("trf")
	}
	if f.Status == "" {
		f.Status = triggerdomain.FiringPending
	}
	if f.EnqueuedAt.IsZero() {
		f.EnqueuedAt = time.Now().UTC()
	}
	out := f
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(f).Error; err != nil {
			if isUniqueViolation(err) {
				var existing triggerdomain.TriggerFiring
				if gErr := tx.Where("workflow_id = ? AND trigger_node_id = ? AND dedup_key = ?",
					f.WorkflowID, f.TriggerNodeID, f.DedupKey).First(&existing).Error; gErr != nil {
					return gErr
				}
				out = &existing
				return nil
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("triggerstore.AppendFiring: %w", err)
	}
	return out, nil
}

// ListPending returns pending firings for the dispatcher to consume, oldest first.
//
// ListPending 返 pending firing 供派发器消费,按入队序。
func (s *Store) ListPending(ctx context.Context, limit int) ([]triggerdomain.TriggerFiring, error) {
	var fs []triggerdomain.TriggerFiring
	q := s.db.WithContext(ctx).Where("status = ?", triggerdomain.FiringPending).Order("enqueued_at asc")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&fs).Error; err != nil {
		return nil, fmt.Errorf("triggerstore.ListPending: %w", err)
	}
	return fs, nil
}

// ClaimFiring atomically claims a pending firing and creates its flowrun in ONE transaction
// (ADR-021): claim (pending→claimed, only if still pending) → create(tx) builds the flowrun →
// backfill flowrun_id + status=started. Crash before commit rolls back (firing stays pending);
// crash after commit leaves status=started + the flowrun (boot replays it). There is no
// claimed-but-no-flowrun intermediate state. Returns ErrFiringNotPending if the claim lost the race.
//
// ClaimFiring 单事务原子认领 + 建 flowrun(ADR-021):无 "claimed 但无 flowrun" 中间态。
func (s *Store) ClaimFiring(ctx context.Context, firingID string, create func(tx *gorm.DB) (flowrunID string, err error)) (string, error) {
	var flowrunID string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&triggerdomain.TriggerFiring{}).
			Where("id = ? AND status = ?", firingID, triggerdomain.FiringPending).
			Update("status", triggerdomain.FiringClaimed)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrFiringNotPending
		}
		fid, cErr := create(tx)
		if cErr != nil {
			return cErr
		}
		flowrunID = fid
		return tx.Model(&triggerdomain.TriggerFiring{}).Where("id = ?", firingID).
			Updates(map[string]any{"status": triggerdomain.FiringStarted, "flowrun_id": fid}).Error
	})
	if err != nil {
		if errors.Is(err, ErrFiringNotPending) {
			return "", err
		}
		return "", fmt.Errorf("triggerstore.ClaimFiring: %w", err)
	}
	return flowrunID, nil
}

// MarkOutcome sets a non-started terminal status on a firing (skipped/superseded/shed) — never
// silently dropped (every firing reaches a terminal status, 17 §6).
//
// MarkOutcome 给 firing 置非 started 终态(skipped/superseded/shed),绝不静默丢。
func (s *Store) MarkOutcome(ctx context.Context, firingID, status string) error {
	if err := s.db.WithContext(ctx).Model(&triggerdomain.TriggerFiring{}).
		Where("id = ?", firingID).Update("status", status).Error; err != nil {
		return fmt.Errorf("triggerstore.MarkOutcome: %w", err)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

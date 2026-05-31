// Package flowrunevent is the append-only journal store: record-once via a dedup_key partial
// unique index (ADR-018), per-flowrun monotonic seq allocated in the write tx (17 §2).
//
// Package flowrunevent 是 append-only journal store。
package flowrunevent

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
)

// Store is the GORM implementation of flowrundomain.JournalRepository.
//
// Store 是 flowrundomain.JournalRepository 的 GORM 实现。
type Store struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Store { return &Store{db: db} }

var _ flowrundomain.JournalRepository = (*Store)(nil)

// AutoMigrateModels returns the journal models to register in db.AutoMigrate.
//
// AutoMigrateModels 返 AutoMigrate 用的 journal model。
func AutoMigrateModels() []interface{} {
	return []interface{}{
		&flowrundomain.FlowRunEvent{},
	}
}

// AppendEvent allocates a per-flowrun seq inside the write tx and inserts. A record-once unique
// violation (dedup_key collision) means already-recorded → return the existing row unchanged
// (first-wins, ADR-018); the discarded seq leaves no gap because the failed insert rolls back.
// Attempt types carry dedup_key='' and are excluded from the partial index, so they never collide.
//
// AppendEvent 在写事务内分配 per-flowrun seq 并插入;撞 record-once 唯一键 = 已记账,返既有行(first-wins)。
func (s *Store) AppendEvent(ctx context.Context, e *flowrundomain.FlowRunEvent) (*flowrundomain.FlowRunEvent, error) {
	if e.ID == "" {
		e.ID = idgenpkg.New("fre")
	}
	e.DedupKey = e.ComputeDedupKey()

	out := e
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxSeq int64
		if err := tx.Model(&flowrundomain.FlowRunEvent{}).
			Where("flowrun_id = ?", e.FlowrunID).
			Select("COALESCE(MAX(seq), 0)").Scan(&maxSeq).Error; err != nil {
			return err
		}
		e.Seq = maxSeq + 1
		if err := tx.Create(e).Error; err != nil {
			if e.DedupKey != "" && isUniqueViolation(err) {
				var existing flowrundomain.FlowRunEvent
				if gErr := tx.Where("flowrun_id = ? AND dedup_key = ?", e.FlowrunID, e.DedupKey).
					First(&existing).Error; gErr != nil {
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
		return nil, fmt.Errorf("flowruneventstore.AppendEvent: %w", err)
	}
	return out, nil
}

// LoadJournal returns the flowrun's events in seq order (the replay input).
//
// LoadJournal 按 seq 序返该 flowrun 的事件(重放输入)。
func (s *Store) LoadJournal(ctx context.Context, flowrunID string) ([]flowrundomain.FlowRunEvent, error) {
	var evs []flowrundomain.FlowRunEvent
	if err := s.db.WithContext(ctx).
		Where("flowrun_id = ?", flowrunID).
		Order("seq asc").Find(&evs).Error; err != nil {
		return nil, fmt.Errorf("flowruneventstore.LoadJournal: %w", err)
	}
	return evs, nil
}

// isUniqueViolation matches modernc.org/sqlite's unique-constraint message.
//
// isUniqueViolation 匹配 modernc sqlite 的唯一约束报错。
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

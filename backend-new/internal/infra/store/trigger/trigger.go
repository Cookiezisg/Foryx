// Package trigger is the orm-backed triggerdomain.Repository: triggers (soft-deleted) +
// trigger_firings (durable inbox, dedup-unique per D3) + trigger_activations (append-only
// action log, no deleted_at per D1). Workspace isolation is automatic (orm ,ws tag).
//
// Package trigger 是 triggerdomain.Repository 的 orm 实现：triggers（软删）+ trigger_firings
// （durable 收件箱，dedup 唯一，D3）+ trigger_activations（只增动作日志，无 deleted_at，D1）。
// workspace 隔离自动（orm ,ws tag）。
package trigger

import (
	"context"
	"errors"
	"fmt"

	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
)

// Schema is the trigger tables' DDL (idempotent, ordered) for cmd/server to collect via
// db.Migrate. triggers carry a free-form config JSON; firings dedup on
// (workflow_id, trigger_id, dedup_key) (D3 idx_trf_dedup); activations are an append-only log.
//
// Schema 是 trigger 三表 DDL（幂等、按序）。triggers 带自由 config JSON；firings 按
// (workflow_id, trigger_id, dedup_key) 去重（D3）；activations 只增日志。
var Schema = []string{
	`CREATE TABLE IF NOT EXISTS triggers (
		id           TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name         TEXT NOT NULL,
		description  TEXT NOT NULL DEFAULT '',
		kind         TEXT NOT NULL CHECK (kind IN ('cron','webhook','fsnotify','sensor')),
		config       TEXT NOT NULL DEFAULT '{}',
		outputs      TEXT NOT NULL DEFAULT '[]',
		created_at   DATETIME NOT NULL,
		updated_at   DATETIME NOT NULL,
		deleted_at   DATETIME
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_triggers_ws_name ON triggers(workspace_id, name) WHERE deleted_at IS NULL`,
	`CREATE INDEX IF NOT EXISTS idx_triggers_ws_created ON triggers(workspace_id, created_at DESC, id DESC) WHERE deleted_at IS NULL`,

	`CREATE TABLE IF NOT EXISTS trigger_firings (
		id            TEXT PRIMARY KEY,
		workspace_id  TEXT NOT NULL,
		trigger_id    TEXT NOT NULL,
		workflow_id   TEXT NOT NULL,
		activation_id TEXT NOT NULL DEFAULT '',
		payload       TEXT NOT NULL DEFAULT '{}',
		dedup_key     TEXT NOT NULL,
		status        TEXT NOT NULL CHECK (status IN ('pending','claimed','started','skipped','superseded','shed')),
		flowrun_id    TEXT NOT NULL DEFAULT '',
		created_at    DATETIME NOT NULL,
		updated_at    DATETIME NOT NULL
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_trf_dedup ON trigger_firings(workflow_id, trigger_id, dedup_key)`,
	`CREATE INDEX IF NOT EXISTS idx_trf_pending ON trigger_firings(status, created_at) WHERE status = 'pending'`,

	`CREATE TABLE IF NOT EXISTS trigger_activations (
		id           TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		trigger_id   TEXT NOT NULL,
		kind         TEXT NOT NULL,
		fired        INTEGER NOT NULL DEFAULT 0,
		return_value TEXT NOT NULL DEFAULT '{}',
		payload      TEXT NOT NULL DEFAULT '{}',
		error        TEXT NOT NULL DEFAULT '',
		detail       TEXT NOT NULL DEFAULT '',
		firing_count INTEGER NOT NULL DEFAULT 0,
		created_at   DATETIME NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_tra_ws_trigger ON trigger_activations(workspace_id, trigger_id, created_at DESC, id DESC)`,
}

// Store implements triggerdomain.Repository over pkg/orm.
type Store struct {
	db   *ormpkg.DB
	trgs *ormpkg.Repo[triggerdomain.Trigger]
	frs  *ormpkg.Repo[triggerdomain.Firing]
	acts *ormpkg.Repo[triggerdomain.Activation]
}

// New constructs a Store bound to the three trigger tables.
func New(db *ormpkg.DB) *Store {
	return &Store{
		db:   db,
		trgs: ormpkg.For[triggerdomain.Trigger](db, "triggers"),
		frs:  ormpkg.For[triggerdomain.Firing](db, "trigger_firings"),
		acts: ormpkg.For[triggerdomain.Activation](db, "trigger_activations"),
	}
}

var _ triggerdomain.Repository = (*Store)(nil)

// --- triggers --------------------------------------------------------------

func (s *Store) SaveTrigger(ctx context.Context, t *triggerdomain.Trigger) error {
	if err := s.trgs.Save(ctx, t); err != nil {
		if errors.Is(err, ormpkg.ErrConflict) {
			return triggerdomain.ErrDuplicateName
		}
		return fmt.Errorf("triggerstore.SaveTrigger: %w", err)
	}
	return nil
}

func (s *Store) GetTrigger(ctx context.Context, id string) (*triggerdomain.Trigger, error) {
	t, err := s.trgs.Get(ctx, id)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, triggerdomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("triggerstore.GetTrigger: %w", err)
	}
	return t, nil
}

func (s *Store) GetTriggerByName(ctx context.Context, name string) (*triggerdomain.Trigger, error) {
	t, err := s.trgs.WhereEq("name", name).First(ctx)
	if errors.Is(err, ormpkg.ErrNotFound) {
		return nil, triggerdomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("triggerstore.GetTriggerByName: %w", err)
	}
	return t, nil
}

func (s *Store) GetTriggersByIDs(ctx context.Context, ids []string) ([]*triggerdomain.Trigger, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.trgs.WhereIn("id", toAny(ids)...).Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("triggerstore.GetTriggersByIDs: %w", err)
	}
	byID := make(map[string]*triggerdomain.Trigger, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	out := make([]*triggerdomain.Trigger, 0, len(ids))
	for _, id := range ids {
		if r, ok := byID[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Store) ListTriggers(ctx context.Context, filter triggerdomain.ListFilter) ([]*triggerdomain.Trigger, string, error) {
	rows, next, err := s.trgs.Query().Page(ctx, filter.Cursor, filter.Limit)
	if err != nil {
		return nil, "", fmt.Errorf("triggerstore.ListTriggers: %w", err)
	}
	return rows, next, nil
}

func (s *Store) ListAllTriggers(ctx context.Context) ([]*triggerdomain.Trigger, error) {
	rows, err := s.trgs.Order("created_at DESC, id DESC").Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("triggerstore.ListAllTriggers: %w", err)
	}
	return rows, nil
}

func (s *Store) DeleteTrigger(ctx context.Context, id string) error {
	ok, err := s.trgs.Delete(ctx, id) // soft-delete (triggers has deleted_at)
	if err != nil {
		return fmt.Errorf("triggerstore.DeleteTrigger: %w", err)
	}
	if !ok {
		return triggerdomain.ErrNotFound
	}
	return nil
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, v := range ss {
		out[i] = v
	}
	return out
}

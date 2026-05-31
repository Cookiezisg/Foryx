package trigger_test

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	triggerstore "github.com/sunweilin/forgify/backend/internal/infra/store/trigger"
)

func newStore(t *testing.T) *triggerstore.Store {
	t.Helper()
	gdb, err := dbinfra.Open(dbinfra.Config{DataDir: ""})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := dbinfra.Migrate(gdb, triggerstore.AutoMigrateModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return triggerstore.New(gdb)
}

// dedup_key UNIQUE makes re-materialization idempotent: the 2nd append returns the existing firing.
func TestAppendFiring_DedupReturnsExisting(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	mk := func() *triggerdomain.TriggerFiring {
		return &triggerdomain.TriggerFiring{WorkflowID: "wf1", TriggerNodeID: "t1", DedupKey: "k1"}
	}
	first, err := s.AppendFiring(ctx, mk())
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := s.AppendFiring(ctx, mk())
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("dedup violated: 2nd id %s, want existing %s", second.ID, first.ID)
	}
	pending, _ := s.ListPending(ctx, 0)
	if len(pending) != 1 {
		t.Fatalf("want 1 firing after dedup, got %d", len(pending))
	}
}

// ClaimFiring is single-transaction (ADR-021): claim+create+backfill atomically; a 2nd claim loses
// the race (ErrFiringNotPending) and create runs exactly once — no claimed-but-no-flowrun strand.
func TestClaimFiring_SingleTxIdempotent(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	f, err := s.AppendFiring(ctx, &triggerdomain.TriggerFiring{WorkflowID: "wf1", TriggerNodeID: "t1", DedupKey: "k1"})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	created := 0
	fid, err := s.ClaimFiring(ctx, f.ID, func(tx *gorm.DB) (string, error) {
		created++
		return "fr_x", nil
	})
	if err != nil || fid != "fr_x" {
		t.Fatalf("claim: id=%s err=%v", fid, err)
	}
	_, err = s.ClaimFiring(ctx, f.ID, func(tx *gorm.DB) (string, error) {
		created++
		return "fr_y", nil
	})
	if !errors.Is(err, triggerstore.ErrFiringNotPending) {
		t.Fatalf("2nd claim must lose with ErrFiringNotPending, got %v", err)
	}
	if created != 1 {
		t.Fatalf("create must run exactly once across two claims (single-tx), ran %d", created)
	}
}

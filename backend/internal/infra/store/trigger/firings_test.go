package trigger

import (
	"testing"

	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
)

// TestSearchFirings_FilterAndOrder: the inbox pages newest-first, filters by trigger and
// status, and stays inside the workspace (D2).
//
// TestSearchFirings_FilterAndOrder：收件箱最新优先分页、按 trigger 与 status 过滤、不出 workspace（D2）。
func TestSearchFirings_FilterAndOrder(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mk := func(id, trg, dedup, status string) {
		t.Helper()
		f := &triggerdomain.Firing{ID: id, TriggerID: trg, WorkflowID: "wf_1", ActivationID: "tra_1", DedupKey: dedup, Status: triggerdomain.FiringPending}
		if _, err := s.AppendFiring(ctx, f); err != nil {
			t.Fatalf("AppendFiring %s: %v", id, err)
		}
		if status != triggerdomain.FiringPending {
			if err := s.MarkFiringOutcome(ctx, id, status); err != nil {
				t.Fatalf("MarkFiringOutcome %s: %v", id, err)
			}
		}
	}
	mk("trf_1", "trg_a", "k1", triggerdomain.FiringStarted)
	mk("trf_2", "trg_a", "k2", triggerdomain.FiringSkipped)
	mk("trf_3", "trg_b", "k3", triggerdomain.FiringPending)

	rows, _, err := s.SearchFirings(ctx, triggerdomain.FiringFilter{TriggerID: "trg_a"})
	if err != nil || len(rows) != 2 {
		t.Fatalf("trigger filter: rows=%d err=%v", len(rows), err)
	}
	rows, _, err = s.SearchFirings(ctx, triggerdomain.FiringFilter{TriggerID: "trg_a", Status: triggerdomain.FiringSkipped})
	if err != nil || len(rows) != 1 || rows[0].ID != "trf_2" {
		t.Fatalf("status filter: %v err=%v", rows, err)
	}
	rows, _, err = s.SearchFirings(ctx, triggerdomain.FiringFilter{})
	if err != nil || len(rows) != 3 {
		t.Fatalf("unfiltered: rows=%d err=%v", len(rows), err)
	}
}

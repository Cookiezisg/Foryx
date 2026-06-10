package trigger

import (
	"testing"

	triggerinfra "github.com/sunweilin/forgify/backend/internal/infra/trigger"
)

// TestAttachOnce_AutoDisarmsAfterFire: a one-shot (staged) workflow fires exactly once, then is
// auto-detached — while a continuously-attached workflow on the same trigger keeps firing. After the
// first fire both run; after the second only the continuous one does.
func TestAttachOnce_AutoDisarmsAfterFire(t *testing.T) {
	s, st := newTestService(t)
	ctx := ctxWS("ws_1")
	fake := &fakeListener{}
	s.cron = fake
	tr := mkCron(t, s, ctx, "t")

	if err := s.AttachOnce(ctx, tr.ID, "wf_once"); err != nil {
		t.Fatalf("AttachOnce: %v", err)
	}
	if err := s.Attach(ctx, tr.ID, "wf_cont"); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if fake.registers != 1 {
		t.Fatalf("two workflows share one listener, want 1 register, got %d", fake.registers)
	}

	// first fire → both wf_once and wf_cont get a firing; then wf_once auto-disarms.
	s.onReport(tr.ID, triggerinfra.Activity{Fired: true, DedupKey: "k1"})
	if firings, _ := st.ListPendingFirings(ctx, 0); len(firings) != 2 {
		t.Fatalf("first fire: want 2 firings (both workflows), got %d", len(firings))
	}
	if fake.unregisters != 0 {
		t.Fatalf("listener must stay up for the continuous workflow, got %d unregisters", fake.unregisters)
	}

	// second fire → only wf_cont (wf_once is disarmed): one more firing, 3 total pending.
	s.onReport(tr.ID, triggerinfra.Activity{Fired: true, DedupKey: "k2"})
	if firings, _ := st.ListPendingFirings(ctx, 0); len(firings) != 3 {
		t.Fatalf("second fire: want 3 total firings (wf_once disarmed), got %d", len(firings))
	}
}

// TestAttachOnce_SoleListenerStopsAfterFire: when the ONLY reference is a one-shot, its single fire
// disarms it and takes the listener 1→0 (it stops) — staging does not leak a hot listener.
func TestAttachOnce_SoleListenerStopsAfterFire(t *testing.T) {
	s, _ := newTestService(t)
	ctx := ctxWS("ws_1")
	fake := &fakeListener{}
	s.cron = fake
	tr := mkCron(t, s, ctx, "t")

	if err := s.AttachOnce(ctx, tr.ID, "wf_once"); err != nil {
		t.Fatalf("AttachOnce: %v", err)
	}
	if fake.registers != 1 {
		t.Fatalf("want 1 register, got %d", fake.registers)
	}

	s.onReport(tr.ID, triggerinfra.Activity{Fired: true, DedupKey: "k1"})
	if fake.unregisters != 1 {
		t.Fatalf("a sole one-shot should stop the listener after its single fire, got %d unregisters", fake.unregisters)
	}
}

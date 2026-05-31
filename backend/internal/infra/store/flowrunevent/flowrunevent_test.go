package flowrunevent_test

import (
	"context"
	"testing"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	flowruneventstore "github.com/sunweilin/forgify/backend/internal/infra/store/flowrunevent"
)

func newStore(t *testing.T) *flowruneventstore.Store {
	t.Helper()
	gdb, err := dbinfra.Open(dbinfra.Config{DataDir: ""})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := dbinfra.Migrate(gdb, flowruneventstore.AutoMigrateModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return flowruneventstore.New(gdb)
}

// record-once (ADR-018): a result event written twice with the same identity collides on
// the dedup_key partial index; the 2nd append is a no-op returning the first (first-wins).
func TestAppendEvent_RecordOnceCollisionReturnsExisting(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	mk := func() *flowrundomain.FlowRunEvent {
		return &flowrundomain.FlowRunEvent{
			FlowrunID: "fr_1", Type: flowrundomain.EventNodeCompleted,
			NodeID: "n1", IterationKey: 0, Generation: 0,
			Result: map[string]any{"v": 1},
		}
	}
	first, err := s.AppendEvent(ctx, mk())
	if err != nil {
		t.Fatalf("first append: %v", err)
	}
	second, err := s.AppendEvent(ctx, mk())
	if err != nil {
		t.Fatalf("second append: %v", err)
	}
	if second.Seq != first.Seq {
		t.Fatalf("record-once violated: 2nd seq %d, want existing %d", second.Seq, first.Seq)
	}
	all, err := s.LoadJournal(ctx, "fr_1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("want 1 journaled event, got %d", len(all))
	}
}

// seq is strictly monotonic per-flowrun and allocated independently per flowrun. node_started
// is attempt-class (dedup_key='') so 5 of them all persist — proving append-many is NOT deduped.
func TestAppendEvent_SeqStrictlyMonotonicPerFlowrun(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	for i := 1; i <= 5; i++ {
		got, err := s.AppendEvent(ctx, &flowrundomain.FlowRunEvent{
			FlowrunID: "fr_x", Type: flowrundomain.EventNodeStarted, NodeID: "n", Attempt: i,
		})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		if got.Seq != int64(i) {
			t.Fatalf("seq not monotonic: got %d want %d", got.Seq, i)
		}
	}
	all, _ := s.LoadJournal(ctx, "fr_x")
	if len(all) != 5 {
		t.Fatalf("attempt-class must append-many: want 5, got %d", len(all))
	}
	other, err := s.AppendEvent(ctx, &flowrundomain.FlowRunEvent{
		FlowrunID: "fr_y", Type: flowrundomain.EventNodeStarted, NodeID: "n",
	})
	if err != nil {
		t.Fatalf("other append: %v", err)
	}
	if other.Seq != 1 {
		t.Fatalf("per-flowrun seq isolation broken: got %d want 1", other.Seq)
	}
}

// the approval timeout↔decision race (17 §2/§9): both journal a signal_received for the same
// approval; same dedup bucket → first-wins, no double-fire. (The review's suspected blocker.)
func TestAppendEvent_SignalReceivedFirstWins(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	decision := &flowrundomain.FlowRunEvent{
		FlowrunID: "fr_a", Type: flowrundomain.EventSignalReceived, NodeID: "appr",
		Result: map[string]any{"decision": "yes", "source": "user"},
	}
	timeout := &flowrundomain.FlowRunEvent{
		FlowrunID: "fr_a", Type: flowrundomain.EventSignalReceived, NodeID: "appr",
		Result: map[string]any{"decision": "no", "source": "timeout"},
	}
	first, err := s.AppendEvent(ctx, decision)
	if err != nil {
		t.Fatalf("decision: %v", err)
	}
	second, err := s.AppendEvent(ctx, timeout)
	if err != nil {
		t.Fatalf("timeout: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("first-wins violated: 2nd id %s, want %s", second.ID, first.ID)
	}
	all, _ := s.LoadJournal(ctx, "fr_a")
	if len(all) != 1 {
		t.Fatalf("double signal recorded: want 1, got %d", len(all))
	}
	src := all[0].Result.(map[string]any)["source"]
	if src != "user" {
		t.Fatalf("first writer (user) did not win: source=%v", src)
	}
}

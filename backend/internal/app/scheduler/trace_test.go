package scheduler

import (
	"context"
	"testing"

	"go.uber.org/zap"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
)

// GetTrace projects the journal seq-ordered; nodeId="" returns the whole run, a nodeId filters to
// that node's entries — loop iterations included, distinguished by iterationKey (08 §6).
func TestGetTrace_FiltersNodeAndKeepsLoopIterations(t *testing.T) {
	journal := newJournal(t)
	ctx := context.Background()
	mustAppend(t, journal, &flowrundomain.FlowRunEvent{FlowrunID: "fr1", Type: flowrundomain.EventNodeStarted, NodeID: "a"})
	mustAppend(t, journal, &flowrundomain.FlowRunEvent{FlowrunID: "fr1", Type: flowrundomain.EventNodeCompleted, NodeID: "a", Result: map[string]any{"v": 1}})
	mustAppend(t, journal, &flowrundomain.FlowRunEvent{FlowrunID: "fr1", Type: flowrundomain.EventNodeStarted, NodeID: "loop", IterationKey: 0})
	mustAppend(t, journal, &flowrundomain.FlowRunEvent{FlowrunID: "fr1", Type: flowrundomain.EventNodeCompleted, NodeID: "loop", IterationKey: 0})
	mustAppend(t, journal, &flowrundomain.FlowRunEvent{FlowrunID: "fr1", Type: flowrundomain.EventNodeStarted, NodeID: "loop", IterationKey: 1})

	s := NewService(newFakeRepo(), &fakeWorkflowReader{}, notificationspkg.New(nil, zap.NewNop()), zap.NewNop())
	s.SetJournal(journal)

	// whole run: all 5 events, strictly seq-ordered.
	all, err := s.GetTrace(ctx, "fr1", "")
	if err != nil {
		t.Fatalf("GetTrace all: %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("whole-run trace want 5 entries, got %d", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].Seq <= all[i-1].Seq {
			t.Fatalf("trace not seq-ordered at %d: %d <= %d", i, all[i].Seq, all[i-1].Seq)
		}
	}

	// per-node: only loop's 3 entries, keeping both iterations distinguishable.
	loop, err := s.GetTrace(ctx, "fr1", "loop")
	if err != nil {
		t.Fatalf("GetTrace loop: %v", err)
	}
	if len(loop) != 3 {
		t.Fatalf("loop trace want 3 entries (iter0 start+complete, iter1 start), got %d", len(loop))
	}
	if loop[0].IterationKey != 0 || loop[2].IterationKey != 1 {
		t.Fatalf("loop iterations not preserved: %+v", loop)
	}
	for _, e := range loop {
		if e.NodeID != "loop" {
			t.Fatalf("node filter leaked %q", e.NodeID)
		}
	}

	// the activity result rides along (UI per-node diagnostic shows input/result).
	if all[1].Type != flowrundomain.EventNodeCompleted || asMap(all[1].Result)["v"] == nil {
		t.Fatalf("completed event should carry its result payload: %+v", all[1])
	}
}

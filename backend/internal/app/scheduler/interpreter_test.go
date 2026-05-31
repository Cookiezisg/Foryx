package scheduler

import (
	"context"
	"testing"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	flowruneventstore "github.com/sunweilin/forgify/backend/internal/infra/store/flowrunevent"
)

// countingRouter records how many times each node was actually dispatched, so a test can
// assert that replay COPIES journaled results rather than re-running them.
type countingRouter struct{ calls map[string]int }

func (c *countingRouter) Dispatch(ctx context.Context, in DispatchInput) DispatchOutput {
	c.calls[in.Node.ID]++
	return DispatchOutput{Outputs: map[string]any{"echo": in.Node.ID}}
}

// trigger(t) -> function(a) -> function(b): a minimal linear flow.
func linearGraph() workflowdomain.Graph {
	return workflowdomain.Graph{
		Name: "linear",
		Nodes: []workflowdomain.NodeSpec{
			{ID: "t", Type: workflowdomain.NodeTypeTrigger},
			{ID: "a", Type: workflowdomain.NodeTypeFunction},
			{ID: "b", Type: workflowdomain.NodeTypeFunction},
		},
		Edges: []workflowdomain.EdgeSpec{
			{ID: "e1", From: "t", To: "a"},
			{ID: "e2", From: "a", To: "b"},
		},
	}
}

// trigger -> case(when payload.x>5 -> hi, else -> lo). case routes via branches[].to.
func caseGraph() workflowdomain.Graph {
	return workflowdomain.Graph{
		Name: "case",
		Nodes: []workflowdomain.NodeSpec{
			{ID: "t", Type: workflowdomain.NodeTypeTrigger},
			{ID: "c", Type: workflowdomain.NodeTypeCondition, Config: map[string]any{
				"branches": []any{
					map[string]any{"when": "payload.x > 5", "to": "hi"},
					map[string]any{"when": "true", "to": "lo"},
				},
			}},
			{ID: "hi", Type: workflowdomain.NodeTypeFunction},
			{ID: "lo", Type: workflowdomain.NodeTypeFunction},
		},
		Edges: []workflowdomain.EdgeSpec{{ID: "e1", From: "t", To: "c"}},
	}
}

func newJournal(t *testing.T) *flowruneventstore.Store {
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

// The承重 invariant (17 §4, ADR-016/019): replaying the same journal is deterministic and
// COPIES journaled activity results — it never re-runs an already-recorded activity.
func TestInterpreter_ReplayIsDeterministicAndCopiesNotReruns(t *testing.T) {
	journal := newJournal(t)
	router := &countingRouter{calls: map[string]int{}}
	graph := linearGraph()
	ctx := context.Background()

	if err := New(journal, router).Run(ctx, "fr_det", graph, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	j1, _ := journal.LoadJournal(ctx, "fr_det")
	if router.calls["a"] != 1 || router.calls["b"] != 1 {
		t.Fatalf("first run should dispatch a,b exactly once: %v", router.calls)
	}

	if err := New(journal, router).Resume(ctx, "fr_det", graph, nil); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if router.calls["a"] != 1 || router.calls["b"] != 1 {
		t.Fatalf("replay re-ran an already-journaled activity (must copy, not re-run): %v", router.calls)
	}
	j2, _ := journal.LoadJournal(ctx, "fr_det")
	if len(j2) != len(j1) {
		t.Fatalf("replay changed the journal: was %d events, now %d", len(j1), len(j2))
	}
	for i := range j1 {
		if j1[i].Type != j2[i].Type || j1[i].NodeID != j2[i].NodeID || j1[i].Seq != j2[i].Seq {
			t.Fatalf("replay diverged at #%d: %+v vs %+v", i, j1[i], j2[i])
		}
	}
}

// A linear run journals node_started+node_completed per activity (not the trigger) in seq order.
func TestInterpreter_LinearRunJournalsEachActivity(t *testing.T) {
	journal := newJournal(t)
	router := &countingRouter{calls: map[string]int{}}
	ctx := context.Background()

	if err := New(journal, router).Run(ctx, "fr_lin", linearGraph(), nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	evs, _ := journal.LoadJournal(ctx, "fr_lin")
	want := []struct {
		typ, node string
	}{
		{flowrundomain.EventNodeStarted, "a"}, {flowrundomain.EventNodeCompleted, "a"},
		{flowrundomain.EventNodeStarted, "b"}, {flowrundomain.EventNodeCompleted, "b"},
	}
	if len(evs) != len(want) {
		t.Fatalf("want %d events, got %d: %+v", len(want), len(evs), evs)
	}
	for i, w := range want {
		if evs[i].Type != w.typ || evs[i].NodeID != w.node {
			t.Fatalf("event #%d: got %s/%s want %s/%s", i, evs[i].Type, evs[i].NodeID, w.typ, w.node)
		}
	}
}

// case node: per-branch CEL guard, first-true-wins; routes via branches[].to + journals branch_taken.
func TestInterpreter_CaseFirstTrueWins(t *testing.T) {
	journal := newJournal(t)
	router := &countingRouter{calls: map[string]int{}}
	ctx := context.Background()

	if err := New(journal, router).Run(ctx, "fr_hi", caseGraph(), map[string]any{"x": 10}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if router.calls["hi"] != 1 || router.calls["lo"] != 0 {
		t.Fatalf("x=10 (>5) must route to hi, not lo: %v", router.calls)
	}
	evs, _ := journal.LoadJournal(ctx, "fr_hi")
	found := false
	for _, e := range evs {
		if e.Type == flowrundomain.EventBranchTaken && e.NodeID == "c" {
			found = true
			if to := asMap(e.Result)["to"]; to != "hi" {
				t.Fatalf("branch_taken to=%v want hi", to)
			}
		}
	}
	if !found {
		t.Fatal("case did not journal branch_taken")
	}
}

// case falls through to the when:"true" branch when the guard is false.
func TestInterpreter_CaseFallthroughToTrueBranch(t *testing.T) {
	journal := newJournal(t)
	router := &countingRouter{calls: map[string]int{}}
	ctx := context.Background()

	if err := New(journal, router).Run(ctx, "fr_lo", caseGraph(), map[string]any{"x": 1}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if router.calls["lo"] != 1 || router.calls["hi"] != 0 {
		t.Fatalf("x=1 (<=5) must fall through to lo: %v", router.calls)
	}
}

// replay copies the recorded branch_taken decision — it does not re-evaluate the guard
// (the basis for deterministic active-branch join, 17 §3).
func TestInterpreter_CaseReplay_CopiesDecision(t *testing.T) {
	journal := newJournal(t)
	router := &countingRouter{calls: map[string]int{}}
	ctx := context.Background()

	if err := New(journal, router).Run(ctx, "fr_cr", caseGraph(), map[string]any{"x": 10}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if err := New(journal, router).Resume(ctx, "fr_cr", caseGraph(), map[string]any{"x": 10}); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if router.calls["hi"] != 1 || router.calls["lo"] != 0 {
		t.Fatalf("replay must copy the branch decision (hi once, lo zero): %v", router.calls)
	}
}

// trigger -> case(loop head): while payload.n < 2, emit n+1 and back-edge to itself; else -> done.
// A structured loop via a case back-edge; counter rides in the payload (04 §loop).
func loopGraph() workflowdomain.Graph {
	return workflowdomain.Graph{
		Name: "loop",
		Nodes: []workflowdomain.NodeSpec{
			{ID: "t", Type: workflowdomain.NodeTypeTrigger},
			{ID: "c", Type: workflowdomain.NodeTypeCondition, Config: map[string]any{
				"branches": []any{
					map[string]any{"when": "payload.n < 2", "to": "c", "emit": map[string]any{"n": "payload.n + 1"}},
					map[string]any{"when": "true", "to": "done"},
				},
			}},
			{ID: "done", Type: workflowdomain.NodeTypeFunction},
		},
		Edges: []workflowdomain.EdgeSpec{{ID: "e1", From: "t", To: "c"}},
	}
}

// the loop runs to exit; each case activation gets a distinct iteration_key (ADR-017 back-edge
// ordinal), so the per-iteration branch_taken events don't collide and `done` runs exactly once.
func TestInterpreter_StructuredLoop_IterationKey(t *testing.T) {
	journal := newJournal(t)
	router := &countingRouter{calls: map[string]int{}}
	ctx := context.Background()

	if err := New(journal, router).Run(ctx, "fr_loop", loopGraph(), map[string]any{"n": 0}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if router.calls["done"] != 1 {
		t.Fatalf("done should run exactly once after the loop exits: %v", router.calls)
	}
	var iters []int
	for _, e := range mustLoad(t, journal, "fr_loop") {
		if e.Type == flowrundomain.EventBranchTaken && e.NodeID == "c" {
			iters = append(iters, e.IterationKey)
		}
	}
	if len(iters) != 3 || iters[0] != 0 || iters[1] != 1 || iters[2] != 2 {
		t.Fatalf("case branch_taken iteration_keys = %v, want [0 1 2]", iters)
	}
}

// replaying a completed loop copies every iteration's recorded decision/result — no re-run.
func TestInterpreter_LoopReplay_NoRerun(t *testing.T) {
	journal := newJournal(t)
	router := &countingRouter{calls: map[string]int{}}
	ctx := context.Background()

	if err := New(journal, router).Run(ctx, "fr_lr", loopGraph(), map[string]any{"n": 0}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if err := New(journal, router).Resume(ctx, "fr_lr", loopGraph(), map[string]any{"n": 0}); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if router.calls["done"] != 1 {
		t.Fatalf("replay must copy the loop (done once total): %v", router.calls)
	}
}

func mustLoad(t *testing.T, j *flowruneventstore.Store, id string) []flowrundomain.FlowRunEvent {
	t.Helper()
	evs, err := j.LoadJournal(context.Background(), id)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return evs
}

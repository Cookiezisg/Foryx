package scheduler

import (
	"context"
	"errors"
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
		Edges: []workflowdomain.EdgeSpec{
			{ID: "e1", From: "t", To: "c"},
			{ID: "e2", From: "c", To: "hi"},
			{ID: "e3", From: "c", To: "lo"},
		},
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

	if _, err := New(journal, router).Run(ctx, "fr_det", graph, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	j1, _ := journal.LoadJournal(ctx, "fr_det")
	if router.calls["a"] != 1 || router.calls["b"] != 1 {
		t.Fatalf("first run should dispatch a,b exactly once: %v", router.calls)
	}

	if _, err := New(journal, router).Resume(ctx, "fr_det", graph, nil); err != nil {
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

	if _, err := New(journal, router).Run(ctx, "fr_lin", linearGraph(), nil); err != nil {
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

	if _, err := New(journal, router).Run(ctx, "fr_hi", caseGraph(), map[string]any{"x": 10}); err != nil {
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

	if _, err := New(journal, router).Run(ctx, "fr_lo", caseGraph(), map[string]any{"x": 1}); err != nil {
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

	if _, err := New(journal, router).Run(ctx, "fr_cr", caseGraph(), map[string]any{"x": 10}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := New(journal, router).Resume(ctx, "fr_cr", caseGraph(), map[string]any{"x": 10}); err != nil {
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
		Edges: []workflowdomain.EdgeSpec{
			{ID: "e1", From: "t", To: "c"},
			{ID: "e2", From: "c", To: "c"},
			{ID: "e3", From: "c", To: "done"},
		},
	}
}

// the loop runs to exit; each case activation gets a distinct iteration_key (ADR-017 back-edge
// ordinal), so the per-iteration branch_taken events don't collide and `done` runs exactly once.
func TestInterpreter_StructuredLoop_IterationKey(t *testing.T) {
	journal := newJournal(t)
	router := &countingRouter{calls: map[string]int{}}
	ctx := context.Background()

	if _, err := New(journal, router).Run(ctx, "fr_loop", loopGraph(), map[string]any{"n": 0}); err != nil {
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

	if _, err := New(journal, router).Run(ctx, "fr_lr", loopGraph(), map[string]any{"n": 0}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := New(journal, router).Resume(ctx, "fr_lr", loopGraph(), map[string]any{"n": 0}); err != nil {
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

// trigger -> f -> {a, b} -> j: an AND-split (f forks) and an AND-join (j awaits both a and b).
func andJoinGraph() workflowdomain.Graph {
	return workflowdomain.Graph{
		Name: "andjoin",
		Nodes: []workflowdomain.NodeSpec{
			{ID: "t", Type: workflowdomain.NodeTypeTrigger},
			{ID: "f", Type: workflowdomain.NodeTypeFunction},
			{ID: "a", Type: workflowdomain.NodeTypeFunction},
			{ID: "b", Type: workflowdomain.NodeTypeFunction},
			{ID: "j", Type: workflowdomain.NodeTypeFunction},
		},
		Edges: []workflowdomain.EdgeSpec{
			{ID: "e1", From: "t", To: "f"},
			{ID: "e2", From: "f", To: "a"},
			{ID: "e3", From: "f", To: "b"},
			{ID: "e4", From: "a", To: "j"},
			{ID: "e5", From: "b", To: "j"},
		},
	}
}

// AND-join (WP3): f forks to a+b; j awaits BOTH (forward in-degree 2) and runs exactly once.
func TestInterpreter_ANDJoin_AwaitsAll(t *testing.T) {
	journal := newJournal(t)
	router := &countingRouter{calls: map[string]int{}}
	if _, err := New(journal, router).Run(context.Background(), "fr_and", andJoinGraph(), nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, n := range []string{"f", "a", "b", "j"} {
		if router.calls[n] != 1 {
			t.Fatalf("AND-join: %s ran %d times, want 1: %v", n, router.calls[n], router.calls)
		}
	}
}

// trigger -> case -> {a, b} -> j: a case diamond. case picks one branch; the join awaits only the
// activated in-edge (the skipped branch must not deadlock it — A-1, 17 §3 active-branch join).
func activeBranchGraph() workflowdomain.Graph {
	return workflowdomain.Graph{
		Name: "activebranch",
		Nodes: []workflowdomain.NodeSpec{
			{ID: "t", Type: workflowdomain.NodeTypeTrigger},
			{ID: "c", Type: workflowdomain.NodeTypeCondition, Config: map[string]any{
				"branches": []any{
					map[string]any{"when": "payload.x > 5", "to": "a"},
					map[string]any{"when": "true", "to": "b"},
				},
			}},
			{ID: "a", Type: workflowdomain.NodeTypeFunction},
			{ID: "b", Type: workflowdomain.NodeTypeFunction},
			{ID: "j", Type: workflowdomain.NodeTypeFunction},
		},
		Edges: []workflowdomain.EdgeSpec{
			{ID: "e1", From: "t", To: "c"},
			{ID: "e2", From: "c", To: "a"},
			{ID: "e3", From: "c", To: "b"},
			{ID: "e4", From: "a", To: "j"},
			{ID: "e5", From: "b", To: "j"},
		},
	}
}

// the case-diamond the old engine dead-locked on: case picks a, b is skipped, j still fires with
// a's input — proving active-branch join does NOT wait for the skipped branch (A-1).
func TestInterpreter_ActiveBranchJoin_NoDeadlock(t *testing.T) {
	journal := newJournal(t)
	router := &countingRouter{calls: map[string]int{}}
	if _, err := New(journal, router).Run(context.Background(), "fr_ab", activeBranchGraph(), map[string]any{"x": 10}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if router.calls["a"] != 1 || router.calls["b"] != 0 || router.calls["j"] != 1 {
		t.Fatalf("active-branch join: want a=1 b=0 j=1, got %v", router.calls)
	}
}

// trigger -> approval -> yes:dy / no:dn. approval routes via FromPort = decision.
func approvalGraph() workflowdomain.Graph {
	return workflowdomain.Graph{
		Name: "approval",
		Nodes: []workflowdomain.NodeSpec{
			{ID: "t", Type: workflowdomain.NodeTypeTrigger},
			{ID: "ap", Type: workflowdomain.NodeTypeApproval},
			{ID: "dy", Type: workflowdomain.NodeTypeFunction},
			{ID: "dn", Type: workflowdomain.NodeTypeFunction},
		},
		Edges: []workflowdomain.EdgeSpec{
			{ID: "e1", From: "t", To: "ap"},
			{ID: "e2", From: "ap", FromPort: "yes", To: "dy"},
			{ID: "e3", From: "ap", FromPort: "no", To: "dn"},
		},
	}
}

// approval parks the run (journals signal_awaited; caller sets status awaiting_signal) until a
// signal arrives — no downstream runs while parked.
func TestInterpreter_Approval_Parks(t *testing.T) {
	journal := newJournal(t)
	router := &countingRouter{calls: map[string]int{}}
	parked, err := New(journal, router).Run(context.Background(), "fr_ap", approvalGraph(), nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !parked {
		t.Fatal("approval must park the run")
	}
	if router.calls["dy"] != 0 || router.calls["dn"] != 0 {
		t.Fatalf("nothing downstream should run while parked: %v", router.calls)
	}
	found := false
	for _, e := range mustLoad(t, journal, "fr_ap") {
		if e.Type == flowrundomain.EventSignalAwaited && e.NodeID == "ap" {
			found = true
		}
	}
	if !found {
		t.Fatal("approval did not journal signal_awaited")
	}
}

// once the decision is journaled (signal_received), re-walking routes via the yes/no port and
// the run completes (durable approval = journal signal; the basis for crash-safe pause/resume).
func TestInterpreter_Approval_ResumeRoutesByDecision(t *testing.T) {
	journal := newJournal(t)
	router := &countingRouter{calls: map[string]int{}}
	ctx := context.Background()
	if _, err := New(journal, router).Run(ctx, "fr_apr", approvalGraph(), nil); err != nil {
		t.Fatalf("park: %v", err)
	}
	if _, err := journal.AppendEvent(ctx, &flowrundomain.FlowRunEvent{
		FlowrunID: "fr_apr", Type: flowrundomain.EventSignalReceived, NodeID: "ap",
		Result: map[string]any{"decision": "yes"},
	}); err != nil {
		t.Fatalf("inject signal: %v", err)
	}
	parked, err := New(journal, router).Resume(ctx, "fr_apr", approvalGraph(), nil)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if parked {
		t.Fatal("after the decision the run must complete, not park")
	}
	if router.calls["dy"] != 1 || router.calls["dn"] != 0 {
		t.Fatalf("decision=yes must route to dy: %v", router.calls)
	}
}

// A cancelled run ctx surfaces as context.Canceled from the walk (so executeRun maps it to
// cancelled), NOT swallowed into a NODE_FAILED (concurrency-error-edges-2).
func TestInterpreter_CancelledCtx_ReturnsCtxErr(t *testing.T) {
	journal := newJournal(t)
	router := &countingRouter{calls: map[string]int{}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New(journal, router).Run(ctx, "fr_cancel", linearGraph(), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ctx must surface context.Canceled, got %v", err)
	}
}

// A malformed emit CEL expression fails the case node (returned error) instead of silently
// writing nil into the payload field (cel-safety-2).
func TestInterpreter_EmitCompileError_FailsNode(t *testing.T) {
	journal := newJournal(t)
	router := &countingRouter{calls: map[string]int{}}
	g := workflowdomain.Graph{
		Name: "bad_emit",
		Nodes: []workflowdomain.NodeSpec{
			{ID: "t", Type: workflowdomain.NodeTypeTrigger},
			{ID: "c", Type: workflowdomain.NodeTypeCondition, Config: map[string]any{
				"branches": []any{
					map[string]any{"when": "true", "to": "x", "emit": map[string]any{"bad": "payload.("}},
				},
			}},
		},
		Edges: []workflowdomain.EdgeSpec{{ID: "e1", From: "t", To: "c"}},
	}
	if _, err := New(journal, router).Run(context.Background(), "fr_bademit", g, nil); err == nil {
		t.Fatal("a malformed emit expr must fail the node, not silently write nil")
	}
}

// ctx is wired (17 §7 input = payload + ctx): a case guard reading ctx.runId routes correctly,
// proving the variable is populated — not the old declared-but-empty fail-to-false (cel-safety-3).
func TestInterpreter_CtxWired_GuardReadsRunId(t *testing.T) {
	journal := newJournal(t)
	router := &countingRouter{calls: map[string]int{}}
	g := workflowdomain.Graph{
		Name: "ctx_guard",
		Nodes: []workflowdomain.NodeSpec{
			{ID: "t", Type: workflowdomain.NodeTypeTrigger},
			{ID: "c", Type: workflowdomain.NodeTypeCondition, Config: map[string]any{
				"branches": []any{
					map[string]any{"when": "ctx.runId == 'fr_ctx'", "to": "hit"},
					map[string]any{"when": "true", "to": "miss"},
				},
			}},
			{ID: "hit", Type: workflowdomain.NodeTypeFunction},
			{ID: "miss", Type: workflowdomain.NodeTypeFunction},
		},
		Edges: []workflowdomain.EdgeSpec{
			{ID: "e1", From: "t", To: "c"},
			{ID: "e2", From: "c", To: "hit"},
			{ID: "e3", From: "c", To: "miss"},
		},
	}
	if _, err := New(journal, router).Run(context.Background(), "fr_ctx", g, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	if router.calls["hit"] != 1 || router.calls["miss"] != 0 {
		t.Fatalf("ctx.runId guard must route to hit (ctx wired, not empty): %v", router.calls)
	}
}

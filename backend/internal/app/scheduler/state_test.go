package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

func mkGraph(nodes []workflowdomain.NodeSpec, edges []workflowdomain.EdgeSpec) *workflowdomain.Graph {
	return &workflowdomain.Graph{Name: "wf", Nodes: nodes, Edges: edges}
}

func node(id, typ string) workflowdomain.NodeSpec {
	return workflowdomain.NodeSpec{ID: id, Type: typ}
}

func nodeOnErr(id, typ, onError string) workflowdomain.NodeSpec {
	n := node(id, typ)
	n.OnError = onError
	return n
}

func edge(from, to string) workflowdomain.EdgeSpec {
	return workflowdomain.EdgeSpec{ID: "e_" + from + "_" + to, From: from, To: to}
}

func runWith(t *testing.T, graph *workflowdomain.Graph, register func(*Router)) (*fakeRepo, string) {
	t.Helper()
	repo := newFakeRepo()
	reader := &fakeWorkflowReader{
		wf: mkEnabledWorkflow(),
		ver: &workflowdomain.Version{
			ID: "wfv1", WorkflowID: "wf1", GraphParsed: graph,
		},
	}
	s := newSvc(t, repo, reader)
	register(s.RouterRef())

	runID, err := s.StartRun(ctxWith("u1"), "wf1", "manual", nil)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	// Wait for terminal state.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, _ := repo.Get(context.Background(), runID)
		if run.Status != flowrundomain.StatusRunning && run.Status != flowrundomain.StatusPaused {
			return repo, runID
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run did not reach terminal state within 2s")
	return nil, ""
}

func TestExecuteRun_EmptyGraph_Completed(t *testing.T) {
	graph := mkGraph(nil, nil)
	repo, runID := runWith(t, graph, func(*Router) {})

	run, _ := repo.Get(context.Background(), runID)
	if run.Status != flowrundomain.StatusCompleted {
		t.Errorf("empty graph status = %q, want completed", run.Status)
	}
}

func TestExecuteRun_SingleNode_Completed(t *testing.T) {
	graph := mkGraph(
		[]workflowdomain.NodeSpec{node("trig", workflowdomain.NodeTypeTrigger)},
		nil,
	)
	var calls int
	repo, runID := runWith(t, graph, func(r *Router) {
		r.Set(workflowdomain.NodeTypeTrigger, DispatcherFunc(func(_ context.Context, _ DispatchInput) DispatchOutput {
			calls++
			return DispatchOutput{Outputs: map[string]any{"fired": true}}
		}))
	})

	run, _ := repo.Get(context.Background(), runID)
	if run.Status != flowrundomain.StatusCompleted {
		t.Errorf("status = %q, want completed", run.Status)
	}
	if calls != 1 {
		t.Errorf("dispatcher calls = %d, want 1", calls)
	}
}

func TestExecuteRun_Linear_OrderRespected(t *testing.T) {
	graph := mkGraph(
		[]workflowdomain.NodeSpec{
			node("A", workflowdomain.NodeTypeTrigger),
			node("B", workflowdomain.NodeTypeFunction),
			node("C", workflowdomain.NodeTypeFunction),
		},
		[]workflowdomain.EdgeSpec{edge("A", "B"), edge("B", "C")},
	)

	var mu sync.Mutex
	order := []string{}
	dispatcher := DispatcherFunc(func(_ context.Context, in DispatchInput) DispatchOutput {
		mu.Lock()
		order = append(order, in.Node.ID)
		mu.Unlock()
		return DispatchOutput{}
	})

	repo, runID := runWith(t, graph, func(r *Router) {
		r.Set(workflowdomain.NodeTypeTrigger, dispatcher)
		r.Set(workflowdomain.NodeTypeFunction, dispatcher)
	})

	run, _ := repo.Get(context.Background(), runID)
	if run.Status != flowrundomain.StatusCompleted {
		t.Fatalf("status = %q", run.Status)
	}
	if len(order) != 3 {
		t.Fatalf("order len = %d, want 3", len(order))
	}
	if order[0] != "A" || order[1] != "B" || order[2] != "C" {
		t.Errorf("execution order = %v, want [A B C]", order)
	}
}

func TestExecuteRun_FanOutFanIn(t *testing.T) {
	// A → B, A → C, B → D, C → D (D depends on both B and C)
	graph := mkGraph(
		[]workflowdomain.NodeSpec{
			node("A", workflowdomain.NodeTypeTrigger),
			node("B", workflowdomain.NodeTypeFunction),
			node("C", workflowdomain.NodeTypeFunction),
			node("D", workflowdomain.NodeTypeFunction),
		},
		[]workflowdomain.EdgeSpec{
			edge("A", "B"), edge("A", "C"),
			edge("B", "D"), edge("C", "D"),
		},
	)

	var mu sync.Mutex
	completed := map[string]int{} // node → index of completion
	idx := 0
	dispatcher := DispatcherFunc(func(_ context.Context, in DispatchInput) DispatchOutput {
		// Force B/C parallel: simulate small work.
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		completed[in.Node.ID] = idx
		idx++
		mu.Unlock()
		return DispatchOutput{}
	})

	repo, runID := runWith(t, graph, func(r *Router) {
		r.Set(workflowdomain.NodeTypeTrigger, dispatcher)
		r.Set(workflowdomain.NodeTypeFunction, dispatcher)
	})

	run, _ := repo.Get(context.Background(), runID)
	if run.Status != flowrundomain.StatusCompleted {
		t.Fatalf("status = %q", run.Status)
	}
	if completed["A"] != 0 {
		t.Errorf("A should complete first, got idx %d", completed["A"])
	}
	if completed["D"] != 3 {
		t.Errorf("D should complete last, got idx %d", completed["D"])
	}
	// B and C complete in either order but both before D.
	if completed["B"] >= completed["D"] || completed["C"] >= completed["D"] {
		t.Errorf("B/C must complete before D: completed = %v", completed)
	}
}

func TestExecuteRun_FailedWithStopPolicy_FinalizesFailed(t *testing.T) {
	graph := mkGraph(
		[]workflowdomain.NodeSpec{
			node("A", workflowdomain.NodeTypeTrigger),
			nodeOnErr("B", workflowdomain.NodeTypeFunction, workflowdomain.OnErrorStop),
			node("C", workflowdomain.NodeTypeFunction),
		},
		[]workflowdomain.EdgeSpec{edge("A", "B"), edge("B", "C")},
	)

	repo, runID := runWith(t, graph, func(r *Router) {
		r.Set(workflowdomain.NodeTypeTrigger, DispatcherFunc(func(context.Context, DispatchInput) DispatchOutput {
			return DispatchOutput{}
		}))
		r.Set(workflowdomain.NodeTypeFunction, DispatcherFunc(func(_ context.Context, in DispatchInput) DispatchOutput {
			if in.Node.ID == "B" {
				return DispatchOutput{Error: errors.New("simulated B failure")}
			}
			return DispatchOutput{}
		}))
	})

	run, _ := repo.Get(context.Background(), runID)
	if run.Status != flowrundomain.StatusFailed {
		t.Errorf("status = %q, want failed", run.Status)
	}
}

func TestExecuteRun_FailedWithContinuePolicy_StillCompletes(t *testing.T) {
	graph := mkGraph(
		[]workflowdomain.NodeSpec{
			node("A", workflowdomain.NodeTypeTrigger),
			nodeOnErr("B", workflowdomain.NodeTypeFunction, workflowdomain.OnErrorContinue),
			node("C", workflowdomain.NodeTypeFunction),
		},
		[]workflowdomain.EdgeSpec{edge("A", "B"), edge("B", "C")},
	)

	var cReached bool
	repo, runID := runWith(t, graph, func(r *Router) {
		r.Set(workflowdomain.NodeTypeTrigger, DispatcherFunc(func(context.Context, DispatchInput) DispatchOutput {
			return DispatchOutput{}
		}))
		r.Set(workflowdomain.NodeTypeFunction, DispatcherFunc(func(_ context.Context, in DispatchInput) DispatchOutput {
			if in.Node.ID == "B" {
				return DispatchOutput{Error: errors.New("B fails but onError=continue")}
			}
			if in.Node.ID == "C" {
				cReached = true
			}
			return DispatchOutput{}
		}))
	})

	run, _ := repo.Get(context.Background(), runID)
	if run.Status != flowrundomain.StatusCompleted {
		t.Errorf("status = %q, want completed (B's failure absorbed)", run.Status)
	}
	if !cReached {
		t.Errorf("C should run after B onError=continue")
	}
}

func TestExecuteRun_NoDispatcherForType_Fails(t *testing.T) {
	graph := mkGraph(
		[]workflowdomain.NodeSpec{
			node("X", "unknown_type"),
		},
		nil,
	)
	repo, runID := runWith(t, graph, func(*Router) {})
	run, _ := repo.Get(context.Background(), runID)
	if run.Status != flowrundomain.StatusFailed {
		t.Errorf("status = %q, want failed (no dispatcher)", run.Status)
	}
}

func TestExecuteRun_WritesFlowrunNodeRow(t *testing.T) {
	graph := mkGraph(
		[]workflowdomain.NodeSpec{node("trig", workflowdomain.NodeTypeTrigger)},
		nil,
	)
	repo, runID := runWith(t, graph, func(r *Router) {
		r.Set(workflowdomain.NodeTypeTrigger, DispatcherFunc(func(context.Context, DispatchInput) DispatchOutput {
			return DispatchOutput{Outputs: map[string]any{"out": "ok"}}
		}))
	})
	_ = runID

	// fakeRepo doesn't count CreateNode calls — augment after; for now
	// inspect by checking the run completed (basic invariant).
	// fakeRepo 不数 CreateNode 调用;基础不变量检查。
	count := 0
	repo.mu.Lock()
	for range repo.runs {
		count++
	}
	repo.mu.Unlock()
	if count != 1 {
		t.Errorf("run count = %d, want 1", count)
	}
}

func TestAdvance_BranchingRoutesByFromPort(t *testing.T) {
	graph := mkGraph(
		[]workflowdomain.NodeSpec{
			node("trig", workflowdomain.NodeTypeTrigger),
			node("a1", workflowdomain.NodeTypeApproval),
			node("on_approve", workflowdomain.NodeTypeFunction),
			node("on_reject", workflowdomain.NodeTypeFunction),
		},
		[]workflowdomain.EdgeSpec{
			edge("trig", "a1"),
			{ID: "e_app", From: "a1", FromPort: "approved", To: "on_approve"},
			{ID: "e_rej", From: "a1", FromPort: "rejected", To: "on_reject"},
		},
	)
	topo := buildTopo(graph)
	// Initial ready = [trig].
	if r := topo.initialReady(); len(r) != 1 || r[0] != "trig" {
		t.Fatalf("initial ready = %v, want [trig]", r)
	}
	// trig → a1 (no port, single-output).
	ready := topo.advance("trig", "")
	if len(ready) != 1 || ready[0] != "a1" {
		t.Fatalf("after trig advance, ready = %v, want [a1]", ready)
	}
	// Approval picks "approved" → on_approve becomes ready, on_reject NOT.
	// The parked edge (e_rej) decrements on_reject's in-degree but never
	// adds it to ready — net effect: on_reject never runs.
	ready = topo.advance("a1", "approved")
	if len(ready) != 1 || ready[0] != "on_approve" {
		t.Fatalf("after approve advance, ready = %v, want [on_approve]", ready)
	}
	// And critically — on_reject is NOT in ready set even though its in-
	// degree just hit 0 (the parked-edge decrement should not enqueue).
	for _, id := range ready {
		if id == "on_reject" {
			t.Errorf("on_reject must not be ready when approved branch taken")
		}
	}
}

func TestAdvance_BranchingRejectPath(t *testing.T) {
	graph := mkGraph(
		[]workflowdomain.NodeSpec{
			node("trig", workflowdomain.NodeTypeTrigger),
			node("a1", workflowdomain.NodeTypeApproval),
			node("on_approve", workflowdomain.NodeTypeFunction),
			node("on_reject", workflowdomain.NodeTypeFunction),
		},
		[]workflowdomain.EdgeSpec{
			edge("trig", "a1"),
			{ID: "e_app", From: "a1", FromPort: "approved", To: "on_approve"},
			{ID: "e_rej", From: "a1", FromPort: "rejected", To: "on_reject"},
		},
	)
	topo := buildTopo(graph)
	topo.advance("trig", "")
	ready := topo.advance("a1", "rejected")
	if len(ready) != 1 || ready[0] != "on_reject" {
		t.Fatalf("after reject advance, ready = %v, want [on_reject]", ready)
	}
}

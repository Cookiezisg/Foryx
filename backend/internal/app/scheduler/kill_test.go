package scheduler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	approvaldomain "github.com/sunweilin/forgify/backend/internal/domain/approval"
	controldomain "github.com/sunweilin/forgify/backend/internal/domain/control"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

// fakeReconciler records which workflows the scheduler asked to settle their drain.
type fakeReconciler struct{ drained []string }

func (f *fakeReconciler) MarkRunAttention(_ context.Context, _ string, _ bool, _ string) error {
	return nil
}

func (f *fakeReconciler) MarkInactiveIfDrained(_ context.Context, workflowID string) error {
	f.drained = append(f.drained, workflowID)
	return nil
}

// TestDrainReconcile_FiresOnRunSettle: when a run reaches a terminal state and its workflow has no
// other runs in flight, the scheduler asks the LifecycleReconciler to settle the drain (the
// :deactivate→draining→inactive auto-flip). A one-node run completes → reconcile fires for wf_1.
func TestDrainReconcile_FiresOnRunSettle(t *testing.T) {
	g := workflowdomain.Graph{
		Nodes: []workflowdomain.Node{
			node("t", workflowdomain.NodeKindTrigger, "trg_1", nil),
			node("a", workflowdomain.NodeKindAction, "fn_1", nil),
		},
		Edges: []workflowdomain.Edge{edge("e", "t", "", "a")},
	}
	svc, store := mkSvc(t, g, newDisp(), nil, nil, "")
	recon := &fakeReconciler{}
	svc.SetLifecycleReconciler(recon)
	ctx := ctxWS("ws_1")

	runID := mustRun(t, svc, ctx, map[string]any{})
	assertRunStatus(t, store, ctx, runID, flowrundomain.StatusCompleted)
	if len(recon.drained) == 0 || recon.drained[len(recon.drained)-1] != "wf_1" {
		t.Fatalf("a settled run with 0 in-flight should reconcile drain for wf_1, got %v", recon.drained)
	}
}

// TestKillWorkflow_CancelsParkedRun: a run parked on an approval is StatusRunning but has no
// in-flight advance (the goroutine returned at the park). Kill marks it cancelled — the simple
// not-blocked path (cancelInflight is a no-op, the store write does the work).
func TestKillWorkflow_CancelsParkedRun(t *testing.T) {
	g := workflowdomain.Graph{
		Nodes: []workflowdomain.Node{
			node("t", workflowdomain.NodeKindTrigger, "trg_1", nil),
			node("ap", workflowdomain.NodeKindApproval, "apf_1", nil),
		},
		Edges: []workflowdomain.Edge{edge("e", "t", "", "ap")},
	}
	apf := &fakeApproval{byID: map[string]*approvaldomain.Version{"apf_1": {Template: "ok?"}}}
	svc, store := mkSvc(t, g, newDisp(), nil, apf, "")
	ctx := ctxWS("ws_1")

	runID := mustRun(t, svc, ctx, map[string]any{})
	assertRunStatus(t, store, ctx, runID, flowrundomain.StatusRunning) // parked → run stays running

	killed, err := svc.KillWorkflow(ctx, "wf_1")
	if err != nil {
		t.Fatalf("KillWorkflow: %v", err)
	}
	if killed != 1 {
		t.Fatalf("killed = %d, want 1", killed)
	}
	assertRunStatus(t, store, ctx, runID, flowrundomain.StatusCancelled)
}

// blockingAgentDispatcher's RunAgent signals that it entered, then blocks until its ctx is cancelled
// — modelling a long agent stuck mid-run. This is exactly the case kill must interrupt.
type blockingAgentDispatcher struct{ entered chan string }

func (d *blockingAgentDispatcher) RunAction(context.Context, string, string, map[string]any) (map[string]any, error) {
	return map[string]any{}, nil
}
func (d *blockingAgentDispatcher) RunAgent(ctx context.Context, ref, _ string, _ map[string]any) (map[string]any, error) {
	d.entered <- ref
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestKillWorkflow_InterruptsBlockedAgent is the core proof of the kill mechanism: a run blocked deep
// inside a long-running agent node is interrupted by KillWorkflow cancelling its registered ctx — the
// blocked advance returns, and the run lands cancelled (NOT failed, because kill marks cancelled
// before the ctx-cancel turns the agent's return into a node failure).
func TestKillWorkflow_InterruptsBlockedAgent(t *testing.T) {
	g := workflowdomain.Graph{
		Nodes: []workflowdomain.Node{
			node("t", workflowdomain.NodeKindTrigger, "trg_1", nil),
			node("a", workflowdomain.NodeKindAgent, "ag_1", nil),
		},
		Edges: []workflowdomain.Edge{edge("e", "t", "", "a")},
	}
	store, _ := newStore(t)
	raw, _ := json.Marshal(g)
	wf := &fakeWorkflows{
		wf:   &workflowdomain.Workflow{ID: "wf_1", Concurrency: workflowdomain.ConcurrencyAllowAll, ActiveVersionID: "wfv_1", LifecycleState: workflowdomain.LifecycleActive},
		ver:  &workflowdomain.Version{ID: "wfv_1", WorkflowID: "wf_1", Version: 1, Graph: string(raw)},
		pins: map[string]string{},
	}
	disp := &blockingAgentDispatcher{entered: make(chan string, 1)}
	svc := NewService(store, wf, &fakeControl{byID: map[string][]controldomain.Branch{}}, &fakeApproval{byID: map[string]*approvaldomain.Version{}}, disp, nil, nil)
	ctx := ctxWS("ws_1")

	done := make(chan struct{})
	go func() {
		_, _ = svc.StartRun(ctx, StartInput{WorkflowID: "wf_1", Payload: map[string]any{}})
		close(done)
	}()

	select {
	case <-disp.entered: // RunAgent started and is now blocking on its ctx
	case <-time.After(2 * time.Second):
		t.Fatal("RunAgent never entered — the run did not reach the agent node")
	}

	running, err := store.ListRunningByWorkflow(ctx, "wf_1")
	if err != nil {
		t.Fatalf("ListRunningByWorkflow: %v", err)
	}
	if len(running) != 1 {
		t.Fatalf("want 1 running run, got %d", len(running))
	}
	runID := running[0].ID

	killed, err := svc.KillWorkflow(ctx, "wf_1")
	if err != nil {
		t.Fatalf("KillWorkflow: %v", err)
	}
	if killed != 1 {
		t.Fatalf("killed = %d, want 1", killed)
	}

	select {
	case <-done: // StartRun's Advance returned — the blocked node was interrupted
	case <-time.After(2 * time.Second):
		t.Fatal("kill did not interrupt the blocked advance")
	}
	assertRunStatus(t, store, ctx, runID, flowrundomain.StatusCancelled)
}

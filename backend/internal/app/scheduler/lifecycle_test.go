package scheduler

import (
	"context"
	"testing"
	"time"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

// Drain on a clean stop waits for in-flight runs; on deadline it cancels them so they finalize
// (graceful shutdown — no running zombies left for boot-reconciliation). Here the in-flight run
// blocks on its ctx; Drain's deadline cancels it, the run unblocks, and Drain returns.
func TestDrain_CancelsInFlightOnDeadline(t *testing.T) {
	repo := newFakeRepo()
	s := newSvc(t, repo, &fakeWorkflowReader{wf: mkEnabledWorkflow(), ver: mkVersion()})
	started := make(chan struct{})
	s.ExecuteFn = func(ctx context.Context, _ *flowrundomain.FlowRun, _ *workflowdomain.Graph) {
		close(started)
		<-ctx.Done() // hold the run in-flight until Drain cancels it
	}
	if _, err := s.StartRun(ctxWith("u1"), "wf1", "manual", nil); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	<-started // the run is now in-flight (runWG == 1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	drained := make(chan struct{})
	go func() { s.Drain(ctx); close(drained) }()

	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("Drain must cancel the in-flight run on deadline and return")
	}
}

// Drain returns promptly when nothing is in-flight.
func TestDrain_NoInFlightReturnsImmediately(t *testing.T) {
	s := newSvc(t, newFakeRepo(), &fakeWorkflowReader{})
	drained := make(chan struct{})
	go func() { s.Drain(context.Background()); close(drained) }()
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("Drain must return immediately with no in-flight runs")
	}
}

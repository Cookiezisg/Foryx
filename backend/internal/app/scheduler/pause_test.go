package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

type pauseFakeRepo struct {
	*fakeRepo
	pausedStates map[string]*flowrundomain.PausedState
}

func newPauseFakeRepo() *pauseFakeRepo {
	return &pauseFakeRepo{
		fakeRepo:     newFakeRepo(),
		pausedStates: make(map[string]*flowrundomain.PausedState),
	}
}

func (r *pauseFakeRepo) SetPausedState(_ context.Context, runID string, ps *flowrundomain.PausedState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pausedStates[runID] = ps
	if run, ok := r.runs[runID]; ok {
		run.PausedState = ps
	}
	return nil
}

func (r *pauseFakeRepo) ClearPausedState(_ context.Context, runID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.pausedStates, runID)
	if run, ok := r.runs[runID]; ok {
		run.PausedState = nil
	}
	return nil
}

func (r *pauseFakeRepo) ListPaused(context.Context) ([]*flowrundomain.FlowRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*flowrundomain.FlowRun, 0)
	for _, run := range r.runs {
		if run.Status == flowrundomain.StatusPaused {
			out = append(out, run)
		}
	}
	return out, nil
}

func TestApprovalNode_PausesRun(t *testing.T) {
	repo := newPauseFakeRepo()
	graph := mkGraph(
		[]workflowdomain.NodeSpec{
			node("trig", workflowdomain.NodeTypeTrigger),
			node("approval", workflowdomain.NodeTypeApproval),
		},
		[]workflowdomain.EdgeSpec{edge("trig", "approval")},
	)
	reader := &fakeWorkflowReader{
		wf:  mkEnabledWorkflow(),
		ver: &workflowdomain.Version{ID: "wfv1", WorkflowID: "wf1", GraphParsed: graph},
	}
	s := newSvcWithPauseRepo(t, repo, reader)

	s.RouterRef().Set(workflowdomain.NodeTypeTrigger, DispatcherFunc(func(context.Context, DispatchInput) DispatchOutput {
		return DispatchOutput{}
	}))
	s.RouterRef().Set(workflowdomain.NodeTypeApproval, NewApprovalDispatcher())

	runID, err := s.StartRun(ctxWith("u1"), "wf1", "manual", nil)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	// Wait for paused state.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, _ := repo.Get(context.Background(), runID)
		if run.Status == flowrundomain.StatusPaused {
			if run.PausedState == nil {
				t.Fatalf("paused without PausedState")
			}
			if run.PausedState.NodeID != "approval" {
				t.Errorf("paused NodeID = %q, want approval", run.PausedState.NodeID)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run did not pause within 2s")
}

func TestResumeApproval_InvalidDecision(t *testing.T) {
	repo := newPauseFakeRepo()
	s := newSvcWithPauseRepo(t, repo, &fakeWorkflowReader{})

	err := s.ResumeApproval(context.Background(), "fr1", "approval", "maybe", "")
	if !errors.Is(err, flowrundomain.ErrApprovalDecisionInvalid) {
		t.Errorf("expected ErrApprovalDecisionInvalid, got %v", err)
	}
}

func TestResumeApproval_NotPaused(t *testing.T) {
	repo := newPauseFakeRepo()
	_ = repo.Create(context.Background(), mkRun("fr1", "u1", "wf1", flowrundomain.StatusRunning))
	s := newSvcWithPauseRepo(t, repo, &fakeWorkflowReader{})

	err := s.ResumeApproval(context.Background(), "fr1", "approval", "approved", "")
	if !errors.Is(err, flowrundomain.ErrNotPaused) {
		t.Errorf("expected ErrNotPaused, got %v", err)
	}
}

func TestResumeApproval_WrongNodeID(t *testing.T) {
	repo := newPauseFakeRepo()
	run := mkRun("fr1", "u1", "wf1", flowrundomain.StatusPaused)
	run.PausedState = &flowrundomain.PausedState{NodeID: "real_approval"}
	_ = repo.Create(context.Background(), run)

	s := newSvcWithPauseRepo(t, repo, &fakeWorkflowReader{})
	err := s.ResumeApproval(context.Background(), "fr1", "wrong_node", "approved", "")
	if !errors.Is(err, flowrundomain.ErrApprovalNodeNotFound) {
		t.Errorf("expected ErrApprovalNodeNotFound, got %v", err)
	}
}

func TestResumeApproval_EndToEnd_FinishesRun(t *testing.T) {
	repo := newPauseFakeRepo()
	graph := mkGraph(
		[]workflowdomain.NodeSpec{
			node("trig", workflowdomain.NodeTypeTrigger),
			node("approval", workflowdomain.NodeTypeApproval),
			node("after", workflowdomain.NodeTypeFunction),
		},
		[]workflowdomain.EdgeSpec{
			edge("trig", "approval"),
			// Approval is a branching node; FromPort selects which downstream
			// path this edge consumes ("approved" vs "rejected"). Without it
			// the scheduler parks the edge → after-node never runs.
			// approval 是分叉节点;FromPort 选 approved/rejected 分支,缺则
			// scheduler park 该边,下游永不跑。
			{ID: "e1", From: "approval", FromPort: "approved", To: "after"},
		},
	)
	reader := &fakeWorkflowReader{
		wf:  mkEnabledWorkflow(),
		ver: &workflowdomain.Version{ID: "wfv1", WorkflowID: "wf1", GraphParsed: graph},
	}
	s := newSvcWithPauseRepo(t, repo, reader)

	var afterCalled atomic.Int32
	s.RouterRef().Set(workflowdomain.NodeTypeTrigger, DispatcherFunc(func(context.Context, DispatchInput) DispatchOutput {
		return DispatchOutput{}
	}))
	s.RouterRef().Set(workflowdomain.NodeTypeApproval, NewApprovalDispatcher())
	s.RouterRef().Set(workflowdomain.NodeTypeFunction, DispatcherFunc(func(context.Context, DispatchInput) DispatchOutput {
		afterCalled.Add(1)
		return DispatchOutput{}
	}))

	runID, err := s.StartRun(ctxWith("u1"), "wf1", "manual", nil)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	// Wait for pause.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, _ := repo.Get(context.Background(), runID)
		if run.Status == flowrundomain.StatusPaused {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Resume with approved decision.
	if err := s.ResumeApproval(ctxWith("u1"), runID, "approval", "approved", "OK"); err != nil {
		t.Fatalf("ResumeApproval: %v", err)
	}

	// Wait for completion.
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, _ := repo.Get(context.Background(), runID)
		if run.Status == flowrundomain.StatusCompleted {
			if afterCalled.Load() != 1 {
				t.Errorf("after-approval node not called, count = %d", afterCalled.Load())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run did not complete after resume")
}

func TestRehydrateOnBoot_RegistersCancelForPaused(t *testing.T) {
	repo := newPauseFakeRepo()
	run := mkRun("fr1", "u1", "wf1", flowrundomain.StatusPaused)
	_ = repo.Create(context.Background(), run)

	s := newSvcWithPauseRepo(t, repo, &fakeWorkflowReader{})
	if err := s.RehydrateOnBoot(context.Background(), "u1"); err != nil {
		t.Fatalf("RehydrateOnBoot: %v", err)
	}
	// Cancel should now find the runID — returns nil (no error).
	// Cancel 应找到 runID — 返 nil。
	if err := s.Cancel(context.Background(), "fr1"); err != nil {
		t.Errorf("Cancel after rehydrate: %v", err)
	}
}

func TestRehydrateOnBoot_IgnoresRunningRuns(t *testing.T) {
	repo := newPauseFakeRepo()
	_ = repo.Create(context.Background(), mkRun("fr_run", "u1", "wf1", flowrundomain.StatusRunning))
	_ = repo.Create(context.Background(), mkRun("fr_done", "u1", "wf1", flowrundomain.StatusCompleted))

	s := newSvcWithPauseRepo(t, repo, &fakeWorkflowReader{})
	_ = s.RehydrateOnBoot(context.Background(), "u1")
	// Cancel of running/done run should not be registered.
	if err := s.Cancel(context.Background(), "fr_run"); !errors.Is(err, flowrundomain.ErrNotCancellable) {
		t.Errorf("running run should not have cancel registered after rehydrate, got %v", err)
	}
}


func newSvcWithPauseRepo(t *testing.T, repo *pauseFakeRepo, reader *fakeWorkflowReader) *Service {
	t.Helper()
	s := newSvc(t, repo.fakeRepo, reader)
	// Re-bind repo to the pause-aware wrapper so SetPausedState +
	// ListPaused override paths are exercised.
	// 把 repo 重绑到 pause-aware wrapper(让 SetPausedState/ListPaused
	// 走 override 路径)。
	s.repo = repo
	return s
}

func mkRun(id, userID, workflowID, status string) *flowrundomain.FlowRun {
	return &flowrundomain.FlowRun{
		ID:           id,
		UserID:       userID,
		WorkflowID:   workflowID,
		VersionID:    "wfv1",
		TriggerKind:  flowrundomain.TriggerKindManual,
		TriggerInput: map[string]any{},
		Status:       status,
		StartedAt:    time.Now().UTC(),
	}
}

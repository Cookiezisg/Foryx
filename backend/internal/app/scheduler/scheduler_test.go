package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

type fakeRepo struct {
	mu       sync.Mutex
	runs     map[string]*flowrundomain.FlowRun
	running  int
	createErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{runs: make(map[string]*flowrundomain.FlowRun)}
}

func (r *fakeRepo) Create(_ context.Context, run *flowrundomain.FlowRun) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.mu.Lock()
	r.runs[run.ID] = run
	r.mu.Unlock()
	return nil
}

func (r *fakeRepo) Get(_ context.Context, id string) (*flowrundomain.FlowRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run, ok := r.runs[id]; ok {
		return run, nil
	}
	return nil, flowrundomain.ErrNotFound
}

func (r *fakeRepo) List(context.Context, flowrundomain.ListFilter) ([]*flowrundomain.FlowRun, string, error) {
	return nil, "", nil
}

func (r *fakeRepo) UpdateStatus(_ context.Context, runID, status string, _ any, _, _ string, _ *time.Time, _ int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run, ok := r.runs[runID]; ok {
		run.Status = status
		return nil
	}
	return flowrundomain.ErrNotFound
}

func (r *fakeRepo) SetPausedState(context.Context, string, *flowrundomain.PausedState) error {
	return nil
}
func (r *fakeRepo) ClearPausedState(context.Context, string) error { return nil }
func (r *fakeRepo) ListPaused(context.Context) ([]*flowrundomain.FlowRun, error) {
	return nil, nil
}

func (r *fakeRepo) CountRunning(_ context.Context, _ string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running, nil
}

func (r *fakeRepo) HardDeleteOldest(context.Context, string, int) error { return nil }

func (r *fakeRepo) CreateNode(context.Context, *flowrundomain.Node) error { return nil }
func (r *fakeRepo) GetNode(_ context.Context, id string) (*flowrundomain.Node, error) {
	return nil, flowrundomain.ErrNodeNotFound
}
func (r *fakeRepo) ListNodes(context.Context, flowrundomain.NodeFilter) ([]*flowrundomain.Node, string, error) {
	return nil, "", nil
}

type fakeWorkflowReader struct {
	wf       *workflowdomain.Workflow
	ver      *workflowdomain.Version
	getErr   error
	verErr   error
}

func (f *fakeWorkflowReader) GetActiveVersion(context.Context, string) (*workflowdomain.Version, error) {
	if f.verErr != nil {
		return nil, f.verErr
	}
	return f.ver, nil
}
func (f *fakeWorkflowReader) GetWorkflow(context.Context, string) (*workflowdomain.Workflow, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.wf, nil
}
func (f *fakeWorkflowReader) ListEnabled(context.Context) ([]*workflowdomain.Workflow, error) {
	return nil, nil
}

func newSvc(t *testing.T, repo *fakeRepo, reader *fakeWorkflowReader) *Service {
	t.Helper()
	log := zaptest.NewLogger(t)
	notif := notificationspkg.New(nil, log)
	return NewService(repo, reader, notif, log)
}

func ctxWith(userID string) context.Context {
	return reqctxpkg.SetUserID(context.Background(), userID)
}

func mkEnabledWorkflow() *workflowdomain.Workflow {
	return &workflowdomain.Workflow{
		ID: "wf1", UserID: "u1", Name: "wf",
		Enabled:        true,
		Concurrency:    workflowdomain.ConcurrencySerial,
		NeedsAttention: false,
	}
}

func mkVersion() *workflowdomain.Version {
	return &workflowdomain.Version{
		ID: "wfv1", WorkflowID: "wf1",
		GraphParsed: &workflowdomain.Graph{Name: "wf"},
	}
}

func TestStartRun_MissingUserID(t *testing.T) {
	s := newSvc(t, newFakeRepo(), &fakeWorkflowReader{wf: mkEnabledWorkflow(), ver: mkVersion()})
	_, err := s.StartRun(context.Background(), "wf1", "manual", nil)
	if err == nil {
		t.Fatalf("expected missing-uid error, got nil")
	}
}

func TestStartRun_WorkflowNotFound(t *testing.T) {
	s := newSvc(t, newFakeRepo(), &fakeWorkflowReader{getErr: workflowdomain.ErrNotFound})
	_, err := s.StartRun(ctxWith("u1"), "wf1", "manual", nil)
	if !errors.Is(err, ErrWorkflowNotFound) {
		t.Errorf("expected ErrWorkflowNotFound, got %v", err)
	}
}

func TestStartRun_Disabled(t *testing.T) {
	wf := mkEnabledWorkflow()
	wf.Enabled = false
	s := newSvc(t, newFakeRepo(), &fakeWorkflowReader{wf: wf, ver: mkVersion()})

	_, err := s.StartRun(ctxWith("u1"), "wf1", "manual", nil)
	if !errors.Is(err, ErrWorkflowDisabled) {
		t.Errorf("expected ErrWorkflowDisabled, got %v", err)
	}
}

func TestStartRun_NeedsAttention(t *testing.T) {
	wf := mkEnabledWorkflow()
	wf.NeedsAttention = true
	s := newSvc(t, newFakeRepo(), &fakeWorkflowReader{wf: wf, ver: mkVersion()})

	_, err := s.StartRun(ctxWith("u1"), "wf1", "manual", nil)
	if !errors.Is(err, ErrWorkflowNeedsAttention) {
		t.Errorf("expected ErrWorkflowNeedsAttention, got %v", err)
	}
}

func TestStartRun_SerialConcurrencyLimit(t *testing.T) {
	repo := newFakeRepo()
	repo.running = 1
	s := newSvc(t, repo, &fakeWorkflowReader{wf: mkEnabledWorkflow(), ver: mkVersion()})

	_, err := s.StartRun(ctxWith("u1"), "wf1", "cron", nil)
	if !errors.Is(err, ErrConcurrencyLimit) {
		t.Errorf("expected ErrConcurrencyLimit, got %v", err)
	}
}

func TestStartRun_HappyPath_CallsExecuteFn(t *testing.T) {
	repo := newFakeRepo()
	s := newSvc(t, repo, &fakeWorkflowReader{wf: mkEnabledWorkflow(), ver: mkVersion()})

	var executed atomic.Int32
	executedCh := make(chan struct{}, 1)
	s.ExecuteFn = func(_ context.Context, _ *flowrundomain.FlowRun, _ *workflowdomain.Graph) {
		executed.Add(1)
		executedCh <- struct{}{}
	}

	runID, err := s.StartRun(ctxWith("u1"), "wf1", "manual", map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if runID == "" {
		t.Errorf("empty runID")
	}

	select {
	case <-executedCh:
	case <-time.After(time.Second):
		t.Fatalf("ExecuteFn not called within 1s")
	}
	if executed.Load() != 1 {
		t.Errorf("ExecuteFn called %d times", executed.Load())
	}

	// Run should be persisted.
	run, _ := repo.Get(context.Background(), runID)
	if run == nil {
		t.Fatalf("run not persisted")
	}
	if run.WorkflowID != "wf1" || run.TriggerKind != "manual" {
		t.Errorf("run fields wrong: %+v", run)
	}
}

func TestStartRun_StubExecute_FinalizesCompleted(t *testing.T) {
	repo := newFakeRepo()
	s := newSvc(t, repo, &fakeWorkflowReader{wf: mkEnabledWorkflow(), ver: mkVersion()})

	runID, err := s.StartRun(ctxWith("u1"), "wf1", "manual", nil)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	// Stub ExecuteFn (default) finalizes immediately.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		run, _ := repo.Get(context.Background(), runID)
		if run.Status == flowrundomain.StatusCompleted {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("stub execute didn't finalize run as completed")
}

func TestCancel_UnknownRun_ReturnsNotCancellable(t *testing.T) {
	s := newSvc(t, newFakeRepo(), &fakeWorkflowReader{})
	err := s.Cancel(context.Background(), "fr_missing")
	if !errors.Is(err, flowrundomain.ErrNotCancellable) {
		t.Errorf("expected ErrNotCancellable, got %v", err)
	}
}

func TestCancel_CancelsRunningCtx(t *testing.T) {
	repo := newFakeRepo()
	s := newSvc(t, repo, &fakeWorkflowReader{wf: mkEnabledWorkflow(), ver: mkVersion()})

	released := make(chan struct{})
	s.ExecuteFn = func(ctx context.Context, _ *flowrundomain.FlowRun, _ *workflowdomain.Graph) {
		<-ctx.Done()
		close(released)
	}
	runID, _ := s.StartRun(ctxWith("u1"), "wf1", "manual", nil)

	if err := s.Cancel(context.Background(), runID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatalf("ExecuteFn ctx not cancelled within 1s")
	}
}

func TestExecuteFn_PanicRecover_FinalizesFailed(t *testing.T) {
	repo := newFakeRepo()
	s := newSvc(t, repo, &fakeWorkflowReader{wf: mkEnabledWorkflow(), ver: mkVersion()})

	s.ExecuteFn = func(context.Context, *flowrundomain.FlowRun, *workflowdomain.Graph) {
		panic("boom")
	}

	runID, _ := s.StartRun(ctxWith("u1"), "wf1", "manual", nil)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		run, _ := repo.Get(context.Background(), runID)
		if run.Status == flowrundomain.StatusFailed {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("panic recover did not finalize run as failed")
}

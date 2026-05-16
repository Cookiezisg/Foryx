package trigger

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"go.uber.org/zap/zaptest"

	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
)

type fakeScheduler struct {
	mu    sync.Mutex
	calls []struct {
		WorkflowID  string
		TriggerKind string
		Input       map[string]any
	}
	startErr error
}

func (f *fakeScheduler) StartRun(_ context.Context, workflowID, triggerKind string, input map[string]any) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return "", f.startErr
	}
	f.calls = append(f.calls, struct {
		WorkflowID  string
		TriggerKind string
		Input       map[string]any
	}{workflowID, triggerKind, input})
	return "fr_fake", nil
}

func (f *fakeScheduler) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func newTestService(t *testing.T) (*Service, *fakeScheduler) {
	t.Helper()
	sched := &fakeScheduler{}
	mux := http.NewServeMux()
	s := New(mux, zaptest.NewLogger(t))
	s.SetScheduler(sched)
	t.Cleanup(s.Shutdown)
	return s, sched
}

func TestRegister_Manual_NoListener(t *testing.T) {
	s, _ := newTestService(t)

	if err := s.RegisterTrigger(triggerdomain.Spec{
		WorkflowID: "wf1", NodeID: "trig1",
		Kind: triggerdomain.KindManual,
	}); err != nil {
		t.Fatalf("Register manual: %v", err)
	}
	states := s.State("wf1")
	if len(states) != 1 || states[0].Status != triggerdomain.StateIdle {
		t.Errorf("manual State: %+v", states)
	}
}

func TestRegister_CronInvalid_TracksSpec_ButErrors(t *testing.T) {
	s, _ := newTestService(t)

	err := s.RegisterTrigger(triggerdomain.Spec{
		WorkflowID: "wf1", NodeID: "trig1",
		Kind:   triggerdomain.KindCron,
		Config: map[string]any{"expression": "not a cron"},
	})
	if !errors.Is(err, triggerdomain.ErrInvalidCronExpression) {
		t.Errorf("expected ErrInvalidCronExpression, got %v", err)
	}
	if got := s.State("wf1"); len(got) != 1 {
		t.Errorf("Spec not tracked after fail: %+v", got)
	}
}

func TestUnregisterByWorkflow_ClearsAllKinds(t *testing.T) {
	s, _ := newTestService(t)

	_ = s.RegisterTrigger(triggerdomain.Spec{
		WorkflowID: "wf1", NodeID: "n1",
		Kind:   triggerdomain.KindCron,
		Config: map[string]any{"expression": "0 0 * * *"},
	})
	_ = s.RegisterTrigger(triggerdomain.Spec{
		WorkflowID: "wf1", NodeID: "n2",
		Kind: triggerdomain.KindManual,
	})
	_ = s.RegisterTrigger(triggerdomain.Spec{
		WorkflowID: "wf2", NodeID: "n3",
		Kind: triggerdomain.KindManual,
	})

	s.UnregisterByWorkflow("wf1")
	if got := s.State("wf1"); len(got) != 0 {
		t.Errorf("wf1 not cleared: %+v", got)
	}
	if got := s.State("wf2"); len(got) != 1 {
		t.Errorf("wf2 affected: %+v", got)
	}
}

func TestFireManual_ForwardsToScheduler(t *testing.T) {
	s, sched := newTestService(t)

	runID, err := s.FireManual(context.Background(), "wf_abc",
		map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("FireManual: %v", err)
	}
	if runID != "fr_fake" {
		t.Errorf("runID = %q, want fr_fake", runID)
	}
	if sched.callCount() != 1 {
		t.Errorf("scheduler call count = %d, want 1", sched.callCount())
	}
}

func TestFireManual_NoScheduler_ReturnsError(t *testing.T) {
	mux := http.NewServeMux()
	s := New(mux, zaptest.NewLogger(t))
	defer s.Shutdown()

	_, err := s.FireManual(context.Background(), "wf_abc", nil)
	if !errors.Is(err, ErrSchedulerNotAttached) {
		t.Errorf("expected ErrSchedulerNotAttached, got %v", err)
	}
}

func TestSetScheduler_ThreadSafe(t *testing.T) {
	mux := http.NewServeMux()
	s := New(mux, zaptest.NewLogger(t))
	defer s.Shutdown()

	var wg sync.WaitGroup
	var setOK atomic.Int32
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.SetScheduler(&fakeScheduler{})
			setOK.Add(1)
		}()
	}
	wg.Wait()
	if setOK.Load() != 10 {
		t.Errorf("concurrent SetScheduler: %d", setOK.Load())
	}
}

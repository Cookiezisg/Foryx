package cron

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
)

func TestRegister_InvalidExpressionReturnsSentinel(t *testing.T) {
	l := New(zaptest.NewLogger(t), func(string, string, map[string]any) {})
	err := l.Register(triggerdomain.Spec{
		WorkflowID: "wf1",
		NodeID:     "trig1",
		Kind:       triggerdomain.KindCron,
		Config:     map[string]any{"expression": "not a cron"},
	})
	if !errors.Is(err, triggerdomain.ErrInvalidCronExpression) {
		t.Errorf("expected ErrInvalidCronExpression, got %v", err)
	}
}

func TestRegister_EmptyExpressionReturnsSentinel(t *testing.T) {
	l := New(zaptest.NewLogger(t), func(string, string, map[string]any) {})
	err := l.Register(triggerdomain.Spec{
		WorkflowID: "wf1",
		NodeID:     "trig1",
		Kind:       triggerdomain.KindCron,
		Config:     map[string]any{},
	})
	if !errors.Is(err, triggerdomain.ErrInvalidCronExpression) {
		t.Errorf("expected ErrInvalidCronExpression, got %v", err)
	}
}

func TestRegisterAndFire(t *testing.T) {
	var fired atomic.Int32
	l := New(zaptest.NewLogger(t), func(string, string, map[string]any) {
		fired.Add(1)
	})
	defer l.Stop()
	l.Start()

	if err := l.Register(triggerdomain.Spec{
		WorkflowID: "wf1",
		NodeID:     "trig1",
		Kind:       triggerdomain.KindCron,
		Config:     map[string]any{"expression": "@every 1s"},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Wait up to 2.5s for at least one fire.
	deadline := time.Now().Add(2500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if fired.Load() > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("cron did not fire within 2.5s; count = %d", fired.Load())
}

func TestUnregister_StopsFiring(t *testing.T) {
	var fired atomic.Int32
	l := New(zaptest.NewLogger(t), func(string, string, map[string]any) {
		fired.Add(1)
	})
	defer l.Stop()
	l.Start()

	_ = l.Register(triggerdomain.Spec{
		WorkflowID: "wf1",
		NodeID:     "trig1",
		Kind:       triggerdomain.KindCron,
		Config:     map[string]any{"expression": "@every 1s"},
	})
	l.Unregister("wf1", "trig1")
	time.Sleep(1500 * time.Millisecond)
	if fired.Load() != 0 {
		t.Errorf("expected 0 fires after Unregister, got %d", fired.Load())
	}
}

func TestState_BeforeAndAfterRegister(t *testing.T) {
	l := New(zaptest.NewLogger(t), func(string, string, map[string]any) {})
	defer l.Stop()
	l.Start()

	st := l.State("wf1", "trig1")
	if st.Status != triggerdomain.StateIdle {
		t.Errorf("pre-register status = %q, want idle", st.Status)
	}
	if st.LastFiredAt != nil || st.NextFireAt != nil {
		t.Errorf("expected nil time pointers before register")
	}

	_ = l.Register(triggerdomain.Spec{
		WorkflowID: "wf1",
		NodeID:     "trig1",
		Kind:       triggerdomain.KindCron,
		Config:     map[string]any{"expression": "0 0 * * *"},
	})
	st = l.State("wf1", "trig1")
	if st.Status != triggerdomain.StateActive {
		t.Errorf("post-register status = %q, want active", st.Status)
	}
	if st.NextFireAt == nil {
		t.Errorf("NextFireAt nil after register")
	}
}

func TestRegister_ReplacesExistingEntry(t *testing.T) {
	var mu sync.Mutex
	calls := []string{}
	l := New(zaptest.NewLogger(t), func(_, nodeID string, _ map[string]any) {
		mu.Lock()
		calls = append(calls, nodeID)
		mu.Unlock()
	})
	defer l.Stop()
	l.Start()

	spec := triggerdomain.Spec{
		WorkflowID: "wf1",
		NodeID:     "trig1",
		Kind:       triggerdomain.KindCron,
		Config:     map[string]any{"expression": "@every 1s"},
	}
	if err := l.Register(spec); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	// Re-register same key with different (still-valid) spec.
	spec.Config["expression"] = "0 0 * * *" // daily, won't fire in test window
	if err := l.Register(spec); err != nil {
		t.Fatalf("second Register: %v", err)
	}
	// Confirm only one entry exists by state lookup.
	st := l.State("wf1", "trig1")
	if st.Status != triggerdomain.StateActive {
		t.Errorf("post-replace status = %q", st.Status)
	}
}

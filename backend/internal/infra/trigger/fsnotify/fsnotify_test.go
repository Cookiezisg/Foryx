package fsnotify

import (
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
)

func TestRegister_PathNotExist_ReturnsSentinel(t *testing.T) {
	l := New(zaptest.NewLogger(t), func(string, string, map[string]any) {})
	defer l.Stop()

	err := l.Register(triggerdomain.Spec{
		WorkflowID: "wf1",
		NodeID:     "trig1",
		Kind:       triggerdomain.KindFsnotify,
		Config:     map[string]any{"path": "/nonexistent/forgify-test-path"},
	})
	if !errors.Is(err, triggerdomain.ErrPathNotExist) {
		t.Errorf("expected ErrPathNotExist, got %v", err)
	}
	st := l.State("wf1", "trig1")
	if st.Status != triggerdomain.StateError {
		t.Errorf("expected state=error, got %q", st.Status)
	}
	if st.LastError == "" {
		t.Errorf("expected non-empty LastError")
	}
}

func TestRegister_EmptyPath_ReturnsSentinel(t *testing.T) {
	l := New(zaptest.NewLogger(t), func(string, string, map[string]any) {})
	defer l.Stop()

	err := l.Register(triggerdomain.Spec{
		WorkflowID: "wf1",
		NodeID:     "trig1",
		Kind:       triggerdomain.KindFsnotify,
		Config:     map[string]any{},
	})
	if !errors.Is(err, triggerdomain.ErrPathNotExist) {
		t.Errorf("expected ErrPathNotExist, got %v", err)
	}
}

func TestRegisterAndFire_OnCreate(t *testing.T) {
	dir := t.TempDir()
	var fired atomic.Int32
	l := New(zaptest.NewLogger(t), func(string, string, map[string]any) {
		fired.Add(1)
	})
	defer l.Stop()

	if err := l.Register(triggerdomain.Spec{
		WorkflowID: "wf1",
		NodeID:     "trig1",
		Kind:       triggerdomain.KindFsnotify,
		Config: map[string]any{
			"path":   dir,
			"events": []any{"create"},
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Create a file under dir.
	target := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Wait up to 2s.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fired.Load() > 0 {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Errorf("did not fire within 2s; count = %d", fired.Load())
}

func TestPatternFilter(t *testing.T) {
	dir := t.TempDir()
	var matched atomic.Int32
	l := New(zaptest.NewLogger(t), func(_, _ string, _ map[string]any) {
		matched.Add(1)
	})
	defer l.Stop()

	if err := l.Register(triggerdomain.Spec{
		WorkflowID: "wf1",
		NodeID:     "trig1",
		Kind:       triggerdomain.KindFsnotify,
		Config: map[string]any{
			"path":    dir,
			"pattern": "*.csv",
			"events":  []any{"create"},
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Non-matching file (txt) and matching (csv).
	_ = os.WriteFile(filepath.Join(dir, "skip.txt"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "data.csv"), []byte("x"), 0o644)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if matched.Load() > 0 {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	// Give it a tiny tail to ensure no extra .txt firings flush in.
	time.Sleep(150 * time.Millisecond)
	if got := matched.Load(); got != 1 {
		t.Errorf("expected exactly 1 fire (csv), got %d", got)
	}
}

func TestUnregister_StopsFiring(t *testing.T) {
	dir := t.TempDir()
	var fired atomic.Int32
	l := New(zaptest.NewLogger(t), func(string, string, map[string]any) {
		fired.Add(1)
	})
	defer l.Stop()

	_ = l.Register(triggerdomain.Spec{
		WorkflowID: "wf1",
		NodeID:     "trig1",
		Kind:       triggerdomain.KindFsnotify,
		Config: map[string]any{
			"path":   dir,
			"events": []any{"create"},
		},
	})
	l.Unregister("wf1", "trig1")

	_ = os.WriteFile(filepath.Join(dir, "after.txt"), []byte("x"), 0o644)
	time.Sleep(300 * time.Millisecond)
	if fired.Load() != 0 {
		t.Errorf("expected 0 fires after Unregister, got %d", fired.Load())
	}
}

func TestState_PreRegisterIdle(t *testing.T) {
	l := New(zaptest.NewLogger(t), func(string, string, map[string]any) {})
	defer l.Stop()
	st := l.State("wf1", "trig1")
	if st.Status != triggerdomain.StateIdle {
		t.Errorf("pre-register status = %q, want idle", st.Status)
	}
}

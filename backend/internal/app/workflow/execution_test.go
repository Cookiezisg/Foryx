package workflow

import (
	"context"
	"errors"
	"slices"
	"testing"

	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

// --- fake execution ports --------------------------------------------------

type fakeBinder struct {
	attach     []string // "triggerID|workflowID"
	attachOnce []string
	detach     []string
}

func (f *fakeBinder) Attach(_ context.Context, t, w string) error {
	f.attach = append(f.attach, t+"|"+w)
	return nil
}
func (f *fakeBinder) AttachOnce(_ context.Context, t, w string) error {
	f.attachOnce = append(f.attachOnce, t+"|"+w)
	return nil
}
func (f *fakeBinder) Detach(t, w string) { f.detach = append(f.detach, t+"|"+w) }

type fakeRunner struct {
	started []string
	killed  []string
	running int
}

func (f *fakeRunner) StartRun(_ context.Context, w string, _ map[string]any) (string, error) {
	f.started = append(f.started, w)
	return "fr_test", nil
}
func (f *fakeRunner) KillWorkflow(_ context.Context, w string) (int, error) {
	f.killed = append(f.killed, w)
	return 3, nil
}
func (f *fakeRunner) CountRunning(_ context.Context, _ string) (int, error) { return f.running, nil }

// TestExecutionLifecycle drives all five D1 actions over a real Service + in-memory store with fake
// binder/runner, asserting each engages the right collaborator and lands the right lifecycle state.
// The workflow's entry trigger ref is "trg_a" (linearOps).
func TestExecutionLifecycle(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	binder, runner := &fakeBinder{}, &fakeRunner{}
	svc.SetExecutionPorts(binder, runner)

	w, _, err := svc.Create(ctx, CreateInput{Name: "pipe", Ops: linearOps(t)})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	wf := w.ID
	key := "trg_a|" + wf

	// activate → Attach(trg_a) + lifecycle active
	got, err := svc.Activate(ctx, wf)
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if !slices.Contains(binder.attach, key) {
		t.Fatalf("activate did not attach %q: %v", key, binder.attach)
	}
	if !got.Active || got.LifecycleState != workflowdomain.LifecycleActive {
		t.Fatalf("activate lifecycle wrong: active=%v state=%q", got.Active, got.LifecycleState)
	}

	// stage on an active workflow → ErrAlreadyActive (no attach-once)
	if err := svc.Stage(ctx, wf); !errors.Is(err, workflowdomain.ErrAlreadyActive) {
		t.Fatalf("Stage on active: want ErrAlreadyActive, got %v", err)
	}

	// deactivate (no runs in flight) → Detach + inactive
	got, err = svc.Deactivate(ctx, wf)
	if err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	if !slices.Contains(binder.detach, key) {
		t.Fatalf("deactivate did not detach %q: %v", key, binder.detach)
	}
	if got.Active || got.LifecycleState != workflowdomain.LifecycleInactive {
		t.Fatalf("deactivate lifecycle wrong: active=%v state=%q", got.Active, got.LifecycleState)
	}

	// stage now (inactive) → AttachOnce
	if err := svc.Stage(ctx, wf); err != nil {
		t.Fatalf("Stage after deactivate: %v", err)
	}
	if !slices.Contains(binder.attachOnce, key) {
		t.Fatalf("stage did not attach-once %q: %v", key, binder.attachOnce)
	}

	// trigger → StartRun, returns the flowrun id
	runID, err := svc.Trigger(ctx, wf, map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if runID != "fr_test" || !slices.Contains(runner.started, wf) {
		t.Fatalf("trigger wrong: runID=%q started=%v", runID, runner.started)
	}

	// kill → Detach + KillWorkflow (returns count) + inactive
	killed, err := svc.Kill(ctx, wf)
	if err != nil {
		t.Fatalf("Kill: %v", err)
	}
	if killed != 3 || !slices.Contains(runner.killed, wf) {
		t.Fatalf("kill wrong: killed=%d list=%v", killed, runner.killed)
	}
}

// TestDeactivateDrainsWhenRunsInFlight: deactivating a workflow that still has running runs lands in
// draining (the scheduler later flips it to inactive when the last run settles), not inactive.
func TestDeactivateDrainsWhenRunsInFlight(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	binder, runner := &fakeBinder{}, &fakeRunner{running: 2}
	svc.SetExecutionPorts(binder, runner)

	w, _, err := svc.Create(ctx, CreateInput{Name: "pipe", Ops: linearOps(t)})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Activate(ctx, w.ID); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	got, err := svc.Deactivate(ctx, w.ID)
	if err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	if got.LifecycleState != workflowdomain.LifecycleDraining {
		t.Fatalf("deactivate with runs in flight: want draining, got %q", got.LifecycleState)
	}
}

// TestExecutionPortsUnwired: the five actions surface a clean error (not a nil panic) when the
// execution ports were never installed.
func TestExecutionPortsUnwired(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	w, _, err := svc.Create(ctx, CreateInput{Name: "pipe", Ops: linearOps(t)})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Trigger(ctx, w.ID, nil); !errors.Is(err, errExecUnavailable) {
		t.Fatalf("Trigger unwired: want errExecUnavailable, got %v", err)
	}
	if _, err := svc.Activate(ctx, w.ID); !errors.Is(err, errExecUnavailable) {
		t.Fatalf("Activate unwired: want errExecUnavailable, got %v", err)
	}
}

// TestEditRevert_RebindLiveListener: editing or reverting an ACTIVE workflow whose entry
// trigger ref changed must re-point the live binding — old ref detached, new ref attached.
// Without this the old trigger fires the workflow forever and the new one is never heard.
// An inactive workflow's edit must NOT touch the binder.
//
// TestEditRevert_RebindLiveListener：编辑/回退 ACTIVE workflow 且入口 trigger ref 变了，必须
// 重指活监听——旧 ref 卸、新 ref 挂。否则旧 trigger 永远触发本 workflow、新的无人听。
// 非 active 的编辑不得碰 binder。
func TestEditRevert_RebindLiveListener(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	binder, runner := &fakeBinder{}, &fakeRunner{}
	svc.SetExecutionPorts(binder, runner)

	w, _, err := svc.Create(ctx, CreateInput{Name: "pipe", Ops: linearOps(t)})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Activate(ctx, w.ID); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	// edit swaps the entry ref trg_a → trg_b on the live workflow
	swap := opsJSON(t, `[{"op":"update_node","id":"t","patch":{"ref":"trg_b"}}]`)
	if _, err := svc.Edit(ctx, EditInput{ID: w.ID, Ops: swap}); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if !slices.Contains(binder.detach, "trg_a|"+w.ID) {
		t.Fatalf("edit did not detach old ref: %v", binder.detach)
	}
	if !slices.Contains(binder.attach, "trg_b|"+w.ID) {
		t.Fatalf("edit did not attach new ref: %v", binder.attach)
	}

	// revert to v1 swaps back trg_b → trg_a
	if _, err := svc.Revert(ctx, w.ID, 1); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	if !slices.Contains(binder.detach, "trg_b|"+w.ID) {
		t.Fatalf("revert did not detach trg_b: %v", binder.detach)
	}
	if c := countOf(binder.attach, "trg_a|"+w.ID); c != 2 { // activate + revert-rebind
		t.Fatalf("revert did not re-attach trg_a (attach=%v)", binder.attach)
	}

	// inactive workflow: edit must not touch the binder
	if _, err := svc.Deactivate(ctx, w.ID); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	nDetach, nAttach := len(binder.detach), len(binder.attach)
	swap2 := opsJSON(t, `[{"op":"update_node","id":"t","patch":{"ref":"trg_c"}}]`)
	if _, err := svc.Edit(ctx, EditInput{ID: w.ID, Ops: swap2}); err != nil {
		t.Fatalf("Edit (inactive): %v", err)
	}
	if len(binder.detach) != nDetach || len(binder.attach) != nAttach {
		t.Fatalf("inactive edit touched binder: detach=%v attach=%v", binder.detach, binder.attach)
	}
}

func countOf(xs []string, want string) int {
	n := 0
	for _, x := range xs {
		if x == want {
			n++
		}
	}
	return n
}

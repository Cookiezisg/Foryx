// Package scheduler retry tests cover the withRetry / dispatchWithPolicies / nextDelay helpers.
// These helpers belong to the loop-body dispatch path (runReadyLoop → dispatchBatch →
// dispatchWithPolicies → withRetry), which is still production code for loop / parallel node
// types that route through subdag.go. They are DISTINCT from the top-level interpreter path
// (New(journal, dispatch).Run) which dispatches activities directly without retry wrapping.
// Both paths are production code; these tests cover the loop-body layer specifically.

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

func newExecCtxForRetry() *ExecutionContext {
	return &ExecutionContext{
		Run:       &flowrundomain.FlowRun{ID: "fr1"},
		Variables: map[string]any{},
		Outputs:   make(map[string]map[string]any),
		Done:      make(map[string]bool),
		Failed:    make(map[string]string),
		Attempts:  make(map[string]int),
		NextPort:  make(map[string]string),
	}
}

func TestWithRetry_NoConfig_SingleAttempt(t *testing.T) {
	var calls atomic.Int32
	node := workflowdomain.NodeSpec{ID: "n", Type: workflowdomain.NodeTypeFunction}
	out := withRetry(context.Background(), node, newExecCtxForRetry(), func(context.Context) DispatchOutput {
		calls.Add(1)
		return DispatchOutput{Error: errors.New("boom")}
	})
	if calls.Load() != 1 {
		t.Errorf("attempts = %d, want 1", calls.Load())
	}
	if out.Error == nil {
		t.Errorf("expected error returned through")
	}
}

func TestWithRetry_MaxAttempts3_AllFail(t *testing.T) {
	var calls atomic.Int32
	node := workflowdomain.NodeSpec{
		ID: "n", Type: workflowdomain.NodeTypeFunction,
		Retry: &workflowdomain.RetryConfig{MaxAttempts: 3, DelayMs: 1, Backoff: "fixed"},
	}
	ec := newExecCtxForRetry()
	out := withRetry(context.Background(), node, ec, func(context.Context) DispatchOutput {
		calls.Add(1)
		return DispatchOutput{Error: errors.New("transient")}
	})
	if calls.Load() != 3 {
		t.Errorf("attempts = %d, want 3", calls.Load())
	}
	if out.Error == nil || out.Error.Error() != "transient" {
		t.Errorf("expected last error to surface, got %v", out.Error)
	}
	if ec.Attempts["n"] != 3 {
		t.Errorf("ExecCtx.Attempts = %d, want 3", ec.Attempts["n"])
	}
}

func TestWithRetry_SuccessOnSecondAttempt(t *testing.T) {
	var calls atomic.Int32
	node := workflowdomain.NodeSpec{
		ID: "n", Type: workflowdomain.NodeTypeFunction,
		Retry: &workflowdomain.RetryConfig{MaxAttempts: 5, DelayMs: 1, Backoff: "fixed"},
	}
	out := withRetry(context.Background(), node, newExecCtxForRetry(), func(context.Context) DispatchOutput {
		if calls.Add(1) == 1 {
			return DispatchOutput{Error: errors.New("first try fail")}
		}
		return DispatchOutput{Outputs: map[string]any{"out": "ok"}}
	})
	if calls.Load() != 2 {
		t.Errorf("attempts = %d, want 2", calls.Load())
	}
	if out.Error != nil {
		t.Errorf("expected success after retry, got %v", out.Error)
	}
}

func TestWithRetry_FatalErrorShortCircuits(t *testing.T) {
	var calls atomic.Int32
	node := workflowdomain.NodeSpec{
		ID: "n", Type: workflowdomain.NodeTypeApproval,
		Retry: &workflowdomain.RetryConfig{MaxAttempts: 5, DelayMs: 1},
	}
	out := withRetry(context.Background(), node, newExecCtxForRetry(), func(context.Context) DispatchOutput {
		calls.Add(1)
		return DispatchOutput{Error: ErrApprovalRequired}
	})
	if calls.Load() != 1 {
		t.Errorf("approval-required must short-circuit, got %d attempts", calls.Load())
	}
	if !errors.Is(out.Error, ErrApprovalRequired) {
		t.Errorf("error not preserved: %v", out.Error)
	}
}

func TestWithRetry_CtxCancelShortCircuits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel
	var calls atomic.Int32
	node := workflowdomain.NodeSpec{
		ID: "n", Type: workflowdomain.NodeTypeFunction,
		Retry: &workflowdomain.RetryConfig{MaxAttempts: 5, DelayMs: 1},
	}
	out := withRetry(ctx, node, newExecCtxForRetry(), func(context.Context) DispatchOutput {
		calls.Add(1)
		return DispatchOutput{Error: errors.New("never reached")}
	})
	if calls.Load() != 0 {
		t.Errorf("cancelled ctx should short-circuit, got %d attempts", calls.Load())
	}
	if !errors.Is(out.Error, context.Canceled) {
		t.Errorf("expected Canceled, got %v", out.Error)
	}
}

func TestNextDelay_Exponential(t *testing.T) {
	got := nextDelay("exponential", 100*time.Millisecond,
		&workflowdomain.RetryConfig{DelayMs: 100})
	if got != 200*time.Millisecond {
		t.Errorf("exponential: got %v, want 200ms", got)
	}
}

func TestNextDelay_Linear(t *testing.T) {
	got := nextDelay("linear", 100*time.Millisecond,
		&workflowdomain.RetryConfig{DelayMs: 100})
	if got != 200*time.Millisecond {
		t.Errorf("linear: got %v, want 200ms", got)
	}
}

func TestNextDelay_Fixed(t *testing.T) {
	got := nextDelay("fixed", 100*time.Millisecond,
		&workflowdomain.RetryConfig{DelayMs: 100})
	if got != 100*time.Millisecond {
		t.Errorf("fixed: got %v, want 100ms", got)
	}
}

func TestNodeTimeoutDuration_OverrideWins(t *testing.T) {
	d := nodeTimeoutDuration(workflowdomain.NodeSpec{
		Type: workflowdomain.NodeTypeFunction, Timeout: 500,
	})
	if d != 500*time.Millisecond {
		t.Errorf("override = %v, want 500ms", d)
	}
}

func TestNodeTimeoutDuration_DefaultByType(t *testing.T) {
	// Pre-existing red (predates M1): the durable revamp removed per-type business
	// timeout defaults (00 mechanism-vs-policy — "无 timeout/retry 等任何业务默认值";
	// not filled = not applied). nodeTimeoutDuration now returns 0 for all types.
	// This test + the old topo-walk scheduler are deleted in M2 (ADR-016).
	t.Skip("per-type timeout defaults removed by durable revamp; old scheduler + this test deleted in M2")
	cases := []struct {
		typ  string
		want time.Duration
	}{
		{workflowdomain.NodeTypeFunction, 30 * time.Second},
		{workflowdomain.NodeTypeLLM, 60 * time.Second},
		{workflowdomain.NodeTypeApproval, 7 * 24 * time.Hour},
		{workflowdomain.NodeTypeCondition, 0},
		{workflowdomain.NodeTypeVariable, 0},
	}
	for _, c := range cases {
		got := nodeTimeoutDuration(workflowdomain.NodeSpec{Type: c.typ})
		if got != c.want {
			t.Errorf("type %q: got %v, want %v", c.typ, got, c.want)
		}
	}
}

func TestDispatchWithPolicies_TimeoutWraps(t *testing.T) {
	repo := newFakeRepo()
	s := newSvc(t, repo, &fakeWorkflowReader{wf: mkEnabledWorkflow(), ver: mkVersion()})

	// Register a slow dispatcher that exceeds the override timeout.
	// 注册一个慢 dispatcher,超过 override timeout。
	s.RouterRef().Set(workflowdomain.NodeTypeFunction, DispatcherFunc(func(ctx context.Context, _ DispatchInput) DispatchOutput {
		select {
		case <-time.After(200 * time.Millisecond):
			return DispatchOutput{}
		case <-ctx.Done():
			return DispatchOutput{Error: ctx.Err()}
		}
	}))

	out := s.dispatchWithPolicies(context.Background(),
		workflowdomain.NodeSpec{ID: "n", Type: workflowdomain.NodeTypeFunction, Timeout: 50},
		nil, newExecCtxForRetry())
	if !errors.Is(out.Error, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", out.Error)
	}
}

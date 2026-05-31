package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
)

// TestRunReadyLoop_DeadlineExceeded_MapsToRunTimeout covers §5.7: ctx hit DeadlineExceeded → status=failed + RUN_TIMEOUT.
//
// TestRunReadyLoop_DeadlineExceeded_MapsToRunTimeout 覆盖 §5.7：ctx 超时 → failed + RUN_TIMEOUT。
func TestRunReadyLoop_DeadlineExceeded_MapsToRunTimeout(t *testing.T) {
	repo := &fakeFlowRunRepo{}
	svc := NewService(repo, nil, notificationspkg.New(nil, zap.NewNop()), zap.NewNop())
	run := mkRunForLoopTest()
	graph := &workflowdomain.Graph{
		Nodes: []workflowdomain.NodeSpec{
			{ID: "n1", Type: workflowdomain.NodeTypeVariable, Config: map[string]any{"operation": "set", "name": "x", "value": 1}},
		},
	}
	execCtx := newExecutionContext(run, graph)
	topo := buildTopo(graph)

	// Pre-cancelled ctx with DeadlineExceeded.
	// 预先 timeout 过期的 ctx。
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(2 * time.Millisecond)

	status, errCode, errMsg, paused := svc.runReadyLoop(ctx, run, execCtx, topo, topo.initialReady())
	if paused {
		t.Fatal("unexpected paused")
	}
	if status != flowrundomain.StatusFailed {
		t.Errorf("status = %q, want failed", status)
	}
	if errCode != "RUN_TIMEOUT" {
		t.Errorf("errCode = %q, want RUN_TIMEOUT", errCode)
	}
	if !strings.Contains(errMsg, "deadline") {
		t.Errorf("errMsg = %q, want contains 'deadline'", errMsg)
	}
}

// TestRunReadyLoop_Cancelled_MapsToCancelled covers explicit cancel (not timeout) path stays as StatusCancelled.
//
// TestRunReadyLoop_Cancelled_MapsToCancelled 覆盖显式 cancel（非 timeout）保持 StatusCancelled。
func TestRunReadyLoop_Cancelled_MapsToCancelled(t *testing.T) {
	repo := &fakeFlowRunRepo{}
	svc := NewService(repo, nil, notificationspkg.New(nil, zap.NewNop()), zap.NewNop())
	run := mkRunForLoopTest()
	graph := &workflowdomain.Graph{
		Nodes: []workflowdomain.NodeSpec{
			{ID: "n1", Type: workflowdomain.NodeTypeVariable, Config: map[string]any{"operation": "set", "name": "x", "value": 1}},
		},
	}
	execCtx := newExecutionContext(run, graph)
	topo := buildTopo(graph)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // explicit cancel before run

	status, errCode, _, _ := svc.runReadyLoop(ctx, run, execCtx, topo, topo.initialReady())
	if status != flowrundomain.StatusCancelled {
		t.Errorf("status = %q, want cancelled", status)
	}
	if errCode != "" {
		t.Errorf("errCode = %q, want empty (cancel != timeout)", errCode)
	}
}

// TestDispatchWithPolicies_DryRun_MocksFunction covers dry-run: function node returns synthetic output.
//
// TestDispatchWithPolicies_DryRun_MocksFunction 覆盖 dry-run：function 节点返合成 output。
func TestDispatchWithPolicies_DryRun_MocksFunction(t *testing.T) {
	repo := &fakeFlowRunRepo{}
	svc := NewService(repo, nil, notificationspkg.New(nil, zap.NewNop()), zap.NewNop())
	router := NewRouter()
	called := false
	router.Set(workflowdomain.NodeTypeFunction, DispatcherFunc(func(_ context.Context, _ DispatchInput) DispatchOutput {
		called = true
		return DispatchOutput{Outputs: map[string]any{"real": "side-effect"}}
	}))
	svc.SetRouter(router)

	run := mkRunForLoopTest()
	run.DryRun = true
	graph := &workflowdomain.Graph{Nodes: []workflowdomain.NodeSpec{
		{ID: "f1", Type: workflowdomain.NodeTypeFunction, Config: map[string]any{"functionId": "fn_x"}},
	}}
	execCtx := newExecutionContext(run, graph)

	out := svc.dispatchWithPolicies(context.Background(), graph.Nodes[0], nil, execCtx)
	if called {
		t.Error("real function dispatcher was called in dry-run mode; should be mocked")
	}
	if out.Outputs["_dryRun"] != true {
		t.Errorf("_dryRun flag missing: %v", out.Outputs)
	}
	if got, _ := out.Outputs["out"].(string); !strings.Contains(got, "DRY RUN") {
		t.Errorf("out = %v, want contains DRY RUN", out.Outputs["out"])
	}
}

// TestDispatchWithPolicies_DryRun_PreservesPureLogic: condition / variable etc still execute normally.
//
// TestDispatchWithPolicies_DryRun_PreservesPureLogic：condition / variable 等纯逻辑节点正常跑。
func TestDispatchWithPolicies_DryRun_PreservesPureLogic(t *testing.T) {
	repo := &fakeFlowRunRepo{}
	svc := NewService(repo, nil, notificationspkg.New(nil, zap.NewNop()), zap.NewNop())
	router := NewRouter()
	called := false
	router.Set(workflowdomain.NodeTypeVariable, DispatcherFunc(func(_ context.Context, _ DispatchInput) DispatchOutput {
		called = true
		return DispatchOutput{Outputs: map[string]any{"variable": "set"}}
	}))
	svc.SetRouter(router)

	run := mkRunForLoopTest()
	run.DryRun = true
	graph := &workflowdomain.Graph{Nodes: []workflowdomain.NodeSpec{
		{ID: "v1", Type: workflowdomain.NodeTypeVariable, Config: map[string]any{}},
	}}
	execCtx := newExecutionContext(run, graph)

	svc.dispatchWithPolicies(context.Background(), graph.Nodes[0], nil, execCtx)
	if !called {
		t.Error("variable dispatcher should run in dry-run (pure logic)")
	}
}

// TestDispatchWithPolicies_DryRun_ApprovalAutoApproves: approval node returns the "yes" port (canon).
//
// TestDispatchWithPolicies_DryRun_ApprovalAutoApproves：approval 节点返 "yes" 端口(canon)。
func TestDispatchWithPolicies_DryRun_ApprovalAutoApproves(t *testing.T) {
	repo := &fakeFlowRunRepo{}
	svc := NewService(repo, nil, notificationspkg.New(nil, zap.NewNop()), zap.NewNop())
	router := NewRouter()
	router.Set(workflowdomain.NodeTypeApproval, DispatcherFunc(func(_ context.Context, _ DispatchInput) DispatchOutput {
		t.Error("real approval dispatcher should NOT be called in dry-run")
		return DispatchOutput{Error: ErrApprovalRequired}
	}))
	svc.SetRouter(router)

	run := mkRunForLoopTest()
	run.DryRun = true
	graph := &workflowdomain.Graph{Nodes: []workflowdomain.NodeSpec{
		{ID: "a1", Type: workflowdomain.NodeTypeApproval, Config: map[string]any{}},
	}}
	execCtx := newExecutionContext(run, graph)

	out := svc.dispatchWithPolicies(context.Background(), graph.Nodes[0], nil, execCtx)
	if errors.Is(out.Error, ErrApprovalRequired) {
		t.Error("approval should be skipped in dry-run; got ErrApprovalRequired")
	}
	if out.NextPort != "yes" {
		t.Errorf("NextPort = %q, want yes", out.NextPort)
	}
}

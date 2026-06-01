package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
)

var errTestFail = errors.New("test: deliberate failure")

// mkLoopTestService spins a Service with a Router that knows `loop` + `variable` for body subgraphs.
//
// mkLoopTestService 构造 Service：Router 含 loop + variable 两个 NodeType（body 用 variable 当 stub）。
func mkLoopTestService(t *testing.T, repo flowrundomain.Repository) *Service {
	t.Helper()
	svc := NewService(repo, nil, notificationspkg.New(nil, zap.NewNop()), zap.NewNop())
	router := NewRouter()
	router.Set(workflowdomain.NodeTypeLoop, NewLoopDispatcher(svc))
	router.Set(workflowdomain.NodeTypeVariable, NewVariableDispatcher())
	svc.SetRouter(router)
	return svc
}

// fakeFlowRunRepo satisfies flowrundomain.Repository for body-iteration tests; only records nodes used.
//
// fakeFlowRunRepo 实现 flowrundomain.Repository，仅 body 迭代测试用；只记 CreateNode。
type fakeFlowRunRepo struct {
	nodes []*flowrundomain.Node
}

func (r *fakeFlowRunRepo) CreateNode(_ context.Context, n *flowrundomain.Node) error {
	r.nodes = append(r.nodes, n)
	return nil
}

func (r *fakeFlowRunRepo) Create(context.Context, *flowrundomain.FlowRun) error  { return nil }
func (r *fakeFlowRunRepo) Get(context.Context, string) (*flowrundomain.FlowRun, error) {
	return nil, nil
}
func (r *fakeFlowRunRepo) List(context.Context, flowrundomain.ListFilter) ([]*flowrundomain.FlowRun, string, error) {
	return nil, "", nil
}
func (r *fakeFlowRunRepo) UpdateStatus(context.Context, string, string, any, string, string, *time.Time, int64) error {
	return nil
}
func (r *fakeFlowRunRepo) ClaimStatus(context.Context, string, string, string) (bool, error) {
	return true, nil
}
func (r *fakeFlowRunRepo) BumpGeneration(context.Context, string) (int, error) { return 1, nil }
func (r *fakeFlowRunRepo) CountRunning(context.Context, string) (int, error)   { return 0, nil }
func (r *fakeFlowRunRepo) SetPausedState(context.Context, string, *flowrundomain.PausedState) error {
	return nil
}
func (r *fakeFlowRunRepo) GetPausedState(context.Context, string) (*flowrundomain.PausedState, error) {
	return nil, nil
}
func (r *fakeFlowRunRepo) ClearPausedState(context.Context, string) error                { return nil }
func (r *fakeFlowRunRepo) ListPaused(context.Context) ([]*flowrundomain.FlowRun, error)  { return nil, nil }
func (r *fakeFlowRunRepo) ListNodes(context.Context, flowrundomain.NodeFilter) ([]*flowrundomain.Node, string, error) {
	return nil, "", nil
}
func (r *fakeFlowRunRepo) GetNode(context.Context, string) (*flowrundomain.Node, error) { return nil, nil }
func (r *fakeFlowRunRepo) HardDeleteOldest(context.Context, string, int) error          { return nil }
func (r *fakeFlowRunRepo) SoftDeleteByWorkflow(context.Context, string) error           { return nil }

func mkRunForLoopTest() *flowrundomain.FlowRun {
	return &flowrundomain.FlowRun{
		ID:        "fr_loop",
		UserID:    "u-loop",
		StartedAt: time.Now().UTC(),
		Status:    flowrundomain.StatusRunning,
	}
}

func mkLoopInput(node workflowdomain.NodeSpec, svc *Service, repo *fakeFlowRunRepo) DispatchInput {
	run := mkRunForLoopTest()
	graph := &workflowdomain.Graph{Nodes: []workflowdomain.NodeSpec{node}}
	execCtx := newExecutionContext(run, graph)
	_ = repo
	_ = svc
	return DispatchInput{
		Node:    node,
		NodeIn:  map[string]any{},
		ExecCtx: execCtx,
	}
}

func TestLoopBody_SimpleForeach_Sequential(t *testing.T) {
	repo := &fakeFlowRunRepo{}
	svc := mkLoopTestService(t, repo)
	d := NewLoopDispatcher(svc)
	node := workflowdomain.NodeSpec{
		ID:   "loop1",
		Type: workflowdomain.NodeTypeLoop,
		Config: map[string]any{
			"items": []any{"red", "green", "blue"},
			"body": map[string]any{
				"nodes": []any{
					map[string]any{
						"id":   "v1",
						"type": "variable",
						"config": map[string]any{
							"operation": "set",
							"name":      "current",
							"value":     "{{ .loop.item }}-{{ .loop.index }}",
						},
					},
				},
				"edges": []any{},
			},
		},
	}
	out := d.Dispatch(context.Background(), mkLoopInput(node, svc, repo))
	if out.Error != nil {
		t.Fatalf("Dispatch err: %v", out.Error)
	}
	if out.Outputs["count"] != 3 {
		t.Errorf("count = %v, want 3", out.Outputs["count"])
	}
	if out.Outputs["successes"] != 3 {
		t.Errorf("successes = %v, want 3", out.Outputs["successes"])
	}
	if len(repo.nodes) != 3 {
		t.Errorf("recorded %d body-node executions, want 3", len(repo.nodes))
	}
	// Each recorded body node carries iteration index 0/1/2 and parent loop ID "loop1".
	// 每个 body node 记录 iteration 0/1/2 + ParentLoopNode = loop1。
	seen := map[int]bool{}
	for _, n := range repo.nodes {
		if n.ParentLoopNode != "loop1" {
			t.Errorf("ParentLoopNode = %q, want loop1", n.ParentLoopNode)
		}
		seen[n.IterationIndex] = true
	}
	if !seen[0] || !seen[1] || !seen[2] {
		t.Errorf("missing iteration indices: %v", seen)
	}
}

func TestLoopBody_FailFast_StopsOnFirstError(t *testing.T) {
	repo := &fakeFlowRunRepo{}
	svc := NewService(repo, nil, notificationspkg.New(nil, zap.NewNop()), zap.NewNop())
	router := NewRouter()
	router.Set(workflowdomain.NodeTypeLoop, NewLoopDispatcher(svc))
	// failing dispatcher: always returns error.
	// 故意失败 dispatcher。
	router.Set("alwaysfail", DispatcherFunc(func(_ context.Context, _ DispatchInput) DispatchOutput {
		return DispatchOutput{Error: errTestFail} // any error
	}))
	svc.SetRouter(router)
	d := NewLoopDispatcher(svc)

	node := workflowdomain.NodeSpec{
		ID:   "loop1",
		Type: workflowdomain.NodeTypeLoop,
		Config: map[string]any{
			"items": []any{"a", "b", "c"},
			"body": map[string]any{
				"nodes": []any{
					map[string]any{"id": "x", "type": "alwaysfail", "config": map[string]any{}},
				},
				"edges": []any{},
			},
		},
	}
	out := d.Dispatch(context.Background(), mkLoopInput(node, svc, repo))
	if out.Error == nil {
		t.Fatalf("expected fail-fast error, got nil")
	}
	// Sequential + fail-fast: should stop after first iteration.
	// 顺序 + fail-fast：第一轮就停。
	if len(repo.nodes) != 1 {
		t.Errorf("recorded %d body-node executions, want 1 (fail-fast)", len(repo.nodes))
	}
}

func TestLoopBody_OnErrorContinue_CollectsFailures(t *testing.T) {
	repo := &fakeFlowRunRepo{}
	svc := NewService(repo, nil, notificationspkg.New(nil, zap.NewNop()), zap.NewNop())
	router := NewRouter()
	router.Set(workflowdomain.NodeTypeLoop, NewLoopDispatcher(svc))
	router.Set("alwaysfail", DispatcherFunc(func(_ context.Context, _ DispatchInput) DispatchOutput {
		return DispatchOutput{Error: errTestFail}
	}))
	svc.SetRouter(router)
	d := NewLoopDispatcher(svc)

	node := workflowdomain.NodeSpec{
		ID:   "loop1",
		Type: workflowdomain.NodeTypeLoop,
		Config: map[string]any{
			"items":   []any{"a", "b"},
			"onError": "continue",
			"body": map[string]any{
				"nodes": []any{
					map[string]any{"id": "x", "type": "alwaysfail", "config": map[string]any{}},
				},
				"edges": []any{},
			},
		},
	}
	out := d.Dispatch(context.Background(), mkLoopInput(node, svc, repo))
	if out.Error != nil {
		t.Fatalf("unexpected error with continue: %v", out.Error)
	}
	failures, _ := out.Outputs["failures"].([]map[string]any)
	if len(failures) != 2 {
		t.Errorf("failures = %d, want 2", len(failures))
	}
	if out.Outputs["successes"] != 0 {
		t.Errorf("successes = %v, want 0", out.Outputs["successes"])
	}
}

func TestLoopBody_ApprovalRejected(t *testing.T) {
	repo := &fakeFlowRunRepo{}
	svc := mkLoopTestService(t, repo)
	d := NewLoopDispatcher(svc)
	node := workflowdomain.NodeSpec{
		ID:   "loop1",
		Type: workflowdomain.NodeTypeLoop,
		Config: map[string]any{
			"items": []any{"a"},
			"body": map[string]any{
				"nodes": []any{
					map[string]any{"id": "approve1", "type": "approval", "config": map[string]any{"prompt": "ok?"}},
				},
				"edges": []any{},
			},
		},
	}
	out := d.Dispatch(context.Background(), mkLoopInput(node, svc, repo))
	if out.Error == nil {
		t.Fatalf("expected approval rejection error, got nil")
	}
}

func TestSubstituteLoopTemplates_NestedStrings(t *testing.T) {
	evalCtx := workflowapp.EvalContext{
		Loop: &workflowapp.LoopContext{Item: "X", Index: 7},
	}
	cfg := map[string]any{
		"key":   "{{ .loop.item }}-{{ .loop.index }}",
		"plain": "literal",
		"nested": map[string]any{
			"inner": "{{ .loop.item }}",
		},
		"list": []any{"{{ .loop.index }}", "static"},
	}
	out, err := SubstituteLoopTemplates(cfg, evalCtx)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out["key"] != "X-7" {
		t.Errorf("key = %v, want X-7", out["key"])
	}
	if out["plain"] != "literal" {
		t.Errorf("plain = %v, want literal", out["plain"])
	}
	if inner, _ := out["nested"].(map[string]any); inner["inner"] != "X" {
		t.Errorf("nested.inner = %v, want X", inner["inner"])
	}
	if lst, _ := out["list"].([]any); lst[0] != "7" || lst[1] != "static" {
		t.Errorf("list = %v, want [7 static]", lst)
	}
}

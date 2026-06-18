package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite"

	approvaldomain "github.com/sunweilin/anselm/backend/internal/domain/approval"
	controldomain "github.com/sunweilin/anselm/backend/internal/domain/control"
	flowrundomain "github.com/sunweilin/anselm/backend/internal/domain/flowrun"
	triggerdomain "github.com/sunweilin/anselm/backend/internal/domain/trigger"
	workflowdomain "github.com/sunweilin/anselm/backend/internal/domain/workflow"
	flowrunstore "github.com/sunweilin/anselm/backend/internal/infra/store/flowrun"
	triggerstore "github.com/sunweilin/anselm/backend/internal/infra/store/trigger"
	ormpkg "github.com/sunweilin/anselm/backend/internal/pkg/orm"
	reqctxpkg "github.com/sunweilin/anselm/backend/internal/pkg/reqctx"
)

// ---- fakes ---------------------------------------------------------------

// fakeDispatcher counts calls per ref and returns {n: callCount} by default (so a looped node's
// result changes each turn). failRefs makes a ref error (exhausted-retry simulation).
type fakeDispatcher struct {
	actionCalls map[string]int
	agentCalls  map[string]int
	failRefs    map[string]bool
	actionPins  map[string]string // ref → last pinnedVersionID received
	agentPins   map[string]string
}

func newDisp() *fakeDispatcher {
	return &fakeDispatcher{
		actionCalls: map[string]int{}, agentCalls: map[string]int{}, failRefs: map[string]bool{},
		actionPins: map[string]string{}, agentPins: map[string]string{},
	}
}
func (d *fakeDispatcher) RunAction(_ context.Context, ref, pin string, _ map[string]any) (map[string]any, error) {
	d.actionCalls[ref]++
	d.actionPins[ref] = pin
	if d.failRefs[ref] {
		return nil, errors.New("action exploded")
	}
	return map[string]any{"n": d.actionCalls[ref]}, nil
}
func (d *fakeDispatcher) RunAgent(_ context.Context, ref, pin string, _ map[string]any) (map[string]any, error) {
	d.agentCalls[ref]++
	d.agentPins[ref] = pin
	return map[string]any{"out": "agent-" + ref}, nil
}

type fakeWorkflows struct {
	wf   *workflowdomain.Workflow
	ver  *workflowdomain.Version
	pins map[string]string
}

func (f *fakeWorkflows) GetWorkflow(context.Context, string) (*workflowdomain.Workflow, error) {
	return f.wf, nil
}
func (f *fakeWorkflows) GetActiveVersion(context.Context, string) (*workflowdomain.Version, error) {
	return f.ver, nil
}
func (f *fakeWorkflows) GetVersion(context.Context, string) (*workflowdomain.Version, error) {
	return f.ver, nil
}
func (f *fakeWorkflows) BuildPinClosure(context.Context, *workflowdomain.Graph) (map[string]string, error) {
	return f.pins, nil
}

type fakeControl struct {
	byID map[string][]controldomain.Branch
}

func (f *fakeControl) Resolve(_ context.Context, id, _ string) ([]controldomain.Branch, error) {
	return f.byID[id], nil
}

type fakeApproval struct {
	byID map[string]*approvaldomain.Version
}

func (f *fakeApproval) Resolve(_ context.Context, id, _ string) (*approvaldomain.Version, error) {
	return f.byID[id], nil
}

// ---- harness -------------------------------------------------------------

func newStore(t *testing.T) (*flowrunstore.Store, *sql.DB) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	schemas := append([]string{}, flowrunstore.Schema...)
	schemas = append(schemas, triggerstore.Schema...) // firing path's claim tx spans both
	for _, stmt := range schemas {
		if _, err := sqlDB.Exec(stmt); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	return flowrunstore.New(ormpkg.Open(sqlDB)), sqlDB
}

func ctxWS(id string) context.Context { return reqctxpkg.SetWorkspaceID(context.Background(), id) }

func node(id, kind, ref string, input map[string]string) workflowdomain.Node {
	return workflowdomain.Node{ID: id, Kind: kind, Ref: ref, Input: input}
}
func edge(id, from, port, to string) workflowdomain.Edge {
	return workflowdomain.Edge{ID: id, From: from, FromPort: port, To: to}
}

func mkSvc(t *testing.T, g workflowdomain.Graph, disp *fakeDispatcher, ctl *fakeControl, apf *fakeApproval, concurrency string) (*Service, *flowrunstore.Store) {
	t.Helper()
	store, _ := newStore(t)
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal graph: %v", err)
	}
	if concurrency == "" {
		concurrency = workflowdomain.ConcurrencyAllowAll
	}
	wf := &fakeWorkflows{
		wf:   &workflowdomain.Workflow{ID: "wf_1", Concurrency: concurrency, ActiveVersionID: "wfv_1", LifecycleState: workflowdomain.LifecycleActive},
		ver:  &workflowdomain.Version{ID: "wfv_1", WorkflowID: "wf_1", Version: 1, Graph: string(raw)},
		pins: map[string]string{},
	}
	if ctl == nil {
		ctl = &fakeControl{byID: map[string][]controldomain.Branch{}}
	}
	if apf == nil {
		apf = &fakeApproval{byID: map[string]*approvaldomain.Version{}}
	}
	return NewService(store, wf, ctl, apf, disp, nil, nil), store
}

func mustRun(t *testing.T, svc *Service, ctx context.Context, payload map[string]any) string {
	t.Helper()
	id, err := svc.StartRun(ctx, StartInput{WorkflowID: "wf_1", Payload: payload})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	return id
}

func assertRunStatus(t *testing.T, store *flowrunstore.Store, ctx context.Context, id, want string) {
	t.Helper()
	run, err := store.GetRun(ctx, id)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != want {
		t.Fatalf("run status = %q, want %q", run.Status, want)
	}
}

func nodeRows(t *testing.T, store *flowrunstore.Store, ctx context.Context, id string) map[string]*flowrundomain.FlowRunNode {
	t.Helper()
	rows, err := store.GetNodes(ctx, id)
	if err != nil {
		t.Fatalf("GetNodes: %v", err)
	}
	out := map[string]*flowrundomain.FlowRunNode{}
	for _, r := range rows {
		out[r.NodeID] = r // last iteration wins for presence checks
	}
	return out
}

// ---- walk: the four control-flow shapes ----------------------------------

func TestWalk_Linear(t *testing.T) {
	g := workflowdomain.Graph{
		Nodes: []workflowdomain.Node{
			node("start", "trigger", "trg_1", nil),
			node("a", "action", "fn_a", map[string]string{"x": "start.v"}),
			node("b", "action", "fn_b", map[string]string{"y": "a.n"}),
		},
		Edges: []workflowdomain.Edge{edge("e1", "start", "", "a"), edge("e2", "a", "", "b")},
	}
	disp := newDisp()
	svc, store := mkSvc(t, g, disp, nil, nil, "")
	ctx := ctxWS("ws_1")
	id := mustRun(t, svc, ctx, map[string]any{"v": "hi"})

	assertRunStatus(t, store, ctx, id, flowrundomain.StatusCompleted)
	if disp.actionCalls["fn_a"] != 1 || disp.actionCalls["fn_b"] != 1 {
		t.Fatalf("linear dispatch counts: %+v", disp.actionCalls)
	}
}

// CR-19: the run's pin closure must reach the dispatch port — function/agent nodes execute the
// version frozen at run start, not whatever is active at dispatch time.
//
// CR-19：run 的 pin 闭包必须传到派发口——function/agent 节点执行 run 启动时冻结的版本，而非派发
// 时刻的 active 版本。
func TestDispatch_PinnedVersionsReachPort(t *testing.T) {
	g := workflowdomain.Graph{
		Nodes: []workflowdomain.Node{
			node("start", "trigger", "trg_1", nil),
			node("a", "action", "fn_a", map[string]string{"x": "start.v"}),
			node("b", "agent", "ag_b", map[string]string{"y": "a.n"}),
		},
		Edges: []workflowdomain.Edge{edge("e1", "start", "", "a"), edge("e2", "a", "", "b")},
	}
	disp := newDisp()
	store, _ := newStore(t)
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal graph: %v", err)
	}
	wf := &fakeWorkflows{
		wf:   &workflowdomain.Workflow{ID: "wf_1", Concurrency: workflowdomain.ConcurrencyAllowAll, ActiveVersionID: "wfv_1", LifecycleState: workflowdomain.LifecycleActive},
		ver:  &workflowdomain.Version{ID: "wfv_1", WorkflowID: "wf_1", Version: 1, Graph: string(raw)},
		pins: map[string]string{"fn_a": "fnv_frozen", "ag_b": "agv_frozen"},
	}
	svc := NewService(store, wf, &fakeControl{byID: map[string][]controldomain.Branch{}}, &fakeApproval{byID: map[string]*approvaldomain.Version{}}, disp, nil, nil)
	ctx := ctxWS("ws_1")
	id := mustRun(t, svc, ctx, map[string]any{"v": "hi"})

	assertRunStatus(t, store, ctx, id, flowrundomain.StatusCompleted)
	if disp.actionPins["fn_a"] != "fnv_frozen" {
		t.Fatalf("function pin not threaded to dispatch: got %q", disp.actionPins["fn_a"])
	}
	if disp.agentPins["ag_b"] != "agv_frozen" {
		t.Fatalf("agent pin not threaded to dispatch: got %q", disp.agentPins["ag_b"])
	}
}

func TestWalk_ParallelAndJoin(t *testing.T) {
	g := workflowdomain.Graph{
		Nodes: []workflowdomain.Node{
			node("start", "trigger", "trg_1", nil),
			node("a", "action", "fn_a", map[string]string{"x": "start.v"}),
			node("b", "action", "fn_b", map[string]string{"x": "start.v"}),
			node("c", "action", "fn_c", map[string]string{"p": "a.n", "q": "b.n"}), // needs BOTH → AND-join
		},
		Edges: []workflowdomain.Edge{
			edge("e1", "start", "", "a"), edge("e2", "start", "", "b"),
			edge("e3", "a", "", "c"), edge("e4", "b", "", "c"),
		},
	}
	disp := newDisp()
	svc, store := mkSvc(t, g, disp, nil, nil, "")
	ctx := ctxWS("ws_1")
	id := mustRun(t, svc, ctx, map[string]any{"v": "x"})

	// c completing proves it saw both a.n and b.n (its input CEL would error otherwise → run failed).
	assertRunStatus(t, store, ctx, id, flowrundomain.StatusCompleted)
	if disp.actionCalls["fn_c"] != 1 {
		t.Fatalf("AND-join c ran %d times", disp.actionCalls["fn_c"])
	}
}

func TestWalk_ControlXOR_SimpleMerge(t *testing.T) {
	g := workflowdomain.Graph{
		Nodes: []workflowdomain.Node{
			node("start", "trigger", "trg_1", nil),
			node("gate", "control", "ctl_1", map[string]string{"v": "start.v"}),
			node("p", "action", "fn_p", map[string]string{"x": "gate.out"}),
			node("e", "action", "fn_e", map[string]string{"x": "gate.out"}),
			node("m", "action", "fn_m", map[string]string{"r": "gate.out"}), // simple-merge of p/e
		},
		Edges: []workflowdomain.Edge{
			edge("e1", "start", "", "gate"),
			edge("e2", "gate", "pass", "p"),
			edge("e3", "gate", "else", "e"),
			edge("e4", "p", "", "m"),
			edge("e5", "e", "", "m"),
		},
	}
	ctl := &fakeControl{byID: map[string][]controldomain.Branch{
		"ctl_1": {
			{Port: "pass", When: `input.v == "go"`, Emit: map[string]string{"out": "input.v"}},
			{Port: "else", When: "true", Emit: map[string]string{"out": "input.v"}},
		},
	}}
	disp := newDisp()
	svc, store := mkSvc(t, g, disp, ctl, nil, "")
	ctx := ctxWS("ws_1")
	id := mustRun(t, svc, ctx, map[string]any{"v": "go"})

	assertRunStatus(t, store, ctx, id, flowrundomain.StatusCompleted)
	if disp.actionCalls["fn_p"] != 1 {
		t.Fatalf("pass branch p should run once, got %d", disp.actionCalls["fn_p"])
	}
	if disp.actionCalls["fn_e"] != 0 {
		t.Fatalf("pruned else branch e must NOT run, got %d", disp.actionCalls["fn_e"])
	}
	if disp.actionCalls["fn_m"] != 1 {
		t.Fatalf("simple-merge m should run once (waiting only on live p), got %d", disp.actionCalls["fn_m"])
	}
}

func TestWalk_LoopWithBackEdge(t *testing.T) {
	g := workflowdomain.Graph{
		Nodes: []workflowdomain.Node{
			node("start", "trigger", "trg_1", nil),
			node("draft", "action", "fn_draft", map[string]string{"seed": "start.v"}),
			node("gate", "control", "ctl_loop", map[string]string{"n": "draft.n"}),
			node("publish", "action", "fn_pub", map[string]string{"out": "gate.n"}),
		},
		Edges: []workflowdomain.Edge{
			edge("e1", "start", "", "draft"),
			edge("e2", "draft", "", "gate"),
			edge("e3", "gate", "done", "publish"),
			edge("e4", "gate", "retry", "draft"), // back edge (control → ancestor)
		},
	}
	ctl := &fakeControl{byID: map[string][]controldomain.Branch{
		"ctl_loop": {
			{Port: "done", When: "input.n >= 2", Emit: map[string]string{"n": "input.n"}},
			{Port: "retry", When: "true", Emit: map[string]string{}},
		},
	}}
	disp := newDisp() // fn_draft returns {n: callCount} → 1, then 2 → loop terminates at n>=2
	svc, store := mkSvc(t, g, disp, ctl, nil, "")
	ctx := ctxWS("ws_1")
	id := mustRun(t, svc, ctx, map[string]any{"v": "topic"})

	assertRunStatus(t, store, ctx, id, flowrundomain.StatusCompleted)
	if disp.actionCalls["fn_draft"] != 2 {
		t.Fatalf("loop should run draft twice (n=1 retry, n=2 done), got %d", disp.actionCalls["fn_draft"])
	}
	if disp.actionCalls["fn_pub"] != 1 {
		t.Fatalf("publish should run once after loop, got %d", disp.actionCalls["fn_pub"])
	}
	// gate has a row at iteration 0 (retry) and iteration 1 (done).
	rows, _ := store.GetNodes(ctx, id)
	var gate0, gate1 bool
	for _, r := range rows {
		if r.NodeID == "gate" && r.Iteration == 0 && asString(r.Result[flowrundomain.ResultKeyPort]) == "retry" {
			gate0 = true
		}
		if r.NodeID == "gate" && r.Iteration == 1 && asString(r.Result[flowrundomain.ResultKeyPort]) == "done" {
			gate1 = true
		}
	}
	if !gate0 || !gate1 {
		t.Fatalf("loop iterations not recorded: gate0=%v gate1=%v", gate0, gate1)
	}
}

// TestScopeFor_BindsAbsentNodesToEmptyMap — regression for F28 (iteration loop): a declared graph
// node with no completed result (e.g. a loop's back-edge predecessor on the first turn) must be
// bound to an EMPTY map in the CEL activation, not omitted. celScopedEnv declares every node id as a
// root and cel-go hard-errors on an unbound declared root an expression references — even inside
// has() — so an absent binding makes the loop-state init `has(pred.x) ? pred.x : seed.x` un-evaluable
// on the first turn. Completed nodes keep their result.
func TestScopeFor_BindsAbsentNodesToEmptyMap(t *testing.T) {
	g := workflowdomain.Graph{
		Nodes: []workflowdomain.Node{
			node("start", "trigger", "trg_1", nil),
			node("check", "control", "ctl_1", nil),
			node("accum", "action", "fn_1", nil),
		},
	}
	rows := []*flowrundomain.FlowRunNode{
		{NodeID: "start", Kind: "trigger", Iteration: 0, Status: flowrundomain.NodeCompleted, Result: map[string]any{"base": 5}},
	}
	scope := newWalk(&g, rows).scopeFor("fr_1", 0)

	if got, _ := scope["start"].(map[string]any); got["base"] != 5 {
		t.Fatalf("completed node must keep its result, got %#v", scope["start"])
	}
	for _, absent := range []string{"check", "accum"} {
		v, ok := scope[absent]
		if !ok {
			t.Fatalf("absent node %q must be bound (not omitted) so cel-go has() works", absent)
		}
		if m, isMap := v.(map[string]any); !isMap || len(m) != 0 {
			t.Fatalf("absent node %q must bind to an empty map, got %#v", absent, v)
		}
	}
	if c, _ := scope["ctx"].(map[string]any); c["runId"] != "fr_1" {
		t.Fatalf("ctx.runId missing: %#v", scope["ctx"])
	}
}

// ---- park / resume + approval first-wins ----------------------------------

func approvalGraph() workflowdomain.Graph {
	return workflowdomain.Graph{
		Nodes: []workflowdomain.Node{
			node("start", "trigger", "trg_1", nil),
			node("human", "approval", "apf_1", map[string]string{"amt": "start.v"}),
			node("publish", "action", "fn_pub", map[string]string{"x": "human.decision"}),
		},
		Edges: []workflowdomain.Edge{
			edge("e1", "start", "", "human"),
			edge("e2", "human", "yes", "publish"),
			// human --no--> (nothing)
		},
	}
}

func TestApproval_ParkResumeYes(t *testing.T) {
	apf := &fakeApproval{byID: map[string]*approvaldomain.Version{
		"apf_1": {Template: "approve {{ input.amt }}?", AllowReason: true},
	}}
	disp := newDisp()
	svc, store := mkSvc(t, approvalGraph(), disp, nil, apf, "")
	ctx := ctxWS("ws_1")
	id := mustRun(t, svc, ctx, map[string]any{"v": "100"})

	// parked, run still running, publish not yet run.
	assertRunStatus(t, store, ctx, id, flowrundomain.StatusRunning)
	rows := nodeRows(t, store, ctx, id)
	if rows["human"] == nil || rows["human"].Status != flowrundomain.NodeParked {
		t.Fatalf("human should be parked: %+v", rows["human"])
	}
	if rows["human"].Result["rendered"] != "approve 100?" {
		t.Fatalf("template render lost: %+v", rows["human"].Result)
	}
	if disp.actionCalls["fn_pub"] != 0 {
		t.Fatalf("publish ran before decision")
	}

	// decide yes → publish runs → completed.
	if err := svc.DecideApproval(ctx, id, "human", "yes", "lgtm"); err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}
	assertRunStatus(t, store, ctx, id, flowrundomain.StatusCompleted)
	if disp.actionCalls["fn_pub"] != 1 {
		t.Fatalf("publish should run after yes, got %d", disp.actionCalls["fn_pub"])
	}

	// deciding again loses (first-wins) → ErrNodeNotParked.
	if err := svc.DecideApproval(ctx, id, "human", "no", "late"); !errors.Is(err, flowrundomain.ErrNodeNotParked) {
		t.Fatalf("second decision should be ErrNodeNotParked, got %v", err)
	}
}

func TestApproval_DecideNo_NoPublish(t *testing.T) {
	apf := &fakeApproval{byID: map[string]*approvaldomain.Version{"apf_1": {Template: "ok?"}}}
	disp := newDisp()
	svc, store := mkSvc(t, approvalGraph(), disp, nil, apf, "")
	ctx := ctxWS("ws_1")
	id := mustRun(t, svc, ctx, map[string]any{"v": "1"})

	if err := svc.DecideApproval(ctx, id, "human", "no", ""); err != nil {
		t.Fatalf("DecideApproval no: %v", err)
	}
	// no → the only out-edge (yes→publish) is pruned → run completes without publish.
	assertRunStatus(t, store, ctx, id, flowrundomain.StatusCompleted)
	if disp.actionCalls["fn_pub"] != 0 {
		t.Fatalf("publish must not run on 'no', got %d", disp.actionCalls["fn_pub"])
	}
}

func TestApproval_Timeout(t *testing.T) {
	apf := &fakeApproval{byID: map[string]*approvaldomain.Version{
		"apf_1": {Template: "ok?", Timeout: "1d", TimeoutBehavior: approvaldomain.TimeoutApprove},
	}}
	disp := newDisp()
	svc, store := mkSvc(t, approvalGraph(), disp, nil, apf, "")
	ctx := ctxWS("ws_1")
	id := mustRun(t, svc, ctx, map[string]any{"v": "1"})
	assertRunStatus(t, store, ctx, id, flowrundomain.StatusRunning)

	// before the deadline: nothing settles.
	if err := svc.CheckTimeouts(ctx, time.Now()); err != nil {
		t.Fatalf("CheckTimeouts early: %v", err)
	}
	assertRunStatus(t, store, ctx, id, flowrundomain.StatusRunning)

	// far past the deadline: approve-behavior settles → publish runs → completed.
	if err := svc.CheckTimeouts(ctx, time.Now().Add(48*time.Hour)); err != nil {
		t.Fatalf("CheckTimeouts late: %v", err)
	}
	assertRunStatus(t, store, ctx, id, flowrundomain.StatusCompleted)
	if disp.actionCalls["fn_pub"] != 1 {
		t.Fatalf("timeout-approve should run publish, got %d", disp.actionCalls["fn_pub"])
	}
}

// ---- blood boundaries: replay determinism + idempotent advance -----------

func TestReplayDeterminism_IdempotentAdvance(t *testing.T) {
	g := workflowdomain.Graph{
		Nodes: []workflowdomain.Node{
			node("start", "trigger", "trg_1", nil),
			node("a", "action", "fn_a", map[string]string{"x": "start.v"}),
			node("b", "action", "fn_b", map[string]string{"y": "a.n"}),
		},
		Edges: []workflowdomain.Edge{edge("e1", "start", "", "a"), edge("e2", "a", "", "b")},
	}
	disp := newDisp()
	svc, store := mkSvc(t, g, disp, nil, nil, "")
	ctx := ctxWS("ws_1")
	id := mustRun(t, svc, ctx, map[string]any{"v": "hi"})
	assertRunStatus(t, store, ctx, id, flowrundomain.StatusCompleted)

	before, _ := store.GetNodes(ctx, id)
	// re-advance a completed run repeatedly: completed rows are copied, never re-run.
	for i := range 3 {
		if err := svc.Advance(ctx, id); err != nil {
			t.Fatalf("re-advance %d: %v", i, err)
		}
	}
	after, _ := store.GetNodes(ctx, id)
	if len(before) != len(after) {
		t.Fatalf("idempotency violated: %d rows → %d rows", len(before), len(after))
	}
	if disp.actionCalls["fn_a"] != 1 || disp.actionCalls["fn_b"] != 1 {
		t.Fatalf("re-advance re-ran activities: %+v", disp.actionCalls)
	}
}

// ---- crash recovery: completed rows skip; a lost row re-runs (at-least-once) ----

func TestCrashRecovery_CompletedRowsSkip(t *testing.T) {
	g := workflowdomain.Graph{
		Nodes: []workflowdomain.Node{
			node("start", "trigger", "trg_1", nil),
			node("a", "action", "fn_a", map[string]string{"x": "start.v"}),
			node("b", "action", "fn_b", map[string]string{"y": "a.n"}),
		},
		Edges: []workflowdomain.Edge{edge("e1", "start", "", "a"), edge("e2", "a", "", "b")},
	}
	disp := newDisp()
	store, _ := newStore(t)
	raw, _ := json.Marshal(g)
	wf := &fakeWorkflows{
		wf:   &workflowdomain.Workflow{ID: "wf_1", Concurrency: workflowdomain.ConcurrencyAllowAll, ActiveVersionID: "wfv_1"},
		ver:  &workflowdomain.Version{ID: "wfv_1", Graph: string(raw)},
		pins: map[string]string{},
	}
	svc := NewService(store, wf, &fakeControl{byID: map[string][]controldomain.Branch{}}, &fakeApproval{byID: map[string]*approvaldomain.Version{}}, disp, nil, nil)
	ctx := ctxWS("ws_1")

	// simulate a crash AFTER 'a' completed (seed trigger + a's completed row), before 'b'.
	run := &flowrundomain.FlowRun{WorkflowID: "wf_1", VersionID: "wfv_1", PinnedRefs: map[string]string{}, Status: flowrundomain.StatusRunning}
	trig := &flowrundomain.FlowRunNode{NodeID: "start", Kind: "trigger", Status: flowrundomain.NodeCompleted, Result: map[string]any{"v": "hi"}}
	id, err := store.CreateRunWithTrigger(ctx, run, trig)
	if err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if _, err := store.InsertNodeResult(ctx, &flowrundomain.FlowRunNode{FlowRunID: id, NodeID: "a", Kind: "action", Status: flowrundomain.NodeCompleted, Result: map[string]any{"n": 1}}); err != nil {
		t.Fatalf("seed a: %v", err)
	}

	// boot recovery re-walks: 'a' is skipped (has a row), only 'b' runs.
	if err := svc.Recover(ctx); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	assertRunStatus(t, store, ctx, id, flowrundomain.StatusCompleted)
	if disp.actionCalls["fn_a"] != 0 {
		t.Fatalf("completed 'a' must NOT be re-run on recovery, got %d", disp.actionCalls["fn_a"])
	}
	if disp.actionCalls["fn_b"] != 1 {
		t.Fatalf("'b' should run once on recovery, got %d", disp.actionCalls["fn_b"])
	}
}

func TestAtLeastOnce_LostRowReRuns(t *testing.T) {
	g := workflowdomain.Graph{
		Nodes: []workflowdomain.Node{
			node("start", "trigger", "trg_1", nil),
			node("a", "action", "fn_a", map[string]string{"x": "start.v"}),
			node("b", "action", "fn_b", map[string]string{"y": "a.n"}),
		},
		Edges: []workflowdomain.Edge{edge("e1", "start", "", "a"), edge("e2", "a", "", "b")},
	}
	disp := newDisp()
	store, sqlDB := newStore(t)
	raw, _ := json.Marshal(g)
	wf := &fakeWorkflows{
		wf:   &workflowdomain.Workflow{ID: "wf_1", Concurrency: workflowdomain.ConcurrencyAllowAll, ActiveVersionID: "wfv_1"},
		ver:  &workflowdomain.Version{ID: "wfv_1", Graph: string(raw)},
		pins: map[string]string{},
	}
	svc := NewService(store, wf, &fakeControl{byID: map[string][]controldomain.Branch{}}, &fakeApproval{byID: map[string]*approvaldomain.Version{}}, disp, nil, nil)
	ctx := ctxWS("ws_1")
	id := mustRun(t, svc, ctx, map[string]any{"v": "hi"})
	assertRunStatus(t, store, ctx, id, flowrundomain.StatusCompleted)
	if disp.actionCalls["fn_b"] != 1 {
		t.Fatalf("b first run count: %d", disp.actionCalls["fn_b"])
	}

	// simulate a crash that lost b's row (ran, but the write never persisted) + reopen the run.
	if _, err := sqlDB.Exec("DELETE FROM flowrun_nodes WHERE flowrun_id = ? AND node_id = 'b'", id); err != nil {
		t.Fatalf("delete b row: %v", err)
	}
	if _, err := sqlDB.Exec("UPDATE flowruns SET status = 'running', completed_at = NULL WHERE id = ?", id); err != nil {
		t.Fatalf("reopen run: %v", err)
	}
	if err := svc.Advance(ctx, id); err != nil {
		t.Fatalf("re-advance: %v", err)
	}
	// at-least-once: the lost activity re-runs (count 2), proving honest at-least-once (not exactly-once).
	if disp.actionCalls["fn_b"] != 2 {
		t.Fatalf("at-least-once: lost 'b' should re-run (count 2), got %d", disp.actionCalls["fn_b"])
	}
	assertRunStatus(t, store, ctx, id, flowrundomain.StatusCompleted)
}

// ---- :replay clears failed rows + re-walks -------------------------------

func TestReplay_FixFailedRun(t *testing.T) {
	g := workflowdomain.Graph{
		Nodes: []workflowdomain.Node{
			node("start", "trigger", "trg_1", nil),
			node("a", "action", "fn_a", map[string]string{"x": "start.v"}),
			node("b", "action", "fn_b", map[string]string{"y": "a.n"}),
		},
		Edges: []workflowdomain.Edge{edge("e1", "start", "", "a"), edge("e2", "a", "", "b")},
	}
	disp := newDisp()
	disp.failRefs["fn_b"] = true // b fails → run fails
	svc, store := mkSvc(t, g, disp, nil, nil, "")
	ctx := ctxWS("ws_1")
	id := mustRun(t, svc, ctx, map[string]any{"v": "hi"})
	assertRunStatus(t, store, ctx, id, flowrundomain.StatusFailed)

	// fix the cause + replay: a's completed row is reused, b re-runs and succeeds.
	disp.failRefs = map[string]bool{}
	if err := svc.Replay(ctx, id); err != nil {
		t.Fatalf("Replay: %v", err)
	}
	assertRunStatus(t, store, ctx, id, flowrundomain.StatusCompleted)
	if disp.actionCalls["fn_a"] != 1 {
		t.Fatalf("replay must NOT re-run completed 'a', got %d", disp.actionCalls["fn_a"])
	}
	if disp.actionCalls["fn_b"] != 2 {
		t.Fatalf("replay should re-run failed 'b' once more (total 2), got %d", disp.actionCalls["fn_b"])
	}
	run, _ := store.GetRun(ctx, id)
	if run.ReplayCount != 1 {
		t.Fatalf("replay_count = %d, want 1", run.ReplayCount)
	}

	// replaying a now-completed run is rejected.
	if err := svc.Replay(ctx, id); !errors.Is(err, flowrundomain.ErrNotReplayable) {
		t.Fatalf("replay of completed run should be ErrNotReplayable, got %v", err)
	}
}

// ---- firing → run single-tx claim + overlap ------------------------------

func firingGraph() workflowdomain.Graph {
	return workflowdomain.Graph{
		Nodes: []workflowdomain.Node{
			node("start", "trigger", "trg_1", nil),
			node("a", "action", "fn_a", map[string]string{"x": "start.orderId"}),
		},
		Edges: []workflowdomain.Edge{edge("e1", "start", "", "a")},
	}
}

func mkSvcWithInbox(t *testing.T, g workflowdomain.Graph, disp *fakeDispatcher, concurrency string) (*Service, *flowrunstore.Store, *triggerstore.Store) {
	t.Helper()
	store, sqlDB := newStore(t)
	trg := triggerstore.New(ormpkg.Open(sqlDB))
	raw, _ := json.Marshal(g)
	wf := &fakeWorkflows{
		wf:   &workflowdomain.Workflow{ID: "wf_1", Concurrency: concurrency, ActiveVersionID: "wfv_1"},
		ver:  &workflowdomain.Version{ID: "wfv_1", Graph: string(raw)},
		pins: map[string]string{},
	}
	svc := NewService(store, wf, &fakeControl{byID: map[string][]controldomain.Branch{}}, &fakeApproval{byID: map[string]*approvaldomain.Version{}}, disp, trg, nil)
	return svc, store, trg
}

func TestFiring_SingleTxClaim(t *testing.T) {
	disp := newDisp()
	svc, store, trg := mkSvcWithInbox(t, firingGraph(), disp, workflowdomain.ConcurrencyAllowAll)
	ctx := ctxWS("ws_1")

	f, err := trg.AppendFiring(ctx, &triggerdomain.Firing{
		WorkspaceID: "ws_1", TriggerID: "trg_1", WorkflowID: "wf_1", DedupKey: "k1",
		Payload: map[string]any{"orderId": "o-9"},
	})
	if err != nil {
		t.Fatalf("AppendFiring: %v", err)
	}
	if err := svc.DrainFirings(ctx); err != nil {
		t.Fatalf("DrainFirings: %v", err)
	}

	// firing claimed → started + flowrun built atomically; the run ran to completion.
	pending, _ := trg.ListPendingFirings(ctx, 10)
	if len(pending) != 0 {
		t.Fatalf("firing should be claimed, %d still pending", len(pending))
	}
	rows, _, _ := store.ListRuns(ctx, flowrundomain.ListFilter{Limit: 10})
	if len(rows) != 1 || rows[0].FiringID != f.ID || rows[0].Status != flowrundomain.StatusCompleted {
		t.Fatalf("claimed run wrong: %+v", rows)
	}
	if disp.actionCalls["fn_a"] != 1 {
		t.Fatalf("firing-driven run should dispatch a once, got %d", disp.actionCalls["fn_a"])
	}
	// the trigger node's result is the firing payload (information, not just signal).
	nodes, _ := store.GetNodes(ctx, rows[0].ID)
	for _, n := range nodes {
		if n.NodeID == "start" && n.Result["orderId"] != "o-9" {
			t.Fatalf("trigger payload not seeded as result: %+v", n.Result)
		}
	}
}

func TestFiring_OverlapSerialDefers_SkipDrops(t *testing.T) {
	// serial: a firing while a run is in flight is deferred (stays pending).
	disp := newDisp()
	svc, store, trg := mkSvcWithInbox(t, firingGraph(), disp, workflowdomain.ConcurrencySerial)
	ctx := ctxWS("ws_1")
	// pre-seed a running run so the workflow is "in flight".
	pre := &flowrundomain.FlowRun{WorkflowID: "wf_1", VersionID: "wfv_1", PinnedRefs: map[string]string{}, Status: flowrundomain.StatusRunning}
	if _, err := store.CreateRunWithTrigger(ctx, pre, &flowrundomain.FlowRunNode{NodeID: "start", Kind: "trigger", Status: flowrundomain.NodeCompleted, Result: map[string]any{}}); err != nil {
		t.Fatalf("pre-run: %v", err)
	}
	if _, err := trg.AppendFiring(ctx, &triggerdomain.Firing{WorkspaceID: "ws_1", TriggerID: "trg_1", WorkflowID: "wf_1", DedupKey: "k1", Payload: map[string]any{"orderId": "o-1"}}); err != nil {
		t.Fatalf("firing: %v", err)
	}
	if err := svc.DrainFirings(ctx); err != nil {
		t.Fatalf("drain serial: %v", err)
	}
	pending, _ := trg.ListPendingFirings(ctx, 10)
	if len(pending) != 1 {
		t.Fatalf("serial should DEFER the firing (stay pending), %d pending", len(pending))
	}

	// Skip: same setup, but a Skip workflow drops the firing.
	disp2 := newDisp()
	svc2, store2, trg2 := mkSvcWithInbox(t, firingGraph(), disp2, workflowdomain.ConcurrencySkip)
	pre2 := &flowrundomain.FlowRun{WorkflowID: "wf_1", VersionID: "wfv_1", PinnedRefs: map[string]string{}, Status: flowrundomain.StatusRunning}
	if _, err := store2.CreateRunWithTrigger(ctx, pre2, &flowrundomain.FlowRunNode{NodeID: "start", Kind: "trigger", Status: flowrundomain.NodeCompleted, Result: map[string]any{}}); err != nil {
		t.Fatalf("pre-run2: %v", err)
	}
	f2, _ := trg2.AppendFiring(ctx, &triggerdomain.Firing{WorkspaceID: "ws_1", TriggerID: "trg_1", WorkflowID: "wf_1", DedupKey: "k1", Payload: map[string]any{}})
	if err := svc2.DrainFirings(ctx); err != nil {
		t.Fatalf("drain skip: %v", err)
	}
	if p, _ := trg2.ListPendingFirings(ctx, 10); len(p) != 0 {
		t.Fatalf("Skip should drop the firing, %d still pending", len(p))
	}
	_ = f2
}

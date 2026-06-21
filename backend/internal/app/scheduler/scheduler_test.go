package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
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
	mu          sync.Mutex // guards all maps — the F174 pool drives RunAction/RunAgent from concurrent workers
	actionCalls map[string]int
	agentCalls  map[string]int
	failRefs    map[string]bool
	actionPins  map[string]string // ref → last pinnedVersionID received
	agentPins   map[string]string
	actionIters map[string][]int // ref → loop iteration seen in ctx per call (F175-M12)
	// Blocking gate (F174 HOL tests): if gateFlag != "" and a node's resolved input[gateFlag] is true,
	// RunAction signals entered<-ref then blocks until gate is closed (or ctx cancelled) — lets a test
	// wedge ONE run (whose trigger payload set the flag) while another run of the same workflow runs free.
	gateFlag string
	gate     chan struct{}
	entered  chan string
}

func newDisp() *fakeDispatcher {
	return &fakeDispatcher{
		actionCalls: map[string]int{}, agentCalls: map[string]int{}, failRefs: map[string]bool{},
		actionPins: map[string]string{}, agentPins: map[string]string{}, actionIters: map[string][]int{},
	}
}

// calls returns a ref's action call count under the lock (use in concurrent/pool tests).
func (d *fakeDispatcher) calls(ref string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.actionCalls[ref]
}

func (d *fakeDispatcher) RunAction(ctx context.Context, ref, pin string, input map[string]any) (map[string]any, error) {
	d.mu.Lock()
	d.actionCalls[ref]++
	d.actionPins[ref] = pin
	it, _ := reqctxpkg.GetFlowrunIteration(ctx)
	d.actionIters[ref] = append(d.actionIters[ref], it)
	n := d.actionCalls[ref]
	fail := d.failRefs[ref]
	gateFlag, gate, entered := d.gateFlag, d.gate, d.entered
	d.mu.Unlock()
	if gateFlag != "" && gate != nil { // wedge runs whose resolved input carries the gate flag (HOL tests)
		if b, _ := input[gateFlag].(bool); b {
			if entered != nil {
				entered <- ref
			}
			select {
			case <-gate:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	if fail {
		return nil, errors.New("action exploded")
	}
	return map[string]any{"n": n}, nil
}
func (d *fakeDispatcher) RunAgent(_ context.Context, ref, pin string, _ map[string]any) (map[string]any, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.agentCalls[ref]++
	d.agentPins[ref] = pin
	return map[string]any{"out": "agent-" + ref}, nil
}

type fakeWorkflows struct {
	wf     *workflowdomain.Workflow
	ver    *workflowdomain.Version
	pins   map[string]string
	getErr error // when set, GetWorkflow returns it (simulates a deleted workflow)
}

func (f *fakeWorkflows) GetWorkflow(context.Context, string) (*workflowdomain.Workflow, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
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

// TestDispatch_InjectsFlowrunIteration pins F175-M12: the scheduler injects the loop iteration into
// ctx for each dispatched node, so an action/agent run on loop turns 0,1,… carries the turn its audit
// row belongs to. A 2-turn loop dispatches draft at iterations [0, 1] (not [0, 0]). Without this,
// every turn's function/agent/handler/mcp audit row would share the same (flowrun_id, node_id) and be
// un-joinable to the right flowrun_nodes truth row.
//
// TestDispatch_InjectsFlowrunIteration 锁 F175-M12：调度器为每个派发节点把循环轮次注入 ctx，使 0,1,…
// 轮跑的 action/agent 带上其审计行所属的轮次。2 轮循环派发 draft 在 iteration [0, 1]（非 [0, 0]）。
func TestDispatch_InjectsFlowrunIteration(t *testing.T) {
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
			edge("e4", "gate", "retry", "draft"), // back edge
		},
	}
	ctl := &fakeControl{byID: map[string][]controldomain.Branch{
		"ctl_loop": {
			{Port: "done", When: "input.n >= 2", Emit: map[string]string{"n": "input.n"}},
			{Port: "retry", When: "true", Emit: map[string]string{}},
		},
	}}
	disp := newDisp() // fn_draft returns {n: callCount} → 1 (retry, iter 0), 2 (done, iter 1)
	svc, store := mkSvc(t, g, disp, ctl, nil, "")
	ctx := ctxWS("ws_1")
	id := mustRun(t, svc, ctx, map[string]any{"v": "topic"})
	assertRunStatus(t, store, ctx, id, flowrundomain.StatusCompleted)

	if got := disp.actionIters["fn_draft"]; len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("draft must be dispatched at iterations [0 1] (F175-M12), got %v", got)
	}
}

// TestWalk_LoopOverflow_FencepostAtMaxPlusOne pins the F175-M1 fencepost: a never-exiting loop
// fails the run, and the body persists EXACTLY MaxIterations+1 rows (iterations 0..MaxIterations)
// — iteration 0 is the forward-edge entry, then MaxIterations back-edge turns succeed before the
// (MaxIterations+1)th is rejected. The error message names the cap as MaxIterations, not the row
// count. Asserting the current 1001-rows / max-index-1000 shape guards against anyone "fixing" the
// apparent off-by-one by flipping `>` to `>=` (which would silently drop a real loop turn).
//
// TestWalk_LoopOverflow_FencepostAtMaxPlusOne 锁 F175-M1 栅栏：永不退出的循环失败该 run，循环体恰
// 持久化 MaxIterations+1 行（iteration 0..MaxIterations）——iteration 0 是前向边入口，随后
// MaxIterations 条回边轮成功、第 MaxIterations+1 条被拒。错误消息把上限标为 MaxIterations、非行数。
// 断言当前 1001 行/最大索引 1000 形状，防有人把 `>` 改成 `>=` 来"修" off-by-one（那会静默吞一条真循环轮）。
func TestWalk_LoopOverflow_FencepostAtMaxPlusOne(t *testing.T) {
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
	// control never exits: done is never true, retry always loops → runaway → overflow.
	// control 永不退出：done 永假、retry 永真 → 失控 → 溢出。
	ctl := &fakeControl{byID: map[string][]controldomain.Branch{
		"ctl_loop": {
			{Port: "done", When: "false", Emit: map[string]string{}},
			{Port: "retry", When: "true", Emit: map[string]string{}},
		},
	}}
	svc, store := mkSvc(t, g, newDisp(), ctl, nil, "")
	ctx := ctxWS("ws_1")
	id := mustRun(t, svc, ctx, map[string]any{"v": "topic"})

	assertRunStatus(t, store, ctx, id, flowrundomain.StatusFailed)

	run, err := store.GetRun(ctx, id)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	// the message names the cap as MaxIterations (the (%d) is the cap, NOT the persisted row count).
	if !strings.Contains(run.Error, fmt.Sprintf("MaxIterations (%d)", MaxIterations)) {
		t.Fatalf("overflow error must name the cap MaxIterations (%d): %q", MaxIterations, run.Error)
	}

	// body ran iterations 0..MaxIterations = MaxIterations+1 rows; max index == MaxIterations.
	rows, _ := store.GetNodes(ctx, id)
	maxIter, draftRows := -1, 0
	for _, r := range rows {
		if r.NodeID == "draft" {
			draftRows++
			if r.Iteration > maxIter {
				maxIter = r.Iteration
			}
		}
	}
	if draftRows != MaxIterations+1 {
		t.Fatalf("loop body rows = %d, want MaxIterations+1 = %d (iterations 0..%d)", draftRows, MaxIterations+1, MaxIterations)
	}
	if maxIter != MaxIterations {
		t.Fatalf("max body iteration = %d, want MaxIterations = %d", maxIter, MaxIterations)
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

// TestFiring_DeletedWorkflowSheds — F137: a pending firing whose workflow was DELETED after the firing
// was queued must be shed (terminal), not left pending and re-attempted (and re-logged) every drain
// tick forever. overlapDecision's GetWorkflow returns WORKFLOW_NOT_FOUND for the deleted workflow.
func TestFiring_DeletedWorkflowSheds(t *testing.T) {
	disp := newDisp()
	svc, store, trg := mkSvcWithInbox(t, firingGraph(), disp, workflowdomain.ConcurrencyAllowAll)
	// The workflow has since been deleted — GetWorkflow now reports it gone.
	svc.workflows.(*fakeWorkflows).getErr = workflowdomain.ErrNotFound
	ctx := ctxWS("ws_1")

	if _, err := trg.AppendFiring(ctx, &triggerdomain.Firing{
		WorkspaceID: "ws_1", TriggerID: "trg_1", WorkflowID: "wf_1", DedupKey: "k1",
		Payload: map[string]any{},
	}); err != nil {
		t.Fatalf("AppendFiring: %v", err)
	}

	// Drain twice: a working shed reaches a terminal status on the first pass; the orphan must NOT
	// reappear as pending on the second (the infinite-loop symptom).
	for i := 0; i < 2; i++ {
		if err := svc.DrainFirings(ctx); err != nil {
			t.Fatalf("DrainFirings #%d: %v", i, err)
		}
	}

	if pending, _ := trg.ListPendingFirings(ctx, 10); len(pending) != 0 {
		t.Fatalf("orphan firing must be shed, %d still pending (would error-loop forever)", len(pending))
	}
	if rows, _, _ := store.ListRuns(ctx, flowrundomain.ListFilter{Limit: 10}); len(rows) != 0 {
		t.Fatalf("a deleted-workflow firing must not create a run, got %d", len(rows))
	}
}

// TestFiring_OverlapReplace_SameBatch — F138: two firings that arrive in the SAME drain batch must let
// the overlap policy engage. Previously DrainFirings advanced each firing's run to COMPLETION inline
// before the next firing was decided, so CountRunningByWorkflow always saw 0 and replace never fired
// (both ran to completion, 0 cancelled). Now every batch firing is seeded as a run BEFORE any is
// advanced, so the later firing sees the earlier in-flight and replaces it. Without the fix this test
// sees 2 completed / 0 cancelled and fails.
func TestFiring_OverlapReplace_SameBatch(t *testing.T) {
	disp := newDisp()
	svc, store, trg := mkSvcWithInbox(t, firingGraph(), disp, workflowdomain.ConcurrencyReplace)
	ctx := ctxWS("ws_1")

	for _, k := range []string{"k1", "k2"} { // two distinct firings (no dedup collapse), one batch
		if _, err := trg.AppendFiring(ctx, &triggerdomain.Firing{
			WorkspaceID: "ws_1", TriggerID: "trg_1", WorkflowID: "wf_1", DedupKey: k,
			Payload: map[string]any{"orderId": k}, // the action reads start.orderId
		}); err != nil {
			t.Fatalf("AppendFiring %s: %v", k, err)
		}
	}

	if err := svc.DrainFirings(ctx); err != nil {
		t.Fatalf("DrainFirings: %v", err)
	}

	// Both firings seeded a run; the earlier was REPLACED (cancelled) by the later, which ran to completion.
	rows, _, _ := store.ListRuns(ctx, flowrundomain.ListFilter{Limit: 10})
	if len(rows) != 2 {
		t.Fatalf("both firings should seed a run, got %d", len(rows))
	}
	var completed, cancelled int
	for _, r := range rows {
		switch r.Status {
		case flowrundomain.StatusCompleted:
			completed++
		case flowrundomain.StatusCancelled:
			cancelled++
		}
	}
	if completed != 1 || cancelled != 1 {
		t.Fatalf("same-batch replace: want 1 completed + 1 cancelled, got %d/%d (%+v)", completed, cancelled, rows)
	}
	// the replaced run never dispatched its action — only the survivor ran.
	if disp.actionCalls["fn_a"] != 1 {
		t.Fatalf("only the surviving run should dispatch the action, got %d", disp.actionCalls["fn_a"])
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

// TestFiring_OverlapBufferOneDefers — buffer_one keeps the latest waiting: with a run in flight, a
// single new firing is deferred (stays pending), like serial (the supersede-older path is covered by
// the trigger store's TestSupersedeAllButNewestPending).
func TestFiring_OverlapBufferOneDefers(t *testing.T) {
	disp := newDisp()
	svc, store, trg := mkSvcWithInbox(t, firingGraph(), disp, workflowdomain.ConcurrencyBufferOne)
	ctx := ctxWS("ws_1")
	pre := &flowrundomain.FlowRun{WorkflowID: "wf_1", VersionID: "wfv_1", PinnedRefs: map[string]string{}, Status: flowrundomain.StatusRunning}
	if _, err := store.CreateRunWithTrigger(ctx, pre, &flowrundomain.FlowRunNode{NodeID: "start", Kind: "trigger", Status: flowrundomain.NodeCompleted, Result: map[string]any{}}); err != nil {
		t.Fatalf("pre-run: %v", err)
	}
	if _, err := trg.AppendFiring(ctx, &triggerdomain.Firing{WorkspaceID: "ws_1", TriggerID: "trg_1", WorkflowID: "wf_1", DedupKey: "k1", Payload: map[string]any{}}); err != nil {
		t.Fatalf("firing: %v", err)
	}
	if err := svc.DrainFirings(ctx); err != nil {
		t.Fatalf("drain buffer_one: %v", err)
	}
	if p, _ := trg.ListPendingFirings(ctx, 10); len(p) != 1 {
		t.Fatalf("buffer_one should DEFER the firing (keep it waiting), %d pending", len(p))
	}
}

// TestFiring_OverlapReplace — replace gracefully cancels the in-flight run and runs the new firing in
// its place: the pre-seeded running run ends up cancelled and the firing is consumed (not left pending).
func TestFiring_OverlapReplace(t *testing.T) {
	disp := newDisp()
	svc, store, trg := mkSvcWithInbox(t, firingGraph(), disp, workflowdomain.ConcurrencyReplace)
	ctx := ctxWS("ws_1")
	pre := &flowrundomain.FlowRun{WorkflowID: "wf_1", VersionID: "wfv_1", PinnedRefs: map[string]string{}, Status: flowrundomain.StatusRunning}
	preID, err := store.CreateRunWithTrigger(ctx, pre, &flowrundomain.FlowRunNode{NodeID: "start", Kind: "trigger", Status: flowrundomain.NodeCompleted, Result: map[string]any{}})
	if err != nil {
		t.Fatalf("pre-run: %v", err)
	}
	if _, err := trg.AppendFiring(ctx, &triggerdomain.Firing{WorkspaceID: "ws_1", TriggerID: "trg_1", WorkflowID: "wf_1", DedupKey: "k1", Payload: map[string]any{}}); err != nil {
		t.Fatalf("firing: %v", err)
	}
	if err := svc.DrainFirings(ctx); err != nil {
		t.Fatalf("drain replace: %v", err)
	}
	got, err := store.GetRun(ctx, preID)
	if err != nil {
		t.Fatalf("get pre-run: %v", err)
	}
	if got.Status != flowrundomain.StatusCancelled {
		t.Fatalf("replace should cancel the in-flight run, status=%q", got.Status)
	}
	if p, _ := trg.ListPendingFirings(ctx, 10); len(p) != 0 {
		t.Fatalf("replace should consume the firing (run it in place), %d still pending", len(p))
	}
}

// TestFiring_OverlapBufferOneRunsNewestOnly — regression for the review's major finding: buffer_one
// must keep ONLY the latest waiting firing even when NOTHING is in flight when they are evaluated.
// Two pending firings + no running run must produce exactly ONE run (the newest); the older is
// superseded, not run. (The pre-fix running==0 short-circuit ran both.)
func TestFiring_OverlapBufferOneRunsNewestOnly(t *testing.T) {
	disp := newDisp()
	svc, _, trg := mkSvcWithInbox(t, firingGraph(), disp, workflowdomain.ConcurrencyBufferOne)
	ctx := ctxWS("ws_1")
	if _, err := trg.AppendFiring(ctx, &triggerdomain.Firing{WorkspaceID: "ws_1", TriggerID: "trg_1", WorkflowID: "wf_1", DedupKey: "k1", Payload: map[string]any{"orderId": "o-1"}}); err != nil {
		t.Fatalf("firing1: %v", err)
	}
	if _, err := trg.AppendFiring(ctx, &triggerdomain.Firing{WorkspaceID: "ws_1", TriggerID: "trg_1", WorkflowID: "wf_1", DedupKey: "k2", Payload: map[string]any{"orderId": "o-2"}}); err != nil {
		t.Fatalf("firing2: %v", err)
	}
	if err := svc.DrainFirings(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if disp.actionCalls["fn_a"] != 1 {
		t.Fatalf("buffer_one should run only the newest firing (1 run), got %d action calls", disp.actionCalls["fn_a"])
	}
	if p, _ := trg.ListPendingFirings(ctx, 10); len(p) != 0 {
		t.Fatalf("both firings resolved (1 run + 1 superseded), %d still pending", len(p))
	}
}

// ---- F174: async Advance worker pool (head-of-line-blocking fix) ----------

// waitForRunStatus polls the DURABLE store until a run reaches `want` or the timeout. Black-box to the
// async Advance pool — it reads the truth table, never pool internals. Used by the F174 tests where the
// pool drives runs off the calling goroutine.
//
// waitForRunStatus 轮询**耐久** store 直到 run 到 `want` 或超时。对异步 Advance 池是黑盒——读真相表、不碰
// 池内部。F174 测试用它（池在调用 goroutine 之外驱动 run）。
func waitForRunStatus(t *testing.T, store *flowrunstore.Store, ctx context.Context, id, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if run, err := store.GetRun(ctx, id); err == nil && run.Status == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	got := "<none>"
	if run, _ := store.GetRun(ctx, id); run != nil {
		got = run.Status
	}
	t.Fatalf("run %s: status %q != want %q within %s", id, got, want, timeout)
}

// holGraph: start → a(action fn_a, input flag=start.slow). A run whose trigger payload sets slow=true
// wedges in fn_a (the dispatcher's gate); slow=false runs free.
func holGraph() workflowdomain.Graph {
	return workflowdomain.Graph{
		Nodes: []workflowdomain.Node{
			node("start", "trigger", "trg_1", nil),
			node("a", "action", "fn_a", map[string]string{"flag": "start.slow"}),
		},
		Edges: []workflowdomain.Edge{edge("e1", "start", "", "a")},
	}
}

func seedRun(t *testing.T, store *flowrunstore.Store, ctx context.Context, payload map[string]any) string {
	t.Helper()
	run := &flowrundomain.FlowRun{WorkflowID: "wf_1", VersionID: "wfv_1", PinnedRefs: map[string]string{}, Status: flowrundomain.StatusRunning}
	id, err := store.CreateRunWithTrigger(ctx, run, &flowrundomain.FlowRunNode{NodeID: "start", Kind: "trigger", Status: flowrundomain.NodeCompleted, Result: payload})
	if err != nil {
		t.Fatalf("seed run: %v", err)
	}
	return id
}

// TestHOL_SlowNodeDoesNotBlockOtherRuns is the core F174 proof: with the pool started, a run wedged in
// a slow node does NOT block another run from advancing to completion. Pre-fix, phase-2 Advance ran
// inline on the single drain goroutine, so run A's 30s node stalled run B (and every later workspace +
// CheckTimeouts) for its whole duration. Deterministic (rendezvous channels, no sleeps), runs -race.
func TestHOL_SlowNodeDoesNotBlockOtherRuns(t *testing.T) {
	disp := newDisp()
	disp.gateFlag, disp.gate, disp.entered = "flag", make(chan struct{}), make(chan string, 2)
	svc, store := mkSvc(t, holGraph(), disp, nil, nil, workflowdomain.ConcurrencyAllowAll)
	svc.StartPool()
	defer svc.StopPool()
	ctx := ctxWS("ws_1")

	idA := seedRun(t, store, ctx, map[string]any{"slow": true})  // wedges in fn_a
	idB := seedRun(t, store, ctx, map[string]any{"slow": false}) // runs free

	svc.enqueueAdvance(ctx, idA)
	select { // A's node is now executing and blocked on the gate
	case <-disp.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("slow node never entered — the pool did not drive run A")
	}
	svc.enqueueAdvance(ctx, idB)

	// THE PROOF: B reaches completed WHILE A is still wedged.
	waitForRunStatus(t, store, ctx, idB, flowrundomain.StatusCompleted, 2*time.Second)
	if r, _ := store.GetRun(ctx, idA); r.Status != flowrundomain.StatusRunning {
		t.Fatalf("run A must still be wedged (running) while blocked, got %q", r.Status)
	}

	close(disp.gate) // release A
	waitForRunStatus(t, store, ctx, idA, flowrundomain.StatusCompleted, 2*time.Second)
}

// TestPool_SameRunNeverDoubleDriven pins the per-run guard (the 命门): two concurrent advances of the
// SAME run must not execute its node twice. A redrive requested while a worker is mid-node collapses to
// a single trailing re-walk, which finds the node already memoized (record-once) and does not re-run.
func TestPool_SameRunNeverDoubleDriven(t *testing.T) {
	disp := newDisp()
	disp.gateFlag, disp.gate, disp.entered = "flag", make(chan struct{}), make(chan string, 2)
	svc, store := mkSvc(t, holGraph(), disp, nil, nil, workflowdomain.ConcurrencyAllowAll)
	svc.StartPool()
	defer svc.StopPool()
	ctx := ctxWS("ws_1")

	id := seedRun(t, store, ctx, map[string]any{"slow": true})
	svc.enqueueAdvance(ctx, id)
	<-disp.entered              // the run is mid-node (fn_a blocked)
	svc.enqueueAdvance(ctx, id) // a second signal for the SAME run while it's in progress → redrive
	svc.enqueueAdvance(ctx, id) // and a third — all must collapse
	close(disp.gate)            // release
	waitForRunStatus(t, store, ctx, id, flowrundomain.StatusCompleted, 2*time.Second)

	if n := disp.calls("fn_a"); n != 1 {
		t.Fatalf("node must run exactly once despite 3 concurrent advances (per-run guard + record-once), ran %d", n)
	}
}

// TestPool_ShutdownDrainsWorkers proves the R3/F100 shutdown contract under the pool: Shutdown cancels a
// wedged node's ctx and StopPool then returns promptly (workers exit, no goroutine leak, no race with a
// later db.Close). A hang here would mean a worker never drains.
func TestPool_ShutdownDrainsWorkers(t *testing.T) {
	disp := newDisp()
	disp.gateFlag, disp.gate, disp.entered = "flag", make(chan struct{}), make(chan string, 1)
	svc, store := mkSvc(t, holGraph(), disp, nil, nil, workflowdomain.ConcurrencyAllowAll)
	svc.StartPool()
	ctx := ctxWS("ws_1")

	id := seedRun(t, store, ctx, map[string]any{"slow": true})
	svc.enqueueAdvance(ctx, id)
	<-disp.entered // node wedged in a worker

	svc.Shutdown() // cancel every in-flight advance (interrupts the wedged node via ctx)
	done := make(chan struct{})
	go func() { svc.StopPool(); close(done) }()
	select {
	case <-done: // workers drained + exited
	case <-time.After(3 * time.Second):
		t.Fatal("StopPool hung — a worker did not drain after Shutdown cancelled the wedged node")
	}
	// the interrupted run did not complete (it was cancelled mid-node).
	if r, _ := store.GetRun(ctx, id); r.Status == flowrundomain.StatusCompleted {
		t.Fatal("a shutdown-interrupted run must not record completed")
	}
}

// TestPool_SendJobRecoversOnClosedQueue pins the F101 shutdown-hardening invariant: a late enqueue whose
// send races StopPool's close(q) must NOT crash the process. Shutdown now bounds its drain waits, so it
// can reach StopPool while a feeder goroutine is still mid-send (the send happens after advMu is released
// — it can't hold the lock). The recover in sendJob turns that benign shutdown race into a dropped
// enqueue (dedup slot cleared, run resumes next boot) instead of a fatal "send on closed channel".
func TestPool_SendJobRecoversOnClosedQueue(t *testing.T) {
	disp := newDisp()
	svc, _ := mkSvc(t, holGraph(), disp, nil, nil, workflowdomain.ConcurrencyAllowAll)
	svc.StartPool()

	// Capture the live queue + mark a run queued exactly as enqueueAdvance does just before its send,
	// then let StopPool close the queue out from under us — the precise mid-send race.
	svc.advMu.Lock()
	q := svc.advQueue
	svc.advQueued["run_x"] = true
	svc.advMu.Unlock()
	svc.StopPool() // closes q

	// The late send must recover (no panic) and clear the dedup slot. A panic here fails the test by
	// crashing the goroutine; reaching the assertion at all proves the recover fired.
	svc.sendJob(q, advanceJob{context.Background(), "run_x"}, "run_x")

	svc.advMu.Lock()
	queued := svc.advQueued["run_x"]
	svc.advMu.Unlock()
	if queued {
		t.Fatal("sendJob on a closed queue must clear the dedup slot so the run can be re-enqueued later")
	}
}

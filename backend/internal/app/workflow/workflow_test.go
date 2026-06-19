package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	_ "github.com/glebarez/go-sqlite"
	"go.uber.org/zap"

	relationdomain "github.com/sunweilin/anselm/backend/internal/domain/relation"
	workflowdomain "github.com/sunweilin/anselm/backend/internal/domain/workflow"
	workflowstore "github.com/sunweilin/anselm/backend/internal/infra/store/workflow"
	ormpkg "github.com/sunweilin/anselm/backend/internal/pkg/orm"
	reqctxpkg "github.com/sunweilin/anselm/backend/internal/pkg/reqctx"
)

// fakeResolver is an in-memory RefResolver: a map from ref → RefInfo; an absent ref returns
// ErrRefNotFound.
type fakeResolver struct{ m map[string]RefInfo }

func (f *fakeResolver) Resolve(_ context.Context, ref string) (RefInfo, error) {
	info, ok := f.m[ref]
	if !ok {
		return RefInfo{}, workflowdomain.ErrRefNotFound
	}
	return info, nil
}

func newSvc(t *testing.T, resolver RefResolver) (*Service, context.Context) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, stmt := range workflowstore.Schema {
		if _, err := sqlDB.Exec(stmt); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	svc := NewService(workflowstore.New(ormpkg.Open(sqlDB)), resolver, nil, zap.NewNop())
	return svc, reqctxpkg.SetWorkspaceID(context.Background(), "ws_1")
}

// opsJSON parses an ops JSON array into []workflowdomain.Op.
func opsJSON(t *testing.T, s string) []workflowdomain.Op {
	t.Helper()
	ops, err := workflowdomain.ParseOps(json.RawMessage(s))
	if err != nil {
		t.Fatalf("ParseOps: %v", err)
	}
	return ops
}

// linearOps builds a valid trigger→action graph.
func linearOps(t *testing.T) []workflowdomain.Op {
	return opsJSON(t, `[
		{"op":"add_node","node":{"id":"t","kind":"trigger","ref":"trg_a"}},
		{"op":"add_node","node":{"id":"a","kind":"action","ref":"fn_b","input":{"x":"t.v"}}},
		{"op":"add_edge","edge":{"id":"e1","from":"t","to":"a"}}
	]`)
}

func TestCreate_WritesV1Active(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	w, v, err := svc.Create(ctx, CreateInput{Name: "pipe", Ops: linearOps(t)})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if v.Version != 1 || w.ActiveVersionID != v.ID {
		t.Fatalf("v1 active: w=%+v v=%+v", w, v)
	}
	// New workflows start parked.
	if w.Active || w.LifecycleState != workflowdomain.LifecycleInactive {
		t.Fatalf("new workflow should be parked: %+v", w)
	}
	got, err := svc.Get(ctx, w.ID)
	if err != nil || got.ActiveVersion == nil || got.ActiveVersion.GraphParsed == nil {
		t.Fatalf("Get active graph: %+v err=%v", got, err)
	}
	if len(got.ActiveVersion.GraphParsed.Nodes) != 2 {
		t.Fatalf("graph not round-tripped: %+v", got.ActiveVersion.GraphParsed)
	}
}

func TestCreate_InvalidGraphRejected(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	// Action node with no trigger → ValidateGraph fails.
	_, _, err := svc.Create(ctx, CreateInput{Name: "bad", Ops: opsJSON(t, `[
		{"op":"add_node","node":{"id":"a","kind":"action","ref":"fn_b","input":{"x":"payload.v"}}}
	]`)})
	if !errors.Is(err, workflowdomain.ErrInvalidGraph) {
		t.Fatalf("want ErrInvalidGraph, got %v", err)
	}
}

func TestCreate_InvalidCELRejected(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	_, _, err := svc.Create(ctx, CreateInput{Name: "badcel", Ops: opsJSON(t, `[
		{"op":"add_node","node":{"id":"t","kind":"trigger","ref":"trg_a"}},
		{"op":"add_node","node":{"id":"a","kind":"action","ref":"fn_b","input":{"x":"payload.("}}},
		{"op":"add_edge","edge":{"id":"e1","from":"t","to":"a"}}
	]`)})
	if !errors.Is(err, workflowdomain.ErrInvalidGraph) {
		t.Fatalf("want ErrInvalidGraph for bad CEL, got %v", err)
	}
}

// TestCreate_NonNodeRefRejected: a node Input referencing a name that is no node at all ("ghost")
// fails at create — caught by the full-graph env pass (the "references an existing node" tier).
//
// TestCreate_NonNodeRefRejected：节点 Input 引用一个根本不存在的名字（"ghost"）在 create 失败——由全图 env
// 那一段（「引用存在节点」层）拦下。
func TestCreate_NonNodeRefRejected(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	_, _, err := svc.Create(ctx, CreateInput{Name: "ghost", Ops: opsJSON(t, `[
		{"op":"add_node","node":{"id":"t","kind":"trigger","ref":"trg_a"}},
		{"op":"add_node","node":{"id":"a","kind":"action","ref":"fn_b","input":{"x":"ghost.v"}}},
		{"op":"add_edge","edge":{"id":"e1","from":"t","to":"a"}}
	]`)})
	if !errors.Is(err, workflowdomain.ErrInvalidGraph) {
		t.Fatalf("want ErrInvalidGraph for ref to non-node, got %v", err)
	}
}

// TestCreate_NonAncestorRefRejected: a node Input referencing an EXISTING but non-ancestor node
// is rejected at create — the visibility lint. Here a and b are parallel siblings off the trigger;
// a reads b.v, but b is not upstream of a (no path b→a), so it must fail.
//
// TestCreate_NonAncestorRefRejected：节点 Input 引用一个**存在但非祖先**的节点在 create 被拒——可见性
// lint。此处 a、b 是 trigger 下的并行兄弟；a 读 b.v，但 b 不在 a 上游（无 b→a 路径），故必拒。
func TestCreate_NonAncestorRefRejected(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	_, _, err := svc.Create(ctx, CreateInput{Name: "vis", Ops: opsJSON(t, `[
		{"op":"add_node","node":{"id":"t","kind":"trigger","ref":"trg_a"}},
		{"op":"add_node","node":{"id":"a","kind":"action","ref":"fn_a","input":{"x":"b.v"}}},
		{"op":"add_node","node":{"id":"b","kind":"action","ref":"fn_b","input":{"y":"t.v"}}},
		{"op":"add_edge","edge":{"id":"e1","from":"t","to":"a"}},
		{"op":"add_edge","edge":{"id":"e2","from":"t","to":"b"}}
	]`)})
	if !errors.Is(err, workflowdomain.ErrInvalidGraph) {
		t.Fatalf("want ErrInvalidGraph for non-ancestor ref, got %v", err)
	}
}

// TestCreate_AncestorRefAccepted: a diamond join may read BOTH branches — they are both
// ancestors of the join. t→a, t→b, a→c, b→c; c reads a.n + b.n → accepted.
//
// TestCreate_AncestorRefAccepted：菱形 join 可读**两条**分支——它们都是 join 的祖先。
func TestCreate_AncestorRefAccepted(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	_, _, err := svc.Create(ctx, CreateInput{Name: "diamond", Ops: opsJSON(t, `[
		{"op":"add_node","node":{"id":"t","kind":"trigger","ref":"trg_a"}},
		{"op":"add_node","node":{"id":"a","kind":"action","ref":"fn_a","input":{"x":"t.v"}}},
		{"op":"add_node","node":{"id":"b","kind":"action","ref":"fn_b","input":{"y":"t.v"}}},
		{"op":"add_node","node":{"id":"c","kind":"action","ref":"fn_c","input":{"sum":"a.n + b.n"}}},
		{"op":"add_edge","edge":{"id":"e1","from":"t","to":"a"}},
		{"op":"add_edge","edge":{"id":"e2","from":"t","to":"b"}},
		{"op":"add_edge","edge":{"id":"e3","from":"a","to":"c"}},
		{"op":"add_edge","edge":{"id":"e4","from":"b","to":"c"}}
	]`)})
	if err != nil {
		t.Fatalf("diamond join reading both ancestors should pass, got %v", err)
	}
}

func TestCreate_DuplicateName(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	if _, _, err := svc.Create(ctx, CreateInput{Name: "dup", Ops: linearOps(t)}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, _, err := svc.Create(ctx, CreateInput{Name: "dup", Ops: linearOps(t)}); !errors.Is(err, workflowdomain.ErrDuplicateName) {
		t.Fatalf("want ErrDuplicateName, got %v", err)
	}
}

func TestEdit_NewVersionPointerMoves(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	w, _, _ := svc.Create(ctx, CreateInput{Name: "e", Ops: linearOps(t)})
	v2, err := svc.Edit(ctx, EditInput{ID: w.ID, Ops: opsJSON(t, `[
		{"op":"add_node","node":{"id":"b","kind":"action","ref":"fn_c","input":{"y":"a.out"}}},
		{"op":"add_edge","edge":{"id":"e2","from":"a","to":"b"}}
	]`)})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if v2.Version != 2 {
		t.Fatalf("want v2, got %d", v2.Version)
	}
	got, _ := svc.Get(ctx, w.ID)
	if got.ActiveVersionID != v2.ID || len(got.ActiveVersion.GraphParsed.Nodes) != 3 {
		t.Fatalf("active not v2 / graph wrong: %+v", got.ActiveVersion)
	}
	if _, err := svc.GetVersionByNumber(ctx, w.ID, 1); err != nil {
		t.Fatalf("v1 should be retained: %v", err)
	}
}

func TestEdit_EmptyOpsRejected(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	w, _, _ := svc.Create(ctx, CreateInput{Name: "e2", Ops: linearOps(t)})
	if _, err := svc.Edit(ctx, EditInput{ID: w.ID, Ops: nil}); !errors.Is(err, workflowdomain.ErrInvalidOps) {
		t.Fatalf("empty ops should be ErrInvalidOps, got %v", err)
	}
}

func TestRevert_MovesPointer(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	w, v1, _ := svc.Create(ctx, CreateInput{Name: "r", Ops: linearOps(t)})
	if _, err := svc.Edit(ctx, EditInput{ID: w.ID, Ops: opsJSON(t, `[{"op":"delete_node","id":"a"}]`)}); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if _, err := svc.Revert(ctx, w.ID, 1); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	got, _ := svc.Get(ctx, w.ID)
	if got.ActiveVersionID != v1.ID {
		t.Fatal("active should be v1 after revert")
	}
}

func TestUpdateMeta_NoVersionBump(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	w, _, _ := svc.Create(ctx, CreateInput{Name: "m", Ops: linearOps(t)})
	name := "renamed"
	if _, err := svc.UpdateMeta(ctx, UpdateMetaInput{ID: w.ID, Name: &name}); err != nil {
		t.Fatalf("UpdateMeta: %v", err)
	}
	got, _ := svc.Get(ctx, w.ID)
	if got.Name != "renamed" {
		t.Fatalf("name not updated: %q", got.Name)
	}
	if n, _ := svc.repo.MaxVersionNumber(ctx, w.ID); n != 1 {
		t.Fatalf("UpdateMeta must not bump version, max=%d", n)
	}
}

func TestLifecycleTransitions(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	w, _, _ := svc.Create(ctx, CreateInput{Name: "lc", Ops: linearOps(t)})

	got, err := svc.SetLifecycle(ctx, w.ID, workflowdomain.LifecycleActive, workflowdomain.ActorUser)
	if err != nil {
		t.Fatalf("SetLifecycle active: %v", err)
	}
	if !got.Active || got.LifecycleState != workflowdomain.LifecycleActive {
		t.Fatalf("active transition wrong: %+v", got)
	}

	got, err = svc.SetLifecycle(ctx, w.ID, workflowdomain.LifecycleDraining, workflowdomain.ActorSystem)
	if err != nil {
		t.Fatalf("SetLifecycle draining: %v", err)
	}
	if got.Active || got.LifecycleState != workflowdomain.LifecycleDraining || got.LastActionBy != workflowdomain.ActorSystem {
		t.Fatalf("draining transition wrong: %+v", got)
	}

	if _, err := svc.SetLifecycle(ctx, w.ID, "bogus", workflowdomain.ActorUser); !errors.Is(err, workflowdomain.ErrInvalidLifecycle) {
		t.Fatalf("bogus state should be ErrInvalidLifecycle, got %v", err)
	}
}

func TestSetNeedsAttention(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	w, _, _ := svc.Create(ctx, CreateInput{Name: "att", Ops: linearOps(t)})
	got, err := svc.SetNeedsAttention(ctx, w.ID, true, "run failed")
	if err != nil {
		t.Fatalf("SetNeedsAttention: %v", err)
	}
	if !got.NeedsAttention || got.AttentionReason != "run failed" {
		t.Fatalf("attention not set: %+v", got)
	}
	got, _ = svc.SetNeedsAttention(ctx, w.ID, false, "ignored")
	if got.NeedsAttention || got.AttentionReason != "" {
		t.Fatalf("attention not cleared: %+v", got)
	}
}

func TestSearch_Substring(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	svc.Create(ctx, CreateInput{Name: "invoice-flow", Ops: linearOps(t)})
	svc.Create(ctx, CreateInput{Name: "spam-flow", Ops: linearOps(t)})
	hits, err := svc.Search(ctx, "invoice")
	if err != nil || len(hits) != 1 || hits[0].Name != "invoice-flow" {
		t.Fatalf("search invoice: %v hits=%v", err, hits)
	}
	all, _ := svc.Search(ctx, "")
	if len(all) != 2 {
		t.Fatalf("empty query should list all, got %d", len(all))
	}
}

func TestDelete_SoftDeleted(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	w, _, _ := svc.Create(ctx, CreateInput{Name: "d", Ops: linearOps(t)})
	if err := svc.Delete(ctx, w.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, w.ID); !errors.Is(err, workflowdomain.ErrNotFound) {
		t.Fatalf("deleted should be NotFound, got %v", err)
	}
}

// --- CapabilityCheck ---

func TestCapabilityCheck_StructuralOnly_NoResolver(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	g, err := workflowdomain.ApplyOps(nil, linearOps(t))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	rep, err := svc.CapabilityCheck(ctx, g)
	if err != nil {
		t.Fatalf("CapabilityCheck: %v", err)
	}
	if !rep.StructurallyValid || rep.Resolved || !rep.OK() {
		t.Fatalf("structural-only report wrong: %+v", rep)
	}
}

func TestCapabilityCheck_RefExists(t *testing.T) {
	resolver := &fakeResolver{m: map[string]RefInfo{
		"trg_a": {Kind: relationdomain.EntityKindTrigger, HasActiveVersion: true, ActiveVersionID: "trgv_1"},
		"fn_b":  {Kind: relationdomain.EntityKindFunction, HasActiveVersion: true, ActiveVersionID: "fnv_1"},
	}}
	svc, ctx := newSvc(t, resolver)
	g, _ := workflowdomain.ApplyOps(nil, linearOps(t))
	rep, err := svc.CapabilityCheck(ctx, g)
	if err != nil {
		t.Fatalf("CapabilityCheck: %v", err)
	}
	if !rep.Resolved || !rep.OK() {
		t.Fatalf("all refs present should be OK: %+v", rep)
	}
}

func TestCapabilityCheck_RefMissing(t *testing.T) {
	resolver := &fakeResolver{m: map[string]RefInfo{
		"trg_a": {Kind: relationdomain.EntityKindTrigger, HasActiveVersion: true},
		// fn_b absent → not found.
	}}
	svc, ctx := newSvc(t, resolver)
	g, _ := workflowdomain.ApplyOps(nil, linearOps(t))
	rep, err := svc.CapabilityCheck(ctx, g)
	if err != nil {
		t.Fatalf("CapabilityCheck: %v", err)
	}
	if rep.OK() || len(rep.Problems) == 0 {
		t.Fatalf("missing ref should surface a problem: %+v", rep)
	}
}

func TestCapabilityCheck_KindMismatch(t *testing.T) {
	resolver := &fakeResolver{m: map[string]RefInfo{
		"trg_a": {Kind: relationdomain.EntityKindFunction, HasActiveVersion: true}, // wrong kind
		"fn_b":  {Kind: relationdomain.EntityKindFunction, HasActiveVersion: true},
	}}
	svc, ctx := newSvc(t, resolver)
	g, _ := workflowdomain.ApplyOps(nil, linearOps(t))
	rep, _ := svc.CapabilityCheck(ctx, g)
	if rep.OK() {
		t.Fatalf("kind mismatch should not be OK: %+v", rep)
	}
}

func TestCapabilityCheck_ControlPortReconciliation(t *testing.T) {
	// Graph: trigger -> control, control --hot--> action, control --cold--> action.
	ops := opsJSON(t, `[
		{"op":"add_node","node":{"id":"t","kind":"trigger","ref":"trg_a"}},
		{"op":"add_node","node":{"id":"c","kind":"control","ref":"ctl_r"}},
		{"op":"add_node","node":{"id":"a","kind":"action","ref":"fn_b","input":{"x":"c.out"}}},
		{"op":"add_edge","edge":{"id":"e1","from":"t","to":"c"}},
		{"op":"add_edge","edge":{"id":"e2","from":"c","fromPort":"hot","to":"a"}},
		{"op":"add_edge","edge":{"id":"e3","from":"c","fromPort":"cold","to":"a"}}
	]`)
	g, _ := workflowdomain.ApplyOps(nil, ops)

	t.Run("all ports present", func(t *testing.T) {
		resolver := &fakeResolver{m: map[string]RefInfo{
			"trg_a": {Kind: relationdomain.EntityKindTrigger, HasActiveVersion: true},
			"ctl_r": {Kind: relationdomain.EntityKindControl, HasActiveVersion: true, BranchPorts: []string{"hot", "cold"}},
			"fn_b":  {Kind: relationdomain.EntityKindFunction, HasActiveVersion: true},
		}}
		svc, ctx := newSvc(t, resolver)
		rep, _ := svc.CapabilityCheck(ctx, g)
		if !rep.OK() {
			t.Fatalf("matching ports should be OK: %+v", rep)
		}
	})

	t.Run("missing branch port", func(t *testing.T) {
		resolver := &fakeResolver{m: map[string]RefInfo{
			"trg_a": {Kind: relationdomain.EntityKindTrigger, HasActiveVersion: true},
			"ctl_r": {Kind: relationdomain.EntityKindControl, HasActiveVersion: true, BranchPorts: []string{"hot"}}, // no "cold"
			"fn_b":  {Kind: relationdomain.EntityKindFunction, HasActiveVersion: true},
		}}
		svc, ctx := newSvc(t, resolver)
		rep, _ := svc.CapabilityCheck(ctx, g)
		if rep.OK() {
			t.Fatalf("absent branch port should surface a problem: %+v", rep)
		}
	})
}

func TestCapabilityCheck_HandlerMethod(t *testing.T) {
	ops := opsJSON(t, `[
		{"op":"add_node","node":{"id":"t","kind":"trigger","ref":"trg_a"}},
		{"op":"add_node","node":{"id":"h","kind":"action","ref":"hd_x.send","input":{"to":"payload.addr"}}},
		{"op":"add_edge","edge":{"id":"e1","from":"t","to":"h"}}
	]`)
	g, _ := workflowdomain.ApplyOps(nil, ops)

	resolver := &fakeResolver{m: map[string]RefInfo{
		"trg_a":     {Kind: relationdomain.EntityKindTrigger, HasActiveVersion: true},
		"hd_x.send": {Kind: relationdomain.EntityKindHandler, HasActiveVersion: true, MethodNames: []string{"recv"}}, // no "send"
	}}
	svc, ctx := newSvc(t, resolver)
	rep, _ := svc.CapabilityCheck(ctx, g)
	if rep.OK() {
		t.Fatalf("missing handler method should surface a problem: %+v", rep)
	}
}

// TestCapabilityCheck_MCPToolName — F51: a bad MCP tool name (the /tool suffix) is faulted like a bad
// handler method, not slipped to a runtime MCP_RPC_ERROR; a disconnected server (no tool names) skips.
func TestCapabilityCheck_MCPToolName(t *testing.T) {
	ops := opsJSON(t, `[
		{"op":"add_node","node":{"id":"t","kind":"trigger","ref":"trg_a"}},
		{"op":"add_node","node":{"id":"m","kind":"action","ref":"mcp:srv/badtool"}},
		{"op":"add_edge","edge":{"id":"e1","from":"t","to":"m"}}
	]`)
	g, _ := workflowdomain.ApplyOps(nil, ops)

	// connected server exposing only "goodtool" → "badtool" is faulted
	resolver := &fakeResolver{m: map[string]RefInfo{
		"trg_a":           {Kind: relationdomain.EntityKindTrigger, HasActiveVersion: true},
		"mcp:srv/badtool": {Kind: relationdomain.EntityKindMCP, HasActiveVersion: true, MCPToolNames: []string{"goodtool"}},
	}}
	svc, ctx := newSvc(t, resolver)
	if rep, _ := svc.CapabilityCheck(ctx, g); rep.OK() {
		t.Fatalf("a bad mcp tool name should surface a problem: %+v", rep)
	}

	// disconnected server (no tool names) → can't validate → skip, no false problem
	resolver2 := &fakeResolver{m: map[string]RefInfo{
		"trg_a":           {Kind: relationdomain.EntityKindTrigger, HasActiveVersion: true},
		"mcp:srv/badtool": {Kind: relationdomain.EntityKindMCP, HasActiveVersion: true},
	}}
	svc2, ctx2 := newSvc(t, resolver2)
	if rep, _ := svc2.CapabilityCheck(ctx2, g); !rep.OK() {
		t.Fatalf("a disconnected mcp server should skip the tool check, not fault it: %+v", rep)
	}
}

// --- BuildPinClosure ---

func TestBuildPinClosure_AgentDepth2(t *testing.T) {
	// Graph: trigger -> agent. The agent mounts fn_inner + hd_h.do.
	ops := opsJSON(t, `[
		{"op":"add_node","node":{"id":"t","kind":"trigger","ref":"trg_a"}},
		{"op":"add_node","node":{"id":"ag","kind":"agent","ref":"ag_worker","input":{"task":"payload.t"}}},
		{"op":"add_edge","edge":{"id":"e1","from":"t","to":"ag"}}
	]`)
	g, _ := workflowdomain.ApplyOps(nil, ops)

	resolver := &fakeResolver{m: map[string]RefInfo{
		"trg_a":     {Kind: relationdomain.EntityKindTrigger, HasActiveVersion: true, ActiveVersionID: "trgv_1"},
		"ag_worker": {Kind: relationdomain.EntityKindAgent, HasActiveVersion: true, ActiveVersionID: "agv_1", AgentCallables: []string{"fn_inner", "hd_h.do"}},
		"fn_inner":  {Kind: relationdomain.EntityKindFunction, HasActiveVersion: true, ActiveVersionID: "fnv_9"},
		"hd_h.do":   {Kind: relationdomain.EntityKindHandler, HasActiveVersion: true, ActiveVersionID: "hdv_3"},
	}}
	svc, ctx := newSvc(t, resolver)
	pins, err := svc.BuildPinClosure(ctx, g)
	if err != nil {
		t.Fatalf("BuildPinClosure: %v", err)
	}
	want := map[string]string{
		"trg_a":     "trgv_1",
		"ag_worker": "agv_1",
		"fn_inner":  "fnv_9",
		"hd_h":      "hdv_3", // handler ref keyed by bare id (method stripped)
	}
	if len(pins) != len(want) {
		t.Fatalf("pin count: got %d want %d (%+v)", len(pins), len(want), pins)
	}
	for k, v := range want {
		if pins[k] != v {
			t.Fatalf("pin[%q]=%q want %q (all=%+v)", k, pins[k], v, pins)
		}
	}
}

func TestBuildPinClosure_NoResolver(t *testing.T) {
	svc, ctx := newSvc(t, nil)
	g, _ := workflowdomain.ApplyOps(nil, linearOps(t))
	pins, err := svc.BuildPinClosure(ctx, g)
	if err != nil || len(pins) != 0 {
		t.Fatalf("no resolver should yield empty pins: %v %+v", err, pins)
	}
}

func TestBuildPinClosure_SkipsVersionlessAndMissing(t *testing.T) {
	resolver := &fakeResolver{m: map[string]RefInfo{
		"trg_a": {Kind: relationdomain.EntityKindTrigger, HasActiveVersion: true, ActiveVersionID: "trgv_1"},
		"fn_b":  {Kind: relationdomain.EntityKindFunction, HasActiveVersion: false}, // no active version → not pinned
	}}
	svc, ctx := newSvc(t, resolver)
	g, _ := workflowdomain.ApplyOps(nil, linearOps(t))
	pins, err := svc.BuildPinClosure(ctx, g)
	if err != nil {
		t.Fatalf("BuildPinClosure: %v", err)
	}
	if _, ok := pins["fn_b"]; ok {
		t.Fatalf("version-less entity should not be pinned: %+v", pins)
	}
	if pins["trg_a"] != "trgv_1" {
		t.Fatalf("trigger should be pinned: %+v", pins)
	}
}

// WorkflowReader compile-time conformance.
var _ WorkflowReader = (*Service)(nil)

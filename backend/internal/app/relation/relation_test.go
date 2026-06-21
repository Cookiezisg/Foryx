package relation

import (
	"context"
	"errors"
	"slices"
	"testing"

	"go.uber.org/zap"

	relationdomain "github.com/sunweilin/anselm/backend/internal/domain/relation"
)

// fakeRepo is an in-memory relationdomain.Repository covering diff-sync, neighborhood
// reads, and purge — no DB. dedup mirrors the store's (from,to,kind) unique index.
//
// fakeRepo 是内存版 relationdomain.Repository，覆盖 diff-sync、邻域读、purge——无 DB。
// dedup 镜像 store 的 (from,to,kind) 唯一索引。
type fakeRepo struct {
	rows []*relationdomain.Relation
}

var _ relationdomain.Repository = (*fakeRepo)(nil)

func newFakeRepo() *fakeRepo { return &fakeRepo{} }

func inKinds(k string, kinds []string) bool {
	if len(kinds) == 0 {
		return true
	}
	return slices.Contains(kinds, k)
}

func (f *fakeRepo) InsertBatch(_ context.Context, rels []*relationdomain.Relation) error {
	for _, r := range rels {
		dup := false
		for _, e := range f.rows {
			if e.FromID == r.FromID && e.ToID == r.ToID && e.Kind == r.Kind {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		cp := *r
		f.rows = append(f.rows, &cp)
	}
	return nil
}

func (f *fakeRepo) UpdateAttrs(_ context.Context, id string, attrs map[string]any) error {
	for _, r := range f.rows {
		if r.ID == id {
			r.Attrs = attrs
		}
	}
	return nil
}

func (f *fakeRepo) DeleteByIDs(_ context.Context, ids []string) error {
	del := make(map[string]bool, len(ids))
	for _, id := range ids {
		del[id] = true
	}
	var out []*relationdomain.Relation
	for _, r := range f.rows {
		if !del[r.ID] {
			out = append(out, r)
		}
	}
	f.rows = out
	return nil
}

func (f *fakeRepo) ListByFromAndKinds(_ context.Context, fromKind, fromID string, kinds []string) ([]*relationdomain.Relation, error) {
	var out []*relationdomain.Relation
	for _, r := range f.rows {
		if r.FromKind == fromKind && r.FromID == fromID && inKinds(r.Kind, kinds) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeRepo) ListByToAndKinds(_ context.Context, toKind, toID string, kinds []string) ([]*relationdomain.Relation, error) {
	var out []*relationdomain.Relation
	for _, r := range f.rows {
		if r.ToKind == toKind && r.ToID == toID && inKinds(r.Kind, kinds) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeRepo) List(_ context.Context, filter relationdomain.Filter, _ string, _ int) ([]*relationdomain.Relation, string, error) {
	var out []*relationdomain.Relation
	for _, r := range f.rows {
		if filter.FromKind != "" && r.FromKind != filter.FromKind {
			continue
		}
		if filter.FromID != "" && r.FromID != filter.FromID {
			continue
		}
		if filter.ToKind != "" && r.ToKind != filter.ToKind {
			continue
		}
		if filter.ToID != "" && r.ToID != filter.ToID {
			continue
		}
		if filter.Kind != "" && r.Kind != filter.Kind {
			continue
		}
		out = append(out, r)
	}
	return out, "", nil
}

func (f *fakeRepo) ListAll(_ context.Context) ([]*relationdomain.Relation, error) {
	return append([]*relationdomain.Relation(nil), f.rows...), nil
}

func (f *fakeRepo) PurgeEntity(_ context.Context, kind, id string) (int64, error) {
	var out []*relationdomain.Relation
	var n int64
	for _, r := range f.rows {
		if (r.FromKind == kind && r.FromID == id) || (r.ToKind == kind && r.ToID == id) {
			n++
			continue
		}
		out = append(out, r)
	}
	f.rows = out
	return n, nil
}

// fakeNamer returns names only for ids it knows; unknown ids are simply absent.
//
// fakeNamer 只返回它认识的 id 的名字；不认识的缺席。
type fakeNamer struct{ names map[string]string }

func (f fakeNamer) NamesByIDs(_ context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range ids {
		if n, ok := f.names[id]; ok {
			out[id] = n
		}
	}
	return out, nil
}

func newSvc(repo relationdomain.Repository, namers map[string]Namer) *Service {
	return NewService(Config{Repo: repo, Namers: namers, Log: zap.NewNop()})
}

func equip(otherKind, otherID string) relationdomain.SyncEdge {
	return relationdomain.SyncEdge{OtherKind: otherKind, OtherID: otherID, Kind: relationdomain.KindEquip}
}

func TestSyncOutgoing_InsertThenReplace(t *testing.T) {
	repo := newFakeRepo()
	svc := newSvc(repo, nil)
	ctx := context.Background()
	wf := "wf_1111111111111111"
	scope := []string{relationdomain.KindEquip}

	if err := svc.SyncOutgoing(ctx, relationdomain.EntityKindWorkflow, wf, scope,
		[]relationdomain.SyncEdge{equip("function", "fn_aaaaaaaaaaaaaaaa"), equip("function", "fn_bbbbbbbbbbbbbbbb")}); err != nil {
		t.Fatalf("sync 1: %v", err)
	}
	if len(repo.rows) != 2 {
		t.Fatalf("want 2 edges after first sync, got %d", len(repo.rows))
	}

	// Re-sync: drop fn_a, keep fn_b, add fn_c → end exactly {fn_b, fn_c}.
	if err := svc.SyncOutgoing(ctx, relationdomain.EntityKindWorkflow, wf, scope,
		[]relationdomain.SyncEdge{equip("function", "fn_bbbbbbbbbbbbbbbb"), equip("function", "fn_cccccccccccccccc")}); err != nil {
		t.Fatalf("sync 2: %v", err)
	}
	got := map[string]bool{}
	for _, r := range repo.rows {
		got[r.ToID] = true
	}
	if len(repo.rows) != 2 || got["fn_aaaaaaaaaaaaaaaa"] || !got["fn_bbbbbbbbbbbbbbbb"] || !got["fn_cccccccccccccccc"] {
		t.Errorf("after re-sync want {fn_b, fn_c}, got %+v", got)
	}
}

func TestSyncOutgoing_UpdatesAttrsInPlace(t *testing.T) {
	repo := newFakeRepo()
	svc := newSvc(repo, nil)
	ctx := context.Background()
	wf := "wf_1111111111111111"
	scope := []string{relationdomain.KindEquip}

	e := equip("function", "fn_aaaaaaaaaaaaaaaa")
	e.Attrs = map[string]any{"n": float64(1)}
	_ = svc.SyncOutgoing(ctx, relationdomain.EntityKindWorkflow, wf, scope, []relationdomain.SyncEdge{e})
	id1 := repo.rows[0].ID

	e.Attrs = map[string]any{"n": float64(2)}
	_ = svc.SyncOutgoing(ctx, relationdomain.EntityKindWorkflow, wf, scope, []relationdomain.SyncEdge{e})
	if len(repo.rows) != 1 {
		t.Fatalf("want 1 edge (updated, not duplicated), got %d", len(repo.rows))
	}
	if repo.rows[0].ID != id1 {
		t.Errorf("edge id changed (%q→%q); want in-place update", id1, repo.rows[0].ID)
	}
	if repo.rows[0].Attrs["n"] != float64(2) {
		t.Errorf("attrs not updated: %+v", repo.rows[0].Attrs)
	}
}

func TestSyncOutgoing_SelfLoopRejected(t *testing.T) {
	svc := newSvc(newFakeRepo(), nil)
	err := svc.SyncOutgoing(context.Background(), relationdomain.EntityKindWorkflow, "wf_1111111111111111",
		[]string{relationdomain.KindEquip},
		[]relationdomain.SyncEdge{equip("workflow", "wf_1111111111111111")})
	if !errors.Is(err, relationdomain.ErrSelfLoop) {
		t.Errorf("want ErrSelfLoop, got %v", err)
	}
}

func TestSyncOutgoing_InvalidKindRejected(t *testing.T) {
	svc := newSvc(newFakeRepo(), nil)
	err := svc.SyncOutgoing(context.Background(), relationdomain.EntityKindWorkflow, "wf_1111111111111111",
		[]string{relationdomain.KindEquip},
		[]relationdomain.SyncEdge{{OtherKind: "function", OtherID: "fn_a", Kind: "uses"}})
	if !errors.Is(err, relationdomain.ErrInvalidKind) {
		t.Errorf("want ErrInvalidKind, got %v", err)
	}
}

func TestSyncIncoming_AtMostOneReplaces(t *testing.T) {
	repo := newFakeRepo()
	svc := newSvc(repo, nil)
	ctx := context.Background()
	ag := "ag_1111111111111111"
	scope := []string{relationdomain.KindCreate}
	createdBy := func(cv string) []relationdomain.SyncEdge {
		return []relationdomain.SyncEdge{{OtherKind: relationdomain.EntityKindConversation, OtherID: cv, Kind: relationdomain.KindCreate}}
	}

	_ = svc.SyncIncoming(ctx, relationdomain.EntityKindAgent, ag, scope, createdBy("cv_aaaaaaaaaaaaaaaa"))
	_ = svc.SyncIncoming(ctx, relationdomain.EntityKindAgent, ag, scope, createdBy("cv_bbbbbbbbbbbbbbbb"))

	if len(repo.rows) != 1 {
		t.Fatalf("want exactly 1 create edge (replaced), got %d", len(repo.rows))
	}
	// Incoming: the conversation is FROM, the agent is TO.
	if repo.rows[0].FromID != "cv_bbbbbbbbbbbbbbbb" || repo.rows[0].ToID != ag || repo.rows[0].Kind != relationdomain.KindCreate {
		t.Errorf("want cv_b --create--> ag, got %+v", repo.rows[0])
	}
}

func TestNeighborhood_BFSDepth(t *testing.T) {
	repo := newFakeRepo()
	// wf_1 --equip--> fn_1 ; doc_1 --link--> fn_1
	repo.rows = []*relationdomain.Relation{
		{ID: "rel_1", Kind: relationdomain.KindEquip, FromKind: "workflow", FromID: "wf_1", ToKind: "function", ToID: "fn_1"},
		{ID: "rel_2", Kind: relationdomain.KindLink, FromKind: "document", FromID: "doc_1", ToKind: "function", ToID: "fn_1"},
	}
	svc := newSvc(repo, nil)

	got1, err := svc.Neighborhood(context.Background(), "workflow", "wf_1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got1) != 1 {
		t.Fatalf("depth 1 from wf_1: want 1 edge, got %d", len(got1))
	}
	// depth 2 reaches doc_1's edge through fn_1.
	got2, err := svc.Neighborhood(context.Background(), "workflow", "wf_1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got2) != 2 {
		t.Fatalf("depth 2 from wf_1: want 2 edges, got %d", len(got2))
	}
}

func TestNeighborhood_DepthOutOfRange(t *testing.T) {
	svc := newSvc(newFakeRepo(), nil)
	for _, d := range []int{0, 4} {
		if _, err := svc.Neighborhood(context.Background(), "workflow", "wf_1", d); !errors.Is(err, relationdomain.ErrDepthOutOfRange) {
			t.Errorf("depth %d: want ErrDepthOutOfRange, got %v", d, err)
		}
	}
}

func TestHydrate_FillsNameAndFallsBackToID(t *testing.T) {
	repo := newFakeRepo()
	repo.rows = []*relationdomain.Relation{
		{ID: "rel_1", Kind: relationdomain.KindEquip, FromKind: "workflow", FromID: "wf_1", ToKind: "function", ToID: "fn_1"},
	}
	// workflow has a namer; function does not → its id is shown verbatim.
	namers := map[string]Namer{
		relationdomain.EntityKindWorkflow: fakeNamer{names: map[string]string{"wf_1": "My Flow"}},
	}
	svc := newSvc(repo, namers)
	views, _, err := svc.List(context.Background(), relationdomain.Filter{}, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 {
		t.Fatalf("want 1 view, got %d", len(views))
	}
	if views[0].FromName != "My Flow" {
		t.Errorf("fromName = %q, want 'My Flow'", views[0].FromName)
	}
	if views[0].ToName != "fn_1" {
		t.Errorf("toName = %q, want fallback to id 'fn_1'", views[0].ToName)
	}
}

func TestGetRelgraph_DedupsNodes(t *testing.T) {
	repo := newFakeRepo()
	// Two edges share fn_1 → fn_1 must appear once among nodes.
	repo.rows = []*relationdomain.Relation{
		{ID: "rel_1", Kind: relationdomain.KindEquip, FromKind: "workflow", FromID: "wf_1", ToKind: "function", ToID: "fn_1"},
		{ID: "rel_2", Kind: relationdomain.KindEquip, FromKind: "agent", FromID: "ag_1", ToKind: "function", ToID: "fn_1"},
	}
	svc := newSvc(repo, nil)
	snap, err := svc.GetRelgraph(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Edges) != 2 {
		t.Fatalf("want 2 edges, got %d", len(snap.Edges))
	}
	if len(snap.Nodes) != 3 { // wf_1, ag_1, fn_1 (deduped)
		t.Fatalf("want 3 deduped nodes, got %d: %+v", len(snap.Nodes), snap.Nodes)
	}
}

func TestPurgeEntity_RemovesBothDirections(t *testing.T) {
	repo := newFakeRepo()
	repo.rows = []*relationdomain.Relation{
		{ID: "rel_1", Kind: relationdomain.KindEquip, FromKind: "workflow", FromID: "wf_1", ToKind: "function", ToID: "fn_1"},
		{ID: "rel_2", Kind: relationdomain.KindEquip, FromKind: "agent", FromID: "ag_1", ToKind: "function", ToID: "fn_1"},
		{ID: "rel_3", Kind: relationdomain.KindEquip, FromKind: "workflow", FromID: "wf_2", ToKind: "handler", ToID: "hd_1"},
	}
	svc := newSvc(repo, nil)
	if err := svc.PurgeEntity(context.Background(), "function", "fn_1"); err != nil {
		t.Fatal(err)
	}
	if len(repo.rows) != 1 || repo.rows[0].ID != "rel_3" {
		t.Errorf("after purge fn_1 want only rel_3 left, got %+v", repo.rows)
	}
}

func TestList_IncompleteFilterRejected(t *testing.T) {
	svc := newSvc(newFakeRepo(), nil)
	_, _, err := svc.List(context.Background(), relationdomain.Filter{FromKind: "function"}, "", 0) // id missing
	if !errors.Is(err, relationdomain.ErrIncompleteFilter) {
		t.Errorf("want ErrIncompleteFilter, got %v", err)
	}
}

// TestCountDependents: the honest "what breaks if I delete this" count = INCOMING equip/link only.
// It must exclude create/edit provenance (the conversation that built it) and the entity's OWN
// outgoing edges — counting those would report a wildly wrong dependent number for workflows/agents.
//
// TestCountDependents：诚实的「删了它什么会坏」计数 = **入向** equip/link only。须排除 create/edit
// 溯源（建它的对话）与本实体**出向**边——把那些算进来会给 workflow/agent 报严重错误的依赖数。
func TestCountDependents(t *testing.T) {
	repo := newFakeRepo()
	repo.rows = []*relationdomain.Relation{
		// 2 incoming equip (a workflow + an agent mounted fn_1) — these ARE dependents.
		{ID: "rel_1", Kind: relationdomain.KindEquip, FromKind: "workflow", FromID: "wf_1", ToKind: "function", ToID: "fn_1"},
		{ID: "rel_2", Kind: relationdomain.KindEquip, FromKind: "agent", FromID: "ag_1", ToKind: "function", ToID: "fn_1"},
		// 1 incoming link (a document linked fn_1) — also a dependent.
		{ID: "rel_3", Kind: relationdomain.KindLink, FromKind: "document", FromID: "doc_1", ToKind: "function", ToID: "fn_1"},
		// incoming create (the conversation that built fn_1) — NOT a dependent (provenance).
		{ID: "rel_4", Kind: relationdomain.KindCreate, FromKind: "conversation", FromID: "cv_1", ToKind: "function", ToID: "fn_1"},
		// fn_1's OWN outgoing equip (fn_1 → some handler) — NOT a dependent (it's the deleted entity's edge).
		{ID: "rel_5", Kind: relationdomain.KindEquip, FromKind: "function", FromID: "fn_1", ToKind: "handler", ToID: "hd_9"},
		// an unrelated entity's edge — must not be counted.
		{ID: "rel_6", Kind: relationdomain.KindEquip, FromKind: "workflow", FromID: "wf_2", ToKind: "function", ToID: "fn_2"},
	}
	svc := newSvc(repo, nil)
	ctx := context.Background()

	n, err := svc.CountDependents(ctx, "function", "fn_1")
	if err != nil {
		t.Fatalf("CountDependents: %v", err)
	}
	if n != 3 {
		t.Fatalf("CountDependents(fn_1) = %d, want 3 (2 equip + 1 link incoming; excludes create + own outgoing)", n)
	}

	// No dependents → 0 (no false alarm).
	if n, _ := svc.CountDependents(ctx, "function", "fn_unreferenced"); n != 0 {
		t.Fatalf("unreferenced entity = %d, want 0", n)
	}

	// Invalid ref rejected.
	if _, err := svc.CountDependents(ctx, "boguskind", "x_1"); !errors.Is(err, relationdomain.ErrInvalidRef) {
		t.Fatalf("invalid kind: err = %v, want ErrInvalidRef", err)
	}
}

// fakeEmitter captures the notifications a deleted entity's dependents trigger.
//
// fakeEmitter 捕获被删实体的依赖触发的通知。
type fakeEmitter struct {
	types    []string
	payloads []map[string]any
	err      error
}

func (e *fakeEmitter) Emit(_ context.Context, eventType string, payload map[string]any) error {
	e.types = append(e.types, eventType)
	e.payloads = append(e.payloads, payload)
	return e.err
}

// TestPurgeEntity_EmitsDependencyBroken: deleting a depended-on entity raises ONE durable,
// aggregated relation.dependency_broken notification naming exactly the incoming equip/link
// dependents (hydrated + deduped) — NOT create-provenance, NOT the entity's own outgoing edges —
// the persisted counterpart to F160's ephemeral delete-tool note (F161).
//
// TestPurgeEntity_EmitsDependencyBroken：删一个被依赖的实体发 ONE 持久化、聚合的
// relation.dependency_broken 通知，恰好点名入向 equip/link 依赖（hydrate + 去重）——非 create 溯源、
// 非本实体出边——是 F160 瞬时 delete-tool 提示的持久对应物（F161）。
func TestPurgeEntity_EmitsDependencyBroken(t *testing.T) {
	repo := newFakeRepo()
	repo.rows = []*relationdomain.Relation{
		{ID: "rel_1", Kind: relationdomain.KindEquip, FromKind: "agent", FromID: "ag_1", ToKind: "function", ToID: "fn_1"},
		{ID: "rel_2", Kind: relationdomain.KindLink, FromKind: "workflow", FromID: "wf_1", ToKind: "function", ToID: "fn_1"},
		// a SECOND link edge from the same workflow (two nodes referencing fn_1) — must dedup to one ref.
		{ID: "rel_3", Kind: relationdomain.KindLink, FromKind: "workflow", FromID: "wf_1", ToKind: "function", ToID: "fn_1"},
		// create-provenance (the conversation that built fn_1) — NOT a dependency break.
		{ID: "rel_4", Kind: relationdomain.KindCreate, FromKind: "conversation", FromID: "cv_1", ToKind: "function", ToID: "fn_1"},
		// fn_1's OWN outgoing equip — NOT a dependent.
		{ID: "rel_5", Kind: relationdomain.KindEquip, FromKind: "function", FromID: "fn_1", ToKind: "handler", ToID: "hd_9"},
	}
	em := &fakeEmitter{}
	svc := NewService(Config{
		Repo: repo,
		Namers: map[string]Namer{
			relationdomain.EntityKindAgent:    fakeNamer{names: map[string]string{"ag_1": "My Agent"}},
			relationdomain.EntityKindWorkflow: fakeNamer{names: map[string]string{"wf_1": "My Flow"}},
		},
		Notif: em,
		Log:   zap.NewNop(),
	})

	if err := svc.PurgeEntity(context.Background(), "function", "fn_1"); err != nil {
		t.Fatal(err)
	}

	if len(em.types) != 1 || em.types[0] != "relation.dependency_broken" {
		t.Fatalf("want exactly one relation.dependency_broken emit, got types=%v", em.types)
	}
	p := em.payloads[0]
	if p["deletedKind"] != "function" || p["deletedId"] != "fn_1" {
		t.Errorf("payload deleted ref = %v/%v, want function/fn_1", p["deletedKind"], p["deletedId"])
	}
	deps, ok := p["dependents"].([]map[string]any)
	if !ok {
		t.Fatalf("dependents wrong type: %T", p["dependents"])
	}
	if len(deps) != 2 { // ag_1 + wf_1 (the duplicate wf_1 link edge deduped; create + own-outgoing excluded)
		t.Fatalf("want 2 deduped dependents (agent + workflow), got %d: %+v", len(deps), deps)
	}
	byID := map[string]map[string]any{}
	for _, d := range deps {
		byID[d["id"].(string)] = d
	}
	if d := byID["ag_1"]; d == nil || d["name"] != "My Agent" || d["kind"] != "agent" || d["edge"] != relationdomain.KindEquip {
		t.Errorf("agent dependent wrong: %+v", d)
	}
	if d := byID["wf_1"]; d == nil || d["name"] != "My Flow" || d["kind"] != "workflow" || d["edge"] != relationdomain.KindLink {
		t.Errorf("workflow dependent wrong: %+v", d)
	}
}

// TestPurgeEntity_NotifyEdgeCases: no incoming dependents → no notification (no false alarm); a nil
// emitter is tolerated (CRUD-only wiring); an emit failure never fails the delete (best-effort).
//
// TestPurgeEntity_NotifyEdgeCases：无入向依赖 → 不发通知（不虚惊）；nil emitter 容忍（仅 CRUD 装配）；
// emit 失败绝不让删除失败（尽力而为）。
func TestPurgeEntity_NotifyEdgeCases(t *testing.T) {
	ctx := context.Background()

	// No dependents (only create-provenance + own outgoing) → no emit.
	repo := newFakeRepo()
	repo.rows = []*relationdomain.Relation{
		{ID: "rel_1", Kind: relationdomain.KindCreate, FromKind: "conversation", FromID: "cv_1", ToKind: "function", ToID: "fn_1"},
		{ID: "rel_2", Kind: relationdomain.KindEquip, FromKind: "function", FromID: "fn_1", ToKind: "handler", ToID: "hd_9"},
	}
	em := &fakeEmitter{}
	svc := NewService(Config{Repo: repo, Notif: em, Log: zap.NewNop()})
	if err := svc.PurgeEntity(ctx, "function", "fn_1"); err != nil {
		t.Fatal(err)
	}
	if len(em.types) != 0 {
		t.Fatalf("no dependents must emit nothing, got %v", em.types)
	}

	// Nil emitter (CRUD-only wiring) with real dependents → no panic, delete still succeeds.
	repo2 := newFakeRepo()
	repo2.rows = []*relationdomain.Relation{
		{ID: "rel_1", Kind: relationdomain.KindEquip, FromKind: "agent", FromID: "ag_1", ToKind: "function", ToID: "fn_1"},
	}
	svcNil := NewService(Config{Repo: repo2, Log: zap.NewNop()})
	if err := svcNil.PurgeEntity(ctx, "function", "fn_1"); err != nil {
		t.Fatal(err)
	}

	// Emit failure is swallowed — the delete must still succeed.
	repo3 := newFakeRepo()
	repo3.rows = []*relationdomain.Relation{
		{ID: "rel_1", Kind: relationdomain.KindEquip, FromKind: "agent", FromID: "ag_1", ToKind: "function", ToID: "fn_1"},
	}
	emErr := &fakeEmitter{err: errors.New("emit boom")}
	svcErr := NewService(Config{Repo: repo3, Notif: emErr, Log: zap.NewNop()})
	if err := svcErr.PurgeEntity(ctx, "function", "fn_1"); err != nil {
		t.Fatalf("emit failure must not fail the delete, got %v", err)
	}
	if len(repo3.rows) != 0 {
		t.Errorf("delete must still purge edges despite emit failure, rows=%+v", repo3.rows)
	}
}

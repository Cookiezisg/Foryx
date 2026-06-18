package document

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"

	documentdomain "github.com/sunweilin/anselm/backend/internal/domain/document"
	mentiondomain "github.com/sunweilin/anselm/backend/internal/domain/mention"
	relationdomain "github.com/sunweilin/anselm/backend/internal/domain/relation"
	dbinfra "github.com/sunweilin/anselm/backend/internal/infra/db"
	documentstore "github.com/sunweilin/anselm/backend/internal/infra/store/document"
	reqctxpkg "github.com/sunweilin/anselm/backend/internal/pkg/reqctx"
)

func newSvc(t *testing.T) (*Service, context.Context) {
	t.Helper()
	db, err := dbinfra.Open(dbinfra.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := dbinfra.Migrate(db, documentstore.Schema...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := NewService(documentstore.New(db), nil, zap.NewNop())
	ctx := reqctxpkg.SetWorkspaceID(context.Background(), "ws_test")
	return svc, ctx
}

func TestCreate_AutoUniquify(t *testing.T) {
	svc, ctx := newSvc(t)
	d1, err := svc.Create(ctx, CreateInput{Name: "Note"})
	if err != nil {
		t.Fatalf("create 1: %v", err)
	}
	d2, err := svc.Create(ctx, CreateInput{Name: "Note"})
	if err != nil {
		t.Fatalf("create 2: %v", err)
	}
	if d1.Name != "Note" || d2.Name != "Note 2" {
		t.Errorf("auto-uniquify: %q, %q (want Note, Note 2)", d1.Name, d2.Name)
	}
}

func TestUpdate_PathCascade(t *testing.T) {
	svc, ctx := newSvc(t)
	root, _ := svc.Create(ctx, CreateInput{Name: "Root"})
	child, _ := svc.Create(ctx, CreateInput{Name: "Child", ParentID: &root.ID})
	if child.Path != "/Root/Child" {
		t.Fatalf("initial child path = %q", child.Path)
	}
	if _, err := svc.Update(ctx, root.ID, UpdateInput{Name: strp("Renamed")}); err != nil {
		t.Fatalf("rename: %v", err)
	}
	child2, _ := svc.Get(ctx, child.ID)
	if child2.Path != "/Renamed/Child" {
		t.Errorf("path cascade failed: child path = %q, want /Renamed/Child", child2.Path)
	}
}

func TestDuplicate_SubtreeDeepCopy(t *testing.T) {
	svc, ctx := newSvc(t)
	root, _ := svc.Create(ctx, CreateInput{Name: "Root", Content: "# root body"})
	child, _ := svc.Create(ctx, CreateInput{Name: "Child", ParentID: &root.ID, Content: "child body"})
	gc, _ := svc.Create(ctx, CreateInput{Name: "Grand", ParentID: &child.ID})

	dup, err := svc.Duplicate(ctx, root.ID, nil) // nil parent → sibling of root (root level)
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	// New root: fresh id, name auto-uniquified ("Root" taken → "Root 2"), same content, root-level.
	if dup.ID == root.ID {
		t.Fatal("duplicate must mint a new id")
	}
	if dup.Name != "Root 2" || dup.ParentID != nil || dup.Path != "/Root 2" {
		t.Fatalf("dup root = name:%q parent:%v path:%q, want Root 2 / nil / /Root 2", dup.Name, dup.ParentID, dup.Path)
	}
	if dup.Content != "# root body" {
		t.Errorf("content not copied: %q", dup.Content)
	}
	// The whole subtree is copied with new ids + remapped paths; the original is untouched.
	kids, _ := svc.ListByParent(ctx, &dup.ID)
	if len(kids) != 1 || kids[0].Name != "Child" || kids[0].ID == child.ID || kids[0].Path != "/Root 2/Child" || kids[0].Content != "child body" {
		t.Fatalf("dup child wrong: %+v", kids)
	}
	grandKids, _ := svc.ListByParent(ctx, &kids[0].ID)
	if len(grandKids) != 1 || grandKids[0].Name != "Grand" || grandKids[0].ID == gc.ID || grandKids[0].Path != "/Root 2/Child/Grand" {
		t.Fatalf("dup grandchild wrong: %+v", grandKids)
	}
	// Original subtree unchanged.
	if origKids, _ := svc.ListByParent(ctx, &root.ID); len(origKids) != 1 || origKids[0].ID != child.ID {
		t.Errorf("original subtree mutated: %+v", origKids)
	}
}

func TestDuplicate_NotFound(t *testing.T) {
	svc, ctx := newSvc(t)
	if _, err := svc.Duplicate(ctx, "doc_ghost", nil); !errors.Is(err, documentdomain.ErrNotFound) {
		t.Errorf("duplicate missing = %v, want ErrNotFound", err)
	}
}

func TestMove_CycleGuard(t *testing.T) {
	svc, ctx := newSvc(t)
	root, _ := svc.Create(ctx, CreateInput{Name: "Root"})
	child, _ := svc.Create(ctx, CreateInput{Name: "Child", ParentID: &root.ID})
	// Move root under its own child → cycle.
	if _, err := svc.Move(ctx, root.ID, MoveInput{ParentID: &child.ID}); !errors.Is(err, documentdomain.ErrInvalidParent) {
		t.Errorf("cycle move: err = %v, want ErrInvalidParent", err)
	}
}

// TestMove_StableReorder — regression for F21 (iteration loop): move_document's position is a
// stable insert-at-index, not a raw absolute assign. Moving the last sibling to position 0 must
// make it first AND keep every sibling position unique + contiguous (the old code raw-assigned the
// index, colliding with the occupant; the visible order then fell to created_at, leaving duplicate
// positions). Verified against the service's own ListByParent ordering.
func TestMove_StableReorder(t *testing.T) {
	svc, ctx := newSvc(t)
	a, _ := svc.Create(ctx, CreateInput{Name: "A"})
	b, _ := svc.Create(ctx, CreateInput{Name: "B"})
	c, _ := svc.Create(ctx, CreateInput{Name: "C"})
	_, _ = a, b
	zero := 0
	if _, err := svc.Move(ctx, c.ID, MoveInput{Position: &zero}); err != nil {
		t.Fatalf("move C to position 0: %v", err)
	}
	kids, err := svc.ListByParent(ctx, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var order []string
	seenPos := map[int]bool{}
	for i, d := range kids {
		order = append(order, d.Name)
		if seenPos[d.Position] {
			t.Errorf("duplicate sibling position %d on %q (collision)", d.Position, d.Name)
		}
		seenPos[d.Position] = true
		if d.Position != i {
			t.Errorf("%q position = %d, want %d (contiguous 0..N)", d.Name, d.Position, i)
		}
	}
	if strings.Join(order, ",") != "C,A,B" {
		t.Errorf("order after moving C to 0 = %v, want [C A B]", order)
	}
}

func TestResolveAttached_NoSubtree(t *testing.T) {
	svc, ctx := newSvc(t)
	root, _ := svc.Create(ctx, CreateInput{Name: "Root"})
	svc.Create(ctx, CreateInput{Name: "Child", ParentID: &root.ID})
	// Attaching the root injects ONLY the root — never its subtree.
	docs, err := svc.ResolveAttached(ctx, []documentdomain.AttachedDocument{{DocumentID: root.ID}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(docs) != 1 || docs[0].ID != root.ID {
		t.Errorf("attach should resolve only the single doc, got %d docs", len(docs))
	}
}

func TestValidate(t *testing.T) {
	svc, ctx := newSvc(t)
	if _, err := svc.Create(ctx, CreateInput{Name: ""}); !errors.Is(err, documentdomain.ErrInvalidName) {
		t.Errorf("empty name: %v", err)
	}
	if _, err := svc.Create(ctx, CreateInput{Name: "a/b"}); !errors.Is(err, documentdomain.ErrInvalidName) {
		t.Errorf("slash name: %v", err)
	}
	big := strings.Repeat("x", documentdomain.MaxContentBytes+1)
	if _, err := svc.Create(ctx, CreateInput{Name: "Big", Content: big}); !errors.Is(err, documentdomain.ErrContentTooLarge) {
		t.Errorf("oversized content: %v", err)
	}
}

// ---- adapters ----

func TestCatalogSource_ListItems(t *testing.T) {
	svc, ctx := newSvc(t)
	svc.Create(ctx, CreateInput{Name: "Doc", Description: "my desc"})
	src := svc.AsCatalogSource()
	if src.Name() != "document" {
		t.Errorf("source name = %q", src.Name())
	}
	items, err := src.ListItems(ctx)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 1 || items[0].Source != "document" || items[0].Name != "/Doc" || items[0].Description != "my desc" {
		t.Errorf("catalog item: %+v", items)
	}
}

func TestMentionResolver_Resolve(t *testing.T) {
	svc, ctx := newSvc(t)
	d, _ := svc.Create(ctx, CreateInput{Name: "Doc", Description: "d", Content: "body"})
	res := svc.AsMentionResolver()
	if res.Type() != mentiondomain.MentionDocument {
		t.Errorf("resolver type = %q", res.Type())
	}
	ref, err := res.Resolve(ctx, d.ID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ref.Name != "Doc" || ref.Content != "d\n\nbody" {
		t.Errorf("reference: name=%q content=%q", ref.Name, ref.Content)
	}
}

// fakeRelSyncer captures the last SyncOutgoing call.
type fakeRelSyncer struct {
	fromKind string
	fromID   string
	synced   []relationdomain.SyncEdge
	purged   []string
}

func (f *fakeRelSyncer) SyncOutgoing(_ context.Context, fromKind, fromID string, _ []string, edges []relationdomain.SyncEdge) error {
	f.fromKind, f.fromID, f.synced = fromKind, fromID, edges
	return nil
}

func (f *fakeRelSyncer) PurgeEntity(_ context.Context, _, id string) error {
	f.purged = append(f.purged, id)
	return nil
}

func TestRelations_WikilinkToLinkEdges(t *testing.T) {
	svc, ctx := newSvc(t)
	fake := &fakeRelSyncer{}
	svc.SetRelationSyncer(fake)
	// fn_ → valid function ref (link edge); zz_ → unknown prefix (filtered out).
	svc.Create(ctx, CreateInput{
		Name:    "Doc",
		Content: "see [[fn_0123456789abcdef]] and [[zz_0123456789abcdef]]",
	})
	if len(fake.synced) != 1 {
		t.Fatalf("synced edges = %d, want 1 (zz_ filtered)", len(fake.synced))
	}
	e := fake.synced[0]
	if e.OtherKind != relationdomain.EntityKindFunction || e.OtherID != "fn_0123456789abcdef" || e.Kind != relationdomain.KindLink {
		t.Errorf("link edge: %+v", e)
	}
	if fake.fromKind != relationdomain.EntityKindDocument {
		t.Errorf("fromKind = %q, want document", fake.fromKind)
	}
}

func TestNamesByIDs(t *testing.T) {
	svc, ctx := newSvc(t)
	d, _ := svc.Create(ctx, CreateInput{Name: "Doc"})
	names, err := svc.NamesByIDs(ctx, []string{d.ID, "doc_missing00000000"})
	if err != nil {
		t.Fatalf("names: %v", err)
	}
	if names[d.ID] != "Doc" {
		t.Errorf("name = %q, want Doc", names[d.ID])
	}
	if _, ok := names["doc_missing00000000"]; ok {
		t.Error("missing id should not appear in the name map")
	}
}

func strp(s string) *string { return &s }

package document

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"

	documentdomain "github.com/sunweilin/forgify/backend/internal/domain/document"
	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
	relationdomain "github.com/sunweilin/forgify/backend/internal/domain/relation"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	documentstore "github.com/sunweilin/forgify/backend/internal/infra/store/document"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
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
	svc := New(documentstore.New(db), nil, zap.NewNop())
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

func TestMove_CycleGuard(t *testing.T) {
	svc, ctx := newSvc(t)
	root, _ := svc.Create(ctx, CreateInput{Name: "Root"})
	child, _ := svc.Create(ctx, CreateInput{Name: "Child", ParentID: &root.ID})
	// Move root under its own child → cycle.
	if _, err := svc.Move(ctx, root.ID, MoveInput{ParentID: &child.ID}); !errors.Is(err, documentdomain.ErrInvalidParent) {
		t.Errorf("cycle move: err = %v, want ErrInvalidParent", err)
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

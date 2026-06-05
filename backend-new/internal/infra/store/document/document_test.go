package document

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "github.com/glebarez/go-sqlite"

	documentdomain "github.com/sunweilin/forgify/backend/internal/domain/document"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, stmt := range Schema {
		if _, err := sqlDB.Exec(stmt); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	return New(ormpkg.Open(sqlDB))
}

func ctxWS(id string) context.Context {
	return reqctxpkg.SetWorkspaceID(context.Background(), id)
}

func ins(t *testing.T, s *Store, ctx context.Context, id string, parentID *string, name, path string) {
	t.Helper()
	if err := s.Insert(ctx, &documentdomain.Document{ID: id, ParentID: parentID, Name: name, Path: path}); err != nil {
		t.Fatalf("insert %s: %v", id, err)
	}
}

func ptr(s string) *string { return &s }

func TestInsertGet_RoundTrip(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	ins(t, s, ctx, "doc_1", nil, "Root", "/Root")
	got, err := s.Get(ctx, "doc_1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Root" || got.Path != "/Root" || got.WorkspaceID != "ws_1" {
		t.Errorf("round-trip: %+v", got)
	}
	if got.CreatedAt.IsZero() {
		t.Error("created_at not auto-stamped")
	}
}

func TestNameConflict_SameParentAndRoot(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	ins(t, s, ctx, "doc_1", nil, "Notes", "/Notes")
	// Same name at root (NULL parent) — COALESCE makes it conflict.
	if err := s.Insert(ctx, &documentdomain.Document{ID: "doc_2", ParentID: nil, Name: "Notes", Path: "/Notes"}); !errors.Is(err, documentdomain.ErrNameConflict) {
		t.Errorf("root dup: err = %v, want ErrNameConflict", err)
	}
	// Same name under a parent.
	ins(t, s, ctx, "doc_p", nil, "Parent", "/Parent")
	ins(t, s, ctx, "doc_c1", ptr("doc_p"), "Child", "/Parent/Child")
	if err := s.Insert(ctx, &documentdomain.Document{ID: "doc_c2", ParentID: ptr("doc_p"), Name: "Child", Path: "/Parent/Child"}); !errors.Is(err, documentdomain.ErrNameConflict) {
		t.Errorf("child dup: err = %v, want ErrNameConflict", err)
	}
}

func TestListByParent_Ordered(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	ins(t, s, ctx, "doc_r", nil, "Root", "/Root")
	// Insert children out of position order; ListByParent must sort by position.
	s.Insert(ctx, &documentdomain.Document{ID: "doc_b", ParentID: ptr("doc_r"), Name: "B", Path: "/Root/B", Position: 2})
	s.Insert(ctx, &documentdomain.Document{ID: "doc_a", ParentID: ptr("doc_r"), Name: "A", Path: "/Root/A", Position: 1})
	kids, err := s.ListByParent(ctx, ptr("doc_r"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(kids) != 2 || kids[0].Name != "A" || kids[1].Name != "B" {
		t.Errorf("not position-ordered: %+v", kids)
	}
	roots, _ := s.ListByParent(ctx, nil)
	if len(roots) != 1 || roots[0].ID != "doc_r" {
		t.Errorf("root list: %+v", roots)
	}
}

func TestSubtreeBFS_And_CountDescendants(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	ins(t, s, ctx, "doc_r", nil, "R", "/R")
	ins(t, s, ctx, "doc_c", ptr("doc_r"), "C", "/R/C")
	ins(t, s, ctx, "doc_g", ptr("doc_c"), "G", "/R/C/G")

	ids, err := s.ListSubtreeIDs(ctx, "doc_r")
	if err != nil {
		t.Fatalf("subtree: %v", err)
	}
	if len(ids) != 3 {
		t.Errorf("subtree ids = %v, want 3", ids)
	}
	n, _ := s.CountDescendants(ctx, "doc_r")
	if n != 2 {
		t.Errorf("descendants = %d, want 2", n)
	}
}

func TestIsAncestor(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	ins(t, s, ctx, "doc_r", nil, "R", "/R")
	ins(t, s, ctx, "doc_c", ptr("doc_r"), "C", "/R/C")
	ins(t, s, ctx, "doc_g", ptr("doc_c"), "G", "/R/C/G")

	if ok, _ := s.IsAncestor(ctx, "doc_r", "doc_g"); !ok {
		t.Error("doc_r should be ancestor of doc_g")
	}
	if ok, _ := s.IsAncestor(ctx, "doc_g", "doc_r"); ok {
		t.Error("doc_g must NOT be ancestor of doc_r")
	}
}

func TestSoftDeleteSubtree_AndNameReuse(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	ins(t, s, ctx, "doc_r", nil, "R", "/R")
	ins(t, s, ctx, "doc_c", ptr("doc_r"), "C", "/R/C")

	n, err := s.SoftDeleteSubtree(ctx, "doc_r")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n != 2 {
		t.Errorf("deletedCount = %d, want 2", n)
	}
	if _, err := s.Get(ctx, "doc_r"); !errors.Is(err, documentdomain.ErrNotFound) {
		t.Errorf("deleted root should miss, err = %v", err)
	}
	// Name freed after soft-delete (partial unique index).
	if err := s.Insert(ctx, &documentdomain.Document{ID: "doc_r2", ParentID: nil, Name: "R", Path: "/R"}); err != nil {
		t.Errorf("name should be reusable after soft-delete, got %v", err)
	}
}

func TestMaxSiblingPosition(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	if p, _ := s.MaxSiblingPosition(ctx, nil); p != -1 {
		t.Errorf("empty max = %d, want -1", p)
	}
	s.Insert(ctx, &documentdomain.Document{ID: "doc_a", Name: "A", Path: "/A", Position: 5})
	if p, _ := s.MaxSiblingPosition(ctx, nil); p != 5 {
		t.Errorf("max = %d, want 5", p)
	}
}

func TestWorkspaceIsolation(t *testing.T) {
	s := newStore(t)
	ins(t, s, ctxWS("ws_1"), "doc_1", nil, "Secret", "/Secret")
	if _, err := s.Get(ctxWS("ws_2"), "doc_1"); !errors.Is(err, documentdomain.ErrNotFound) {
		t.Errorf("cross-workspace Get should miss, err = %v", err)
	}
}

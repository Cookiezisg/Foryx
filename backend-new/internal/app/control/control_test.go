package control

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "github.com/glebarez/go-sqlite"
	"go.uber.org/zap"

	controldomain "github.com/sunweilin/forgify/backend/internal/domain/control"
	controlstore "github.com/sunweilin/forgify/backend/internal/infra/store/control"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

func newSvc(t *testing.T) (*Service, context.Context) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, stmt := range controlstore.Schema {
		if _, err := sqlDB.Exec(stmt); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	svc := NewService(controlstore.New(ormpkg.Open(sqlDB)), nil, zap.NewNop())
	return svc, reqctxpkg.SetWorkspaceID(context.Background(), "ws_1")
}

func catchAll(port string) controldomain.Branch {
	return controldomain.Branch{Port: port, When: "true"}
}

func TestCreate_WritesV1Active(t *testing.T) {
	svc, ctx := newSvc(t)
	c, v, err := svc.Create(ctx, CreateInput{
		Name: "router", Description: "route by score",
		Branches: []controldomain.Branch{
			{Port: "pass", When: "input.score >= 0.9", Emit: map[string]string{"ok": "true"}},
			catchAll("retry"),
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if v.Version != 1 || c.ActiveVersionID != v.ID {
		t.Fatalf("v1 active: c=%+v v=%+v", c, v)
	}
	got, err := svc.Get(ctx, c.ID)
	if err != nil || got.ActiveVersion == nil || len(got.ActiveVersion.Branches) != 2 {
		t.Fatalf("Get active branches: %+v err=%v", got, err)
	}
	if got.ActiveVersion.Branches[0].Emit["ok"] != "true" {
		t.Fatalf("emit not round-tripped: %+v", got.ActiveVersion.Branches[0])
	}
}

func TestCreate_InvalidWhenCEL(t *testing.T) {
	svc, ctx := newSvc(t)
	_, _, err := svc.Create(ctx, CreateInput{Name: "bad", Branches: []controldomain.Branch{
		{Port: "x", When: "input.("}, catchAll("y"),
	}})
	if !errors.Is(err, controldomain.ErrInvalidCEL) {
		t.Fatalf("want ErrInvalidCEL, got %v", err)
	}
}

func TestCreate_InvalidEmitCEL(t *testing.T) {
	svc, ctx := newSvc(t)
	_, _, err := svc.Create(ctx, CreateInput{Name: "bad2", Branches: []controldomain.Branch{
		{Port: "x", When: "true", Emit: map[string]string{"k": "input.("}},
	}})
	if !errors.Is(err, controldomain.ErrInvalidCEL) {
		t.Fatalf("want ErrInvalidCEL (emit), got %v", err)
	}
}

func TestCreate_NoCatchAll(t *testing.T) {
	svc, ctx := newSvc(t)
	_, _, err := svc.Create(ctx, CreateInput{Name: "nc", Branches: []controldomain.Branch{
		{Port: "x", When: "input.a > 1"},
	}})
	if !errors.Is(err, controldomain.ErrNoCatchAll) {
		t.Fatalf("want ErrNoCatchAll, got %v", err)
	}
}

func TestCreate_EmptyName(t *testing.T) {
	svc, ctx := newSvc(t)
	_, _, err := svc.Create(ctx, CreateInput{Name: "  ", Branches: []controldomain.Branch{catchAll("o")}})
	if !errors.Is(err, controldomain.ErrInvalidName) {
		t.Fatalf("want ErrInvalidName, got %v", err)
	}
}

func TestCreate_DuplicateName(t *testing.T) {
	svc, ctx := newSvc(t)
	mk := func() error {
		_, _, err := svc.Create(ctx, CreateInput{Name: "dup", Branches: []controldomain.Branch{catchAll("o")}})
		return err
	}
	if err := mk(); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := mk(); !errors.Is(err, controldomain.ErrDuplicateName) {
		t.Fatalf("want ErrDuplicateName, got %v", err)
	}
}

func TestEdit_NewVersionPointerMoves(t *testing.T) {
	svc, ctx := newSvc(t)
	c, _, _ := svc.Create(ctx, CreateInput{Name: "e", Branches: []controldomain.Branch{catchAll("a")}})
	v2, err := svc.Edit(ctx, EditInput{ID: c.ID, Branches: []controldomain.Branch{
		{Port: "a", When: "input.x > 0"}, catchAll("b"),
	}})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if v2.Version != 2 {
		t.Fatalf("want v2, got %d", v2.Version)
	}
	got, _ := svc.Get(ctx, c.ID)
	if got.ActiveVersionID != v2.ID {
		t.Fatal("active should be v2")
	}
	if _, err := svc.GetVersionByNumber(ctx, c.ID, 1); err != nil {
		t.Fatalf("v1 should be retained: %v", err)
	}
}

func TestRevert_MovesPointer(t *testing.T) {
	svc, ctx := newSvc(t)
	c, v1, _ := svc.Create(ctx, CreateInput{Name: "r", Branches: []controldomain.Branch{catchAll("a")}})
	if _, err := svc.Edit(ctx, EditInput{ID: c.ID, Branches: []controldomain.Branch{catchAll("b")}}); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if _, err := svc.Revert(ctx, c.ID, 1); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	got, _ := svc.Get(ctx, c.ID)
	if got.ActiveVersionID != v1.ID {
		t.Fatal("active should be v1 after revert")
	}
}

func TestUpdateMeta_NoVersionBump(t *testing.T) {
	svc, ctx := newSvc(t)
	c, _, _ := svc.Create(ctx, CreateInput{Name: "m", Branches: []controldomain.Branch{catchAll("a")}})
	name := "renamed"
	if _, err := svc.UpdateMeta(ctx, UpdateMetaInput{ID: c.ID, Name: &name}); err != nil {
		t.Fatalf("UpdateMeta: %v", err)
	}
	got, _ := svc.Get(ctx, c.ID)
	if got.Name != "renamed" {
		t.Fatalf("name not updated: %q", got.Name)
	}
	if n, _ := svc.repo.MaxVersionNumber(ctx, c.ID); n != 1 {
		t.Fatalf("UpdateMeta must not bump version, max=%d", n)
	}
}

func TestSearch_Substring(t *testing.T) {
	svc, ctx := newSvc(t)
	svc.Create(ctx, CreateInput{Name: "invoice-router", Branches: []controldomain.Branch{catchAll("a")}})
	svc.Create(ctx, CreateInput{Name: "spam-filter", Branches: []controldomain.Branch{catchAll("a")}})
	hits, err := svc.Search(ctx, "router")
	if err != nil || len(hits) != 1 || hits[0].Name != "invoice-router" {
		t.Fatalf("search router: %v hits=%v", err, hits)
	}
	all, _ := svc.Search(ctx, "")
	if len(all) != 2 {
		t.Fatalf("empty query should list all, got %d", len(all))
	}
}

func TestDelete_SoftDeleted(t *testing.T) {
	svc, ctx := newSvc(t)
	c, _, _ := svc.Create(ctx, CreateInput{Name: "d", Branches: []controldomain.Branch{catchAll("a")}})
	if err := svc.Delete(ctx, c.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, c.ID); !errors.Is(err, controldomain.ErrNotFound) {
		t.Fatalf("deleted should be NotFound, got %v", err)
	}
}

func TestResolve_ActiveAndPinned(t *testing.T) {
	svc, ctx := newSvc(t)
	c, v1, _ := svc.Create(ctx, CreateInput{Name: "rs", Branches: []controldomain.Branch{catchAll("a")}})
	if _, err := svc.Edit(ctx, EditInput{ID: c.ID, Branches: []controldomain.Branch{
		{Port: "x", When: "input.n > 0"}, catchAll("b"),
	}}); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	act, err := svc.Resolve(ctx, c.ID, "")
	if err != nil || len(act) != 2 {
		t.Fatalf("resolve active: %v len=%d", err, len(act))
	}
	pin, err := svc.Resolve(ctx, c.ID, v1.ID)
	if err != nil || len(pin) != 1 || pin[0].Port != "a" {
		t.Fatalf("resolve pinned v1: %v %+v", err, pin)
	}
}

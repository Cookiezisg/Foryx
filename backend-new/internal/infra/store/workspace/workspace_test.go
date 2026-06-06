package workspace

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "github.com/glebarez/go-sqlite"

	workspacedomain "github.com/sunweilin/forgify/backend/internal/domain/workspace"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
)

// newStore opens an in-memory db, applies the workspaces schema, returns a Store.
//
// newStore 开内存 db、应用 workspaces schema、返回 Store。
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

func mustSave(t *testing.T, s *Store, id, name string) {
	t.Helper()
	w := &workspacedomain.Workspace{ID: id, Name: name, Language: workspacedomain.LanguageZhCN}
	if err := s.Save(context.Background(), w); err != nil {
		t.Fatalf("save %s: %v", id, err)
	}
}

func TestStore_SaveGetRoundTrip(t *testing.T) {
	s := newStore(t)
	mustSave(t, s, "ws_1", "Alpha")

	got, err := s.Get(context.Background(), "ws_1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Alpha" || got.Language != workspacedomain.LanguageZhCN {
		t.Errorf("got %+v", got)
	}
	// Save auto-stamps timestamps even with no workspace in ctx (isolation root).
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Error("save should stamp created/updated")
	}
}

func TestStore_DefaultSearchKeyID_RoundTrip(t *testing.T) {
	s := newStore(t)
	w := &workspacedomain.Workspace{
		ID: "ws_1", Name: "Alpha", Language: workspacedomain.LanguageZhCN,
		DefaultSearchKeyID: "aki_search",
	}
	if err := s.Save(context.Background(), w); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.Get(context.Background(), "ws_1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.DefaultSearchKeyID != "aki_search" {
		t.Fatalf("DefaultSearchKeyID round-trip = %q, want aki_search", got.DefaultSearchKeyID)
	}
}

func TestStore_DuplicateName_ErrNameConflict(t *testing.T) {
	s := newStore(t)
	mustSave(t, s, "ws_1", "Dup")
	// Different id, same name → UNIQUE(name) → orm.ErrConflict → domain ErrNameConflict.
	err := s.Save(context.Background(), &workspacedomain.Workspace{
		ID: "ws_2", Name: "Dup", Language: workspacedomain.LanguageZhCN,
	})
	if !errors.Is(err, workspacedomain.ErrNameConflict) {
		t.Errorf("duplicate name: err = %v, want ErrNameConflict", err)
	}
}

func TestStore_GetMissing_ErrNotFound(t *testing.T) {
	s := newStore(t)
	if _, err := s.Get(context.Background(), "ws_missing"); !errors.Is(err, workspacedomain.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestStore_ListNoWorkspaceCtx(t *testing.T) {
	s := newStore(t)
	// The isolation root has no workspace_id, so List must work with a bare ctx —
	// onboarding lists workspaces before any is selected.
	mustSave(t, s, "ws_1", "First")
	mustSave(t, s, "ws_2", "Second")

	rows, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("list len = %d, want 2", len(rows))
	}
	ids := map[string]bool{rows[0].ID: true, rows[1].ID: true}
	if !ids["ws_1"] || !ids["ws_2"] {
		t.Errorf("list missing ids: %+v", rows)
	}
}

func TestStore_SoftDelete_ThenMisses_NameReusable(t *testing.T) {
	s := newStore(t)
	mustSave(t, s, "ws_1", "Name")

	if err := s.Delete(context.Background(), "ws_1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Get(context.Background(), "ws_1"); !errors.Is(err, workspacedomain.ErrNotFound) {
		t.Errorf("soft-deleted should miss, err = %v", err)
	}
	// Partial unique index excludes soft-deleted rows → the name is free again.
	if err := s.Save(context.Background(), &workspacedomain.Workspace{
		ID: "ws_2", Name: "Name", Language: workspacedomain.LanguageZhCN,
	}); err != nil {
		t.Errorf("name should be reusable after soft delete, got %v", err)
	}
}

func TestStore_TouchLastUsed(t *testing.T) {
	s := newStore(t)
	mustSave(t, s, "ws_1", "W")
	if err := s.TouchLastUsed(context.Background(), "ws_1"); err != nil {
		t.Fatalf("touch: %v", err)
	}
	got, _ := s.Get(context.Background(), "ws_1")
	if got.LastUsedAt == nil {
		t.Error("last_used_at should be set")
	}
	if err := s.TouchLastUsed(context.Background(), "ws_missing"); !errors.Is(err, workspacedomain.ErrNotFound) {
		t.Errorf("touch missing: err = %v, want ErrNotFound", err)
	}
}

func TestStore_Count(t *testing.T) {
	s := newStore(t)
	mustSave(t, s, "ws_1", "A")
	mustSave(t, s, "ws_2", "B")
	n, err := s.Count(context.Background())
	if err != nil || n != 2 {
		t.Errorf("count = %d, err = %v; want 2", n, err)
	}
}

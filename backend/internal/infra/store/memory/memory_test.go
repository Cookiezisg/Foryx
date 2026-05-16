package memory

import (
	"context"
	"errors"
	"testing"

	gormlogger "gorm.io/gorm/logger"

	memorydomain "github.com/sunweilin/forgify/backend/internal/domain/memory"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	db, err := dbinfra.Open(dbinfra.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(db) })
	if err := dbinfra.Migrate(db, AutoMigrateModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(db)
}

func ctxFor() context.Context { return context.Background() }

func mkMem(id, name, ty, desc string) *memorydomain.Memory {
	return &memorydomain.Memory{
		ID: id, Name: name, Type: ty, Description: desc, Content: "...",
		Source: memorydomain.SourceUser,
	}
}

func TestSave_InsertAndGetByName(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor()
	m := mkMem("mem_1", "user_role", memorydomain.TypeUser, "Go engineer")
	if err := s.Save(ctx, m); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.GetByName(ctx, "user_role")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.Description != "Go engineer" {
		t.Errorf("Description = %q", got.Description)
	}
}

func TestSave_DuplicateName_ReturnsErrNameConflict(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor()
	if err := s.Save(ctx, mkMem("mem_1", "user_role", memorydomain.TypeUser, "first")); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	err := s.Save(ctx, mkMem("mem_2", "user_role", memorydomain.TypeUser, "second"))
	if !errors.Is(err, memorydomain.ErrNameConflict) {
		t.Errorf("Save second with same name: got %v, want ErrNameConflict", err)
	}
}

func TestSave_SoftDeletedReleaseslName(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor()
	if err := s.Save(ctx, mkMem("mem_1", "user_role", memorydomain.TypeUser, "first")); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	if err := s.Delete(ctx, "user_role"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Save(ctx, mkMem("mem_2", "user_role", memorydomain.TypeUser, "reborn")); err != nil {
		t.Errorf("Save after soft-delete: %v", err)
	}
}

func TestGetByName_NotFound(t *testing.T) {
	s := newStore(t)
	_, err := s.GetByName(ctxFor(), "nope")
	if !errors.Is(err, memorydomain.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestList_FilterByTypeAndPinned(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor()
	must := func(m *memorydomain.Memory) {
		t.Helper()
		if err := s.Save(ctx, m); err != nil {
			t.Fatalf("Save %s: %v", m.Name, err)
		}
	}
	m1 := mkMem("mem_1", "a", memorydomain.TypeUser, "")
	m1.Pinned = true
	m2 := mkMem("mem_2", "b", memorydomain.TypeUser, "")
	m3 := mkMem("mem_3", "c", memorydomain.TypeFeedback, "")
	m3.Pinned = true
	must(m1)
	must(m2)
	must(m3)

	all, err := s.List(ctx, memorydomain.ListFilter{})
	if err != nil || len(all) != 3 {
		t.Fatalf("List all: len=%d err=%v", len(all), err)
	}

	users, _ := s.List(ctx, memorydomain.ListFilter{Type: memorydomain.TypeUser})
	if len(users) != 2 {
		t.Errorf("type=user: len=%d, want 2", len(users))
	}

	yes := true
	pinned, _ := s.List(ctx, memorydomain.ListFilter{Pinned: &yes})
	if len(pinned) != 2 {
		t.Errorf("pinned=true: len=%d, want 2", len(pinned))
	}
}

func TestListPinned(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor()
	m1 := mkMem("mem_1", "a", memorydomain.TypeUser, "")
	m1.Pinned = true
	m2 := mkMem("mem_2", "b", memorydomain.TypeUser, "")
	_ = s.Save(ctx, m1)
	_ = s.Save(ctx, m2)
	rows, err := s.ListPinned(ctx)
	if err != nil || len(rows) != 1 || rows[0].Name != "a" {
		t.Errorf("ListPinned: rows=%+v err=%v", rows, err)
	}
}

func TestListForIndex_NonPinnedOnly(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor()
	for i, name := range []string{"a", "b", "c"} {
		m := mkMem("mem_"+name, name, memorydomain.TypeUser, "")
		if i == 0 {
			m.Pinned = true
		}
		_ = s.Save(ctx, m)
	}
	rows, err := s.ListForIndex(ctx, 100)
	if err != nil {
		t.Fatalf("ListForIndex: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("ListForIndex len=%d, want 2 (skip pinned)", len(rows))
	}
	for _, r := range rows {
		if r.Pinned {
			t.Errorf("ListForIndex returned pinned: %s", r.Name)
		}
	}
}

func TestMarkAccessed_BumpsCount(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor()
	_ = s.Save(ctx, mkMem("mem_1", "a", memorydomain.TypeUser, ""))
	if err := s.MarkAccessed(ctx, "a"); err != nil {
		t.Fatalf("MarkAccessed: %v", err)
	}
	if err := s.MarkAccessed(ctx, "a"); err != nil {
		t.Fatalf("MarkAccessed 2: %v", err)
	}
	got, _ := s.GetByName(ctx, "a")
	if got.AccessCount != 2 {
		t.Errorf("AccessCount=%d, want 2", got.AccessCount)
	}
	if got.AccessedAt == nil {
		t.Errorf("AccessedAt should be set")
	}
}

func TestMarkAccessed_UnknownName(t *testing.T) {
	s := newStore(t)
	err := s.MarkAccessed(ctxFor(), "nope")
	if !errors.Is(err, memorydomain.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	s := newStore(t)
	err := s.Delete(ctxFor(), "nope")
	if !errors.Is(err, memorydomain.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

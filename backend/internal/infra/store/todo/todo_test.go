// todo_test.go — integration tests for todostore.Store against an
// in-memory SQLite. Covers Create / Get / List / Update / SoftDelete
// happy paths + soft-delete invisibility.
//
// todo_test.go — todostore.Store 在内存 SQLite 上的集成测试。
package todo

import (
	"context"
	"errors"
	"testing"

	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"

	tododomain "github.com/sunweilin/forgify/backend/internal/domain/todo"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := dbinfra.Open(dbinfra.Config{DataDir: ""})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := dbinfra.Migrate(db, &tododomain.Todo{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(db)
}

func TestStore_CreateAndGet_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	in := &tododomain.Todo{
		ID:             "td_test_001",
		ConversationID: "cv_alpha",
		Subject:        "Run tests",
		ActiveForm:     "Running tests",
		Status:         tododomain.StatusPending,
	}
	if err := s.Create(ctx, in); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := s.Get(ctx, "td_test_001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Subject != "Run tests" || got.ConversationID != "cv_alpha" {
		t.Errorf("got %+v", got)
	}
}

func TestStore_Get_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Get(context.Background(), "td_does_not_exist")
	if !errors.Is(err, tododomain.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestStore_ListByConversation_OrdersByCreatedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for i, id := range []string{"td_a", "td_b", "td_c"} {
		t := &tododomain.Todo{
			ID:             id,
			ConversationID: "cv_x",
			Subject:        id,
			Status:         tododomain.StatusPending,
		}
		if err := s.Create(ctx, t); err != nil {
			panic(err)
		}
		_ = i
	}
	rows, err := s.ListByConversation(ctx, "cv_x")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("len = %d, want 3", len(rows))
	}
	// Each created later than the previous → ascending order preserved.
	// 后建的在后；升序保持。
	for i := 1; i < len(rows); i++ {
		if rows[i].CreatedAt.Before(rows[i-1].CreatedAt) {
			t.Errorf("not ascending: rows[%d]=%v then rows[%d]=%v",
				i-1, rows[i-1].CreatedAt, i, rows[i].CreatedAt)
		}
	}
}

func TestStore_ListByConversation_FiltersByConvID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.Create(ctx, &tododomain.Todo{ID: "td_a", ConversationID: "cv_alpha", Subject: "a", Status: tododomain.StatusPending}); err != nil {
		t.Fatalf("seed cv_alpha: %v", err)
	}
	if err := s.Create(ctx, &tododomain.Todo{ID: "td_b", ConversationID: "cv_beta", Subject: "b", Status: tododomain.StatusPending}); err != nil {
		t.Fatalf("seed cv_beta: %v", err)
	}
	rows, err := s.ListByConversation(ctx, "cv_alpha")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "td_a" {
		t.Errorf("expected only td_a, got %+v", rows)
	}
}

func TestStore_Update_PersistsChanges(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	in := &tododomain.Todo{ID: "td_u", ConversationID: "cv_x", Subject: "old", Status: tododomain.StatusPending}
	if err := s.Create(ctx, in); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _ := s.Get(ctx, "td_u")
	got.Subject = "new"
	got.Status = tododomain.StatusInProgress
	if err := s.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	round, err := s.Get(ctx, "td_u")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if round.Subject != "new" || round.Status != tododomain.StatusInProgress {
		t.Errorf("round-trip: %+v", round)
	}
}

func TestStore_SoftDelete_HidesRow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.Create(ctx, &tododomain.Todo{ID: "td_del", ConversationID: "cv_x", Subject: "x", Status: tododomain.StatusPending}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.SoftDelete(ctx, "td_del"); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	_, err := s.Get(ctx, "td_del")
	if !errors.Is(err, tododomain.ErrNotFound) {
		t.Errorf("after SoftDelete want ErrNotFound, got %v", err)
	}
	rows, _ := s.ListByConversation(ctx, "cv_x")
	if len(rows) != 0 {
		t.Errorf("list should exclude soft-deleted; got %d rows", len(rows))
	}
}

func TestStore_SoftDelete_UnknownID_ReturnsNotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.SoftDelete(context.Background(), "td_nope")
	if !errors.Is(err, tododomain.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

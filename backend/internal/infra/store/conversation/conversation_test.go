package conversation

import (
	"context"
	"errors"
	"testing"
	"time"

	gormlogger "gorm.io/gorm/logger"

	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

const (
	userAlice = "u-alice"
	userBob   = "u-bob"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	database, err := dbinfra.Open(dbinfra.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(database) })
	if err := dbinfra.Migrate(database, &convdomain.Conversation{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(database)
}

func ctxFor(uid string) context.Context {
	return reqctxpkg.SetUserID(context.Background(), uid)
}

func mkConv(id, uid, title string) *convdomain.Conversation {
	return &convdomain.Conversation{ID: id, UserID: uid, Title: title}
}

func TestSave_InsertAndGet(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	c := mkConv("cv1", userAlice, "My Chat")
	if err := s.Save(ctx, c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Get(ctx, "cv1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "My Chat" {
		t.Errorf("Title = %q, want My Chat", got.Title)
	}
}

func TestSave_ExistingRowReplaced(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	c := mkConv("cv1", userAlice, "Old")
	if err := s.Save(ctx, c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	c.Title = "New"
	if err := s.Save(ctx, c); err != nil {
		t.Fatalf("Save update: %v", err)
	}
	got, _ := s.Get(ctx, "cv1")
	if got.Title != "New" {
		t.Errorf("Title = %q, want New", got.Title)
	}
}

func TestGet_NotFound(t *testing.T) {
	s := newStore(t)
	_, err := s.Get(ctxFor(userAlice), "missing")
	if !errors.Is(err, convdomain.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestGet_CrossUserIsolation(t *testing.T) {
	s := newStore(t)
	if err := s.Save(ctxFor(userAlice), mkConv("cv1", userAlice, "Alice")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	_, err := s.Get(ctxFor(userBob), "cv1")
	if !errors.Is(err, convdomain.ErrNotFound) {
		t.Errorf("Bob sees Alice's conversation: got %v", err)
	}
}

func TestGet_MissingUserID(t *testing.T) {
	s := newStore(t)
	_, err := s.Get(context.Background(), "cv1")
	if err == nil {
		t.Fatal("want wiring error, got nil")
	}
	if errors.Is(err, convdomain.ErrNotFound) {
		t.Error("wiring bug leaked as ErrNotFound")
	}
}

func TestDelete_SoftDeletes(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	if err := s.Save(ctx, mkConv("cv1", userAlice, "test")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Delete(ctx, "cv1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := s.Get(ctx, "cv1")
	if !errors.Is(err, convdomain.ErrNotFound) {
		t.Errorf("Get after Delete: got %v, want ErrNotFound", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	s := newStore(t)
	err := s.Delete(ctxFor(userAlice), "missing")
	if !errors.Is(err, convdomain.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestDelete_CrossUserIsolation(t *testing.T) {
	s := newStore(t)
	if err := s.Save(ctxFor(userAlice), mkConv("cv1", userAlice, "Alice")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Delete(ctxFor(userBob), "cv1"); !errors.Is(err, convdomain.ErrNotFound) {
		t.Errorf("Bob deleting Alice's conversation: got %v, want ErrNotFound", err)
	}
	if _, err := s.Get(ctxFor(userAlice), "cv1"); err != nil {
		t.Errorf("Alice's conversation gone after Bob's failed delete: %v", err)
	}
}

func TestList_Basic(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	for _, id := range []string{"a", "b", "c"} {
		if err := s.Save(ctx, mkConv(id, userAlice, id)); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	rows, next, err := s.List(ctx, convdomain.ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	if next != "" {
		t.Errorf("unexpected nextCursor: %q", next)
	}
	if rows[0].ID != "c" || rows[2].ID != "a" {
		t.Errorf("order wrong: [%s %s %s], want [c b a]", rows[0].ID, rows[1].ID, rows[2].ID)
	}
}

func TestList_Pagination(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	for _, id := range []string{"a", "b", "c", "d", "e"} {
		if err := s.Save(ctx, mkConv(id, userAlice, id)); err != nil {
			t.Fatalf("Save: %v", err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	page1, cursor, err := s.List(ctx, convdomain.ListFilter{Limit: 2})
	if err != nil || len(page1) != 2 || cursor == "" {
		t.Fatalf("page1: len=%d cursor=%q err=%v", len(page1), cursor, err)
	}
	page2, cursor2, err := s.List(ctx, convdomain.ListFilter{Limit: 2, Cursor: cursor})
	if err != nil || len(page2) != 2 || cursor2 == "" {
		t.Fatalf("page2: len=%d cursor=%q err=%v", len(page2), cursor2, err)
	}
	page3, next, err := s.List(ctx, convdomain.ListFilter{Limit: 2, Cursor: cursor2})
	if err != nil || len(page3) != 1 || next != "" {
		t.Fatalf("page3: len=%d next=%q err=%v", len(page3), next, err)
	}
}

func TestList_CrossUserIsolation(t *testing.T) {
	s := newStore(t)
	if err := s.Save(ctxFor(userAlice), mkConv("a1", userAlice, "Alice")); err != nil {
		t.Fatalf("Save Alice: %v", err)
	}
	if err := s.Save(ctxFor(userBob), mkConv("b1", userBob, "Bob")); err != nil {
		t.Fatalf("Save Bob: %v", err)
	}
	rows, _, _ := s.List(ctxFor(userAlice), convdomain.ListFilter{Limit: 10})
	if len(rows) != 1 || rows[0].ID != "a1" {
		t.Errorf("Alice sees wrong rows: %+v", rows)
	}
}

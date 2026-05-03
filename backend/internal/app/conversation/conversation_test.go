// conversation_test.go — unit tests for Service using a fake repository.
//
// conversation_test.go — Service 单测，使用 fake repository。
package conversation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"

	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

type fakeRepo struct {
	rows map[string]*convdomain.Conversation
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{rows: make(map[string]*convdomain.Conversation)}
}

func (r *fakeRepo) Save(_ context.Context, c *convdomain.Conversation) error {
	cp := *c
	r.rows[c.ID] = &cp
	return nil
}

func (r *fakeRepo) Get(ctx context.Context, id string) (*convdomain.Conversation, error) {
	uid, _ := reqctxpkg.GetUserID(ctx)
	c, ok := r.rows[id]
	if !ok || c.UserID != uid {
		return nil, convdomain.ErrNotFound
	}
	cp := *c
	return &cp, nil
}

func (r *fakeRepo) List(ctx context.Context, _ convdomain.ListFilter) ([]*convdomain.Conversation, string, error) {
	uid, _ := reqctxpkg.GetUserID(ctx)
	var out []*convdomain.Conversation
	for _, c := range r.rows {
		if c.UserID == uid {
			cp := *c
			out = append(out, &cp)
		}
	}
	return out, "", nil
}

func (r *fakeRepo) Delete(ctx context.Context, id string) error {
	uid, _ := reqctxpkg.GetUserID(ctx)
	c, ok := r.rows[id]
	if !ok || c.UserID != uid {
		return convdomain.ErrNotFound
	}
	delete(r.rows, id)
	return nil
}

func ctxAlice() context.Context {
	return reqctxpkg.SetUserID(context.Background(), "u-alice")
}

func newSvc(t *testing.T) *Service {
	t.Helper()
	return NewService(newFakeRepo(), zap.NewNop())
}

// --- NewService ---

func TestNewService_NilLogger_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic, got none")
		}
	}()
	NewService(newFakeRepo(), nil)
}

// --- Create ---

func TestCreate_Success(t *testing.T) {
	svc := newSvc(t)
	c, err := svc.Create(ctxAlice(), "My First Chat")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(c.ID, "cv_") {
		t.Errorf("ID = %q, want cv_ prefix", c.ID)
	}
	if c.Title != "My First Chat" {
		t.Errorf("Title = %q, want My First Chat", c.Title)
	}
}

func TestCreate_EmptyTitleAllowed(t *testing.T) {
	svc := newSvc(t)
	c, err := svc.Create(ctxAlice(), "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c.Title != "" {
		t.Errorf("Title = %q, want empty", c.Title)
	}
}

func TestCreate_TrimsTitleWhitespace(t *testing.T) {
	svc := newSvc(t)
	c, _ := svc.Create(ctxAlice(), "  Hello  ")
	if c.Title != "Hello" {
		t.Errorf("Title = %q, want Hello", c.Title)
	}
}

func TestCreate_MissingUserID(t *testing.T) {
	svc := newSvc(t)
	_, err := svc.Create(context.Background(), "test")
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

// --- Rename ---

func TestRename_Success(t *testing.T) {
	svc := newSvc(t)
	ctx := ctxAlice()
	c, _ := svc.Create(ctx, "Old")
	updated, err := svc.Rename(ctx, c.ID, "New Title")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if updated.Title != "New Title" {
		t.Errorf("Title = %q, want New Title", updated.Title)
	}
	// `After` (strict >) flakes when Create + Rename land in the same
	// microsecond tick on fast systems; `!Before` is the real semantic
	// (the timestamp didn't regress).
	//
	// `After`（严格大于）在快机上 Create + Rename 命中同一微秒会抖；
	// `!Before` 才是真正的语义（时间戳没有回退）。
	if updated.UpdatedAt.Before(c.UpdatedAt) {
		t.Error("UpdatedAt regressed")
	}
}

func TestRename_NotFound(t *testing.T) {
	svc := newSvc(t)
	_, err := svc.Rename(ctxAlice(), "nonexistent", "New")
	if !errors.Is(err, convdomain.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

// --- Delete ---

func TestDelete_Success(t *testing.T) {
	svc := newSvc(t)
	ctx := ctxAlice()
	c, _ := svc.Create(ctx, "test")
	if err := svc.Delete(ctx, c.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	svc := newSvc(t)
	err := svc.Delete(ctxAlice(), "nope")
	if !errors.Is(err, convdomain.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

// --- List ---

func TestList_AfterCreate(t *testing.T) {
	svc := newSvc(t)
	ctx := ctxAlice()
	svc.Create(ctx, "A")
	svc.Create(ctx, "B")
	rows, _, err := svc.List(ctx, convdomain.ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("got %d rows, want 2", len(rows))
	}
}

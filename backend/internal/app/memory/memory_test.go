package memory

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	memorydomain "github.com/sunweilin/forgify/backend/internal/domain/memory"
)

type fakeRepo struct {
	mu    sync.Mutex
	items map[string]*memorydomain.Memory
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{items: map[string]*memorydomain.Memory{}}
}

func (r *fakeRepo) Save(_ context.Context, m *memorydomain.Memory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.items {
		if existing.Name == m.Name && existing.ID != m.ID {
			return memorydomain.ErrNameConflict
		}
	}
	r.items[m.Name] = m
	return nil
}

func (r *fakeRepo) GetByName(_ context.Context, name string) (*memorydomain.Memory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.items[name]
	if !ok {
		return nil, memorydomain.ErrNotFound
	}
	return m, nil
}

func (r *fakeRepo) GetByID(_ context.Context, id string) (*memorydomain.Memory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range r.items {
		if m.ID == id {
			return m, nil
		}
	}
	return nil, memorydomain.ErrNotFound
}

func (r *fakeRepo) List(_ context.Context, filter memorydomain.ListFilter) ([]*memorydomain.Memory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*memorydomain.Memory{}
	for _, m := range r.items {
		if filter.Type != "" && m.Type != filter.Type {
			continue
		}
		if filter.Pinned != nil && m.Pinned != *filter.Pinned {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *fakeRepo) ListPinned(_ context.Context) ([]*memorydomain.Memory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*memorydomain.Memory{}
	for _, m := range r.items {
		if m.Pinned {
			out = append(out, m)
		}
	}
	return out, nil
}

func (r *fakeRepo) ListForIndex(_ context.Context, limit int) ([]*memorydomain.Memory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*memorydomain.Memory{}
	for _, m := range r.items {
		if !m.Pinned {
			out = append(out, m)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *fakeRepo) MarkAccessed(_ context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.items[name]
	if !ok {
		return memorydomain.ErrNotFound
	}
	now := time.Now().UTC()
	m.AccessedAt = &now
	m.AccessCount++
	return nil
}

func (r *fakeRepo) Delete(_ context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[name]; !ok {
		return memorydomain.ErrNotFound
	}
	delete(r.items, name)
	return nil
}

func newService(t *testing.T) *Service {
	t.Helper()
	return New(newFakeRepo(), nil, zaptest.NewLogger(t))
}

func TestUpsert_Insert(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	m, err := s.Upsert(ctx, UpsertInput{
		Name:        "user_role",
		Type:        memorydomain.TypeUser,
		Description: "Go engineer",
		Content:     "User is a Go engineer.",
		Source:      memorydomain.SourceUser,
	})
	if err != nil {
		t.Fatalf("Upsert insert: %v", err)
	}
	if !strings.HasPrefix(m.ID, "mem_") {
		t.Errorf("ID prefix wrong: %q", m.ID)
	}
	if m.Pinned {
		t.Errorf("Pinned default should be false")
	}
}

func TestUpsert_Update_PreservesSourceAndID(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	first, _ := s.Upsert(ctx, UpsertInput{
		Name: "x", Type: memorydomain.TypeUser, Description: "v1", Content: "c1",
		Source: memorydomain.SourceAI,
	})

	updated, err := s.Upsert(ctx, UpsertInput{
		Name: "x", Type: memorydomain.TypeFeedback, Description: "v2", Content: "c2",
		Source: memorydomain.SourceUser,
	})
	if err != nil {
		t.Fatalf("Upsert update: %v", err)
	}
	if updated.ID != first.ID {
		t.Errorf("ID changed on update: %q vs %q", updated.ID, first.ID)
	}
	if updated.Source != memorydomain.SourceAI {
		t.Errorf("Source should be preserved (AI), got %q", updated.Source)
	}
	if updated.Type != memorydomain.TypeFeedback || updated.Description != "v2" {
		t.Errorf("Type/Description not updated")
	}
}

func TestUpsert_ValidationErrors(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	cases := map[string]UpsertInput{
		"empty name":   {Name: "", Type: memorydomain.TypeUser, Description: "d", Content: "c", Source: memorydomain.SourceUser},
		"bad name":     {Name: "Bad-Name", Type: memorydomain.TypeUser, Description: "d", Content: "c", Source: memorydomain.SourceUser},
		"unknown type": {Name: "x", Type: "weird", Description: "d", Content: "c", Source: memorydomain.SourceUser},
		"empty source": {Name: "x", Type: memorydomain.TypeUser, Description: "d", Content: "c", Source: ""},
		"empty desc":   {Name: "x", Type: memorydomain.TypeUser, Description: "  ", Content: "c", Source: memorydomain.SourceUser},
	}
	for name, in := range cases {
		_, err := s.Upsert(ctx, in)
		if err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestUpsert_InvalidName_ReturnsErrInvalidName(t *testing.T) {
	s := newService(t)
	_, err := s.Upsert(context.Background(), UpsertInput{
		Name: "Bad Name!", Type: memorydomain.TypeUser, Description: "d", Content: "c",
		Source: memorydomain.SourceUser,
	})
	if !errors.Is(err, memorydomain.ErrInvalidName) {
		t.Errorf("got %v, want ErrInvalidName", err)
	}
}

func TestGet_BumpsAccessStats(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	_, _ = s.Upsert(ctx, UpsertInput{
		Name: "x", Type: memorydomain.TypeUser, Description: "d", Content: "c",
		Source: memorydomain.SourceUser,
	})

	got1, err := s.Get(ctx, "x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got1.AccessCount != 1 {
		t.Errorf("AccessCount after 1 Get: %d, want 1", got1.AccessCount)
	}

	_, _ = s.Get(ctx, "x")
	got3, _ := s.Get(ctx, "x")
	if got3.AccessCount != 3 {
		t.Errorf("AccessCount after 3 Gets: %d, want 3", got3.AccessCount)
	}
}

func TestPinUnpin(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	_, _ = s.Upsert(ctx, UpsertInput{
		Name: "x", Type: memorydomain.TypeUser, Description: "d", Content: "c",
		Source: memorydomain.SourceUser,
	})
	m, _ := s.Pin(ctx, "x")
	if !m.Pinned {
		t.Errorf("Pin should set pinned=true")
	}
	m, _ = s.Unpin(ctx, "x")
	if m.Pinned {
		t.Errorf("Unpin should set pinned=false")
	}
}

func TestListPinned_AfterPin(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	for _, n := range []string{"a", "b", "c"} {
		_, _ = s.Upsert(ctx, UpsertInput{
			Name: n, Type: memorydomain.TypeUser, Description: "d", Content: "c",
			Source: memorydomain.SourceUser,
		})
	}
	_, _ = s.Pin(ctx, "a")
	_, _ = s.Pin(ctx, "c")

	pinned, _ := s.ListPinned(ctx)
	if len(pinned) != 2 {
		t.Errorf("ListPinned len = %d, want 2", len(pinned))
	}
}

func TestListIndex_FormatAndExcludesPinned(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	_, _ = s.Upsert(ctx, UpsertInput{
		Name: "p", Type: memorydomain.TypeUser, Description: "pinned desc", Content: "c",
		Source: memorydomain.SourceUser,
	})
	_, _ = s.Pin(ctx, "p")
	_, _ = s.Upsert(ctx, UpsertInput{
		Name: "q", Type: memorydomain.TypeFeedback, Description: "qdesc", Content: "c",
		Source: memorydomain.SourceUser,
	})

	idx, err := s.ListIndex(ctx, 100)
	if err != nil {
		t.Fatalf("ListIndex: %v", err)
	}
	if strings.Contains(idx, "pinned desc") {
		t.Errorf("ListIndex should exclude pinned: %s", idx)
	}
	if !strings.Contains(idx, "qdesc") {
		t.Errorf("ListIndex should include non-pinned: %s", idx)
	}
	if !strings.Contains(idx, "[feedback] q") {
		t.Errorf("ListIndex format wrong: %s", idx)
	}
}

func TestListIndex_EmptyReturnsEmptyString(t *testing.T) {
	s := newService(t)
	idx, err := s.ListIndex(context.Background(), 100)
	if err != nil {
		t.Fatalf("ListIndex: %v", err)
	}
	if idx != "" {
		t.Errorf("ListIndex empty store: got %q, want empty", idx)
	}
}

func TestDelete_NotFound(t *testing.T) {
	s := newService(t)
	err := s.Delete(context.Background(), "nope")
	if !errors.Is(err, memorydomain.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestDelete_PublishesAndRemoves(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	_, _ = s.Upsert(ctx, UpsertInput{
		Name: "x", Type: memorydomain.TypeUser, Description: "d", Content: "c",
		Source: memorydomain.SourceUser,
	})
	if err := s.Delete(ctx, "x"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, "x"); !errors.Is(err, memorydomain.ErrNotFound) {
		t.Errorf("after Delete, Get got %v, want ErrNotFound", err)
	}
}

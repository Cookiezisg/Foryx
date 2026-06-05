package memory

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"

	memorydomain "github.com/sunweilin/forgify/backend/internal/domain/memory"
)

type fakeRepo struct {
	items map[string]*memorydomain.Memory
}

func newFakeRepo() *fakeRepo { return &fakeRepo{items: map[string]*memorydomain.Memory{}} }

func (f *fakeRepo) Save(_ context.Context, m *memorydomain.Memory) error {
	cp := *m
	f.items[m.Name] = &cp
	return nil
}
func (f *fakeRepo) Get(_ context.Context, name string) (*memorydomain.Memory, error) {
	m, ok := f.items[name]
	if !ok {
		return nil, memorydomain.ErrNotFound
	}
	cp := *m
	return &cp, nil
}
func (f *fakeRepo) List(_ context.Context, filter memorydomain.ListFilter) ([]*memorydomain.Memory, error) {
	var out []*memorydomain.Memory
	for _, m := range f.items {
		if filter.Pinned != nil && m.Pinned != *filter.Pinned {
			continue
		}
		cp := *m
		out = append(out, &cp)
	}
	return out, nil
}
func (f *fakeRepo) Delete(_ context.Context, name string) error {
	if _, ok := f.items[name]; !ok {
		return memorydomain.ErrNotFound
	}
	delete(f.items, name)
	return nil
}

type fakeEmitter struct{ events []string }

func (f *fakeEmitter) Emit(_ context.Context, eventType string, _ map[string]any) error {
	f.events = append(f.events, eventType)
	return nil
}

func TestUpsert_CreateThenUpdate_Notifies(t *testing.T) {
	repo := newFakeRepo()
	em := &fakeEmitter{}
	svc := NewService(repo, em, zap.NewNop())
	in := UpsertInput{Name: "foo", Description: "d", Content: "c", Source: "ai"}
	if _, err := svc.Upsert(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	in.Content = "c2"
	if _, err := svc.Upsert(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if len(em.events) != 2 || em.events[0] != "memory.created" || em.events[1] != "memory.updated" {
		t.Errorf("notify events = %v, want [memory.created, memory.updated]", em.events)
	}
}

func TestUpsert_Validates(t *testing.T) {
	svc := NewService(newFakeRepo(), nil, zap.NewNop())
	cases := []struct {
		name string
		in   UpsertInput
		want error
	}{
		{"bad name", UpsertInput{Name: "Bad Name", Description: "d", Content: "c", Source: "ai"}, memorydomain.ErrInvalidName},
		{"bad source", UpsertInput{Name: "ok", Description: "d", Content: "c", Source: "robot"}, memorydomain.ErrInvalidSource},
		{"no content", UpsertInput{Name: "ok", Description: "d", Content: "", Source: "ai"}, memorydomain.ErrInvalidInput},
	}
	for _, c := range cases {
		if _, err := svc.Upsert(context.Background(), c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}

func TestForSystemPrompt_TwoSections(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, nil, zap.NewNop())
	repo.items["rule"] = &memorydomain.Memory{Name: "rule", Description: "用户规则", Content: "全文规则", Pinned: true, Source: "user"}
	repo.items["note"] = &memorydomain.Memory{Name: "note", Description: "AI 笔记", Content: "笔记全文", Pinned: false, Source: "ai"}
	out := svc.ForSystemPrompt(context.Background())
	if !strings.Contains(out, "## Memory (pinned)") || !strings.Contains(out, "全文规则") {
		t.Errorf("missing pinned full text:\n%s", out)
	}
	if !strings.Contains(out, "## Memory index") || !strings.Contains(out, "- note: AI 笔记") {
		t.Errorf("missing index line:\n%s", out)
	}
	// non-pinned full content must NOT leak — only its description appears in the index.
	if strings.Contains(out, "笔记全文") {
		t.Errorf("non-pinned content leaked into prompt:\n%s", out)
	}
}

func TestForSystemPrompt_Empty(t *testing.T) {
	svc := NewService(newFakeRepo(), nil, zap.NewNop())
	if out := svc.ForSystemPrompt(context.Background()); out != "" {
		t.Errorf("empty memory should yield empty prompt, got:\n%q", out)
	}
}

func TestDelete_Notifies(t *testing.T) {
	repo := newFakeRepo()
	em := &fakeEmitter{}
	repo.items["x"] = &memorydomain.Memory{Name: "x"}
	svc := NewService(repo, em, zap.NewNop())
	if err := svc.Delete(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	if len(em.events) != 1 || em.events[0] != "memory.deleted" {
		t.Errorf("delete notify = %v, want [memory.deleted]", em.events)
	}
}

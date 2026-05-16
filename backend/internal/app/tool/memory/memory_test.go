package memory

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	memoryapp "github.com/sunweilin/forgify/backend/internal/app/memory"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	memorydomain "github.com/sunweilin/forgify/backend/internal/domain/memory"
)

// fakeRepo — same pattern as app/memory/memory_test.go but local copy
// (we cannot import a test file from another package).
type fakeRepo struct {
	mu    sync.Mutex
	items map[string]*memorydomain.Memory
}

func newFakeRepo() *fakeRepo { return &fakeRepo{items: map[string]*memorydomain.Memory{}} }

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
	if m, ok := r.items[name]; ok {
		return m, nil
	}
	return nil, memorydomain.ErrNotFound
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
func (r *fakeRepo) List(_ context.Context, _ memorydomain.ListFilter) ([]*memorydomain.Memory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*memorydomain.Memory{}
	for _, m := range r.items {
		out = append(out, m)
	}
	return out, nil
}
func (r *fakeRepo) ListPinned(_ context.Context) ([]*memorydomain.Memory, error) {
	return nil, nil
}
func (r *fakeRepo) ListForIndex(_ context.Context, _ int) ([]*memorydomain.Memory, error) {
	return nil, nil
}
func (r *fakeRepo) MarkAccessed(_ context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.items[name]
	if !ok {
		return memorydomain.ErrNotFound
	}
	now := time.Now()
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

func newService(t *testing.T) *memoryapp.Service {
	return memoryapp.New(newFakeRepo(), nil, zaptest.NewLogger(t))
}

func TestMemoryTools_Factory(t *testing.T) {
	svc := newService(t)
	tools := MemoryTools(svc)
	if len(tools) != 3 {
		t.Fatalf("MemoryTools count = %d, want 3", len(tools))
	}
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Name()] = true
	}
	for _, want := range []string{"read_memory", "write_memory", "forget_memory"} {
		if !names[want] {
			t.Errorf("missing tool %q", want)
		}
	}
}

func TestReadMemory_Identity(t *testing.T) {
	tl := &ReadMemory{}
	if tl.Name() != "read_memory" {
		t.Errorf("Name")
	}
	if tl.Description() == "" {
		t.Errorf("Description should be non-empty")
	}
	if !tl.IsReadOnly() {
		t.Errorf("read_memory should be read-only")
	}
	if tl.RequiresWorkspace() {
		t.Errorf("read_memory should not require workspace")
	}
}

func TestReadMemory_ValidateInput(t *testing.T) {
	tl := &ReadMemory{}
	if err := tl.ValidateInput(json.RawMessage(`{"name":"x"}`)); err != nil {
		t.Errorf("valid input rejected: %v", err)
	}
	if err := tl.ValidateInput(json.RawMessage(`{"name":""}`)); err == nil {
		t.Errorf("empty name should fail")
	}
	if err := tl.ValidateInput(json.RawMessage(`{`)); err == nil {
		t.Errorf("bad JSON should fail")
	}
}

func TestReadMemory_Execute_FoundAndMissing(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	_, _ = svc.Upsert(ctx, memoryapp.UpsertInput{
		Name: "x", Type: memorydomain.TypeUser, Description: "desc", Content: "body",
		Source: memorydomain.SourceUser,
	})

	tl := &ReadMemory{svc: svc}
	out, err := tl.Execute(ctx, `{"name":"x"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "body") || !strings.Contains(out, "type=user") {
		t.Errorf("Execute output missing markers: %s", out)
	}

	out, err = tl.Execute(ctx, `{"name":"nope"}`)
	if err != nil {
		t.Errorf("missing should return friendly string, not Go err: %v", err)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("missing output unexpected: %s", out)
	}
}

func TestWriteMemory_Identity(t *testing.T) {
	tl := &WriteMemory{}
	if tl.Name() != "write_memory" {
		t.Errorf("Name")
	}
	if tl.IsReadOnly() {
		t.Errorf("write_memory should not be read-only")
	}
}

func TestWriteMemory_ValidateInput(t *testing.T) {
	tl := &WriteMemory{}
	valid := `{"name":"a","type":"user","description":"d","content":"c"}`
	if err := tl.ValidateInput(json.RawMessage(valid)); err != nil {
		t.Errorf("valid: %v", err)
	}
	cases := map[string]string{
		"empty name":   `{"name":"","type":"user","description":"d","content":"c"}`,
		"bad type":     `{"name":"a","type":"weird","description":"d","content":"c"}`,
		"empty desc":   `{"name":"a","type":"user","description":"","content":"c"}`,
		"empty content": `{"name":"a","type":"user","description":"d","content":""}`,
	}
	for label, args := range cases {
		if err := tl.ValidateInput(json.RawMessage(args)); err == nil {
			t.Errorf("%s should fail validation", label)
		}
	}
}

func TestWriteMemory_Execute_PersistsAndReturnsConfirmation(t *testing.T) {
	svc := newService(t)
	tl := &WriteMemory{svc: svc}
	ctx := context.Background()
	out, err := tl.Execute(ctx, `{"name":"a","type":"user","description":"d","content":"c"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, `Saved memory "a"`) {
		t.Errorf("confirmation unexpected: %s", out)
	}
	m, err := svc.Get(ctx, "a")
	if err != nil {
		t.Fatalf("Get after write: %v", err)
	}
	if m.Source != memorydomain.SourceAI {
		t.Errorf("write_memory should mark source=ai, got %q", m.Source)
	}
}

func TestWriteMemory_Execute_InvalidNameFriendly(t *testing.T) {
	svc := newService(t)
	tl := &WriteMemory{svc: svc}
	out, err := tl.Execute(context.Background(), `{"name":"Bad Name","type":"user","description":"d","content":"c"}`)
	if err != nil {
		t.Errorf("invalid name should return friendly string, not Go err: %v", err)
	}
	if !strings.Contains(out, "invalid") {
		t.Errorf("friendly text missing: %s", out)
	}
}

func TestForgetMemory_Identity(t *testing.T) {
	tl := &ForgetMemory{}
	if tl.Name() != "forget_memory" {
		t.Errorf("Name")
	}
	if tl.IsReadOnly() {
		t.Errorf("forget_memory should not be read-only")
	}
}

func TestForgetMemory_Execute_FoundAndMissing(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	_, _ = svc.Upsert(ctx, memoryapp.UpsertInput{
		Name: "x", Type: memorydomain.TypeUser, Description: "d", Content: "c",
		Source: memorydomain.SourceUser,
	})

	tl := &ForgetMemory{svc: svc}
	out, err := tl.Execute(ctx, `{"name":"x"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Forgotten") {
		t.Errorf("confirmation unexpected: %s", out)
	}

	out, err = tl.Execute(ctx, `{"name":"x"}`)
	if err != nil {
		t.Errorf("idempotent forget should return friendly: %v", err)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("expected 'not found' marker: %s", out)
	}
}

func TestAllTools_CheckPermissions_DefaultAllow(t *testing.T) {
	svc := newService(t)
	for _, tl := range MemoryTools(svc) {
		res := tl.CheckPermissions(json.RawMessage(`{}`), toolapp.PermissionModeDefault)
		if res != toolapp.PermissionAllow {
			t.Errorf("tool %s: expected PermissionAllow, got %v", tl.Name(), res)
		}
	}
}

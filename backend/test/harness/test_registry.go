//go:build pipeline

package harness

import (
	"context"
	"fmt"

	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
)

type testRegistrySource struct {
	byName map[string]mcpdomain.RegistryEntry
	all    []mcpdomain.RegistryEntry
}

func newTestRegistrySource(entries ...mcpdomain.RegistryEntry) *testRegistrySource {
	t := &testRegistrySource{
		byName: make(map[string]mcpdomain.RegistryEntry, len(entries)),
		all:    make([]mcpdomain.RegistryEntry, 0, len(entries)),
	}
	for _, e := range entries {
		t.byName[e.Name] = e
		t.all = append(t.all, e)
	}
	return t
}

func (t *testRegistrySource) List(_ context.Context) ([]mcpdomain.RegistryEntry, error) {
	out := make([]mcpdomain.RegistryEntry, len(t.all))
	copy(out, t.all)
	return out, nil
}

func (t *testRegistrySource) Get(_ context.Context, name string) (*mcpdomain.RegistryEntry, error) {
	e, ok := t.byName[name]
	if !ok {
		return nil, fmt.Errorf("test registry: %w: %q", mcpdomain.ErrRegistryEntryNotFound, name)
	}
	cp := e
	return &cp, nil
}

var _ mcpdomain.RegistrySource = (*testRegistrySource)(nil)

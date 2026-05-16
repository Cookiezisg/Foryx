package sandbox

import (
	"context"
	"fmt"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

type fakeToolRegistry struct {
	paths map[string]string
}

func newFakeToolRegistry(paths map[string]string) *fakeToolRegistry {
	return &fakeToolRegistry{paths: paths}
}

func (f *fakeToolRegistry) EnsureTool(ctx context.Context, kind, version string) (string, error) {
	p, ok := f.paths[kind]
	if !ok {
		return "", fmt.Errorf("fakeToolRegistry: kind %q not seeded: %w", kind, sandboxdomain.ErrRuntimeNotSupported)
	}
	return p, nil
}

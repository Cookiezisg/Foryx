//go:build pipeline

// test_registry.go — in-memory RegistrySource used by the pipeline test
// harness. Production main.go wires CuratedRegistrySource (21
// hand-picked entries); tests need controllable fixtures for install
// path coverage (e.g. the `everything` reference server, an entry with
// a forced required arg). Lives here (not in infra/mcp) so production
// code never depends on it.
//
// test_registry.go ——pipeline 测试 harness 用的内存 RegistrySource。生产
// 走 CuratedRegistrySource；测试要可控 fixtures 覆盖 install 路径（如
// `everything` 参考 server / 强制必填 arg 的样本）。放在 harness 包内，
// 生产代码永不依赖。
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

func (t *testRegistrySource) Search(_ context.Context, query string) ([]mcpdomain.RegistryEntry, error) {
	if query == "" {
		return nil, mcpdomain.ErrQueryRequired
	}
	out := make([]mcpdomain.RegistryEntry, 0, len(t.all))
	for _, e := range t.all {
		if contains(e.Name, query) || contains(e.Description, query) {
			out = append(out, e)
		}
	}
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

func contains(haystack, needle string) bool {
	if haystack == "" || needle == "" {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			a, b := haystack[i+j], needle[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

var _ mcpdomain.RegistrySource = (*testRegistrySource)(nil)

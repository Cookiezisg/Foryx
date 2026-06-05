package memory

import (
	"context"
	"errors"
	"testing"

	memorydomain "github.com/sunweilin/forgify/backend/internal/domain/memory"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

func wsCtx(t *testing.T) (context.Context, *Store) {
	t.Helper()
	s := New(t.TempDir())
	ctx := reqctxpkg.SetWorkspaceID(context.Background(), "ws_test")
	return ctx, s
}

func TestSaveGetRoundtrip(t *testing.T) {
	ctx, s := wsCtx(t)
	m := &memorydomain.Memory{Name: "no-python38", Description: "语言偏好", Content: "用户不喜欢 Python 3.8", Pinned: true, Source: "user"}
	if err := s.Save(ctx, m); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, "no-python38")
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "语言偏好" || got.Content != "用户不喜欢 Python 3.8" || !got.Pinned || got.Source != "user" {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be the file mtime, not zero")
	}
}

func TestGet_NotFound(t *testing.T) {
	ctx, s := wsCtx(t)
	if _, err := s.Get(ctx, "missing"); !errors.Is(err, memorydomain.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestList_EmptyAndPinnedFilter(t *testing.T) {
	ctx, s := wsCtx(t)
	// brand-new workspace: no dir → empty, not an error.
	if items, err := s.List(ctx, memorydomain.ListFilter{}); err != nil || len(items) != 0 {
		t.Fatalf("empty list: err=%v len=%d", err, len(items))
	}
	_ = s.Save(ctx, &memorydomain.Memory{Name: "a", Description: "d", Content: "c", Pinned: true, Source: "ai"})
	_ = s.Save(ctx, &memorydomain.Memory{Name: "b", Description: "d", Content: "c", Pinned: false, Source: "ai"})
	all, _ := s.List(ctx, memorydomain.ListFilter{})
	if len(all) != 2 {
		t.Fatalf("want 2, got %d", len(all))
	}
	yes := true
	pinned, _ := s.List(ctx, memorydomain.ListFilter{Pinned: &yes})
	if len(pinned) != 1 || pinned[0].Name != "a" {
		t.Errorf("pinned filter wrong: %+v", pinned)
	}
}

func TestSave_InvalidNameRejectedNoTraversal(t *testing.T) {
	ctx, s := wsCtx(t)
	if err := s.Save(ctx, &memorydomain.Memory{Name: "../escape"}); !errors.Is(err, memorydomain.ErrInvalidName) {
		t.Errorf("want ErrInvalidName for traversal name, got %v", err)
	}
}

func TestDelete(t *testing.T) {
	ctx, s := wsCtx(t)
	_ = s.Save(ctx, &memorydomain.Memory{Name: "x", Description: "d", Content: "c", Source: "ai"})
	if err := s.Delete(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "x"); !errors.Is(err, memorydomain.ErrNotFound) {
		t.Errorf("after delete want NotFound, got %v", err)
	}
	if err := s.Delete(ctx, "x"); !errors.Is(err, memorydomain.ErrNotFound) {
		t.Errorf("delete missing want NotFound, got %v", err)
	}
}

func TestWorkspaceIsolation(t *testing.T) {
	s := New(t.TempDir())
	ctxA := reqctxpkg.SetWorkspaceID(context.Background(), "ws_a")
	ctxB := reqctxpkg.SetWorkspaceID(context.Background(), "ws_b")
	_ = s.Save(ctxA, &memorydomain.Memory{Name: "secret", Description: "d", Content: "A's", Source: "ai"})
	// workspace B must not see A's memory (separate directory).
	if _, err := s.Get(ctxB, "secret"); !errors.Is(err, memorydomain.ErrNotFound) {
		t.Errorf("workspace B saw A's memory: %v", err)
	}
}

func TestFrontmatterRoundtrip(t *testing.T) {
	m := &memorydomain.Memory{Name: "x", Description: "desc", Content: "line1\nline2", Pinned: true, Source: "user"}
	parsed := parseFile(renderFile(m), "x")
	if parsed.Description != "desc" || parsed.Content != "line1\nline2" || !parsed.Pinned || parsed.Source != "user" {
		t.Errorf("frontmatter roundtrip mismatch: %+v", parsed)
	}
}

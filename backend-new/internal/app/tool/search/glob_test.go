package search

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
)

func newGlob() *Glob { return &Glob{pathGuard: pathguardpkg.New(nil)} }

func TestGlob_ValidateInput(t *testing.T) {
	g := newGlob()
	if err := g.ValidateInput([]byte(`{"pattern":"","path":"/x"}`)); !errors.Is(err, ErrEmptyPattern) {
		t.Fatalf("empty pattern: want ErrEmptyPattern, got %v", err)
	}
	if err := g.ValidateInput([]byte(`{"pattern":"*.go","path":""}`)); !errors.Is(err, ErrPathRequired) {
		t.Fatalf("empty path: want ErrPathRequired, got %v", err)
	}
	if err := g.ValidateInput([]byte(`{"pattern":"*.go","path":"/x","limit":-1}`)); err == nil {
		t.Fatalf("negative limit: want error, got nil")
	}
	if err := g.ValidateInput([]byte(`{"pattern":"*.go","path":"/x"}`)); err != nil {
		t.Fatalf("happy: want nil, got %v", err)
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func parseGlob(t *testing.T, out string) globResult {
	t.Helper()
	var r globResult
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	return r
}

func TestGlob_Execute_Recursive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.go"))
	writeFile(t, filepath.Join(dir, "b.go"))
	writeFile(t, filepath.Join(dir, "sub", "c.go"))
	writeFile(t, filepath.Join(dir, "sub", "d.txt"))

	out, err := newGlob().Execute(context.Background(), `{"pattern":"**/*.go","path":"`+dir+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseGlob(t, out)
	if r.Total != 3 {
		t.Fatalf("want 3 .go matches, got %d:\n%s", r.Total, out)
	}
	if r.Root != filepath.Clean(dir) {
		t.Fatalf("root = %q, want %q", r.Root, dir)
	}
}

func TestGlob_Execute_NonRecursive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.go"))
	writeFile(t, filepath.Join(dir, "sub", "c.go"))

	out, err := newGlob().Execute(context.Background(), `{"pattern":"*.go","path":"`+dir+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseGlob(t, out)
	if r.Total != 1 {
		t.Fatalf("want 1 (non-recursive), got %d:\n%s", r.Total, out)
	}
}

func TestGlob_Execute_MtimeDescending(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "older.go")
	newer := filepath.Join(dir, "newer.go")
	writeFile(t, older)
	writeFile(t, newer)
	base := time.Now()
	if err := os.Chtimes(older, base, base.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, base, base); err != nil {
		t.Fatal(err)
	}
	out, err := newGlob().Execute(context.Background(), `{"pattern":"*.go","path":"`+dir+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseGlob(t, out)
	if len(r.Matches) != 2 || !strings.HasSuffix(r.Matches[0].Path, "newer.go") {
		t.Fatalf("newest must come first:\n%s", out)
	}
}

func TestGlob_Execute_LimitTruncates(t *testing.T) {
	dir := t.TempDir()
	for i := range 5 {
		writeFile(t, filepath.Join(dir, string(rune('a'+i))+".go"))
	}
	out, err := newGlob().Execute(context.Background(), `{"pattern":"*.go","path":"`+dir+`","limit":2}`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseGlob(t, out)
	if r.Total != 5 || len(r.Matches) != 2 || !r.Truncated {
		t.Fatalf("want total=5 matches=2 truncated=true, got total=%d matches=%d truncated=%v", r.Total, len(r.Matches), r.Truncated)
	}
}

func TestGlob_Execute_RootNotFound(t *testing.T) {
	out, err := newGlob().Execute(context.Background(), `{"pattern":"*.go","path":"/no/such/root"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Search root not found") {
		t.Fatalf("got %q", out)
	}
}

func TestGlob_Execute_RootNotDir(t *testing.T) {
	f := filepath.Join(t.TempDir(), "a.txt")
	writeFile(t, f)
	out, err := newGlob().Execute(context.Background(), `{"pattern":"*.go","path":"`+f+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "must be a directory") {
		t.Fatalf("got %q", out)
	}
}

func TestGlob_Execute_PathGuardDeny(t *testing.T) {
	g := &Glob{pathGuard: pathguardpkg.New([]string{"/etc/"})}
	out, err := g.Execute(context.Background(), `{"pattern":"*","path":"/etc"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "denied by safety guard") {
		t.Fatalf("got %q", out)
	}
}

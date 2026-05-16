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

func newTestGlob() *Glob {
	return &Glob{pathGuard: pathguardpkg.NewDefault()}
}

// seedGlobTree builds a small fixture: 3 .go files (different mtimes for
// sort verification), 1 .md file, 1 nested dir, 1 noise dir. Returns the
// absolute root.
//
// seedGlobTree 种一棵 fixture：3 个 .go（不同 mtime 验证排序）+ 1 .md +
// 1 嵌套目录 + 1 noise 目录。返绝对 root。
func seedGlobTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mk := func(rel, body string) string {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
		return full
	}
	old := mk("old.go", "// old\n")
	mid := mk("mid.go", "// mid\n")
	newest := mk("new.go", "// newest\n")
	mk("notes.md", "doc\n")
	mk("sub/inner.go", "package sub\n")

	now := time.Now()
	if err := os.Chtimes(old, now.Add(-3*time.Hour), now.Add(-3*time.Hour)); err != nil {
		t.Fatalf("chtimes old: %v", err)
	}
	if err := os.Chtimes(mid, now.Add(-1*time.Hour), now.Add(-1*time.Hour)); err != nil {
		t.Fatalf("chtimes mid: %v", err)
	}
	if err := os.Chtimes(newest, now, now); err != nil {
		t.Fatalf("chtimes new: %v", err)
	}
	return root
}

func runGlob(t *testing.T, g *Glob, args globArgs) globResult {
	t.Helper()
	body, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, err := g.Execute(context.Background(), string(body))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var res globResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("unmarshal result (raw=%q): %v", out, err)
	}
	return res
}


func TestGlob_ValidateInput_RequiresPattern(t *testing.T) {
	g := newTestGlob()
	if err := g.ValidateInput(json.RawMessage(`{}`)); !errors.Is(err, ErrEmptyPattern) {
		t.Fatalf("want ErrEmptyPattern, got %v", err)
	}
}

func TestGlob_ValidateInput_RejectsRelativePath(t *testing.T) {
	g := newTestGlob()
	err := g.ValidateInput(json.RawMessage(`{"pattern":"*","path":"rel/path"}`))
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("want absolute-path error, got %v", err)
	}
}

func TestGlob_ValidateInput_RejectsNegativeLimit(t *testing.T) {
	g := newTestGlob()
	err := g.ValidateInput(json.RawMessage(`{"pattern":"*","limit":-1}`))
	if err == nil {
		t.Fatal("expected error for negative limit")
	}
}

func TestGlob_ValidateInput_AcceptsValidArgs(t *testing.T) {
	g := newTestGlob()
	dir := t.TempDir()
	body, _ := json.Marshal(globArgs{Pattern: "*.go", Path: dir, Limit: 50})
	if err := g.ValidateInput(body); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}


func TestGlobArgs_NormalizeFillsDefaults(t *testing.T) {
	a := globArgs{}
	a.normalize()
	if a.Limit != defaultGlobLimit {
		t.Errorf("Limit default = %d, want %d", a.Limit, defaultGlobLimit)
	}
	if a.Path == "" {
		t.Error("Path should default to cwd")
	}
}

func TestGlobArgs_NormalizeCapsHardLimit(t *testing.T) {
	a := globArgs{Limit: 100_000}
	a.normalize()
	if a.Limit != maxGlobLimit {
		t.Errorf("Limit hard cap = %d, want %d", a.Limit, maxGlobLimit)
	}
}


func TestGlob_StarMatchesImmediateChildren(t *testing.T) {
	g := newTestGlob()
	root := seedGlobTree(t)
	res := runGlob(t, g, globArgs{Pattern: "*.go", Path: root})
	// Top-level: 3 .go files; sub/inner.go must NOT match a non-recursive `*`.
	// 顶层 3 个 .go；sub/inner.go 不应被非递归 `*` 命中。
	if res.Total != 3 {
		t.Errorf("total = %d, want 3 (got matches: %+v)", res.Total, res.Matches)
	}
	for _, m := range res.Matches {
		if strings.Contains(m.Path, "sub/") || strings.Contains(m.Path, "sub\\") {
			t.Errorf("non-recursive *.go should not include nested file: %s", m.Path)
		}
	}
}

func TestGlob_DoubleStarMatchesRecursive(t *testing.T) {
	g := newTestGlob()
	root := seedGlobTree(t)
	res := runGlob(t, g, globArgs{Pattern: "**/*.go", Path: root})
	// 4 total: 3 top + 1 nested.
	if res.Total != 4 {
		t.Errorf("total = %d, want 4 (got %+v)", res.Total, res.Matches)
	}
	found := false
	for _, m := range res.Matches {
		if strings.HasSuffix(m.Path, "inner.go") {
			found = true
		}
	}
	if !found {
		t.Errorf("**/*.go should include sub/inner.go; got %+v", res.Matches)
	}
}

func TestGlob_StarLikeLS_ListsAllChildren(t *testing.T) {
	// Pattern "*" with a directory path is the LS replacement: lists every
	// immediate child (files + dirs) so the LLM never needs a separate LS tool.
	// pattern "*" + 目录 = LS 替代：列出所有直系子项。
	g := newTestGlob()
	root := seedGlobTree(t)
	res := runGlob(t, g, globArgs{Pattern: "*", Path: root})
	wantNames := map[string]bool{
		"old.go": true, "mid.go": true, "new.go": true, "notes.md": true, "sub": true,
	}
	if res.Total != len(wantNames) {
		t.Errorf("total = %d, want %d (matches: %+v)", res.Total, len(wantNames), res.Matches)
	}
	for _, m := range res.Matches {
		base := filepath.Base(m.Path)
		if !wantNames[base] {
			t.Errorf("unexpected match: %s", m.Path)
		}
		delete(wantNames, base)
	}
	if len(wantNames) != 0 {
		t.Errorf("missing names: %v", wantNames)
	}
}


func TestGlob_TypeField_DistinguishesFileAndDir(t *testing.T) {
	g := newTestGlob()
	root := seedGlobTree(t)
	res := runGlob(t, g, globArgs{Pattern: "*", Path: root})
	gotTypes := map[string]string{}
	for _, m := range res.Matches {
		gotTypes[filepath.Base(m.Path)] = m.Type
	}
	if gotTypes["sub"] != "dir" {
		t.Errorf("sub should be dir, got %q", gotTypes["sub"])
	}
	if gotTypes["new.go"] != "file" {
		t.Errorf("new.go should be file, got %q", gotTypes["new.go"])
	}
}

func TestGlob_TypeField_ReportsSymlink(t *testing.T) {
	g := newTestGlob()
	root := t.TempDir()
	target := filepath.Join(root, "real.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported on this fs: %v", err)
	}
	res := runGlob(t, g, globArgs{Pattern: "link.txt", Path: root})
	if len(res.Matches) != 1 || res.Matches[0].Type != "symlink" {
		t.Errorf("symlink not classified: %+v", res.Matches)
	}
}

func TestGlob_SizeAndMTime_Populated(t *testing.T) {
	g := newTestGlob()
	root := seedGlobTree(t)
	res := runGlob(t, g, globArgs{Pattern: "new.go", Path: root})
	if len(res.Matches) != 1 {
		t.Fatalf("want 1 match, got %d", len(res.Matches))
	}
	m := res.Matches[0]
	if m.Size <= 0 {
		t.Errorf("size not populated: %d", m.Size)
	}
	if m.MTime.IsZero() {
		t.Errorf("mtime not populated")
	}
}


func TestGlob_SortedByMTimeDescending(t *testing.T) {
	g := newTestGlob()
	root := seedGlobTree(t)
	res := runGlob(t, g, globArgs{Pattern: "*.go", Path: root})
	if len(res.Matches) < 2 {
		t.Fatalf("need >=2 matches to verify sort, got %d", len(res.Matches))
	}
	for i := 1; i < len(res.Matches); i++ {
		if res.Matches[i].MTime.After(res.Matches[i-1].MTime) {
			t.Errorf("not descending by mtime at index %d: %v then %v",
				i, res.Matches[i-1].MTime, res.Matches[i].MTime)
		}
	}
	// Sanity: newest file should be first.
	// Sanity 第一个应是最新 new.go。
	if filepath.Base(res.Matches[0].Path) != "new.go" {
		t.Errorf("first match should be new.go, got %s", filepath.Base(res.Matches[0].Path))
	}
}


func TestGlob_LimitTruncatesAndFlagsTrue(t *testing.T) {
	g := newTestGlob()
	root := seedGlobTree(t)
	res := runGlob(t, g, globArgs{Pattern: "*.go", Path: root, Limit: 2})
	if !res.Truncated {
		t.Error("truncated should be true when limit < total")
	}
	if len(res.Matches) != 2 {
		t.Errorf("matches len = %d, want 2", len(res.Matches))
	}
	if res.Total != 3 {
		t.Errorf("total should reflect pre-truncation count = 3, got %d", res.Total)
	}
}

func TestGlob_NoTruncationWhenUnderLimit(t *testing.T) {
	g := newTestGlob()
	root := seedGlobTree(t)
	res := runGlob(t, g, globArgs{Pattern: "*.go", Path: root, Limit: 100})
	if res.Truncated {
		t.Errorf("truncated should be false when total < limit; total=%d", res.Total)
	}
}


func TestGlob_PathGuard_DeniesSensitivePath(t *testing.T) {
	g := newTestGlob()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir; PathGuard test needs ~ expansion")
	}
	denied := filepath.Join(home, ".ssh")
	body, _ := json.Marshal(globArgs{Pattern: "*", Path: denied})
	out, err := g.Execute(context.Background(), string(body))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "denied") {
		t.Errorf("expected PathGuard denial, got: %q", out)
	}
}

func TestGlob_NonexistentRoot_ReportsClearly(t *testing.T) {
	g := newTestGlob()
	missing := filepath.Join(t.TempDir(), "nope")
	body, _ := json.Marshal(globArgs{Pattern: "*", Path: missing})
	out, err := g.Execute(context.Background(), string(body))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("expected not-found message, got: %q", out)
	}
}

func TestGlob_RootIsFile_ReportsClearly(t *testing.T) {
	g := newTestGlob()
	dir := t.TempDir()
	target := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	body, _ := json.Marshal(globArgs{Pattern: "*", Path: target})
	out, err := g.Execute(context.Background(), string(body))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "must be a directory") {
		t.Errorf("expected directory-required message, got: %q", out)
	}
}

func TestGlob_NoMatches_EmptyMatchesAndTotalZero(t *testing.T) {
	g := newTestGlob()
	root := t.TempDir()
	res := runGlob(t, g, globArgs{Pattern: "*.zzznosuch", Path: root})
	if res.Total != 0 {
		t.Errorf("total = %d, want 0", res.Total)
	}
	if len(res.Matches) != 0 {
		t.Errorf("matches len = %d, want 0", len(res.Matches))
	}
	if res.Truncated {
		t.Error("truncated should be false with zero matches")
	}
}


func TestGlob_IdentityMethods(t *testing.T) {
	g := newTestGlob()
	if g.Name() != "Glob" {
		t.Errorf("Name = %q, want Glob", g.Name())
	}
	if g.Description() == "" {
		t.Error("Description should not be empty")
	}
	if len(g.Parameters()) == 0 {
		t.Error("Parameters should not be empty")
	}
}

func TestGlob_StaticMetadata(t *testing.T) {
	g := newTestGlob()
	if !g.IsReadOnly() {
		t.Error("Glob should be read-only")
	}
	if g.NeedsReadFirst() {
		t.Error("Glob should not require Read first")
	}
	if !g.RequiresWorkspace() {
		t.Error("Glob should require workspace")
	}
}

func TestGlob_Schema_IsParsableObject(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(globSchema, &doc); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	if doc["type"] != "object" {
		t.Errorf("schema type = %v, want object", doc["type"])
	}
	props, ok := doc["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties not an object")
	}
	for _, want := range []string{"pattern", "path", "limit"} {
		if _, ok := props[want]; !ok {
			t.Errorf("schema missing property %q", want)
		}
	}
}

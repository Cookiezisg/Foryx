package search

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.uber.org/zap"

	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
)

// newStdlibGrep forces the pure-Go backend (rgPath empty) so tests are
// deterministic regardless of whether ripgrep is installed.
//
// newStdlibGrep 强制纯 Go 后端（rgPath 空），使测试不依赖系统是否装 ripgrep、确定性。
func newStdlibGrep() *Grep {
	return &Grep{pathGuard: pathguardpkg.New(nil), rgPath: "", log: zap.NewNop()}
}

func grepFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestGrep_ValidateInput(t *testing.T) {
	g := newStdlibGrep()
	if err := g.ValidateInput([]byte(`{"pattern":"","path":"/x"}`)); !errors.Is(err, ErrEmptyPattern) {
		t.Fatalf("empty pattern: %v", err)
	}
	if err := g.ValidateInput([]byte(`{"pattern":"x","path":""}`)); !errors.Is(err, ErrPathRequired) {
		t.Fatalf("empty path: %v", err)
	}
	if err := g.ValidateInput([]byte(`{"pattern":"x","path":"/x","output_mode":"bogus"}`)); !errors.Is(err, ErrInvalidOutputMode) {
		t.Fatalf("bad mode: %v", err)
	}
	if err := g.ValidateInput([]byte(`{"pattern":"x","path":"/x","-A":-1}`)); err == nil {
		t.Fatalf("negative -A: want error")
	}
	if err := g.ValidateInput([]byte(`{"pattern":"x","path":"/x"}`)); err != nil {
		t.Fatalf("happy: %v", err)
	}
}

func TestGrep_Stdlib_FilesWithMatches(t *testing.T) {
	dir := t.TempDir()
	hit := grepFile(t, dir, "hit.txt", "alpha\nbeta\n")
	grepFile(t, dir, "miss.txt", "gamma\n")
	out, err := newStdlibGrep().Execute(context.Background(), `{"pattern":"beta","path":"`+dir+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, hit) || strings.Contains(out, "miss.txt") {
		t.Fatalf("want only hit.txt, got:\n%s", out)
	}
}

func TestGrep_Stdlib_Content_WithLineNumbers(t *testing.T) {
	dir := t.TempDir()
	f := grepFile(t, dir, "a.txt", "line1\nfoo here\nline3\n")
	out, err := newStdlibGrep().Execute(context.Background(), `{"pattern":"foo","path":"`+f+`","output_mode":"content","-n":true}`)
	if err != nil {
		t.Fatal(err)
	}
	// single-file search: no path prefix; -n shows line number → "2:foo here"
	if !strings.Contains(out, "2:foo here") {
		t.Fatalf("want '2:foo here', got:\n%s", out)
	}
}

func TestGrep_Stdlib_Content_Context(t *testing.T) {
	dir := t.TempDir()
	f := grepFile(t, dir, "a.txt", "a\nb\nMATCH\nd\ne\n")
	out, err := newStdlibGrep().Execute(context.Background(), `{"pattern":"MATCH","path":"`+f+`","output_mode":"content","-B":1,"-A":1,"-n":true}`)
	if err != nil {
		t.Fatal(err)
	}
	// context lines use '-' separator, match uses ':'
	if !strings.Contains(out, "2-b") || !strings.Contains(out, "3:MATCH") || !strings.Contains(out, "4-d") {
		t.Fatalf("want context b/MATCH/d, got:\n%s", out)
	}
}

func TestGrep_Stdlib_Count(t *testing.T) {
	dir := t.TempDir()
	f := grepFile(t, dir, "a.txt", "x\nx\ny\nx\n")
	out, err := newStdlibGrep().Execute(context.Background(), `{"pattern":"x","path":"`+f+`","output_mode":"count"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, f+":3") {
		t.Fatalf("want count 3, got:\n%s", out)
	}
}

func TestGrep_Stdlib_TypeFilter(t *testing.T) {
	dir := t.TempDir()
	grepFile(t, dir, "a.go", "needle\n")
	grepFile(t, dir, "b.txt", "needle\n")
	out, err := newStdlibGrep().Execute(context.Background(), `{"pattern":"needle","path":"`+dir+`","type":"go"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "a.go") || strings.Contains(out, "b.txt") {
		t.Fatalf("type filter should match only .go, got:\n%s", out)
	}
}

func TestGrep_Stdlib_IgnoreCase(t *testing.T) {
	dir := t.TempDir()
	f := grepFile(t, dir, "a.txt", "Hello World\n")
	out, err := newStdlibGrep().Execute(context.Background(), `{"pattern":"hello","path":"`+f+`","output_mode":"content","-i":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Hello World") {
		t.Fatalf("case-insensitive should match, got:\n%s", out)
	}
}

func TestGrep_Stdlib_Multiline(t *testing.T) {
	dir := t.TempDir()
	f := grepFile(t, dir, "a.txt", "start\nmiddle\nend\n")
	out, err := newStdlibGrep().Execute(context.Background(), `{"pattern":"start.*end","path":"`+f+`","multiline":true,"output_mode":"count"}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "No matches") {
		t.Fatalf("multiline should match across lines, got:\n%s", out)
	}
}

func TestGrep_Stdlib_NoMatch(t *testing.T) {
	dir := t.TempDir()
	grepFile(t, dir, "a.txt", "nothing here\n")
	out, err := newStdlibGrep().Execute(context.Background(), `{"pattern":"absent","path":"`+dir+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No matches") {
		t.Fatalf("want No matches, got:\n%s", out)
	}
}

func TestGrep_Stdlib_HeadLimit(t *testing.T) {
	dir := t.TempDir()
	for i := range 5 {
		grepFile(t, dir, string(rune('a'+i))+".txt", "match\n")
	}
	out, err := newStdlibGrep().Execute(context.Background(), `{"pattern":"match","path":"`+dir+`","head_limit":2}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "truncated at 2 files") {
		t.Fatalf("want head_limit truncation, got:\n%s", out)
	}
}

func TestGrep_Stdlib_SkipsNoiseDirs(t *testing.T) {
	dir := t.TempDir()
	grepFile(t, dir, "real.txt", "secret\n")
	grepFile(t, dir, ".git/config", "secret\n")
	grepFile(t, dir, "node_modules/pkg/index.js", "secret\n")
	out, err := newStdlibGrep().Execute(context.Background(), `{"pattern":"secret","path":"`+dir+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "real.txt") {
		t.Fatalf("should find real.txt:\n%s", out)
	}
	if strings.Contains(out, ".git") || strings.Contains(out, "node_modules") {
		t.Fatalf("must skip noise dirs:\n%s", out)
	}
}

func TestGrep_Stdlib_PathGuardDeny(t *testing.T) {
	g := &Grep{pathGuard: pathguardpkg.New([]string{"/etc/"}), rgPath: "", log: zap.NewNop()}
	out, err := g.Execute(context.Background(), `{"pattern":"x","path":"/etc"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "denied by safety guard") {
		t.Fatalf("got %q", out)
	}
}

func TestBuildRgArgs(t *testing.T) {
	args := grepArgs{
		Pattern:    "foo",
		Path:       "/root",
		OutputMode: OutputModeContent,
		ShowLines:  true,
		Before:     2,
		After:      3,
		IgnoreCase: true,
		Glob:       "*.go",
		Type:       "go",
	}
	got := buildRgArgs(args)
	for _, want := range []string{"--no-heading", "-n", "-B", "2", "-A", "3", "-i", "--glob", "*.go", "--type", "go", "-e", "foo", "/root"} {
		if !slices.Contains(got, want) {
			t.Fatalf("buildRgArgs missing %q in %v", want, got)
		}
	}
}

// TestGrep_Rg_Integration runs the real ripgrep backend if rg is installed,
// confirming the two backends agree on a basic search.
//
// TestGrep_Rg_Integration 装了 rg 时跑真实 ripgrep 后端，确认两后端在基础搜索上一致。
func TestGrep_Rg_Integration(t *testing.T) {
	rg, err := exec.LookPath("rg")
	if err != nil {
		t.Skip("rg not installed")
	}
	dir := t.TempDir()
	hit := grepFile(t, dir, "hit.txt", "alpha\nfindme\n")
	grepFile(t, dir, "miss.txt", "nope\n")
	g := &Grep{pathGuard: pathguardpkg.New(nil), rgPath: rg, log: zap.NewNop()}
	out, err := g.Execute(context.Background(), `{"pattern":"findme","path":"`+dir+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, hit) {
		t.Fatalf("rg backend should find hit.txt, got:\n%s", out)
	}
}

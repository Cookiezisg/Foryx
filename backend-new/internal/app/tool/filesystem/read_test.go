package filesystem

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentstatepkg "github.com/sunweilin/forgify/backend/internal/pkg/agentstate"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// newReadFixture builds a permissive Read tool + ctx with AgentState seeded.
// Tests that need a stricter PathGuard build their own.
//
// newReadFixture 建一个宽松守卫的 Read + 已埋 AgentState 的 ctx。需更严守卫的测试自建。
func newReadFixture(t *testing.T) (*Read, context.Context, *agentstatepkg.AgentState) {
	t.Helper()
	state := agentstatepkg.New()
	ctx := reqctxpkg.WithAgentState(context.Background(), state)
	return &Read{pathGuard: pathguardpkg.New(nil)}, ctx, state
}

func TestRead_ValidateInput(t *testing.T) {
	r := &Read{pathGuard: pathguardpkg.New(nil)}
	cases := []struct {
		name string
		json string
		want error
	}{
		{"empty path", `{"file_path":""}`, ErrEmptyFilePath},
		{"negative offset", `{"file_path":"/x","offset":-1}`, ErrNegativeOffset},
		{"negative limit", `{"file_path":"/x","limit":-1}`, ErrNegativeLimit},
		{"happy", `{"file_path":"/x"}`, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.ValidateInput([]byte(c.json))
			if !errors.Is(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestRead_Execute_TildeExpanded(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("home dir unknown")
	}
	r := &Read{pathGuard: pathguardpkg.New(nil)}
	// Nonexistent file under ~ — proves ~ expanded to home (the not-found message
	// references the home-based absolute path) without creating anything.
	out, err := r.Execute(context.Background(), `{"file_path":"~/__forgify_nonexistent_xyz__.txt"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, home) {
		t.Fatalf("~ not expanded to home: %q", out)
	}
}

func TestRead_Execute_RelativeRejected(t *testing.T) {
	r := &Read{pathGuard: pathguardpkg.New(nil)}
	out, err := r.Execute(context.Background(), `{"file_path":"rel.go"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "must be absolute") {
		t.Fatalf("relative path should be rejected via fspath.Expand: %q", out)
	}
}

func TestRead_Execute_HappyPath_CatN(t *testing.T) {
	r, ctx, state := newReadFixture(t)
	path := filepath.Join(t.TempDir(), "a.txt")
	body := "alpha\nbeta\ngamma\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := r.Execute(ctx, `{"file_path":"`+path+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	// cat -n format: "%5d\t<line>\n"
	if !strings.Contains(out, "    1\talpha\n") || !strings.Contains(out, "    3\tgamma\n") {
		t.Fatalf("output missing cat -n format:\n%s", out)
	}
	// MarkRead must stamp the size.
	size, ok := state.WasRead(path)
	if !ok || size != int64(len(body)) {
		t.Fatalf("MarkRead not stamped: ok=%v size=%d", ok, size)
	}
}

func TestRead_Execute_OffsetLimit(t *testing.T) {
	r, ctx, _ := newReadFixture(t)
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("L1\nL2\nL3\nL4\nL5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := r.Execute(ctx, `{"file_path":"`+path+`","offset":2,"limit":2}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "    2\tL2\n") || !strings.Contains(out, "    3\tL3\n") {
		t.Fatalf("expected lines 2-3 only:\n%s", out)
	}
	if strings.Contains(out, "L1") || strings.Contains(out, "L4") {
		t.Fatalf("leaked lines outside window:\n%s", out)
	}
	// 5 lines total, emitted 2 starting at offset 2 → 2 lines remain → truncation marker.
	if !strings.Contains(out, "truncated at line 3") {
		t.Fatalf("missing truncation marker:\n%s", out)
	}
}

func TestRead_Execute_EmptyFile(t *testing.T) {
	r, ctx, state := newReadFixture(t)
	path := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := r.Execute(ctx, `{"file_path":"`+path+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "empty contents") {
		t.Fatalf("empty-file reminder missing:\n%s", out)
	}
	// Empty file still gets stamped (size 0) so a follow-up Write can verify Read-first.
	if size, ok := state.WasRead(path); !ok || size != 0 {
		t.Fatalf("empty file not stamped: ok=%v size=%d", ok, size)
	}
}

func TestRead_Execute_NotFound(t *testing.T) {
	r, ctx, _ := newReadFixture(t)
	out, err := r.Execute(ctx, `{"file_path":"/no/such/file.txt"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "File not found") {
		t.Fatalf("expected not-found message, got:\n%s", out)
	}
}

func TestRead_Execute_IsDirectory(t *testing.T) {
	r, ctx, _ := newReadFixture(t)
	out, err := r.Execute(ctx, `{"file_path":"`+t.TempDir()+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "directory") || !strings.Contains(out, "Glob") {
		t.Fatalf("expected directory hint with Glob suggestion:\n%s", out)
	}
}

func TestRead_Execute_PathGuardDeny(t *testing.T) {
	r := &Read{pathGuard: pathguardpkg.New([]string{"/etc/"})}
	ctx := reqctxpkg.WithAgentState(context.Background(), agentstatepkg.New())
	out, err := r.Execute(ctx, `{"file_path":"/etc/passwd"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "denied by safety guard") {
		t.Fatalf("expected guard deny, got:\n%s", out)
	}
}

func TestRead_Execute_NoAgentState_TolerantSkipsStamp(t *testing.T) {
	// Read is read-only — AgentState absent is OK (just no stamp).
	// Only Write/Edit are fail-closed on missing state.
	//
	// Read 是只读——AgentState 缺失 OK（不盖章）。只有 Write/Edit fail-closed。
	r := &Read{pathGuard: pathguardpkg.New(nil)}
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := r.Execute(context.Background(), `{"file_path":"`+path+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "    1\thi") {
		t.Fatalf("expected content even without state:\n%s", out)
	}
}

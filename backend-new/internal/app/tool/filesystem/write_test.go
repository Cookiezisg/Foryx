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

func newWriteFixture(t *testing.T) (*Write, context.Context, *agentstatepkg.AgentState) {
	t.Helper()
	state := agentstatepkg.New()
	ctx := reqctxpkg.WithAgentState(context.Background(), state)
	return &Write{pathGuard: pathguardpkg.New(nil)}, ctx, state
}

func TestWrite_ValidateInput(t *testing.T) {
	w := &Write{pathGuard: pathguardpkg.New(nil)}
	cases := []struct {
		name      string
		json      string
		wantErrIs error
		wantMatch string
	}{
		{"empty path", `{"file_path":"","content":""}`, ErrEmptyFilePath, ""},
		{"missing content", `{"file_path":"/x"}`, nil, "content field is required"},
		{"empty content ok", `{"file_path":"/x","content":""}`, nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := w.ValidateInput([]byte(c.json))
			if c.wantErrIs != nil {
				if !errors.Is(got, c.wantErrIs) {
					t.Fatalf("err = %v, want Is(%v)", got, c.wantErrIs)
				}
				return
			}
			if c.wantMatch == "" {
				if got != nil {
					t.Fatalf("expected nil err, got %v", got)
				}
				return
			}
			if got == nil || !strings.Contains(got.Error(), c.wantMatch) {
				t.Fatalf("err = %v, want contains %q", got, c.wantMatch)
			}
		})
	}
}

func TestWrite_Execute_TildeExpanded(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("home dir unknown")
	}
	w := &Write{pathGuard: pathguardpkg.New(nil)}
	ctx := reqctxpkg.WithAgentState(context.Background(), agentstatepkg.New())
	// Parent dir missing → fails before writing anything, but the message proves
	// ~ expanded to the home-based absolute path. No file is created.
	out, err := w.Execute(ctx, `{"file_path":"~/__forgify_nope_dir__/x.txt","content":"y"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, home) {
		t.Fatalf("~ not expanded to home: %q", out)
	}
}

func TestWrite_Execute_RelativeRejected(t *testing.T) {
	w := &Write{pathGuard: pathguardpkg.New(nil)}
	ctx := reqctxpkg.WithAgentState(context.Background(), agentstatepkg.New())
	out, err := w.Execute(ctx, `{"file_path":"rel.txt","content":"y"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "must be absolute") {
		t.Fatalf("relative path should be rejected: %q", out)
	}
}

func TestWrite_Execute_NewFile(t *testing.T) {
	w, ctx, state := newWriteFixture(t)
	path := filepath.Join(t.TempDir(), "new.txt")
	out, err := w.Execute(ctx, `{"file_path":"`+path+`","content":"hello"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Wrote ") {
		t.Fatalf("expected Wrote message, got %q", out)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "hello" {
		t.Fatalf("file content = %q, want hello", got)
	}
	// Newly written file is stamped, so subsequent Edit could verify Read-first via the write.
	if size, ok := state.WasRead(path); !ok || size != 5 {
		t.Fatalf("post-write stamp missing: ok=%v size=%d", ok, size)
	}
}

func TestWrite_Execute_Overwrite_RequiresPriorRead(t *testing.T) {
	w, ctx, _ := newWriteFixture(t)
	path := filepath.Join(t.TempDir(), "existing.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Did not Read first → must be refused.
	out, err := w.Execute(ctx, `{"file_path":"`+path+`","content":"new"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "must be read first") {
		t.Fatalf("expected Read-first refusal, got %q", out)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "old" {
		t.Fatalf("file modified despite refusal: %q", got)
	}
}

func TestWrite_Execute_Overwrite_AfterReadStamp(t *testing.T) {
	w, ctx, state := newWriteFixture(t)
	path := filepath.Join(t.TempDir(), "existing.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Simulate Read having stamped it.
	state.MarkRead(path, 3)

	out, err := w.Execute(ctx, `{"file_path":"`+path+`","content":"new"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Wrote ") {
		t.Fatalf("expected Wrote message, got %q", out)
	}
}

func TestWrite_Execute_NoAgentState_OverwriteFailClosed(t *testing.T) {
	// Write is fail-closed when AgentState is missing: silently allowing would
	// defeat the write-before-read invariant.
	//
	// Write 在 AgentState 缺失时 fail-closed：静默放行会让写前必读形同虚设。
	w := &Write{pathGuard: pathguardpkg.New(nil)}
	path := filepath.Join(t.TempDir(), "existing.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := w.Execute(context.Background(), `{"file_path":"`+path+`","content":"new"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "agent state missing") {
		t.Fatalf("expected fail-closed message, got %q", out)
	}
}

func TestWrite_Execute_ParentMissing(t *testing.T) {
	w, ctx, _ := newWriteFixture(t)
	path := filepath.Join(t.TempDir(), "nope", "a.txt")
	out, err := w.Execute(ctx, `{"file_path":"`+path+`","content":"x"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Parent directory does not exist") {
		t.Fatalf("expected parent-missing hint, got %q", out)
	}
}

func TestWrite_Execute_ParentIsFile(t *testing.T) {
	w, ctx, _ := newWriteFixture(t)
	parentAsFile := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(parentAsFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parentAsFile, "child.txt")
	out, err := w.Execute(ctx, `{"file_path":"`+path+`","content":"x"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "not a directory") {
		t.Fatalf("expected parent-not-dir hint, got %q", out)
	}
}

func TestWrite_Execute_TargetIsDirectory(t *testing.T) {
	w, ctx, _ := newWriteFixture(t)
	out, err := w.Execute(ctx, `{"file_path":"`+t.TempDir()+`","content":"x"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Path is a directory") {
		t.Fatalf("expected is-directory refusal, got %q", out)
	}
}

func TestWrite_Execute_PathGuardWriteDeniesGitDir(t *testing.T) {
	// AllowWrite must reject .git/ (DefaultWriteOnlyExtras) even though plain
	// Allow would let it through. This is the whole point of the split.
	//
	// AllowWrite 必须拒 .git/（DefaultWriteOnlyExtras）即使 Allow 会放行——这正是分流要旨。
	w := &Write{pathGuard: pathguardpkg.NewWithWriteExtras(nil, []string{".git/"})}
	ctx := reqctxpkg.WithAgentState(context.Background(), agentstatepkg.New())
	gitFile := filepath.Join(t.TempDir(), ".git", "HEAD")
	if err := os.MkdirAll(filepath.Dir(gitFile), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := w.Execute(ctx, `{"file_path":"`+gitFile+`","content":"junk"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "denied by safety guard") {
		t.Fatalf("expected .git/ write deny, got %q", out)
	}
}

func TestWrite_Execute_PreservesMode(t *testing.T) {
	w, ctx, state := newWriteFixture(t)
	path := filepath.Join(t.TempDir(), "existing.txt")
	// Create with non-default 0o600 to detect CreateTemp-default leak.
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	state.MarkRead(path, 3)
	if _, err := w.Execute(ctx, `{"file_path":"`+path+`","content":"new"}`); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 0600 (CreateTemp default 0600 must not silently overwrite)", info.Mode().Perm())
	}
}

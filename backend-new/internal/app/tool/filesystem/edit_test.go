package filesystem

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentstatepkg "github.com/sunweilin/forgify/backend/internal/pkg/agentstate"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

func newEditFixture(t *testing.T) (*Edit, context.Context, *agentstatepkg.AgentState) {
	t.Helper()
	state := agentstatepkg.New()
	ctx := reqctxpkg.WithAgentState(context.Background(), state)
	return &Edit{pathGuard: pathguardpkg.New(nil)}, ctx, state
}

// prepareReadFile writes content and marks it Read so Edit's guards pass.
//
// prepareReadFile 写文件 + 盖章为已读，使 Edit 的守卫放行。
func prepareReadFile(t *testing.T, state *agentstatepkg.AgentState, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	state.MarkRead(path, int64(len(content)))
}

func TestEdit_ValidateInput(t *testing.T) {
	e := &Edit{pathGuard: pathguardpkg.New(nil)}
	cases := []struct {
		name      string
		json      string
		wantErrIs error
		wantMatch string
	}{
		{"empty path", `{"file_path":"","old_string":"a","new_string":"b"}`, ErrEmptyFilePath, ""},
		{"empty old", `{"file_path":"/x","old_string":"","new_string":"b"}`, ErrEmptyOldString, ""},
		{"missing old", `{"file_path":"/x","new_string":"b"}`, ErrEmptyOldString, ""},
		{"missing new", `{"file_path":"/x","old_string":"a"}`, nil, "new_string field is required"},
		{"noop", `{"file_path":"/x","old_string":"a","new_string":"a"}`, ErrEditNoOp, ""},
		{"empty new ok", `{"file_path":"/x","old_string":"a","new_string":""}`, nil, ""},
		{"happy", `{"file_path":"/x","old_string":"a","new_string":"b"}`, nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := e.ValidateInput(json.RawMessage(c.json))
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

func TestEdit_Execute_TildeExpanded(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("home dir unknown")
	}
	e := &Edit{pathGuard: pathguardpkg.New(nil)}
	ctx := reqctxpkg.WithAgentState(context.Background(), agentstatepkg.New())
	// Nonexistent file under ~ — proves ~ expanded; no side effect.
	out, err := e.Execute(ctx, `{"file_path":"~/__forgify_nope__.txt","old_string":"a","new_string":"b"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, home) {
		t.Fatalf("~ not expanded to home: %q", out)
	}
}

func TestEdit_Execute_RelativeRejected(t *testing.T) {
	e := &Edit{pathGuard: pathguardpkg.New(nil)}
	ctx := reqctxpkg.WithAgentState(context.Background(), agentstatepkg.New())
	out, err := e.Execute(ctx, `{"file_path":"rel.txt","old_string":"a","new_string":"b"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "must be absolute") {
		t.Fatalf("relative path should be rejected: %q", out)
	}
}

func TestEdit_Execute_SingleReplace(t *testing.T) {
	e, ctx, state := newEditFixture(t)
	path := filepath.Join(t.TempDir(), "a.txt")
	prepareReadFile(t, state, path, "hello world")

	out, err := e.Execute(ctx, `{"file_path":"`+path+`","old_string":"world","new_string":"forgify"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Replaced 1 occurrence") {
		t.Fatalf("expected 1-occurrence message, got %q", out)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "hello forgify" {
		t.Fatalf("file = %q, want %q", got, "hello forgify")
	}
	// MarkRead must reflect the new size.
	size, _ := state.WasRead(path)
	if size != int64(len("hello forgify")) {
		t.Fatalf("post-edit stamp size = %d, want %d", size, len("hello forgify"))
	}
}

func TestEdit_Execute_ReplaceAll(t *testing.T) {
	e, ctx, state := newEditFixture(t)
	path := filepath.Join(t.TempDir(), "a.txt")
	prepareReadFile(t, state, path, "foo bar foo baz foo")

	out, err := e.Execute(ctx, `{"file_path":"`+path+`","old_string":"foo","new_string":"X","replace_all":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Replaced 3 occurrences") {
		t.Fatalf("expected 3-occurrence message, got %q", out)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "X bar X baz X" {
		t.Fatalf("file = %q", got)
	}
}

func TestEdit_Execute_MultiMatchWithoutReplaceAll(t *testing.T) {
	e, ctx, state := newEditFixture(t)
	path := filepath.Join(t.TempDir(), "a.txt")
	prepareReadFile(t, state, path, "x x x")

	out, err := e.Execute(ctx, `{"file_path":"`+path+`","old_string":"x","new_string":"y"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Found 3 matches") || !strings.Contains(out, "replace_all") {
		t.Fatalf("expected ambiguity refusal, got %q", out)
	}
	// File must be untouched.
	got, _ := os.ReadFile(path)
	if string(got) != "x x x" {
		t.Fatalf("file modified despite refusal: %q", got)
	}
}

func TestEdit_Execute_NoMatch(t *testing.T) {
	e, ctx, state := newEditFixture(t)
	path := filepath.Join(t.TempDir(), "a.txt")
	prepareReadFile(t, state, path, "hello")

	out, err := e.Execute(ctx, `{"file_path":"`+path+`","old_string":"absent","new_string":"y"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "old_string not found") {
		t.Fatalf("expected not-found message, got %q", out)
	}
}

func TestEdit_Execute_RequiresPriorRead(t *testing.T) {
	e, ctx, _ := newEditFixture(t)
	path := filepath.Join(t.TempDir(), "a.txt")
	// Write directly without stamping.
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := e.Execute(ctx, `{"file_path":"`+path+`","old_string":"hello","new_string":"hi"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "must be read first") {
		t.Fatalf("expected Read-first refusal, got %q", out)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "hello" {
		t.Fatalf("file modified despite refusal: %q", got)
	}
}

func TestEdit_Execute_NoAgentState_FailClosed(t *testing.T) {
	e := &Edit{pathGuard: pathguardpkg.New(nil)}
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := e.Execute(context.Background(), `{"file_path":"`+path+`","old_string":"hello","new_string":"hi"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "agent state missing") {
		t.Fatalf("expected fail-closed message, got %q", out)
	}
}

func TestEdit_Execute_SizeDrift_Rejected(t *testing.T) {
	// External modification since last Read: size mismatch must abort.
	// 自上次 Read 起被外部改：size 不符必须中止。
	e, ctx, state := newEditFixture(t)
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	state.MarkRead(path, 3) // pretend we read a 3-byte version
	out, err := e.Execute(ctx, `{"file_path":"`+path+`","old_string":"hello","new_string":"hi"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "modified since last read") {
		t.Fatalf("expected drift refusal, got %q", out)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "hello" {
		t.Fatalf("file modified despite drift refusal: %q", got)
	}
}

func TestEdit_Execute_FileNotFound(t *testing.T) {
	e, ctx, _ := newEditFixture(t)
	out, err := e.Execute(ctx, `{"file_path":"/no/such/file.txt","old_string":"a","new_string":"b"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "File not found") || !strings.Contains(out, "Write") {
		t.Fatalf("expected not-found w/ Write hint, got %q", out)
	}
}

func TestEdit_Execute_IsDirectory(t *testing.T) {
	e, ctx, _ := newEditFixture(t)
	out, err := e.Execute(ctx, `{"file_path":"`+t.TempDir()+`","old_string":"a","new_string":"b"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "directory") {
		t.Fatalf("expected directory refusal, got %q", out)
	}
}

func TestEdit_Execute_PathGuardWriteDeniesEnvFile(t *testing.T) {
	e := &Edit{pathGuard: pathguardpkg.NewWithWriteExtras(nil, []string{".env"})}
	state := agentstatepkg.New()
	ctx := reqctxpkg.WithAgentState(context.Background(), state)
	envPath := filepath.Join(t.TempDir(), ".env")
	prepareReadFile(t, state, envPath, "SECRET=x")
	out, err := e.Execute(ctx, `{"file_path":"`+envPath+`","old_string":"x","new_string":"y"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "denied by safety guard") {
		t.Fatalf("expected .env write deny, got %q", out)
	}
}

func TestEdit_Execute_PreservesMode(t *testing.T) {
	e, ctx, state := newEditFixture(t)
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	state.MarkRead(path, 5)
	if _, err := e.Execute(ctx, `{"file_path":"`+path+`","old_string":"hello","new_string":"hi"}`); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 0600", info.Mode().Perm())
	}
}

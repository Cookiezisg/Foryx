package filesystem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	agentstatepkg "github.com/sunweilin/forgify/backend/internal/pkg/agentstate"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)


// allowAll is a PathGuard that permits every path. Used in tests that focus
// on Read behavior independent of the safety guard.
//
// allowAll 是放行任何路径的 PathGuard，用于关注 Read 行为本身的测试。
type allowAll struct{}

func (allowAll) Allow(string) (bool, string) { return true, "" }

// denyAll is a PathGuard that rejects every path with a fixed reason.
//
// denyAll 是拒绝任何路径的 PathGuard，固定 reason。
type denyAll struct{}

func (denyAll) Allow(string) (bool, string) { return false, "denied for test" }

// newRead builds a Read tool with the provided guard. Most tests use allowAll;
// PathGuard interaction has its own dedicated test.
//
// newRead 用给定 guard 构造 Read 工具。多数测试用 allowAll；PathGuard 交互
// 有专门测试。
func newRead(g pathguardpkg.PathGuard) *Read { return &Read{pathGuard: g} }

// ctxWithState returns a ctx carrying a fresh AgentState plus the state
// itself for assertions.
//
// ctxWithState 返回携带新建 AgentState 的 ctx + state 本身用于断言。
func ctxWithState() (context.Context, *agentstatepkg.AgentState) {
	state := &agentstatepkg.AgentState{}
	ctx := reqctxpkg.WithAgentState(context.Background(), state)
	return ctx, state
}

// writeTempFile drops the given content into a tmp file and returns its
// absolute path.
//
// writeTempFile 把 content 写入临时文件，返回绝对路径。
func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	return p
}

// callExecute is a tiny convenience wrapper for typing args as a struct.
//
// callExecute 是个小封装，让 args 写成 struct。
func callExecute(t *testing.T, r *Read, ctx context.Context, args map[string]any) (string, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return r.Execute(ctx, string(raw))
}


func TestRead_IdentityAndMetadata(t *testing.T) {
	r := newRead(allowAll{})
	if r.Name() != "Read" {
		t.Errorf("Name = %q, want Read", r.Name())
	}
	if r.Description() == "" {
		t.Error("Description empty")
	}
	if len(r.Parameters()) == 0 {
		t.Error("Parameters empty")
	}
	// Schema must be valid JSON.
	var schema map[string]any
	if err := json.Unmarshal(r.Parameters(), &schema); err != nil {
		t.Errorf("Parameters not valid JSON: %v", err)
	}
	if !r.IsReadOnly() {
		t.Error("IsReadOnly should be true")
	}
	if r.NeedsReadFirst() {
		t.Error("NeedsReadFirst should be false")
	}
	if !r.RequiresWorkspace() {
		t.Error("RequiresWorkspace should be true")
	}
}

func TestRead_CheckPermissionsAlwaysAllow(t *testing.T) {
	r := newRead(allowAll{})
	got := r.CheckPermissions(json.RawMessage(`{}`), toolapp.PermissionModeDefault)
	if got != toolapp.PermissionAllow {
		t.Errorf("CheckPermissions = %v, want PermissionAllow", got)
	}
}


func TestValidateInput_Empty(t *testing.T) {
	err := newRead(allowAll{}).ValidateInput(json.RawMessage(`{}`))
	if !errors.Is(err, ErrEmptyFilePath) {
		t.Errorf("err = %v, want ErrEmptyFilePath", err)
	}
}

func TestValidateInput_Relative(t *testing.T) {
	err := newRead(allowAll{}).ValidateInput(json.RawMessage(`{"file_path":"foo.txt"}`))
	if !errors.Is(err, ErrPathNotAbsolute) {
		t.Errorf("err = %v, want ErrPathNotAbsolute", err)
	}
}

func TestValidateInput_NegativeOffset(t *testing.T) {
	err := newRead(allowAll{}).ValidateInput(json.RawMessage(`{"file_path":"/x","offset":-1}`))
	if !errors.Is(err, ErrNegativeOffset) {
		t.Errorf("err = %v, want ErrNegativeOffset", err)
	}
}

func TestValidateInput_NegativeLimit(t *testing.T) {
	err := newRead(allowAll{}).ValidateInput(json.RawMessage(`{"file_path":"/x","limit":-1}`))
	if !errors.Is(err, ErrNegativeLimit) {
		t.Errorf("err = %v, want ErrNegativeLimit", err)
	}
}

func TestValidateInput_HappyPath(t *testing.T) {
	cases := []string{
		`{"file_path":"/x"}`,
		`{"file_path":"/x","offset":1,"limit":100}`,
		`{"file_path":"/x","offset":0,"limit":0}`, // zero = use default, not invalid
	}
	for _, c := range cases {
		if err := newRead(allowAll{}).ValidateInput(json.RawMessage(c)); err != nil {
			t.Errorf("expected nil err for %s, got %v", c, err)
		}
	}
}


func TestExecute_BasicTextFile(t *testing.T) {
	path := writeTempFile(t, "hello.txt", "first\nsecond\nthird\n")
	ctx, state := ctxWithState()
	r := newRead(allowAll{})

	out, err := callExecute(t, r, ctx, map[string]any{"file_path": path})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	want := "    1\tfirst\n    2\tsecond\n    3\tthird\n"
	if out != want {
		t.Errorf("output mismatch:\ngot:  %q\nwant: %q", out, want)
	}

	// Should mark file as seen with the actual size.
	// 应把 path 标为已读，size 是真实大小。
	if sz, ok := state.WasRead(path); !ok || sz <= 0 {
		t.Errorf("expected MarkRead(path, >0); got size=%d ok=%v", sz, ok)
	}
}

func TestExecute_EmptyFile(t *testing.T) {
	path := writeTempFile(t, "empty.txt", "")
	ctx, state := ctxWithState()
	r := newRead(allowAll{})

	out, err := callExecute(t, r, ctx, map[string]any{"file_path": path})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "system-reminder") {
		t.Errorf("expected system-reminder for empty file, got %q", out)
	}

	// Empty file should still mark as seen (size 0).
	// 空文件也要标记（size=0），让 Edit/Write 不被卡。
	if sz, ok := state.WasRead(path); !ok || sz != 0 {
		t.Errorf("expected MarkRead(path, 0); got size=%d ok=%v", sz, ok)
	}
}

func TestExecute_OffsetSkipsLines(t *testing.T) {
	path := writeTempFile(t, "five.txt", "a\nb\nc\nd\ne\n")
	ctx, _ := ctxWithState()
	r := newRead(allowAll{})

	out, err := callExecute(t, r, ctx, map[string]any{"file_path": path, "offset": 3})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := "    3\tc\n    4\td\n    5\te\n"
	if out != want {
		t.Errorf("offset skip mismatch:\ngot:  %q\nwant: %q", out, want)
	}
}

func TestExecute_LimitTruncates(t *testing.T) {
	// 5 lines but limit 2 → expect lines 1-2 + truncation hint.
	path := writeTempFile(t, "five.txt", "a\nb\nc\nd\ne\n")
	ctx, _ := ctxWithState()
	r := newRead(allowAll{})

	out, err := callExecute(t, r, ctx, map[string]any{"file_path": path, "limit": 2})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out, "    1\ta\n") || !strings.Contains(out, "    2\tb\n") {
		t.Errorf("expected lines 1-2 in output, got %q", out)
	}
	if strings.Contains(out, "    3\t") {
		t.Errorf("line 3 should be truncated, got %q", out)
	}
	if !strings.Contains(out, "[truncated at line 2") {
		t.Errorf("expected truncation hint, got %q", out)
	}
}

func TestExecute_LimitExactlyMatchesNoTruncationHint(t *testing.T) {
	// 3 lines, limit 3 → all returned, NO truncation hint.
	path := writeTempFile(t, "three.txt", "a\nb\nc\n")
	ctx, _ := ctxWithState()
	r := newRead(allowAll{})

	out, err := callExecute(t, r, ctx, map[string]any{"file_path": path, "limit": 3})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(out, "truncated") {
		t.Errorf("limit == file lines should NOT show truncation hint, got %q", out)
	}
}

func TestExecute_OffsetAndLimitTogether(t *testing.T) {
	// 10 lines, offset 4, limit 2 → lines 4-5 + truncation hint (lines 6-10 still exist).
	var b strings.Builder
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&b, "line%d\n", i)
	}
	path := writeTempFile(t, "ten.txt", b.String())
	ctx, _ := ctxWithState()
	r := newRead(allowAll{})

	out, err := callExecute(t, r, ctx, map[string]any{"file_path": path, "offset": 4, "limit": 2})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "    4\tline4\n    5\tline5\n") {
		t.Errorf("expected lines 4-5, got %q", out)
	}
	if !strings.Contains(out, "[truncated at line 5") {
		t.Errorf("expected truncation hint at line 5, got %q", out)
	}
}

func TestExecute_NoTrailingNewlineLastLine(t *testing.T) {
	// File without trailing \n should still emit the last line (bufio.Scanner drops EOLs).
	path := writeTempFile(t, "no-eol.txt", "alpha\nbeta")
	ctx, _ := ctxWithState()
	r := newRead(allowAll{})

	out, err := callExecute(t, r, ctx, map[string]any{"file_path": path})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := "    1\talpha\n    2\tbeta\n"
	if out != want {
		t.Errorf("got %q want %q", out, want)
	}
}


func TestExecute_FileNotFound(t *testing.T) {
	ctx, _ := ctxWithState()
	r := newRead(allowAll{})
	missing := filepath.Join(t.TempDir(), "missing.txt")

	out, err := callExecute(t, r, ctx, map[string]any{"file_path": missing})
	if err != nil {
		t.Fatalf("Execute returned Go err for missing file (should be friendly string): %v", err)
	}
	if !strings.Contains(out, "File not found") {
		t.Errorf("expected 'File not found' message, got %q", out)
	}
}

func TestExecute_PathIsDirectory(t *testing.T) {
	ctx, _ := ctxWithState()
	r := newRead(allowAll{})
	dir := t.TempDir()

	out, err := callExecute(t, r, ctx, map[string]any{"file_path": dir})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "directory") {
		t.Errorf("expected directory message, got %q", out)
	}
}

func TestExecute_PathGuardDenied(t *testing.T) {
	ctx, state := ctxWithState()
	r := newRead(denyAll{})

	out, err := callExecute(t, r, ctx, map[string]any{"file_path": "/some/path"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "denied for test") {
		t.Errorf("expected denial reason, got %q", out)
	}
	// Denied path should NOT be marked as seen — Edit/Write would otherwise
	// pass their guard for a path Read couldn't actually access.
	// 被拒路径不应标 seen——否则 Edit/Write 会通过它们的 guard 操作 Read 实际
	// 访问不到的 path。
	if _, ok := state.WasRead("/some/path"); ok {
		t.Error("denied path was incorrectly marked as seen")
	}
}

func TestExecute_NoAgentStateInCtx(t *testing.T) {
	// Defensive: AgentState missing → Read still succeeds, just no MarkRead.
	// 防御：AgentState 缺失 → Read 仍成功，只是不调 MarkRead。
	path := writeTempFile(t, "ok.txt", "hi\n")
	r := newRead(allowAll{})

	out, err := r.Execute(context.Background(), fmt.Sprintf(`{"file_path":%q}`, path))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "hi") {
		t.Errorf("expected file content, got %q", out)
	}
}

func TestExecute_LineTooLong(t *testing.T) {
	// One line exceeding maxScannerLineBytes triggers bufio.ErrTooLong.
	// We surface it as a friendly tool_result string.
	bigLine := strings.Repeat("x", maxScannerLineBytes+1)
	path := writeTempFile(t, "huge.txt", bigLine)
	ctx, _ := ctxWithState()
	r := newRead(allowAll{})

	out, err := callExecute(t, r, ctx, map[string]any{"file_path": path})
	if err != nil {
		t.Fatalf("Execute: %v (should be friendly string)", err)
	}
	if !strings.Contains(out, "Failed to read") {
		t.Errorf("expected 'Failed to read' message, got %q", out[:min(200, len(out))])
	}
}

func TestExecute_OffsetBeyondEOF(t *testing.T) {
	path := writeTempFile(t, "two.txt", "a\nb\n")
	ctx, _ := ctxWithState()
	r := newRead(allowAll{})

	// offset 99 beyond EOF → empty content but still successful (file exists).
	out, err := callExecute(t, r, ctx, map[string]any{"file_path": path, "offset": 99})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "" {
		t.Errorf("expected empty output for offset beyond EOF, got %q", out)
	}
}


func TestExecute_DefaultLimitAppliedWhenZero(t *testing.T) {
	// Build > defaultLimit lines to confirm default kicks in (2000+1).
	var b strings.Builder
	for i := 1; i <= defaultLimit+5; i++ {
		fmt.Fprintf(&b, "line%d\n", i)
	}
	path := writeTempFile(t, "many.txt", b.String())
	ctx, _ := ctxWithState()
	r := newRead(allowAll{})

	out, err := callExecute(t, r, ctx, map[string]any{"file_path": path}) // omit limit
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Should include line defaultLimit but not defaultLimit+1.
	if !strings.Contains(out, fmt.Sprintf("\tline%d\n", defaultLimit)) {
		t.Errorf("expected line%d in output (default limit %d)", defaultLimit, defaultLimit)
	}
	if strings.Contains(out, fmt.Sprintf("\tline%d\n", defaultLimit+1)) {
		t.Errorf("line%d should be truncated by default limit", defaultLimit+1)
	}
	if !strings.Contains(out, "[truncated") {
		t.Error("expected truncation hint when default limit kicks in")
	}
}


var _ toolapp.Tool = (*Read)(nil)

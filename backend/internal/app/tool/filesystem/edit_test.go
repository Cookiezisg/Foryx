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
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)


func newEdit() *Edit { return &Edit{pathGuard: allowAll{}} }

// callEdit is a tiny wrapper to drive Execute with a struct.
//
// callEdit 是个小封装。
func callEdit(t *testing.T, e *Edit, ctx context.Context, args map[string]any) (string, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return e.Execute(ctx, string(raw))
}


func TestEdit_IdentityAndMetadata(t *testing.T) {
	e := newEdit()
	if e.Name() != "Edit" {
		t.Errorf("Name = %q, want Edit", e.Name())
	}
	if e.Description() == "" {
		t.Error("Description empty")
	}
	if len(e.Parameters()) == 0 {
		t.Error("Parameters empty")
	}
	var schema map[string]any
	if err := json.Unmarshal(e.Parameters(), &schema); err != nil {
		t.Errorf("Parameters not valid JSON: %v", err)
	}
	if e.IsReadOnly() {
		t.Error("IsReadOnly should be false")
	}
	if !e.NeedsReadFirst() {
		t.Error("NeedsReadFirst should be true")
	}
	if !e.RequiresWorkspace() {
		t.Error("RequiresWorkspace should be true")
	}
}

func TestEdit_CheckPermissionsAlwaysAllow(t *testing.T) {
	e := newEdit()
	got := e.CheckPermissions(json.RawMessage(`{}`), toolapp.PermissionModeDefault)
	if got != toolapp.PermissionAllow {
		t.Errorf("CheckPermissions = %v, want PermissionAllow", got)
	}
}


func TestEditValidateInput_EmptyFilePath(t *testing.T) {
	err := newEdit().ValidateInput(json.RawMessage(`{"old_string":"a","new_string":"b"}`))
	if !errors.Is(err, ErrEmptyFilePath) {
		t.Errorf("err = %v, want ErrEmptyFilePath", err)
	}
}

func TestEditValidateInput_RelativePath(t *testing.T) {
	err := newEdit().ValidateInput(json.RawMessage(`{"file_path":"foo.txt","old_string":"a","new_string":"b"}`))
	if !errors.Is(err, ErrPathNotAbsolute) {
		t.Errorf("err = %v, want ErrPathNotAbsolute", err)
	}
}

func TestEditValidateInput_EmptyOldString(t *testing.T) {
	err := newEdit().ValidateInput(json.RawMessage(`{"file_path":"/x","old_string":"","new_string":"b"}`))
	if !errors.Is(err, ErrEmptyOldString) {
		t.Errorf("err = %v, want ErrEmptyOldString", err)
	}
}

func TestEditValidateInput_MissingOldString(t *testing.T) {
	err := newEdit().ValidateInput(json.RawMessage(`{"file_path":"/x","new_string":"b"}`))
	if !errors.Is(err, ErrEmptyOldString) {
		t.Errorf("err = %v, want ErrEmptyOldString", err)
	}
}

func TestEditValidateInput_MissingNewString(t *testing.T) {
	err := newEdit().ValidateInput(json.RawMessage(`{"file_path":"/x","old_string":"a"}`))
	if err == nil {
		t.Error("expected err for missing new_string")
	}
}

func TestEditValidateInput_NoOpEdit(t *testing.T) {
	err := newEdit().ValidateInput(json.RawMessage(`{"file_path":"/x","old_string":"same","new_string":"same"}`))
	if !errors.Is(err, ErrEditNoOp) {
		t.Errorf("err = %v, want ErrEditNoOp", err)
	}
}

func TestEditValidateInput_HappyPath(t *testing.T) {
	cases := []string{
		`{"file_path":"/x","old_string":"a","new_string":"b"}`,
		`{"file_path":"/x","old_string":"a","new_string":""}`,                    // empty new_string is delete
		`{"file_path":"/x","old_string":"a","new_string":"b","replace_all":true}`,
	}
	for _, c := range cases {
		if err := newEdit().ValidateInput(json.RawMessage(c)); err != nil {
			t.Errorf("expected nil err for %s, got %v", c, err)
		}
	}
}


// readWriteSetup creates a file, marks it as Read in a fresh AgentState,
// returns ctx + path. Used by every Edit success-path test.
//
// readWriteSetup 创建文件、在新建的 AgentState 里标 Read，返回 ctx + path。
// 每个 Edit 成功路径测试都用它。
func readWriteSetup(t *testing.T, name, content string) (context.Context, string) {
	t.Helper()
	path := writeTempFile(t, name, content)
	ctx, state := ctxWithState()
	state.MarkRead(path, int64(len(content)))
	return ctx, path
}

func TestEditExecute_SingleReplace(t *testing.T) {
	ctx, path := readWriteSetup(t, "src.txt", "hello world\n")
	e := newEdit()

	out, err := callEdit(t, e, ctx, map[string]any{
		"file_path":  path,
		"old_string": "world",
		"new_string": "Forgify",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Replaced 1 occurrence") {
		t.Errorf("expected explicit '1 occurrence', got %q", out)
	}
	if got := readContent(t, path); got != "hello Forgify\n" {
		t.Errorf("content = %q, want %q", got, "hello Forgify\n")
	}
}

func TestEditExecute_ReplaceAllMultiple(t *testing.T) {
	ctx, path := readWriteSetup(t, "names.txt", "Alice\nAlice\nAlice\n")
	e := newEdit()

	out, err := callEdit(t, e, ctx, map[string]any{
		"file_path":   path,
		"old_string":  "Alice",
		"new_string":  "Bob",
		"replace_all": true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Replaced 3 occurrences") {
		t.Errorf("expected explicit '3 occurrences', got %q", out)
	}
	if got := readContent(t, path); got != "Bob\nBob\nBob\n" {
		t.Errorf("content = %q", got)
	}
}

func TestEditExecute_CrossLineOldString(t *testing.T) {
	// old_string spanning multiple lines (contains \n) is supported.
	// 跨多行的 old_string（含 \n）应支持。
	ctx, path := readWriteSetup(t, "multi.txt", "line a\nline b\nline c\n")
	e := newEdit()

	out, err := callEdit(t, e, ctx, map[string]any{
		"file_path":  path,
		"old_string": "line a\nline b\n",
		"new_string": "MERGED\n",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Replaced 1 occurrence") {
		t.Errorf("expected success, got %q", out)
	}
	if got := readContent(t, path); got != "MERGED\nline c\n" {
		t.Errorf("content = %q", got)
	}
}

func TestEditExecute_RegexMetacharsTreatedLiterally(t *testing.T) {
	// old_string contains regex metacharacters; matching must be literal.
	// old_string 含 regex 元字符；匹配应为字面量。
	ctx, path := readWriteSetup(t, "code.txt", "func foo() { return .+* }\n")
	e := newEdit()

	out, err := callEdit(t, e, ctx, map[string]any{
		"file_path":  path,
		"old_string": ".+*",
		"new_string": "nil",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Replaced") {
		t.Errorf("expected literal match success, got %q", out)
	}
	if !strings.Contains(readContent(t, path), "return nil ") {
		t.Errorf("literal replacement did not happen")
	}
}

func TestEditExecute_DeleteByEmptyNewString(t *testing.T) {
	ctx, path := readWriteSetup(t, "del.txt", "before [REMOVE_ME] after\n")
	e := newEdit()

	out, err := callEdit(t, e, ctx, map[string]any{
		"file_path":  path,
		"old_string": " [REMOVE_ME]",
		"new_string": "",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Replaced 1 occurrence") {
		t.Errorf("got %q", out)
	}
	if got := readContent(t, path); got != "before after\n" {
		t.Errorf("content = %q", got)
	}
}

func TestEditExecute_MarkReadUpdatedAfterEdit(t *testing.T) {
	// After Edit, SeenFiles[path] should reflect the new file size so a
	// follow-up Edit on the same path passes the size-match guard.
	// Edit 后 SeenFiles[path] 应反映新 size，让对同 path 的后续 Edit 通过
	// size 匹配守卫。
	ctx, _ := ctxWithState() // empty state
	path := writeTempFile(t, "chain.txt", "v1 here\n")
	state, _ := reqctxpkg.GetAgentState(ctx)
	state.MarkRead(path, int64(len("v1 here\n")))

	e := newEdit()
	if _, err := callEdit(t, e, ctx, map[string]any{
		"file_path": path, "old_string": "v1", "new_string": "v2-longer",
	}); err != nil {
		t.Fatalf("first Edit: %v", err)
	}

	// Second Edit on same path should NOT trigger size-mismatch guard.
	// 对同 path 的第二次 Edit 不应触发 size 失配守卫。
	out, err := callEdit(t, e, ctx, map[string]any{
		"file_path": path, "old_string": "v2-longer", "new_string": "v3",
	})
	if err != nil {
		t.Fatalf("second Edit: %v", err)
	}
	if !strings.Contains(out, "Replaced 1 occurrence") {
		t.Errorf("chained Edit failed: %q", out)
	}
	if !strings.Contains(readContent(t, path), "v3") {
		t.Errorf("v3 not present in final content")
	}
}


func TestEditExecute_FileNotFound(t *testing.T) {
	ctx, _ := ctxWithState()
	e := newEdit()
	missing := filepath.Join(t.TempDir(), "nope.txt")

	out, err := callEdit(t, e, ctx, map[string]any{
		"file_path": missing, "old_string": "a", "new_string": "b",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "File not found") {
		t.Errorf("expected file-not-found, got %q", out)
	}
}

func TestEditExecute_PathIsDirectory(t *testing.T) {
	ctx, _ := ctxWithState()
	e := newEdit()
	out, err := callEdit(t, e, ctx, map[string]any{
		"file_path": t.TempDir(), "old_string": "a", "new_string": "b",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "directory") {
		t.Errorf("expected directory message, got %q", out)
	}
}

func TestEditExecute_PathGuardDenied(t *testing.T) {
	ctx, _ := ctxWithState()
	path := writeTempFile(t, "x.txt", "abc")
	e := &Edit{pathGuard: denyAll{}}

	out, err := callEdit(t, e, ctx, map[string]any{
		"file_path": path, "old_string": "a", "new_string": "z",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "denied for test") {
		t.Errorf("expected guard denial, got %q", out)
	}
	if got := readContent(t, path); got != "abc" {
		t.Errorf("content modified despite denial: %q", got)
	}
}

func TestEditExecute_NoAgentStateRefuses(t *testing.T) {
	path := writeTempFile(t, "x.txt", "abc")
	e := newEdit()

	out, err := e.Execute(context.Background(),
		fmt.Sprintf(`{"file_path":%q,"old_string":"a","new_string":"z"}`, path))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "agent state missing") {
		t.Errorf("expected agent-state-missing denial, got %q", out)
	}
	if got := readContent(t, path); got != "abc" {
		t.Errorf("content modified despite denial: %q", got)
	}
}

func TestEditExecute_NotReadFirst(t *testing.T) {
	// File exists, but never Read in this conversation.
	// 文件存在但本对话内没 Read 过。
	ctx, _ := ctxWithState() // empty AgentState
	path := writeTempFile(t, "x.txt", "abc")
	e := newEdit()

	out, err := callEdit(t, e, ctx, map[string]any{
		"file_path": path, "old_string": "a", "new_string": "z",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "must be read first") {
		t.Errorf("expected must-Read-first denial, got %q", out)
	}
	if got := readContent(t, path); got != "abc" {
		t.Errorf("content modified despite denial: %q", got)
	}
}

func TestEditExecute_ExternalModificationDetected(t *testing.T) {
	// Read records size 5 → external write changes size → Edit detects.
	// Read 记 size 5 → 外部 write 改 size → Edit 检测出。
	ctx, _ := ctxWithState()
	path := writeTempFile(t, "x.txt", "hello")
	state, _ := reqctxpkg.GetAgentState(ctx)
	state.MarkRead(path, 5)

	// Simulate external modification.
	if err := os.WriteFile(path, []byte("hello world (longer now)"), 0o644); err != nil {
		t.Fatalf("external write: %v", err)
	}

	out, err := callEdit(t, newEdit(), ctx, map[string]any{
		"file_path": path, "old_string": "hello", "new_string": "bye",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "modified since last read") {
		t.Errorf("expected external-modification denial, got %q", out)
	}
}

func TestEditExecute_ZeroMatches(t *testing.T) {
	ctx, path := readWriteSetup(t, "x.txt", "alpha beta")
	out, err := callEdit(t, newEdit(), ctx, map[string]any{
		"file_path": path, "old_string": "gamma", "new_string": "delta",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("expected 'not found', got %q", out)
	}
	if got := readContent(t, path); got != "alpha beta" {
		t.Errorf("content modified: %q", got)
	}
}

func TestEditExecute_MultipleMatchesWithoutReplaceAll(t *testing.T) {
	ctx, path := readWriteSetup(t, "x.txt", "one one one")
	out, err := callEdit(t, newEdit(), ctx, map[string]any{
		"file_path": path, "old_string": "one", "new_string": "1",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Found 3 matches") {
		t.Errorf("expected 'Found 3 matches', got %q", out)
	}
	if !strings.Contains(out, "replace_all is false") {
		t.Errorf("expected replace_all hint, got %q", out)
	}
	if got := readContent(t, path); got != "one one one" {
		t.Errorf("content modified despite N>1 + replace_all=false: %q", got)
	}
}


func TestEditExecute_PreservesFileMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "perm.txt")
	if err := os.WriteFile(path, []byte("foo"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ctx, state := ctxWithState()
	state.MarkRead(path, 3)

	if _, err := callEdit(t, newEdit(), ctx, map[string]any{
		"file_path": path, "old_string": "foo", "new_string": "bar",
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 0o600 (preserved)", got)
	}
}


func TestEditExecute_MarkdownBoldReplaceAll_NoSilentSkip(t *testing.T) {
	// CC #51986: replace_all on patterns near markdown bold close (e.g.
	// " **[WEAK]**") silently skips matches and consumes adjacent newlines.
	// Forgify trusts Go stdlib (decision D1); strings.ReplaceAll has no such
	// bug. This test fixes the property: 5 matches → all 5 replaced, no
	// newline corruption.
	//
	// CC #51986：replace_all 在 markdown 加粗 close 附近（如 " **[WEAK]**"）
	// 会静默跳过且吃 newline。Forgify 信任 Go stdlib（决策 D1）；
	// strings.ReplaceAll 无此 bug。本测试钉死该属性：5 处匹配 → 全替，
	// 不破坏 newline。
	const original = `# Report

The crypto used was AES-128 **[WEAK]** which is no good.
The IV was reused **[WEAK]** as expected.
The key derivation was PBKDF2 **[WEAK]** at 100 rounds.
The MAC was HMAC-MD5 **[WEAK]** also bad.
The randomness was math/rand **[WEAK]** which is critical.
`
	ctx, _ := ctxWithState()
	path := writeTempFile(t, "report.md", original)
	state, _ := reqctxpkg.GetAgentState(ctx)
	state.MarkRead(path, int64(len(original)))

	out, err := callEdit(t, newEdit(), ctx, map[string]any{
		"file_path":   path,
		"old_string":  " **[WEAK]**",
		"new_string":  " **[CRITICAL]**",
		"replace_all": true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Replaced 5 occurrences") {
		t.Errorf("expected '5 occurrences' (not silent skip), got %q", out)
	}

	got := readContent(t, path)
	if c := strings.Count(got, "[CRITICAL]"); c != 5 {
		t.Errorf("expected 5 CRITICAL replacements, got %d. Content:\n%s", c, got)
	}
	if strings.Contains(got, "[WEAK]") {
		t.Errorf("[WEAK] should have been fully replaced; content:\n%s", got)
	}
	// Line count must be preserved (newlines not consumed).
	// 行数必须保留（不吃 newline）。
	if got, want := strings.Count(got, "\n"), strings.Count(original, "\n"); got != want {
		t.Errorf("line count changed: got %d, want %d (newline consumption regression)", got, want)
	}
}

// Compile-time check.
var _ toolapp.Tool = (*Edit)(nil)

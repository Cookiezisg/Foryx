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
)

// newWrite is a small constructor mirror of newRead for symmetry.
//
// newWrite 跟 newRead 对称的小构造函数。
func newWrite() *Write { return &Write{pathGuard: allowAll{}} }

// readContent reads a file's bytes for assertion convenience.
//
// readContent 读文件字节，方便断言。
func readContent(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readContent(%s): %v", path, err)
	}
	return string(b)
}

// callWrite is a tiny convenience wrapper.
//
// callWrite 是个小封装。
func callWrite(t *testing.T, w *Write, ctx context.Context, args map[string]any) (string, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return w.Execute(ctx, string(raw))
}


func TestWrite_IdentityAndMetadata(t *testing.T) {
	w := newWrite()
	if w.Name() != "Write" {
		t.Errorf("Name = %q, want Write", w.Name())
	}
	if w.Description() == "" {
		t.Error("Description empty")
	}
	if len(w.Parameters()) == 0 {
		t.Error("Parameters empty")
	}
	var schema map[string]any
	if err := json.Unmarshal(w.Parameters(), &schema); err != nil {
		t.Errorf("Parameters not valid JSON: %v", err)
	}
	if w.IsReadOnly() {
		t.Error("IsReadOnly should be false")
	}
	if !w.NeedsReadFirst() {
		t.Error("NeedsReadFirst should be true")
	}
	if !w.RequiresWorkspace() {
		t.Error("RequiresWorkspace should be true")
	}
}

func TestWrite_CheckPermissionsAlwaysAllow(t *testing.T) {
	w := newWrite()
	got := w.CheckPermissions(json.RawMessage(`{}`), toolapp.PermissionModeDefault)
	if got != toolapp.PermissionAllow {
		t.Errorf("CheckPermissions = %v, want PermissionAllow", got)
	}
}


func TestWriteValidateInput_EmptyFilePath(t *testing.T) {
	err := newWrite().ValidateInput(json.RawMessage(`{"content":"x"}`))
	if !errors.Is(err, ErrEmptyFilePath) {
		t.Errorf("err = %v, want ErrEmptyFilePath", err)
	}
}

func TestWriteValidateInput_RelativePath(t *testing.T) {
	err := newWrite().ValidateInput(json.RawMessage(`{"file_path":"foo.txt","content":"x"}`))
	if !errors.Is(err, ErrPathNotAbsolute) {
		t.Errorf("err = %v, want ErrPathNotAbsolute", err)
	}
}

func TestWriteValidateInput_ContentMissing(t *testing.T) {
	// Missing content key (vs empty string) should error — we want LLM to be
	// explicit about its intent.
	// content key 缺失（vs 空串）应报错——要让 LLM 显式表达意图。
	err := newWrite().ValidateInput(json.RawMessage(`{"file_path":"/x"}`))
	if err == nil {
		t.Error("expected err for missing content key")
	}
}

func TestWriteValidateInput_EmptyContentOK(t *testing.T) {
	// Empty string content is valid (creating empty files).
	// 空串 content 合法（创建空文件）。
	err := newWrite().ValidateInput(json.RawMessage(`{"file_path":"/x","content":""}`))
	if err != nil {
		t.Errorf("expected nil err for empty content, got %v", err)
	}
}


func TestWriteExecute_NewFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "new.txt")
	ctx, state := ctxWithState()
	w := newWrite()

	out, err := callWrite(t, w, ctx, map[string]any{"file_path": target, "content": "hello\n"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Wrote ") {
		t.Errorf("expected success message, got %q", out)
	}
	if got := readContent(t, target); got != "hello\n" {
		t.Errorf("content = %q, want %q", got, "hello\n")
	}
	// New file should be marked as Read so subsequent Edit on the same path passes.
	// 新建文件应标 Read，让对同 path 的 Edit 通过。
	if sz, ok := state.WasRead(target); !ok || sz != int64(len("hello\n")) {
		t.Errorf("expected MarkRead with size %d, got size=%d ok=%v", len("hello\n"), sz, ok)
	}
}

func TestWriteExecute_EmptyContent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "empty.txt")
	ctx, state := ctxWithState()
	w := newWrite()

	out, err := callWrite(t, w, ctx, map[string]any{"file_path": target, "content": ""})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Wrote ") {
		t.Errorf("expected success message, got %q", out)
	}
	info, _ := os.Stat(target)
	if info == nil || info.Size() != 0 {
		t.Errorf("expected empty file, got info=%v", info)
	}
	if sz, ok := state.WasRead(target); !ok || sz != 0 {
		t.Errorf("expected MarkRead with size 0, got size=%d ok=%v", sz, ok)
	}
}

func TestWriteExecute_OverwriteAfterRead(t *testing.T) {
	// Existing file → must be Read first → then Write succeeds.
	// 已存在 → 必须先 Read → 然后 Write 成功。
	dir := t.TempDir()
	target := filepath.Join(dir, "exists.txt")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ctx, state := ctxWithState()
	state.MarkRead(target, 3) // simulate Read having seen the original
	w := newWrite()

	out, err := callWrite(t, w, ctx, map[string]any{"file_path": target, "content": "new"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Wrote ") {
		t.Errorf("expected success message, got %q", out)
	}
	if got := readContent(t, target); got != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}
}


func TestWriteExecute_OverwriteWithoutReadDenied(t *testing.T) {
	// Existing file + no Read → should be denied with helpful message.
	// 已存在 + 未 Read → 拒绝并给友好消息。
	dir := t.TempDir()
	target := filepath.Join(dir, "exists.txt")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ctx, _ := ctxWithState() // empty AgentState — file NOT marked as read
	w := newWrite()

	out, err := callWrite(t, w, ctx, map[string]any{"file_path": target, "content": "new"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "must be read first") {
		t.Errorf("expected must-Read-first denial, got %q", out)
	}
	// Original content must be intact.
	// 原始内容必须保留。
	if got := readContent(t, target); got != "old" {
		t.Errorf("content modified despite denial: %q", got)
	}
}

func TestWriteExecute_NoAgentStateRefusesOverwrite(t *testing.T) {
	// Defensive: AgentState missing in ctx → refuse overwrite (rather than
	// silently allowing it, which would defeat the must-Read-first guard).
	// 防御：ctx 缺 AgentState → 拒绝覆写（静默放过会让守卫形同虚设）。
	dir := t.TempDir()
	target := filepath.Join(dir, "exists.txt")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := newWrite()
	out, err := w.Execute(context.Background(),
		fmt.Sprintf(`{"file_path":%q,"content":"new"}`, target))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "agent state missing") {
		t.Errorf("expected agent-state-missing denial, got %q", out)
	}
	if got := readContent(t, target); got != "old" {
		t.Errorf("content modified despite denial: %q", got)
	}
}

func TestWriteExecute_NoAgentStateAllowsNewFile(t *testing.T) {
	// New file (target doesn't exist) doesn't need Read-first guard, so
	// missing AgentState should not block creation. (It just means no MarkRead.)
	// 新文件（目标不存在）不走 must-Read-first 守卫，AgentState 缺失不应
	// 阻塞创建（仅意味没 MarkRead）。
	dir := t.TempDir()
	target := filepath.Join(dir, "fresh.txt")

	w := newWrite()
	out, err := w.Execute(context.Background(),
		fmt.Sprintf(`{"file_path":%q,"content":"hi"}`, target))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Wrote ") {
		t.Errorf("expected success, got %q", out)
	}
	if readContent(t, target) != "hi" {
		t.Errorf("file content not written")
	}
}


func TestWriteExecute_ParentDirMissing(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "no-such-parent", "file.txt")
	ctx, _ := ctxWithState()
	w := newWrite()

	out, err := callWrite(t, w, ctx, map[string]any{"file_path": target, "content": "x"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Parent directory does not exist") {
		t.Errorf("expected parent-missing message, got %q", out)
	}
}

func TestWriteExecute_PathIsExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "iam-a-dir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ctx, _ := ctxWithState()
	w := newWrite()

	out, err := callWrite(t, w, ctx, map[string]any{"file_path": subdir, "content": "x"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "directory") {
		t.Errorf("expected directory message, got %q", out)
	}
}

func TestWriteExecute_PathGuardDenied(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "x.txt")
	ctx, _ := ctxWithState()
	w := &Write{pathGuard: denyAll{}}

	out, err := callWrite(t, w, ctx, map[string]any{"file_path": target, "content": "x"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "denied for test") {
		t.Errorf("expected guard denial, got %q", out)
	}
	if _, statErr := os.Stat(target); statErr == nil {
		t.Errorf("file was created despite guard denial")
	}
}


func TestWriteExecute_PreservesExistingFileMode(t *testing.T) {
	// Pre-existing file with 0o600 → after Read + overwrite, mode should
	// remain 0o600 (not silently shrink to 0o644 or to CreateTemp's 0o600).
	// 已存在 0o600 文件 → Read + 覆写后 mode 应保持 0o600（不静默改成 0o644
	// 或 CreateTemp 默认的 0o600）。
	dir := t.TempDir()
	target := filepath.Join(dir, "perm.txt")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ctx, state := ctxWithState()
	state.MarkRead(target, 3)
	w := newWrite()

	out, err := callWrite(t, w, ctx, map[string]any{"file_path": target, "content": "new"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Wrote ") {
		t.Errorf("expected success, got %q", out)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 0o600 (must preserve original mode)", got)
	}
}

func TestWriteExecute_NewFileGetsDefaultMode(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "fresh.txt")
	ctx, _ := ctxWithState()
	w := newWrite()

	out, err := callWrite(t, w, ctx, map[string]any{"file_path": target, "content": "x"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Wrote ") {
		t.Errorf("expected success, got %q", out)
	}
	info, _ := os.Stat(target)
	if info == nil {
		t.Fatal("file not created")
	}
	if got := info.Mode().Perm(); got != defaultFileMode {
		t.Errorf("mode = %o, want defaultFileMode %o", got, defaultFileMode)
	}
}

// Compile-time check.
var _ toolapp.Tool = (*Write)(nil)

// silence unused-import on *agentstatepkg via a no-op (the test file
// imports the package transitively via ctxWithState helper from read_test.go,
// which is in the same package; this var is a defensive marker).
//
// 这个 var 是保险标记（agentstatepkg 经由同包的 ctxWithState 间接使用）。
var _ = (*agentstatepkg.AgentState)(nil)

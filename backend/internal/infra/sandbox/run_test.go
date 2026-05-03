package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// ── buildDriver ───────────────────────────────────────────────────────────────

func TestBuildDriver_SubstitutesFuncName(t *testing.T) {
	got := buildDriver("parse_csv")
	if !strings.Contains(got, "parse_csv(**_input)") {
		t.Errorf("driver should call parse_csv(**_input), got:\n%s", got)
	}
	if strings.Contains(got, "{FUNC_NAME}") {
		t.Errorf("placeholder not replaced, got:\n%s", got)
	}
}

func TestBuildDriver_OnlyFirstPlaceholder(t *testing.T) {
	// Sanity: only one {FUNC_NAME} expected, but if someone adds more we want
	// to know the fix-once policy still holds. Substring count is a smoke test.
	got := buildDriver("foo")
	if strings.Count(got, "foo") != 1 {
		t.Errorf("driver should reference funcName exactly once, got:\n%s", got)
	}
}

// ── extractFuncName ───────────────────────────────────────────────────────────

func TestExtractFuncName_BasicForms(t *testing.T) {
	cases := []struct{ code, want string }{
		{"def parse_csv(text: str) -> list:\n    pass", "parse_csv"},
		{"def add(a, b):\n    return a+b", "add"},
		{"\n\ndef my_func() -> dict:\n    pass", "my_func"},
		{"# comment\ndef hello():\n    return 1", "hello"},
	}
	for _, c := range cases {
		got, err := extractFuncName(c.code)
		if err != nil {
			t.Errorf("extractFuncName(%q): unexpected error: %v", c.code[:min(20, len(c.code))], err)
			continue
		}
		if got != c.want {
			t.Errorf("extractFuncName: want %q, got %q", c.want, got)
		}
	}
}

func TestExtractFuncName_NoFunction(t *testing.T) {
	_, err := extractFuncName("x = 1\ny = 2")
	if err == nil {
		t.Error("expected error for code with no function definition")
	}
}

func TestExtractFuncName_FirstFunctionWins(t *testing.T) {
	code := `
def first():
    pass

def second():
    pass
`
	got, err := extractFuncName(code)
	if err != nil {
		t.Fatal(err)
	}
	if got != "first" {
		t.Errorf("first def should win, got %q", got)
	}
}

// ── writeAtomic ───────────────────────────────────────────────────────────────

func TestWriteAtomic_BasicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")

	if err := writeAtomic(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("writeAtomic err: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Errorf("content = %q, want %q", got, "hello")
	}

	// .tmp file should not linger after rename.
	// rename 后 .tmp 不该残留。
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf(".tmp should be cleaned up, stat err: %v", err)
	}
}

func TestWriteAtomic_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	os.WriteFile(path, []byte("old"), 0o644)

	if err := writeAtomic(path, []byte("new"), 0o644); err != nil {
		t.Fatalf("writeAtomic err: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}
}

// ── SyncError ─────────────────────────────────────────────────────────────────

func TestSyncError_ErrorReturnsStderr(t *testing.T) {
	cause := errors.New("exit code 1")
	se := &SyncError{Cause: cause, Stderr: "× No solution found"}
	if se.Error() != "× No solution found" {
		t.Errorf("Error() should return Stderr, got %q", se.Error())
	}
}

func TestSyncError_UnwrapToCause(t *testing.T) {
	cause := errors.New("exit code 1")
	se := &SyncError{Cause: cause, Stderr: "stderr text"}
	if !errors.Is(se, cause) {
		t.Errorf("errors.Is should walk to Cause via Unwrap")
	}
}

// ── Run / Sync / Destroy / DestroyEnv / WriteCodeFile guard ───────────────────

func TestRun_GuardsBeforeBootstrap(t *testing.T) {
	s := New(Config{DataDir: t.TempDir(), Logger: zap.NewNop()})
	_, err := s.Run(context.Background(), RunRequest{ForgeID: "f_x", VersionID: "fv_x", EnvID: "env_x", Code: "def f():\n    return 1"})
	if !errors.Is(err, errBootstrapPending) {
		t.Errorf("Run before Bootstrap should return errBootstrapPending, got %v", err)
	}
}

func TestSync_GuardsBeforeBootstrap(t *testing.T) {
	s := New(Config{DataDir: t.TempDir(), Logger: zap.NewNop()})
	err := s.Sync(context.Background(), SyncRequest{ForgeID: "f_x", VersionID: "fv_x", EnvID: "env_x"})
	if !errors.Is(err, errBootstrapPending) {
		t.Errorf("Sync before Bootstrap should return errBootstrapPending, got %v", err)
	}
}

func TestDestroy_GuardsBeforeBootstrap(t *testing.T) {
	s := New(Config{DataDir: t.TempDir(), Logger: zap.NewNop()})
	err := s.Destroy(context.Background(), "f_x")
	if !errors.Is(err, errBootstrapPending) {
		t.Errorf("Destroy before Bootstrap should return errBootstrapPending, got %v", err)
	}
}

func TestDestroyEnv_GuardsBeforeBootstrap(t *testing.T) {
	s := New(Config{DataDir: t.TempDir(), Logger: zap.NewNop()})
	err := s.DestroyEnv(context.Background(), "f_x", "env_x")
	if !errors.Is(err, errBootstrapPending) {
		t.Errorf("DestroyEnv before Bootstrap should return errBootstrapPending, got %v", err)
	}
}

func TestWriteCodeFile_GuardsBeforeBootstrap(t *testing.T) {
	s := New(Config{DataDir: t.TempDir(), Logger: zap.NewNop()})
	err := s.WriteCodeFile(context.Background(), "f_x", "fv_x", "def f():\n    return 1", "")
	if !errors.Is(err, errBootstrapPending) {
		t.Errorf("WriteCodeFile before Bootstrap should return errBootstrapPending, got %v", err)
	}
}

// ── WriteCodeFile (file IO works without uv/python) ───────────────────────────

func TestWriteCodeFile_WritesMainPyWithDriver(t *testing.T) {
	dir := t.TempDir()
	s := New(Config{DataDir: dir, Logger: zap.NewNop()})
	s.bootstrapped = true // skip Bootstrap requirements for file-only test

	code := `def hello(name):
    return f"hi {name}"`
	if err := s.WriteCodeFile(context.Background(), "f_x", "fv_x", code, "hello"); err != nil {
		t.Fatalf("WriteCodeFile err: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "forges", "f_x", "versions", "fv_x", "main.py"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "def hello(name):") {
		t.Errorf("main.py missing user code, got:\n%s", got)
	}
	if !strings.Contains(string(got), "hello(**_input)") {
		t.Errorf("main.py missing driver call, got:\n%s", got)
	}
}

func TestWriteCodeFile_FallsBackToExtractedFuncName(t *testing.T) {
	dir := t.TempDir()
	s := New(Config{DataDir: dir, Logger: zap.NewNop()})
	s.bootstrapped = true

	code := `def auto_extracted():
    return 42`
	// EntryFunction empty → sandbox falls back to extractFuncName.
	if err := s.WriteCodeFile(context.Background(), "f_x", "fv_y", code, ""); err != nil {
		t.Fatalf("WriteCodeFile err: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "forges", "f_x", "versions", "fv_y", "main.py"))
	if !strings.Contains(string(got), "auto_extracted(**_input)") {
		t.Errorf("driver should call auto_extracted, got:\n%s", got)
	}
}

func TestWriteCodeFile_ErrorsOnNoFunction(t *testing.T) {
	s := New(Config{DataDir: t.TempDir(), Logger: zap.NewNop()})
	s.bootstrapped = true
	err := s.WriteCodeFile(context.Background(), "f_x", "fv_x", "x = 1\ny = 2", "")
	if err == nil {
		t.Error("expected error when no function found and no entry hint")
	}
}

// ── Destroy / DestroyEnv (file IO works without uv/python) ────────────────────

func TestDestroy_RemovesForgeDir(t *testing.T) {
	dir := t.TempDir()
	s := New(Config{DataDir: dir, Logger: zap.NewNop()})
	s.bootstrapped = true

	// Set up some files under forges/f_x.
	target := filepath.Join(dir, "forges", "f_x", "envs", "env_a", "marker")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(target, []byte("x"), 0o644)

	if err := s.Destroy(context.Background(), "f_x"); err != nil {
		t.Fatalf("Destroy err: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "forges", "f_x")); !os.IsNotExist(err) {
		t.Errorf("forge dir should be gone, stat err: %v", err)
	}
	// Other forges untouched.
	otherTarget := filepath.Join(dir, "forges", "f_y", "marker")
	os.MkdirAll(filepath.Dir(otherTarget), 0o755)
	os.WriteFile(otherTarget, []byte("y"), 0o644)
	// Re-Destroy f_x and verify f_y still exists.
	s.Destroy(context.Background(), "f_x")
	if _, err := os.Stat(otherTarget); err != nil {
		t.Errorf("destroying f_x should not affect f_y, got: %v", err)
	}
}

func TestDestroyEnv_RemovesOnlyOneEnvDir(t *testing.T) {
	dir := t.TempDir()
	s := New(Config{DataDir: dir, Logger: zap.NewNop()})
	s.bootstrapped = true

	// Two env dirs under forges/f_x/envs.
	pathA := filepath.Join(dir, "forges", "f_x", "envs", "env_a", "marker")
	pathB := filepath.Join(dir, "forges", "f_x", "envs", "env_b", "marker")
	for _, p := range []string{pathA, pathB} {
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}

	if err := s.DestroyEnv(context.Background(), "f_x", "env_a"); err != nil {
		t.Fatalf("DestroyEnv err: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "forges", "f_x", "envs", "env_a")); !os.IsNotExist(err) {
		t.Errorf("env_a should be gone")
	}
	if _, err := os.Stat(pathB); err != nil {
		t.Errorf("env_b should still exist, stat err: %v", err)
	}
}

func TestDestroy_NonexistentForgeIsNotError(t *testing.T) {
	s := New(Config{DataDir: t.TempDir(), Logger: zap.NewNop()})
	s.bootstrapped = true

	// Destroying a forge that was never created should be a no-op (idempotent).
	if err := s.Destroy(context.Background(), "f_never_existed"); err != nil {
		t.Errorf("Destroy on non-existent forge should be no-op, got %v", err)
	}
}

// ── Run / Sync request shape sanity ───────────────────────────────────────────

func TestRunRequest_HasExpectedFields(t *testing.T) {
	// Compile-time check that RunRequest accepts the documented fields.
	// 编译期检查 RunRequest 接受文档约定的字段。
	_ = RunRequest{
		ForgeID:       "f_x",
		VersionID:     "fv_x",
		EnvID:         "env_x",
		Code:          "def f(): return 1",
		EntryFunction: "f",
		Input:         map[string]any{"x": 1},
	}
}

func TestSyncRequest_HasExpectedFields(t *testing.T) {
	_ = SyncRequest{
		ForgeID:       "f_x",
		VersionID:     "fv_x",
		EnvID:         "env_x",
		Dependencies:  []string{"pandas"},
		PythonVersion: ">=3.12",
		OnProgress:    func(stage, detail string) {},
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

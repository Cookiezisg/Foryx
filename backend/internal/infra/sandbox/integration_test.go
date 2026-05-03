// integration_test.go: end-to-end Sync / Run / Destroy tests that drive a
// real uv binary against a real Python interpreter. Gated by env vars per
// §T3 — skip silently when not set so unit-test runs / CI / offline work.
//
// To run locally:
//
//	export FORGIFY_TEST_UV=$(which uv)
//	export FORGIFY_TEST_PYTHON=$(which python3)
//	go test -count=1 ./internal/infra/sandbox/...
//
// (The realUVAndPython helper symlinks these into <dataDir>/bin/uv and
// <dataDir>/bin/python/bin/python3 so all path-derivation code in
// paths.go behaves as in production. Bootstrap itself is bypassed —
// it's tested separately via Bootstrap-specific tests when a full
// resource dir is provided.)
//
// integration_test.go：用真实 uv 二进制 + 真实 Python 解释器跑 Sync /
// Run / Destroy 端到端测试。按 §T3 用环境变量门控——未设时静默跳过，
// 不影响单元测试 / CI / 离线场景。
//
// 本地跑见上方英文示例。realUVAndPython helper 把系统工具 symlink 到
// `<dataDir>/bin/uv` 和 `<dataDir>/bin/python/bin/python3`，让 paths.go
// 的路径推导跟生产一致。Bootstrap 本身被绕过——它由独立的 Bootstrap
// 测试在提供完整资源目录时跑。

package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

// realUVAndPython prepares a Sandbox whose UVPath() and PythonPath()
// resolve to symlinks pointing at the system-installed uv + python
// (read from FORGIFY_TEST_UV / FORGIFY_TEST_PYTHON). Skips the test if
// either env var is unset.
//
// realUVAndPython 准备一个 Sandbox，UVPath() / PythonPath() 解析到指向
// 系统 uv + python 的 symlink（从 FORGIFY_TEST_UV / FORGIFY_TEST_PYTHON
// 读）。任一 env 缺失则 skip。
func realUVAndPython(t *testing.T) *Sandbox {
	t.Helper()
	uvSrc := os.Getenv("FORGIFY_TEST_UV")
	pySrc := os.Getenv("FORGIFY_TEST_PYTHON")
	if uvSrc == "" || pySrc == "" {
		t.Skip("FORGIFY_TEST_UV / FORGIFY_TEST_PYTHON not set; skipping integration test")
	}
	if runtime.GOOS == "windows" {
		t.Skip("integration tests use unix symlinks; windows path requires different setup")
	}

	dataDir := t.TempDir()
	s := New(Config{DataDir: dataDir, DefaultPython: ">=3.12", Logger: zap.NewNop()})

	// Symlink uv into the path bundledUVPath() returns.
	// 把 uv 链到 bundledUVPath() 的位置。
	if err := os.MkdirAll(filepath.Join(dataDir, "bin"), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.Symlink(uvSrc, s.UVPath()); err != nil {
		t.Fatalf("symlink uv: %v", err)
	}

	// Symlink python into the path bundledPythonPath() returns.
	// 把 python 链到 bundledPythonPath() 的位置。
	pyDest := s.PythonPath()
	if err := os.MkdirAll(filepath.Dir(pyDest), 0o755); err != nil {
		t.Fatalf("mkdir python parent: %v", err)
	}
	if err := os.Symlink(pySrc, pyDest); err != nil {
		t.Fatalf("symlink python: %v", err)
	}

	// Need uv-cache too — withUVEnv sets UV_CACHE_DIR, uv expects it writable.
	// uv-cache 也要——withUVEnv 设 UV_CACHE_DIR，uv 期望可写。
	if err := os.MkdirAll(uvCacheDir(dataDir), 0o755); err != nil {
		t.Fatalf("mkdir uv-cache: %v", err)
	}

	s.bootstrapped = true
	return s
}

// ── Sync ──────────────────────────────────────────────────────────────────────

func TestSync_StdlibOnlyNoDeps(t *testing.T) {
	s := realUVAndPython(t)

	envID := ComputeEnvID(nil, ">=3.12")
	stages := make([]string, 0)

	err := s.Sync(context.Background(), SyncRequest{
		ForgeID:       "f_stdlib",
		VersionID:     "fv_a",
		EnvID:         envID,
		PythonVersion: ">=3.12",
		OnProgress: func(stage, _ string) {
			stages = append(stages, stage)
		},
	})
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	venvPath := filepath.Join(envDir(s.cfg.DataDir, "f_stdlib", envID), ".venv")
	if _, err := os.Stat(venvPath); err != nil {
		t.Errorf(".venv not created at %s: %v", venvPath, err)
	}
	// Should hit at least Resolved + Installed for the project itself.
	// 至少触发 Resolved + Installed（项目本身）。
	if len(stages) == 0 {
		t.Errorf("expected progress callbacks, got none — stages=%v", stages)
	}
}

func TestSync_IdempotentSkipsExisting(t *testing.T) {
	s := realUVAndPython(t)
	envID := ComputeEnvID(nil, ">=3.12")
	req := SyncRequest{
		ForgeID:       "f_idem",
		VersionID:     "fv_x",
		EnvID:         envID,
		PythonVersion: ">=3.12",
	}

	if err := s.Sync(context.Background(), req); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Second call should skip via stat check — bound elapsed time tightly.
	// 第二次应靠 stat 跳过——elapsed 时间应很短。
	start := time.Now()
	if err := s.Sync(context.Background(), req); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("second sync should skip, took %v", elapsed)
	}
}

func TestSync_BadDependencyFailsWithStderr(t *testing.T) {
	s := realUVAndPython(t)

	deps := []string{"this-package-totally-does-not-exist-xyz123"}
	envID := ComputeEnvID(deps, ">=3.12")

	err := s.Sync(context.Background(), SyncRequest{
		ForgeID:       "f_bad",
		VersionID:     "fv_bad",
		EnvID:         envID,
		Dependencies:  deps,
		PythonVersion: ">=3.12",
	})
	if err == nil {
		t.Fatal("expected sync to fail for nonexistent package")
	}

	se, ok := err.(*SyncError)
	if !ok {
		t.Fatalf("expected *SyncError, got %T: %v", err, err)
	}
	if se.Stderr == "" {
		t.Errorf("SyncError.Stderr should contain uv error message; got empty")
	}
	// uv typically reports "No solution found" or similar for unresolvable deps.
	// uv 对解析不出的依赖通常报 "No solution found" 之类。
	t.Logf("SyncError stderr (sample): %s", se.Stderr)
}

// ── Run ───────────────────────────────────────────────────────────────────────

// syncedSandbox returns a sandbox with a stdlib-only env already synced for
// the given forge/version.
//
// syncedSandbox 返回一个已为指定 forge/version 完成 stdlib 环境 sync 的
// sandbox。
func syncedSandbox(t *testing.T, forgeID, versionID string) (*Sandbox, string) {
	t.Helper()
	s := realUVAndPython(t)
	envID := ComputeEnvID(nil, ">=3.12")
	if err := s.Sync(context.Background(), SyncRequest{
		ForgeID:       forgeID,
		VersionID:     versionID,
		EnvID:         envID,
		PythonVersion: ">=3.12",
	}); err != nil {
		t.Fatalf("sync setup: %v", err)
	}
	return s, envID
}

func TestRun_BasicExecution(t *testing.T) {
	s, envID := syncedSandbox(t, "f_run", "fv_run")

	code := `def add(a, b):
    return a + b
`
	result, err := s.Run(context.Background(), RunRequest{
		ForgeID:   "f_run",
		VersionID: "fv_run",
		EnvID:     envID,
		Code:      code,
		Input:     map[string]any{"a": 2, "b": 3},
	})
	if err != nil {
		t.Fatalf("Run unexpected error: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected ok=true, got error: %s", result.ErrorMsg)
	}
	// JSON int → float64.
	got, ok := result.Output.(float64)
	if !ok {
		t.Fatalf("expected float64 output, got %T (%v)", result.Output, result.Output)
	}
	if got != 5 {
		t.Errorf("expected 5, got %v", got)
	}
}

func TestRun_StringOutput(t *testing.T) {
	s, envID := syncedSandbox(t, "f_str", "fv_str")

	code := `def greet(name):
    return f"hi {name}"
`
	result, err := s.Run(context.Background(), RunRequest{
		ForgeID:   "f_str",
		VersionID: "fv_str",
		EnvID:     envID,
		Code:      code,
		Input:     map[string]any{"name": "world"},
	})
	if err != nil {
		t.Fatalf("Run err: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected ok=true, got: %s", result.ErrorMsg)
	}
	if result.Output.(string) != "hi world" {
		t.Errorf("expected 'hi world', got %v", result.Output)
	}
}

func TestRun_DefaultArgument(t *testing.T) {
	s, envID := syncedSandbox(t, "f_def", "fv_def")

	code := `def repeat(text, times=2):
    return text * times
`
	result, err := s.Run(context.Background(), RunRequest{
		ForgeID:   "f_def",
		VersionID: "fv_def",
		EnvID:     envID,
		Code:      code,
		Input:     map[string]any{"text": "ab"},
	})
	if err != nil {
		t.Fatalf("Run err: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected ok=true, got: %s", result.ErrorMsg)
	}
	if result.Output.(string) != "abab" {
		t.Errorf("expected 'abab', got %v", result.Output)
	}
}

func TestRun_PythonExceptionReturnsOKFalse(t *testing.T) {
	s, envID := syncedSandbox(t, "f_exc", "fv_exc")

	code := `def divide(a, b):
    return a / b
`
	result, err := s.Run(context.Background(), RunRequest{
		ForgeID:   "f_exc",
		VersionID: "fv_exc",
		EnvID:     envID,
		Code:      code,
		Input:     map[string]any{"a": 1, "b": 0},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.OK {
		t.Errorf("expected ok=false for division by zero")
	}
	if !strings.Contains(result.ErrorMsg, "ZeroDivision") && !strings.Contains(result.ErrorMsg, "division by zero") {
		t.Errorf("error msg should mention division error, got: %s", result.ErrorMsg)
	}
}

func TestRun_ContextCancelKillsProcessTree(t *testing.T) {
	s, envID := syncedSandbox(t, "f_cancel", "fv_cancel")

	// Spawn a child that itself sleeps. ctx-cancel must kill the whole
	// tree (Python parent + sub-Python via subprocess.run); otherwise
	// elapsed time tail is huge.
	//
	// 起一个 fork 子进程 sleep。ctx-cancel 必须杀整个进程树
	// （Python 父 + subprocess.run 起的子 Python），否则 elapsed 会很长。
	code := `def slow():
    import subprocess, sys
    subprocess.run([sys.executable, "-c", "import time; time.sleep(10)"])
    return "done"
`
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	result, err := s.Run(ctx, RunRequest{
		ForgeID:   "f_cancel",
		VersionID: "fv_cancel",
		EnvID:     envID,
		Code:      code,
		Input:     map[string]any{},
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.OK {
		t.Errorf("expected ok=false when cancelled")
	}
	// Should be near 300ms; allow up to 3s for kill propagation.
	// 应接近 300ms；给 3s 给 kill 传播。
	if elapsed > 3*time.Second {
		t.Errorf("ctx-cancel took too long (%v), tree may not have been killed", elapsed)
	}
}

// TestRun_MissingEnvFallsBackForStdlib documents an important uv 0.11 behavior:
// `uv run --no-sync` with no .venv falls back to the UV_PYTHON interpreter
// (bundled Python) directly. So stdlib-only code runs fine even without a
// synced venv — the LLM's punt-to-AI flow only fails when forge code
// actually tries to import a non-stdlib package.
//
// TestRun_MissingEnvFallsBackForStdlib 记录一个关键的 uv 0.11 行为：
// `uv run --no-sync` 在无 .venv 时回退到 UV_PYTHON 解释器（捆绑 Python）
// 直接跑。所以仅 stdlib 代码即使没 sync 过也能正常运行——LLM "punt 给 AI"
// 流程只在 forge 代码尝试 import 非 stdlib 包时才失败。
func TestRun_MissingEnvFallsBackForStdlib(t *testing.T) {
	s := realUVAndPython(t)

	code := `def f():
    return 1
`
	result, err := s.Run(context.Background(), RunRequest{
		ForgeID:   "f_no_env",
		VersionID: "fv_no_env",
		EnvID:     "env_doesnotexist",
		Code:      code,
		Input:     map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	// Stdlib-only forge succeeds even without venv — uv falls back to UV_PYTHON.
	// This is acceptable: the LLM doesn't need to resync if the code works.
	if !result.OK {
		t.Errorf("stdlib-only forge should succeed via uv fallback even with missing env, got ErrorMsg=%q", result.ErrorMsg)
	}
}

// TestRun_MissingEnvFailsWhenCodeImportsThirdParty asserts the natural error
// path that punt-to-AI relies on: when a forge needs a non-stdlib package
// and the venv is missing (evicted), uv's fallback Python doesn't have the
// package, so the import fails with an ImportError that surfaces to the
// LLM via tool_result.
//
// TestRun_MissingEnvFailsWhenCodeImportsThirdParty 验证 punt-to-AI 依赖
// 的自然错误路径：当 forge 需要非 stdlib 包但 venv 不存在（被驱逐），
// uv 回退的 Python 没有该包，import 失败抛 ImportError，通过 tool_result
// 透传给 LLM。
func TestRun_MissingEnvFailsWhenCodeImportsThirdParty(t *testing.T) {
	s := realUVAndPython(t)

	// import a clearly-not-stdlib package; must fail at import time.
	// import 一个明显非 stdlib 包；必然在 import 阶段失败。
	code := `def f():
    import this_package_will_never_exist_xyz123
    return 1
`
	result, err := s.Run(context.Background(), RunRequest{
		ForgeID:   "f_no_env_3rdparty",
		VersionID: "fv_x",
		EnvID:     "env_doesnotexist",
		Code:      code,
		Input:     map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.OK {
		t.Errorf("expected ok=false when forge imports unavailable package")
	}
	if !strings.Contains(result.ErrorMsg, "ModuleNotFoundError") &&
		!strings.Contains(result.ErrorMsg, "No module named") &&
		!strings.Contains(result.ErrorMsg, "ImportError") {
		t.Errorf("expected ImportError-ish stderr, got: %s", result.ErrorMsg)
	}
}

// ── Destroy / DestroyEnv with real synced state ───────────────────────────────

func TestDestroy_AfterSyncRemovesEverything(t *testing.T) {
	s, _ := syncedSandbox(t, "f_destroy", "fv_d")

	if _, err := os.Stat(forgeDir(s.cfg.DataDir, "f_destroy")); err != nil {
		t.Fatalf("forge dir should exist after sync: %v", err)
	}

	if err := s.Destroy(context.Background(), "f_destroy"); err != nil {
		t.Fatalf("Destroy err: %v", err)
	}

	if _, err := os.Stat(forgeDir(s.cfg.DataDir, "f_destroy")); !os.IsNotExist(err) {
		t.Errorf("forge dir should be gone after Destroy")
	}
}

func TestDestroyEnv_KeepsOtherEnvs(t *testing.T) {
	s := realUVAndPython(t)

	envA := ComputeEnvID([]string{}, ">=3.12")
	// Use an env spec just different enough to produce a distinct EnvID.
	// 用一个刚好不同的 spec 制造另一个 EnvID。
	envB := ComputeEnvID([]string{}, ">=3.13")

	for _, eid := range []string{envA, envB} {
		if err := s.Sync(context.Background(), SyncRequest{
			ForgeID:       "f_destroy_env",
			VersionID:     "fv_x",
			EnvID:         eid,
			PythonVersion: ">=3.12", // both syncs use the same actual Python
		}); err != nil {
			t.Fatalf("sync %s: %v", eid, err)
		}
	}

	if err := s.DestroyEnv(context.Background(), "f_destroy_env", envA); err != nil {
		t.Fatalf("DestroyEnv: %v", err)
	}
	if _, err := os.Stat(envDir(s.cfg.DataDir, "f_destroy_env", envA)); !os.IsNotExist(err) {
		t.Errorf("envA should be gone")
	}
	if _, err := os.Stat(envDir(s.cfg.DataDir, "f_destroy_env", envB)); err != nil {
		t.Errorf("envB should remain: %v", err)
	}
}

// ── WriteCodeFile + Run without re-sync ───────────────────────────────────────

func TestWriteCodeFile_ThenRun(t *testing.T) {
	s, envID := syncedSandbox(t, "f_write", "fv_w")

	// Write code via WriteCodeFile (no sync re-trigger).
	// 通过 WriteCodeFile 写代码（不再触发 sync）。
	code := `def double(x):
    return x * 2
`
	if err := s.WriteCodeFile(context.Background(), "f_write", "fv_w", code, "double"); err != nil {
		t.Fatalf("WriteCodeFile: %v", err)
	}

	// Run uses the same EnvID; venv already there from syncedSandbox.
	// Run 用同一个 EnvID；venv 已被 syncedSandbox sync 好。
	result, err := s.Run(context.Background(), RunRequest{
		ForgeID:       "f_write",
		VersionID:     "fv_w",
		EnvID:         envID,
		Code:          code,
		EntryFunction: "double",
		Input:         map[string]any{"x": 7},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected ok=true, got: %s", result.ErrorMsg)
	}
	if got := result.Output.(float64); got != 14 {
		t.Errorf("expected 14, got %v", got)
	}
}

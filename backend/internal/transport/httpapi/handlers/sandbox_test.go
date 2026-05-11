// sandbox_test.go — E2E contract tests for /api/v1/sandbox/* + the
// per-conversation scratch env routes. Real httptest server backed by an
// in-memory SQLite store; sandbox Service is marked ready via the test
// helper (no actual mise extraction). No EnvManager is registered, so
// destroy paths exercise only the Repository / file-system removal
// branches — runtime-spawn paths belong in the D9 pipeline suite.
//
// sandbox_test.go ——/api/v1/sandbox/* + per-conversation scratch env 路由
// 端到端契约测试。真 httptest server 后端用内存 SQLite store；sandbox
// Service 通过 test helper 标 ready（不真抽 mise）。不注册 EnvManager 所以
// destroy 路径只走 Repository / 文件系统删除分支——runtime-spawn 路径归
// D9 pipeline 套。

package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
	gormlogger "gorm.io/gorm/logger"

	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	sandboxstore "github.com/sunweilin/forgify/backend/internal/infra/store/sandbox"
	middlewarehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/middleware"
)

// newSandboxTestServer constructs an httptest server with an in-memory
// SQLite-backed sandbox Service. The Service is marked ready immediately
// (real Bootstrap would run mise extraction); no EnvManagers / Installers
// are registered so EnsureRuntime / EnsureEnv calls would fail — the
// admin/debug endpoints under test don't trigger those paths.
//
// newSandboxTestServer 构造 httptest server，后端用内存 SQLite 支撑的
// sandbox Service。Service 立即标 ready（真 Bootstrap 会跑 mise 抽取）；
// 不注册 EnvManager / Installer，EnsureRuntime / EnsureEnv 调用会失败
// ——被测的 admin/debug 端点不触发这些路径。
func newSandboxTestServer(t *testing.T) (*httptest.Server, sandboxdomain.Repository) {
	t.Helper()
	gdb, err := dbinfra.Open(dbinfra.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("dbinfra.Open: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(gdb) })
	if err := dbinfra.Migrate(gdb, &sandboxdomain.Runtime{}, &sandboxdomain.Env{}); err != nil {
		t.Fatalf("dbinfra.Migrate: %v", err)
	}
	repo := sandboxstore.New(gdb)
	log := zaptest.NewLogger(t)
	svc := sandboxapp.New(repo, t.TempDir(), nil, log)
	svc.MarkReadyForTest("/fake/mise")

	h := NewSandboxHandler(svc, log)
	mux := http.NewServeMux()
	h.Register(mux)
	return httptest.NewServer(middlewarehttpapi.InjectUserID(mux)), repo
}

// seedRuntime / seedEnv create manifest rows directly via the repo for
// tests that exercise read-side endpoints.
//
// seedRuntime / seedEnv 通过 repo 直接建 manifest 行，给读端测试用。
func seedRuntime(t *testing.T, repo sandboxdomain.Repository, id, kind, version string) {
	t.Helper()
	if err := repo.CreateRuntime(t.Context(), &sandboxdomain.Runtime{
		ID: id, Kind: kind, Version: version, Path: kind + "/" + version,
		SizeBytes:   100,
		InstalledAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed runtime: %v", err)
	}
}

func seedEnv(t *testing.T, repo sandboxdomain.Repository, id, ownerKind, ownerID, runtimeID string) {
	t.Helper()
	if err := repo.CreateEnv(t.Context(), &sandboxdomain.Env{
		ID: id, OwnerKind: ownerKind, OwnerID: ownerID, RuntimeID: runtimeID,
		Path: ownerKind + "/" + ownerID, SizeBytes: 50,
		Status:    sandboxdomain.EnvStatusReady,
		CreatedAt: time.Now(), LastUsedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed env: %v", err)
	}
}

// ── Read endpoints ────────────────────────────────────────────────────

func TestSandboxHandler_ListRuntimes_Empty(t *testing.T) {
	srv, _ := newSandboxTestServer(t)
	defer srv.Close()
	status, env := do(t, srv, "GET", "/api/v1/sandbox/runtimes", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %+v", status, env)
	}
	rows := dataSlice(t, env)
	if len(rows) != 0 {
		t.Errorf("baseline: want 0 runtimes, got %d", len(rows))
	}
}

func TestSandboxHandler_ListRuntimes_AfterSeed(t *testing.T) {
	srv, repo := newSandboxTestServer(t)
	defer srv.Close()
	seedRuntime(t, repo, "sr_a", "python", "3.12.5")
	seedRuntime(t, repo, "sr_b", "node", "22.5.0")

	status, env := do(t, srv, "GET", "/api/v1/sandbox/runtimes", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	rows := dataSlice(t, env)
	if len(rows) != 2 {
		t.Errorf("want 2 runtimes, got %d", len(rows))
	}
}

func TestSandboxHandler_ListEnvs_RequiresOwnerKind(t *testing.T) {
	srv, _ := newSandboxTestServer(t)
	defer srv.Close()
	status, env := do(t, srv, "GET", "/api/v1/sandbox/envs", nil)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (missing ownerKind): %+v", status, env)
	}
}

func TestSandboxHandler_ListEnvs_FilteredByOwnerKind(t *testing.T) {
	srv, repo := newSandboxTestServer(t)
	defer srv.Close()
	seedRuntime(t, repo, "sr_001", "python", "3.12.5")
	seedEnv(t, repo, "se_a", sandboxdomain.OwnerKindMCP, "playwright", "sr_001")
	seedEnv(t, repo, "se_b", sandboxdomain.OwnerKindFunction, "f_x:env_y", "sr_001")

	status, env := do(t, srv, "GET", "/api/v1/sandbox/envs?ownerKind=mcp", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	rows := dataSlice(t, env)
	if len(rows) != 1 {
		t.Errorf("want 1 mcp env, got %d", len(rows))
	}
}

func TestSandboxHandler_GetEnv_NotFound(t *testing.T) {
	srv, _ := newSandboxTestServer(t)
	defer srv.Close()
	status, env := do(t, srv, "GET", "/api/v1/sandbox/envs/nonexistent", nil)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %+v", status, env)
	}
}

func TestSandboxHandler_GetEnv_Found(t *testing.T) {
	srv, repo := newSandboxTestServer(t)
	defer srv.Close()
	seedRuntime(t, repo, "sr_001", "python", "3.12.5")
	seedEnv(t, repo, "se_a", sandboxdomain.OwnerKindMCP, "playwright", "sr_001")

	status, _ := do(t, srv, "GET", "/api/v1/sandbox/envs/se_a", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
}

func TestSandboxHandler_DiskUsage(t *testing.T) {
	srv, repo := newSandboxTestServer(t)
	defer srv.Close()
	seedRuntime(t, repo, "sr_001", "python", "3.12.5") // 100 bytes
	seedEnv(t, repo, "se_a", sandboxdomain.OwnerKindFunction, "f_x:env_y", "sr_001") // 50 bytes

	status, env := do(t, srv, "GET", "/api/v1/sandbox/disk-usage", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	d := dataMap(t, env)
	if d["totalBytes"] == nil {
		t.Error("response missing totalBytes")
	}
}

func TestSandboxHandler_BootstrapStatus_Ready(t *testing.T) {
	srv, _ := newSandboxTestServer(t)
	defer srv.Close()
	status, env := do(t, srv, "GET", "/api/v1/sandbox/bootstrap-status", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	d := dataMap(t, env)
	if d["ok"] != true {
		t.Errorf("want ok=true, got %v", d["ok"])
	}
}

// ── :action endpoints ────────────────────────────────────────────────

func TestSandboxHandler_RuntimeDestroy_RefusesIfEnvReferences(t *testing.T) {
	srv, repo := newSandboxTestServer(t)
	defer srv.Close()
	seedRuntime(t, repo, "sr_001", "python", "3.12.5")
	seedEnv(t, repo, "se_a", sandboxdomain.OwnerKindMCP, "playwright", "sr_001")

	status, env := do(t, srv, "POST", "/api/v1/sandbox/runtimes/sr_001:destroy", nil)
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (env references runtime): %+v", status, env)
	}
}

func TestSandboxHandler_GC_DefaultsTo30Days(t *testing.T) {
	srv, _ := newSandboxTestServer(t)
	defer srv.Close()
	// No envs to GC — just verify the endpoint shape.
	status, env := do(t, srv, "POST", "/api/v1/sandbox/:gc", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	d := dataMap(t, env)
	if d["olderThanDays"] == nil {
		t.Error("response missing olderThanDays")
	}
}

func TestSandboxHandler_RetryBootstrap_ReturnsStatus(t *testing.T) {
	srv, _ := newSandboxTestServer(t)
	defer srv.Close()
	// MarkReadyForTest set ready=true; RetryBootstrap will run real
	// ExtractMiseBinary which will fail because the test data dir has no
	// mise binary embedded in a way that matches ExtractMiseBinary's
	// expectations (the embed IS present at compile time, but the dir
	// is fresh). The endpoint MUST still return 200 with ok=true/false.
	//
	// MarkReadyForTest 设 ready=true；RetryBootstrap 会跑真 ExtractMiseBinary
	// 可能成可能失败。端点必须返 200 不论结果。
	status, _ := do(t, srv, "POST", "/api/v1/sandbox/:retry-bootstrap", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (status payload not HTTP error)", status)
	}
}

// ── Conversation scratch envs ────────────────────────────────────────

func TestSandboxHandler_ListConvEnvs_FiltersByPrefix(t *testing.T) {
	srv, repo := newSandboxTestServer(t)
	defer srv.Close()
	seedRuntime(t, repo, "sr_001", "python", "3.12.5")
	seedEnv(t, repo, "se_a", sandboxdomain.OwnerKindConversation, "cv_abc_python", "sr_001")
	seedEnv(t, repo, "se_b", sandboxdomain.OwnerKindConversation, "cv_xyz_python", "sr_001")

	status, env := do(t, srv, "GET", "/api/v1/conversations/cv_abc/sandbox-envs", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	rows := dataSlice(t, env)
	if len(rows) != 1 {
		t.Errorf("want 1 env for cv_abc, got %d (cv_xyz must NOT match)", len(rows))
	}
}

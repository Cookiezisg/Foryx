// spawn_test.go — end-to-end tests for Service.Spawn / SpawnLongLived /
// Shutdown. Uses an in-memory SQLite store and a fake EnvManager that
// resolves bin names via $PATH (echo / cat / sleep) so the suite stays
// portable. Real EnvManager + real mise spawn is exercised in the D9
// pipeline suite.
//
// spawn_test.go ——Service.Spawn / SpawnLongLived / Shutdown 端到端测试。
// 用内存 SQLite store + fake EnvManager 通过 $PATH 解析 bin 名（echo /
// cat / sleep）保持可移植。真 EnvManager + 真 mise spawn 在 D9 pipeline
// 套覆盖。

package sandbox

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	gormlogger "gorm.io/gorm/logger"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	sandboxstore "github.com/sunweilin/forgify/backend/internal/infra/store/sandbox"
)

// fakeEnvManager satisfies sandboxdomain.EnvManager by resolving binary
// names via $PATH (so tests can spawn echo / cat / sleep without a real
// mise install). Kind() returns the construction-time tag so multiple
// fake managers can coexist.
//
// fakeEnvManager 通过 $PATH 解析 binary 名满足 sandboxdomain.EnvManager
// （让测试不真起 mise 就能 spawn echo / cat / sleep）。Kind() 返构造时
// tag 让多 fake manager 共存。
type fakeEnvManager struct{ kind string }

func (f fakeEnvManager) Kind() string                                            { return f.kind }
func (fakeEnvManager) CreateEnv(context.Context, string, string) error           { return nil }
func (fakeEnvManager) InstallDeps(context.Context, string, string, []string, sandboxdomain.ProgressFunc) error {
	return nil
}
func (fakeEnvManager) EnvBin(_ string, binName string) string {
	if p, err := exec.LookPath(binName); err == nil {
		return p
	}
	return binName
}
func (fakeEnvManager) EnvDir(envPath string) string { return envPath }

// newServiceWithEnv builds a Service backed by in-memory SQLite, marks
// it ready, registers a fake EnvManager for the given runtime kind, and
// pre-seeds a Runtime + Env row so Spawn can resolve owner → env. Returns
// the service + the seeded owner.
//
// newServiceWithEnv 起内存 SQLite 支持的 Service，标 ready，给指定 runtime
// kind 注册 fake EnvManager，预填 Runtime + Env 行让 Spawn 能解析 owner →
// env。返 service + 预填的 owner。
func newServiceWithEnv(t *testing.T, kind string) (*Service, sandboxdomain.Owner) {
	t.Helper()
	db, err := dbinfra.Open(dbinfra.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(db) })
	if err := dbinfra.Migrate(db, &sandboxdomain.Runtime{}, &sandboxdomain.Env{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := sandboxstore.New(db)
	svc := New(repo, t.TempDir(), nil, zap.NewNop())
	svc.MarkReadyForTest("/fake/mise")
	svc.RegisterEnvManager(fakeEnvManager{kind: kind})

	ctx := context.Background()
	rt := &sandboxdomain.Runtime{
		ID: "sr_test", Kind: kind, Version: "1.0",
		Path:        "fake/" + kind + "/1.0",
		InstalledAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := repo.CreateRuntime(ctx, rt); err != nil {
		t.Fatalf("seed runtime: %v", err)
	}
	owner := sandboxdomain.Owner{Kind: sandboxdomain.OwnerKindFunction, ID: "f_test"}
	envRel := "envs/forge/f_test"
	env := &sandboxdomain.Env{
		ID: "se_test", OwnerKind: owner.Kind, OwnerID: owner.ID,
		RuntimeID: rt.ID, Path: envRel,
		Status: sandboxdomain.EnvStatusReady,
		CreatedAt: time.Now(), LastUsedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := repo.CreateEnv(ctx, env); err != nil {
		t.Fatalf("seed env: %v", err)
	}
	// Production EnsureEnv mkdirs envPath; test bypasses EnsureEnv, so do
	// it here — exec.Cmd.Dir on a missing directory produces a misleading
	// "no such file or directory" error attributed to the binary.
	//
	// 生产 EnsureEnv mkdir envPath；测试绕过 EnsureEnv 这里手动 mkdir——
	// exec.Cmd.Dir 设不存在目录会报"no such file or directory"误归到 binary。
	if err := os.MkdirAll(filepath.Join(svc.SandboxRoot(), envRel), 0o755); err != nil {
		t.Fatalf("mkdir env path: %v", err)
	}
	return svc, owner
}

// ── Spawn ─────────────────────────────────────────────────────────────

func TestServiceSpawn_HappyPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses 'echo' via PATH; D14 Windows pipeline covers spawn separately")
	}
	svc, owner := newServiceWithEnv(t, "fake-py")
	res, err := svc.Spawn(context.Background(), owner, sandboxdomain.SpawnOpts{
		Cmd:  "echo",
		Args: []string{"hello service"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if !res.Ok {
		t.Errorf("Ok = false (exit %d, stderr %q)", res.ExitCode, res.Stderr)
	}
	if got := strings.TrimSpace(string(res.Stdout)); got != "hello service" {
		t.Errorf("stdout = %q, want %q", got, "hello service")
	}
}

func TestServiceSpawn_NotReady_Errors(t *testing.T) {
	svc, owner := newServiceWithEnv(t, "fake-py")
	svc.bootstrapped.Store(false) // simulate degraded mode
	_, err := svc.Spawn(context.Background(), owner, sandboxdomain.SpawnOpts{Cmd: "echo"})
	if err == nil {
		t.Fatal("want error in degraded mode, got nil")
	}
	if !errors.Is(err, sandboxdomain.ErrSpawnFailed) {
		t.Errorf("err must wrap ErrSpawnFailed, got %v", err)
	}
}

func TestServiceSpawn_EmptyCmd_Errors(t *testing.T) {
	svc, owner := newServiceWithEnv(t, "fake-py")
	_, err := svc.Spawn(context.Background(), owner, sandboxdomain.SpawnOpts{})
	if err == nil {
		t.Fatal("want error for empty cmd, got nil")
	}
}

func TestServiceSpawn_OwnerMismatch_Errors(t *testing.T) {
	svc, _ := newServiceWithEnv(t, "fake-py")
	_, err := svc.Spawn(context.Background(),
		sandboxdomain.Owner{Kind: sandboxdomain.OwnerKindMCP, ID: "nonexistent"},
		sandboxdomain.SpawnOpts{Cmd: "echo"})
	if err == nil {
		t.Fatal("want error for nonexistent env, got nil")
	}
	if !errors.Is(err, sandboxdomain.ErrEnvNotFound) {
		t.Errorf("err must wrap ErrEnvNotFound, got %v", err)
	}
}

func TestServiceSpawn_AbsoluteCmd_BypassesEnvBin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/echo")
	}
	svc, owner := newServiceWithEnv(t, "fake-py")
	echoBin, err := exec.LookPath("echo")
	if err != nil {
		t.Fatalf("look up echo: %v", err)
	}
	// Pass absolute path; resolveCmd should skip EnvBin lookup and use it directly.
	// 传绝对路径；resolveCmd 跳过 EnvBin 直接用。
	res, err := svc.Spawn(context.Background(), owner, sandboxdomain.SpawnOpts{
		Cmd:  echoBin,
		Args: []string{"absolute"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if got := strings.TrimSpace(string(res.Stdout)); got != "absolute" {
		t.Errorf("stdout = %q, want %q", got, "absolute")
	}
}

func TestServiceSpawn_EnvOverlay(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh + env command")
	}
	svc, owner := newServiceWithEnv(t, "fake-py")
	// Use sh -c to print the var so we exercise mergeEnv end-to-end.
	// 用 sh -c 打印变量端到端验证 mergeEnv。
	shBin, err := exec.LookPath("sh")
	if err != nil {
		t.Fatalf("look up sh: %v", err)
	}
	res, err := svc.Spawn(context.Background(), owner, sandboxdomain.SpawnOpts{
		Cmd:  shBin,
		Args: []string{"-c", "echo $FORGIFY_TEST_VAR"},
		Env:  map[string]string{"FORGIFY_TEST_VAR": "overlay-works"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if got := strings.TrimSpace(string(res.Stdout)); got != "overlay-works" {
		t.Errorf("env overlay broken: stdout = %q, want overlay-works", got)
	}
}

// ── SpawnLongLived ───────────────────────────────────────────────────

func TestServiceSpawnLongLived_RegistersHandle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses cat")
	}
	svc, owner := newServiceWithEnv(t, "fake-py")
	if c := svc.ActiveHandleCountForTest(); c != 0 {
		t.Errorf("baseline handle count = %d, want 0", c)
	}

	handle, err := svc.SpawnLongLived(context.Background(), owner, sandboxdomain.SpawnOpts{
		Cmd: "cat",
	})
	if err != nil {
		t.Fatalf("SpawnLongLived: %v", err)
	}
	if c := svc.ActiveHandleCountForTest(); c != 1 {
		t.Errorf("after SpawnLongLived count = %d, want 1", c)
	}

	// Close stdin → cat exits → Wait returns → handle un-registers.
	// 关 stdin → cat 退 → Wait 返 → handle 反注册。
	_ = handle.Stdin().Close()
	_, _ = io.Copy(io.Discard, handle.Stdout())
	if err := handle.Wait(); err != nil {
		t.Errorf("Wait: %v", err)
	}
	if c := svc.ActiveHandleCountForTest(); c != 0 {
		t.Errorf("after Wait count = %d, want 0 (handle should un-register)", c)
	}
}

func TestServiceSpawnLongLived_KillUnregisters(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sleep")
	}
	svc, owner := newServiceWithEnv(t, "fake-py")
	handle, err := svc.SpawnLongLived(context.Background(), owner, sandboxdomain.SpawnOpts{
		Cmd: "sleep", Args: []string{"30"},
	})
	if err != nil {
		t.Fatalf("SpawnLongLived: %v", err)
	}
	if c := svc.ActiveHandleCountForTest(); c != 1 {
		t.Errorf("after spawn count = %d, want 1", c)
	}
	if err := handle.Kill(); err != nil {
		t.Errorf("Kill: %v", err)
	}
	if c := svc.ActiveHandleCountForTest(); c != 0 {
		t.Errorf("after Kill count = %d, want 0", c)
	}
	_ = handle.Wait() // reap
}

// ── Shutdown ──────────────────────────────────────────────────────────

func TestServiceShutdown_KillsAllActiveHandles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sleep")
	}
	svc, owner := newServiceWithEnv(t, "fake-py")

	const n = 3
	handles := make([]sandboxdomain.LongLivedHandle, n)
	for i := 0; i < n; i++ {
		h, err := svc.SpawnLongLived(context.Background(), owner, sandboxdomain.SpawnOpts{
			Cmd: "sleep", Args: []string{"30"},
		})
		if err != nil {
			t.Fatalf("SpawnLongLived[%d]: %v", i, err)
		}
		handles[i] = h
	}
	if c := svc.ActiveHandleCountForTest(); c != n {
		t.Errorf("after spawning %d handles count = %d", n, c)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := svc.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown: %v", err)
	}

	// All handles should be killed; reap them so the test doesn't leak.
	// 所有 handle 应被杀；reap 防测试 leak。
	for _, h := range handles {
		_ = h.Wait()
	}
}

func TestServiceShutdown_NoActiveHandles_Succeeds(t *testing.T) {
	svc, _ := newServiceWithEnv(t, "fake-py")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown with no active handles: %v", err)
	}
}

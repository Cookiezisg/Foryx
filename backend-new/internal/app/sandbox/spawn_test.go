package sandbox

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"go.uber.org/zap"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	sandboxstore "github.com/sunweilin/forgify/backend/internal/infra/store/sandbox"
)

// ---- shared test doubles ----

// fakeEnvManager resolves a bare cmd to a real binary on PATH (so spawn can run
// echo/cat/sleep), with cwd = envPath and args passed through.
//
// fakeEnvManager 把裸 cmd 解析为 PATH 上的真 binary（让 spawn 能跑 echo/cat/sleep），
// cwd = envPath，args 透传。
type fakeEnvManager struct{ kind string }

func (f fakeEnvManager) Kind() string                                  { return f.kind }
func (fakeEnvManager) CreateEnv(context.Context, string, string) error { return nil }
func (fakeEnvManager) InstallDeps(context.Context, string, string, []string, sandboxdomain.ProgressFunc) error {
	return nil
}

func (fakeEnvManager) ResolveExec(_, envPath string, opts sandboxdomain.SpawnOpts) (string, []string, string) {
	cmd := opts.Cmd
	if p, err := exec.LookPath(cmd); err == nil {
		cmd = p
	}
	return cmd, opts.Args, envPath
}

// fakeInstaller installs nothing — it just satisfies the EnsureRuntime path.
//
// fakeInstaller 不装任何东西——只满足 EnsureRuntime 流程。
type fakeInstaller struct{ kind string }

func (f fakeInstaller) Kind() string { return f.kind }
func (f fakeInstaller) Install(context.Context, string, string, sandboxdomain.ProgressFunc) (string, error) {
	return "runtimes/" + f.kind, nil
}
func (fakeInstaller) Locate(string, string) (string, error)          { return "/fake/bin", nil }
func (fakeInstaller) ResolveDefault(context.Context) (string, error) { return "1.0", nil }
func (fakeInstaller) NormalizeVersion(v string) string               { return v }

func newSvc(t *testing.T, kind string) *Service {
	t.Helper()
	db, err := dbinfra.Open(dbinfra.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := dbinfra.Migrate(db, sandboxstore.Schema...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := New(sandboxstore.New(db), t.TempDir(), nil, zap.NewNop())
	svc.MarkReadyForTest("/fake/mise")
	svc.RegisterInstaller(fakeInstaller{kind: kind})
	svc.RegisterEnvManager(fakeEnvManager{kind: kind})
	return svc
}

// newServiceWithEnv seeds a ready runtime + env so Spawn can resolve them.
//
// newServiceWithEnv 预置 ready 的 runtime + env，使 Spawn 能解析到它们。
func newServiceWithEnv(t *testing.T, kind string) (*Service, sandboxdomain.Owner) {
	t.Helper()
	svc := newSvc(t, kind)
	ctx := context.Background()
	if err := svc.repo.CreateRuntime(ctx, &sandboxdomain.Runtime{
		ID: "sr_test", Kind: kind, Version: "1.0", Path: "fake/" + kind,
	}); err != nil {
		t.Fatalf("seed runtime: %v", err)
	}
	owner := sandboxdomain.Owner{Kind: sandboxdomain.OwnerKindFunction, ID: "fn_test"}
	envRel := filepath.Join("envs", owner.Kind, owner.ID)
	if err := svc.repo.CreateEnv(ctx, &sandboxdomain.Env{
		ID: "se_test", OwnerKind: owner.Kind, OwnerID: owner.ID, RuntimeID: "sr_test",
		Path: envRel, Status: sandboxdomain.EnvStatusReady, LastUsedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed env: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(svc.SandboxRoot(), envRel), 0o755); err != nil {
		t.Fatalf("mkdir env path: %v", err)
	}
	return svc, owner
}

// ---- Spawn ----

func TestServiceSpawn_HappyPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses echo via PATH")
	}
	svc, owner := newServiceWithEnv(t, "fake-py")
	res, err := svc.Spawn(context.Background(), owner, sandboxdomain.SpawnOpts{
		Cmd: "echo", Args: []string{"hello service"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if !res.Ok || string(res.Stdout) != "hello service\n" {
		t.Errorf("Ok=%v stdout=%q", res.Ok, res.Stdout)
	}
}

func TestServiceSpawn_NotReady_Errors(t *testing.T) {
	svc, owner := newServiceWithEnv(t, "fake-py")
	svc.bootstrapped.Store(false)
	_, err := svc.Spawn(context.Background(), owner, sandboxdomain.SpawnOpts{Cmd: "echo"})
	if !errors.Is(err, sandboxdomain.ErrSpawnFailed) {
		t.Errorf("degraded spawn: err = %v, want ErrSpawnFailed", err)
	}
}

func TestServiceSpawn_EmptyCmd_Errors(t *testing.T) {
	svc, owner := newServiceWithEnv(t, "fake-py")
	if _, err := svc.Spawn(context.Background(), owner, sandboxdomain.SpawnOpts{}); !errors.Is(err, sandboxdomain.ErrCmdRequired) {
		t.Errorf("empty cmd: err = %v, want ErrCmdRequired", err)
	}
}

func TestServiceSpawn_UnknownOwner_ErrEnvNotFound(t *testing.T) {
	svc, _ := newServiceWithEnv(t, "fake-py")
	_, err := svc.Spawn(context.Background(),
		sandboxdomain.Owner{Kind: sandboxdomain.OwnerKindMCP, ID: "nope"},
		sandboxdomain.SpawnOpts{Cmd: "echo"})
	if !errors.Is(err, sandboxdomain.ErrEnvNotFound) {
		t.Errorf("unknown owner: err = %v, want ErrEnvNotFound", err)
	}
}

// ---- long-lived handle lifecycle ----

func TestServiceSpawnLongLived_RegistersAndUnregisters(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses cat")
	}
	svc, owner := newServiceWithEnv(t, "fake-py")
	if c := svc.ActiveHandleCountForTest(); c != 0 {
		t.Fatalf("baseline handle count = %d", c)
	}
	handle, err := svc.SpawnLongLived(context.Background(), owner, sandboxdomain.SpawnOpts{Cmd: "cat"})
	if err != nil {
		t.Fatalf("SpawnLongLived: %v", err)
	}
	if c := svc.ActiveHandleCountForTest(); c != 1 {
		t.Errorf("after spawn count = %d, want 1", c)
	}
	_ = handle.Stdin().Close()
	_, _ = io.Copy(io.Discard, handle.Stdout())
	if err := handle.Wait(); err != nil {
		t.Errorf("Wait: %v", err)
	}
	if c := svc.ActiveHandleCountForTest(); c != 0 {
		t.Errorf("after Wait count = %d, want 0 (auto-unregister)", c)
	}
}

func TestServiceShutdown_KillsAllHandles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sleep")
	}
	svc, owner := newServiceWithEnv(t, "fake-py")
	var handles []sandboxdomain.LongLivedHandle
	for range 3 {
		h, err := svc.SpawnLongLived(context.Background(), owner, sandboxdomain.SpawnOpts{
			Cmd: "sleep", Args: []string{"30"},
		})
		if err != nil {
			t.Fatalf("SpawnLongLived: %v", err)
		}
		handles = append(handles, h)
	}
	if c := svc.ActiveHandleCountForTest(); c != 3 {
		t.Errorf("after spawning 3, count = %d", c)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := svc.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
	for _, h := range handles {
		_ = h.Wait()
	}
}

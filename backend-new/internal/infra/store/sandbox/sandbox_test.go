package sandbox

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, stmt := range Schema {
		if _, err := sqlDB.Exec(stmt); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	return New(ormpkg.Open(sqlDB))
}

func seedRuntime(t *testing.T, s *Store, id, kind, version string) {
	t.Helper()
	if err := s.CreateRuntime(context.Background(), &sandboxdomain.Runtime{
		ID: id, Kind: kind, Version: version, Path: "runtimes/" + kind + "/" + version, SizeBytes: 1000,
	}); err != nil {
		t.Fatalf("seed runtime %s: %v", id, err)
	}
}

func seedEnv(t *testing.T, s *Store, id, ownerKind, ownerID, runtimeID string, deps []string) {
	t.Helper()
	if err := s.CreateEnv(context.Background(), &sandboxdomain.Env{
		ID: id, OwnerKind: ownerKind, OwnerID: ownerID, RuntimeID: runtimeID,
		Deps: deps, Path: "envs/" + ownerKind + "/" + ownerID,
		SizeBytes: 500, Status: sandboxdomain.EnvStatusReady, LastUsedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed env %s: %v", id, err)
	}
}

func TestRuntime_CreateFindGet_RoundTrips(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	seedRuntime(t, s, "sr_1", "python", "3.12.5")

	byPair, err := s.FindRuntime(ctx, "python", "3.12.5")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if byPair.ID != "sr_1" || byPair.Path != "runtimes/python/3.12.5" {
		t.Errorf("find round-trip: %+v", byPair)
	}
	byID, err := s.GetRuntime(ctx, "sr_1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if byID.Kind != "python" || byID.Version != "3.12.5" {
		t.Errorf("get round-trip: %+v", byID)
	}
	if byID.InstalledAt.IsZero() {
		t.Error("installed_at not auto-stamped by orm")
	}
}

func TestRuntime_Miss_ErrRuntimeNotFound(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if _, err := s.GetRuntime(ctx, "sr_nope"); !errors.Is(err, sandboxdomain.ErrRuntimeNotFound) {
		t.Errorf("GetRuntime miss: err = %v, want ErrRuntimeNotFound", err)
	}
	if _, err := s.FindRuntime(ctx, "python", "9.9"); !errors.Is(err, sandboxdomain.ErrRuntimeNotFound) {
		t.Errorf("FindRuntime miss: err = %v, want ErrRuntimeNotFound", err)
	}
}

func TestRuntime_ListOrdered_DeleteHard(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	seedRuntime(t, s, "sr_2", "python", "3.12")
	seedRuntime(t, s, "sr_1", "node", "22")

	rows, err := s.ListRuntimes(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 || rows[0].Kind != "node" || rows[1].Kind != "python" {
		t.Errorf("list not ordered by kind: %+v", rows)
	}

	if err := s.DeleteRuntime(ctx, "sr_1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetRuntime(ctx, "sr_1"); !errors.Is(err, sandboxdomain.ErrRuntimeNotFound) {
		t.Errorf("hard-deleted runtime should miss, err = %v", err)
	}
}

func TestEnv_CreateFindByOwner_DepsRoundTrip(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	seedRuntime(t, s, "sr_1", "python", "3.12")
	seedEnv(t, s, "se_1", sandboxdomain.OwnerKindFunction, "fn_abc", "sr_1", []string{"requests==2.31", "numpy"})

	byOwner, err := s.FindEnvByOwner(ctx, sandboxdomain.OwnerKindFunction, "fn_abc")
	if err != nil {
		t.Fatalf("find by owner: %v", err)
	}
	if byOwner.ID != "se_1" {
		t.Errorf("find by owner id = %q", byOwner.ID)
	}
	if len(byOwner.Deps) != 2 || byOwner.Deps[0] != "requests==2.31" {
		t.Errorf("deps json round-trip failed: %+v", byOwner.Deps)
	}
}

func TestEnv_Miss_ErrEnvNotFound(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if _, err := s.GetEnv(ctx, "se_nope"); !errors.Is(err, sandboxdomain.ErrEnvNotFound) {
		t.Errorf("GetEnv miss: err = %v", err)
	}
	if _, err := s.FindEnvByOwner(ctx, "function", "fn_nope"); !errors.Is(err, sandboxdomain.ErrEnvNotFound) {
		t.Errorf("FindEnvByOwner miss: err = %v", err)
	}
}

func TestEnv_ListByOwnerKind_And_ByRuntime(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	seedRuntime(t, s, "sr_1", "python", "3.12")
	seedEnv(t, s, "se_1", sandboxdomain.OwnerKindFunction, "fn_a", "sr_1", nil)
	seedEnv(t, s, "se_2", sandboxdomain.OwnerKindHandler, "hd_b", "sr_1", nil)

	fns, err := s.ListEnvsByOwnerKind(ctx, sandboxdomain.OwnerKindFunction)
	if err != nil {
		t.Fatalf("list by owner kind: %v", err)
	}
	if len(fns) != 1 || fns[0].OwnerID != "fn_a" {
		t.Errorf("list by owner kind: %+v", fns)
	}

	refs, err := s.ListEnvsByRuntime(ctx, "sr_1")
	if err != nil {
		t.Fatalf("list by runtime: %v", err)
	}
	if len(refs) != 2 {
		t.Errorf("list by runtime = %d, want 2 (GC dependency check)", len(refs))
	}
}

func TestTotalSizeBytes_SumsBothTables(t *testing.T) {
	s := newStore(t)
	seedRuntime(t, s, "sr_1", "python", "3.12")            // 1000
	seedEnv(t, s, "se_1", "function", "fn_a", "sr_1", nil) // 500

	total, err := s.TotalSizeBytes(context.Background())
	if err != nil {
		t.Fatalf("total: %v", err)
	}
	if total != 1500 {
		t.Errorf("total size = %d, want 1500 (runtime 1000 + env 500)", total)
	}
}

func TestRunningPID_SetListClear(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	seedRuntime(t, s, "sr_1", "python", "3.12")
	seedEnv(t, s, "se_1", "handler", "hd_a", "sr_1", nil)

	if err := s.SetEnvRunningPID(ctx, "se_1", 4242); err != nil {
		t.Fatalf("set pid: %v", err)
	}
	live, err := s.ListEnvsWithRunningPID(ctx)
	if err != nil {
		t.Fatalf("list running: %v", err)
	}
	if len(live) != 1 || live[0].RunningPID != 4242 {
		t.Errorf("running pid not tracked: %+v", live)
	}

	if err := s.ClearEnvRunningPID(ctx, "se_1"); err != nil {
		t.Fatalf("clear pid: %v", err)
	}
	if live, _ := s.ListEnvsWithRunningPID(ctx); len(live) != 0 {
		t.Errorf("running pid not cleared: %+v", live)
	}
}

func TestEnv_ListLastUsedBefore(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	seedRuntime(t, s, "sr_1", "python", "3.12")
	// Seed an env, then force its last_used_at into the past via UpdateEnv.
	seedEnv(t, s, "se_old", "function", "fn_old", "sr_1", nil)
	old, _ := s.GetEnv(ctx, "se_old")
	old.LastUsedAt = time.Now().Add(-48 * time.Hour)
	if err := s.UpdateEnv(ctx, old); err != nil {
		t.Fatalf("update: %v", err)
	}
	seedEnv(t, s, "se_new", "function", "fn_new", "sr_1", nil) // last_used = now

	stale, err := s.ListEnvsLastUsedBefore(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("list stale: %v", err)
	}
	if len(stale) != 1 || stale[0].ID != "se_old" {
		t.Errorf("GC cutoff query: %+v", stale)
	}
}

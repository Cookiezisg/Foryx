package sandbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
)

var _ sandboxdomain.Repository = (*Store)(nil)

func newStore(t *testing.T) *Store {
	t.Helper()
	database, err := dbinfra.Open(dbinfra.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(database) })
	if err := dbinfra.Migrate(database,
		&sandboxdomain.Runtime{},
		&sandboxdomain.Env{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(database)
}

func mkRuntime(id, kind, version string) *sandboxdomain.Runtime {
	return &sandboxdomain.Runtime{
		ID:          id,
		Kind:        kind,
		Version:     version,
		Path:        kind + "/" + version,
		SizeBytes:   100,
		InstalledAt: time.Now(),
	}
}

func mkEnv(id, ownerKind, ownerID, runtimeID string) *sandboxdomain.Env {
	return &sandboxdomain.Env{
		ID:         id,
		OwnerKind:  ownerKind,
		OwnerID:    ownerID,
		RuntimeID:  runtimeID,
		Deps:       []string{"pkg-a", "pkg-b"},
		Path:       ownerKind + "/" + ownerID,
		SizeBytes:  50,
		Status:     sandboxdomain.EnvStatusReady,
		CreatedAt:  time.Now(),
		LastUsedAt: time.Now(),
	}
}


func TestCreateAndGetRuntime(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	r := mkRuntime("sr_001", "python", "3.12.5")
	if err := s.CreateRuntime(ctx, r); err != nil {
		t.Fatalf("CreateRuntime: %v", err)
	}
	got, err := s.GetRuntime(ctx, "sr_001")
	if err != nil {
		t.Fatalf("GetRuntime: %v", err)
	}
	if got.Kind != "python" || got.Version != "3.12.5" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestGetRuntime_NotFound(t *testing.T) {
	s := newStore(t)
	_, err := s.GetRuntime(context.Background(), "sr_missing")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("want gorm.ErrRecordNotFound, got %v", err)
	}
}

func TestCreateRuntime_UniqueKindVersion(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.CreateRuntime(ctx, mkRuntime("sr_001", "python", "3.12.5")); err != nil {
		t.Fatalf("first CreateRuntime: %v", err)
	}
	err := s.CreateRuntime(ctx, mkRuntime("sr_002", "python", "3.12.5"))
	if err == nil {
		t.Fatal("want UNIQUE violation, got nil")
	}
}

func TestFindRuntime_ExactMatch(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.CreateRuntime(ctx, mkRuntime("sr_001", "node", "22.5.0")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, err := s.FindRuntime(ctx, "node", "22.5.0")
	if err != nil {
		t.Fatalf("FindRuntime: %v", err)
	}
	if got.ID != "sr_001" {
		t.Errorf("want sr_001, got %s", got.ID)
	}
	_, err = s.FindRuntime(ctx, "node", "20.0.0")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("missing version: want gorm.ErrRecordNotFound, got %v", err)
	}
}

func TestListRuntimes_OrderByKindThenVersion(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	for i, r := range []*sandboxdomain.Runtime{
		mkRuntime("sr_a", "python", "3.12.5"),
		mkRuntime("sr_b", "node", "22.5.0"),
		mkRuntime("sr_c", "node", "20.0.0"),
	} {
		if err := s.CreateRuntime(ctx, r); err != nil {
			t.Fatalf("seed[%d]: %v", i, err)
		}
	}
	rows, err := s.ListRuntimes(ctx)
	if err != nil {
		t.Fatalf("ListRuntimes: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}
	want := []string{"sr_c", "sr_b", "sr_a"}
	for i, w := range want {
		if rows[i].ID != w {
			t.Errorf("rows[%d]: want %s, got %s", i, w, rows[i].ID)
		}
	}
}

func TestUpdateRuntime_RefreshSize(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	r := mkRuntime("sr_001", "python", "3.12.5")
	if err := s.CreateRuntime(ctx, r); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r.SizeBytes = 9999
	if err := s.UpdateRuntime(ctx, r); err != nil {
		t.Fatalf("UpdateRuntime: %v", err)
	}
	got, err := s.GetRuntime(ctx, "sr_001")
	if err != nil {
		t.Fatalf("re-get: %v", err)
	}
	if got.SizeBytes != 9999 {
		t.Errorf("SizeBytes = %d, want 9999 — UpdateRuntime should persist", got.SizeBytes)
	}
}

func TestDeleteRuntime(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.CreateRuntime(ctx, mkRuntime("sr_001", "python", "3.12.5")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.DeleteRuntime(ctx, "sr_001"); err != nil {
		t.Fatalf("DeleteRuntime: %v", err)
	}
	_, err := s.GetRuntime(ctx, "sr_001")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("post-delete: want gorm.ErrRecordNotFound, got %v", err)
	}
}


func TestCreateAndGetEnv(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.CreateRuntime(ctx, mkRuntime("sr_001", "python", "3.12.5")); err != nil {
		t.Fatalf("seed runtime: %v", err)
	}
	e := mkEnv("se_001", sandboxdomain.OwnerKindMCP, "playwright", "sr_001")
	if err := s.CreateEnv(ctx, e); err != nil {
		t.Fatalf("CreateEnv: %v", err)
	}
	got, err := s.GetEnv(ctx, "se_001")
	if err != nil {
		t.Fatalf("GetEnv: %v", err)
	}
	if got.OwnerKind != sandboxdomain.OwnerKindMCP || got.OwnerID != "playwright" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if len(got.Deps) != 2 || got.Deps[0] != "pkg-a" {
		t.Errorf("Deps JSON serialization broken: %v", got.Deps)
	}
}

func TestGetEnv_NotFound(t *testing.T) {
	s := newStore(t)
	_, err := s.GetEnv(context.Background(), "se_missing")
	if !errors.Is(err, sandboxdomain.ErrEnvNotFound) {
		t.Errorf("want ErrEnvNotFound, got %v", err)
	}
}

func TestCreateEnv_UniqueOwner(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.CreateRuntime(ctx, mkRuntime("sr_001", "python", "3.12.5")); err != nil {
		t.Fatalf("seed runtime: %v", err)
	}
	if err := s.CreateEnv(ctx, mkEnv("se_001", sandboxdomain.OwnerKindMCP, "playwright", "sr_001")); err != nil {
		t.Fatalf("first CreateEnv: %v", err)
	}
	err := s.CreateEnv(ctx, mkEnv("se_002", sandboxdomain.OwnerKindMCP, "playwright", "sr_001"))
	if err == nil {
		t.Fatal("want UNIQUE violation, got nil")
	}
}

func TestFindEnvByOwner(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.CreateRuntime(ctx, mkRuntime("sr_001", "python", "3.12.5")); err != nil {
		t.Fatalf("seed runtime: %v", err)
	}
	if err := s.CreateEnv(ctx, mkEnv("se_001", sandboxdomain.OwnerKindFunction, "f_abc", "sr_001")); err != nil {
		t.Fatalf("seed env: %v", err)
	}
	got, err := s.FindEnvByOwner(ctx, sandboxdomain.OwnerKindFunction, "f_abc")
	if err != nil {
		t.Fatalf("FindEnvByOwner: %v", err)
	}
	if got.ID != "se_001" {
		t.Errorf("want se_001, got %s", got.ID)
	}
	_, err = s.FindEnvByOwner(ctx, sandboxdomain.OwnerKindFunction, "f_missing")
	if !errors.Is(err, sandboxdomain.ErrEnvNotFound) {
		t.Errorf("missing owner: want ErrEnvNotFound, got %v", err)
	}
}

func TestListEnvsByRuntime_FKReverseLookup(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.CreateRuntime(ctx, mkRuntime("sr_001", "python", "3.12.5")); err != nil {
		t.Fatalf("seed runtime: %v", err)
	}
	if err := s.CreateRuntime(ctx, mkRuntime("sr_002", "node", "22.5.0")); err != nil {
		t.Fatalf("seed runtime 2: %v", err)
	}
	for i, e := range []*sandboxdomain.Env{
		mkEnv("se_a", sandboxdomain.OwnerKindMCP, "playwright", "sr_001"),
		mkEnv("se_b", sandboxdomain.OwnerKindFunction, "f_one", "sr_001"),
		mkEnv("se_c", sandboxdomain.OwnerKindMCP, "context7", "sr_002"),
	} {
		if err := s.CreateEnv(ctx, e); err != nil {
			t.Fatalf("seed env[%d]: %v", i, err)
		}
	}
	pyEnvs, err := s.ListEnvsByRuntime(ctx, "sr_001")
	if err != nil {
		t.Fatalf("ListEnvsByRuntime: %v", err)
	}
	if len(pyEnvs) != 2 {
		t.Errorf("want 2 envs on sr_001, got %d", len(pyEnvs))
	}
	nodeEnvs, err := s.ListEnvsByRuntime(ctx, "sr_002")
	if err != nil {
		t.Fatalf("ListEnvsByRuntime node: %v", err)
	}
	if len(nodeEnvs) != 1 {
		t.Errorf("want 1 env on sr_002, got %d", len(nodeEnvs))
	}
}

func TestListEnvsByOwnerKind_OrderedByLastUsed(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.CreateRuntime(ctx, mkRuntime("sr_001", "python", "3.12.5")); err != nil {
		t.Fatalf("seed runtime: %v", err)
	}
	now := time.Now()
	old := mkEnv("se_old", sandboxdomain.OwnerKindMCP, "old-server", "sr_001")
	old.LastUsedAt = now.Add(-2 * time.Hour)
	recent := mkEnv("se_new", sandboxdomain.OwnerKindMCP, "new-server", "sr_001")
	recent.LastUsedAt = now
	if err := s.CreateEnv(ctx, old); err != nil {
		t.Fatalf("seed old: %v", err)
	}
	if err := s.CreateEnv(ctx, recent); err != nil {
		t.Fatalf("seed recent: %v", err)
	}
	rows, err := s.ListEnvsByOwnerKind(ctx, sandboxdomain.OwnerKindMCP)
	if err != nil {
		t.Fatalf("ListEnvsByOwnerKind: %v", err)
	}
	if len(rows) != 2 || rows[0].ID != "se_new" {
		t.Errorf("want se_new first (DESC by last_used_at), got %v", []string{rows[0].ID, rows[1].ID})
	}
}

func TestUpdateEnv_StatusTransition(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.CreateRuntime(ctx, mkRuntime("sr_001", "python", "3.12.5")); err != nil {
		t.Fatalf("seed runtime: %v", err)
	}
	e := mkEnv("se_001", sandboxdomain.OwnerKindMCP, "playwright", "sr_001")
	e.Status = sandboxdomain.EnvStatusInstalling
	if err := s.CreateEnv(ctx, e); err != nil {
		t.Fatalf("seed: %v", err)
	}
	e.Status = sandboxdomain.EnvStatusReady
	if err := s.UpdateEnv(ctx, e); err != nil {
		t.Fatalf("UpdateEnv: %v", err)
	}
	got, err := s.GetEnv(ctx, "se_001")
	if err != nil {
		t.Fatalf("re-get: %v", err)
	}
	if got.Status != sandboxdomain.EnvStatusReady {
		t.Errorf("want ready, got %s", got.Status)
	}
}

func TestDeleteEnv(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.CreateRuntime(ctx, mkRuntime("sr_001", "python", "3.12.5")); err != nil {
		t.Fatalf("seed runtime: %v", err)
	}
	if err := s.CreateEnv(ctx, mkEnv("se_001", sandboxdomain.OwnerKindFunction, "f_abc", "sr_001")); err != nil {
		t.Fatalf("seed env: %v", err)
	}
	if err := s.DeleteEnv(ctx, "se_001"); err != nil {
		t.Fatalf("DeleteEnv: %v", err)
	}
	_, err := s.GetEnv(ctx, "se_001")
	if !errors.Is(err, sandboxdomain.ErrEnvNotFound) {
		t.Errorf("post-delete: want ErrEnvNotFound, got %v", err)
	}
}


func TestTotalSizeBytes(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	got, err := s.TotalSizeBytes(ctx)
	if err != nil {
		t.Fatalf("TotalSizeBytes empty: %v", err)
	}
	if got != 0 {
		t.Errorf("empty: want 0, got %d", got)
	}

	r := mkRuntime("sr_001", "python", "3.12.5")
	r.SizeBytes = 1000
	if err := s.CreateRuntime(ctx, r); err != nil {
		t.Fatalf("seed runtime: %v", err)
	}
	e := mkEnv("se_001", sandboxdomain.OwnerKindFunction, "f_abc", "sr_001")
	e.SizeBytes = 250
	if err := s.CreateEnv(ctx, e); err != nil {
		t.Fatalf("seed env: %v", err)
	}
	got, err = s.TotalSizeBytes(ctx)
	if err != nil {
		t.Fatalf("TotalSizeBytes: %v", err)
	}
	if got != 1250 {
		t.Errorf("want 1250, got %d", got)
	}
}


func TestSetAndListEnvsWithRunningPID(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	if err := s.CreateRuntime(ctx, mkRuntime("sr_001", "python", "3.12.5")); err != nil {
		t.Fatalf("seed runtime: %v", err)
	}
	if err := s.CreateEnv(ctx, mkEnv("se_a", sandboxdomain.OwnerKindMCP, "playwright", "sr_001")); err != nil {
		t.Fatalf("seed env a: %v", err)
	}
	if err := s.CreateEnv(ctx, mkEnv("se_b", sandboxdomain.OwnerKindMCP, "context7", "sr_001")); err != nil {
		t.Fatalf("seed env b: %v", err)
	}

	rows, err := s.ListEnvsWithRunningPID(ctx)
	if err != nil {
		t.Fatalf("ListEnvsWithRunningPID empty: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("baseline: want 0 envs with PID, got %d", len(rows))
	}

	if err := s.SetEnvRunningPID(ctx, "se_a", 12345); err != nil {
		t.Fatalf("SetEnvRunningPID: %v", err)
	}
	rows, err = s.ListEnvsWithRunningPID(ctx)
	if err != nil {
		t.Fatalf("ListEnvsWithRunningPID after set: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "se_a" {
		t.Fatalf("want only se_a, got %v", rows)
	}
	if rows[0].RunningPID != 12345 {
		t.Errorf("RunningPID = %d, want 12345", rows[0].RunningPID)
	}
}

func TestClearEnvRunningPID(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.CreateRuntime(ctx, mkRuntime("sr_001", "python", "3.12.5")); err != nil {
		t.Fatalf("seed runtime: %v", err)
	}
	if err := s.CreateEnv(ctx, mkEnv("se_a", sandboxdomain.OwnerKindMCP, "playwright", "sr_001")); err != nil {
		t.Fatalf("seed env: %v", err)
	}
	if err := s.SetEnvRunningPID(ctx, "se_a", 12345); err != nil {
		t.Fatalf("SetEnvRunningPID: %v", err)
	}
	if err := s.ClearEnvRunningPID(ctx, "se_a"); err != nil {
		t.Fatalf("ClearEnvRunningPID: %v", err)
	}
	rows, err := s.ListEnvsWithRunningPID(ctx)
	if err != nil {
		t.Fatalf("ListEnvsWithRunningPID after clear: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("after clear: want 0 envs with PID, got %d", len(rows))
	}
}

func TestListEnvsLastUsedBefore(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.CreateRuntime(ctx, mkRuntime("sr_001", "python", "3.12.5")); err != nil {
		t.Fatalf("seed runtime: %v", err)
	}
	now := time.Now()
	for i, ent := range []struct {
		id       string
		ownerID  string
		lastUsed time.Time
	}{
		{"se_old1", "f_old1", now.Add(-3 * time.Hour)},
		{"se_old2", "f_old2", now.Add(-2 * time.Hour)},
		{"se_new", "f_new", now.Add(-30 * time.Minute)},
	} {
		e := mkEnv(ent.id, sandboxdomain.OwnerKindFunction, ent.ownerID, "sr_001")
		e.LastUsedAt = ent.lastUsed
		if err := s.CreateEnv(ctx, e); err != nil {
			t.Fatalf("seed[%d]: %v", i, err)
		}
	}
	cutoff := now.Add(-1 * time.Hour)
	rows, err := s.ListEnvsLastUsedBefore(ctx, cutoff)
	if err != nil {
		t.Fatalf("ListEnvsLastUsedBefore: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 envs older than cutoff, got %d", len(rows))
	}
	if rows[0].ID != "se_old1" || rows[1].ID != "se_old2" {
		t.Errorf("wrong order: %v", []string{rows[0].ID, rows[1].ID})
	}
}

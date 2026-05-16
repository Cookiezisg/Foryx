package function

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	gormlogger "gorm.io/gorm/logger"

	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

const (
	userAlice = "u-alice"
	userBob   = "u-bob"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	database, err := dbinfra.Open(dbinfra.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(database) })
	if err := dbinfra.Migrate(database, AutoMigrateModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(database)
}

func ctxFor(userID string) context.Context {
	return reqctxpkg.SetUserID(context.Background(), userID)
}

func mkFunction(id, userID, name string) *functiondomain.Function {
	return &functiondomain.Function{
		ID:          id,
		UserID:      userID,
		Name:        name,
		Description: "test-" + id,
		Tags:        []string{},
	}
}

func mkVersion(id, functionID, status string) *functiondomain.Version {
	return &functiondomain.Version{
		ID:            id,
		FunctionID:    functionID,
		Status:        status,
		Code:          "def " + id + "(): pass",
		Parameters:    []functiondomain.ParameterSpec{},
		ReturnSchema:  map[string]any{},
		Dependencies:  []string{},
		PythonVersion: functiondomain.DefaultPythonVersion,
		EnvStatus:     functiondomain.EnvStatusPending,
	}
}

func TestSaveFunction_HappyPath(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	f := mkFunction("fn1", userAlice, "to-pdf")
	if err := s.SaveFunction(ctx, f); err != nil {
		t.Fatalf("SaveFunction: %v", err)
	}

	got, err := s.GetFunction(ctx, "fn1")
	if err != nil {
		t.Fatalf("GetFunction: %v", err)
	}
	if got.Name != "to-pdf" || got.UserID != userAlice {
		t.Errorf("unexpected fields: %+v", got)
	}
}

func TestSaveFunction_DuplicateName(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	f1 := mkFunction("fn1", userAlice, "to-pdf")
	if err := s.SaveFunction(ctx, f1); err != nil {
		t.Fatalf("first SaveFunction: %v", err)
	}

	f2 := mkFunction("fn2", userAlice, "to-pdf") // same name, different id
	err := s.SaveFunction(ctx, f2)
	if !errors.Is(err, functiondomain.ErrDuplicateName) {
		t.Errorf("expected ErrDuplicateName, got: %v", err)
	}
}

func TestSaveFunction_SoftDeleteAllowsRecreate(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	f1 := mkFunction("fn1", userAlice, "to-pdf")
	if err := s.SaveFunction(ctx, f1); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFunction(ctx, "fn1"); err != nil {
		t.Fatalf("DeleteFunction: %v", err)
	}

	f2 := mkFunction("fn2", userAlice, "to-pdf")
	if err := s.SaveFunction(ctx, f2); err != nil {
		t.Errorf("recreate after soft-delete should work, got: %v", err)
	}
}

func TestGetFunction_CrossUserIsolated(t *testing.T) {
	s := newStore(t)
	ctxA := ctxFor(userAlice)
	ctxB := ctxFor(userBob)

	f := mkFunction("fn1", userAlice, "alice-fn")
	if err := s.SaveFunction(ctxA, f); err != nil {
		t.Fatal(err)
	}

	_, err := s.GetFunction(ctxB, "fn1")
	if !errors.Is(err, functiondomain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for cross-user, got: %v", err)
	}
}

func TestGetFunctionByName(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	f := mkFunction("fn1", userAlice, "to-pdf")
	if err := s.SaveFunction(ctx, f); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetFunctionByName(ctx, "to-pdf")
	if err != nil {
		t.Fatalf("GetFunctionByName: %v", err)
	}
	if got.ID != "fn1" {
		t.Errorf("got id %q, want fn1", got.ID)
	}

	_, err = s.GetFunctionByName(ctx, "does-not-exist")
	if !errors.Is(err, functiondomain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for missing name, got: %v", err)
	}
}

func TestListFunctions_Pagination(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	for i := 0; i < 5; i++ {
		f := mkFunction(
			fmt.Sprintf("fn%d", i),
			userAlice,
			fmt.Sprintf("name%d", i),
		)
		f.CreatedAt = time.Now().UTC().Add(time.Duration(i) * time.Second)
		if err := s.SaveFunction(ctx, f); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	rows, next, err := s.ListFunctions(ctx, functiondomain.ListFilter{Limit: 2})
	if err != nil {
		t.Fatalf("ListFunctions page 1: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("page 1: want 2 rows, got %d", len(rows))
	}
	if next == "" {
		t.Error("page 1: expected nextCursor")
	}

	rows2, next2, err := s.ListFunctions(ctx, functiondomain.ListFilter{Limit: 2, Cursor: next})
	if err != nil {
		t.Fatalf("ListFunctions page 2: %v", err)
	}
	if len(rows2) != 2 {
		t.Errorf("page 2: want 2 rows, got %d", len(rows2))
	}
	if rows2[0].ID == rows[0].ID || rows2[0].ID == rows[1].ID {
		t.Errorf("page 2 should not repeat page 1: %v vs %v", rows2[0].ID, rows[0].ID)
	}

	rows3, next3, err := s.ListFunctions(ctx, functiondomain.ListFilter{Limit: 2, Cursor: next2})
	if err != nil {
		t.Fatalf("ListFunctions page 3: %v", err)
	}
	if len(rows3) != 1 {
		t.Errorf("page 3: want 1 row, got %d", len(rows3))
	}
	if next3 != "" {
		t.Errorf("page 3 should have no nextCursor, got %q", next3)
	}
}

func TestListAllFunctions(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	for i := 0; i < 3; i++ {
		f := mkFunction(fmt.Sprintf("fn%d", i), userAlice, fmt.Sprintf("name%d", i))
		if err := s.SaveFunction(ctx, f); err != nil {
			t.Fatal(err)
		}
	}

	rows, err := s.ListAllFunctions(ctx)
	if err != nil {
		t.Fatalf("ListAllFunctions: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("want 3 rows, got %d", len(rows))
	}
}

func TestGetFunctionsByIDs_PreservesOrder(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	for i := 0; i < 3; i++ {
		f := mkFunction(fmt.Sprintf("fn%d", i), userAlice, fmt.Sprintf("name%d", i))
		if err := s.SaveFunction(ctx, f); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.GetFunctionsByIDs(ctx, []string{"fn2", "fn0", "fn1"})
	if err != nil {
		t.Fatalf("GetFunctionsByIDs: %v", err)
	}
	want := []string{"fn2", "fn0", "fn1"}
	for i, f := range got {
		if f.ID != want[i] {
			t.Errorf("position %d: got %q, want %q", i, f.ID, want[i])
		}
	}
}

func TestSetActiveVersion(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	f := mkFunction("fn1", userAlice, "to-pdf")
	if err := s.SaveFunction(ctx, f); err != nil {
		t.Fatal(err)
	}

	if err := s.SetActiveVersion(ctx, "fn1", "fnv1"); err != nil {
		t.Fatalf("SetActiveVersion: %v", err)
	}

	got, _ := s.GetFunction(ctx, "fn1")
	if got.ActiveVersionID != "fnv1" {
		t.Errorf("ActiveVersionID = %q, want fnv1", got.ActiveVersionID)
	}

	if err := s.SetActiveVersion(ctxFor(userBob), "fn1", "fnv2"); !errors.Is(err, functiondomain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for cross-user SetActiveVersion, got: %v", err)
	}
}

func TestSaveAndGetVersion(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	f := mkFunction("fn1", userAlice, "to-pdf")
	_ = s.SaveFunction(ctx, f)

	v := mkVersion("fnv1", "fn1", functiondomain.StatusPending)
	if err := s.SaveVersion(ctx, v); err != nil {
		t.Fatalf("SaveVersion: %v", err)
	}

	got, err := s.GetVersion(ctx, "fnv1")
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if got.Status != functiondomain.StatusPending {
		t.Errorf("Status = %q, want pending", got.Status)
	}

	_, err = s.GetVersion(ctx, "missing")
	if !errors.Is(err, functiondomain.ErrVersionNotFound) {
		t.Errorf("expected ErrVersionNotFound, got: %v", err)
	}
}

func TestGetPending(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	_ = s.SaveFunction(ctx, mkFunction("fn1", userAlice, "to-pdf"))

	if _, err := s.GetPending(ctx, "fn1"); !errors.Is(err, functiondomain.ErrPendingNotFound) {
		t.Errorf("expected ErrPendingNotFound when no pending, got: %v", err)
	}

	v := mkVersion("fnv1", "fn1", functiondomain.StatusPending)
	_ = s.SaveVersion(ctx, v)

	got, err := s.GetPending(ctx, "fn1")
	if err != nil {
		t.Fatalf("GetPending: %v", err)
	}
	if got.ID != "fnv1" {
		t.Errorf("got id %q, want fnv1", got.ID)
	}
}

func TestUpdateVersionStatus(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	_ = s.SaveFunction(ctx, mkFunction("fn1", userAlice, "to-pdf"))
	v := mkVersion("fnv1", "fn1", functiondomain.StatusPending)
	_ = s.SaveVersion(ctx, v)

	versionN := 1
	if err := s.UpdateVersionStatus(ctx, "fnv1", functiondomain.StatusAccepted, &versionN); err != nil {
		t.Fatalf("UpdateVersionStatus accepted: %v", err)
	}
	got, _ := s.GetVersion(ctx, "fnv1")
	if got.Status != functiondomain.StatusAccepted {
		t.Errorf("Status = %q, want accepted", got.Status)
	}
	if got.Version == nil || *got.Version != 1 {
		t.Errorf("Version = %v, want *1", got.Version)
	}

	gotByN, err := s.GetVersionByNumber(ctx, "fn1", 1)
	if err != nil {
		t.Fatalf("GetVersionByNumber: %v", err)
	}
	if gotByN.ID != "fnv1" {
		t.Errorf("got id %q, want fnv1", gotByN.ID)
	}

	if err := s.UpdateVersionStatus(ctx, "missing", functiondomain.StatusRejected, nil); !errors.Is(err, functiondomain.ErrVersionNotFound) {
		t.Errorf("expected ErrVersionNotFound, got: %v", err)
	}
}

func TestUpdateVersionEnv(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	_ = s.SaveFunction(ctx, mkFunction("fn1", userAlice, "to-pdf"))
	_ = s.SaveVersion(ctx, mkVersion("fnv1", "fn1", functiondomain.StatusPending))

	now := time.Now().UTC()
	if err := s.UpdateVersionEnv(ctx, "fnv1", functiondomain.EnvStatusReady, "", "installing", "deps installed", &now); err != nil {
		t.Fatalf("UpdateVersionEnv: %v", err)
	}

	got, _ := s.GetVersion(ctx, "fnv1")
	if got.EnvStatus != functiondomain.EnvStatusReady {
		t.Errorf("EnvStatus = %q, want ready", got.EnvStatus)
	}
	if got.EnvSyncStage != "installing" {
		t.Errorf("EnvSyncStage = %q, want installing", got.EnvSyncStage)
	}
}

func TestListVersions_FilterByStatus(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	_ = s.SaveFunction(ctx, mkFunction("fn1", userAlice, "to-pdf"))

	for i := 0; i < 3; i++ {
		v := mkVersion(fmt.Sprintf("fnv-acc-%d", i), "fn1", functiondomain.StatusAccepted)
		_ = s.SaveVersion(ctx, v)
	}
	for i := 0; i < 2; i++ {
		v := mkVersion(fmt.Sprintf("fnv-pen-%d", i), "fn1", functiondomain.StatusPending)
		_ = s.SaveVersion(ctx, v)
	}

	accepted, _, err := s.ListVersions(ctx, "fn1", functiondomain.VersionListFilter{Status: functiondomain.StatusAccepted})
	if err != nil {
		t.Fatalf("ListVersions accepted: %v", err)
	}
	if len(accepted) != 3 {
		t.Errorf("accepted: want 3, got %d", len(accepted))
	}

	pending, _, _ := s.ListVersions(ctx, "fn1", functiondomain.VersionListFilter{Status: functiondomain.StatusPending})
	if len(pending) != 2 {
		t.Errorf("pending: want 2, got %d", len(pending))
	}

	all, _, _ := s.ListVersions(ctx, "fn1", functiondomain.VersionListFilter{})
	if len(all) != 5 {
		t.Errorf("all: want 5, got %d", len(all))
	}
}

func TestHardDeleteOldestAccepted_KeepsNewest(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	_ = s.SaveFunction(ctx, mkFunction("fn1", userAlice, "to-pdf"))

	for i := 0; i < 7; i++ {
		v := mkVersion(fmt.Sprintf("fnv%d", i), "fn1", functiondomain.StatusAccepted)
		v.CreatedAt = time.Now().UTC().Add(time.Duration(i) * time.Second)
		n := i + 1
		v.Version = &n
		_ = s.SaveVersion(ctx, v)
	}

	if err := s.HardDeleteOldestAccepted(ctx, "fn1", 3); err != nil {
		t.Fatalf("HardDeleteOldestAccepted: %v", err)
	}

	rows, _, _ := s.ListVersions(ctx, "fn1", functiondomain.VersionListFilter{})
	if len(rows) != 3 {
		t.Errorf("want 3 versions after prune, got %d", len(rows))
	}

	wantIDs := map[string]bool{"fnv6": true, "fnv5": true, "fnv4": true}
	for _, r := range rows {
		if !wantIDs[r.ID] {
			t.Errorf("unexpected surviving version: %s", r.ID)
		}
	}
}

func TestHardDeleteOldestAccepted_NoOpUnderCap(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	_ = s.SaveFunction(ctx, mkFunction("fn1", userAlice, "to-pdf"))

	for i := 0; i < 3; i++ {
		v := mkVersion(fmt.Sprintf("fnv%d", i), "fn1", functiondomain.StatusAccepted)
		_ = s.SaveVersion(ctx, v)
	}

	if err := s.HardDeleteOldestAccepted(ctx, "fn1", 5); err != nil {
		t.Fatalf("HardDeleteOldestAccepted: %v", err)
	}

	rows, _, _ := s.ListVersions(ctx, "fn1", functiondomain.VersionListFilter{})
	if len(rows) != 3 {
		t.Errorf("expected 3 rows unchanged, got %d", len(rows))
	}
}

func TestDeleteFunction_NotFound(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	if err := s.DeleteFunction(ctx, "missing"); !errors.Is(err, functiondomain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

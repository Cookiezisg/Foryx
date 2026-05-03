// Package forge — integration tests for Store using an in-memory SQLite.
// Covers CRUD, user scoping, version/pending lifecycle, unified execution
// history (run+test), cursor pagination, and the interface satisfaction
// compile-time check.
//
// Package forge — Store 集成测试（内存 SQLite）。
// 覆盖 CRUD、用户隔离、版本/pending 生命周期、统一执行历史（run+test）、
// cursor 分页、接口满足检查。
package forge

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	gormlogger "gorm.io/gorm/logger"

	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// compile-time interface satisfaction check.
var _ forgedomain.Repository = (*Store)(nil)

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
	if err := dbinfra.Migrate(database,
		&forgedomain.Forge{},
		&forgedomain.ForgeVersion{},
		&forgedomain.ForgeTestCase{},
		&forgedomain.ForgeExecution{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(database)
}

func ctxFor(userID string) context.Context {
	return reqctxpkg.SetUserID(context.Background(), userID)
}

func mkForge(id, userID, name string) *forgedomain.Forge {
	return &forgedomain.Forge{
		ID:           id,
		UserID:       userID,
		Name:         name,
		Description:  "desc " + name,
		Code:         "def " + name + "(): pass",
		Parameters:   "[]",
		ReturnSchema: "{}",
		Tags:         "[]",
		VersionCount: 1,
	}
}

// ── Forge CRUD ─────────────────────────────────────────────────────────────────

func TestSaveAndGetForge(t *testing.T) {
	s := newStore(t)
	f := mkForge("f_001", userAlice, "parse_csv")
	if err := s.SaveForge(ctxFor(userAlice), f); err != nil {
		t.Fatalf("SaveForge: %v", err)
	}
	got, err := s.GetForge(ctxFor(userAlice), "f_001")
	if err != nil {
		t.Fatalf("GetForge: %v", err)
	}
	if got.Name != "parse_csv" {
		t.Errorf("name: want parse_csv, got %s", got.Name)
	}
}

func TestGetForge_NotFound(t *testing.T) {
	s := newStore(t)
	_, err := s.GetForge(ctxFor(userAlice), "f_missing")
	if !errors.Is(err, forgedomain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetForge_UserIsolation(t *testing.T) {
	s := newStore(t)
	if err := s.SaveForge(ctxFor(userAlice), mkForge("f_001", userAlice, "forge")); err != nil {
		t.Fatal(err)
	}
	_, err := s.GetForge(ctxFor(userBob), "f_001")
	if !errors.Is(err, forgedomain.ErrNotFound) {
		t.Errorf("Bob should not see Alice's forge, got %v", err)
	}
}

func TestDeleteForge_SoftDelete(t *testing.T) {
	s := newStore(t)
	if err := s.SaveForge(ctxFor(userAlice), mkForge("f_001", userAlice, "forge")); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteForge(ctxFor(userAlice), "f_001"); err != nil {
		t.Fatalf("DeleteForge: %v", err)
	}
	_, err := s.GetForge(ctxFor(userAlice), "f_001")
	if !errors.Is(err, forgedomain.ErrNotFound) {
		t.Errorf("deleted forge should not be found, got %v", err)
	}
}

func TestListAllForges(t *testing.T) {
	s := newStore(t)
	for _, name := range []string{"forge_a", "forge_b", "forge_c"} {
		if err := s.SaveForge(ctxFor(userAlice), mkForge("f_"+name, userAlice, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SaveForge(ctxFor(userBob), mkForge("f_bob", userBob, "bob_forge")); err != nil {
		t.Fatal(err)
	}
	forges, err := s.ListAllForges(ctxFor(userAlice))
	if err != nil {
		t.Fatalf("ListAllForges: %v", err)
	}
	if len(forges) != 3 {
		t.Errorf("want 3 forges, got %d", len(forges))
	}
}

func TestGetForgesByIDs_OrderPreserved(t *testing.T) {
	s := newStore(t)
	for _, id := range []string{"f_1", "f_2", "f_3"} {
		if err := s.SaveForge(ctxFor(userAlice), mkForge(id, userAlice, "forge_"+id)); err != nil {
			t.Fatal(err)
		}
	}
	forges, err := s.GetForgesByIDs(ctxFor(userAlice), []string{"f_3", "f_1"})
	if err != nil {
		t.Fatalf("GetForgesByIDs: %v", err)
	}
	if len(forges) != 2 || forges[0].ID != "f_3" || forges[1].ID != "f_1" {
		t.Errorf("order not preserved: %v", forges)
	}
}

// ── Versions ─────────────────────────────────────────────────────────────────

func mkVersion(id, forgeID, userID, status string, version *int) *forgedomain.ForgeVersion {
	return &forgedomain.ForgeVersion{
		ID:           id,
		ForgeID:      forgeID,
		UserID:       userID,
		Version:      version,
		Status:       status,
		Name:         "forge",
		Code:         "def forge(): pass",
		ChangeReason: "initial",
	}
}

func intPtr(n int) *int { return &n }

func TestVersionLifecycle(t *testing.T) {
	s := newStore(t)
	if err := s.SaveForge(ctxFor(userAlice), mkForge("f_001", userAlice, "forge")); err != nil {
		t.Fatal(err)
	}

	pending := mkVersion("fv_p1", "f_001", userAlice, forgedomain.VersionStatusPending, nil)
	if err := s.SaveVersion(ctxFor(userAlice), pending); err != nil {
		t.Fatalf("SaveVersion pending: %v", err)
	}

	got, err := s.GetActivePending(ctxFor(userAlice), "f_001")
	if err != nil {
		t.Fatalf("GetActivePending: %v", err)
	}
	if got.ID != "fv_p1" {
		t.Errorf("want fv_p1, got %s", got.ID)
	}

	if err := s.UpdateVersionStatus(ctxFor(userAlice), "fv_p1", forgedomain.VersionStatusAccepted, intPtr(1)); err != nil {
		t.Fatalf("UpdateVersionStatus: %v", err)
	}

	_, err = s.GetActivePending(ctxFor(userAlice), "f_001")
	if !errors.Is(err, forgedomain.ErrPendingNotFound) {
		t.Errorf("expected ErrPendingNotFound after accept, got %v", err)
	}

	v, err := s.GetVersion(ctxFor(userAlice), "f_001", 1)
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if *v.Version != 1 {
		t.Errorf("want version=1, got %d", *v.Version)
	}
}

func TestDeleteOldestAcceptedVersion(t *testing.T) {
	s := newStore(t)
	if err := s.SaveForge(ctxFor(userAlice), mkForge("f_001", userAlice, "forge")); err != nil {
		t.Fatal(err)
	}
	for i, vid := range []string{"fv_v1", "fv_v2", "fv_v3"} {
		v := mkVersion(vid, "f_001", userAlice, forgedomain.VersionStatusAccepted, intPtr(i+1))
		v.CreatedAt = time.Now().Add(time.Duration(i) * time.Second)
		if err := s.SaveVersion(ctxFor(userAlice), v); err != nil {
			t.Fatal(err)
		}
	}
	n, _ := s.CountAcceptedVersions(ctxFor(userAlice), "f_001")
	if n != 3 {
		t.Fatalf("want 3 versions, got %d", n)
	}
	if err := s.DeleteOldestAcceptedVersion(ctxFor(userAlice), "f_001"); err != nil {
		t.Fatalf("DeleteOldestAcceptedVersion: %v", err)
	}
	n, _ = s.CountAcceptedVersions(ctxFor(userAlice), "f_001")
	if n != 2 {
		t.Errorf("want 2 versions after delete, got %d", n)
	}
	_, err := s.GetVersion(ctxFor(userAlice), "f_001", 1)
	if !errors.Is(err, forgedomain.ErrVersionNotFound) {
		t.Errorf("v1 should be deleted, got %v", err)
	}
}

// ── Test cases ────────────────────────────────────────────────────────────────

func TestTestCaseCRUD(t *testing.T) {
	s := newStore(t)
	if err := s.SaveForge(ctxFor(userAlice), mkForge("f_001", userAlice, "forge")); err != nil {
		t.Fatal(err)
	}
	tc := &forgedomain.ForgeTestCase{
		ID:             "tc_001",
		ForgeID:        "f_001",
		UserID:         userAlice,
		Name:           "basic",
		InputData:      `{"x":1}`,
		ExpectedOutput: `2`,
	}
	if err := s.SaveTestCase(ctxFor(userAlice), tc); err != nil {
		t.Fatalf("SaveTestCase: %v", err)
	}
	got, err := s.GetTestCase(ctxFor(userAlice), "tc_001")
	if err != nil {
		t.Fatalf("GetTestCase: %v", err)
	}
	if got.Name != "basic" {
		t.Errorf("want name=basic, got %s", got.Name)
	}
	if err := s.DeleteTestCase(ctxFor(userAlice), "tc_001"); err != nil {
		t.Fatalf("DeleteTestCase: %v", err)
	}
	_, err = s.GetTestCase(ctxFor(userAlice), "tc_001")
	if !errors.Is(err, forgedomain.ErrTestCaseNotFound) {
		t.Errorf("expected ErrTestCaseNotFound after delete, got %v", err)
	}
}

// ── Executions (unified run + test history) ───────────────────────────────────

func mkExecution(id, forgeID, userID, kind string, t time.Time) *forgedomain.ForgeExecution {
	return &forgedomain.ForgeExecution{
		ID:           id,
		ForgeID:      forgeID,
		UserID:       userID,
		ForgeVersion: 1,
		Kind:         kind,
		Input:        "{}",
		OK:           true,
		TriggeredBy:  forgedomain.TriggeredByHTTP,
		CreatedAt:    t,
	}
}

func TestExecutionRetention(t *testing.T) {
	s := newStore(t)
	if err := s.SaveForge(ctxFor(userAlice), mkForge("f_001", userAlice, "forge")); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		e := mkExecution(fmt.Sprintf("fe_%02d", i), "f_001", userAlice,
			forgedomain.ExecutionKindRun, time.Now().Add(time.Duration(i)*time.Second))
		if err := s.SaveExecution(ctxFor(userAlice), e); err != nil {
			t.Fatalf("SaveExecution: %v", err)
		}
	}
	n, err := s.CountExecutions(ctxFor(userAlice), "f_001")
	if err != nil || n != 3 {
		t.Fatalf("want count=3, got %d, err=%v", n, err)
	}
	if err := s.DeleteOldestExecution(ctxFor(userAlice), "f_001"); err != nil {
		t.Fatalf("DeleteOldestExecution: %v", err)
	}
	n, _ = s.CountExecutions(ctxFor(userAlice), "f_001")
	if n != 2 {
		t.Errorf("want 2 after delete, got %d", n)
	}
}

func TestExecutionFilter_KindAndBatch(t *testing.T) {
	s := newStore(t)
	if err := s.SaveForge(ctxFor(userAlice), mkForge("f_001", userAlice, "forge")); err != nil {
		t.Fatal(err)
	}
	pass := true
	now := time.Now()
	// 2 run rows + 3 test rows in one batch.
	// 2 行 run + 3 行同批次 test。
	for i := range 2 {
		e := mkExecution(fmt.Sprintf("fe_run_%02d", i), "f_001", userAlice,
			forgedomain.ExecutionKindRun, now.Add(time.Duration(i)*time.Second))
		if err := s.SaveExecution(ctxFor(userAlice), e); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 3 {
		e := mkExecution(fmt.Sprintf("fe_test_%02d", i), "f_001", userAlice,
			forgedomain.ExecutionKindTest, now.Add(time.Duration(10+i)*time.Second))
		e.TestCaseID = fmt.Sprintf("tc_%02d", i)
		e.BatchID = "batch_001"
		e.Pass = &pass
		if err := s.SaveExecution(ctxFor(userAlice), e); err != nil {
			t.Fatal(err)
		}
	}

	// Kind filter.
	runs, _, err := s.ListExecutions(ctxFor(userAlice), forgedomain.ExecutionFilter{
		ForgeID: "f_001", Kind: forgedomain.ExecutionKindRun,
	})
	if err != nil {
		t.Fatalf("ListExecutions kind=run: %v", err)
	}
	if len(runs) != 2 {
		t.Errorf("want 2 run rows, got %d", len(runs))
	}

	// Batch filter — expect ASC ordering.
	// batch 过滤——预期 ASC 排序。
	batch, _, err := s.ListExecutions(ctxFor(userAlice), forgedomain.ExecutionFilter{
		ForgeID: "f_001", BatchID: "batch_001",
	})
	if err != nil {
		t.Fatalf("ListExecutions batch: %v", err)
	}
	if len(batch) != 3 {
		t.Fatalf("want 3 batch rows, got %d", len(batch))
	}
	if batch[0].ID != "fe_test_00" || batch[2].ID != "fe_test_02" {
		t.Errorf("batch should be ASC, got order %s, %s, %s",
			batch[0].ID, batch[1].ID, batch[2].ID)
	}
}

func TestExecutionFilter_ChatContext(t *testing.T) {
	s := newStore(t)
	if err := s.SaveForge(ctxFor(userAlice), mkForge("f_001", userAlice, "forge")); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i := range 3 {
		e := mkExecution(fmt.Sprintf("fe_%02d", i), "f_001", userAlice,
			forgedomain.ExecutionKindRun, now.Add(time.Duration(i)*time.Second))
		e.TriggeredBy = forgedomain.TriggeredByChat
		e.ConversationID = "cv_xyz"
		e.MessageID = fmt.Sprintf("msg_%02d", i)
		if err := s.SaveExecution(ctxFor(userAlice), e); err != nil {
			t.Fatal(err)
		}
	}
	// Filter by conversation: all 3.
	conv, _, err := s.ListExecutions(ctxFor(userAlice), forgedomain.ExecutionFilter{
		ConversationID: "cv_xyz",
	})
	if err != nil {
		t.Fatalf("ListExecutions conv: %v", err)
	}
	if len(conv) != 3 {
		t.Errorf("want 3 by conversation, got %d", len(conv))
	}
	// Filter by single message.
	msg, _, err := s.ListExecutions(ctxFor(userAlice), forgedomain.ExecutionFilter{
		MessageID: "msg_01",
	})
	if err != nil {
		t.Fatalf("ListExecutions msg: %v", err)
	}
	if len(msg) != 1 || msg[0].ID != "fe_01" {
		t.Errorf("want 1 row fe_01, got %v", msg)
	}
}

func TestExecutionPagination_Cursor(t *testing.T) {
	s := newStore(t)
	if err := s.SaveForge(ctxFor(userAlice), mkForge("f_001", userAlice, "forge")); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i := range 5 {
		e := mkExecution(fmt.Sprintf("fe_%02d", i), "f_001", userAlice,
			forgedomain.ExecutionKindRun, now.Add(time.Duration(i)*time.Second))
		if err := s.SaveExecution(ctxFor(userAlice), e); err != nil {
			t.Fatal(err)
		}
	}
	// First page (DESC; newest first): limit=2 → fe_04, fe_03.
	page1, next, err := s.ListExecutions(ctxFor(userAlice), forgedomain.ExecutionFilter{
		ForgeID: "f_001", Limit: 2,
	})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1) != 2 || page1[0].ID != "fe_04" || page1[1].ID != "fe_03" {
		t.Fatalf("page1 wrong: %v", page1)
	}
	if next == "" {
		t.Fatal("expected nextCursor on page1")
	}
	// Second page: fe_02, fe_01.
	page2, next2, err := s.ListExecutions(ctxFor(userAlice), forgedomain.ExecutionFilter{
		ForgeID: "f_001", Limit: 2, Cursor: next,
	})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 2 || page2[0].ID != "fe_02" || page2[1].ID != "fe_01" {
		t.Errorf("page2 wrong: %v", page2)
	}
	if next2 == "" {
		t.Fatal("expected nextCursor on page2")
	}
	// Final page: fe_00, no nextCursor.
	page3, next3, err := s.ListExecutions(ctxFor(userAlice), forgedomain.ExecutionFilter{
		ForgeID: "f_001", Limit: 2, Cursor: next2,
	})
	if err != nil {
		t.Fatalf("page3: %v", err)
	}
	if len(page3) != 1 || page3[0].ID != "fe_00" {
		t.Errorf("page3 wrong: %v", page3)
	}
	if next3 != "" {
		t.Errorf("expected empty cursor on final page, got %q", next3)
	}
}

// ── Sandbox iteration: ActiveVersionID + Env* on ForgeVersion ─────────────────

func mkEnvVersion(id, forgeID, userID, envID, status string) *forgedomain.ForgeVersion {
	v := mkVersion(id, forgeID, userID, status, nil)
	v.Dependencies = "[]"
	v.EnvID = envID
	v.EnvStatus = forgedomain.EnvStatusPending
	return v
}

func TestUpdateForgeActiveVersion(t *testing.T) {
	s := newStore(t)
	if err := s.SaveForge(ctxFor(userAlice), mkForge("f_001", userAlice, "x")); err != nil {
		t.Fatal(err)
	}

	if err := s.UpdateForgeActiveVersion(ctxFor(userAlice), "f_001", "fv_v1"); err != nil {
		t.Fatalf("UpdateForgeActiveVersion: %v", err)
	}
	got, err := s.GetForge(ctxFor(userAlice), "f_001")
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveVersionID != "fv_v1" {
		t.Errorf("ActiveVersionID = %q, want fv_v1", got.ActiveVersionID)
	}

	// Cross-user mutation must be blocked by user_id WHERE clause.
	// 跨用户修改必须被 user_id 谓词挡掉。
	if err := s.UpdateForgeActiveVersion(ctxFor(userBob), "f_001", "fv_evil"); err != nil {
		t.Fatalf("UpdateForgeActiveVersion (bob): %v", err)
	}
	got, _ = s.GetForge(ctxFor(userAlice), "f_001")
	if got.ActiveVersionID != "fv_v1" {
		t.Errorf("Bob should not be able to mutate Alice's forge; got ActiveVersionID=%q", got.ActiveVersionID)
	}
}

func TestGetVersionByID_FoundAndNotFound(t *testing.T) {
	s := newStore(t)
	if err := s.SaveVersion(ctxFor(userAlice), mkEnvVersion("fv_a", "f_001", userAlice, "env_aaa", forgedomain.VersionStatusPending)); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetVersionByID(ctxFor(userAlice), "fv_a")
	if err != nil {
		t.Fatalf("GetVersionByID: %v", err)
	}
	if got.EnvID != "env_aaa" {
		t.Errorf("EnvID round-trip wrong: %q", got.EnvID)
	}

	// Missing → ErrVersionNotFound.
	if _, err := s.GetVersionByID(ctxFor(userAlice), "fv_missing"); !errors.Is(err, forgedomain.ErrVersionNotFound) {
		t.Errorf("expected ErrVersionNotFound, got %v", err)
	}

	// Cross-user → ErrVersionNotFound (user_id scoping).
	if _, err := s.GetVersionByID(ctxFor(userBob), "fv_a"); !errors.Is(err, forgedomain.ErrVersionNotFound) {
		t.Errorf("Bob should not see Alice's version; got %v", err)
	}
}

func TestUpdateVersionEnvStatus_ReadyStampsSyncedAt(t *testing.T) {
	s := newStore(t)
	v := mkEnvVersion("fv_a", "f_001", userAlice, "env_aaa", forgedomain.VersionStatusPending)
	if err := s.SaveVersion(ctxFor(userAlice), v); err != nil {
		t.Fatal(err)
	}

	before := time.Now().UTC()
	if err := s.UpdateVersionEnvStatus(ctxFor(userAlice), "fv_a", forgedomain.EnvStatusReady, ""); err != nil {
		t.Fatalf("UpdateVersionEnvStatus ready: %v", err)
	}
	after := time.Now().UTC()

	got, _ := s.GetVersionByID(ctxFor(userAlice), "fv_a")
	if got.EnvStatus != forgedomain.EnvStatusReady {
		t.Errorf("EnvStatus = %q, want ready", got.EnvStatus)
	}
	if got.EnvSyncedAt == nil {
		t.Fatal("EnvSyncedAt should be set when transitioning to ready")
	}
	if got.EnvSyncedAt.Before(before.Add(-time.Second)) || got.EnvSyncedAt.After(after.Add(time.Second)) {
		t.Errorf("EnvSyncedAt out of expected window: %v", got.EnvSyncedAt)
	}
	if got.EnvError != "" {
		t.Errorf("EnvError should be empty for ready, got %q", got.EnvError)
	}
}

func TestUpdateVersionEnvStatus_FailedCarriesError(t *testing.T) {
	s := newStore(t)
	if err := s.SaveVersion(ctxFor(userAlice), mkEnvVersion("fv_a", "f_001", userAlice, "env_aaa", forgedomain.VersionStatusPending)); err != nil {
		t.Fatal(err)
	}

	stderr := "× No solution found"
	if err := s.UpdateVersionEnvStatus(ctxFor(userAlice), "fv_a", forgedomain.EnvStatusFailed, stderr); err != nil {
		t.Fatalf("UpdateVersionEnvStatus failed: %v", err)
	}

	got, _ := s.GetVersionByID(ctxFor(userAlice), "fv_a")
	if got.EnvStatus != forgedomain.EnvStatusFailed {
		t.Errorf("EnvStatus = %q, want failed", got.EnvStatus)
	}
	if got.EnvError != stderr {
		t.Errorf("EnvError = %q, want %q", got.EnvError, stderr)
	}
	if got.EnvSyncedAt != nil {
		t.Errorf("EnvSyncedAt should be nil for failed, got %v", got.EnvSyncedAt)
	}
}

func TestUpdateVersionEnvStatus_SyncingResetsSyncedAt(t *testing.T) {
	s := newStore(t)
	if err := s.SaveVersion(ctxFor(userAlice), mkEnvVersion("fv_a", "f_001", userAlice, "env_aaa", forgedomain.VersionStatusPending)); err != nil {
		t.Fatal(err)
	}
	// First → ready (sets EnvSyncedAt)
	_ = s.UpdateVersionEnvStatus(ctxFor(userAlice), "fv_a", forgedomain.EnvStatusReady, "")
	// Then re-sync: ready → syncing, EnvSyncedAt should clear.
	_ = s.UpdateVersionEnvStatus(ctxFor(userAlice), "fv_a", forgedomain.EnvStatusSyncing, "")

	got, _ := s.GetVersionByID(ctxFor(userAlice), "fv_a")
	if got.EnvStatus != forgedomain.EnvStatusSyncing {
		t.Errorf("EnvStatus = %q, want syncing", got.EnvStatus)
	}
	if got.EnvSyncedAt != nil {
		t.Errorf("syncing should clear EnvSyncedAt, got %v", got.EnvSyncedAt)
	}
}

func TestUpdateVersionEnvProgress(t *testing.T) {
	s := newStore(t)
	if err := s.SaveVersion(ctxFor(userAlice), mkEnvVersion("fv_a", "f_001", userAlice, "env_aaa", forgedomain.VersionStatusPending)); err != nil {
		t.Fatal(err)
	}

	if err := s.UpdateVersionEnvProgress(ctxFor(userAlice), "fv_a", "preparing", "Prepared 12 packages"); err != nil {
		t.Fatalf("UpdateVersionEnvProgress: %v", err)
	}

	got, _ := s.GetVersionByID(ctxFor(userAlice), "fv_a")
	if got.EnvSyncStage != "preparing" {
		t.Errorf("EnvSyncStage = %q, want preparing", got.EnvSyncStage)
	}
	if got.EnvSyncDetail != "Prepared 12 packages" {
		t.Errorf("EnvSyncDetail = %q, want 'Prepared 12 packages'", got.EnvSyncDetail)
	}
}

func TestUpdateVersionEnvID_OnPendingWorks(t *testing.T) {
	s := newStore(t)
	if err := s.SaveVersion(ctxFor(userAlice), mkEnvVersion("fv_a", "f_001", userAlice, "env_aaa", forgedomain.VersionStatusPending)); err != nil {
		t.Fatal(err)
	}

	if err := s.UpdateVersionEnvID(ctxFor(userAlice), "fv_a", "env_bbb"); err != nil {
		t.Fatalf("UpdateVersionEnvID: %v", err)
	}
	got, _ := s.GetVersionByID(ctxFor(userAlice), "fv_a")
	if got.EnvID != "env_bbb" {
		t.Errorf("EnvID = %q, want env_bbb", got.EnvID)
	}
}

func TestUpdateVersionEnvID_RefusesAccepted(t *testing.T) {
	s := newStore(t)
	if err := s.SaveVersion(ctxFor(userAlice), mkEnvVersion("fv_a", "f_001", userAlice, "env_aaa", forgedomain.VersionStatusAccepted)); err != nil {
		t.Fatal(err)
	}

	// Should be a silent no-op (RowsAffected=0) — accepted is immutable.
	// 应静默 no-op（RowsAffected=0）——accepted 不可变。
	if err := s.UpdateVersionEnvID(ctxFor(userAlice), "fv_a", "env_bbb"); err != nil {
		t.Fatalf("UpdateVersionEnvID: %v", err)
	}
	got, _ := s.GetVersionByID(ctxFor(userAlice), "fv_a")
	if got.EnvID != "env_aaa" {
		t.Errorf("accepted version's EnvID should be unchanged, got %q", got.EnvID)
	}
}

func TestListEnvIDsForForge_OrderByRecency(t *testing.T) {
	s := newStore(t)

	// Three versions with three different EnvIDs, saved in order so
	// updated_at strictly increases.
	// 三个版本对应三个不同 EnvID，按顺序保存以保证 updated_at 严格递增。
	versions := []struct {
		id, envID string
	}{
		{"fv_old", "env_aaa"}, // saved first → oldest
		{"fv_mid", "env_bbb"},
		{"fv_new", "env_ccc"}, // saved last → newest
	}
	for _, v := range versions {
		if err := s.SaveVersion(ctxFor(userAlice), mkEnvVersion(v.id, "f_001", userAlice, v.envID, forgedomain.VersionStatusAccepted)); err != nil {
			t.Fatal(err)
		}
		// SQLite millisecond resolution; force a gap to make recency
		// ordering deterministic.
		// SQLite 毫秒精度；强制间隔保证 recency 排序确定。
		time.Sleep(2 * time.Millisecond)
	}

	got, err := s.ListEnvIDsForForge(ctxFor(userAlice), "f_001")
	if err != nil {
		t.Fatalf("ListEnvIDsForForge: %v", err)
	}
	want := []string{"env_ccc", "env_bbb", "env_aaa"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (got %v)", len(got), len(want), got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestListEnvIDsForForge_DistinctsAndExcludesEmpty(t *testing.T) {
	s := newStore(t)

	// Two versions sharing same EnvID; one with empty EnvID (legacy / future
	// pending pre-EnvID-compute).
	// 两个版本共用同一个 EnvID；一个 EnvID 为空（legacy / pending 在算
	// EnvID 之前）。
	rows := []struct{ id, env string }{
		{"fv_1", "env_shared"},
		{"fv_2", "env_shared"},
		{"fv_3", ""},
	}
	for _, r := range rows {
		if err := s.SaveVersion(ctxFor(userAlice), mkEnvVersion(r.id, "f_001", userAlice, r.env, forgedomain.VersionStatusAccepted)); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ListEnvIDsForForge(ctxFor(userAlice), "f_001")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "env_shared" {
		t.Errorf("expected exactly [env_shared], got %v", got)
	}
}

func TestListEnvIDsForForge_UserScoping(t *testing.T) {
	s := newStore(t)
	_ = s.SaveVersion(ctxFor(userAlice), mkEnvVersion("fv_a", "f_001", userAlice, "env_alice", forgedomain.VersionStatusAccepted))
	_ = s.SaveVersion(ctxFor(userBob), mkEnvVersion("fv_b", "f_001", userBob, "env_bob", forgedomain.VersionStatusAccepted))

	got, err := s.ListEnvIDsForForge(ctxFor(userAlice), "f_001")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "env_alice" {
		t.Errorf("Alice should only see env_alice, got %v", got)
	}
}

// Ensure the test file references the sandbox iteration domain additions —
// guards against accidental removal of new fields / constants.
//
// 引用沙箱迭代加的 domain 字段 / 常量——防误删。
var _ = []any{
	forgedomain.EnvStatusPending,
	forgedomain.EnvStatusSyncing,
	forgedomain.EnvStatusReady,
	forgedomain.EnvStatusFailed,
	forgedomain.EnvStatusEvicted,
	forgedomain.MaxEnvIDsPerForge,
	forgedomain.DefaultPythonVersion,
	forgedomain.ErrEnvNotReady,
	forgedomain.ErrEnvFailed,
	forgedomain.ErrSandboxUnavailable,
	forgedomain.ErrDependencyResolution,
}

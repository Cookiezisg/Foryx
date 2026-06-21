package function

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite"

	functiondomain "github.com/sunweilin/anselm/backend/internal/domain/function"
	ormpkg "github.com/sunweilin/anselm/backend/internal/pkg/orm"
	reqctxpkg "github.com/sunweilin/anselm/backend/internal/pkg/reqctx"
	schemapkg "github.com/sunweilin/anselm/backend/internal/pkg/schema"
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

func ctxWS(id string) context.Context { return reqctxpkg.SetWorkspaceID(context.Background(), id) }

func mkFn(t *testing.T, s *Store, ctx context.Context, id, name, activeVer string) {
	t.Helper()
	if err := s.SaveFunction(ctx, &functiondomain.Function{ID: id, Name: name, ActiveVersionID: activeVer, Tags: []string{}}); err != nil {
		t.Fatalf("SaveFunction %s: %v", id, err)
	}
}

func mkVer(t *testing.T, s *Store, ctx context.Context, id, fnID string, n int) {
	t.Helper()
	v := &functiondomain.Version{
		ID: id, FunctionID: fnID, Version: n, Code: "def main():\n    return 1",
		Inputs:       []schemapkg.Field{{Name: "x", Type: schemapkg.TypeString}},
		Outputs:      []schemapkg.Field{{Name: "y", Type: schemapkg.TypeNumber}},
		Dependencies: []string{}, EnvStatus: functiondomain.EnvStatusPending,
		EnvID: "env_" + id, // deterministic per-version env so trim's returned reclaim list is assertable
	}
	if err := s.SaveVersion(ctx, v); err != nil {
		t.Fatalf("SaveVersion %s: %v", id, err)
	}
}

func TestFunction_RoundTrip_WorkspaceFilled(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkFn(t, s, ctx, "fn_1", "alpha", "")
	got, err := s.GetFunction(ctx, "fn_1")
	if err != nil {
		t.Fatalf("GetFunction: %v", err)
	}
	if got.Name != "alpha" || got.WorkspaceID != "ws_1" {
		t.Fatalf("round-trip: %+v", got)
	}
	if got.CreatedAt.IsZero() {
		t.Error("created_at not auto-stamped")
	}
}

func TestFunction_DuplicateName(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkFn(t, s, ctx, "fn_1", "dup", "")
	err := s.SaveFunction(ctx, &functiondomain.Function{ID: "fn_2", Name: "dup", Tags: []string{}})
	if !errors.Is(err, functiondomain.ErrDuplicateName) {
		t.Fatalf("want ErrDuplicateName, got %v", err)
	}
}

// TestFunction_VersionConflict — F73: a concurrent :edit that races the (function_id, version) unique
// index gets the specific FUNCTION_VERSION_CONFLICT, not the generic orm ORM_CONFLICT fallback. The
// same translation is applied verbatim in all 6 versioned-entity stores' version Save.
func TestFunction_VersionConflict(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkFn(t, s, ctx, "fn_1", "a", "")
	mkVer(t, s, ctx, "fnv_1", "fn_1", 1)
	f, err := s.GetFunction(ctx, "fn_1")
	if err != nil {
		t.Fatalf("GetFunction: %v", err)
	}
	v2 := &functiondomain.Version{
		ID: "fnv_2", FunctionID: "fn_1", Version: 1, Code: "def main():\n    return 2",
		Inputs: []schemapkg.Field{}, Outputs: []schemapkg.Field{}, Dependencies: []string{}, EnvStatus: functiondomain.EnvStatusPending,
	}
	if err := s.SaveVersionAndActivate(ctx, v2, f); !errors.Is(err, functiondomain.ErrVersionConflict) {
		t.Fatalf("a same-number version save should be ErrVersionConflict, got %v", err)
	}
}

func TestFunction_WorkspaceIsolation(t *testing.T) {
	s := newStore(t)
	mkFn(t, s, ctxWS("ws_1"), "fn_1", "a", "")
	// Same name allowed in another workspace, and ws_2 cannot see ws_1's row.
	mkFn(t, s, ctxWS("ws_2"), "fn_2", "a", "")
	if _, err := s.GetFunction(ctxWS("ws_2"), "fn_1"); !errors.Is(err, functiondomain.ErrNotFound) {
		t.Fatalf("cross-workspace read should be NotFound, got %v", err)
	}
}

func TestFunction_SoftDelete(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkFn(t, s, ctx, "fn_1", "a", "")
	if err := s.DeleteFunction(ctx, "fn_1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetFunction(ctx, "fn_1"); !errors.Is(err, functiondomain.ErrNotFound) {
		t.Fatalf("deleted function should be NotFound, got %v", err)
	}
	if err := s.DeleteFunction(ctx, "fn_1"); !errors.Is(err, functiondomain.ErrNotFound) {
		t.Fatalf("re-delete should be NotFound, got %v", err)
	}
}

func TestFunction_ListPagination(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	for i := range 5 {
		mkFn(t, s, ctx, "fn_"+string(rune('a'+i)), "n"+string(rune('a'+i)), "")
		time.Sleep(time.Millisecond) // distinct created_at for stable keyset
	}
	page1, next, err := s.ListFunctions(ctx, functiondomain.ListFilter{Limit: 2})
	if err != nil || len(page1) != 2 || next == "" {
		t.Fatalf("page1: rows=%d next=%q err=%v", len(page1), next, err)
	}
	page2, _, err := s.ListFunctions(ctx, functiondomain.ListFilter{Limit: 2, Cursor: next})
	if err != nil || len(page2) != 2 {
		t.Fatalf("page2: rows=%d err=%v", len(page2), err)
	}
	if page1[0].ID == page2[0].ID {
		t.Fatal("pages overlap")
	}
}

func TestVersion_MaxAndByNumber(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkFn(t, s, ctx, "fn_1", "a", "")
	if n, err := s.MaxVersionNumber(ctx, "fn_1"); err != nil || n != 0 {
		t.Fatalf("max with no versions: n=%d err=%v", n, err)
	}
	mkVer(t, s, ctx, "fnv_1", "fn_1", 1)
	mkVer(t, s, ctx, "fnv_2", "fn_1", 2)
	if n, err := s.MaxVersionNumber(ctx, "fn_1"); err != nil || n != 2 {
		t.Fatalf("max: n=%d err=%v", n, err)
	}
	v, err := s.GetVersionByNumber(ctx, "fn_1", 2)
	if err != nil || v.ID != "fnv_2" {
		t.Fatalf("by number: %+v err=%v", v, err)
	}
	if _, err := s.GetVersionByNumber(ctx, "fn_1", 9); !errors.Is(err, functiondomain.ErrVersionNotFound) {
		t.Fatalf("missing number should be ErrVersionNotFound, got %v", err)
	}
}

func TestVersion_UpdateEnvRewritesDeps(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkFn(t, s, ctx, "fn_1", "a", "")
	mkVer(t, s, ctx, "fnv_1", "fn_1", 1)
	now := time.Now().UTC()
	if err := s.UpdateVersionEnv(ctx, "fnv_1", functiondomain.EnvStatusReady, "", []string{"pandas==2.0", "numpy"}, &now); err != nil {
		t.Fatalf("UpdateVersionEnv: %v", err)
	}
	v, _ := s.GetVersion(ctx, "fnv_1")
	if v.EnvStatus != functiondomain.EnvStatusReady {
		t.Fatalf("env status = %q", v.EnvStatus)
	}
	if len(v.Dependencies) != 2 || v.Dependencies[0] != "pandas==2.0" {
		t.Fatalf("deps not rewritten: %v", v.Dependencies)
	}
	if v.EnvSyncedAt == nil {
		t.Fatal("env_synced_at not set")
	}
}

func TestVersion_TrimProtectsActive(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	// active is v1 (oldest) — simulating a revert; trim must spare it.
	mkFn(t, s, ctx, "fn_1", "a", "fnv_1")
	for i := 1; i <= 5; i++ {
		mkVer(t, s, ctx, "fnv_"+string(rune('0'+i)), "fn_1", i)
	}
	trimmedEnvs, err := s.TrimOldestVersions(ctx, "fn_1", 3)
	if err != nil {
		t.Fatalf("trim: %v", err)
	}
	// keep newest 3 (v3,v4,v5); v1,v2 are beyond — but v1 is active → only v2 deleted.
	// The returned reclaim list must name exactly v2's env (so the caller frees its orphaned venv).
	if len(trimmedEnvs) != 1 || trimmedEnvs[0] != "env_fnv_2" {
		t.Fatalf("trim must return only v2's env id for reclaim, got %v", trimmedEnvs)
	}
	if _, err := s.GetVersion(ctx, "fnv_1"); err != nil {
		t.Fatalf("active v1 must survive trim, got %v", err)
	}
	if _, err := s.GetVersion(ctx, "fnv_2"); !errors.Is(err, functiondomain.ErrVersionNotFound) {
		t.Fatalf("v2 should be trimmed, got %v", err)
	}
	if _, err := s.GetVersion(ctx, "fnv_3"); err != nil {
		t.Fatalf("v3 should survive, got %v", err)
	}
}

func TestFunction_GetByIDsPreservesOrder(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkFn(t, s, ctx, "fn_a", "a", "")
	mkFn(t, s, ctx, "fn_b", "b", "")
	rows, err := s.GetFunctionsByIDs(ctx, []string{"fn_b", "fn_a", "fn_missing"})
	if err != nil {
		t.Fatalf("GetFunctionsByIDs: %v", err)
	}
	if len(rows) != 2 || rows[0].ID != "fn_b" || rows[1].ID != "fn_a" {
		t.Fatalf("order not preserved / missing not skipped: %v", rows)
	}
}

func TestExecutions_SaveListAggregates(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	now := time.Now().UTC()
	save := func(id, status string) {
		e := &functiondomain.Execution{
			ID: id, FunctionID: "fn_1", VersionID: "fnv_1", Status: status,
			TriggeredBy: functiondomain.TriggeredByChat, Input: map[string]any{}, StartedAt: now, EndedAt: now,
		}
		if err := s.SaveExecution(ctx, e); err != nil {
			t.Fatalf("SaveExecution %s: %v", id, err)
		}
	}
	save("fne_1", functiondomain.ExecutionStatusOK)
	save("fne_2", functiondomain.ExecutionStatusOK)
	save("fne_3", functiondomain.ExecutionStatusFailed)

	rows, _, err := s.ListExecutions(ctx, functiondomain.ExecutionFilter{FunctionID: "fn_1"})
	if err != nil || len(rows) != 3 {
		t.Fatalf("list: rows=%d err=%v", len(rows), err)
	}
	agg, err := s.ComputeExecutionAggregates(ctx, functiondomain.ExecutionFilter{FunctionID: "fn_1"})
	if err != nil {
		t.Fatalf("aggregates: %v", err)
	}
	if agg.OKCount != 2 || agg.FailedCount != 1 {
		t.Fatalf("aggregates: %+v", agg)
	}
	one, err := s.GetExecutionByID(ctx, "fne_3")
	if err != nil || one.Status != functiondomain.ExecutionStatusFailed {
		t.Fatalf("get exec: %+v err=%v", one, err)
	}
	if _, err := s.GetExecutionByID(ctx, "fne_missing"); !errors.Is(err, functiondomain.ErrExecutionNotFound) {
		t.Fatalf("missing exec should be ErrExecutionNotFound, got %v", err)
	}
}

// TestExecutions_LogsOnGetNotList: logs persist on the row and come back on the
// single-record Get, but lists travel light (logs blanked).
//
// TestExecutions_LogsOnGetNotList：logs 随行落盘、单条 Get 读回；列表轻装（logs 置空）。
func TestExecutions_LogsOnGetNotList(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	now := time.Now().UTC()
	e := &functiondomain.Execution{
		ID: "fne_logs", FunctionID: "fn_1", VersionID: "fnv_1",
		Status: functiondomain.ExecutionStatusOK, TriggeredBy: functiondomain.TriggeredByChat,
		Input: map[string]any{}, Logs: "step 1\nstep 2\n", StartedAt: now, EndedAt: now,
	}
	if err := s.SaveExecution(ctx, e); err != nil {
		t.Fatalf("SaveExecution: %v", err)
	}
	one, err := s.GetExecutionByID(ctx, "fne_logs")
	if err != nil || one.Logs != "step 1\nstep 2\n" {
		t.Fatalf("get should carry logs: %+v err=%v", one, err)
	}
	rows, _, err := s.ListExecutions(ctx, functiondomain.ExecutionFilter{FunctionID: "fn_1"})
	if err != nil || len(rows) == 0 {
		t.Fatalf("list: %v", err)
	}
	for _, r := range rows {
		if r.Logs != "" {
			t.Fatalf("list must blank logs, got %q on %s", r.Logs, r.ID)
		}
	}
}

// TestListExecutions_RejectsInvalidStatus pins F168-M2 for the execution-list path (agent/handler/mcp
// stores mirror this exactly): an out-of-enum status is rejected 422 instead of silently empty.
func TestListExecutions_RejectsInvalidStatus(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	if _, _, err := s.ListExecutions(ctx, functiondomain.ExecutionFilter{Status: "running"}); !errors.Is(err, functiondomain.ErrInvalidExecutionStatus) {
		t.Fatalf("invalid status must return ErrInvalidExecutionStatus, got %v", err) // "running" is a flowrun status, not an execution status
	}
	if _, _, err := s.ListExecutions(ctx, functiondomain.ExecutionFilter{Status: functiondomain.ExecutionStatusOK}); err != nil {
		t.Fatalf("valid status must succeed, got %v", err)
	}
	if _, _, err := s.ListExecutions(ctx, functiondomain.ExecutionFilter{}); err != nil {
		t.Fatalf("empty filter must succeed, got %v", err)
	}
}

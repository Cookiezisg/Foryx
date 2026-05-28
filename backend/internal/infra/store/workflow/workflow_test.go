package workflow

import (
	"context"
	"errors"
	"fmt"
	"testing"

	gormlogger "gorm.io/gorm/logger"

	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
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

func mkWorkflow(id, userID, name string) *workflowdomain.Workflow {
	return &workflowdomain.Workflow{
		ID:          id,
		UserID:      userID,
		Name:        name,
		Description: "test-" + id,
		Tags:        []string{},
		Enabled:     true,
		Concurrency: workflowdomain.ConcurrencySerial,
	}
}

func mkVersion(id, workflowID, status string) *workflowdomain.Version {
	return &workflowdomain.Version{
		ID:         id,
		WorkflowID: workflowID,
		Status:     status,
		Graph:      `{"nodes":[],"edges":[]}`,
	}
}


func TestSaveWorkflow_HappyPath(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	w := mkWorkflow("wf1", userAlice, "email-watcher")
	if err := s.SaveWorkflow(ctx, w); err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}
	got, err := s.GetWorkflow(ctx, "wf1")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if got.Name != "email-watcher" {
		t.Errorf("Name = %q, want email-watcher", got.Name)
	}
}

func TestSaveWorkflow_DuplicateName(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	_ = s.SaveWorkflow(ctx, mkWorkflow("wf1", userAlice, "shared"))
	err := s.SaveWorkflow(ctx, mkWorkflow("wf2", userAlice, "shared"))
	if !errors.Is(err, workflowdomain.ErrDuplicateName) {
		t.Fatalf("expected ErrDuplicateName, got %v", err)
	}
}

func TestSaveWorkflow_SoftDeletedAllowsReuse(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	_ = s.SaveWorkflow(ctx, mkWorkflow("wf1", userAlice, "name-x"))
	if err := s.DeleteWorkflow(ctx, "wf1"); err != nil {
		t.Fatalf("DeleteWorkflow: %v", err)
	}
	if err := s.SaveWorkflow(ctx, mkWorkflow("wf2", userAlice, "name-x")); err != nil {
		t.Errorf("re-create after soft-delete should succeed, got %v", err)
	}
}

func TestGetWorkflow_CrossUserReturnsNotFound(t *testing.T) {
	s := newStore(t)
	_ = s.SaveWorkflow(ctxFor(userAlice), mkWorkflow("wf1", userAlice, "alice-wf"))

	_, err := s.GetWorkflow(ctxFor(userBob), "wf1")
	if !errors.Is(err, workflowdomain.ErrNotFound) {
		t.Errorf("cross-user GET should return ErrNotFound, got %v", err)
	}
}

func TestListWorkflows_Pagination(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	for i := 0; i < 5; i++ {
		_ = s.SaveWorkflow(ctx, mkWorkflow(fmt.Sprintf("wf%d", i), userAlice, fmt.Sprintf("wf-%d", i)))
	}
	rows, next, err := s.ListWorkflows(ctx, workflowdomain.ListFilter{Limit: 3})
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("first page len = %d, want 3", len(rows))
	}
	if next == "" {
		t.Errorf("expected non-empty nextCursor on full page")
	}
	rows2, _, _ := s.ListWorkflows(ctx, workflowdomain.ListFilter{Cursor: next, Limit: 3})
	if len(rows2) != 2 {
		t.Errorf("second page len = %d, want 2", len(rows2))
	}
}

func TestListWorkflows_EnabledOnly(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	w1 := mkWorkflow("wf1", userAlice, "enabled-wf")
	w1.Enabled = true
	w2 := mkWorkflow("wf2", userAlice, "disabled-wf")
	w2.Enabled = false
	_ = s.SaveWorkflow(ctx, w1)
	_ = s.SaveWorkflow(ctx, w2)

	all, _, _ := s.ListWorkflows(ctx, workflowdomain.ListFilter{})
	if len(all) != 2 {
		t.Errorf("all: got %d, want 2", len(all))
	}
	enabled, _, _ := s.ListWorkflows(ctx, workflowdomain.ListFilter{EnabledOnly: true})
	if len(enabled) != 1 || enabled[0].ID != "wf1" {
		t.Errorf("enabled-only: got %d rows, want 1 (wf1)", len(enabled))
	}
}

func TestSetActiveVersion_AndSetNeedsAttention(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	_ = s.SaveWorkflow(ctx, mkWorkflow("wf1", userAlice, "n"))

	if err := s.SetActiveVersion(ctx, "wf1", "wfv_active"); err != nil {
		t.Fatalf("SetActiveVersion: %v", err)
	}
	got, _ := s.GetWorkflow(ctx, "wf1")
	if got.ActiveVersionID != "wfv_active" {
		t.Errorf("ActiveVersionID = %q, want wfv_active", got.ActiveVersionID)
	}

	if err := s.SetNeedsAttention(ctx, "wf1", true, "handler removed"); err != nil {
		t.Fatalf("SetNeedsAttention: %v", err)
	}
	got, _ = s.GetWorkflow(ctx, "wf1")
	if !got.NeedsAttention || got.AttentionReason != "handler removed" {
		t.Errorf("NeedsAttention=%v reason=%q, want true / handler removed", got.NeedsAttention, got.AttentionReason)
	}
}


func TestSaveVersion_AndGetPending(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	_ = s.SaveWorkflow(ctx, mkWorkflow("wf1", userAlice, "n"))

	if err := s.SaveVersion(ctx, mkVersion("wfv1", "wf1", workflowdomain.StatusPending)); err != nil {
		t.Fatalf("SaveVersion: %v", err)
	}
	pending, err := s.GetPending(ctx, "wf1")
	if err != nil {
		t.Fatalf("GetPending: %v", err)
	}
	if pending.ID != "wfv1" {
		t.Errorf("pending.ID = %q, want wfv1", pending.ID)
	}

	one := 1
	if err := s.UpdateVersionStatus(ctx, "wfv1", workflowdomain.StatusAccepted, &one); err != nil {
		t.Fatalf("UpdateVersionStatus: %v", err)
	}
	if _, err := s.GetPending(ctx, "wf1"); !errors.Is(err, workflowdomain.ErrPendingNotFound) {
		t.Errorf("after accept, GetPending should return ErrPendingNotFound, got %v", err)
	}
}

func TestGetVersionByNumber(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	_ = s.SaveWorkflow(ctx, mkWorkflow("wf1", userAlice, "n"))

	v := mkVersion("wfv1", "wf1", workflowdomain.StatusAccepted)
	one := 1
	v.Version = &one
	_ = s.SaveVersion(ctx, v)

	got, err := s.GetVersionByNumber(ctx, "wf1", 1)
	if err != nil {
		t.Fatalf("GetVersionByNumber: %v", err)
	}
	if got.ID != "wfv1" {
		t.Errorf("v.ID = %q, want wfv1", got.ID)
	}

	if _, err := s.GetVersionByNumber(ctx, "wf1", 99); !errors.Is(err, workflowdomain.ErrVersionNotFound) {
		t.Errorf("missing version should return ErrVersionNotFound, got %v", err)
	}
}

func TestHardDeleteVersion_Removes(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	_ = s.SaveWorkflow(ctx, mkWorkflow("wf1", userAlice, "n"))
	_ = s.SaveVersion(ctx, mkVersion("wfv1", "wf1", workflowdomain.StatusPending))

	if err := s.HardDeleteVersion(ctx, "wfv1"); err != nil {
		t.Fatalf("HardDeleteVersion: %v", err)
	}
	if _, err := s.GetVersion(ctx, "wfv1"); !errors.Is(err, workflowdomain.ErrVersionNotFound) {
		t.Errorf("after HardDeleteVersion, GET should return ErrVersionNotFound, got %v", err)
	}
}

func TestHardDeleteOldestAccepted_TrimsToKeep(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	_ = s.SaveWorkflow(ctx, mkWorkflow("wf1", userAlice, "n"))

	for i := 1; i <= 6; i++ {
		v := mkVersion(fmt.Sprintf("wfv%d", i), "wf1", workflowdomain.StatusAccepted)
		n := i
		v.Version = &n
		_ = s.SaveVersion(ctx, v)
	}

	if err := s.HardDeleteOldestAccepted(ctx, "wf1", 4); err != nil {
		t.Fatalf("HardDeleteOldestAccepted: %v", err)
	}
	rows, _, _ := s.ListVersions(ctx, "wf1", workflowdomain.VersionListFilter{Status: workflowdomain.StatusAccepted, Limit: 10})
	if len(rows) != 4 {
		t.Errorf("after trim len = %d, want 4", len(rows))
	}
}

func TestUpdateVersionStatus_MissingReturnsErrVersionNotFound(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	one := 1
	if err := s.UpdateVersionStatus(ctx, "missing", workflowdomain.StatusAccepted, &one); !errors.Is(err, workflowdomain.ErrVersionNotFound) {
		t.Errorf("expected ErrVersionNotFound, got %v", err)
	}
}

// graphWithApiKey returns a workflow_version.graph JSON literal containing
// node.modelOverride.apiKeyId; Task 8 will formalise the NodeSpec field, but
// for the LIKE-based reference scan only the substring matters.
//
// graphWithApiKey 拼出包含 node.modelOverride.apiKeyId 的 graph JSON 字面量；
// Task 8 会规范化 NodeSpec 字段,这里只关心 LIKE 子串匹配。
func graphWithApiKey(apiKeyID string) string {
	return `{"nodes":[{"nodeId":"n1","modelOverride":{"apiKeyId":"` + apiKeyID + `","modelId":"gpt-4o"}}],"edges":[]}`
}

func TestStore_AnyReferencesApiKey_True(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	if err := s.SaveWorkflow(ctx, mkWorkflow("wf1", userAlice, "wf-with-override")); err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}
	v := mkVersion("v1", "wf1", workflowdomain.StatusPending)
	v.Graph = graphWithApiKey("aki_x")
	if err := s.SaveVersion(ctx, v); err != nil {
		t.Fatalf("SaveVersion: %v", err)
	}
	got, err := s.AnyReferencesApiKey(ctx, "aki_x")
	if err != nil {
		t.Fatalf("AnyReferencesApiKey: %v", err)
	}
	if !got {
		t.Error("got false, want true (node override references aki_x)")
	}
}

func TestStore_AnyReferencesApiKey_False(t *testing.T) {
	s := newStore(t)
	got, err := s.AnyReferencesApiKey(ctxFor(userAlice), "aki_x")
	if err != nil {
		t.Fatalf("AnyReferencesApiKey: %v", err)
	}
	if got {
		t.Error("got true on empty store, want false")
	}
}

func TestStore_AnyReferencesApiKey_CrossUserIsolated(t *testing.T) {
	s := newStore(t)

	if err := s.SaveWorkflow(ctxFor(userAlice), mkWorkflow("wf-a", userAlice, "alice-wf")); err != nil {
		t.Fatalf("SaveWorkflow Alice: %v", err)
	}
	v := mkVersion("v-a", "wf-a", workflowdomain.StatusPending)
	v.Graph = graphWithApiKey("aki_x")
	if err := s.SaveVersion(ctxFor(userAlice), v); err != nil {
		t.Fatalf("SaveVersion Alice: %v", err)
	}
	got, err := s.AnyReferencesApiKey(ctxFor(userBob), "aki_x")
	if err != nil {
		t.Fatalf("AnyReferencesApiKey: %v", err)
	}
	if got {
		t.Error("got true: Bob sees Alice's reference, want false (cross-user isolated via workflow join)")
	}
}

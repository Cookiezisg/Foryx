package approval

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite"

	approvaldomain "github.com/sunweilin/forgify/backend/internal/domain/approval"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
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

func mkForm(t *testing.T, s *Store, ctx context.Context, id, name, activeVer string) {
	t.Helper()
	if err := s.SaveForm(ctx, &approvaldomain.ApprovalForm{ID: id, Name: name, ActiveVersionID: activeVer}); err != nil {
		t.Fatalf("SaveForm %s: %v", id, err)
	}
}

func mkVer(t *testing.T, s *Store, ctx context.Context, id, formID string, n int) {
	t.Helper()
	v := &approvaldomain.Version{ID: id, ApprovalID: formID, Version: n, Template: "ok?"}
	if err := s.SaveVersion(ctx, v); err != nil {
		t.Fatalf("SaveVersion %s: %v", id, err)
	}
}

func TestApproval_RoundTrip_WorkspaceFilled(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkForm(t, s, ctx, "apf_1", "email-send", "")
	got, err := s.GetForm(ctx, "apf_1")
	if err != nil {
		t.Fatalf("GetForm: %v", err)
	}
	if got.Name != "email-send" || got.WorkspaceID != "ws_1" {
		t.Fatalf("round-trip: %+v", got)
	}
	if got.CreatedAt.IsZero() {
		t.Error("created_at not auto-stamped")
	}
}

func TestApproval_TemplateRulesRoundTrip(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	v := &approvaldomain.Version{
		ID: "apfv_1", ApprovalID: "apf_1", Version: 1,
		Template: "批准对 {{ input.user }} 的退款?", AllowReason: true, Timeout: "30d", TimeoutBehavior: "reject",
	}
	if err := s.SaveVersion(ctx, v); err != nil {
		t.Fatalf("SaveVersion: %v", err)
	}
	got, err := s.GetVersion(ctx, "apfv_1")
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if got.Template != v.Template || !got.AllowReason || got.Timeout != "30d" || got.TimeoutBehavior != "reject" {
		t.Fatalf("template/rules not round-tripped (allow_reason bool?): %+v", got)
	}
}

func TestApproval_DuplicateName(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkForm(t, s, ctx, "apf_1", "dup", "")
	err := s.SaveForm(ctx, &approvaldomain.ApprovalForm{ID: "apf_2", Name: "dup"})
	if !errors.Is(err, approvaldomain.ErrDuplicateName) {
		t.Fatalf("want ErrDuplicateName, got %v", err)
	}
}

func TestApproval_WorkspaceIsolation(t *testing.T) {
	s := newStore(t)
	mkForm(t, s, ctxWS("ws_1"), "apf_1", "a", "")
	mkForm(t, s, ctxWS("ws_2"), "apf_2", "a", "") // same name OK in another workspace
	if _, err := s.GetForm(ctxWS("ws_2"), "apf_1"); !errors.Is(err, approvaldomain.ErrNotFound) {
		t.Fatalf("cross-workspace read should be NotFound, got %v", err)
	}
}

func TestApproval_SoftDelete(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkForm(t, s, ctx, "apf_1", "a", "")
	if err := s.DeleteForm(ctx, "apf_1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetForm(ctx, "apf_1"); !errors.Is(err, approvaldomain.ErrNotFound) {
		t.Fatalf("deleted should be NotFound, got %v", err)
	}
	if err := s.DeleteForm(ctx, "apf_1"); !errors.Is(err, approvaldomain.ErrNotFound) {
		t.Fatalf("re-delete should be NotFound, got %v", err)
	}
}

func TestApproval_ListPagination(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	for i := range 5 {
		mkForm(t, s, ctx, "apf_"+string(rune('a'+i)), "n"+string(rune('a'+i)), "")
		time.Sleep(time.Millisecond)
	}
	page1, next, err := s.ListForms(ctx, approvaldomain.ListFilter{Limit: 2})
	if err != nil || len(page1) != 2 || next == "" {
		t.Fatalf("page1: rows=%d next=%q err=%v", len(page1), next, err)
	}
	page2, _, err := s.ListForms(ctx, approvaldomain.ListFilter{Limit: 2, Cursor: next})
	if err != nil || len(page2) != 2 {
		t.Fatalf("page2: rows=%d err=%v", len(page2), err)
	}
	if page1[0].ID == page2[0].ID {
		t.Fatal("pages overlap")
	}
}

func TestApproval_VersionMaxAndByNumber(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkForm(t, s, ctx, "apf_1", "a", "")
	if n, err := s.MaxVersionNumber(ctx, "apf_1"); err != nil || n != 0 {
		t.Fatalf("max with no versions: n=%d err=%v", n, err)
	}
	mkVer(t, s, ctx, "apfv_1", "apf_1", 1)
	mkVer(t, s, ctx, "apfv_2", "apf_1", 2)
	if n, err := s.MaxVersionNumber(ctx, "apf_1"); err != nil || n != 2 {
		t.Fatalf("max: n=%d err=%v", n, err)
	}
	v, err := s.GetVersionByNumber(ctx, "apf_1", 2)
	if err != nil || v.ID != "apfv_2" {
		t.Fatalf("by number: %+v err=%v", v, err)
	}
	if _, err := s.GetVersionByNumber(ctx, "apf_1", 9); !errors.Is(err, approvaldomain.ErrVersionNotFound) {
		t.Fatalf("missing number should be ErrVersionNotFound, got %v", err)
	}
}

func TestApproval_TrimProtectsActive(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkForm(t, s, ctx, "apf_1", "a", "apfv_1") // active = v1 (oldest), as after a revert
	for i := 1; i <= 5; i++ {
		mkVer(t, s, ctx, "apfv_"+string(rune('0'+i)), "apf_1", i)
	}
	if err := s.TrimOldestVersions(ctx, "apf_1", 3); err != nil {
		t.Fatalf("trim: %v", err)
	}
	if _, err := s.GetVersion(ctx, "apfv_1"); err != nil {
		t.Fatalf("active v1 must survive trim, got %v", err)
	}
	if _, err := s.GetVersion(ctx, "apfv_2"); !errors.Is(err, approvaldomain.ErrVersionNotFound) {
		t.Fatalf("v2 should be trimmed, got %v", err)
	}
	if _, err := s.GetVersion(ctx, "apfv_3"); err != nil {
		t.Fatalf("v3 should survive, got %v", err)
	}
}

func TestApproval_SetActiveVersion(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkForm(t, s, ctx, "apf_1", "a", "apfv_1")
	if err := s.SetActiveVersion(ctx, "apf_1", "apfv_2"); err != nil {
		t.Fatalf("SetActiveVersion: %v", err)
	}
	got, _ := s.GetForm(ctx, "apf_1")
	if got.ActiveVersionID != "apfv_2" {
		t.Fatalf("active not moved: %q", got.ActiveVersionID)
	}
	if err := s.SetActiveVersion(ctx, "apf_missing", "apfv_x"); !errors.Is(err, approvaldomain.ErrNotFound) {
		t.Fatalf("missing form should be NotFound, got %v", err)
	}
}

func TestApproval_GetByIDsPreservesOrder(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkForm(t, s, ctx, "apf_a", "a", "")
	mkForm(t, s, ctx, "apf_b", "b", "")
	rows, err := s.GetFormsByIDs(ctx, []string{"apf_b", "apf_a", "apf_missing"})
	if err != nil {
		t.Fatalf("GetFormsByIDs: %v", err)
	}
	if len(rows) != 2 || rows[0].ID != "apf_b" || rows[1].ID != "apf_a" {
		t.Fatalf("order not preserved / missing not skipped: %v", rows)
	}
}

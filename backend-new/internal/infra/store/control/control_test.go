package control

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite"

	controldomain "github.com/sunweilin/forgify/backend/internal/domain/control"
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

func mkCtl(t *testing.T, s *Store, ctx context.Context, id, name, activeVer string) {
	t.Helper()
	if err := s.SaveControl(ctx, &controldomain.ControlLogic{ID: id, Name: name, ActiveVersionID: activeVer}); err != nil {
		t.Fatalf("SaveControl %s: %v", id, err)
	}
}

func mkVer(t *testing.T, s *Store, ctx context.Context, id, ctlID string, n int) {
	t.Helper()
	v := &controldomain.Version{ID: id, ControlID: ctlID, Version: n, Branches: []controldomain.Branch{{Port: "out", When: "true"}}}
	if err := s.SaveVersion(ctx, v); err != nil {
		t.Fatalf("SaveVersion %s: %v", id, err)
	}
}

func TestControl_RoundTrip_WorkspaceFilled(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkCtl(t, s, ctx, "ctl_1", "router", "")
	got, err := s.GetControl(ctx, "ctl_1")
	if err != nil {
		t.Fatalf("GetControl: %v", err)
	}
	if got.Name != "router" || got.WorkspaceID != "ws_1" {
		t.Fatalf("round-trip: %+v", got)
	}
	if got.CreatedAt.IsZero() {
		t.Error("created_at not auto-stamped")
	}
}

func TestControl_BranchesEmitRoundTrip(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	v := &controldomain.Version{
		ID: "ctlv_1", ControlID: "ctl_1", Version: 1,
		Branches: []controldomain.Branch{
			{Port: "retry", When: "input.attempt < 3", Emit: map[string]string{"attempt": "input.attempt + 1"}},
			{Port: "done", When: "true"},
		},
	}
	if err := s.SaveVersion(ctx, v); err != nil {
		t.Fatalf("SaveVersion: %v", err)
	}
	got, err := s.GetVersion(ctx, "ctlv_1")
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if len(got.Branches) != 2 || got.Branches[0].Port != "retry" || got.Branches[0].Emit["attempt"] != "input.attempt + 1" {
		t.Fatalf("branches/emit not round-tripped: %+v", got.Branches)
	}
}

func TestControl_DuplicateName(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkCtl(t, s, ctx, "ctl_1", "dup", "")
	err := s.SaveControl(ctx, &controldomain.ControlLogic{ID: "ctl_2", Name: "dup"})
	if !errors.Is(err, controldomain.ErrDuplicateName) {
		t.Fatalf("want ErrDuplicateName, got %v", err)
	}
}

func TestControl_WorkspaceIsolation(t *testing.T) {
	s := newStore(t)
	mkCtl(t, s, ctxWS("ws_1"), "ctl_1", "a", "")
	mkCtl(t, s, ctxWS("ws_2"), "ctl_2", "a", "") // same name OK in another workspace
	if _, err := s.GetControl(ctxWS("ws_2"), "ctl_1"); !errors.Is(err, controldomain.ErrNotFound) {
		t.Fatalf("cross-workspace read should be NotFound, got %v", err)
	}
}

func TestControl_SoftDelete(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkCtl(t, s, ctx, "ctl_1", "a", "")
	if err := s.DeleteControl(ctx, "ctl_1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetControl(ctx, "ctl_1"); !errors.Is(err, controldomain.ErrNotFound) {
		t.Fatalf("deleted should be NotFound, got %v", err)
	}
	if err := s.DeleteControl(ctx, "ctl_1"); !errors.Is(err, controldomain.ErrNotFound) {
		t.Fatalf("re-delete should be NotFound, got %v", err)
	}
}

func TestControl_ListPagination(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	for i := range 5 {
		mkCtl(t, s, ctx, "ctl_"+string(rune('a'+i)), "n"+string(rune('a'+i)), "")
		time.Sleep(time.Millisecond)
	}
	page1, next, err := s.ListControls(ctx, controldomain.ListFilter{Limit: 2})
	if err != nil || len(page1) != 2 || next == "" {
		t.Fatalf("page1: rows=%d next=%q err=%v", len(page1), next, err)
	}
	page2, _, err := s.ListControls(ctx, controldomain.ListFilter{Limit: 2, Cursor: next})
	if err != nil || len(page2) != 2 {
		t.Fatalf("page2: rows=%d err=%v", len(page2), err)
	}
	if page1[0].ID == page2[0].ID {
		t.Fatal("pages overlap")
	}
}

func TestControl_VersionMaxAndByNumber(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkCtl(t, s, ctx, "ctl_1", "a", "")
	if n, err := s.MaxVersionNumber(ctx, "ctl_1"); err != nil || n != 0 {
		t.Fatalf("max with no versions: n=%d err=%v", n, err)
	}
	mkVer(t, s, ctx, "ctlv_1", "ctl_1", 1)
	mkVer(t, s, ctx, "ctlv_2", "ctl_1", 2)
	if n, err := s.MaxVersionNumber(ctx, "ctl_1"); err != nil || n != 2 {
		t.Fatalf("max: n=%d err=%v", n, err)
	}
	v, err := s.GetVersionByNumber(ctx, "ctl_1", 2)
	if err != nil || v.ID != "ctlv_2" {
		t.Fatalf("by number: %+v err=%v", v, err)
	}
	if _, err := s.GetVersionByNumber(ctx, "ctl_1", 9); !errors.Is(err, controldomain.ErrVersionNotFound) {
		t.Fatalf("missing number should be ErrVersionNotFound, got %v", err)
	}
}

func TestControl_TrimProtectsActive(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkCtl(t, s, ctx, "ctl_1", "a", "ctlv_1") // active = v1 (oldest), as after a revert
	for i := 1; i <= 5; i++ {
		mkVer(t, s, ctx, "ctlv_"+string(rune('0'+i)), "ctl_1", i)
	}
	if err := s.TrimOldestVersions(ctx, "ctl_1", 3); err != nil {
		t.Fatalf("trim: %v", err)
	}
	if _, err := s.GetVersion(ctx, "ctlv_1"); err != nil {
		t.Fatalf("active v1 must survive trim, got %v", err)
	}
	if _, err := s.GetVersion(ctx, "ctlv_2"); !errors.Is(err, controldomain.ErrVersionNotFound) {
		t.Fatalf("v2 should be trimmed, got %v", err)
	}
	if _, err := s.GetVersion(ctx, "ctlv_3"); err != nil {
		t.Fatalf("v3 should survive, got %v", err)
	}
}

func TestControl_SetActiveVersion(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkCtl(t, s, ctx, "ctl_1", "a", "ctlv_1")
	if err := s.SetActiveVersion(ctx, "ctl_1", "ctlv_2"); err != nil {
		t.Fatalf("SetActiveVersion: %v", err)
	}
	got, _ := s.GetControl(ctx, "ctl_1")
	if got.ActiveVersionID != "ctlv_2" {
		t.Fatalf("active not moved: %q", got.ActiveVersionID)
	}
	if err := s.SetActiveVersion(ctx, "ctl_missing", "ctlv_x"); !errors.Is(err, controldomain.ErrNotFound) {
		t.Fatalf("missing control should be NotFound, got %v", err)
	}
}

func TestControl_GetByIDsPreservesOrder(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	mkCtl(t, s, ctx, "ctl_a", "a", "")
	mkCtl(t, s, ctx, "ctl_b", "b", "")
	rows, err := s.GetControlsByIDs(ctx, []string{"ctl_b", "ctl_a", "ctl_missing"})
	if err != nil {
		t.Fatalf("GetControlsByIDs: %v", err)
	}
	if len(rows) != 2 || rows[0].ID != "ctl_b" || rows[1].ID != "ctl_a" {
		t.Fatalf("order not preserved / missing not skipped: %v", rows)
	}
}

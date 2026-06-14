package conversation

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite"

	conversationdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	documentdomain "github.com/sunweilin/forgify/backend/internal/domain/document"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
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

func ctxWS(id string) context.Context {
	return reqctxpkg.SetWorkspaceID(context.Background(), id)
}

// seed inserts a conversation then pins created_at to `at` (same driver as Create, so the
// stored value round-trips) — making List ordering deterministic regardless of clock resolution.
//
// seed 插入对话后把 created_at 钉到 `at`（同 Create 的驱动、存储值可往返）——使 List 排序与时钟
// 精度无关、可确定断言。
func seed(t *testing.T, s *Store, ctx context.Context, id, title string, pinned, archived bool, at time.Time) {
	t.Helper()
	c := &conversationdomain.Conversation{ID: id, Title: title, Pinned: pinned, Archived: archived}
	if err := s.Insert(ctx, c); err != nil {
		t.Fatalf("insert %s: %v", id, err)
	}
	// Set both created_at and last_message_at to `at`: last_message_at is the List sort/cursor key,
	// created_at backs other assertions. (Seeding bypasses the app layer, so set them explicitly.)
	// created_at 与 last_message_at 都设为 at：last_message_at 是 List 排序/游标键，created_at 撑其他断言。
	if _, err := s.db.Exec(ctx, "UPDATE conversations SET created_at = ?, last_message_at = ? WHERE id = ?", at.UTC(), at.UTC(), id); err != nil {
		t.Fatalf("seed time %s: %v", id, err)
	}
}

func ids(rows []*conversationdomain.Conversation) []string {
	out := make([]string, len(rows))
	for i, c := range rows {
		out[i] = c.ID
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var (
	t1 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	t3 = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
)

func TestInsertGet_RoundTrip(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	if err := s.Insert(ctx, &conversationdomain.Conversation{ID: "cv_1", Title: "Hello"}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := s.Get(ctx, "cv_1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "Hello" || got.WorkspaceID != "ws_1" {
		t.Errorf("round-trip: %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Error("timestamps not auto-stamped")
	}
}

func TestGet_NotFound(t *testing.T) {
	s := newStore(t)
	if _, err := s.Get(ctxWS("ws_1"), "cv_x"); !errors.Is(err, conversationdomain.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestModelOverride_AndAttachedJSONRoundTrip(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	ref := &modeldomain.ModelRef{APIKeyID: "aki_1", ModelID: "claude-sonnet-4", Options: map[string]string{"reasoning_effort": "high"}}
	in := &conversationdomain.Conversation{
		ID:                "cv_1",
		ModelOverride:     ref,
		AttachedDocuments: []documentdomain.AttachedDocument{{DocumentID: "doc_1"}},
	}
	if err := s.Insert(ctx, in); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := s.Get(ctx, "cv_1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ModelOverride == nil || got.ModelOverride.APIKeyID != "aki_1" ||
		got.ModelOverride.ModelID != "claude-sonnet-4" || got.ModelOverride.Options["reasoning_effort"] != "high" {
		t.Errorf("override round-trip: %+v", got.ModelOverride)
	}
	if len(got.AttachedDocuments) != 1 || got.AttachedDocuments[0].DocumentID != "doc_1" {
		t.Errorf("attached round-trip: %+v", got.AttachedDocuments)
	}
}

func TestList_PinnedFirstThenNewest(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	seed(t, s, ctx, "cv_old_pin", "old pinned", true, false, t1)
	seed(t, s, ctx, "cv_mid", "mid", false, false, t2)
	seed(t, s, ctx, "cv_new", "new", false, false, t3)
	rows, next, err := s.List(ctx, conversationdomain.ListFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if next != "" {
		t.Errorf("unexpected next cursor: %q", next)
	}
	// pinned first (despite oldest created_at), then unpinned newest→oldest.
	if got := ids(rows); !equal(got, []string{"cv_old_pin", "cv_new", "cv_mid"}) {
		t.Errorf("order = %v, want [cv_old_pin cv_new cv_mid]", got)
	}
}

func TestList_ArchivedFilter(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	seed(t, s, ctx, "cv_active", "a", false, false, t2)
	seed(t, s, ctx, "cv_arch", "b", false, true, t1)

	rows, _, _ := s.List(ctx, conversationdomain.ListFilter{}) // nil → exclude archived
	if got := ids(rows); !equal(got, []string{"cv_active"}) {
		t.Errorf("default = %v, want [cv_active]", got)
	}
	yes := true
	rows, _, _ = s.List(ctx, conversationdomain.ListFilter{Archived: &yes}) // archived only
	if got := ids(rows); !equal(got, []string{"cv_arch"}) {
		t.Errorf("archived = %v, want [cv_arch]", got)
	}
	no := false
	rows, _, _ = s.List(ctx, conversationdomain.ListFilter{Archived: &no}) // active only
	if got := ids(rows); !equal(got, []string{"cv_active"}) {
		t.Errorf("active = %v, want [cv_active]", got)
	}
}

func TestList_SearchTitle(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	seed(t, s, ctx, "cv_1", "Quarterly report", false, false, t1)
	seed(t, s, ctx, "cv_2", "Random chat", false, false, t2)
	rows, _, err := s.List(ctx, conversationdomain.ListFilter{Search: "report"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got := ids(rows); !equal(got, []string{"cv_1"}) {
		t.Errorf("search = %v, want [cv_1]", got)
	}
}

func TestList_CursorPaging(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	seed(t, s, ctx, "cv_a", "a", false, false, t1)
	seed(t, s, ctx, "cv_b", "b", false, false, t2)
	seed(t, s, ctx, "cv_c", "c", false, false, t3)
	p1, next, err := s.List(ctx, conversationdomain.ListFilter{Limit: 2})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if got := ids(p1); !equal(got, []string{"cv_c", "cv_b"}) {
		t.Errorf("page1 = %v, want [cv_c cv_b]", got)
	}
	if next == "" {
		t.Fatal("expected next cursor")
	}
	p2, next2, err := s.List(ctx, conversationdomain.ListFilter{Limit: 2, Cursor: next})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if got := ids(p2); !equal(got, []string{"cv_a"}) {
		t.Errorf("page2 = %v, want [cv_a]", got)
	}
	if next2 != "" {
		t.Errorf("unexpected next2: %q", next2)
	}
}

// TestList_RecencySortByLastMessage decorrelates id-order from activity-order to prove the list
// keys on last_message_at, not id or created_at: ids descend (cv_z > cv_m > cv_a) OPPOSITE to
// recency (cv_a most recent). A regression that sorted by id/created_at would flip the result and
// the keyset cursor would skip/duplicate. Also exercises the cursor across the boundary.
//
// TestList_RecencySortByLastMessage 把 id 序与活跃序解耦，证明列表按 last_message_at 而非 id/created_at：
// id 降序(cv_z>cv_m>cv_a)与活跃度(cv_a 最近)相反。若回归成按 id/created_at 排，结果会翻转、游标会漏/重。
func TestList_RecencySortByLastMessage(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	// id order (z>m>a) is the REVERSE of recency (cv_a newest t3, cv_m t2, cv_z oldest t1).
	seed(t, s, ctx, "cv_z", "z", false, false, t1)
	seed(t, s, ctx, "cv_m", "m", false, false, t2)
	seed(t, s, ctx, "cv_a", "a", false, false, t3)
	rows, _, err := s.List(ctx, conversationdomain.ListFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got := ids(rows); !equal(got, []string{"cv_a", "cv_m", "cv_z"}) {
		t.Fatalf("recency order = %v, want [cv_a cv_m cv_z] (most-recent last_message_at first)", got)
	}
	// Keyset cursor must walk last_message_at, not id: page 1 = [cv_a], page 2 = [cv_m].
	p1, next, err := s.List(ctx, conversationdomain.ListFilter{Limit: 1})
	if err != nil || len(p1) != 1 || p1[0].ID != "cv_a" || next == "" {
		t.Fatalf("page1 = %v next=%q err=%v, want [cv_a] with cursor", ids(p1), next, err)
	}
	p2, _, err := s.List(ctx, conversationdomain.ListFilter{Limit: 1, Cursor: next})
	if err != nil || len(p2) != 1 || p2[0].ID != "cv_m" {
		t.Fatalf("page2 = %v err=%v, want [cv_m] (cursor walks last_message_at, not id)", ids(p2), err)
	}
}

func TestSoftDelete_NotFoundAndExcluded(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	seed(t, s, ctx, "cv_1", "x", false, false, t1)
	if err := s.SoftDelete(ctx, "cv_1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Get(ctx, "cv_1"); !errors.Is(err, conversationdomain.ErrNotFound) {
		t.Errorf("get after delete = %v, want ErrNotFound", err)
	}
	if rows, _, _ := s.List(ctx, conversationdomain.ListFilter{}); len(rows) != 0 {
		t.Errorf("list after delete = %v, want empty", ids(rows))
	}
	if err := s.SoftDelete(ctx, "cv_1"); !errors.Is(err, conversationdomain.ErrNotFound) {
		t.Errorf("re-delete = %v, want ErrNotFound", err)
	}
}

func TestSoftDelete_Unknown(t *testing.T) {
	s := newStore(t)
	if err := s.SoftDelete(ctxWS("ws_1"), "cv_x"); !errors.Is(err, conversationdomain.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestWorkspaceIsolation(t *testing.T) {
	s := newStore(t)
	ws1, ws2 := ctxWS("ws_1"), ctxWS("ws_2")
	seed(t, s, ws1, "cv_1", "in ws1", false, false, t1)
	if _, err := s.Get(ws2, "cv_1"); !errors.Is(err, conversationdomain.ErrNotFound) {
		t.Errorf("cross-ws get = %v, want ErrNotFound", err)
	}
	if rows, _, _ := s.List(ws2, conversationdomain.ListFilter{}); len(rows) != 0 {
		t.Errorf("ws2 list = %v, want empty", ids(rows))
	}
	if rows, _, _ := s.List(ws1, conversationdomain.ListFilter{}); !equal(ids(rows), []string{"cv_1"}) {
		t.Errorf("ws1 list = %v, want [cv_1]", ids(rows))
	}
}

func TestGetBatch(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	seed(t, s, ctx, "cv_1", "one", false, false, t1)
	seed(t, s, ctx, "cv_2", "two", false, false, t2)
	rows, err := s.GetBatch(ctx, []string{"cv_1", "cv_2", "cv_missing"})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("batch len = %d, want 2", len(rows))
	}
	if r, err := s.GetBatch(ctx, nil); err != nil || r != nil {
		t.Errorf("empty batch = %v, %v", r, err)
	}
}

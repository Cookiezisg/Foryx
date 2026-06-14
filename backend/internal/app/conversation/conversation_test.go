package conversation

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "github.com/glebarez/go-sqlite"
	"go.uber.org/zap"

	conversationdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	conversationstore "github.com/sunweilin/forgify/backend/internal/infra/store/conversation"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// fakeEmitter records every Emit so tests assert the broadcast action without a real bus.
//
// fakeEmitter 记录每次 Emit，使测试断言广播动作而无需真 bus。
type fakeEmitter struct{ events []string }

func (f *fakeEmitter) Emit(_ context.Context, eventType string, _ map[string]any) error {
	f.events = append(f.events, eventType)
	return nil
}

func (f *fakeEmitter) last() string {
	if len(f.events) == 0 {
		return ""
	}
	return f.events[len(f.events)-1]
}

// fakeRelations records PurgeEntity calls.
//
// fakeRelations 记录 PurgeEntity 调用。
type fakeRelations struct{ purged []string }

func (f *fakeRelations) PurgeEntity(_ context.Context, kind, id string) error {
	f.purged = append(f.purged, kind+":"+id)
	return nil
}

// newSvc wires the Service over a real in-memory store + fakes, so the tests exercise the full
// app→store→orm stack offline (JSON round-trip, soft-delete, isolation).
//
// newSvc 把 Service 接在真 in-memory store + fake 上，使测试离线走全栈 app→store→orm。
func newSvc(t *testing.T) (*Service, *fakeEmitter, *fakeRelations, context.Context) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, stmt := range conversationstore.Schema {
		if _, err := sqlDB.Exec(stmt); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	em := &fakeEmitter{}
	svc := NewService(conversationstore.New(ormpkg.Open(sqlDB)), em, zap.NewNop())
	rel := &fakeRelations{}
	svc.SetRelationSyncer(rel)
	return svc, em, rel, reqctxpkg.SetWorkspaceID(context.Background(), "ws_1")
}

func TestCreate_TrimsTitle_EmitsCreated(t *testing.T) {
	svc, em, _, ctx := newSvc(t)
	c, err := svc.Create(ctx, "  Hi  ")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.Title != "Hi" {
		t.Errorf("title not trimmed: %q", c.Title)
	}
	if len(c.ID) < 3 || c.ID[:3] != "cv_" {
		t.Errorf("id prefix: %s", c.ID)
	}
	if len(em.events) != 1 || em.events[0] != "conversation.created" {
		t.Errorf("events = %v", em.events)
	}
}

type fakeQuerier struct{ generating map[string]bool }

func (f fakeQuerier) IsGenerating(id string) bool { return f.generating[id] }

// TestDerivesIsGenerating: Get/List fill the derived IsGenerating from the injected querier; with
// no querier wired it stays false (never crashes, never invents state).
func TestDerivesIsGenerating(t *testing.T) {
	svc, _, _, ctx := newSvc(t)
	a, _ := svc.Create(ctx, "a")
	b, _ := svc.Create(ctx, "b")
	svc.SetGeneratingQuerier(fakeQuerier{generating: map[string]bool{a.ID: true}})

	ga, _ := svc.Get(ctx, a.ID)
	gb, _ := svc.Get(ctx, b.ID)
	if !ga.IsGenerating || gb.IsGenerating {
		t.Errorf("Get: a=%v b=%v, want a=true b=false", ga.IsGenerating, gb.IsGenerating)
	}
	rows, _, err := svc.List(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, c := range rows {
		if want := c.ID == a.ID; c.IsGenerating != want {
			t.Errorf("List: %s isGenerating=%v want %v", c.ID, c.IsGenerating, want)
		}
	}

	// No querier wired (default) → derived flag stays false, no panic.
	svc2, _, _, ctx2 := newSvc(t)
	c, _ := svc2.Create(ctx2, "c")
	if gc, _ := svc2.Get(ctx2, c.ID); gc.IsGenerating {
		t.Error("nil querier → IsGenerating must be false")
	}
}

func TestCreateWithSystemPrompt(t *testing.T) {
	svc, _, _, ctx := newSvc(t)
	c, err := svc.CreateWithSystemPrompt(ctx, "", "You are helpful")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.SystemPrompt != "You are helpful" {
		t.Errorf("sysprompt = %q", c.SystemPrompt)
	}
}

func TestUpdate_ModelOverride_SetThenClear(t *testing.T) {
	svc, em, _, ctx := newSvc(t)
	c, _ := svc.Create(ctx, "t")

	set := &modeldomain.ModelRef{APIKeyID: "aki_1", ModelID: "m1"}
	got, err := svc.Update(ctx, c.ID, UpdateInput{ModelOverride: &set})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if got.ModelOverride == nil || got.ModelOverride.ModelID != "m1" {
		t.Errorf("set: %+v", got.ModelOverride)
	}
	if em.last() != "conversation.model_override" {
		t.Errorf("set event = %v", em.events)
	}

	var none *modeldomain.ModelRef // &nil = explicit clear
	got, err = svc.Update(ctx, c.ID, UpdateInput{ModelOverride: &none})
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got.ModelOverride != nil {
		t.Errorf("clear: %+v", got.ModelOverride)
	}
}

func TestUpdate_InvalidModelOverride(t *testing.T) {
	svc, _, _, ctx := newSvc(t)
	c, _ := svc.Create(ctx, "t")
	bad := &modeldomain.ModelRef{APIKeyID: "aki_1"} // missing modelId
	if _, err := svc.Update(ctx, c.ID, UpdateInput{ModelOverride: &bad}); !errors.Is(err, conversationdomain.ErrInvalidModelOverride) {
		t.Errorf("err = %v, want ErrInvalidModelOverride", err)
	}
}

func TestUpdate_PinThenArchive_EmitActions(t *testing.T) {
	svc, em, _, ctx := newSvc(t)
	c, _ := svc.Create(ctx, "t")
	yes := true
	if _, err := svc.Update(ctx, c.ID, UpdateInput{Pinned: &yes}); err != nil {
		t.Fatal(err)
	}
	if em.last() != "conversation.pinned" {
		t.Errorf("pin event = %v", em.events)
	}
	if _, err := svc.Update(ctx, c.ID, UpdateInput{Archived: &yes}); err != nil {
		t.Fatal(err)
	}
	if em.last() != "conversation.archived" {
		t.Errorf("archive event = %v", em.events)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	svc, _, _, ctx := newSvc(t)
	title := "x"
	if _, err := svc.Update(ctx, "cv_missing", UpdateInput{Title: &title}); !errors.Is(err, conversationdomain.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestDelete_EmitsAndPurges(t *testing.T) {
	svc, em, rel, ctx := newSvc(t)
	c, _ := svc.Create(ctx, "t")
	if err := svc.Delete(ctx, c.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if em.last() != "conversation.deleted" {
		t.Errorf("delete event = %v", em.events)
	}
	if len(rel.purged) != 1 || rel.purged[0] != "conversation:"+c.ID {
		t.Errorf("purged = %v", rel.purged)
	}
	if _, err := svc.Get(ctx, c.ID); !errors.Is(err, conversationdomain.ErrNotFound) {
		t.Errorf("get after delete = %v, want ErrNotFound", err)
	}
}

func TestNamesByIDs_LabelFallback(t *testing.T) {
	svc, _, _, ctx := newSvc(t)
	titled, _ := svc.Create(ctx, "My Thread")
	untitled, _ := svc.Create(ctx, "")
	names, err := svc.NamesByIDs(ctx, []string{titled.ID, untitled.ID})
	if err != nil {
		t.Fatalf("names: %v", err)
	}
	if names[titled.ID] != "My Thread" {
		t.Errorf("titled label = %q", names[titled.ID])
	}
	if names[untitled.ID] != "(未命名对话)" {
		t.Errorf("untitled label = %q", names[untitled.ID])
	}
}

func TestSetSummary_PersistsAndEmits(t *testing.T) {
	svc, em, _, ctx := newSvc(t)
	c, _ := svc.Create(ctx, "Thread")

	if err := svc.SetSummary(ctx, c.ID, "the running summary", 42); err != nil {
		t.Fatalf("SetSummary: %v", err)
	}

	got, err := svc.Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Summary != "the running summary" || got.SummaryCoversUpToSeq != 42 {
		t.Fatalf("summary/watermark not persisted: %q / %d", got.Summary, got.SummaryCoversUpToSeq)
	}
	if em.last() != "conversation.compacted" {
		t.Fatalf("expected conversation.compacted emit, got %q", em.last())
	}
}

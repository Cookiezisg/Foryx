// todo_test.go — unit tests for todoapp.Service. Uses an in-memory
// SQLite-backed todostore + a recording fake bridge so we can verify
// each mutation publishes the right Todo event.
//
// todo_test.go — todoapp.Service 单测：内存 SQLite 跑 todostore + 记录式
// fake bridge，验证每次变更都发出正确的 Todo 事件。
package todo

import (
	"context"
	"errors"
	"sync"
	"testing"

	"go.uber.org/zap"

	eventsdomain "github.com/sunweilin/forgify/backend/internal/domain/events"
	tododomain "github.com/sunweilin/forgify/backend/internal/domain/todo"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	todostore "github.com/sunweilin/forgify/backend/internal/infra/store/todo"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

// recordingBridge implements eventsdomain.Bridge by appending each
// Publish call to a slice — sufficient for verifying our Service
// publishes the entity-state event on every mutation.
//
// recordingBridge 实现 eventsdomain.Bridge：每次 Publish 追加切片；够用
// 来验 Service 在每次变更时都发了 entity-state 事件。
type recordingBridge struct {
	mu      sync.Mutex
	records []bridgeRecord
}

type bridgeRecord struct {
	Key   string
	Event eventsdomain.Event
}

func (r *recordingBridge) Publish(_ context.Context, key string, e eventsdomain.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, bridgeRecord{Key: key, Event: e})
}

func (r *recordingBridge) Subscribe(context.Context, string) (<-chan eventsdomain.Event, func()) {
	panic("recordingBridge does not implement Subscribe in tests")
}

func (r *recordingBridge) snapshot() []bridgeRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]bridgeRecord, len(r.records))
	copy(out, r.records)
	return out
}

// newTestService spins up an in-memory SQLite + todostore + recording
// bridge and returns a wired Service. Returns the bridge so tests can
// inspect publish records.
//
// newTestService 跑一份内存 SQLite + todostore + 记录式 bridge，返
// 装好的 Service；同时返 bridge 让测试查 publish 记录。
func newTestService(t *testing.T) (*Service, *recordingBridge) {
	t.Helper()
	db, err := dbinfra.Open(dbinfra.Config{DataDir: ""})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := dbinfra.Migrate(db, &tododomain.Todo{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	bridge := &recordingBridge{}
	svc := NewService(todostore.New(db), bridge, zap.NewNop())
	return svc, bridge
}

func ctxWithConv(id string) context.Context {
	return reqctxpkg.WithConversationID(context.Background(), id)
}

// ── Create ───────────────────────────────────────────────────────────────────

func TestService_Create_HappyPath(t *testing.T) {
	svc, bridge := newTestService(t)
	ctx := ctxWithConv("cv_alpha")
	got, err := svc.Create(ctx, CreateInput{Subject: "Run tests", ActiveForm: "Running tests"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID == "" {
		t.Error("expected ID assigned")
	}
	if got.ConversationID != "cv_alpha" {
		t.Errorf("ConversationID = %q", got.ConversationID)
	}
	if got.Status != tododomain.StatusPending {
		t.Errorf("Status = %q, want pending", got.Status)
	}
	// Bridge should have one Todo event with conv key.
	// bridge 应有一条 Todo 事件，key 是 conv。
	recs := bridge.snapshot()
	if len(recs) != 1 || recs[0].Key != "cv_alpha" {
		t.Errorf("bridge records = %+v", recs)
	}
	if recs[0].Event.EventName() != "todo" {
		t.Errorf("event name = %q, want todo", recs[0].Event.EventName())
	}
}

func TestService_Create_EmptySubjectRejected(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := ctxWithConv("cv_x")
	_, err := svc.Create(ctx, CreateInput{Subject: "  "})
	if !errors.Is(err, tododomain.ErrSubjectRequired) {
		t.Errorf("want ErrSubjectRequired, got %v", err)
	}
}

func TestService_Create_NoConvID_ReturnsSentinel(t *testing.T) {
	svc, _ := newTestService(t)
	_, err := svc.Create(context.Background(), CreateInput{Subject: "x"})
	if !errors.Is(err, reqctxpkg.ErrMissingConversationID) {
		t.Errorf("want ErrMissingConversationID, got %v", err)
	}
}

// ── Get ──────────────────────────────────────────────────────────────────────

func TestService_Get_ReturnsNotFoundForOtherConv(t *testing.T) {
	svc, _ := newTestService(t)
	ctxA := ctxWithConv("cv_alpha")
	created, err := svc.Create(ctxA, CreateInput{Subject: "private"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	ctxB := ctxWithConv("cv_beta")
	_, err = svc.Get(ctxB, created.ID)
	if !errors.Is(err, tododomain.ErrNotFound) {
		t.Errorf("cross-conv Get want ErrNotFound, got %v", err)
	}
}

// ── List ─────────────────────────────────────────────────────────────────────

func TestService_List_ScopesToCurrentConv(t *testing.T) {
	svc, _ := newTestService(t)
	ctxA, ctxB := ctxWithConv("cv_a"), ctxWithConv("cv_b")
	if _, err := svc.Create(ctxA, CreateInput{Subject: "in A"}); err != nil {
		t.Fatalf("create A: %v", err)
	}
	if _, err := svc.Create(ctxB, CreateInput{Subject: "in B"}); err != nil {
		t.Fatalf("create B: %v", err)
	}
	rowsA, err := svc.List(ctxA)
	if err != nil {
		t.Fatalf("List A: %v", err)
	}
	if len(rowsA) != 1 || rowsA[0].Subject != "in A" {
		t.Errorf("List A = %+v", rowsA)
	}
	rowsB, err := svc.List(ctxB)
	if err != nil {
		t.Fatalf("List B: %v", err)
	}
	if len(rowsB) != 1 || rowsB[0].Subject != "in B" {
		t.Errorf("List B = %+v", rowsB)
	}
}

// ── Update ───────────────────────────────────────────────────────────────────

func TestService_Update_PartialFieldsApplied(t *testing.T) {
	svc, bridge := newTestService(t)
	ctx := ctxWithConv("cv_x")
	created, err := svc.Create(ctx, CreateInput{Subject: "old subject"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	newStatus := tododomain.StatusInProgress
	updated, err := svc.Update(ctx, created.ID, UpdateInput{Status: &newStatus})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Status != tododomain.StatusInProgress {
		t.Errorf("Status = %q", updated.Status)
	}
	if updated.Subject != "old subject" {
		t.Errorf("Subject should not have changed: %q", updated.Subject)
	}
	// Bridge should have 2 records: create + update.
	// bridge 应有 2 条记录：create + update。
	if recs := bridge.snapshot(); len(recs) != 2 {
		t.Errorf("expected 2 bridge events, got %d", len(recs))
	}
}

func TestService_Update_RejectsInvalidStatus(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := ctxWithConv("cv_x")
	created, err := svc.Create(ctx, CreateInput{Subject: "x"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	bogus := "wat"
	_, err = svc.Update(ctx, created.ID, UpdateInput{Status: &bogus})
	if !errors.Is(err, tododomain.ErrInvalidStatus) {
		t.Errorf("want ErrInvalidStatus, got %v", err)
	}
}

func TestService_Update_RejectsCrossConvAsNotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctxA := ctxWithConv("cv_a")
	created, err := svc.Create(ctxA, CreateInput{Subject: "x"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	ctxB := ctxWithConv("cv_b")
	subj := "hijack"
	_, err = svc.Update(ctxB, created.ID, UpdateInput{Subject: &subj})
	if !errors.Is(err, tododomain.ErrNotFound) {
		t.Errorf("cross-conv update should be ErrNotFound, got %v", err)
	}
}

// ── Delete ───────────────────────────────────────────────────────────────────

func TestService_Delete_SoftDeletesAndPublishesFinalSnapshot(t *testing.T) {
	svc, bridge := newTestService(t)
	ctx := ctxWithConv("cv_x")
	created, err := svc.Create(ctx, CreateInput{Subject: "to delete"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// Subsequent Get → not found.
	// 后续 Get → not found。
	if _, err := svc.Get(ctx, created.ID); !errors.Is(err, tododomain.ErrNotFound) {
		t.Errorf("after Delete want ErrNotFound, got %v", err)
	}
	// Bridge: 2 records (create + delete final snapshot).
	// bridge：2 条（create + delete 最终快照）。
	recs := bridge.snapshot()
	if len(recs) != 2 {
		t.Fatalf("expected 2 bridge events, got %d", len(recs))
	}
	deletedEvent, ok := recs[1].Event.(eventsdomain.Todo)
	if !ok || deletedEvent.Todo == nil {
		t.Fatalf("expected Todo event, got %T", recs[1].Event)
	}
	if deletedEvent.Todo.Status != tododomain.StatusDeleted {
		t.Errorf("final snapshot Status = %q, want %q", deletedEvent.Todo.Status, tododomain.StatusDeleted)
	}
}

func TestService_Delete_RejectsCrossConvAsNotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctxA := ctxWithConv("cv_a")
	created, err := svc.Create(ctxA, CreateInput{Subject: "x"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	ctxB := ctxWithConv("cv_b")
	if err := svc.Delete(ctxB, created.ID); !errors.Is(err, tododomain.ErrNotFound) {
		t.Errorf("cross-conv Delete should be ErrNotFound, got %v", err)
	}
}

// ── ID format ────────────────────────────────────────────────────────────────

func TestService_Create_AssignsTDPrefix(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := ctxWithConv("cv_x")
	created, err := svc.Create(ctx, CreateInput{Subject: "x"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := created.ID; len(got) < 3 || got[:3] != "td_" {
		t.Errorf("ID prefix = %q, want td_*", got)
	}
}

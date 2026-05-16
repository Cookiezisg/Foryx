package flowrun

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	gormlogger "gorm.io/gorm/logger"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
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

func mkRun(id, userID, workflowID, status string) *flowrundomain.FlowRun {
	return &flowrundomain.FlowRun{
		ID:           id,
		UserID:       userID,
		WorkflowID:   workflowID,
		VersionID:    "wfv_test",
		TriggerKind:  flowrundomain.TriggerKindManual,
		TriggerInput: map[string]any{"k": "v"},
		Status:       status,
		StartedAt:    time.Now().UTC(),
	}
}

func mkNode(id, userID, flowrunID, nodeID, nodeType, status string) *flowrundomain.Node {
	now := time.Now().UTC()
	return &flowrundomain.Node{
		ID:          id,
		UserID:      userID,
		Status:      status,
		TriggeredBy: "workflow",
		Input:       map[string]any{"in": 1},
		StartedAt:   now,
		EndedAt:     now,
		FlowrunID:   flowrunID,
		NodeID:      nodeID,
		NodeType:    nodeType,
		Attempts:    1,
	}
}


func TestCreate_HappyPath(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	r := mkRun("fr1", userAlice, "wf1", flowrundomain.StatusRunning)
	if err := s.Create(ctx, r); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := s.Get(ctx, "fr1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.WorkflowID != "wf1" {
		t.Errorf("WorkflowID = %q, want wf1", got.WorkflowID)
	}
	if got.Status != flowrundomain.StatusRunning {
		t.Errorf("Status = %q, want running", got.Status)
	}
}

func TestGet_CrossUserReturnsNotFound(t *testing.T) {
	s := newStore(t)
	_ = s.Create(ctxFor(userAlice), mkRun("fr1", userAlice, "wf1", flowrundomain.StatusRunning))

	_, err := s.Get(ctxFor(userBob), "fr1")
	if !errors.Is(err, flowrundomain.ErrNotFound) {
		t.Errorf("cross-user GET should return ErrNotFound, got %v", err)
	}
}

func TestList_Pagination(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	for i := 0; i < 5; i++ {
		_ = s.Create(ctx, mkRun(fmt.Sprintf("fr%d", i), userAlice, "wf1", flowrundomain.StatusCompleted))
		time.Sleep(time.Millisecond) // ensure distinct started_at
	}
	rows, next, err := s.List(ctx, flowrundomain.ListFilter{Limit: 3})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("first page len = %d, want 3", len(rows))
	}
	if next == "" {
		t.Errorf("expected non-empty nextCursor on full page")
	}
}

func TestList_FilterByWorkflowAndStatus(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	_ = s.Create(ctx, mkRun("fr1", userAlice, "wf1", flowrundomain.StatusRunning))
	_ = s.Create(ctx, mkRun("fr2", userAlice, "wf1", flowrundomain.StatusCompleted))
	_ = s.Create(ctx, mkRun("fr3", userAlice, "wf2", flowrundomain.StatusRunning))

	rows, _, _ := s.List(ctx, flowrundomain.ListFilter{WorkflowID: "wf1"})
	if len(rows) != 2 {
		t.Errorf("wf1 filter: got %d, want 2", len(rows))
	}
	rows, _, _ = s.List(ctx, flowrundomain.ListFilter{Status: flowrundomain.StatusRunning})
	if len(rows) != 2 {
		t.Errorf("status filter: got %d, want 2", len(rows))
	}
}

func TestUpdateStatus_TerminalFields(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	_ = s.Create(ctx, mkRun("fr1", userAlice, "wf1", flowrundomain.StatusRunning))

	endedAt := time.Now().UTC()
	output := map[string]any{"result": "OK"}
	if err := s.UpdateStatus(ctx, "fr1", flowrundomain.StatusCompleted, output, "", "", &endedAt, 1500); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, _ := s.Get(ctx, "fr1")
	if got.Status != flowrundomain.StatusCompleted {
		t.Errorf("Status = %q", got.Status)
	}
	if got.ElapsedMs != 1500 {
		t.Errorf("ElapsedMs = %d", got.ElapsedMs)
	}
	if got.EndedAt == nil {
		t.Errorf("EndedAt nil")
	}
}

func TestUpdateStatus_MissingReturnsErrNotFound(t *testing.T) {
	s := newStore(t)
	if err := s.UpdateStatus(ctxFor(userAlice), "missing", flowrundomain.StatusCompleted, nil, "", "", nil, 0); !errors.Is(err, flowrundomain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestPausedState_RoundTrip(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	_ = s.Create(ctx, mkRun("fr1", userAlice, "wf1", flowrundomain.StatusRunning))

	ps := &flowrundomain.PausedState{
		NodeID:    "approval_1",
		Variables: map[string]any{"x": "y"},
		Position:  []string{"approval_1"},
		PausedAt:  time.Now().UTC(),
	}
	if err := s.SetPausedState(ctx, "fr1", ps); err != nil {
		t.Fatalf("SetPausedState: %v", err)
	}
	got, _ := s.Get(ctx, "fr1")
	if got.PausedState == nil {
		t.Fatal("PausedState nil after Set")
	}
	if got.PausedState.NodeID != "approval_1" {
		t.Errorf("NodeID round-trip = %q", got.PausedState.NodeID)
	}

	if err := s.ClearPausedState(ctx, "fr1"); err != nil {
		t.Fatalf("ClearPausedState: %v", err)
	}
	got, _ = s.Get(ctx, "fr1")
	if got.PausedState != nil {
		t.Errorf("PausedState not cleared: %+v", got.PausedState)
	}
}

func TestListPaused(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	_ = s.Create(ctx, mkRun("fr1", userAlice, "wf1", flowrundomain.StatusPaused))
	_ = s.Create(ctx, mkRun("fr2", userAlice, "wf1", flowrundomain.StatusRunning))
	_ = s.Create(ctx, mkRun("fr3", userAlice, "wf1", flowrundomain.StatusPaused))

	rows, err := s.ListPaused(ctx)
	if err != nil {
		t.Fatalf("ListPaused: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("paused len = %d, want 2", len(rows))
	}
}

func TestCountRunning(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	_ = s.Create(ctx, mkRun("fr1", userAlice, "wf1", flowrundomain.StatusRunning))
	_ = s.Create(ctx, mkRun("fr2", userAlice, "wf1", flowrundomain.StatusRunning))
	_ = s.Create(ctx, mkRun("fr3", userAlice, "wf1", flowrundomain.StatusCompleted))
	_ = s.Create(ctx, mkRun("fr4", userAlice, "wf2", flowrundomain.StatusRunning))

	count, err := s.CountRunning(ctx, "wf1")
	if err != nil {
		t.Fatalf("CountRunning: %v", err)
	}
	if count != 2 {
		t.Errorf("CountRunning(wf1) = %d, want 2", count)
	}
}

func TestHardDeleteOldest_TrimsToKeep(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	for i := 0; i < 6; i++ {
		_ = s.Create(ctx, mkRun(fmt.Sprintf("fr%d", i), userAlice, "wf1", flowrundomain.StatusCompleted))
		time.Sleep(time.Millisecond)
	}
	if err := s.HardDeleteOldest(ctx, "wf1", 4); err != nil {
		t.Fatalf("HardDeleteOldest: %v", err)
	}
	rows, _, _ := s.List(ctx, flowrundomain.ListFilter{WorkflowID: "wf1", Limit: 100})
	if len(rows) != 4 {
		t.Errorf("after trim len = %d, want 4", len(rows))
	}
}


func TestNode_CreateAndGet(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	if err := s.CreateNode(ctx, mkNode("frn1", userAlice, "fr1", "step1", "function", flowrundomain.NodeStatusOK)); err != nil {
		t.Fatalf("CreateNode: %v", err)
	}
	got, err := s.GetNode(ctx, "frn1")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.NodeID != "step1" {
		t.Errorf("NodeID = %q", got.NodeID)
	}
	if got.NodeType != "function" {
		t.Errorf("NodeType = %q", got.NodeType)
	}
}

func TestNode_CrossUserNotFound(t *testing.T) {
	s := newStore(t)
	_ = s.CreateNode(ctxFor(userAlice), mkNode("frn1", userAlice, "fr1", "step1", "function", flowrundomain.NodeStatusOK))

	_, err := s.GetNode(ctxFor(userBob), "frn1")
	if !errors.Is(err, flowrundomain.ErrNodeNotFound) {
		t.Errorf("cross-user GetNode should return ErrNodeNotFound, got %v", err)
	}
}

func TestNode_ListByFlowrun(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	for i := 0; i < 3; i++ {
		_ = s.CreateNode(ctx, mkNode(fmt.Sprintf("frn%d", i), userAlice, "fr1", fmt.Sprintf("step%d", i), "function", flowrundomain.NodeStatusOK))
		time.Sleep(time.Millisecond)
	}
	_ = s.CreateNode(ctx, mkNode("frn_other", userAlice, "fr2", "step1", "function", flowrundomain.NodeStatusOK))

	rows, _, err := s.ListNodes(ctx, flowrundomain.NodeFilter{FlowrunID: "fr1"})
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("listNodes(fr1) = %d, want 3", len(rows))
	}
	// Chronological order (started_at ASC).
	for i := 1; i < len(rows); i++ {
		if rows[i].StartedAt.Before(rows[i-1].StartedAt) {
			t.Errorf("ListNodes not chronological: %v before %v", rows[i].StartedAt, rows[i-1].StartedAt)
		}
	}
}

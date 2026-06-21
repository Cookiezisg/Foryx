package flowrun

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "github.com/glebarez/go-sqlite"

	flowrundomain "github.com/sunweilin/anselm/backend/internal/domain/flowrun"
	ormpkg "github.com/sunweilin/anselm/backend/internal/pkg/orm"
	reqctxpkg "github.com/sunweilin/anselm/backend/internal/pkg/reqctx"
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

// mkRun seeds a run + its trigger node (payload = trigger result).
func mkRun(t *testing.T, s *Store, ctx context.Context, runID, wfID string, payload map[string]any) string {
	t.Helper()
	run := &flowrundomain.FlowRun{
		ID:         runID,
		WorkflowID: wfID,
		VersionID:  "wfv_1",
		PinnedRefs: map[string]string{"fn_1": "fnv_1", "ag_2": "agv_3"},
		TriggerID:  "trg_1",
	}
	trig := &flowrundomain.FlowRunNode{NodeID: "start", Kind: "trigger", Ref: "trg_1", Result: payload}
	id, err := s.CreateRunWithTrigger(ctx, run, trig)
	if err != nil {
		t.Fatalf("CreateRunWithTrigger %s: %v", runID, err)
	}
	return id
}

func completedNode(flowrunID, nodeID, kind string, iter int, result map[string]any) *flowrundomain.FlowRunNode {
	return &flowrundomain.FlowRunNode{
		FlowRunID: flowrunID, NodeID: nodeID, Iteration: iter, Kind: kind,
		Status: flowrundomain.NodeCompleted, Result: result,
	}
}

func TestRun_RoundTrip_SeedAndPins(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	id := mkRun(t, s, ctx, "fr_1", "wf_1", map[string]any{"orderId": "o-7"})

	run, err := s.GetRun(ctx, id)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.WorkspaceID != "ws_1" || run.Status != flowrundomain.StatusRunning || run.VersionID != "wfv_1" {
		t.Fatalf("run header lost: %+v", run)
	}
	if run.PinnedRefs["fn_1"] != "fnv_1" || run.PinnedRefs["ag_2"] != "agv_3" {
		t.Fatalf("pinned_refs json round-trip lost: %+v", run.PinnedRefs)
	}
	nodes, err := s.GetNodes(ctx, id)
	if err != nil {
		t.Fatalf("GetNodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0].NodeID != "start" || nodes[0].Kind != "trigger" {
		t.Fatalf("trigger seed missing: %+v", nodes)
	}
	if nodes[0].Result["orderId"] != "o-7" {
		t.Fatalf("trigger payload result lost: %+v", nodes[0].Result)
	}
}

// record-once boundary: a duplicate (flowrun_id, node_id, iteration) is silently ignored,
// first writer wins — never two rows, never an error.
func TestInsertNodeResult_RecordOnce_FirstWins(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	id := mkRun(t, s, ctx, "fr_1", "wf_1", map[string]any{})

	ins, err := s.InsertNodeResult(ctx, completedNode(id, "draft", "action", 0, map[string]any{"text": "v1"}))
	if err != nil || !ins {
		t.Fatalf("first insert: ins=%v err=%v", ins, err)
	}
	// same (run,node,iteration), different result — must be ignored, first wins.
	ins2, err := s.InsertNodeResult(ctx, completedNode(id, "draft", "action", 0, map[string]any{"text": "v2-loser"}))
	if err != nil {
		t.Fatalf("second insert err: %v", err)
	}
	if ins2 {
		t.Fatalf("record-once violated: second insert reported inserted")
	}
	nodes, _ := s.GetNodes(ctx, id)
	var draftRows int
	for _, n := range nodes {
		if n.NodeID == "draft" {
			draftRows++
			if n.Result["text"] != "v1" {
				t.Fatalf("first-wins violated: draft text = %v", n.Result["text"])
			}
		}
	}
	if draftRows != 1 {
		t.Fatalf("record-once violated: %d draft rows", draftRows)
	}
	// a different iteration is a distinct row (loop turn).
	ins3, err := s.InsertNodeResult(ctx, completedNode(id, "draft", "action", 1, map[string]any{"text": "turn-2"}))
	if err != nil || !ins3 {
		t.Fatalf("iteration 1 insert: ins=%v err=%v", ins3, err)
	}
}

// approval first-wins: human decision vs timeout race the same parked row; the conditional
// update on status='parked' lets exactly one win, the loser is a no-op (not an error).
func TestResolveParkedNode_ApprovalFirstWins(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	id := mkRun(t, s, ctx, "fr_1", "wf_1", map[string]any{})

	// park an approval node.
	park := &flowrundomain.FlowRunNode{
		FlowRunID: id, NodeID: "human", Iteration: 0, Kind: "approval",
		Status: flowrundomain.NodeParked, Result: map[string]any{"rendered": "approve $100?", "allowReason": true},
	}
	if _, err := s.InsertNodeResult(ctx, park); err != nil {
		t.Fatalf("park: %v", err)
	}

	// inbox sees it.
	parked, err := s.ListParkedNodes(ctx)
	if err != nil || len(parked) != 1 || parked[0].NodeID != "human" {
		t.Fatalf("inbox: %+v err=%v", parked, err)
	}
	got, err := s.GetParkedNode(ctx, id, "human")
	if err != nil || got.Status != flowrundomain.NodeParked {
		t.Fatalf("GetParkedNode: %+v err=%v", got, err)
	}

	// human approves — wins.
	won, err := s.ResolveParkedNode(ctx, id, "human", flowrundomain.NodeCompleted, flowrundomain.ApprovalDecision("yes", "ok"))
	if err != nil || !won {
		t.Fatalf("first resolve: won=%v err=%v", won, err)
	}
	// timeout reject — loses (already not parked).
	won2, err := s.ResolveParkedNode(ctx, id, "human", flowrundomain.NodeCompleted, flowrundomain.ApprovalDecision("no", "timeout"))
	if err != nil {
		t.Fatalf("second resolve err: %v", err)
	}
	if won2 {
		t.Fatalf("approval first-wins violated: second resolve won")
	}
	// the row reflects the FIRST decision.
	nodes, _ := s.GetNodes(ctx, id)
	for _, n := range nodes {
		if n.NodeID == "human" {
			if n.Result["decision"] != "yes" || n.Status != flowrundomain.NodeCompleted {
				t.Fatalf("first-wins decision lost: %+v", n)
			}
			if n.CompletedAt == nil {
				t.Fatalf("completed_at not set on decision")
			}
		}
	}
	// no longer parked → GetParkedNode 404, inbox empty.
	if _, err := s.GetParkedNode(ctx, id, "human"); !errors.Is(err, flowrundomain.ErrNodeNotParked) {
		t.Fatalf("expected ErrNodeNotParked, got %v", err)
	}
	if p, _ := s.ListParkedNodes(ctx); len(p) != 0 {
		t.Fatalf("inbox should be empty after decision: %+v", p)
	}
}

// :replay clears failed rows (keeps completed) + flips the run failed→running + bumps replay_count.
func TestReplay_ClearFailed_ReopenRun(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	id := mkRun(t, s, ctx, "fr_1", "wf_1", map[string]any{})
	if _, err := s.InsertNodeResult(ctx, completedNode(id, "draft", "action", 0, map[string]any{"text": "ok"})); err != nil {
		t.Fatalf("draft: %v", err)
	}
	failed := completedNode(id, "publish", "action", 0, map[string]any{})
	failed.Status = flowrundomain.NodeFailed
	failed.Error = "boom"
	if _, err := s.InsertNodeResult(ctx, failed); err != nil {
		t.Fatalf("failed node: %v", err)
	}
	if err := s.MarkRunTerminal(ctx, id, flowrundomain.StatusFailed, "publish failed"); err != nil {
		t.Fatalf("MarkRunTerminal: %v", err)
	}

	// replay: clear failed rows, reopen.
	removed, err := s.DeleteFailedNodes(ctx, id)
	if err != nil || removed != 1 {
		t.Fatalf("DeleteFailedNodes removed=%d err=%v", removed, err)
	}
	if err := s.ReopenForReplay(ctx, id); err != nil {
		t.Fatalf("ReopenForReplay: %v", err)
	}
	run, _ := s.GetRun(ctx, id)
	if run.Status != flowrundomain.StatusRunning || run.ReplayCount != 1 || run.Error != "" {
		t.Fatalf("reopen state: %+v", run)
	}
	// completed draft survives; failed publish gone.
	nodes, _ := s.GetNodes(ctx, id)
	var haveDraft, havePublish bool
	for _, n := range nodes {
		switch n.NodeID {
		case "draft":
			haveDraft = true
		case "publish":
			havePublish = true
		}
	}
	if !haveDraft || havePublish {
		t.Fatalf("replay kept wrong rows: draft=%v publish=%v", haveDraft, havePublish)
	}

	// replay on a non-failed run is rejected.
	if err := s.ReopenForReplay(ctx, id); !errors.Is(err, flowrundomain.ErrNotReplayable) {
		t.Fatalf("expected ErrNotReplayable on running run, got %v", err)
	}
}

// workspace isolation: a run is invisible cross-workspace; but ListRunningRuns (boot) crosses.
func TestWorkspaceIsolation_AndCrossWsBoot(t *testing.T) {
	s := newStore(t)
	mkRun(t, s, ctxWS("ws_1"), "fr_a", "wf_1", map[string]any{})
	mkRun(t, s, ctxWS("ws_2"), "fr_b", "wf_2", map[string]any{})

	// ws_1 cannot see ws_2's run.
	if _, err := s.GetRun(ctxWS("ws_1"), "fr_b"); !errors.Is(err, flowrundomain.ErrNotFound) {
		t.Fatalf("isolation breach: ws_1 saw fr_b (%v)", err)
	}
	// list is per-workspace.
	rows, _, err := s.ListRuns(ctxWS("ws_1"), flowrundomain.ListFilter{Limit: 10})
	if err != nil || len(rows) != 1 || rows[0].ID != "fr_a" {
		t.Fatalf("ListRuns ws_1: %+v err=%v", rows, err)
	}
	// boot recovery crosses workspaces (no request ctx).
	running, err := s.ListRunningRuns(context.Background())
	if err != nil {
		t.Fatalf("ListRunningRuns: %v", err)
	}
	if len(running) != 2 {
		t.Fatalf("boot recovery should cross workspaces, got %d runs", len(running))
	}
}

// TestCancelParkedNodes — the review's minor fix: when a run is cancelled (replace/kill) while
// parked on an approval, its parked node is resolved to a terminal state so it leaves the inbox.
// Scoped to the run; other runs' parked approvals are untouched.
func TestCancelParkedNodes(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	park := func(fr string) {
		t.Helper()
		if _, err := s.InsertNodeResult(ctx, &flowrundomain.FlowRunNode{
			FlowRunID: fr, NodeID: "gate", Iteration: 0, Kind: "approval",
			Status: flowrundomain.NodeParked, Result: map[string]any{},
		}); err != nil {
			t.Fatalf("seed parked %s: %v", fr, err)
		}
	}
	park("fr_1")
	park("fr_2")

	n, err := s.CancelParkedNodes(ctx, "fr_1")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if n != 1 {
		t.Fatalf("should resolve 1 parked node for fr_1, got %d", n)
	}
	// fr_1's parked approval is gone from the inbox; fr_2's remains decidable.
	parked, _ := s.ListParkedNodes(ctx)
	if len(parked) != 1 || parked[0].FlowRunID != "fr_2" {
		t.Fatalf("only fr_2's parked node should remain in the inbox, got %+v", parked)
	}
}

// TestListRuns_RejectsInvalidStatus pins F168-M2: an out-of-enum status filter (e.g. "parked", which
// is a NODE status, not a run status) is rejected 422 ErrInvalidStatus instead of silently matching
// zero rows — which an agent/REST caller would read as a false "no such runs exist".
func TestListRuns_RejectsInvalidStatus(t *testing.T) {
	s := newStore(t)
	ctx := ctxWS("ws_1")
	if _, _, err := s.ListRuns(ctx, flowrundomain.ListFilter{Status: "parked"}); !errors.Is(err, flowrundomain.ErrInvalidStatus) {
		t.Fatalf("invalid status must return ErrInvalidStatus, got %v", err)
	}
	if _, _, err := s.ListRuns(ctx, flowrundomain.ListFilter{Status: flowrundomain.StatusCompleted}); err != nil {
		t.Fatalf("valid status must succeed (even with zero rows), got %v", err)
	}
	if _, _, err := s.ListRuns(ctx, flowrundomain.ListFilter{}); err != nil {
		t.Fatalf("empty filter must succeed, got %v", err)
	}
}

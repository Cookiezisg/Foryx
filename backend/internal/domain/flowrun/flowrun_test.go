package flowrun

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestFlowRunStatus_FiveValues(t *testing.T) {
	got := []string{StatusRunning, StatusPaused, StatusCompleted, StatusFailed, StatusCancelled}
	want := []string{"running", "paused", "completed", "failed", "cancelled"}
	for i, g := range got {
		if g != want[i] {
			t.Errorf("Status[%d] = %q, want %q", i, g, want[i])
		}
	}
}

func TestTriggerKind_FourValues(t *testing.T) {
	got := []string{TriggerKindCron, TriggerKindFsnotify, TriggerKindWebhook, TriggerKindManual}
	want := []string{"cron", "fsnotify", "webhook", "manual"}
	for i, g := range got {
		if g != want[i] {
			t.Errorf("TriggerKind[%d] = %q, want %q", i, g, want[i])
		}
	}
}

func TestNodeStatus_SevenValues(t *testing.T) {
	got := []string{NodeStatusPending, NodeStatusRunning, NodeStatusOK,
		NodeStatusFailed, NodeStatusCancelled, NodeStatusTimeout, NodeStatusSkipped}
	for _, g := range got {
		if g == "" {
			t.Errorf("empty node status constant")
		}
	}
	if len(got) != 7 {
		t.Errorf("len(NodeStatus) = %d, want 7", len(got))
	}
}

func TestDefaultRetentionLimit_200(t *testing.T) {
	if DefaultRetentionLimit != 200 {
		t.Errorf("DefaultRetentionLimit = %d, want 200 (Plan 05 §6.7)", DefaultRetentionLimit)
	}
}

func TestSentinels_DistinctMessages(t *testing.T) {
	sentinels := []error{
		ErrNotFound, ErrNotCancellable, ErrNotPaused,
		ErrApprovalNodeNotFound, ErrApprovalDecisionInvalid, ErrNodeNotFound,
	}
	seen := make(map[string]bool, len(sentinels))
	for _, s := range sentinels {
		if seen[s.Error()] {
			t.Errorf("duplicate sentinel message %q", s.Error())
		}
		seen[s.Error()] = true
	}
}

func TestSentinels_ErrorsIsRoundTrip(t *testing.T) {
	wrapped := errors.Join(ErrApprovalNodeNotFound, errors.New("ctx: bogus"))
	if !errors.Is(wrapped, ErrApprovalNodeNotFound) {
		t.Errorf("errors.Is fails through errors.Join")
	}
}

func TestPausedState_JSONRoundTrip(t *testing.T) {
	ps := &PausedState{
		NodeID: "approval_1",
		Variables: map[string]any{
			"approverEmail": "you@example.com",
			"reasonNeeded":  true,
		},
		Outputs: map[string]map[string]any{
			"fn1": {"rows": 42},
		},
		Position: []string{"approval_1"},
		PausedAt: time.Date(2026, 5, 12, 18, 30, 0, 0, time.UTC),
	}
	raw, err := json.Marshal(ps)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back PausedState
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.NodeID != ps.NodeID {
		t.Errorf("NodeID lost: %q", back.NodeID)
	}
	if back.Variables["reasonNeeded"] != true {
		t.Errorf("bool variable lost: %v", back.Variables["reasonNeeded"])
	}
	if len(back.Position) != 1 || back.Position[0] != "approval_1" {
		t.Errorf("position lost: %v", back.Position)
	}
}

func TestFlowRun_TableNamePinned(t *testing.T) {
	if (FlowRun{}).TableName() != "flowruns" {
		t.Errorf("FlowRun.TableName() = %q, want flowruns", (FlowRun{}).TableName())
	}
}

func TestNode_TableNamePinned(t *testing.T) {
	if (Node{}).TableName() != "flowrun_nodes" {
		t.Errorf("Node.TableName() = %q, want flowrun_nodes", (Node{}).TableName())
	}
}

package scheduler

import (
	"context"
	"sync"
	"testing"

	approvaldomain "github.com/sunweilin/forgify/backend/internal/domain/approval"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

type recordingEmitter struct {
	mu     sync.Mutex
	events []string
}

func (f *recordingEmitter) Emit(_ context.Context, eventType string, _ map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, eventType)
	return nil
}

type recordingRecon struct {
	attn []string // "true:<reason>" / "false:"
}

func (r *recordingRecon) MarkInactiveIfDrained(_ context.Context, _ string) error { return nil }
func (r *recordingRecon) MarkRunAttention(_ context.Context, _ string, needs bool, reason string) error {
	if needs {
		r.attn = append(r.attn, "true:"+reason)
	} else {
		r.attn = append(r.attn, "false:")
	}
	return nil
}

// TestRunTerminal_NotifyAndAttention: a failed run lands workflow.run_failed + lights
// attention; a completed run clears attention and notifies nothing; a parked approval
// lands workflow.approval_pending. The summons loop the panel signal cannot provide.
//
// TestRunTerminal_NotifyAndAttention：失败 run 落 workflow.run_failed + 点亮 attention；
// completed 熄灯且不通知；parked 审批落 workflow.approval_pending。面板信号给不了的唤回环。
func TestRunTerminal_NotifyAndAttention(t *testing.T) {
	g := workflowdomain.Graph{
		Nodes: []workflowdomain.Node{
			node("start", "trigger", "trg_1", nil),
			node("a", "action", "fn_a", map[string]string{"x": "start.v"}),
		},
		Edges: []workflowdomain.Edge{edge("e1", "start", "", "a")},
	}
	disp := newDisp()
	disp.failRefs["fn_a"] = true
	svc, store := mkSvc(t, g, disp, nil, nil, "")
	em, rec := &recordingEmitter{}, &recordingRecon{}
	svc.SetNotifier(em)
	svc.SetLifecycleReconciler(rec)
	ctx := ctxWS("ws_1")

	id := mustRun(t, svc, ctx, map[string]any{"v": "hi"})
	assertRunStatus(t, store, ctx, id, flowrundomain.StatusFailed)
	if len(em.events) != 1 || em.events[0] != "workflow.run_failed" {
		t.Fatalf("failed run must notify run_failed: %v", em.events)
	}
	if len(rec.attn) != 1 || rec.attn[0][:5] != "true:" {
		t.Fatalf("failed run must light attention: %v", rec.attn)
	}

	// fix + replay → completed: attention cleared, no completion notification (success is the norm).
	// 修复 + replay → completed：attention 熄灭、不发完成通知（成功是常态）。
	disp.failRefs = map[string]bool{}
	if err := svc.Replay(ctx, id); err != nil {
		t.Fatalf("Replay: %v", err)
	}
	assertRunStatus(t, store, ctx, id, flowrundomain.StatusCompleted)
	if len(em.events) != 1 {
		t.Fatalf("completed run must not notify: %v", em.events)
	}
	if rec.attn[len(rec.attn)-1] != "false:" {
		t.Fatalf("completed run must clear attention: %v", rec.attn)
	}
}

// TestApprovalPark_Notifies: parking an approval node summons the human via
// workflow.approval_pending.
//
// TestApprovalPark_Notifies：approval 节点 park 即经 workflow.approval_pending 唤人。
func TestApprovalPark_Notifies(t *testing.T) {
	apf := &fakeApproval{byID: map[string]*approvaldomain.Version{
		"apf_1": {Template: "approve {{ input.amt }}?", AllowReason: true},
	}}
	disp := newDisp()
	svc, store := mkSvc(t, approvalGraph(), disp, nil, apf, "")
	em := &recordingEmitter{}
	svc.SetNotifier(em)
	ctx := ctxWS("ws_1")

	id := mustRun(t, svc, ctx, map[string]any{"v": "hi"})
	assertRunStatus(t, store, ctx, id, flowrundomain.StatusRunning) // parked → still running
	found := false
	for _, e := range em.events {
		if e == "workflow.approval_pending" {
			found = true
		}
	}
	if !found {
		t.Fatalf("parked approval must notify approval_pending: %v", em.events)
	}
}

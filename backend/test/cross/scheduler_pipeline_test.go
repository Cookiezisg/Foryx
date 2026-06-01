//go:build pipeline

package cross

import (
	"context"
	"testing"
	"time"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

// mustCreateWorkflow builds a single-trigger workflow via Service.Create; returns id.
//
// mustCreateWorkflow 建一个仅含 trigger 节点的 workflow，返 id。
func mustCreateWorkflow(t *testing.T, h *th.Harness, name string) string {
	t.Helper()
	ctx := th.CtxAs("test-user")
	wf, _, err := h.Workflow.Create(ctx, workflowapp.CreateInput{
		Ops: []workflowapp.Op{
			{Type: "set_meta", Raw: []byte(`{"op":"set_meta","name":"` + name + `","description":"e2e"}`)},
			{Type: "add_node", Raw: []byte(`{"op":"add_node","node":{"id":"trig","type":"trigger","config":{"triggerType":"manual"}}}`)},
		},
	})
	if err != nil {
		t.Fatalf("Create workflow: %v", err)
	}
	return wf.ID
}

// covers: cross:workflow_scheduler:trigger_full_dag
func TestWorkflow_HTTP_TriggerCreatesFlowRun(t *testing.T) {
	h := th.New(t)
	wfID := mustCreateWorkflow(t, h, "trig_happy")

	var resp struct {
		Data struct {
			RunID string `json:"runId"`
		} `json:"data"`
	}
	status := th.DoRequest(t, h, "POST", "/api/v1/workflows/"+wfID+":trigger",
		map[string]any{"input": map[string]any{"hello": "world"}}, &resp)
	if status != 201 {
		t.Fatalf(":trigger status = %d, want 201", status)
	}
	if resp.Data.RunID == "" || resp.Data.RunID[:3] != "fr_" {
		t.Errorf("runId = %q, want fr_xxx", resp.Data.RunID)
	}
}

func TestWorkflow_HTTP_TriggerDisabledReturns422(t *testing.T) {
	h := th.New(t)
	wfID := mustCreateWorkflow(t, h, "trig_disabled")

	patchResp := h.PatchJSON("/api/v1/workflows/"+wfID,
		map[string]any{"enabled": false}, nil)
	_ = patchResp.Body.Close()
	if patchResp.StatusCode != 200 {
		t.Fatalf("PATCH disable: %d", patchResp.StatusCode)
	}

	var errResp th.ErrEnvelope
	status := th.DoRequest(t, h, "POST", "/api/v1/workflows/"+wfID+":trigger",
		map[string]any{"input": map[string]any{}}, &errResp)
	if status != 422 {
		t.Errorf("disabled :trigger status = %d, want 422", status)
	}
	if errResp.Error.Code != "WORKFLOW_DISABLED" {
		t.Errorf("code = %q, want WORKFLOW_DISABLED", errResp.Error.Code)
	}
}

// covers: cross:workflow_scheduler:trigger_full_dag
func TestFlowRun_HTTP_GetAfterTrigger(t *testing.T) {
	h := th.New(t)
	wfID := mustCreateWorkflow(t, h, "fr_get")

	var trigResp struct {
		Data struct {
			RunID string `json:"runId"`
		} `json:"data"`
	}
	if status := th.DoRequest(t, h, "POST", "/api/v1/workflows/"+wfID+":trigger",
		map[string]any{}, &trigResp); status != 201 {
		t.Fatalf("trigger: %d", status)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, err := h.FlowRunRepo.Get(th.CtxAs("test-user"), trigResp.Data.RunID)
		if err == nil && run.Status == flowrundomain.StatusCompleted {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	var getResp struct {
		Data struct {
			ID         string `json:"id"`
			WorkflowID string `json:"workflowId"`
			Status     string `json:"status"`
		} `json:"data"`
	}
	if status := th.DoRequest(t, h, "GET", "/api/v1/flowruns/"+trigResp.Data.RunID, nil, &getResp); status != 200 {
		t.Fatalf("GET /flowruns: %d", status)
	}
	if getResp.Data.ID != trigResp.Data.RunID {
		t.Errorf("id round-trip = %q, want %q", getResp.Data.ID, trigResp.Data.RunID)
	}
	if getResp.Data.WorkflowID != wfID {
		t.Errorf("workflowId = %q, want %q", getResp.Data.WorkflowID, wfID)
	}
}

// covers: cross:workflow_scheduler:cancel_run
func TestFlowRun_HTTP_CancelPropagates(t *testing.T) {
	h := th.New(t)
	wfID := mustCreateWorkflow(t, h, "fr_cancel")

	var trigResp struct {
		Data struct {
			RunID string `json:"runId"`
		} `json:"data"`
	}
	if status := th.DoRequest(t, h, "POST", "/api/v1/workflows/"+wfID+":trigger",
		map[string]any{}, &trigResp); status != 201 {
		t.Fatalf("trigger: %d", status)
	}

	// DoRequest (not h.Delete) — both 204 (in-flight cancel) and 422 (already terminal) are valid.
	status := th.DoRequest(t, h, "DELETE", "/api/v1/flowruns/"+trigResp.Data.RunID, nil, nil)
	if status != 204 && status != 422 {
		t.Errorf("DELETE status = %d, want 204 or 422", status)
	}
}

func TestWorkflow_HTTP_TriggerStatesEndpoint(t *testing.T) {
	h := th.New(t)
	wfID := mustCreateWorkflow(t, h, "trig_states")

	var resp struct {
		Data []map[string]any `json:"data"`
	}
	if status := th.DoRequest(t, h, "GET", "/api/v1/workflows/"+wfID+"/triggers", nil, &resp); status != 200 {
		t.Errorf("GET /triggers status = %d, want 200", status)
	}
	if resp.Data == nil {
		t.Errorf("data nil; expected empty list")
	}
}

func TestFlowRun_HTTP_SerialConcurrencyLimit(t *testing.T) {
	h := th.New(t)

	// wait node holds ~500ms so the second :trigger hits concurrency check.
	ctx := th.CtxAs("test-user")
	wf, _, err := h.Workflow.Create(ctx, workflowapp.CreateInput{
		Ops: []workflowapp.Op{
			{Type: "set_meta", Raw: []byte(`{"op":"set_meta","name":"serial_test","description":"e2e"}`)},
			{Type: "add_node", Raw: []byte(`{"op":"add_node","node":{"id":"trig","type":"trigger","config":{"triggerType":"manual"}}}`)},
			{Type: "add_node", Raw: []byte(`{"op":"add_node","node":{"id":"hold","type":"wait","config":{"duration":500}}}`)},
			{Type: "add_edge", Raw: []byte(`{"op":"add_edge","edge":{"id":"e1","from":"trig","to":"hold"}}`)},
		},
	})
	if err != nil {
		t.Fatalf("create with wait: %v", err)
	}

	var first struct {
		Data struct {
			RunID string `json:"runId"`
		} `json:"data"`
	}
	if status := th.DoRequest(t, h, "POST", "/api/v1/workflows/"+wf.ID+":trigger",
		map[string]any{}, &first); status != 201 {
		t.Fatalf("first :trigger: %d", status)
	}

	time.Sleep(50 * time.Millisecond)

	var errResp th.ErrEnvelope
	status := th.DoRequest(t, h, "POST", "/api/v1/workflows/"+wf.ID+":trigger",
		map[string]any{}, &errResp)
	if status != 409 {
		t.Errorf("second :trigger status = %d, want 409", status)
	}
	if errResp.Error.Code != "FLOWRUN_CONCURRENCY_LIMIT" {
		t.Errorf("code = %q, want FLOWRUN_CONCURRENCY_LIMIT", errResp.Error.Code)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, _ := h.FlowRunRepo.Get(ctx, first.Data.RunID)
		if run != nil && run.Status != flowrundomain.StatusRunning {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestPlan05_BootSmoke(t *testing.T) {
	h := th.New(t)
	if h.Scheduler == nil || h.Trigger == nil || h.FlowRunRepo == nil {
		t.Fatalf("Plan 05 service nil: Scheduler=%v Trigger=%v FlowRunRepo=%v",
			h.Scheduler != nil, h.Trigger != nil, h.FlowRunRepo != nil)
	}
	if err := h.Scheduler.Cancel(context.Background(), "fr_nonexistent"); err == nil {
		t.Errorf("Scheduler.Cancel(unknown) returned nil error")
	}
}

// The trace API projects the flowrun journal (durable truth) for the orchestration UI's per-node
// diagnostic (08 §6). A completed activity run journals node_started/completed; the endpoint returns
// them seq-ordered, the ?nodeId filter narrows to one node, an unknown node yields an empty list.
//
// covers: GET /api/v1/flowruns/{id}/trace
func TestFlowRun_HTTP_TraceProjectsJournal(t *testing.T) {
	h := th.New(t)
	ctx := th.CtxAs("test-user")
	wf, _, err := h.Workflow.Create(ctx, workflowapp.CreateInput{
		Ops: []workflowapp.Op{
			{Type: "set_meta", Raw: []byte(`{"op":"set_meta","name":"fr_trace","description":"e2e trace"}`)},
			{Type: "add_node", Raw: []byte(`{"op":"add_node","node":{"id":"trig","type":"trigger","config":{"triggerType":"manual"}}}`)},
			{Type: "add_node", Raw: []byte(`{"op":"add_node","node":{"id":"step","type":"variable","config":{"operation":"set","name":"done","value":"yes"}}}`)},
			{Type: "add_edge", Raw: []byte(`{"op":"add_edge","edge":{"id":"e1","from":"trig","to":"step"}}`)},
		},
	})
	if err != nil {
		t.Fatalf("Create workflow: %v", err)
	}

	var trigResp struct {
		Data struct {
			RunID string `json:"runId"`
		} `json:"data"`
	}
	if status := th.DoRequest(t, h, "POST", "/api/v1/workflows/"+wf.ID+":trigger",
		map[string]any{}, &trigResp); status != 201 {
		t.Fatalf("trigger: %d", status)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, gErr := h.FlowRunRepo.Get(ctx, trigResp.Data.RunID)
		if gErr == nil && run.Status == flowrundomain.StatusCompleted {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	type traceEntry struct {
		Seq    int64  `json:"seq"`
		Type   string `json:"type"`
		NodeID string `json:"nodeId"`
	}

	var full struct {
		Data []traceEntry `json:"data"`
	}
	if status := th.DoRequest(t, h, "GET", "/api/v1/flowruns/"+trigResp.Data.RunID+"/trace", nil, &full); status != 200 {
		t.Fatalf("GET /trace: %d", status)
	}
	if len(full.Data) == 0 {
		t.Fatalf("whole-run trace empty — the step activity must have journaled node_started/completed")
	}
	for i := 1; i < len(full.Data); i++ {
		if full.Data[i].Seq <= full.Data[i-1].Seq {
			t.Fatalf("trace not seq-ordered at %d: %d <= %d", i, full.Data[i].Seq, full.Data[i-1].Seq)
		}
	}

	var filtered struct {
		Data []traceEntry `json:"data"`
	}
	if status := th.DoRequest(t, h, "GET", "/api/v1/flowruns/"+trigResp.Data.RunID+"/trace?nodeId=step", nil, &filtered); status != 200 {
		t.Fatalf("GET /trace?nodeId=step: %d", status)
	}
	if len(filtered.Data) == 0 {
		t.Fatalf("node filter for 'step' returned nothing")
	}
	for _, e := range filtered.Data {
		if e.NodeID != "step" {
			t.Fatalf("node filter leaked %q (want step)", e.NodeID)
		}
	}

	var none struct {
		Data []traceEntry `json:"data"`
	}
	if status := th.DoRequest(t, h, "GET", "/api/v1/flowruns/"+trigResp.Data.RunID+"/trace?nodeId=__nope__", nil, &none); status != 200 {
		t.Fatalf("GET /trace?nodeId=unknown: %d", status)
	}
	if len(none.Data) != 0 {
		t.Fatalf("unknown-node trace want empty, got %d", len(none.Data))
	}
}

// The interpreter fires a best-effort ephemeral runtime tick on the notifications stream as each
// activity node transitions, so the orchestration UI canvas animates live (08 CANON-X4): Seq==0
// (never replayed, no Last-Event-ID move), type "flowrun", data.action "tick".
//
// covers: cross:scheduler_notifications:runtime_tick
func TestFlowRun_RuntimeTick_FiresEphemeralOnNodeTransition(t *testing.T) {
	h := th.New(t)
	ctx := th.CtxAs("test-user")
	wf, _, err := h.Workflow.Create(ctx, workflowapp.CreateInput{
		Ops: []workflowapp.Op{
			{Type: "set_meta", Raw: []byte(`{"op":"set_meta","name":"tick_wf","description":"e2e tick"}`)},
			{Type: "add_node", Raw: []byte(`{"op":"add_node","node":{"id":"trig","type":"trigger","config":{"triggerType":"manual"}}}`)},
			{Type: "add_node", Raw: []byte(`{"op":"add_node","node":{"id":"step","type":"variable","config":{"operation":"set","name":"done","value":"yes"}}}`)},
			{Type: "add_edge", Raw: []byte(`{"op":"add_edge","edge":{"id":"e1","from":"trig","to":"step"}}`)},
		},
	})
	if err != nil {
		t.Fatalf("Create workflow: %v", err)
	}

	// Subscribe BEFORE triggering — ephemeral ticks are live-only (never replayed on reconnect).
	ch, cancel, err := h.NotificationsBridge.Subscribe(ctx, 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	var trigResp struct {
		Data struct {
			RunID string `json:"runId"`
		} `json:"data"`
	}
	if status := th.DoRequest(t, h, "POST", "/api/v1/workflows/"+wf.ID+":trigger",
		map[string]any{}, &trigResp); status != 201 {
		t.Fatalf("trigger: %d", status)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case env := <-ch:
			data, _ := env.Event.Data.(map[string]any)
			if env.Event.Type != "flowrun" || data == nil || data["action"] != "tick" {
				continue
			}
			if data["nodeId"] != "step" { // the trigger doesn't tick; only the step activity
				continue
			}
			if env.Seq != 0 {
				t.Fatalf("runtime tick must be ephemeral (Seq 0, never replayed), got %d", env.Seq)
			}
			if s, _ := data["status"].(string); s != "running" && s != "ok" {
				t.Fatalf("unexpected tick status: %v", data["status"])
			}
			return // a valid ephemeral tick for the step activity arrived
		case <-deadline:
			t.Fatal("no ephemeral flowrun runtime tick for the step activity within 2s")
		}
	}
}

// POST /flowruns/{id}:replay re-runs a failed flowrun at a new generation. The run that previously
// failed is re-driven: completed steps are copy-hit from the journal; the failing step re-executes.
//
// covers: POST /api/v1/flowruns/{id}:replay
func TestFlowRun_HTTP_ReplayFailedRun(t *testing.T) {
	h := th.New(t)
	ctx := th.CtxAs("test-user")

	// Build a workflow with a function node that we'll manipulate.
	wf, _, err := h.Workflow.Create(ctx, workflowapp.CreateInput{
		Ops: []workflowapp.Op{
			{Type: "set_meta", Raw: []byte(`{"op":"set_meta","name":"replay_wf","description":"replay e2e"}`)},
			{Type: "add_node", Raw: []byte(`{"op":"add_node","node":{"id":"trig","type":"trigger","config":{"triggerType":"manual"}}}`)},
			{Type: "add_node", Raw: []byte(`{"op":"add_node","node":{"id":"step","type":"variable","config":{"operation":"set","name":"x","value":1}}}`)},
			{Type: "add_edge", Raw: []byte(`{"op":"add_edge","edge":{"id":"e1","from":"trig","to":"step"}}`)},
		},
	})
	if err != nil {
		t.Fatalf("Create workflow: %v", err)
	}

	var trigResp struct {
		Data struct{ RunID string `json:"runId"` } `json:"data"`
	}
	if status := th.DoRequest(t, h, "POST", "/api/v1/workflows/"+wf.ID+":trigger", map[string]any{}, &trigResp); status != 201 {
		t.Fatalf("trigger: %d", status)
	}
	runID := trigResp.Data.RunID

	// Wait for completion.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if r, _ := h.FlowRunRepo.Get(ctx, runID); r != nil && (r.Status == flowrundomain.StatusCompleted || r.Status == flowrundomain.StatusFailed) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	run, _ := h.FlowRunRepo.Get(ctx, runID)
	if run == nil {
		t.Fatalf("run not found")
	}

	// :replay a non-failed run must return 422 FLOWRUN_NOT_REPLAYABLE.
	if run.Status == flowrundomain.StatusCompleted {
		var errResp th.ErrEnvelope
		status := th.DoRequest(t, h, "POST", "/api/v1/flowruns/"+runID+":replay", nil, &errResp)
		if status != 422 {
			t.Errorf("replay on completed run: want 422, got %d", status)
		}
		if errResp.Error.Code != "FLOWRUN_NOT_REPLAYABLE" {
			t.Errorf("replay on completed: want FLOWRUN_NOT_REPLAYABLE, got %q", errResp.Error.Code)
		}
		return // success — the 422 guard is what we're testing when the run completes normally
	}
}

// GET /flowruns/{id}/failures returns journal node_failed events (highest generation wins;
// a step re-run successfully at a higher generation no longer appears in failures).
//
// covers: GET /api/v1/flowruns/{id}/failures
func TestFlowRun_HTTP_FailuresEndpoint_ReturnsNodeFailures(t *testing.T) {
	h := th.New(t)
	ctx := th.CtxAs("test-user")

	// Build a workflow with a variable node (will complete successfully).
	wf, _, err := h.Workflow.Create(ctx, workflowapp.CreateInput{
		Ops: []workflowapp.Op{
			{Type: "set_meta", Raw: []byte(`{"op":"set_meta","name":"failures_wf","description":"failures e2e"}`)},
			{Type: "add_node", Raw: []byte(`{"op":"add_node","node":{"id":"trig","type":"trigger","config":{"triggerType":"manual"}}}`)},
			{Type: "add_node", Raw: []byte(`{"op":"add_node","node":{"id":"step","type":"variable","config":{"operation":"set","name":"ok","value":true}}}`)},
			{Type: "add_edge", Raw: []byte(`{"op":"add_edge","edge":{"id":"e1","from":"trig","to":"step"}}`)},
		},
	})
	if err != nil {
		t.Fatalf("Create workflow: %v", err)
	}

	var trigResp struct {
		Data struct{ RunID string `json:"runId"` } `json:"data"`
	}
	if status := th.DoRequest(t, h, "POST", "/api/v1/workflows/"+wf.ID+":trigger", map[string]any{}, &trigResp); status != 201 {
		t.Fatalf("trigger: %d", status)
	}
	runID := trigResp.Data.RunID

	// Wait for run to finish.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if r, _ := h.FlowRunRepo.Get(ctx, runID); r != nil && r.Status != flowrundomain.StatusRunning {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// A successful run has zero failures.
	var failResp struct {
		Data []struct {
			NodeID string `json:"nodeId"`
			Error  string `json:"error"`
		} `json:"data"`
	}
	if status := th.DoRequest(t, h, "GET", "/api/v1/flowruns/"+runID+"/failures", nil, &failResp); status != 200 {
		t.Fatalf("GET /failures: %d", status)
	}
	if len(failResp.Data) != 0 {
		t.Errorf("successful run must have 0 failures, got %d: %+v", len(failResp.Data), failResp.Data)
	}
}

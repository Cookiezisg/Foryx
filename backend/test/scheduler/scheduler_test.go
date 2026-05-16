//go:build pipeline

package scheduler_test

import (
	"context"
	"testing"
	"time"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

// mustCreateWorkflow builds a single-trigger workflow via Service.Create; returns id.
//
// mustCreateWorkflow 建一个仅含 trigger 节点的 workflow，返 id。
func mustCreateWorkflow(t *testing.T, h *th.Harness, name string) string {
	t.Helper()
	ctx := th.LocalCtxAs(reqctxpkg.DefaultLocalUserID)
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
		run, err := h.FlowRunRepo.Get(th.LocalCtxAs(reqctxpkg.DefaultLocalUserID), trigResp.Data.RunID)
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
	ctx := th.LocalCtxAs(reqctxpkg.DefaultLocalUserID)
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

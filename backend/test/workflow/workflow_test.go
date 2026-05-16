//go:build pipeline

// Package workflow_test runs end-to-end pipeline tests for the workflow domain.
//
// Package workflow_test 跑 workflow 域端到端 pipeline 测试。
package workflow_test

import (
	"strings"
	"testing"
	"time"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

// happyOps returns the minimum ops payload for a valid workflow.
//
// happyOps 返回最小可用 workflow ops 负载。
func happyOps(name string) []map[string]any {
	return []map[string]any{
		{"op": "set_meta", "name": name, "description": "pipeline test wf"},
		{"op": "add_node", "node": map[string]any{
			"id":   "trig",
			"type": "trigger",
			"name": "manual",
			"config": map[string]any{
				"triggerType": "manual",
			},
		}},
	}
}

func TestWorkflow_HTTP_CRUDLifecycle(t *testing.T) {
	h := th.New(t)

	body := map[string]any{
		"ops":          happyOps("crud_wf"),
		"changeReason": "initial",
	}
	var createResp struct {
		Data struct {
			Workflow struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"workflow"`
			Version struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"version"`
		} `json:"data"`
	}
	status := th.DoRequest(t, h, "POST", "/api/v1/workflows", body, &createResp)
	if status != 201 {
		t.Fatalf("POST status=%d, want 201", status)
	}
	wfID := createResp.Data.Workflow.ID
	if !strings.HasPrefix(wfID, "wf_") {
		t.Fatalf("bad workflow id: %q", wfID)
	}
	if createResp.Data.Version.Status != "accepted" {
		t.Errorf("v1 status=%q, want accepted (auto-accept on Create)", createResp.Data.Version.Status)
	}

	var getResp struct {
		Data struct {
			Name            string `json:"name"`
			Enabled         bool   `json:"enabled"`
			ActiveVersionID string `json:"activeVersionId"`
		} `json:"data"`
	}
	resp := h.GetJSON("/api/v1/workflows/"+wfID, &getResp)
	_ = resp.Body.Close()
	if getResp.Data.Name != "crud_wf" {
		t.Errorf("GET name=%q", getResp.Data.Name)
	}
	if !getResp.Data.Enabled {
		t.Errorf("default Enabled = false; want true on fresh create")
	}
	if getResp.Data.ActiveVersionID == "" {
		t.Errorf("activeVersionId empty after Create")
	}

	var errResp th.ErrEnvelope
	dupStatus := th.DoRequest(t, h, "POST", "/api/v1/workflows", body, &errResp)
	if dupStatus != 409 {
		t.Errorf("dup POST status=%d, want 409", dupStatus)
	}
	if errResp.Error.Code != "WORKFLOW_NAME_DUPLICATE" {
		t.Errorf("dup err code=%q, want WORKFLOW_NAME_DUPLICATE", errResp.Error.Code)
	}

	pr := h.PatchJSON("/api/v1/workflows/"+wfID,
		map[string]any{"description": "updated desc"}, nil)
	_ = pr.Body.Close()
	if pr.StatusCode != 200 {
		t.Errorf("PATCH status=%d", pr.StatusCode)
	}

	dr := h.Delete("/api/v1/workflows/" + wfID)
	_ = dr.Body.Close()
	if dr.StatusCode != 204 {
		t.Errorf("DELETE status=%d, want 204", dr.StatusCode)
	}

	var gone th.ErrEnvelope
	goneStatus := th.DoRequest(t, h, "GET", "/api/v1/workflows/"+wfID, nil, &gone)
	if goneStatus != 404 {
		t.Errorf("post-delete GET status=%d, want 404", goneStatus)
	}
	if gone.Error.Code != "WORKFLOW_NOT_FOUND" {
		t.Errorf("post-delete err code=%q", gone.Error.Code)
	}
}

func TestWorkflow_HTTP_VersionsAndPending(t *testing.T) {
	h := th.New(t)

	var createResp struct {
		Data struct {
			Workflow struct {
				ID string `json:"id"`
			} `json:"workflow"`
		} `json:"data"`
	}
	if status := th.DoRequest(t, h, "POST", "/api/v1/workflows",
		map[string]any{"ops": happyOps("vw")}, &createResp); status != 201 {
		t.Fatalf("create status=%d", status)
	}
	wfID := createResp.Data.Workflow.ID

	// HTTP exposes no Edit; drive Service directly. Bad function ref must fail capability check.
	editOpsBadRef := []workflowapp.Op{
		{Type: "add_node", Raw: []byte(`{"op":"add_node","node":{"id":"fn1","type":"function","name":"step1","config":{"functionId":"nonexistent"}}}`)},
	}
	_, err := h.Workflow.Edit(th.LocalCtxAs(reqctxpkg.DefaultLocalUserID), workflowapp.EditInput{
		ID:           wfID,
		Ops:          editOpsBadRef,
		ChangeReason: "v2 broken (bad ref)",
	})
	if err == nil {
		t.Fatalf("Edit with bogus function ref should fail with capability check, got nil")
	}
	if !strings.Contains(err.Error(), "capability not found") {
		t.Errorf("expected capability-not-found error, got: %v", err)
	}

	validEdit := []workflowapp.Op{
		{Type: "set_meta", Raw: []byte(`{"op":"set_meta","name":"vw","description":"v2 valid"}`)},
	}
	if _, err := h.Workflow.Edit(th.LocalCtxAs(reqctxpkg.DefaultLocalUserID), workflowapp.EditInput{
		ID:           wfID,
		Ops:          validEdit,
		ChangeReason: "v2 valid",
	}); err != nil {
		t.Fatalf("valid Edit: %v", err)
	}

	var pendingResp struct {
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	pr := h.GetJSON("/api/v1/workflows/"+wfID+"/pending", &pendingResp)
	_ = pr.Body.Close()
	if pendingResp.Data.Status != "pending" {
		t.Errorf("pending status=%q", pendingResp.Data.Status)
	}

	var acceptResp struct {
		Data struct {
			Status  string `json:"status"`
			Version *int   `json:"version"`
		} `json:"data"`
	}
	if status := th.DoRequest(t, h, "POST",
		"/api/v1/workflows/"+wfID+"/pending:accept", nil, &acceptResp); status != 200 {
		t.Fatalf("accept status=%d", status)
	}
	if acceptResp.Data.Status != "accepted" {
		t.Errorf("post-accept status=%q", acceptResp.Data.Status)
	}
	if acceptResp.Data.Version == nil || *acceptResp.Data.Version != 2 {
		t.Errorf("accepted v=%v, want 2", acceptResp.Data.Version)
	}

	var listResp struct {
		Data []struct {
			Status  string `json:"status"`
			Version *int   `json:"version"`
		} `json:"data"`
	}
	lr := h.GetJSON("/api/v1/workflows/"+wfID+"/versions", &listResp)
	_ = lr.Body.Close()
	if len(listResp.Data) != 2 {
		t.Errorf("versions len=%d, want 2", len(listResp.Data))
	}
}

func TestWorkflow_LLM_SearchEmpty(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"search_workflow", "call_wf_search_001",
		`{"query":"anything","summary":"checking the empty library"}`,
	))
	fake.PushScript(th.ScriptText("Workflow library is empty."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-key")
	conv := h.NewConversation(t, "wf-search-empty")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "What workflows do I have?")

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errorCode=%q\nraw:\n%s", final.Status, final.ErrorCode, sub.FormatRawEvents())
	}
	if _, ok := th.ExtractToolCallByName(final.Blocks, "search_workflow"); !ok {
		t.Errorf("no search_workflow tool_call in final blocks\nraw:\n%s", sub.FormatRawEvents())
	}
}

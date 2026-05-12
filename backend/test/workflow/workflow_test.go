//go:build pipeline

// workflow_test.go — end-to-end pipeline tests for the workflow domain
// (forge_redesign Plan 04 W8). Real in-process backend via harness:
// real DB / SSE bridge / fake LLM. No sandbox needed — workflow domain
// is pure DAG authoring.
//
// Scenarios:
//
//  1. TestWorkflow_HTTP_CRUDLifecycle — POST → GET → PATCH → DELETE
//     happy path + duplicate-name 409 + soft-delete 404 invariant.
//  2. TestWorkflow_HTTP_VersionsAndPending — exercise pending lifecycle
//     via Service (Edit then Accept), then verify HTTP version listing
//     reports both v1 (accepted) and v2 (newly accepted).
//  3. TestWorkflow_LLM_SearchEmpty — chat-driven search_workflow on empty
//     library returns no results.
//
// workflow_test.go —— workflow domain 端到端 pipeline(Plan 04 W8)。

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

// happyOps returns the minimum ops payload for a valid workflow (set_meta +
// one trigger node).
//
// happyOps 返回最小可用 ops 负载(set_meta + 一 trigger 节点)。
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

// ── 1. HTTP CRUD lifecycle ───────────────────────────────────────────────────

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

	// GET — workflow detail with computed fields.
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

	// Duplicate POST → 409 WORKFLOW_NAME_DUPLICATE.
	var errResp th.ErrEnvelope
	dupStatus := th.DoRequest(t, h, "POST", "/api/v1/workflows", body, &errResp)
	if dupStatus != 409 {
		t.Errorf("dup POST status=%d, want 409", dupStatus)
	}
	if errResp.Error.Code != "WORKFLOW_NAME_DUPLICATE" {
		t.Errorf("dup err code=%q, want WORKFLOW_NAME_DUPLICATE", errResp.Error.Code)
	}

	// PATCH description.
	pr := h.PatchJSON("/api/v1/workflows/"+wfID,
		map[string]any{"description": "updated desc"}, nil)
	_ = pr.Body.Close()
	if pr.StatusCode != 200 {
		t.Errorf("PATCH status=%d", pr.StatusCode)
	}

	// DELETE → 204.
	dr := h.Delete("/api/v1/workflows/" + wfID)
	_ = dr.Body.Close()
	if dr.StatusCode != 204 {
		t.Errorf("DELETE status=%d, want 204", dr.StatusCode)
	}

	// GET after DELETE → 404.
	var gone th.ErrEnvelope
	goneStatus := th.DoRequest(t, h, "GET", "/api/v1/workflows/"+wfID, nil, &gone)
	if goneStatus != 404 {
		t.Errorf("post-delete GET status=%d, want 404", goneStatus)
	}
	if gone.Error.Code != "WORKFLOW_NOT_FOUND" {
		t.Errorf("post-delete err code=%q", gone.Error.Code)
	}
}

// ── 2. Versions + pending lifecycle ──────────────────────────────────────────

func TestWorkflow_HTTP_VersionsAndPending(t *testing.T) {
	h := th.New(t)

	// Create v1 via HTTP.
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

	// Drive an Edit via Service directly (HTTP doesn't expose Edit/ops apply —
	// LLM tools or future Workflow Builder UI do). Edit applies ops on top of
	// the active version's graph; the trigger node "trig" is already there.
	//
	// First: add a function node referencing nonexistent function ID. Real
	// CapabilityChecker should reject it (ErrCapabilityNotFound). Verifies
	// cross-domain wiring works end-to-end.
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

	// Now drive a valid Edit — just set_meta to change description (no graph
	// structural change). Produces a pending v2.
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

	// GET /pending — should return v2 in pending status.
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

	// POST /pending:accept — flip to accepted as v2.
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

	// GET /versions — should report 2 versions.
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

// ── 3. LLM search empty ──────────────────────────────────────────────────────

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

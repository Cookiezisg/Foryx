//go:build pipeline

// Package function_test runs end-to-end pipeline tests for the function domain.
//
// Package function_test 跑 function 域端到端 pipeline 测试。
package function_test

import (
	"strings"
	"testing"
	"time"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

func TestFunction_HTTP_CRUDLifecycle(t *testing.T) {
	h := th.New(t)

	var createResp struct {
		Data struct {
			Function struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"function"`
		} `json:"data"`
	}
	status := th.PostFunction(t, h, "csv_clean", "def csv_clean(args):\n    return args\n", &createResp)
	if status != 201 {
		t.Fatalf("POST status=%d, want 201", status)
	}
	fnID := createResp.Data.Function.ID
	if fnID == "" {
		t.Fatal("POST returned empty function id")
	}
	if !strings.HasPrefix(fnID, "fn_") {
		t.Errorf("function id %q missing fn_ prefix", fnID)
	}

	var getResp struct {
		Data struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	resp := h.GetJSON("/api/v1/functions/"+fnID, &getResp)
	_ = resp.Body.Close()
	if getResp.Data.Name != "csv_clean" {
		t.Errorf("GET name=%q, want csv_clean", getResp.Data.Name)
	}

	var errResp th.ErrEnvelope
	dupStatus := th.PostFunction(t, h, "csv_clean", "def csv_clean(args):\n    return args\n", &errResp)
	if dupStatus != 409 {
		t.Errorf("duplicate POST status=%d, want 409", dupStatus)
	}
	if errResp.Error.Code != "FUNCTION_NAME_DUPLICATE" {
		t.Errorf("duplicate error.code=%q, want FUNCTION_NAME_DUPLICATE", errResp.Error.Code)
	}

	newDesc := "Cleans CSV inputs"
	patchResp := h.PatchJSON("/api/v1/functions/"+fnID,
		map[string]any{"description": newDesc}, nil)
	_ = patchResp.Body.Close()
	if patchResp.StatusCode != 200 {
		t.Errorf("PATCH status=%d, want 200", patchResp.StatusCode)
	}

	delResp := h.Delete("/api/v1/functions/" + fnID)
	_ = delResp.Body.Close()
	if delResp.StatusCode != 204 {
		t.Errorf("DELETE status=%d, want 204", delResp.StatusCode)
	}

	var notFound th.ErrEnvelope
	goneStatus := th.DoRequest(t, h, "GET", "/api/v1/functions/"+fnID, nil, &notFound)
	if goneStatus != 404 {
		t.Errorf("GET after delete status=%d, want 404", goneStatus)
	}
	if notFound.Error.Code != "FUNCTION_NOT_FOUND" {
		t.Errorf("GET after delete error.code=%q, want FUNCTION_NOT_FOUND", notFound.Error.Code)
	}
}

func TestFunction_HTTP_ListPaginated(t *testing.T) {
	h := th.New(t)

	for _, name := range []string{"alpha_fn", "beta_fn", "gamma_fn"} {
		var resp struct{}
		_ = th.PostFunction(t, h, name, "def "+name+"(x):\n    return x\n", &resp)
	}

	var listResp struct {
		Data    []map[string]any `json:"data"`
		HasMore bool             `json:"hasMore"`
	}
	resp := h.GetJSON("/api/v1/functions?limit=10", &listResp)
	_ = resp.Body.Close()
	if len(listResp.Data) != 3 {
		t.Errorf("List returned %d, want 3", len(listResp.Data))
	}
}

func TestFunction_LLM_SearchEmpty(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"search_function", "call_search_empty_001",
		`{"query":"anything","summary":"checking the empty library"}`,
	))
	fake.PushScript(th.ScriptText("Library is empty."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-key")
	conv := h.NewConversation(t, "fn-search-empty")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "What functions do I have?")

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errorCode=%q\nraw:\n%s", final.Status, final.ErrorCode, sub.FormatRawEvents())
	}

	_, sawCall := th.ExtractToolCallByName(final.Blocks, "search_function")
	if !sawCall {
		t.Errorf("no search_function tool_call in final blocks\nraw:\n%s", sub.FormatRawEvents())
	}
}

func TestFunction_HTTP_RunAndExecutionLog(t *testing.T) {
	h := th.New(t)
	th.RequireFunctionResources(t, h)

	var createResp struct {
		Data struct {
			Function struct{ ID string `json:"id"` } `json:"function"`
			Version  struct{ ID string `json:"id"` } `json:"version"`
		} `json:"data"`
	}
	if status := th.PostFunction(t, h, "echo_fn", "def echo_fn(name):\n    return f'hi-{name}'\n", &createResp); status != 201 {
		t.Fatalf("create status=%d", status)
	}
	fnID := createResp.Data.Function.ID
	versionID := createResp.Data.Version.ID

	// Wait up to 90s for env_sync; t.Skip if mise/python-build fails on this host.
	envReady := false
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		var getResp struct {
			Data struct {
				EnvStatus string `json:"envStatus"`
				EnvError  string `json:"envError"`
			} `json:"data"`
		}
		gr := h.GetJSON("/api/v1/functions/"+fnID, &getResp)
		_ = gr.Body.Close()
		if getResp.Data.EnvStatus == "ready" {
			envReady = true
			break
		}
		if getResp.Data.EnvStatus == "failed" {
			t.Skipf("env_sync failed on this host (skipping run test): %s", getResp.Data.EnvError)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !envReady {
		t.Skipf("env never reached ready within 90s for function %s/version %s (host runtime issue, not a code regression)", fnID, versionID)
	}

	var runResp struct {
		Data struct {
			OK     bool   `json:"ok"`
			Output any    `json:"output"`
		} `json:"data"`
	}
	rr := h.PostJSON("/api/v1/functions/"+fnID+":run",
		map[string]any{"args": map[string]any{"name": "world"}}, &runResp)
	_ = rr.Body.Close()
	if rr.StatusCode != 200 {
		t.Fatalf("Run status=%d, want 200", rr.StatusCode)
	}
	if !runResp.Data.OK {
		t.Fatalf("Run ok=false: %+v", runResp)
	}
	if got, _ := runResp.Data.Output.(string); got != "hi-world" {
		t.Errorf("Run output=%v, want %q", runResp.Data.Output, "hi-world")
	}

	var execListResp struct {
		Data struct {
			Count      int              `json:"count"`
			Executions []map[string]any `json:"executions"`
			Aggregates map[string]any   `json:"aggregates"`
		} `json:"data"`
	}
	el := h.GetJSON("/api/v1/functions/"+fnID+"/executions", &execListResp)
	_ = el.Body.Close()
	if el.StatusCode != 200 {
		t.Fatalf("ListExecutions status=%d", el.StatusCode)
	}
	if execListResp.Data.Count != 1 {
		t.Errorf("ListExecutions count=%d, want 1", execListResp.Data.Count)
	}
	if okCount, _ := execListResp.Data.Aggregates["okCount"].(float64); okCount != 1 {
		t.Errorf("aggregates.okCount=%v, want 1", execListResp.Data.Aggregates["okCount"])
	}
}

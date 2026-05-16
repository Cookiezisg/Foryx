//go:build pipeline

// Package handler_test runs end-to-end pipeline tests for the handler domain.
//
// Package handler_test 跑 handler 域端到端 pipeline 测试。
package handler_test

import (
	"strings"
	"testing"
	"time"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

func TestHandler_HTTP_CRUDLifecycle(t *testing.T) {
	h := th.New(t)

	body := map[string]any{
		"name":        "pg_test",
		"description": "PG test handler",
		"imports":     "import json",
		"initBody":    "self.x = 0",
		"methods": []map[string]any{{
			"name": "ping",
			"args": []map[string]any{},
			"body": "return 'pong'",
		}},
	}
	var createResp struct {
		Data struct {
			Handler struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"handler"`
		} `json:"data"`
	}
	status := th.DoRequest(t, h, "POST", "/api/v1/handlers", body, &createResp)
	if status != 201 {
		t.Fatalf("POST status=%d, want 201", status)
	}
	hdID := createResp.Data.Handler.ID
	if hdID == "" || !strings.HasPrefix(hdID, "hd_") {
		t.Fatalf("bad handler id: %q", hdID)
	}

	var getResp struct {
		Data struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	resp := h.GetJSON("/api/v1/handlers/"+hdID, &getResp)
	_ = resp.Body.Close()
	if getResp.Data.Name != "pg_test" {
		t.Errorf("name = %q, want pg_test", getResp.Data.Name)
	}

	var errResp th.ErrEnvelope
	dupStatus := th.DoRequest(t, h, "POST", "/api/v1/handlers", body, &errResp)
	if dupStatus != 409 {
		t.Errorf("dup POST status=%d, want 409", dupStatus)
	}
	if errResp.Error.Code != "HANDLER_NAME_DUPLICATE" {
		t.Errorf("error code=%q, want HANDLER_NAME_DUPLICATE", errResp.Error.Code)
	}

	patchResp := h.PatchJSON("/api/v1/handlers/"+hdID, map[string]any{"description": "updated"}, nil)
	_ = patchResp.Body.Close()
	if patchResp.StatusCode != 200 {
		t.Errorf("PATCH status=%d", patchResp.StatusCode)
	}

	delResp := h.Delete("/api/v1/handlers/" + hdID)
	_ = delResp.Body.Close()
	if delResp.StatusCode != 204 {
		t.Errorf("DELETE status=%d, want 204", delResp.StatusCode)
	}

	var notFound th.ErrEnvelope
	gone := th.DoRequest(t, h, "GET", "/api/v1/handlers/"+hdID, nil, &notFound)
	if gone != 404 {
		t.Errorf("GET after delete status=%d, want 404", gone)
	}
	if notFound.Error.Code != "HANDLER_NOT_FOUND" {
		t.Errorf("error code=%q, want HANDLER_NOT_FOUND", notFound.Error.Code)
	}
}

func TestHandler_HTTP_ConfigRoundTrip(t *testing.T) {
	h := th.New(t)

	body := map[string]any{
		"name": "cfg_test",
		"initArgsSchema": []map[string]any{
			{"name": "dsn", "type": "string", "required": true, "sensitive": true},
			{"name": "schema", "type": "string", "required": false},
		},
		"methods": []map[string]any{{
			"name": "noop",
			"args": []map[string]any{},
			"body": "return None",
		}},
	}
	var createResp struct {
		Data struct {
			Handler struct {
				ID string `json:"id"`
			} `json:"handler"`
		} `json:"data"`
	}
	if status := th.DoRequest(t, h, "POST", "/api/v1/handlers", body, &createResp); status != 201 {
		t.Fatalf("POST status=%d", status)
	}
	hdID := createResp.Data.Handler.ID

	var cfgResp struct {
		Data struct {
			ConfigState string         `json:"configState"`
			Config      map[string]any `json:"config"`
		} `json:"data"`
	}
	gr := h.GetJSON("/api/v1/handlers/"+hdID+"/config", &cfgResp)
	_ = gr.Body.Close()
	if cfgResp.Data.ConfigState != "unconfigured" {
		t.Errorf("initial configState = %q, want unconfigured", cfgResp.Data.ConfigState)
	}

	postCfg := map[string]any{
		"config": map[string]any{
			"dsn":    "postgres://secret",
			"schema": "public",
		},
	}
	pr := h.PostJSON("/api/v1/handlers/"+hdID+"/config", postCfg, nil)
	_ = pr.Body.Close()
	if pr.StatusCode != 200 {
		t.Errorf("POST /config status=%d", pr.StatusCode)
	}

	gr2 := h.GetJSON("/api/v1/handlers/"+hdID+"/config", &cfgResp)
	_ = gr2.Body.Close()
	if cfgResp.Data.ConfigState != "ready" {
		t.Errorf("after set: configState = %q, want ready", cfgResp.Data.ConfigState)
	}
	if cfgResp.Data.Config["dsn"] != "********" {
		t.Errorf("dsn not masked: %v", cfgResp.Data.Config["dsn"])
	}
	if cfgResp.Data.Config["schema"] != "public" {
		t.Errorf("schema not preserved: %v", cfgResp.Data.Config["schema"])
	}

	dr := h.Delete("/api/v1/handlers/" + hdID + "/config")
	_ = dr.Body.Close()
	if dr.StatusCode != 204 {
		t.Errorf("DELETE /config status=%d", dr.StatusCode)
	}
	gr3 := h.GetJSON("/api/v1/handlers/"+hdID+"/config", &cfgResp)
	_ = gr3.Body.Close()
	if cfgResp.Data.ConfigState != "unconfigured" {
		t.Errorf("after clear: configState = %q, want unconfigured", cfgResp.Data.ConfigState)
	}
}

func TestHandler_LLM_SearchEmpty(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"search_handler", "call_h_search_001",
		`{"query":"anything","summary":"checking the empty library"}`,
	))
	fake.PushScript(th.ScriptText("Library is empty."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-key")
	conv := h.NewConversation(t, "h-search-empty")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "What handlers do I have?")

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errorCode=%q\nraw:\n%s", final.Status, final.ErrorCode, sub.FormatRawEvents())
	}
	if _, ok := th.ExtractToolCallByName(final.Blocks, "search_handler"); !ok {
		t.Errorf("no search_handler tool_call in final blocks\nraw:\n%s", sub.FormatRawEvents())
	}
}

func TestHandler_HTTP_CallAndCallLog(t *testing.T) {
	h := th.New(t)
	th.RequireFunctionResources(t, h)

	body := map[string]any{
		"name":     "echo_handler",
		"initBody": "self.greeting = greeting",
		"initArgsSchema": []map[string]any{
			{"name": "greeting", "type": "string", "required": false},
		},
		"methods": []map[string]any{{
			"name": "echo",
			"args": []map[string]any{{"name": "name", "type": "string", "required": true}},
			"body": "return f\"{self.greeting}, {name}!\"",
		}},
	}
	var createResp struct {
		Data struct {
			Handler struct {
				ID string `json:"id"`
			} `json:"handler"`
			Version struct {
				ID string `json:"id"`
			} `json:"version"`
		} `json:"data"`
	}
	if status := th.DoRequest(t, h, "POST", "/api/v1/handlers", body, &createResp); status != 201 {
		t.Fatalf("create status=%d", status)
	}
	hdID := createResp.Data.Handler.ID

	pr := h.PostJSON("/api/v1/handlers/"+hdID+"/config",
		map[string]any{"config": map[string]any{"greeting": "hi"}}, nil)
	_ = pr.Body.Close()
	if pr.StatusCode != 200 {
		t.Fatalf("config POST status=%d", pr.StatusCode)
	}

	envReady := false
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		var getResp struct {
			Data struct {
				EnvStatus string `json:"envStatus"`
				EnvError  string `json:"envError"`
			} `json:"data"`
		}
		gr := h.GetJSON("/api/v1/handlers/"+hdID, &getResp)
		_ = gr.Body.Close()
		if getResp.Data.EnvStatus == "ready" {
			envReady = true
			break
		}
		if getResp.Data.EnvStatus == "failed" {
			t.Skipf("env_sync failed on this host: %s", getResp.Data.EnvError)
		}
		break
	}
	_ = envReady

	var runResp struct {
		Data struct {
			Result any `json:"result"`
		} `json:"data"`
	}
	rr := h.PostJSON("/api/v1/handlers/"+hdID+":call",
		map[string]any{"method": "echo", "args": map[string]any{"name": "world"}}, &runResp)
	_ = rr.Body.Close()
	if rr.StatusCode != 200 {
		var errBody th.ErrEnvelope
		_ = th.DoRequest(t, h, "POST", "/api/v1/handlers/"+hdID+":call",
			map[string]any{"method": "echo", "args": map[string]any{"name": "world"}}, &errBody)
		if errBody.Error.Code == "HANDLER_ENV_FAILED" || errBody.Error.Code == "HANDLER_INSTANCE_SPAWN_FAILED" {
			t.Skipf("env not buildable on this host: %s", errBody.Error.Message)
		}
		t.Fatalf("call status=%d body=%+v", rr.StatusCode, errBody)
	}
	if got, _ := runResp.Data.Result.(string); got != "hi, world!" {
		t.Errorf("result = %v, want hi, world!", runResp.Data.Result)
	}

	var callsResp struct {
		Data struct {
			Count      int              `json:"count"`
			Calls      []map[string]any `json:"calls"`
			Aggregates map[string]any   `json:"aggregates"`
		} `json:"data"`
	}
	cr := h.GetJSON("/api/v1/handlers/"+hdID+"/calls", &callsResp)
	_ = cr.Body.Close()
	if callsResp.Data.Count != 1 {
		t.Errorf("calls count = %d, want 1", callsResp.Data.Count)
	}
	if ok, _ := callsResp.Data.Aggregates["okCount"].(float64); ok != 1 {
		t.Errorf("aggregates.okCount = %v, want 1", callsResp.Data.Aggregates["okCount"])
	}
}

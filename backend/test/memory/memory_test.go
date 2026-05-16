//go:build pipeline

// Package memory_test runs pipeline tests for cross-conversation memory.
//
// Package memory_test 跑跨对话 memory pipeline 测试。
package memory_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	memorydomain "github.com/sunweilin/forgify/backend/internal/domain/memory"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

func TestMemory_UserPinnedReachesLLM(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptText("Acknowledged."))
	fake.PushDefault(th.ScriptText("Title"))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")

	pinned := true
	createReq := map[string]any{
		"name":        "user_prefers_go",
		"type":        "user",
		"description": "User is a senior Go engineer.",
		"content":     "Has 10 years of Go experience. Prefers clean architecture.",
		"pinned":      &pinned,
	}
	var createResp struct {
		Data memorydomain.Memory `json:"data"`
	}
	if code := th.DoRequest(t, h, http.MethodPost, "/api/v1/memories", createReq, &createResp); code != http.StatusCreated {
		t.Fatalf("POST /api/v1/memories: status=%d", code)
	}
	if !createResp.Data.Pinned {
		t.Fatalf("created memory not pinned: %+v", createResp.Data)
	}

	conv := h.NewConversation(t, "memory-user-pinned")
	sub := h.SubscribeSSE(t, conv.ID)
	th.PostMessage(t, h, conv.ID, "Hi.")
	final := sub.WaitForAssistantTerminal(60 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errMsg=%q\nraw:\n%s",
			final.Status, final.ErrorMessage, sub.FormatRawEvents())
	}

	prompt := fake.LastSystemPrompt()
	for _, want := range []string{
		"──── Pinned memories ────",
		"user_prefers_go",
		"Has 10 years of Go experience",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("system prompt missing %q\nfull prompt:\n%s", want, prompt)
		}
	}
}

func TestMemory_UnpinnedOnlyIndex(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptText("OK."))
	fake.PushDefault(th.ScriptText("Title"))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")

	const secretBody = "Detailed-internal-content-that-must-not-leak"
	createReq := map[string]any{
		"name":        "deploy_pipeline",
		"type":        "reference",
		"description": "Deploy via Jenkins job 'prod-cut'.",
		"content":     secretBody,
	}
	if code := th.DoRequest(t, h, http.MethodPost, "/api/v1/memories", createReq, nil); code != http.StatusCreated {
		t.Fatalf("POST /api/v1/memories: status=%d", code)
	}

	conv := h.NewConversation(t, "memory-unpinned-index")
	sub := h.SubscribeSSE(t, conv.ID)
	th.PostMessage(t, h, conv.ID, "Hi.")
	final := sub.WaitForAssistantTerminal(60 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errMsg=%q\nraw:\n%s",
			final.Status, final.ErrorMessage, sub.FormatRawEvents())
	}

	prompt := fake.LastSystemPrompt()
	for _, want := range []string{
		"──── Memory index ────",
		"[reference] deploy_pipeline",
		"Deploy via Jenkins job 'prod-cut'.",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("system prompt missing %q\nfull prompt:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, secretBody) {
		t.Errorf("system prompt leaked unpinned Content body %q\nfull prompt:\n%s", secretBody, prompt)
	}
}

func TestMemory_AIWritePersistsAndNotifies(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"write_memory", "call_fake_mem_write",
		`{"summary":"learned user role","name":"user_role","type":"user","description":"User is a data scientist on the observability team.","content":"User mentioned working on logging infrastructure."}`,
	))
	fake.PushScript(th.ScriptText("Saved that for next time."))
	fake.PushDefault(th.ScriptText("Title"))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "memory-ai-write")
	sub := h.SubscribeSSE(t, conv.ID)
	th.PostMessage(t, h, conv.ID, "I'm a data scientist on the observability team.")

	final := sub.WaitForAssistantTerminal(60 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errMsg=%q\nraw:\n%s",
			final.Status, final.ErrorMessage, sub.FormatRawEvents())
	}

	got, err := h.Memory.Get(h.LocalCtx(), "user_role")
	if err != nil {
		t.Fatalf("Memory.Get(user_role): %v", err)
	}
	if got.Source != memorydomain.SourceAI {
		t.Errorf("memory.source = %q, want %q", got.Source, memorydomain.SourceAI)
	}
	if got.Type != memorydomain.TypeUser {
		t.Errorf("memory.type = %q, want %q", got.Type, memorydomain.TypeUser)
	}
	if !strings.Contains(got.Content, "logging infrastructure") {
		t.Errorf("memory.content unexpected: %q", got.Content)
	}

	if !sawMemoryNotification(sub.RawEvents(), "user_role", "created") {
		t.Errorf("did not see memory 'created' notification for user_role\nraw:\n%s",
			sub.FormatRawEvents())
	}

	res, ok := th.ExtractToolResultByCallID(final.Blocks, "call_fake_mem_write")
	if !ok {
		t.Fatalf("no write_memory tool_result\nraw:\n%s", sub.FormatRawEvents())
	}
	if v, _ := res["ok"].(bool); !v {
		t.Errorf("write_memory tool_result.ok=false: %v", res)
	}
}

func TestMemory_PinTogglesContent(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptText("First reply."))
	fake.PushScript(th.ScriptText("Second reply."))
	fake.PushDefault(th.ScriptText("Title"))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")

	const fullBody = "Always start work by running make test-unit, then dogfood."
	createReq := map[string]any{
		"name":        "workflow_rule",
		"type":        "feedback",
		"description": "Test-unit then dogfood before declaring done.",
		"content":     fullBody,
	}
	if code := th.DoRequest(t, h, http.MethodPost, "/api/v1/memories", createReq, nil); code != http.StatusCreated {
		t.Fatalf("POST /api/v1/memories: status=%d", code)
	}

	conv1 := h.NewConversation(t, "memory-pin-r1")
	sub1 := h.SubscribeSSE(t, conv1.ID)
	th.PostMessage(t, h, conv1.ID, "Hi.")
	final1 := sub1.WaitForAssistantTerminal(60 * time.Second)
	if final1.Status != chatdomain.StatusCompleted {
		t.Fatalf("round1 status=%q errMsg=%q", final1.Status, final1.ErrorMessage)
	}
	if strings.Contains(fake.LastSystemPrompt(), fullBody) {
		t.Errorf("round1 system prompt unexpectedly contained pinned body before pinning:\n%s",
			fake.LastSystemPrompt())
	}

	var pinResp struct {
		Data memorydomain.Memory `json:"data"`
	}
	if code := th.DoRequest(t, h, http.MethodPost, "/api/v1/memories/workflow_rule:pin", nil, &pinResp); code != http.StatusOK {
		t.Fatalf("POST :pin: status=%d", code)
	}
	if !pinResp.Data.Pinned {
		t.Fatalf("pin response Pinned=false: %+v", pinResp.Data)
	}

	conv2 := h.NewConversation(t, "memory-pin-r2")
	sub2 := h.SubscribeSSE(t, conv2.ID)
	th.PostMessage(t, h, conv2.ID, "Hi.")
	final2 := sub2.WaitForAssistantTerminal(60 * time.Second)
	if final2.Status != chatdomain.StatusCompleted {
		t.Fatalf("round2 status=%q errMsg=%q", final2.Status, final2.ErrorMessage)
	}

	prompt2 := fake.LastSystemPrompt()
	for _, want := range []string{
		"──── Pinned memories ────",
		"workflow_rule",
		fullBody,
	} {
		if !strings.Contains(prompt2, want) {
			t.Errorf("round2 system prompt missing %q\nfull prompt:\n%s", want, prompt2)
		}
	}
}

// sawMemoryNotification scans raw SSE for a memory notification matching name + action.
//
// sawMemoryNotification 扫 raw SSE 找匹配 name + action 的 memory 通知。
func sawMemoryNotification(events []th.RawEvent, wantName, wantAction string) bool {
	for _, e := range events {
		if e.Source != "notifications" {
			continue
		}
		var env struct {
			Type string `json:"type"`
			Data struct {
				Name   string `json:"name"`
				Action string `json:"action"`
			} `json:"data"`
		}
		if err := json.Unmarshal(e.Data, &env); err != nil {
			continue
		}
		if env.Type == "memory" && env.Data.Name == wantName && env.Data.Action == wantAction {
			return true
		}
	}
	return false
}

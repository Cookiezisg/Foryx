//go:build pipeline

// uxtodo_test.go — pipeline tests for the U batch (todo family + Ask
// family). Drives the full chat ReAct loop with a fake LLM.
//
// Scenarios:
//  1. TodoCreateAndList — round 1 TodoCreate, round 2 TodoList; verify
//     both tool_results carry the expected JSON shapes (a fresh Todo
//     entity from Create, total/todos list from List).
//  2. AskUserQuestionAnswerDelivered — fake LLM scripts AskUserQuestion,
//     a side goroutine POSTs the answer to
//     /api/v1/conversations/{id}/answers, verify the tool returns the
//     user's answer string in tool_result.
//
// Timeout / cancellation paths for AskUserQuestion are exercised by
// in-package unit tests; running the 5-minute default-timeout path
// here would be impractical.
//
// uxtodo_test.go — U batch（todo 家族 + Ask 家族）pipeline 测试。
package uxtodo_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

// ── 1. Todo create + list round-trip ─────────────────────────────────────────

func TestUxTodo_TodoCreateAndList(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"TodoCreate", "call_fake_td_create",
		`{"summary":"plan first todo","subject":"Run unit tests","active_form":"Running unit tests"}`,
	))
	fake.PushScript(th.ScriptSingleToolCall(
		"TodoList", "call_fake_td_list",
		`{"summary":"verify"}`,
	))
	fake.PushScript(th.ScriptText("Created and listed."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "uxtodo-todo")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Make a todo list and dump it.")

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errCode=%q errMsg=%q\nraw:\n%s",
			final.Status, final.ErrorCode, final.ErrorMessage, sub.FormatRawEvents())
	}

	createRes, ok := th.ExtractToolResultByCallID(final.Blocks, "call_fake_td_create")
	if !ok {
		t.Fatalf("no TodoCreate tool_result\nraw:\n%s", sub.FormatRawEvents())
	}
	if v, _ := createRes["ok"].(bool); !v {
		t.Errorf("TodoCreate tool_result.ok = false: %v", createRes)
	}
	createText, _ := createRes["result"].(string)
	for _, want := range []string{`"id":`, `"td_`, `"Run unit tests"`, `"pending"`} {
		if !strings.Contains(createText, want) {
			t.Errorf("TodoCreate result missing %q in:\n%s", want, createText)
		}
	}

	listRes, ok := th.ExtractToolResultByCallID(final.Blocks, "call_fake_td_list")
	if !ok {
		t.Fatalf("no TodoList tool_result")
	}
	listText, _ := listRes["result"].(string)
	for _, want := range []string{`"total"`, `"Run unit tests"`, `"pending"`} {
		if !strings.Contains(listText, want) {
			t.Errorf("TodoList result missing %q in:\n%s", want, listText)
		}
	}
	// total should be exactly 1 — accept any whitespace between the colon
	// and the digit because json.MarshalIndent's spacing isn't load-bearing.
	// total 应正好 1——容忍冒号后空白（MarshalIndent 排版非关键）。
	if !strings.Contains(listText, `"total": 1`) && !strings.Contains(listText, `"total":1`) {
		t.Errorf("TodoList total != 1 in:\n%s", listText)
	}
}

// ── 2. AskUserQuestion answer-delivery round-trip ────────────────────────────

func TestUxTodo_AskUserQuestionAnswerDelivered(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"AskUserQuestion", "call_fake_ask_001",
		`{"summary":"asking confirmation","question":"Proceed?","options":["yes","no"]}`,
	))
	fake.PushScript(th.ScriptText("Got the user's answer."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "uxtodo-ask")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Ask me before doing it.")

	// Side goroutine: wait briefly for the tool to register its Wait,
	// then POST the answer through the public HTTP endpoint.
	// 旁路 goroutine：稍等 tool 完成 Wait 注册，再走公共 HTTP 端点 POST 答案。
	deliveryDone := make(chan int, 1)
	go func() {
		time.Sleep(200 * time.Millisecond)
		body := map[string]string{
			"toolCallId": "call_fake_ask_001",
			"answer":     "yes go ahead",
		}
		resp := h.PostJSON("/api/v1/conversations/"+conv.ID+"/answers", body, nil)
		deliveryDone <- resp.StatusCode
		_ = resp.Body.Close()
	}()

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errCode=%q errMsg=%q\nraw:\n%s",
			final.Status, final.ErrorCode, final.ErrorMessage, sub.FormatRawEvents())
	}

	select {
	case code := <-deliveryDone:
		if code != http.StatusNoContent {
			t.Errorf("answer POST status = %d, want 204", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("answer-delivery POST never completed")
	}

	askRes, ok := th.ExtractToolResultByCallID(final.Blocks, "call_fake_ask_001")
	if !ok {
		t.Fatalf("no AskUserQuestion tool_result\nraw:\n%s", sub.FormatRawEvents())
	}
	if v, _ := askRes["ok"].(bool); !v {
		t.Errorf("AskUserQuestion tool_result.ok = false: %v", askRes)
	}
	resultText, _ := askRes["result"].(string)
	if resultText != "yes go ahead" {
		t.Errorf("answer = %q, want %q", resultText, "yes go ahead")
	}
}

// ── 3. Answer endpoint rejects unknown toolCallId with 404 ───────────────────

func TestUxTodo_AnswerEndpoint_UnknownCallID_404(t *testing.T) {
	h := th.New(t)
	conv := h.NewConversation(t, "uxtodo-ask-404")

	// DoRequest tolerates non-2xx responses; PostJSON would t.Fatalf.
	// DoRequest 容忍非 2xx；PostJSON 会 t.Fatalf。
	body := map[string]string{
		"toolCallId": "call_does_not_exist",
		"answer":     "x",
	}
	status := th.DoRequest(t, h, http.MethodPost,
		"/api/v1/conversations/"+conv.ID+"/answers", body, nil)
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}

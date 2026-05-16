//go:build pipeline

// Package uxtodo_test runs pipeline tests for the U batch (todo + Ask families).
//
// Package uxtodo_test 跑 U batch（todo + Ask）pipeline 测试。
package uxtodo_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

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
	if !strings.Contains(listText, `"total": 1`) && !strings.Contains(listText, `"total":1`) {
		t.Errorf("TodoList total != 1 in:\n%s", listText)
	}
}

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

func TestUxTodo_AnswerEndpoint_UnknownCallID_404(t *testing.T) {
	h := th.New(t)
	conv := h.NewConversation(t, "uxtodo-ask-404")

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

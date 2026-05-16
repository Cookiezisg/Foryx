//go:build pipeline

// Package web_test runs pipeline tests for web system tools (WebFetch/WebSearch).
//
// Package web_test 跑 web 系统工具（WebFetch/WebSearch）pipeline 测试。
package web_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

func TestWeb_WebFetchBlocksLoopback(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"WebFetch", "call_fake_fetch_001",
		`{"summary":"snooping localhost","url":"http://127.0.0.1/admin","prompt":"What's on this page?"}`,
	))
	fake.PushScript(th.ScriptText("I cannot fetch loopback addresses."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "web-ssrf")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Try fetching http://127.0.0.1/admin.")

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errCode=%q errMsg=%q\nraw:\n%s",
			final.Status, final.ErrorCode, final.ErrorMessage, sub.FormatRawEvents())
	}

	fetchID, ok := th.ExtractToolCallByName(final.Blocks, "WebFetch")
	if !ok {
		t.Fatalf("no WebFetch tool_call in final blocks\nraw:\n%s", sub.FormatRawEvents())
	}
	res, ok := th.ExtractToolResultByCallID(final.Blocks, fetchID)
	if !ok {
		t.Fatalf("no WebFetch tool_result for call %q", fetchID)
	}
	if v, _ := res["ok"].(bool); !v {
		t.Errorf("WebFetch tool_result.ok = false; expected true. data: %v", res)
	}
	resultText, _ := res["result"].(string)
	if !strings.Contains(resultText, "loopback") {
		t.Errorf("expected loopback rejection in tool_result, got: %q", resultText)
	}
}

func TestWeb_WebSearchRejectsEmptyQuery(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"WebSearch", "call_fake_search_001",
		`{"summary":"empty search","query":""}`,
	))
	fake.PushScript(th.ScriptText("I cannot search with an empty query."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "web-validate")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Search the web.")

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q\nraw:\n%s", final.Status, sub.FormatRawEvents())
	}

	searchID, ok := th.ExtractToolCallByName(final.Blocks, "WebSearch")
	if !ok {
		t.Fatalf("no WebSearch tool_call in final blocks")
	}
	res, ok := th.ExtractToolResultByCallID(final.Blocks, searchID)
	if !ok {
		t.Fatal("no WebSearch tool_result")
	}
	if v, _ := res["ok"].(bool); v {
		t.Errorf("WebSearch tool_result.ok = true; expected false on validation failure. data: %v", res)
	}
	resultText := fmt.Sprintf("%v", res["result"])
	if !strings.Contains(resultText, "query is required") {
		t.Errorf("expected ErrEmptyQuery message in result, got: %q", resultText)
	}
}

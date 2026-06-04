package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func deepseekBody(t *testing.T, req Request) dsRequest {
	t.Helper()
	httpReq, err := newDeepSeekProvider().BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(httpReq.Body)
	var dr dsRequest
	if err := json.Unmarshal(raw, &dr); err != nil {
		t.Fatal(err)
	}
	return dr
}

func TestDeepSeekBuildRequest(t *testing.T) {
	httpReq, err := newDeepSeekProvider().BuildRequest(context.Background(), Request{
		ModelID:  "deepseek-chat",
		Key:      "sk-ds",
		BaseURL:  "https://api.deepseek.com",
		Messages: []LLMMessage{{Role: RoleUser, Content: "hi"}},
		Options:  map[string]string{"thinking": "enabled", "reasoning_effort": "high"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := httpReq.URL.String(); got != "https://api.deepseek.com/chat/completions" {
		t.Errorf("url = %s", got)
	}
	if httpReq.Header.Get("Authorization") != "Bearer sk-ds" {
		t.Errorf("auth = %q", httpReq.Header.Get("Authorization"))
	}
	raw, _ := io.ReadAll(httpReq.Body)
	var dr dsRequest
	_ = json.Unmarshal(raw, &dr)
	if dr.Thinking == nil || dr.Thinking.Type != "enabled" {
		t.Errorf("thinking = %+v, want enabled", dr.Thinking)
	}
	// Native value passes through verbatim — no normalization map.
	// 原生值原样透传——无归一化映射。
	if dr.ReasoningEffort != "high" {
		t.Errorf("reasoning_effort = %q, want high", dr.ReasoningEffort)
	}
}

func TestDeepSeekReasoningRoundTrip(t *testing.T) {
	// Plain assistant turn (no tool_calls) → reasoning_content stripped.
	dr := deepseekBody(t, Request{
		ModelID: "m",
		Messages: []LLMMessage{
			{Role: RoleUser, Content: "q"},
			{Role: RoleAssistant, Content: "a", ReasoningContent: "secret thoughts"},
			{Role: RoleUser, Content: "q2"},
		},
	})
	for _, m := range dr.Messages {
		if m.Role == "assistant" && m.ReasoningContent != "" {
			t.Errorf("plain assistant turn should strip reasoning_content, got %q", m.ReasoningContent)
		}
	}

	// Tool-call assistant turn → reasoning_content preserved.
	dr = deepseekBody(t, Request{
		ModelID: "m",
		Messages: []LLMMessage{
			{Role: RoleAssistant, ReasoningContent: "kept", ToolCalls: []LLMToolCall{{ID: "t1", Name: "f", Arguments: "{}"}}},
			{Role: RoleTool, ToolCallID: "t1", Content: "result"},
		},
	})
	var found bool
	for _, m := range dr.Messages {
		if m.Role == "assistant" && m.ReasoningContent == "kept" {
			found = true
		}
	}
	if !found {
		t.Error("tool-call assistant turn should preserve reasoning_content")
	}
}

func TestDeepSeekParseStream(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning_content":"think"}}]}`,
		`data: {"choices":[{"delta":{"content":"ans"}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"t1","function":{"name":"f","arguments":"{}"}}]}}]}`,
		`data: {"choices":[{"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3}}`,
		`data: [DONE]`,
	}, "\n\n") + "\n\n"

	resp := &http.Response{Body: io.NopCloser(strings.NewReader(sse))}
	events := collect(newDeepSeekProvider().ParseStream(context.Background(), resp, Request{}))

	var reasoning, text string
	var order []StreamEventType
	var sawTool, sawFinish bool
	for _, ev := range events {
		switch ev.Type {
		case EventReasoning:
			reasoning += ev.Delta
			order = append(order, ev.Type)
		case EventText:
			text += ev.Delta
			order = append(order, ev.Type)
		case EventToolStart:
			sawTool = true
		case EventFinish:
			sawFinish = true
			if ev.InputTokens != 2 || ev.OutputTokens != 3 {
				t.Errorf("finish tokens = %d/%d", ev.InputTokens, ev.OutputTokens)
			}
		case EventError:
			t.Fatalf("unexpected error: %v", ev.Err)
		}
	}
	if reasoning != "think" || text != "ans" {
		t.Errorf("reasoning=%q text=%q", reasoning, text)
	}
	// reasoning must come before text
	if len(order) < 2 || order[0] != EventReasoning || order[1] != EventText {
		t.Errorf("event order = %v, want reasoning before text", order)
	}
	if !sawTool || !sawFinish {
		t.Errorf("sawTool=%v sawFinish=%v", sawTool, sawFinish)
	}
}

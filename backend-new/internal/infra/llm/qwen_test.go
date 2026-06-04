package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestQwenBuildRequest(t *testing.T) {
	p := newQwenProvider()
	req := Request{
		ModelID:  "qwen3-max",
		Key:      "sk-qwen",
		BaseURL:  "https://dashscope.aliyuncs.com/compatible-mode/v1",
		System:   "you are helpful",
		Messages: []LLMMessage{{Role: RoleUser, Content: "hi"}},
		Tools:    []ToolDef{{Name: "get_weather", Description: "d", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}
	httpReq, err := p.BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if httpReq.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", httpReq.Method)
	}
	if got := httpReq.URL.String(); got != "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions" {
		t.Errorf("url = %s", got)
	}
	if got := httpReq.Header.Get("Authorization"); got != "Bearer sk-qwen" {
		t.Errorf("auth = %q", got)
	}
	body, _ := io.ReadAll(httpReq.Body)
	var qr qwenRequest
	if err := json.Unmarshal(body, &qr); err != nil {
		t.Fatal(err)
	}
	if qr.Model != "qwen3-max" || !qr.Stream {
		t.Errorf("model=%s stream=%v", qr.Model, qr.Stream)
	}
	if len(qr.Tools) != 1 || qr.Tools[0].Function.Name != "get_weather" {
		t.Errorf("tools = %+v", qr.Tools)
	}
	if len(qr.Messages) != 2 || qr.Messages[0].Role != "system" || qr.Messages[1].Role != "user" {
		t.Errorf("messages = %+v", qr.Messages)
	}
}

// TestQwenBuildRequestThinkingKnobs drives Qwen's native knobs from Options:
// enable_thinking ("true"/"false" → *bool) and thinking_budget (digit string → int).
// Values pass through verbatim with no normalization.
//
// TestQwenBuildRequestThinkingKnobs 用 Options 驱动 Qwen 原生旋钮：enable_thinking
// （"true"/"false" → *bool）与 thinking_budget（数字串 → int）。原生值直接透传、无归一化。
func TestQwenBuildRequestThinkingKnobs(t *testing.T) {
	p := newQwenProvider()
	thinkingOf := func(opts map[string]string) qwenRequest {
		req := Request{ModelID: "qwen3-max", Key: "k", BaseURL: "https://x", Options: opts}
		httpReq, err := p.BuildRequest(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(httpReq.Body)
		var qr qwenRequest
		_ = json.Unmarshal(body, &qr)
		return qr
	}

	// absent → enable_thinking omitted entirely.
	// 不设 → 完全省略 enable_thinking。
	if qr := thinkingOf(nil); qr.EnableThinking != nil {
		t.Errorf("absent → enable_thinking = %v, want nil", *qr.EnableThinking)
	}

	// "false" → enable_thinking=false (explicit, reaches the wire via *bool).
	// "false" → enable_thinking=false（显式，经 *bool 上线）。
	qr := thinkingOf(map[string]string{"enable_thinking": "false"})
	if qr.EnableThinking == nil || *qr.EnableThinking {
		t.Errorf("false → enable_thinking = %v, want false", qr.EnableThinking)
	}
	if qr.ThinkingBudget != 0 {
		t.Errorf("false → thinking_budget = %d, want 0", qr.ThinkingBudget)
	}

	// "true" + budget → enable_thinking=true + thinking_budget passed through.
	// "true" + budget → enable_thinking=true + thinking_budget 透传。
	qr = thinkingOf(map[string]string{"enable_thinking": "true", "thinking_budget": "4096"})
	if qr.EnableThinking == nil || !*qr.EnableThinking {
		t.Errorf("true → enable_thinking = %v, want true", qr.EnableThinking)
	}
	if qr.ThinkingBudget != 4096 {
		t.Errorf("true → thinking_budget = %d, want 4096", qr.ThinkingBudget)
	}

	// "true" without budget → enable_thinking=true, budget omitted.
	// "true" 无 budget → enable_thinking=true，省略 budget。
	qr = thinkingOf(map[string]string{"enable_thinking": "true"})
	if qr.EnableThinking == nil || !*qr.EnableThinking {
		t.Errorf("true/no-budget → enable_thinking = %v, want true", qr.EnableThinking)
	}
	if qr.ThinkingBudget != 0 {
		t.Errorf("true/no-budget → thinking_budget = %d, want 0", qr.ThinkingBudget)
	}
}

// TestQwenBuildRequestStreamFlag verifies DisableStream flips stream=false (and drops
// stream_options); the native thinking knob is independent of streaming.
//
// TestQwenBuildRequestStreamFlag 验 DisableStream 使 stream=false（并去掉 stream_options）；
// 原生 thinking 旋钮与流式无关。
func TestQwenBuildRequestStreamFlag(t *testing.T) {
	p := newQwenProvider()
	req := Request{
		ModelID:       "qwen3-max",
		Key:           "k",
		BaseURL:       "https://x",
		DisableStream: true,
	}
	httpReq, err := p.BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(httpReq.Body)
	var qr qwenRequest
	_ = json.Unmarshal(body, &qr)
	if qr.Stream {
		t.Errorf("stream = true, want false on DisableStream")
	}
	if qr.StreamOptions != nil {
		t.Errorf("stream_options = %+v, want omitted on DisableStream", qr.StreamOptions)
	}
}

func TestQwenParseStream(t *testing.T) {
	p := newQwenProvider()
	resp := &http.Response{Body: sseBody(
		`data: {"choices":[{"delta":{"reasoning_content":"think"}}]}`,
		`data: {"choices":[{"delta":{"content":"Hel"}}]}`,
		`data: {"choices":[{"delta":{"content":"lo"}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"f","arguments":"{}"}}]}}]}`,
		`data: {"choices":[{"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`,
		`data: [DONE]`,
	)}
	events := collect(p.ParseStream(context.Background(), resp, Request{}))

	var text, reasoning strings.Builder
	var sawToolStart, sawFinish bool
	for _, ev := range events {
		switch ev.Type {
		case EventText:
			text.WriteString(ev.Delta)
		case EventReasoning:
			reasoning.WriteString(ev.Delta)
		case EventToolStart:
			sawToolStart = true
			if ev.ToolName != "f" || ev.ToolID != "call_1" {
				t.Errorf("tool_start = %+v", ev)
			}
		case EventFinish:
			sawFinish = true
			if ev.FinishReason != "stop" || ev.InputTokens != 3 || ev.OutputTokens != 2 {
				t.Errorf("finish = %+v", ev)
			}
		case EventError:
			t.Fatalf("unexpected error event: %v", ev.Err)
		}
	}
	if text.String() != "Hello" {
		t.Errorf("text = %q, want Hello", text.String())
	}
	if reasoning.String() != "think" {
		t.Errorf("reasoning = %q, want think", reasoning.String())
	}
	if !sawToolStart || !sawFinish {
		t.Errorf("missing events: toolStart=%v finish=%v", sawToolStart, sawFinish)
	}
}

// TestQwenParseStreamFlatError verifies the DashScope flat error envelope
// {code,message,request_id} arriving as a 200 chunk (no nested "error") surfaces as a
// provider EventError rather than being silently dropped.
//
// TestQwenParseStreamFlatError 验 DashScope 扁平错误信封 {code,message,request_id} 以
// 200 chunk 返回（无嵌套 "error"）时 emit provider EventError，而非静默丢弃。
func TestQwenParseStreamFlatError(t *testing.T) {
	p := newQwenProvider()
	resp := &http.Response{Body: sseBody(
		`data: {"code":"InvalidParameter","message":"enable_thinking must be set to false for non-streaming calls","request_id":"req-1"}`,
	)}
	events := collect(p.ParseStream(context.Background(), resp, Request{}))
	if len(events) != 1 {
		t.Fatalf("events = %+v, want exactly 1 error", events)
	}
	ev := events[0]
	if ev.Type != EventError {
		t.Fatalf("type = %s, want error", ev.Type)
	}
	if ev.Err == nil || !strings.Contains(ev.Err.Error(), "InvalidParameter") {
		t.Errorf("err = %v, want flat code surfaced", ev.Err)
	}
}

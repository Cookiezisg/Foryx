package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCustomBuildRequest(t *testing.T) {
	p := newCustomProvider()
	req := Request{
		ModelID:  "my-model",
		Key:      "sk-custom",
		BaseURL:  "https://endpoint.example.com/v1",
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
	if got := httpReq.URL.String(); got != "https://endpoint.example.com/v1/chat/completions" {
		t.Errorf("url = %s", got)
	}
	if got := httpReq.Header.Get("Authorization"); got != "Bearer sk-custom" {
		t.Errorf("auth = %q", got)
	}
	body, _ := io.ReadAll(httpReq.Body)
	var cr customRequest
	if err := json.Unmarshal(body, &cr); err != nil {
		t.Fatal(err)
	}
	if cr.Model != "my-model" || !cr.Stream {
		t.Errorf("model=%s stream=%v", cr.Model, cr.Stream)
	}
	if len(cr.Tools) != 1 || cr.Tools[0].Function.Name != "get_weather" {
		t.Errorf("tools = %+v", cr.Tools)
	}
	if len(cr.Messages) != 2 || cr.Messages[0].Role != "system" || cr.Messages[1].Role != "user" {
		t.Errorf("messages = %+v", cr.Messages)
	}
}

// TestCustomNoThinking asserts the defining quirk: a custom endpoint is generic, so it
// ignores req.Options entirely and NO thinking/reasoning knob is ever emitted — even when
// Options carries native knobs from other providers (a stray field could 400 an endpoint
// that lacks it).
//
// TestCustomNoThinking 验本家特点：通用端点彻底忽略 req.Options，永不发 thinking/reasoning 旋钮——
// 即便 Options 携带其他家原生旋钮也不发（多余字段可能让不支持的端点 400）。
func TestCustomNoThinking(t *testing.T) {
	p := newCustomProvider()
	base := Request{ModelID: "m", Key: "k", BaseURL: "https://x"}

	// rawFieldsOf decodes the body into a generic map so we can assert no thinking
	// knob is present under any spelling (no reasoning_effort, no thinking, no reasoning).
	// rawFieldsOf 解到通用 map，断言任何拼写的 thinking 旋钮都不存在。
	rawFieldsOf := func(req Request) map[string]json.RawMessage {
		httpReq, err := p.BuildRequest(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(httpReq.Body)
		var m map[string]json.RawMessage
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}

	for _, opts := range []map[string]string{
		nil,                          // no knobs
		{"reasoning_effort": "high"}, // OpenRouter-style knob
		{"thinking": "enabled", "effort": "high"},           // Anthropic-style knobs
		{"thinkingBudget": "-1", "enable_thinking": "true"}, // Gemini/Qwen-style knobs
	} {
		req := base
		req.Options = opts
		fields := rawFieldsOf(req)
		for _, key := range []string{"reasoning_effort", "thinking", "reasoning", "effort", "thinkingBudget", "enable_thinking"} {
			if _, ok := fields[key]; ok {
				t.Errorf("options=%+v emitted %q field, want none", opts, key)
			}
		}
	}
}

func TestCustomParseStream(t *testing.T) {
	p := newCustomProvider()
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

func TestCustomParseNonStreaming(t *testing.T) {
	p := newCustomProvider()
	body := `{"choices":[{"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1}}`
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
	events := collect(p.ParseStream(context.Background(), resp, Request{DisableStream: true}))
	if len(events) != 2 || events[0].Type != EventText || events[0].Delta != "done" || events[1].Type != EventFinish {
		t.Errorf("non-streaming events = %+v", events)
	}
}

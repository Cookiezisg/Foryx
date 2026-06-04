package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMoonshotBuildRequest(t *testing.T) {
	p := newMoonshotProvider()
	req := Request{
		ModelID:  "kimi-k2-0905-preview",
		Key:      "sk-moon",
		BaseURL:  "https://api.moonshot.cn/v1",
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
	if got := httpReq.URL.String(); got != "https://api.moonshot.cn/v1/chat/completions" {
		t.Errorf("url = %s", got)
	}
	if got := httpReq.Header.Get("Authorization"); got != "Bearer sk-moon" {
		t.Errorf("auth = %q", got)
	}
	if got := httpReq.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("content-type = %q", got)
	}
	body, _ := io.ReadAll(httpReq.Body)
	var ms moonshotRequest
	if err := json.Unmarshal(body, &ms); err != nil {
		t.Fatal(err)
	}
	if ms.Model != "kimi-k2-0905-preview" || !ms.Stream {
		t.Errorf("model=%s stream=%v", ms.Model, ms.Stream)
	}
	if ms.StreamOptions == nil || !ms.StreamOptions.IncludeUsage {
		t.Errorf("stream_options = %+v, want include_usage", ms.StreamOptions)
	}
	if ms.Thinking != nil {
		t.Errorf("thinking = %+v, want omitted when Options carries no thinking", ms.Thinking)
	}
	if len(ms.Tools) != 1 || ms.Tools[0].Function.Name != "get_weather" {
		t.Errorf("tools = %+v", ms.Tools)
	}
	if len(ms.Messages) != 2 || ms.Messages[0].Role != "system" || ms.Messages[1].Role != "user" {
		t.Errorf("messages = %+v", ms.Messages)
	}
}

func TestMoonshotBuildRequestThinkingModes(t *testing.T) {
	p := newMoonshotProvider()
	base := Request{ModelID: "kimi-k2.5", Key: "k", BaseURL: "https://x"}
	// thinkingOf returns the encoded thinking.type, or "<nil>" when the field is omitted.
	// thinkingOf 返回编码后的 thinking.type；字段省略时返回 "<nil>"。
	thinkingOf := func(req Request) string {
		httpReq, err := p.BuildRequest(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(httpReq.Body)
		var ms moonshotRequest
		_ = json.Unmarshal(body, &ms)
		if ms.Thinking == nil {
			return "<nil>"
		}
		return ms.Thinking.Type
	}
	// No thinking in Options → field omitted.
	// Options 无 thinking → 字段省略。
	if got := thinkingOf(base); got != "<nil>" {
		t.Errorf("no options → %q, want omitted", got)
	}
	// Native thinking value passes through verbatim into thinking:{type}.
	// 原生 thinking 值原样进 thinking:{type}。
	base.Options = map[string]string{"thinking": "enabled"}
	if got := thinkingOf(base); got != "enabled" {
		t.Errorf("enabled → %q, want enabled", got)
	}
	base.Options = map[string]string{"thinking": "disabled"}
	if got := thinkingOf(base); got != "disabled" {
		t.Errorf("disabled → %q, want disabled", got)
	}
}

func TestMoonshotParseStream(t *testing.T) {
	p := newMoonshotProvider()
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
	if reasoning.String() != "think" {
		t.Errorf("reasoning = %q, want think", reasoning.String())
	}
	if text.String() != "Hello" {
		t.Errorf("text = %q, want Hello", text.String())
	}
	if !sawToolStart || !sawFinish {
		t.Errorf("missing events: toolStart=%v finish=%v", sawToolStart, sawFinish)
	}
}

func TestMoonshotParseStreamInStreamError(t *testing.T) {
	p := newMoonshotProvider()
	resp := &http.Response{Body: sseBody(
		`data: {"error":{"message":"boom"}}`,
	)}
	events := collect(p.ParseStream(context.Background(), resp, Request{}))
	if len(events) != 1 || events[0].Type != EventError {
		t.Fatalf("events = %+v, want single error", events)
	}
	if !strings.Contains(events[0].Err.Error(), "boom") {
		t.Errorf("err = %v, want to contain boom", events[0].Err)
	}
}

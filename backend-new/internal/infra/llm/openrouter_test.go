package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestOpenRouterBuildRequest(t *testing.T) {
	p := newOpenRouterProvider()
	req := Request{
		ModelID:  "anthropic/claude-3.5-sonnet",
		Key:      "sk-or-test",
		BaseURL:  "https://openrouter.ai/api/v1",
		System:   "you are helpful",
		Messages: []LLMMessage{{Role: RoleUser, Content: "hi"}},
		Tools:    []ToolDef{{Name: "get_weather", Description: "d", Parameters: json.RawMessage(`{"type":"object"}`)}},
		Options:  map[string]string{"reasoning_effort": "high"},
	}
	httpReq, err := p.BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if httpReq.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", httpReq.Method)
	}
	if got := httpReq.URL.String(); got != "https://openrouter.ai/api/v1/chat/completions" {
		t.Errorf("url = %s", got)
	}
	if got := httpReq.Header.Get("Authorization"); got != "Bearer sk-or-test" {
		t.Errorf("auth = %q", got)
	}
	body, _ := io.ReadAll(httpReq.Body)
	var or orRequest
	if err := json.Unmarshal(body, &or); err != nil {
		t.Fatal(err)
	}
	if or.Model != "anthropic/claude-3.5-sonnet" || !or.Stream {
		t.Errorf("model=%s stream=%v", or.Model, or.Stream)
	}
	if or.Reasoning == nil || or.Reasoning.Effort != "high" {
		t.Errorf("reasoning = %+v, want {effort:high}", or.Reasoning)
	}
	if len(or.Tools) != 1 || or.Tools[0].Function.Name != "get_weather" {
		t.Errorf("tools = %+v", or.Tools)
	}
	if len(or.Messages) != 2 || or.Messages[0].Role != "system" || or.Messages[1].Role != "user" {
		t.Errorf("messages = %+v", or.Messages)
	}
}

// TestOpenRouterBuildRequestReasoningEffort drives OpenRouter's sole native knob from
// Options: reasoning_effort → reasoning:{effort}, passed through verbatim. Absent → no
// reasoning field at all (the upstream decides).
//
// TestOpenRouterBuildRequestReasoningEffort 用 Options 驱动 OpenRouter 唯一原生旋钮：
// reasoning_effort → reasoning:{effort}，原样透传。不设 → 完全不发 reasoning 字段（由上游定）。
func TestOpenRouterBuildRequestReasoningEffort(t *testing.T) {
	p := newOpenRouterProvider()
	reasoningOf := func(opts map[string]string) *orReasoning {
		req := Request{ModelID: "m", Key: "k", BaseURL: "https://x", Options: opts}
		httpReq, err := p.BuildRequest(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(httpReq.Body)
		var or orRequest
		_ = json.Unmarshal(body, &or)
		return or.Reasoning
	}

	// absent → no reasoning field at all.
	// 不设 → 完全不发 reasoning 字段。
	if got := reasoningOf(nil); got != nil {
		t.Errorf("absent → %+v, want omitted", got)
	}

	// reasoning_effort passes through verbatim into reasoning:{effort}.
	// reasoning_effort 原样透传进 reasoning:{effort}。
	if got := reasoningOf(map[string]string{"reasoning_effort": "low"}); got == nil || got.Effort != "low" {
		t.Errorf("low → %+v, want {effort:low}", got)
	}
	if got := reasoningOf(map[string]string{"reasoning_effort": "xhigh"}); got == nil || got.Effort != "xhigh" {
		t.Errorf("xhigh → %+v, want {effort:xhigh}", got)
	}
}

func TestOpenRouterParseStream(t *testing.T) {
	p := newOpenRouterProvider()
	resp := &http.Response{Body: sseBody(
		`: OPENROUTER PROCESSING`,
		`data: {"choices":[{"delta":{"reasoning":"think"}}]}`,
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

// TestOpenRouterParseStreamReasoningContentAlias verifies the CN-family alias
// (reasoning_content) is read when the primary reasoning field is absent.
//
// 验证主 reasoning 字段缺失时，读取 CN 家族别名 reasoning_content。
func TestOpenRouterParseStreamReasoningContentAlias(t *testing.T) {
	p := newOpenRouterProvider()
	resp := &http.Response{Body: sseBody(
		`data: {"choices":[{"delta":{"reasoning_content":"deep"}}]}`,
		`data: [DONE]`,
	)}
	events := collect(p.ParseStream(context.Background(), resp, Request{}))
	var reasoning strings.Builder
	for _, ev := range events {
		if ev.Type == EventReasoning {
			reasoning.WriteString(ev.Delta)
		}
	}
	if reasoning.String() != "deep" {
		t.Errorf("reasoning = %q, want deep", reasoning.String())
	}
}

// TestOpenRouterParseStreamInStreamError verifies a mid-stream error object surfaces as a
// terminal EventError wrapping ErrProviderError — OpenRouter's quirk of reporting upstream
// failures after a 200.
//
// 验证流中 error 对象冒泡为终态 EventError 并包 ErrProviderError——OpenRouter 在 200 后报
// 上游失败的怪癖。
func TestOpenRouterParseStreamInStreamError(t *testing.T) {
	p := newOpenRouterProvider()
	resp := &http.Response{Body: sseBody(
		`data: {"choices":[{"delta":{"content":"partial"}}]}`,
		`data: {"error":{"message":"upstream exploded"}}`,
		`data: [DONE]`,
	)}
	events := collect(p.ParseStream(context.Background(), resp, Request{}))

	var sawErr bool
	for _, ev := range events {
		if ev.Type == EventError {
			sawErr = true
			if !errors.Is(ev.Err, ErrProviderError) {
				t.Errorf("err = %v, want wraps ErrProviderError", ev.Err)
			}
			if !strings.Contains(ev.Err.Error(), "upstream exploded") {
				t.Errorf("err = %v, want contains upstream message", ev.Err)
			}
		}
	}
	if !sawErr {
		t.Error("expected in-stream EventError, got none")
	}
}

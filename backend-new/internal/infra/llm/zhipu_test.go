package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestZhipuBuildRequest(t *testing.T) {
	p := newZhipuProvider()
	req := Request{
		ModelID:  "glm-4.6",
		Key:      "zk-test",
		BaseURL:  "https://open.bigmodel.cn/api/paas/v4",
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
	if got := httpReq.URL.String(); got != "https://open.bigmodel.cn/api/paas/v4/chat/completions" {
		t.Errorf("url = %s", got)
	}
	if got := httpReq.Header.Get("Authorization"); got != "Bearer zk-test" {
		t.Errorf("auth = %q", got)
	}
	body, _ := io.ReadAll(httpReq.Body)
	var zr zhipuRequest
	if err := json.Unmarshal(body, &zr); err != nil {
		t.Fatal(err)
	}
	if zr.Model != "glm-4.6" || !zr.Stream {
		t.Errorf("model=%s stream=%v", zr.Model, zr.Stream)
	}
	// tool_choice quirk: must be "auto" when tools are present.
	// tool_choice quirk：有 tools 时必须为 "auto"。
	if zr.ToolChoice != "auto" {
		t.Errorf("tool_choice = %q, want auto", zr.ToolChoice)
	}
	if len(zr.Tools) != 1 || zr.Tools[0].Function.Name != "get_weather" {
		t.Errorf("tools = %+v", zr.Tools)
	}
	if len(zr.Messages) != 2 || zr.Messages[0].Role != "system" || zr.Messages[1].Role != "user" {
		t.Errorf("messages = %+v", zr.Messages)
	}
}

func TestZhipuBuildRequestToolless(t *testing.T) {
	p := newZhipuProvider()
	req := Request{ModelID: "glm-4.6", Key: "k", BaseURL: "https://x", Messages: []LLMMessage{{Role: RoleUser, Content: "hi"}}}
	httpReq, err := p.BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(httpReq.Body)
	var zr zhipuRequest
	_ = json.Unmarshal(body, &zr)
	// No tools → tool_choice omitted entirely.
	// 无 tools → tool_choice 整字段省略。
	if zr.ToolChoice != "" {
		t.Errorf("tool_choice = %q, want omitted for tool-less request", zr.ToolChoice)
	}
}

func TestZhipuBuildRequestThinkingModes(t *testing.T) {
	p := newZhipuProvider()
	base := Request{ModelID: "glm-4.6", Key: "k", BaseURL: "https://x"}
	thinkingOf := func(req Request) *zhipuThinking {
		httpReq, err := p.BuildRequest(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(httpReq.Body)
		var zr zhipuRequest
		_ = json.Unmarshal(body, &zr)
		return zr.Thinking
	}
	// No Options → thinking field omitted.
	// 无 Options → thinking 字段省略。
	if got := thinkingOf(base); got != nil {
		t.Errorf("no options → %+v, want omitted", got)
	}
	// Native thinking value passes through verbatim into thinking:{type}.
	// 原生 thinking 值原样进 thinking:{type}。
	base.Options = map[string]string{"thinking": "enabled"}
	if got := thinkingOf(base); got == nil || got.Type != "enabled" {
		t.Errorf("enabled → %+v, want {type:enabled}", got)
	}
	base.Options = map[string]string{"thinking": "disabled"}
	if got := thinkingOf(base); got == nil || got.Type != "disabled" {
		t.Errorf("disabled → %+v, want {type:disabled}", got)
	}
}

func TestZhipuParseStream(t *testing.T) {
	p := newZhipuProvider()
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
		case EventReasoning:
			reasoning.WriteString(ev.Delta)
		case EventText:
			text.WriteString(ev.Delta)
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

// TestZhipuParseStreamExtendedFinish verifies a Zhipu-specific finish_reason
// ("sensitive") passes through verbatim inside EventFinish.
//
// TestZhipuParseStreamExtendedFinish 验证 Zhipu 专属 finish_reason（sensitive）原样透传。
func TestZhipuParseStreamExtendedFinish(t *testing.T) {
	p := newZhipuProvider()
	resp := &http.Response{Body: sseBody(
		`data: {"choices":[{"delta":{"content":"x"},"finish_reason":"sensitive"}]}`,
		`data: [DONE]`,
	)}
	events := collect(p.ParseStream(context.Background(), resp, Request{}))
	var sawFinish bool
	for _, ev := range events {
		if ev.Type == EventFinish {
			sawFinish = true
			if ev.FinishReason != "sensitive" {
				t.Errorf("finish_reason = %q, want sensitive", ev.FinishReason)
			}
		}
	}
	if !sawFinish {
		t.Error("missing finish event")
	}
}

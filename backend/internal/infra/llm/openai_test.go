package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"testing"
)

func collectEvents(sseText string) []StreamEvent {
	var events []StreamEvent
	r := strings.NewReader(sseText)
	parseOpenAISSE(context.Background(), r, func(e StreamEvent) bool {
		events = append(events, e)
		return true
	})
	return events
}

func TestParseSSE_TextOnly(t *testing.T) {
	sse := `data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"choices":[{"delta":{"content":" world"},"finish_reason":null}]}

data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}

data: [DONE]
`
	events := collectEvents(sse)

	textEvents := filterType(events, EventText)
	if len(textEvents) != 2 {
		t.Fatalf("want 2 text events, got %d", len(textEvents))
	}
	if textEvents[0].Delta != "Hello" || textEvents[1].Delta != " world" {
		t.Errorf("text deltas = %q %q", textEvents[0].Delta, textEvents[1].Delta)
	}

	finishEvents := filterType(events, EventFinish)
	if len(finishEvents) != 1 {
		t.Fatalf("want 1 finish event, got %d", len(finishEvents))
	}
	if finishEvents[0].FinishReason != "stop" {
		t.Errorf("finish reason = %q, want stop", finishEvents[0].FinishReason)
	}
	if finishEvents[0].InputTokens != 5 || finishEvents[0].OutputTokens != 2 {
		t.Errorf("tokens = in:%d out:%d, want in:5 out:2",
			finishEvents[0].InputTokens, finishEvents[0].OutputTokens)
	}
}

func TestParseSSE_ToolCall(t *testing.T) {
	// Simulates OpenAI streaming a single tool call across multiple chunks.
	// 模拟 OpenAI 跨多个 chunk 流式传输一个 tool call。
	sse := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"cit"}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"y\":\"Beijing\"}"}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]
`
	events := collectEvents(sse)

	starts := filterType(events, EventToolStart)
	if len(starts) != 1 {
		t.Fatalf("want 1 EventToolStart, got %d", len(starts))
	}
	if starts[0].ToolName != "get_weather" || starts[0].ToolID != "call_1" {
		t.Errorf("tool start: name=%q id=%q", starts[0].ToolName, starts[0].ToolID)
	}

	deltas := filterType(events, EventToolDelta)
	if len(deltas) != 2 {
		t.Fatalf("want 2 EventToolDelta, got %d", len(deltas))
	}
	assembled := deltas[0].ArgsDelta + deltas[1].ArgsDelta
	var args map[string]any
	if err := json.Unmarshal([]byte(assembled), &args); err != nil {
		t.Errorf("assembled args not valid JSON: %q", assembled)
	}
	if args["city"] != "Beijing" {
		t.Errorf("city = %v, want Beijing", args["city"])
	}
}

func TestParseSSE_ParallelToolCalls(t *testing.T) {
	// Two tool calls in one response, each with a different index.
	// 一次响应中两个 tool call，各有不同 index。
	sse := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":\"Beijing\"}"}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_2","function":{"name":"get_time","arguments":""}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"tz\":\"UTC\"}"}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]
`
	events := collectEvents(sse)

	starts := filterType(events, EventToolStart)
	if len(starts) != 2 {
		t.Fatalf("want 2 EventToolStart, got %d", len(starts))
	}
	names := []string{starts[0].ToolName, starts[1].ToolName}
	if !slices.Contains(names, "get_weather") || !slices.Contains(names, "get_time") {
		t.Errorf("tool names = %v", names)
	}
}

func TestParseSSE_ReasoningContent(t *testing.T) {
	// DeepSeek-R1 style: reasoning_content before content.
	// DeepSeek-R1 风格：reasoning_content 在 content 之前。
	sse := `data: {"choices":[{"delta":{"reasoning_content":"Let me think..."},"finish_reason":null}]}

data: {"choices":[{"delta":{"content":"The answer is 42."},"finish_reason":"stop"}]}

data: [DONE]
`
	events := collectEvents(sse)

	reasoning := filterType(events, EventReasoning)
	if len(reasoning) != 1 || reasoning[0].Delta != "Let me think..." {
		t.Errorf("reasoning events = %+v", reasoning)
	}
	texts := filterType(events, EventText)
	if len(texts) != 1 || texts[0].Delta != "The answer is 42." {
		t.Errorf("text events = %+v", texts)
	}
}

func TestParseSSE_UsageOnlyChunk(t *testing.T) {
	// Some providers send a final usage-only chunk with no choices.
	// 某些 provider 在最后发一个无 choices 的 usage-only chunk。
	sse := `data: {"choices":[{"delta":{"content":"hi"},"finish_reason":"stop"}]}

data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":1}}

data: [DONE]
`
	events := collectEvents(sse)
	finishes := filterType(events, EventFinish)
	// The usage-only chunk should emit an additional EventFinish with tokens.
	// usage-only chunk 应额外发一个带 token 的 EventFinish。
	found := false
	for _, f := range finishes {
		if f.InputTokens == 10 && f.OutputTokens == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("no EventFinish with usage tokens; got %+v", finishes)
	}
}

func TestParseSSE_ContextCancelled(t *testing.T) {
	sse := `data: {"choices":[{"delta":{"content":"a"},"finish_reason":null}]}

data: {"choices":[{"delta":{"content":"b"},"finish_reason":null}]}

data: [DONE]
`
	ctx, cancel := context.WithCancel(context.Background())
	var count int
	parseOpenAISSE(ctx, strings.NewReader(sse), func(e StreamEvent) bool {
		count++
		cancel() // cancel after first event
		return false
	})
	if count != 1 {
		t.Errorf("expected exactly 1 event before cancel, got %d", count)
	}
}

func TestBuildOpenAIBody_SystemPrepended(t *testing.T) {
	req := Request{
		ModelID: "gpt-4o",
		System:  "You are helpful.",
		Messages: []LLMMessage{
			{Role: RoleUser, Content: "Hello"},
		},
	}
	body, err := buildOpenAIBody(req)
	if err != nil {
		t.Fatalf("buildOpenAIBody: %v", err)
	}
	var out oaiRequest
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Messages) != 2 {
		t.Fatalf("want 2 messages (system + user), got %d", len(out.Messages))
	}
	var systemContent string
	json.Unmarshal(out.Messages[0].Content, &systemContent)
	if systemContent != "You are helpful." {
		t.Errorf("system content = %q", systemContent)
	}
}

func TestBuildOpenAIBody_ToolCall(t *testing.T) {
	req := Request{
		ModelID: "gpt-4o",
		Messages: []LLMMessage{
			{
				Role:    RoleAssistant,
				Content: "",
				ToolCalls: []LLMToolCall{
					{ID: "call_1", Name: "get_weather", Arguments: `{"city":"Beijing"}`},
				},
			},
			{Role: RoleTool, Content: "晴，25°C", ToolCallID: "call_1"},
		},
	}
	body, err := buildOpenAIBody(req)
	if err != nil {
		t.Fatalf("buildOpenAIBody: %v", err)
	}
	var out oaiRequest
	json.Unmarshal(body, &out)
	if len(out.Messages) != 2 {
		t.Fatalf("want 2 messages, got %d", len(out.Messages))
	}
	if len(out.Messages[0].ToolCalls) != 1 {
		t.Errorf("assistant should have 1 tool call")
	}
	if out.Messages[1].ToolCallID != "call_1" {
		t.Errorf("tool message tool_call_id = %q", out.Messages[1].ToolCallID)
	}
}

func TestBuildOpenAIBody_StreamEnabled(t *testing.T) {
	req := Request{ModelID: "gpt-4o", Messages: []LLMMessage{{Role: RoleUser, Content: "hi"}}}
	body, _ := buildOpenAIBody(req)
	var out oaiRequest
	json.Unmarshal(body, &out)
	if !out.Stream {
		t.Error("stream should be true")
	}
	if out.StreamOptions == nil || !out.StreamOptions.IncludeUsage {
		t.Error("stream_options.include_usage should be true")
	}
}

func TestBuildOpenAIBody_MultiModalUser(t *testing.T) {
	req := Request{
		ModelID: "gpt-4o",
		Messages: []LLMMessage{{
			Role: RoleUser,
			Parts: []ContentPart{
				{Type: "text", Text: "What's in this image?"},
				{Type: "image_url", ImageURL: "data:image/png;base64,abc"},
			},
		}},
	}
	body, err := buildOpenAIBody(req)
	if err != nil {
		t.Fatalf("buildOpenAIBody: %v", err)
	}
	var out oaiRequest
	json.Unmarshal(body, &out)

	var parts []oaiContentPart
	if err := json.Unmarshal(out.Messages[0].Content, &parts); err != nil {
		t.Fatalf("content is not a parts array: %v", err)
	}
	if len(parts) != 2 {
		t.Fatalf("want 2 parts, got %d", len(parts))
	}
}

func TestBuildOpenAIBody_ReasoningOnly_PromotedToContent(t *testing.T) {
	req := Request{
		ModelID: "deepseek-chat",
		Messages: []LLMMessage{
			{
				Role:             RoleAssistant,
				Content:          "",
				ReasoningContent: "你好！我是 Forgify",
			},
		},
	}
	body, err := buildOpenAIBody(req)
	if err != nil {
		t.Fatalf("buildOpenAIBody: %v", err)
	}
	var out oaiRequest
	json.Unmarshal(body, &out)
	var content string
	if err := json.Unmarshal(out.Messages[0].Content, &content); err != nil {
		t.Fatalf("content not a string: %v", err)
	}
	if content != "你好！我是 Forgify" {
		t.Errorf("content = %q, want fallback-promoted from reasoning_content", content)
	}
	if out.Messages[0].ReasoningContent != "你好！我是 Forgify" {
		t.Errorf("reasoning_content not preserved: %q", out.Messages[0].ReasoningContent)
	}
}

// TestBuildOpenAIBody_ReasoningWithText_NoPromotion: text present →
// no fallback, reasoning/content distinction preserved.
func TestBuildOpenAIBody_ReasoningWithText_NoPromotion(t *testing.T) {
	req := Request{
		ModelID: "deepseek-chat",
		Messages: []LLMMessage{
			{
				Role:             RoleAssistant,
				Content:          "the answer is 42",
				ReasoningContent: "thinking...",
			},
		},
	}
	body, _ := buildOpenAIBody(req)
	var out oaiRequest
	json.Unmarshal(body, &out)
	var content string
	json.Unmarshal(out.Messages[0].Content, &content)
	if content != "the answer is 42" {
		t.Errorf("content = %q (must NOT be promoted from reasoning when text present)", content)
	}
	if out.Messages[0].ReasoningContent != "thinking..." {
		t.Errorf("reasoning_content lost")
	}
}

// TestBuildOpenAIBody_ReasoningWithToolCall_NoPromotion: tool_calls present
// → no fallback (reasoning + tool_calls is a valid wire shape on its own).
func TestBuildOpenAIBody_ReasoningWithToolCall_NoPromotion(t *testing.T) {
	req := Request{
		ModelID: "deepseek-chat",
		Messages: []LLMMessage{
			{
				Role:             RoleAssistant,
				Content:          "",
				ReasoningContent: "I should look this up",
				ToolCalls: []LLMToolCall{
					{ID: "c1", Name: "search", Arguments: `{"q":"x"}`},
				},
			},
		},
	}
	body, _ := buildOpenAIBody(req)
	var out oaiRequest
	json.Unmarshal(body, &out)
	var content string
	json.Unmarshal(out.Messages[0].Content, &content)
	if content != "" {
		t.Errorf("content = %q (must remain empty; tool_calls suppress fallback)", content)
	}
}

// TestBuildOpenAIBody_AssistantContentAlwaysEmitted: assistant with tool_calls
// but empty content must still emit `"content": ""` (not null, not omitted).
// Strict providers (OpenAI, Zhipu GLM) reject `content: null`; an explicit
// empty string satisfies the "set" check.
//
// assistant 有 tool_calls 但 content 空时仍 emit `"content": ""`
// （不是 null 不是 omit）。严格 provider 拒 null。
func TestBuildOpenAIBody_AssistantContentAlwaysEmitted(t *testing.T) {
	req := Request{
		ModelID: "gpt-4o",
		Messages: []LLMMessage{
			{
				Role:    RoleAssistant,
				Content: "",
				ToolCalls: []LLMToolCall{
					{ID: "c1", Name: "x", Arguments: `{}`},
				},
			},
		},
	}
	body, _ := buildOpenAIBody(req)
	// Quick string scan since json.RawMessage handling is annoying for
	// "is this field present" assertions.
	// 用字符串扫描快速判断字段是否真的 present。
	if !bytes.Contains(body, []byte(`"content":""`)) {
		t.Errorf("expected `\"content\":\"\"` in body, got: %s", body)
	}
	if bytes.Contains(body, []byte(`"content":null`)) {
		t.Errorf("body must NOT contain `\"content\":null`")
	}
}

// TestParseSSE_MidStreamError_OpenRouter verifies OpenRouter's quirk:
// once any byte streams the HTTP status locks at 200 but the actual error
// arrives as an in-stream SSE chunk with a top-level `error` field. The
// parser must surface this as EventError; without TE-23's chunk.Error
// detection, the chunk parses to {Choices: nil, Error: nil} and the
// stream silently terminates with no user-visible explanation.
//
// OpenRouter quirk：流开始后 HTTP 状态锁 200，错误以 SSE chunk 形式抵达
// （顶层 error 字段）。解析器必须 surface 为 EventError；TE-23 之前流
// 静默终止用户看不到原因。
func TestParseSSE_MidStreamError_OpenRouter(t *testing.T) {
	body := `data: {"choices":[{"delta":{"role":"assistant","content":"hello "}}]}

data: {"error":{"message":"upstream model timeout","type":"upstream_error","code":"timeout"}}

`
	events := collectEvents(body)
	_ = t
	gotError := false
	gotText := false
	for _, ev := range events {
		if ev.Type == EventText && ev.Delta == "hello " {
			gotText = true
		}
		if ev.Type == EventError {
			gotError = true
			if ev.Err == nil {
				t.Errorf("EventError.Err is nil")
			} else if !strings.Contains(ev.Err.Error(), "upstream model timeout") {
				t.Errorf("error doesn't contain upstream message: %v", ev.Err)
			}
		}
	}
	if !gotText {
		t.Errorf("text delta before error should still be emitted")
	}
	if !gotError {
		t.Errorf("expected EventError for in-stream error chunk; got events: %+v", events)
	}
}

// TestParseSSE_ToolCalls_IndexAllZero_OllamaQuirk verifies the TE-24 fallback:
// providers (Ollama, some Gemini paths) leave tool_calls[].index at 0 for
// every parallel tool call. Without per-ID synthesis they all collide on
// key 0 → second tool's name dropped, args buffers merged. The fallback
// disambiguates by tool ID and assigns synthetic indices 0, 1, 2...
//
// Ollama / 部分 Gemini 不填 index，多个并发 tool_call 都 index=0 撞键。
// 兜底按 tool ID 区分并合成 index 0, 1, 2...
func TestParseSSE_ToolCalls_IndexAllZero_OllamaQuirk(t *testing.T) {
	body := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_A","function":{"name":"toolA","arguments":""}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_B","function":{"name":"toolB","arguments":""}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_A","function":{"arguments":"{\"x\":1}"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_B","function":{"arguments":"{\"y\":2}"}}]}}]}

`
	events := collectEvents(body)
	// Expect 2 ToolStart (one per distinct ID) + 2 ToolDelta routed to
	// the right (synthesized) indices.
	// 期望 2 个 ToolStart（按 ID 区分）+ 2 个 ToolDelta 路由到正确的合成 index。
	var starts []StreamEvent
	var deltas []StreamEvent
	for _, ev := range events {
		switch ev.Type {
		case EventToolStart:
			starts = append(starts, ev)
		case EventToolDelta:
			deltas = append(deltas, ev)
		}
	}
	if len(starts) != 2 {
		t.Fatalf("want 2 ToolStart events (one per unique ID), got %d: %+v", len(starts), starts)
	}
	if starts[0].ToolID != "call_A" || starts[1].ToolID != "call_B" {
		t.Errorf("ToolStart IDs wrong: %q %q", starts[0].ToolID, starts[1].ToolID)
	}
	if starts[0].ToolIndex == starts[1].ToolIndex {
		t.Errorf("ToolStart indices must differ; both got %d", starts[0].ToolIndex)
	}
	// Verify deltas route to the matching synthetic indices (call_A → 0,
	// call_B → 1, since they're seen in that order).
	// 验证 delta 路由到正确的合成 index。
	if len(deltas) != 2 {
		t.Fatalf("want 2 ToolDelta events, got %d", len(deltas))
	}
	if deltas[0].ToolIndex != starts[0].ToolIndex || deltas[0].ArgsDelta != `{"x":1}` {
		t.Errorf("first delta should match call_A index + args; got idx=%d args=%q",
			deltas[0].ToolIndex, deltas[0].ArgsDelta)
	}
	if deltas[1].ToolIndex != starts[1].ToolIndex || deltas[1].ArgsDelta != `{"y":2}` {
		t.Errorf("second delta should match call_B index + args; got idx=%d args=%q",
			deltas[1].ToolIndex, deltas[1].ArgsDelta)
	}
}

// TestParseOpenAINonStreaming_SynthesizesEvents verifies the non-streaming
// path (used for Ollama+tools per TE-24) synthesizes the same StreamEvent
// sequence as the streaming path would for equivalent content. Lets the
// rest of the system treat both wire modes uniformly.
//
// 非流式路径（Ollama+tools 走）合成 StreamEvent 序列，与流式路径输出一致。
func TestParseOpenAINonStreaming_SynthesizesEvents(t *testing.T) {
	body := `{
		"choices": [{
			"message": {
				"role": "assistant",
				"content": "let me check",
				"tool_calls": [
					{"index": 0, "id": "call_X", "function": {"name": "search", "arguments": "{\"q\":\"forgify\"}"}}
				]
			},
			"finish_reason": "tool_calls"
		}],
		"usage": {"prompt_tokens": 100, "completion_tokens": 20}
	}`
	var events []StreamEvent
	parseOpenAINonStreaming(strings.NewReader(body), func(e StreamEvent) bool {
		events = append(events, e)
		return true
	})
	if len(events) < 4 {
		t.Fatalf("expect Text + ToolStart + ToolDelta + Finish, got %d events", len(events))
	}
	if events[0].Type != EventText || events[0].Delta != "let me check" {
		t.Errorf("first event = %+v, want EventText 'let me check'", events[0])
	}
	if events[1].Type != EventToolStart || events[1].ToolName != "search" || events[1].ToolID != "call_X" {
		t.Errorf("second event = %+v, want EventToolStart search/call_X", events[1])
	}
	if events[2].Type != EventToolDelta || events[2].ArgsDelta != `{"q":"forgify"}` {
		t.Errorf("third event = %+v, want EventToolDelta with args", events[2])
	}
	last := events[len(events)-1]
	if last.Type != EventFinish || last.FinishReason != "tool_calls" {
		t.Errorf("last event = %+v, want EventFinish tool_calls", last)
	}
	if last.InputTokens != 100 || last.OutputTokens != 20 {
		t.Errorf("usage missing in finish event: %+v", last)
	}
}

func TestClassifyHTTPError(t *testing.T) {
	cases := []struct {
		status int
		substr string
	}{
		{401, "authentication"},
		{429, "rate limit"},
		{400, "bad request"},
		{404, "not found"},
		{500, "provider error"},
	}
	for _, c := range cases {
		err := classifyHTTPError(c.status, []byte("detail"))
		if err == nil {
			t.Errorf("status %d: want error, got nil", c.status)
			continue
		}
		if !strings.Contains(strings.ToLower(err.Error()), c.substr) {
			t.Errorf("status %d: error %q does not contain %q", c.status, err.Error(), c.substr)
		}
	}
}

func filterType(events []StreamEvent, t StreamEventType) []StreamEvent {
	var out []StreamEvent
	for _, e := range events {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out
}

var _ io.Reader = (*strings.Reader)(nil)

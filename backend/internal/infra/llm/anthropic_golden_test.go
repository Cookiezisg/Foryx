package llm

// anthropic_golden_test.go — L1 wire-shape + L2 httptest parse tests for the
// Anthropic native provider. No API key, no real network. Matches
// 03-implementation-reference §4 golden request body + SSE fixture.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ──────────────────────────────────────────────────────────────────────────────
// L1: BuildRequest wire-shape assertions
// ──────────────────────────────────────────────────────────────────────────────

// TestBuildRequest_Anthropic_GoldenShape verifies the native body shape for
// the Anthropic provider matches 03 §4: posts to base+/v1/messages; model +
// max_tokens present (REQUIRED); system as top-level block-array (not a
// message); tools use input_schema (not parameters); messages user/assistant
// only; auth via x-api-key + anthropic-version header (NOT Authorization Bearer).
//
// 验证 Anthropic BuildRequest wire shape：端点/header/body 与 03 §4 黄金请求体一致。
func TestBuildRequest_Anthropic_GoldenShape(t *testing.T) {
	p := newAnthropicProvider()
	req := Request{
		ModelID: "claude-sonnet-4-5",
		BaseURL: "https://api.anthropic.com",
		Key:     "sk-ant-test",
		System:  "weather assistant",
		Messages: []LLMMessage{
			{Role: RoleUser, Content: "Weather in SF?"},
		},
		Tools: []ToolDef{
			{
				Name:        "get_weather",
				Description: "Get weather",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}`),
			},
		},
	}

	httpReq, err := p.BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	// URL must be base + /v1/messages.
	// URL 必须是 base + /v1/messages。
	wantURL := "https://api.anthropic.com/v1/messages"
	if httpReq.URL.String() != wantURL {
		t.Errorf("URL = %q, want %q", httpReq.URL.String(), wantURL)
	}

	// Auth headers: x-api-key (not Authorization Bearer), anthropic-version.
	// 鉴权头：x-api-key（不是 Authorization Bearer），anthropic-version。
	if got := httpReq.Header.Get("x-api-key"); got != "sk-ant-test" {
		t.Errorf("x-api-key = %q, want sk-ant-test", got)
	}
	if httpReq.Header.Get("Authorization") != "" {
		t.Errorf("Authorization header must be absent for Anthropic (uses x-api-key); got: %q",
			httpReq.Header.Get("Authorization"))
	}
	if got := httpReq.Header.Get("anthropic-version"); got != "2023-06-01" {
		t.Errorf("anthropic-version = %q, want 2023-06-01", got)
	}
	if got := httpReq.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	var body anthropicRequest
	if err := json.NewDecoder(httpReq.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	// model present.
	if body.Model != "claude-sonnet-4-5" {
		t.Errorf("model = %q, want claude-sonnet-4-5", body.Model)
	}

	// max_tokens REQUIRED and non-zero.
	// max_tokens 必须存在且非零（Anthropic 规定 REQUIRED）。
	if body.MaxTokens <= 0 {
		t.Errorf("max_tokens = %d, must be > 0 (REQUIRED by Anthropic)", body.MaxTokens)
	}

	// stream must be true.
	if !body.Stream {
		t.Error("stream must be true")
	}

	// system is top-level, not a message; it must be a block array.
	// system 是顶层字段，不是 message；必须是 block 数组。
	if body.System == nil {
		t.Fatal("system field must be present")
	}
	var sysBlocks []anthropicSystemBlock
	if err := json.Unmarshal(body.System, &sysBlocks); err != nil {
		t.Fatalf("system is not a block array: %v — raw: %s", err, body.System)
	}
	if len(sysBlocks) != 1 {
		t.Fatalf("want 1 system block, got %d", len(sysBlocks))
	}
	if sysBlocks[0].Type != "text" {
		t.Errorf("system block type = %q, want text", sysBlocks[0].Type)
	}
	if sysBlocks[0].Text != "weather assistant" {
		t.Errorf("system block text = %q, want weather assistant", sysBlocks[0].Text)
	}

	// messages: only user/assistant roles (no system role in messages array).
	// messages 只能有 user/assistant（system 不进 messages 数组）。
	if len(body.Messages) != 1 {
		t.Fatalf("want 1 message (user), got %d", len(body.Messages))
	}
	if body.Messages[0].Role != "user" {
		t.Errorf("message[0].role = %q, want user", body.Messages[0].Role)
	}
	for _, m := range body.Messages {
		if m.Role == "system" {
			t.Errorf("system must not appear in messages array; got role %q", m.Role)
		}
	}

	// tools use input_schema (not parameters).
	// tools 用 input_schema，不是 parameters。
	if len(body.Tools) != 1 {
		t.Fatalf("want 1 tool, got %d", len(body.Tools))
	}
	tool := body.Tools[0]
	if tool.Name != "get_weather" {
		t.Errorf("tool.name = %q, want get_weather", tool.Name)
	}
	if tool.InputSchema == nil {
		t.Error("tool.input_schema must be present (Anthropic uses input_schema, not parameters)")
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
		t.Fatalf("tool.input_schema is not valid JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("tool.input_schema.type = %v, want object", schema["type"])
	}

	// There must be no "parameters" key at the tool level (would be wrong wire).
	// tool 不得有顶层 "parameters" 字段（Anthropic wire 只认 input_schema）。
	var rawTool map[string]json.RawMessage
	rawBody := make(map[string]json.RawMessage)
	if err := json.NewDecoder(strings.NewReader(`{}`)).Decode(&rawBody); err == nil {
		// re-marshal body to inspect raw tool
	}
	rawBodyBytes, _ := json.Marshal(body)
	if err := json.Unmarshal(rawBodyBytes, &rawBody); err == nil {
		var rawTools []map[string]json.RawMessage
		if err := json.Unmarshal(rawBody["tools"], &rawTools); err == nil && len(rawTools) > 0 {
			rawTool = rawTools[0]
			if _, hasParams := rawTool["parameters"]; hasParams {
				t.Error("tool must not have 'parameters' key; Anthropic wire uses 'input_schema'")
			}
		}
	}
}

// TestBuildRequest_Anthropic_NoSystem verifies that when system is empty the
// field is omitted (omitempty) and messages still encodes correctly.
//
// system 为空时 system 字段省略，messages 正确编码。
func TestBuildRequest_Anthropic_NoSystem(t *testing.T) {
	p := newAnthropicProvider()
	req := Request{
		ModelID: "claude-sonnet-4-5",
		BaseURL: "https://api.anthropic.com",
		Key:     "sk-ant-test",
		Messages: []LLMMessage{
			{Role: RoleUser, Content: "Hello"},
		},
	}
	httpReq, err := p.BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	var rawBody map[string]json.RawMessage
	if err := json.NewDecoder(httpReq.Body).Decode(&rawBody); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := rawBody["system"]; ok {
		t.Error("system field must be absent when system is empty")
	}
	if _, ok := rawBody["tools"]; ok {
		t.Error("tools field must be absent when no tools")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// L2: ParseStream via httptest — 03 §4 SSE fixture
// ──────────────────────────────────────────────────────────────────────────────

// anthropicSSEServer returns an httptest.Server that serves the given body
// with Content-Type text/event-stream and status 200.
//
// 返回以 text/event-stream 原样响应 body 的 httptest Server。
func anthropicSSEServer(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, body)
	}))
}

// collectAnthropicFromServer points the Anthropic provider at a test server
// and collects all StreamEvents from ParseStream via real HTTP.
//
// 把 Anthropic provider 指向测试服务器，通过真实 HTTP 收集 StreamEvent。
func collectAnthropicFromServer(t *testing.T, srv *httptest.Server) []StreamEvent {
	t.Helper()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("http.Get: %v", err)
	}
	p := newAnthropicProvider()
	req := Request{ModelID: "claude-sonnet-4-5", BaseURL: srv.URL}
	var events []StreamEvent
	for ev := range p.ParseStream(context.Background(), resp, req) {
		events = append(events, ev)
	}
	return events
}

// TestParseStream_Anthropic_FullFixture feeds the 03 §4 SSE fixture through
// ParseStream and asserts the full event sequence:
//   - message_start → input tokens captured
//   - thinking_delta → EventReasoning
//   - signature_delta → silently dropped (no StreamEvent field today; P3.6)
//   - text_delta → EventText
//   - content_block_start(tool_use) → EventToolStart
//   - input_json_delta → EventToolDelta, args accumulate correctly
//   - message_delta(stop_reason) → EventFinish with input+output tokens
//   - message_stop → no event (graceful termination)
//
// 对照 03 §4 黄金 SSE fixture 验证完整事件序列。
func TestParseStream_Anthropic_FullFixture(t *testing.T) {
	// 03 §4 golden SSE fixture: message_start → thinking block → text block
	// → tool_use block → message_delta(stop_reason) → message_stop.
	//
	// signature_delta is included to document current behavior: it is silently
	// dropped because anthropicBlockDelta.Delta has no Signature field.
	// TODO(P3.6): capture signature for thinking round-trip.
	fixture := `event: message_start
data: {"type":"message_start","message":{"usage":{"input_tokens":42}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"I need to check the weather API."}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"opaque-sig-data-abc123"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Let me check the weather."}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: content_block_start
data: {"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_weather_01","name":"get_weather","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"location\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"\"San Francisco\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":2}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":87}}

event: message_stop
data: {"type":"message_stop"}

`
	srv := anthropicSSEServer(fixture)
	defer srv.Close()
	events := collectAnthropicFromServer(t, srv)

	// thinking_delta → EventReasoning.
	reasoning := filterType(events, EventReasoning)
	if len(reasoning) != 1 {
		t.Fatalf("want 1 EventReasoning (thinking_delta), got %d", len(reasoning))
	}
	if reasoning[0].Delta != "I need to check the weather API." {
		t.Errorf("reasoning delta = %q, want 'I need to check the weather API.'", reasoning[0].Delta)
	}

	// text_delta → EventText.
	texts := filterType(events, EventText)
	if len(texts) != 1 {
		t.Fatalf("want 1 EventText, got %d", len(texts))
	}
	if texts[0].Delta != "Let me check the weather." {
		t.Errorf("text delta = %q, want 'Let me check the weather.'", texts[0].Delta)
	}

	// content_block_start(tool_use) → EventToolStart.
	starts := filterType(events, EventToolStart)
	if len(starts) != 1 {
		t.Fatalf("want 1 EventToolStart, got %d", len(starts))
	}
	if starts[0].ToolName != "get_weather" {
		t.Errorf("tool name = %q, want get_weather", starts[0].ToolName)
	}
	if starts[0].ToolID != "toolu_weather_01" {
		t.Errorf("tool id = %q, want toolu_weather_01", starts[0].ToolID)
	}
	if starts[0].ToolIndex != 2 {
		t.Errorf("tool index = %d, want 2", starts[0].ToolIndex)
	}

	// input_json_delta × 2 → EventToolDelta; assembled args must be valid JSON.
	// input_json_delta × 2 → EventToolDelta；拼装后须是合法 JSON。
	deltas := filterType(events, EventToolDelta)
	if len(deltas) != 2 {
		t.Fatalf("want 2 EventToolDelta (partial_json chunks), got %d", len(deltas))
	}
	assembled := deltas[0].ArgsDelta + deltas[1].ArgsDelta
	var args map[string]any
	if err := json.Unmarshal([]byte(assembled), &args); err != nil {
		t.Errorf("assembled tool args not valid JSON: %q err: %v", assembled, err)
	}
	if args["location"] != "San Francisco" {
		t.Errorf("args.location = %v, want San Francisco", args["location"])
	}

	// message_delta(stop_reason) → EventFinish with input+output tokens.
	// message_delta(stop_reason) → EventFinish，含 input/output token。
	finishes := filterType(events, EventFinish)
	if len(finishes) != 1 {
		t.Fatalf("want 1 EventFinish, got %d", len(finishes))
	}
	if finishes[0].FinishReason != "tool_use" {
		t.Errorf("finish_reason = %q, want tool_use", finishes[0].FinishReason)
	}
	if finishes[0].InputTokens != 42 {
		t.Errorf("input_tokens = %d, want 42", finishes[0].InputTokens)
	}
	if finishes[0].OutputTokens != 87 {
		t.Errorf("output_tokens = %d, want 87", finishes[0].OutputTokens)
	}

	// message_stop emits no StreamEvent — stream ends cleanly.
	// message_stop 不发 StreamEvent——流干净结束。
	errEvents := filterType(events, EventError)
	if len(errEvents) != 0 {
		t.Errorf("no EventError expected; got: %+v", errEvents)
	}
}

// TestParseStream_Anthropic_SignatureDeltaDropped documents that the current
// parser silently drops signature_delta events (anthropicBlockDelta.Delta has
// no Signature field → emitAnthropicDelta switch falls through). This is
// intentional tracking of the P3.6 gap.
//
// 记录当前 parser 静默丢弃 signature_delta（anthropicBlockDelta.Delta 无 Signature
// 字段，emitAnthropicDelta switch 无 case 匹配）。这是 P3.6 gap 的有意记录。
//
// TODO(P3.6): capture signature for thinking round-trip — add Signature field
// to anthropicBlockDelta.Delta and a new StreamEvent field to carry it through
// the history store, so multi-turn thinking works without 400.
func TestParseStream_Anthropic_SignatureDeltaDropped(t *testing.T) {
	// A fixture with ONLY a signature_delta — current behavior: zero events.
	// 仅含 signature_delta 的 fixture——当前行为：零事件输出。
	fixture := `event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"opaque-sig-abc"}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}

`
	events := collectAnthropicEvents(fixture)

	// Document: NO StreamEvent is emitted for signature_delta today.
	// 记录：当前 signature_delta 不产生任何 StreamEvent。
	for _, ev := range events {
		// Finish event from message_delta is fine. Error events would be a regression.
		if ev.Type == EventError {
			t.Errorf("unexpected EventError for signature_delta: %v", ev.Err)
		}
	}

	// Assert no dedicated "signature" event exists (we have no such event type).
	// 断言不存在专用 signature 事件类型（当前设计无此 StreamEventType）。
	t.Log("P3.6: signature_delta is silently dropped — no StreamEvent emitted for it. " +
		"Multi-turn thinking (Anthropic's tool_use loops) will fail without round-trip of signature. " +
		"Fix: add Delta.Signature field to anthropicBlockDelta and carry through event + store.")
}

// TestParseStream_Anthropic_EndTurnFinish verifies that message_delta with
// stop_reason=end_turn produces EventFinish with correct usage tokens.
//
// 验证 stop_reason=end_turn 产生 EventFinish，token 正确。
func TestParseStream_Anthropic_EndTurnFinish(t *testing.T) {
	fixture := `event: message_start
data: {"type":"message_start","message":{"usage":{"input_tokens":15}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Paris."}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}

event: message_stop
data: {"type":"message_stop"}

`
	srv := anthropicSSEServer(fixture)
	defer srv.Close()
	events := collectAnthropicFromServer(t, srv)

	texts := filterType(events, EventText)
	if len(texts) != 1 || texts[0].Delta != "Paris." {
		t.Errorf("text events = %+v", texts)
	}
	finishes := filterType(events, EventFinish)
	if len(finishes) != 1 {
		t.Fatalf("want 1 EventFinish, got %d", len(finishes))
	}
	if finishes[0].FinishReason != "end_turn" {
		t.Errorf("finish_reason = %q, want end_turn", finishes[0].FinishReason)
	}
	if finishes[0].InputTokens != 15 || finishes[0].OutputTokens != 3 {
		t.Errorf("tokens: in=%d out=%d, want in=15 out=3",
			finishes[0].InputTokens, finishes[0].OutputTokens)
	}

	errEvents := filterType(events, EventError)
	if len(errEvents) != 0 {
		t.Errorf("unexpected error events: %+v", errEvents)
	}
}

// TestBuildRequest_Anthropic_MaxTokensPerModel verifies that max_tokens is now
// derived from modelcaps.Lookup("anthropic", modelID) rather than the old
// hardcoded constant (8096). claude-sonnet-4-5 matches the "claude-sonnet-4"
// prefix rule → MaxOutput == 64000. An unknown model uses the modelcaps global
// fallback (Cap{MaxOutput:8192}) — larger than the old 8096 constant — and only
// falls back to anthropicDefaultMaxTokens (8096) when Lookup returns MaxOutput==0.
//
// 验证 max_tokens 现在从 modelcaps.Lookup 派生，不再硬编码。
// claude-sonnet-4-5 匹配 "claude-sonnet-4" 前缀规则 → MaxOutput==64000；
// 完全未知 model 用 modelcaps 全局 fallback (MaxOutput=8192)；
// 仅 Lookup 返回 MaxOutput==0 时才退回 anthropicDefaultMaxTokens (8096)。
func TestBuildRequest_Anthropic_MaxTokensPerModel(t *testing.T) {
	p := newAnthropicProvider()

	cases := []struct {
		modelID    string
		wantMaxTok int
		desc       string
	}{
		{"claude-sonnet-4-5", 64_000, "known model matches claude-sonnet-4 rule → 64000"},
		// A model ID with no matching provider prefix hits the global fallback Cap{MaxOutput:8192}.
		// provider 无前缀匹配时命中全局 fallback Cap{MaxOutput:8192}。
		{"totally-unknown-model-zzz", 8_192, "unknown model uses modelcaps global fallback 8192"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			req := Request{
				ModelID:  tc.modelID,
				BaseURL:  "https://api.anthropic.com",
				Key:      "sk-ant-test",
				Messages: []LLMMessage{{Role: RoleUser, Content: "hi"}},
			}
			httpReq, err := p.BuildRequest(context.Background(), req)
			if err != nil {
				t.Fatalf("BuildRequest: %v", err)
			}
			var body anthropicRequest
			if err := json.NewDecoder(httpReq.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body.MaxTokens != tc.wantMaxTok {
				t.Errorf("max_tokens = %d, want %d", body.MaxTokens, tc.wantMaxTok)
			}
		})
	}
}

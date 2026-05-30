package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ──────────────────────────────────────────────────────────────────────────────
// L1: BuildRequest byte/shape assertions (no key, no network)
// ──────────────────────────────────────────────────────────────────────────────

// TestBuildRequest_DeepSeek_GoldenShape verifies that BuildRequest for the
// DeepSeek compat provider emits the correct wire fields: model, messages
// (system → role:system, user), tools in OpenAI shape, stream:true,
// stream_options.include_usage:true. Matches 03-implementation-reference §3.
//
// 验证 DeepSeek BuildRequest 的 wire shape：model/messages/tools/stream/stream_options，
// 对照 03 §3 黄金请求体。Thinking 字段属 P3，此处不断言。
func TestBuildRequest_DeepSeek_GoldenShape(t *testing.T) {
	p := providerRegistry["deepseek"]
	req := Request{
		ModelID: "deepseek-v4-pro",
		BaseURL: "https://api.deepseek.com",
		Key:     "sk-test",
		System:  "helpful",
		Messages: []LLMMessage{
			{Role: RoleUser, Content: "Weather in SF?"},
		},
		Tools: []ToolDef{
			{
				Name:        "get_weather",
				Description: "",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}`),
			},
		},
	}
	httpReq, err := p.BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	var body oaiRequest
	if err := json.NewDecoder(httpReq.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Model != "deepseek-v4-pro" {
		t.Errorf("model = %q, want deepseek-v4-pro", body.Model)
	}
	if !body.Stream {
		t.Error("stream must be true")
	}
	if body.StreamOptions == nil || !body.StreamOptions.IncludeUsage {
		t.Error("stream_options.include_usage must be true")
	}
	// system message prepended as role:system.
	// system 消息以 role:system 置首。
	if len(body.Messages) < 2 {
		t.Fatalf("want at least 2 messages (system + user), got %d", len(body.Messages))
	}
	if body.Messages[0].Role != "system" {
		t.Errorf("first message role = %q, want system", body.Messages[0].Role)
	}
	var systemContent string
	if err := json.Unmarshal(body.Messages[0].Content, &systemContent); err != nil {
		t.Fatalf("system content not a string: %v", err)
	}
	if systemContent != "helpful" {
		t.Errorf("system content = %q, want helpful", systemContent)
	}
	if body.Messages[1].Role != "user" {
		t.Errorf("second message role = %q, want user", body.Messages[1].Role)
	}
	// tools in OpenAI shape: type:function + function.{name,parameters}.
	// tools 采用 OpenAI shape：type:function + function 子对象。
	if len(body.Tools) != 1 {
		t.Fatalf("want 1 tool, got %d", len(body.Tools))
	}
	tool := body.Tools[0]
	if tool.Type != "function" {
		t.Errorf("tool.type = %q, want function", tool.Type)
	}
	if tool.Function.Name != "get_weather" {
		t.Errorf("tool.function.name = %q, want get_weather", tool.Function.Name)
	}
	if tool.Function.Parameters == nil {
		t.Error("tool.function.parameters must be present")
	}
	var params map[string]any
	if err := json.Unmarshal(tool.Function.Parameters, &params); err != nil {
		t.Fatalf("tool parameters not valid JSON: %v", err)
	}
	if params["type"] != "object" {
		t.Errorf("tool parameters.type = %v, want object", params["type"])
	}
}

// TestChatURL_IsBaseSlashChatCompletions verifies that every provider that
// speaks /chat/completions appends it to its base URL. openai and deepseek now
// have their own provider types but still speak the same /chat/completions path;
// the test uses the Provider interface directly.
//
// 验证所有 /chat/completions provider 的 chat 端点 = base + /chat/completions。
// openai 和 deepseek 已迁移为自有类型，仍走 /chat/completions；此处直接用 Provider 接口。
func TestChatURL_IsBaseSlashChatCompletions(t *testing.T) {
	cases := []struct {
		provider string
		wantBase string
	}{
		{"openai", "https://api.openai.com/v1"},
		{"deepseek", "https://api.deepseek.com"},
		{"qwen", "https://dashscope.aliyuncs.com/compatible-mode/v1"},
		{"openrouter", "https://openrouter.ai/api/v1"},
		{"ollama", "http://localhost:11434/v1"},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			p, ok := providerRegistry[tc.provider]
			if !ok {
				t.Fatalf("provider %q not in registry", tc.provider)
			}
			req := Request{
				ModelID: "test-model",
				BaseURL: tc.wantBase,
				Key:     "k",
				Messages: []LLMMessage{
					{Role: RoleUser, Content: "hi"},
				},
			}
			httpReq, err := p.BuildRequest(context.Background(), req)
			if err != nil {
				t.Fatalf("BuildRequest: %v", err)
			}
			wantURL := tc.wantBase + "/chat/completions"
			if httpReq.URL.String() != wantURL {
				t.Errorf("URL = %q, want %q", httpReq.URL.String(), wantURL)
			}
		})
	}
}

// TestDefaultBaseURL_AllKnownCompatProviders checks each compat provider's
// DefaultBaseURL matches the canonical value from 03-implementation-reference.
//
// 每个 compat provider 的 DefaultBaseURL 与 03 §各节正确值一致。
func TestDefaultBaseURL_AllKnownCompatProviders(t *testing.T) {
	cases := []struct{ name, wantBase string }{
		{"openai", "https://api.openai.com/v1"},
		{"deepseek", "https://api.deepseek.com"},
		// google is native generateContent (not /chat/completions) but still has a
		// canonical DefaultBaseURL; only the base value is asserted here.
		// google 是原生 generateContent（非 /chat/completions），但仍有规范的
		// DefaultBaseURL；此处只断言 base 值。
		{"google", "https://generativelanguage.googleapis.com/v1beta"},
		{"qwen", "https://dashscope.aliyuncs.com/compatible-mode/v1"},
		{"zhipu", "https://open.bigmodel.cn/api/paas/v4"},
		{"moonshot", "https://api.moonshot.cn/v1"},
		{"doubao", "https://ark.cn-beijing.volces.com/api/v3"},
		{"openrouter", "https://openrouter.ai/api/v1"},
		// ollama and custom require caller-supplied base_url.
		// ollama 和 custom 需要 caller 提供 base_url，DefaultBaseURL 为空。
		{"ollama", ""},
		{"custom", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := providerRegistry[tc.name]
			if !ok {
				t.Fatalf("provider %q missing from registry", tc.name)
			}
			got := p.DefaultBaseURL()
			if got != tc.wantBase {
				t.Errorf("DefaultBaseURL() = %q, want %q", got, tc.wantBase)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// L2: ParseStream via httptest (no key, no real network)
// ──────────────────────────────────────────────────────────────────────────────

// sseServer builds a test SSE server that writes body verbatim with
// Content-Type text/event-stream and status 200.
//
// sseServer 构造一个 test SSE 服务，将 body 原样以 text/event-stream 写回。
func sseServer(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, body)
	}))
}

// collectFromServer points openaiProvider at the given server, fires ParseStream,
// and returns all events. It bypasses BuildRequest to avoid needing an actual
// body; ParseStream is the target.
//
// collectFromServer 把 openaiProvider 指向测试服务器执行 ParseStream，
// 绕过 BuildRequest（SSE 解析才是目标）。
func collectFromServer(t *testing.T, srv *httptest.Server) []StreamEvent {
	t.Helper()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("http.Get: %v", err)
	}

	p := newOpenAIProvider()
	req := Request{ModelID: "test-model", BaseURL: srv.URL}

	var events []StreamEvent
	for ev := range p.ParseStream(context.Background(), resp, req) {
		events = append(events, ev)
	}
	return events
}

// TestParseStream_StandardCompat_DeepSeek verifies the standard compat SSE
// path for a DeepSeek-style response: content deltas → EventText, tool-call
// deltas with index → EventToolStart + EventToolDelta, finish + usage.
// Matches 03-implementation-reference §3 golden SSE expectations.
//
// 验证 DeepSeek 风格 SSE 的标准 compat 解析路径：
// content delta→EventText；tool-call delta→EventToolStart+EventToolDelta；
// finish+usage 正确。对照 03 §3。
func TestParseStream_StandardCompat_DeepSeek(t *testing.T) {
	fixture := `data: {"choices":[{"delta":{"content":"The weather "},"finish_reason":null}]}

data: {"choices":[{"delta":{"content":"is sunny."},"finish_reason":null}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_w1","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"location\":"}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"San Francisco\"}"}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":20,"completion_tokens":8}}

data: [DONE]
`
	srv := sseServer(fixture)
	defer srv.Close()
	events := collectFromServer(t, srv)

	texts := filterType(events, EventText)
	if len(texts) != 2 {
		t.Fatalf("want 2 EventText, got %d", len(texts))
	}
	if texts[0].Delta != "The weather " || texts[1].Delta != "is sunny." {
		t.Errorf("text deltas = %q %q", texts[0].Delta, texts[1].Delta)
	}

	starts := filterType(events, EventToolStart)
	if len(starts) != 1 {
		t.Fatalf("want 1 EventToolStart, got %d", len(starts))
	}
	if starts[0].ToolName != "get_weather" || starts[0].ToolID != "call_w1" {
		t.Errorf("tool start: name=%q id=%q", starts[0].ToolName, starts[0].ToolID)
	}

	deltas := filterType(events, EventToolDelta)
	if len(deltas) != 2 {
		t.Fatalf("want 2 EventToolDelta, got %d", len(deltas))
	}
	assembled := deltas[0].ArgsDelta + deltas[1].ArgsDelta
	var args map[string]any
	if err := json.Unmarshal([]byte(assembled), &args); err != nil {
		t.Errorf("assembled args not valid JSON: %q err: %v", assembled, err)
	}
	if args["location"] != "San Francisco" {
		t.Errorf("args.location = %v, want San Francisco", args["location"])
	}

	finishes := filterType(events, EventFinish)
	if len(finishes) != 1 {
		t.Fatalf("want 1 EventFinish, got %d", len(finishes))
	}
	if finishes[0].FinishReason != "tool_calls" {
		t.Errorf("finish_reason = %q, want tool_calls", finishes[0].FinishReason)
	}
	if finishes[0].InputTokens != 20 || finishes[0].OutputTokens != 8 {
		t.Errorf("usage: in=%d out=%d, want in=20 out=8",
			finishes[0].InputTokens, finishes[0].OutputTokens)
	}
}

// TestParseStream_DeepSeek_ReasoningContentBeforeContent verifies the
// DeepSeek SSE pattern: delta.reasoning_content deltas → EventReasoning,
// ordered before content deltas → EventText. Matches 03 §3 golden SSE.
//
// 验证 DeepSeek delta.reasoning_content 先于 content 流到；
// delta.reasoning_content→EventReasoning，delta.content→EventText。
func TestParseStream_DeepSeek_ReasoningContentBeforeContent(t *testing.T) {
	fixture := `data: {"choices":[{"delta":{"reasoning_content":"Let me think step by step."},"finish_reason":null}]}

data: {"choices":[{"delta":{"reasoning_content":" First, check the API."},"finish_reason":null}]}

data: {"choices":[{"delta":{"content":"The weather is cloudy."},"finish_reason":"stop"}]}

data: [DONE]
`
	srv := sseServer(fixture)
	defer srv.Close()
	events := collectFromServer(t, srv)

	reasoning := filterType(events, EventReasoning)
	if len(reasoning) != 2 {
		t.Fatalf("want 2 EventReasoning, got %d: %+v", len(reasoning), reasoning)
	}
	if reasoning[0].Delta != "Let me think step by step." {
		t.Errorf("reasoning[0] = %q", reasoning[0].Delta)
	}
	if reasoning[1].Delta != " First, check the API." {
		t.Errorf("reasoning[1] = %q", reasoning[1].Delta)
	}

	texts := filterType(events, EventText)
	if len(texts) != 1 || texts[0].Delta != "The weather is cloudy." {
		t.Errorf("text events = %+v", texts)
	}

	// reasoning indices must precede text indices in the full event list.
	// reasoning 事件必须在 text 事件之前。
	var firstReasoningIdx, firstTextIdx int
	firstReasoningIdx = -1
	firstTextIdx = -1
	for i, ev := range events {
		if ev.Type == EventReasoning && firstReasoningIdx < 0 {
			firstReasoningIdx = i
		}
		if ev.Type == EventText && firstTextIdx < 0 {
			firstTextIdx = i
		}
	}
	if firstReasoningIdx < 0 || firstTextIdx < 0 {
		t.Fatal("missing reasoning or text events")
	}
	if firstReasoningIdx >= firstTextIdx {
		t.Errorf("reasoning events must come before text events; reasoning at %d, text at %d",
			firstReasoningIdx, firstTextIdx)
	}
}

// TestParseStream_Ollama_ReasoningField verifies that Ollama /v1's
// delta.reasoning (no underscore) is correctly mapped to EventReasoning.
// This is the 🔴 gap: the parser must recognise "reasoning" not just
// "reasoning_content". Fixed by adding Reasoning field to oaiDelta.
//
// 验证 Ollama /v1 的 delta.reasoning（无下划线）正确映射到 EventReasoning。
// 这是 🔴 gap：parser 需同时识别 "reasoning" 和 "reasoning_content"。
func TestParseStream_Ollama_ReasoningField(t *testing.T) {
	// Ollama /v1 SSE uses "reasoning" (not "reasoning_content").
	// Ollama /v1 SSE 用 "reasoning" 而非 "reasoning_content"。
	fixture := `data: {"choices":[{"delta":{"reasoning":"I should check the weather API first."},"finish_reason":null}]}

data: {"choices":[{"delta":{"content":"The weather in Toronto is rainy."},"finish_reason":"stop"}]}

data: [DONE]
`
	srv := sseServer(fixture)
	defer srv.Close()
	events := collectFromServer(t, srv)

	reasoning := filterType(events, EventReasoning)
	if len(reasoning) != 1 {
		t.Fatalf("want 1 EventReasoning for Ollama 'reasoning' field, got %d: %+v", len(reasoning), reasoning)
	}
	if reasoning[0].Delta != "I should check the weather API first." {
		t.Errorf("reasoning delta = %q", reasoning[0].Delta)
	}

	texts := filterType(events, EventText)
	if len(texts) != 1 || texts[0].Delta != "The weather in Toronto is rainy." {
		t.Errorf("text events = %+v", texts)
	}
}

// TestParseStream_Ollama_ReasoningNonStreaming verifies the non-streaming path
// also handles Ollama's "reasoning" field (used when DisableStream=true+tools).
//
// 验证非流式路径（DisableStream=true+tools 场景）也能处理 Ollama "reasoning" 字段。
func TestParseStream_Ollama_ReasoningNonStreaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "Here is the answer.",
					"reasoning": "Thinking about this carefully."
				},
				"finish_reason": "stop"
			}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5}
		}`)
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("http.Get: %v", err)
	}

	var events []StreamEvent
	// DisableStream=true routes through parseOpenAINonStreaming.
	// DisableStream=true 走 parseOpenAINonStreaming。
	parseOpenAINonStreaming(resp.Body, func(ev StreamEvent) bool {
		events = append(events, ev)
		return true
	})

	reasoning := filterType(events, EventReasoning)
	if len(reasoning) != 1 || reasoning[0].Delta != "Thinking about this carefully." {
		t.Errorf("reasoning events = %+v", reasoning)
	}
	texts := filterType(events, EventText)
	if len(texts) != 1 || texts[0].Delta != "Here is the answer." {
		t.Errorf("text events = %+v", texts)
	}
}

// TestParseStream_Qwen_FlatErrorEnvelope verifies that Qwen DashScope's flat
// error format {"code":"...","message":"...","request_id":"..."} arriving as
// an SSE chunk maps to EventError wrapping ErrProviderError.
// This is the 🔴 gap: the parser must detect code/message at top level when
// no nested "error" object is present. Fixed by adding Code/ErrMsg fields to
// oaiChunk and detecting them in emitOpenAIChunk.
//
// 验证 Qwen 扁平错误信封（顶层 code/message/request_id）以 SSE chunk 返回时
// 正确映射为 EventError（含 ErrProviderError sentinel）。
// 🔴 gap：需在 oaiChunk 加 code/message 字段并在 emitOpenAIChunk 检测。
func TestParseStream_Qwen_FlatErrorEnvelope(t *testing.T) {
	// Qwen DashScope sends errors as top-level JSON when a parameter is invalid.
	// 参数无效时 Qwen DashScope 将 error 以顶层字段形式返回。
	fixture := `data: {"code":"InvalidParameter","message":"enable_thinking must be set to false for non-streaming calls","request_id":"req-abc123"}

`
	events := collectEvents(fixture)

	errEvents := filterType(events, EventError)
	if len(errEvents) != 1 {
		t.Fatalf("want 1 EventError for Qwen flat error, got %d: %+v", len(errEvents), events)
	}
	ev := errEvents[0]
	if ev.Err == nil {
		t.Fatal("EventError.Err must not be nil")
	}
	if !errors.Is(ev.Err, ErrProviderError) {
		t.Errorf("error must wrap ErrProviderError; got: %v", ev.Err)
	}
	if !strings.Contains(ev.Err.Error(), "InvalidParameter") {
		t.Errorf("error should contain code 'InvalidParameter'; got: %v", ev.Err)
	}
	if !strings.Contains(ev.Err.Error(), "enable_thinking") {
		t.Errorf("error should contain the message; got: %v", ev.Err)
	}
}

// TestParseStream_Qwen_FlatError_NoSilentTermination ensures that a Qwen flat
// error chunk does not silently drop through as an empty choices chunk (the
// pre-fix behaviour where Code was ignored).
//
// 确保 Qwen 扁平错误不会被静默忽略（修复前：无 choices → return true，无 EventError）。
func TestParseStream_Qwen_FlatError_NoSilentTermination(t *testing.T) {
	// If the parser silently ignores the flat error the stream terminates with
	// no events at all — assert we get at least one EventError.
	// 如果 parser 静默忽略扁平错误，stream 以零事件终止——断言至少一个 EventError。
	fixture := `data: {"code":"Throttling.RateQuota","message":"Requests rate limit exceeded","request_id":"rq-123"}

`
	events := collectEvents(fixture)
	if len(filterType(events, EventError)) == 0 {
		t.Errorf("Qwen flat error must yield EventError; events: %+v", events)
	}
}

// TestParseStream_OpenRouter_CommentLineskip verifies that OpenRouter's SSE
// keep-alive comment lines (": OPENROUTER PROCESSING") are skipped without
// breaking parsing. Already handled by the "data: " prefix filter; this test
// pins the behaviour.
//
// 验证 OpenRouter SSE 心跳注释行（": OPENROUTER PROCESSING"）被跳过，
// 不影响后续 data 行的解析。已由 "data: " 前缀过滤保证；此测试锁定该行为。
func TestParseStream_OpenRouter_CommentLineskip(t *testing.T) {
	fixture := `: OPENROUTER PROCESSING

data: {"choices":[{"delta":{"content":"Hello "},"finish_reason":null}]}

: OPENROUTER PROCESSING

data: {"choices":[{"delta":{"content":"world"},"finish_reason":"stop"}]}

data: [DONE]
`
	srv := sseServer(fixture)
	defer srv.Close()
	events := collectFromServer(t, srv)

	errEvents := filterType(events, EventError)
	if len(errEvents) != 0 {
		t.Errorf("no errors expected; got: %+v", errEvents)
	}
	texts := filterType(events, EventText)
	if len(texts) != 2 {
		t.Fatalf("want 2 EventText, got %d", len(texts))
	}
	if texts[0].Delta != "Hello " || texts[1].Delta != "world" {
		t.Errorf("text deltas = %q %q", texts[0].Delta, texts[1].Delta)
	}
	finishes := filterType(events, EventFinish)
	if len(finishes) != 1 || finishes[0].FinishReason != "stop" {
		t.Errorf("finish events = %+v", finishes)
	}
}

// TestParseStream_ReasoningBeforeContent_InSameChunk verifies that when both
// reasoning_content and content appear in a single delta chunk, reasoning is
// emitted first. This guards the ordering fix in emitOpenAIChunk.
//
// 验证同一 delta chunk 中 reasoning_content 和 content 并存时 reasoning 先 emit。
func TestParseStream_ReasoningBeforeContent_InSameChunk(t *testing.T) {
	// Synthetic chunk with both fields present simultaneously.
	// 合成的同时含两字段的 chunk。
	fixture := `data: {"choices":[{"delta":{"reasoning_content":"thinking","content":"answer"},"finish_reason":"stop"}]}

data: [DONE]
`
	events := collectEvents(fixture)

	var firstRIdx, firstTIdx int
	firstRIdx = -1
	firstTIdx = -1
	for i, ev := range events {
		if ev.Type == EventReasoning && firstRIdx < 0 {
			firstRIdx = i
		}
		if ev.Type == EventText && firstTIdx < 0 {
			firstTIdx = i
		}
	}
	if firstRIdx < 0 {
		t.Fatal("no EventReasoning emitted")
	}
	if firstTIdx < 0 {
		t.Fatal("no EventText emitted")
	}
	if firstRIdx >= firstTIdx {
		t.Errorf("reasoning must precede text in same-chunk case; reasoning@%d text@%d",
			firstRIdx, firstTIdx)
	}
}

// TestParseStream_Httptest_FullRoundtrip uses an httptest.Server returning a
// multi-event OpenRouter-style SSE fixture (comment lines + content + usage
// chunk) and asserts the full event sequence via ParseStream — exercising the
// real HTTP round-trip path including doRequest + provider.ParseStream.
//
// 用 httptest.Server 返回完整 OpenRouter 风格 SSE（注释行+content+usage chunk），
// 通过真实 HTTP 往返（doRequest+ParseStream）验证完整事件序列。
func TestParseStream_Httptest_FullRoundtrip(t *testing.T) {
	fixture := `: OPENROUTER PROCESSING

data: {"choices":[{"delta":{"reasoning_content":"step 1"},"finish_reason":null}]}

: OPENROUTER PROCESSING

data: {"choices":[{"delta":{"content":"2+2=4"},"finish_reason":"stop"}]}

data: {"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3}}

data: [DONE]
`
	srv := sseServer(fixture)
	defer srv.Close()
	events := collectFromServer(t, srv)

	reasoning := filterType(events, EventReasoning)
	if len(reasoning) != 1 || reasoning[0].Delta != "step 1" {
		t.Errorf("reasoning = %+v", reasoning)
	}
	texts := filterType(events, EventText)
	if len(texts) != 1 || texts[0].Delta != "2+2=4" {
		t.Errorf("text = %+v", texts)
	}
	errEvents := filterType(events, EventError)
	if len(errEvents) != 0 {
		t.Errorf("no errors expected; got %+v", errEvents)
	}
	finishes := filterType(events, EventFinish)
	// expect at least one finish with usage.
	// 至少一个带 usage 的 finish 事件。
	foundUsage := false
	for _, f := range finishes {
		if f.InputTokens == 7 && f.OutputTokens == 3 {
			foundUsage = true
		}
	}
	if !foundUsage {
		t.Errorf("expected EventFinish with usage tokens 7/3; finishes: %+v", finishes)
	}
}

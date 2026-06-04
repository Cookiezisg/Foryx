package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestOllamaBuildRequest(t *testing.T) {
	p := newOllamaProvider()
	req := Request{
		ModelID:  "qwen3",
		Key:      "ollama-key",
		BaseURL:  "http://localhost:11434/v1",
		System:   "you are helpful",
		Messages: []LLMMessage{{Role: RoleUser, Content: "hi"}},
		Options:  map[string]string{"think": "high"},
	}
	httpReq, err := p.BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if httpReq.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", httpReq.Method)
	}
	if got := httpReq.URL.String(); got != "http://localhost:11434/v1/chat/completions" {
		t.Errorf("url = %s", got)
	}
	if got := httpReq.Header.Get("Authorization"); got != "Bearer ollama-key" {
		t.Errorf("auth = %q", got)
	}
	body, _ := io.ReadAll(httpReq.Body)
	var ol ollamaRequest
	if err := json.Unmarshal(body, &ol); err != nil {
		t.Fatal(err)
	}
	if ol.Model != "qwen3" || !ol.Stream {
		t.Errorf("model=%s stream=%v", ol.Model, ol.Stream)
	}
	// GPT-OSS effort string passes through verbatim into the top-level "think" field.
	// GPT-OSS effort 串原样进顶层 "think" 字段。
	if ol.Think != "high" {
		t.Errorf("think = %v, want high", ol.Think)
	}
	if len(ol.Messages) != 2 || ol.Messages[0].Role != "system" || ol.Messages[1].Role != "user" {
		t.Errorf("messages = %+v", ol.Messages)
	}
}

// TestOllamaBuildRequestThinkAndOptions verifies the native knobs: top-level "think" is a
// bool for most models ("true"/"false") and an effort string for GPT-OSS ("low"/.../"high"),
// num_ctx lands under options.num_ctx, and MaxTokens maps to options.num_predict. Missing keys
// are omitted entirely.
//
// 验证原生旋钮：顶层 "think" 多数 model 是 bool（"true"/"false"），GPT-OSS 是 effort 串；
// num_ctx 落 options.num_ctx，MaxTokens 映射 options.num_predict。缺省 key 整字段省略。
func TestOllamaBuildRequestThinkAndOptions(t *testing.T) {
	p := newOllamaProvider()
	base := Request{ModelID: "qwen3", Key: "k", BaseURL: "http://x"}
	encode := func(req Request) ollamaRequest {
		httpReq, err := p.BuildRequest(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(httpReq.Body)
		var ol ollamaRequest
		_ = json.Unmarshal(body, &ol)
		return ol
	}

	// No Options, no MaxTokens → think omitted, options omitted.
	// 无 Options、无 MaxTokens → think 省略、options 省略。
	if ol := encode(base); ol.Think != nil || ol.Options != nil {
		t.Errorf("no knobs → think=%v options=%v, want both omitted", ol.Think, ol.Options)
	}

	// think:"true" → top-level bool true.
	// think:"true" → 顶层 bool true。
	base.Options = map[string]string{"think": "true"}
	if ol := encode(base); ol.Think != true {
		t.Errorf("think:true → %v (%T), want bool true", ol.Think, ol.Think)
	}

	// think:"false" → top-level bool false (present, not omitted: omitempty drops false, so it
	// will be absent on the wire; decoding yields nil). We assert it is not the string "false".
	// think:"false" → 顶层 bool false（omitempty 会丢 false，线缆缺省、解码得 nil）；断言不是字符串。
	base.Options = map[string]string{"think": "false"}
	if ol := encode(base); ol.Think == "false" {
		t.Errorf("think:false → %v, want bool false (not string)", ol.Think)
	}

	// think GPT-OSS effort string → passes through verbatim.
	// think GPT-OSS effort 串 → 原样透传。
	base.Options = map[string]string{"think": "medium"}
	if ol := encode(base); ol.Think != "medium" {
		t.Errorf("think:medium → %v, want medium", ol.Think)
	}

	// num_ctx → options.num_ctx (JSON number decodes into any as float64).
	// num_ctx → options.num_ctx（JSON 数字解码进 any 为 float64）。
	base.Options = map[string]string{"num_ctx": "8192"}
	if ol := encode(base); ol.Options == nil || ol.Options["num_ctx"] != float64(8192) {
		t.Errorf("num_ctx → options=%v, want num_ctx=8192", ol.Options)
	}

	// MaxTokens → options.num_predict.
	// MaxTokens → options.num_predict。
	base.Options = nil
	base.MaxTokens = 512
	if ol := encode(base); ol.Options == nil || ol.Options["num_predict"] != float64(512) {
		t.Errorf("MaxTokens → options=%v, want num_predict=512", ol.Options)
	}
}

// TestOllamaBuildRequestToolsForceNonStream verifies the Ollama quirk: any request with
// tools is forced non-streaming (stream:false) because Ollama drops tool_calls when
// streaming.
//
// 验证 Ollama 怪癖：带 tools 的请求强制非流式（stream:false），因为 Ollama streaming 会吞 tool_calls。
func TestOllamaBuildRequestToolsForceNonStream(t *testing.T) {
	p := newOllamaProvider()
	req := Request{
		ModelID:  "qwen3",
		Key:      "k",
		BaseURL:  "http://x",
		Messages: []LLMMessage{{Role: RoleUser, Content: "hi"}},
		Tools:    []ToolDef{{Name: "get_weather", Description: "d", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}
	httpReq, err := p.BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(httpReq.Body)
	var ol ollamaRequest
	if err := json.Unmarshal(body, &ol); err != nil {
		t.Fatal(err)
	}
	if ol.Stream {
		t.Errorf("stream = true, want false when tools present")
	}
	if ol.StreamOptions != nil {
		t.Errorf("stream_options = %+v, want nil when non-streaming", ol.StreamOptions)
	}
	if len(ol.Tools) != 1 || ol.Tools[0].Function.Name != "get_weather" {
		t.Errorf("tools = %+v", ol.Tools)
	}
}

func TestOllamaParseStream(t *testing.T) {
	p := newOllamaProvider()
	resp := &http.Response{Body: sseBody(
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

// TestOllamaParseNonStreaming feeds a single non-streaming JSON response (the path taken
// when tools are present) and verifies reasoning/text/tool_start/finish synthesis. Ollama's
// non-streaming message carries thinking in "reasoning" (no underscore).
//
// 喂单条非流式 JSON 响应（有 tools 时走此路径），验 reasoning/text/tool_start/finish 合成。
// Ollama 非流式 message 用 "reasoning"（无下划线）传思考。
func TestOllamaParseNonStreaming(t *testing.T) {
	p := newOllamaProvider()
	body := `{"choices":[{"message":{"role":"assistant","reasoning":"hmm","content":"done","tool_calls":[{"id":"call_1","function":{"name":"f","arguments":"{\"q\":1}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":1}}`
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
	events := collect(p.ParseStream(context.Background(), resp, Request{DisableStream: true}))

	var text, reasoning strings.Builder
	var sawToolStart, sawToolDelta, sawFinish bool
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
		case EventToolDelta:
			sawToolDelta = true
			if ev.ArgsDelta != `{"q":1}` {
				t.Errorf("tool_delta args = %q", ev.ArgsDelta)
			}
		case EventFinish:
			sawFinish = true
			if ev.FinishReason != "tool_calls" || ev.InputTokens != 5 || ev.OutputTokens != 1 {
				t.Errorf("finish = %+v", ev)
			}
		case EventError:
			t.Fatalf("unexpected error event: %v", ev.Err)
		}
	}
	if reasoning.String() != "hmm" {
		t.Errorf("reasoning = %q, want hmm", reasoning.String())
	}
	if text.String() != "done" {
		t.Errorf("text = %q, want done", text.String())
	}
	if !sawToolStart || !sawToolDelta || !sawFinish {
		t.Errorf("missing events: toolStart=%v toolDelta=%v finish=%v", sawToolStart, sawToolDelta, sawFinish)
	}
}

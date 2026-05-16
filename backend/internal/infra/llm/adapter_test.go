package llm

import (
	"context"
	"iter"
	"testing"
)

func TestAdapter_LookupKnownProviders(t *testing.T) {
	cases := []struct {
		name      string
		provider  string
		wantBase  string
		wantNoBase bool // true means BaseURL must be empty (caller required to supply)
	}{
		{"openai",     "openai",     "https://api.openai.com/v1", false},
		{"anthropic",  "anthropic",  "https://api.anthropic.com", false},
		{"gemini",     "google",     "https://generativelanguage.googleapis.com/v1beta/openai", false},
		{"deepseek",   "deepseek",   "https://api.deepseek.com", false},
		{"openrouter", "openrouter", "https://openrouter.ai/api/v1", false},
		{"qwen",       "qwen",       "https://dashscope.aliyuncs.com/compatible-mode/v1", false},
		{"zhipu",      "zhipu",      "https://open.bigmodel.cn/api/paas/v4", false},
		{"moonshot",   "moonshot",   "https://api.moonshot.cn/v1", false},
		{"doubao",     "doubao",     "https://ark.cn-beijing.volces.com/api/v3", false},
		{"ollama",     "ollama",     "", true},
		{"custom",     "custom",     "", true},
		{"mock",       "mock",       "mock://in-process", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := lookupAdapter(tc.provider)
			if a.Name() != tc.provider {
				t.Errorf("Name() = %q, want %q", a.Name(), tc.provider)
			}
			got := a.DefaultBaseURL()
			if tc.wantNoBase {
				if got != "" {
					t.Errorf("DefaultBaseURL() = %q, want empty (caller must supply)", got)
				}
				return
			}
			if got != tc.wantBase {
				t.Errorf("DefaultBaseURL() = %q, want %q", got, tc.wantBase)
			}
		})
	}
}

func TestAdapter_UnknownProviderFallsBackToOpenAI(t *testing.T) {
	a := lookupAdapter("not-a-real-provider")
	if a.Name() != "openai" {
		t.Errorf("unknown provider should fall back to openaiAdapter, got %q", a.Name())
	}
}

func TestAdapter_AllRegisteredHaveStableNames(t *testing.T) {
	wantNames := []string{
		"openai", "anthropic", "google", "deepseek", "openrouter",
		"qwen", "zhipu", "moonshot", "doubao", "ollama", "custom", "mock",
	}
	if len(adapters) != len(wantNames) {
		t.Fatalf("adapters len = %d, want %d", len(adapters), len(wantNames))
	}
	for i, want := range wantNames {
		if adapters[i].Name() != want {
			t.Errorf("adapters[%d].Name() = %q, want %q", i, adapters[i].Name(), want)
		}
	}
}

type fakeWireClient struct {
	gotReq Request
	emit   []StreamEvent
}

func (c *fakeWireClient) Stream(ctx context.Context, req Request) iter.Seq[StreamEvent] {
	c.gotReq = req
	return func(yield func(StreamEvent) bool) {
		for _, e := range c.emit {
			if !yield(e) {
				return
			}
		}
	}
}

func TestAdapterWrappedClient_HooksInvoked(t *testing.T) {
	fake := &fakeWireClient{
		emit: []StreamEvent{
			{Type: EventText, Delta: "a"},
			{Type: EventText, Delta: "b"},
		},
	}
	wrapped := &adapterWrappedClient{
		inner: fake,
		adapter: spyAdapter{
			before: func(r *Request) { r.System = "MUTATED" },
			after: func(ev StreamEvent) []StreamEvent {
				return []StreamEvent{ev, {Type: EventText, Delta: ev.Delta + "_dup"}}
			},
		},
	}
	var got []StreamEvent
	for ev := range wrapped.Stream(context.Background(), Request{System: "ORIGINAL"}) {
		got = append(got, ev)
	}
	if fake.gotReq.System != "MUTATED" {
		t.Errorf("BeforeRequest hook didn't fire; inner client received System=%q", fake.gotReq.System)
	}
	if len(got) != 4 {
		t.Fatalf("AfterStreamEvent fan-out broken; got %d events, want 4", len(got))
	}
	wantDeltas := []string{"a", "a_dup", "b", "b_dup"}
	for i, w := range wantDeltas {
		if got[i].Delta != w {
			t.Errorf("got[%d].Delta = %q, want %q", i, got[i].Delta, w)
		}
	}
}

func TestOllamaAdapter_DisablesStreamWhenToolsPresent(t *testing.T) {
	a := lookupAdapter("ollama")

	r1 := Request{}
	a.BeforeRequest(&r1)
	if r1.DisableStream {
		t.Errorf("Ollama without tools should leave DisableStream=false")
	}

	r2 := Request{Tools: []ToolDef{{Name: "search"}}}
	a.BeforeRequest(&r2)
	if !r2.DisableStream {
		t.Errorf("Ollama with tools should set DisableStream=true (avoids ollama#12557)")
	}
}

func TestNonOllamaAdapters_DontTouchStream(t *testing.T) {
	for _, name := range []string{"openai", "deepseek", "qwen", "moonshot", "google", "anthropic"} {
		a := lookupAdapter(name)
		req := Request{Tools: []ToolDef{{Name: "x"}}}
		a.BeforeRequest(&req)
		if req.DisableStream {
			t.Errorf("%s should not set DisableStream; only Ollama needs that quirk", name)
		}
	}
}

func TestDeepseekAdapter_StripsReasoningOnPlainTurn(t *testing.T) {
	a := lookupAdapter("deepseek")
	req := Request{
		Messages: []LLMMessage{
			{Role: RoleUser, Content: "hi"},
			{
				Role:             RoleAssistant,
				Content:          "hello",
				ReasoningContent: "let me think about how to greet",
			},
			{Role: RoleUser, Content: "next"},
		},
	}
	a.BeforeRequest(&req)
	if req.Messages[1].ReasoningContent != "" {
		t.Errorf("plain assistant turn should have reasoning_content stripped; got %q",
			req.Messages[1].ReasoningContent)
	}
	if req.Messages[1].Content != "hello" {
		t.Errorf("Content should not be touched; got %q", req.Messages[1].Content)
	}
}

func TestDeepseekAdapter_PreservesReasoningOnToolCallTurn(t *testing.T) {
	a := lookupAdapter("deepseek")
	req := Request{
		Messages: []LLMMessage{
			{
				Role:             RoleAssistant,
				ReasoningContent: "I should look this up",
				ToolCalls:        []LLMToolCall{{ID: "c1", Name: "search"}},
			},
		},
	}
	a.BeforeRequest(&req)
	if req.Messages[0].ReasoningContent != "I should look this up" {
		t.Errorf("tool-call turn must preserve reasoning_content (V3.2 requirement); got %q",
			req.Messages[0].ReasoningContent)
	}
}

func TestDeepseekAdapter_DoesntTouchUserOrToolMessages(t *testing.T) {
	a := lookupAdapter("deepseek")
	req := Request{
		Messages: []LLMMessage{
			{Role: RoleUser, Content: "hi", ReasoningContent: "should-not-be-touched"},
			{Role: RoleTool, ToolCallID: "x", Content: "result"},
		},
	}
	a.BeforeRequest(&req)
	if req.Messages[0].ReasoningContent != "should-not-be-touched" {
		t.Errorf("user message reasoning_content should not be modified")
	}
}

type spyAdapter struct {
	baseAdapter
	before func(r *Request)
	after  func(ev StreamEvent) []StreamEvent
}

func (a spyAdapter) Name() string                                  { return "spy" }
func (a spyAdapter) DefaultBaseURL() string                        { return "spy://" }
func (a spyAdapter) BeforeRequest(r *Request)                      { a.before(r) }
func (a spyAdapter) AfterStreamEvent(ev StreamEvent) []StreamEvent { return a.after(ev) }

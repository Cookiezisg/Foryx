// adapter_test.go — unit tests for the per-provider Adapter abstraction.
// No network calls — all assertions are about adapter routing, default
// base URLs, and the wrapping client's hook invocation.
//
// adapter_test.go — Adapter 抽象的单元测试。无网络。

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

// TestAdapter_UnknownProviderFallsBackToOpenAI verifies the fallback for
// typo'd or experimental provider names. Lenient default keeps Forgify
// usable when users plug in a not-yet-registered OpenAI-compat endpoint.
//
// 未知 provider 回落 openaiAdapter 让 Forgify 仍能用未注册的 OpenAI-compat 端点。
func TestAdapter_UnknownProviderFallsBackToOpenAI(t *testing.T) {
	a := lookupAdapter("not-a-real-provider")
	if a.Name() != "openai" {
		t.Errorf("unknown provider should fall back to openaiAdapter, got %q", a.Name())
	}
}

// TestAdapter_BaseURLConsistencyWithApikeyProviders ensures the Adapter's
// default URLs match what apikey/providers.go declares. Drift between the
// two registries was the original motivation for centralizing into Adapter
// — this test guards against re-introducing duplicate sources of truth.
//
// Adapter 默认 URL 与 apikey/providers.go 必须一致——抽象出 Adapter 的
// 初衷就是去重，本测试防回归。
//
// NOTE: this test cross-imports apikey package. If circular-import issues
// arise, move it to a higher-level integration test. For now infra/llm
// is at the bottom of the dep graph, so importing apikey from here would
// be a cycle. Instead we declare the expected URLs inline; the apikey
// package contains a mirror that an integration test compares.
//
// 注：此测试本想跨包 import apikey 验证一致；但 apikey 在 infra/llm 之上
// 会导致循环依赖。这里 inline 期望 URL；apikey 那侧有反向 mirror 由集成
// 测试 cross-check。
func TestAdapter_AllRegisteredHaveStableNames(t *testing.T) {
	// Stable iteration order via slice — assertion-friendly.
	// slice 顺序稳定方便断言。
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

// fakeWireClient lets us assert what the adapter wrapper does without
// hitting real provider clients. Captures inbound BeforeRequest call,
// emits a programmable event sequence to AfterStreamEvent.
//
// fakeWireClient 让我们断言 adapter wrapper 的行为而不调真 client。
// 捕获 BeforeRequest 调用，emit 可编程 event 序列给 AfterStreamEvent。
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

// TestAdapterWrappedClient_HooksInvoked verifies the wrapper actually
// calls BeforeRequest before forwarding and AfterStreamEvent on every
// inbound event. Uses a fake adapter that mutates Request.System and
// fans out each EventText into 2 events.
//
// 验证 wrapper 真的在转发前调 BeforeRequest，对每个入站 event 调
// AfterStreamEvent。用 fake adapter 改 System + 把 EventText 扩成 2 条。
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

// TestOllamaAdapter_DisablesStreamWhenToolsPresent verifies the TE-24 fix
// for Ollama's OpenAI-compat tool-call quirk: when tools are sent with
// streaming on, Ollama silently drops tool_calls. Adapter forces
// DisableStream=true so the OpenAI client falls back to non-streaming
// reception, which returns tool_calls properly.
//
// Ollama 在 streaming+tools 下吞 tool_calls；adapter 自动关流式让其用
// 非流式接收。
func TestOllamaAdapter_DisablesStreamWhenToolsPresent(t *testing.T) {
	a := lookupAdapter("ollama")

	// Without tools: stream stays on.
	// 无 tools：保持流式。
	r1 := Request{}
	a.BeforeRequest(&r1)
	if r1.DisableStream {
		t.Errorf("Ollama without tools should leave DisableStream=false")
	}

	// With tools: stream auto-disabled.
	// 有 tools：自动关流式。
	r2 := Request{Tools: []ToolDef{{Name: "search"}}}
	a.BeforeRequest(&r2)
	if !r2.DisableStream {
		t.Errorf("Ollama with tools should set DisableStream=true (avoids ollama#12557)")
	}
}

// TestNonOllamaAdapters_DontTouchStream verifies other adapters don't
// accidentally force non-streaming. Default behavior must stay streaming.
//
// 其它 adapter 不应误关流式，保持默认流式行为。
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

type spyAdapter struct {
	baseAdapter
	before func(r *Request)
	after  func(ev StreamEvent) []StreamEvent
}

func (a spyAdapter) Name() string                                  { return "spy" }
func (a spyAdapter) DefaultBaseURL() string                        { return "spy://" }
func (a spyAdapter) BeforeRequest(r *Request)                      { a.before(r) }
func (a spyAdapter) AfterStreamEvent(ev StreamEvent) []StreamEvent { return a.after(ev) }

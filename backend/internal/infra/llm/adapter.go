// adapter.go — Provider-specific wire-level adaptation layer.
//
// Each LLM provider Forgify supports has its own quirks: a non-default
// base URL, OpenAI-incompatible parameter ranges, mid-stream error
// formats, etc. This file centralizes them so the OpenAI / Anthropic
// wire clients stay clean and per-provider behavior lives in one place.
//
// What lives HERE (wire-level, provider-specific):
//   - DefaultBaseURL — the canonical base URL for the provider; Adapter
//     replaces the switch that used to live in factory.go::resolveBaseURL,
//     which duplicated the table in apikey/providers.go.
//   - BeforeRequest  — outbound Request mutations (e.g. future temperature
//     clamping for Moonshot, enable_thinking injection for Qwen). Currently
//     mostly no-ops because Request struct is intentionally minimal; the
//     hook is here so future Request field additions don't break the
//     contract.
//   - AfterStreamEvent — incoming StreamEvent transformation (e.g. fan
//     out / drop / rewrite). Currently no-ops; reserved for provider-
//     specific event-level fixups when they appear.
//
// What does NOT live here:
//   - OpenAI-compat protocol baseline compliance (assistant content !=
//     null, reasoning-only fallback, mid-stream SSE error detection,
//     tool_call.index synthesis). Those live in openai.go because they
//     apply to every provider that speaks OpenAI-compat, not specific
//     providers.
//   - Anthropic-protocol baseline compliance (thinking-block round-trip).
//     Lives in anthropic.go for the same reason.
//   - User-facing provider configuration (display name, TestMethod,
//     BaseURLRequired). Lives in apikey/providers.go because that's app-
//     layer semantics, not wire concerns.
//
// adapter.go ——provider 级 wire 适配层。
//
// 每家 LLM provider 都有自己的怪癖：默认 base URL、OpenAI 不兼容的参数范围、
// 流中错误格式等。本文集中处理，OpenAI / Anthropic wire 客户端保持干净，
// 每家 provider 行为聚于一处。
//
// 这里管：DefaultBaseURL（替代 factory.go::resolveBaseURL 的 switch；
// 之前与 apikey/providers.go 表重复）；BeforeRequest（出站 Request 修改）；
// AfterStreamEvent（入站 StreamEvent 转换）。
//
// 这里不管：OpenAI/Anthropic 协议基线合规（在各自 client 内）；user-facing
// provider 配置（在 apikey/providers.go）。

package llm

import (
	"context"
	"iter"
)

// Adapter encapsulates provider-specific wire-level behavior on top of
// the OpenAI-compat or Anthropic-native baseline clients. One instance
// per provider id; lookup via lookupAdapter(name).
//
// Adapter 在 OpenAI-compat / Anthropic-native baseline 之上封装 provider 级
// wire 行为。每个 provider id 一个实例；用 lookupAdapter(name) 查找。
type Adapter interface {
	// Name returns the provider id (matches apikey.providers map key).
	Name() string

	// DefaultBaseURL returns the canonical base URL for this provider.
	// Replaces factory.go::resolveBaseURL switch. Empty string ("") means
	// "no default — caller must supply BaseURL explicitly" (used by
	// ollama / custom / mock).
	DefaultBaseURL() string

	// BeforeRequest mutates the outgoing Request before the wire client
	// sends it. Most adapters are no-ops today because Request is
	// intentionally minimal; reserved for future provider-specific
	// adjustments (Moonshot temperature clamping, Qwen enable_thinking,
	// etc.) once Request grows the relevant fields.
	BeforeRequest(req *Request)

	// AfterStreamEvent inspects/transforms incoming StreamEvents. Returning
	// nil drops the event; returning multiple events fans out. Currently
	// no-ops; reserved for provider-specific event fixups (e.g. dropping
	// keep-alive comments encoded as malformed events).
	AfterStreamEvent(ev StreamEvent) []StreamEvent
}

// baseAdapter provides no-op implementations of BeforeRequest and
// AfterStreamEvent so concrete adapters only override what they actually
// change. Embed this struct in any new adapter.
//
// baseAdapter 提供 BeforeRequest / AfterStreamEvent 的 no-op 默认实现，
// 具体 adapter 嵌入后只覆盖真正改变的方法。
type baseAdapter struct{}

func (baseAdapter) BeforeRequest(*Request)                            {}
func (baseAdapter) AfterStreamEvent(ev StreamEvent) []StreamEvent     { return []StreamEvent{ev} }

// ── Concrete adapters ────────────────────────────────────────────────────────
//
// One small struct per provider. Most are baseURL-only today; behavior
// hooks become more interesting as Request gains parameters or new
// quirks emerge.
//
// 每个 provider 一个小 struct。当前多数仅设 baseURL；待 Request 增字段或
// 新 quirk 出现，行为 hook 才有更多用武之地。

type openaiAdapter struct{ baseAdapter }

func (openaiAdapter) Name() string           { return "openai" }
func (openaiAdapter) DefaultBaseURL() string { return "https://api.openai.com/v1" }

type anthropicAdapter struct{ baseAdapter }

func (anthropicAdapter) Name() string { return "anthropic" }

// Anthropic client appends /v1/messages itself; the base URL is just the host.
// Anthropic 客户端自行拼接 /v1/messages，base URL 只给 host。
func (anthropicAdapter) DefaultBaseURL() string { return "https://api.anthropic.com" }

type geminiAdapter struct{ baseAdapter }

func (geminiAdapter) Name() string { return "google" }

// Gemini exposes its OpenAI-compat surface at /v1beta/openai/, NOT at the
// root. Without the path, the OpenAI client appends /chat/completions to
// https://generativelanguage.googleapis.com and gets 404 on every call.
// This was the #5 P0 in the multi-provider audit.
//
// Gemini 的 OpenAI-compat 端点在 /v1beta/openai/ 而非根路径。不加路径
// OpenAI 客户端会把 /chat/completions 拼到根 → 每次都 404。多 provider
// 自检中的 P0 #5。
func (geminiAdapter) DefaultBaseURL() string {
	return "https://generativelanguage.googleapis.com/v1beta/openai"
}

type deepseekAdapter struct{ baseAdapter }

func (deepseekAdapter) Name() string           { return "deepseek" }
func (deepseekAdapter) DefaultBaseURL() string { return "https://api.deepseek.com" }

type openrouterAdapter struct{ baseAdapter }

func (openrouterAdapter) Name() string           { return "openrouter" }
func (openrouterAdapter) DefaultBaseURL() string { return "https://openrouter.ai/api/v1" }

type qwenAdapter struct{ baseAdapter }

func (qwenAdapter) Name() string { return "qwen" }
func (qwenAdapter) DefaultBaseURL() string {
	return "https://dashscope.aliyuncs.com/compatible-mode/v1"
}

type zhipuAdapter struct{ baseAdapter }

func (zhipuAdapter) Name() string           { return "zhipu" }
func (zhipuAdapter) DefaultBaseURL() string { return "https://open.bigmodel.cn/api/paas/v4" }

type moonshotAdapter struct{ baseAdapter }

func (moonshotAdapter) Name() string           { return "moonshot" }
func (moonshotAdapter) DefaultBaseURL() string { return "https://api.moonshot.cn/v1" }

type doubaoAdapter struct{ baseAdapter }

func (doubaoAdapter) Name() string           { return "doubao" }
func (doubaoAdapter) DefaultBaseURL() string { return "https://ark.cn-beijing.volces.com/api/v3" }

type ollamaAdapter struct{ baseAdapter }

func (ollamaAdapter) Name() string           { return "ollama" }
func (ollamaAdapter) DefaultBaseURL() string { return "" } // BaseURLRequired in apikey/providers.go

// BeforeRequest forces non-streaming when tools are present. Ollama's
// OpenAI-compat path (/v1/chat/completions) silently drops tool_calls
// when streaming is on (open issues ollama #12557, #9632, #7881). The
// model emits the tool call internally but the SSE chunks have no
// tool_calls field — content shows empty with finish_reason=stop. The
// only reliable workaround on the OpenAI-compat surface is to ask the
// non-streaming endpoint, which returns tool_calls properly. Tool-less
// turns (text generation only) keep streaming for the usual UX.
//
// Ollama OpenAI-compat 端点在 streaming + tools 下静默吞 tool_calls
// （ollama #12557 / #9632 / #7881）。模型内部产生工具调用但 SSE chunk 无
// tool_calls 字段，content 空 + finish_reason=stop。OpenAI-compat 唯一可靠
// 解法是切非流式。无 tools 的 turn 仍流式保 UX。
func (ollamaAdapter) BeforeRequest(req *Request) {
	if len(req.Tools) > 0 {
		req.DisableStream = true
	}
}

type customAdapter struct{ baseAdapter }

func (customAdapter) Name() string           { return "custom" }
func (customAdapter) DefaultBaseURL() string { return "" } // BaseURLRequired

type mockAdapter struct{ baseAdapter }

func (mockAdapter) Name() string           { return "mock" }
func (mockAdapter) DefaultBaseURL() string { return "mock://in-process" }

// ── Registry ─────────────────────────────────────────────────────────────────

// adapters is the in-memory registry. Adding a new provider = one entry
// here + the matching ProviderMeta in apikey/providers.go. Slice (not
// map) so iteration order is stable for tests.
//
// adapters 是内存注册表。新增 provider = 这里加一行 + apikey/providers.go
// 加对应 ProviderMeta。用 slice 不用 map，迭代顺序稳定方便测试。
var adapters = []Adapter{
	openaiAdapter{},
	anthropicAdapter{},
	geminiAdapter{},
	deepseekAdapter{},
	openrouterAdapter{},
	qwenAdapter{},
	zhipuAdapter{},
	moonshotAdapter{},
	doubaoAdapter{},
	ollamaAdapter{},
	customAdapter{},
	mockAdapter{},
}

// lookupAdapter returns the adapter for the given provider name. Unknown
// providers fall back to openaiAdapter (most lenient OpenAI-compat
// baseline) — this keeps Forgify functional when a user types a typo
// or experiments with a new OpenAI-compat provider not yet in the list.
//
// lookupAdapter 按 provider name 返回 adapter。未知 provider 回落 openaiAdapter
// （最宽容的 OpenAI-compat 基线），让用户拼错或实验新 provider 时仍能用。
func lookupAdapter(name string) Adapter {
	for _, a := range adapters {
		if a.Name() == name {
			return a
		}
	}
	return openaiAdapter{}
}

// ── Wrapping client ──────────────────────────────────────────────────────────

// adapterWrappedClient applies an Adapter's BeforeRequest + AfterStreamEvent
// hooks around any underlying Client. Factory.Build wraps every client
// in this layer so adapter hooks always run, even for providers that
// share a wire client (most go through openaiClient).
//
// adapterWrappedClient 在底层 Client 外包一层 Adapter 的钩子。Factory.Build
// 给每个 client 都包一层，让钩子总是触发——即使多个 provider 共享同一 wire
// client（多数走 openaiClient）。
type adapterWrappedClient struct {
	inner   Client
	adapter Adapter
}

func (c *adapterWrappedClient) Stream(ctx context.Context, req Request) iter.Seq[StreamEvent] {
	c.adapter.BeforeRequest(&req)
	innerSeq := c.inner.Stream(ctx, req)
	return func(yield func(StreamEvent) bool) {
		for ev := range innerSeq {
			for _, transformed := range c.adapter.AfterStreamEvent(ev) {
				if !yield(transformed) {
					return
				}
			}
		}
	}
}

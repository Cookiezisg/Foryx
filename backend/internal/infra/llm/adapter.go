package llm

import (
	"context"
	"iter"
)

// Adapter encapsulates provider-specific wire-level behavior on top of baseline clients.
//
// Adapter 在 baseline wire client 之上封装 provider 级行为。
type Adapter interface {
	Name() string
	DefaultBaseURL() string
	BeforeRequest(req *Request)
	AfterStreamEvent(ev StreamEvent) []StreamEvent
}

type baseAdapter struct{}

func (baseAdapter) BeforeRequest(*Request)                        {}
func (baseAdapter) AfterStreamEvent(ev StreamEvent) []StreamEvent { return []StreamEvent{ev} }

type openaiAdapter struct{ baseAdapter }

func (openaiAdapter) Name() string           { return "openai" }
func (openaiAdapter) DefaultBaseURL() string { return "https://api.openai.com/v1" }

type anthropicAdapter struct{ baseAdapter }

func (anthropicAdapter) Name() string           { return "anthropic" }
func (anthropicAdapter) DefaultBaseURL() string { return "https://api.anthropic.com" }

type geminiAdapter struct{ baseAdapter }

func (geminiAdapter) Name() string { return "google" }

// DefaultBaseURL — Gemini's OpenAI-compat surface lives at /v1beta/openai/, not root.
// DefaultBaseURL —— Gemini 的 OpenAI-compat 端点在 /v1beta/openai/，非根路径。
func (geminiAdapter) DefaultBaseURL() string {
	return "https://generativelanguage.googleapis.com/v1beta/openai"
}

type deepseekAdapter struct{ baseAdapter }

func (deepseekAdapter) Name() string           { return "deepseek" }
func (deepseekAdapter) DefaultBaseURL() string { return "https://api.deepseek.com" }

// BeforeRequest enforces DeepSeek's turn-type-dependent reasoning_content round-trip rule.
//
// BeforeRequest 守 DeepSeek 按 turn 类型的 reasoning_content round-trip 规则。
func (deepseekAdapter) BeforeRequest(req *Request) {
	for i := range req.Messages {
		m := &req.Messages[i]
		if m.Role != RoleAssistant {
			continue
		}
		// Plain assistant turn: strip; tool-call turn: preserve (V3.2+ requires it).
		// 纯文字 turn 剥；含 tool_calls turn 保留（V3.2+ 必须）。
		if len(m.ToolCalls) == 0 {
			m.ReasoningContent = ""
		}
	}
}

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
func (ollamaAdapter) DefaultBaseURL() string { return "" }

// BeforeRequest forces non-streaming when tools are present (Ollama drops tool_calls when streaming).
//
// BeforeRequest 有 tools 时强制非流式（Ollama streaming 时会吞 tool_calls）。
func (ollamaAdapter) BeforeRequest(req *Request) {
	if len(req.Tools) > 0 {
		req.DisableStream = true
	}
}

type customAdapter struct{ baseAdapter }

func (customAdapter) Name() string           { return "custom" }
func (customAdapter) DefaultBaseURL() string { return "" }

type mockAdapter struct{ baseAdapter }

func (mockAdapter) Name() string           { return "mock" }
func (mockAdapter) DefaultBaseURL() string { return "mock://in-process" }

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

// lookupAdapter returns the named Adapter; unknown providers fall back to openaiAdapter.
//
// lookupAdapter 按 name 返 Adapter；未知 provider 回落到 openaiAdapter。
func lookupAdapter(name string) Adapter {
	for _, a := range adapters {
		if a.Name() == name {
			return a
		}
	}
	return openaiAdapter{}
}

// adapterWrappedClient applies Adapter hooks around an inner Client.
//
// adapterWrappedClient 在内部 Client 外包一层 Adapter 钩子。
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

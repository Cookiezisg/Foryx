package llm

import (
	"context"
	"iter"
	"net/http"
)

// Provider is one LLM wire dialect: it owns how a Request becomes an HTTP
// request (body shape, auth headers, base-url + path) and how the response
// becomes the typed StreamEvent stream. Identity (Name / DefaultBaseURL)
// drives registry lookup and base-url resolution. Thinking-encoding will
// land inside BuildRequest in P3.
//
// Provider 是一种 LLM wire 方言：负责 Request→HTTP 请求（body 形状、auth
// 头、base-url+path）与响应→StreamEvent 流。Name / DefaultBaseURL 供注册表
// 查找与 base-url 解析。P3 的 thinking 编码落在 BuildRequest 内。
type Provider interface {
	Name() string
	DefaultBaseURL() string
	BuildRequest(ctx context.Context, req Request) (*http.Request, error)
	ParseStream(ctx context.Context, resp *http.Response, req Request) iter.Seq[StreamEvent]
}

// providerClient adapts a Provider to the Client contract by running the shared
// transport iron-law (build → do → status-map → parse). It is the single copy
// of the request/response plumbing every Provider funnels through, so callers
// keep seeing the unchanged Client.Stream interface.
//
// providerClient 把 Provider 适配成 Client：跑共享传输铁律（build → do →
// status-map → parse）。它是所有 Provider 共用的唯一请求/响应管道，故调用方
// 看到的 Client.Stream 契约不变。
type providerClient struct {
	provider Provider
	http     *http.Client
}

func (c *providerClient) Stream(ctx context.Context, req Request) iter.Seq[StreamEvent] {
	return func(yield func(StreamEvent) bool) {
		httpReq, err := c.provider.BuildRequest(ctx, req)
		if err != nil {
			yield(StreamEvent{Type: EventError, Err: err})
			return
		}
		resp, ok := doRequest(c.http, httpReq, "llm."+c.provider.Name(), yield)
		if !ok {
			return
		}
		defer resp.Body.Close()

		for ev := range c.provider.ParseStream(ctx, resp, req) {
			if !yield(ev) {
				return
			}
		}
	}
}

// providerRegistry maps a Config.Provider name to its Provider. Unknown names
// (and bare "custom" without anthropic-compatible) fall through to the
// OpenAI-compat default in lookupProvider — they all speak /chat/completions.
//
// providerRegistry 把 Config.Provider name 映射到 Provider。未知 name（及未声明
// anthropic-compatible 的裸 "custom"）在 lookupProvider 回落到 OpenAI-compat 默认
// —— 它们都讲 /chat/completions。
var providerRegistry = buildProviderRegistry()

// buildProviderRegistry constructs the canonical Provider registry. Every
// provider now uses its own self-contained Provider type: openai / deepseek /
// qwen / zhipu / moonshot / doubao / openrouter / ollama / custom speak
// OpenAI-compat; anthropic and google (native generateContent) speak their own
// dialects. mock is absent (Build short-circuits to MockClient).
//
// buildProviderRegistry 构建权威 Provider 注册表。每个 provider 均用各自独立的
// Provider 类型：openai/deepseek/qwen/zhipu/moonshot/doubao/openrouter/ollama/
// custom 讲 OpenAI-compat；anthropic 与 google（原生 generateContent）讲各自方言；
// mock 缺席（Build 短路到 MockClient）。
func buildProviderRegistry() map[string]Provider {
	reg := map[string]Provider{
		// anthropic: native-dialect provider.
		// anthropic：原生方言 provider。
		"anthropic": newAnthropicProvider(),
		// google: native generateContent provider (reasoning-text readback +
		// thoughtSignature round-trip; 03 §5). Replaces the OpenAI-compat shim.
		// google：原生 generateContent provider（推理文本读回 + thoughtSignature
		// 回传，03 §5），取代 OpenAI-compat 垫片。
		"google": newGeminiProvider(),
		// openai: self-contained provider (reasoning_effort for o-series, 03 §2).
		// openai：自有 provider（o 系列 reasoning_effort，03 §2）。
		"openai": newOpenAIProvider(),
		// deepseek: self-contained provider (reasoning_content round-trip + thinking, 03 §3).
		// deepseek：自有 provider（reasoning_content round-trip + thinking，03 §3）。
		"deepseek": newDeepSeekProvider(),
		// qwen: self-contained provider (enable_thinking bool + stream guard + flat error envelope, 03 §6).
		// qwen：自有 provider（enable_thinking bool + 流式守卫 + 扁平错误信封，03 §6）。
		"qwen": newQwenProvider(),
		// zhipu: self-contained provider (thinking:{type} + tool_choice:"auto" only, 03 §7).
		// zhipu：自有 provider（thinking:{type} + tool_choice 只支持 "auto"，03 §7）。
		"zhipu": newZhipuProvider(),
		// moonshot: self-contained provider (thinking:{type} for k2.5/k2.6; reasoning_content, 03 §8).
		// moonshot：自有 provider（k2.5/k2.6 的 thinking:{type}；reasoning_content，03 §8）。
		"moonshot": newMoonshotProvider(),
		// doubao: self-contained provider (thinking:{type:enabled|disabled} + budget_tokens, 03 §9).
		// doubao：自有 provider（thinking:{type} + budget_tokens，03 §9）。
		"doubao": newDoubaoProvider(),
		// openrouter: self-contained provider (reasoning:{effort|max_tokens} + ':' skip, 03 §10).
		// openrouter：自有 provider（reasoning:{effort|max_tokens} + ':' 行跳过，03 §10）。
		"openrouter": newOpenRouterProvider(),
		// ollama: self-contained provider (reasoning_effort + stream-disable-on-tools, 03 §11).
		// ollama：自有 provider（reasoning_effort + tools 时关流，03 §11）。
		"ollama": newOllamaProvider(),
		// custom: self-contained provider (plain OpenAI-compat, no thinking encoding).
		// custom：自有 provider（纯 OpenAI-compat，无 thinking 编码）。
		"custom": newCustomProvider(),
	}
	return reg
}

// lookupProvider resolves the Provider for a Config; "custom" + anthropic-compatible
// routes to anthropic, every other unknown name falls back to OpenAI-compat (matching
// the historical default wire client).
//
// lookupProvider 按 Config 解析 Provider；"custom"+anthropic-compatible 路由到
// anthropic，其余未知 name 回落 OpenAI-compat（与历史默认 wire client 一致）。
func lookupProvider(cfg Config) Provider {
	if cfg.Provider == "custom" && cfg.APIFormat == "anthropic-compatible" {
		return providerRegistry["anthropic"]
	}
	if p, ok := providerRegistry[cfg.Provider]; ok {
		return p
	}
	return providerRegistry["openai"]
}

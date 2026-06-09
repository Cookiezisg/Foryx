package llm

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"time"

	limitspkg "github.com/sunweilin/forgify/backend/internal/pkg/limits"
)

// Provider is one LLM wire dialect: it owns how a Request becomes an HTTP request (body
// shape, auth headers, base-url + path) and how the response becomes the typed
// StreamEvent stream. Identity (Name / DefaultBaseURL) drives registry lookup and
// base-url resolution. Each provider implements this fully self-contained.
//
// Provider 是一种 LLM wire 方言：负责 Request→HTTP 请求（body 形状、auth 头、
// base-url+path）与响应→StreamEvent 流。Name / DefaultBaseURL 供注册表查找与 base-url
// 解析。每个 provider 完整自包含地实现它。
type Provider interface {
	Name() string
	DefaultBaseURL() string
	BuildRequest(ctx context.Context, req Request) (*http.Request, error)
	ParseStream(ctx context.Context, resp *http.Response, req Request) iter.Seq[StreamEvent]

	// DescribeModels parses this provider's raw /models probe body (archived by apikey) into
	// ModelInfo, each carrying its native configurable knobs. Rich providers (gemini/moonshot/
	// openrouter) read specs+knobs from the payload; lean ones (openai/deepseek/...) read ids and
	// fill specs+knobs from their own static table. Pure parsing, no network.
	//
	// DescribeModels 解析本家 /models 探测原始返回（apikey 存档）为 ModelInfo，每个带原生可调旋钮。
	// 富家（gemini/moonshot/openrouter）从载荷读规格+旋钮，贫家读 id 并用自家静态表补。纯解析、不联网。
	DescribeModels(rawProbe string) ([]ModelInfo, error)
}

// providerClient adapts a Provider to Client by running the shared transport iron-law
// (build → do → status-map → parse). It is the single copy of request/response plumbing
// every Provider funnels through, plus a per-event idle timer for dead-socket detection.
//
// providerClient 把 Provider 适配成 Client：跑共享传输铁律（build → do → status-map →
// parse），是所有 Provider 共用的唯一请求/响应管道，外加逐事件 idle 计时器探测死连接。
type providerClient struct {
	provider Provider
	http     *http.Client
}

func (c *providerClient) Stream(ctx context.Context, req Request) iter.Seq[StreamEvent] {
	return func(yield func(StreamEvent) bool) {
		// Idle timeout: cancel if the stream produces no event for the idle window. A
		// dead-socket detector, NOT a total wall-clock cap — the timer resets on every
		// event, so a healthy long stream (deep reasoning, big generation) never trips it.
		// ctx cancellation (user stop / turn timeout) stays the primary control.
		//
		// idle 超时：流在 idle 窗口内无事件则取消。死连接探测，非总墙钟——每个事件重置
		// 计时器，健康长流永不触发。ctx 取消（用户 stop / turn 超时）仍是主控。
		streamCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		idle := time.Duration(limitspkg.Current().Timeout.LLMIdleSec) * time.Second
		var timer *time.Timer
		if idle > 0 {
			timer = time.AfterFunc(idle, cancel)
			defer timer.Stop()
		}

		httpReq, err := c.provider.BuildRequest(streamCtx, req)
		if err != nil {
			yield(StreamEvent{Type: EventError, Err: err})
			return
		}
		resp, ok := doRequest(c.http, httpReq, "llm."+c.provider.Name(), yield)
		if !ok {
			return
		}
		defer resp.Body.Close()

		for ev := range c.provider.ParseStream(streamCtx, resp, req) {
			if timer != nil {
				timer.Reset(idle)
			}
			if !yield(ev) {
				return
			}
		}

		// If our idle timer fired (streamCtx cancelled while the parent ctx is still
		// alive), the stream went silent mid-flight — surface it as a provider error
		// instead of a phantom user-cancel (which would mislabel the turn).
		//
		// 若 idle 计时器触发（streamCtx 取消而父 ctx 仍活），流中途静默——报 provider 错，
		// 而非伪装成用户取消（会误标该回合）。
		if streamCtx.Err() != nil && ctx.Err() == nil {
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("%w: llm.%s: no stream activity for %s (connection appears dead)", ErrProviderError, c.provider.Name(), idle)})
		}
	}
}

// providerRegistry maps a Config.Provider name to its Provider. Unknown names fall back
// to the OpenAI-compat default in lookupProvider — they all speak /chat/completions.
// Providers are registered here as each is ported (one self-contained entry per provider).
//
// providerRegistry 把 Config.Provider name 映射到 Provider。未知 name 在 lookupProvider
// 回落 OpenAI-compat 默认——它们都讲 /chat/completions。每移植一家就在此注册一条（各自包含）。
var providerRegistry = buildProviderRegistry()

func buildProviderRegistry() map[string]Provider {
	return map[string]Provider{
		"openai":     newOpenAIProvider(),
		"anthropic":  newAnthropicProvider(),
		"google":     newGeminiProvider(),
		"deepseek":   newDeepSeekProvider(),
		"qwen":       newQwenProvider(),
		"zhipu":      newZhipuProvider(),
		"moonshot":   newMoonshotProvider(),
		"doubao":     newDoubaoProvider(),
		"openrouter": newOpenRouterProvider(),
		"ollama":     newOllamaProvider(),
		"custom":     newCustomProvider(),
	}
}

// lookupProvider resolves the Provider for a Config; "custom" + anthropic-compatible
// routes to the anthropic dialect, every other unknown name falls back to OpenAI-compat.
//
// lookupProvider 按 Config 解析 Provider；"custom"+anthropic-compatible 路由到 anthropic
// 方言，其余未知 name 回落 OpenAI-compat。
func lookupProvider(cfg Config) Provider {
	if cfg.Provider == "custom" && cfg.APIFormat == "anthropic-compatible" {
		return providerRegistry["anthropic"]
	}
	if p, ok := providerRegistry[cfg.Provider]; ok {
		return p
	}
	return providerRegistry["openai"]
}

// ModelInfo is one usable model with its capability specs and configurable knobs, assembled by
// a Provider from its /models payload (+ static fallback). The model module aggregates these
// across a workspace's keys for the capabilities surface.
//
// ModelInfo 是一个可用模型及其能力规格与可调旋钮，由 Provider 从 /models 载荷(+静态兜底)装配。
// model 模块跨 workspace 的 key 聚合它们供 capabilities 面用。
type ModelInfo struct {
	ID            string `json:"id"`
	DisplayName   string `json:"displayName"`
	ContextWindow int    `json:"contextWindow"`
	MaxOutput     int    `json:"maxOutput"`
	Vision        bool   `json:"vision"`     // accepts image input natively (OpenAI-compat image_url path)
	NativeDocs    bool   `json:"nativeDocs"` // accepts an inline document (PDF) natively
	Knobs         []Knob `json:"knobs"`
}

// Knob describes one configurable parameter as a render-ready descriptor: a uniform container
// whose content is entirely native — key and values are each provider's own wire vocabulary,
// never translated or normalised. The frontend renders generically from it.
//
// Knob 把一个可配置参数描述成可渲染描述符：统一「容器」，内容全原生——key 与取值是各家自己的
// wire 词表，绝不翻译或归一。前端据此通用渲染。
type Knob struct {
	Key     string   `json:"key"`              // native param name, e.g. "reasoning_effort"
	Label   string   `json:"label"`            // display label
	Type    string   `json:"type"`             // control: "enum" | "int" | "bool"
	Values  []string `json:"values,omitempty"` // native enum values, e.g. ["high","max"]
	Default string   `json:"default,omitempty"`
}

// DescribeModels resolves the Provider for name and parses its raw probe body; unknown names
// fall back to the OpenAI-compat dialect (see lookupProvider).
//
// DescribeModels 按 name 解析 Provider 并解析其探测原始返回；未知 name 回落 OpenAI-compat。
func DescribeModels(provider, rawProbe string) ([]ModelInfo, error) {
	return lookupProvider(Config{Provider: provider}).DescribeModels(rawProbe)
}

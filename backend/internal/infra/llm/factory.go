// factory.go — Provider dispatch: maps (provider, config) to the correct
// Client implementation and resolves the default BaseURL when not supplied.
//
// factory.go — Provider 分派：把（provider, config）映射到正确的 Client 实现，
// 并在未提供 BaseURL 时解析 provider 默认值。
package llm

import "fmt"

// Config carries everything needed to pick and configure a Client.
//
// Config 携带选择和配置 Client 所需的全部信息。
type Config struct {
	Provider  string // "openai" | "anthropic" | "ollama" | "deepseek" | "custom" | …
	APIFormat string // custom provider only: "openai-compatible" | "anthropic-compatible"
	ModelID   string
	Key       string
	BaseURL   string // overrides the provider default when non-empty
}

// Factory creates Clients. It owns one shared HTTP client per wire protocol
// so connections are reused across requests, plus a singleton MockClient
// used when provider="mock" (dev /dev/mock-llm/* surface — script queue
// shared across all Stream calls so testend can push a script then drive
// chat to consume it).
//
// Factory 创建 Client。每种协议共用一个 HTTP client，跨请求复用连接。
// 加一个 MockClient 单例供 provider="mock" 时用（dev /dev/mock-llm/*
// 端面 — script 队列跨 Stream 调用共享，testend 推脚本后驱动 chat 消费）。
type Factory struct {
	openai    *openAIClient
	anthropic *anthropicClient
	mock      *MockClient
}

// NewFactory constructs a Factory ready for use.
//
// NewFactory 构造一个可直接使用的 Factory。
func NewFactory() *Factory {
	return &Factory{
		openai:    newOpenAIClient(),
		anthropic: newAnthropicClient(),
		mock:      NewMockClient(),
	}
}

// Mock returns the singleton MockClient. Used by /dev/mock-llm/* HTTP
// handlers to push scripts + inspect last-request, and by tests that
// want direct in-process driving without going through the dev HTTP
// surface.
//
// Mock 返 MockClient 单例。供 /dev/mock-llm/* HTTP handler push 脚本 +
// 查 last-request 用，也供想跳 dev HTTP 直接 in-process 驱动的测试用。
func (f *Factory) Mock() *MockClient { return f.mock }

// Build returns the Client and resolved BaseURL for the given Config.
//
// Build 返回给定 Config 对应的 Client 和解析后的 BaseURL。
func (f *Factory) Build(cfg Config) (Client, string, error) {
	baseURL, err := resolveBaseURL(cfg)
	if err != nil {
		return nil, "", err
	}
	switch cfg.Provider {
	case "anthropic":
		return f.anthropic, baseURL, nil
	case "mock":
		return f.mock, baseURL, nil
	case "custom":
		if cfg.APIFormat == "anthropic-compatible" {
			return f.anthropic, baseURL, nil
		}
		return f.openai, baseURL, nil
	default:
		return f.openai, baseURL, nil
	}
}

// resolveBaseURL returns cfg.BaseURL when set, or the provider's default.
//
// resolveBaseURL 有 cfg.BaseURL 时直接返回，否则返回 provider 默认值。
func resolveBaseURL(cfg Config) (string, error) {
	if cfg.BaseURL != "" {
		return cfg.BaseURL, nil
	}
	switch cfg.Provider {
	case "openai":
		return "https://api.openai.com/v1", nil
	case "anthropic":
		// Anthropic client appends /v1/messages itself; base is just the host.
		// Anthropic client 自行拼接 /v1/messages，这里只给 host。
		return "https://api.anthropic.com", nil
	case "ollama":
		return "http://localhost:11434/v1", nil
	case "deepseek":
		return "https://api.deepseek.com/v1", nil
	case "qwen", "tongyi":
		return "https://dashscope.aliyuncs.com/compatible-mode/v1", nil
	case "moonshot":
		return "https://api.moonshot.cn/v1", nil
	case "mock":
		// Mock provider is in-process; BaseURL is unused by MockClient.
		// Return an obvious sentinel so log lines + admin views surface
		// 'this LLM is faked' without scattering 'mock' specials elsewhere.
		// mock provider in-process；MockClient 不用 BaseURL。返显眼哨兵让
		// log + 管理页面看到"这是假 LLM"，不必到处加 'mock' 特例。
		return "mock://in-process", nil
	case "custom":
		return "", fmt.Errorf("llm: custom provider requires base_url")
	default:
		return "", fmt.Errorf("llm: unknown provider %q", cfg.Provider)
	}
}

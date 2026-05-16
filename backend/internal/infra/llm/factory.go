package llm

import "fmt"

// Config selects and configures a Client.
//
// Config 用来选择并配置 Client。
type Config struct {
	Provider  string
	APIFormat string
	ModelID   string
	Key       string
	BaseURL   string
}

// Factory builds Clients per provider; HTTP clients are shared across requests.
//
// Factory 按 provider 构造 Client；HTTP client 跨请求复用。
type Factory struct {
	openai    *openAIClient
	anthropic *anthropicClient
	mock      *MockClient
	tracer    *TraceRecorder
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

// Mock returns the singleton MockClient (used by /dev/mock-llm/* and tests).
//
// Mock 返回 MockClient 单例（供 /dev/mock-llm/* 和测试使用）。
func (f *Factory) Mock() *MockClient { return f.mock }

// SetTracer enables LLM call tracing; Build will then wrap every Client in a recorder.
//
// SetTracer 启用 LLM 调用跟踪；Build 会用 recorder 包每个 Client。
func (f *Factory) SetTracer(r *TraceRecorder) { f.tracer = r }

// Tracer returns the active recorder, or nil if SetTracer wasn't called.
//
// Tracer 返回当前 recorder，未设则为 nil。
func (f *Factory) Tracer() *TraceRecorder { return f.tracer }

// Build returns the Client and resolved BaseURL for the given Config.
//
// Build 返回 Config 对应的 Client 与解析后的 BaseURL。
func (f *Factory) Build(cfg Config) (Client, string, error) {
	baseURL, err := resolveBaseURL(cfg)
	if err != nil {
		return nil, "", err
	}
	var client Client
	switch cfg.Provider {
	case "anthropic":
		client = f.anthropic
	case "mock":
		client = f.mock
	case "custom":
		if cfg.APIFormat == "anthropic-compatible" {
			client = f.anthropic
		} else {
			client = f.openai
		}
	default:
		client = f.openai
	}
	client = &adapterWrappedClient{inner: client, adapter: lookupAdapter(cfg.Provider)}
	if f.tracer != nil {
		client = &recordingClient{inner: client, recorder: f.tracer}
	}
	return client, baseURL, nil
}

func resolveBaseURL(cfg Config) (string, error) {
	if cfg.BaseURL != "" {
		return cfg.BaseURL, nil
	}
	a := lookupAdapter(cfg.Provider)
	url := a.DefaultBaseURL()
	if url == "" {
		return "", fmt.Errorf("llm.factory.resolveBaseURL: %s provider requires base_url: %w", cfg.Provider, ErrBadRequest)
	}
	return url, nil
}

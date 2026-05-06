// providers.go — hardcoded whitelist of supported LLM providers and their
// metadata (default base URL, test method).
//
// Adding a new provider:
//  1. Add a ProviderMeta entry to the providers map below.
//  2. If it introduces a new TestMethod, implement the matching branch
//     in HTTPTester.Test.
//
// providers.go — 支持的 LLM provider 白名单和元数据（默认 base URL、测试方式）。
//
// 新增 provider 步骤：
//  1. 在下方 providers map 加一条 ProviderMeta。
//  2. 若引入新的 TestMethod，需在 HTTPTester.Test 实现对应分支。

package apikey

// TestMethod enumerates the HTTP pattern used to test connectivity.
//
// TestMethod 枚举测试连通性的 HTTP 调用模式。
type TestMethod string

const (
	TestMethodGetModels        TestMethod = "get_models"
	TestMethodAnthropicPing    TestMethod = "anthropic_ping"
	TestMethodGoogleListModels TestMethod = "google_list_models"
	TestMethodOllamaTags       TestMethod = "ollama_tags"
	TestMethodCustom           TestMethod = "custom"
	// TestMethodAlwaysOK is for the "mock" dev provider — no real
	// connectivity to test, so the connectivity check is a no-op
	// returning a synthetic ok result with a single model slot.
	//
	// TestMethodAlwaysOK 给 "mock" dev provider——无真实连通性，测试是
	// no-op 返合成 ok 结果含单 model slot。
	TestMethodAlwaysOK TestMethod = "always_ok"
)

// ProviderMeta describes a supported LLM provider.
//
// ProviderMeta 描述一个支持的 LLM provider。
type ProviderMeta struct {
	Name            string
	DisplayName     string
	DefaultBaseURL  string
	BaseURLRequired bool
	TestMethod      TestMethod
}

var providers = map[string]ProviderMeta{
	"openai":     {Name: "openai", DisplayName: "OpenAI", DefaultBaseURL: "https://api.openai.com/v1", TestMethod: TestMethodGetModels},
	"anthropic":  {Name: "anthropic", DisplayName: "Anthropic", DefaultBaseURL: "https://api.anthropic.com", TestMethod: TestMethodAnthropicPing},
	"google":     {Name: "google", DisplayName: "Google Gemini", DefaultBaseURL: "https://generativelanguage.googleapis.com", TestMethod: TestMethodGoogleListModels},
	"deepseek":   {Name: "deepseek", DisplayName: "DeepSeek", DefaultBaseURL: "https://api.deepseek.com", TestMethod: TestMethodGetModels},
	"openrouter": {Name: "openrouter", DisplayName: "OpenRouter", DefaultBaseURL: "https://openrouter.ai/api/v1", TestMethod: TestMethodGetModels},
	"qwen":       {Name: "qwen", DisplayName: "通义千问 (Alibaba Qwen)", DefaultBaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", TestMethod: TestMethodGetModels},
	"zhipu":      {Name: "zhipu", DisplayName: "智谱 GLM", DefaultBaseURL: "https://open.bigmodel.cn/api/paas/v4", TestMethod: TestMethodGetModels},
	"moonshot":   {Name: "moonshot", DisplayName: "Moonshot Kimi", DefaultBaseURL: "https://api.moonshot.cn/v1", TestMethod: TestMethodGetModels},
	"doubao":     {Name: "doubao", DisplayName: "字节豆包 (Doubao)", DefaultBaseURL: "https://ark.cn-beijing.volces.com/api/v3", TestMethod: TestMethodGetModels},
	"ollama":     {Name: "ollama", DisplayName: "Ollama (local)", BaseURLRequired: true, TestMethod: TestMethodOllamaTags},
	"custom":     {Name: "custom", DisplayName: "Custom (OpenAI/Anthropic compatible)", BaseURLRequired: true, TestMethod: TestMethodCustom},
	// Dev-only provider: testend's Mock LLM tab pushes scripts via
	// /dev/mock-llm/scripts; chat resolves provider="mock" → factory
	// returns the singleton MockClient that pops the next script on
	// each Stream call.
	//
	// Dev-only provider：testend Mock LLM tab 经 /dev/mock-llm/scripts
	// 推脚本；chat 解析 provider="mock" → factory 返 MockClient 单例，
	// 每次 Stream 弹下一脚本。
	"mock":       {Name: "mock", DisplayName: "Mock (dev — testend-driven scripts)", TestMethod: TestMethodAlwaysOK},
}

// GetProviderMeta returns metadata for the given provider name.
// Returns false if the name is not in the whitelist.
//
// GetProviderMeta 返回指定 provider 的元数据。bool 为 false 表示不在白名单内。
func GetProviderMeta(name string) (ProviderMeta, bool) {
	m, ok := providers[name]
	return m, ok
}

// IsValidProvider reports whether the name is a supported provider.
//
// IsValidProvider 报告名字是否为支持的 provider。
func IsValidProvider(name string) bool {
	_, ok := providers[name]
	return ok
}

// ListProviders returns all supported provider names (unordered).
// Production code does not call this — it exists for the contract test
// that asserts the registry stays at exactly 11 providers.
//
// ListProviders 返回所有支持的 provider 名字（无序）。
// 生产代码不调用本函数——它存在仅为支撑"注册表恒为 11 项"契约测试。
func ListProviders() []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	return names
}

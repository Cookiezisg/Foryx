package apikey

// ProviderCategory groups providers by integration kind (LLM vs search).
//
// ProviderCategory 按集成类别分组 provider（LLM / 搜索）。
type ProviderCategory string

const (
	CategoryLLM    ProviderCategory = "llm"
	CategorySearch ProviderCategory = "search"
)

// TestMethod enumerates the HTTP pattern used to test connectivity.
//
// TestMethod 枚举连通性测试用的 HTTP 模式。
type TestMethod string

const (
	TestMethodGetModels        TestMethod = "get_models"
	TestMethodAnthropicPing    TestMethod = "anthropic_ping"
	TestMethodGoogleListModels TestMethod = "google_list_models"
	TestMethodOllamaTags       TestMethod = "ollama_tags"
	TestMethodCustom           TestMethod = "custom"
	TestMethodAlwaysOK         TestMethod = "always_ok"
	TestMethodSearchPing       TestMethod = "search_ping"
)

// ProviderMeta describes a supported provider integration.
//
// ProviderMeta 描述一个已支持的 provider 集成。
type ProviderMeta struct {
	Name            string
	DisplayName     string
	DefaultBaseURL  string
	BaseURLRequired bool
	TestMethod      TestMethod
	Category        ProviderCategory
}

var providers = map[string]ProviderMeta{
	"openai":     {Name: "openai", DisplayName: "OpenAI", DefaultBaseURL: "https://api.openai.com/v1", TestMethod: TestMethodGetModels, Category: CategoryLLM},
	"anthropic":  {Name: "anthropic", DisplayName: "Anthropic", DefaultBaseURL: "https://api.anthropic.com", TestMethod: TestMethodAnthropicPing, Category: CategoryLLM},
	"google":     {Name: "google", DisplayName: "Google Gemini", DefaultBaseURL: "https://generativelanguage.googleapis.com", TestMethod: TestMethodGoogleListModels, Category: CategoryLLM},
	"deepseek":   {Name: "deepseek", DisplayName: "DeepSeek", DefaultBaseURL: "https://api.deepseek.com", TestMethod: TestMethodGetModels, Category: CategoryLLM},
	"openrouter": {Name: "openrouter", DisplayName: "OpenRouter", DefaultBaseURL: "https://openrouter.ai/api/v1", TestMethod: TestMethodGetModels, Category: CategoryLLM},
	"qwen":       {Name: "qwen", DisplayName: "通义千问 (Alibaba Qwen)", DefaultBaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", TestMethod: TestMethodGetModels, Category: CategoryLLM},
	"zhipu":      {Name: "zhipu", DisplayName: "智谱 GLM", DefaultBaseURL: "https://open.bigmodel.cn/api/paas/v4", TestMethod: TestMethodGetModels, Category: CategoryLLM},
	"moonshot":   {Name: "moonshot", DisplayName: "Moonshot Kimi", DefaultBaseURL: "https://api.moonshot.cn/v1", TestMethod: TestMethodGetModels, Category: CategoryLLM},
	"doubao":     {Name: "doubao", DisplayName: "字节豆包 (Doubao)", DefaultBaseURL: "https://ark.cn-beijing.volces.com/api/v3", TestMethod: TestMethodGetModels, Category: CategoryLLM},
	"ollama":     {Name: "ollama", DisplayName: "Ollama (local)", BaseURLRequired: true, TestMethod: TestMethodOllamaTags, Category: CategoryLLM},
	"custom":     {Name: "custom", DisplayName: "Custom (OpenAI/Anthropic compatible)", BaseURLRequired: true, TestMethod: TestMethodCustom, Category: CategoryLLM},
	"mock":       {Name: "mock", DisplayName: "Mock (dev — testend-driven scripts)", TestMethod: TestMethodAlwaysOK, Category: CategoryLLM},

	"brave":  {Name: "brave", DisplayName: "Brave Search", DefaultBaseURL: "https://api.search.brave.com/res/v1", TestMethod: TestMethodSearchPing, Category: CategorySearch},
	"serper": {Name: "serper", DisplayName: "Serper.dev (Google search)", DefaultBaseURL: "https://google.serper.dev", TestMethod: TestMethodSearchPing, Category: CategorySearch},
	"tavily": {Name: "tavily", DisplayName: "Tavily (AI-tuned search)", DefaultBaseURL: "https://api.tavily.com", TestMethod: TestMethodSearchPing, Category: CategorySearch},
	"bocha":  {Name: "bocha", DisplayName: "博查 Bocha (CN search)", DefaultBaseURL: "https://api.bochaai.com/v1", TestMethod: TestMethodSearchPing, Category: CategorySearch},
}

// GetProviderMeta returns provider metadata; bool false if not whitelisted.
//
// GetProviderMeta 返回 provider 元数据；不在白名单时 bool 为 false。
func GetProviderMeta(name string) (ProviderMeta, bool) {
	m, ok := providers[name]
	return m, ok
}

func isValidProvider(name string) bool {
	_, ok := providers[name]
	return ok
}

// ListProviders returns all supported provider names (unordered).
//
// ListProviders 返回全部已支持 provider 名字（无序）。
func ListProviders() []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	return names
}

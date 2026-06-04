package apikey

import "sort"

// ProviderCategory groups providers by integration kind (LLM vs search), for
// display grouping only — no selection logic hangs off it (that's downstream).
//
// ProviderCategory 按集成类别分组（LLM / 搜索），仅供展示分组——不挂任何选择逻辑（那在下游）。
type ProviderCategory string

const (
	CategoryLLM    ProviderCategory = "llm"
	CategorySearch ProviderCategory = "search"
)

// TestMethod enumerates how to probe a provider's connectivity (which endpoint +
// auth style). This is "how to knock", not "how to read the answer".
//
// TestMethod 枚举探测一家连通性的方式（哪个端点 + 认证）。这是「怎么敲门」，不是「怎么读回信」。
type TestMethod string

const (
	TestMethodGetModels        TestMethod = "get_models"
	TestMethodAnthropicModels  TestMethod = "anthropic_models"
	TestMethodGoogleListModels TestMethod = "google_list_models"
	TestMethodOllamaTags       TestMethod = "ollama_tags"
	TestMethodCustom           TestMethod = "custom"
	TestMethodAlwaysOK         TestMethod = "always_ok"
	TestMethodSearchPing       TestMethod = "search_ping"
)

// ProviderMeta is what apikey needs to validate, connect to, and probe a
// provider — nothing about models or selection.
//
// ProviderMeta 是 apikey 校验、连接、探测一家所需——不含模型、不含选择。
type ProviderMeta struct {
	Name            string           `json:"name"`
	DisplayName     string           `json:"displayName"`
	DefaultBaseURL  string           `json:"defaultBaseUrl,omitempty"`
	BaseURLRequired bool             `json:"baseUrlRequired"`
	TestMethod      TestMethod       `json:"-"`
	Category        ProviderCategory `json:"category"`
}

var providers = map[string]ProviderMeta{
	"openai":     {Name: "openai", DisplayName: "OpenAI", DefaultBaseURL: "https://api.openai.com/v1", TestMethod: TestMethodGetModels, Category: CategoryLLM},
	"anthropic":  {Name: "anthropic", DisplayName: "Anthropic", DefaultBaseURL: "https://api.anthropic.com", TestMethod: TestMethodAnthropicModels, Category: CategoryLLM},
	"google":     {Name: "google", DisplayName: "Google Gemini", DefaultBaseURL: "https://generativelanguage.googleapis.com/v1beta", TestMethod: TestMethodGoogleListModels, Category: CategoryLLM},
	"deepseek":   {Name: "deepseek", DisplayName: "DeepSeek", DefaultBaseURL: "https://api.deepseek.com", TestMethod: TestMethodGetModels, Category: CategoryLLM},
	"openrouter": {Name: "openrouter", DisplayName: "OpenRouter", DefaultBaseURL: "https://openrouter.ai/api/v1", TestMethod: TestMethodGetModels, Category: CategoryLLM},
	"qwen":       {Name: "qwen", DisplayName: "通义千问 (Alibaba Qwen)", DefaultBaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", TestMethod: TestMethodGetModels, Category: CategoryLLM},
	"zhipu":      {Name: "zhipu", DisplayName: "智谱 GLM", DefaultBaseURL: "https://open.bigmodel.cn/api/paas/v4", TestMethod: TestMethodGetModels, Category: CategoryLLM},
	"moonshot":   {Name: "moonshot", DisplayName: "Moonshot Kimi", DefaultBaseURL: "https://api.moonshot.cn/v1", TestMethod: TestMethodGetModels, Category: CategoryLLM},
	"doubao":     {Name: "doubao", DisplayName: "字节豆包 (Doubao)", DefaultBaseURL: "https://ark.cn-beijing.volces.com/api/v3", TestMethod: TestMethodGetModels, Category: CategoryLLM},
	"ollama":     {Name: "ollama", DisplayName: "Ollama (local)", BaseURLRequired: true, TestMethod: TestMethodOllamaTags, Category: CategoryLLM},
	"custom":     {Name: "custom", DisplayName: "Custom (OpenAI/Anthropic compatible)", BaseURLRequired: true, TestMethod: TestMethodCustom, Category: CategoryLLM},
	"mock":       {Name: "mock", DisplayName: "Mock (dev)", TestMethod: TestMethodAlwaysOK, Category: CategoryLLM},

	"brave":  {Name: "brave", DisplayName: "Brave Search", DefaultBaseURL: "https://api.search.brave.com/res/v1", TestMethod: TestMethodSearchPing, Category: CategorySearch},
	"serper": {Name: "serper", DisplayName: "Serper.dev (Google search)", DefaultBaseURL: "https://google.serper.dev", TestMethod: TestMethodSearchPing, Category: CategorySearch},
	"tavily": {Name: "tavily", DisplayName: "Tavily (AI-tuned search)", DefaultBaseURL: "https://api.tavily.com", TestMethod: TestMethodSearchPing, Category: CategorySearch},
	"bocha":  {Name: "bocha", DisplayName: "博查 Bocha (CN search)", DefaultBaseURL: "https://api.bochaai.com/v1", TestMethod: TestMethodSearchPing, Category: CategorySearch},
}

// GetProviderMeta returns provider metadata; ok=false if not whitelisted.
//
// GetProviderMeta 返回 provider 元数据；不在白名单时 ok=false。
func GetProviderMeta(name string) (ProviderMeta, bool) {
	m, ok := providers[name]
	return m, ok
}

func isValidProvider(name string) bool {
	_, ok := providers[name]
	return ok
}

// ListProviders returns all supported providers, sorted by name for a stable
// response (the GET /providers config endpoint).
//
// ListProviders 返回所有支持的 provider，按 name 排序保证响应稳定（GET /providers 配置端点）。
func ListProviders() []ProviderMeta {
	out := make([]ProviderMeta, 0, len(providers))
	for _, m := range providers {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

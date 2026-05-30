package llm

import (
	"testing"
)

func TestLookupProvider_KnownNamesResolveToSelf(t *testing.T) {
	cases := []struct {
		provider string
		wantName string
		wantBase string
	}{
		{"openai", "openai", "https://api.openai.com/v1"},
		{"deepseek", "deepseek", "https://api.deepseek.com"},
		{"google", "google", "https://generativelanguage.googleapis.com/v1beta/openai"},
		{"qwen", "qwen", "https://dashscope.aliyuncs.com/compatible-mode/v1"},
		{"zhipu", "zhipu", "https://open.bigmodel.cn/api/paas/v4"},
		{"moonshot", "moonshot", "https://api.moonshot.cn/v1"},
		{"doubao", "doubao", "https://ark.cn-beijing.volces.com/api/v3"},
		{"openrouter", "openrouter", "https://openrouter.ai/api/v1"},
		{"anthropic", "anthropic", "https://api.anthropic.com"},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			p := lookupProvider(Config{Provider: tc.provider})
			if p.Name() != tc.wantName {
				t.Errorf("Name() = %q, want %q", p.Name(), tc.wantName)
			}
			if p.DefaultBaseURL() != tc.wantBase {
				t.Errorf("DefaultBaseURL() = %q, want %q", p.DefaultBaseURL(), tc.wantBase)
			}
		})
	}
}

func TestLookupProvider_UnknownFallsBackToOpenAICompat(t *testing.T) {
	p := lookupProvider(Config{Provider: "not-a-real-provider"})
	if p.Name() != "openai" {
		t.Errorf("unknown provider should fall back to openai-compat, got %q", p.Name())
	}
}

// custom defaults to the OpenAI-compat wire dialect (its own identity, compat
// body/SSE) — only an explicit anthropic-compatible APIFormat reroutes it to
// the Anthropic provider.
//
// custom 默认走 OpenAI-compat wire 方言（自有身份，compat body/SSE）；只有显式
// anthropic-compatible 才改路由到 Anthropic。
func TestLookupProvider_CustomDefaultsToOpenAICompat(t *testing.T) {
	p := lookupProvider(Config{Provider: "custom"})
	if _, ok := p.(*openAICompatProvider); !ok {
		t.Errorf("bare custom should use the openai-compat dialect, got %T", p)
	}
	if p.Name() != "custom" {
		t.Errorf("bare custom should keep its own identity, got Name()=%q", p.Name())
	}
}

func TestLookupProvider_CustomAnthropicCompatRoutesToAnthropic(t *testing.T) {
	p := lookupProvider(Config{Provider: "custom", APIFormat: "anthropic-compatible"})
	if p.Name() != "anthropic" {
		t.Errorf("custom+anthropic-compatible should route to anthropic provider, got %q", p.Name())
	}
}

// ollama is OpenAI-compat with an empty default base URL — the caller must
// supply base_url, matching resolveBaseURL's required-base-url path.
//
// ollama 是 OpenAI-compat 但默认 base URL 为空——caller 必须给 base_url。
func TestLookupProvider_OllamaIsCompatWithEmptyBase(t *testing.T) {
	p := lookupProvider(Config{Provider: "ollama"})
	if p.Name() != "ollama" {
		t.Errorf("Name() = %q, want ollama", p.Name())
	}
	if p.DefaultBaseURL() != "" {
		t.Errorf("ollama DefaultBaseURL() = %q, want empty", p.DefaultBaseURL())
	}
}

// mock is intentionally absent from the registry — Build short-circuits to the
// MockClient, so the wire registry has no mock Provider to resolve.
//
// mock 故意不在 registry——Build 直接短路到 MockClient，wire registry 无 mock Provider。
func TestProviderRegistry_OmitsMock(t *testing.T) {
	if _, ok := providerRegistry["mock"]; ok {
		t.Error("mock must not be a wire Provider; Build short-circuits to MockClient")
	}
}

// TestProviderRegistry_DefaultBaseURLs asserts each provider's DefaultBaseURL via the
// registry, covering the full set that was previously tested via the Adapter layer.
//
// TestProviderRegistry_DefaultBaseURLs 通过 registry 断言每个 provider 的 DefaultBaseURL。
func TestProviderRegistry_DefaultBaseURLs(t *testing.T) {
	cases := []struct {
		provider   string
		wantBase   string
		wantNoBase bool // caller must supply base_url
	}{
		{"openai", "https://api.openai.com/v1", false},
		{"anthropic", "https://api.anthropic.com", false},
		{"google", "https://generativelanguage.googleapis.com/v1beta/openai", false},
		{"deepseek", "https://api.deepseek.com", false},
		{"openrouter", "https://openrouter.ai/api/v1", false},
		{"qwen", "https://dashscope.aliyuncs.com/compatible-mode/v1", false},
		{"zhipu", "https://open.bigmodel.cn/api/paas/v4", false},
		{"moonshot", "https://api.moonshot.cn/v1", false},
		{"doubao", "https://ark.cn-beijing.volces.com/api/v3", false},
		{"ollama", "", true},
		{"custom", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			p, ok := providerRegistry[tc.provider]
			if !ok {
				t.Fatalf("provider %q not found in registry", tc.provider)
			}
			got := p.DefaultBaseURL()
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

// TestDeepseekProvider_StripsReasoningOnPlainTurn asserts that deepseekBeforeRequest
// removes reasoning_content from a plain (no tool_calls) assistant turn.
//
// TestDeepseekProvider_StripsReasoningOnPlainTurn 断言纯 assistant turn 的 reasoning_content 被剥除。
func TestDeepseekProvider_StripsReasoningOnPlainTurn(t *testing.T) {
	req := Request{
		Messages: []LLMMessage{
			{Role: RoleUser, Content: "hi"},
			{
				Role:             RoleAssistant,
				Content:          "hello",
				ReasoningContent: "let me think about how to greet",
			},
			{Role: RoleUser, Content: "next"},
		},
	}
	deepseekBeforeRequest(&req)
	if req.Messages[1].ReasoningContent != "" {
		t.Errorf("plain assistant turn should have reasoning_content stripped; got %q",
			req.Messages[1].ReasoningContent)
	}
	if req.Messages[1].Content != "hello" {
		t.Errorf("Content should not be touched; got %q", req.Messages[1].Content)
	}
}

// TestDeepseekProvider_PreservesReasoningOnToolCallTurn asserts that deepseekBeforeRequest
// keeps reasoning_content on an assistant turn that also has tool_calls (V3.2+).
//
// TestDeepseekProvider_PreservesReasoningOnToolCallTurn 断言含 tool_calls 的 turn 保留 reasoning_content（V3.2+）。
func TestDeepseekProvider_PreservesReasoningOnToolCallTurn(t *testing.T) {
	req := Request{
		Messages: []LLMMessage{
			{
				Role:             RoleAssistant,
				ReasoningContent: "I should look this up",
				ToolCalls:        []LLMToolCall{{ID: "c1", Name: "search"}},
			},
		},
	}
	deepseekBeforeRequest(&req)
	if req.Messages[0].ReasoningContent != "I should look this up" {
		t.Errorf("tool-call turn must preserve reasoning_content (V3.2 requirement); got %q",
			req.Messages[0].ReasoningContent)
	}
}

// TestDeepseekProvider_DoesntTouchNonAssistantMessages asserts that deepseekBeforeRequest
// leaves user and tool messages untouched.
//
// TestDeepseekProvider_DoesntTouchNonAssistantMessages 断言非 assistant 消息不受影响。
func TestDeepseekProvider_DoesntTouchNonAssistantMessages(t *testing.T) {
	req := Request{
		Messages: []LLMMessage{
			{Role: RoleUser, Content: "hi", ReasoningContent: "should-not-be-touched"},
			{Role: RoleTool, ToolCallID: "x", Content: "result"},
		},
	}
	deepseekBeforeRequest(&req)
	if req.Messages[0].ReasoningContent != "should-not-be-touched" {
		t.Errorf("user message reasoning_content should not be modified")
	}
}

// TestOllamaProvider_DisablesStreamWhenToolsPresent asserts that ollamaBeforeRequest
// sets DisableStream=true when tools are present, and leaves it false otherwise.
//
// TestOllamaProvider_DisablesStreamWhenToolsPresent 断言有 tools 时设 DisableStream，无 tools 时不动。
func TestOllamaProvider_DisablesStreamWhenToolsPresent(t *testing.T) {
	r1 := Request{}
	ollamaBeforeRequest(&r1)
	if r1.DisableStream {
		t.Errorf("ollama without tools should leave DisableStream=false")
	}

	r2 := Request{Tools: []ToolDef{{Name: "search"}}}
	ollamaBeforeRequest(&r2)
	if !r2.DisableStream {
		t.Errorf("ollama with tools should set DisableStream=true (avoids ollama#12557)")
	}
}

// TestNonBehavioralProviders_NilBeforeRequest asserts that OpenAI-compat
// providers without custom mutations carry a nil beforeRequest hook.
// openai and deepseek are excluded — they are now self-contained providers,
// not openAICompatProvider instances (their pre-request logic lives in BuildRequest).
//
// TestNonBehavioralProviders_NilBeforeRequest 断言无自定义变换的 compat provider 钩子为 nil。
// openai/deepseek 已迁移为自有类型，预处理逻辑内嵌于 BuildRequest，故排除在外。
func TestNonBehavioralProviders_NilBeforeRequest(t *testing.T) {
	for _, name := range []string{"qwen", "moonshot", "google", "openrouter", "zhipu", "doubao"} {
		p, ok := providerRegistry[name]
		if !ok {
			t.Fatalf("provider %q not in registry", name)
		}
		cp, ok := p.(*openAICompatProvider)
		if !ok {
			t.Fatalf("provider %q is not *openAICompatProvider: %T", name, p)
		}
		if cp.beforeRequest != nil {
			req := Request{Tools: []ToolDef{{Name: "x"}}}
			cp.beforeRequest(&req)
			if req.DisableStream {
				t.Errorf("%s should not set DisableStream; only Ollama needs that quirk", name)
			}
		}
	}
}

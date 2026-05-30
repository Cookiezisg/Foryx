package llm

import (
	"context"
	"encoding/json"
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
		{"google", "google", "https://generativelanguage.googleapis.com/v1beta"},
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

// custom defaults to its own self-contained OpenAI-compat provider (customProvider)
// — only an explicit anthropic-compatible APIFormat reroutes it to the Anthropic
// provider.
//
// custom 默认走自有的 OpenAI-compat provider（customProvider）；只有显式
// anthropic-compatible 才改路由到 Anthropic。
func TestLookupProvider_CustomDefaultsToOpenAICompat(t *testing.T) {
	p := lookupProvider(Config{Provider: "custom"})
	if _, ok := p.(*customProvider); !ok {
		t.Errorf("bare custom should use customProvider, got %T", p)
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

// ollama has an empty default base URL — the caller must supply base_url,
// matching resolveBaseURL's required-base-url path. Uses self-contained ollamaProvider.
//
// ollama 默认 base URL 为空——caller 必须给 base_url；使用自有 ollamaProvider。
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
		{"google", "https://generativelanguage.googleapis.com/v1beta", false},
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

// TestDeepseekProvider_StripsReasoningOnPlainTurn asserts that
// deepseekProvider.BuildRequest omits reasoning_content from a plain (no
// tool_calls) assistant turn in the wire body.
//
// TestDeepseekProvider_StripsReasoningOnPlainTurn 断言纯 assistant turn 的
// reasoning_content 在 wire body 中被剥除。
func TestDeepseekProvider_StripsReasoningOnPlainTurn(t *testing.T) {
	p := newDeepSeekProvider()
	req := Request{
		ModelID: "deepseek-chat",
		BaseURL: "https://api.deepseek.com",
		Key:     "sk-test",
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
	httpReq, err := p.BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	var body oaiRequest
	if err := json.NewDecoder(httpReq.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	// Index 1 = assistant (after system-less prepend: user at 0, assistant at 1, user at 2).
	// 索引 1 = assistant 消息（无 system，user→0，assistant→1，user→2）。
	if body.Messages[1].ReasoningContent != "" {
		t.Errorf("plain assistant turn: reasoning_content must be stripped in wire body; got %q",
			body.Messages[1].ReasoningContent)
	}
	var content string
	if err := json.Unmarshal(body.Messages[1].Content, &content); err != nil {
		t.Fatalf("decode content: %v", err)
	}
	if content != "hello" {
		t.Errorf("Content must be preserved; got %q", content)
	}
}

// TestDeepseekProvider_PreservesReasoningOnToolCallTurn asserts that
// deepseekProvider.BuildRequest keeps reasoning_content on an assistant turn
// that also has tool_calls (V3.2+ requirement).
//
// TestDeepseekProvider_PreservesReasoningOnToolCallTurn 断言含 tool_calls 的
// turn 在 wire body 中保留 reasoning_content（V3.2+ 必须）。
func TestDeepseekProvider_PreservesReasoningOnToolCallTurn(t *testing.T) {
	p := newDeepSeekProvider()
	req := Request{
		ModelID: "deepseek-chat",
		BaseURL: "https://api.deepseek.com",
		Key:     "sk-test",
		Messages: []LLMMessage{
			{
				Role:             RoleAssistant,
				ReasoningContent: "I should look this up",
				ToolCalls:        []LLMToolCall{{ID: "c1", Name: "search", Arguments: `{}`}},
			},
			{Role: RoleTool, ToolCallID: "c1", Content: "result"},
			{Role: RoleUser, Content: "ok"},
		},
	}
	httpReq, err := p.BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	var body oaiRequest
	if err := json.NewDecoder(httpReq.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Messages[0].ReasoningContent != "I should look this up" {
		t.Errorf("tool-call turn must preserve reasoning_content (V3.2); got %q",
			body.Messages[0].ReasoningContent)
	}
}

// TestDeepseekProvider_DoesntTouchNonAssistantMessages asserts that
// deepseekProvider.BuildRequest leaves user and tool message content intact.
//
// TestDeepseekProvider_DoesntTouchNonAssistantMessages 断言非 assistant 消息内容不受影响。
func TestDeepseekProvider_DoesntTouchNonAssistantMessages(t *testing.T) {
	p := newDeepSeekProvider()
	req := Request{
		ModelID: "deepseek-chat",
		BaseURL: "https://api.deepseek.com",
		Key:     "sk-test",
		Messages: []LLMMessage{
			{Role: RoleUser, Content: "hi"},
			{Role: RoleAssistant, Content: "ok", ToolCalls: []LLMToolCall{{ID: "c1", Name: "x", Arguments: `{}`}}},
			{Role: RoleTool, ToolCallID: "c1", Content: "result"},
		},
	}
	httpReq, err := p.BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	var body oaiRequest
	if err := json.NewDecoder(httpReq.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	// User message at index 0.
	// 索引 0 = user 消息。
	var userContent string
	if err := json.Unmarshal(body.Messages[0].Content, &userContent); err != nil {
		t.Fatalf("decode user content: %v", err)
	}
	if userContent != "hi" {
		t.Errorf("user message content must not be modified; got %q", userContent)
	}
	// Tool message at index 2.
	// 索引 2 = tool 消息。
	var toolContent string
	if err := json.Unmarshal(body.Messages[2].Content, &toolContent); err != nil {
		t.Fatalf("decode tool content: %v", err)
	}
	if toolContent != "result" {
		t.Errorf("tool message content must not be modified; got %q", toolContent)
	}
}

// TestOllamaProvider_DisablesStreamWhenToolsPresent asserts that ollamaProvider.BuildRequest
// sets stream:false when tools are present (tool_call drop quirk avoidance), and
// leaves stream:true otherwise. Tests the behaviour via BuildRequest instead of
// the old ollamaBeforeRequest hook (now folded into BuildRequest).
//
// TestOllamaProvider_DisablesStreamWhenToolsPresent 断言有 tools 时 ollamaProvider.BuildRequest
// 发 stream:false（避免 tool_call 丢失），无 tools 时发 stream:true。
// 通过 BuildRequest 测试（ollamaBeforeRequest 已内嵌其中）。
func TestOllamaProvider_DisablesStreamWhenToolsPresent(t *testing.T) {
	p := newOllamaProvider()
	baseURL := "http://localhost:11434/v1"

	// Without tools: stream should be true.
	// 无 tools：stream 应为 true。
	r1 := Request{
		ModelID:  "qwen3",
		BaseURL:  baseURL,
		Key:      "ollama",
		Messages: []LLMMessage{{Role: RoleUser, Content: "hi"}},
	}
	httpReq1, err := p.BuildRequest(context.Background(), r1)
	if err != nil {
		t.Fatalf("BuildRequest (no tools): %v", err)
	}
	var body1 ollamaRequest
	if err := json.NewDecoder(httpReq1.Body).Decode(&body1); err != nil {
		t.Fatalf("decode body1: %v", err)
	}
	if !body1.Stream {
		t.Errorf("ollama without tools should have stream:true")
	}

	// With tools: stream should be false (non-streaming to avoid tool_call drop).
	// 有 tools：stream 应为 false（非流式，避免 tool_call 丢失）。
	r2 := Request{
		ModelID:  "qwen3",
		BaseURL:  baseURL,
		Key:      "ollama",
		Messages: []LLMMessage{{Role: RoleUser, Content: "search for cats"}},
		Tools:    []ToolDef{{Name: "search"}},
	}
	httpReq2, err := p.BuildRequest(context.Background(), r2)
	if err != nil {
		t.Fatalf("BuildRequest (with tools): %v", err)
	}
	var body2 ollamaRequest
	if err := json.NewDecoder(httpReq2.Body).Decode(&body2); err != nil {
		t.Fatalf("decode body2: %v", err)
	}
	if body2.Stream {
		t.Errorf("ollama with tools should have stream:false (avoids tool_call drop quirk)")
	}
}

// TestGoogleProvider_IsNativeGemini asserts that google resolves to the native
// geminiProvider (not the OpenAI-compat shim) after the R4 migration.
//
// TestGoogleProvider_IsNativeGemini 断言 R4 后 google 解析到原生 geminiProvider
//（而非 OpenAI-compat 垫片）。
func TestGoogleProvider_IsNativeGemini(t *testing.T) {
	p, ok := providerRegistry["google"]
	if !ok {
		t.Fatal("provider google not in registry")
	}
	if _, ok := p.(*geminiProvider); !ok {
		t.Fatalf("google must be the native *geminiProvider, got %T", p)
	}
}

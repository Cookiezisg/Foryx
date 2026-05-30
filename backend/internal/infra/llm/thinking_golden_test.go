package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

// buildAnthropicBodyForTest is a helper for Anthropic thinking tests that
// calls buildAnthropicBody directly (the native provider, not openAICompat).
//
// buildAnthropicBodyForTest 直接调 buildAnthropicBody（原生 provider）供 Anthropic thinking 测试用。
func buildAnthropicBodyForTest(t *testing.T, req Request) map[string]json.RawMessage {
	t.Helper()
	raw, err := buildAnthropicBody(req)
	if err != nil {
		t.Fatalf("buildAnthropicBody: %v", err)
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	return parsed
}

// ──────────────────────────────────────────────────────────────────────────────
// Thinking encoding golden tests (P3.3)
//
// For every provider: BuildRequest with Thinking=nil → zero thinking fields
// (regression guard). Thinking={Mode:"on"} → exact wire shape per 03 §1.
// Thinking={Mode:"off"} → explicit-disable form where defined.
// ──────────────────────────────────────────────────────────────────────────────

// buildProviderBody is a helper that calls BuildRequest for a named provider
// and returns the raw JSON body. Works with any self-contained Provider from
// the registry.
//
// buildProviderBody 用命名 provider 调 BuildRequest，返回原始 JSON body；
// 适用于 registry 中任何自有 Provider 类型。
func buildProviderBody(t *testing.T, providerName, baseURL string, req Request) []byte {
	t.Helper()
	p, ok := providerRegistry[providerName]
	if !ok {
		t.Fatalf("provider %q not in registry", providerName)
	}
	req.BaseURL = baseURL
	httpReq, err := p.BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(httpReq.Body)
	return buf.Bytes()
}

// assertNoThinkingFields asserts that none of the known thinking wire fields
// appear in the JSON body. Used as the regression guard for nil ThinkingSpec.
//
// assertNoThinkingFields 断言 JSON body 中无任何 thinking 字段；用于 nil spec 回归守卫。
func assertNoThinkingFields(t *testing.T, body []byte) {
	t.Helper()
	thinkingFields := []string{
		`"reasoning_effort"`,
		`"thinking"`,
		`"enable_thinking"`,
		`"thinking_budget"`,
		`"reasoning":{`,
	}
	for _, field := range thinkingFields {
		if bytes.Contains(body, []byte(field)) {
			t.Errorf("nil ThinkingSpec: body must not contain %q; got: %s", field, body)
		}
	}
}

// minimalReq returns a minimal valid Request with no Thinking set.
//
// minimalReq 返回未设 Thinking 的最小合法 Request。
func minimalReq(modelID string) Request {
	return Request{
		ModelID: modelID,
		Key:     "sk-test",
		Messages: []LLMMessage{
			{Role: RoleUser, Content: "hi"},
		},
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// OpenAI
// ──────────────────────────────────────────────────────────────────────────────

// TestThinking_OpenAI_NilSpec_NoFields verifies that nil ThinkingSpec emits no
// thinking fields — byte-identical default behaviour.
//
// 验证 nil ThinkingSpec 时 OpenAI 请求不含任何 thinking 字段。
func TestThinking_OpenAI_NilSpec_NoFields(t *testing.T) {
	req := minimalReq("o3-mini")
	body := buildProviderBody(t, "openai", "https://api.openai.com/v1", req)
	assertNoThinkingFields(t, body)
}

// TestThinking_OpenAI_On_ReasoningEffortHigh verifies Mode="on" + Effort="high"
// emits reasoning_effort:"high" and no other thinking fields.
//
// 验证 OpenAI Mode="on"+Effort="high" 只 emit reasoning_effort:"high"。
func TestThinking_OpenAI_On_ReasoningEffortHigh(t *testing.T) {
	req := minimalReq("o3-mini")
	req.Thinking = &ThinkingSpec{Mode: "on", Effort: "high"}
	body := buildProviderBody(t, "openai", "https://api.openai.com/v1", req)

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var effort string
	if err := json.Unmarshal(parsed["reasoning_effort"], &effort); err != nil {
		t.Fatalf("reasoning_effort not present or not a string: %v", err)
	}
	if effort != "high" {
		t.Errorf("reasoning_effort = %q, want high", effort)
	}
	if _, ok := parsed["thinking"]; ok {
		t.Errorf("openai should not emit 'thinking' field")
	}
	if _, ok := parsed["enable_thinking"]; ok {
		t.Errorf("openai should not emit 'enable_thinking' field")
	}
}

// TestThinking_OpenAI_On_EmptyEffort_DefaultsMedium verifies that empty Effort
// defaults to "medium".
//
// 验证 Effort 为空时默认 "medium"。
func TestThinking_OpenAI_On_EmptyEffort_DefaultsMedium(t *testing.T) {
	req := minimalReq("o3-mini")
	req.Thinking = &ThinkingSpec{Mode: "on", Effort: ""}
	body := buildProviderBody(t, "openai", "https://api.openai.com/v1", req)

	var parsed map[string]json.RawMessage
	json.Unmarshal(body, &parsed)
	var effort string
	json.Unmarshal(parsed["reasoning_effort"], &effort)
	if effort != "medium" {
		t.Errorf("empty Effort should default to medium; got %q", effort)
	}
}

// TestThinking_OpenAI_Off_EmitsNone verifies Mode="off" emits
// reasoning_effort:"none" (in the allowed set for OpenAI reasoning models).
//
// 验证 Mode="off" emit reasoning_effort:"none"。
func TestThinking_OpenAI_Off_EmitsNone(t *testing.T) {
	req := minimalReq("o3-mini")
	req.Thinking = &ThinkingSpec{Mode: "off"}
	body := buildProviderBody(t, "openai", "https://api.openai.com/v1", req)

	var parsed map[string]json.RawMessage
	json.Unmarshal(body, &parsed)
	var effort string
	json.Unmarshal(parsed["reasoning_effort"], &effort)
	if effort != "none" {
		t.Errorf("Mode=off reasoning_effort = %q, want none", effort)
	}
}

// TestThinking_OpenAI_Auto_NoFields verifies Mode="auto" emits no fields.
//
// 验证 Mode="auto" 不发任何字段。
func TestThinking_OpenAI_Auto_NoFields(t *testing.T) {
	req := minimalReq("o3-mini")
	req.Thinking = &ThinkingSpec{Mode: "auto"}
	body := buildProviderBody(t, "openai", "https://api.openai.com/v1", req)
	assertNoThinkingFields(t, body)
}

// ──────────────────────────────────────────────────────────────────────────────
// DeepSeek
// ──────────────────────────────────────────────────────────────────────────────

// TestThinking_DeepSeek_NilSpec_NoFields verifies nil ThinkingSpec on DeepSeek.
//
// 验证 DeepSeek nil ThinkingSpec 不含 thinking 字段。
func TestThinking_DeepSeek_NilSpec_NoFields(t *testing.T) {
	req := minimalReq("deepseek-v4-pro")
	body := buildProviderBody(t, "deepseek", "https://api.deepseek.com", req)
	assertNoThinkingFields(t, body)
}

// TestThinking_DeepSeek_On_ThinkingEnabledPlusEffort verifies Mode="on"+Effort="high"
// emits thinking:{type:"enabled"} AND reasoning_effort:"high". Matches 03 §3 golden.
//
// 验证 DeepSeek Mode="on" emit thinking:{type:"enabled"} + reasoning_effort:"high"，
// 对照 03 §3 黄金请求。
func TestThinking_DeepSeek_On_ThinkingEnabledPlusEffort(t *testing.T) {
	req := minimalReq("deepseek-v4-pro")
	req.Thinking = &ThinkingSpec{Mode: "on", Effort: "high"}
	body := buildProviderBody(t, "deepseek", "https://api.deepseek.com", req)

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// thinking:{type:"enabled"}
	if _, ok := parsed["thinking"]; !ok {
		t.Fatalf("deepseek: body missing 'thinking' field; body: %s", body)
	}
	var thinking map[string]string
	if err := json.Unmarshal(parsed["thinking"], &thinking); err != nil {
		t.Fatalf("thinking field not object: %v", err)
	}
	if thinking["type"] != "enabled" {
		t.Errorf("thinking.type = %q, want enabled", thinking["type"])
	}

	// reasoning_effort:"high"
	if _, ok := parsed["reasoning_effort"]; !ok {
		t.Fatalf("deepseek: body missing 'reasoning_effort' field; body: %s", body)
	}
	var effort string
	json.Unmarshal(parsed["reasoning_effort"], &effort)
	if effort != "high" {
		t.Errorf("reasoning_effort = %q, want high", effort)
	}
}

// TestThinking_DeepSeek_On_EffortXhigh_MapsToMax verifies xhigh→max mapping.
//
// 验证 Effort="xhigh" 映射到 "max"。
func TestThinking_DeepSeek_On_EffortXhigh_MapsToMax(t *testing.T) {
	req := minimalReq("deepseek-v4-pro")
	req.Thinking = &ThinkingSpec{Mode: "on", Effort: "xhigh"}
	body := buildProviderBody(t, "deepseek", "https://api.deepseek.com", req)

	var parsed map[string]json.RawMessage
	json.Unmarshal(body, &parsed)
	var effort string
	json.Unmarshal(parsed["reasoning_effort"], &effort)
	if effort != "max" {
		t.Errorf("Effort=xhigh should map to max for deepseek; got %q", effort)
	}
}

// TestThinking_DeepSeek_On_EffortLow_MapsToHigh verifies low→high mapping.
//
// 验证 Effort="low" 映射到 "high"。
func TestThinking_DeepSeek_On_EffortLow_MapsToHigh(t *testing.T) {
	req := minimalReq("deepseek-v4-pro")
	req.Thinking = &ThinkingSpec{Mode: "on", Effort: "low"}
	body := buildProviderBody(t, "deepseek", "https://api.deepseek.com", req)

	var parsed map[string]json.RawMessage
	json.Unmarshal(body, &parsed)
	var effort string
	json.Unmarshal(parsed["reasoning_effort"], &effort)
	if effort != "high" {
		t.Errorf("Effort=low should map to high for deepseek; got %q", effort)
	}
}

// TestThinking_DeepSeek_Off_ThinkingDisabled verifies Mode="off" emits
// thinking:{type:"disabled"}.
//
// 验证 DeepSeek Mode="off" emit thinking:{type:"disabled"}。
func TestThinking_DeepSeek_Off_ThinkingDisabled(t *testing.T) {
	req := minimalReq("deepseek-v4-pro")
	req.Thinking = &ThinkingSpec{Mode: "off"}
	body := buildProviderBody(t, "deepseek", "https://api.deepseek.com", req)

	var parsed map[string]json.RawMessage
	json.Unmarshal(body, &parsed)
	if _, ok := parsed["thinking"]; !ok {
		t.Fatalf("deepseek off: body missing 'thinking' field; body: %s", body)
	}
	var thinking map[string]string
	json.Unmarshal(parsed["thinking"], &thinking)
	if thinking["type"] != "disabled" {
		t.Errorf("thinking.type = %q, want disabled", thinking["type"])
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Qwen
// ──────────────────────────────────────────────────────────────────────────────

// TestThinking_Qwen_NilSpec_NoFields verifies nil ThinkingSpec on Qwen.
//
// 验证 Qwen nil ThinkingSpec 不含 thinking 字段。
func TestThinking_Qwen_NilSpec_NoFields(t *testing.T) {
	req := minimalReq("qwen-plus")
	body := buildProviderBody(t, "qwen", "https://dashscope.aliyuncs.com/compatible-mode/v1", req)
	assertNoThinkingFields(t, body)
}

// TestThinking_Qwen_On_EnableThinkingTrue verifies Mode="on" emits
// enable_thinking:true. Matches 03 §6 golden.
//
// 验证 Qwen Mode="on" emit enable_thinking:true，对照 03 §6 黄金请求。
func TestThinking_Qwen_On_EnableThinkingTrue(t *testing.T) {
	req := minimalReq("qwen-plus")
	req.Thinking = &ThinkingSpec{Mode: "on"}
	body := buildProviderBody(t, "qwen", "https://dashscope.aliyuncs.com/compatible-mode/v1", req)

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := parsed["enable_thinking"]; !ok {
		t.Fatalf("qwen: body missing 'enable_thinking' field; body: %s", body)
	}
	var et bool
	json.Unmarshal(parsed["enable_thinking"], &et)
	if !et {
		t.Errorf("enable_thinking = %v, want true", et)
	}
	if _, ok := parsed["reasoning_effort"]; ok {
		t.Errorf("qwen should not emit 'reasoning_effort'")
	}
}

// TestThinking_Qwen_On_WithBudget verifies Mode="on"+Budget emits thinking_budget.
//
// 验证 Qwen Mode="on"+Budget emit thinking_budget。
func TestThinking_Qwen_On_WithBudget(t *testing.T) {
	req := minimalReq("qwen-plus")
	req.Thinking = &ThinkingSpec{Mode: "on", Budget: 512}
	body := buildProviderBody(t, "qwen", "https://dashscope.aliyuncs.com/compatible-mode/v1", req)

	var parsed map[string]json.RawMessage
	json.Unmarshal(body, &parsed)
	if _, ok := parsed["thinking_budget"]; !ok {
		t.Fatalf("qwen: body missing 'thinking_budget' when Budget>0; body: %s", body)
	}
	var budget int
	json.Unmarshal(parsed["thinking_budget"], &budget)
	if budget != 512 {
		t.Errorf("thinking_budget = %d, want 512", budget)
	}
}

// TestThinking_Qwen_Off_EnableThinkingFalse verifies Mode="off" emits
// enable_thinking:false.
//
// 验证 Qwen Mode="off" emit enable_thinking:false。
func TestThinking_Qwen_Off_EnableThinkingFalse(t *testing.T) {
	req := minimalReq("qwen-plus")
	req.Thinking = &ThinkingSpec{Mode: "off"}
	body := buildProviderBody(t, "qwen", "https://dashscope.aliyuncs.com/compatible-mode/v1", req)

	var parsed map[string]json.RawMessage
	json.Unmarshal(body, &parsed)
	if _, ok := parsed["enable_thinking"]; !ok {
		t.Fatalf("qwen off: body missing 'enable_thinking'; body: %s", body)
	}
	var et bool
	json.Unmarshal(parsed["enable_thinking"], &et)
	if et {
		t.Errorf("enable_thinking = %v, want false", et)
	}
}

// TestThinking_Qwen_StreamGuard_DisableStreamPlusOn_NoEnableThinking verifies
// that when DisableStream=true and Mode="on", enable_thinking is NOT emitted
// (Qwen 400s if enable_thinking=true on non-streaming requests).
//
// 验证 DisableStream=true+Mode="on" 时不 emit enable_thinking（Qwen 非流式+on → 400）。
func TestThinking_Qwen_StreamGuard_DisableStreamPlusOn_NoEnableThinking(t *testing.T) {
	req := minimalReq("qwen-plus")
	req.DisableStream = true
	req.Thinking = &ThinkingSpec{Mode: "on"}
	body := buildProviderBody(t, "qwen", "https://dashscope.aliyuncs.com/compatible-mode/v1", req)

	if bytes.Contains(body, []byte(`"enable_thinking"`)) {
		t.Errorf("qwen stream guard: enable_thinking must not appear when DisableStream=true+Mode=on; body: %s", body)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Zhipu
// ──────────────────────────────────────────────────────────────────────────────

// TestThinking_Zhipu_NilSpec_NoFields verifies nil ThinkingSpec on Zhipu.
//
// 验证 Zhipu nil ThinkingSpec 不含 thinking 字段。
func TestThinking_Zhipu_NilSpec_NoFields(t *testing.T) {
	req := minimalReq("glm-4.6")
	body := buildProviderBody(t, "zhipu", "https://open.bigmodel.cn/api/paas/v4", req)
	assertNoThinkingFields(t, body)
}

// TestThinking_Zhipu_On_ThinkingEnabled verifies Mode="on" emits
// thinking:{type:"enabled"}. Matches 03 §7 golden.
//
// 验证 Zhipu Mode="on" emit thinking:{type:"enabled"}，对照 03 §7 黄金请求。
func TestThinking_Zhipu_On_ThinkingEnabled(t *testing.T) {
	req := minimalReq("glm-4.6")
	req.Thinking = &ThinkingSpec{Mode: "on", Effort: "high"}
	body := buildProviderBody(t, "zhipu", "https://open.bigmodel.cn/api/paas/v4", req)

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := parsed["thinking"]; !ok {
		t.Fatalf("zhipu: body missing 'thinking' field; body: %s", body)
	}
	var thinking map[string]string
	json.Unmarshal(parsed["thinking"], &thinking)
	if thinking["type"] != "enabled" {
		t.Errorf("thinking.type = %q, want enabled", thinking["type"])
	}
	if _, ok := parsed["reasoning_effort"]; ok {
		t.Errorf("zhipu should not emit 'reasoning_effort'")
	}
}

// TestThinking_Zhipu_Off_ThinkingDisabled verifies Mode="off" emits
// thinking:{type:"disabled"}.
//
// 验证 Zhipu Mode="off" emit thinking:{type:"disabled"}。
func TestThinking_Zhipu_Off_ThinkingDisabled(t *testing.T) {
	req := minimalReq("glm-4.6")
	req.Thinking = &ThinkingSpec{Mode: "off"}
	body := buildProviderBody(t, "zhipu", "https://open.bigmodel.cn/api/paas/v4", req)

	var parsed map[string]json.RawMessage
	json.Unmarshal(body, &parsed)
	var thinking map[string]string
	json.Unmarshal(parsed["thinking"], &thinking)
	if thinking["type"] != "disabled" {
		t.Errorf("thinking.type = %q, want disabled", thinking["type"])
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Moonshot
// ──────────────────────────────────────────────────────────────────────────────

// TestThinking_Moonshot_NilSpec_NoFields verifies nil ThinkingSpec on Moonshot.
//
// 验证 Moonshot nil ThinkingSpec 不含 thinking 字段。
func TestThinking_Moonshot_NilSpec_NoFields(t *testing.T) {
	req := minimalReq("kimi-k2-thinking")
	body := buildProviderBody(t, "moonshot", "https://api.moonshot.cn/v1", req)
	assertNoThinkingFields(t, body)
}

// TestThinking_Moonshot_On_ThinkingEnabled verifies Mode="on" emits
// thinking:{type:"enabled"}. Matches 03 §8.
//
// 验证 Moonshot Mode="on" emit thinking:{type:"enabled"}，对照 03 §8。
func TestThinking_Moonshot_On_ThinkingEnabled(t *testing.T) {
	req := minimalReq("kimi-k2.5")
	req.Thinking = &ThinkingSpec{Mode: "on"}
	body := buildProviderBody(t, "moonshot", "https://api.moonshot.cn/v1", req)

	var parsed map[string]json.RawMessage
	json.Unmarshal(body, &parsed)
	if _, ok := parsed["thinking"]; !ok {
		t.Fatalf("moonshot: body missing 'thinking' field; body: %s", body)
	}
	var thinking map[string]string
	json.Unmarshal(parsed["thinking"], &thinking)
	if thinking["type"] != "enabled" {
		t.Errorf("thinking.type = %q, want enabled", thinking["type"])
	}
}

// TestThinking_Moonshot_Off_ThinkingDisabled verifies Mode="off" emits
// thinking:{type:"disabled"}.
//
// 验证 Moonshot Mode="off" emit thinking:{type:"disabled"}。
func TestThinking_Moonshot_Off_ThinkingDisabled(t *testing.T) {
	req := minimalReq("kimi-k2.5")
	req.Thinking = &ThinkingSpec{Mode: "off"}
	body := buildProviderBody(t, "moonshot", "https://api.moonshot.cn/v1", req)

	var parsed map[string]json.RawMessage
	json.Unmarshal(body, &parsed)
	var thinking map[string]string
	json.Unmarshal(parsed["thinking"], &thinking)
	if thinking["type"] != "disabled" {
		t.Errorf("thinking.type = %q, want disabled", thinking["type"])
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Doubao
// ──────────────────────────────────────────────────────────────────────────────

// TestThinking_Doubao_NilSpec_NoFields verifies nil ThinkingSpec on Doubao.
//
// 验证 Doubao nil ThinkingSpec 不含 thinking 字段。
func TestThinking_Doubao_NilSpec_NoFields(t *testing.T) {
	req := minimalReq("doubao-seed-1-6-thinking-250715")
	body := buildProviderBody(t, "doubao", "https://ark.cn-beijing.volces.com/api/v3", req)
	assertNoThinkingFields(t, body)
}

// TestThinking_Doubao_On_ThinkingEnabledWithBudget verifies Mode="on"+Budget
// emits thinking:{type:"enabled", budget_tokens:N}. Matches 03 §9 golden.
//
// 验证 Doubao Mode="on"+Budget emit thinking:{type:"enabled",budget_tokens:N}，
// 对照 03 §9 黄金请求。
func TestThinking_Doubao_On_ThinkingEnabledWithBudget(t *testing.T) {
	req := minimalReq("doubao-seed-1-6-thinking-250715")
	req.Thinking = &ThinkingSpec{Mode: "on", Budget: 32000}
	body := buildProviderBody(t, "doubao", "https://ark.cn-beijing.volces.com/api/v3", req)

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := parsed["thinking"]; !ok {
		t.Fatalf("doubao: body missing 'thinking' field; body: %s", body)
	}
	var thinking map[string]json.RawMessage
	json.Unmarshal(parsed["thinking"], &thinking)
	var typStr string
	json.Unmarshal(thinking["type"], &typStr)
	if typStr != "enabled" {
		t.Errorf("thinking.type = %q, want enabled", typStr)
	}
	var budget int
	json.Unmarshal(thinking["budget_tokens"], &budget)
	if budget != 32000 {
		t.Errorf("thinking.budget_tokens = %d, want 32000", budget)
	}
}

// TestThinking_Doubao_On_NoBudget verifies Mode="on" without Budget emits
// thinking:{type:"enabled"} without budget_tokens.
//
// 验证 Doubao Mode="on" 无 Budget 时 emit thinking:{type:"enabled"}（无 budget_tokens）。
func TestThinking_Doubao_On_NoBudget(t *testing.T) {
	req := minimalReq("doubao-seed-1-6-thinking-250715")
	req.Thinking = &ThinkingSpec{Mode: "on"}
	body := buildProviderBody(t, "doubao", "https://ark.cn-beijing.volces.com/api/v3", req)

	if bytes.Contains(body, []byte(`"budget_tokens"`)) {
		t.Errorf("doubao: budget_tokens must not appear when Budget=0; body: %s", body)
	}
}

// TestThinking_Doubao_Off_ThinkingDisabled verifies Mode="off" emits
// thinking:{type:"disabled"}.
//
// 验证 Doubao Mode="off" emit thinking:{type:"disabled"}。
func TestThinking_Doubao_Off_ThinkingDisabled(t *testing.T) {
	req := minimalReq("doubao-seed-1-6-thinking-250715")
	req.Thinking = &ThinkingSpec{Mode: "off"}
	body := buildProviderBody(t, "doubao", "https://ark.cn-beijing.volces.com/api/v3", req)

	var parsed map[string]json.RawMessage
	json.Unmarshal(body, &parsed)
	var thinking map[string]string
	json.Unmarshal(parsed["thinking"], &thinking)
	if thinking["type"] != "disabled" {
		t.Errorf("thinking.type = %q, want disabled", thinking["type"])
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// OpenRouter
// ──────────────────────────────────────────────────────────────────────────────

// TestThinking_OpenRouter_NilSpec_NoFields verifies nil ThinkingSpec on OpenRouter.
//
// 验证 OpenRouter nil ThinkingSpec 不含 thinking 字段。
func TestThinking_OpenRouter_NilSpec_NoFields(t *testing.T) {
	req := minimalReq("anthropic/claude-sonnet-4")
	body := buildProviderBody(t, "openrouter", "https://openrouter.ai/api/v1", req)
	assertNoThinkingFields(t, body)
}

// TestThinking_OpenRouter_On_ReasoningEffortHigh verifies Mode="on"+Effort="high"
// emits reasoning:{effort:"high"}. Matches 03 §10 golden.
//
// 验证 OpenRouter Mode="on"+Effort="high" emit reasoning:{effort:"high"}，
// 对照 03 §10 黄金请求。
func TestThinking_OpenRouter_On_ReasoningEffortHigh(t *testing.T) {
	req := minimalReq("anthropic/claude-sonnet-4")
	req.Thinking = &ThinkingSpec{Mode: "on", Effort: "high"}
	body := buildProviderBody(t, "openrouter", "https://openrouter.ai/api/v1", req)

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := parsed["reasoning"]; !ok {
		t.Fatalf("openrouter: body missing 'reasoning' field; body: %s", body)
	}
	var reasoning map[string]json.RawMessage
	json.Unmarshal(parsed["reasoning"], &reasoning)
	var effort string
	json.Unmarshal(reasoning["effort"], &effort)
	if effort != "high" {
		t.Errorf("reasoning.effort = %q, want high", effort)
	}
	if _, ok := reasoning["max_tokens"]; ok {
		t.Errorf("openrouter: reasoning.max_tokens should not appear when effort is set")
	}
}

// TestThinking_OpenRouter_On_BudgetWhenNoEffort verifies Mode="on"+Budget (no Effort)
// emits reasoning:{max_tokens:N}.
//
// 验证 OpenRouter Mode="on"+Budget（无 Effort）emit reasoning:{max_tokens:N}。
func TestThinking_OpenRouter_On_BudgetWhenNoEffort(t *testing.T) {
	req := minimalReq("anthropic/claude-sonnet-4")
	req.Thinking = &ThinkingSpec{Mode: "on", Budget: 4096}
	body := buildProviderBody(t, "openrouter", "https://openrouter.ai/api/v1", req)

	var parsed map[string]json.RawMessage
	json.Unmarshal(body, &parsed)
	var reasoning map[string]json.RawMessage
	json.Unmarshal(parsed["reasoning"], &reasoning)
	var mt int
	json.Unmarshal(reasoning["max_tokens"], &mt)
	if mt != 4096 {
		t.Errorf("reasoning.max_tokens = %d, want 4096", mt)
	}
	if _, ok := reasoning["effort"]; ok {
		t.Errorf("openrouter: reasoning.effort must not appear when Budget is used without Effort")
	}
}

// TestThinking_OpenRouter_On_EffortPreferredOverBudget verifies that when both
// Effort and Budget are set, effort is preferred (mutually exclusive fields).
//
// 验证 Effort 和 Budget 同时设置时 effort 优先（互斥字段）。
func TestThinking_OpenRouter_On_EffortPreferredOverBudget(t *testing.T) {
	req := minimalReq("anthropic/claude-sonnet-4")
	req.Thinking = &ThinkingSpec{Mode: "on", Effort: "medium", Budget: 4096}
	body := buildProviderBody(t, "openrouter", "https://openrouter.ai/api/v1", req)

	var parsed map[string]json.RawMessage
	json.Unmarshal(body, &parsed)
	var reasoning map[string]json.RawMessage
	json.Unmarshal(parsed["reasoning"], &reasoning)
	var effort string
	json.Unmarshal(reasoning["effort"], &effort)
	if effort != "medium" {
		t.Errorf("effort should be preferred over budget; got %q", effort)
	}
	if _, ok := reasoning["max_tokens"]; ok {
		t.Errorf("max_tokens must not appear when effort is set")
	}
}

// TestThinking_OpenRouter_Off_NoReasoningField verifies Mode="off" omits
// reasoning (no documented clean disable form for OpenRouter).
//
// 验证 OpenRouter Mode="off" 不发 reasoning 字段（无文档化关闭形）。
func TestThinking_OpenRouter_Off_NoReasoningField(t *testing.T) {
	req := minimalReq("anthropic/claude-sonnet-4")
	req.Thinking = &ThinkingSpec{Mode: "off"}
	body := buildProviderBody(t, "openrouter", "https://openrouter.ai/api/v1", req)

	if bytes.Contains(body, []byte(`"reasoning"`)) {
		t.Errorf("openrouter off: reasoning field must be absent (no documented disable form); body: %s", body)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Google Gemini (native generateContent)
// ──────────────────────────────────────────────────────────────────────────────

const geminiNativeBase = "https://generativelanguage.googleapis.com/v1beta"

// TestThinking_Gemini_NilSpec_NoThinkingConfig verifies nil ThinkingSpec emits no
// thinkingConfig — but generationConfig is still present, carrying maxOutputTokens
// (always sent so Gemini doesn't truncate at its ~8192 default).
//
// 验证原生 Gemini nil ThinkingSpec 不发 thinkingConfig；但 generationConfig 仍在，
// 携带 maxOutputTokens（始终发，避免 Gemini 默认 ~8192 截断）。
func TestThinking_Gemini_NilSpec_NoThinkingConfig(t *testing.T) {
	req := minimalReq("gemini-2.5-flash")
	body := buildProviderBody(t, "google", geminiNativeBase, req)
	if bytes.Contains(body, []byte(`"thinkingConfig"`)) {
		t.Errorf("nil ThinkingSpec: body must not contain thinkingConfig; got: %s", body)
	}
	if !bytes.Contains(body, []byte(`"maxOutputTokens"`)) {
		t.Errorf("nil ThinkingSpec: body should carry maxOutputTokens (always sent); got: %s", body)
	}
}

// TestThinking_Gemini_On_ThinkingBudget verifies Mode="on" emits
// thinkingConfig{thinkingBudget>0, includeThoughts:true}. Matches 03 §5 native.
//
// 验证原生 Gemini Mode="on" emit thinkingConfig{thinkingBudget>0,
// includeThoughts:true}，对照 03 §5 native。
func TestThinking_Gemini_On_ThinkingBudget(t *testing.T) {
	req := minimalReq("gemini-2.5-flash")
	req.Thinking = &ThinkingSpec{Mode: "on", Budget: 1024}
	body := buildProviderBody(t, "google", geminiNativeBase, req)

	var parsed struct {
		GenerationConfig struct {
			ThinkingConfig struct {
				ThinkingBudget  *int `json:"thinkingBudget"`
				IncludeThoughts bool `json:"includeThoughts"`
			} `json:"thinkingConfig"`
		} `json:"generationConfig"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	tc := parsed.GenerationConfig.ThinkingConfig
	if tc.ThinkingBudget == nil || *tc.ThinkingBudget != 1024 {
		t.Errorf("gemini: thinkingBudget = %v, want 1024; body: %s", tc.ThinkingBudget, body)
	}
	if !tc.IncludeThoughts {
		t.Errorf("gemini on: includeThoughts must be true; body: %s", body)
	}
}

// TestThinking_Gemini_On_DefaultsBudgetFromCaps verifies Mode="on" with no
// explicit Budget falls back to the model's catalog BudgetMax (non-zero).
//
// 验证 Mode="on" 未给 Budget 时回退到模型目录 BudgetMax（非零）。
func TestThinking_Gemini_On_DefaultsBudgetFromCaps(t *testing.T) {
	req := minimalReq("gemini-2.5-flash")
	req.Thinking = &ThinkingSpec{Mode: "on"}
	body := buildProviderBody(t, "google", geminiNativeBase, req)

	var parsed struct {
		GenerationConfig struct {
			ThinkingConfig struct {
				ThinkingBudget *int `json:"thinkingBudget"`
			} `json:"thinkingConfig"`
		} `json:"generationConfig"`
	}
	json.Unmarshal(body, &parsed)
	got := parsed.GenerationConfig.ThinkingConfig.ThinkingBudget
	if got == nil || *got <= 0 {
		t.Errorf("gemini on without explicit budget: thinkingBudget must default >0; got %v; body: %s", got, body)
	}
}

// TestThinking_Gemini_Off_BudgetZero verifies Mode="off" emits
// thinkingConfig{thinkingBudget:0} (the documented disable form).
//
// 验证 Mode="off" emit thinkingConfig{thinkingBudget:0}（文档化的关闭形）。
func TestThinking_Gemini_Off_BudgetZero(t *testing.T) {
	req := minimalReq("gemini-2.5-flash")
	req.Thinking = &ThinkingSpec{Mode: "off"}
	body := buildProviderBody(t, "google", geminiNativeBase, req)

	var parsed struct {
		GenerationConfig struct {
			ThinkingConfig struct {
				ThinkingBudget  *int `json:"thinkingBudget"`
				IncludeThoughts bool `json:"includeThoughts"`
			} `json:"thinkingConfig"`
		} `json:"generationConfig"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	tc := parsed.GenerationConfig.ThinkingConfig
	if tc.ThinkingBudget == nil || *tc.ThinkingBudget != 0 {
		t.Errorf("gemini off: thinkingBudget must be 0; got %v; body: %s", tc.ThinkingBudget, body)
	}
	if tc.IncludeThoughts {
		t.Errorf("gemini off: includeThoughts must be false/absent; body: %s", body)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Ollama
// ──────────────────────────────────────────────────────────────────────────────

// TestThinking_Ollama_NilSpec_NoFields verifies nil ThinkingSpec on Ollama.
//
// 验证 Ollama nil ThinkingSpec 不含 thinking 字段。
func TestThinking_Ollama_NilSpec_NoFields(t *testing.T) {
	req := minimalReq("deepseek-r1")
	req.BaseURL = "http://localhost:11434/v1"
	body := buildProviderBody(t, "ollama", "http://localhost:11434/v1", req)
	assertNoThinkingFields(t, body)
}

// TestThinking_Ollama_On_ReasoningEffortHigh verifies Mode="on"+Effort="high"
// emits reasoning_effort:"high". Matches 03 §11.
//
// 验证 Ollama Mode="on"+Effort="high" emit reasoning_effort:"high"，对照 03 §11。
func TestThinking_Ollama_On_ReasoningEffortHigh(t *testing.T) {
	req := minimalReq("deepseek-r1")
	req.Thinking = &ThinkingSpec{Mode: "on", Effort: "high"}
	body := buildProviderBody(t, "ollama", "http://localhost:11434/v1", req)

	var parsed map[string]json.RawMessage
	json.Unmarshal(body, &parsed)
	var effort string
	json.Unmarshal(parsed["reasoning_effort"], &effort)
	if effort != "high" {
		t.Errorf("ollama: reasoning_effort = %q, want high", effort)
	}
}

// TestThinking_Ollama_On_EmptyEffort_DefaultsMedium verifies empty Effort defaults to "medium".
//
// 验证 Ollama 空 Effort 默认 "medium"。
func TestThinking_Ollama_On_EmptyEffort_DefaultsMedium(t *testing.T) {
	req := minimalReq("deepseek-r1")
	req.Thinking = &ThinkingSpec{Mode: "on", Effort: ""}
	body := buildProviderBody(t, "ollama", "http://localhost:11434/v1", req)

	var parsed map[string]json.RawMessage
	json.Unmarshal(body, &parsed)
	var effort string
	json.Unmarshal(parsed["reasoning_effort"], &effort)
	if effort != "medium" {
		t.Errorf("ollama empty Effort should default to medium; got %q", effort)
	}
}

// TestThinking_Ollama_Off_EmitsNone verifies Mode="off" emits reasoning_effort:"none".
//
// 验证 Ollama Mode="off" emit reasoning_effort:"none"。
func TestThinking_Ollama_Off_EmitsNone(t *testing.T) {
	req := minimalReq("deepseek-r1")
	req.Thinking = &ThinkingSpec{Mode: "off"}
	body := buildProviderBody(t, "ollama", "http://localhost:11434/v1", req)

	var parsed map[string]json.RawMessage
	json.Unmarshal(body, &parsed)
	var effort string
	json.Unmarshal(parsed["reasoning_effort"], &effort)
	if effort != "none" {
		t.Errorf("ollama off: reasoning_effort = %q, want none", effort)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Anthropic (P3.4)
// ──────────────────────────────────────────────────────────────────────────────

// TestThinking_Anthropic_NilSpec_NoThinkingField verifies that a nil ThinkingSpec
// produces no thinking field and that max_tokens reflects modelcaps (not the old
// hardcoded 8096). claude-sonnet-4-5 → 64000.
//
// 验证 nil ThinkingSpec 时 Anthropic 请求不含 thinking 字段，max_tokens 反映 modelcaps
// 而非旧常量 8096（claude-sonnet-4-5 → 64000）。
func TestThinking_Anthropic_NilSpec_NoThinkingField(t *testing.T) {
	req := minimalReq("claude-sonnet-4-5")
	parsed := buildAnthropicBodyForTest(t, req)

	if _, ok := parsed["thinking"]; ok {
		t.Errorf("nil ThinkingSpec: 'thinking' field must be absent; body: %s", mustMarshal(parsed))
	}

	var maxTok int
	if err := json.Unmarshal(parsed["max_tokens"], &maxTok); err != nil {
		t.Fatalf("max_tokens not present: %v", err)
	}
	// claude-sonnet-4-5 matches the "claude-sonnet-4" modelcaps rule → MaxOutput=64000.
	// claude-sonnet-4-5 匹配 "claude-sonnet-4" 规则 → MaxOutput=64000，不再是 8096。
	if maxTok != 64_000 {
		t.Errorf("max_tokens = %d, want 64000 (per-model from modelcaps, not hardcoded 8096)", maxTok)
	}
}

// TestThinking_Anthropic_On_BudgetExplicit verifies Mode="on" + Budget=5000:
// thinking:{type:"enabled",budget_tokens:5000}, max_tokens > 5000, no temperature.
//
// 验证 Mode="on"+Budget=5000：thinking.type=enabled、budget_tokens=5000、
// max_tokens>5000、无 temperature。
func TestThinking_Anthropic_On_BudgetExplicit(t *testing.T) {
	req := minimalReq("claude-sonnet-4-5")
	req.Thinking = &ThinkingSpec{Mode: "on", Budget: 5000}
	parsed := buildAnthropicBodyForTest(t, req)

	thinkingRaw, ok := parsed["thinking"]
	if !ok {
		t.Fatalf("body missing 'thinking' field; body: %s", mustMarshal(parsed))
	}
	var thinking map[string]json.RawMessage
	if err := json.Unmarshal(thinkingRaw, &thinking); err != nil {
		t.Fatalf("thinking field not object: %v", err)
	}
	var typStr string
	json.Unmarshal(thinking["type"], &typStr)
	if typStr != "enabled" {
		t.Errorf("thinking.type = %q, want enabled", typStr)
	}
	var budget int
	json.Unmarshal(thinking["budget_tokens"], &budget)
	if budget != 5000 {
		t.Errorf("thinking.budget_tokens = %d, want 5000", budget)
	}

	var maxTok int
	json.Unmarshal(parsed["max_tokens"], &maxTok)
	if maxTok <= 5000 {
		t.Errorf("max_tokens = %d, must be > budget (5000)", maxTok)
	}

	// temperature must be absent (Anthropic 400s when thinking on + temperature present).
	// thinking 开启时 temperature 必须省略（Anthropic 400）。
	if _, ok := parsed["temperature"]; ok {
		t.Errorf("temperature must be absent when thinking is enabled; body: %s", mustMarshal(parsed))
	}
}

// TestThinking_Anthropic_On_BudgetZero_DefaultsApplied verifies Mode="on" +
// Budget=0: budget is defaulted (≥1024, < max_tokens).
//
// 验证 Mode="on"+Budget=0：budget 有合理默认值（≥1024 且 < max_tokens）。
func TestThinking_Anthropic_On_BudgetZero_DefaultsApplied(t *testing.T) {
	req := minimalReq("claude-sonnet-4-5")
	req.Thinking = &ThinkingSpec{Mode: "on", Budget: 0}
	parsed := buildAnthropicBodyForTest(t, req)

	thinkingRaw, ok := parsed["thinking"]
	if !ok {
		t.Fatalf("body missing 'thinking' field; body: %s", mustMarshal(parsed))
	}
	var thinking map[string]json.RawMessage
	json.Unmarshal(thinkingRaw, &thinking)
	var budget int
	json.Unmarshal(thinking["budget_tokens"], &budget)
	if budget < 1024 {
		t.Errorf("defaulted budget = %d, must be ≥ 1024", budget)
	}

	var maxTok int
	json.Unmarshal(parsed["max_tokens"], &maxTok)
	if budget >= maxTok {
		t.Errorf("budget %d must be < max_tokens %d", budget, maxTok)
	}
}

// TestThinking_Anthropic_On_BudgetHuge_MaxTokensBumped verifies that when
// Budget ≥ modelcaps.MaxOutput (64000 for claude-sonnet-4-5), max_tokens is
// bumped to budget+1024 so the Anthropic constraint (budget < max_tokens) holds.
//
// 验证 Budget ≥ modelcaps.MaxOutput 时 max_tokens 上调至 budget+1024，
// 保证 Anthropic 约束（budget < max_tokens）成立。
func TestThinking_Anthropic_On_BudgetHuge_MaxTokensBumped(t *testing.T) {
	req := minimalReq("claude-sonnet-4-5")
	req.Thinking = &ThinkingSpec{Mode: "on", Budget: 70_000} // > 64000 cap
	parsed := buildAnthropicBodyForTest(t, req)

	var thinking map[string]json.RawMessage
	json.Unmarshal(parsed["thinking"], &thinking)
	var budget int
	json.Unmarshal(thinking["budget_tokens"], &budget)

	var maxTok int
	json.Unmarshal(parsed["max_tokens"], &maxTok)

	if budget >= maxTok {
		t.Errorf("budget %d must be < max_tokens %d (Anthropic constraint)", budget, maxTok)
	}
	if maxTok != budget+1024 {
		t.Errorf("max_tokens = %d, want budget+1024 = %d", maxTok, budget+1024)
	}
}

// TestThinking_Anthropic_Off_DisabledForm verifies Mode="off" emits
// thinking:{type:"disabled"} per 03 §4. No temperature constraint for off.
//
// 验证 Mode="off" emit thinking:{type:"disabled"}，对照 03 §4。
func TestThinking_Anthropic_Off_DisabledForm(t *testing.T) {
	req := minimalReq("claude-sonnet-4-5")
	req.Thinking = &ThinkingSpec{Mode: "off"}
	parsed := buildAnthropicBodyForTest(t, req)

	thinkingRaw, ok := parsed["thinking"]
	if !ok {
		t.Fatalf("body missing 'thinking' field for Mode=off; body: %s", mustMarshal(parsed))
	}
	var thinking map[string]json.RawMessage
	json.Unmarshal(thinkingRaw, &thinking)
	var typStr string
	json.Unmarshal(thinking["type"], &typStr)
	if typStr != "disabled" {
		t.Errorf("thinking.type = %q, want disabled", typStr)
	}
	if _, ok := thinking["budget_tokens"]; ok {
		t.Errorf("thinking.budget_tokens must be absent for Mode=off")
	}
}

// TestThinking_Anthropic_Auto_NoThinkingField verifies Mode="auto" emits
// nothing — current default behaviour (nil and "auto" both silent).
//
// 验证 Mode="auto" 不发 thinking 字段（与 nil 行为相同）。
func TestThinking_Anthropic_Auto_NoThinkingField(t *testing.T) {
	req := minimalReq("claude-sonnet-4-5")
	req.Thinking = &ThinkingSpec{Mode: "auto"}
	parsed := buildAnthropicBodyForTest(t, req)

	if _, ok := parsed["thinking"]; ok {
		t.Errorf("Mode=auto: 'thinking' field must be absent; body: %s", mustMarshal(parsed))
	}
}

// mustMarshal is a test-only JSON marshaller that panics on error.
//
// mustMarshal 是仅测试用的 JSON 序列化辅助，出错 panic。
func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// providers_test.go — unit tests for the provider registry.
//
// providers_test.go — provider 注册表的单元测试。
package apikey

import (
	"slices"
	"testing"
)

// expectedProviders is the contract: these 11 production names must always
// be supported. Removing one is a breaking change for existing data.
// Plus "mock" — dev-only provider added by TE-4a for the testend Mock LLM
// tab; not removable but treated as a separate slot since user-facing
// provider lists should hide it.
//
// expectedProviders 是契约：11 个生产 provider 必须始终被支持。移除任一项
// 对已有数据造成破坏性变更。再加 "mock"——TE-4a 加的 dev-only provider
// 给 testend Mock LLM tab 用；不可移但作独立 slot，因用户面 provider 列
// 表应隐藏它。
var expectedProviders = []string{
	"openai", "anthropic", "google", "deepseek", "openrouter",
	"qwen", "zhipu", "moonshot", "doubao", "ollama", "custom",
}

const expectedDevProviders = 1 // "mock"

func TestListProviders_ContainsAll(t *testing.T) {
	got := ListProviders()
	want := len(expectedProviders) + expectedDevProviders
	if len(got) != want {
		t.Errorf("count: got %d, want %d (11 production + %d dev)", len(got), want, expectedDevProviders)
	}
	for _, name := range expectedProviders {
		if !slices.Contains(got, name) {
			t.Errorf("missing production provider: %q", name)
		}
	}
	if !slices.Contains(got, "mock") {
		t.Errorf("missing dev provider: \"mock\" (added in TE-4a)")
	}
}

func TestIsValidProvider(t *testing.T) {
	for _, name := range expectedProviders {
		if !IsValidProvider(name) {
			t.Errorf("IsValidProvider(%q) = false, want true", name)
		}
	}

	invalid := []string{"", "OpenAI", "chatgpt", "baidu", "unknown", " openai"}
	for _, name := range invalid {
		if IsValidProvider(name) {
			t.Errorf("IsValidProvider(%q) = true, want false", name)
		}
	}
}

func TestGetProviderMeta_AllHaveRequiredFields(t *testing.T) {
	for _, name := range expectedProviders {
		m, ok := GetProviderMeta(name)
		if !ok {
			t.Errorf("GetProviderMeta(%q) not found", name)
			continue
		}
		if m.Name != name {
			t.Errorf("%s: Name mismatch = %q", name, m.Name)
		}
		if m.DisplayName == "" {
			t.Errorf("%s: missing DisplayName", name)
		}
		if m.TestMethod == "" {
			t.Errorf("%s: missing TestMethod", name)
		}
	}
}

func TestGetProviderMeta_BaseURLRequiredFlags(t *testing.T) {
	// ollama and custom MUST require base_url (no sensible default).
	// Everyone else MUST have a default base_url.
	//
	// ollama 和 custom **必须**要求 base_url（没有合理默认值）。
	// 其他 provider **必须**有默认 base_url。
	cases := []struct {
		name            string
		baseURLRequired bool
	}{
		{"openai", false},
		{"anthropic", false},
		{"google", false},
		{"deepseek", false},
		{"openrouter", false},
		{"qwen", false},
		{"zhipu", false},
		{"moonshot", false},
		{"doubao", false},
		{"ollama", true},
		{"custom", true},
	}
	for _, c := range cases {
		m, _ := GetProviderMeta(c.name)
		if m.BaseURLRequired != c.baseURLRequired {
			t.Errorf("%s: BaseURLRequired = %v, want %v", c.name, m.BaseURLRequired, c.baseURLRequired)
		}
		if !c.baseURLRequired && m.DefaultBaseURL == "" {
			t.Errorf("%s: missing DefaultBaseURL (not required-provider)", c.name)
		}
	}
}

func TestGetProviderMeta_Unknown(t *testing.T) {
	_, ok := GetProviderMeta("nonexistent")
	if ok {
		t.Errorf("GetProviderMeta(\"nonexistent\") = true, want false")
	}
}

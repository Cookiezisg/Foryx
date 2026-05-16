package apikey

import (
	"slices"
	"testing"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
)

var expectedProviders = []string{
	"openai", "anthropic", "google", "deepseek", "openrouter",
	"qwen", "zhipu", "moonshot", "doubao", "ollama", "custom",
}

const expectedDevProviders = 1 // "mock"

var expectedSearchProviders = []string{"brave", "serper", "tavily", "bocha"}

func TestListProviders_ContainsAll(t *testing.T) {
	got := ListProviders()
	want := len(expectedProviders) + expectedDevProviders + len(expectedSearchProviders)
	if len(got) != want {
		t.Errorf("count: got %d, want %d (%d LLM production + %d dev + %d search)",
			len(got), want, len(expectedProviders), expectedDevProviders, len(expectedSearchProviders))
	}
	for _, name := range expectedProviders {
		if !slices.Contains(got, name) {
			t.Errorf("missing LLM production provider: %q", name)
		}
	}
	if !slices.Contains(got, "mock") {
		t.Errorf("missing dev provider: \"mock\" (added in TE-4a)")
	}
	for _, name := range expectedSearchProviders {
		if !slices.Contains(got, name) {
			t.Errorf("missing search provider: %q", name)
		}
	}
}

func TestIsValidProvider(t *testing.T) {
	for _, name := range expectedProviders {
		if !isValidProvider(name) {
			t.Errorf("isValidProvider(%q) = false, want true", name)
		}
	}

	invalid := []string{"", "OpenAI", "chatgpt", "baidu", "unknown", " openai"}
	for _, name := range invalid {
		if isValidProvider(name) {
			t.Errorf("isValidProvider(%q) = true, want false", name)
		}
	}
}

func TestGetProviderMeta_AllHaveRequiredFields(t *testing.T) {
	allNames := append([]string{}, expectedProviders...)
	allNames = append(allNames, expectedSearchProviders...)
	allNames = append(allNames, "mock")
	for _, name := range allNames {
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
		if m.Category == "" {
			t.Errorf("%s: missing Category", name)
		}
	}
}

func TestGetProviderMeta_SearchProvidersHaveSearchCategory(t *testing.T) {
	for _, name := range expectedSearchProviders {
		m, ok := GetProviderMeta(name)
		if !ok {
			t.Fatalf("GetProviderMeta(%q) not found", name)
		}
		if m.Category != CategorySearch {
			t.Errorf("%s: Category = %q, want %q", name, m.Category, CategorySearch)
		}
		if m.TestMethod != TestMethodSearchPing {
			t.Errorf("%s: TestMethod = %q, want %q", name, m.TestMethod, TestMethodSearchPing)
		}
		if m.DefaultBaseURL == "" {
			t.Errorf("%s: missing DefaultBaseURL", name)
		}
	}
}

func TestSearchProviderPriority_MatchesExpectedSet(t *testing.T) {
	priority := apikeydomain.SearchProviderPriority
	if len(priority) != len(expectedSearchProviders) {
		t.Errorf("SearchProviderPriority len = %d, want %d", len(priority), len(expectedSearchProviders))
	}
	for _, name := range priority {
		if !slices.Contains(expectedSearchProviders, name) {
			t.Errorf("SearchProviderPriority has %q which is not in expectedSearchProviders", name)
		}
	}
}

func TestGetProviderMeta_BaseURLRequiredFlags(t *testing.T) {
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

package websearch

import "testing"

func TestIsProvider(t *testing.T) {
	valid := []string{ProviderBrave, ProviderSerper, ProviderTavily, ProviderBocha}
	for _, p := range valid {
		if !IsProvider(p) {
			t.Fatalf("IsProvider(%q) = false, want true", p)
		}
	}
	invalid := []string{"", "openai", "anthropic", "google", "Brave", "duckduckgo"}
	for _, p := range invalid {
		if IsProvider(p) {
			t.Fatalf("IsProvider(%q) = true, want false", p)
		}
	}
}

func TestProviders(t *testing.T) {
	got := Providers()
	if len(got) != 4 {
		t.Fatalf("Providers() len = %d, want 4", len(got))
	}
	want := []string{"brave", "serper", "tavily", "bocha"}
	for i, p := range want {
		if got[i] != p {
			t.Fatalf("Providers()[%d] = %q, want %q", i, got[i], p)
		}
	}
}

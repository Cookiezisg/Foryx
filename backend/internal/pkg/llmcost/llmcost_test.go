// llmcost_test.go — Lookup + Estimate sanity.
//
// llmcost_test.go ——Lookup + Estimate 烟雾测试。
package llmcost

import (
	"math"
	"testing"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestLookup_ExactMatch(t *testing.T) {
	r, ok := Lookup("deepseek", "deepseek-chat")
	if !ok || r.InputPerMTok != 0.27 {
		t.Errorf("deepseek-chat = %+v ok=%v, want 0.27/1.10", r, ok)
	}
}

func TestLookup_PrefixMatch(t *testing.T) {
	r, ok := Lookup("anthropic", "claude-opus-4-7")
	if !ok || r.InputPerMTok != 15.0 {
		t.Errorf("claude-opus-4-7 prefix = %+v ok=%v, want 15/75", r, ok)
	}
}

func TestLookup_OllamaAny(t *testing.T) {
	r, ok := Lookup("ollama", "llama3.2:7b")
	if !ok || r.InputPerMTok != 0 {
		t.Errorf("ollama any = %+v ok=%v, want 0/0", r, ok)
	}
}

func TestLookup_UnknownReturnsFalse(t *testing.T) {
	_, ok := Lookup("totally-unknown", "made-up")
	if ok {
		t.Errorf("unknown should return ok=false")
	}
}

func TestLookup_EmptyProvider(t *testing.T) {
	_, ok := Lookup("", "deepseek-chat")
	if ok {
		t.Errorf("empty provider should return ok=false")
	}
}

func TestEstimate_DeepSeek(t *testing.T) {
	// 1M input tokens * 0.27 + 0.5M output * 1.10 = 0.27 + 0.55 = 0.82
	got := Estimate("deepseek", "deepseek-chat", 1_000_000, 500_000)
	want := 0.82
	if !almostEqual(got, want) {
		t.Errorf("Estimate(deepseek 1M/500K) = %f, want %f", got, want)
	}
}

func TestEstimate_Unknown_Zero(t *testing.T) {
	got := Estimate("totally-unknown", "made-up", 1_000_000, 1_000_000)
	if got != 0 {
		t.Errorf("unknown Estimate = %f, want 0", got)
	}
}

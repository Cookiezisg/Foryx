package modelmeta

import "testing"

func TestLookup_ExactMatch(t *testing.T) {
	got := Lookup("deepseek", "deepseek-chat")
	if got.ContextWindow != 64000 {
		t.Errorf("ContextWindow = %d, want 64000", got.ContextWindow)
	}
}

func TestLookup_CaseInsensitive(t *testing.T) {
	got := Lookup("DeepSeek", "DeepSeek-Chat")
	if got.ContextWindow != 64000 {
		t.Errorf("ContextWindow = %d, want 64000", got.ContextWindow)
	}
}

func TestLookup_PrefixMatch(t *testing.T) {
	got := Lookup("anthropic", "claude-opus-4-7")
	if got.ContextWindow != 200000 {
		t.Errorf("ContextWindow = %d, want 200000", got.ContextWindow)
	}
}

func TestLookup_UnknownFallback(t *testing.T) {
	got := Lookup("totally-unknown", "made-up-model")
	if got.ContextWindow != 8000 {
		t.Errorf("fallback ContextWindow = %d, want 8000", got.ContextWindow)
	}
}

func TestLookup_EmptyInputReturnsFallback(t *testing.T) {
	got := Lookup("", "deepseek-chat")
	if got.ContextWindow != 8000 {
		t.Errorf("empty provider ContextWindow = %d, want fallback 8000", got.ContextWindow)
	}
}

func TestUsableInput_SubtractsOutputAndBuffer(t *testing.T) {
	m := ModelMeta{ContextWindow: 64000, MaxOutput: 8192}
	want := 64000 - 8192 - SafetyBuffer
	if got := m.UsableInput(); got != want {
		t.Errorf("UsableInput() = %d, want %d", got, want)
	}
}

func TestUsableInput_FloorsAtThousand(t *testing.T) {
	m := ModelMeta{ContextWindow: 4000, MaxOutput: 3500}
	if got := m.UsableInput(); got != 1000 {
		t.Errorf("UsableInput() floor = %d, want 1000", got)
	}
}

package modelcaps

import "testing"

func TestLookup_DeepSeekV4_Effort1M(t *testing.T) {
	c := Lookup("deepseek", "deepseek-v4-pro")
	if c.ContextWindow != 1_000_000 { t.Fatalf("window=%d want 1M", c.ContextWindow) }
	if c.Thinking != ShapeEffort { t.Fatalf("shape=%v want effort", c.Thinking) }
}

func TestLookup_ClaudeOpus48_Effort1M(t *testing.T) {
	c := Lookup("anthropic", "claude-opus-4-8")
	if c.ContextWindow != 1_000_000 { t.Fatalf("window=%d", c.ContextWindow) }
	if c.Thinking != ShapeEffort { t.Fatalf("shape=%v want effort(adaptive)", c.Thinking) }
}

func TestLookup_Sonnet45_200K_Budget(t *testing.T) {
	c := Lookup("anthropic", "claude-sonnet-4-5")
	if c.ContextWindow != 200_000 { t.Fatalf("window=%d want 200K", c.ContextWindow) }
	if c.Thinking != ShapeBudget { t.Fatalf("shape=%v want budget", c.Thinking) }
}

func TestLookup_GLM46_Toggle(t *testing.T) {
	c := Lookup("zhipu", "glm-4.6")
	if c.Thinking != ShapeToggle { t.Fatalf("shape=%v want toggle", c.Thinking) }
}

func TestLookup_Unknown_Fallback(t *testing.T) {
	c := Lookup("deepseek", "totally-new-model-2099")
	if c.ContextWindow == 0 { t.Fatal("fallback must give nonzero window") }
}

func TestLookup_MostSpecificPrefixWins(t *testing.T) {
	// deepseek-v4-* must match the v4 rule (1M), not the generic deepseek fallback (128K).
	c := Lookup("deepseek", "deepseek-v4-flash")
	if c.ContextWindow != 1_000_000 { t.Fatalf("v4-flash window=%d want 1M", c.ContextWindow) }
}

func TestUsableInput_SubtractsOutputAndBuffer(t *testing.T) {
	c := Cap{ContextWindow: 200_000, MaxOutput: 64_000}
	if got := c.UsableInput(); got != 200_000-64_000-SafetyBuffer {
		t.Fatalf("usable=%d", got)
	}
}

func TestUsableInput_Floor(t *testing.T) {
	c := Cap{ContextWindow: 1000, MaxOutput: 900}
	if got := c.UsableInput(); got != 1000 {
		t.Fatalf("usable=%d want floor 1000", got)
	}
}

func TestApply_NilOverlay_ReturnsBase(t *testing.T) {
	base := Cap{ContextWindow: 200_000, Thinking: ShapeBudget}
	got := Apply(base, nil)
	// EffortValues is []string (not comparable); check the scalar fields that matter.
	if got.ContextWindow != base.ContextWindow || got.Thinking != base.Thinking || got.MaxOutput != base.MaxOutput {
		t.Fatalf("nil overlay must return base unchanged, got %+v", got)
	}
}

func TestApply_OverridesOnlySetFields(t *testing.T) {
	base := Cap{ContextWindow: 200_000, MaxOutput: 64_000, Thinking: ShapeBudget}
	shape := ShapeNone
	win := 32_000
	got := Apply(base, &CapOverride{Thinking: &shape, ContextWindow: &win})
	if got.Thinking != ShapeNone { t.Fatalf("thinking not overridden") }
	if got.ContextWindow != 32_000 { t.Fatalf("window not overridden") }
	if got.MaxOutput != 64_000 { t.Fatalf("maxOutput should stay from base") }
}

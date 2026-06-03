package limits

import "testing"

func TestDefault_HighCeilings(t *testing.T) {
	d := Default()
	if d.Agent.MaxSteps != 150 {
		t.Errorf("MaxSteps = %d, want 150", d.Agent.MaxSteps)
	}
	if d.Output.UnknownModelMaxTokens != 64000 {
		t.Errorf("UnknownModelMaxTokens = %d, want 64000", d.Output.UnknownModelMaxTokens)
	}
	if d.Timeout.LLMIdleSec != 150 {
		t.Errorf("LLMIdleSec = %d, want 150", d.Timeout.LLMIdleSec)
	}
	if d.Tools.SearchTopN != 10 {
		t.Errorf("SearchTopN = %d, want 10", d.Tools.SearchTopN)
	}
	if d.Context.SoftRatio != 0.70 || d.Context.HardRatio != 0.85 {
		t.Errorf("ratios = %v/%v, want 0.70/0.85", d.Context.SoftRatio, d.Context.HardRatio)
	}
	if d.Workflow.AgentNodeMaxTurnsHard != 50 {
		t.Errorf("AgentNodeMaxTurnsHard = %d, want 50", d.Workflow.AgentNodeMaxTurnsHard)
	}
}

// Default must satisfy func() Limits so it injects directly as the getter.
func TestDefault_IsGetterShape(t *testing.T) {
	var getter func() Limits = Default
	if getter().Tools.SearchTopN != 10 {
		t.Fatal("getter shape broken")
	}
}

func TestCurrent_DefaultsToDefault(t *testing.T) {
	// With no provider set, Current mirrors Default.
	// 未设 provider 时 Current 等同 Default。
	if Current().Agent.MaxSteps != Default().Agent.MaxSteps {
		t.Errorf("Current should default to Default()")
	}
}

func TestSetProvider_SwapsSource(t *testing.T) {
	defer SetProvider(Default) // restore global state after the test
	custom := Limits{Agent: AgentLimits{MaxSteps: 7}}
	SetProvider(func() Limits { return custom })
	if got := Current().Agent.MaxSteps; got != 7 {
		t.Errorf("Current().MaxSteps = %d, want 7 after SetProvider", got)
	}
}

func TestSetProvider_NilIgnored(t *testing.T) {
	defer SetProvider(Default) // restore global state after the test
	SetProvider(func() Limits { return Limits{Agent: AgentLimits{MaxSteps: 7}} })
	SetProvider(nil) // must be ignored — keep the previous provider
	if got := Current().Agent.MaxSteps; got != 7 {
		t.Errorf("nil SetProvider should be ignored, got MaxSteps=%d", got)
	}
}

func TestMaxSearchTopN(t *testing.T) {
	if MaxSearchTopN != 50 {
		t.Errorf("MaxSearchTopN = %d, want 50", MaxSearchTopN)
	}
}

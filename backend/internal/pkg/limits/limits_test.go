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

// Default must satisfy func() Limits so it injects directly as the P0 getter.
func TestDefault_IsGetterShape(t *testing.T) {
	var getter func() Limits = Default
	if getter().Tools.SearchTopN != 10 {
		t.Fatal("getter shape broken")
	}
}

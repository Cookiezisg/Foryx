package limits

import "testing"

// TestDefault_MatchesPreWiringConstants pins the defaults to the values the code enforced
// before limits was wired — wiring must not change behavior.
//
// TestDefault_MatchesPreWiringConstants 把默认值钉到接线前代码实际执行的常量——接线不得改行为。
func TestDefault_MatchesPreWiringConstants(t *testing.T) {
	d := Default()
	if d.Agent.MaxSteps != 25 || d.Agent.InvokeMaxTurns != 10 {
		t.Fatalf("agent defaults drifted: %+v", d.Agent)
	}
	if d.Context.TriggerRatio != 0.80 {
		t.Fatalf("context default drifted: %+v", d.Context)
	}
	if d.Timeout.LLMIdleSec != 150 || d.Timeout.MCPCallSec != 180 || d.Timeout.BashDefaultTimeoutSec != 120 {
		t.Fatalf("timeout defaults drifted: %+v", d.Timeout)
	}
	if d.Tools.ReadDefaultLines != 2000 || d.Tools.BashOutputCapKB != 256 || d.Tools.ToolResultCapKB != 256 {
		t.Fatalf("tools defaults drifted: %+v", d.Tools)
	}
	if d.Guards.AttachmentMaxMB != 50 || d.Guards.WebhookBodyMaxMB != 10 {
		t.Fatalf("guards defaults drifted: %+v", d.Guards)
	}
}

func TestWithDefaults_FillsZeros(t *testing.T) {
	l := WithDefaults(Limits{Agent: AgentLimits{MaxSteps: 7}})
	if l.Agent.MaxSteps != 7 {
		t.Fatalf("explicit value overwritten: %+v", l.Agent)
	}
	if l.Agent.InvokeMaxTurns != 10 || l.Timeout.MCPCallSec != 180 || l.Context.TriggerRatio != 0.80 {
		t.Fatalf("zeros not filled: %+v", l)
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

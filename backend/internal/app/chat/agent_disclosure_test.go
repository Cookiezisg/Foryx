package chat

import (
	"strings"
	"testing"
)

// TestAgentCategoryDisclosed guards the agentRef-discoverability bug: the agent forge tools live in
// the lazy "agent" group, so the LLM only finds them if the group is disclosed. Before the fix the
// static toolsSection omitted "agent" entirely and categoryLabels had no "agent" entry → the AI
// didn't know it could create an agent directly and fell back to a workflow agent NODE.
func TestAgentCategoryDisclosed(t *testing.T) {
	// (1) categoryLabels must have a real, descriptive "agent" entry (not a bare fallback).
	label, ok := categoryLabels["agent"]
	if !ok || label == "" {
		t.Fatalf("categoryLabels missing 'agent' — capability index renders bare 'agent' with no description")
	}
	if !strings.Contains(strings.ToLower(label), "agent") {
		t.Errorf("agent label %q should describe agent entities", label)
	}

	// (2) The always-loaded toolsSection must mention agent so the LLM's baseline model includes it.
	if !strings.Contains(toolsSection, "agent") {
		t.Errorf("toolsSection omits 'agent' — the AI won't know an agent tool category exists")
	}

	// (3) All four quadrinity forge entities must be disclosed as activatable categories.
	for _, cat := range []string{"function", "handler", "workflow", "agent"} {
		if _, ok := categoryLabels[cat]; !ok {
			t.Errorf("categoryLabels missing quadrinity member %q", cat)
		}
	}
}

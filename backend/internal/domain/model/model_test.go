package model

import (
	"slices"
	"testing"
)

func TestIsValidScenario(t *testing.T) {
	valid := []string{"chat"}
	for _, s := range valid {
		if !IsValidScenario(s) {
			t.Errorf("IsValidScenario(%q) = false, want true", s)
		}
	}

	invalid := []string{"", "Chat", "CHAT", "workflow_llm", "embedding", " chat"}
	for _, s := range invalid {
		if IsValidScenario(s) {
			t.Errorf("IsValidScenario(%q) = true, want false", s)
		}
	}
}

func TestListScenarios_ContainsChat(t *testing.T) {
	got := ListScenarios()
	if len(got) == 0 {
		t.Fatal("ListScenarios() returned empty slice")
	}
	if !slices.Contains(got, ScenarioChat) {
		t.Errorf("ListScenarios() missing %q", ScenarioChat)
	}
}

func TestListScenarios_MatchIsValid(t *testing.T) {
	// Every scenario returned by ListScenarios must pass IsValidScenario.
	// 确保 ListScenarios 返回的每一项都能通过 IsValidScenario 校验。
	for _, s := range ListScenarios() {
		if !IsValidScenario(s) {
			t.Errorf("IsValidScenario(%q) = false, but it is in ListScenarios()", s)
		}
	}
}

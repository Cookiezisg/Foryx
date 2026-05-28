package model_test

import (
	"testing"

	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
)

func TestIsValidScenario_NewSet(t *testing.T) {
	cases := []struct {
		name  string
		valid bool
	}{
		{modeldomain.ScenarioDialogue, true},
		{modeldomain.ScenarioUtility, true},
		{modeldomain.ScenarioAgent, true},
		{"chat", false},        // legacy: removed
		{"web_summary", false}, // legacy: removed
		{"", false},
		{"garbage", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := modeldomain.IsValidScenario(c.name); got != c.valid {
				t.Fatalf("IsValidScenario(%q)=%v, want %v", c.name, got, c.valid)
			}
		})
	}
}

func TestListScenarios_NewSet(t *testing.T) {
	got := modeldomain.ListScenarios()
	want := []string{
		modeldomain.ScenarioDialogue,
		modeldomain.ScenarioUtility,
		modeldomain.ScenarioAgent,
	}
	if len(got) != len(want) {
		t.Fatalf("ListScenarios: got %d, want %d", len(got), len(want))
	}
	for i, s := range want {
		if got[i] != s {
			t.Fatalf("ListScenarios[%d]=%q, want %q", i, got[i], s)
		}
	}
}

package catalog

import (
	"strings"
	"testing"

	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
)

func TestBuildPrompt_ContainsAllItems(t *testing.T) {
	items := []catalogdomain.Item{
		{Source: "forge", ID: "f_a", Name: "alpha", Description: "first forge"},
		{Source: "skill", ID: "s_b", Name: "beta", Description: "first skill"},
		{Source: "mcp", ID: "github", Name: "github", Description: "github server"},
	}
	gMap := map[string]catalogdomain.Granularity{
		"forge": catalogdomain.PerItem,
		"skill": catalogdomain.PerItem,
		"mcp":   catalogdomain.PerServer,
	}
	got := buildPrompt(items, gMap)

	for _, want := range []string{
		"f_a", "alpha", "first forge",
		"s_b", "beta", "first skill",
		"github", "github server",
		"granularity=PerItem", "granularity=PerServer",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q\nfull prompt:\n%s", want, got)
		}
	}
}

func TestBuildPrompt_NoRetryHintArtifact(t *testing.T) {
	got := buildPrompt(
		[]catalogdomain.Item{{Source: "forge", ID: "f", Name: "x", Description: "y"}},
		map[string]catalogdomain.Granularity{"forge": catalogdomain.PerItem},
	)
	if strings.Contains(got, "previous attempt missed") {
		t.Errorf("retry-hint phrasing leaked into single-attempt prompt:\n%s", got)
	}
}

func TestNewLLMGenerator_NilLogOK(t *testing.T) {
	g := NewLLMGenerator(nil, nil, nil, nil)
	if g == nil {
		t.Fatal("NewLLMGenerator returned nil")
	}
	if g.log == nil {
		t.Error("log should be non-nil after construction even with nil arg")
	}
}

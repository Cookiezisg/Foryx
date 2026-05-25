package chat

import (
	"context"
	"strings"
	"testing"

	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
)

type fakePromptProvider struct {
	text string
}

func (f *fakePromptProvider) GetForSystemPrompt(_ context.Context) string { return f.text }

func TestBuildSystemPrompt_NilProvider_SkipsCatalogBlock(t *testing.T) {
	s := &Service{}
	conv := &convdomain.Conversation{}
	got := s.buildSystemPrompt(context.Background(), conv)
	if !strings.Contains(got, "You are Forgify") {
		t.Errorf("base prompt lost: %q", got)
	}
	if strings.Contains(got, "## Available capabilities") {
		t.Errorf("catalog block leaked into system prompt with nil provider:\n%s", got)
	}
}

func TestBuildSystemPrompt_EmptyProviderText_SkipsCatalogBlock(t *testing.T) {
	s := &Service{catalog: &fakePromptProvider{text: ""}}
	conv := &convdomain.Conversation{}
	got := s.buildSystemPrompt(context.Background(), conv)

	if strings.Contains(got, "## Available capabilities") {
		t.Errorf("catalog block leaked into system prompt with empty provider:\n%s", got)
	}
}

func TestBuildSystemPrompt_NonEmptyProvider_InjectsCatalogBlock(t *testing.T) {
	provider := &fakePromptProvider{text: "## Available capabilities\n- 5 forges...\n"}
	s := &Service{catalog: provider}
	conv := &convdomain.Conversation{}
	got := s.buildSystemPrompt(context.Background(), conv)

	if !strings.Contains(got, "You are Forgify") {
		t.Errorf("base prompt lost: %q", got)
	}
	if !strings.Contains(got, "## Available capabilities") {
		t.Errorf("catalog block missing from system prompt:\n%s", got)
	}
	if !strings.Contains(got, "5 forges") {
		t.Errorf("catalog body missing:\n%s", got)
	}
	if idx := strings.Index(got, "## Available capabilities"); idx <= strings.Index(got, "You are Forgify") {
		t.Errorf("catalog block came before intro; ordering wrong:\n%s", got)
	}
}

func TestBuildSystemPrompt_ConvSystemPromptStillIncluded(t *testing.T) {
	s := &Service{catalog: &fakePromptProvider{text: "## CAT"}}
	conv := &convdomain.Conversation{SystemPrompt: "extra CONV hint"}
	got := s.buildSystemPrompt(context.Background(), conv)
	if !strings.Contains(got, "extra CONV hint") {
		t.Errorf("conv.SystemPrompt lost: %q", got)
	}
	if !strings.Contains(got, "## CAT") {
		t.Errorf("catalog block lost: %q", got)
	}
}

func TestBuildSystemPrompt_AlwaysIncludesMultiAgentForging(t *testing.T) {
	cases := []struct {
		name    string
		catalog *fakePromptProvider
	}{
		{"nil-catalog", nil},
		{"empty-catalog", &fakePromptProvider{text: ""}},
		{"populated-catalog", &fakePromptProvider{text: "## Available capabilities\n..."}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{}
			if tc.catalog != nil {
				s.catalog = tc.catalog
			}
			got := s.buildSystemPrompt(context.Background(), &convdomain.Conversation{})
			if !strings.Contains(got, "## Multi-agent forging") {
				t.Errorf("multi-agent section missing:\n%s", got)
			}
			if !strings.Contains(got, "Subagent") {
				t.Errorf("Subagent keyword missing from multi-agent section")
			}
			if !strings.Contains(got, "D21") {
				t.Errorf("D21 awareness missing — sub-agent workflow ops restriction must be taught")
			}
			if !strings.Contains(got, "configState") {
				t.Errorf("configState gate teaching missing")
			}
		})
	}
}

func TestSetSystemPromptProvider_AfterConstruction(t *testing.T) {
	s := &Service{}
	if s.catalog != nil {
		t.Fatal("catalog non-nil before setter called")
	}
	provider := &fakePromptProvider{text: "## hello"}
	s.SetSystemPromptProvider(provider)
	if s.catalog == nil {
		t.Fatal("catalog still nil after setter")
	}
	if got := s.catalog.GetForSystemPrompt(context.Background()); got != "## hello" {
		t.Errorf("setter wired wrong provider; got %q", got)
	}
}

func TestSystemPromptSections_ToolConventionsSection(t *testing.T) {
	s := &Service{}
	conv := &convdomain.Conversation{}
	sections := s.SystemPromptSections(context.Background(), conv)

	var found *PromptSection
	for i := range sections {
		if sections[i].Name == "tool_conventions" {
			found = &sections[i]
			break
		}
	}
	if found == nil {
		t.Fatal("tool_conventions section missing from SystemPromptSections")
	}
	if !strings.Contains(found.Content, "execution_group") {
		t.Errorf("tool_conventions content missing 'execution_group': %q", found.Content)
	}
	if !strings.Contains(found.Content, "destructive") {
		t.Errorf("tool_conventions content missing 'destructive': %q", found.Content)
	}

	// Must appear immediately after "base" (index 1).
	if len(sections) < 2 || sections[1].Name != "tool_conventions" {
		t.Errorf("tool_conventions not at index 1 (after base); order: %v",
			func() []string {
				names := make([]string, len(sections))
				for i, s := range sections {
					names[i] = s.Name
				}
				return names
			}())
	}
}

package agentstate

import (
	"encoding/json"
	"sync"
	"testing"

	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
)

func TestActiveSkill_NilWhenUnset(t *testing.T) {
	var s AgentState
	if got := s.ActiveSkill(); got != nil {
		t.Errorf("ActiveSkill() = %v on zero AgentState, want nil", got)
	}
	if s.IsToolPreApprovedBySkill("Bash", []byte(`{}`)) {
		t.Error("IsToolPreApprovedBySkill = true with no active skill")
	}
}

func TestActiveSkill_SetGetClear(t *testing.T) {
	var s AgentState
	sk := &skilldomain.Skill{Name: "pr-review"}
	s.SetActiveSkill(sk)
	if got := s.ActiveSkill(); got != sk {
		t.Errorf("ActiveSkill mismatch after Set: %v", got)
	}

	s.ClearActiveSkillIfMatches("other-skill")
	if got := s.ActiveSkill(); got != sk {
		t.Errorf("ClearActiveSkillIfMatches mismatched name should not clear; got %v", got)
	}

	s.ClearActiveSkillIfMatches("pr-review")
	if got := s.ActiveSkill(); got != nil {
		t.Errorf("ClearActiveSkillIfMatches matching name should clear; got %v", got)
	}
}

func TestActiveSkill_LastWriteWins(t *testing.T) {
	var s AgentState
	a := &skilldomain.Skill{Name: "a"}
	b := &skilldomain.Skill{Name: "b"}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); s.SetActiveSkill(a) }()
	go func() { defer wg.Done(); s.SetActiveSkill(b) }()
	wg.Wait()

	got := s.ActiveSkill()
	if got != a && got != b {
		t.Errorf("after concurrent set, got %v, want one of {a,b}", got)
	}
}

func TestIsToolPreApprovedBySkill_BareName(t *testing.T) {
	var s AgentState
	s.SetActiveSkill(&skilldomain.Skill{
		Name: "x",
		Frontmatter: skilldomain.Frontmatter{
			AllowedTools: []string{"Read", "Grep"},
		},
	})

	tests := []struct {
		tool string
		want bool
	}{
		{"Read", true},
		{"Grep", true},
		{"Bash", false},
		{"WebFetch", false},
	}
	for _, tt := range tests {
		got := s.IsToolPreApprovedBySkill(tt.tool, []byte(`{}`))
		if got != tt.want {
			t.Errorf("IsToolPreApprovedBySkill(%q) = %v, want %v", tt.tool, got, tt.want)
		}
	}
}

func TestIsToolPreApprovedBySkill_BashAnyArgs(t *testing.T) {
	var s AgentState
	s.SetActiveSkill(&skilldomain.Skill{
		Frontmatter: skilldomain.Frontmatter{
			AllowedTools: []string{"Bash"},
		},
	})
	for _, cmd := range []string{"git status", "rm -rf /", "echo hi"} {
		args, _ := json.Marshal(map[string]string{"command": cmd})
		if !s.IsToolPreApprovedBySkill("Bash", args) {
			t.Errorf("bare 'Bash' should match any command; rejected %q", cmd)
		}
	}
}

func TestIsToolPreApprovedBySkill_BashWildcard(t *testing.T) {
	var s AgentState
	s.SetActiveSkill(&skilldomain.Skill{
		Frontmatter: skilldomain.Frontmatter{
			AllowedTools: []string{"Bash(git *)", "Bash(npm test)"},
		},
	})
	tests := []struct {
		cmd  string
		want bool
	}{
		{"git status", true},
		{"git push --force", true},
		{"git", false},     // trailing space in pattern requires content after
		{"npm test", true}, // exact match
		{"npm tests", false},
		{"rm -rf /", false},
	}
	for _, tt := range tests {
		args, _ := json.Marshal(map[string]string{"command": tt.cmd})
		got := s.IsToolPreApprovedBySkill("Bash", args)
		if got != tt.want {
			t.Errorf("Bash %q: got %v, want %v", tt.cmd, got, tt.want)
		}
	}
}

func TestIsToolPreApprovedBySkill_MalformedPatternIsRejected(t *testing.T) {
	// Malformed pattern must fail closed.
	// 畸形 pattern 必须 fail closed。
	var s AgentState
	s.SetActiveSkill(&skilldomain.Skill{
		Frontmatter: skilldomain.Frontmatter{
			AllowedTools: []string{"Bash(git", "Read("},
		},
	})
	if s.IsToolPreApprovedBySkill("Bash", []byte(`{"command":"git status"}`)) {
		t.Error("malformed Bash( pattern should not match")
	}
	if s.IsToolPreApprovedBySkill("Read", []byte(`{}`)) {
		t.Error("malformed Read( pattern should not match")
	}
}

func TestIsToolPreApprovedBySkill_ParenForNonBashFallsThrough(t *testing.T) {
	var s AgentState
	s.SetActiveSkill(&skilldomain.Skill{
		Frontmatter: skilldomain.Frontmatter{
			AllowedTools: []string{"WebFetch(https://example.com/*)"},
		},
	})
	if s.IsToolPreApprovedBySkill("WebFetch", []byte(`{"url":"https://example.com/foo"}`)) {
		t.Error("V1 paren pattern on non-Bash tool should fall through to non-match")
	}
}

func TestWildcardMatch(t *testing.T) {
	tests := []struct {
		pattern string
		subject string
		want    bool
	}{
		{"git *", "git status", true},
		{"git *", "git push --force", true},
		{"git *", "git", false},
		{"npm test", "npm test", true},
		{"npm test", "npm tests", false},
		{"*foo*", "barfoobar", true},
		{"*foo*", "barbaz", false},
		{"*", "anything", true},
		{"*", "", true},
		{"prefix*suffix", "prefix-MIDDLE-suffix", true},
		{"prefix*suffix", "prefix-suffix", true},
		{"prefix*suffix", "prefixsuffix", true},
		{"prefix*suffix", "wrong", false},
	}
	for _, tt := range tests {
		got := wildcardMatch(tt.pattern, tt.subject)
		if got != tt.want {
			t.Errorf("wildcardMatch(%q, %q) = %v, want %v", tt.pattern, tt.subject, got, tt.want)
		}
	}
}

func TestIsToolPreApprovedBySkill_BadJSONArgsDoesNotPanic(t *testing.T) {
	var s AgentState
	s.SetActiveSkill(&skilldomain.Skill{
		Frontmatter: skilldomain.Frontmatter{
			AllowedTools: []string{"Bash", "Bash(git *)"},
		},
	})
	bad := []byte(`{not json}`)
	if !s.IsToolPreApprovedBySkill("Bash", bad) {
		t.Error("bare Bash should still match even with malformed args")
	}
}

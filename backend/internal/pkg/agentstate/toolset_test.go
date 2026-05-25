package agentstate

import (
	"sort"
	"sync"
	"testing"
)

func TestActivateGroup_ActivatedGroupsRoundtrip(t *testing.T) {
	var s AgentState
	s.ActivateGroup("function")
	got := s.ActivatedGroups()
	if len(got) != 1 || got[0] != "function" {
		t.Errorf("ActivatedGroups() = %v, want [function]", got)
	}
}

func TestActivateGroup_Dedup(t *testing.T) {
	var s AgentState
	s.ActivateGroup("handler")
	s.ActivateGroup("handler")
	s.ActivateGroup("handler")
	got := s.ActivatedGroups()
	if len(got) != 1 {
		t.Errorf("ActivatedGroups() after 3x same = %v (len %d), want exactly 1", got, len(got))
	}
}

func TestActivatedGroups_MultipleCategories(t *testing.T) {
	var s AgentState
	s.ActivateGroup("function")
	s.ActivateGroup("workflow")
	s.ActivateGroup("skill")
	got := s.ActivatedGroups()
	if len(got) != 3 {
		t.Errorf("len(ActivatedGroups()) = %d, want 3", len(got))
	}
	sort.Strings(got)
	want := []string{"function", "skill", "workflow"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("ActivatedGroups()[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestActivatedGroups_ZeroValue(t *testing.T) {
	var s AgentState
	if got := s.ActivatedGroups(); got != nil {
		t.Errorf("ActivatedGroups() on zero AgentState = %v, want nil", got)
	}
}

func TestActivateGroup_ConcurrentSafe(t *testing.T) {
	// Concurrent ActivateGroup + ActivatedGroups must not race.
	//
	// 并发 ActivateGroup + ActivatedGroups 不得 race。
	var s AgentState
	var wg sync.WaitGroup
	cats := []string{"function", "handler", "workflow", "mcp", "document", "skill"}
	for _, c := range cats {
		c := c
		wg.Add(2)
		go func() { defer wg.Done(); s.ActivateGroup(c) }()
		go func() { defer wg.Done(); _ = s.ActivatedGroups() }()
	}
	wg.Wait()
	// All written cats must appear — exact count depends on ordering but at least all are present.
	got := s.ActivatedGroups()
	gotSet := make(map[string]bool, len(got))
	for _, g := range got {
		gotSet[g] = true
	}
	for _, c := range cats {
		if !gotSet[c] {
			t.Errorf("ActivatedGroups() missing %q after concurrent ActivateGroup", c)
		}
	}
}

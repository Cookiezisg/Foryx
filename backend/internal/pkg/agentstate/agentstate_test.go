package agentstate

import (
	"testing"
)

func TestMarkRead_Roundtrip(t *testing.T) {
	s := &AgentState{}
	s.MarkRead("/tmp/a.txt", 1234)
	got, ok := s.WasRead("/tmp/a.txt")
	if !ok {
		t.Fatalf("WasRead: expected hit")
	}
	if got != 1234 {
		t.Errorf("WasRead size = %d, want 1234", got)
	}
}

func TestWasRead_Missing(t *testing.T) {
	s := &AgentState{}
	if _, ok := s.WasRead("/never"); ok {
		t.Error("WasRead on absent path should return ok=false")
	}
}

func TestCwd_ZeroValue(t *testing.T) {
	s := &AgentState{}
	if got := s.Cwd(); got != "" {
		t.Errorf("zero-value Cwd = %q, want empty", got)
	}
}

func TestSetCwd_Roundtrip(t *testing.T) {
	s := &AgentState{}
	s.SetCwd("/work")
	if got := s.Cwd(); got != "/work" {
		t.Errorf("Cwd = %q, want /work", got)
	}
}

package llm

import (
	"testing"
)

func TestSanitize_NoOpOnWellFormed(t *testing.T) {
	in := []LLMMessage{
		{Role: RoleUser, Content: "hi"},
		{Role: RoleAssistant, Content: "let me check", ToolCalls: []LLMToolCall{
			{ID: "call_1", Name: "search", Arguments: "{}"},
		}},
		{Role: RoleTool, ToolCallID: "call_1", Content: "result"},
		{Role: RoleAssistant, Content: "done"},
	}
	out := SanitizeMessages(in)
	if len(out) != 4 {
		t.Fatalf("well-formed history changed length: %d → %d", len(in), len(out))
	}
}

func TestSanitize_MissingToolMessage_StubInserted(t *testing.T) {
	in := []LLMMessage{
		{Role: RoleAssistant, ToolCalls: []LLMToolCall{
			{ID: "call_X", Name: "search", Arguments: "{}"},
		}},
		{Role: RoleUser, Content: "what happened?"},
	}
	out := SanitizeMessages(in)
	if len(out) != 3 {
		t.Fatalf("want 3 messages (assistant + stub tool + user), got %d", len(out))
	}
	if out[1].Role != RoleTool || out[1].ToolCallID != "call_X" {
		t.Errorf("synthesized tool stub missing or wrong id: %+v", out[1])
	}
	if out[1].Content == "" {
		t.Errorf("stub tool message must have non-empty content (LLM looks at it)")
	}
}

func TestSanitize_PartialMissing_StubsForUnpaired(t *testing.T) {
	in := []LLMMessage{
		{Role: RoleAssistant, ToolCalls: []LLMToolCall{
			{ID: "call_A", Name: "tool_a"},
			{ID: "call_B", Name: "tool_b"},
		}},
		{Role: RoleTool, ToolCallID: "call_A", Content: "A result"},
	}
	out := SanitizeMessages(in)
	if len(out) != 3 {
		t.Fatalf("want 3 messages, got %d: %+v", len(out), out)
	}
	if out[1].ToolCallID != "call_A" || out[1].Content != "A result" {
		t.Errorf("real tool result mangled: %+v", out[1])
	}
	if out[2].ToolCallID != "call_B" {
		t.Errorf("missing call_B stub")
	}
}

func TestSanitize_StrayToolMessageDropped(t *testing.T) {
	in := []LLMMessage{
		{Role: RoleUser, Content: "hi"},
		{Role: RoleTool, ToolCallID: "call_orphan", Content: "lost result"},
		{Role: RoleAssistant, Content: "hello"},
	}
	out := SanitizeMessages(in)
	if len(out) != 2 {
		t.Fatalf("stray tool message should be dropped; got %d messages: %+v", len(out), out)
	}
	for _, m := range out {
		if m.Role == RoleTool {
			t.Errorf("stray tool survived: %+v", m)
		}
	}
}

func TestSanitize_IDMismatchInRunDropped(t *testing.T) {
	in := []LLMMessage{
		{Role: RoleAssistant, ToolCalls: []LLMToolCall{
			{ID: "call_X"},
		}},
		{Role: RoleTool, ToolCallID: "call_TYPO", Content: "bogus"},
		{Role: RoleUser, Content: "next"},
	}
	out := SanitizeMessages(in)
	if len(out) != 3 {
		t.Fatalf("want 3 messages (assistant + stub + user), got %d: %+v", len(out), out)
	}
	if out[1].ToolCallID != "call_X" {
		t.Errorf("expected stub for call_X, got %+v", out[1])
	}
}

func TestSanitize_Idempotent(t *testing.T) {
	in := []LLMMessage{
		{Role: RoleAssistant, ToolCalls: []LLMToolCall{{ID: "call_1"}}},
		{Role: RoleUser, Content: "next"},
	}
	once := SanitizeMessages(in)
	twice := SanitizeMessages(once)
	if len(once) != len(twice) {
		t.Fatalf("not idempotent: %d → %d", len(once), len(twice))
	}
}

func TestSanitize_EmptyInput(t *testing.T) {
	out := SanitizeMessages(nil)
	if len(out) != 0 {
		t.Errorf("nil input should return nil/empty, got %+v", out)
	}
}

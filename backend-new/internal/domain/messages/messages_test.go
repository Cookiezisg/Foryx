package messages

import "testing"

func TestIsValidBlockType(t *testing.T) {
	for _, v := range []string{BlockTypeText, BlockTypeReasoning, BlockTypeToolCall, BlockTypeToolResult, BlockTypeCompaction, BlockTypeProgress} {
		if !IsValidBlockType(v) {
			t.Errorf("IsValidBlockType(%q) = false, want true", v)
		}
	}
	// progress IS a first-class persisted block (a tool's intermediate output, kept for replay but
	// excluded from LLM history by the type whitelist). "message" is NOT a block type — sub-agent
	// nesting is expressed via stream Open.ParentID, not a block.
	for _, bad := range []string{"", "message", "unknown"} {
		if IsValidBlockType(bad) {
			t.Errorf("IsValidBlockType(%q) = true, want false", bad)
		}
	}
}

func TestIsValidStatus(t *testing.T) {
	for _, v := range []string{StatusPending, StatusStreaming, StatusCompleted, StatusError, StatusCancelled} {
		if !IsValidStatus(v) {
			t.Errorf("IsValidStatus(%q) = false, want true", v)
		}
	}
	for _, bad := range []string{"", "done", "running"} {
		if IsValidStatus(bad) {
			t.Errorf("IsValidStatus(%q) = true, want false", bad)
		}
	}
}

func TestIsValidContextRole(t *testing.T) {
	for _, v := range []string{ContextRoleHot, ContextRoleWarm, ContextRoleCold, ContextRoleArchived} {
		if !IsValidContextRole(v) {
			t.Errorf("IsValidContextRole(%q) = false, want true", v)
		}
	}
	for _, bad := range []string{"", "frozen"} {
		if IsValidContextRole(bad) {
			t.Errorf("IsValidContextRole(%q) = true, want false", bad)
		}
	}
}

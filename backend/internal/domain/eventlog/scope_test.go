package eventlog

import (
	"testing"
)

func TestScope_StringRoundTrip(t *testing.T) {
	cases := []Scope{
		{Kind: "conversation", ID: "cv_abc123"},
		{Kind: "function", ID: "fn_xyz"},
		{Kind: "handler", ID: "hd_xyz"},
		{Kind: "flowrun", ID: "frun_xyz"},
	}
	for _, s := range cases {
		parsed, err := ParseScope(s.String())
		if err != nil {
			t.Errorf("ParseScope(%q): %v", s.String(), err)
			continue
		}
		if parsed != s {
			t.Errorf("round-trip: got %+v, want %+v", parsed, s)
		}
	}
}

func TestScope_StringFormat(t *testing.T) {
	s := Scope{Kind: "function", ID: "fn_abc"}
	if got := s.String(); got != "function:fn_abc" {
		t.Errorf("String() = %q, want function:fn_abc", got)
	}
}

func TestParseScope_IDWithColon(t *testing.T) {
	// ParseScope must split on the FIRST ':' only — id may contain ':'.
	// ParseScope 只在首个 ':' 切，id 自身可含 ':'。
	s, err := ParseScope("conversation:cv:abc")
	if err != nil {
		t.Fatalf("ParseScope: %v", err)
	}
	if s.Kind != "conversation" || s.ID != "cv:abc" {
		t.Errorf("got %+v, want {conversation, cv:abc}", s)
	}
}

func TestParseScope_RejectsMalformed(t *testing.T) {
	bad := []string{
		"",
		"no_colon",
		":missing_kind",
		"missing_id:",
	}
	for _, raw := range bad {
		if _, err := ParseScope(raw); err == nil {
			t.Errorf("ParseScope(%q) expected error", raw)
		}
	}
}

func TestIsValidKind(t *testing.T) {
	for _, kind := range []string{
		KindConversation, KindFlowRun, KindFunction, KindHandler, KindWorkflow,
	} {
		if !IsValidKind(kind) {
			t.Errorf("IsValidKind(%q) = false, want true", kind)
		}
	}
	for _, bad := range []string{"", "frobnicate", "ConvERSation", "skill"} {
		if IsValidKind(bad) {
			t.Errorf("IsValidKind(%q) = true, want false", bad)
		}
	}
}

func TestConversationScope(t *testing.T) {
	s := ConversationScope("cv_xxx")
	if s.Kind != KindConversation || s.ID != "cv_xxx" {
		t.Errorf("ConversationScope = %+v, want {conversation, cv_xxx}", s)
	}
}

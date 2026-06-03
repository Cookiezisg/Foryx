package stream

import (
	"errors"
	"testing"
)

func TestValidateEvent_Valid(t *testing.T) {
	valid := []Event{
		{Scope: Scope{Kind: KindConversation, ID: "c1"}, ID: "n1", Frame: Open{Node: Node{Type: "text"}}},
		{Scope: Scope{Kind: KindConversation, ID: "c1"}, ID: "n1", Frame: Delta{Chunk: "hi"}},
		{Scope: Scope{Kind: KindConversation, ID: "c1"}, ID: "n1", Frame: Delta{}}, // empty chunk is a no-op
		{Scope: Scope{Kind: KindFunction, ID: "fn1"}, ID: "n1", Frame: Close{Status: StatusCompleted}},
		{Scope: Scope{Kind: KindFunction, ID: "fn1"}, ID: "n1", Frame: Close{Status: StatusCompleted, Result: &Node{Type: "text"}}},
		{Scope: Scope{Kind: KindWorkspace}, ID: "n1", Frame: Signal{Node: Node{Type: "entity_changed"}}},
		{Scope: Scope{Kind: KindWorkspace}, ID: "n1", Frame: Signal{Node: Node{Type: "flowrun_tick"}, Ephemeral: true}},
	}
	for i, e := range valid {
		if err := ValidateEvent(e); err != nil {
			t.Errorf("valid[%d]: ValidateEvent = %v, want nil", i, err)
		}
	}
}

func TestValidateEvent_Invalid(t *testing.T) {
	invalid := []struct {
		name string
		e    Event
	}{
		{"bad scope kind", Event{Scope: Scope{Kind: "bogus", ID: "x"}, ID: "n1", Frame: Delta{}}},
		{"empty node id", Event{Scope: Scope{Kind: KindConversation, ID: "c1"}, Frame: Delta{}}},
		{"nil frame", Event{Scope: Scope{Kind: KindConversation, ID: "c1"}, ID: "n1"}},
		{"open with empty node type", Event{Scope: Scope{Kind: KindConversation, ID: "c1"}, ID: "n1", Frame: Open{Node: Node{}}}},
		{"close with non-terminal status", Event{Scope: Scope{Kind: KindFunction, ID: "f1"}, ID: "n1", Frame: Close{Status: "streaming"}}},
		{"close result with empty node type", Event{Scope: Scope{Kind: KindFunction, ID: "f1"}, ID: "n1", Frame: Close{Status: StatusCompleted, Result: &Node{}}}},
		{"signal with empty node type", Event{Scope: Scope{Kind: KindWorkspace}, ID: "n1", Frame: Signal{Node: Node{}}}},
	}
	for _, tt := range invalid {
		err := ValidateEvent(tt.e)
		if err == nil {
			t.Errorf("%s: ValidateEvent = nil, want error", tt.name)
			continue
		}
		if !errors.Is(err, ErrInvalidEvent) {
			t.Errorf("%s: error %v is not ErrInvalidEvent", tt.name, err)
		}
	}
}

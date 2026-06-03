package stream

import "testing"

func TestFrameDurable(t *testing.T) {
	tests := []struct {
		name  string
		frame Frame
		want  bool
	}{
		{"open is durable", Open{Node: Node{Type: "text"}}, true},
		{"delta is ephemeral", Delta{Chunk: "x"}, false},
		{"close is durable", Close{Status: StatusCompleted}, true},
		{"non-ephemeral signal is durable", Signal{Node: Node{Type: "entity_changed"}}, true},
		{"ephemeral signal is lossy", Signal{Node: Node{Type: "flowrun_tick"}, Ephemeral: true}, false},
	}
	for _, tt := range tests {
		if got := tt.frame.Durable(); got != tt.want {
			t.Errorf("%s: Durable() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

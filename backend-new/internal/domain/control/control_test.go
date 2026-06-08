package control

import (
	"errors"
	"testing"
)

func TestValidateBranches(t *testing.T) {
	cases := []struct {
		name     string
		branches []Branch
		want     error
	}{
		{"empty", nil, ErrInvalidBranches},
		{"port empty", []Branch{{Port: "", When: "true"}}, ErrInvalidBranches},
		{"port whitespace", []Branch{{Port: "  ", When: "true"}}, ErrInvalidBranches},
		{"port duplicate", []Branch{{Port: "a", When: "input.x > 1"}, {Port: "a", When: "true"}}, ErrInvalidBranches},
		{"no catch-all", []Branch{{Port: "a", When: "input.x > 1"}}, ErrNoCatchAll},
		{"catch-all not last", []Branch{{Port: "a", When: "true"}, {Port: "b", When: "input.x > 1"}}, ErrNoCatchAll},
		{"ok single catch-all", []Branch{{Port: "a", When: "true"}}, nil},
		{"ok multi", []Branch{{Port: "a", When: "input.x > 1"}, {Port: "b", When: "true"}}, nil},
		{"catch-all trimmed", []Branch{{Port: "a", When: "  true  "}}, nil},
	}
	for _, c := range cases {
		if err := ValidateBranches(c.branches); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

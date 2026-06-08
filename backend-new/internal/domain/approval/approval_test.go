package approval

import (
	"errors"
	"testing"
	"time"
)

func TestValidateForm(t *testing.T) {
	cases := []struct {
		name, template, timeout, behavior string
		want                              error
	}{
		{"empty template", "", "", "", ErrInvalidTemplate},
		{"whitespace template", "   ", "", "", ErrInvalidTemplate},
		{"ok no timeout", "批准?", "", "", nil},
		{"timeout no behavior", "批准?", "30d", "", ErrInvalidTimeout},
		{"timeout bad behavior", "批准?", "30d", "maybe", ErrInvalidTimeout},
		{"timeout ok reject", "批准?", "30d", "reject", nil},
		{"timeout ok approve", "批准?", "2h", "approve", nil},
		{"timeout bad duration", "批准?", "30x", "reject", ErrInvalidTimeout},
	}
	for _, c := range cases {
		if err := ValidateForm(c.template, c.timeout, c.behavior); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

func TestParseTimeout(t *testing.T) {
	okCases := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"30d", 30 * 24 * time.Hour},
		{"2w", 2 * 7 * 24 * time.Hour},
		{"2h", 2 * time.Hour},
		{"90m", 90 * time.Minute},
	}
	for _, c := range okCases {
		got, err := ParseTimeout(c.in)
		if err != nil || got != c.want {
			t.Errorf("ParseTimeout(%q) = %v, %v; want %v, nil", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{"30x", "abc", "-5d", "d"} {
		if _, err := ParseTimeout(bad); err == nil {
			t.Errorf("ParseTimeout(%q) expected error", bad)
		}
	}
}

func TestIsValidTimeoutBehavior(t *testing.T) {
	for _, b := range []string{"reject", "approve", "fail"} {
		if !IsValidTimeoutBehavior(b) {
			t.Errorf("IsValidTimeoutBehavior(%q) = false, want true", b)
		}
	}
	for _, b := range []string{"", "maybe", "yes"} {
		if IsValidTimeoutBehavior(b) {
			t.Errorf("IsValidTimeoutBehavior(%q) = true, want false", b)
		}
	}
}

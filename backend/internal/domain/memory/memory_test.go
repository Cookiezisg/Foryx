package memory

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIsValidType(t *testing.T) {
	cases := map[string]bool{
		"user":      true,
		"feedback":  true,
		"project":   true,
		"reference": true,
		"":          false,
		"other":     false,
		"USER":      false,
	}
	for v, want := range cases {
		if got := IsValidType(v); got != want {
			t.Errorf("IsValidType(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestListTypes_EquivalentToIsValid(t *testing.T) {
	types := ListTypes()
	if len(types) != 4 {
		t.Fatalf("ListTypes length = %d, want 4", len(types))
	}
	for _, ty := range types {
		if !IsValidType(ty) {
			t.Errorf("ListTypes returned %q but IsValidType says invalid", ty)
		}
	}
}

func TestIsValidSource(t *testing.T) {
	cases := map[string]bool{
		"user":  true,
		"ai":    true,
		"":      false,
		"agent": false,
	}
	for v, want := range cases {
		if got := IsValidSource(v); got != want {
			t.Errorf("IsValidSource(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestNameRegex(t *testing.T) {
	valid := []string{
		"user_role",
		"a",
		"my_python_3_12",
		"feedback_no_emoji",
	}
	invalid := []string{
		"",
		"User_role",            // uppercase start
		"_underscore_start",    // underscore start
		"1digit_start",         // digit start
		"with space",
		"with-dash",
		"with.dot",
		strings.Repeat("x", 65), // too long
	}
	for _, s := range valid {
		if !NameRegex.MatchString(s) {
			t.Errorf("NameRegex should match %q", s)
		}
	}
	for _, s := range invalid {
		if NameRegex.MatchString(s) {
			t.Errorf("NameRegex should NOT match %q", s)
		}
	}
}

func TestSentinels_UniqueAndPrefixed(t *testing.T) {
	sentinels := []error{ErrNotFound, ErrNameConflict, ErrInvalidName}
	seen := make(map[string]bool, len(sentinels))
	for _, e := range sentinels {
		msg := e.Error()
		if !strings.HasPrefix(msg, "memory: ") {
			t.Errorf("sentinel %q must start with 'memory: '", msg)
		}
		if seen[msg] {
			t.Errorf("duplicate sentinel message: %q", msg)
		}
		seen[msg] = true
	}
}

func TestMemory_JSONRoundTrip(t *testing.T) {
	m := Memory{
		ID:          "mem_abc123",
		Name:        "user_role",
		Type:        TypeUser,
		Description: "Go backend engineer",
		Content:     "User is a Go backend engineer working on Forgify v1.2.",
		Pinned:      true,
		Source:      SourceUser,
		Metadata:    map[string]any{"foo": "bar"},
		AccessCount: 3,
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Spot-check camelCase per §N3.
	mustContain := []string{
		`"id":"mem_abc123"`,
		`"name":"user_role"`,
		`"type":"user"`,
		`"description":"Go backend engineer"`,
		`"pinned":true`,
		`"source":"user"`,
		`"accessCount":3`,
	}
	got := string(data)
	for _, want := range mustContain {
		if !strings.Contains(got, want) {
			t.Errorf("JSON missing %q; got: %s", want, got)
		}
	}

	var back Memory
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Name != m.Name || back.Type != m.Type || back.Pinned != m.Pinned {
		t.Errorf("round-trip mismatch: %+v vs %+v", back, m)
	}
}

func TestMemory_TableName(t *testing.T) {
	if (Memory{}).TableName() != "memories" {
		t.Errorf("TableName should be 'memories'")
	}
}

package schema

import "testing"

// TestValidateFields_NormalizesAliases — regression for F5 (iteration loop): an agent's natural
// type vocabulary (integer/int/float, str, bool, dict, list) is normalized to the coarse canonical
// types instead of bouncing with "invalid type". Genuinely unknown types still reject.
func TestValidateFields_NormalizesAliases(t *testing.T) {
	fields := []Field{
		{Name: "a", Type: "integer"},
		{Name: "b", Type: "str"},
		{Name: "c", Type: "bool"},
		{Name: "d", Type: "list"},
		{Name: "e", Type: "dict"},
		{Name: "f", Type: "number"}, // already canonical
	}
	if err := ValidateFields(fields); err != nil {
		t.Fatalf("authoring aliases should be accepted + normalized: %v", err)
	}
	want := []string{"number", "string", "boolean", "array", "object", "number"}
	for i, f := range fields {
		if f.Type != want[i] {
			t.Errorf("field %s normalized to %q, want %q", f.Name, f.Type, want[i])
		}
	}

	if err := ValidateFields([]Field{{Name: "x", Type: "frobnicate"}}); err == nil {
		t.Fatal("a genuinely unknown type must still reject")
	}
}

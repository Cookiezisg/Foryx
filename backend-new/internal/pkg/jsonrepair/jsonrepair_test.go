package jsonrepair

import (
	"encoding/json"
	"testing"
)

func TestRepair_Empty(t *testing.T) {
	if got := Repair(""); got != "" {
		t.Errorf("Repair(\"\") = %q, want empty", got)
	}
}

func TestRepair_AlreadyValid(t *testing.T) {
	valid := `{"a":1,"b":[2,3],"c":"x"}`
	if got := Repair(valid); got != valid {
		t.Errorf("Repair(valid) = %q, want unchanged", got)
	}
}

func TestRepair_EscapesLiteralNewlineInString(t *testing.T) {
	// Literal newline byte inside a JSON string is invalid; repair escapes it.
	in := "{\"a\":\"line1\nline2\"}"
	got := Repair(in)
	if !json.Valid([]byte(got)) {
		t.Fatalf("repaired output not valid JSON: %q", got)
	}
	var out struct{ A string }
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.A != "line1\nline2" {
		t.Errorf("A = %q, want line1<newline>line2", out.A)
	}
}

func TestRepair_EscapesTabAndCarriageReturn(t *testing.T) {
	in := "{\"a\":\"x\ty\rz\"}"
	got := Repair(in)
	if !json.Valid([]byte(got)) {
		t.Fatalf("repaired output not valid JSON: %q", got)
	}
}

func TestRepair_BalancesMissingBrace(t *testing.T) {
	got := Repair(`{"a":1`)
	if !json.Valid([]byte(got)) {
		t.Fatalf("not valid: %q", got)
	}
	if got != `{"a":1}` {
		t.Errorf("got %q, want {\"a\":1}", got)
	}
}

func TestRepair_BalancesMissingBracket(t *testing.T) {
	got := Repair(`{"a":[1,2`)
	if !json.Valid([]byte(got)) {
		t.Fatalf("not valid: %q", got)
	}
	if got != `{"a":[1,2]}` {
		t.Errorf("got %q, want {\"a\":[1,2]}", got)
	}
}

func TestRepair_CombinedControlAndBrackets(t *testing.T) {
	// Literal newline in string AND a missing closing brace.
	in := "{\"a\":\"line1\nline2\""
	got := Repair(in)
	if !json.Valid([]byte(got)) {
		t.Fatalf("repaired not valid: %q", got)
	}
	var out struct{ A string }
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.A != "line1\nline2" {
		t.Errorf("A = %q", out.A)
	}
}

func TestRepair_UnrepairableReturnedUnchanged(t *testing.T) {
	in := `{"a":}` // missing value — not one of the two handled modes
	if got := Repair(in); got != in {
		t.Errorf("Repair(%q) = %q, want unchanged", in, got)
	}
}

func TestRepair_PreservesValidEscapes(t *testing.T) {
	// Already-valid escaped tab — fast path returns unchanged.
	in := `{"a":"tab\tok"}`
	if got := Repair(in); got != in {
		t.Errorf("Repair(%q) = %q, want unchanged", in, got)
	}
}

func TestRepairBytes(t *testing.T) {
	got := RepairBytes([]byte(`{"a":1`))
	if !json.Valid(got) {
		t.Fatalf("not valid: %s", got)
	}
	if string(got) != `{"a":1}` {
		t.Errorf("got %s, want {\"a\":1}", got)
	}
}

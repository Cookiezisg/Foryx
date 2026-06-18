package cel

import (
	"strings"
	"testing"
)

// TestCompileFor — F14 (iteration loop): CompileFor restricts an expression to EXACTLY the given
// root variables (no auto-ctx), so author-time validation of a context-restricted entity CEL
// (control/approval read input only; sensor reads payload only) rejects a wrong-namespace ref at
// create/edit instead of letting the permissive package env accept it and fail at runtime.
func TestCompileFor(t *testing.T) {
	ok := []struct {
		roots []string
		expr  string
	}{
		{[]string{"input"}, "input.score >= 0.9"},
		{[]string{"input"}, "has(input.n) ? input.n : 0"},
		{[]string{"payload"}, "payload.value > 0"},
	}
	for _, c := range ok {
		if _, err := CompileFor(c.roots, c.expr); err != nil {
			t.Errorf("CompileFor(%v, %q) should compile, got %v", c.roots, c.expr, err)
		}
	}

	bad := []struct {
		roots []string
		expr  string
	}{
		{[]string{"input"}, "payload.x > 0"}, // wrong root for control/approval
		{[]string{"input"}, "ctx.runId"},     // no auto-ctx
		{[]string{"payload"}, "input.x"},     // wrong root for sensor
	}
	for _, c := range bad {
		if _, err := CompileFor(c.roots, c.expr); err == nil {
			t.Errorf("CompileFor(%v, %q) must fail (out-of-namespace root)", c.roots, c.expr)
		}
	}
}

// TestEval_NoSuchOverloadHint — F49 (round-4 malformed lane): natural arithmetic mixing a payload/
// result number (binds as a CEL double) with an integer literal fails with a bare cel-go
// "no such overload"; Eval now appends an actionable cast hint so an agent doesn't burn turns + flowrun
// versions guessing (the lane saw 4 wasted versions on exactly "start.n + 5").
func TestEval_NoSuchOverloadHint(t *testing.T) {
	prg, err := CompileFor([]string{"start"}, "start.n + 5")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	_, err = prg.Eval(map[string]any{"start": map[string]any{"n": float64(2)}})
	if err == nil {
		t.Fatal("double + int arithmetic should error (cel-go no-such-overload)")
	}
	if !strings.Contains(err.Error(), "cast one") {
		t.Fatalf("eval error must carry the actionable cast hint, got: %v", err)
	}
	// A clean expression still evals with no hint noise.
	prg2, _ := CompileFor([]string{"start"}, "int(start.n) + 5")
	if got, err := prg2.Eval(map[string]any{"start": map[string]any{"n": float64(2)}}); err != nil || got != int64(7) {
		t.Fatalf("int(start.n)+5 should eval to 7, got %v err=%v", got, err)
	}
}

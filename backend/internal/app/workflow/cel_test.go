package workflow

import "testing"

func toI64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return -1
}

// case.when is a bare CEL boolean guard; first-true-wins is the interpreter's job, this proves
// the guard evaluates correctly (ADR-011, 04 §expression).
func TestCompileCEL_BoolGuard(t *testing.T) {
	p, err := CompileCEL("payload.attempt > 5")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	got, err := p.EvalBool(map[string]any{"attempt": 6}, nil)
	if err != nil || !got {
		t.Fatalf("attempt=6 >5: got %v err %v", got, err)
	}
	got, err = p.EvalBool(map[string]any{"attempt": 3}, nil)
	if err != nil || got {
		t.Fatalf("attempt=3 >5: got %v err %v want false", got, err)
	}
}

// emit / tool.args are bare CEL producing typed values (00: payload.x+1 → number 6, not "6").
func TestCompileCEL_TypedArithmetic(t *testing.T) {
	p, err := CompileCEL("payload.x + 1")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	got, err := p.Eval(map[string]any{"x": 5}, nil)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if toI64(got) != 6 {
		t.Fatalf("x+1 with x=5: got %v (%T) want 6", got, got)
	}
}

// G9 fail-to-false: a guard touching a missing field errors; the interpreter treats that as false.
func TestEvalBool_MissingField_Errors(t *testing.T) {
	p, err := CompileCEL("payload.nope > 5")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if _, err := p.EvalBool(map[string]any{}, nil); err == nil {
		t.Fatal("missing field must error so the caller applies fail-to-false (G9)")
	}
}

// 00 §determinism: now()/wall-clock must be unavailable — control flow reading the current
// moment would diverge on replay. The env exposes no such function.
func TestCompileCEL_NoWallClock(t *testing.T) {
	if _, err := CompileCEL("now()"); err == nil {
		t.Fatal("now() must not compile (would break replay determinism)")
	}
}

// emit can build nested structures; refToGo must convert CEL lists/maps to native Go.
func TestEval_NestedListMap(t *testing.T) {
	p, err := CompileCEL(`{"items": [payload.a, payload.a + 1]}`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	got, err := p.Eval(map[string]any{"a": 10}, nil)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("want map, got %T", got)
	}
	items, ok := m["items"].([]any)
	if !ok || len(items) != 2 || toI64(items[0]) != 10 || toI64(items[1]) != 11 {
		t.Fatalf("nested items wrong: %#v", m["items"])
	}
}

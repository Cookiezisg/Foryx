package tool

import (
	"context"
	"errors"
	"testing"
)

type fakeCounter struct {
	n   int
	err error
}

func (f fakeCounter) CountDependents(context.Context, string, string) (int, error) {
	return f.n, f.err
}

// TestDependentCount_NilAndErrorSafe: a delete must never fail because the advisory count read did.
func TestDependentCount_NilAndErrorSafe(t *testing.T) {
	ctx := context.Background()
	if got := DependentCount(ctx, nil, "function", "fn_1"); got != 0 {
		t.Fatalf("nil counter = %d, want 0", got)
	}
	if got := DependentCount(ctx, fakeCounter{n: 5}, "function", "fn_1"); got != 5 {
		t.Fatalf("counter(5) = %d, want 5", got)
	}
	if got := DependentCount(ctx, fakeCounter{err: errors.New("db down")}, "function", "fn_1"); got != 0 {
		t.Fatalf("counter error = %d, want 0 (advisory; delete must not fail)", got)
	}
}

// TestAnnotateDependents: deps>0 folds in dependents + note; deps==0 leaves the map untouched (no
// false alarm on a delete of something nothing referenced).
func TestAnnotateDependents(t *testing.T) {
	withDeps := AnnotateDependents(map[string]any{"id": "fn_1", "deleted": true}, 3)
	if withDeps["dependents"] != 3 {
		t.Fatalf("dependents = %v, want 3", withDeps["dependents"])
	}
	if _, ok := withDeps["note"]; !ok {
		t.Fatal("a positive count must add a repair note")
	}

	noDeps := AnnotateDependents(map[string]any{"id": "fn_1", "deleted": true}, 0)
	if _, ok := noDeps["dependents"]; ok {
		t.Fatal("zero dependents must not add the dependents key (no false alarm)")
	}
	if _, ok := noDeps["note"]; ok {
		t.Fatal("zero dependents must not add a note")
	}
}

// TestDependentSuffix: the string-result counterpart — non-empty only when there are dependents.
func TestDependentSuffix(t *testing.T) {
	if s := DependentSuffix(0); s != "" {
		t.Fatalf("zero suffix = %q, want empty", s)
	}
	if s := DependentSuffix(-1); s != "" {
		t.Fatalf("negative suffix = %q, want empty", s)
	}
	if s := DependentSuffix(2); s == "" {
		t.Fatal("positive count must produce a non-empty suffix")
	}
}

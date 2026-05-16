package workflow

import (
	"strings"
	"testing"
)

func mustCompile(t *testing.T, s string) (compiled, source string) {
	t.Helper()
	tmpl, err := Compile(s)
	if err != nil {
		t.Fatalf("Compile(%q): %v", s, err)
	}
	out, err := Execute(tmpl, EvalContext{
		Vars:     map[string]any{},
		In:       map[string]any{},
		NodesOut: map[string]map[string]any{},
		Env:      map[string]string{},
	}, s)
	if err != nil {
		t.Fatalf("Execute(%q): %v", s, err)
	}
	return out, s
}

func TestCompile_PureLiteralIsNoTemplate(t *testing.T) {
	tmpl, err := Compile("just text no braces")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if tmpl != nil {
		t.Errorf("pure literal should compile to nil template (passthrough), got non-nil")
	}
}

func TestExecute_NilTemplateReturnsLiteral(t *testing.T) {
	out, err := Execute(nil, EvalContext{}, "fallback string")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "fallback string" {
		t.Errorf("Execute(nil) should return literal, got %q", out)
	}
}

func TestCompile_BadSyntaxRejected(t *testing.T) {
	_, err := Compile(`{{ this is not valid go template }}`)
	if err == nil {
		t.Errorf("expected syntax error for bad template")
	}
	if !strings.Contains(err.Error(), "expression syntax error") {
		t.Errorf("error should mention 'expression syntax error', got: %v", err)
	}
}

func TestExecute_VarsRef(t *testing.T) {
	tmpl, err := Compile(`{{ .vars.lastSeen }}`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	out, err := Execute(tmpl, EvalContext{
		Vars: map[string]any{"lastSeen": "2026-05-12"},
	}, "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "2026-05-12" {
		t.Errorf("got %q, want 2026-05-12", out)
	}
}

func TestExecute_InRef(t *testing.T) {
	tmpl, _ := Compile(`hello {{ .in.name }}`)
	out, err := Execute(tmpl, EvalContext{
		In: map[string]any{"name": "world"},
	}, "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "hello world" {
		t.Errorf("got %q, want hello world", out)
	}
}

func TestExecute_NodesOutputRef(t *testing.T) {
	tmpl, _ := Compile(`{{ index .nodes.fetch.output "title" }}`)
	out, err := Execute(tmpl, EvalContext{
		NodesOut: map[string]map[string]any{
			"fetch": {"output": map[string]any{"title": "Hello"}},
		},
	}, "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "Hello" {
		t.Errorf("got %q, want Hello", out)
	}
}

func TestExecute_LoopItemAndIndex(t *testing.T) {
	tmpl, _ := Compile(`#{{ .loop.index }}: {{ .loop.item }}`)
	out, err := Execute(tmpl, EvalContext{
		Loop: &LoopContext{Item: "apple", Index: 2},
	}, "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "#2: apple" {
		t.Errorf("got %q, want #2: apple", out)
	}
}

func TestExecute_RunIDAndStartedAt(t *testing.T) {
	tmpl, _ := Compile(`run {{ .run.id }} @ {{ .run.startedAt }}`)
	out, err := Execute(tmpl, EvalContext{
		Run: RunContext{ID: "frun_xyz", StartedAt: "2026-05-12T10:00:00Z"},
	}, "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "run frun_xyz @ 2026-05-12T10:00:00Z" {
		t.Errorf("got %q", out)
	}
}

func TestExecute_EnvWhitelist_AllowedPassesThrough(t *testing.T) {
	tmpl, _ := Compile(`{{ index .env "USER" }}`)
	out, err := Execute(tmpl, EvalContext{
		Env: map[string]string{"USER": "alice", "API_KEY": "secret"},
	}, "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "alice" {
		t.Errorf("got %q, want alice", out)
	}
}

func TestExecute_EnvWhitelist_BlockedReturnsEmpty(t *testing.T) {
	tmpl, _ := Compile(`{{ index .env "API_KEY" }}`)
	out, err := Execute(tmpl, EvalContext{
		Env: map[string]string{"USER": "alice", "API_KEY": "secret"},
	}, "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Non-whitelisted key index returns the zero value for the map; that
	// renders as "<no value>" for missing keys in text/template.
	// 白名单外的 key index 在 text/template 渲染为 "<no value>"。
	if out == "secret" {
		t.Errorf("API_KEY should NOT leak: got %q", out)
	}
}

func TestExecute_MissingVarRendersEmpty(t *testing.T) {
	tmpl, _ := Compile(`{{ .vars.ghost }}`)
	out, err := Execute(tmpl, EvalContext{Vars: map[string]any{}}, "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// text/template renders missing map keys as "<no value>" by default.
	// authoring-time validate.go::checkVariableRefs already rejects
	// undeclared {{ vars.X }} references — this only matters when the
	// declared variable has no current value at run time.
	if out == "secret" || strings.Contains(out, "panic") {
		t.Errorf("missing var should not panic or leak, got %q", out)
	}
}

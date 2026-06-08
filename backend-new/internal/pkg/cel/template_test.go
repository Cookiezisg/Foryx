package cel

import (
	"strings"
	"testing"
)

func TestCompileTemplate_OK(t *testing.T) {
	cases := []string{
		"no spans at all",
		"hello {{ input.name }}",
		"{{ input.a }} and {{ input.b }}",
		"{{ input.items.size() }} items",
		"", // empty template is valid
	}
	for _, c := range cases {
		if _, err := CompileTemplate(c); err != nil {
			t.Errorf("CompileTemplate(%q) unexpected error: %v", c, err)
		}
	}
}

func TestCompileTemplate_Errors(t *testing.T) {
	cases := []string{
		"unterminated {{ input.x", // no closing }}
		"bad expr {{ input.( }}",  // syntax error in span
		"wall clock {{ now() }}",  // unknown function (no now())
	}
	for _, c := range cases {
		if _, err := CompileTemplate(c); err == nil {
			t.Errorf("CompileTemplate(%q) expected error, got nil", c)
		}
	}
}

func TestTemplate_Render(t *testing.T) {
	tmpl, err := CompileTemplate("批准对 {{ input.user.name }} 的退款 {{ input.amount }} 元?")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	out, err := tmpl.Render(map[string]any{"input": map[string]any{
		"user":   map[string]any{"name": "alice"},
		"amount": 42,
	}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, "alice") || !strings.Contains(out, "42") {
		t.Fatalf("render output: %q", out)
	}
}

func TestTemplate_RenderPureLiteral(t *testing.T) {
	tmpl, _ := CompileTemplate("是否批准发送?")
	out, err := tmpl.Render(nil)
	if err != nil || out != "是否批准发送?" {
		t.Fatalf("pure literal render: %q err=%v", out, err)
	}
}

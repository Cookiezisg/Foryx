package sandbox

import (
	"strings"
	"testing"
)

func TestRenderPyproject_Basic(t *testing.T) {
	got := renderPyproject("f_abc", []string{"pandas>=2.0", "requests"}, ">=3.12", ">=3.12")

	must := []string{
		`name = "forge-f_abc"`,
		`version = "0.0.0"`,
		`requires-python = ">=3.12"`,
		`"pandas>=2.0",`,
		`"requests",`,
	}
	for _, s := range must {
		if !strings.Contains(got, s) {
			t.Errorf("output missing %q\n--- output ---\n%s", s, got)
		}
	}
}

func TestRenderPyproject_NoDeps(t *testing.T) {
	got := renderPyproject("f_x", nil, ">=3.12", ">=3.12")
	if !strings.Contains(got, "dependencies = [\n]") {
		t.Errorf("empty deps should produce 'dependencies = [\\n]', got:\n%s", got)
	}
}

func TestRenderPyproject_BlankDepsFiltered(t *testing.T) {
	got := renderPyproject("f_x", []string{"pandas", "", "  "}, ">=3.12", ">=3.12")
	// Only one dep should appear (the non-blank one).
	// 只该出现一条 dep（非空那条）。
	if !strings.Contains(got, `"pandas",`) {
		t.Errorf("expected 'pandas' dep to be present, got:\n%s", got)
	}
	if strings.Count(got, `,`) != 1 {
		// One comma per dep entry; blank deps should produce zero entries.
		// 每条 dep 一个逗号；空 dep 不该产生条目。
		t.Errorf("expected exactly 1 dep entry (1 comma), got:\n%s", got)
	}
}

func TestRenderPyproject_PerVersionPythonOverridesDefault(t *testing.T) {
	got := renderPyproject("f_x", nil, ">=3.13", ">=3.12")
	if !strings.Contains(got, `requires-python = ">=3.13"`) {
		t.Errorf("per-version python should win over default, got:\n%s", got)
	}
}

func TestRenderPyproject_FallsBackToDefaultPython(t *testing.T) {
	got := renderPyproject("f_x", nil, "", ">=3.11")
	if !strings.Contains(got, `requires-python = ">=3.11"`) {
		t.Errorf("empty per-version should fall back to default, got:\n%s", got)
	}
}

func TestRenderPyproject_HardcodedFallback(t *testing.T) {
	got := renderPyproject("f_x", nil, "", "")
	if !strings.Contains(got, `requires-python = ">=3.12"`) {
		t.Errorf("both empty should fall back to >=3.12, got:\n%s", got)
	}
}

func TestRenderPyproject_QuoteEscape(t *testing.T) {
	// strconv.Quote handles double quotes and backslashes — even an attacker
	// shoving '"]\\nmalicious' into a specifier can't escape the field.
	// strconv.Quote 处理双引号和反斜杠——攻击者塞 '"]\\nmalicious' 也无法
	// 突破字段边界。
	got := renderPyproject("f_x", []string{`evil"]\nmalicious`}, ">=3.12", ">=3.12")
	// The malicious payload must remain inside the quoted string — finding
	// the literal Go-escaped form means strconv.Quote did its job.
	if !strings.Contains(got, `"evil\"]\\nmalicious"`) {
		t.Errorf("dep specifier should be quote-escaped, got:\n%s", got)
	}
}

func TestRenderPyproject_DepWithExtras(t *testing.T) {
	got := renderPyproject("f_x", []string{"pandas[excel]>=2.0"}, ">=3.12", ">=3.12")
	if !strings.Contains(got, `"pandas[excel]>=2.0",`) {
		t.Errorf("dep with extras should pass through verbatim, got:\n%s", got)
	}
}

func TestRenderPyproject_PythonVersionWhitespaceTrimmed(t *testing.T) {
	got := renderPyproject("f_x", nil, "  >=3.12  ", "")
	if !strings.Contains(got, `requires-python = ">=3.12"`) {
		t.Errorf("python version whitespace should be trimmed, got:\n%s", got)
	}
}

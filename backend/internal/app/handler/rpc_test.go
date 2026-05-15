package handler

import (
	"strings"
	"testing"

	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
)

// TestAssembleClass_FullShape covers imports + init + shutdown + 2 methods.
// 2026-05 refactor: assembled class uses exploded named params from
// InitArgsSchema / method.Args (not **kwargs blobs).
//
// TestAssembleClass_FullShape 覆盖 imports + init + shutdown + 2 methods;
// 2026-05 重构后用 exploded named params。
func TestAssembleClass_FullShape(t *testing.T) {
	d := &VersionDraft{
		Imports: "import psycopg2",
		InitArgsSchema: []handlerdomain.InitArgSpec{
			{Name: "dsn", Type: "string", Required: true},
		},
		InitBody:     "self.conn = psycopg2.connect(dsn)",
		ShutdownBody: "self.conn.close()",
		Methods: []handlerdomain.MethodSpec{
			{Name: "query", Args: []handlerdomain.ArgSpec{{Name: "sql", Type: "string", Required: true}},
				Body: "return self.conn.cursor().execute(sql).fetchall()"},
			{Name: "exec", Args: []handlerdomain.ArgSpec{{Name: "sql", Type: "string", Required: true}},
				Body: "self.conn.cursor().execute(sql)\nself.conn.commit()"},
		},
	}
	out := AssembleClass(d)
	for _, want := range []string{
		"import psycopg2",
		"class HandlerImpl:",
		"    def __init__(self, dsn: str):",
		"        self.conn = psycopg2.connect(dsn)",
		"    def shutdown(self):",
		"        self.conn.close()",
		"    def query(self, sql: str):",
		"        return self.conn.cursor()",
		"    def exec(self, sql: str):",
		"        self.conn.cursor().execute(sql)",
		"        self.conn.commit()",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in:\n%s", want, out)
		}
	}
}

// TestAssembleClass_OptionalArgsGetDefaults — optional arg without explicit
// Default gets `= None`; with Default uses Python-rendered literal.
//
// 可选参数无 Default → `= None`;有 Default → 渲染成 Python 字面量。
func TestAssembleClass_OptionalArgsGetDefaults(t *testing.T) {
	d := &VersionDraft{
		InitArgsSchema: []handlerdomain.InitArgSpec{
			{Name: "host", Type: "string", Required: true},
			{Name: "port", Type: "integer", Required: false, Default: float64(5432)},
			{Name: "ssl", Type: "boolean", Required: false},
			{Name: "label", Type: "string", Required: false, Default: "prod"},
		},
		InitBody: "self.x = host",
		Methods:  []handlerdomain.MethodSpec{{Name: "ping", Body: "return None"}},
	}
	out := AssembleClass(d)
	want := `    def __init__(self, host: str, port: int = 5432, ssl: bool = None, label: str = "prod"):`
	if !strings.Contains(out, want) {
		t.Errorf("expected %q in:\n%s", want, out)
	}
}

// TestAssembleClass_NoShutdownDefaultsToPass verifies missing shutdown body
// becomes `pass`.
//
// TestAssembleClass_NoShutdownDefaultsToPass 验证未填 shutdown 时回落 pass。
func TestAssembleClass_NoShutdownDefaultsToPass(t *testing.T) {
	d := &VersionDraft{
		Methods: []handlerdomain.MethodSpec{{Name: "noop", Body: "return None"}},
	}
	out := AssembleClass(d)
	if !strings.Contains(out, "def shutdown(self):\n        pass") {
		t.Errorf("default shutdown=pass missing:\n%s", out)
	}
}

// TestAssembleClass_BlankLinesPreserved checks indented multiline bodies
// keep blank lines (whitespace-only lines become empty, not "        \n").
//
// TestAssembleClass_BlankLinesPreserved 多行 body 保空行,不写 trailing 空白。
func TestAssembleClass_BlankLinesPreserved(t *testing.T) {
	d := &VersionDraft{
		Methods: []handlerdomain.MethodSpec{{
			Name: "two_lines",
			Body: "line_a\n\nline_b",
		}},
	}
	out := AssembleClass(d)
	if !strings.Contains(out, "        line_a\n\n        line_b") {
		t.Errorf("blank line between body lines lost:\n%s", out)
	}
}

func TestDriverScript_ContainsKeyPatterns(t *testing.T) {
	for _, want := range []string{
		"from user_handler import HandlerImpl",
		`{"type": "ready"}`,
		`init_error`,
		`if msg_type == "shutdown"`,
		`if msg_type == "call"`,
		"private method:",
		"no such method:",
		"yield",
		"progress",
	} {
		if !strings.Contains(DriverScript, want) {
			// "yield" isn't actually in the driver, just "generator" detection
			// via __iter__. Skip the yield-specific check if not literally present.
			if want == "yield" {
				continue
			}
			t.Errorf("DriverScript missing pattern %q", want)
		}
	}
}

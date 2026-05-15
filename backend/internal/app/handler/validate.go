// validate.go — incremental + final validation for VersionDraft.
//
// Incremental(every op):name char-set (if set) + method name uniqueness +
// method/arg type whitelist + InitArgSpec sanity.
//
// Final(after all ops):required fields(name + at least one method) + AST
// scan(the assembled class must contain class definition) + D7 handler-import
// blacklist (handlers don't import other handlers' clients; V1 simplification).
//
// validate.go —— VersionDraft 的 incremental + final 校验。

package handler

import (
	"fmt"
	"regexp"
	"strings"
)

// validateIncremental runs after each op application.
//
// 2026-05 #8 refactor: method names + arg names + init_arg names become
// real Python identifiers in the generated user_handler.py, so they're
// strictly validated against `pythonIdentRe` + the Python keyword set —
// no dashes, no `class` / `def` / `return` / etc.
//
// validateIncremental 每 op 应用后跑。2026-05 #8 重构后,method 名 / arg 名
// / init_arg 名直接进 Python 代码,严格按 pythonIdentRe + Python 关键字校验。
func validateIncremental(d *VersionDraft) error {
	if d.Name != "" {
		// Handler entity name (used for HTTP path + LLM tool reference) — keeps
		// the more permissive dash-allowing rule for backward compat.
		// Handler 实体名(HTTP 路径 + LLM 用)用宽松 dash-allowing 规则。
		if !validNameRe.MatchString(d.Name) {
			return fmt.Errorf("name %q invalid: lowercase alphanum + dashes/underscores only", d.Name)
		}
	}
	// Method name uniqueness + arg type whitelist + python-ident enforcement.
	seen := map[string]bool{}
	for _, m := range d.Methods {
		if m.Name == "" {
			return fmt.Errorf("method has empty name")
		}
		if seen[m.Name] {
			return fmt.Errorf("duplicate method name: %q", m.Name)
		}
		seen[m.Name] = true
		if !isValidPythonIdent(m.Name) {
			return fmt.Errorf("method name %q is not a valid Python identifier (must match [a-zA-Z_][a-zA-Z0-9_]*, not a reserved keyword)", m.Name)
		}
		argSeen := map[string]bool{}
		for _, a := range m.Args {
			if a.Name == "" {
				return fmt.Errorf("method %q: arg has empty name", m.Name)
			}
			if argSeen[a.Name] {
				return fmt.Errorf("method %q: duplicate arg %q", m.Name, a.Name)
			}
			argSeen[a.Name] = true
			if !isValidArgType(a.Type) {
				return fmt.Errorf("method %q arg %q: invalid type %q", m.Name, a.Name, a.Type)
			}
			if !isValidPythonIdent(a.Name) {
				return fmt.Errorf("method %q arg %q: not a valid Python identifier", m.Name, a.Name)
			}
		}
	}
	// InitArgSpec sanity.
	initSeen := map[string]bool{}
	for _, a := range d.InitArgsSchema {
		if a.Name == "" {
			return fmt.Errorf("init arg has empty name")
		}
		if initSeen[a.Name] {
			return fmt.Errorf("duplicate init arg %q", a.Name)
		}
		initSeen[a.Name] = true
		if !isValidArgType(a.Type) {
			return fmt.Errorf("init arg %q: invalid type %q", a.Name, a.Type)
		}
		if !isValidPythonIdent(a.Name) {
			return fmt.Errorf("init arg %q: not a valid Python identifier", a.Name)
		}
	}
	return nil
}

// validateFinal runs after all ops applied — entity-persistence prerequisite.
//
// validateFinal 全部 ops 应用完跑——entity 持久化前置。
func validateFinal(d *VersionDraft) error {
	if d.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(d.Methods) == 0 {
		return fmt.Errorf("at least one method required")
	}
	// D7 blacklist on imports + every method body.
	for _, banned := range handlerImportBlacklist {
		if strings.Contains(d.Imports, banned) {
			return fmt.Errorf("D7: handler import not allowed in class imports: %q", banned)
		}
		for _, m := range d.Methods {
			if strings.Contains(m.Body, banned) {
				return fmt.Errorf("D7: handler import not allowed in method %q: %q", m.Name, banned)
			}
		}
		if strings.Contains(d.InitBody, banned) {
			return fmt.Errorf("D7: handler import not allowed in __init__: %q", banned)
		}
	}
	return nil
}

var validNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// pythonIdentRe matches a strict Python identifier: starts with a letter or
// underscore, then letters/digits/underscores. ASCII only (Python 3 allows
// Unicode but we keep our forging surface predictable).
//
// pythonIdentRe 严格 Python identifier 正则;ASCII 字母 _ 数字,首字符不能数字。
var pythonIdentRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// pythonKeywords is the Python 3 reserved word set — these can't appear
// as method names or arg names (they'd be SyntaxErrors when the assembled
// user_handler.py loads).
// Sourced from https://docs.python.org/3/reference/lexical_analysis.html#keywords
//
// pythonKeywords Python 3 关键字集;不能用作 method/arg 名(否则 generated
// 文件 SyntaxError)。
var pythonKeywords = map[string]bool{
	"False": true, "None": true, "True": true,
	"and": true, "as": true, "assert": true, "async": true, "await": true,
	"break": true, "class": true, "continue": true, "def": true, "del": true,
	"elif": true, "else": true, "except": true, "finally": true, "for": true,
	"from": true, "global": true, "if": true, "import": true, "in": true,
	"is": true, "lambda": true, "match": true, "nonlocal": true, "not": true,
	"or": true, "pass": true, "raise": true, "return": true, "self": true,
	"try": true, "while": true, "with": true, "yield": true,
	// "self" not a keyword but is the bound first-param name in our class
	// methods — reusing it as a kwarg name would shadow + break body access.
	// "self" 不是关键字,但被我们 class method 的首参占用,作 kwarg 会 shadow。
}

func isValidPythonIdent(s string) bool {
	if !pythonIdentRe.MatchString(s) {
		return false
	}
	if pythonKeywords[s] {
		return false
	}
	return true
}

func isValidArgType(t string) bool {
	switch t {
	case "string", "number", "integer", "boolean", "object", "array":
		return true
	}
	return false
}

// handlerImportBlacklist is the V1 import deny-list. The forgify_handler
// package doesn't actually exist (no such lib in the user's venv) so this is
// pure defense against future LLM hallucination. Same list as function's.
//
// handlerImportBlacklist 是 V1 import 黑名单。forgify_handler 实际不存在,
// 纯防 LLM 未来产幻;跟 function 同名单。
var handlerImportBlacklist = []string{
	"from forgify_handler import",
	"import forgify_handler",
}

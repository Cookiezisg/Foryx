package handler

import (
	"fmt"
	"regexp"
	"strings"
)

func validateIncremental(d *VersionDraft) error {
	if d.Name != "" {
		if !validNameRe.MatchString(d.Name) {
			return fmt.Errorf("name %q invalid: lowercase alphanum + dashes/underscores only", d.Name)
		}
	}
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

func validateFinal(d *VersionDraft) error {
	if d.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(d.Methods) == 0 {
		return fmt.Errorf("at least one method required")
	}
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

var pythonIdentRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// pythonKeywords is the Python 3 reserved word set plus `self` (our bound first-param).
//
// pythonKeywords Python 3 关键字集，外加我们 class method 首参 `self`。
var pythonKeywords = map[string]bool{
	"False": true, "None": true, "True": true,
	"and": true, "as": true, "assert": true, "async": true, "await": true,
	"break": true, "class": true, "continue": true, "def": true, "del": true,
	"elif": true, "else": true, "except": true, "finally": true, "for": true,
	"from": true, "global": true, "if": true, "import": true, "in": true,
	"is": true, "lambda": true, "match": true, "nonlocal": true, "not": true,
	"or": true, "pass": true, "raise": true, "return": true, "self": true,
	"try": true, "while": true, "with": true, "yield": true,
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

var handlerImportBlacklist = []string{
	"from forgify_handler import",
	"import forgify_handler",
}

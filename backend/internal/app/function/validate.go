package function

import (
	"fmt"
	"regexp"
	"strings"

	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
)

func validateIncremental(d *VersionDraft) error {
	if d.Name != "" {
		if !validNameRe.MatchString(d.Name) {
			return fmt.Errorf("name %q invalid: lowercase alphanum + dashes/underscores only", d.Name)
		}
	}
	if len(d.Parameters) > 0 {
		seen := map[string]bool{}
		for _, p := range d.Parameters {
			if p.Name == "" {
				return fmt.Errorf("parameter has empty name")
			}
			if seen[p.Name] {
				return fmt.Errorf("duplicate parameter name: %q", p.Name)
			}
			seen[p.Name] = true
			if !isValidParamType(p.Type) {
				return fmt.Errorf("parameter %q invalid type: %q", p.Name, p.Type)
			}
		}
	}
	return nil
}

func validateFinal(d *VersionDraft) error {
	if d.Name == "" {
		return fmt.Errorf("name is required")
	}
	if d.Code == "" {
		return fmt.Errorf("code is required")
	}
	if err := scanPythonAST(d.Code, d.Name); err != nil {
		return fmt.Errorf("AST scan: %w", err)
	}
	if err := checkParamConsistency(d.Code, d.Name, d.Parameters); err != nil {
		return fmt.Errorf("param consistency: %w", err)
	}
	return nil
}

var validNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func isValidParamType(t string) bool {
	switch t {
	case "string", "number", "integer", "boolean", "object", "array":
		return true
	}
	return false
}

func scanPythonAST(code, name string) error {
	_ = name
	if !strings.Contains(code, "\ndef ") && !strings.HasPrefix(code, "def ") {
		return fmt.Errorf("code must contain at least one top-level def")
	}
	for _, blacklisted := range handlerImportBlacklist {
		if strings.Contains(code, blacklisted) {
			return fmt.Errorf("D7: handler import not allowed: %q", blacklisted)
		}
	}
	return nil
}

var handlerImportBlacklist = []string{
	"from forgify_handler import",
	"import forgify_handler",
}

func checkParamConsistency(code, name string, params []functiondomain.ParameterSpec) error {
	_ = code
	_ = name
	_ = params
	return nil
}

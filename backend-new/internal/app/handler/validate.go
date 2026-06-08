package handler

import (
	"fmt"
	"regexp"

	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
)

var validNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// validateIncremental checks invariants that must hold after every op (cheap, partial).
//
// validateIncremental 校验每个 op 后必须成立的不变式（廉价、部分）。
func validateIncremental(d *VersionDraft) error {
	if d.Name != "" && !validNameRe.MatchString(d.Name) {
		return fmt.Errorf("name %q invalid: lowercase alphanumeric + dashes/underscores, 1-64 chars", d.Name)
	}
	seenMethods := map[string]bool{}
	for _, m := range d.Methods {
		if m.Name == "" {
			return fmt.Errorf("method has empty name")
		}
		if seenMethods[m.Name] {
			return fmt.Errorf("duplicate method name: %q", m.Name)
		}
		seenMethods[m.Name] = true
		if err := schemapkg.ValidateFields(m.Inputs); err != nil {
			return fmt.Errorf("method %q inputs: %w", m.Name, err)
		}
		if err := schemapkg.ValidateFields(m.Outputs); err != nil {
			return fmt.Errorf("method %q outputs: %w", m.Name, err)
		}
	}
	seenArgs := map[string]bool{}
	for _, a := range d.InitArgsSchema {
		if a.Name == "" {
			return fmt.Errorf("init arg has empty name")
		}
		if seenArgs[a.Name] {
			return fmt.Errorf("duplicate init arg: %q", a.Name)
		}
		seenArgs[a.Name] = true
		if !validArgType(a.Type) {
			return fmt.Errorf("init arg %q invalid type: %q", a.Name, a.Type)
		}
	}
	return nil
}

// validateFinal checks the completed class draft is usable: named + at least one method.
//
// validateFinal 校验完成的类草稿可用：有名 + 至少一个方法。
func validateFinal(d *VersionDraft) error {
	if d.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(d.Methods) == 0 {
		return fmt.Errorf("a handler needs at least one method")
	}
	return nil
}

func validArgType(t string) bool {
	switch t {
	case "string", "number", "integer", "boolean", "object", "array":
		return true
	}
	return false
}

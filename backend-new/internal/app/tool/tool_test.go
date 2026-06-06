package tool

import (
	"context"
	"encoding/json"
	"testing"
)

// fakeTool is a minimal Tool for ToLLMDef / Toolset tests.
type fakeTool struct {
	name   string
	params string
}

func (f fakeTool) Name() string                                  { return f.name }
func (f fakeTool) Description() string                           { return "desc of " + f.name }
func (f fakeTool) Parameters() json.RawMessage                   { return json.RawMessage(f.params) }
func (fakeTool) ValidateInput(json.RawMessage) error             { return nil }
func (fakeTool) Execute(context.Context, string) (string, error) { return "ok", nil }

var _ Tool = fakeTool{}

const objSchema = `{"type":"object","required":["path"],"properties":{"path":{"type":"string"}}}`

func parseObj(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, raw)
	}
	return m
}

func TestInject_AddsThreeFieldsAndRequiresSummaryDanger(t *testing.T) {
	m := parseObj(t, injectStandardFields(json.RawMessage(objSchema)))
	props := m["properties"].(map[string]any)
	for _, f := range []string{"summary", "danger", "execution_group", "path"} {
		if _, ok := props[f]; !ok {
			t.Errorf("missing property %q", f)
		}
	}
	enum := props["danger"].(map[string]any)["enum"].([]any)
	if len(enum) != 3 || enum[0] != "safe" || enum[1] != "cautious" || enum[2] != "dangerous" {
		t.Errorf("danger enum = %v", enum)
	}
	req := m["required"].([]any)
	if len(req) != 3 || req[0] != "summary" || req[1] != "danger" || req[2] != "path" {
		t.Errorf("required = %v, want [summary danger path]", req)
	}
}

func TestInject_NoRequiredNoProperties(t *testing.T) {
	m := parseObj(t, injectStandardFields(json.RawMessage(`{"type":"object"}`)))
	req := m["required"].([]any)
	if len(req) != 2 || req[0] != "summary" || req[1] != "danger" {
		t.Errorf("required = %v, want [summary danger]", req)
	}
	if props := m["properties"].(map[string]any); len(props) != 3 {
		t.Errorf("properties = %v, want only the 3 injected", props)
	}
}

func TestInject_ConflictPanics(t *testing.T) {
	for _, f := range []string{"summary", "danger", "execution_group"} {
		t.Run(f, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("conflict on %q did not panic", f)
				}
			}()
			injectStandardFields(json.RawMessage(`{"type":"object","properties":{"` + f + `":{"type":"string"}}}`))
		})
	}
}

func TestInject_NonObjectPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("non-object schema did not panic")
		}
	}()
	injectStandardFields(json.RawMessage(`["not","an","object"]`))
}

func TestStrip_ExtractsAllThree(t *testing.T) {
	f, clean := StripStandardFields(`{"summary":"reading config","danger":"cautious","execution_group":2,"path":"/x"}`)
	if f.Summary != "reading config" || f.Danger != DangerCautious || f.ExecutionGroup != 2 {
		t.Errorf("fields = %+v", f)
	}
	if m := parseObj(t, json.RawMessage(clean)); len(m) != 1 || m["path"] != "/x" {
		t.Errorf("clean args = %s, want only path", clean)
	}
}

func TestStrip_DefaultsWhenMissing(t *testing.T) {
	f, clean := StripStandardFields(`{"path":"/x"}`)
	if f.Summary != "" || f.Danger != DangerSafe || f.ExecutionGroup != 0 {
		t.Errorf("defaults = %+v, want {summary:\"\" danger:safe group:0}", f)
	}
	if clean != `{"path":"/x"}` {
		t.Errorf("clean = %s", clean)
	}
}

func TestStrip_InvalidDangerFallsBackToSafe(t *testing.T) {
	if f, _ := StripStandardFields(`{"danger":"nuclear","path":"/x"}`); f.Danger != DangerSafe {
		t.Errorf("invalid danger → %q, want safe", f.Danger)
	}
}

func TestStrip_NegativeExecutionGroupNormalized(t *testing.T) {
	if f, _ := StripStandardFields(`{"execution_group":-5,"path":"/x"}`); f.ExecutionGroup != 0 {
		t.Errorf("negative exec group → %d, want 0", f.ExecutionGroup)
	}
}

func TestStrip_RepairsMalformedJSON(t *testing.T) {
	// missing closing brace — jsonrepair should recover it
	f, clean := StripStandardFields(`{"summary":"x","danger":"safe","path":"/y"`)
	if f.Summary != "x" {
		t.Errorf("repair failed to extract fields: %+v", f)
	}
	if m := parseObj(t, json.RawMessage(clean)); m["path"] != "/y" {
		t.Errorf("repaired clean args = %s", clean)
	}
}

func TestToLLMDef_InjectsAndLeavesOriginalUntouched(t *testing.T) {
	tl := fakeTool{name: "Read", params: objSchema}
	def := ToLLMDef(tl)
	if def.Name != "Read" || def.Description != "desc of Read" {
		t.Errorf("def = %+v", def)
	}
	if props := parseObj(t, def.Parameters)["properties"].(map[string]any); props["summary"] == nil {
		t.Error("ToLLMDef did not inject summary")
	}
	if string(tl.Parameters()) != objSchema {
		t.Error("ToLLMDef mutated the tool's original Parameters")
	}
}

func TestToLLMDefs_Batch(t *testing.T) {
	defs := ToLLMDefs([]Tool{
		fakeTool{name: "a", params: `{"type":"object"}`},
		fakeTool{name: "b", params: `{"type":"object"}`},
	})
	if len(defs) != 2 || defs[0].Name != "a" || defs[1].Name != "b" {
		t.Errorf("defs = %+v", defs)
	}
}

func TestIsValidDanger(t *testing.T) {
	for _, v := range []string{"safe", "cautious", "dangerous"} {
		if !IsValidDanger(v) {
			t.Errorf("%q should be valid", v)
		}
	}
	for _, v := range []string{"", "SAFE", "destructive", "high"} {
		if IsValidDanger(v) {
			t.Errorf("%q should be invalid", v)
		}
	}
}

func TestToolset_All(t *testing.T) {
	ts := Toolset{
		Resident: []Tool{fakeTool{name: "r1"}, fakeTool{name: "r2"}},
		Lazy: map[string][]Tool{
			"skill": {fakeTool{name: "s1"}},
			"mcp":   {fakeTool{name: "m1"}, fakeTool{name: "m2"}},
		},
	}
	all := ts.All()
	if len(all) != 5 {
		t.Fatalf("All() = %d tools, want 5", len(all))
	}
	if all[0].Name() != "r1" || all[1].Name() != "r2" {
		t.Errorf("resident not first: %s %s", all[0].Name(), all[1].Name())
	}
}

package tool

import (
	"context"
	"encoding/json"
	"testing"

	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

func TestInjectStandardFields_AddsAllThreeFields(t *testing.T) {
	params := json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string"}
		},
		"required": ["path"]
	}`)

	result := injectStandardFields(params)

	var schema map[string]json.RawMessage
	if err := json.Unmarshal(result, &schema); err != nil {
		t.Fatalf("result not valid JSON: %v", err)
	}

	var props map[string]json.RawMessage
	if err := json.Unmarshal(schema["properties"], &props); err != nil {
		t.Fatalf("properties not valid JSON: %v", err)
	}
	if _, ok := props["summary"]; !ok {
		t.Error("summary field not injected into properties")
	}
	if _, ok := props["destructive"]; !ok {
		t.Error("destructive field not injected into properties")
	}
	if _, ok := props["execution_group"]; !ok {
		t.Error("execution_group field not injected into properties")
	}

	var required []string
	json.Unmarshal(schema["required"], &required)
	if len(required) == 0 || required[0] != "summary" {
		t.Errorf("summary not first in required: %v", required)
	}
	if !contains(required, "path") {
		t.Error("original 'path' field removed from required")
	}
	if contains(required, "destructive") {
		t.Errorf("destructive must NOT be in required (default false): %v", required)
	}
	if contains(required, "execution_group") {
		t.Errorf("execution_group must NOT be in required (optional): %v", required)
	}
}

func TestInjectStandardFields_NoRequired(t *testing.T) {
	params := json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}}}`)
	result := injectStandardFields(params)

	var schema map[string]json.RawMessage
	json.Unmarshal(result, &schema)

	var required []string
	json.Unmarshal(schema["required"], &required)
	if !contains(required, "summary") {
		t.Errorf("summary not in required: %v", required)
	}
}

func TestInjectStandardFields_SummaryConflictPanics(t *testing.T) {
	params := json.RawMessage(`{
		"type": "object",
		"properties": {
			"summary": {"type": "string"}
		}
	}`)
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on summary conflict, got none")
		}
	}()
	injectStandardFields(params)
}

func TestInjectStandardFields_DestructiveConflictPanics(t *testing.T) {
	params := json.RawMessage(`{
		"type": "object",
		"properties": {
			"destructive": {"type": "boolean"}
		}
	}`)
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on destructive conflict, got none")
		}
	}()
	injectStandardFields(params)
}

func TestInjectStandardFields_ExecutionGroupConflictPanics(t *testing.T) {
	params := json.RawMessage(`{
		"type": "object",
		"properties": {
			"execution_group": {"type": "integer"}
		}
	}`)
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on execution_group conflict, got none")
		}
	}()
	injectStandardFields(params)
}

func TestInjectStandardFields_NonObjectPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for non-object schema, got none")
		}
	}()
	injectStandardFields(json.RawMessage(`"just a string"`))
}

func TestStripStandardFields_ExtractsAllThree(t *testing.T) {
	args := `{"summary":"Deleting all logs","destructive":true,"execution_group":2,"path":"/var/log"}`
	fields, stripped := StripStandardFields(args)

	if fields.Summary != "Deleting all logs" {
		t.Errorf("Summary = %q, want 'Deleting all logs'", fields.Summary)
	}
	if !fields.Destructive {
		t.Error("Destructive = false, want true")
	}
	if fields.ExecutionGroup != 2 {
		t.Errorf("ExecutionGroup = %d, want 2", fields.ExecutionGroup)
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(stripped), &m); err != nil {
		t.Fatalf("stripped not valid JSON: %v", err)
	}
	if _, ok := m["summary"]; ok {
		t.Error("summary still present in stripped JSON")
	}
	if _, ok := m["destructive"]; ok {
		t.Error("destructive still present in stripped JSON")
	}
	if _, ok := m["execution_group"]; ok {
		t.Error("execution_group still present in stripped JSON")
	}
	if m["path"] != "/var/log" {
		t.Errorf("path = %v, want /var/log", m["path"])
	}
}

func TestStripStandardFields_DefaultsWhenMissing(t *testing.T) {
	args := `{"path":"/etc/hosts"}`
	fields, stripped := StripStandardFields(args)

	if fields.Summary != "" {
		t.Errorf("Summary = %q, want empty", fields.Summary)
	}
	if fields.Destructive {
		t.Error("Destructive = true, want false")
	}
	if fields.ExecutionGroup != 0 {
		t.Errorf("ExecutionGroup = %d, want 0 (auto)", fields.ExecutionGroup)
	}
	var m map[string]any
	json.Unmarshal([]byte(stripped), &m)
	if m["path"] != "/etc/hosts" {
		t.Errorf("args modified unexpectedly: %s", stripped)
	}
}

func TestStripStandardFields_OnlySummary(t *testing.T) {
	args := `{"summary":"reading","path":"/tmp"}`
	fields, stripped := StripStandardFields(args)
	if fields.Summary != "reading" {
		t.Errorf("Summary = %q, want 'reading'", fields.Summary)
	}
	if fields.Destructive {
		t.Error("Destructive should default to false")
	}
	if fields.ExecutionGroup != 0 {
		t.Errorf("ExecutionGroup should default to 0, got %d", fields.ExecutionGroup)
	}
	var m map[string]any
	json.Unmarshal([]byte(stripped), &m)
	if _, ok := m["summary"]; ok {
		t.Error("summary not stripped")
	}
}

func TestStripStandardFields_OnlyDestructive(t *testing.T) {
	args := `{"destructive":true,"command":"rm"}`
	fields, stripped := StripStandardFields(args)
	if fields.Summary != "" {
		t.Errorf("Summary = %q, want empty", fields.Summary)
	}
	if !fields.Destructive {
		t.Error("Destructive = false, want true")
	}
	var m map[string]any
	json.Unmarshal([]byte(stripped), &m)
	if _, ok := m["destructive"]; ok {
		t.Error("destructive not stripped")
	}
}

func TestStripStandardFields_OnlyExecutionGroup(t *testing.T) {
	args := `{"execution_group":3,"command":"ls"}`
	fields, stripped := StripStandardFields(args)
	if fields.ExecutionGroup != 3 {
		t.Errorf("ExecutionGroup = %d, want 3", fields.ExecutionGroup)
	}
	var m map[string]any
	json.Unmarshal([]byte(stripped), &m)
	if _, ok := m["execution_group"]; ok {
		t.Error("execution_group not stripped")
	}
	if m["command"] != "ls" {
		t.Errorf("command lost: %v", m["command"])
	}
}

func TestStripStandardFields_NegativeExecutionGroupNormalizedToZero(t *testing.T) {
	args := `{"execution_group":-5,"path":"/x"}`
	fields, _ := StripStandardFields(args)
	if fields.ExecutionGroup != 0 {
		t.Errorf("ExecutionGroup = %d, want 0 (negative normalized)", fields.ExecutionGroup)
	}
}

func TestStripStandardFields_InvalidJSON(t *testing.T) {
	args := `not-json`
	fields, stripped := StripStandardFields(args)
	if fields.Summary != "" {
		t.Errorf("expected empty Summary for invalid JSON, got %q", fields.Summary)
	}
	if fields.Destructive {
		t.Error("expected Destructive=false for invalid JSON")
	}
	if fields.ExecutionGroup != 0 {
		t.Errorf("expected ExecutionGroup=0 for invalid JSON, got %d", fields.ExecutionGroup)
	}
	if stripped != args {
		t.Errorf("stripped should equal input for invalid JSON, got %q", stripped)
	}
}

type stubTool struct {
	name   string
	desc   string
	params json.RawMessage
}

func (s *stubTool) Name() string                        { return s.name }
func (s *stubTool) Description() string                 { return s.desc }
func (s *stubTool) Parameters() json.RawMessage         { return s.params }
func (s *stubTool) IsReadOnly() bool                    { return true }
func (s *stubTool) NeedsReadFirst() bool                { return false }
func (s *stubTool) RequiresWorkspace() bool             { return false }
func (s *stubTool) ValidateInput(json.RawMessage) error { return nil }
func (s *stubTool) CheckPermissions(json.RawMessage, PermissionMode) PermissionResult {
	return PermissionAllow
}
func (s *stubTool) Execute(_ context.Context, _ string) (string, error) { return "", nil }

func TestToLLMDef_AllThreeStandardFieldsInjected(t *testing.T) {
	tool := &stubTool{
		name:   "read_file",
		desc:   "Reads a file",
		params: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
	}
	def := ToLLMDef(tool)

	if def.Name != "read_file" || def.Description != "Reads a file" {
		t.Errorf("name/desc mismatch: %q %q", def.Name, def.Description)
	}

	var schema map[string]json.RawMessage
	json.Unmarshal(def.Parameters, &schema)
	var props map[string]json.RawMessage
	json.Unmarshal(schema["properties"], &props)
	if _, ok := props["summary"]; !ok {
		t.Error("ToLLMDef should inject summary into parameters")
	}
	if _, ok := props["destructive"]; !ok {
		t.Error("ToLLMDef should inject destructive into parameters")
	}
	if _, ok := props["execution_group"]; !ok {
		t.Error("ToLLMDef should inject execution_group into parameters")
	}
}

func TestToLLMDefs_BatchConversion(t *testing.T) {
	tools := []Tool{
		&stubTool{name: "t1", params: json.RawMessage(`{"type":"object","properties":{}}`)},
		&stubTool{name: "t2", params: json.RawMessage(`{"type":"object","properties":{}}`)},
	}
	defs := ToLLMDefs(tools)
	if len(defs) != 2 {
		t.Fatalf("want 2 defs, got %d", len(defs))
	}
	if defs[0].Name != "t1" || defs[1].Name != "t2" {
		t.Errorf("names = %q %q", defs[0].Name, defs[1].Name)
	}
}

func TestToLLMDef_OriginalParamsUnchanged(t *testing.T) {
	original := `{"type":"object","properties":{"path":{"type":"string"}}}`
	tool := &stubTool{params: json.RawMessage(original)}
	ToLLMDef(tool)
	if string(tool.params) != original {
		t.Errorf("original params mutated: %s", tool.params)
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func TestInjectStandardFields_DescriptionsAreSlim(t *testing.T) {
	params := json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)
	result := injectStandardFields(params)
	var schema map[string]json.RawMessage
	json.Unmarshal(result, &schema)
	var props map[string]json.RawMessage
	json.Unmarshal(schema["properties"], &props)
	for _, f := range []string{"summary", "destructive", "execution_group"} {
		var field struct {
			Description string `json:"description"`
		}
		json.Unmarshal(props[f], &field)
		if len(field.Description) > 120 {
			t.Errorf("%s description too long (%d chars); long guidance must live in tool_conventions, not per-tool schema", f, len(field.Description))
		}
	}
}

var _ Tool = (*stubTool)(nil)
var _ llminfra.ToolDef = llminfra.ToolDef{}

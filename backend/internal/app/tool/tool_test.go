// tool_test.go — unit tests for Tool interface utilities:
// injectStandardFields, StripStandardFields, ToLLMDef.
//
// tool_test.go — Tool 接口工具函数的单元测试：
// injectStandardFields、StripStandardFields、ToLLMDef。
package tool

import (
	"context"
	"encoding/json"
	"testing"

	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// ── injectStandardFields ──────────────────────────────────────────────────────

func TestInjectStandardFields_AddsSummaryAndDestructive(t *testing.T) {
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

	var required []string
	json.Unmarshal(schema["required"], &required)
	if len(required) == 0 || required[0] != "summary" {
		t.Errorf("summary not first in required: %v", required)
	}
	if !contains(required, "path") {
		t.Error("original 'path' field removed from required")
	}
	// destructive 不应进 required（默认 false）
	if contains(required, "destructive") {
		t.Errorf("destructive must NOT be in required (default false): %v", required)
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

func TestInjectStandardFields_NonObjectPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for non-object schema, got none")
		}
	}()
	injectStandardFields(json.RawMessage(`"just a string"`))
}

// ── StripStandardFields ───────────────────────────────────────────────────────

func TestStripStandardFields_ExtractsBoth(t *testing.T) {
	args := `{"summary":"Deleting all logs","destructive":true,"path":"/var/log"}`
	summary, destructive, stripped := StripStandardFields(args)

	if summary != "Deleting all logs" {
		t.Errorf("summary = %q, want 'Deleting all logs'", summary)
	}
	if !destructive {
		t.Error("destructive = false, want true")
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
	if m["path"] != "/var/log" {
		t.Errorf("path = %v, want /var/log", m["path"])
	}
}

func TestStripStandardFields_DefaultsWhenMissing(t *testing.T) {
	// summary missing → empty; destructive missing → false (zero values).
	// summary 缺失 → 空；destructive 缺失 → false（零值）。
	args := `{"path":"/etc/hosts"}`
	summary, destructive, stripped := StripStandardFields(args)

	if summary != "" {
		t.Errorf("summary = %q, want empty", summary)
	}
	if destructive {
		t.Error("destructive = true, want false")
	}
	var m map[string]any
	json.Unmarshal([]byte(stripped), &m)
	if m["path"] != "/etc/hosts" {
		t.Errorf("args modified unexpectedly: %s", stripped)
	}
}

func TestStripStandardFields_OnlySummary(t *testing.T) {
	args := `{"summary":"reading","path":"/tmp"}`
	summary, destructive, stripped := StripStandardFields(args)
	if summary != "reading" {
		t.Errorf("summary = %q, want 'reading'", summary)
	}
	if destructive {
		t.Error("destructive should default to false")
	}
	var m map[string]any
	json.Unmarshal([]byte(stripped), &m)
	if _, ok := m["summary"]; ok {
		t.Error("summary not stripped")
	}
}

func TestStripStandardFields_OnlyDestructive(t *testing.T) {
	args := `{"destructive":true,"command":"rm"}`
	summary, destructive, stripped := StripStandardFields(args)
	if summary != "" {
		t.Errorf("summary = %q, want empty", summary)
	}
	if !destructive {
		t.Error("destructive = false, want true")
	}
	var m map[string]any
	json.Unmarshal([]byte(stripped), &m)
	if _, ok := m["destructive"]; ok {
		t.Error("destructive not stripped")
	}
}

func TestStripStandardFields_InvalidJSON(t *testing.T) {
	args := `not-json`
	summary, destructive, stripped := StripStandardFields(args)
	if summary != "" {
		t.Errorf("expected empty summary for invalid JSON, got %q", summary)
	}
	if destructive {
		t.Error("expected destructive=false for invalid JSON")
	}
	if stripped != args {
		t.Errorf("stripped should equal input for invalid JSON, got %q", stripped)
	}
}

// ── ToLLMDef ─────────────────────────────────────────────────────────────────

// stubTool implements the full Tool interface for testing.
//
// stubTool 用于测试，实现完整 Tool 接口。
type stubTool struct {
	name   string
	desc   string
	params json.RawMessage
}

func (s *stubTool) Name() string                           { return s.name }
func (s *stubTool) Description() string                    { return s.desc }
func (s *stubTool) Parameters() json.RawMessage            { return s.params }
func (s *stubTool) IsReadOnly() bool                       { return true }
func (s *stubTool) NeedsReadFirst() bool                   { return false }
func (s *stubTool) RequiresWorkspace() bool                { return false }
func (s *stubTool) IsConcurrencySafe(json.RawMessage) bool { return true }
func (s *stubTool) ValidateInput(json.RawMessage) error    { return nil }
func (s *stubTool) CheckPermissions(json.RawMessage, PermissionMode) PermissionResult {
	return PermissionAllow
}
func (s *stubTool) Execute(_ context.Context, _ string) (string, error) { return "", nil }

func TestToLLMDef_SummaryAndDestructiveInjected(t *testing.T) {
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
	// Original tool.Parameters() should not be mutated.
	// 原始 tool.Parameters() 不应被修改。
	if string(tool.params) != original {
		t.Errorf("original params mutated: %s", tool.params)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// Compile-time checks.
var _ Tool = (*stubTool)(nil)
var _ llminfra.ToolDef = llminfra.ToolDef{}

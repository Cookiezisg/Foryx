package function

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestApplyOps_SetMeta(t *testing.T) {
	s := &Service{}
	base := &VersionDraft{}
	rawMeta, _ := json.Marshal(map[string]any{
		"name":        "to-pdf",
		"description": "convert markdown to pdf",
	})
	ops := []Op{{Type: "set_meta", Raw: rawMeta}}

	out, results, err := s.ApplyOps(context.Background(), base, ops, "")
	if err == nil {
		t.Fatalf("expected final validation to fail without code, got nil")
	}
	if !strings.Contains(err.Error(), "code is required") {
		t.Fatalf("expected code-required error, got: %v", err)
	}
	if len(results) != 1 || !results[0].OK {
		t.Errorf("expected 1 OK per-op result before final fail, got %+v", results)
	}
	if base.Name != "" {
		t.Errorf("cloneDraft failed: base.Name mutated to %q", base.Name)
	}
	_ = out
}

func TestApplyOps_FullHappyPath(t *testing.T) {
	s := &Service{}
	rawMeta, _ := json.Marshal(map[string]any{"name": "to_pdf", "description": "convert"})
	rawCode, _ := json.Marshal(map[string]any{"code": "def to_pdf(x):\n    return x\n"})
	ops := []Op{
		{Type: "set_meta", Raw: rawMeta},
		{Type: "set_code", Raw: rawCode},
	}
	out, results, err := s.ApplyOps(context.Background(), &VersionDraft{}, ops, "")
	if err != nil {
		t.Fatalf("ApplyOps full path: %v", err)
	}
	if out.Name != "to_pdf" {
		t.Errorf("expected name=to_pdf, got %q", out.Name)
	}
	if !strings.Contains(out.Code, "def to_pdf") {
		t.Errorf("expected code to contain 'def to_pdf', got %q", out.Code)
	}
	if len(results) != 2 || !results[0].OK || !results[1].OK {
		t.Errorf("expected 2 OK results, got %+v", results)
	}
}

func TestApplyOps_UnknownOpRejected(t *testing.T) {
	s := &Service{}
	ops := []Op{{Type: "frobnicate", Raw: json.RawMessage(`{}`)}}
	_, _, err := s.ApplyOps(context.Background(), &VersionDraft{}, ops, "")
	if err == nil || !strings.Contains(err.Error(), "unknown op type") {
		t.Errorf("expected unknown-op-type error, got %v", err)
	}
}

func TestApplyOps_DuplicateParam(t *testing.T) {
	s := &Service{}
	rawParams, _ := json.Marshal(map[string]any{
		"parameters": []map[string]any{
			{"name": "x", "type": "string", "required": true},
			{"name": "x", "type": "integer", "required": false},
		},
	})
	ops := []Op{{Type: "set_parameters", Raw: rawParams}}
	_, _, err := s.ApplyOps(context.Background(), &VersionDraft{}, ops, "")
	if err == nil || !strings.Contains(err.Error(), "duplicate parameter") {
		t.Errorf("expected duplicate parameter error, got %v", err)
	}
}

func TestApplyOps_InvalidName(t *testing.T) {
	s := &Service{}
	rawMeta, _ := json.Marshal(map[string]any{"name": "BadName"})
	ops := []Op{{Type: "set_meta", Raw: rawMeta}}
	_, _, err := s.ApplyOps(context.Background(), &VersionDraft{}, ops, "")
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected name-invalid error, got %v", err)
	}
}

func TestApplyOps_FinalMissingName(t *testing.T) {
	s := &Service{}
	rawCode, _ := json.Marshal(map[string]any{"code": "def x():\n    pass\n"})
	ops := []Op{{Type: "set_code", Raw: rawCode}}
	_, _, err := s.ApplyOps(context.Background(), &VersionDraft{}, ops, "")
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected 'name is required' error, got %v", err)
	}
}

func TestApplyOps_ASTScanRejectsHandlerImport(t *testing.T) {
	s := &Service{}
	rawMeta, _ := json.Marshal(map[string]any{"name": "bad"})
	rawCode, _ := json.Marshal(map[string]any{
		"code": "from forgify_handler import call\ndef bad():\n    return call()\n",
	})
	ops := []Op{
		{Type: "set_meta", Raw: rawMeta},
		{Type: "set_code", Raw: rawCode},
	}
	_, _, err := s.ApplyOps(context.Background(), &VersionDraft{}, ops, "")
	if err == nil || !strings.Contains(err.Error(), "handler import not allowed") {
		t.Errorf("expected D7 handler-import error, got %v", err)
	}
}

func TestApplyOps_SetDependencies_UsesDependenciesKey(t *testing.T) {
	s := &Service{}
	rawMeta, _ := json.Marshal(map[string]any{"name": "to_pdf", "description": "x"})
	rawCode, _ := json.Marshal(map[string]any{"code": "def to_pdf():\n    return 1\n"})
	rawDeps, _ := json.Marshal(map[string]any{"dependencies": []string{"reportlab"}})
	ops := []Op{
		{Type: "set_meta", Raw: rawMeta},
		{Type: "set_code", Raw: rawCode},
		{Type: "set_dependencies", Raw: rawDeps},
	}
	out, _, err := s.ApplyOps(context.Background(), &VersionDraft{}, ops, "")
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	if len(out.Dependencies) != 1 || out.Dependencies[0] != "reportlab" {
		t.Fatalf("Dependencies = %v, want [reportlab] (set_dependencies must read the \"dependencies\" key)", out.Dependencies)
	}
}

package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
)

func TestApplyOps_FullClassAssembly(t *testing.T) {
	s := &Service{}
	rawMeta, _ := json.Marshal(map[string]any{"name": "pg_handler", "description": "PG handler"})
	rawImports, _ := json.Marshal(map[string]any{"imports": "import psycopg2"})
	rawInit, _ := json.Marshal(map[string]any{"init_body": "self.conn = psycopg2.connect(**init_args)"})
	rawMethod1, _ := json.Marshal(map[string]any{
		"method": map[string]any{
			"name": "query",
			"args": []map[string]any{{"name": "sql", "type": "string", "required": true}},
			"body": "return self.conn.cursor().execute(sql).fetchall()",
		},
	})
	rawMethod2, _ := json.Marshal(map[string]any{
		"method": map[string]any{
			"name": "exec",
			"args": []map[string]any{{"name": "sql", "type": "string", "required": true}},
			"body": "self.conn.cursor().execute(sql); self.conn.commit()",
		},
	})

	ops := []Op{
		{Type: "set_meta", Raw: rawMeta},
		{Type: "set_imports", Raw: rawImports},
		{Type: "set_init", Raw: rawInit},
		{Type: "add_method", Raw: rawMethod1},
		{Type: "add_method", Raw: rawMethod2},
	}

	out, results, err := s.ApplyOps(context.Background(), nil, ops, "")
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("results len = %d, want 5", len(results))
	}
	if out.Name != "pg_handler" {
		t.Errorf("Name = %q, want pg_handler", out.Name)
	}
	if len(out.Methods) != 2 {
		t.Errorf("Methods len = %d, want 2", len(out.Methods))
	}
}

func TestApplyOps_AddMethod_DuplicateRejected(t *testing.T) {
	s := &Service{}
	raw, _ := json.Marshal(map[string]any{
		"method": map[string]any{
			"name": "query",
			"args": []map[string]any{},
			"body": "pass",
		},
	})
	ops := []Op{
		{Type: "add_method", Raw: raw},
		{Type: "add_method", Raw: raw},
	}
	_, _, err := s.ApplyOps(context.Background(), nil, ops, "")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected duplicate-method error, got %v", err)
	}
}

func TestApplyOps_DeleteMethod_MissingRejected(t *testing.T) {
	s := &Service{}
	raw, _ := json.Marshal(map[string]any{"name": "nope"})
	ops := []Op{{Type: "delete_method", Raw: raw}}
	_, _, err := s.ApplyOps(context.Background(), nil, ops, "")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestApplyOps_UpdateMethod_MergePatch(t *testing.T) {
	s := &Service{}
	rawAdd, _ := json.Marshal(map[string]any{
		"method": map[string]any{
			"name":        "query",
			"description": "old desc",
			"args":        []map[string]any{{"name": "sql", "type": "string", "required": true}},
			"body":        "old body",
		},
	})
	// Patch updates description + body, leaves args alone.
	patch, _ := json.Marshal(map[string]any{"description": "new desc", "body": "new body"})
	rawUpdate, _ := json.Marshal(map[string]any{"name": "query", "patch": json.RawMessage(patch)})

	ops := []Op{
		{Type: "add_method", Raw: rawAdd},
		{Type: "update_method", Raw: rawUpdate},
	}
	// Add a minimal name+method so final validation passes.
	rawMeta, _ := json.Marshal(map[string]any{"name": "hd"})
	ops = append([]Op{{Type: "set_meta", Raw: rawMeta}}, ops...)

	out, _, err := s.ApplyOps(context.Background(), nil, ops, "")
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	m := out.Methods[0]
	if m.Description != "new desc" {
		t.Errorf("Description = %q, want new desc", m.Description)
	}
	if m.Body != "new body" {
		t.Errorf("Body = %q, want new body", m.Body)
	}
	if len(m.Args) != 1 || m.Args[0].Name != "sql" {
		t.Errorf("Args lost during patch: %+v", m.Args)
	}
}

func TestApplyOps_FinalRequiresName(t *testing.T) {
	s := &Service{}
	rawMethod, _ := json.Marshal(map[string]any{
		"method": map[string]any{
			"name": "m",
			"args": []map[string]any{},
			"body": "pass",
		},
	})
	ops := []Op{{Type: "add_method", Raw: rawMethod}}
	_, _, err := s.ApplyOps(context.Background(), nil, ops, "")
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected name-required error, got %v", err)
	}
}

func TestApplyOps_FinalRequiresMethod(t *testing.T) {
	s := &Service{}
	rawMeta, _ := json.Marshal(map[string]any{"name": "no_methods"})
	ops := []Op{{Type: "set_meta", Raw: rawMeta}}
	_, _, err := s.ApplyOps(context.Background(), nil, ops, "")
	if err == nil || !strings.Contains(err.Error(), "at least one method") {
		t.Errorf("expected at-least-one-method error, got %v", err)
	}
}

func TestApplyOps_D7BlacklistOnImports(t *testing.T) {
	s := &Service{}
	rawMeta, _ := json.Marshal(map[string]any{"name": "x"})
	rawImports, _ := json.Marshal(map[string]any{"imports": "from forgify_handler import call"})
	rawMethod, _ := json.Marshal(map[string]any{
		"method": map[string]any{
			"name": "m",
			"args": []map[string]any{},
			"body": "pass",
		},
	})
	ops := []Op{
		{Type: "set_meta", Raw: rawMeta},
		{Type: "set_imports", Raw: rawImports},
		{Type: "add_method", Raw: rawMethod},
	}
	_, _, err := s.ApplyOps(context.Background(), nil, ops, "")
	if err == nil || !strings.Contains(err.Error(), "handler import not allowed") {
		t.Errorf("expected D7 import error, got %v", err)
	}
}

func TestApplyOps_InvalidArgType(t *testing.T) {
	s := &Service{}
	rawMethod, _ := json.Marshal(map[string]any{
		"method": map[string]any{
			"name": "m",
			"args": []map[string]any{{"name": "x", "type": "frobnicate", "required": true}},
			"body": "pass",
		},
	})
	ops := []Op{{Type: "add_method", Raw: rawMethod}}
	_, _, err := s.ApplyOps(context.Background(), nil, ops, "")
	if err == nil || !strings.Contains(err.Error(), "invalid type") {
		t.Errorf("expected invalid-type error, got %v", err)
	}
}

func TestApplyOps_UnknownOpRejected(t *testing.T) {
	s := &Service{}
	ops := []Op{{Type: "frobnicate", Raw: json.RawMessage(`{}`)}}
	_, _, err := s.ApplyOps(context.Background(), nil, ops, "")
	if err == nil || !strings.Contains(err.Error(), "unknown op type") {
		t.Errorf("expected unknown-op error, got %v", err)
	}
}

func TestApplyOps_SetInitArgsSchema(t *testing.T) {
	s := &Service{}
	rawMeta, _ := json.Marshal(map[string]any{"name": "ia"})
	rawSchema, _ := json.Marshal(map[string]any{
		"args": []map[string]any{
			{"name": "dsn", "type": "string", "required": true, "sensitive": true},
		},
	})
	rawMethod, _ := json.Marshal(map[string]any{
		"method": map[string]any{
			"name": "noop",
			"args": []map[string]any{},
			"body": "pass",
		},
	})
	ops := []Op{
		{Type: "set_meta", Raw: rawMeta},
		{Type: "set_init_args_schema", Raw: rawSchema},
		{Type: "add_method", Raw: rawMethod},
	}
	out, _, err := s.ApplyOps(context.Background(), nil, ops, "")
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	if len(out.InitArgsSchema) != 1 || out.InitArgsSchema[0].Name != "dsn" || !out.InitArgsSchema[0].Sensitive {
		t.Errorf("InitArgsSchema = %+v", out.InitArgsSchema)
	}
}

func TestParseOps_PreservesDiscriminator(t *testing.T) {
	wire := []byte(`[{"op":"set_meta","name":"x"},{"op":"add_method","method":{"name":"m","args":[],"body":"pass"}}]`)
	ops, err := ParseOps(wire)
	if err != nil {
		t.Fatalf("ParseOps: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("ops len = %d, want 2", len(ops))
	}
	if ops[0].Type != "set_meta" || ops[1].Type != "add_method" {
		t.Errorf("ops types = %q/%q", ops[0].Type, ops[1].Type)
	}
}

var _ handlerdomain.MethodSpec

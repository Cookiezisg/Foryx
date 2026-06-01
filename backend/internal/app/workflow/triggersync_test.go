package workflow

import (
	"testing"

	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

// TestExtractTriggers_CanonNestedSpec verifies the 17 §7 canon shape {kind, spec} is unwrapped:
// kind from config.kind, listener config from config.spec.
func TestExtractTriggers_CanonNestedSpec(t *testing.T) {
	g := &workflowdomain.Graph{Nodes: []workflowdomain.NodeSpec{
		{ID: "t1", Type: workflowdomain.NodeTypeTrigger, Config: map[string]any{
			"kind": "cron",
			"spec": map[string]any{"expression": "0 0 * * *"},
		}},
		{ID: "a1", Type: workflowdomain.NodeTypeFunction},
	}}
	got := extractTriggers(g)
	if len(got) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(got))
	}
	if got[0].NodeID != "t1" || got[0].Kind != "cron" {
		t.Errorf("got %+v, want {t1, cron}", got[0])
	}
	if got[0].Config["expression"] != "0 0 * * *" {
		t.Errorf("nested spec not unwrapped: %+v", got[0].Config)
	}
}

// TestExtractTriggers_FlatConfig verifies the flat shape (listener keys directly on config) works,
// and that meta keys (kind/payloadSchema) are stripped from the listener spec.
func TestExtractTriggers_FlatConfig(t *testing.T) {
	g := &workflowdomain.Graph{Nodes: []workflowdomain.NodeSpec{
		{ID: "t1", Type: workflowdomain.NodeTypeTrigger, Config: map[string]any{
			"kind":          "cron",
			"expression":    "@every 1m",
			"payloadSchema": map[string]any{"x": "string"},
		}},
	}}
	got := extractTriggers(g)
	if len(got) != 1 || got[0].Kind != "cron" {
		t.Fatalf("got %+v", got)
	}
	if got[0].Config["expression"] != "@every 1m" {
		t.Errorf("flat expression not carried: %+v", got[0].Config)
	}
	if _, leaked := got[0].Config["payloadSchema"]; leaked {
		t.Errorf("meta key payloadSchema leaked into listener spec: %+v", got[0].Config)
	}
	if _, leaked := got[0].Config["kind"]; leaked {
		t.Errorf("meta key kind leaked into listener spec: %+v", got[0].Config)
	}
}

// TestExtractTriggers_TriggerTypeFallback verifies the legacy config.triggerType drift is honored
// when config.kind is absent (some pipeline tests author {triggerType:manual}).
func TestExtractTriggers_TriggerTypeFallback(t *testing.T) {
	g := &workflowdomain.Graph{Nodes: []workflowdomain.NodeSpec{
		{ID: "t1", Type: workflowdomain.NodeTypeTrigger, Config: map[string]any{"triggerType": "manual"}},
	}}
	got := extractTriggers(g)
	if len(got) != 1 || got[0].Kind != "manual" {
		t.Errorf("triggerType fallback failed: %+v", got)
	}
}

// TestExtractTriggers_DefaultsManual verifies a trigger node with no kind defaults to manual
// (rather than producing an empty kind that RegisterTrigger would reject).
func TestExtractTriggers_DefaultsManual(t *testing.T) {
	g := &workflowdomain.Graph{Nodes: []workflowdomain.NodeSpec{
		{ID: "t1", Type: workflowdomain.NodeTypeTrigger, Config: map[string]any{}},
	}}
	got := extractTriggers(g)
	if len(got) != 1 || got[0].Kind != "manual" {
		t.Errorf("expected manual default, got %+v", got)
	}
}

// TestExtractTriggers_IgnoresNonTriggerNodes verifies only trigger-typed nodes are extracted.
func TestExtractTriggers_IgnoresNonTriggerNodes(t *testing.T) {
	g := &workflowdomain.Graph{Nodes: []workflowdomain.NodeSpec{
		{ID: "a", Type: workflowdomain.NodeTypeFunction},
		{ID: "b", Type: workflowdomain.NodeTypeAgent},
		{ID: "t", Type: workflowdomain.NodeTypeTrigger, Config: map[string]any{"kind": "webhook", "spec": map[string]any{"path": "hook"}}},
	}}
	got := extractTriggers(g)
	if len(got) != 1 || got[0].NodeID != "t" {
		t.Errorf("expected only the trigger node, got %+v", got)
	}
}

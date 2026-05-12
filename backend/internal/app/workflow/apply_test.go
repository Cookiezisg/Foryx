// apply_test.go — unit tests for the workflow ops engine. Each op family
// gets a happy-path test + at least one failure mode.
//
// apply_test.go —— workflow ops 引擎单测;每个 op 家族 happy + 1 失败模式。

package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

func opsFromJSON(t *testing.T, raw string) []Op {
	t.Helper()
	ops, err := ParseOps(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("ParseOps: %v", err)
	}
	return ops
}

// ── set_meta ─────────────────────────────────────────────────────────────────

func TestApply_SetMeta(t *testing.T) {
	ops := opsFromJSON(t, `[{"op":"set_meta","name":"my-wf","description":"desc","tags":["x","y"]}]`)
	g, err := ApplyOps(context.Background(), nil, ops, "")
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	if g.Name != "my-wf" || g.Description != "desc" || len(g.Tags) != 2 {
		t.Errorf("g = %+v, want name=my-wf desc=desc tags=[x y]", g)
	}
}

// ── add_node / update_node / delete_node ────────────────────────────────────

func TestApply_AddNode_HappyPath(t *testing.T) {
	ops := opsFromJSON(t, `[
		{"op":"add_node","node":{"id":"trig","type":"trigger","config":{"kind":"manual"}}},
		{"op":"add_node","node":{"id":"fn1","type":"function","config":{"functionId":"fn_x"}}}
	]`)
	g, err := ApplyOps(context.Background(), nil, ops, "")
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	if len(g.Nodes) != 2 {
		t.Errorf("len(g.Nodes) = %d, want 2", len(g.Nodes))
	}
}

func TestApply_AddNode_DuplicateID(t *testing.T) {
	ops := opsFromJSON(t, `[
		{"op":"add_node","node":{"id":"x","type":"trigger"}},
		{"op":"add_node","node":{"id":"x","type":"function"}}
	]`)
	_, err := ApplyOps(context.Background(), nil, ops, "")
	if !errors.Is(err, workflowdomain.ErrOpInvalid) {
		t.Errorf("expected ErrOpInvalid for duplicate id, got %v", err)
	}
}

func TestApply_AddNode_UnknownType(t *testing.T) {
	ops := opsFromJSON(t, `[{"op":"add_node","node":{"id":"x","type":"frobnicate"}}]`)
	_, err := ApplyOps(context.Background(), nil, ops, "")
	if !errors.Is(err, workflowdomain.ErrOpInvalid) || !strings.Contains(err.Error(), "frobnicate") {
		t.Errorf("expected ErrOpInvalid + 'frobnicate' in msg, got %v", err)
	}
}

func TestApply_UpdateNode_MergePatch(t *testing.T) {
	ops := opsFromJSON(t, `[
		{"op":"add_node","node":{"id":"n1","type":"http","config":{"method":"GET","url":"https://a"}}},
		{"op":"update_node","id":"n1","patch":{"config":{"method":"POST","headers":{"X":"1"}}}}
	]`)
	g, err := ApplyOps(context.Background(), nil, ops, "")
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	if g.Nodes[0].Config["method"] != "POST" {
		t.Errorf("method = %v, want POST", g.Nodes[0].Config["method"])
	}
	if g.Nodes[0].Config["url"] != "https://a" {
		t.Errorf("url = %v, want preserved", g.Nodes[0].Config["url"])
	}
	hdrs, _ := g.Nodes[0].Config["headers"].(map[string]any)
	if hdrs["X"] != "1" {
		t.Errorf("headers = %v, want X=1 added", g.Nodes[0].Config["headers"])
	}
}

func TestApply_UpdateNode_MergePatch_NullDeletes(t *testing.T) {
	ops := opsFromJSON(t, `[
		{"op":"add_node","node":{"id":"n1","type":"http","config":{"method":"GET","timeout":10}}},
		{"op":"update_node","id":"n1","patch":{"config":{"timeout":null}}}
	]`)
	g, _ := ApplyOps(context.Background(), nil, ops, "")
	if _, present := g.Nodes[0].Config["timeout"]; present {
		t.Errorf("timeout should be deleted by null patch, got %v", g.Nodes[0].Config["timeout"])
	}
}

func TestApply_DeleteNode_CascadesEdges(t *testing.T) {
	ops := opsFromJSON(t, `[
		{"op":"add_node","node":{"id":"a","type":"trigger"}},
		{"op":"add_node","node":{"id":"b","type":"function","config":{"functionId":"fn"}}},
		{"op":"add_node","node":{"id":"c","type":"function","config":{"functionId":"fn"}}},
		{"op":"add_edge","edge":{"from":"a.next","to":"b.input"}},
		{"op":"add_edge","edge":{"from":"b.output","to":"c.input"}},
		{"op":"delete_node","id":"b"}
	]`)
	g, err := ApplyOps(context.Background(), nil, ops, "")
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	if len(g.Nodes) != 2 {
		t.Errorf("len(Nodes) = %d, want 2", len(g.Nodes))
	}
	if len(g.Edges) != 0 {
		t.Errorf("edges to/from b should cascade-delete, got %d remaining", len(g.Edges))
	}
}

// ── add_edge / update_edge / delete_edge ────────────────────────────────────

func TestApply_AddEdge_AutoID(t *testing.T) {
	ops := opsFromJSON(t, `[
		{"op":"add_node","node":{"id":"a","type":"trigger"}},
		{"op":"add_node","node":{"id":"b","type":"function","config":{"functionId":"fn"}}},
		{"op":"add_edge","edge":{"from":"a.next","to":"b.input"}}
	]`)
	g, err := ApplyOps(context.Background(), nil, ops, "")
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	if len(g.Edges) != 1 || g.Edges[0].ID == "" {
		t.Errorf("auto-id should populate, got %+v", g.Edges)
	}
}

func TestApply_AddEdge_DuplicateFromTo(t *testing.T) {
	ops := opsFromJSON(t, `[
		{"op":"add_node","node":{"id":"a","type":"trigger"}},
		{"op":"add_node","node":{"id":"b","type":"function","config":{"functionId":"fn"}}},
		{"op":"add_edge","edge":{"from":"a.next","to":"b.input"}},
		{"op":"add_edge","edge":{"from":"a.next","to":"b.input"}}
	]`)
	_, err := ApplyOps(context.Background(), nil, ops, "")
	if !errors.Is(err, workflowdomain.ErrOpInvalid) {
		t.Errorf("expected ErrOpInvalid for duplicate edge, got %v", err)
	}
}

func TestApply_DeleteEdge(t *testing.T) {
	ops := opsFromJSON(t, `[
		{"op":"add_node","node":{"id":"a","type":"trigger"}},
		{"op":"add_node","node":{"id":"b","type":"function","config":{"functionId":"fn"}}},
		{"op":"add_edge","edge":{"id":"e1","from":"a.next","to":"b.input"}},
		{"op":"delete_edge","edgeId":"e1"}
	]`)
	g, _ := ApplyOps(context.Background(), nil, ops, "")
	if len(g.Edges) != 0 {
		t.Errorf("delete_edge should remove the edge, got %d", len(g.Edges))
	}
}

// ── set_variable / unset_variable ───────────────────────────────────────────

func TestApply_SetVariable_AddAndOverwrite(t *testing.T) {
	ops := opsFromJSON(t, `[
		{"op":"set_variable","name":"lastSeen","type":"string","default":"2026-01-01"},
		{"op":"set_variable","name":"lastSeen","type":"string","default":"2026-05-12"}
	]`)
	g, _ := ApplyOps(context.Background(), nil, ops, "")
	if len(g.Variables) != 1 || g.Variables[0].Default != "2026-05-12" {
		t.Errorf("overwrite should leave 1 var with new default, got %+v", g.Variables)
	}
}

func TestApply_UnsetVariable_NotFound(t *testing.T) {
	ops := opsFromJSON(t, `[{"op":"unset_variable","name":"x"}]`)
	_, err := ApplyOps(context.Background(), nil, ops, "")
	if !errors.Is(err, workflowdomain.ErrOpInvalid) {
		t.Errorf("expected ErrOpInvalid for unknown var, got %v", err)
	}
}

// ── ParseOps error paths ────────────────────────────────────────────────────

func TestParseOps_NotArray(t *testing.T) {
	_, err := ParseOps(json.RawMessage(`{"op":"set_meta"}`))
	if err == nil {
		t.Errorf("expected error for non-array ops payload")
	}
}

func TestParseOps_EmptyOp(t *testing.T) {
	_, err := ParseOps(json.RawMessage(`[{"name":"x"}]`))
	if err == nil {
		t.Errorf("expected error for missing 'op' discriminator")
	}
}

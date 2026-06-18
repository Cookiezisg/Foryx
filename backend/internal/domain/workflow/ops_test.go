package workflow

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	errorspkg "github.com/sunweilin/anselm/backend/internal/pkg/errors"
)

// applyJSON parses an ops JSON array and applies it to base, returning the new graph.
func applyJSON(t *testing.T, base *Graph, opsJSON string) (*Graph, error) {
	t.Helper()
	ops, err := ParseOps(json.RawMessage(opsJSON))
	if err != nil {
		return nil, err
	}
	return ApplyOps(base, ops)
}

func TestApplyOps_AddNodeAndEdge(t *testing.T) {
	g, err := applyJSON(t, nil, `[
		{"op":"add_node","node":{"id":"t","kind":"trigger","ref":"trg_a"}},
		{"op":"add_node","node":{"id":"a","kind":"action","ref":"fn_b","input":{"x":"input.y"}}},
		{"op":"add_edge","edge":{"id":"e1","from":"t","to":"a"}}
	]`)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(g.Nodes) != 2 || len(g.Edges) != 1 {
		t.Fatalf("want 2 nodes + 1 edge, got %d/%d", len(g.Nodes), len(g.Edges))
	}
	if g.Nodes[1].Input["x"] != "input.y" {
		t.Fatalf("input wiring lost: %+v", g.Nodes[1])
	}
	if err := ValidateGraph(g); err != nil {
		t.Fatalf("built graph should validate: %v", err)
	}
}

func TestApplyOps_DuplicateNodeRejected(t *testing.T) {
	base := &Graph{Nodes: []Node{trigNode("t")}}
	_, err := applyJSON(t, base, `[{"op":"add_node","node":{"id":"t","kind":"action","ref":"fn_b"}}]`)
	if !errors.Is(err, ErrInvalidOps) {
		t.Fatalf("want ErrInvalidOps for dup node, got %v", err)
	}
}

func TestApplyOps_UpdateNodeMergePatch(t *testing.T) {
	base := &Graph{Nodes: []Node{trigNode("t"), actNode("a")}}
	g, err := applyJSON(t, base, `[{"op":"update_node","id":"a","patch":{"input":{"x":"input.z","w":"payload.k"},"notes":"hi"}}]`)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	n := g.Nodes[1]
	if n.Input["x"] != "input.z" || n.Input["w"] != "payload.k" {
		t.Fatalf("input not replaced: %+v", n.Input)
	}
	if n.Notes != "hi" {
		t.Fatalf("notes not patched: %q", n.Notes)
	}
	// Untouched fields survive (ref, kind).
	if n.Ref != "fn_bbbb" || n.Kind != NodeKindAction {
		t.Fatalf("untouched fields clobbered: %+v", n)
	}
	// id is immutable even if the patch tries to change it.
	g2, err := applyJSON(t, base, `[{"op":"update_node","id":"a","patch":{"id":"hijacked"}}]`)
	if err != nil {
		t.Fatalf("apply2: %v", err)
	}
	if g2.Nodes[1].ID != "a" {
		t.Fatalf("id must be immutable across update, got %q", g2.Nodes[1].ID)
	}
}

func TestApplyOps_UpdateUnknownNodeRejected(t *testing.T) {
	base := &Graph{Nodes: []Node{trigNode("t")}}
	_, err := applyJSON(t, base, `[{"op":"update_node","id":"ghost","patch":{"notes":"x"}}]`)
	if !errors.Is(err, ErrInvalidOps) {
		t.Fatalf("want ErrInvalidOps, got %v", err)
	}
}

func TestApplyOps_DeleteNodeCascadesEdges(t *testing.T) {
	base := &Graph{
		Nodes: []Node{trigNode("t"), actNode("a"), actNode("b")},
		Edges: []Edge{edge("e1", "t", "a"), edge("e2", "a", "b"), edge("e3", "t", "b")},
	}
	g, err := applyJSON(t, base, `[{"op":"delete_node","id":"a"}]`)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(g.Nodes) != 2 {
		t.Fatalf("node a not removed: %d nodes", len(g.Nodes))
	}
	// e1 (t->a) and e2 (a->b) must be cascade-deleted; only e3 (t->b) survives.
	if len(g.Edges) != 1 || g.Edges[0].ID != "e3" {
		t.Fatalf("cascade edge delete wrong: %+v", g.Edges)
	}
}

func TestApplyOps_UpdateAndDeleteEdge(t *testing.T) {
	base := &Graph{
		Nodes: []Node{trigNode("t"), ctlNode("c"), actNode("a")},
		Edges: []Edge{edge("e1", "t", "c"), edgeP("e2", "c", "old", "a")},
	}
	g, err := applyJSON(t, base, `[
		{"op":"update_edge","id":"e2","patch":{"fromPort":"new"}},
		{"op":"delete_edge","id":"e1"}
	]`)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(g.Edges) != 1 || g.Edges[0].ID != "e2" || g.Edges[0].FromPort != "new" {
		t.Fatalf("edge update/delete wrong: %+v", g.Edges)
	}
}

func TestApplyOps_DeleteUnknownEdgeRejected(t *testing.T) {
	base := &Graph{Nodes: []Node{trigNode("t")}}
	_, err := applyJSON(t, base, `[{"op":"delete_edge","id":"nope"}]`)
	if !errors.Is(err, ErrInvalidOps) {
		t.Fatalf("want ErrInvalidOps, got %v", err)
	}
}

func TestApplyOps_DoesNotMutateBase(t *testing.T) {
	base := &Graph{Nodes: []Node{trigNode("t")}, Edges: []Edge{}}
	_, err := applyJSON(t, base, `[{"op":"add_node","node":{"id":"a","kind":"action","ref":"fn_b"}}]`)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(base.Nodes) != 1 {
		t.Fatalf("base was mutated: %d nodes", len(base.Nodes))
	}
}

func TestParseOps_Errors(t *testing.T) {
	if _, err := ParseOps(json.RawMessage(`{"op":"x"}`)); !errors.Is(err, ErrInvalidOps) {
		t.Fatalf("non-array should be ErrInvalidOps, got %v", err)
	}
	if _, err := ParseOps(json.RawMessage(`[{"node":{}}]`)); !errors.Is(err, ErrInvalidOps) {
		t.Fatalf("missing discriminator should be ErrInvalidOps, got %v", err)
	}
}

func TestApplyOps_UnknownOpRejected(t *testing.T) {
	_, err := applyJSON(t, nil, `[{"op":"frobnicate","id":"x"}]`)
	if !errors.Is(err, ErrInvalidOps) {
		t.Fatalf("unknown op should be ErrInvalidOps, got %v", err)
	}
}

func TestExtractMeta(t *testing.T) {
	ops, err := ParseOps(json.RawMessage(`[
		{"op":"set_meta","name":"alpha","tags":["x"]},
		{"op":"add_node","node":{"id":"t","kind":"trigger","ref":"trg_a"}},
		{"op":"set_meta","description":"d","name":"beta"}
	]`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	patch, err := ExtractMeta(ops)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if patch.Name == nil || *patch.Name != "beta" {
		t.Fatalf("later set_meta name should win: %+v", patch.Name)
	}
	if patch.Description == nil || *patch.Description != "d" {
		t.Fatalf("description not extracted: %+v", patch.Description)
	}
	if patch.Tags == nil || len(*patch.Tags) != 1 || (*patch.Tags)[0] != "x" {
		t.Fatalf("tags not extracted: %+v", patch.Tags)
	}
	// set_meta must NOT add graph nodes.
	g, err := ApplyOps(nil, ops)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(g.Nodes) != 1 {
		t.Fatalf("set_meta leaked into graph: %d nodes", len(g.Nodes))
	}
}

// TestApplyOps_StrayTopLevelInputRejected — round-10: a node-content field (input/ref/kind/...) placed
// at the OP level instead of inside "node" is silently dropped by JSON, so the node runs with no
// wiring and fails opaquely at runtime ("missing required argument"). Reject it at authoring time.
func TestApplyOps_StrayTopLevelInputRejected(t *testing.T) {
	base := &Graph{}
	_, err := applyJSON(t, base, `[{"op":"add_node","input":{"x":"t.y"},"node":{"id":"a","kind":"action","ref":"fn_b"}}]`)
	if !errors.Is(err, ErrInvalidOps) {
		t.Fatalf("a misplaced top-level input should be rejected as ErrInvalidOps, got %v", err)
	}
	// The rejection must name the misplaced 'input' (carried in Details.reason → surfaced to the LLM),
	// not just say "invalid ops" — so the agent fixes the placement instead of guessing.
	var ee *errorspkg.Error
	if !errors.As(err, &ee) {
		t.Fatalf("expected ErrInvalidOps as an errorspkg.Error, got %v", err)
	}
	if reason, _ := ee.Details["reason"].(string); !strings.Contains(reason, "input") {
		t.Fatalf("rejection must name 'input' in Details.reason; got %q", reason)
	}
	if _, err := applyJSON(t, base, `[{"op":"add_node","node":{"id":"a","kind":"action","ref":"fn_b","input":{"x":"t.y"}}}]`); err != nil {
		t.Fatalf("input INSIDE node must be accepted, got %v", err)
	}
}

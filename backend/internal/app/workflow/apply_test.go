package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

// fakeKeys is the minimum KeyProvider stub for apply unit tests: byID maps
// known apiKeyId → Credentials, unknown ids return apikey.ErrNotFound (F1 contract).
//
// fakeKeys 是 apply 单测最小 KeyProvider 桩;byID 命中返 Credentials,
// 未命中返 apikey.ErrNotFound(F1 契约)。
type fakeKeys struct {
	byID map[string]apikeydomain.Credentials
}

func (f *fakeKeys) ResolveCredentials(context.Context, string) (apikeydomain.Credentials, error) {
	return apikeydomain.Credentials{}, nil
}
func (f *fakeKeys) ResolveCredentialsByID(_ context.Context, id string) (apikeydomain.Credentials, error) {
	if f.byID == nil {
		return apikeydomain.Credentials{}, apikeydomain.ErrNotFound
	}
	c, ok := f.byID[id]
	if !ok {
		return apikeydomain.Credentials{}, apikeydomain.ErrNotFound
	}
	return c, nil
}
func (f *fakeKeys) MarkInvalid(context.Context, string, string) error { return nil }
func (f *fakeKeys) DefaultSearchProvider(context.Context) string      { return "" }

func newFakeKeys() *fakeKeys {
	return &fakeKeys{byID: map[string]apikeydomain.Credentials{
		"aki_test": {Provider: "anthropic", Key: "sk-test", BaseURL: ""},
	}}
}

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
	g, err := ApplyOps(context.Background(), nil, ops, "", nil)
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
	g, err := ApplyOps(context.Background(), nil, ops, "", nil)
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
	_, err := ApplyOps(context.Background(), nil, ops, "", nil)
	if !errors.Is(err, workflowdomain.ErrOpInvalid) {
		t.Errorf("expected ErrOpInvalid for duplicate id, got %v", err)
	}
}

func TestApply_AddNode_UnknownType(t *testing.T) {
	ops := opsFromJSON(t, `[{"op":"add_node","node":{"id":"x","type":"frobnicate"}}]`)
	_, err := ApplyOps(context.Background(), nil, ops, "", nil)
	if !errors.Is(err, workflowdomain.ErrOpInvalid) || !strings.Contains(err.Error(), "frobnicate") {
		t.Errorf("expected ErrOpInvalid + 'frobnicate' in msg, got %v", err)
	}
}

func TestApply_UpdateNode_MergePatch(t *testing.T) {
	ops := opsFromJSON(t, `[
		{"op":"add_node","node":{"id":"n1","type":"http","config":{"method":"GET","url":"https://a"}}},
		{"op":"update_node","nodeId":"n1","patch":{"config":{"method":"POST","headers":{"X":"1"}}}}
	]`)
	g, err := ApplyOps(context.Background(), nil, ops, "", nil)
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
		{"op":"update_node","nodeId":"n1","patch":{"config":{"timeout":null}}}
	]`)
	g, _ := ApplyOps(context.Background(), nil, ops, "", nil)
	if _, present := g.Nodes[0].Config["timeout"]; present {
		t.Errorf("timeout should be deleted by null patch, got %v", g.Nodes[0].Config["timeout"])
	}
}

func TestApply_DeleteNode_CascadesEdges(t *testing.T) {
	ops := opsFromJSON(t, `[
		{"op":"add_node","node":{"id":"a","type":"trigger"}},
		{"op":"add_node","node":{"id":"b","type":"function","config":{"functionId":"fn"}}},
		{"op":"add_node","node":{"id":"c","type":"function","config":{"functionId":"fn"}}},
		{"op":"add_edge","edge":{"from": "a","to": "b"}},
		{"op":"add_edge","edge":{"from": "b","to": "c"}},
		{"op":"delete_node","nodeId":"b"}
	]`)
	g, err := ApplyOps(context.Background(), nil, ops, "", nil)
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
		{"op":"add_edge","edge":{"from": "a","to": "b"}}
	]`)
	g, err := ApplyOps(context.Background(), nil, ops, "", nil)
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
		{"op":"add_edge","edge":{"from": "a","to": "b"}},
		{"op":"add_edge","edge":{"from": "a","to": "b"}}
	]`)
	_, err := ApplyOps(context.Background(), nil, ops, "", nil)
	if !errors.Is(err, workflowdomain.ErrOpInvalid) {
		t.Errorf("expected ErrOpInvalid for duplicate edge, got %v", err)
	}
}

func TestApply_DeleteEdge(t *testing.T) {
	ops := opsFromJSON(t, `[
		{"op":"add_node","node":{"id":"a","type":"trigger"}},
		{"op":"add_node","node":{"id":"b","type":"function","config":{"functionId":"fn"}}},
		{"op":"add_edge","edge":{"id":"e1","from": "a","to": "b"}},
		{"op":"delete_edge","edgeId":"e1"}
	]`)
	g, _ := ApplyOps(context.Background(), nil, ops, "", nil)
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
	g, _ := ApplyOps(context.Background(), nil, ops, "", nil)
	if len(g.Variables) != 1 || g.Variables[0].Default != "2026-05-12" {
		t.Errorf("overwrite should leave 1 var with new default, got %+v", g.Variables)
	}
}

func TestApply_UnsetVariable_NotFound(t *testing.T) {
	ops := opsFromJSON(t, `[{"op":"unset_variable","name":"x"}]`)
	_, err := ApplyOps(context.Background(), nil, ops, "", nil)
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

// ── set_node_model_override ─────────────────────────────────────────────────

func TestApply_SetNodeModelOverride_SetsField(t *testing.T) {
	ops := opsFromJSON(t, `[
		{"op":"add_node","node":{"id":"node_1","type":"agent","config":{"scenario":"agent"}}},
		{"op":"set_node_model_override","nodeId":"node_1","modelOverride":{"apiKeyId":"aki_test","modelId":"claude-haiku-4-5"}}
	]`)
	g, err := ApplyOps(context.Background(), nil, ops, "", newFakeKeys())
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	idx := findNode(g, "node_1")
	if idx < 0 {
		t.Fatalf("node_1 missing")
	}
	mo := g.Nodes[idx].ModelOverride
	if mo == nil {
		t.Fatal("modelOverride nil")
	}
	if mo.APIKeyID != "aki_test" || mo.ModelID != "claude-haiku-4-5" {
		t.Fatalf("got %+v, want {aki_test, claude-haiku-4-5}", mo)
	}
}

func TestApply_SetNodeModelOverride_Clears(t *testing.T) {
	ops := opsFromJSON(t, `[
		{"op":"add_node","node":{"id":"node_1","type":"agent","config":{"scenario":"agent"}}},
		{"op":"set_node_model_override","nodeId":"node_1","modelOverride":{"apiKeyId":"aki_test","modelId":"claude-haiku-4-5"}},
		{"op":"set_node_model_override","nodeId":"node_1"}
	]`)
	g, err := ApplyOps(context.Background(), nil, ops, "", newFakeKeys())
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	idx := findNode(g, "node_1")
	if g.Nodes[idx].ModelOverride != nil {
		t.Fatalf("expected nil, got %+v", g.Nodes[idx].ModelOverride)
	}
}

func TestApply_SetNodeModelOverride_UnknownKey_Returns404(t *testing.T) {
	ops := opsFromJSON(t, `[
		{"op":"add_node","node":{"id":"node_1","type":"agent","config":{"scenario":"agent"}}},
		{"op":"set_node_model_override","nodeId":"node_1","modelOverride":{"apiKeyId":"aki_nonexistent","modelId":"x"}}
	]`)
	_, err := ApplyOps(context.Background(), nil, ops, "", newFakeKeys())
	if !errors.Is(err, apikeydomain.ErrNotFound) {
		t.Fatalf("want apikey.ErrNotFound, got %v", err)
	}
}

func TestApply_SetNodeModelOverride_MissingAPIKeyID_Returns400(t *testing.T) {
	ops := opsFromJSON(t, `[
		{"op":"add_node","node":{"id":"node_1","type":"agent","config":{"scenario":"agent"}}},
		{"op":"set_node_model_override","nodeId":"node_1","modelOverride":{"modelId":"x"}}
	]`)
	_, err := ApplyOps(context.Background(), nil, ops, "", newFakeKeys())
	if !errors.Is(err, workflowdomain.ErrInvalidNodeModelOverride) {
		t.Fatalf("want ErrInvalidNodeModelOverride, got %v", err)
	}
}

func TestApply_SetNodeModelOverride_ThinkingSurvives(t *testing.T) {
	ops := opsFromJSON(t, `[
		{"op":"add_node","node":{"id":"node_1","type":"agent","config":{"scenario":"agent"}}},
		{"op":"set_node_model_override","nodeId":"node_1","modelOverride":{
			"apiKeyId":"aki_test","modelId":"claude-sonnet-4-5",
			"thinking":{"mode":"on","effort":"high"}
		}}
	]`)
	g, err := ApplyOps(context.Background(), nil, ops, "", newFakeKeys())
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	idx := findNode(g, "node_1")
	if idx < 0 {
		t.Fatal("node_1 missing")
	}
	mo := g.Nodes[idx].ModelOverride
	if mo == nil {
		t.Fatal("ModelOverride is nil")
	}
	if mo.Thinking == nil {
		t.Fatal("Thinking is nil, want non-nil")
	}
	if mo.Thinking.Mode != "on" || mo.Thinking.Effort != "high" {
		t.Errorf("Thinking = %+v, want {Mode:on Effort:high}", mo.Thinking)
	}
	// Verify F1 fields are still set correctly.
	if mo.APIKeyID != "aki_test" || mo.ModelID != "claude-sonnet-4-5" {
		t.Errorf("base fields = {%q %q}, want {aki_test claude-sonnet-4-5}", mo.APIKeyID, mo.ModelID)
	}
}

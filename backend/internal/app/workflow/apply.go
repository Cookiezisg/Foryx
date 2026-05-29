package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
)

// Op is one wire-level op; Type is the discriminator and Raw is the unmodified body.
//
// Op 是单条 LLM op；Type 判别，Raw 是未动 body。
type Op struct {
	Type string          `json:"op"`
	Raw  json.RawMessage `json:"-"`
}

// ParseOps decodes the LLM JSON array into []Op.
//
// ParseOps 把 LLM JSON 数组解码为 []Op。
func ParseOps(raw json.RawMessage) ([]Op, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, fmt.Errorf("ops must be a JSON array: %w", err)
	}
	ops := make([]Op, 0, len(arr))
	for i, item := range arr {
		var head struct {
			Type string `json:"op"`
		}
		if err := json.Unmarshal(item, &head); err != nil {
			return nil, fmt.Errorf("op[%d] missing 'op' discriminator: %w", i, err)
		}
		if head.Type == "" {
			return nil, fmt.Errorf("op[%d] has empty 'op' field", i)
		}
		ops = append(ops, Op{Type: head.Type, Raw: item})
	}
	return ops, nil
}

// ApplyOps applies ops to a base graph (nil = from scratch) and returns the final graph.
// keyProvider is optional; non-nil enables F1 validation on set_node_model_override
// (cross-user / unknown api_key surfaces apikey.ErrNotFound for 404 mapping).
//
// ApplyOps 把 ops 应用到 base 图（nil = 从零建）并返最终图。
// keyProvider 可空;非空开启 set_node_model_override 的 F1 校验
// (跨用户 / 未知 api_key 直接走 apikey.ErrNotFound → 404)。
func ApplyOps(ctx context.Context, base *workflowdomain.Graph, ops []Op, progressBlockID string, keyProvider apikeydomain.KeyProvider) (*workflowdomain.Graph, error) {
	g := cloneGraph(base)
	em := eventlogpkg.From(ctx)

	for i, op := range ops {
		if err := applyOne(ctx, g, op, keyProvider); err != nil {
			// Preserve cross-domain sentinels (F1 routes to their own HTTP code
			// in errmap; otherwise they'd be flattened to WORKFLOW_OP_INVALID).
			//
			// 保留跨 domain sentinel（F1 走自己的 errmap，否则被 WORKFLOW_OP_INVALID 吞掉）。
			if errors.Is(err, apikeydomain.ErrNotFound) ||
				errors.Is(err, workflowdomain.ErrInvalidNodeModelOverride) {
				return nil, fmt.Errorf("op[%d] %s: %w", i, op.Type, err)
			}
			return nil, fmt.Errorf("op[%d] %s: %w: %v", i, op.Type, workflowdomain.ErrOpInvalid, err)
		}
		if progressBlockID != "" {
			em.DeltaBlock(ctx, progressBlockID,
				fmt.Sprintf("op[%d] %s ✓\n", i, op.Type))
		}
		_ = eventlogdomain.StatusStreaming
	}
	return g, nil
}

func applyOne(ctx context.Context, g *workflowdomain.Graph, op Op, keyProvider apikeydomain.KeyProvider) error {
	switch op.Type {
	case workflowdomain.OpSetMeta:
		return applySetMeta(g, op.Raw)
	case workflowdomain.OpAddNode:
		return applyAddNode(g, op.Raw)
	case workflowdomain.OpUpdateNode:
		return applyUpdateNode(g, op.Raw)
	case workflowdomain.OpDeleteNode:
		return applyDeleteNode(g, op.Raw)
	case workflowdomain.OpAddEdge:
		return applyAddEdge(g, op.Raw)
	case workflowdomain.OpUpdateEdge:
		return applyUpdateEdge(g, op.Raw)
	case workflowdomain.OpDeleteEdge:
		return applyDeleteEdge(g, op.Raw)
	case workflowdomain.OpSetVariable:
		return applySetVariable(g, op.Raw)
	case workflowdomain.OpUnsetVariable:
		return applyUnsetVariable(g, op.Raw)
	case workflowdomain.OpSetNodeModelOverride:
		return applySetNodeModelOverride(ctx, g, op.Raw, keyProvider)
	default:
		return fmt.Errorf("unknown op type %q", op.Type)
	}
}

func applySetMeta(g *workflowdomain.Graph, raw json.RawMessage) error {
	var p struct {
		Name        *string   `json:"name"`
		Description *string   `json:"description"`
		Tags        *[]string `json:"tags"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("set_meta unmarshal: %w", err)
	}
	if p.Name != nil {
		g.Name = *p.Name
	}
	if p.Description != nil {
		g.Description = *p.Description
	}
	if p.Tags != nil {
		g.Tags = append([]string(nil), (*p.Tags)...)
	}
	return nil
}

func applyAddNode(g *workflowdomain.Graph, raw json.RawMessage) error {
	var p struct {
		Node workflowdomain.NodeSpec `json:"node"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("add_node unmarshal: %w", err)
	}
	if p.Node.ID == "" {
		return fmt.Errorf("add_node: empty node id")
	}
	if !workflowdomain.IsValidNodeType(p.Node.Type) {
		if isPseudoTerminalType(p.Node.Type) {
			return fmt.Errorf("add_node: node %q uses type %q — workflows have no terminal node; the DAG ends implicitly when no edges remain. Remove this node and let the last real node be the leaf",
				p.Node.ID, p.Node.Type)
		}
		return fmt.Errorf("add_node: unknown node type %q", p.Node.Type)
	}
	if findNode(g, p.Node.ID) >= 0 {
		return fmt.Errorf("add_node: duplicate id %q", p.Node.ID)
	}
	g.Nodes = append(g.Nodes, p.Node)
	return nil
}

func applyUpdateNode(g *workflowdomain.Graph, raw json.RawMessage) error {
	var p struct {
		NodeID string          `json:"nodeId"`
		Patch  json.RawMessage `json:"patch"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("update_node unmarshal: %w", err)
	}
	if p.NodeID == "" {
		return fmt.Errorf("update_node: empty nodeId")
	}
	idx := findNode(g, p.NodeID)
	if idx < 0 {
		return fmt.Errorf("update_node: node %q not found", p.NodeID)
	}
	merged, err := mergeNodePatch(g.Nodes[idx], p.Patch)
	if err != nil {
		return fmt.Errorf("update_node: %w", err)
	}
	g.Nodes[idx] = merged
	return nil
}

func applyDeleteNode(g *workflowdomain.Graph, raw json.RawMessage) error {
	var p struct {
		NodeID string `json:"nodeId"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("delete_node unmarshal: %w", err)
	}
	if p.NodeID == "" {
		return fmt.Errorf("delete_node: empty nodeId")
	}
	idx := findNode(g, p.NodeID)
	if idx < 0 {
		return fmt.Errorf("delete_node: node %q not found", p.NodeID)
	}
	g.Nodes = append(g.Nodes[:idx], g.Nodes[idx+1:]...)
	// Cascade delete edges touching this node — orphan edges would fail final
	// validation anyway, but cleaning them in the same op keeps the
	// intermediate state ergonomic for downstream ops in the same batch.
	//
	// 级联删触及该节点的 edge — orphan edge 在最终校验也会 fail,但同 op
	// 顺带清掉让批次中后续 op 看到干净 state。
	kept := g.Edges[:0]
	for _, e := range g.Edges {
		if edgeRefsNode(e, p.NodeID) {
			continue
		}
		kept = append(kept, e)
	}
	g.Edges = kept
	return nil
}

func applyAddEdge(g *workflowdomain.Graph, raw json.RawMessage) error {
	var p struct {
		Edge workflowdomain.EdgeSpec `json:"edge"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("add_edge unmarshal: %w", err)
	}
	if p.Edge.From == "" || p.Edge.To == "" {
		return fmt.Errorf("add_edge: from/to empty")
	}
	// Edge ID is system-generated when blank (LLM never supplies one per
	// spec §3.2); use a stable-ish counter so order matters for ops history.
	if p.Edge.ID == "" {
		p.Edge.ID = fmt.Sprintf("edge_%d", len(g.Edges)+1)
	}
	for _, e := range g.Edges {
		if e.ID == p.Edge.ID {
			return fmt.Errorf("add_edge: duplicate id %q", p.Edge.ID)
		}
		if e.From == p.Edge.From && e.To == p.Edge.To {
			return fmt.Errorf("add_edge: duplicate edge %s → %s", p.Edge.From, p.Edge.To)
		}
	}
	g.Edges = append(g.Edges, p.Edge)
	return nil
}

func applyUpdateEdge(g *workflowdomain.Graph, raw json.RawMessage) error {
	var p struct {
		EdgeID string          `json:"edgeId"`
		Patch  json.RawMessage `json:"patch"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("update_edge unmarshal: %w", err)
	}
	if p.EdgeID == "" {
		return fmt.Errorf("update_edge: empty edgeId")
	}
	idx := findEdge(g, p.EdgeID)
	if idx < 0 {
		return fmt.Errorf("update_edge: edge %q not found", p.EdgeID)
	}
	merged, err := mergeEdgePatch(g.Edges[idx], p.Patch)
	if err != nil {
		return fmt.Errorf("update_edge: %w", err)
	}
	g.Edges[idx] = merged
	return nil
}

func applyDeleteEdge(g *workflowdomain.Graph, raw json.RawMessage) error {
	var p struct {
		EdgeID string `json:"edgeId"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("delete_edge unmarshal: %w", err)
	}
	idx := findEdge(g, p.EdgeID)
	if idx < 0 {
		return fmt.Errorf("delete_edge: edge %q not found", p.EdgeID)
	}
	g.Edges = append(g.Edges[:idx], g.Edges[idx+1:]...)
	return nil
}

func applySetVariable(g *workflowdomain.Graph, raw json.RawMessage) error {
	var v workflowdomain.VariableSpec
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("set_variable unmarshal: %w", err)
	}
	if v.Name == "" {
		return fmt.Errorf("set_variable: empty name")
	}
	if !workflowdomain.IsValidVariableType(v.Type) {
		return fmt.Errorf("set_variable: unknown type %q", v.Type)
	}
	for i := range g.Variables {
		if g.Variables[i].Name == v.Name {
			g.Variables[i] = v
			return nil
		}
	}
	g.Variables = append(g.Variables, v)
	return nil
}

func applyUnsetVariable(g *workflowdomain.Graph, raw json.RawMessage) error {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("unset_variable unmarshal: %w", err)
	}
	if p.Name == "" {
		return fmt.Errorf("unset_variable: empty name")
	}
	kept := g.Variables[:0]
	found := false
	for _, v := range g.Variables {
		if v.Name == p.Name {
			found = true
			continue
		}
		kept = append(kept, v)
	}
	if !found {
		return fmt.Errorf("unset_variable: variable %q not declared", p.Name)
	}
	g.Variables = kept
	return nil
}

// applySetNodeModelOverride sets or clears NodeSpec.ModelOverride.
// Omitting modelOverride (or sending null) clears; setting requires both
// apiKeyId and modelId. F1: apiKeyId must reference a user-owned key when
// keyProvider is wired (cross-user lookups → apikey.ErrNotFound for 404).
//
// applySetNodeModelOverride 设置或清除节点 ModelOverride。
// 不传 modelOverride（或传 null）= 清空;设置则 apiKeyId 与 modelId 都必填。
// F1：keyProvider 已装时 apiKeyId 必须属当前 user（跨用户 → apikey.ErrNotFound → 404）。
func applySetNodeModelOverride(ctx context.Context, g *workflowdomain.Graph, raw json.RawMessage, keyProvider apikeydomain.KeyProvider) error {
	var p struct {
		NodeID        string          `json:"nodeId"`
		ModelOverride json.RawMessage `json:"modelOverride"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("set_node_model_override unmarshal: %w", err)
	}
	if p.NodeID == "" {
		return fmt.Errorf("set_node_model_override: empty nodeId")
	}
	idx := findNode(g, p.NodeID)
	if idx < 0 {
		return fmt.Errorf("set_node_model_override: node %q not found", p.NodeID)
	}

	// Omitted field or explicit null both clear the override.
	if len(p.ModelOverride) == 0 || string(p.ModelOverride) == "null" {
		g.Nodes[idx].ModelOverride = nil
		return nil
	}

	var ref modeldomain.ModelRef
	if err := json.Unmarshal(p.ModelOverride, &ref); err != nil {
		return fmt.Errorf("%w: modelOverride must be object: %v", workflowdomain.ErrInvalidNodeModelOverride, err)
	}
	apiKeyID := strings.TrimSpace(ref.APIKeyID)
	modelID := strings.TrimSpace(ref.ModelID)
	if apiKeyID == "" || modelID == "" {
		return fmt.Errorf("%w: apiKeyId=%q modelId=%q", workflowdomain.ErrInvalidNodeModelOverride, apiKeyID, modelID)
	}

	// F1: apiKeyId must reference an existing api_key owned by current user.
	//
	// F1 校验:apiKeyId 必须存在且属当前 user。
	if keyProvider != nil {
		if _, err := keyProvider.ResolveCredentialsByID(ctx, apiKeyID); err != nil {
			return fmt.Errorf("set_node_model_override: %w", err)
		}
	}

	// Assign the fully-parsed ref so Thinking (and any future ModelRef fields)
	// survive the round-trip rather than being silently dropped.
	//
	// 直接赋值已解析的 ref，让 Thinking 等字段随之保留，而非重新构造只含两字段的对象。
	ref.APIKeyID = apiKeyID
	ref.ModelID = modelID
	g.Nodes[idx].ModelOverride = &ref
	return nil
}

func cloneGraph(base *workflowdomain.Graph) *workflowdomain.Graph {
	if base == nil {
		return &workflowdomain.Graph{
			Nodes: []workflowdomain.NodeSpec{},
			Edges: []workflowdomain.EdgeSpec{},
		}
	}
	g := &workflowdomain.Graph{
		Name:        base.Name,
		Description: base.Description,
		Tags:        append([]string(nil), base.Tags...),
		Variables:   append([]workflowdomain.VariableSpec(nil), base.Variables...),
		Nodes:       append([]workflowdomain.NodeSpec(nil), base.Nodes...),
		Edges:       append([]workflowdomain.EdgeSpec(nil), base.Edges...),
	}
	for i := range g.Nodes {
		if g.Nodes[i].Config != nil {
			cfg, _ := json.Marshal(g.Nodes[i].Config)
			var copied map[string]any
			_ = json.Unmarshal(cfg, &copied)
			g.Nodes[i].Config = copied
		}
	}
	return g
}

func findNode(g *workflowdomain.Graph, id string) int {
	for i, n := range g.Nodes {
		if n.ID == id {
			return i
		}
	}
	return -1
}

func findEdge(g *workflowdomain.Graph, id string) int {
	for i, e := range g.Edges {
		if e.ID == id {
			return i
		}
	}
	return -1
}

func edgeRefsNode(e workflowdomain.EdgeSpec, nodeID string) bool {
	return e.From == nodeID || e.To == nodeID
}

func mergeNodePatch(target workflowdomain.NodeSpec, patch json.RawMessage) (workflowdomain.NodeSpec, error) {
	rawTarget, err := json.Marshal(target)
	if err != nil {
		return target, fmt.Errorf("marshal target: %w", err)
	}
	var targetMap map[string]any
	if err := json.Unmarshal(rawTarget, &targetMap); err != nil {
		return target, fmt.Errorf("target → map: %w", err)
	}
	var patchMap map[string]any
	if err := json.Unmarshal(patch, &patchMap); err != nil {
		return target, fmt.Errorf("patch unmarshal: %w", err)
	}
	merged := mergePatch(targetMap, patchMap)
	rawMerged, err := json.Marshal(merged)
	if err != nil {
		return target, fmt.Errorf("marshal merged: %w", err)
	}
	var out workflowdomain.NodeSpec
	if err := json.Unmarshal(rawMerged, &out); err != nil {
		return target, fmt.Errorf("merged → NodeSpec: %w", err)
	}
	return out, nil
}

func mergeEdgePatch(target workflowdomain.EdgeSpec, patch json.RawMessage) (workflowdomain.EdgeSpec, error) {
	rawTarget, err := json.Marshal(target)
	if err != nil {
		return target, fmt.Errorf("marshal target: %w", err)
	}
	var targetMap map[string]any
	if err := json.Unmarshal(rawTarget, &targetMap); err != nil {
		return target, fmt.Errorf("target → map: %w", err)
	}
	var patchMap map[string]any
	if err := json.Unmarshal(patch, &patchMap); err != nil {
		return target, fmt.Errorf("patch unmarshal: %w", err)
	}
	merged := mergePatch(targetMap, patchMap)
	rawMerged, err := json.Marshal(merged)
	if err != nil {
		return target, fmt.Errorf("marshal merged: %w", err)
	}
	var out workflowdomain.EdgeSpec
	if err := json.Unmarshal(rawMerged, &out); err != nil {
		return target, fmt.Errorf("merged → EdgeSpec: %w", err)
	}
	return out, nil
}

// mergePatch implements RFC 7396 recursively; nil values delete the target key.
//
// mergePatch 递归实现 RFC 7396；patch 中 nil 删 target key。
func mergePatch(target, patch map[string]any) map[string]any {
	if target == nil {
		target = map[string]any{}
	}
	for k, v := range patch {
		if v == nil {
			delete(target, k)
			continue
		}
		if sub, ok := v.(map[string]any); ok {
			if existing, exists := target[k].(map[string]any); exists {
				target[k] = mergePatch(existing, sub)
				continue
			}
		}
		target[k] = v
	}
	return target
}

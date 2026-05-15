// apply.go — ops engine for Workflow authoring. 9 ops mutate a Graph
// snapshot (set_meta / add_node / update_node / delete_node / add_edge /
// update_edge / delete_edge / set_variable / unset_variable). Each op
// fires a progress delta on the caller's eventlog block for streaming
// UX. update_node / update_edge use RFC 7396 JSON Merge Patch (same
// scheme as handler's update_method) so LLM "patch just this field"
// flows survive structural drift.
//
// Final-state validation (DAG / refs / required trigger / etc.) lives in
// validate.go and runs at the end of ApplyOps, not per-op — intermediate
// states are allowed to be transiently invalid (add_node before add_edge
// referencing it, etc.).
//
// apply.go —— Workflow authoring 的 ops 引擎。9 op 改 Graph 快照;每 op 推
// 一个 progress delta 给流式 UX 看;update_node/edge 走 RFC 7396 JSON Merge
// Patch(同 handler.update_method);最终态校验在 validate.go,中间态允许
// 临时不合法。

package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
)

// Op is one wire-level op from the LLM. Type is the discriminator; Raw is
// the unmodified body so per-op handlers can decode it into their own
// schema without forcing a single big union type up here.
//
// Op 是来自 LLM 的线上 op 一条。Type 判别;Raw 是未动 body,让每 op handler
// 自己解码到各自 schema。
type Op struct {
	Type string          `json:"op"`
	Raw  json.RawMessage `json:"-"`
}

// ParseOps decodes the JSON array the LLM emits into []Op (each object
// preserved as Raw so the per-op handler decodes its own fields).
//
// ParseOps 把 LLM 发的 JSON 数组解为 []Op;每对象 Raw 保留给后续 handler 用。
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

// ApplyOps applies ops to a base graph and returns the final graph. base
// may be nil (create-from-scratch). progressBlockID, if non-empty, is the
// eventlog progress block to receive one delta per op for streaming UX.
//
// Per-op failures wrap workflowdomain.ErrOpInvalid with context. No
// final-state validation here — callers run validate.go's ValidateGraph
// after ApplyOps when they want the strict check (Service.Create / Edit
// always do).
//
// ApplyOps 把 ops 应用到 base 图返最终图。base 可 nil(从零建)。
// progressBlockID 非空则每 op 推一条 delta。per-op 失败 wrap ErrOpInvalid;
// 不做最终态校验,调用方自己跑 ValidateGraph(Service.Create/Edit 都跑)。
func ApplyOps(ctx context.Context, base *workflowdomain.Graph, ops []Op, progressBlockID string) (*workflowdomain.Graph, error) {
	g := cloneGraph(base)
	em := eventlogpkg.From(ctx)

	for i, op := range ops {
		if err := applyOne(g, op); err != nil {
			return nil, fmt.Errorf("op[%d] %s: %w: %v", i, op.Type, workflowdomain.ErrOpInvalid, err)
		}
		if progressBlockID != "" {
			em.DeltaBlock(ctx, progressBlockID,
				fmt.Sprintf("op[%d] %s ✓\n", i, op.Type))
		}
		_ = eventlogdomain.StatusStreaming // keep import used; eventlogdomain alias is referenced via the progress block flow
	}
	return g, nil
}

// applyOne dispatches one op to its handler. Each handler mutates g in place.
//
// applyOne 派发一个 op 给对应 handler;handler in-place 改 g。
func applyOne(g *workflowdomain.Graph, op Op) error {
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
	default:
		return fmt.Errorf("unknown op type %q", op.Type)
	}
}

// ── per-op handlers ──────────────────────────────────────────────────────────

func applySetMeta(g *workflowdomain.Graph, raw json.RawMessage) error {
	var p struct {
		Name        *string  `json:"name"`
		Description *string  `json:"description"`
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
		ID    string          `json:"id"`
		Patch json.RawMessage `json:"patch"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("update_node unmarshal: %w", err)
	}
	if p.ID == "" {
		return fmt.Errorf("update_node: empty id")
	}
	idx := findNode(g, p.ID)
	if idx < 0 {
		return fmt.Errorf("update_node: node %q not found", p.ID)
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
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("delete_node unmarshal: %w", err)
	}
	if p.ID == "" {
		return fmt.Errorf("delete_node: empty id")
	}
	idx := findNode(g, p.ID)
	if idx < 0 {
		return fmt.Errorf("delete_node: node %q not found", p.ID)
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
		if edgeRefsNode(e, p.ID) {
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

// ── helpers ──────────────────────────────────────────────────────────────────

// cloneGraph deep-copies base so applies don't mutate the caller's view.
// nil base returns an empty Graph (create-from-scratch path).
//
// cloneGraph 深拷 base;nil 返空 Graph(create-from-scratch)。
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
	// Node.Config is a map[string]any — shallow-copy via JSON round-trip per
	// node so update_node mutations don't bleed into the caller's base.
	// 配置 map 深拷;否则 update_node 改 g.Nodes[i].Config 会污染 base。
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

// edgeRefsNode reports whether edge e references node id on either side.
// Post-port-refactor (2026-05), From / To are plain node IDs — no dot
// parsing needed.
//
// edgeRefsNode 报告 edge 是否引用某节点(任一端)。port 重构后 From/To 是
// 纯 node ID,不再走点分隔。
func edgeRefsNode(e workflowdomain.EdgeSpec, nodeID string) bool {
	return e.From == nodeID || e.To == nodeID
}

// mergeNodePatch applies a JSON Merge Patch (RFC 7396) to one NodeSpec.
// Same pattern as handler.mergeMethodPatch.
//
// mergeNodePatch 对 NodeSpec 应用 JSON Merge Patch(RFC 7396);跟
// handler.mergeMethodPatch 同模式。
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

// mergePatch implements RFC 7396 — patch values overwrite recursively; nil
// values in patch delete the target key.
//
// mergePatch 实现 RFC 7396 — patch 值覆盖(递归);nil 值删 target 键。
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

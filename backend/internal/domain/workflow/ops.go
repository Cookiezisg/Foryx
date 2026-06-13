package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Op is a JSON-discriminated graph-edit op; Type lives in the `op` field, Raw holds the
// body. Mirrors function's forge-op model but graph-shaped: the seven ops below edit a
// graph (nodes/edges) plus the header meta, each taking effect in declared order.
//
// Op 是 JSON 判别式图编辑 op；Type 在 `op` 字段，Raw 存 body。镜像 function 的锻造 op 模型但图
// 形：下面七个 op 编辑图（nodes/edges）+ 头部 meta，按声明序逐个生效。
type Op struct {
	Type string
	Raw  json.RawMessage
}

// Op types. set_meta patches header identity (name/description/tags) and does not touch the
// graph; the rest mutate the graph.
//
// Op 类型。set_meta 改头部身份（name/description/tags）、不动图；其余改图。
const (
	OpSetMeta    = "set_meta"
	OpAddNode    = "add_node"
	OpUpdateNode = "update_node"
	OpDeleteNode = "delete_node"
	OpAddEdge    = "add_edge"
	OpUpdateEdge = "update_edge"
	OpDeleteEdge = "delete_edge"
)

// MetaPatch is the accumulated header change from set_meta ops; nil field = unchanged. The
// app layer applies it to the Workflow row (a header concern; ApplyOps only owns the graph).
//
// MetaPatch 是 set_meta ops 累积的头部变更；nil 字段 = 不变。app 层把它应用到 Workflow 行
// （头部关注点；ApplyOps 只管图）。
type MetaPatch struct {
	Name        *string
	Description *string
	Tags        *[]string
	Concurrency *string // overlap policy; validated by the consumer. overlap 政策；消费方校验。
}

// ParseOps decodes the wire format (JSON array with `op` discriminator) into []Op. Unlike
// function it does NOT json-repair: workflow ops come from a structured editor / tool, not
// free-form LLM prose, so a malformed body is a real error to surface, not paper over.
//
// ParseOps 把线上格式（带 `op` 判别字段的 JSON 数组）解码为 []Op。与 function 不同，它不修 JSON：
// workflow ops 来自结构化编辑器 / 工具、非自由 LLM 文本，故畸形体是该上呈的真错误，不掩盖。
func ParseOps(raw json.RawMessage) ([]Op, error) {
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, ErrInvalidOps.WithDetails(map[string]any{"reason": fmt.Sprintf("ops is not a JSON array: %v", err)})
	}
	ops := make([]Op, 0, len(arr))
	for i, r := range arr {
		var disc struct {
			Op string `json:"op"`
		}
		if err := json.Unmarshal(r, &disc); err != nil {
			return nil, ErrInvalidOps.WithDetails(map[string]any{"reason": fmt.Sprintf("ops[%d]: %v", i, err)})
		}
		if disc.Op == "" {
			return nil, ErrInvalidOps.WithDetails(map[string]any{"reason": fmt.Sprintf("ops[%d]: missing 'op' discriminator", i)})
		}
		ops = append(ops, Op{Type: disc.Op, Raw: r})
	}
	return ops, nil
}

// ApplyOps applies the graph-mutating ops to a clone of base and returns the new graph. It
// never mutates base. set_meta ops are recognized (and shape-checked) but do not touch the
// graph — use ExtractMeta to fold them into the header. A malformed op or one that leaves
// the graph inconsistent (unknown node/edge id, duplicate id) returns ErrInvalidOps; final
// structural validity (ValidateGraph) is the caller's separate gate.
//
// ApplyOps 把改图 ops 应用到 base 的克隆并返回新图。绝不改 base。set_meta 被识别（并查形状）但
// 不动图——用 ExtractMeta 把它折进头部。畸形 op 或令图不一致的 op（未知 node/edge id、重复 id）
// 返回 ErrInvalidOps；最终结构合法（ValidateGraph）是调用方另设的闸。
func ApplyOps(base *Graph, ops []Op) (*Graph, error) {
	g := cloneGraph(base)
	for i, op := range ops {
		if err := applyOne(g, op); err != nil {
			return nil, ErrInvalidOps.WithDetails(map[string]any{"reason": fmt.Sprintf("ops[%d] (%s): %v", i, op.Type, err)})
		}
	}
	return g, nil
}

// ExtractMeta folds every set_meta op (in order) into one MetaPatch; later ops win per
// field. A malformed set_meta body returns ErrInvalidOps.
//
// ExtractMeta 把每个 set_meta op（按序）折成一个 MetaPatch；后者按字段胜出。畸形 set_meta 体
// 返回 ErrInvalidOps。
func ExtractMeta(ops []Op) (MetaPatch, error) {
	var patch MetaPatch
	for i, op := range ops {
		if op.Type != OpSetMeta {
			continue
		}
		var p struct {
			Name        *string   `json:"name,omitempty"`
			Description *string   `json:"description,omitempty"`
			Tags        *[]string `json:"tags,omitempty"`
			Concurrency *string   `json:"concurrency,omitempty"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return MetaPatch{}, ErrInvalidOps.WithDetails(map[string]any{"reason": fmt.Sprintf("ops[%d] (set_meta): %v", i, err)})
		}
		if p.Concurrency != nil {
			patch.Concurrency = p.Concurrency
		}
		if p.Name != nil {
			patch.Name = p.Name
		}
		if p.Description != nil {
			patch.Description = p.Description
		}
		if p.Tags != nil {
			patch.Tags = p.Tags
		}
	}
	return patch, nil
}

func applyOne(g *Graph, op Op) error {
	switch op.Type {
	case OpSetMeta:
		// Graph-neutral: shape-check the body so a malformed set_meta is caught here too,
		// but apply nothing to the graph (ExtractMeta owns the header projection).
		//
		// 与图无关：在此也查 body 形状以捕获畸形 set_meta，但不动图（ExtractMeta 负责头部投影）。
		var p struct {
			Name        *string   `json:"name,omitempty"`
			Description *string   `json:"description,omitempty"`
			Tags        *[]string `json:"tags,omitempty"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("set_meta unmarshal: %w", err)
		}
		return nil

	case OpAddNode:
		var p struct {
			Node Node `json:"node"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("add_node unmarshal: %w", err)
		}
		if strings.TrimSpace(p.Node.ID) == "" {
			return fmt.Errorf("add_node: node.id is required")
		}
		if findNode(g, p.Node.ID) >= 0 {
			return fmt.Errorf("add_node: node id %q already exists", p.Node.ID)
		}
		g.Nodes = append(g.Nodes, p.Node)
		return nil

	case OpUpdateNode:
		// RFC7396-ish merge patch on a node's wiring: re-marshal the target node, JSON-merge
		// the patch over it (a present key overwrites, including a deep replace of input/retry),
		// then unmarshal back. id is immutable (a patch may not move a node's identity).
		//
		// 节点接线的 RFC7396 式合并 patch：把目标节点重 marshal、用 patch JSON-merge 覆盖（出现的键
		// 覆写，含 input/retry 的深替换）、再 unmarshal 回来。id 不可变（patch 不得移动节点身份）。
		var p struct {
			ID    string          `json:"id"`
			Patch json.RawMessage `json:"patch"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("update_node unmarshal: %w", err)
		}
		idx := findNode(g, p.ID)
		if idx < 0 {
			return fmt.Errorf("update_node: unknown node id %q", p.ID)
		}
		merged, err := mergeNode(g.Nodes[idx], p.Patch)
		if err != nil {
			return fmt.Errorf("update_node %q: %w", p.ID, err)
		}
		merged.ID = g.Nodes[idx].ID // id is immutable across an update
		g.Nodes[idx] = merged
		return nil

	case OpDeleteNode:
		var p struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("delete_node unmarshal: %w", err)
		}
		idx := findNode(g, p.ID)
		if idx < 0 {
			return fmt.Errorf("delete_node: unknown node id %q", p.ID)
		}
		g.Nodes = append(g.Nodes[:idx], g.Nodes[idx+1:]...)
		// Cascade: drop every edge touching the deleted node, so no dangling edge survives.
		//
		// 级联：删掉触及被删节点的每条边，使无悬挂边残留。
		kept := g.Edges[:0:0]
		for _, e := range g.Edges {
			if e.From != p.ID && e.To != p.ID {
				kept = append(kept, e)
			}
		}
		g.Edges = kept
		return nil

	case OpAddEdge:
		var p struct {
			Edge Edge `json:"edge"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("add_edge unmarshal: %w", err)
		}
		if strings.TrimSpace(p.Edge.ID) == "" {
			return fmt.Errorf("add_edge: edge.id is required")
		}
		if findEdge(g, p.Edge.ID) >= 0 {
			return fmt.Errorf("add_edge: edge id %q already exists", p.Edge.ID)
		}
		g.Edges = append(g.Edges, p.Edge)
		return nil

	case OpUpdateEdge:
		var p struct {
			ID    string          `json:"id"`
			Patch json.RawMessage `json:"patch"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("update_edge unmarshal: %w", err)
		}
		idx := findEdge(g, p.ID)
		if idx < 0 {
			return fmt.Errorf("update_edge: unknown edge id %q", p.ID)
		}
		merged, err := mergeEdge(g.Edges[idx], p.Patch)
		if err != nil {
			return fmt.Errorf("update_edge %q: %w", p.ID, err)
		}
		merged.ID = g.Edges[idx].ID // id is immutable across an update
		g.Edges[idx] = merged
		return nil

	case OpDeleteEdge:
		var p struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("delete_edge unmarshal: %w", err)
		}
		idx := findEdge(g, p.ID)
		if idx < 0 {
			return fmt.Errorf("delete_edge: unknown edge id %q", p.ID)
		}
		g.Edges = append(g.Edges[:idx], g.Edges[idx+1:]...)
		return nil

	default:
		return fmt.Errorf("unknown op type %q", op.Type)
	}
}

// mergeNode applies a JSON merge patch over node n: marshal n, overlay patch, unmarshal.
// A present key replaces (input/retry are replaced wholesale, not deep-merged — the merge
// is at the top-level node field granularity, which is what a wiring edit wants).
//
// mergeNode 在节点 n 上套 JSON merge patch：marshal n、叠 patch、unmarshal。出现的键替换
// （input/retry 整体替换、非深合并——合并粒度为节点顶层字段，正合接线编辑所需）。
func mergeNode(n Node, patch json.RawMessage) (Node, error) {
	if len(patch) == 0 {
		return n, nil
	}
	base, err := json.Marshal(n)
	if err != nil {
		return Node{}, err
	}
	merged, err := mergeJSON(base, patch)
	if err != nil {
		return Node{}, err
	}
	var out Node
	if err := json.Unmarshal(merged, &out); err != nil {
		return Node{}, err
	}
	return out, nil
}

func mergeEdge(e Edge, patch json.RawMessage) (Edge, error) {
	if len(patch) == 0 {
		return e, nil
	}
	base, err := json.Marshal(e)
	if err != nil {
		return Edge{}, err
	}
	merged, err := mergeJSON(base, patch)
	if err != nil {
		return Edge{}, err
	}
	var out Edge
	if err := json.Unmarshal(merged, &out); err != nil {
		return Edge{}, err
	}
	return out, nil
}

// mergeJSON overlays patch object onto base object at the top level (a present key wins;
// objects are NOT recursed — top-level granularity). Both must be JSON objects.
//
// mergeJSON 把 patch 对象顶层叠到 base 对象（出现的键胜；对象不递归——顶层粒度）。两者须为 JSON 对象。
func mergeJSON(base, patch json.RawMessage) (json.RawMessage, error) {
	var b, p map[string]json.RawMessage
	if err := json.Unmarshal(base, &b); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(patch, &p); err != nil {
		return nil, fmt.Errorf("patch is not an object: %w", err)
	}
	for k, v := range p {
		b[k] = v
	}
	return json.Marshal(b)
}

func findNode(g *Graph, id string) int {
	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			return i
		}
	}
	return -1
}

func findEdge(g *Graph, id string) int {
	for i := range g.Edges {
		if g.Edges[i].ID == id {
			return i
		}
	}
	return -1
}

// cloneGraph deep-copies a graph so ApplyOps never mutates its input. A nil base yields an
// empty (non-nil) graph.
//
// cloneGraph 深拷图，使 ApplyOps 绝不改输入。nil base 得空（非 nil）图。
func cloneGraph(g *Graph) *Graph {
	out := &Graph{Nodes: []Node{}, Edges: []Edge{}}
	if g == nil {
		return out
	}
	for _, n := range g.Nodes {
		out.Nodes = append(out.Nodes, cloneNode(n))
	}
	out.Edges = append(out.Edges, g.Edges...)
	return out
}

func cloneNode(n Node) Node {
	c := n
	if n.Input != nil {
		c.Input = make(map[string]string, len(n.Input))
		for k, v := range n.Input {
			c.Input[k] = v
		}
	}
	if n.Retry != nil {
		r := *n.Retry
		c.Retry = &r
	}
	if n.Pos != nil {
		p := *n.Pos
		c.Pos = &p
	}
	return c
}

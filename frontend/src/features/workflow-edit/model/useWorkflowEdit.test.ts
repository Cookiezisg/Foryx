// Unit tests for the pure diff helpers in useWorkflowEdit.
// Covers diffToOps three-way (add/update/delete) + nodeToSpec/edgeToSpec mapping.
//
// 覆盖 diffToOps 三向 diff（add/update/delete）和属性映射函数。

import { describe, expect, it } from "vitest";
import {
  diffToOps,
  nodeToSpec,
  edgeToSpec,
  edgeKey,
  type CanvasNode,
  type CanvasEdge,
  type CanvasGraph,
} from "./useWorkflowEdit";

// ── Helpers ───────────────────────────────────────────────────────────────

function makeNode(id: string, overrides: Partial<CanvasNode> = {}): CanvasNode {
  return {
    id,
    kind: "function",
    label: id,
    notes: "",
    config: {},
    onError: "",
    timeout: 0,
    x: 100,
    y: 200,
    ...overrides,
  };
}

function makeEdge(from: string, to: string, overrides: Partial<CanvasEdge> = {}): CanvasEdge {
  return { id: `e_${from}_${to}`, from, to, ...overrides };
}

function emptyGraph(): CanvasGraph {
  return { nodes: [], edges: [] };
}

// ── diffToOps: add ────────────────────────────────────────────────────────

describe("diffToOps — add", () => {
  it("adds a new node when it does not exist in orig", () => {
    const orig = emptyGraph();
    const next: CanvasGraph = { nodes: [makeNode("n1")], edges: [] };
    const ops = diffToOps(orig, next);
    expect(ops).toHaveLength(1);
    expect(ops[0].op).toBe("add_node");
    expect((ops[0].node as any).id).toBe("n1");
  });

  it("adds a new edge when it does not exist in orig", () => {
    const n1 = makeNode("n1");
    const n2 = makeNode("n2");
    const orig: CanvasGraph = { nodes: [n1, n2], edges: [] };
    const next: CanvasGraph = { nodes: [n1, n2], edges: [makeEdge("n1", "n2")] };
    const ops = diffToOps(orig, next);
    const edgeOps = ops.filter((o) => o.op === "add_edge");
    expect(edgeOps).toHaveLength(1);
    expect((edgeOps[0].edge as any).from).toBe("n1");
    expect((edgeOps[0].edge as any).to).toBe("n2");
  });

  it("emits add_node + add_edge when both are new", () => {
    const orig = emptyGraph();
    const n1 = makeNode("n1");
    const n2 = makeNode("n2");
    const next: CanvasGraph = { nodes: [n1, n2], edges: [makeEdge("n1", "n2")] };
    const ops = diffToOps(orig, next);
    const nodeOps = ops.filter((o) => o.op === "add_node");
    const edgeOps = ops.filter((o) => o.op === "add_edge");
    expect(nodeOps).toHaveLength(2);
    expect(edgeOps).toHaveLength(1);
  });
});

// ── diffToOps: update ─────────────────────────────────────────────────────

describe("diffToOps — update", () => {
  it("emits update_node when config changes", () => {
    const orig: CanvasGraph = { nodes: [makeNode("n1", { config: { a: 1 } })], edges: [] };
    const next: CanvasGraph = { nodes: [makeNode("n1", { config: { a: 2 } })], edges: [] };
    const ops = diffToOps(orig, next);
    expect(ops).toHaveLength(1);
    expect(ops[0].op).toBe("update_node");
    expect((ops[0].node as any).id).toBe("n1");
  });

  it("emits update_node when notes changes", () => {
    const orig: CanvasGraph = { nodes: [makeNode("n1", { notes: "old" })], edges: [] };
    const next: CanvasGraph = { nodes: [makeNode("n1", { notes: "new" })], edges: [] };
    const ops = diffToOps(orig, next);
    expect(ops).toHaveLength(1);
    expect(ops[0].op).toBe("update_node");
  });

  it("emits update_node when position changes", () => {
    const orig: CanvasGraph = { nodes: [makeNode("n1", { x: 100, y: 200 })], edges: [] };
    const next: CanvasGraph = { nodes: [makeNode("n1", { x: 300, y: 400 })], edges: [] };
    const ops = diffToOps(orig, next);
    expect(ops).toHaveLength(1);
    expect(ops[0].op).toBe("update_node");
  });

  it("emits update_node when timeout changes", () => {
    const orig: CanvasGraph = { nodes: [makeNode("n1", { timeout: 0 })], edges: [] };
    const next: CanvasGraph = { nodes: [makeNode("n1", { timeout: 30 })], edges: [] };
    const ops = diffToOps(orig, next);
    expect(ops).toHaveLength(1);
    expect(ops[0].op).toBe("update_node");
  });

  it("emits update_node when onError changes", () => {
    const orig: CanvasGraph = { nodes: [makeNode("n1", { onError: "" })], edges: [] };
    const next: CanvasGraph = { nodes: [makeNode("n1", { onError: "skip" })], edges: [] };
    const ops = diffToOps(orig, next);
    expect(ops).toHaveLength(1);
    expect(ops[0].op).toBe("update_node");
  });

  it("emits update_node (not add/delete) for unchanged canvas nodes", () => {
    const node = makeNode("n1");
    const orig: CanvasGraph = { nodes: [node], edges: [] };
    const next: CanvasGraph = { nodes: [{ ...node }], edges: [] };
    // nodeChanged checks a.type vs b.kind; canvas nodes have no .type field,
    // so undefined !== "function" → always detects change. This is the original
    // verbatim behaviour preserved exactly.
    const ops = diffToOps(orig, next);
    expect(ops.some((o) => o.op === "update_node")).toBe(true);
    expect(ops.every((o) => o.op !== "add_node")).toBe(true);
    expect(ops.every((o) => o.op !== "delete_node")).toBe(true);
  });
});

// ── diffToOps: delete ─────────────────────────────────────────────────────

describe("diffToOps — delete", () => {
  it("emits delete_node when a node is removed", () => {
    const n1 = makeNode("n1");
    const n2 = makeNode("n2");
    const orig: CanvasGraph = { nodes: [n1, n2], edges: [] };
    const next: CanvasGraph = { nodes: [n1], edges: [] };
    const ops = diffToOps(orig, next);
    const del = ops.filter((o) => o.op === "delete_node");
    expect(del).toHaveLength(1);
    expect(del[0].id).toBe("n2");
  });

  it("emits delete_edge when an edge is removed", () => {
    const n1 = makeNode("n1");
    const n2 = makeNode("n2");
    const e = makeEdge("n1", "n2");
    const orig: CanvasGraph = { nodes: [n1, n2], edges: [e] };
    const next: CanvasGraph = { nodes: [n1, n2], edges: [] };
    const ops = diffToOps(orig, next);
    const del = ops.filter((o) => o.op === "delete_edge");
    expect(del).toHaveLength(1);
    expect(del[0].id).toBe(e.id);
  });

  it("falls back to edgeKey as delete id when edge has no id", () => {
    const n1 = makeNode("n1");
    const n2 = makeNode("n2");
    const e: CanvasEdge = { from: "n1", to: "n2" };
    const orig: CanvasGraph = { nodes: [n1, n2], edges: [e] };
    const next: CanvasGraph = { nodes: [n1, n2], edges: [] };
    const ops = diffToOps(orig, next);
    const del = ops.filter((o) => o.op === "delete_edge");
    expect(del).toHaveLength(1);
    expect(del[0].id).toBe(edgeKey(e));
  });

  it("emits delete_node + delete_edge when node with connected edge is removed", () => {
    const n1 = makeNode("n1");
    const n2 = makeNode("n2");
    const e = makeEdge("n1", "n2");
    const orig: CanvasGraph = { nodes: [n1, n2], edges: [e] };
    const next: CanvasGraph = { nodes: [n1], edges: [] };
    const ops = diffToOps(orig, next);
    expect(ops.some((o) => o.op === "delete_node" && o.id === "n2")).toBe(true);
    expect(ops.some((o) => o.op === "delete_edge")).toBe(true);
  });

  it("emits no ops for empty→empty graph", () => {
    const ops = diffToOps(emptyGraph(), emptyGraph());
    expect(ops).toHaveLength(0);
  });
});

// ── nodeToSpec: attribute mapping ─────────────────────────────────────────

describe("nodeToSpec — attribute mapping", () => {
  it("maps id, kind→type, x/y→position (rounded), config, notes, onError, timeout", () => {
    const n = makeNode("n1", {
      kind: "handler",
      x: 123.7,
      y: 456.2,
      config: { ref: "foo" },
      notes: "a note",
      onError: "fail",
      timeout: 30,
    });
    const spec = nodeToSpec(n);
    expect(spec.id).toBe("n1");
    expect(spec.type).toBe("handler");
    expect(spec.position).toEqual({ x: 124, y: 456 });
    expect(spec.config).toEqual({ ref: "foo" });
    expect(spec.notes).toBe("a note");
    expect(spec.onError).toBe("fail");
    expect(spec.timeout).toBe(30);
    expect(spec).not.toHaveProperty("retry");
  });

  it("includes retry only when present on node", () => {
    const n = makeNode("n1", { retry: { maxAttempts: 3 } });
    const spec = nodeToSpec(n);
    expect(spec).toHaveProperty("retry");
    expect((spec as any).retry).toEqual({ maxAttempts: 3 });
  });

  it("defaults config to {} when missing", () => {
    const n = makeNode("n1");
    n.config = undefined as any;
    const spec = nodeToSpec(n);
    expect(spec.config).toEqual({});
  });

  it("defaults notes/onError to empty string", () => {
    const n = makeNode("n1");
    n.notes = undefined as any;
    n.onError = undefined as any;
    const spec = nodeToSpec(n);
    expect(spec.notes).toBe("");
    expect(spec.onError).toBe("");
  });

  it("defaults timeout to 0", () => {
    const n = makeNode("n1");
    n.timeout = undefined as any;
    const spec = nodeToSpec(n);
    expect(spec.timeout).toBe(0);
  });
});

// ── edgeToSpec: attribute mapping ─────────────────────────────────────────

describe("edgeToSpec — attribute mapping", () => {
  it("preserves id, from, to", () => {
    const e = makeEdge("n1", "n2", { id: "e_fixed" });
    const spec = edgeToSpec(e);
    expect(spec.id).toBe("e_fixed");
    expect(spec.from).toBe("n1");
    expect(spec.to).toBe("n2");
  });

  it("generates an id when edge has no id", () => {
    const e: CanvasEdge = { from: "n1", to: "n2" };
    const spec = edgeToSpec(e);
    expect(spec.id).toMatch(/^e_/);
  });

  it("includes fromPort only when present", () => {
    const withPort = makeEdge("n1", "n2", { fromPort: "out" });
    const withoutPort = makeEdge("n1", "n2");
    expect(edgeToSpec(withPort)).toHaveProperty("fromPort", "out");
    expect(edgeToSpec(withoutPort)).not.toHaveProperty("fromPort");
  });

  it("includes toPort only when present", () => {
    const withPort = makeEdge("n1", "n2", { toPort: "in" });
    const withoutPort = makeEdge("n1", "n2");
    expect(edgeToSpec(withPort)).toHaveProperty("toPort", "in");
    expect(edgeToSpec(withoutPort)).not.toHaveProperty("toPort");
  });
});

// ── edgeKey ───────────────────────────────────────────────────────────────

describe("edgeKey", () => {
  it("produces stable key from from/to", () => {
    const e = makeEdge("n1", "n2");
    expect(edgeKey(e)).toBe("n1|->n2|");
  });

  it("includes fromPort and toPort in key", () => {
    const e = makeEdge("n1", "n2", { fromPort: "out", toPort: "in" });
    expect(edgeKey(e)).toBe("n1|out->n2|in");
  });

  it("two edges with same from/to but different ports have different keys", () => {
    const e1 = makeEdge("n1", "n2", { fromPort: "a" });
    const e2 = makeEdge("n1", "n2", { fromPort: "b" });
    expect(edgeKey(e1)).not.toBe(edgeKey(e2));
  });
});

// useWorkflowEdit — extra branch coverage.
// 1) markDirty with identical graph → ops.length=0 branch (line 146: setDirty(false) + return).
// 2) nodeChanged position?.x branches: set a.type===b.kind so the early-return
//    at line 80 is NOT hit, then position branches on lines 82-83 are reachable.

import { describe, expect, it, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React, { createElement } from "react";
import { diffToOps, useWorkflowEdit, type CanvasNode, type CanvasGraph } from "./useWorkflowEdit";

vi.mock("@entities/workflow", () => ({
  useEditWorkflow: () => ({ mutate: vi.fn(), isPending: false }),
}));

function makeWrapper() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client }, children);
  return { wrapper };
}

// ── markDirty with no-op graph (ops.length === 0 branch) ──────────────────

describe("useWorkflowEdit — markDirty with identical graph", () => {
  it("identicalGraph_resetsDirtyAndReturns", () => {
    vi.useFakeTimers();
    const { wrapper } = makeWrapper();
    // Use a fixed orig graph that, when diffed against itself, produces 0 ops.
    // Canvas nodes always have undefined .type so nodeChanged returns true for
    // same node id (undefined !== "function"). Use empty graphs to guarantee 0 ops.
    const orig: CanvasGraph = { nodes: [], edges: [] };
    const { result } = renderHook(() => useWorkflowEdit("wf_empty", orig), { wrapper });

    act(() => { result.current.markDirty({ nodes: [], edges: [] }); });
    expect(result.current.dirty).toBe(true);

    // Advance past the 2s debounce to trigger the timeout callback.
    act(() => { vi.advanceTimersByTime(2100); });
    // ops.length === 0 → setDirty(false) branch taken.
    expect(result.current.dirty).toBe(false);

    vi.useRealTimers();
  });
});

// ── nodeChanged position?.x branches ─────────────────────────────────────
// To reach line 82 (position?.x ?? x check), a.type must equal b.kind so the
// early return at line 80 does NOT fire. We cast orig node with .type set.

type NodeWithType = CanvasNode & { type?: string; position?: { x: number; y: number } };

describe("diffToOps — position?.x branches in nodeChanged", () => {
  it("positionPresent_same_noUpdate", () => {
    const orig: NodeWithType = {
      id: "n1", kind: "function", type: "function", label: "n1",
      notes: "", config: {}, onError: "", timeout: 0, x: 100, y: 200,
      position: { x: 100, y: 200 },
    };
    const next: CanvasNode = {
      id: "n1", kind: "function", label: "n1",
      notes: "", config: {}, onError: "", timeout: 0, x: 100, y: 200,
    };
    const origGraph: CanvasGraph = { nodes: [orig as CanvasNode], edges: [] };
    const nextGraph: CanvasGraph = { nodes: [next], edges: [] };
    const ops = diffToOps(origGraph, nextGraph);
    // type===kind, position.x===x, position.y===y, all equal → no update_node
    const updateOps = ops.filter((o) => o.op === "update_node");
    expect(updateOps).toHaveLength(0);
  });

  it("positionPresent_differentX_triggersUpdate", () => {
    const orig: NodeWithType = {
      id: "n1", kind: "function", type: "function", label: "n1",
      notes: "", config: {}, onError: "", timeout: 0, x: 100, y: 200,
      position: { x: 100, y: 200 },
    };
    const next: CanvasNode = {
      id: "n1", kind: "function", label: "n1",
      notes: "", config: {}, onError: "", timeout: 0, x: 300, y: 200,
    };
    const origGraph: CanvasGraph = { nodes: [orig as CanvasNode], edges: [] };
    const nextGraph: CanvasGraph = { nodes: [next], edges: [] };
    const ops = diffToOps(origGraph, nextGraph);
    // position.x(100) !== next.x(300) → update
    expect(ops.filter((o) => o.op === "update_node")).toHaveLength(1);
  });

  it("noPosition_fallsBackToX_same_noUpdate", () => {
    const orig: NodeWithType = {
      id: "n1", kind: "function", type: "function", label: "n1",
      notes: "", config: {}, onError: "", timeout: 0, x: 100, y: 200,
    };
    const next: CanvasNode = {
      id: "n1", kind: "function", label: "n1",
      notes: "", config: {}, onError: "", timeout: 0, x: 100, y: 200,
    };
    const origGraph: CanvasGraph = { nodes: [orig as CanvasNode], edges: [] };
    const nextGraph: CanvasGraph = { nodes: [next], edges: [] };
    const ops = diffToOps(origGraph, nextGraph);
    // no position → falls back to x/y; same values → no update
    expect(ops.filter((o) => o.op === "update_node")).toHaveLength(0);
  });
});

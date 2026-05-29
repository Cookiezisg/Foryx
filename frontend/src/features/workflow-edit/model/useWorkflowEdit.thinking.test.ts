// useWorkflowEdit — modelOverrideEq thinking comparison regression guard.
// Verifies that a thinking-only change on an existing node produces a
// set_node_model_override op (not silently swallowed).
//
// modelOverrideEq 含 thinking 字段比较的回归守卫：thinking-only 变化必须触发
// set_node_model_override op，不能静默丢弃。

import { describe, expect, it, vi } from "vitest";
import { diffToOps, type CanvasNode, type CanvasGraph } from "./useWorkflowEdit";

vi.mock("@entities/workflow", () => ({
  useEditWorkflow: () => ({ mutate: vi.fn(), isPending: false }),
}));

function makeAgent(id: string, override?: CanvasNode["modelOverride"]): CanvasNode {
  return {
    id,
    kind: "agent",
    label: id,
    notes: "",
    config: {},
    onError: "",
    timeout: 0,
    x: 100,
    y: 200,
    modelOverride: override ?? null,
  };
}

// ── modelOverrideEq thinking comparisons ─────────────────────────────────

describe("diffToOps — thinking-only modelOverride changes", () => {
  it("thinkingOnly_modeChange_emitsSetOverrideOp", () => {
    const orig: CanvasGraph = {
      nodes: [makeAgent("n1", { apiKeyId: "aki_a", modelId: "m_a" })],
      edges: [],
    };
    const next: CanvasGraph = {
      nodes: [makeAgent("n1", { apiKeyId: "aki_a", modelId: "m_a", thinking: { mode: "on" } })],
      edges: [],
    };
    const ops = diffToOps(orig, next);
    const overrideOps = ops.filter((o) => o.op === "set_node_model_override");
    expect(overrideOps).toHaveLength(1);
    expect(overrideOps[0]).toMatchObject({
      op: "set_node_model_override",
      nodeId: "n1",
      modelOverride: { apiKeyId: "aki_a", modelId: "m_a", thinking: { mode: "on" } },
    });
  });

  it("thinkingOnly_effortChange_emitsSetOverrideOp", () => {
    const orig: CanvasGraph = {
      nodes: [makeAgent("n1", { apiKeyId: "aki_a", modelId: "m_a", thinking: { mode: "on", effort: "low" } })],
      edges: [],
    };
    const next: CanvasGraph = {
      nodes: [makeAgent("n1", { apiKeyId: "aki_a", modelId: "m_a", thinking: { mode: "on", effort: "high" } })],
      edges: [],
    };
    const ops = diffToOps(orig, next);
    const overrideOps = ops.filter((o) => o.op === "set_node_model_override");
    expect(overrideOps).toHaveLength(1);
  });

  it("thinkingOnly_budgetChange_emitsSetOverrideOp", () => {
    const orig: CanvasGraph = {
      nodes: [makeAgent("n1", { apiKeyId: "aki_a", modelId: "m_a", thinking: { mode: "on", budget: 1024 } })],
      edges: [],
    };
    const next: CanvasGraph = {
      nodes: [makeAgent("n1", { apiKeyId: "aki_a", modelId: "m_a", thinking: { mode: "on", budget: 8000 } })],
      edges: [],
    };
    const ops = diffToOps(orig, next);
    const overrideOps = ops.filter((o) => o.op === "set_node_model_override");
    expect(overrideOps).toHaveLength(1);
  });

  it("sameThinking_doesNotEmitOp", () => {
    const override = { apiKeyId: "aki_a", modelId: "m_a", thinking: { mode: "on" as const, effort: "high" } };
    const orig: CanvasGraph = { nodes: [makeAgent("n1", { ...override, thinking: { ...override.thinking } })], edges: [] };
    const next: CanvasGraph = { nodes: [makeAgent("n1", { ...override, thinking: { ...override.thinking } })], edges: [] };
    const ops = diffToOps(orig, next);
    expect(ops.filter((o) => o.op === "set_node_model_override")).toHaveLength(0);
  });

  it("thinkingRemovedFromOverride_emitsSetOverrideOp", () => {
    const orig: CanvasGraph = {
      nodes: [makeAgent("n1", { apiKeyId: "aki_a", modelId: "m_a", thinking: { mode: "off" } })],
      edges: [],
    };
    const next: CanvasGraph = {
      nodes: [makeAgent("n1", { apiKeyId: "aki_a", modelId: "m_a" })],
      edges: [],
    };
    const ops = diffToOps(orig, next);
    const overrideOps = ops.filter((o) => o.op === "set_node_model_override");
    expect(overrideOps).toHaveLength(1);
  });
});

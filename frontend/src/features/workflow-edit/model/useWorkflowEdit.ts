// Orchestrates diff-based autosave for the workflow editor canvas.
// 2s debounced timer + diffToOps → POST :edit. Verbatim from WorkflowEditor.
//
// 封装 WorkflowEditor 的 diff + 2s 防抖 autosave 编排；算法逐字保留。

import { useCallback, useEffect, useRef, useState } from "react";
import { useEditWorkflow } from "@entities/workflow";
import type { WorkflowEditOp } from "@entities/workflow";
import type { ModelRef } from "@entities/conversation";

// ── Canvas-shape node/edge types (local to the editor canvas) ────────────

export interface CanvasNode {
  id: string;
  kind: string;
  label?: string;
  notes?: string;
  config?: Record<string, unknown>;
  onError?: string;
  timeout?: number;
  retry?: unknown;
  modelOverride?: ModelRef | null;
  x: number;
  y: number;
  sub?: string;
}

export interface CanvasEdge {
  id?: string;
  from: string;
  to: string;
  fromPort?: string;
  toPort?: string;
  fromHandle?: string;
  toHandle?: string;
}

export interface CanvasGraph {
  nodes: CanvasNode[];
  edges: CanvasEdge[];
}

// ── Pure diff helpers — exported for unit-testing ────────────────────────

export function edgeKey(e: CanvasEdge): string {
  return `${e.from}|${e.fromPort || ""}->${e.to}|${e.toPort || ""}`;
}

function newEdgeId(): string {
  return "e_" + Math.random().toString(36).slice(2, 8);
}

export function nodeToSpec(n: CanvasNode) {
  return {
    id: n.id,
    type: n.kind,
    position: { x: Math.round(n.x), y: Math.round(n.y) },
    config: n.config || {},
    notes: n.notes || "",
    onError: n.onError || "",
    timeout: n.timeout || 0,
    ...(n.retry ? { retry: n.retry } : {}),
    ...(n.modelOverride ? { modelOverride: n.modelOverride } : {}),
  };
}

// nodeToPatch — wire shape for update_node: full post-edit content as the
// merge-patch body. modelOverride is excluded (carried by the dedicated
// set_node_model_override op so the backend can validate apiKeyId ownership);
// id is excluded because nodeId carries it.
//
// nodeToPatch —— update_node 的 patch 体，发送编辑后全字段;
// modelOverride 不进 patch(由专属 op 携带,后端校验 apiKeyId 归属);
// id 不进 patch(nodeId 已携带)。
export function nodeToPatch(n: CanvasNode) {
  return {
    type: n.kind,
    position: { x: Math.round(n.x), y: Math.round(n.y) },
    config: n.config || {},
    notes: n.notes || "",
    onError: n.onError || "",
    timeout: n.timeout || 0,
    ...(n.retry ? { retry: n.retry } : {}),
  };
}

// modelOverrideEq — true when two ModelRef|null|undefined values are
// semantically equal (treating null and undefined as "no override"). Compares
// apiKeyId, modelId, AND thinking (mode+effort+budget) so a thinking-only
// change triggers set_node_model_override.
//
// modelOverrideEq —— 比较两个 ModelRef,null/undefined 语义相同;含 thinking 字段
// 比较,thinking-only 变化也会触发 set_node_model_override。
function thinkingEq(a: ModelRef["thinking"], b: ModelRef["thinking"]): boolean {
  if (!a && !b) return true;
  if (!a || !b) return false;
  return a.mode === b.mode && (a.effort ?? "") === (b.effort ?? "") && (a.budget ?? 0) === (b.budget ?? 0);
}

function modelOverrideEq(a?: ModelRef | null, b?: ModelRef | null): boolean {
  if (!a && !b) return true;
  if (!a || !b) return false;
  return a.apiKeyId === b.apiKeyId && a.modelId === b.modelId && thinkingEq(a.thinking, b.thinking);
}

export function edgeToSpec(e: CanvasEdge) {
  return {
    id: e.id || newEdgeId(),
    from: e.from,
    to: e.to,
    ...(e.fromPort ? { fromPort: e.fromPort } : {}),
    ...(e.toPort ? { toPort: e.toPort } : {}),
  };
}

// Verbatim from WorkflowEditor — `a` is a canvas node from orig; accesses
// a.type / a.position which are absent on canvas nodes (always undefined),
// matching the original behaviour exactly.
//
// 原始 nodeChanged 逐字保留；orig 侧 canvas 节点无 .type/.position，与原文一致。
function nodeChanged(a: CanvasNode & { type?: string; position?: { x: number; y: number } }, b: CanvasNode): boolean {
  if (a.type !== b.kind) return true;
  if ((a.notes || "") !== (b.notes || "")) return true;
  if ((a.position?.x ?? a.x) !== b.x) return true;
  if ((a.position?.y ?? a.y) !== b.y) return true;
  if ((a.timeout || 0) !== (b.timeout || 0)) return true;
  if ((a.onError || "") !== (b.onError || "")) return true;
  const ac = JSON.stringify(a.config || {});
  const bc = JSON.stringify(b.config || {});
  return ac !== bc;
}

// Three-way diff: compare orig vs next canvas graph, produce backend Op[].
// Verbatim from WorkflowEditor.diffToOps — adds/updates/deletes nodes and edges.
// Plus a dedicated set_node_model_override op when an existing node's
// modelOverride flipped (backend op shape differs from update_node patch).
//
// 三向 diff：orig vs next → add/update/delete ops 逐字保留;
// 现存节点 modelOverride 变化 → 专属 op,让后端校验 apiKeyId 归属。
export function diffToOps(orig: CanvasGraph, next: CanvasGraph): WorkflowEditOp[] {
  const ops: WorkflowEditOp[] = [];
  const oN = new Map((orig.nodes || []).map((n) => [n.id, n]));
  const nN = new Map(next.nodes.map((n) => [n.id, n]));
  // Adds + updates
  for (const n of next.nodes) {
    const o = oN.get(n.id);
    if (!o) {
      ops.push({ op: "add_node", node: nodeToSpec(n) });
    } else if (nodeChanged(o, n)) {
      ops.push({ op: "update_node", nodeId: n.id, patch: nodeToPatch(n) });
    }
    // modelOverride flip on existing node → dedicated op.
    if (o && !modelOverrideEq(o.modelOverride, n.modelOverride)) {
      ops.push({
        op: "set_node_model_override",
        nodeId: n.id,
        modelOverride: n.modelOverride ?? null,
      });
    }
  }
  // Removes
  for (const o of orig.nodes || []) {
    if (!nN.has(o.id)) ops.push({ op: "delete_node", id: o.id });
  }
  // Edges
  const oE = new Map((orig.edges || []).map((e) => [edgeKey(e), e]));
  const nE = new Map(next.edges.map((e) => [edgeKey(e), e]));
  for (const e of next.edges) {
    if (!oE.has(edgeKey(e))) ops.push({ op: "add_edge", edge: edgeToSpec(e) });
  }
  for (const e of orig.edges || []) {
    if (!nE.has(edgeKey(e))) ops.push({ op: "delete_edge", id: e.id || edgeKey(e) });
  }
  return ops;
}

// ── Hook ─────────────────────────────────────────────────────────────────

export function useWorkflowEdit(workflowId: string, original: CanvasGraph) {
  const edit = useEditWorkflow(workflowId);

  const [dirty, setDirty] = useState(false);
  const [savedAt, setSavedAt] = useState<Date | null>(null);
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Cleanup timer on unmount.
  useEffect(() => () => { if (saveTimer.current) clearTimeout(saveTimer.current); }, []);

  // 2s debounced autosave: mark dirty → clear prev timer → diff → POST :edit.
  // Verbatim timer + diff logic from WorkflowEditor.markDirty.
  //
  // 2s 防抖 autosave：dirty → clearTimeout → diffToOps → mutate；逐字保留。
  const markDirty = useCallback(
    (graph: CanvasGraph) => {
      setDirty(true);
      if (saveTimer.current) clearTimeout(saveTimer.current);
      saveTimer.current = setTimeout(() => {
        const ops = diffToOps(original, graph);
        if (ops.length === 0) { setDirty(false); return; }
        edit.mutate(
          { ops, changeReason: "editor autosave" },
          {
            onSuccess: () => { setDirty(false); setSavedAt(new Date()); },
            // Save errors handled by global MutationCache onError via errorMap.
          },
        );
      }, 2000);
    },
    [original, edit],
  );

  // Called when the version changes externally so the dirty indicator
  // doesn't persist after the canvas is reset to the new version's graph.
  const resetDirty = useCallback(() => {
    if (saveTimer.current) clearTimeout(saveTimer.current);
    setDirty(false);
  }, []);

  return {
    markDirty,
    resetDirty,
    dirty,
    savedAt,
    isSaving: edit.isPending,
  };
}

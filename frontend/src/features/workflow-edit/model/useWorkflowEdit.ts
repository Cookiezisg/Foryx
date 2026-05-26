// Orchestrates diff-based autosave for the workflow editor canvas.
// 2s debounced timer + diffToOps → POST :edit. Verbatim from WorkflowEditor.
//
// 封装 WorkflowEditor 的 diff + 2s 防抖 autosave 编排；算法逐字保留。

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useEditWorkflow } from "@entities/workflow";
import type { WorkflowEditOp } from "@entities/workflow";
// TODO(阶段4): ui store 拆进 app/model 后,将此 import 替换为正式 FSD 路径。
// eslint-disable-next-line boundaries/dependencies
import { useUIStore } from "../../../store/ui.js";

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
  };
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
//
// 三向 diff：orig vs next → add/update/delete ops 逐字保留。
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
      ops.push({ op: "update_node", node: nodeToSpec(n) });
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
  const { t } = useTranslation("forge");
  const edit = useEditWorkflow(workflowId);
  const pushToast = useUIStore((s: any) => s.pushToast);

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
            onError: (e: Error) =>
              pushToast({ kind: "error", title: t("detail.saveFail"), desc: e.message }),
          },
        );
      }, 2000);
    },
    [original, edit, pushToast, t],
  );

  return {
    markDirty,
    dirty,
    savedAt,
    isSaving: edit.isPending,
  };
}

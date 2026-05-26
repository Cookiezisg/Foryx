// WorkflowDetail — read-only DAG canvas + VersionRail. Full editor
// (pan/zoom/drag/connect) lands in Phase 11 polish.
//
// WorkflowDetail —— 只读 DAG 画布 + VersionRail；编辑画布 Phase 11 落地。

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Icon } from "@shared/ui/Icon";
import { Button } from "@shared/ui/Button";
import { KindChip } from "@shared/ui/KindChip.tsx";
import { StatusBadge } from "@shared/ui/StatusBadge.tsx";
import { EntityRelMeta } from "@/widgets/entity-rel-meta/EntityRelMeta.tsx";
import { VersionRail } from "@/widgets/version-rail/VersionRail.tsx";
import { AskAiTrigger } from "@/widgets/ask-ai-trigger/AskAiTrigger.tsx";
import { RunDrawer } from "./RunDrawer.tsx";
import { CapabilityCheckPanel } from "./CapabilityCheckPanel.tsx";
import { WorkflowEditor } from "@features/workflow-edit";
import { useWorkflow, useWorkflowVersions } from "@entities/workflow";
import { useForgeProgress } from "@shared/model";
import { useToastStore } from "@shared/ui/toastStore";
import { useForgeReview } from "@features/forge-review";

const NODE_W = 184;
const NODE_H = 76;

interface WorkflowDetailProps {
  forge: any;
  onBack: () => void;
  onOpenExecute?: (id: string) => void;
}

export function WorkflowDetail({ forge, onBack, onOpenExecute }: WorkflowDetailProps) {
  const { t } = useTranslation(["forge", "common"]);
  const { data: wf = forge } = useWorkflow(forge.id);
  const { data: versionsRaw = [] } = useWorkflowVersions(forge.id);
  const versions = versionsRaw as any[];
  const pushToast = useToastStore((s) => s.pushToast);
  const { accept: onAccept, reject: onReject } = useForgeReview("workflow", forge.id);
  const progress = useForgeProgress((s) => s.active[`workflow:${forge.id}`]);

  const currentV = versions.find((v) => v.state === "current") || versions[0];
  const pendingV = versions.find((v) => v.state === "pending");
  const deployedV = versions.find((v) => v.state === "deployed");

  const [selectedId, setSelectedId] = useState(null);
  const [runOpen, setRunOpen] = useState(false);
  const effectiveSelected = selectedId || pendingV?.id || currentV?.id;
  const selectedV = versions.find((v) => v.id === effectiveSelected) || currentV;

  return (
    <div className="page">
      <div className="page-header" style={{ paddingTop: 18 }}>
        <div className="page-header-text" style={{ gap: 6 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 12, color: "var(--fg-muted)" }}>
            <Button size="xs" variant="ghost" onClick={onBack}>← {t("common:back")}</Button>
            <KindChip kind="workflow" />
            <span className="cell-mono" style={{ color: "var(--fg-faint)" }}>{forge.id}</span>
            {progress && progress.status === "running" && (
              <span className="badge streaming"><span className="dot" />{t("detail.forging")}</span>
            )}
          </div>
          <div className="page-title" style={{ fontFamily: "var(--font-mono)" }}>{wf.name}</div>
          <div className="page-subtitle" style={{ display: "flex", alignItems: "center", gap: 4, flexWrap: "wrap" }}>
            <span>{wf.desc || wf.description || ""}</span>
            <EntityRelMeta entityId={wf.id} kind="workflow" />
          </div>
        </div>
        <div className="page-actions">
          <CapabilityCheckPanel workflowId={wf.id} />
          <Button size="sm" onClick={() => setRunOpen(true)}><Icon.Play /> {t("workflow.triggerBtn")}</Button>
          {pendingV && (
            <>
              <Button size="sm" variant="danger" onClick={onReject}>
                <Icon.X /> {t("detail.revert")}
              </Button>
              <Button size="sm" variant="accent" onClick={onAccept}>
                <Icon.Check /> {t("detail.accept")}
              </Button>
            </>
          )}
          <AskAiTrigger
            kind="workflow"
            entityId={wf.id}
            context={`Workflow · ${wf.name}`}
            suggestions={t("workflow.aiSuggestions", { returnObjects: true }) as any}
          />
        </div>
      </div>

      <div className="vr-shell">
        <div className="vr-main" style={{ padding: 0, position: "relative" }}>
          {effectiveSelected === currentV?.id
            ? <WorkflowEditor workflowId={wf.id} version={selectedV} />
            : <DagCanvas version={selectedV} />}
        </div>
        <VersionRail
          versions={versions}
          currentId={currentV?.id}
          pendingId={pendingV?.id}
          deployedId={deployedV?.id}
          showDeploy
          selectedId={effectiveSelected}
          onSelect={setSelectedId}
          onAccept={onAccept}
          onRevert={onReject}
          onDeploy={() => pushToast({ kind: "success", title: t("detail.deployRequested") })}
        />
      </div>
      <RunDrawer open={runOpen} onClose={() => setRunOpen(false)} kind="workflow" entity={wf} onOpenExecute={onOpenExecute} />
    </div>
  );
}

// DagCanvas — auto-layout nodes from a version's graph + render cubic
// bezier edges. Pan via mouse drag, zoom via wheel. Read-only.
function DagCanvas({ version }: { version: any }) {
  const { t } = useTranslation("forge");
  const graph = useMemo(() => normaliseGraph(version), [version]);
  const [transform, setTransform] = useState({ x: 0, y: 0, scale: 1 });
  const [panning, setPanning] = useState(false);
  const panStart = useState({ current: null })[0];

  if (!graph) {
    return <div className="empty" style={{ padding: 40 }}><div className="sub">{t("workflow.noGraph")}</div></div>;
  }

  const { nodes, edges } = graph;
  const byId: Record<string, any> = Object.fromEntries(nodes.map((n: any) => [n.id, n]));

  const onMouseDown = (e: React.MouseEvent) => {
    if (e.button !== 0) return;
    if (!(e.target as HTMLElement).classList.contains("wf-canvas-inner") && !(e.target as HTMLElement).classList.contains("wf-canvas")) return;
    panStart.current = { x: e.clientX, y: e.clientY, tx: transform.x, ty: transform.y };
    setPanning(true);
  };
  const onMouseMove = (e: React.MouseEvent) => {
    if (!panning || !panStart.current) return;
    const dx = e.clientX - panStart.current.x;
    const dy = e.clientY - panStart.current.y;
    setTransform((t) => ({ ...t, x: (panStart.current as any).tx + dx, y: (panStart.current as any).ty + dy }));
  };
  const onMouseUp = () => { setPanning(false); panStart.current = null; };
  const onWheel = (e: React.WheelEvent) => {
    e.preventDefault();
    setTransform((t) => {
      const scale = Math.max(0.25, Math.min(2.5, t.scale * (1 - e.deltaY * 0.0015)));
      return { ...t, scale };
    });
  };

  return (
    <div
      className={"wf-canvas" + (panning ? " is-panning" : "")}
      onMouseDown={onMouseDown}
      onMouseMove={onMouseMove}
      onMouseUp={onMouseUp}
      onMouseLeave={onMouseUp}
      onWheel={onWheel}
      style={{ position: "absolute", inset: 0, overflow: "hidden", cursor: panning ? "grabbing" : "grab" }}
    >
      <div
        className="wf-canvas-inner"
        style={{ transform: `translate(${transform.x}px, ${transform.y}px) scale(${transform.scale})`, transformOrigin: "0 0", position: "absolute", inset: 0 }}
      >
        <svg className="wf-edges" style={{ overflow: "visible", position: "absolute", inset: 0, pointerEvents: "none" }}>
          <defs>
            <marker id="wf-arr-ro" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
              <path d="M0 0 L10 5 L0 10 z" fill="var(--border-strong)" />
            </marker>
          </defs>
          {edges.map((e: any, i: number) => {
            const a = byId[e.from], b = byId[e.to];
            if (!a || !b) return null;
            return <path key={i} d={edgePath(a, b)} fill="none" stroke="var(--border-strong)" strokeWidth={1.4} markerEnd="url(#wf-arr-ro)" />;
          })}
        </svg>
        {nodes.map((n: any) => (
          <div
            key={n.id}
            className="wf-node"
            style={{ left: n.x, top: n.y, width: NODE_W, position: "absolute" }}
          >
            <div className="wf-node-head">
              <div className={"wf-node-icon kind-" + n.kind}>{iconFor(n.kind)}</div>
              <div className="wf-node-title">{n.label || n.id}</div>
            </div>
            <div className="wf-node-sub">{n.sub || n.ref || ""}</div>
          </div>
        ))}
      </div>
      <div className="wf-canvas-toolbar">
        <button className="icon-btn" title={t("workflow.canvas.zoomIn")} onClick={() => setTransform((t) => ({ ...t, scale: Math.min(2.5, t.scale * 1.2) }))}><Icon.Plus /></button>
        <button className="icon-btn" title={t("workflow.canvas.zoomOut")} onClick={() => setTransform((t) => ({ ...t, scale: Math.max(0.25, t.scale / 1.2) }))}>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round"><path d="M5 12h14"/></svg>
        </button>
        <button className="icon-btn" title={t("workflow.canvas.reset")} onClick={() => setTransform({ x: 0, y: 0, scale: 1 })}><Icon.Refresh /></button>
        <div className="wf-zoom">{Math.round(transform.scale * 100)}%</div>
      </div>
    </div>
  );
}

function iconFor(kind: string) {
  const I = ({
    trigger:   Icon.Zap,
    function:  Icon.Code,
    handler:   Icon.Server,
    mcp:       Icon.Server,
    skill:     Icon.Sparkles,
    llm:       Icon.Brain,
    agent:     Icon.Bot,
    http:      Icon.Globe,
    condition: Icon.GitBranch,
    loop:      Icon.Refresh,
    parallel:  Icon.Layers,
    approval:  Icon.Pause,
    wait:      Icon.Clock,
    variable:  Icon.Database,
  } as Record<string, React.ComponentType<any>>)[kind] || Icon.Code;
  return <I />;
}

function edgePath(a: any, b: any) {
  const sx = a.x + NODE_W / 2, sy = a.y + NODE_H;
  const ex = b.x + NODE_W / 2, ey = b.y;
  const dy = Math.max(30, (ey - sy) / 2);
  return `M ${sx} ${sy} C ${sx} ${sy + dy}, ${ex} ${ey - dy}, ${ex} ${ey}`;
}

// normaliseGraph — pulls nodes/edges out of various server shapes and
// runs a topological auto-layout if positions aren't present.
function normaliseGraph(version: any) {
  if (!version) return null;
  const g = version.graph || version;
  const nodes = (g.nodes || []).map((n: any) => ({ ...n, x: n.x, y: n.y }));
  const edges = (g.edges || []).map((e: any) => ({ from: e.from || e.fromId, to: e.to || e.toId }));
  if (!nodes.length) return null;

  if (nodes.every((n: any) => typeof n.x === "number" && typeof n.y === "number")) {
    return { nodes, edges };
  }
  // Kahn-style BFS topological layering for auto-layout.
  const incoming: Record<string, number> = Object.fromEntries(nodes.map((n: any) => [n.id, 0]));
  edges.forEach((e: any) => { if (incoming[e.to] != null) incoming[e.to]++; });
  const layer: Record<string, number> = {};
  const queue: string[] = nodes.filter((n: any) => incoming[n.id] === 0).map((n: any) => n.id);
  queue.forEach((id: string) => { layer[id] = 0; });
  let head = 0;
  while (head < queue.length) {
    const id = queue[head++];
    edges.filter((e: any) => e.from === id).forEach((e: any) => {
      const newL = (layer[id] || 0) + 1;
      if (layer[e.to] == null || layer[e.to] < newL) layer[e.to] = newL;
      if (!queue.includes(e.to)) queue.push(e.to);
    });
  }
  const byLayer: Record<number, string[]> = {};
  nodes.forEach((n: any) => {
    const L = layer[n.id] ?? 0;
    if (!byLayer[L]) byLayer[L] = [];
    byLayer[L].push(n.id);
  });
  const xGap = 240, yGap = 140;
  Object.keys(byLayer).map(Number).sort((a: number, b: number) => a - b).forEach((L) => {
    byLayer[L].forEach((id: string, i: number) => {
      const offset = (i - (byLayer[L].length - 1) / 2) * yGap;
      const node = nodes.find((n: any) => n.id === id);
      node.x = 200 + offset;
      node.y = 60 + L * yGap;
    });
  });
  return { nodes, edges };
}

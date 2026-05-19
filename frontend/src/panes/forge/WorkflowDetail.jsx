// WorkflowDetail — read-only DAG canvas + VersionRail. Full editor
// (pan/zoom/drag/connect) lands in Phase 11 polish.
//
// WorkflowDetail —— 只读 DAG 画布 + VersionRail；编辑画布 Phase 11 落地。

import { useMemo, useState } from "react";
import { Icon } from "../../components/primitives/Icon.jsx";
import { Button } from "../../components/primitives/Button.jsx";
import { KindChip } from "../../components/shared/KindChip.jsx";
import { StatusBadge } from "../../components/shared/StatusBadge.jsx";
import { EntityRelMeta } from "../../components/shared/EntityRelMeta.jsx";
import { VersionRail } from "../../components/shared/VersionRail.jsx";
import { AskAiTrigger } from "../../components/shared/AskAiTrigger.jsx";
import { useWorkflow, useWorkflowVersions, useAcceptWorkflow } from "../../api/forge.js";
import { useForgeProgress } from "../../sse/useForge.js";
import { useUIStore } from "../../store/ui.js";

const NODE_W = 184;
const NODE_H = 76;

export function WorkflowDetail({ forge, onBack }) {
  const { data: wf = forge } = useWorkflow(forge.id);
  const { data: versions = [] } = useWorkflowVersions(forge.id);
  const pushToast = useUIStore((s) => s.pushToast);
  const accept = useAcceptWorkflow();
  const progress = useForgeProgress((s) => s.active[`workflow:${forge.id}`]);

  const currentV = versions.find((v) => v.state === "current") || versions[0];
  const pendingV = versions.find((v) => v.state === "pending");
  const deployedV = versions.find((v) => v.state === "deployed");

  const [selectedId, setSelectedId] = useState(null);
  const effectiveSelected = selectedId || pendingV?.id || currentV?.id;
  const selectedV = versions.find((v) => v.id === effectiveSelected) || currentV;

  return (
    <div className="page">
      <div className="page-header" style={{ paddingTop: 18 }}>
        <div className="page-header-text" style={{ gap: 6 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 12, color: "var(--fg-muted)" }}>
            <Button size="xs" variant="ghost" onClick={onBack}>← 返回</Button>
            <KindChip kind="workflow" />
            <span className="cell-mono" style={{ color: "var(--fg-faint)" }}>{forge.id}</span>
            {progress && progress.status === "running" && (
              <span className="badge streaming"><span className="dot" />锻造中</span>
            )}
          </div>
          <div className="page-title" style={{ fontFamily: "var(--font-mono)" }}>{wf.name}</div>
          <div className="page-subtitle" style={{ display: "flex", alignItems: "center", gap: 4, flexWrap: "wrap" }}>
            <span>{wf.desc || wf.description || ""}</span>
            <EntityRelMeta entityId={wf.id} />
          </div>
        </div>
        <div className="page-actions">
          <Button size="sm"><Icon.Eye /> Capability check</Button>
          <Button size="sm"><Icon.Play /> 试跑</Button>
          {pendingV && (
            <Button size="sm" variant="accent" onClick={() => accept.mutate(forge.id, {
              onSuccess: () => pushToast({ kind: "success", title: "Accepted" }),
            })}>
              <Icon.Check /> Accept
            </Button>
          )}
          <AskAiTrigger
            kind="workflow"
            entityId={wf.id}
            context={`Workflow · ${wf.name}`}
            suggestions={["在写入前加重试", "改成每天早上 8 点触发"]}
          />
        </div>
      </div>

      <div className="vr-shell">
        <div className="vr-main" style={{ padding: 0, position: "relative" }}>
          <DagCanvas version={selectedV} />
        </div>
        <VersionRail
          versions={versions}
          currentId={currentV?.id}
          pendingId={pendingV?.id}
          deployedId={deployedV?.id}
          showDeploy
          selectedId={effectiveSelected}
          onSelect={setSelectedId}
          onAccept={() => accept.mutate(forge.id)}
          onRevert={() => {}}
          onDeploy={() => pushToast({ kind: "success", title: "Deploy 已请求" })}
        />
      </div>
    </div>
  );
}

// DagCanvas — auto-layout nodes from a version's graph + render cubic
// bezier edges. Pan via mouse drag, zoom via wheel. Read-only.
function DagCanvas({ version }) {
  const graph = useMemo(() => normaliseGraph(version), [version]);
  const [transform, setTransform] = useState({ x: 0, y: 0, scale: 1 });
  const [panning, setPanning] = useState(false);
  const panStart = useState({ current: null })[0];

  if (!graph) {
    return <div className="empty" style={{ padding: 40 }}><div className="sub">该版本没有 graph 数据</div></div>;
  }

  const { nodes, edges } = graph;
  const byId = Object.fromEntries(nodes.map((n) => [n.id, n]));

  const onMouseDown = (e) => {
    if (e.button !== 0) return;
    if (!e.target.classList.contains("wf-canvas-inner") && !e.target.classList.contains("wf-canvas")) return;
    panStart.current = { x: e.clientX, y: e.clientY, tx: transform.x, ty: transform.y };
    setPanning(true);
  };
  const onMouseMove = (e) => {
    if (!panning || !panStart.current) return;
    const dx = e.clientX - panStart.current.x;
    const dy = e.clientY - panStart.current.y;
    setTransform((t) => ({ ...t, x: panStart.current.tx + dx, y: panStart.current.ty + dy }));
  };
  const onMouseUp = () => { setPanning(false); panStart.current = null; };
  const onWheel = (e) => {
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
          {edges.map((e, i) => {
            const a = byId[e.from], b = byId[e.to];
            if (!a || !b) return null;
            return <path key={i} d={edgePath(a, b)} fill="none" stroke="var(--border-strong)" strokeWidth={1.4} markerEnd="url(#wf-arr-ro)" />;
          })}
        </svg>
        {nodes.map((n) => (
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
        <button className="icon-btn" title="放大" onClick={() => setTransform((t) => ({ ...t, scale: Math.min(2.5, t.scale * 1.2) }))}><Icon.Plus /></button>
        <button className="icon-btn" title="缩小" onClick={() => setTransform((t) => ({ ...t, scale: Math.max(0.25, t.scale / 1.2) }))}>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round"><path d="M5 12h14"/></svg>
        </button>
        <button className="icon-btn" title="复位" onClick={() => setTransform({ x: 0, y: 0, scale: 1 })}><Icon.Refresh /></button>
        <div className="wf-zoom">{Math.round(transform.scale * 100)}%</div>
      </div>
    </div>
  );
}

function iconFor(kind) {
  const I = {
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
  }[kind] || Icon.Code;
  return <I />;
}

function edgePath(a, b) {
  const sx = a.x + NODE_W / 2, sy = a.y + NODE_H;
  const ex = b.x + NODE_W / 2, ey = b.y;
  const dy = Math.max(30, (ey - sy) / 2);
  return `M ${sx} ${sy} C ${sx} ${sy + dy}, ${ex} ${ey - dy}, ${ex} ${ey}`;
}

// normaliseGraph — pulls nodes/edges out of various server shapes and
// runs a topological auto-layout if positions aren't present.
function normaliseGraph(version) {
  if (!version) return null;
  const g = version.graph || version;
  const nodes = (g.nodes || []).map((n) => ({ ...n, x: n.x, y: n.y }));
  const edges = (g.edges || []).map((e) => ({ from: e.from || e.fromId, to: e.to || e.toId }));
  if (!nodes.length) return null;

  if (nodes.every((n) => typeof n.x === "number" && typeof n.y === "number")) {
    return { nodes, edges };
  }
  // Kahn-style BFS topological layering for auto-layout.
  const incoming = Object.fromEntries(nodes.map((n) => [n.id, 0]));
  edges.forEach((e) => { if (incoming[e.to] != null) incoming[e.to]++; });
  const layer = {};
  const queue = nodes.filter((n) => incoming[n.id] === 0).map((n) => n.id);
  queue.forEach((id) => { layer[id] = 0; });
  let head = 0;
  while (head < queue.length) {
    const id = queue[head++];
    edges.filter((e) => e.from === id).forEach((e) => {
      const newL = (layer[id] || 0) + 1;
      if (layer[e.to] == null || layer[e.to] < newL) layer[e.to] = newL;
      if (!queue.includes(e.to)) queue.push(e.to);
    });
  }
  const byLayer = {};
  nodes.forEach((n) => {
    const L = layer[n.id] ?? 0;
    if (!byLayer[L]) byLayer[L] = [];
    byLayer[L].push(n.id);
  });
  const xGap = 240, yGap = 140;
  Object.keys(byLayer).map(Number).sort((a, b) => a - b).forEach((L) => {
    byLayer[L].forEach((id, i) => {
      const offset = (i - (byLayer[L].length - 1) / 2) * yGap;
      const node = nodes.find((n) => n.id === id);
      node.x = 200 + offset;
      node.y = 60 + L * yGap;
    });
  });
  return { nodes, edges };
}

// RelGraph — force-directed entity relationship graph (Obsidian-style).
//
//   <RelGraph />                          full view (Observe pane)
//   <RelGraphPopover entityId kind />     mini focused graph (RelMore "…" trigger)
//
// RelGraph —— 力导向实体引用图。
//   - 节点：function/handler/workflow/document/skill/mcp/conversation/memory
//   - 颜色编码 by kind；degree → 节点半径
//   - 拖节点、滚轮缩放、画布平移
//   - 右侧 detail 面板列出入/出引用，点开跳转

import React, { useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { Icon } from "@shared/ui/Icon";
import { Button } from "@shared/ui/Button";
import { FloatingInspector } from "../../shared/ui/FloatingInspector.tsx";
import { useNeighborhood } from "@entities/relation";
import { navigate } from "@shared/lib/navigation";
import { useEntityDirectory, normEdges, guessKind } from "@features/entity-link";

const KIND_COLOR: Record<string, string> = {
  function: "#2383E2", handler: "#0F7B6C", workflow: "#D97757",
  skill: "#B25E10",    mcp: "#6940A5",     memory: "#9A4A6F",
  conversation: "#3D5A80", document: "#5E6470", flowrun: "#888888",
};
const KIND_ICON: Record<string, string> = {
  function: "Code", handler: "Server", workflow: "Workflow", skill: "Sparkles",
  mcp: "Server", memory: "Brain", conversation: "MessageSquare", document: "FileText",
  flowrun: "Play",
};
const KIND_LABEL_BASE = {
  function: "Function", handler: "Handler", workflow: "Workflow", skill: "Skill",
  mcp: "MCP", memory: "Memory", flowrun: "FlowRun",
};

// 8 closed relation kinds (see backend domain/relation/relation.go). Short
// human labels — direction implied by the → / ← section header.
// These keys map to misc.relGraph.relLabels.* for i18n.
const REL_LABEL_KEYS: Record<string, string> = {
  workflow_uses_function:     "workflow_uses_function",
  workflow_uses_handler:      "workflow_uses_handler",
  workflow_uses_mcp:          "workflow_uses_mcp",
  workflow_uses_skill:        "workflow_uses_skill",
  workflow_uses_document:     "workflow_uses_document",
  conversation_forged_entity: "conversation_forged_entity",
  conversation_edited_entity: "conversation_edited_entity",
  document_links_entity:      "document_links_entity",
};

// ── Force-directed canvas ────────────────────────────────────────────────
function GraphCanvas({ nodes, edges, focusId, selected, onSelect, width, height }: {
  nodes: any[]; edges: any[]; focusId?: string; selected: string | null;
  onSelect: (id: string) => void; width: number; height: number;
}) {
  const positions = useRef<Record<string, { x: number; y: number }>>({});
  const velocities = useRef<Record<string, { vx: number; vy: number }>>({});
  const dragRef = useRef<any>(null);
  const panRef = useRef<any>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [transform, setTransform] = useState({ x: 0, y: 0, scale: 1 });
  const [hover, setHover] = useState(null);
  const [, rerender] = useState(0);

  const degree = useMemo(() => {
    const d = Object.fromEntries(nodes.map((n) => [n.id, 0]));
    edges.forEach((e) => { if (d[e.from] != null) d[e.from]++; if (d[e.to] != null) d[e.to]++; });
    return d;
  }, [nodes, edges]);

  // Init positions on node-set change.
  useEffect(() => {
    const next: Record<string, { x: number; y: number }> = {}, vels: Record<string, { vx: number; vy: number }> = {};
    nodes.forEach((n, i) => {
      const prev = positions.current[n.id];
      const angle = (i / Math.max(1, nodes.length)) * Math.PI * 2;
      const r = Math.min(width, height) * 0.32;
      next[n.id] = prev || { x: width / 2 + r * Math.cos(angle), y: height / 2 + r * Math.sin(angle) };
      vels[n.id] = { vx: 0, vy: 0 };
    });
    positions.current = next; velocities.current = vels;
    rerender((x) => x + 1);
  }, [nodes.length, edges.length, width, height]);

  // Continuous tick.
  useEffect(() => {
    let raf: number;
    const tick = () => {
      const N = nodes.length;
      const center = { x: width / 2, y: height / 2 };
      const repulseK = 2200, springK = 0.04, springLen = 110, damping = 0.82, centerK = 0.002;
      for (let i = 0; i < N; i++) {
        const a = nodes[i], pa = positions.current[a.id];
        if (!pa) continue;
        for (let j = i + 1; j < N; j++) {
          const b = nodes[j], pb = positions.current[b.id];
          if (!pb) continue;
          const dx = pa.x - pb.x, dy = pa.y - pb.y;
          const d2 = dx * dx + dy * dy + 0.01, d = Math.sqrt(d2);
          const f = repulseK / d2, fx = (dx / d) * f, fy = (dy / d) * f;
          velocities.current[a.id].vx += fx; velocities.current[a.id].vy += fy;
          velocities.current[b.id].vx -= fx; velocities.current[b.id].vy -= fy;
        }
      }
      edges.forEach((e) => {
        const pa = positions.current[e.from], pb = positions.current[e.to];
        if (!pa || !pb) return;
        const dx = pb.x - pa.x, dy = pb.y - pa.y;
        const d = Math.sqrt(dx * dx + dy * dy) || 1;
        const f = springK * (d - springLen);
        const fx = (dx / d) * f, fy = (dy / d) * f;
        velocities.current[e.from].vx += fx; velocities.current[e.from].vy += fy;
        velocities.current[e.to].vx -= fx; velocities.current[e.to].vy -= fy;
      });
      nodes.forEach((n) => {
        const p = positions.current[n.id];
        if (!p) return;
        velocities.current[n.id].vx += (center.x - p.x) * centerK;
        velocities.current[n.id].vy += (center.y - p.y) * centerK;
      });
      nodes.forEach((n) => {
        const p = positions.current[n.id], v = velocities.current[n.id];
        if (!p || !v) return;
        if (dragRef.current?.id === n.id) { v.vx = 0; v.vy = 0; return; }
        v.vx *= damping; v.vy *= damping; p.x += v.vx; p.y += v.vy;
      });
      rerender((x) => (x + 1) % 1e9);
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [nodes, edges, width, height]);

  const clientToCanvas = (cx: number, cy: number) => {
    const r = containerRef.current.getBoundingClientRect();
    return { x: (cx - r.left - transform.x) / transform.scale, y: (cy - r.top - transform.y) / transform.scale };
  };

  const onSvgMouseDown = (e: React.MouseEvent<SVGSVGElement>) => {
    if (e.button !== 0) return;
    if (e.target === e.currentTarget || (e.target as SVGElement).tagName === "rect" || (e.target as SVGElement).tagName === "svg") {
      panRef.current = { sx: e.clientX, sy: e.clientY, tx: transform.x, ty: transform.y };
    }
  };
  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      if (dragRef.current) {
        const c = clientToCanvas(e.clientX, e.clientY);
        const p = positions.current[dragRef.current.id];
        if (p) {
          const nx = c.x - dragRef.current.dx, ny = c.y - dragRef.current.dy;
          dragRef.current.lastDx = nx - p.x; dragRef.current.lastDy = ny - p.y;
          p.x = nx; p.y = ny;
        }
      } else if (panRef.current) {
        setTransform((t) => ({
          ...t,
          x: panRef.current.tx + (e.clientX - panRef.current.sx),
          y: panRef.current.ty + (e.clientY - panRef.current.sy),
        }));
      }
    };
    const onUp = () => {
      if (dragRef.current) {
        const v = velocities.current[dragRef.current.id];
        if (v && dragRef.current.lastDx != null) {
          v.vx = dragRef.current.lastDx * 2; v.vy = dragRef.current.lastDy * 2;
        }
        dragRef.current = null;
      }
      panRef.current = null;
    };
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
    return () => {
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
    };
  }, [transform]);

  const onNodeMouseDown = (e: React.MouseEvent, n: any) => {
    e.stopPropagation();
    const c = clientToCanvas(e.clientX, e.clientY);
    const p = positions.current[n.id];
    dragRef.current = { id: n.id, dx: c.x - p.x, dy: c.y - p.y };
    onSelect(n.id);
  };
  const onWheel = (e: React.WheelEvent) => {
    e.preventDefault();
    const r = containerRef.current!.getBoundingClientRect();
    const mx = e.clientX - r.left, my = e.clientY - r.top;
    const delta = -e.deltaY * 0.0015;
    setTransform((t) => {
      const scale = Math.max(0.25, Math.min(3, t.scale * (1 + delta)));
      const ratio = scale / t.scale;
      return { x: mx - (mx - t.x) * ratio, y: my - (my - t.y) * ratio, scale };
    });
  };

  const isPan = panRef.current != null;
  return (
    <div ref={containerRef} className={"rg-container" + (isPan ? " is-panning" : "")} onWheel={onWheel}
         style={{ width: "100%", height: "100%", position: "relative", overflow: "hidden" }}>
      <svg width={width} height={height} className="rg-svg" onMouseDown={onSvgMouseDown}
           style={{
             transform: `translate(${transform.x}px, ${transform.y}px) scale(${transform.scale})`,
             transformOrigin: "0 0",
             cursor: isPan ? "grabbing" : "default",
           }}>
        <rect x="-5000" y="-5000" width="10000" height="10000" fill="transparent" />
        {edges.map((e, i) => {
          const pa = positions.current[e.from], pb = positions.current[e.to];
          if (!pa || !pb) return null;
          const active = selected === e.from || selected === e.to || hover === e.from || hover === e.to;
          const fade = (selected || hover) && !active;
          return (
            <line key={i}
              x1={pa.x} y1={pa.y} x2={pb.x} y2={pb.y}
              stroke={active ? "var(--accent)" : "var(--border-strong)"}
              strokeWidth={active ? 1.4 : 0.8}
              opacity={fade ? 0.10 : (active ? 0.9 : 0.35)} />
          );
        })}
        {nodes.map((n) => {
          const p = positions.current[n.id];
          if (!p) return null;
          const r = focusId === n.id ? 8 : 3 + Math.min(5, (degree[n.id] || 0) * 0.6);
          const isSel = selected === n.id, isHov = hover === n.id, isFocus = focusId === n.id;
          const fade = (selected || hover) && !isSel && !isHov && !isFocus;
          return (
            <g key={n.id} transform={`translate(${p.x},${p.y})`} className="rg-node"
               onMouseDown={(e) => onNodeMouseDown(e, n)}
               onClick={(e) => { e.stopPropagation(); onSelect(n.id); }}
               onMouseEnter={() => setHover(n.id)}
               onMouseLeave={() => setHover(null)}>
              {(isSel || isFocus) && (
                <circle r={r + 5} fill="none" stroke="var(--accent)" strokeWidth="1.2" opacity="0.55" />
              )}
              <circle r={r}
                fill={KIND_COLOR[n.kind] || "#999"}
                stroke="var(--bg-paper)" strokeWidth={isHov ? 2 : 1}
                opacity={fade ? 0.20 : 1}
                style={{ cursor: dragRef.current?.id === n.id ? "grabbing" : "grab" }} />
            </g>
          );
        })}
        {(hover || focusId) && (() => {
          const id = hover || focusId;
          const n = nodes.find((x) => x.id === id);
          const p = positions.current[id];
          if (!n || !p) return null;
          const text = n.label.length > 28 ? n.label.slice(0, 28) + "…" : n.label;
          return (
            <text x={p.x} y={p.y + 20} textAnchor="middle" fontSize="11"
              fontFamily="var(--font-sans)" fontWeight="500" fill="var(--fg-strong)"
              pointerEvents="none"
              style={{ paintOrder: "stroke", stroke: "var(--bg-paper)", strokeWidth: 4, strokeLinejoin: "round" }}>
              {text}
            </text>
          );
        })()}
      </svg>
    </div>
  );
}

// ── Detail panel ─────────────────────────────────────────────────────────
function adjacency(entityId: string, allEdges: any[], allNodes: any[]) {
  const incoming: any[] = [], outgoing: any[] = [];
  allEdges.forEach((e: any) => {
    if (e.from === entityId) {
      const t = allNodes.find((n: any) => n.id === e.to);
      if (t) outgoing.push({ edge: e, node: t });
    }
    if (e.to === entityId) {
      const s = allNodes.find((n: any) => n.id === e.from);
      if (s) incoming.push({ edge: e, node: s });
    }
  });
  return { incoming, outgoing };
}

function NodeDetail({ node, allNodes, allEdges, onSelect }: { node: any; allNodes: any[]; allEdges: any[]; onSelect: (id: string) => void }) {
  const { t } = useTranslation("misc");

  // Rendered inside a FloatingInspector — drop our own outer container.
  // FloatingInspector head already shows kind label; we keep icon+name+id+open here.
  const Ic = (Icon as Record<string, React.ComponentType<any>>)[KIND_ICON[node.kind]] || Icon.Code;
  const { incoming, outgoing } = adjacency(node.id, allEdges, allNodes);

  const openTarget = () => {
    if (node.kind === "conversation") { navigate.openConv(node.id); return; }
    if (node.kind === "document") { navigate.setActiveDocument(node.id); return; }
    if (node.kind === "skill")    { navigate.openEntity("skills", node.id); return; }
    if (node.kind === "mcp")      { navigate.openEntity("mcp", node.id); return; }
    if (node.kind === "memory")   { navigate.openEntity("memory", node.id); return; }
    if (node.kind === "flowrun")  { navigate.openEntity("execute", node.id); return; }
    navigate.openEntity("forge", node.id);
  };

  return (
    <>
      <div className="rg-detail-head">
        <div className="rg-detail-icon" style={{ background: KIND_COLOR[node.kind] }}>
          <Ic style={{ width: 14, height: 14, color: "white" }} />
        </div>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div className="rg-detail-name">{node.label}</div>
          <div className="rg-detail-id cell-mono">{node.id}</div>
        </div>
        <Button size="xs" onClick={openTarget}><Icon.ArrowRight /> {t("relGraph.open")}</Button>
      </div>
      <div className="rg-detail-body">
        {node.sub && <div className="rg-detail-sub">{node.sub}</div>}
        <AdjacencySection label={t("relGraph.outgoing", { count: outgoing.length })} list={outgoing} onSelect={onSelect} t={t} />
        <AdjacencySection label={t("relGraph.incoming", { count: incoming.length })} list={incoming} onSelect={onSelect} t={t} />
        {outgoing.length === 0 && incoming.length === 0 && (
          <div style={{ fontSize: 12, color: "var(--fg-faint)" }}>{t("relGraph.noRelations")}</div>
        )}
      </div>
    </>
  );
}
function AdjacencySection({ label, list, onSelect, t }: { label: string; list: any[]; onSelect: (id: string) => void; t: any }) {
  if (list.length === 0) return null;
  return (
    <div className="rg-section">
      <div className="rg-section-label">{label}</div>
      {list.map((x, i) => {
        const kindLabel = t("relGraph.kinds." + x.node.kind, { defaultValue: x.node.kind });
        const relKey = REL_LABEL_KEYS[x.edge.kind];
        const relLabel = relKey
          ? t("relGraph.relLabels." + relKey, { defaultValue: x.edge.kind })
          : x.edge.kind;
        return (
          <button key={i} className="rg-adj-row" onClick={() => onSelect(x.node.id)}>
            <span className="rg-adj-dot" style={{ background: KIND_COLOR[x.node.kind] }} />
            <span className="rg-adj-kind">{kindLabel}</span>
            <span className="rg-adj-name">{x.node.label}</span>
            {x.edge.kind && <span className="rg-adj-rel">{relLabel}</span>}
          </button>
        );
      })}
    </div>
  );
}

// ── Auto-size host ───────────────────────────────────────────────────────
function RGAutoSize({ children }: { children: (w: number, h: number) => React.ReactNode }) {
  const ref = useRef(null);
  const [size, setSize] = useState({ w: 600, h: 500 });
  useEffect(() => {
    if (!ref.current) return;
    const update = () => {
      const r = ref.current.getBoundingClientRect();
      setSize({ w: Math.max(300, r.width), h: Math.max(300, r.height) });
    };
    update();
    const ro = new ResizeObserver(update);
    ro.observe(ref.current);
    return () => ro.disconnect();
  }, []);
  return <div ref={ref} className="rg-canvas-host" style={{ width: "100%", height: "100%" }}>{children(size.w, size.h)}</div>;
}

// ── Full graph view ──────────────────────────────────────────────────────
export function RelGraph() {
  const { t } = useTranslation("misc");
  const { nodes: allNodes, edges: allEdges } = useEntityDirectory();

  const [selected, setSelected] = useState(null);
  const [kindFilter, setKindFilter] = useState(new Set());

  const filtered = useMemo(() => {
    if (kindFilter.size === 0) return { nodes: allNodes, edges: allEdges };
    const nodes = allNodes.filter((n) => kindFilter.has(n.kind));
    const ids = new Set(nodes.map((n) => n.id));
    const edges = allEdges.filter((e) => ids.has(e.from) && ids.has(e.to));
    return { nodes, edges };
  }, [allNodes, allEdges, kindFilter]);

  const selectedNode = filtered.nodes.find((n) => n.id === selected);
  const shellRef = useRef(null);

  const allKinds = [...Object.keys(KIND_LABEL_BASE), "conversation", "document"];

  const kindLabel = (k: string) => t("relGraph.kinds." + k, { defaultValue: k });

  return (
    <div className="rg-shell" ref={shellRef}>
      <div className="rg-main">
        <div className="rg-toolbar">
          <span style={{ fontSize: 11, color: "var(--fg-faint)", fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.06em" }}>
            {t("relGraph.filter")}
          </span>
          {allKinds.filter((k) => k !== "flowrun").map((k) => {
            const active = kindFilter.size === 0 || kindFilter.has(k);
            return (
              <button key={k}
                className={"rg-kind-filter" + (active ? " is-active" : "")}
                onClick={() => setKindFilter((s) => {
                  const n = new Set(s);
                  if (n.has(k)) n.delete(k);
                  else if (n.size === 0) return new Set([k]);
                  else n.add(k);
                  return n;
                })}
                style={{ "--kc": KIND_COLOR[k] } as React.CSSProperties}>
                <span className="rg-kind-dot" />
                {kindLabel(k)}
              </button>
            );
          })}
          <Button size="xs" variant="ghost" onClick={() => setKindFilter(new Set())}>{t("relGraph.all")}</Button>
          <div style={{ flex: 1 }} />
          <span style={{ fontSize: 11, color: "var(--fg-faint)", fontFamily: "var(--font-mono)" }}>
            {t("relGraph.nodeCount", { nodes: filtered.nodes.length, edges: filtered.edges.length })}
          </span>
        </div>
        <RGAutoSize>
          {(w, h) => (
            <GraphCanvas nodes={filtered.nodes} edges={filtered.edges}
                         selected={selected} onSelect={setSelected}
                         width={w} height={h} />
          )}
        </RGAutoSize>
      </div>
      <FloatingInspector
        open={!!selectedNode}
        onClose={() => setSelected(null)}
        title={selectedNode ? kindLabel(selectedNode.kind) : ""}
        width={320}
        anchorRef={shellRef}
      >
        {selectedNode && (
          <NodeDetail node={selectedNode} allNodes={allNodes} allEdges={allEdges} onSelect={setSelected} />
        )}
      </FloatingInspector>
    </div>
  );
}

// ── Mini popover focused on a single entity ──────────────────────────────
export function RelGraphPopover({ entityId, kind, onClose, paneEl }: { entityId: string; kind?: string; onClose: () => void; paneEl?: Element | null }) {
  const { t } = useTranslation("misc");
  const { nodes: allNodes } = useEntityDirectory();
  const { data: nb } = useNeighborhood({ kind: kind || guessKind(entityId), id: entityId, depth: 2 });
  const nbAny = nb as any;
  const nodes = useMemo(() => {
    const ids = new Set([entityId, ...(nbAny?.nodes || []).map((n: any) => n.id || n.entityId)]);
    return allNodes.filter((n) => ids.has(n.id));
  }, [allNodes, nb, entityId]);
  const edges = useMemo(() => normEdges(nbAny?.edges || nbAny?.relations || []), [nb]);
  const [selected, setSelected] = useState(entityId);
  const selectedNode = nodes.find((n) => n.id === selected);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const body = (
    <div className="rg-popover-scrim" onClick={onClose}>
      <div className="rg-popover" onClick={(e) => e.stopPropagation()}>
        <div className="rg-popover-head">
          <Icon.GitBranch style={{ width: 14, height: 14, color: "var(--accent)" }} />
          <div style={{ fontSize: 13, fontWeight: 600, color: "var(--fg-strong)" }}>{t("relGraph.refTitle")}</div>
          <span className="cell-mono" style={{ fontSize: 11, color: "var(--fg-muted)" }}>
            {t("relGraph.focusLabel")} · {entityId}
          </span>
          <div style={{ flex: 1 }} />
          <Button size="xs" variant="ghost" onClick={() => { onClose(); navigate.openPane("observe"); }}>
            {t("relGraph.fullGraph")}
          </Button>
          <button className="icon-btn" onClick={onClose}><Icon.X /></button>
        </div>
        <div className="rg-popover-body">
          <div className="rg-popover-canvas">
            <GraphCanvas nodes={nodes} edges={edges} selected={selected} onSelect={setSelected}
                         focusId={entityId} width={420} height={300} />
          </div>
          <NodeDetail node={selectedNode} allNodes={nodes} allEdges={edges} onSelect={setSelected} />
        </div>
      </div>
    </div>
  );
  if (paneEl) return createPortal(body, paneEl);
  return body;
}

// ── "..." trigger that opens RelGraphPopover ─────────────────────────────
export function RelMore({ entityId, kind, label }: { entityId: string; kind?: string; label?: string }) {
  const { t } = useTranslation("misc");
  const [open, setOpen] = useState(false);
  const [paneEl, setPaneEl] = useState(null);
  const btnRef = useRef(null);

  const onClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    const pane = (btnRef.current as HTMLElement).closest(".pane");
    setPaneEl(pane || null);
    setOpen(true);
  };

  return (
    <>
      <button ref={btnRef} className="rel-more-btn" onClick={onClick} title={label || t("entityRelMeta.viewRefs")}>
        <Icon.MoreHorizontal />
      </button>
      {open && <RelGraphPopover entityId={entityId} kind={kind} paneEl={paneEl} onClose={() => setOpen(false)} />}
    </>
  );
}


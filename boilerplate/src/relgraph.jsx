/* eslint-disable react/prop-types */
// Relgraph — force-directed entity relationship graph (Obsidian-style).
//
// Two entry points:
//   <RelGraph onPickEntity={fn} />     — full view (used inside Observe pane)
//   <RelGraphPopover entityId="…" onClose={fn} />  — mini focused graph (used by "…" trigger)

const { useState: useRelState, useEffect: useRelEffect, useRef: useRelRef, useMemo: useRelMemo } = React;

// ── Build node list from window.Forgify (entity directory) ──────────────
function entityDirectory() {
  const out = [];
  Forgify.forges.forEach(f => out.push({
    id: f.id,
    kind: f.kind,
    label: f.name,
    sub: f.desc,
  }));
  Forgify.skills.forEach(s => out.push({ id: s.id, kind: "skill", label: s.name, sub: s.description }));
  Forgify.mcpServers.forEach(m => out.push({ id: m.id, kind: "mcp", label: m.name, sub: (m.tools || 0) + " tools" }));
  Forgify.conversations.forEach(c => out.push({ id: c.id, kind: "conversation", label: c.title, sub: c.model }));
  function walk(nodes) {
    for (const n of nodes) {
      if (n.kind === "page") out.push({ id: n.id, kind: "document", label: n.title, sub: "document" });
      if (n.children) walk(n.children);
    }
  }
  walk(Forgify.documents);
  return out;
}

function lookupEntity(id) {
  return entityDirectory().find(e => e.id === id);
}

// ── Visual mapping ───────────────────────────────────────────────────────
const KIND_COLOR = {
  function:     "#2383E2",
  handler:      "#0F7B6C",
  workflow:     "#D97757",
  skill:        "#B25E10",
  mcp:          "#6940A5",
  memory:       "#9A4A6F",
  conversation: "#3D5A80",
  document:     "#5E6470",
  flowrun:      "#888888",
};

const KIND_ICON = {
  function: "Code", handler: "Server", workflow: "Workflow", skill: "Sparkles",
  mcp: "Server", memory: "Brain", conversation: "MessageSquare", document: "FileText",
  flowrun: "Play",
};

const KIND_LABEL = {
  function: "Function", handler: "Handler", workflow: "Workflow", skill: "Skill",
  mcp: "MCP", memory: "Memory", conversation: "对话", document: "文档", flowrun: "FlowRun",
};

const EDGE_KIND_LABEL = {
  uses: "使用",
  uses_doc: "引用文档",
  forged_from: "由此对话锻造",
  discussed_in: "对话中讨论",
  attached_to: "附在对话",
  referenced_in: "在文档中提到",
  instance_of: "实例属于",
  about: "记录关于",
};

// ── Force-directed layout (very simple, iterative) ──────────────────────
function runLayout(nodes, edges, opts = {}) {
  const { iters = 220, w = 600, h = 500, centerForceK = 0.005, repulseK = 1800, springK = 0.045, springLen = 90, damping = 0.85 } = opts;
  // Init positions: ring around center
  const positions = {};
  const vels = {};
  nodes.forEach((n, i) => {
    const angle = (i / nodes.length) * Math.PI * 2;
    const r = Math.min(w, h) * 0.32;
    positions[n.id] = { x: w / 2 + r * Math.cos(angle), y: h / 2 + r * Math.sin(angle) };
    vels[n.id] = { vx: 0, vy: 0 };
  });
  // Build adjacency for spring length tuning
  const adjEdges = edges.filter(e => positions[e.from] && positions[e.to]);
  for (let step = 0; step < iters; step++) {
    // Repulsive between all pairs
    for (let i = 0; i < nodes.length; i++) {
      for (let j = i + 1; j < nodes.length; j++) {
        const a = nodes[i], b = nodes[j];
        const pa = positions[a.id], pb = positions[b.id];
        let dx = pa.x - pb.x, dy = pa.y - pb.y;
        let dist2 = dx * dx + dy * dy + 0.01;
        const dist = Math.sqrt(dist2);
        const f = repulseK / dist2;
        const fx = (dx / dist) * f;
        const fy = (dy / dist) * f;
        vels[a.id].vx += fx; vels[a.id].vy += fy;
        vels[b.id].vx -= fx; vels[b.id].vy -= fy;
      }
    }
    // Spring along edges
    adjEdges.forEach(e => {
      const pa = positions[e.from], pb = positions[e.to];
      const dx = pb.x - pa.x, dy = pb.y - pa.y;
      const dist = Math.sqrt(dx * dx + dy * dy) || 1;
      const diff = dist - springLen;
      const f = springK * diff;
      const fx = (dx / dist) * f;
      const fy = (dy / dist) * f;
      vels[e.from].vx += fx; vels[e.from].vy += fy;
      vels[e.to].vx -= fx; vels[e.to].vy -= fy;
    });
    // Pull weakly to center
    nodes.forEach(n => {
      const p = positions[n.id];
      vels[n.id].vx += (w / 2 - p.x) * centerForceK;
      vels[n.id].vy += (h / 2 - p.y) * centerForceK;
    });
    // Integrate
    nodes.forEach(n => {
      const v = vels[n.id];
      v.vx *= damping;
      v.vy *= damping;
      positions[n.id].x += v.vx;
      positions[n.id].y += v.vy;
      // Clamp inside box
      positions[n.id].x = Math.max(20, Math.min(w - 20, positions[n.id].x));
      positions[n.id].y = Math.max(20, Math.min(h - 20, positions[n.id].y));
    });
  }
  return positions;
}

// ── Edge degree → node size ─────────────────────────────────────────────
function buildDegree(nodes, edges) {
  const d = Object.fromEntries(nodes.map(n => [n.id, 0]));
  edges.forEach(e => { if (d[e.from] != null) d[e.from]++; if (d[e.to] != null) d[e.to]++; });
  return d;
}

// ── Filter graph to neighborhood of an entity (k-hop) ───────────────────
function neighborhood(entityId, k, allNodes, allEdges) {
  const keep = new Set([entityId]);
  let frontier = new Set([entityId]);
  for (let i = 0; i < k; i++) {
    const next = new Set();
    allEdges.forEach(e => {
      if (frontier.has(e.from) && !keep.has(e.to)) { keep.add(e.to); next.add(e.to); }
      if (frontier.has(e.to)   && !keep.has(e.from)) { keep.add(e.from); next.add(e.from); }
    });
    frontier = next;
    if (frontier.size === 0) break;
  }
  const nodes = allNodes.filter(n => keep.has(n.id));
  const edges = allEdges.filter(e => keep.has(e.from) && keep.has(e.to));
  return { nodes, edges };
}

// ── Get adjacent (any direction) entities and the edges connecting them ─
function adjacencyOf(entityId, allEdges, allNodes) {
  const incoming = [];
  const outgoing = [];
  allEdges.forEach(e => {
    if (e.from === entityId) {
      const target = allNodes.find(n => n.id === e.to);
      if (target) outgoing.push({ edge: e, node: target });
    }
    if (e.to === entityId) {
      const source = allNodes.find(n => n.id === e.from);
      if (source) incoming.push({ edge: e, node: source });
    }
  });
  return { incoming, outgoing };
}

// ── The actual graph SVG ─────────────────────────────────────────────────
function GraphCanvas({ nodes, edges, selected, onSelect, focusId, width, height }) {
  const degree = useRelMemo(() => buildDegree(nodes, edges), [nodes, edges]);
  const [hover, setHover] = useRelState(null);
  const [transform, setTransform] = useRelState({ x: 0, y: 0, scale: 1 });
  const [, rerender] = useRelState(0);

  // simulation refs (persist across rerenders)
  const positions = useRelRef({});
  const velocities = useRelRef({});
  const dragRef = useRelRef(null);      // { id, dx, dy } pinned
  const panRef = useRelRef(null);       // { sx, sy, tx, ty } during pan
  const containerRef = useRelRef(null);

  // ── Initialize positions once when nodes change (warm start with runLayout) ──
  useRelEffect(() => {
    const init = runLayout(nodes, edges, { w: width, h: height });
    const next = {};
    const vels = {};
    nodes.forEach(n => {
      const prev = positions.current[n.id];
      next[n.id] = prev || init[n.id] || { x: width / 2, y: height / 2 };
      vels[n.id] = { vx: 0, vy: 0 };
    });
    positions.current = next;
    velocities.current = vels;
    rerender(x => x + 1);
  }, [nodes.length, edges.length, width, height]);

  // ── Continuous force simulation ──
  useRelEffect(() => {
    let raf;
    const tick = () => {
      const N = nodes.length;
      const repulseK = 2200;
      const springK = 0.04;
      const springLen = 110;
      const damping = 0.82;
      const center = { x: width / 2, y: height / 2 };
      const centerK = 0.002;

      // Repulsive between all pairs
      for (let i = 0; i < N; i++) {
        const a = nodes[i];
        const pa = positions.current[a.id];
        if (!pa) continue;
        for (let j = i + 1; j < N; j++) {
          const b = nodes[j];
          const pb = positions.current[b.id];
          if (!pb) continue;
          let dx = pa.x - pb.x, dy = pa.y - pb.y;
          let dist2 = dx * dx + dy * dy + 0.01;
          const dist = Math.sqrt(dist2);
          const f = repulseK / dist2;
          const fx = (dx / dist) * f;
          const fy = (dy / dist) * f;
          velocities.current[a.id].vx += fx; velocities.current[a.id].vy += fy;
          velocities.current[b.id].vx -= fx; velocities.current[b.id].vy -= fy;
        }
      }
      // Springs
      edges.forEach(e => {
        const pa = positions.current[e.from], pb = positions.current[e.to];
        if (!pa || !pb) return;
        const dx = pb.x - pa.x, dy = pb.y - pa.y;
        const dist = Math.sqrt(dx * dx + dy * dy) || 1;
        const diff = dist - springLen;
        const f = springK * diff;
        const fx = (dx / dist) * f;
        const fy = (dy / dist) * f;
        velocities.current[e.from].vx += fx; velocities.current[e.from].vy += fy;
        velocities.current[e.to].vx -= fx; velocities.current[e.to].vy -= fy;
      });
      // Weak center pull (keeps the cluster from drifting off)
      nodes.forEach(n => {
        const p = positions.current[n.id];
        if (!p) return;
        velocities.current[n.id].vx += (center.x - p.x) * centerK;
        velocities.current[n.id].vy += (center.y - p.y) * centerK;
      });
      // Integrate (skip pinned node)
      nodes.forEach(n => {
        const p = positions.current[n.id];
        const v = velocities.current[n.id];
        if (!p || !v) return;
        if (dragRef.current && dragRef.current.id === n.id) {
          // pinned — position controlled by user; freeze velocity
          v.vx = 0; v.vy = 0;
          return;
        }
        v.vx *= damping;
        v.vy *= damping;
        p.x += v.vx;
        p.y += v.vy;
      });
      rerender(x => (x + 1) % 1e9);
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [nodes, edges, width, height]);

  // ── Coords helpers ──
  const clientToCanvas = (cx, cy) => {
    const r = containerRef.current.getBoundingClientRect();
    return {
      x: (cx - r.left - transform.x) / transform.scale,
      y: (cy - r.top - transform.y) / transform.scale,
    };
  };

  // ── Mouse handlers ──
  const onSvgMouseDown = (e) => {
    if (e.button !== 0) return;
    if (e.target === e.currentTarget || e.target.tagName === "rect" || e.target.tagName === "svg") {
      panRef.current = { sx: e.clientX, sy: e.clientY, tx: transform.x, ty: transform.y };
    }
  };
  useRelEffect(() => {
    const onMove = (e) => {
      if (dragRef.current) {
        const c = clientToCanvas(e.clientX, e.clientY);
        const p = positions.current[dragRef.current.id];
        if (p) {
          // Give a tiny velocity nudge to neighbors via Newton-ish push
          const newX = c.x - dragRef.current.dx;
          const newY = c.y - dragRef.current.dy;
          // velocity record for "throw" on release
          dragRef.current.lastDx = newX - p.x;
          dragRef.current.lastDy = newY - p.y;
          p.x = newX;
          p.y = newY;
        }
      } else if (panRef.current) {
        setTransform(t => ({
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
          v.vx = dragRef.current.lastDx * 2;
          v.vy = dragRef.current.lastDy * 2;
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

  const onNodeMouseDown = (e, n) => {
    e.stopPropagation();
    const c = clientToCanvas(e.clientX, e.clientY);
    const p = positions.current[n.id];
    dragRef.current = { id: n.id, dx: c.x - p.x, dy: c.y - p.y };
    onSelect(n.id);
  };

  const onWheel = (e) => {
    e.preventDefault();
    const r = containerRef.current.getBoundingClientRect();
    const mx = e.clientX - r.left;
    const my = e.clientY - r.top;
    const delta = -e.deltaY * 0.0015;
    setTransform(t => {
      const scale = Math.max(0.25, Math.min(3, t.scale * (1 + delta)));
      const ratio = scale / t.scale;
      return {
        x: mx - (mx - t.x) * ratio,
        y: my - (my - t.y) * ratio,
        scale,
      };
    });
  };

  // ── Toolbar actions ──
  const zoomBy = (factor) => {
    setTransform(t => {
      const scale = Math.max(0.25, Math.min(3, t.scale * factor));
      const r = containerRef.current.getBoundingClientRect();
      const mx = r.width / 2;
      const my = r.height / 2;
      const ratio = scale / t.scale;
      return { x: mx - (mx - t.x) * ratio, y: my - (my - t.y) * ratio, scale };
    });
  };
  const fitToContent = () => {
    if (!containerRef.current || nodes.length === 0) return;
    const xs = nodes.map(n => positions.current[n.id]?.x).filter(v => v != null);
    const ys = nodes.map(n => positions.current[n.id]?.y).filter(v => v != null);
    if (!xs.length) return;
    const minX = Math.min(...xs), maxX = Math.max(...xs);
    const minY = Math.min(...ys), maxY = Math.max(...ys);
    const r = containerRef.current.getBoundingClientRect();
    const pad = 60;
    const sx = (r.width - pad * 2) / Math.max(1, maxX - minX);
    const sy = (r.height - pad * 2) / Math.max(1, maxY - minY);
    const scale = Math.max(0.25, Math.min(1.5, Math.min(sx, sy)));
    setTransform({
      x: -minX * scale + (r.width - (maxX - minX) * scale) / 2,
      y: -minY * scale + (r.height - (maxY - minY) * scale) / 2,
      scale,
    });
  };
  const resetView = () => setTransform({ x: 0, y: 0, scale: 1 });

  // ── Render ──
  const isPan = panRef.current != null;
  return (
    <div
      ref={containerRef}
      className={"rg-container" + (isPan ? " is-panning" : "")}
      onWheel={onWheel}
      style={{ width: "100%", height: "100%", position: "relative", overflow: "hidden" }}
    >
      <svg
        width={width}
        height={height}
        className="rg-svg"
        onMouseDown={onSvgMouseDown}
        style={{
          transform: `translate(${transform.x}px, ${transform.y}px) scale(${transform.scale})`,
          transformOrigin: "0 0",
          cursor: isPan ? "grabbing" : "default",
        }}
      >
        <rect x="-5000" y="-5000" width="10000" height="10000" fill="transparent" />
        {edges.map((e, i) => {
          const pa = positions.current[e.from], pb = positions.current[e.to];
          if (!pa || !pb) return null;
          const touchesActive = selected === e.from || selected === e.to || hover === e.from || hover === e.to;
          const fade = (selected || hover) && !touchesActive;
          return (
            <line
              key={i}
              x1={pa.x} y1={pa.y} x2={pb.x} y2={pb.y}
              stroke={touchesActive ? "var(--accent)" : "var(--border-strong)"}
              strokeWidth={touchesActive ? 1.4 : 0.8}
              opacity={fade ? 0.10 : (touchesActive ? 0.9 : 0.35)}
            />
          );
        })}
        {nodes.map(n => {
          const p = positions.current[n.id];
          if (!p) return null;
          const r = focusId === n.id ? 8 : 3 + Math.min(5, (degree[n.id] || 0) * 0.6);
          const isSelected = selected === n.id;
          const isHover = hover === n.id;
          const isFocus = focusId === n.id;
          const fade = (selected || hover) && !isSelected && !isHover && !isFocus;
          return (
            <g
              key={n.id}
              transform={`translate(${p.x},${p.y})`}
              className="rg-node"
              onMouseDown={(e) => onNodeMouseDown(e, n)}
              onClick={(e) => { e.stopPropagation(); onSelect(n.id); }}
              onMouseEnter={() => setHover(n.id)}
              onMouseLeave={() => setHover(null)}
            >
              {(isSelected || isFocus) && (
                <circle r={r + 5} fill="none" stroke="var(--accent)" strokeWidth="1.2" opacity="0.55" />
              )}
              <circle
                r={r}
                fill={KIND_COLOR[n.kind] || "#999"}
                stroke="var(--bg-paper)"
                strokeWidth={isHover ? 2 : 1}
                opacity={fade ? 0.20 : 1}
                style={{ cursor: dragRef.current?.id === n.id ? "grabbing" : "grab" }}
              />
            </g>
          );
        })}
        {(hover || focusId) && (() => {
          const id = hover || focusId;
          const n = nodes.find(x => x.id === id);
          const p = positions.current[id];
          if (!n || !p) return null;
          const text = n.label.length > 28 ? n.label.slice(0, 28) + "…" : n.label;
          const charW = 6.2;
          const w = Math.min(280, Math.max(60, text.length * charW + 16));
          return (
            <g pointerEvents="none">
              <rect
                x={p.x - w / 2} y={p.y + 6}
                width={w} height={22}
                rx="4"
                fill="var(--bg-elev)"
                stroke="var(--border)"
                strokeWidth="1"
                style={{ filter: "drop-shadow(0 1px 3px rgba(0,0,0,0.10))" }}
              />
              <text
                x={p.x} y={p.y + 21}
                textAnchor="middle"
                fontSize="11"
                fontFamily="var(--font-sans)"
                fontWeight="500"
                fill="var(--fg-strong)"
              >
                {text}
              </text>
            </g>
          );
        })()}
      </svg>

      <div className="rg-toolbar">
        <button className="icon-btn" title="放大" onClick={() => zoomBy(1.2)}><Icon.Plus /></button>
        <button className="icon-btn" title="缩小" onClick={() => zoomBy(1 / 1.2)}>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round"><path d="M5 12h14"/></svg>
        </button>
        <div className="rg-toolbar-sep" />
        <button className="icon-btn" title="适配画面" onClick={fitToContent}><Icon.Filter /></button>
        <button className="icon-btn" title="复位" onClick={resetView}><Icon.Refresh /></button>
        <div className="rg-zoom-pct">{Math.round(transform.scale * 100)}%</div>
      </div>
    </div>
  );
}

// ── Detail panel showing selected node + its neighbors ──────────────────
function NodeDetail({ node, allNodes, allEdges, onSelect }) {
  if (!node) {
    return (
      <div className="rg-detail">
        <div className="empty" style={{ padding: "32px 16px" }}>
          <Icon.GitBranch className="icon" />
          <div className="title">点一个节点查看</div>
          <div className="sub">点节点看引用</div>
        </div>
      </div>
    );
  }
  const Ic = Icon[KIND_ICON[node.kind]] || Icon.Code;
  const { incoming, outgoing } = adjacencyOf(node.id, allEdges, allNodes);
  return (
    <div className="rg-detail">
      <div className="rg-detail-head">
        <div className="rg-detail-icon" style={{ background: KIND_COLOR[node.kind] }}>
          <Ic style={{ width: 14, height: 14, color: "white" }} />
        </div>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div className="rg-detail-kind">{KIND_LABEL[node.kind]}</div>
          <div className="rg-detail-name">{node.label}</div>
          <div className="rg-detail-id cell-mono">{node.id}</div>
        </div>
        <button
          className="btn btn-xs"
          onClick={() => {
            if (!window.Shell) return;
            if (node.kind === "conversation") window.Shell.openConv(node.id);
            else if (node.kind === "document") window.Shell.openEntity?.("documents", node.id);
            else if (node.kind === "skill")    window.Shell.openEntity?.("skills", node.id);
            else if (node.kind === "mcp")      window.Shell.openEntity?.("mcp", node.id);
            else if (node.kind === "memory")   window.Shell.openEntity?.("memory", node.id);
            else if (node.kind === "flowrun")  window.Shell.openEntity?.("execute", node.id);
            else                               window.Shell.openEntity?.("forge", node.id);
          }}
        >
          <Icon.ArrowRight /> 打开
        </button>
      </div>
      <div className="rg-detail-body">
        {node.sub && <div className="rg-detail-sub">{node.sub}</div>}

        {outgoing.length > 0 && (
          <div className="rg-section">
            <div className="rg-section-label">→ 引用 / 使用 ({outgoing.length})</div>
            {outgoing.map((x, i) => (
              <button key={i} className="rg-adj-row" onClick={() => onSelect(x.node.id)}>
                <span className="rg-adj-dot" style={{ background: KIND_COLOR[x.node.kind] }} />
                <span className="rg-adj-kind">{KIND_LABEL[x.node.kind]}</span>
                <span className="rg-adj-name">{x.node.label}</span>
                <span className="rg-adj-rel">{EDGE_KIND_LABEL[x.edge.kind] || x.edge.kind}</span>
              </button>
            ))}
          </div>
        )}

        {incoming.length > 0 && (
          <div className="rg-section">
            <div className="rg-section-label">← 被引用 ({incoming.length})</div>
            {incoming.map((x, i) => (
              <button key={i} className="rg-adj-row" onClick={() => onSelect(x.node.id)}>
                <span className="rg-adj-dot" style={{ background: KIND_COLOR[x.node.kind] }} />
                <span className="rg-adj-kind">{KIND_LABEL[x.node.kind]}</span>
                <span className="rg-adj-name">{x.node.label}</span>
                <span className="rg-adj-rel">{EDGE_KIND_LABEL[x.edge.kind] || x.edge.kind}</span>
              </button>
            ))}
          </div>
        )}

        {outgoing.length === 0 && incoming.length === 0 && (
          <div style={{ fontSize: 12, color: "var(--fg-faint)" }}>暂无引用关系</div>
        )}
      </div>
    </div>
  );
}

// ── Full graph view (Observe pane) ──────────────────────────────────────
function RelGraph() {
  const allNodes = useRelMemo(() => entityDirectory(), []);
  const allEdges = Forgify.relations;
  const [selected, setSelected] = useRelState(null);
  const [kindFilter, setKindFilter] = useRelState(new Set());

  const filtered = useRelMemo(() => {
    if (kindFilter.size === 0) return { nodes: allNodes, edges: allEdges };
    const nodes = allNodes.filter(n => kindFilter.has(n.kind));
    const ids = new Set(nodes.map(n => n.id));
    const edges = allEdges.filter(e => ids.has(e.from) && ids.has(e.to));
    return { nodes, edges };
  }, [allNodes, allEdges, kindFilter]);

  const selectedNode = filtered.nodes.find(n => n.id === selected);

  return (
    <div className="rg-shell">
      <div className="rg-main">
        <div className="rg-toolbar">
          <span style={{ fontSize: 11, color: "var(--fg-faint)", fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.06em" }}>
            过滤
          </span>
          {["function", "handler", "workflow", "skill", "mcp", "conversation", "document"].map(k => {
            const active = kindFilter.size === 0 || kindFilter.has(k);
            return (
              <button
                key={k}
                className={"rg-kind-filter" + (active ? " is-active" : "")}
                onClick={() => setKindFilter(s => {
                  const n = new Set(s);
                  if (n.has(k)) n.delete(k);
                  else if (n.size === 0) {
                    // First click: select only this kind
                    return new Set([k]);
                  } else n.add(k);
                  return n;
                })}
                style={{ "--kc": KIND_COLOR[k] }}
              >
                <span className="rg-kind-dot" />
                {KIND_LABEL[k]}
              </button>
            );
          })}
          <button className="btn btn-xs btn-ghost" onClick={() => setKindFilter(new Set())}>
            全部
          </button>
          <div style={{ flex: 1 }} />
          <span style={{ fontSize: 11, color: "var(--fg-faint)", fontFamily: "var(--font-mono)" }}>
            {filtered.nodes.length} 节点 · {filtered.edges.length} 边
          </span>
        </div>
        <RGAutoSize>
          {(w, h) => (
            <GraphCanvas
              nodes={filtered.nodes}
              edges={filtered.edges}
              selected={selected}
              onSelect={setSelected}
              width={w}
              height={h}
            />
          )}
        </RGAutoSize>
      </div>
      <NodeDetail node={selectedNode} allNodes={allNodes} allEdges={allEdges} onSelect={setSelected} />
    </div>
  );
}

function RGAutoSize({ children }) {
  const ref = useRelRef(null);
  const [size, setSize] = useRelState({ w: 600, h: 500 });
  useRelEffect(() => {
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
  return <div ref={ref} className="rg-canvas-host">{children(size.w, size.h)}</div>;
}

// ── Mini popover focused on a single entity ─────────────────────────────
function RelGraphPopover({ entityId, onClose, paneEl }) {
  const allNodes = useRelMemo(() => entityDirectory(), []);
  const allEdges = Forgify.relations;
  const { nodes, edges } = useRelMemo(() => neighborhood(entityId, 2, allNodes, allEdges), [entityId, allNodes, allEdges]);
  const [selected, setSelected] = useRelState(entityId);
  const selectedNode = nodes.find(n => n.id === selected);

  useRelEffect(() => {
    const onKey = (e) => { if (e.key === "Escape") onClose(); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const body = (
    <div className="rg-popover-scrim" onClick={onClose}>
      <div className="rg-popover" onClick={e => e.stopPropagation()}>
        <div className="rg-popover-head">
          <Icon.GitBranch style={{ width: 14, height: 14, color: "var(--accent)" }} />
          <div style={{ fontSize: 13, fontWeight: 600, color: "var(--fg-strong)" }}>引用关系</div>
          <span className="cell-mono" style={{ fontSize: 11, color: "var(--fg-muted)" }}>
            焦点 · {entityId}
          </span>
          <div style={{ flex: 1 }} />
          <button
            className="btn btn-xs btn-ghost"
            onClick={() => { onClose(); window.Shell?.openPane("observe"); }}
            title="在洞察里打开完整图谱"
          >
            完整图谱 →
          </button>
          <button className="icon-btn" onClick={onClose}><Icon.X /></button>
        </div>
        <div className="rg-popover-body">
          <div className="rg-popover-canvas">
            <GraphCanvas
              nodes={nodes}
              edges={edges}
              selected={selected}
              onSelect={setSelected}
              focusId={entityId}
              width={420}
              height={300}
            />
          </div>
          <NodeDetail node={selectedNode} allNodes={nodes} allEdges={edges} onSelect={setSelected} />
        </div>
      </div>
    </div>
  );

  // Render inside the originating pane so the popover lives in that pane's coord system.
  if (paneEl) return ReactDOM.createPortal(body, paneEl);
  return body;
}

// ── "..." trigger that opens RelGraphPopover ────────────────────────────
function RelMore({ entityId, label }) {
  const [open, setOpen] = useRelState(false);
  const [paneEl, setPaneEl] = useRelState(null);
  const btnRef = useRelRef(null);

  const onClick = (e) => {
    e.stopPropagation();
    const pane = btnRef.current.closest(".pane");
    setPaneEl(pane || null);
    setOpen(true);
  };

  return (
    <>
      <button ref={btnRef} className="rel-more-btn" onClick={onClick} title={label || "查看引用关系"}>
        <Icon.MoreHorizontal />
      </button>
      {open && <RelGraphPopover entityId={entityId} paneEl={paneEl} onClose={() => setOpen(false)} />}
    </>
  );
}

window.RelGraph = RelGraph;
window.RelGraphPopover = RelGraphPopover;
window.RelMore = RelMore;
window.lookupEntity = lookupEntity;
window.adjacencyOf = adjacencyOf;

// ── Meta line for any entity (shows most-recently-updated relation) ─────
const VERB_TEMPLATE = {
  uses:          (kind) => kind === "document" ? "引用文档" : "使用",
  uses_doc:      () => "引用文档",
  forged_from:   () => "由对话锻造",
  discussed_in:  () => "在对话中讨论",
  attached_to:   () => "附在对话",
  referenced_in: () => "被引用于",
  instance_of:   () => "属于工作流",
  about:         () => "被记忆关联",
  produced:      () => "产出于对话",
};

function EntityRelMeta({ entityId, inline = false }) {
  const allNodes = useRelMemo(() => entityDirectory(), []);
  const allEdges = Forgify.relations || [];
  const { incoming, outgoing } = adjacencyOf(entityId, allEdges, allNodes);
  const all = [...outgoing.map(x => ({ ...x, direction: "out" })), ...incoming.map(x => ({ ...x, direction: "in" }))];
  if (all.length === 0) return null;
  // Pick a "primary" — first incoming forged_from / discussed_in, else first outgoing uses, else first
  const primary =
    all.find(x => x.direction === "in" && x.edge.kind === "forged_from") ||
    all.find(x => x.direction === "in" && x.edge.kind === "discussed_in") ||
    all.find(x => x.direction === "out" && (x.edge.kind === "uses" || x.edge.kind === "uses_doc")) ||
    all[0];
  if (!primary) return null;
  const verbFn = VERB_TEMPLATE[primary.edge.kind] || (() => "关联");
  const verb = verbFn(primary.node.kind);
  const Ic = Icon[KIND_ICON[primary.node.kind]] || Icon.Code;
  return (
    <span className="entity-rel-meta" style={{ display: "inline-flex", alignItems: "center", gap: 4, flexWrap: "wrap" }}>
      <span style={{ color: "var(--fg-faint)" }}>·</span>
      <span style={{ color: "var(--fg-muted)" }}>{verb}</span>
      <button
        className="entity-link"
        onClick={() => {
          if (!window.Shell) return;
          if (primary.node.kind === "conversation") window.Shell.openConv(primary.node.id);
          else if (primary.node.kind === "document") window.Shell.openEntity?.("documents", primary.node.id);
          else if (primary.node.kind === "skill")    window.Shell.openEntity?.("skills", primary.node.id);
          else if (primary.node.kind === "mcp")      window.Shell.openEntity?.("mcp", primary.node.id);
          else                                       window.Shell.openEntity?.("forge", primary.node.id);
        }}
      >
        <Ic className="icon" />{primary.node.id}
      </button>
      <RelMore entityId={entityId} label="查看所有引用关系" />
    </span>
  );
}

window.EntityRelMeta = EntityRelMeta;

// ── Generic action menu (replaces RelMore in list views) ────────────────
function ActionMenu({ items }) {
  const [open, setOpen] = useRelState(false);
  const [pos, setPos] = useRelState(null);
  const btnRef = useRelRef(null);
  const onClick = (e) => {
    e.stopPropagation();
    if (open) { setOpen(false); return; }
    const r = btnRef.current.getBoundingClientRect();
    setPos({
      top: r.bottom + 4,
      left: Math.min(r.left, window.innerWidth - 200),
    });
    setOpen(true);
  };
  useRelEffect(() => {
    if (!open) return;
    const close = (e) => {
      // ignore clicks inside the popover
      if (e.target.closest(".action-menu")) return;
      setOpen(false);
    };
    setTimeout(() => window.addEventListener("click", close), 0);
    const onScroll = () => setOpen(false);
    window.addEventListener("scroll", onScroll, true);
    window.addEventListener("resize", onScroll);
    return () => {
      window.removeEventListener("click", close);
      window.removeEventListener("scroll", onScroll, true);
      window.removeEventListener("resize", onScroll);
    };
  }, [open]);
  return (
    <div className="action-menu-wrap" onClick={(e) => e.stopPropagation()}>
      <button ref={btnRef} className="rel-more-btn" onClick={onClick} title="操作"><Icon.MoreHorizontal /></button>
      {open && pos && ReactDOM.createPortal(
        <div className="action-menu" style={{ position: "fixed", top: pos.top, left: pos.left }}>
          {items.map((it, i) => it === "divider" ? (
            <div key={i} className="action-divider" />
          ) : (
            <button key={i} className={it.danger ? "is-danger" : ""} onClick={() => { it.onClick?.(); setOpen(false); }}>
              {it.icon && React.createElement(it.icon)}
              {it.label}
              {it.shortcut && <span className="shortcut">{it.shortcut}</span>}
            </button>
          ))}
        </div>,
        document.body
      )}
    </div>
  );
}

window.ActionMenu = ActionMenu;


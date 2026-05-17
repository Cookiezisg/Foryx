/* eslint-disable react/prop-types */
// Workflow detail — VersionRail + DAG canvas
//
//   Current → editable canvas (existing WorkflowEditor)
//   Other   → read-only DAG with diff overlay (added/removed/changed) + change list

const { useState: useWfState, useRef: useWfRef, useEffect: useWfEffect, useMemo: useWfMemo } = React;

const WF_NODE_W = 184;
const WF_NODE_H = 76;

const WF_NODE_KINDS = [
  { kind: "trigger",   label: "Trigger",   icon: "Zap",      desc: "Cron / Webhook / Manual" },
  { kind: "function",  label: "Function",  icon: "Code",     desc: "纯函数 · 沙箱执行" },
  { kind: "handler",   label: "Handler",   icon: "Server",   desc: "Stateful 类调用" },
  { kind: "mcp",       label: "MCP Tool",  icon: "Server",   desc: "调 MCP server" },
  { kind: "skill",     label: "Skill",     icon: "Sparkles", desc: "SKILL.md 模板" },
  { kind: "llm",       label: "LLM",       icon: "Brain",    desc: "纯 LLM 节点" },
  { kind: "http",      label: "HTTP",      icon: "Globe",    desc: "外部 API" },
  { kind: "condition", label: "Condition", icon: "GitBranch",desc: "分支判断" },
  { kind: "loop",      label: "Loop",      icon: "Refresh",  desc: "迭代" },
  { kind: "parallel",  label: "Parallel",  icon: "Layers",   desc: "并行 fan-out" },
  { kind: "approval",  label: "Approval",  icon: "Pause",    desc: "等待人工" },
  { kind: "wait",      label: "Wait",      icon: "Clock",    desc: "定时延迟" },
  { kind: "variable",  label: "Variable",  icon: "Database", desc: "读写变量" },
];

const HANDLES = ["top", "right", "bottom", "left"];
const HANDLE_OFFSET = {
  top:    { x: WF_NODE_W / 2,  y: 0,           dx: 0,  dy: -1 },
  right:  { x: WF_NODE_W,      y: WF_NODE_H/2, dx: 1,  dy: 0  },
  bottom: { x: WF_NODE_W / 2,  y: WF_NODE_H,   dx: 0,  dy: 1  },
  left:   { x: 0,              y: WF_NODE_H/2, dx: -1, dy: 0  },
};
function edgePathBetween(a, b, fromH, toH) {
  const f = HANDLE_OFFSET[fromH || "bottom"];
  const t = HANDLE_OFFSET[toH || "top"];
  const sx = a.x + f.x, sy = a.y + f.y;
  const ex = b.x + t.x, ey = b.y + t.y;
  const dist = Math.max(40, Math.hypot(ex - sx, ey - sy) * 0.4);
  return `M ${sx} ${sy} C ${sx + f.dx*dist} ${sy + f.dy*dist}, ${ex + t.dx*dist} ${ey + t.dy*dist}, ${ex} ${ey}`;
}

function nodeIcon(kind) {
  return Icon[
    kind === "trigger" ? "Zap" :
    kind === "function" ? "Code" :
    kind === "handler" ? "Server" :
    kind === "approval" ? "Pause" :
    kind === "variable" ? "Database" :
    kind === "condition" ? "GitBranch" :
    kind === "loop" ? "Refresh" :
    kind === "parallel" ? "Layers" :
    kind === "wait" ? "Clock" :
    kind === "skill" ? "Sparkles" :
    kind === "mcp" ? "Server" :
    kind === "llm" ? "Brain" :
    kind === "http" ? "Globe" :
    "Code"
  ] || Icon.Code;
}

// ─────────────────────── EDITABLE CANVAS (current version) ───────────────────────
function Palette({ onAdd }) {
  const [q, setQ] = useWfState("");
  const list = WF_NODE_KINDS.filter(k => (k.label + k.desc).toLowerCase().includes(q.toLowerCase()));
  return (
    <aside className="wf-palette">
      <div className="search-input" style={{ width: "100%" }}>
        <Icon.Search className="icon" />
        <input placeholder="拖入节点…" value={q} onChange={e => setQ(e.target.value)} />
      </div>
      <div className="wf-palette-label">节点</div>
      <div className="wf-palette-list">
        {list.map(k => {
          const Ic = Icon[k.icon] || Icon.Code;
          return (
            <button
              key={k.kind}
              className="wf-palette-item"
              draggable
              onDragStart={e => e.dataTransfer.setData("kind", k.kind)}
              onClick={() => onAdd(k.kind)}
            >
              <div className="wf-palette-icon"><Ic /></div>
              <div>
                <div className="wf-palette-name">{k.label}</div>
                <div className="wf-palette-desc">{k.desc}</div>
              </div>
            </button>
          );
        })}
      </div>
    </aside>
  );
}

function CanvasNode({ node, selected, diffMark, onMouseDown, onHandleMouseDown, connectingFrom }) {
  const Ic = nodeIcon(node.kind);
  return (
    <div
      className={[
        "wf-node",
        selected && "is-selected",
        diffMark && "is-diff-" + diffMark,
      ].filter(Boolean).join(" ")}
      style={{ left: node.x, top: node.y, width: WF_NODE_W }}
      onMouseDown={onMouseDown}
      data-id={node.id}
    >
      {onHandleMouseDown && HANDLES.map(h => (
        <div key={h} className={"wf-node-handle " + h + (connectingFrom?.id === node.id && connectingFrom?.handle === h ? " is-active" : "")}
             data-handle={h} data-id={node.id}
             onMouseDown={(e) => onHandleMouseDown(e, node.id, h)} />
      ))}
      <div className="wf-node-head">
        <div className={"wf-node-icon kind-" + node.kind}><Ic /></div>
        <div className="wf-node-title">{node.label}</div>
        {diffMark && <span className={"wf-diff-mark wf-diff-mark-" + diffMark}>
          {diffMark === "added" ? "+" : diffMark === "removed" ? "−" : "Δ"}
        </span>}
      </div>
      <div className="wf-node-sub">{node.sub}</div>
    </div>
  );
}

// ── Diff helpers ────────────────────────────────────────────────────────
function buildDiff(currentVer, otherVer) {
  const curN = new Map(currentVer.nodes.map(n => [n.id, n]));
  const othN = new Map(otherVer.nodes.map(n => [n.id, n]));
  const curE = new Set(currentVer.edges.map(e => e.from + "→" + e.to));
  const othE = new Set(otherVer.edges.map(e => e.from + "→" + e.to));

  const nodeChanges = [];
  const allNodeIds = new Set([...curN.keys(), ...othN.keys()]);
  for (const id of allNodeIds) {
    const a = curN.get(id);
    const b = othN.get(id);
    if (!a) { nodeChanges.push({ id, kind: "added", node: b }); continue; }
    if (!b) { nodeChanges.push({ id, kind: "removed", node: a }); continue; }
    // changed fields
    const changes = [];
    if (a.label !== b.label) changes.push({ field: "标签", a: a.label, b: b.label });
    if (a.sub !== b.sub) changes.push({ field: "引用", a: a.sub, b: b.sub });
    if (a.kind !== b.kind) changes.push({ field: "类型", a: a.kind, b: b.kind });
    if ((a.retry || 0) !== (b.retry || 0)) changes.push({ field: "重试", a: a.retry, b: b.retry });
    if ((a.timeout || 0) !== (b.timeout || 0)) changes.push({ field: "超时", a: a.timeout + "s", b: b.timeout + "s" });
    if (a.onError !== b.onError) changes.push({ field: "onError", a: a.onError, b: b.onError });
    if (changes.length > 0) nodeChanges.push({ id, kind: "changed", node: b, prevNode: a, changes });
  }

  const edgeChanges = [];
  for (const e of currentVer.edges) {
    const key = e.from + "→" + e.to;
    if (!othE.has(key)) edgeChanges.push({ kind: "removed", edge: e });
  }
  for (const e of otherVer.edges) {
    const key = e.from + "→" + e.to;
    if (!curE.has(key)) edgeChanges.push({ kind: "added", edge: e });
  }

  return { nodeChanges, edgeChanges };
}

// ─────────────────────── DIFF VIEW (read-only DAG + change list) ─────────────────
function WorkflowDiffView({ currentV, otherV, pendingV }) {
  const [selectedNodeId, setSelectedNodeId] = useWfState(null);
  const { nodeChanges, edgeChanges } = useWfMemo(() => buildDiff(currentV, otherV), [currentV, otherV]);
  const isPending = otherV.id === pendingV?.id;

  // Build a "merged" node list for rendering: prefer B (otherV) positions, fall back to A (currentV) for removed.
  const byIdA = new Map(currentV.nodes.map(n => [n.id, n]));
  const byIdB = new Map(otherV.nodes.map(n => [n.id, n]));
  const allIds = new Set([...byIdA.keys(), ...byIdB.keys()]);
  const renderNodes = [...allIds].map(id => {
    const a = byIdA.get(id);
    const b = byIdB.get(id);
    if (!a && b) return { ...b, _diff: "added" };
    if (a && !b) return { ...a, _diff: "removed" };
    const ch = nodeChanges.find(c => c.id === id && c.kind === "changed");
    return { ...b, _diff: ch ? "changed" : null };
  });

  // Edges: union with diff marks
  const allEdges = [];
  const seen = new Set();
  for (const e of currentV.edges) {
    const key = e.from + "→" + e.to;
    seen.add(key);
    const inB = otherV.edges.some(x => x.from === e.from && x.to === e.to);
    allEdges.push({ ...e, _diff: inB ? null : "removed" });
  }
  for (const e of otherV.edges) {
    const key = e.from + "→" + e.to;
    if (seen.has(key)) continue;
    allEdges.push({ ...e, _diff: "added" });
  }

  const byIdRender = new Map(renderNodes.map(n => [n.id, n]));
  const totalChanges = nodeChanges.length + edgeChanges.length;

  return (
    <div className="wf-diff">
      <aside className="wf-diff-list">
        <div className="wf-diff-list-head">
          <Icon.GitBranch style={{ width: 13, height: 13 }} />
          <span>变更清单 · {totalChanges}</span>
        </div>

        {nodeChanges.length > 0 && (
          <>
            <div className="wf-diff-section-label">节点 · {nodeChanges.length}</div>
            {nodeChanges.map((c, i) => (
              <button key={i}
                      className={"wf-diff-item wf-diff-item-" + c.kind + (selectedNodeId === c.id ? " is-active" : "")}
                      onClick={() => setSelectedNodeId(c.id)}>
                <span className={"wf-diff-dot wf-diff-dot-" + c.kind} />
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div className="wf-diff-item-title">
                    <span className="cell-mono">{c.node.label}</span>
                    <span style={{ fontSize: 10, color: "var(--fg-faint)", marginLeft: 4 }}>{c.node.kind}</span>
                  </div>
                  {c.kind === "changed" && c.changes && (
                    <div className="wf-diff-item-changes">
                      {c.changes.map((ch, j) => (
                        <div key={j} className="wf-diff-chg-row">
                          <span className="wf-diff-chg-field">{ch.field}</span>
                          <span className="wf-diff-chg-a">{String(ch.a)}</span>
                          <span style={{ color: "var(--fg-faint)" }}>→</span>
                          <span className="wf-diff-chg-b">{String(ch.b)}</span>
                        </div>
                      ))}
                    </div>
                  )}
                  {c.kind !== "changed" && (
                    <div className="wf-diff-item-sub">{c.node.sub}</div>
                  )}
                </div>
              </button>
            ))}
          </>
        )}

        {edgeChanges.length > 0 && (
          <>
            <div className="wf-diff-section-label" style={{ marginTop: 12 }}>连线 · {edgeChanges.length}</div>
            {edgeChanges.map((c, i) => (
              <div key={i} className={"wf-diff-item wf-diff-item-" + c.kind}>
                <span className={"wf-diff-dot wf-diff-dot-" + c.kind} />
                <div style={{ flex: 1 }}>
                  <div className="wf-diff-item-title cell-mono">{c.edge.from} → {c.edge.to}</div>
                </div>
              </div>
            ))}
          </>
        )}

        {totalChanges === 0 && (
          <div style={{ padding: 24, color: "var(--fg-faint)", textAlign: "center" }}>两个版本完全一致</div>
        )}

        <div className="wf-diff-legend">
          <span><span className="wf-diff-dot wf-diff-dot-added" /> 新增</span>
          <span><span className="wf-diff-dot wf-diff-dot-removed" /> 删除</span>
          <span><span className="wf-diff-dot wf-diff-dot-changed" /> 修改</span>
        </div>
      </aside>

      <div className="wf-diff-canvas">
        <svg className="wf-edges" style={{ overflow: "visible" }}>
          <defs>
            <marker id="wf-arr-d" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
              <path d="M0 0 L10 5 L0 10 z" fill="var(--border-strong)" />
            </marker>
          </defs>
          {allEdges.map((e, i) => {
            const a = byIdRender.get(e.from), b = byIdRender.get(e.to);
            if (!a || !b) return null;
            const color = e._diff === "added" ? "var(--status-success)"
                         : e._diff === "removed" ? "var(--status-error)"
                         : "var(--border-strong)";
            return <path
              key={i}
              d={edgePathBetween(a, b)}
              fill="none" stroke={color}
              strokeWidth={e._diff ? 2 : 1.4}
              strokeDasharray={e._diff === "removed" ? "5 4" : ""}
              markerEnd="url(#wf-arr-d)"
              opacity={e._diff ? 0.95 : 0.5}
            />;
          })}
        </svg>
        {renderNodes.map(n => (
          <CanvasNode key={n.id} node={n} diffMark={n._diff} selected={selectedNodeId === n.id}
                      onMouseDown={(e) => { e.stopPropagation(); setSelectedNodeId(n.id); }} />
        ))}
      </div>
    </div>
  );
}

// ── Auto layout: topological BFS, layer placement ──────────────────────
function autoLayout(nodes, edges, direction = "vertical") {
  const incoming = Object.fromEntries(nodes.map(n => [n.id, 0]));
  edges.forEach(e => { if (incoming[e.to] != null) incoming[e.to]++; });
  const layer = {};
  const queue = nodes.filter(n => incoming[n.id] === 0).map(n => n.id);
  queue.forEach(id => { layer[id] = 0; });
  let head = 0;
  while (head < queue.length) {
    const id = queue[head++];
    edges.filter(e => e.from === id).forEach(e => {
      const newL = (layer[id] || 0) + 1;
      if (layer[e.to] == null || layer[e.to] < newL) layer[e.to] = newL;
      if (!queue.includes(e.to)) queue.push(e.to);
    });
  }
  nodes.forEach(n => { if (layer[n.id] == null) layer[n.id] = 0; });
  const byLayer = {};
  nodes.forEach(n => {
    const L = layer[n.id];
    if (!byLayer[L]) byLayer[L] = [];
    byLayer[L].push(n.id);
  });
  const xGap = 240, yGap = 140;
  const result = {};
  Object.keys(byLayer).map(Number).sort((a, b) => a - b).forEach(L => {
    const ids = byLayer[L];
    ids.forEach((id, i) => {
      const offset = (i - (ids.length - 1) / 2) * yGap;
      if (direction === "vertical") result[id] = { x: 200 + offset, y: 60 + L * yGap };
      else                          result[id] = { x: 80 + L * xGap, y: 200 + offset };
    });
  });
  const defaultHandles = direction === "vertical"
    ? { from: "bottom", to: "top" }
    : { from: "right",  to: "left" };
  return { positions: result, defaultHandles };
}

// ─────────────────────── FULL EDITABLE CANVAS (current version) ──────────────────
function WorkflowEditor({ version, onChange }) {
  const [nodes, setNodes] = useWfState(() => version.nodes.map(n => ({ ...n })));
  const [edges, setEdges] = useWfState(() => version.edges.map(e => ({ from: e.from, to: e.to, fromHandle: "bottom", toHandle: "top" })));
  const [selected, setSelected] = useWfState(null);
  const [transform, setTransform] = useWfState({ x: 0, y: 0, scale: 1 });
  const [dragNodeId, setDragNodeId] = useWfState(null);
  const [dragOffset, setDragOffset] = useWfState({ x: 0, y: 0 });
  const [connecting, setConnecting] = useWfState(null);
  const [panning, setPanning] = useWfState(false);
  const canvasRef = useWfRef(null);
  const panStart = useWfRef(null);

  const byId = useWfMemo(() => Object.fromEntries(nodes.map(n => [n.id, n])), [nodes]);

  const clientToCanvas = (cx, cy) => {
    const r = canvasRef.current.getBoundingClientRect();
    return { x: (cx - r.left - transform.x) / transform.scale, y: (cy - r.top - transform.y) / transform.scale };
  };

  const onNodeMouseDown = (e, id) => {
    if (e.target.dataset.handle) return;
    e.stopPropagation();
    const c = clientToCanvas(e.clientX, e.clientY);
    const node = byId[id];
    setDragNodeId(id);
    setDragOffset({ x: c.x - node.x, y: c.y - node.y });
    setSelected(id);
  };
  const onHandleMouseDown = (e, id, handle) => {
    e.stopPropagation();
    setConnecting({ id, handle, x: e.clientX, y: e.clientY });
  };
  useWfEffect(() => {
    if (!dragNodeId && !connecting && !panning) return;
    const onMove = (e) => {
      if (dragNodeId) {
        const c = clientToCanvas(e.clientX, e.clientY);
        setNodes(ns => ns.map(n => n.id === dragNodeId ? { ...n, x: c.x - dragOffset.x, y: c.y - dragOffset.y } : n));
      } else if (connecting) {
        setConnecting(c => c && { ...c, x: e.clientX, y: e.clientY });
      } else if (panning) {
        const dx = e.clientX - panStart.current.x;
        const dy = e.clientY - panStart.current.y;
        setTransform(t => ({ ...t, x: panStart.current.tx + dx, y: panStart.current.ty + dy }));
      }
    };
    const onUp = (e) => {
      if (connecting) {
        const t = document.elementFromPoint(e.clientX, e.clientY)?.closest("[data-handle][data-id]");
        if (t) {
          const toId = t.dataset.id;
          const toHandle = t.dataset.handle;
          if (toId !== connecting.id) {
            setEdges(es => es.find(x => x.from === connecting.id && x.to === toId) ? es : [...es, { from: connecting.id, to: toId, fromHandle: connecting.handle, toHandle }]);
            onChange?.();
          }
        }
        setConnecting(null);
      }
      setDragNodeId(null);
      setPanning(false);
      panStart.current = null;
    };
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
    return () => { window.removeEventListener("mousemove", onMove); window.removeEventListener("mouseup", onUp); };
  }, [dragNodeId, dragOffset, connecting, panning, transform.scale]);

  const onCanvasMouseDown = (e) => {
    if (e.target.classList.contains("wf-canvas-inner") || e.target.classList.contains("wf-canvas")) {
      panStart.current = { x: e.clientX, y: e.clientY, tx: transform.x, ty: transform.y };
      setPanning(true);
      setSelected(null);
    }
  };
  const onWheel = (e) => {
    e.preventDefault();
    const r = canvasRef.current.getBoundingClientRect();
    const mx = e.clientX - r.left, my = e.clientY - r.top;
    setTransform(t => {
      const scale = Math.max(0.25, Math.min(2.5, t.scale * (1 - e.deltaY * 0.0015)));
      const ratio = scale / t.scale;
      return { x: mx - (mx - t.x) * ratio, y: my - (my - t.y) * ratio, scale };
    });
  };
  const onPaletteAdd = (kind) => {
    const id = "n_" + Math.random().toString(36).slice(2, 7);
    setNodes(ns => [...ns, { id, kind, label: kind, sub: "", x: 320, y: 220 }]);
    setSelected(id);
    onChange?.();
  };
  const onCanvasDrop = (e) => {
    e.preventDefault();
    const kind = e.dataTransfer.getData("kind");
    if (!kind) return;
    const c = clientToCanvas(e.clientX, e.clientY);
    const id = "n_" + Math.random().toString(36).slice(2, 7);
    setNodes(ns => [...ns, { id, kind, label: kind, sub: "", x: c.x - WF_NODE_W/2, y: c.y - WF_NODE_H/2 }]);
    setSelected(id);
    onChange?.();
  };
  const resetView = () => setTransform({ x: 0, y: 0, scale: 1 });
  const zoomBy = (factor) => {
    const r = canvasRef.current.getBoundingClientRect();
    const mx = r.width / 2, my = r.height / 2;
    setTransform(t => {
      const scale = Math.max(0.25, Math.min(2.5, t.scale * factor));
      const ratio = scale / t.scale;
      return { x: mx - (mx - t.x) * ratio, y: my - (my - t.y) * ratio, scale };
    });
  };
  const fitToContent = () => {
    if (!canvasRef.current || nodes.length === 0) return resetView();
    const minX = Math.min(...nodes.map(n => n.x));
    const minY = Math.min(...nodes.map(n => n.y));
    const maxX = Math.max(...nodes.map(n => n.x + WF_NODE_W));
    const maxY = Math.max(...nodes.map(n => n.y + WF_NODE_H));
    const r = canvasRef.current.getBoundingClientRect();
    const pad = 60;
    const sx = (r.width - pad * 2) / Math.max(1, maxX - minX);
    const sy = (r.height - pad * 2) / Math.max(1, maxY - minY);
    const scale = Math.max(0.35, Math.min(1.2, Math.min(sx, sy)));
    const x = -minX * scale + (r.width - (maxX - minX) * scale) / 2;
    const y = -minY * scale + (r.height - (maxY - minY) * scale) / 2;
    setTransform({ x, y, scale });
  };
  const doAutoLayout = (direction) => {
    const { positions, defaultHandles } = autoLayout(nodes, edges, direction);
    setNodes(ns => ns.map(n => positions[n.id] ? { ...n, ...positions[n.id] } : n));
    setEdges(es => es.map(e => ({ ...e, fromHandle: defaultHandles.from, toHandle: defaultHandles.to })));
    onChange?.();
    setTimeout(fitToContent, 50);
  };

  // Fit-to-content on mount + whenever canvas size changes
  useWfEffect(() => {
    if (!canvasRef.current) return;
    const t = setTimeout(fitToContent, 60);
    const ro = new ResizeObserver(() => fitToContent());
    ro.observe(canvasRef.current);
    return () => { clearTimeout(t); ro.disconnect(); };
    // eslint-disable-next-line
  }, []);

  return (
    <div className="wf-editor">
      <Palette onAdd={onPaletteAdd} />
      <div
        ref={canvasRef}
        className={"wf-canvas" + (panning ? " is-panning" : "")}
        onDragOver={e => e.preventDefault()}
        onDrop={onCanvasDrop}
        onMouseDown={onCanvasMouseDown}
        onWheel={onWheel}
      >
        <div className="wf-canvas-inner" style={{ transform: `translate(${transform.x}px, ${transform.y}px) scale(${transform.scale})` }}>
          <svg className="wf-edges" style={{ overflow: "visible" }}>
            <defs>
              <marker id="wf-arr" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
                <path d="M0 0 L10 5 L0 10 z" fill="var(--border-strong)" />
              </marker>
            </defs>
            {edges.map((e, i) => {
              const a = byId[e.from], b = byId[e.to];
              if (!a || !b) return null;
              return <path key={i} d={edgePathBetween(a, b, e.fromHandle, e.toHandle)}
                           fill="none" stroke={selected === e.from || selected === e.to ? "var(--accent)" : "var(--border-strong)"}
                           strokeWidth={1.4} markerEnd="url(#wf-arr)" />;
            })}
            {connecting && (() => {
              const a = byId[connecting.id];
              if (!a) return null;
              const h = HANDLE_OFFSET[connecting.handle];
              const sx = a.x + h.x, sy = a.y + h.y;
              const c = clientToCanvas(connecting.x, connecting.y);
              return <path d={`M ${sx} ${sy} L ${c.x} ${c.y}`} stroke="var(--accent)" strokeWidth="1.6" strokeDasharray="5 4" fill="none" />;
            })()}
          </svg>
          {nodes.map(n => (
            <CanvasNode key={n.id} node={n} selected={selected === n.id}
                        onMouseDown={(e) => onNodeMouseDown(e, n.id)}
                        onHandleMouseDown={onHandleMouseDown}
                        connectingFrom={connecting} />
          ))}
        </div>
        <div className="wf-canvas-toolbar">
          <button className="icon-btn" title="自动垂直排列" onClick={() => doAutoLayout("vertical")}>
            <Icon.Layers style={{ transform: "rotate(90deg)" }} />
          </button>
          <button className="icon-btn" title="自动水平排列" onClick={() => doAutoLayout("horizontal")}>
            <Icon.Layers />
          </button>
          <div className="wf-toolbar-sep" />
          <button className="icon-btn" title="放大" onClick={() => zoomBy(1.2)}><Icon.Plus /></button>
          <button className="icon-btn" title="缩小" onClick={() => zoomBy(1/1.2)}>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round"><path d="M5 12h14"/></svg>
          </button>
          <button className="icon-btn" title="适配画面" onClick={fitToContent}><Icon.Filter /></button>
          <button className="icon-btn" title="复位" onClick={resetView}><Icon.Refresh /></button>
          <div className="wf-zoom">{Math.round(transform.scale * 100)}%</div>
        </div>
      </div>
    </div>
  );
}

// ─────────────────────── DETAIL PAGE ───────────────────────
function WorkflowView({ forge, onBack }) {
  const detail = Forgify.workflowDetails[forge?.id || "wf_weekly_training"] || Forgify.workflowDetails.wf_weekly_training;
  const versions = detail.versions;
  const currentV = versions.find(v => v.state === "current") || versions[0];
  const pendingV = versions.find(v => v.state === "pending");
  const deployedV = versions.find(v => v.state === "deployed");
  const [selectedId, setSelectedId] = useWfState(pendingV ? pendingV.id : currentV.id);
  const [dirty, setDirty] = useWfState(false);

  const selectedV = versions.find(v => v.id === selectedId) || currentV;
  const isViewingCurrent = selectedV.id === currentV.id;

  React.useEffect(() => {
    if (!dirty) return;
    const t = setTimeout(() => setDirty(false), 1500);
    return () => clearTimeout(t);
  }, [dirty]);

  return (
    <div className="page">
      <div className="page-header" style={{ paddingTop: 18 }}>
        <div className="page-header-text" style={{ gap: 6 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 12, color: "var(--fg-muted)" }}>
            {onBack && <button onClick={onBack} className="btn btn-xs btn-ghost">← 返回</button>}
            <KindChip kind="workflow" />
            <span className="cell-mono" style={{ color: "var(--fg-faint)" }}>{forge?.id || "wf_weekly_training"}</span>
          </div>
          <div className="page-title" style={{ fontFamily: "var(--font-mono)" }}>
            {forge?.name || "weekly-training-summary"}
          </div>
          <div className="page-subtitle" style={{ display: "flex", alignItems: "center", gap: 4, flexWrap: "wrap" }}>
            <span>{forge?.desc || currentV.description}</span>
            <EntityRelMeta entityId={forge?.id || "wf_weekly_training"} />
          </div>
        </div>
        <div className="page-actions">
          {isViewingCurrent && (
            <span className={"wf-saved" + (dirty ? " is-dirty" : "")}>
              <span className="dot" />{dirty ? "未保存" : "已保存"}
            </span>
          )}
          <button className="btn btn-sm"><Icon.Eye /> Capability check</button>
          <button className="btn btn-sm"><Icon.Play /> 试跑</button>
          <AskAiTrigger context={"Workflow · " + (forge?.name || currentV.label)} suggestions={["在 Notion 写入前加重试", "把 trigger 改成每天 8 点"]} />
        </div>
      </div>

      <div className="vr-shell">
        <div className="vr-main" style={{ padding: 0, display: "flex", flexDirection: "column", overflow: "hidden" }}>
          {isViewingCurrent
            ? <WorkflowEditor version={currentV} onChange={() => setDirty(true)} />
            : <WorkflowDiffView currentV={currentV} otherV={selectedV} pendingV={pendingV} />}
        </div>

        <VersionRail
          versions={versions}
          currentId={currentV.id}
          pendingId={pendingV?.id || null}
          deployedId={deployedV?.id || currentV.id}
          showDeploy={true}
          selectedId={selectedId}
          onSelect={setSelectedId}
          onAccept={() => window.Shell?.toast({ kind: "success", title: "已 Accept", desc: pendingV?.label, undo: () => {} })}
          onRevert={() => window.Shell?.toast({ kind: "warn", title: "已 Revert pending", desc: pendingV?.label, undo: () => {} })}
          onDeploy={() => window.Shell?.toast({ kind: "success", title: "已部署 " + currentV.label + " 到生产", desc: "调度器已切到新版本", undo: () => {} })}
        />
      </div>
    </div>
  );
}

window.WorkflowView = WorkflowView;

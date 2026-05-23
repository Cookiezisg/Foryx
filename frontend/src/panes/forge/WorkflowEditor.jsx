// WorkflowEditor — editable DAG canvas for the current workflow version.
//
//   - Left palette (13 node kinds) with drag-to-canvas + click-to-add
//   - Pan / zoom / fit-to-content / auto-layout (vertical | horizontal)
//   - Drag nodes; 4-handle (top/right/bottom/left) connect-to-create-edge
//   - Right inspector for the selected node (label, config json, retry/timeout)
//   - 2s debounced autosave: diff vs original → ops → POST /workflows/{id}:edit
//     (creates/iterates a pending version that user can later Accept)
//
// WorkflowEditor —— 当前版本的可编辑 DAG。13 种节点 palette，拖入/点入；4 个
// 连接 handle；自动布局；2s 防抖 autosave 经 :edit 产 pending 版本，等用户
// Accept 才落到 active。

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Icon } from "../../components/primitives/Icon.jsx";
import { Button } from "../../components/primitives/Button.jsx";
import { useEditWorkflow } from "../../api/forge.js";
import { useUIStore } from "../../store/ui.js";

const NODE_W = 184;
const NODE_H = 76;

const NODE_KINDS = [
  { kind: "trigger",   label: "Trigger",   icon: "Zap",      desc: "Cron / Webhook / Manual" },
  { kind: "function",  label: "Function",  icon: "Code",     desc: "纯函数 · 沙箱执行" },
  { kind: "handler",   label: "Handler",   icon: "Server",   desc: "Stateful 类调用" },
  { kind: "mcp",       label: "MCP Tool",  icon: "Server",   desc: "调 MCP server" },
  { kind: "skill",     label: "Skill",     icon: "Sparkles", desc: "SKILL.md 模板" },
  { kind: "llm",       label: "LLM",       icon: "Brain",    desc: "纯 LLM 节点" },
  { kind: "agent",     label: "Agent",     icon: "Bot",      desc: "Sub-agent 调用" },
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
  top:    { x: NODE_W / 2,  y: 0,         dx: 0,  dy: -1 },
  right:  { x: NODE_W,      y: NODE_H / 2, dx: 1,  dy: 0  },
  bottom: { x: NODE_W / 2,  y: NODE_H,     dx: 0,  dy: 1  },
  left:   { x: 0,           y: NODE_H / 2, dx: -1, dy: 0  },
};

function edgePathBetween(a, b, fromH = "bottom", toH = "top") {
  const f = HANDLE_OFFSET[fromH] || HANDLE_OFFSET.bottom;
  const t = HANDLE_OFFSET[toH]   || HANDLE_OFFSET.top;
  const sx = a.x + f.x, sy = a.y + f.y;
  const ex = b.x + t.x, ey = b.y + t.y;
  const dist = Math.max(40, Math.hypot(ex - sx, ey - sy) * 0.4);
  return `M ${sx} ${sy} C ${sx + f.dx * dist} ${sy + f.dy * dist}, ${ex + t.dx * dist} ${ey + t.dy * dist}, ${ex} ${ey}`;
}

function iconFor(kind) {
  const I = {
    trigger: Icon.Zap, function: Icon.Code, handler: Icon.Server, mcp: Icon.Server,
    skill: Icon.Sparkles, llm: Icon.Brain, agent: Icon.Bot, http: Icon.Globe,
    condition: Icon.GitBranch, loop: Icon.Refresh, parallel: Icon.Layers,
    approval: Icon.Pause, wait: Icon.Clock, variable: Icon.Database,
  }[kind] || Icon.Code;
  return <I />;
}

function newNodeId() { return "n_" + Math.random().toString(36).slice(2, 8); }
function newEdgeId() { return "e_" + Math.random().toString(36).slice(2, 8); }

// ── Diff helper: convert {nodes, edges} delta to backend Op[] ────────────
function diffToOps(orig, next) {
  const ops = [];
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

function edgeKey(e) { return `${e.from}|${e.fromPort || ""}->${e.to}|${e.toPort || ""}`; }
function nodeChanged(a, b) {
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
function nodeToSpec(n) {
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
function edgeToSpec(e) {
  return {
    id: e.id || newEdgeId(),
    from: e.from,
    to: e.to,
    ...(e.fromPort ? { fromPort: e.fromPort } : {}),
    ...(e.toPort ? { toPort: e.toPort } : {}),
  };
}

// ── Auto-layout: topological BFS, layer placement ────────────────────────
function autoLayout(nodes, edges, direction = "vertical") {
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
  nodes.forEach((n) => { if (layer[n.id] == null) layer[n.id] = 0; });
  const byLayer = {};
  nodes.forEach((n) => {
    const L = layer[n.id];
    if (!byLayer[L]) byLayer[L] = [];
    byLayer[L].push(n.id);
  });
  const xGap = 240, yGap = 140;
  const result = {};
  Object.keys(byLayer).map(Number).sort((a, b) => a - b).forEach((L) => {
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

// ── Palette ──────────────────────────────────────────────────────────────
function Palette({ onAdd }) {
  const [q, setQ] = useState("");
  const list = NODE_KINDS.filter((k) =>
    (k.label + k.desc).toLowerCase().includes(q.toLowerCase()));
  return (
    <aside className="wf-palette">
      <div className="search-input" style={{ maxWidth: "none" }}>
        <Icon.Search className="icon" />
        <input placeholder="拖入节点…" value={q} onChange={(e) => setQ(e.target.value)} />
      </div>
      <div className="wf-palette-label">节点 · {list.length}</div>
      <div className="wf-palette-list">
        {list.map((k) => {
          const Ic = Icon[k.icon] || Icon.Code;
          return (
            <button
              key={k.kind}
              className="wf-palette-item"
              draggable
              onDragStart={(e) => e.dataTransfer.setData("kind", k.kind)}
              onClick={() => onAdd(k.kind)}
              title={k.desc}
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

// ── CanvasNode ───────────────────────────────────────────────────────────
function CanvasNode({ node, selected, onMouseDown, onHandleMouseDown, connectingFrom }) {
  return (
    <div
      className={"wf-node" + (selected ? " is-selected" : "")}
      style={{ left: node.x, top: node.y, width: NODE_W, position: "absolute" }}
      onMouseDown={onMouseDown}
      data-id={node.id}
    >
      {HANDLES.map((h) => (
        <div
          key={h}
          className={"wf-node-handle " + h +
            (connectingFrom?.id === node.id && connectingFrom?.handle === h ? " is-active" : "")}
          data-handle={h}
          data-id={node.id}
          onMouseDown={(e) => onHandleMouseDown(e, node.id, h)}
        />
      ))}
      <div className="wf-node-head">
        <div className={"wf-node-icon kind-" + node.kind}>{iconFor(node.kind)}</div>
        <div className="wf-node-title">{node.label || node.id}</div>
      </div>
      <div className="wf-node-sub">{node.sub || node.config?.ref || node.notes || ""}</div>
    </div>
  );
}

// ── Inspector ────────────────────────────────────────────────────────────
function Inspector({ node, onChange, onDelete }) {
  if (!node) {
    return (
      <aside className="wf-inspector">
        <div className="empty" style={{ padding: 18 }}>
          <Icon.Filter className="icon" />
          <div className="title">点节点配置</div>
          <div className="sub">label · config · 重试</div>
        </div>
      </aside>
    );
  }
  const [text, setText] = useState(JSON.stringify(node.config || {}, null, 2));
  useEffect(() => setText(JSON.stringify(node.config || {}, null, 2)), [node.id]);

  const commitJson = () => {
    try {
      const obj = text.trim() ? JSON.parse(text) : {};
      onChange({ config: obj });
    } catch {
      // ignore — user typing
    }
  };

  return (
    <aside className="wf-inspector">
      <div className="wf-inspector-head">
        <div className="wf-inspector-title">
          {iconFor(node.kind)} <span>{node.kind}</span>
        </div>
        <button className="icon-btn" onClick={onDelete} title="删除节点"><Icon.Trash /></button>
      </div>
      <div className="wf-inspector-body">
        <label className="drawer-label">ID</label>
        <input
          className="cfg-input" readOnly value={node.id}
          style={{ fontFamily: "var(--font-mono)", fontSize: 11 }}
        />

        <label className="drawer-label" style={{ marginTop: 10 }}>Label</label>
        <input
          className="cfg-input"
          value={node.label || ""}
          onChange={(e) => onChange({ label: e.target.value })}
          placeholder="节点显示名"
        />

        <label className="drawer-label" style={{ marginTop: 10 }}>备注</label>
        <input
          className="cfg-input"
          value={node.notes || ""}
          onChange={(e) => onChange({ notes: e.target.value })}
          placeholder="可选 — 在节点下面显示"
        />

        <label className="drawer-label" style={{ marginTop: 10 }}>超时 (秒) · onError</label>
        <div style={{ display: "flex", gap: 6 }}>
          <input
            type="number" min={0}
            className="cfg-input" style={{ width: 100 }}
            value={node.timeout || 0}
            onChange={(e) => onChange({ timeout: parseInt(e.target.value) || 0 })}
          />
          <select
            className="cfg-input" style={{ flex: 1 }}
            value={node.onError || ""}
            onChange={(e) => onChange({ onError: e.target.value })}
          >
            <option value="">默认（fail）</option>
            <option value="fail">fail</option>
            <option value="skip">skip</option>
            <option value="continue">continue</option>
          </select>
        </div>

        <label className="drawer-label" style={{ marginTop: 10 }}>Config (JSON)</label>
        <textarea
          className="run-drawer-input"
          rows={10}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onBlur={commitJson}
          spellCheck={false}
        />
      </div>
    </aside>
  );
}

// ── Main editor ──────────────────────────────────────────────────────────
export function WorkflowEditor({ workflowId, version }) {
  const original = useMemo(() => parseGraph(version), [version?.id]);
  const [nodes, setNodes] = useState(original.nodes);
  const [edges, setEdges] = useState(original.edges);
  const [selected, setSelected] = useState(null);
  const [transform, setTransform] = useState({ x: 0, y: 0, scale: 1 });
  const [dragNodeId, setDragNodeId] = useState(null);
  const [dragOffset, setDragOffset] = useState({ x: 0, y: 0 });
  const [connecting, setConnecting] = useState(null);
  const [panning, setPanning] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [savedAt, setSavedAt] = useState(null);
  const canvasRef = useRef(null);
  const panStart = useRef(null);
  const saveTimer = useRef(null);

  const edit = useEditWorkflow(workflowId);
  const pushToast = useUIStore((s) => s.pushToast);

  // Reset when version changes externally.
  useEffect(() => {
    setNodes(original.nodes);
    setEdges(original.edges);
    setDirty(false);
    setSelected(null);
  }, [version?.id]);

  const byId = useMemo(() => Object.fromEntries(nodes.map((n) => [n.id, n])), [nodes]);

  const clientToCanvas = useCallback((cx, cy) => {
    const r = canvasRef.current.getBoundingClientRect();
    return { x: (cx - r.left - transform.x) / transform.scale, y: (cy - r.top - transform.y) / transform.scale };
  }, [transform]);

  // Debounced autosave: every change → 2s timer → diff → POST :edit.
  const markDirty = useCallback(() => {
    setDirty(true);
    if (saveTimer.current) clearTimeout(saveTimer.current);
    saveTimer.current = setTimeout(() => {
      const ops = diffToOps(original, { nodes, edges });
      if (ops.length === 0) { setDirty(false); return; }
      edit.mutate(
        { ops, changeReason: "editor autosave" },
        {
          onSuccess: () => { setDirty(false); setSavedAt(new Date()); },
          onError: (e) => pushToast({ kind: "error", title: "保存失败", desc: e.message }),
        }
      );
    }, 2000);
  }, [nodes, edges, original, edit, pushToast]);

  useEffect(() => () => clearTimeout(saveTimer.current), []);

  // ── Node mouse interactions ────────────────────────────────────────
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
  useEffect(() => {
    if (!dragNodeId && !connecting && !panning) return;
    const onMove = (e) => {
      if (dragNodeId) {
        const c = clientToCanvas(e.clientX, e.clientY);
        setNodes((ns) => ns.map((n) =>
          n.id === dragNodeId ? { ...n, x: c.x - dragOffset.x, y: c.y - dragOffset.y } : n));
      } else if (connecting) {
        setConnecting((c) => c && { ...c, x: e.clientX, y: e.clientY });
      } else if (panning) {
        const dx = e.clientX - panStart.current.x;
        const dy = e.clientY - panStart.current.y;
        setTransform((t) => ({ ...t, x: panStart.current.tx + dx, y: panStart.current.ty + dy }));
      }
    };
    const onUp = (e) => {
      if (connecting) {
        const t = document.elementFromPoint(e.clientX, e.clientY)?.closest("[data-handle][data-id]");
        if (t) {
          const toId = t.dataset.id;
          const toHandle = t.dataset.handle;
          if (toId !== connecting.id) {
            setEdges((es) => es.find((x) => x.from === connecting.id && x.to === toId)
              ? es
              : [...es, { id: newEdgeId(), from: connecting.id, to: toId, fromHandle: connecting.handle, toHandle }]);
            markDirty();
          }
        }
        setConnecting(null);
      }
      if (dragNodeId) markDirty();
      setDragNodeId(null);
      setPanning(false);
      panStart.current = null;
    };
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
    return () => { window.removeEventListener("mousemove", onMove); window.removeEventListener("mouseup", onUp); };
  }, [dragNodeId, dragOffset, connecting, panning, transform.scale, clientToCanvas, markDirty]);

  // ── Canvas-level pan + zoom ────────────────────────────────────────
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
    setTransform((t) => {
      const scale = Math.max(0.25, Math.min(2.5, t.scale * (1 - e.deltaY * 0.0015)));
      const ratio = scale / t.scale;
      return { x: mx - (mx - t.x) * ratio, y: my - (my - t.y) * ratio, scale };
    });
  };
  const onPaletteAdd = (kind) => {
    const id = newNodeId();
    setNodes((ns) => [...ns, { id, kind, label: kind, x: 320, y: 220, config: {}, notes: "" }]);
    setSelected(id);
    markDirty();
  };
  const onCanvasDrop = (e) => {
    e.preventDefault();
    const kind = e.dataTransfer.getData("kind");
    if (!kind) return;
    const c = clientToCanvas(e.clientX, e.clientY);
    const id = newNodeId();
    setNodes((ns) => [...ns, {
      id, kind, label: kind,
      x: c.x - NODE_W / 2, y: c.y - NODE_H / 2,
      config: {}, notes: "",
    }]);
    setSelected(id);
    markDirty();
  };

  const resetView = () => setTransform({ x: 0, y: 0, scale: 1 });
  const zoomBy = (factor) => {
    const r = canvasRef.current.getBoundingClientRect();
    const mx = r.width / 2, my = r.height / 2;
    setTransform((t) => {
      const scale = Math.max(0.25, Math.min(2.5, t.scale * factor));
      const ratio = scale / t.scale;
      return { x: mx - (mx - t.x) * ratio, y: my - (my - t.y) * ratio, scale };
    });
  };
  const fitToContent = useCallback(() => {
    if (!canvasRef.current || nodes.length === 0) return resetView();
    const minX = Math.min(...nodes.map((n) => n.x));
    const minY = Math.min(...nodes.map((n) => n.y));
    const maxX = Math.max(...nodes.map((n) => n.x + NODE_W));
    const maxY = Math.max(...nodes.map((n) => n.y + NODE_H));
    const r = canvasRef.current.getBoundingClientRect();
    const pad = 60;
    const sx = (r.width - pad * 2) / Math.max(1, maxX - minX);
    const sy = (r.height - pad * 2) / Math.max(1, maxY - minY);
    const scale = Math.max(0.35, Math.min(1.2, Math.min(sx, sy)));
    const x = -minX * scale + (r.width - (maxX - minX) * scale) / 2;
    const y = -minY * scale + (r.height - (maxY - minY) * scale) / 2;
    setTransform({ x, y, scale });
  }, [nodes]);
  const doAutoLayout = (direction) => {
    const { positions, defaultHandles } = autoLayout(nodes, edges, direction);
    setNodes((ns) => ns.map((n) => positions[n.id] ? { ...n, ...positions[n.id] } : n));
    setEdges((es) => es.map((e) => ({ ...e, fromHandle: defaultHandles.from, toHandle: defaultHandles.to })));
    markDirty();
    setTimeout(fitToContent, 50);
  };

  useEffect(() => {
    if (!canvasRef.current) return;
    const t = setTimeout(fitToContent, 60);
    const ro = new ResizeObserver(() => fitToContent());
    ro.observe(canvasRef.current);
    return () => { clearTimeout(t); ro.disconnect(); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const onNodePatch = (patch) => {
    setNodes((ns) => ns.map((n) => n.id === selected ? { ...n, ...patch } : n));
    markDirty();
  };
  const onNodeDelete = () => {
    setNodes((ns) => ns.filter((n) => n.id !== selected));
    setEdges((es) => es.filter((e) => e.from !== selected && e.to !== selected));
    setSelected(null);
    markDirty();
  };

  const selectedNode = nodes.find((n) => n.id === selected);
  const status =
    edit.isPending ? "saving"
    : dirty        ? "dirty"
    : savedAt      ? "saved"
                   : "clean";

  return (
    <div className="wf-editor">
      <Palette onAdd={onPaletteAdd} />
      <div
        ref={canvasRef}
        className={"wf-canvas" + (panning ? " is-panning" : "")}
        onDragOver={(e) => e.preventDefault()}
        onDrop={onCanvasDrop}
        onMouseDown={onCanvasMouseDown}
        onWheel={onWheel}
      >
        <div className="wf-canvas-inner"
             style={{ transform: `translate(${transform.x}px, ${transform.y}px) scale(${transform.scale})`, transformOrigin: "0 0" }}>
          <svg className="wf-edges" style={{ overflow: "visible" }}>
            <defs>
              <marker id="wf-arr-ed" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
                <path d="M0 0 L10 5 L0 10 z" fill="var(--border-strong)" />
              </marker>
            </defs>
            {edges.map((e, i) => {
              const a = byId[e.from], b = byId[e.to];
              if (!a || !b) return null;
              return <path key={e.id || i} d={edgePathBetween(a, b, e.fromHandle, e.toHandle)}
                           fill="none"
                           stroke={selected === e.from || selected === e.to ? "var(--accent)" : "var(--border-strong)"}
                           strokeWidth={1.4} markerEnd="url(#wf-arr-ed)" />;
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
          {nodes.map((n) => (
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
        <div className={"wf-saved is-" + status}>
          {status === "saving" && <><span className="spinner" /> 保存中…</>}
          {status === "dirty"  && <><span className="dot" /> 未保存（2s 后自动）</>}
          {status === "saved"  && <><span className="dot" /> 已保存 · Accept 后生效</>}
          {status === "clean"  && <><span className="dot" /> 已保存</>}
        </div>
      </div>
      <Inspector node={selectedNode} onChange={onNodePatch} onDelete={onNodeDelete} />
    </div>
  );
}

// ── normaliseGraph — pull {nodes, edges} from version.graph in canvas shape ─
function parseGraph(version) {
  if (!version) return { nodes: [], edges: [] };
  const g = version.graph || version;
  const inNodes = g.nodes || [];
  const inEdges = g.edges || [];

  let nodes = inNodes.map((n) => ({
    id: n.id,
    kind: n.type || n.kind || "function",
    label: n.label || n.id,
    notes: n.notes || "",
    config: n.config || {},
    onError: n.onError || "",
    timeout: n.timeout || 0,
    retry: n.retry,
    x: n.position?.x ?? n.x ?? 0,
    y: n.position?.y ?? n.y ?? 0,
  }));

  // Auto-layout if positions missing.
  if (nodes.length && nodes.every((n) => !n.x && !n.y)) {
    const { positions, defaultHandles } = autoLayout(nodes, inEdges, "vertical");
    nodes = nodes.map((n) => positions[n.id] ? { ...n, ...positions[n.id] } : n);
    const edges = inEdges.map((e) => ({
      id: e.id, from: e.from || e.fromId, to: e.to || e.toId,
      fromPort: e.fromPort, toPort: e.toPort,
      fromHandle: defaultHandles.from, toHandle: defaultHandles.to,
    }));
    return { nodes, edges };
  }
  const edges = inEdges.map((e) => ({
    id: e.id, from: e.from || e.fromId, to: e.to || e.toId,
    fromPort: e.fromPort, toPort: e.toPort,
    fromHandle: "bottom", toHandle: "top",
  }));
  return { nodes, edges };
}

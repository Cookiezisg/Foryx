/* Anselm 原语 G4 — <an-graph-canvas>（🪂 逃生舱：自绘 SVG + 吃 token）。workflow 编排图 + flowrun 运行态的统一画布。
   与后端逐字对齐（domain/workflow graph.go·ops.go + scheduler/walk.go）：可归约控制流图——前向边成 DAG、回边只能从 control/approval 出；节点 5 类（trigger/action/agent/control/approval）；执行按 (node,iteration) 展开循环（回边 +1）。
   分层布局（Sugiyama-lite）+ 浮动正交连线（锚到朝向对方那面，任意摆位都顺）。编辑动作生成后端 :edit ops。节点 chip 图标走 window.icon（NODE_ICON 单源）、色走类型 token。
   API：.graph={nodes,edges} · .run（运行态）· 属性 mode(edit|run)/dir(LR|TB)；方法 setMode/setDir/setRun/setGraph/getGraph/relayout/fit/zoomBy/select/addNode/deleteSelected/updateNode/updateEdge/resolveApproval/getNode/getEdge/getSelection/isBack。
   事件（composed）：an-graph-select{sel} · an-graph-change{ops,label} · an-graph-toast{msg} · an-graph-view{view}。 */
(function () {
  const SVGNS = "http://www.w3.org/2000/svg";
  const NW = 188, NH = 60, GAPX = 84, GAPY = 44, PAD = 48, STUB = 22, CORNER = 12, LOOP_GAP = 26;

  // 类型：色走 token（仅 chip / 细描边，正文克制）；ico = NODE_ICON 的图标 key（→ window.icon）。
  const KIND = {
    trigger:  { label: "触发", c: "var(--violet)", s: "var(--violet-soft)", prefix: "trg_", ico: "trigger" },
    action:   { label: "动作", c: "var(--accent)", s: "var(--accent-soft)", prefix: "fn_", ico: "action" },
    agent:    { label: "智能体", c: "var(--teal)", s: "var(--teal-soft)", prefix: "ag_", ico: "agent" },
    control:  { label: "分支", c: "var(--warn)", s: "var(--warn-soft)", prefix: "ctl_", ico: "control" },
    approval: { label: "审批", c: "var(--danger)", s: "var(--danger-soft)", prefix: "apf_", ico: "approval" },
  };
  const KIND_ORDER = ["trigger", "action", "agent", "control", "approval"];
  const STATE = {
    completed: { c: "var(--ink-3)", ring: "var(--line)", fill: "var(--island)", label: "已完成" },
    running:   { c: "var(--accent)", ring: "var(--accent-line)", fill: "var(--island)", label: "运行中" },
    failed:    { c: "var(--danger)", ring: "var(--danger)", fill: "var(--island)", label: "失败" },
    parked:    { c: "var(--warn)", ring: "var(--warn)", fill: "var(--island)", label: "待审批" },
    future:    { c: "var(--ink-3)", ring: "var(--line)", fill: "var(--island-2)", label: "未运行" },
    ready:     { c: "var(--ink-3)", ring: "var(--line)", fill: "var(--island)", label: "" },
  };

  let _seq = 0;
  const uid = (p) => p + "_" + (_seq++).toString(36) + Math.floor(performance.now()).toString(36).slice(-3);
  const clone = (g) => ({ nodes: g.nodes.map((n) => ({ ...n, input: { ...(n.input || {}) }, retry: n.retry ? { ...n.retry } : undefined })), edges: g.edges.map((e) => ({ ...e })) });
  const nodeById = (g, id) => g.nodes.find((n) => n.id === id);
  const iconInner = (key) => (window.icon ? window.icon(key).replace(/^<svg[^>]*>/, "").replace(/<\/svg>$/, "") : "");

  // ── 纯图算法（与后端同义）──
  function backEdges(g) {
    const out = {}; g.edges.forEach((e) => (out[e.from] = out[e.from] || []).push(e));
    const color = {}, back = new Set();
    g.nodes.forEach((n) => {
      if (color[n.id]) return; const st = [{ id: n.id, i: 0 }]; color[n.id] = 1;
      while (st.length) {
        const f = st[st.length - 1], es = out[f.id] || [];
        if (f.i >= es.length) { color[f.id] = 2; st.pop(); continue; }
        const e = es[f.i++], c = color[e.to] || 0;
        if (c === 1) back.add(e.id); else if (c === 0) { color[e.to] = 1; st.push({ id: e.to, i: 0 }); }
      }
    });
    return back;
  }
  function reachable(g, from, to) {
    const adj = {}; g.edges.forEach((e) => (adj[e.from] = adj[e.from] || []).push(e.to));
    const seen = new Set([from]), q = [from];
    while (q.length) { const u = q.shift(); for (const v of (adj[u] || [])) { if (v === to) return true; if (!seen.has(v)) { seen.add(v); q.push(v); } } }
    return false;
  }
  function validateEdge(g, from, to) {
    if (from === to) return { ok: false, reason: "不支持自环：节点不能连接自身" };
    if (g.edges.some((e) => e.from === from && e.to === to)) return { ok: false, reason: "该连线已存在" };
    const src = nodeById(g, from), isBack = reachable(g, to, from);
    if (isBack && src.kind !== "control" && src.kind !== "approval") return { ok: false, reason: "回边仅可从 control / approval 节点发出：循环须经分支决策闭合" };
    let port = "";
    if (src.kind === "control") { const used = g.edges.filter((e) => e.from === from && e.port).length; port = isBack ? "retry" : "branch" + (used + 1); }
    else if (src.kind === "approval") { const used = new Set(g.edges.filter((e) => e.from === from).map((e) => e.port)); port = !used.has("yes") ? "yes" : (!used.has("no") ? "no" : ""); if (!port) return { ok: false, reason: "approval 节点仅有 yes / no 两个出口" }; }
    return { ok: true, isBack, port };
  }
  // 后端 :edit ops 生成（线上发什么这里生成什么）
  const OPS = {
    addNode: (n) => ({ op: "add_node", node: nodeWire(n) }), updateNode: (id, patch) => ({ op: "update_node", id, patch }), deleteNode: (id) => ({ op: "delete_node", id }),
    addEdge: (e) => ({ op: "add_edge", edge: edgeWire(e) }), updateEdge: (id, patch) => ({ op: "update_edge", id, patch }), deleteEdge: (id) => ({ op: "delete_edge", id }),
  };
  function nodeWire(n) { const o = { id: n.id, kind: n.kind, ref: n.ref }; if (n.input && Object.keys(n.input).length) o.input = n.input; if (n.retry) o.retry = n.retry; if (n.pos) o.pos = n.pos; return o; }
  function edgeWire(e) { const o = { id: e.id, from: e.from, to: e.to }; if (e.port) o.fromPort = e.port; return o; }

  // ── 几何 / 路由（浮动正交边）──
  const el = (t, a) => { const e = document.createElementNS(SVGNS, t); for (const k in a) e.setAttribute(k, a[k]); return e; };
  const esc = (s) => String(s == null ? "" : s).replace(/[&<>]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;" }[c]));
  const center = (n) => ({ x: n.x + NW / 2, y: n.y + NH / 2 });
  const anchorPt = (n, side) => ({ top: { x: n.x + NW / 2, y: n.y }, bottom: { x: n.x + NW / 2, y: n.y + NH }, left: { x: n.x, y: n.y + NH / 2 }, right: { x: n.x + NW, y: n.y + NH / 2 } }[side]);
  const normal = (side) => ({ top: { x: 0, y: -1 }, bottom: { x: 0, y: 1 }, left: { x: -1, y: 0 }, right: { x: 1, y: 0 } }[side]);
  function facing(a, b) { const ac = center(a), bc = center(b), dx = bc.x - ac.x, dy = bc.y - ac.y, horiz = Math.abs(dx) * NH >= Math.abs(dy) * NW; return horiz ? [dx >= 0 ? "right" : "left", dx >= 0 ? "left" : "right"] : [dy >= 0 ? "bottom" : "top", dy >= 0 ? "top" : "bottom"]; }
  function roundedPath(raw, r) {
    const pts = []; raw.forEach((p) => { const l = pts[pts.length - 1]; if (!l || Math.abs(l.x - p.x) > 0.5 || Math.abs(l.y - p.y) > 0.5) pts.push(p); });
    if (pts.length < 2) return pts.length ? `M${pts[0].x},${pts[0].y}` : "";
    let d = `M${pts[0].x},${pts[0].y}`;
    for (let i = 1; i < pts.length - 1; i++) { const p = pts[i], a = pts[i - 1], b = pts[i + 1], v1 = { x: p.x - a.x, y: p.y - a.y }, v2 = { x: b.x - p.x, y: b.y - p.y }, l1 = Math.hypot(v1.x, v1.y) || 1, l2 = Math.hypot(v2.x, v2.y) || 1, rr = Math.min(r, l1 / 2, l2 / 2); d += ` L${p.x - v1.x / l1 * rr},${p.y - v1.y / l1 * rr} Q${p.x},${p.y} ${p.x + v2.x / l2 * rr},${p.y + v2.y / l2 * rr}`; }
    const last = pts[pts.length - 1]; return d + ` L${last.x},${last.y}`;
  }
  function pointAtMid(pts) { let total = 0; for (let i = 1; i < pts.length; i++) total += Math.hypot(pts[i].x - pts[i - 1].x, pts[i].y - pts[i - 1].y); let d = total / 2; for (let i = 1; i < pts.length; i++) { const seg = Math.hypot(pts[i].x - pts[i - 1].x, pts[i].y - pts[i - 1].y); if (d <= seg) { const t = seg ? d / seg : 0; return { x: pts[i - 1].x + (pts[i].x - pts[i - 1].x) * t, y: pts[i - 1].y + (pts[i].y - pts[i - 1].y) * t }; } d -= seg; } return pts[pts.length - 1]; }
  function orthoPath(a, b) {
    const [sa, sb] = facing(a, b), S = anchorPt(a, sa), T = anchorPt(b, sb), ns = normal(sa), nt = normal(sb);
    const S1 = { x: S.x + ns.x * STUB, y: S.y + ns.y * STUB }, T1 = { x: T.x + nt.x * STUB, y: T.y + nt.y * STUB }, sh = sa === "left" || sa === "right", th = sb === "left" || sb === "right";
    let pts;
    if (sh && th) { const mx = (S1.x + T1.x) / 2; pts = [S, S1, { x: mx, y: S1.y }, { x: mx, y: T1.y }, T1, T]; }
    else if (!sh && !th) { const my = (S1.y + T1.y) / 2; pts = [S, S1, { x: S1.x, y: my }, { x: T1.x, y: my }, T1, T]; }
    else { const corner = sh ? { x: T1.x, y: S1.y } : { x: S1.x, y: T1.y }; pts = [S, S1, corner, T1, T]; }
    return { pts, d: roundedPath(pts, CORNER), mid: pointAtMid(pts) };
  }
  function layout(G, dir) {
    const back = backEdges(G), fwd = G.edges.filter((e) => !back.has(e.id)), succ = {}, pred = {}, indeg = {};
    G.nodes.forEach((n) => { succ[n.id] = []; pred[n.id] = []; indeg[n.id] = 0; });
    fwd.forEach((e) => { if (succ[e.from]) { succ[e.from].push(e.to); pred[e.to].push(e.from); indeg[e.to]++; } });
    const rank = {}, q = G.nodes.filter((n) => indeg[n.id] === 0).map((n) => n.id); q.forEach((id) => (rank[id] = 0));
    const ind = { ...indeg };
    while (q.length) { const u = q.shift(); succ[u].forEach((v) => { rank[v] = Math.max(rank[v] ?? 0, (rank[u] ?? 0) + 1); if (--ind[v] === 0) q.push(v); }); }
    G.nodes.forEach((n) => { if (rank[n.id] == null) rank[n.id] = 0; });
    const maxR = Math.max(0, ...G.nodes.map((n) => rank[n.id])), layers = Array.from({ length: maxR + 1 }, () => []);
    G.nodes.forEach((n) => layers[rank[n.id]].push(n.id));
    const pos = {}; layers.forEach((L) => L.forEach((id, i) => (pos[id] = i)));
    const med = (id, adj) => { const ps = adj[id].map((x) => pos[x]).filter((v) => v != null); if (!ps.length) return pos[id]; ps.sort((a, b) => a - b); return ps[(ps.length - 1) >> 1]; };
    for (let p = 0; p < 8; p++) { const down = p % 2 === 0; (down ? [...layers.keys()] : [...layers.keys()].reverse()).forEach((li) => { const adj = down ? pred : succ; layers[li] = layers[li].map((id) => ({ id, m: med(id, adj) })).sort((a, b) => a.m - b.m).map((o) => o.id); layers[li].forEach((id, i) => (pos[id] = i)); }); }
    const horiz = dir === "LR", main = horiz ? NW + GAPX : NH + GAPY, cross = horiz ? NH + GAPY : NW + GAPX, maxLen = Math.max(...layers.map((L) => L.length));
    layers.forEach((L, li) => { const off = (maxLen - L.length) * cross / 2; L.forEach((id, i) => { const node = nodeById(G, id), m = PAD + li * main, c = PAD + off + i * cross; if (horiz) { node.x = m; node.y = c; } else { node.x = c; node.y = m; } }); });
  }
  function mk(id, color, big) { const s = big ? "M0,0 L7.5,3.2 L0,6.4 Z" : "M0,0 L7,3 L0,6 Z", r = big ? 3.2 : 3, w = big ? 10 : 9; return `<marker id="${id}" markerWidth="${w}" markerHeight="${w}" refX="${big ? 8 : 7.5}" refY="${r}" orient="auto-start-reverse" markerUnits="userSpaceOnUse"><path d="${s}" fill="${color}"/></marker>`; }

  // ── 引擎（挂进 host，返回 handle）──
  function mountEngine(host, opts) {
    const S = { G: opts.graph || { nodes: [], edges: [] }, mode: opts.mode || "edit", dir: opts.dir || "LR", run: opts.run || null, sel: null, view: { x: 60, y: 60, k: 1 }, back: new Set(), bounds: { maxX: 0, maxY: 0 } };
    const onSelect = opts.onSelect || (() => {}), onChange = opts.onChange || (() => {}), toast = opts.onToast || (() => {});
    const cv = el("svg", { class: "fg-canvas" }); cv.style.cssText = "position:absolute;inset:0;width:100%;height:100%;display:block"; host.appendChild(cv);

    function recompute() { S.back = backEdges(S.G); }
    function bounds() { S.bounds.maxX = Math.max(NW, ...S.G.nodes.map((n) => n.x + NW)); S.bounds.maxY = Math.max(NH, ...S.G.nodes.map((n) => n.y + NH)); const ch = S.back.size ? 16 + S.back.size * LOOP_GAP + 8 : 0, horiz = S.dir === "LR"; S._w = S.bounds.maxX + PAD + (horiz ? 0 : ch); S._h = S.bounds.maxY + PAD + (horiz ? ch : 0); }
    function loopPath(a, b, bi) {
      const off = 16 + bi * LOOP_GAP;
      if (S.dir === "LR") { const sx = a.x + NW / 2, sy = a.y + NH, tx = b.x + NW / 2, ty = b.y + NH, ch = S.bounds.maxY + off, pts = [{ x: sx, y: sy }, { x: sx, y: ch }, { x: tx, y: ch }, { x: tx, y: ty }]; return { pts, d: roundedPath(pts, CORNER), mid: { x: (sx + tx) / 2, y: ch } }; }
      const sx = a.x + NW, sy = a.y + NH / 2, tx = b.x + NW, ty = b.y + NH / 2, ch = S.bounds.maxX + off, pts = [{ x: sx, y: sy }, { x: ch, y: sy }, { x: ch, y: ty }, { x: tx, y: ty }]; return { pts, d: roundedPath(pts, CORNER), mid: { x: ch, y: (sy + ty) / 2 } };
    }
    const toGraph = (ev) => { const r = cv.getBoundingClientRect(); return { x: (ev.clientX - r.left - S.view.x) / S.view.k, y: (ev.clientY - r.top - S.view.y) / S.view.k }; };
    const nodeAt = (gx, gy) => { for (let i = S.G.nodes.length - 1; i >= 0; i--) { const n = S.G.nodes[i]; if (gx >= n.x && gx <= n.x + NW && gy >= n.y && gy <= n.y + NH) return n; } return null; };
    const applyView = () => { const r = cv.querySelector("#fg-root"); if (r) r.setAttribute("transform", `translate(${S.view.x},${S.view.y}) scale(${S.view.k})`); opts.onView && opts.onView({ x: S.view.x, y: S.view.y, k: S.view.k, w: S._w, h: S._h }); };

    function render() {
      recompute(); bounds(); cv.innerHTML = "";
      const defs = el("defs", {});
      defs.innerHTML = mk("ah", "var(--edge)") + mk("ah-taken", "var(--ink)") + mk("ah-fut", "var(--edge-future)") + mk("ah-loop", "var(--accent)", true) + mk("ah-sel", "var(--accent)") +
        '<filter id="fg-lift" x="-30%" y="-40%" width="160%" height="180%"><feDropShadow dx="0" dy="1.5" stdDeviation="3" flood-color="#000" flood-opacity="0.10"/></filter>';
      cv.appendChild(defs);
      const root = el("g", { id: "fg-root", transform: `translate(${S.view.x},${S.view.y}) scale(${S.view.k})` }); cv.appendChild(root);
      const eL = el("g", {}), nL = el("g", {}), oL = el("g", { id: "fg-overlay" }); root.appendChild(eL); root.appendChild(nL); root.appendChild(oL);
      const by = {}; S.G.nodes.forEach((n) => (by[n.id] = n));
      const taken = new Set(S.run && S.mode === "run" ? S.run.taken : []), live = S.mode === "run" && S.run ? S.run.live : null;
      let li = 0; const loopOrd = {}; S.G.edges.forEach((e) => { if (S.back.has(e.id)) loopOrd[e.id] = li++; });

      S.G.edges.forEach((e) => {
        const a = by[e.from], b = by[e.to]; if (!a || !b) return;
        const isBack = S.back.has(e.id), selE = S.sel && S.sel.type === "edge" && S.sel.id === e.id;
        let tier = "base";
        if (S.mode === "run" && S.run) tier = e.id === live ? "live" : taken.has(e.id) ? "taken" : (S.run.state[e.to] && S.run.state[e.to] !== "future" ? "base" : "future");
        const route = isBack ? loopPath(a, b, loopOrd[e.id]) : orthoPath(a, b);
        let stroke = "var(--edge)", mk2 = "url(#ah)", w = 1.8, dash = "";
        if (isBack) { stroke = "var(--accent)"; mk2 = "url(#ah-loop)"; dash = "6 5"; }
        if (tier === "taken") { stroke = "var(--ink)"; mk2 = "url(#ah-taken)"; w = 2.3; if (isBack) dash = "6 5"; }
        if (tier === "future") { stroke = "var(--edge-future)"; mk2 = "url(#ah-fut)"; dash = "5 5"; }
        if (tier === "live") { stroke = "var(--accent)"; mk2 = "url(#ah-loop)"; w = 2.6; dash = isBack ? "6 5" : ""; }
        if (selE) { stroke = "var(--accent)"; mk2 = "url(#ah-sel)"; w = 2.6; }
        const p = el("path", { d: route.d, fill: "none", stroke, "stroke-width": w, "marker-end": mk2, "stroke-linecap": "round", "stroke-linejoin": "round" }); if (dash) p.setAttribute("stroke-dasharray", dash); eL.appendChild(p);
        if (S.mode === "edit") { const hit = el("path", { d: route.d, fill: "none", stroke: "transparent", "stroke-width": 14, class: "fg-edge-hit" }); hit.style.cursor = "pointer"; hit.addEventListener("pointerdown", (ev) => { ev.stopPropagation(); selectEdge(e.id); }); eL.appendChild(hit); }
        if (tier === "live") { const comet = el("circle", { r: 3.6, fill: "var(--accent)" }); comet.appendChild(el("animateMotion", { dur: "1.1s", repeatCount: "indefinite", path: route.d })); eL.appendChild(comet); }
        if (e.port) { const g = el("g", { transform: `translate(${route.mid.x},${route.mid.y})` }), tw = e.port.length * 12 + 14; g.appendChild(el("rect", { x: -tw / 2, y: -9, width: tw, height: 18, rx: 6, fill: "var(--island)", stroke: isBack || selE ? "var(--accent-line)" : "var(--line)", "stroke-width": 1 })); const t = el("text", { x: 0, y: 4, "text-anchor": "middle", "font-size": 11, "font-weight": 600, fill: isBack ? "var(--accent)" : "var(--ink-2)" }); t.textContent = e.port; g.appendChild(t); eL.appendChild(g); }
      });

      S.G.nodes.forEach((n) => {
        const k = KIND[n.kind] || KIND.action, st = (S.mode === "run" && S.run) ? (S.run.state[n.id] || "future") : "ready", ST = STATE[st] || STATE.ready, iters = (S.mode === "run" && S.run && S.run.iters[n.id]) || 0, selN = S.sel && S.sel.type === "node" && S.sel.id === n.id;
        const g = el("g", { class: "fg-node", transform: `translate(${n.x},${n.y})`, "data-id": n.id });
        if (iters > 1) { g.appendChild(el("rect", { x: 6, y: 6, width: NW, height: NH, rx: 14, fill: ST.fill, stroke: ST.ring, opacity: .35 })); g.appendChild(el("rect", { x: 3, y: 3, width: NW, height: NH, rx: 14, fill: ST.fill, stroke: ST.ring, opacity: .6 })); }
        const card = el("rect", { class: "fg-card", width: NW, height: NH, rx: 14, fill: ST.fill, stroke: selN ? "var(--accent)" : ST.ring, "stroke-width": selN ? 2 : (st === "running" || st === "failed" || st === "parked" ? 1.6 : 1), filter: "url(#fg-lift)" }); g.appendChild(card);
        if (st === "running") { const pr = el("rect", { width: NW, height: NH, rx: 14, fill: "none", stroke: "var(--accent)", "stroke-width": 2, opacity: 0 }); pr.appendChild(el("animate", { attributeName: "opacity", values: "0;.5;0", dur: "1.6s", repeatCount: "indefinite" })); g.appendChild(pr); }
        if (st === "future") card.setAttribute("stroke-dasharray", "4 4");
        g.appendChild(el("rect", { x: 12, y: 17, width: 26, height: 26, rx: 8, fill: k.s }));
        const ig = el("svg", { x: 16, y: 21, width: 18, height: 18, viewBox: "0 0 24 24", fill: "none", stroke: k.c, "stroke-width": 2, "stroke-linecap": "round", "stroke-linejoin": "round" }); ig.innerHTML = iconInner(k.ico); g.appendChild(ig);
        const id = el("text", { x: 48, y: 26, "font-size": 13.5, "font-weight": 600, fill: "var(--ink)" }); id.textContent = n.id; g.appendChild(id);
        const ref = el("text", { x: 48, y: 44, "font-size": 11.5, fill: "var(--ink-3)", "font-family": "var(--mono)" }); ref.textContent = (n.ref || "").length > 20 ? n.ref.slice(0, 19) + "…" : n.ref; g.appendChild(ref);
        if (S.mode === "run" && S.run) g.appendChild(el("circle", { cx: NW - 15, cy: 15, r: 4, fill: ST.c }));
        else if (n.retry) { g.appendChild(el("circle", { cx: NW - 16, cy: 16, r: 7.5, fill: "var(--warn-soft)" })); const rt = el("text", { x: NW - 16, y: 19.5, "text-anchor": "middle", "font-size": 9, "font-weight": 700, fill: "var(--warn)" }); rt.textContent = "↻"; g.appendChild(rt); }
        if (iters > 1) { g.appendChild(el("rect", { x: NW - 40, y: NH - 19, width: 32, height: 15, rx: 7.5, fill: "var(--accent-soft)" })); const t = el("text", { x: NW - 24, y: NH - 8, "text-anchor": "middle", "font-size": 10.5, "font-weight": 700, fill: "var(--accent)" }); t.textContent = "×" + iters; g.appendChild(t); }
        if (S.mode === "edit") [["top", NW / 2, 0], ["right", NW, NH / 2], ["bottom", NW / 2, NH], ["left", 0, NH / 2]].forEach(([side, hx, hy]) => { const hg = el("g", { class: "fg-handle" }); hg.appendChild(el("circle", { cx: hx, cy: hy, r: 11, fill: "transparent" })); hg.appendChild(el("circle", { class: "fg-handle-dot", cx: hx, cy: hy, r: 4.5 })); hg.style.cursor = "crosshair"; hg.addEventListener("pointerdown", (ev) => startConnect(ev, n, side)); g.appendChild(hg); });
        g.addEventListener("pointerdown", (ev) => startNodeDrag(ev, n));
        nL.appendChild(g);
      });
    }

    cv.addEventListener("wheel", (ev) => { ev.preventDefault(); const r = cv.getBoundingClientRect(), mx = ev.clientX - r.left, my = ev.clientY - r.top, f = Math.exp(-ev.deltaY * 0.0015), nk = Math.min(2.5, Math.max(.2, S.view.k * f)), kr = nk / S.view.k; S.view.x = mx - (mx - S.view.x) * kr; S.view.y = my - (my - S.view.y) * kr; S.view.k = nk; applyView(); }, { passive: false });
    let pan = null;
    cv.addEventListener("pointerdown", (ev) => { if (ev.target.closest(".fg-node") || ev.target.closest(".fg-edge-hit")) return; if (S.sel) { S.sel = null; render(); onSelect(null); } pan = { x: ev.clientX, y: ev.clientY, vx: S.view.x, vy: S.view.y }; cv.style.cursor = "grabbing"; cv.setPointerCapture(ev.pointerId); });
    cv.addEventListener("pointermove", (ev) => { if (!pan) return; S.view.x = pan.vx + (ev.clientX - pan.x); S.view.y = pan.vy + (ev.clientY - pan.y); applyView(); });
    cv.addEventListener("pointerup", () => { pan = null; cv.style.cursor = "grab"; });
    function fit() { bounds(); const r = cv.getBoundingClientRect(), pd = 48, k = Math.min((r.width - pd * 2) / S._w, (r.height - pd * 2) / S._h, 1.3); S.view.k = Math.max(.25, k || 1); S.view.x = (r.width - S._w * S.view.k) / 2; S.view.y = (r.height - S._h * S.view.k) / 2; applyView(); }

    let nd = null;
    function startNodeDrag(ev, n) { if (ev.target.closest(".fg-handle")) return; ev.stopPropagation(); selectNode(n.id); if (S.mode !== "edit") return; nd = { n, x: ev.clientX, y: ev.clientY, nx: n.x, ny: n.y, moved: false }; window.addEventListener("pointermove", ndMove); window.addEventListener("pointerup", ndUp); }
    function ndMove(ev) { if (!nd) return; const dx = (ev.clientX - nd.x) / S.view.k, dy = (ev.clientY - nd.y) / S.view.k; if (Math.abs(dx) + Math.abs(dy) > 2) nd.moved = true; nd.n.x = nd.nx + dx; nd.n.y = nd.ny + dy; render(); }
    function ndUp() { window.removeEventListener("pointermove", ndMove); window.removeEventListener("pointerup", ndUp); if (nd && nd.moved) { nd.n.pos = { x: Math.round(nd.n.x), y: Math.round(nd.n.y) }; onChange({ ops: [OPS.updateNode(nd.n.id, { pos: nd.n.pos })], label: "移动", minor: true }); } nd = null; }

    let conn = null;
    function startConnect(ev, node, side) { ev.stopPropagation(); ev.preventDefault(); cv.classList.add("connecting"); const A = anchorPt(node, side), nrm = normal(side); conn = { from: node.id, A, nrm, path: el("path", { fill: "none", stroke: "var(--accent)", "stroke-width": 2, "stroke-dasharray": "4 4", "stroke-linecap": "round" }) }; cv.querySelector("#fg-overlay").appendChild(conn.path); window.addEventListener("pointermove", connMove); window.addEventListener("pointerup", connUp); }
    function connMove(ev) { if (!conn) return; const p = toGraph(ev), c1 = { x: conn.A.x + conn.nrm.x * 44, y: conn.A.y + conn.nrm.y * 44 }; conn.path.setAttribute("d", `M${conn.A.x},${conn.A.y} C${c1.x},${c1.y} ${p.x},${p.y} ${p.x},${p.y}`); const t = nodeAt(p.x, p.y); cv.querySelectorAll(".fg-node").forEach((g) => g.classList.toggle("hl", !!t && t.id !== conn.from && g.dataset.id === t.id)); }
    function connUp(ev) { window.removeEventListener("pointermove", connMove); window.removeEventListener("pointerup", connUp); cv.classList.remove("connecting"); cv.querySelectorAll(".fg-node").forEach((g) => g.classList.remove("hl")); const p = toGraph(ev), t = nodeAt(p.x, p.y); if (conn.path.remove) conn.path.remove(); const from = conn.from; conn = null; if (t && t.id !== from) addEdge(from, t.id); }

    function addEdge(from, to) { const v = validateEdge(S.G, from, to); if (!v.ok) return toast(v.reason); const edge = { id: uid("e"), from, to, port: v.port }; S.G.edges.push(edge); recompute(); onChange({ ops: [OPS.addEdge(edge)], label: "连接 " + from + "→" + to + (v.port ? " (" + v.port + ")" : "") }); selectEdge(edge.id); toast("已连接 " + from + " → " + to + (v.port ? " (" + v.port + ")" : "")); }
    function addNode(kind, at) { const id = uniqueId(kind), node = { id, kind, ref: KIND[kind].prefix + "new", input: {} }; const r = cv.getBoundingClientRect(); node.x = at ? at.x : (r.width / 2 - S.view.x) / S.view.k - NW / 2; node.y = at ? at.y : (r.height / 2 - S.view.y) / S.view.k - NH / 2; node.pos = { x: Math.round(node.x), y: Math.round(node.y) }; S.G.nodes.push(node); if (S.run) S.run.state[id] = "future"; recompute(); onChange({ ops: [OPS.addNode(node)], label: "添加节点 " + id }); selectNode(id); toast("已添加节点 " + id + "：拖拽定位，悬停显示四周连接点"); }
    function uniqueId(kind) { const base = ({ trigger: "trigger", action: "task", agent: "agent", control: "route", approval: "review" })[kind]; let i = 1, id = base; while (nodeById(S.G, id)) id = base + (++i); return id; }
    function deleteSel() {
      if (!S.sel || S.mode !== "edit") return;
      if (S.sel.type === "node") { const id = S.sel.id, cascade = S.G.edges.filter((e) => e.from === id || e.to === id).map((e) => OPS.deleteEdge(e.id)); S.G.nodes = S.G.nodes.filter((n) => n.id !== id); S.G.edges = S.G.edges.filter((e) => e.from !== id && e.to !== id); recompute(); onChange({ ops: [OPS.deleteNode(id), ...cascade], label: "删节点 " + id }); }
      else { const id = S.sel.id; S.G.edges = S.G.edges.filter((e) => e.id !== id); recompute(); onChange({ ops: [OPS.deleteEdge(id)], label: "删边" }); }
      S.sel = null; onSelect(null); render();
    }
    function updateNode(id, patch) { const node = nodeById(S.G, id); if (!node) return; Object.assign(node, patch); if (patch.input) node.input = patch.input; recompute(); onChange({ ops: [OPS.updateNode(id, patch)], label: "改节点 " + id }); render(); onSelect(S.sel); }
    function updateEdge(id, patch) { const e = S.G.edges.find((x) => x.id === id); if (!e) return; Object.assign(e, patch); recompute(); onChange({ ops: [OPS.updateEdge(id, { fromPort: patch.port })], label: "改端口" }); render(); onSelect(S.sel); }
    function selectNode(id) { S.sel = { type: "node", id }; render(); onSelect(S.sel); }
    function selectEdge(id) { S.sel = { type: "edge", id }; render(); onSelect(S.sel); }
    function resolveApproval(id, decision) { if (!S.run) return; S.run.state[id] = "completed"; const m = S.run.memo[id] || (S.run.memo[id] = {}); delete m.parked; m.decision = decision; const next = S.G.edges.find((e) => e.from === id && e.port === decision); if (next) { S.run.taken.push(next.id); S.run.state[next.to] = "running"; S.run.live = next.id; } render(); onSelect(S.sel); }

    recompute(); render(); layout(S.G, S.dir); render(); requestAnimationFrame(fit);
    return {
      setMode(m) { S.mode = m; if (m === "run") S.sel = null; render(); onSelect(S.sel); },
      setDir(d) { S.dir = d; layout(S.G, d); render(); fit(); },
      setRun(r) { S.run = r; render(); },
      setGraph(g) { S.G = g; S.sel = null; recompute(); layout(S.G, S.dir); render(); fit(); onSelect(null); },
      getGraph: () => clone(S.G), getRun: () => S.run, getSelection: () => S.sel,
      getNode: (id) => nodeById(S.G, id), getEdge: (id) => S.G.edges.find((e) => e.id === id),
      relayout() { layout(S.G, S.dir); render(); fit(); }, fit,
      zoomBy(f) { const r = cv.getBoundingClientRect(), mx = r.width / 2, my = r.height / 2, nk = Math.min(2.5, Math.max(.2, S.view.k * f)), kr = nk / S.view.k; S.view.x = mx - (mx - S.view.x) * kr; S.view.y = my - (my - S.view.y) * kr; S.view.k = nk; applyView(); },
      addNode, addEdge, updateNode, updateEdge, deleteSelected: deleteSel, resolveApproval,
      select: (sel) => { if (!sel) { S.sel = null; render(); onSelect(null); } else if (sel.type === "edge") selectEdge(sel.id); else selectNode(sel.id); },
      isBack: (id) => S.back.has(id),
    };
  }

  class AnGraphCanvas extends window.AnElement {
    static tag = "an-graph-canvas";
    static observed = [];   // mode/dir 走方法不走重渲（重渲会丢视图/选中）
    static css = `
      :host { display: block; height: 100%; position: relative; }
      /* [framed]：实体页内嵌图框——占满父宽 + 定高 + card 描边圆角 + island 底（width:100% 防块在非拉伸容器里塌成边宽） */
      :host([framed]) { display: block; width: var(--w-full); height: var(--h-graph-preview); border: var(--hairline) solid var(--line); border-radius: var(--r-card); background: var(--island); overflow: hidden; }
      /* [toolbar]：左上角悬浮缩放组（zoomBy/fit 是画布自有能力，外设随画布走，不在每个消费点重拼） */
      .gtools { position: absolute; left: var(--sp-3); top: var(--sp-3); z-index: 2; display: flex; align-items: center; gap: var(--gap-tight);
        background: var(--island); border: var(--hairline) solid var(--line); border-radius: var(--r-btn); box-shadow: var(--shadow-float); padding: var(--grid); }
      .gt { display: inline-flex; align-items: center; justify-content: center; gap: var(--gap-tight); height: var(--ctl-sm); min-width: var(--ctl-sm);
        padding: 0; border: 0; background: none; border-radius: var(--r-tag); color: var(--ink-2); cursor: pointer; transition: background var(--d-fast), color var(--d-fast); }
      .gt:hover { background: var(--island-3); color: var(--ink); }
      .gt svg { width: var(--icon-sm); height: var(--icon-sm); }
      .gt-enter { padding: 0 var(--btn-pad-x-sm); font-size: var(--t-meta); font-weight: 600; color: var(--accent); }
      .gt-enter:hover { background: var(--accent-soft); color: var(--accent); }
      .gt-sep { width: var(--hairline); height: var(--ctl-sm); background: var(--line); margin: 0 var(--grid); }
      .stage { position: relative; width: 100%; height: 100%; overflow: hidden; }
      .stage::before { content: ""; position: absolute; inset: 0; pointer-events: none;
        background-image: radial-gradient(var(--grid-dot) var(--hairline), transparent var(--hairline)); background-size: var(--sp-6) var(--sp-6); }
      .fg-canvas { cursor: grab; }
      .fg-handle { opacity: 0; pointer-events: all; }
      .fg-node:hover .fg-handle, .fg-canvas.connecting .fg-handle { opacity: 1; }
      .fg-handle-dot { fill: var(--island); stroke: var(--accent); stroke-width: 1.5; }
      .fg-handle:hover .fg-handle-dot { fill: var(--accent); }
      .fg-node.hl .fg-card { stroke: var(--accent) !important; stroke-width: 2 !important; }
    `;
    render() {
      const ib = (icon, gt, title) => `<button type="button" class="gt" data-gt="${gt}" title="${title}">${window.icon(icon)}</button>`;
      const enter = this.has("enterable") ? `<span class="gt-sep"></span><button type="button" class="gt gt-enter" data-gt="enter">${window.icon("workflow")}进入编辑器</button>` : "";
      const tools = this.has("toolbar") ? `<div class="gtools">${ib("zoom-out", "zoomout", "缩小")}${ib("zoom-in", "zoomin", "放大")}${ib("expand", "fit", "适应")}${enter}</div>` : "";
      return `<div class="stage"></div>${tools}`;
    }
    hydrate() {
      if (this._h) return;   // 只挂一次（observed 空、不会重渲）
      const stage = this.$(".stage"); if (!stage) return;
      this._h = mountEngine(stage, {
        graph: this._graph || { nodes: [], edges: [] }, run: this._run || null,
        mode: this.attr("mode", "edit"), dir: this.attr("dir", "LR"),
        onSelect: (sel) => this.emit("an-graph-select", { sel }),
        onChange: (c) => this.emit("an-graph-change", c),
        onToast: (msg) => this.emit("an-graph-toast", { msg }),
        onView: (v) => this.emit("an-graph-view", v),
      });
      // 悬浮工具条：缩放/适应走画布自有方法；进入编辑器派 composed 业务事件（消费方切编辑器海洋）
      this.$$(".gt").forEach((b) => b.addEventListener("click", () => {
        const a = b.dataset.gt;
        if (a === "zoomout") this.zoomBy(1 / 1.2);
        else if (a === "zoomin") this.zoomBy(1.2);
        else if (a === "fit") this.fit();
        else if (a === "enter") this.emit("an-graph-editor", {});
      }));
      // framed（实体页定高框）：尺寸达到真实可用值即自动 fit 把整图缩进框内（含回边弧），杜绝裁切；
      // 用 ResizeObserver 在尺寸【从塌缩态变到可用宽】时触发，并随视口变化重新 fit（比一次性轮询稳，不会因初始 0 宽错过）。
      if (this.has("framed")) {
        let lw = 0, lh = 0;
        const tryFit = () => {
          if (!this.isConnected || !this._h) return;
          const r = this.getBoundingClientRect();
          if (r.width > 80 && r.height > 80 && (Math.abs(r.width - lw) > 4 || Math.abs(r.height - lh) > 4)) { lw = r.width; lh = r.height; this.fit(); }
        };
        // RO 跟视口变化重新 fit；多档 setTimeout 兜底——惰性 tab pane 布局完成时机不定，确保至少一次在真实尺寸后落地。
        if (window.ResizeObserver) { this._ro = new ResizeObserver(tryFit); this._ro.observe(this); }
        [80, 250, 600, 1100].forEach((ms) => setTimeout(tryFit, ms));
      }
    }
    // 脱离 DOM 即弃引擎句柄 + 摘 RO——重新挂载（disconnect→reconnect）时 connectedCallback 重渲会重建空 stage + .gtools，
    // 清 _h 让 hydrate 据 _graph 重新 mountEngine 并重绑 .gt + 重新 fit（否则单挂守卫会留下空画布 + 失效工具条）。
    disconnectedCallback() { this._h = null; if (this._ro) { this._ro.disconnect(); this._ro = null; } }
    set graph(g) { this._graph = g; if (this._h) this._h.setGraph(g); }
    get graph() { return this._h ? this._h.getGraph() : this._graph; }
    set run(r) { this._run = r; if (this._h) this._h.setRun(r); }
    setMode(m) { this.setAttribute("mode", m); this._h && this._h.setMode(m); }
    setDir(d) { this.setAttribute("dir", d); this._h && this._h.setDir(d); }
    relayout() { this._h && this._h.relayout(); }
    fit() { this._h && this._h.fit(); }
    zoomBy(f) { this._h && this._h.zoomBy(f); }
    addNode(k) { this._h && this._h.addNode(k); }
    deleteSelected() { this._h && this._h.deleteSelected(); }
    updateNode(id, p) { this._h && this._h.updateNode(id, p); }
    updateEdge(id, p) { this._h && this._h.updateEdge(id, p); }
    resolveApproval(id, d) { this._h && this._h.resolveApproval(id, d); }
    select(s) { this._h && this._h.select(s); }
    getGraph() { return this._h ? this._h.getGraph() : this._graph; }
    getRun() { return this._h ? this._h.getRun() : this._run; }
    getNode(id) { return this._h && this._h.getNode(id); }
    getEdge(id) { return this._h && this._h.getEdge(id); }
    getSelection() { return this._h && this._h.getSelection(); }
    isBack(id) { return this._h && this._h.isBack(id); }
  }
  window.AnElement.define(AnGraphCanvas);
  window.AnGraph = { KIND, KIND_ORDER, STATE };   // 编辑器 feature 复用类型元数据
})();

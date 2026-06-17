/* Anselm feature — 图编辑器海洋（sea）：workflow 编排图的【纯编辑】器——只改定义，无运行态。
   why 无运行态：flowrun 的逐节点记忆化/审批决策/重放是 scheduler 海洋的事；编辑器只管「定义这张图」，运行观测不混进来（否则编辑面承载两套心智、与 schedular 职责重叠）。
   顶栏 = <an-toolbar bordered>（左 返回 · 主 添加节点/自动布局/方向）。中央 an-graph-canvas[mode=edit toolbar]（自带悬浮缩放）。右岛 = 检查器（点节点/边改定义）。
   编辑动作经 an-graph-change 抛后端 :edit ops（toast 反馈）。目标 workflow 经 window.GRAPH_EDIT_TARGET 传入；返回走 Intent.select 回 entities。接线编辑复用 an-wire-list 原语。 */
window.FEATURE = window.FEATURE || {};
window.FEATURE["graph-editor"] = Object.assign(window.FEATURE["graph-editor"] || {}, {
  sea: (ctx) => {
    const reg = window.ENTITY_REGISTRY || [];
    const ent = reg.find((e) => e.id === window.GRAPH_EDIT_TARGET && e.kind === "workflow") || reg.find((e) => e.kind === "workflow") || {};
    const base = (ent.data && ent.data.graph) || { nodes: [], edges: [] };
    const graph = { nodes: (base.nodes || []).map((n) => ({ ...n, input: { ...(n.input || {}) }, retry: n.retry ? { ...n.retry } : undefined })), edges: (base.edges || []).map((e) => ({ ...e })) };
    const KIND = (window.AnGraph && window.AnGraph.KIND) || {}, KORDER = (window.AnGraph && window.AnGraph.KIND_ORDER) || [];
    const toast = (m) => window.AnToast && window.AnToast.show({ text: m });

    const wrap = document.createElement("div");
    wrap.style.cssText = "flex:1; min-height:0; display:flex; flex-direction:column;";
    const bar = document.createElement("an-toolbar");
    bar.setAttribute("bordered", ""); bar.style.flex = "none";
    const stage = document.createElement("div");
    stage.style.cssText = "flex:1; min-height:0; position:relative;";
    const cv = document.createElement("an-graph-canvas");
    cv.setAttribute("mode", "edit"); cv.setAttribute("dir", "LR"); cv.setAttribute("toolbar", "");   // 缩放走画布自带悬浮组（不在顶栏重拼）；mode 恒 edit、无运行态注入
    cv.graph = graph;
    stage.appendChild(cv);
    wrap.append(bar, stage);

    const btn = (icon, label, fn) => { const b = document.createElement("an-button"); b.setAttribute("size", "sm"); if (icon) b.setAttribute("icon", icon); if (label) b.textContent = label; b.addEventListener("click", fn); return b; };

    // ← 返回（回 entities 海洋并选回该 workflow）→ toolbar 左槽
    const back = btn("chevr", "返回", () => ctx.Intent.select({ kind: "entity", id: ent.id }));
    back.setAttribute("variant", "ghost"); back.setAttribute("slot", "left");
    bar.appendChild(back);
    // 主区（默认槽）：添加节点（5 类菜单）/ 自动布局 / 方向（无「模式」切换——编辑器恒编辑态）
    const addBtn = btn("plus", "添加节点", () => window.AnMenu.open(addBtn, {
      align: "start", namespace: "add-node",
      items: KORDER.map((k) => ({ value: k, label: (KIND[k] || {}).label + " · " + k, icon: ((window.NODE_ICON || {})[k]) || k })),
      onPick: (k) => cv.addNode(k),
    }));
    bar.appendChild(addBtn);
    bar.appendChild(btn("spin", "自动布局", () => cv.relayout()));
    const dir = document.createElement("an-segmented"); dir.items = [{ value: "LR", label: "横向" }, { value: "TB", label: "纵向" }]; dir.value = "LR";
    dir.addEventListener("an-pick", (ev) => cv.setDir(ev.detail.value)); bar.appendChild(dir);

    // 右岛检查器
    const island = document.createElement("an-right-island");
    island.setAttribute("title", "检查器"); island.setAttribute("icon", "workflow");
    const ins = document.createElement("div"); island.appendChild(ins);
    if (ctx.shell) ctx.shell.setRight(island);
    const empty = () => { ins.innerHTML = ""; const s = document.createElement("an-state"); s.setAttribute("variant", "empty"); s.setAttribute("icon", "workflow"); s.setAttribute("title", "未选中"); s.setAttribute("hint", "选择节点或连线进行编辑；拖拽空白区域平移画布。"); ins.appendChild(s); };

    // ── 检查器渲染 ──
    function fieldRow(label, control) { const f = document.createElement("an-field"); f.setAttribute("label", label); f.appendChild(control); return f; }
    function sectionLabel(t) { const d = document.createElement("an-group-label"); d.textContent = t; return d; }   // uppercase-meta 小标题单源

    function renderInspector(sel) {
      ins.innerHTML = "";
      if (!sel) return empty();
      if (sel.type === "edge") return edgeInspector(sel.id);
      const n = cv.getNode(sel.id); if (!n) return empty();
      // —— 节点定义编辑（编辑器只此一态） ——
      const dd = document.createElement("an-dropdown");
      dd.options = KORDER.map((k) => ({ value: k, label: (KIND[k] || {}).label + " · " + k }));
      dd.value = n.kind;
      dd.addEventListener("an-change", (ev) => { const nk = ev.detail.value; const patch = { kind: nk }; if (n.ref === (KIND[n.kind] || {}).prefix + "new") patch.ref = (KIND[nk] || {}).prefix + "new"; cv.updateNode(n.id, patch); });
      ins.appendChild(fieldRow("类型", dd));
      const refIn = document.createElement("an-input"); refIn.setAttribute("mono", ""); refIn.setAttribute("value", n.ref || "");
      refIn.addEventListener("focusout", () => cv.updateNode(n.id, { ref: (refIn.value || "").trim() }));
      ins.appendChild(fieldRow("ref", refIn));
      // 输入映射（field → CEL）→ an-wire-list 原语（可增删行 + 回收 map）
      ins.appendChild(sectionLabel("输入映射（field → CEL）"));
      const wl = document.createElement("an-wire-list"); wl.setAttribute("addlabel", "添加映射");
      wl.rows = n.input || {};
      wl.addEventListener("an-wire-change", (ev) => cv.updateNode(n.id, { input: ev.detail.map }));
      ins.appendChild(wl);
      // retry（action）
      if (n.kind === "action") {
        ins.appendChild(sectionLabel("Durable 重试（同轮重试，区别于循环迭代）"));
        const kv = document.createElement("an-kv");
        kv.rows = [
          { key: "启用", value: n.retry ? "是" : "否", editable: true, editor: "select", options: ["是", "否"] },
          { key: "maxAttempts", value: String(n.retry ? n.retry.maxAttempts : 3), editable: true },
          { key: "backoff", value: n.retry ? n.retry.backoff : "exponential", editable: true, editor: "select", options: ["fixed", "exponential"] },
          { key: "delayMs", value: String(n.retry ? n.retry.delayMs : 1000), editable: true },
        ];
        kv.addEventListener("an-kv-change", () => {
          const r = {}; kv.rows.forEach((x) => (r[x.key] = x.value));
          cv.updateNode(n.id, { retry: r["启用"] === "是" ? { maxAttempts: +r.maxAttempts || 1, backoff: r.backoff, delayMs: +r.delayMs || 0 } : undefined });
        });
        ins.appendChild(kv);
      }
      // 出口
      const outs = cv.getGraph().edges.filter((e) => e.from === n.id);
      if (outs.length) { ins.appendChild(sectionLabel("出口")); outs.forEach((e) => { const r = document.createElement("an-row"); r.setAttribute("passive", ""); r.setAttribute("label", "→ " + e.to); if (e.port) r.setAttribute("meta", e.port + (cv.isBack(e.id) ? " ↩" : "")); ins.appendChild(r); }); }
      const del = document.createElement("div"); del.style.cssText = "margin-top:var(--sp-4);";
      del.appendChild(btnDanger("trash", "删除节点", () => cv.deleteSelected())); ins.appendChild(del);
    }

    function btnDanger(icon, label, fn) { const b = document.createElement("an-button"); b.setAttribute("variant", "danger"); b.setAttribute("size", "sm"); b.setAttribute("icon", icon); b.textContent = label; b.addEventListener("click", fn); return b; }

    function edgeInspector(id) {
      const e = cv.getEdge(id); if (!e) return empty();
      const src = cv.getNode(e.from), portable = src && (src.kind === "control" || src.kind === "approval");
      const head = document.createElement("an-row"); head.setAttribute("passive", ""); head.setAttribute("label", e.from + " → " + e.to); if (cv.isBack(id)) head.setAttribute("meta", "回边/循环"); ins.appendChild(head);
      if (portable) {
        const pin = document.createElement("an-input"); pin.setAttribute("value", e.port || ""); if (src.kind === "approval") pin.setAttribute("placeholder", "yes / no");
        pin.addEventListener("focusout", () => cv.updateEdge(id, { port: (pin.value || "").trim() }));
        ins.appendChild(fieldRow("端口", pin));
      } else { const h = document.createElement("an-callout"); h.setAttribute("tone", "info"); h.textContent = "仅 control / approval 源的边带端口。"; ins.appendChild(h); }
      const del = document.createElement("div"); del.style.cssText = "margin-top:var(--sp-4);";
      del.appendChild(btnDanger("trash", "删除连线", () => cv.deleteSelected())); ins.appendChild(del);
    }

    cv.addEventListener("an-graph-select", (ev) => renderInspector(ev.detail.sel));
    cv.addEventListener("an-graph-change", (ev) => { (ev.detail.ops || []).forEach((op) => toast("ops · " + op.op)); });
    cv.addEventListener("an-graph-toast", (ev) => toast(ev.detail.msg));
    empty();
    return wrap;
  },
});

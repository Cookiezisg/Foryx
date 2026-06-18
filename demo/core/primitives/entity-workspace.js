/* Anselm 原语 — <an-entity-workspace>。chat 右岛实体工作台（v2）= entities SSE 流的实体面板镜像，与对话流 block-tree[messages] 并行双写。
   「跟着对话长出来」：对话起步无右岛；主对话首个 tool call 触发某 item → ws.ensure + setActive → 外层 setRight 让右岛出现；后续触发的 item 进右上角下拉选择器，active 随最新触发而变。
   组织（自绘头，故宿主 an-right-island 用 headless 只给皮肤）：
     .head 左 = an-status-dot(active item 态) + 真名（fetch_article / Todo / 'Explore · …'，非 'function'）；右 = 下拉钮。
     .body 双态：item 态 = 该 item 的 canonical 完整岛屿（一个 an-tabs 切 facet，懒建隐藏不销毁）；picker 态 = 下拉钮点开后整岛变「分类选择列表」（an-segmented 状态筛 + an-sidebar-list，仅显非空分类，类左岛实体页）。
   每 item 按 category/kind 一套固定 facet（全量），未触及 facet 显 an-state 空态：
     Function 概览/源码/版本/终端/历史 · Handler 概览/源码/配置/终端/历史 · Agent 概览/指令/挂载/版本/轨迹/历史 · Workflow 概览/图/版本/运行图/历史 · Trigger 概览/firing · Todo 看板 · Subagent 轨迹/概览。
   本件只承载结构 + live 子元素入口；逐字/逐行流式由 chat sea 持 timer 驱动（守「DB 行真相、流只实时」）。
   model（JS 属性）：{ items:[ItemSpec] }；ItemSpec={ id, category:'entity'|'todo'|'subagent', kind?(category=entity 时 = function|handler|agent|workflow|trigger), name, lang?, status(anState 键 idle|run|wait|err|done), meta?, revert?, facets:[FacetSpec] }；
     FacetSpec={ key, label, empty?:{icon,title,hint}, 按 key 携 rows/callout/code/before/after/range/note/versions/args/trace/nodes/blocks/items/columns/aggregates/maskedConfig/missing }。
   命令式（sea 回合驱动）：ensure(id)（建该 item 岛 + 入 picker）· setActive(id)（切 head + body）· focus(id,facet)→该 facet live 元素（供流式）· facetEl(id,facet) · setItemStatus(id,status) · setTodo(id,items) · openPicker/closePicker · hasEnsured/size · emit 'an-revert'{id,name}。 */
(function () {
  const el = window.el;
  const KIND = window.ENTITY_KINDS || {};
  const CAT_ORDER = ["subagent", "todo", "function", "handler", "agent", "workflow", "trigger"];
  const catMeta = (c) => c === "todo" ? { icon: "list", label: "Todo" }
    : c === "subagent" ? { icon: "subagent", label: "Subagent" }
    : { icon: (KIND[c] || {}).icon || c, label: (KIND[c] || {}).label || c };
  const FILTERS = [{ value: "all", label: "全部" }, { value: "run", label: "活动中" }, { value: "done", label: "已结束" }, { value: "err", label: "失败" }];

  class AnEntityWorkspace extends window.AnElement {
    static tag = "an-entity-workspace";
    static observed = [];
    static css = `
      :host { display: flex; flex-direction: column; height: 100%; min-height: 0; }
      /* 自绘头：左 状态点+真名（ellipsis），右 下拉钮 */
      .head { flex: none; display: flex; align-items: center; gap: var(--gap); height: var(--island-head); padding: var(--sp-2) var(--sp-3) 0 var(--sp-4); }
      .hlead { flex: 1; min-width: 0; display: flex; align-items: center; gap: var(--gap-tight); }
      .hname { min-width: 0; font-size: var(--t-body); font-weight: 600; color: var(--ink); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
      .pbtn { flex: none; display: grid; place-items: center; width: var(--ctl); height: var(--ctl); border-radius: var(--r-btn);
        color: var(--ink-3); cursor: pointer; transition: background var(--d-fast), color var(--d-fast), transform var(--d-mid) var(--ease-spring); }
      .pbtn:hover { background: var(--island-3); color: var(--ink); }
      .pbtn svg { width: var(--icon-sm); height: var(--icon-sm); }
      :host([picker]) .pbtn { transform: rotate(180deg); color: var(--ink); }
      /* body 双态滚动区 */
      .body { flex: 1; min-height: 0; overflow-x: hidden; overflow-y: auto; padding: var(--sp-1) var(--sp-4) var(--sp-4);
        scrollbar-width: none; -ms-overflow-style: none; }
      .body::-webkit-scrollbar { width: 0; height: 0; }
      .item-host > an-tabs[hidden] { display: none; }
      .picker-host { display: flex; flex-direction: column; gap: var(--sp-3); }
      .picker-host .pfilter { align-self: flex-start; }
      :host([picker]) .item-host { display: none; }
      :host(:not([picker])) .picker-host { display: none; }
    `;

    set model(v) { this._spec = v || { items: [] }; if (this.isConnected) this._build(); }
    get model() { return this._spec || { items: [] }; }

    render() {
      return `<div class="head">
          <span class="hlead"><an-status-dot class="hdot" state="idle"></an-status-dot><span class="hname"></span></span>
          <button type="button" class="pbtn" aria-label="切换项">${window.icon("chevd", 14)}</button>
        </div>
        <div class="body">
          <div class="item-host"></div>
          <div class="picker-host">
            <an-segmented class="pfilter"></an-segmented>
            <an-sidebar-list class="plist" no-new></an-sidebar-list>
          </div>
        </div>`;
    }

    hydrate() {
      this.$(".pbtn").addEventListener("click", () => (this.hasAttribute("picker") ? this.closePicker() : this.openPicker()));
      const seg = this.$(".pfilter"); seg.items = FILTERS;
      seg.addEventListener("an-pick", (e) => { this._filter = e.detail.value; this._rebuildPicker(); });
      this.$(".plist").addEventListener("an-select", (e) => { this.closePicker(); this.setActive(e.detail.id); });
      this._build();
    }

    _build() {
      const host = this.$(".item-host"); if (!host) return;
      host.innerHTML = "";
      this._islands = new Map();   // id → { tabs, facets:Map<key,{root,stream}>, spec }
      this._byId = new Map();      // id → ItemSpec
      this._ensured = new Set();
      this._active = null;
      this._filter = "all";
      (this._spec && this._spec.items || []).forEach((it) => this._byId.set(it.id, it));
      this._rebuildPicker();
      // auto：静态展示（reference 画廊 / 独立页）——全 ensure + 停首项；非 auto 由 sea 逐次 ensure（= 跟着对话动态出现）
      if (this.has("auto")) {
        (this._spec && this._spec.items || []).forEach((it) => this.ensure(it.id));
        const first = this.attr("active") || ((this._spec && this._spec.items || [])[0] || {}).id;
        if (first) this.setActive(first);
      }
    }

    // ── item 岛（懒建，ensure 触发）：一个 an-tabs 切该 item 的全量 facet ──
    _buildItem(spec) {
      const tabs = el("an-tabs", { hidden: true });
      const facets = new Map();
      tabs.items = (spec.facets || []).map((f) => {
        const built = this._buildFacet(spec, f);
        facets.set(f.key, built);
        return { key: f.key, label: f.label || f.key, render: (pane) => pane.append(built.root) };
      });
      this.$(".item-host").append(tabs);
      const isl = { tabs, facets, spec };
      this._islands.set(spec.id, isl);
      return isl;
    }

    // facet → live 元素（{root 挂 tab pane, stream 供 sea 流式喂入}）；空态走 an-state
    _buildFacet(item, f) {
      if (f.empty) return { root: el("an-state", { variant: "empty", icon: f.empty.icon || "inbox", title: f.empty.title || "暂无", hint: f.empty.hint || "" }), stream: null };
      const lang = item.lang || "python";
      const kind = KIND[item.kind] || {};
      const k = f.key;

      if (k === "code" || k === "prompt" || k === "graph") {   // 源码/指令/图 ops → code-editor（op=create 流式喂）
        const ce = el("an-code-editor", { lang: f.lang || lang });
        if (f.code != null) ce.textContent = f.code;
        return { root: ce, stream: ce };
      }
      if (k === "versions") {   // 版本 Diff（op=edit 流式喂 after）+ 可选版本列表
        const wrap = el("div");
        const d = el("an-version-diff", { lang: f.lang || lang });
        if (f.range) d.setAttribute("range", f.range);
        if (f.note) d.setAttribute("note", f.note);
        if (f.before != null) d.before = f.before;
        if (f.after != null) d.after = f.after;
        wrap.append(d);
        if (f.versions && f.versions.length) {
          const card = el("an-info-card", { title: "版本", icon: "history" });
          const t = el("an-thin-table"); t.columns = f.versionCols || [{ key: "v", label: "版本" }, { key: "reason", label: "变更" }, { key: "builtIn", label: "来源" }];
          t.rows = f.versions; card.append(t); wrap.append(card);
        }
        return { root: wrap, stream: d };
      }
      if (k === "run") {   // 终端（op=run 触发，数据走 data-trace 种子）
        const rt = el("an-run-terminal", { verb: kind.verb || "Run", vico: "play", lang: "json" });
        if (f.args != null) rt.setAttribute("args", f.args);
        if (f.gate) rt.setAttribute("gate", f.gate);
        if (f.trace) rt.setAttribute("data-trace", JSON.stringify(f.trace));
        return { root: rt, stream: rt };
      }
      if (k === "flowrun") {   // 运行图节点甘特（op=flowrun 逐节点点亮）
        const g = el("an-node-gantt");
        if (f.nodes) g.nodes = f.nodes;
        const wrap = el("div");
        if (f.flowMeta) { const card = el("an-info-card", { title: "flowrun", icon: "scheduler" }); const kv = el("an-kv", { wrap: true }); kv.rows = f.flowMeta; card.append(kv); wrap.append(card); }
        wrap.append(g);
        return { root: wrap, stream: g };
      }
      if (k === "trace") {   // 轨迹（agent invoke / subagent ReAct，op=trace 逐块流）
        const bt = el("an-block-tree", { nested: true });
        if (f.blocks) bt.blocks = f.blocks;
        return { root: bt, stream: bt };
      }
      if (k === "board") {   // Todo 看板（setTodo 整表替换）
        const bt = el("an-block-tree", { nested: true });
        bt.blocks = [{ type: "todo", open: true, items: f.items || [] }];
        return { root: bt, stream: bt };
      }
      if (k === "history" || k === "firings") {   // 发丝历史表 + 可选聚合条
        const wrap = el("div");
        if (f.aggregates) wrap.append(el("an-callout", { tone: "info", html: f.aggregates }));
        const t = el("an-thin-table", { selectable: true });
        t.columns = f.columns || [];
        t.rows = f.rows || [];
        wrap.append(t);
        return { root: wrap, stream: null };
      }
      // overview / config / mounts → info-card + kv（+ callout）
      const card = el("an-info-card", { title: item.name, icon: kind.icon || (item.category === "subagent" ? "subagent" : item.kind || "blocks"), meta: item.meta || "" });
      if (f.callout) card.append(el("an-callout", { tone: f.callout.tone || "warn", html: f.callout.text }));
      const kv = el("an-kv", { wrap: true }); kv.rows = f.rows || [];
      card.append(kv);
      if (f.missing && f.missing.length) card.append(el("an-callout", { tone: "warn", html: "待配置：" + f.missing.join(" · ") }));
      return { root: card, stream: null };
    }

    // ── 命令式 API ──
    hasEnsured() { return this._ensured && this._ensured.size > 0; }
    size() { return this._ensured ? this._ensured.size : 0; }

    ensure(id) {   // item 入岛（懒建 + 进 picker）；首个 ensure → size 1，外层据此 setRight
      if (!id || !this._byId.has(id) || this._ensured.has(id)) return id;
      this._buildItem(this._byId.get(id));
      this._ensured.add(id);
      this._rebuildPicker();
      return id;
    }

    setActive(id) {   // 切 head 名/点 + body 挂该 item tabs（active 跟随对话）
      if (!id) return;
      this.ensure(id);
      this._active = id;
      this._islands.forEach((isl, k) => isl.tabs.toggleAttribute("hidden", k !== id));
      const spec = this._byId.get(id) || {};
      this.closePicker();
      const nm = this.$(".hname"); if (nm) nm.textContent = spec.name || id;
      const dot = this.$(".hdot"); if (dot) dot.setAttribute("state", spec.status || "idle");
      this._rebuildPicker();
    }

    focus(id, facetKey) {   // 切实体 + facet → 返回该 facet 的 live 流式元素
      this.setActive(id);
      const isl = this._islands.get(id); if (!isl) return null;
      if (facetKey && isl.facets.has(facetKey)) { isl.tabs.select(facetKey, false); return (isl.facets.get(facetKey) || {}).stream || null; }
      return null;
    }
    facetEl(id, facetKey) { const isl = this._islands.get(id); return isl && isl.facets.has(facetKey) ? (isl.facets.get(facetKey).stream || null) : null; }
    // facet 种子规格（流式数据单源——sea 据此流式喂入，turn 步无需重带 code/nodes/blocks）
    facetSpec(id, facetKey) { const sp = this._byId && this._byId.get(id); return sp ? (sp.facets || []).find((f) => f.key === facetKey) : null; }

    setItemStatus(id, status) {   // item 整体态变（picker 行点 + head 点）
      const spec = this._byId.get(id); if (!spec) return;
      spec.status = status;
      if (this._active === id) { const dot = this.$(".hdot"); if (dot) dot.setAttribute("state", status || "idle"); }
      this._rebuildPicker();
    }

    setTodo(id, items) {   // Todo item 看板整表替换
      const isl = this._islands.get(id) || (this.ensure(id), this._islands.get(id));
      if (!isl) return;
      const bt = (isl.facets.get("board") || {}).stream;
      if (bt) bt.blocks = [{ type: "todo", open: true, items: items || [] }];
      const spec = this._byId.get(id);
      if (spec) { const has = (items || []).some((t) => t.status === "in_progress"); const all = (items || []).length && (items || []).every((t) => t.status === "completed"); spec.status = has ? "run" : all ? "done" : "idle"; this._rebuildPicker(); if (this._active === id) { const dot = this.$(".hdot"); if (dot) dot.setAttribute("state", spec.status); } }
    }

    openPicker() {
      this.setAttribute("picker", "");
      const nm = this.$(".hname"); if (nm) nm.textContent = "选择 · " + this.size() + " 项";
      this._rebuildPicker();
    }
    closePicker() {
      if (!this.hasAttribute("picker")) return;
      this.removeAttribute("picker");
      const spec = this._byId.get(this._active) || {};
      const nm = this.$(".hname"); if (nm) nm.textContent = spec.name || "";
    }

    // picker 列表 = an-sidebar-list model：按 category 分组（仅非空 + 状态筛），每行 状态点 + 真名 + meta
    _rebuildPicker() {
      const list = this.$(".plist"); if (!list) return;
      const byCat = {};
      (this._spec && this._spec.items || []).forEach((it) => {
        if (!this._ensured.has(it.id)) return;
        if (this._filter && this._filter !== "all" && (it.status || "idle") !== this._filter) return;
        const cat = it.category === "entity" ? it.kind : it.category;
        (byCat[cat] = byCat[cat] || []).push(it);
      });
      const types = CAT_ORDER.filter((c) => byCat[c] && byCat[c].length).map((c) => {
        const m = catMeta(c);
        return { icon: m.icon, label: m.label, count: byCat[c].length, open: true,
          rows: byCat[c].map((it) => ({ id: it.id, dot: it.status || "idle", label: it.name, meta: it.meta || "", selected: it.id === this._active })) };
      });
      list.model = { filterPlaceholder: "搜索", groups: [{ types }] };
    }
  }
  window.AnElement.define(AnEntityWorkspace);
})();

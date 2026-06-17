/* Anselm feature — entities 海面（sea）：选中实体 → ocean-header（面包屑 + 可改名标题 + 状态徽 + 执行动作）+ tab（概览 schema 页 / 版本 diff）；可执行实体挂右岛试运行终端。
   订阅 Intent.on('entity')（rail 点行走 Intent.select 路由到本海洋，owns:["entity"]）。数据走共享注册表 window.ENTITY_REGISTRY。
   就地编辑（#1）：标题派 an-title-change、说明字段派 an-field-change → 回写注册表（mock，真后端走 PATCH）。状态→徽 tone 走 anTone 单源。 */
window.FEATURE = window.FEATURE || {};
window.FEATURE.entities = Object.assign(window.FEATURE.entities || {}, {
  sea: (ctx) => {
    const K = window.ENTITY_KINDS || {};
    const reg = window.ENTITY_REGISTRY || [];
    const byId = (id) => reg.find((e) => e.id === id);
    const page = document.createElement("an-page");   // 海面填充由 an-page :host 自带（flex:1/min-height:0），不再裹多余 wrap div
    let cur = null;   // 当前实体（图框「进入编辑器」据此切编辑器海洋；就地编辑回写它）

    // 实体页头：面包屑 + 可改名标题（editable）+ 状态徽（tone 走 anTone 单源）+ 执行动作组
    function header(e) {
      const kind = K[e.kind] || {};
      const h = document.createElement("an-ocean-header");
      h.setAttribute("crumb", "Entities|" + (kind.label || e.kind));
      h.setAttribute("title", e.label);
      h.setAttribute("editable", "");   // 标题就地改名（#1）
      if (e.dot) {
        const b = document.createElement("an-badge");
        b.setAttribute("slot", "meta"); b.setAttribute("dot", e.dot);
        const tone = window.anTone(e.dot); if (tone) b.setAttribute("tone", tone);   // 状态→徽色单源
        b.textContent = e.meta || (kind.label || e.kind);
        h.appendChild(b);
      }
      const ag = document.createElement("an-action-group");
      ag.setAttribute("slot", "actions"); ag.setAttribute("end", "");
      if (kind.verb) {   // 可执行 → 主 CTA（动词来自 entity-kinds 单源，对齐后端 N5）
        const run = document.createElement("an-button");
        run.setAttribute("variant", "primary"); run.setAttribute("icon", kind.icon || "run");
        run.textContent = kind.verb;
        run.addEventListener("click", () => { const rt = ctx.shell && ctx.shell.querySelector('[slot="right"] an-run-terminal'); if (rt && rt.run) rt.run(); });
        ag.appendChild(run);
      }
      const more = document.createElement("an-button");   // … = 该 kind 全动作（单源 actions.js）
      more.setAttribute("variant", "icon"); more.setAttribute("icon", "more");
      more.addEventListener("click", () => window.openEntityMenu(more, e, ctx));
      ag.appendChild(more);
      h.appendChild(ag);
      return h;
    }

    // 版本视图（#5）：左版本列表（an-row 轨）+ 右单框 unified diff（选某版与下一更旧版逐行红绿）
    function versionView(e) {
      const versions = e.versions || [], lang = e.versLang || "text";
      const grid = document.createElement("div");
      grid.style.cssText = "display:grid; grid-template-columns:minmax(0,2fr) minmax(0,3fr); gap:var(--sp-6); align-items:start;";
      const list = document.createElement("div"); list.style.cssText = "display:flex; flex-direction:column;";
      const diff = document.createElement("an-version-diff"); diff.setAttribute("lang", lang);
      let sel = 0;
      const setDiff = () => {
        const nv = versions[sel], ov = versions[sel + 1];
        diff.before = ov ? ov.src : ""; diff.after = nv ? nv.src : "";
        diff.setAttribute("range", ov ? (ov.v + " → " + nv.v) : (nv ? nv.v + " · 最早版本" : ""));
        if (nv && nv.reason) diff.setAttribute("note", nv.reason); else diff.removeAttribute("note");
      };
      const paint = () => {
        // 版本行：label=v号(+当前)、dot 标当前、hint=日期 · 变更原因（走 hint 多行换行，不挤右尾 meta）
        list.replaceChildren(...versions.map((v, i) => {
          const r = document.createElement("an-row");
          r.setAttribute("label", v.v + (v.active ? " · 当前" : ""));
          if (v.active) r.setAttribute("dot", "done");
          const hint = [v.t, v.reason].filter(Boolean).join(" · ");
          if (hint) r.setAttribute("hint", hint);
          if (i === sel) r.setAttribute("selected", "");
          r.addEventListener("an-select", () => { sel = i; setDiff(); paint(); });
          return r;
        }));
      };
      paint(); setDiff();
      grid.append(list, diff);
      return grid;
    }

    // 右岛试运行终端（仅可执行实体）；非可执行 → 收起右岛
    function runIsland(e) {
      const kind = K[e.kind] || {};
      if (!kind.verb) return null;
      const isle = document.createElement("an-right-island");
      isle.setAttribute("title", "试运行 · " + kind.verb); isle.setAttribute("icon", kind.icon || "run");
      const rt = document.createElement("an-run-terminal");
      rt.setAttribute("verb", kind.verb); rt.setAttribute("vico", "play"); rt.setAttribute("lang", "json");
      if (e.args != null) rt.setAttribute("args", e.args);
      if (e.trace) rt.setAttribute("data-trace", JSON.stringify(e.trace));
      isle.appendChild(rt);
      return isle;
    }

    function show(id) {
      const e = byId(id) || reg[0]; if (!e) return;
      cur = e;
      const kids = [header(e)];
      const versions = e.versions || [];
      if (versions.length) {   // 有版本 → 概览 / 版本 双 tab（版本是并列视图、非概览附属段）
        const tabs = document.createElement("an-tabs");
        tabs.items = [
          { key: "overview", label: "概览", render: (p) => p.appendChild(window.renderEntitySections(e.kind, e.data)) },
          { key: "versions", label: "版本", count: versions.length, render: (p) => p.appendChild(versionView(e)) },
        ];
        kids.push(tabs);
      } else {
        kids.push(window.renderEntitySections(e.kind, e.data));
      }
      page.replaceChildren(...kids);   // 复用同一 an-page，仅换光 DOM 内容（滚动壳/RO 持久）
      if (ctx.shell) ctx.shell.setRight(runIsland(e));   // 注入即开 / null 即收
    }

    // 反应式选中：旧 sea（page 已 detached）不再抢渲染——多次进入叠加 Intent.on，靠此守卫只让当前 sea 响应
    ctx.Intent.on("entity", (sel) => { if (page.isConnected) show(sel.id); });
    // 图框「进入编辑器」→ 切编辑器海洋（带当前实体 id）
    page.addEventListener("an-graph-editor", () => { if (cur && ctx.Intent.act) ctx.Intent.act({ verb: "editGraph", kind: cur.kind, id: cur.id }); });
    // 就地改名 / 改说明 → 回写注册表（rail 行同源、下次渲染即新值）；真后端走 PATCH，失败回滚
    page.addEventListener("an-title-change", (ev) => {
      if (!cur) return;
      cur.label = ev.detail.value;
      window.AnToast && window.AnToast.show({ text: "已重命名为「" + ev.detail.value + "」（mock）" });
    });
    page.addEventListener("an-field-change", (ev) => {
      if (cur && (ev.detail.label === "说明" || ev.detail.label === "角色") && cur.data) { cur.data.description = ev.detail.value; window.AnToast && window.AnToast.show({ text: "已更新说明（mock）" }); }
    });
    if (reg[0]) show(reg[0].id);   // 初始展示首个实体（此刻 page 尚未挂载，是本 sea 自身首渲）
    return page;
  },
});

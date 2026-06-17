/* Anselm demo — 装配根 / 控制器（composition root，≈ Flutter app + router）。
   职责：MANIFEST → 侧栏导航模型；接 <an-sidebar> 事件切海洋；按 manifest 路径【懒加载】features/<id>/{rail,sea}.js
        （模块自注册到 window.FEATURE[id]），未就绪则优雅占位；注入 Intent 导航器。
   feature 契约：features/<id>/rail.js 执行
        (window.FEATURE = window.FEATURE||{}, window.FEATURE.<id> = window.FEATURE.<id>||{}).rail = (ctx) => Node;  // sea 同理
        ctx = { id, manifest, Intent, Live, rail(items) }。rail(items) 是共享的侧栏构建助手（避免每模块重抄）。
   why 懒加载：加一个海洋 = 加 manifest 一行 + 自己的 features/<id>/ 文件夹，零改装配根；未铺的海洋自动占位、不报错。 */
(function () {
  const M = window.MANIFEST || [];
  const byId = (id) => M.find((f) => f.id === id);
  const navOceans = M.filter((f) => f.nav);
  const avatarId = (M.find((f) => f.axis === "avatar") || {}).id || "settings";
  const bellId = (M.find((f) => f.axis === "bell") || {}).id || "notifications";
  const defId = (M.find((f) => f.default) || navOceans[0] || {}).id;

  // ── 共享侧栏构建助手（经 ctx.rail 注入各 feature；items = [["g",标签] | ["r",{icon,dot,label,meta,hint,depth,passive,id}]]） ──
  function buildRail(items) {
    const w = document.createElement("div");
    (items || []).forEach(([t, v]) => w.appendChild(t === "g" ? groupEl(v) : rowEl(v)));
    // 点行 → 同 rail 内单选（真后端时由 feature 自行接 Intent.select）
    w.addEventListener("an-select", (e) => {
      if (e.target.tagName !== "AN-ROW") return;
      w.querySelectorAll("an-row[selected]").forEach((x) => x.removeAttribute("selected"));
      e.target.setAttribute("selected", "");
    });
    return w;
  }
  function rowEl(o) {
    const r = document.createElement("an-row");
    ["icon", "dot", "label", "meta", "hint", "id"].forEach((k) => { if (o[k] != null && o[k] !== "") r.setAttribute(k === "id" ? "data-id" : k, o[k]); });
    if (o.depth) r.setAttribute("depth", String(o.depth));
    if (o.passive) r.setAttribute("passive", "");
    return r;
  }
  function groupEl(t) {
    const d = document.createElement("an-group-label");   // uppercase-meta 小标题单源（不再内联 cssText）
    d.textContent = t;
    return d;
  }

  // ── 懒加载：每脚本只注入一次；onerror 亦 resolve（未就绪海洋走占位、不抛错） ──
  const scripts = {};
  function loadScript(src) {
    if (scripts[src]) return scripts[src];
    scripts[src] = new Promise((res) => {
      const s = document.createElement("script");
      s.src = src; s.onload = () => res(true); s.onerror = () => res(false);
      document.head.appendChild(s);
    });
    return scripts[src];
  }
  let _shell = null, _sidebar = null;   // boot 注入；供 feature ctx 控制右岛 / 侧栏

  // 取某海洋的 rail / sea 节点：先加载 manifest.deps（feature 共享模块，如实体注册表），再调工厂；未就绪 null（调用方占位）
  async function part(id, kind) {
    const f = byId(id); const src = f && f[kind];
    if (f && f.deps && !f._deps) f._deps = Promise.all(f.deps.map(loadScript));
    if (f && f._deps) await f._deps;
    const reg = () => (window.FEATURE && window.FEATURE[id] && window.FEATURE[id][kind]);
    if (!reg() && src) await loadScript(src);
    const fn = reg();
    try { return fn ? fn({ id, manifest: f, Intent: window.Intent, Live: window.Live, rail: buildRail, shell: _shell, sidebar: _sidebar }) : null; }
    catch (e) { console.warn("[feature]", id, kind, e); return null; }
  }

  // ── 占位（feature 模块未就绪时） ──
  function placeholderSea(id) {
    // 未铺海洋的占位走 an-state（居中井 + 标题 + 说明），不再手搓 icon/title/desc cssText
    const f = byId(id) || {};
    const s = document.createElement("an-state");
    s.setAttribute("icon", f.icon || "blocks");
    s.setAttribute("title", (f.label || id) + " 海洋");
    s.setAttribute("hint", (f.desc ? f.desc + " · " : "") + "Phase 3 · 此面将按 CAPABILITY.md 从后端能力 + 用户心智铺实");
    return s;
  }
  function placeholderRail() {
    const d = document.createElement("div");
    d.style.cssText = "padding:var(--sp-6) var(--sp-3); color:var(--ink-3); font-size:var(--t-meta);";
    d.textContent = "侧栏设计中…";
    return d;
  }

  function boot() {
    const shell = document.querySelector("an-shell");
    const sidebar = document.querySelector("an-sidebar");
    if (!shell || !sidebar) return;
    _shell = shell; _sidebar = sidebar;
    sidebar.model = { ws: "Personal", nav: navOceans.map((f) => ({ id: f.id, label: f.label, icon: f.icon })) };

    let current = null, prevNav = defId, token = 0;
    Intent.setCurrent(() => current);
    Intent.setNavigator((id) => switchOcean(id));
    // editGraph 动作（实体 … / 图框「进入编辑器」）→ 切图编辑器海洋，带目标 workflow id
    Intent.onAct((a) => { if (a && a.verb === "editGraph") { window.GRAPH_EDIT_TARGET = a.id; switchOcean("graph-editor"); } });

    async function switchOcean(id) {
      const f = byId(id); if (!f) return;
      const my = ++token;   // 防竞态：异步加载期间又切走 → 丢弃过期结果
      if (f.axis === "bell") {
        if (current === bellId) { switchOcean(prevNav); return; }   // toggle off → 回上一个海洋
        current = bellId; sidebar.setActive("bell"); sidebar.setUnread(false);
        const rail = await part(bellId, "rail"); if (my === token) sidebar.setRail(rail || placeholderRail());
        return;   // 通知轴只接管侧栏，海面不动（SPEC：铃铛镜像头像）
      }
      current = id; prevNav = id;
      sidebar.setActive(f.axis === "avatar" ? "avatar" : id);
      shell.setRight(null);   // 进新海洋先清右岛（feature sea 按需自行 setRight）
      const [rail, sea] = await Promise.all([part(id, "rail"), part(id, "sea")]);
      if (my !== token) return;   // 已切走，丢弃
      sidebar.setRail(rail || placeholderRail());
      shell.setSea(sea || placeholderSea(id));
    }

    sidebar.addEventListener("an-nav", (e) => switchOcean(e.detail.id));
    sidebar.addEventListener("an-axis", (e) => switchOcean(e.detail.which === "avatar" ? avatarId : bellId));
    sidebar.addEventListener("an-toggle-left", () => shell.toggleLeft());

    switchOcean(defId);
    setTimeout(() => sidebar.peek("竞品动态日报流程 · 等待审批"), 1600);
  }

  if (document.readyState !== "loading") boot();
  else document.addEventListener("DOMContentLoaded", boot);
})();

/* Anselm feature — entities 侧栏（rail）：实体按 kind 分组的 <an-sidebar-list>（New + 域内垂搜 + 排序菜单）。
   点实体行 → Intent.select({kind:'entity', id}) → 路由回本海洋、sea 渲染该实体页（owns:["entity"]）。
   数据走共享注册表 window.ENTITY_REGISTRY（manifest deps 加载）；kind 元数据走 ENTITY_KINDS 单源。 */
window.FEATURE = window.FEATURE || {};
window.FEATURE.entities = Object.assign(window.FEATURE.entities || {}, {
  rail: (ctx) => {
    const K = window.ENTITY_KINDS || {};
    const reg = window.ENTITY_REGISTRY || [];
    const byKind = {};
    reg.forEach((e) => { (byKind[e.kind] = byKind[e.kind] || []).push(e); });
    // 9 kind 归 4 大组（逻辑节点 / 控制节点 / 工作流 / 外部组件），组可折叠（chat 式头）；组内 type(kind) 仍可折叠
    const GROUPS = [
      ["逻辑节点", ["function", "handler", "agent", "trigger"]],
      ["控制节点", ["control", "approval"]],
      ["工作流", ["workflow"]],
      ["外部组件", ["mcp", "skill"]],
    ];
    const typeOf = (k) => ({
      icon: K[k].icon, label: K[k].label || k, count: byKind[k].length, open: true,
      rows: byKind[k].map((e) => ({ id: e.id, label: e.label, meta: e.meta, dot: e.dot })),
    });
    const groups = GROUPS
      .map(([label, kinds]) => ({ label, types: kinds.filter((k) => byKind[k]).map(typeOf) }))
      .filter((g) => g.types.length);
    const el = document.createElement("an-sidebar-list");
    el.setAttribute("more", "");   // 每行尾 … 动作（#8）
    el.model = { newLabel: "New Entity", filterPlaceholder: "过滤实体…", groups };
    // 只认带 id 的选中（sidebar-list 自身 re-emit）；内层 an-row 的无 id an-select 经 shadow 重定向也冒到这，须滤掉
    el.addEventListener("an-select", (ev) => { if (ev.detail && ev.detail.id != null) ctx.Intent.select({ kind: "entity", id: ev.detail.id }); });
    el.addEventListener("an-new", () => ctx.Intent.act && ctx.Intent.act({ verb: "create", kind: "entity" }));
    // 行 … → 该实体的动作菜单（单源 entities/actions.js）
    el.addEventListener("an-row-more", (ev) => {
      const ent = reg.find((x) => x.id === ev.detail.id);
      if (ent && window.openEntityMenu) window.openEntityMenu(ev.detail.anchor, ent, ctx);
    });
    return el;
  },
});

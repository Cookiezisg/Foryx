/* Anselm feature — 图编辑器海洋（rail）：返回 + 节点大纲 + 类型图例。主交互在画布 / 工具条 / 右岛检查器；rail 给概览与回退。 */
window.FEATURE = window.FEATURE || {};
window.FEATURE["graph-editor"] = Object.assign(window.FEATURE["graph-editor"] || {}, {
  rail: (ctx) => {
    const reg = window.ENTITY_REGISTRY || [];
    const ent = reg.find((e) => e.id === window.GRAPH_EDIT_TARGET && e.kind === "workflow") || reg.find((e) => e.kind === "workflow") || {};
    const graph = (ent.data && ent.data.graph) || { nodes: [], edges: [] };
    const NODE_ICON = window.NODE_ICON || {};
    const KIND = (window.AnGraph && window.AnGraph.KIND) || {};

    const w = document.createElement("div");
    w.style.cssText = "display:flex; flex-direction:column; min-height:0; height:100%;";

    // 返回（回 entities 海洋并选回该 workflow）
    const back = document.createElement("an-button");
    back.setAttribute("block", ""); back.setAttribute("icon", "chevr");
    back.textContent = "返回 " + (ent.label || "workflow");
    back.addEventListener("click", () => ctx.Intent.select({ kind: "entity", id: ent.id }));
    w.appendChild(back);

    // 节点大纲（icon 走 NODE_ICON 单源；只读概览）
    const outline = ctx.rail([["g", "节点 · " + (graph.nodes || []).length]].concat(
      (graph.nodes || []).map((n) => ["r", { icon: NODE_ICON[n.kind] || "workflow", label: n.id, meta: (KIND[n.kind] || {}).label || n.kind, passive: true }])
    ));
    outline.style.cssText = "flex:1; min-height:0; overflow-y:auto; margin-top:var(--sp-2);";
    w.appendChild(outline);

    // 图例：5 类节点色——内化进 an-kind-legend（自取 AnGraph 数据）；rail 只做底部分隔 + 留白的放置
    const legend = document.createElement("an-kind-legend");
    legend.style.cssText = "flex:none; padding:var(--sp-3) var(--grid) var(--sp-1); border-top:var(--hairline) solid var(--line);";
    w.appendChild(legend);
    return w;
  },
});

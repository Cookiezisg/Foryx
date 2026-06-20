/* Anselm feature — settings 侧栏（rail，头像轴）：六类设置导航。点行 → Intent.select({kind:settingsCat}) → 本海洋 sea 渲对应页。 */
window.FEATURE = window.FEATURE || {};
window.FEATURE.settings = Object.assign(window.FEATURE.settings || {}, {
  rail: (ctx) => {
    const w = ctx.rail([
      ["g", "设置"],
      ["r", { icon: "gear", label: "通用", id: "general" }],
      ["r", { icon: "agent", label: "模型与 Key", id: "models" }],
      ["r", { icon: "mcp", label: "MCP 与市场", id: "mcp" }],
      ["r", { icon: "doc", label: "技能", id: "skills" }],
      ["r", { icon: "box", label: "运行时与索引", id: "runtime" }],
      ["r", { icon: "sliders", label: "高级", id: "advanced" }],
    ]);
    const first = w.querySelector("an-row[data-id]"); if (first) first.setAttribute("selected", "");   // 预选首类
    w.addEventListener("an-select", (e) => {
      if (e.target.tagName === "AN-ROW" && e.target.dataset.id) ctx.Intent.select({ kind: "settingsCat", id: e.target.dataset.id });
    });
    return w;
  },
});

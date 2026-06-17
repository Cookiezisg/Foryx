/* Anselm feature — settings 侧栏（rail，头像轴）。Phase 3.0：占位设置类目；Phase 3.1 接 workspace/apikey/model/sandbox 域。 */
window.FEATURE = window.FEATURE || {};
window.FEATURE.settings = Object.assign(window.FEATURE.settings || {}, {
  rail: (ctx) => ctx.rail([
    ["g", "设置"],
    ["r", { icon: "gear", label: "通用" }],
    ["r", { icon: "agent", label: "模型与密钥" }],
    ["r", { icon: "search", label: "搜索" }],
    ["r", { icon: "mcp", label: "连接器 (MCP)" }],
    ["r", { icon: "box", label: "运行时" }],
  ]),
});

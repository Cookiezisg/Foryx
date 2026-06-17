/* Anselm feature — documents 侧栏（rail）。Phase 3.0：占位文档树；Phase 3.1 接 /documents/tree（嵌套 + 拖拽 + New）。 */
window.FEATURE = window.FEATURE || {};
window.FEATURE.documents = Object.assign(window.FEATURE.documents || {}, {
  rail: (ctx) => ctx.rail([
    ["g", "文档库"],
    ["r", { icon: "folder", label: "Inbox" }],
    ["r", { icon: "doc", label: "竞品调研.md", depth: 1, meta: "2k" }],
    ["r", { icon: "doc", label: "PRD 草稿.md", depth: 1, meta: "8k" }],
    ["r", { icon: "folder", label: "Archive" }],
  ]),
});

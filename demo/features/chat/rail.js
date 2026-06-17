/* Anselm feature — chat 侧栏（rail）。Phase 3.0：占位会话流，证明 loader 契约；Phase 3.1 接真 conversations（?sort/?archived）。 */
window.FEATURE = window.FEATURE || {};
window.FEATURE.chat = Object.assign(window.FEATURE.chat || {}, {
  rail: (ctx) => ctx.rail([
    ["g", "最近"],
    ["r", { dot: "run", label: "竞品动态日报流程", meta: "刚刚" }],
    ["r", { dot: "done", label: "发票处理 v3 迭代", meta: "2 时" }],
    ["r", { label: "重构 triage agent 提示词", meta: "昨天" }],
    ["g", "已归档"],
    ["r", { label: "周报汇总", meta: "上周" }],
  ]),
});

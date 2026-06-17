/* Anselm feature — notifications 侧栏（rail，铃铛轴）。Phase 3.0：占位收件箱；Phase 3.1 接 notifications 流（需要你 / FYI 两段 + 已读）。 */
window.FEATURE = window.FEATURE || {};
window.FEATURE.notifications = Object.assign(window.FEATURE.notifications || {}, {
  rail: (ctx) => ctx.rail([
    ["g", "需要你"],
    ["r", { dot: "wait", label: "日报流程 · 等待审批", hint: "scheduler · 2 分钟前" }],
    ["g", "动态"],
    ["r", { dot: "err", label: "sync_crm 运行失败", hint: "5 分钟前", passive: true }],
    ["r", { dot: "done", label: "process_invoice 已编辑 v3", hint: "1 时前", passive: true }],
  ]),
});

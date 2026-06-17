/* Anselm feature — scheduler 侧栏（rail）。Phase 3.0：占位工作流运行；Phase 3.1 接 GET /flowruns（运行态/历史）。 */
window.FEATURE = window.FEATURE || {};
window.FEATURE.scheduler = Object.assign(window.FEATURE.scheduler || {}, {
  rail: (ctx) => ctx.rail([
    ["g", "工作流"],
    ["r", { dot: "run", label: "nightly_report", meta: "运行中" }],
    ["r", { dot: "done", label: "invoice_pipeline", meta: "12m" }],
    ["r", { dot: "wait", label: "approval_gate_flow", meta: "待审批" }],
    ["r", { dot: "err", label: "sync_crm", meta: "失败" }],
  ]),
});

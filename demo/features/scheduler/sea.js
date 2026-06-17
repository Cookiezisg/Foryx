/* Anselm feature — scheduler 海洋（sea）：单 workflow 的 durable 执行驾驶舱。
   左岛选 workflow → 本海洋只展示【该 workflow 的运行状态】：
     ① 运行看板 an-run-board（左 = 每次 run 列表，因 workflow 被 trigger 多次 → 多条 flowrun；右 = 选中 run 的逐节点甘特）
     ② 运行图 an-graph-canvas[mode=run]（选中 run 的活态图）
     ③ 右岛节点调试（flowrun 头 + 逐 (节点,轮次) 记忆化 result / 状态 / 耗时 / 错误 / parked 审批）。
   同步：点 run 列表 → 看板内甘特随切 + 运行图 + 节点调试同步；点图节点 / 甘特行 → 右岛出该节点调试。 */
window.FEATURE = window.FEATURE || {};
window.FEATURE.scheduler = Object.assign(window.FEATURE.scheduler || {}, {
  sea: (ctx) => {
    const RUNS = window.SCHED_RUNS || [];
    const WFS = window.SCHED_WORKFLOWS || [];

    const el = window.el;   // 共享元素工厂（地基 base.js），不再各 feature 重抄

    let curRun = null;

    // ── 持久骨架（一次性建，切 workflow 只更新内容，不重建）──
    const page = el("an-page");
    const header = el("an-ocean-header", { crumb: "Scheduler", title: "运行驾驶舱" });
    const metaSpan = el("span", { slot: "meta" }); header.append(metaSpan);

    const board = el("an-run-board");
    const boardSec = el("an-section", { label: "运行" }); boardSec.append(board);

    const cv = el("an-graph-canvas", { framed: true, toolbar: true, mode: "run", dir: "LR" });
    const graphSec = el("an-section", { label: "运行图" }); graphSec.append(cv);

    page.append(header, boardSec, graphSec);

    // ── 右岛：运行详情 + 节点调试 ──
    const island = el("an-right-island", { title: "运行详情", icon: "scheduler" });
    function renderIsland(r, nodeId) {
      island.innerHTML = "";
      const headCard = el("an-info-card", { title: "flowrun 头", icon: "workflow", meta: r.status });
      const kv = el("an-kv"); kv.setAttribute("wrap", ""); kv.rows = r.head || [];
      headCard.append(kv);
      const acts = el("an-action-group");
      if (r.status === "failed") { const b = el("an-button", { size: "sm", icon: "history" }, ":replay"); b.addEventListener("click", () => window.AnToast.show({ text: ":replay 清 failed 行、自断点续跑 · replay_count++" })); acts.append(b); }
      if (r.status === "running" || r.status === "parked") { const b = el("an-button", { size: "sm", variant: "danger", icon: "stop" }, ":kill"); b.addEventListener("click", () => window.AnToast.show({ text: ":kill 标 cancelled + 取消在途 ctx" })); acts.append(b); }
      acts.setAttribute("slot", "actions"); headCard.append(acts);   // 直挂 info-card actions 槽，恢复其空动作自动塌陷
      island.append(headCard);

      const d = (r.nodeDetail || {})[nodeId];
      if (d) {
        const nc = el("an-info-card", { title: "节点 · " + nodeId, icon: "sliders" });
        const nkv = el("an-kv"); nkv.setAttribute("wrap", ""); nkv.rows = d.kv || [];
        nc.append(nkv);
        if (d.code) { const ce = el("an-code-editor", { lang: d.lang || "text", editable: "false" }); ce.textContent = d.code; nc.append(ce); }
        if (d.json) { const jt = el("an-json-tree", { root: "false" }); jt.data = d.json; nc.append(jt); }
        if (d.parked) nc.append(el("an-approval-gate", { flavor: "durable", title: "待审批", prompt: d.parked.prompt, ddl: d.parked.ddl }));
        island.append(nc);
      } else if (nodeId) {
        island.append(el("an-callout", { tone: "info" }, "节点 " + nodeId + " 无记忆化详情（future / 本 demo 仅 parked run 含逐节点调试）。"));
      } else {
        island.append(el("an-callout", { tone: "info" }, "点运行图节点或甘特行 → 看该 (节点,轮次) 的记忆化 result / 状态 / 耗时 / 错误。"));
      }
    }

    function loadRun(r) {
      curRun = r;
      cv.graph = { nodes: r.graph.nodes, edges: r.graph.edges };
      cv.run = r.graph.run || null;
      graphSec.setAttribute("label", "运行图 · " + r.id + " · " + r.status);
      renderIsland(r, null);
    }

    function loadWorkflow(wfId) {
      const wf = WFS.find((w) => w.id === wfId) || WFS[0] || {};
      const runs = RUNS.filter((r) => r.wf === wf.id).slice().sort((a, b) => (a.tMin || 0) - (b.tMin || 0));   // 最近在上
      header.setAttribute("crumb", "Scheduler | " + (wf.label || ""));
      header.setAttribute("title", wf.label || "运行驾驶舱");
      metaSpan.textContent = [wf.meta, runs.length + " 次运行"].filter(Boolean).join(" · ");
      boardSec.setAttribute("label", "运行 · 每次 trigger 一条 flowrun（点左列表切运行图 + 甘特 + 调试）");
      board.runs = runs;
      const init = runs.find((r) => r.selected) || runs[0];
      if (init) { board.selectedId = init.id; loadRun(init); }
      else { curRun = null; renderIsland({ status: "—", head: [["该 workflow", "暂无运行（等待 trigger 触发）"]] }, null); cv.graph = { nodes: [], edges: [] }; cv.run = null; }
    }

    // ── 同步接线 ──
    board.addEventListener("an-run-pick", (ev) => { const r = RUNS.find((x) => x.id === ev.detail.id); if (r) loadRun(r); });
    board.addEventListener("an-node-pick", (ev) => { if (curRun) renderIsland(curRun, ev.detail.id); });
    cv.addEventListener("an-graph-select", (ev) => { const s = ev.detail.sel; if (curRun && s && s.type === "node") renderIsland(curRun, s.id); });

    ctx.Intent.on("workflow", (selv) => { if (page.isConnected && selv && selv.id) loadWorkflow(selv.id); });
    loadWorkflow(window.SCHED_DEFAULT || (WFS[0] || {}).id);
    if (ctx.shell) ctx.shell.setRight(island);
    return page;
  },
});

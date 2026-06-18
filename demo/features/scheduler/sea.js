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
      const headCard = el("an-info-card", { title: "运行信息", icon: "workflow", meta: r.status });
      const kv = el("an-kv"); kv.setAttribute("wrap", ""); kv.rows = r.head || [];
      headCard.append(kv);
      const acts = el("an-action-group");
      if (r.status === "failed") { const b = el("an-button", { size: "sm", icon: "history" }, "重跑"); b.addEventListener("click", () => window.AnToast.show({ text: "已重跑（从失败处续跑）" })); acts.append(b); }
      if (r.status === "running" || r.status === "parked") { const b = el("an-button", { size: "sm", variant: "danger", icon: "stop" }, "终止"); b.addEventListener("click", () => window.AnToast.show({ text: "已终止运行" })); acts.append(b); }
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
        island.append(el("an-callout", { tone: "info" }, "节点 " + nodeId + " 暂无执行详情。"));
      } else {
        island.append(el("an-callout", { tone: "info" }, "选择一个节点查看执行详情。"));
      }
    }

    function loadRun(r) {
      curRun = r;
      cv.graph = { nodes: r.graph.nodes, edges: r.graph.edges };
      cv.run = r.graph.run || null;
      graphSec.setAttribute("label", "运行图");
      renderIsland(r, null);
    }

    // ── 空态（运维总览驾驶舱）：今日运行 KPI + 需要你收件箱（failed/parked）+ 在途/最近；跨 workflow 聚合 SCHED_RUNS 全集。懒建一次缓存、与 board/graph 互斥显隐。复用原语不手搓。──
    let overviewSec = null;
    function buildOverview() {
      const sec = el("div");
      const counts = RUNS.reduce((a, r) => { a.total++; a[r.status] = (a[r.status] || 0) + 1; return a; }, { total: 0 });
      const kpiSec = el("an-section", { label: "今日运行" });
      const strip = el("div"); strip.style.cssText = "display:flex; flex-wrap:wrap; gap:var(--sp-8); padding:0 var(--sp-2);";   // 紧凑 stat 条（无边、靠留白），非大数字卡
      [["运行", counts.total, "idle"], ["已完成", counts.completed || 0, "done"], ["失败", counts.failed || 0, "err"], ["在途", counts.running || 0, "run"], ["待审批", counts.parked || 0, "wait"]].forEach(([label, n, dot]) => {
        const stat = el("div"); stat.style.cssText = "display:flex; flex-direction:column; gap:var(--grid);";
        const num = el("div"); num.style.cssText = "display:flex; align-items:center; gap:var(--gap-tight); font-size:var(--t-h2); font-weight:600; color:var(--ink); font-variant-numeric:tabular-nums;";
        num.append(el("an-status-dot", { state: dot }), document.createTextNode(String(n)));
        const lbl = el("div"); lbl.style.cssText = "font-size:var(--t-meta); color:var(--ink-3);";
        lbl.textContent = label;
        stat.append(num, lbl);
        strip.append(stat);
      });
      kpiSec.append(strip);
      const inboxSec = el("an-section", { label: "需要你" });
      const inbox = el("an-thin-table", { selectable: "" });
      inbox.columns = [{ key: "id", label: "flowrun" }, { key: "reason", label: "原因" }, { key: "wf", label: "workflow" }, { key: "age", label: "时间", align: "right" }];
      inbox.rows = [
        { id: "fr_c3d471a8", reason: "failed · 可 :replay", wf: "pr_merge_flow", age: "4h 前" },
        { id: "fr_b7e0c431", reason: "parked · 待审批（剩 7h41m）", wf: "pr_merge_flow", age: "13m 前" },
      ];
      inbox.addEventListener("an-row-click", (ev) => openRun(ev.detail.row.id));
      inboxSec.append(inbox);
      const recentSec = el("an-section", { label: "在途 / 最近" });
      const recent = el("an-thin-table", { selectable: "" });
      recent.columns = [{ key: "id", label: "flowrun" }, { key: "wf", label: "workflow" }, { key: "status", label: "状态" }, { key: "when", label: "时间", align: "right" }];
      recent.rows = RUNS.slice().sort((a, b) => (a.tMin || 0) - (b.tMin || 0)).map((r) => ({ id: r.id, wf: r.wfLabel, status: r.status, when: r.when }));
      recent.addEventListener("an-row-click", (ev) => openRun(ev.detail.row.id));
      recentSec.append(recent);
      sec.append(kpiSec, inboxSec, recentSec);
      return sec;
    }
    function showOverview() {
      curRun = null;
      header.setAttribute("crumb", "Scheduler");
      header.setAttribute("title", "运维总览");
      metaSpan.textContent = RUNS.length + " 次运行 · " + RUNS.filter((r) => r.status === "failed" || r.status === "parked").length + " 需要你";
      boardSec.style.display = "none"; graphSec.style.display = "none";
      if (!overviewSec) { overviewSec = buildOverview(); page.append(overviewSec); }
      overviewSec.style.display = "";
      if (ctx.shell) ctx.shell.setRight(null);
    }
    function openRun(runId) {   // 收件箱/最近行 → 切到该 run 的 wf board + 选中该 run
      const r = RUNS.find((x) => x.id === runId); if (!r) return;
      loadWorkflow(r.wf); board.selectedId = r.id; loadRun(r);
    }

    function loadWorkflow(wfId) {
      boardSec.style.display = ""; graphSec.style.display = "";   // 从总览回 board：显运行视图、隐总览、开右岛详情
      if (overviewSec) overviewSec.style.display = "none";
      if (ctx.shell) ctx.shell.setRight(island);
      const wf = WFS.find((w) => w.id === wfId) || WFS[0] || {};
      const runs = RUNS.filter((r) => r.wf === wf.id).slice().sort((a, b) => (a.tMin || 0) - (b.tMin || 0));   // 最近在上
      header.setAttribute("crumb", "Scheduler | " + (wf.label || ""));
      header.setAttribute("title", wf.label || "运行驾驶舱");
      metaSpan.textContent = [wf.meta, runs.length + " 次运行"].filter(Boolean).join(" · ");
      boardSec.setAttribute("label", "运行记录");
      board.runs = runs;
      const init = runs.find((r) => r.selected) || runs[0];
      if (init) { board.selectedId = init.id; loadRun(init); }
      else { curRun = null; renderIsland({ status: "—", head: [["该 workflow", "暂无运行（等待 trigger 触发）"]] }, null); cv.graph = { nodes: [], edges: [] }; cv.run = null; }
    }

    // ── 同步接线 ──
    board.addEventListener("an-run-pick", (ev) => { const r = RUNS.find((x) => x.id === ev.detail.id); if (r) loadRun(r); });
    board.addEventListener("an-node-pick", (ev) => { if (curRun) renderIsland(curRun, ev.detail.id); });
    cv.addEventListener("an-graph-select", (ev) => { const s = ev.detail.sel; if (curRun && s && s.type === "node") renderIsland(curRun, s.id); });

    ctx.Intent.on("workflow", (selv) => { if (!page.isConnected) return; if (selv && selv.id) loadWorkflow(selv.id); else showOverview(); });
    showOverview();   // 默认进海洋 = 运维总览（选 workflow / 点收件箱才进运行视图）
    return page;
  },
});

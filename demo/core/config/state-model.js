/* Anselm demo — 状态翻译单源。
   后端各域状态字串五花八门，统一折叠成 <an-status-dot> 的 5 个通用态：idle / run / wait / err / done
   （颜色在 status-dot 原语里，本文件只管语义映射 + 中文标签）。
   anState(s) = 任意状态字串 → DOT key（单一翻译路径）；anTone(s) = 同一状态 → <an-badge> tone（消费方不再各写 if-else 映射表）。 */
(function () {
  window.STATE_MODEL = {
    DOT: { idle: "空闲", run: "运行中", wait: "等待", err: "失败", done: "完成" },
    ALIAS: {
      running: "run", completed: "done", failed: "err", cancelled: "idle", parked: "wait",
      active: "done", inactive: "idle", draining: "wait",
      listening: "run", fired: "done", pending: "wait", waiting: "wait", ok: "done", error: "err", future: "idle",
    },
    // DOT → badge tone（5 通用态各自的徽色；idle 无 tone = 中性）。状态→tone 唯一来源，杜绝 feature 各写并行表。
    TONE: { err: "danger", wait: "warn", done: "ok", run: "accent", idle: "" },
    ENV: { ready: "就绪", syncing: "构建中", failed: "构建失败" },
    CFG: { complete: "已配置", incomplete: "待配置" },
    CONN: { ready: "已连接", connecting: "连接中", degraded: "降级", failed: "已断开" },
  };
  window.anState = function (s) {
    const k = String(s == null ? "" : s).toLowerCase();
    if (window.STATE_MODEL.DOT[k]) return k;
    return window.STATE_MODEL.ALIAS[k] || "idle";
  };
  // 任意状态字串 → badge tone（先折成 DOT 通用态、再查 TONE）；无对应（idle）返 ""，调用方据空判断不挂 tone。
  window.anTone = function (s) { return window.STATE_MODEL.TONE[window.anState(s)] || ""; };
})();

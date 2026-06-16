/* Foryx demo — 组件 status-dot（单一事实源；收掉 en-st/eo-st/wf-st/cv-st/rp + ENV/CFG/CONN 各处副本）。
   契约：组件 = 工厂函数 → html/handle；自载同名 .css；只读令牌；fg- 前缀；不碰别的海洋。
   API：StatusDot.dot(state,{size}) → html · StatusDot.badge(mapKey,state) → html · StatusDot.setSt(el,state) 原位改态。
   state 任意（idle/run/wait/err/done/listening 或 flowrun 词汇 running/completed/failed/parked…，经 stState 折正）。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);
  const M = window.STATE_MODEL;

  function dot(state, opts = {}) {
    const k = window.stState(state), d = M.DOT[k] || M.DOT.idle;
    const cls = `fg-dot ${k}${d.pulse ? ' pulse' : ''}${d.hollow ? ' hollow' : ''}`;
    return `<span class="${cls}" style="--dsz:${opts.size || 7}px" title="${d.label}"></span>`;
  }
  // 类型化徽（env 物化 / handler config / mcp 连接）→ 点 + 文案
  function badge(mapKey, state) {
    const map = M[mapKey] || M.ENV, [stt, label] = map[state] || Object.values(map)[0];
    return `<span class="fg-badge" data-st="${mapKey}">${dot(stt, { size: 7 })}<span class="fg-badge-t">${label}</span></span>`;
  }
  // 原位改态（给一个含 .fg-dot 的元素换 state；anchor 可传 .fg-badge 连带换文案）
  function setSt(el, state, mapKey) {
    if (!el) return;
    if (mapKey) { const map = M[mapKey] || M.ENV, [stt, label] = map[state] || Object.values(map)[0]; el.innerHTML = badge(mapKey, state); return; }
    const k = window.stState(state), d = M.DOT[k] || M.DOT.idle, dd = el.classList.contains('fg-dot') ? el : el.querySelector('.fg-dot');
    if (!dd) return;
    dd.className = `fg-dot ${k}${d.pulse ? ' pulse' : ''}${d.hollow ? ' hollow' : ''}`;
    dd.title = d.label;
  }
  // 状态文案（给 sub 行用）
  const label = state => (M.DOT[window.stState(state)] || M.DOT.idle).label;

  window.StatusDot = { dot, badge, setSt, label };
})();

/* Foryx demo — 组件 run-chip（单一事实源；收掉 scheduler 海洋的 run-rail/run-chip + sch-badge 两处模板）。
   契约：组件 = 工厂函数 → handle/html；自载同名 .css；只读令牌 + 状态模型 + StatusDot；fg- 前缀；不碰别的海洋。
   API：RunChip.rail(host,runs,{current,onPick}) → {el,setCurrent(i)}（一条横向运行历史 pill 轨：点 StatusDot + run id + 相对时间 + 末活 LIVE）
        RunChip.headBadge(state) → html（运行头状态徽：running/completed/failed/cancelled/waiting）。
   runs 每项 = {id,state,when,parked?}；点 chip 既切轨态又经 Intent.select({kind:'run'}) 派发——切换是本地视图、选中是全局意图，两者并存。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  const esc = s => String(s == null ? '' : s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

  // 运行头徽：5 态枚举各自上色，文案取 StatusDot.label（经 stState 折正）；点用 StatusDot.dot（脉冲随态）
  const HEAD = ['running', 'completed', 'failed', 'cancelled', 'waiting'];
  function headBadge(state) {
    const st = HEAD.includes(state) ? state : 'cancelled';
    return `<span class="fg-runbadge ${st}">${window.StatusDot.dot(st, { size: 7 })}<span class="fg-runbadge-t">${esc(window.StatusDot.label(st))}</span></span>`;
  }

  // run chip 一枚：StatusDot（parked→等审批用 wait，否则随 run.state 折正）+ 相对时间 + 可选 LIVE
  // 运行靠人读的相对时间辨识（id 只作路由数据 data-run，不显——机器 ID 不给人看）
  function chipHTML(r, on, live) {
    const dotState = r.state === 'running' && r.parked ? 'waiting' : r.state;
    const tag = live ? `<span class="fg-runchip-live">LIVE</span>` : '';
    return `<button class="fg-runchip${on ? ' on' : ''}" data-run="${esc(r.id)}">`
      + window.StatusDot.dot(dotState, { size: 7 })
      + `<span class="fg-runchip-t">${esc(r.when)}</span>${tag}</button>`;
  }

  function rail(host, runs, opts = {}) {
    const list = runs || [];
    const onPick = opts.onPick;
    let cur = opts.current == null ? -1 : opts.current;

    const el = window.tag('div.fg-runrail');
    const render = () => {
      const lastLive = list.length - 1;
      el.innerHTML = `<span class="fg-runrail-label">运行历史</span>`
        + list.map((r, i) => chipHTML(r, i === cur, i === lastLive && r.state === 'running')).join('');
    };
    render();

    // 委托点击：切本地轨态（重渲染高亮）→ 回调宿主 → 经一个前门派发 run 选中意图（不碰任何海洋函数）
    el.addEventListener('click', e => {
      const b = e.target.closest && e.target.closest('.fg-runchip');
      if (!b || !el.contains(b)) return;
      const i = list.findIndex(r => r.id === b.dataset.run);
      if (i < 0) return;
      cur = i; render();
      if (onPick) onPick(i, list[i]);
      window.Intent && Intent.select({ kind: 'run', id: list[i].id });
    });

    if (host) host.appendChild(el);
    return { el, setCurrent(i) { cur = i; render(); } };
  }

  window.RunChip = { rail, headBadge };
})();

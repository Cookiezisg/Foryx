/* Foryx demo — 运行海洋（Scheduler）海面：薄组合（样板）。本海洋是信息最密的驾驶舱——主区 + 右岛同载。
   只调组件库 + Shell + Intent，自己几乎不画像素：RunChip(头徽/历史) + RunGraph(活运行图) + Attention(stuck) + KV(指标/概览) + RefPill + IterationStepper/ApprovalGate(节点抽屉)。
   主区居中列（对齐 documents/settings 调性，非沾满）：运行抬头 + 指标条 + stuck 横幅 + 活运行图 + 节点活动清单 + 运行历史。
   右岛常驻：默认「运行概览」（状态/触发/进度/节点分解/待办），点图节点 → 切「节点详情」（记忆化结果/迭代/拍板/错误），头部按钮可开合。
   选中通道：侧栏 workflow 行发 Intent.select({kind:'workflow'}) → 本海洋 Intent.on('workflow') 切驾驶舱；图节点/清单行点击 → 右岛看记忆化结果。
   零机器 ID：run/trigger 等内部 ID 只作路由数据（data-*），界面只显人读标签（名称/相对时间/版本/进度）。依赖 mock/workflows.js。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);
  const W = () => window.MOCK_WORKFLOWS || {};
  const firstName = () => Object.keys(W())[0];
  const NICO = k => (window.NODE_ICON || {})[k] || 'action';
  const fold = st => window.stState ? window.stState(st) : st;
  let stage, island, curName;

  const ST_TX = { running: '运行中', completed: '已完成', failed: '失败', cancelled: '已取消', waiting: '等审批', parked: '等审批', future: '未运行' };
  const REF_KIND = { action: 'function', agent: 'agent', control: 'control', approval: 'approval', trigger: 'trigger' };

  // 当前展示运行：cur 指针优先、否则末次
  const pickRun = wf => wf.runs[wf.cur] != null ? wf.runs[wf.cur] : wf.runs[wf.runs.length - 1];
  // 节点态聚合（指标 + 分解）
  const tally = run => {
    const v = Object.values(run.state || {});
    return { total: v.length, done: v.filter(x => x === 'completed').length, run: v.filter(x => x === 'running').length, fail: v.filter(x => x === 'failed').length, park: v.filter(x => x === 'parked').length };
  };
  // 节点记忆化结果摘要（人读一行）
  const memoLine = (run, id) => {
    const m = run.memo[id] || {}, st = run.state[id] || 'future';
    if (m.error) return m.error.split('：')[0].split(':')[0];
    if (m.parked) return '等待拍板';
    if (m.decision) return m.decision + (m.reason ? ' · ' + m.reason : '');
    if (m.loop) { const last = m.loop[m.loop.length - 1] || {}; return last.out || '迭代中'; }
    if (m.out) return m.out;
    return st === 'future' ? '尚未运行' : '—';
  };
  // 当前阻塞节点（parked / failed）
  const blockedNode = run => {
    const k = Object.keys(run.state || {});
    const p = k.find(id => run.state[id] === 'parked');
    if (p) return { id: p, kind: 'parked', m: run.memo[p] || {} };
    const f = k.find(id => run.state[id] === 'failed');
    if (f) return { id: f, kind: 'failed', m: run.memo[f] || {} };
    return null;
  };

  // ===== 右岛 =====
  function ensureIsland() {
    if (island && island.el && island.el.isConnected) return island;
    island = RightIsland.create('scheduler', { title: '运行概览', icon: 'scheduler', width: 360 });
    return island;
  }
  const islandHead = (ico, title, back) =>
    `${back ? `<button class="sch-ov-back" title="返回运行概览">${icon('chevr', 16)}</button>` : `<span class="fg-island-ico">${icon(ico, 17)}</span>`}` +
    `<span class="fg-island-title">${title}</span><button class="fg-island-x">${icon('close', 16)}</button>`;

  // 默认：运行概览（状态 + 指标 + 节点分解 + 待办）
  function renderOverview(wf, run) {
    ensureIsland();
    island.setHead(islandHead('scheduler', '运行概览', false));
    island.el.querySelector('.fg-island-x').onclick = () => island.hide();
    const b = island.body; b.innerHTML = '';
    if (!run) { b.appendChild(tag('div.sch-ov-empty', '该工作流暂无运行记录')); return; }

    const stVals = Object.values(run.state), hasParked = stVals.includes('parked');
    const badge = (run.runState === 'running' && hasParked) ? 'waiting' : run.runState;
    b.appendChild(tag('div.sch-ov-hd', `${RunChip.headBadge(badge)}<span class="sch-ov-when">${run.when}</span>`));

    const t = tally(run);
    const rows = [
      ['触发', wf.triggerLabel || '手动'],
      ['进度', `${run.pos} 节点`],
      ['版本', run.version],
      ['用时', run.dur || '—'],
    ];
    if (wf.next) rows.push(['下次运行', wf.next]);
    if (run.replay) rows.push(['重试', `第 ${run.replay} 次重放`]);
    KV.defs(b, rows);

    // 节点分解（小色块条 + 图例）
    const seg = [['done', t.done, '完成', 'completed'], ['run', t.run, '运行', 'running'], ['park', t.park, '等待', 'waiting'], ['fail', t.fail, '失败', 'failed']].filter(x => x[1]);
    const bd = tag('div.sch-ov-bd');
    bd.appendChild(tag('div.sch-ov-bd-h', `${t.total} 个节点`));
    const bar = tag('div.sch-ov-bar');
    seg.forEach(([k, n]) => { const s = tag('span.sch-ov-seg.' + k); s.style.flex = String(n); bar.appendChild(s); });
    bd.appendChild(bar);
    bd.appendChild(tag('div.sch-ov-bd-legend', seg.map(([k, n, lab, dot]) => `<span class="sch-ov-lg">${StatusDot.dot(dot, { size: 6 })}${n} ${lab}</span>`).join('')));
    b.appendChild(bd);

    // 待办 / 错误（最 actionable，置底高亮）
    const blk = blockedNode(run);
    if (blk && blk.kind === 'parked') {
      const sec = tag('div.sch-ov-sec', `<div class="sch-ov-sec-h">${icon('shield', 13)} 等你拍板</div>`);
      b.appendChild(sec);
      ApprovalGate.mount(sec, { flavor: 'durable', title: blk.id, prompt: blk.m.prompt, ddl: blk.m.ddl, allowReason: true });
    } else if (blk && blk.kind === 'failed') {
      b.appendChild(tag('div.sch-ov-sec', `<div class="sch-ov-sec-h">${icon('close', 13)} 失败于 ${blk.id}</div>`));
      const e = tag('div.sch-nderr'); e.textContent = blk.m.error || ''; b.appendChild(e);
    }
  }

  // 节点详情（点图节点 / 清单行）
  function openNode(wf, run, id) {
    ensureIsland();
    const nd = run.nodes.find(x => x.id === id) || {}, m = run.memo[id] || {}, st = run.state[id] || 'future';
    island.setHead(islandHead(NICO(nd.kind), id, true));
    island.el.querySelector('.fg-island-x').onclick = () => island.hide();
    island.el.querySelector('.sch-ov-back').onclick = () => renderOverview(wf, run);
    const b = island.body; b.innerHTML = '';
    b.appendChild(tag('div.sch-ndsub', `${nd.kind || ''} · ${StatusDot.dot(st)} ${ST_TX[fold(st)] || st}`));
    if (nd.ref) { const r = tag('div.sch-ndref', `引用 ${RefPill.html(REF_KIND[nd.kind] || nd.kind, nd.ref, nd.ref)} <span class="sch-ndver">${run.version}</span>`); RefPill.wire(r); b.appendChild(r); }
    if (m.loop) IterationStepper.mount(b, { items: m.loop });
    else if (m.parked) ApprovalGate.mount(b, { flavor: 'durable', title: id, prompt: m.prompt, ddl: m.ddl, allowReason: true });
    else if (m.error) { const e = tag('div.sch-nderr'); e.textContent = m.error; b.appendChild(e); }
    else if (m.decision) { const k = tag('div'); KV.defs(k, [['决定', m.decision], ['理由', m.reason || '—']]); b.appendChild(k); }
    else { const o = tag('div.sch-ndout'); o.textContent = st === 'future' ? '尚未运行' : (m.out || '—'); b.appendChild(o); }
    island.show();
  }

  // ===== 主区驾驶舱（居中列） =====
  function cockpit(name) {
    const wf = W()[name]; curName = name;
    if (!wf || !wf.runs.length) { stage.innerHTML = `<div class="sch-col"><div class="sch-empty">${name} · 暂无运行记录</div></div>`; renderOverview(wf || {}, null); return; }
    const run = pickRun(wf);

    const stVals = Object.values(run.state), hasParked = stVals.includes('parked');
    const badge = (run.runState === 'running' && hasParked) ? 'waiting' : run.runState;
    const metric = (k, v) => `<span class="sch-m"><span class="sch-m-k">${k}</span><span class="sch-m-v">${v}</span></span>`;
    // 工作流级排程（跟工作流、不跟单次运行）：触发方式 + 下次
    const sched = [
      `<span class="sch-m"><span class="sch-m-k">${icon(run.trigger ? 'trigger' : 'play', 12)} 触发</span><span class="sch-m-v">${wf.triggerLabel || '手动'}</span></span>`,
      wf.next ? metric('下次运行', wf.next) : '',
    ].filter(Boolean).join('');
    // 运行级指标（跟选中那次运行）：进度 + 用时 + 版本 + 重试
    const runMetrics = [
      `<span class="sch-m"><span class="sch-m-k">进度</span><span class="sch-m-v"><b>${run.pos}</b> 节点</span></span>`,
      metric('用时', run.dur || '—'),
      metric('版本', run.version),
      run.replay ? metric('重试', `第 ${run.replay} 次`) : '',
    ].filter(Boolean).join('');

    let stuck = '';
    const blk = blockedNode(run);
    if (blk && blk.kind === 'parked') stuck = Attention.html('shield', `停在 <b>${blk.id}</b> · 等待 ${blk.m.form || '审批'}（${blk.m.ddl || ''}）`, { tone: 'warn' });
    else if (blk && blk.kind === 'failed') stuck = Attention.html('close', `<b>${blk.id}</b> 失败 · ${blk.m.error || ''}`, { tone: 'danger' });

    // 节点活动清单（人读 timeline：状态 + 名 + 实体 + 记忆化结果摘要 + 迭代）
    const rows = run.nodes.map(nd => {
      const st = run.state[nd.id] || 'future', its = (run.iters || {})[nd.id] || 1;
      const dot = st === 'future' ? `<span class="sch-na-fut"></span>` : StatusDot.dot(fold(st), { size: 7 });
      const iter = its > 1 ? `<span class="sch-na-iter">×${its}</span>` : '';
      return `<button class="sch-na ${st}" data-node="${nd.id}">
        <span class="sch-na-st">${dot}</span>
        <span class="sch-na-nm">${nd.id}</span>
        <span class="sch-na-ref">${nd.ref}</span>${iter}
        <span class="sch-na-out">${memoLine(run, nd.id)}</span></button>`;
    }).join('');

    stage.innerHTML = `
      <div class="sch-col">
        <div class="sch-wfhd">
          <span class="sch-wf-ico">${icon('scheduler', 18)}</span>
          <h1 class="sch-title">${name}</h1>
          <span class="grow"></span>
          <span class="sch-wf-sched">${sched}</span>
        </div>
        <div class="sch-hist">
          <div class="sch-sec-h">运行历史</div>
          <div class="sch-rail" id="schRail"></div>
        </div>
        <div class="sch-run">
          <div class="sch-run-hd">${RunChip.headBadge(badge)}<span class="sch-run-when">${run.when}</span><span class="grow"></span><span class="sch-run-metrics">${runMetrics}</span></div>
          ${stuck}
          <div class="sch-graph" id="schGraph"></div>
          <div class="sch-na-wrap">
            <div class="sch-sec-h">节点活动</div>
            <div class="sch-na-list" id="schNodes">${rows}</div>
          </div>
        </div>
      </div>`;

    RunGraph.render(stage.querySelector('#schGraph'), Object.assign({ onNode: id => openNode(wf, run, id) }, run));
    stage.querySelectorAll('.sch-na').forEach(r => r.onclick = () => openNode(wf, run, r.dataset.node));
    RunChip.rail(stage.querySelector('#schRail'), wf.runs.map(r => ({ id: r.id, state: r.runState, when: r.when, live: r.runState === 'running' })), {
      current: wf.cur != null ? wf.cur : wf.runs.length - 1,
      onPick: i => { wf.cur = i; cockpit(name); },
    });
    const sc = stage.parentElement; if (sc) sc.scrollTop = 0;
    renderOverview(wf, run);   // 右岛常驻概览
  }

  Shell.registerOcean('scheduler', {
    crumb: '运行',
    build(sea) {
      sea.innerHTML = `<div class="sch"><div class="sch-scroll scroll-fade" id="schScroll"><div id="schStage"></div></div></div>`;
      stage = sea.querySelector('#schStage');
      Shell.headExtra(`<button class="ibtn" id="schPanel" title="运行概览">${icon('panel')}</button>`);
      Shell.$('#schPanel').onclick = () => { ensureIsland(); island.toggle(); };
      cockpit(firstName());
      if (island) island.show();   // 信息最密海洋：右岛默认常驻概览
    },
  });

  // 选中通道：侧栏 workflow 行 → Intent.select({kind:'workflow'}) → 切驾驶舱
  Intent.on('workflow', sel => { if (stage) cockpit(sel.id); });
})();

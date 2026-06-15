/* Forgify design-lab — 【Scheduler/Operate 海洋】的左侧栏内容（独立文件，一人负责；与外壳/别的海洋解耦）。
   定位：每个 workflow 的运行状态一览（运行中/等你审批/失败/上次成功/在线空闲）。点一条 → 主区看该 wf 的详细 run（逐节点 + 历史）。
   触发器不在此（归 Entities 海洋，它是实体）；逐节点钻取/审批卡/firing 台账都在主区，侧栏只给状态一览 + 过滤。
   外壳 sidebar.js 据四导航懒加载本文件；自注入 scheduler.css，经 SideBar.register('scheduler', render) 挂载。依赖 icon()。 */
(function () {
  const dir = new URL('.', document.currentScript.src).href;
  if (!document.querySelector('link[data-sb="scheduler"]')) {
    const l = document.createElement('link');
    l.rel = 'stylesheet'; l.href = dir + 'scheduler.css'; l.dataset.sb = 'scheduler';
    document.head.appendChild(l);
  }

  // 每个 workflow 的派生运行状态（示意）。接后端：GET /workflows + 派生态（有 running flowrun=run / parked 审批=wait / needsAttention=err / 上次成功=done / 在线监听空闲=idle）。
  const WF = [
    { n: '研报抓取流', st: 'run', meta: '运行中 3/5' },
    { n: '竞品监控流', st: 'wait', meta: '等你审批' },
    { n: '账单对账流', st: 'err', meta: '失败 · 2m' },
    { n: '日报汇总流', st: 'done', meta: '今天 9:02', on: true },
    { n: 'Slack 通知流', st: 'idle', meta: '昨天' },
    { n: 'PDF 批处理流', st: 'idle', meta: 'Jun 8' },
    { n: '旧版迁移流', st: 'done', meta: 'Jun 1' },
  ];
  let sort = 'attention';   // 默认按「需关注」排（运行中/等你/失败 浮顶）

  const row = w => `<div class="wf wf-${w.st}${w.on ? ' on' : ''}" data-id="${w.n}"><span class="wf-st"></span><span class="wf-t">${w.n}</span><span class="wf-meta">${w.meta}</span><span class="wf-more">${icon('more', 16)}</span></div>`;
  const opt = (v, on, label) => `<button class="sm-opt${on ? ' on' : ''}" data-sort="${v}"><span class="sm-ck">${icon('check', 14)}</span>${label}</button>`;

  function build() {
    return `
      <div class="sch-filter">${icon('search', 16)}<input type="text" placeholder="Filter workflows…">
        <button class="sm-btn" title="Sort">${icon('sliders', 16)}</button>
        <div class="sm-menu">
          <div class="sm-h">Sort by</div>
          ${opt('attention', sort === 'attention', 'Needs attention')}
          ${opt('recent', sort === 'recent', 'Recent run')}
          ${opt('name', sort === 'name', 'Name')}
        </div>
      </div>
      <div class="sch-list">${WF.map(row).join('')}</div>`;
  }

  function render(host) {
    host.innerHTML = build();
    // 选中 workflow（主区给详细 run）
    host.querySelectorAll('.wf').forEach(it => it.onclick = e => {
      if (e.target.closest('.wf-more')) return;
      host.querySelectorAll('.wf').forEach(x => x.classList.remove('on')); it.classList.add('on');
      if (window.Shell && Shell.openWorkflow) Shell.openWorkflow(it.dataset.id);   // 选中 → 海面驾驶舱切到该 wf 的 run（外壳通道；海洋未挂则仅高亮）
    });
    // 排序菜单（示意单选；真排序接后端 List sort / 派生态）
    const btn = host.querySelector('.sm-btn'), menu = host.querySelector('.sm-menu');
    btn.onclick = e => { e.stopPropagation(); const open = menu.classList.toggle('open'); btn.classList.toggle('on', open); };
    menu.addEventListener('click', e => e.stopPropagation());
    menu.querySelectorAll('[data-sort]').forEach(o => o.onclick = () => {
      menu.querySelectorAll('[data-sort]').forEach(x => x.classList.remove('on')); o.classList.add('on'); sort = o.dataset.sort;
    });
    // 名称快滤
    const fin = host.querySelector('.sch-filter input');
    fin.oninput = () => {
      const q = fin.value.trim().toLowerCase();
      host.querySelectorAll('.wf').forEach(w => { w.style.display = w.querySelector('.wf-t').textContent.toLowerCase().includes(q) ? '' : 'none'; });
    };
  }

  // 点菜单外收起
  document.addEventListener('click', () => {
    const m = document.querySelector('#sidebody .sm-menu.open');
    if (m) { m.classList.remove('open'); document.querySelector('#sidebody .sm-btn')?.classList.remove('on'); }
  });

  SideBar.register('scheduler', render);
})();

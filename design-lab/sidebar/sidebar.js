/* Forgify design-lab — 左侧栏模块（单独，一人负责）。
   自挂载进 Shell.left。依赖：shared/icons.js（icon）+ shared/shell.js（Shell.left）。只读它们、不改。
   注：模式段/Recents 现为示意；接后端时换成真数据，外壳契约不变。 */
(function () {
  const left = Shell.left;
  const html = document.documentElement;

  // 首帧无闪：注入前先据 localStorage 把终态宽度/收起态写到 <html>（避免 240→实际宽跳变）
  let w0 = parseInt(localStorage.getItem('fg.side.w'), 10);
  if (!(w0 >= 200 && w0 <= 420)) w0 = 240;                 // 脏值/NaN 回退
  html.style.setProperty('--side-w', w0 + 'px');
  html.dataset.side = localStorage.getItem('fg.side.collapsed') === '1' ? 'off' : 'on';

  left.innerHTML = `
    <div class="side-top">
      <div class="lights"><span class="light r"></span><span class="light y"></span><span class="light g"></span></div>
      <span class="grow"></span>
      <button class="ibtn" data-i="side"></button>
      <button class="ibtn" data-i="search"></button>
    </div>
    <div class="modeseg" id="modeseg">
      <button data-m="chat"><span class="ico" data-i="chat"></span><span class="lbl">Chat</span></button>
      <button data-m="tasks"><span class="ico" data-i="tasks"></span><span class="lbl">Tasks</span></button>
      <button class="on" data-m="code"><span class="ico" data-i="code"></span><span class="lbl">Code</span></button>
    </div>
    <div class="sact">
      <button class="sitem"><span class="ico" data-i="plus"></span> New session</button>
      <button class="sitem"><span class="ico" data-i="zap"></span> Routines</button>
      <button class="sitem"><span class="ico" data-i="dispatch"></span> Dispatch <span class="beta">Beta</span></button>
      <button class="sitem"><span class="ico" data-i="sliders"></span> Customize</button>
      <button class="sitem"><span class="ico" data-i="chevd"></span> More</button>
    </div>
    <div class="recents">
      <div class="rec-head"><span>Recents</span><button class="ibtn" data-i="sort"></button></div>
      <div id="reclist"></div>
    </div>
    <div class="suser">
      <span class="av">sw</span>
      <span class="m"><b>Sun Weilin</b><span class="plan">· Max</span></span>
      <span class="chev" data-i="chevd"></span>
    </div>`;

  const sz = { side: 18, search: 18, chat: 15, tasks: 15, code: 15, plus: 18, zap: 18, dispatch: 18, sliders: 18, chevd: 14, sort: 15 };
  left.querySelectorAll('[data-i]').forEach(el => { const k = el.dataset.i; el.innerHTML = icon(k, sz[k] || 18); });

  const SESS = [
    ['前端设计 (fork)', 'run', true], ['前端部署', 'run', false], ['Backend重构 [Done]', 'done', false],
    ['列名检查 [adhoc]', 'done', false], ['版本控制管理 [done]', 'done', false], ['Workflow重构 Implement [done]', 'done', false],
    ['Workflow重构 Review [done]', 'done', false], ['HardCode治理专项 [done]', 'done', false],
    ['Workflow Feature迭代探索 [done]', 'done', false], ['E2E修复 [Done]', 'done', false], ['Testend修复 [Done]', 'done', false],
    ['API模型配置迭代 [Done]', 'done', false], ['前端文档页面优化 [Done]', 'done', false],
  ];
  const rl = left.querySelector('#reclist');
  rl.innerHTML = SESS.map(([t, st, on]) =>
    `<div class="ritem${on ? ' on' : ''}"><span class="d${st === 'run' ? ' run' : ''}"></span><span class="t">${t}</span></div>`).join('');
  rl.querySelectorAll('.ritem').forEach(it => it.onclick = () => {
    rl.querySelectorAll('.ritem').forEach(x => x.classList.remove('on')); it.classList.add('on');
  });

  const seg = left.querySelector('#modeseg');
  seg.querySelectorAll('button').forEach(b => b.onclick = () => {
    seg.querySelectorAll('button').forEach(x => x.classList.remove('on')); b.classList.add('on');
  });

  // —— 收起/展开 + 拖拽调宽（状态/交互/持久化全归侧栏；单一真相 = html[data-side]） ——
  function toggle() {
    const off = html.dataset.side === 'off';
    html.dataset.side = off ? 'on' : 'off';
    localStorage.setItem('fg.side.collapsed', off ? '0' : '1');
  }
  left.querySelector('[data-i="side"]').onclick = toggle;   // 岛顶折叠按钮（展开态可见）

  // 再展开按钮 → shell 的中性 #head-lead 槽（收起后岛全隐、按钮需岛外有家；收起语义不进内核）
  const reopen = document.createElement('button');
  reopen.className = 'ibtn side-reopen';
  reopen.title = '展开侧栏';
  reopen.innerHTML = icon('side', 18);
  reopen.onclick = toggle;
  Shell.headLead.appendChild(reopen);

  // 拖拽手柄（贴右内缘）：window 级监听 + flag（稳健处理指针移出窗口）；
  // move 中只改 CSS var、pointerup 才落盘；拖拽中关 transition 跟手。
  const grip = document.createElement('div');
  grip.className = 'side-grip';
  left.appendChild(grip);
  let sx = 0, sw = 0, dragging = false;
  grip.addEventListener('pointerdown', e => {
    if (html.dataset.side !== 'on') return;                 // 收起态不响应（双保险①）
    dragging = true; sx = e.clientX; sw = Shell.sideWidth || 240;
    html.dataset.sideDragging = '';
    document.body.style.userSelect = 'none'; document.body.style.cursor = 'col-resize';
    e.preventDefault();
  });
  window.addEventListener('pointermove', e => {
    if (!dragging) return;
    const next = Math.max(200, Math.min(420, sw + (e.clientX - sx)));   // clamp[200,420]
    html.style.setProperty('--side-w', next + 'px');
  });
  window.addEventListener('pointerup', () => {
    if (!dragging) return;
    dragging = false;
    delete html.dataset.sideDragging;
    document.body.style.userSelect = ''; document.body.style.cursor = '';
    localStorage.setItem('fg.side.w', Math.round(Shell.sideWidth));     // 仅松手时落盘
  });
})();

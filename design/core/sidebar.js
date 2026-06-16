/* Foryx demo — 左侧栏【固定外壳 chrome】（核心）。与海洋侧栏内容彻底解耦。
   只管：岛皮肤 + 红绿灯/折叠/搜索 + 四导航(由 manifest 生成) + 工作区/通知/设置两轴 + 收起拖拽 + #sidebody 宿主。
   海洋侧栏内容各一文件 features/<id>/rail.js，经 SideBar.register(id,render) 注册；据 manifest 经 loader 懒加载。
   nav/选中切换不再硬编码——四导航来自 MANIFEST.filter(nav)，两轴来自 manifest.axis。默认挂载由 boot.js 决定。 */
(function () {
  const left = Shell.left;
  const html = document.documentElement;
  const M = window.MANIFEST || [];
  const NAV = M.filter(f => f.nav).map(f => [f.id, f.label, f.icon]);
  const NAME = Object.fromEntries(M.map(f => [f.id, f.label]));
  const AVATAR_ID = (M.find(f => f.axis === 'avatar') || {}).id || 'settings';
  const BELL_ID = (M.find(f => f.axis === 'bell') || {}).id || 'notifications';

  // 首帧无闪：注入前据 localStorage 写终态宽度/收起态。宽度 clamp[240,420]。
  let w0 = parseInt(localStorage.getItem('fg.side.w'), 10);
  if (!(w0 >= 240 && w0 <= 420)) w0 = 240;
  html.style.setProperty('--side-w', w0 + 'px');
  html.dataset.side = localStorage.getItem('fg.side.collapsed') === '1' ? 'off' : 'on';

  left.innerHTML = `
    <div class="side-top">
      <div class="lights"><i class="light r"></i><i class="light y"></i><i class="light g"></i></div>
      <span class="grow"></span>
      <button class="ibtn" data-act="collapse" title="收起侧栏">${icon('side', 18)}</button>
      <button class="ibtn" title="搜索">${icon('search', 18)}</button>
    </div>
    <div class="modeseg" id="modeseg">
      ${NAV.map(([id, label, ic]) => `<button data-m="${id}"><span class="ico">${icon(ic, 18)}</span><span class="lbl">${label}</span></button>`).join('')}
    </div>
    <div id="sidebody"></div>
    <div class="sfoot">
      <button class="ws" title="工作区主页 / 设置"><span class="ws-ava"></span><span class="ws-name"></span></button>
      <button class="sf-act" title="通知">${icon('bell', 18)}<span class="sf-dot" style="display:none"></span></button>
    </div>`;

  const WS = 'Personal';
  left.querySelector('.ws-name').textContent = WS;
  left.querySelector('.ws-ava').textContent = WS.trim().split(/\s+/).slice(0, 2).map(w => w[0]).join('').toUpperCase();

  // ===== SideBar 契约：外壳 ↔ 海洋侧栏内容 =====
  const sidebody = left.querySelector('#sidebody');
  const placeholder = (id, txt) => `<div style="flex:1;display:grid;place-items:center;color:var(--ink-3);font-size:var(--t-md)">${NAME[id] || id} · ${txt}</div>`;
  window.SideBar = {
    _render: {}, _cur: null,
    register(id, render) { this._render[id] = render; if (id === this._cur) render(sidebody); },
    mount(id) {
      this._cur = id;
      if (this._render[id]) { this._render[id](sidebody); return; }
      sidebody.innerHTML = `<div class="side-soon">${NAME[id] || id} 侧栏加载中…</div>`;
      if (window.loader) loader.loadFeature(id).then(() => { if (this._render[id]) this._render[id](sidebody); else sidebody.innerHTML = `<div class="side-soon">${NAME[id] || id} 侧栏设计中…</div>`; });
    },
    setUnread(has) { const d = left.querySelector('.sf-dot'); if (d) d.style.display = has ? '' : 'none'; },
    exitNotif() { exitNotif(); },
  };

  // ===== 导航中枢：四 tab 切海洋(侧栏+海面)；头像→设置 / 铃铛→通知（两轴镜像：均接管侧栏 #sidebody；设置另用海面铺类目详情）。 =====
  const seg = left.querySelector('.modeseg');
  const bell = left.querySelector('.sf-act');
  let cur = NAV.length ? NAV[0][0] : null;
  let _sideBack = null;

  const mountSea = id => {
    document.querySelectorAll('[data-ocean-head]').forEach(el => el.remove());
    if (Shell.oceans && Shell.oceans[id]) { Shell.mount(id); return Promise.resolve(true); }
    Shell.sea.innerHTML = placeholder(id, '加载中…');
    return (window.loader ? loader.loadFeature(id) : Promise.resolve(false)).then(() => {
      if (Shell.oceans && Shell.oceans[id]) { Shell.mount(id); return true; }
      Shell.sea.innerHTML = placeholder(id, '海面待接入'); return false;
    });
  };
  function nav(id) {
    cur = id;
    bell.classList.remove('on');
    seg.querySelectorAll('button').forEach(x => x.classList.toggle('on', x.dataset.m === id));
    SideBar.mount(id);
    return mountSea(id);
  }
  Shell.toOcean = nav;
  seg.querySelectorAll('button').forEach(b => b.onclick = () => nav(b.dataset.m));

  // 头像 = 设置入口（接管侧栏类目导航 features/settings/rail.js + 海面铺详情；镜像铃铛通知）
  left.querySelector('.ws').onclick = () => {
    Shell._back = cur;
    bell.classList.remove('on');
    seg.querySelectorAll('button').forEach(x => x.classList.remove('on'));
    SideBar.mount(AVATAR_ID);
    mountSea(AVATAR_ID);
  };

  // 铃铛 = 通知入口(侧栏接管轴，镜像头像)：#sidebody 换通知 Inbox、四 tab 熄灭、铃铛高亮，海面不动。
  function enterNotif() {
    peekDismiss();
    _sideBack = SideBar._cur || cur;
    seg.querySelectorAll('button').forEach(x => x.classList.remove('on'));
    bell.classList.add('on');
    SideBar.mount(BELL_ID);
  }
  function exitNotif() {
    bell.classList.remove('on');
    const back = _sideBack || cur || (NAV[0] && NAV[0][0]);
    seg.querySelectorAll('button').forEach(x => x.classList.toggle('on', x.dataset.m === back));
    SideBar.mount(back);
  }
  bell.onclick = () => bell.classList.contains('on') ? exitNotif() : enterNotif();

  // 左下角 peek：actionable 到达时冒极简 pill。点 pill/查看 → 进 Inbox。
  let peekTimer = null;
  function peekDismiss() { clearTimeout(peekTimer); const p = left.querySelector('.sf-peek'); if (p) { p.classList.remove('in'); setTimeout(() => p.remove(), 220); } }
  function peekShow(text) {
    if (bell.classList.contains('on')) return;
    peekDismiss(); SideBar.setUnread(true);
    const p = document.createElement('div'); p.className = 'sf-peek';
    p.innerHTML = `<span class="sf-peek-d"></span><span class="sf-peek-t">${text}</span><button class="sf-peek-go">查看</button><button class="sf-peek-x" title="忽略">${icon('close', 13)}</button>`;
    left.appendChild(p); setTimeout(() => p.classList.add('in'), 16);
    const go = () => { peekDismiss(); enterNotif(); };
    p.querySelector('.sf-peek-go').onclick = go; p.querySelector('.sf-peek-t').onclick = go;
    p.querySelector('.sf-peek-x').onclick = e => { e.stopPropagation(); peekDismiss(); };
    p.onmouseenter = () => clearTimeout(peekTimer);
    p.onmouseleave = () => { peekTimer = setTimeout(peekDismiss, 2500); };
    peekTimer = setTimeout(peekDismiss, 8000);
  }
  window.SideBar.peek = peekShow;
  setTimeout(() => peekShow('竞品动态日报流程 · 等待审批'), 2200);

  // ===== 收起/展开 + 拖拽调宽（状态/持久化全归侧栏；单一真相 = html[data-side] + --side-w） =====
  function toggle() { const off = html.dataset.side === 'off'; html.dataset.side = off ? 'on' : 'off'; localStorage.setItem('fg.side.collapsed', off ? '0' : '1'); }
  left.querySelector('[data-act="collapse"]').onclick = toggle;
  const reopen = document.createElement('button');
  reopen.className = 'ibtn side-reopen'; reopen.title = '展开侧栏'; reopen.innerHTML = icon('side', 18); reopen.onclick = toggle;
  Shell.headLead.appendChild(reopen);

  const grip = document.createElement('div'); grip.className = 'side-grip'; left.appendChild(grip);
  let sx = 0, sw = 0, dragging = false;
  grip.addEventListener('pointerdown', e => {
    if (html.dataset.side !== 'on') return;
    dragging = true; sx = e.clientX; sw = Shell.sideWidth || 240;
    html.dataset.sideDragging = ''; document.body.style.userSelect = 'none'; document.body.style.cursor = 'col-resize'; e.preventDefault();
  });
  window.addEventListener('pointermove', e => { if (!dragging) return; html.style.setProperty('--side-w', Math.max(240, Math.min(420, sw + (e.clientX - sx))) + 'px'); });
  window.addEventListener('pointerup', () => { if (!dragging) return; dragging = false; delete html.dataset.sideDragging; document.body.style.userSelect = ''; document.body.style.cursor = ''; localStorage.setItem('fg.side.w', Math.round(Shell.sideWidth)); });
})();

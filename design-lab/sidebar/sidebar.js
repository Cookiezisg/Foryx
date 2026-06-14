/* Forgify design-lab — 左侧栏模块（单独，一人负责）。
   自挂载进 Shell.left。依赖：shared/icons.js（icon）+ shared/shell.js（Shell.left）。只读它们、不改。
   注：模式段/Recents 现为示意；接后端时换成真数据，外壳契约不变。 */
(function () {
  const left = Shell.left;
  const html = document.documentElement;

  // 首帧无闪：注入前先据 localStorage 把终态宽度/收起态写到 <html>（避免 240→实际宽跳变）
  let w0 = parseInt(localStorage.getItem('fg.side.w'), 10);
  if (!(w0 >= 240 && w0 <= 420)) w0 = 240;                 // 脏值/NaN/旧窄值 回退(下限 240=最长标签 Documents 恒显)
  html.style.setProperty('--side-w', w0 + 'px');
  html.dataset.side = localStorage.getItem('fg.side.collapsed') === '1' ? 'off' : 'on';

  left.innerHTML = `
    <div class="side-top">
      <div class="lights"><span class="light r"></span><span class="light y"></span><span class="light g"></span></div>
      <span class="grow"></span>
      <button class="ibtn" data-i="side"></button>
      <button class="ibtn" data-i="search"></button>
    </div>
    <!-- 四导航(海洋切换器):Notion 式选中展开药丸。中间两个(Entities/Scheduler)的「侧栏内容」待定,
         此处只立切换器骨架;data-m 用真海洋 id,便于日后接 Shell.mount。命名待拍:Scheduler 偏窄(候选 Runs/Operate)。 -->
    <div class="modeseg" id="modeseg">
      <button class="on" data-m="chat"><span class="ico" data-i="chat"></span><span class="lbl">Chat</span></button>
      <button data-m="entities"><span class="ico" data-i="entities"></span><span class="lbl">Entities</span></button>
      <button data-m="scheduler"><span class="ico" data-i="scheduler"></span><span class="lbl">Scheduler</span></button>
      <button data-m="documents"><span class="ico" data-i="doc"></span><span class="lbl">Documents</span></button>
    </div>
    <!-- 海洋专属内容区:据顶部四导航切换。当前实现 Chat(会话史);其余海洋待设计,占位。 -->
    <div id="sidebody"></div>
    <!-- 底部:工作区(圆头像+名,点切换、无箭头) + 通知 + 设置。学 Claude Code 用户行清爽;英文名;铃铛/齿轮 15px 同顶部导航。 -->
    <div class="sfoot">
      <button class="ws" title="Switch workspace">
        <span class="ws-ava" id="ws-ava"></span>
        <span class="ws-name" id="ws-name"></span>
      </button>
      <button class="sf-act" title="Notifications"><span data-i="bell"></span><span class="sf-dot"></span></button>
      <button class="sf-act" title="Settings"><span data-i="gear"></span></button>
    </div>`;

  const sz = { side: 18, search: 18, chat: 18, entities: 18, scheduler: 18, doc: 18, bell: 18, gear: 18 };   // 仅静态结构用;Chat 内容区的图标在 buildChat 里直接 icon() 调
  left.querySelectorAll('[data-i]').forEach(el => { const k = el.dataset.i; el.innerHTML = icon(k, sz[k] || 18); });

  // 工作区身份(示意;接后端换真 workspace)。头像 = 名字首字母(最多两词)
  const WS = 'Personal';
  left.querySelector('#ws-name').textContent = WS;
  left.querySelector('#ws-ava').textContent = WS.trim().split(/\s+/).slice(0, 2).map(w => w[0]).join('').toUpperCase();

  // ===== 海洋专属侧栏内容（据四导航切换；当前实现 Chat，其余海洋占位） =====
  const body = left.querySelector('#sidebody');
  const OCEAN_NAME = { entities: 'Entities', scheduler: 'Scheduler', documents: 'Documents' };

  // 展示选项状态（filter 行 sliders 控制，作用于全部对话）
  let dispSort = 'recent', dispGroup = true, dispTime = false;

  // Chat 会话史（示意数据）。接后端：列表/置顶/归档/标题滤/排序(List sort) 已有；运行点=B3 isGenerating；分组+时间=B4 last_message_at。
  function buildChat() {
    const PINNED = [{ t: '竞品动态日报流程' }, { t: 'Researcher agent 调优', on: true }];
    const GROUPS = [
      ['Today', [{ t: '修复 CEL 校验器', st: 'run', time: '14:32' }, { t: 'Webhook 入库 handler', st: 'wait', time: '11:08' }]],
      ['Yesterday', [{ t: '周报自动汇总 workflow', st: 'err', time: 'Tue' }, { t: '文档问答 agent', st: 'unread', time: 'Tue' }]],
      ['Previous 7 days', [{ t: '账单对账流程', time: 'Jun 9' }, { t: 'Slack 通知 trigger', time: 'Jun 8' }, { t: 'PDF 提取 function', time: 'Jun 7' }]],
      ['Older', [{ t: 'Notion 同步实验', time: 'May 28' }, { t: '旧版迁移笔记', time: 'May 20' }]],
    ];
    const ARCHIVED = [{ t: '临时调试 agent' }, { t: '废弃的爬虫流程' }, { t: '一次性数据清洗' }];
    // 行：状态点在首(空心=闲置 / accent脉冲=生成中 / 琥珀=等你 / 红=失败 / 实心灰=未读)+ 标题 + 时间戳(可选) + 悬浮 ⋯
    const row = c => `<div class="cv${c.on ? ' on' : ''}"><span class="cv-st${c.st ? ' ' + c.st : ''}"></span><span class="t">${c.t}</span>${c.time ? `<span class="cv-time">${c.time}</span>` : ''}<span class="cv-more">${icon('more', 16)}</span></div>`;
    const opt = (attr, val, on, label) => `<button class="cdisp-opt${on ? ' on' : ''}" ${attr}="${val}"><span class="ck">${icon('check', 14)}</span>${label}</button>`;
    // 空区不渲染（无置顶/无归档则该区整段不出）
    const pinned = PINNED.length ? `
        <div class="cvsec collapsible open">
          <button class="cvsec-h cvsec-tog"><span class="t">Pinned</span><span class="chev">${icon('chevr', 14)}</span></button>
          <div class="cvsec-body">${PINNED.map(row).join('')}</div>
        </div>` : '';
    const recents = `
        <div class="cvsec">
          <div class="cvsec-h"><span class="t">Recents</span></div>
          <div class="cvsec-body">${GROUPS.map(([l, items]) => `<div class="cvsub">${l}</div>${items.map(row).join('')}`).join('')}</div>
        </div>`;
    const archived = ARCHIVED.length ? `
        <div class="cvsec collapsible">
          <button class="cvsec-h cvsec-tog"><span class="t">Archived</span><span class="cnt">${ARCHIVED.length}</span><span class="chev">${icon('chevr', 14)}</span></button>
          <div class="cvsec-body">${ARCHIVED.map(row).join('')}</div>
        </div>` : '';
    return `
      <button class="newconv">${icon('plus', 18)} New conversation</button>
      <div class="cfilter">${icon('search', 16)}<input type="text" placeholder="Filter conversations…">
        <button class="cdisp" title="Display options">${icon('sliders', 16)}</button>
        <div class="cdisp-menu">
          <div class="cdisp-h">Sort by</div>
          ${opt('data-sort', 'recent', dispSort === 'recent', 'Recent activity')}
          ${opt('data-sort', 'created', dispSort === 'created', 'Date created')}
          ${opt('data-sort', 'title', dispSort === 'title', 'Title A–Z')}
          <div class="cdisp-h">Display</div>
          ${opt('data-toggle', 'group', dispGroup, 'Group by date')}
          ${opt('data-toggle', 'time', dispTime, 'Show timestamps')}
        </div>
      </div>
      <div class="cvlist${dispGroup ? '' : ' no-group'}${dispTime ? ' show-time' : ''}">
        ${pinned}${recents}${archived}
      </div>`;
  }

  function wireChat() {
    // Pinned/Archived 折叠
    body.querySelectorAll('.cvsec-tog').forEach(h => h.onclick = () => h.closest('.collapsible').classList.toggle('open'));
    // 选中对话
    body.querySelectorAll('.cv').forEach(it => it.onclick = e => {
      if (e.target.closest('.cv-more')) return;
      body.querySelectorAll('.cv').forEach(x => x.classList.remove('on')); it.classList.add('on');
    });
    // 展示菜单（filter 行 sliders，作用于全部）：排序(单选,示意) + 分组/时间戳(开关,实时改 .cvlist 类)
    const disp = body.querySelector('.cdisp'), dmenu = body.querySelector('.cdisp-menu'), clist = body.querySelector('.cvlist');
    disp.onclick = e => { e.stopPropagation(); const open = dmenu.classList.toggle('open'); disp.classList.toggle('on', open); };
    dmenu.addEventListener('click', e => e.stopPropagation());   // 菜单内点击不冒泡（不被外部收起）
    dmenu.querySelectorAll('[data-sort]').forEach(o => o.onclick = () => {
      dmenu.querySelectorAll('[data-sort]').forEach(x => x.classList.remove('on')); o.classList.add('on'); dispSort = o.dataset.sort;
    });
    dmenu.querySelector('[data-toggle="group"]').onclick = function () { dispGroup = !dispGroup; this.classList.toggle('on', dispGroup); clist.classList.toggle('no-group', !dispGroup); };
    dmenu.querySelector('[data-toggle="time"]').onclick = function () { dispTime = !dispTime; this.classList.toggle('on', dispTime); clist.classList.toggle('show-time', dispTime); };
    // 标题快滤
    const fin = body.querySelector('.cfilter input');
    fin.oninput = () => {
      const q = fin.value.trim().toLowerCase();
      body.querySelectorAll('.cvsec-body .cv').forEach(cv => {
        cv.style.display = cv.querySelector('.t').textContent.toLowerCase().includes(q) ? '' : 'none';
      });
    };
  }

  function renderBody(ocean) {
    if (ocean === 'chat') { body.innerHTML = buildChat(); wireChat(); }
    else { body.innerHTML = `<div class="side-soon">${OCEAN_NAME[ocean] || ocean} 侧栏设计中…</div>`; }
  }

  const seg = left.querySelector('#modeseg');
  seg.querySelectorAll('button').forEach(b => b.onclick = () => {
    seg.querySelectorAll('button').forEach(x => x.classList.remove('on'));
    b.classList.add('on');
    renderBody(b.dataset.m);
  });
  renderBody('chat');   // 默认进 Chat 海洋

  // 点菜单外收起展示选项菜单（一次性挂载；body 重渲染后查询仍有效）
  document.addEventListener('click', () => {
    const m = body.querySelector('.cdisp-menu.open');
    if (m) { m.classList.remove('open'); body.querySelector('.cdisp')?.classList.remove('on'); }
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
    const next = Math.max(240, Math.min(420, sw + (e.clientX - sx)));   // clamp[240,420]（下限 240 保英文标签恒显）
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

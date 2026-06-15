/* Forgify design-lab — 【Chat 海洋】的左侧栏内容（独立文件，一人负责；与外壳/别的海洋解耦）。
   外壳 sidebar.js 据四导航按需懒加载本文件；本文件自注入 chat.css，经 SideBar.register('chat', render) 挂载。
   只碰 render(host) 给的宿主元素。依赖 icon()（只读）。
   会话史：新建 + 标题快滤 + 展示选项(排序/分组/时间戳) + 三区 Pinned/Recents/Archived + 行首状态点。 */
(function () {
  // 自注入样式（自包含模块，只加载一次）
  const dir = new URL('.', document.currentScript.src).href;
  if (!document.querySelector('link[data-sb="chat"]')) {
    const l = document.createElement('link');
    l.rel = 'stylesheet'; l.href = dir + 'chat.css'; l.dataset.sb = 'chat';
    document.head.appendChild(l);
  }

  // 示意数据。接后端：列表/置顶/归档/标题滤/排序(List sort) 已有；状态点=B3 isGenerating + humanloop；分组+时间戳=B4 last_message_at。
  // 每个会话绑一个 chat 场景 id（→ window.ChatOcean.open）。id 缺省的行只选中、不切场景（安全 no-op）。
  const PINNED = [{ t: '竞品动态日报流程', id: 'wf-weekly-report', on: true }, { t: 'Researcher agent 调优', id: 'agent-researcher-tune' }];
  const GROUPS = [
    ['Today', [{ t: '修复 CEL 校验器', st: 'run', time: '14:32', id: 'control-cel-fix' }, { t: 'Webhook 入库 handler', st: 'wait', time: '11:08', id: 'webhook-handler' }]],
    ['Yesterday', [{ t: '周报自动汇总 workflow', st: 'done', time: 'Tue', id: 'wf-weekly-report' }, { t: '文档问答 agent', st: 'done', time: 'Tue', id: 'document-qa-agent' }]],
    ['Previous 7 days', [{ t: '发布前过目 approval', time: 'Jun 9', id: 'approval-publish-gate' }, { t: 'Slack 通知 trigger', time: 'Jun 8', id: 'trigger-slack-webhook' }, { t: 'PDF 提取 function', time: 'Jun 7', id: 'fn-pdf-extract' }]],
    ['Older', [{ t: 'Notion 同步实验', time: 'May 28', id: 'mcp-notion-sync' }, { t: '账单对账流程', time: 'Jun 9', id: 'wf-weekly-report' }, { t: '旧版迁移笔记', time: 'May 20' }]],
  ];
  const ARCHIVED = [{ t: '临时调试 agent' }, { t: '废弃的爬虫流程' }, { t: '一次性数据清洗' }];

  // 展示选项状态（filter 行 sliders 控制，作用于全部；跨重绘保留）
  let sort = 'recent', group = true, showTime = false;

  // 行：状态点在首(空心=闲置 / 蓝脉冲=生成中 / 橙脉冲=提问 / 红=失败 / 绿=待查看) + 标题 + 时间戳(可选) + 悬浮 ⋯
  const row = c => `<div class="cv${c.on ? ' on' : ''}" data-cid="${c.id || ''}"><span class="cv-st ${c.st || 'idle'}"></span><span class="cv-t">${c.t}</span><span class="cv-time">${c.time || ''}</span><span class="cv-more">${icon('more', 16)}</span></div>`;
  const opt = (k, v, on, label) => `<button class="cm-opt${on ? ' on' : ''}" data-${k}="${v}"><span class="cm-ck">${icon('check', 14)}</span>${label}</button>`;
  const sec = (cls, head, rows) => `<div class="cvsec ${cls}">${head}<div class="cvsec-body">${rows}</div></div>`;

  function build() {
    const pinned = PINNED.length ? sec('collapsible open',
      `<button class="cvsec-h tog"><span class="cvsec-t">Pinned</span><span class="chev">${icon('chevr', 14)}</span></button>`,
      PINNED.map(row).join('')) : '';
    const recents = sec('',
      `<div class="cvsec-h"><span class="cvsec-t">Recents</span></div>`,
      GROUPS.map(([l, items]) => `<div class="cvsub">${l}</div>${items.map(row).join('')}`).join(''));
    const archived = ARCHIVED.length ? sec('collapsible',
      `<button class="cvsec-h tog"><span class="cvsec-t">Archived</span><span class="cnt">${ARCHIVED.length}</span><span class="chev">${icon('chevr', 14)}</span></button>`,
      ARCHIVED.map(row).join('')) : '';
    return `
      <button class="newconv">${icon('plus', 18)}<span>New conversation</span></button>
      <div class="cfilter">${icon('search', 16)}<input type="text" placeholder="Filter conversations…">
        <button class="cm-btn" title="Display options">${icon('sliders', 16)}</button>
        <div class="cm-menu">
          <div class="cm-h">Sort by</div>
          ${opt('sort', 'recent', sort === 'recent', 'Recent activity')}
          ${opt('sort', 'created', sort === 'created', 'Date created')}
          ${opt('sort', 'title', sort === 'title', 'Title A–Z')}
          <div class="cm-h">Display</div>
          ${opt('toggle', 'group', group, 'Group by date')}
          ${opt('toggle', 'time', showTime, 'Show timestamps')}
        </div>
      </div>
      <div class="cvlist${group ? '' : ' no-group'}${showTime ? ' show-time' : ''}">${pinned}${recents}${archived}</div>`;
  }

  function render(host) {
    host.innerHTML = build();
    // 折叠 Pinned/Archived
    host.querySelectorAll('.tog').forEach(h => h.onclick = () => h.closest('.collapsible').classList.toggle('open'));
    // 选中对话
    host.querySelectorAll('.cv').forEach(it => it.onclick = e => {
      if (e.target.closest('.cv-more')) return;
      host.querySelectorAll('.cv').forEach(x => x.classList.remove('on')); it.classList.add('on');
      const cid = it.dataset.cid;   // 绑定的 chat 场景；标题用行标签覆盖（多会话复用同场景各显其名）。ChatOcean 未就绪则只选中（安全）
      if (cid && window.ChatOcean) window.ChatOcean.open(cid, it.querySelector('.cv-t').textContent);
    });
    // 展示菜单（filter 行 sliders，作用于全部）：排序单选(示意) + 分组/时间戳开关(实时改 .cvlist 类)
    const btn = host.querySelector('.cm-btn'), menu = host.querySelector('.cm-menu'), list = host.querySelector('.cvlist');
    btn.onclick = e => { e.stopPropagation(); const open = menu.classList.toggle('open'); btn.classList.toggle('on', open); };
    menu.addEventListener('click', e => e.stopPropagation());
    menu.querySelectorAll('[data-sort]').forEach(o => o.onclick = () => {
      menu.querySelectorAll('[data-sort]').forEach(x => x.classList.remove('on')); o.classList.add('on'); sort = o.dataset.sort;
    });
    menu.querySelector('[data-toggle="group"]').onclick = function () { group = !group; this.classList.toggle('on', group); list.classList.toggle('no-group', !group); };
    menu.querySelector('[data-toggle="time"]').onclick = function () { showTime = !showTime; this.classList.toggle('on', showTime); list.classList.toggle('show-time', showTime); };
    // 标题快滤
    const fin = host.querySelector('.cfilter input');
    fin.oninput = () => {
      const q = fin.value.trim().toLowerCase();
      host.querySelectorAll('.cvsec-body .cv').forEach(cv => { cv.style.display = cv.querySelector('.cv-t').textContent.toLowerCase().includes(q) ? '' : 'none'; });
    };
  }

  // 点菜单外收起展示菜单（一次性）
  document.addEventListener('click', () => {
    const m = document.querySelector('#sidebody .cm-menu.open');
    if (m) { m.classList.remove('open'); document.querySelector('#sidebody .cm-btn')?.classList.remove('on'); }
  });

  SideBar.register('chat', render);
})();

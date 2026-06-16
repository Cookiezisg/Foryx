/* Foryx demo — 对话海洋侧栏内容：会话史（薄）。
   新建 + 标题快滤 + 展示选项(排序/分组/时间戳) + 三区 Pinned/Recents/Archived + 行首状态点（组件 StatusDot）。
   点行 → Intent.select({kind:'conversation', id, title}) → sea.js Intent.on('conversation') 播放该脚本。依赖 mock/conversations.js。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);
  const S = () => (window.MOCK_CONVERSATIONS || {}).sessions || { pinned: [], groups: [], archived: [] };

  // 展示选项（filter 行 sliders；跨重绘保留）。showTime 默认开：时间戳常驻靠右，hover 让位给 ⋯（Notion 式 meta↔action 同槽）
  let sort = 'recent', group = true, showTime = true;

  // 行：状态点（StatusDot 通用 5 态）+ 标题 + 时间戳(可选) + 悬浮 ⋯
  const row = c => `<div class="chat-cv${c.on ? ' on' : ''}" data-cid="${c.id || ''}">${StatusDot.dot(c.st || 'idle')}<span class="chat-cv-t">${c.title}</span><span class="chat-cv-time">${c.time || ''}</span><span class="chat-cv-more">${icon('more', 16)}</span></div>`;
  const opt = (k, v, on, label) => `<button class="chat-cm-opt${on ? ' on' : ''}" data-${k}="${v}"><span class="chat-cm-ck">${icon('check', 14)}</span>${label}</button>`;
  const sec = (cls, head, rows) => `<div class="chat-cvsec ${cls}">${head}<div class="chat-cvsec-body">${rows}</div></div>`;

  function build() {
    const s = S();
    const pinned = (s.pinned || []).length ? sec('collapsible open',
      `<button class="chat-cvsec-h tog"><span class="chat-cvsec-t">Pinned</span><span class="chat-chev">${icon('chevr', 14)}</span></button>`,
      s.pinned.map(row).join('')) : '';
    const recents = sec('',
      `<div class="chat-cvsec-h"><span class="chat-cvsec-t">Recents</span></div>`,
      (s.groups || []).map(([l, items]) => `<div class="chat-cvsub">${l}</div>${items.map(row).join('')}`).join(''));
    const archived = (s.archived || []).length ? sec('collapsible',
      `<button class="chat-cvsec-h tog"><span class="chat-cvsec-t">Archived</span><span class="chat-cnt">${s.archived.length}</span><span class="chat-chev">${icon('chevr', 14)}</span></button>`,
      s.archived.map(row).join('')) : '';
    return `
      <button class="chat-newconv">${icon('plus', 18)}<span>New conversation</span></button>
      <div class="chat-cfilter">${icon('search', 16)}<input type="text" placeholder="Filter conversations…">
        <button class="chat-cm-btn" title="Display options">${icon('sliders', 16)}</button>
        <div class="chat-cm-menu">
          <div class="chat-cm-h">Sort by</div>
          ${opt('sort', 'recent', sort === 'recent', 'Recent activity')}
          ${opt('sort', 'created', sort === 'created', 'Date created')}
          ${opt('sort', 'title', sort === 'title', 'Title A–Z')}
          <div class="chat-cm-h">Display</div>
          ${opt('toggle', 'group', group, 'Group by date')}
          ${opt('toggle', 'time', showTime, 'Show timestamps')}
        </div>
      </div>
      <div class="chat-cvlist${group ? '' : ' no-group'}${showTime ? ' show-time' : ''}">${pinned}${recents}${archived}</div>`;
  }

  function render(host) {
    host.innerHTML = build();

    // 折叠 Pinned/Archived
    host.querySelectorAll('.tog').forEach(h => h.onclick = () => h.closest('.collapsible').classList.toggle('open'));

    // 选中对话 → Intent.select（标题用行标签覆盖：多会话复用同脚本各显其名）。id 缺省的行只选中、不切场景
    host.querySelectorAll('.chat-cv').forEach(it => it.onclick = e => {
      if (e.target.closest('.chat-cv-more')) return;
      host.querySelectorAll('.chat-cv').forEach(x => x.classList.remove('on')); it.classList.add('on');
      const cid = it.dataset.cid;
      if (cid) Intent.select({ kind: 'conversation', id: cid, title: it.querySelector('.chat-cv-t').textContent });
    });

    // 展示菜单（排序单选示意 + 分组/时间戳开关实时改 .chat-cvlist 类）
    const btn = host.querySelector('.chat-cm-btn'), menu = host.querySelector('.chat-cm-menu'), list = host.querySelector('.chat-cvlist');
    btn.onclick = e => { e.stopPropagation(); const open = menu.classList.toggle('open'); btn.classList.toggle('on', open); };
    menu.addEventListener('click', e => e.stopPropagation());
    menu.querySelectorAll('[data-sort]').forEach(o => o.onclick = () => {
      menu.querySelectorAll('[data-sort]').forEach(x => x.classList.remove('on')); o.classList.add('on'); sort = o.dataset.sort;
    });
    menu.querySelector('[data-toggle="group"]').onclick = function () { group = !group; this.classList.toggle('on', group); list.classList.toggle('no-group', !group); };
    menu.querySelector('[data-toggle="time"]').onclick = function () { showTime = !showTime; this.classList.toggle('on', showTime); list.classList.toggle('show-time', showTime); };

    // 标题快滤
    const fin = host.querySelector('.chat-cfilter input');
    fin.oninput = () => {
      const q = fin.value.trim().toLowerCase();
      host.querySelectorAll('.chat-cvsec-body .chat-cv').forEach(cv => { cv.style.display = cv.querySelector('.chat-cv-t').textContent.toLowerCase().includes(q) ? '' : 'none'; });
    };

    // 默认高亮首个 pinned（与 sea 默认开同步）
    const first = host.querySelector('.chat-cv'); if (first && !host.querySelector('.chat-cv.on')) first.classList.add('on');

    // 点菜单外收起展示菜单
    document.addEventListener('click', () => { const m = host.querySelector('.chat-cm-menu.open'); if (m) { m.classList.remove('open'); const b = host.querySelector('.chat-cm-btn.on'); if (b) b.classList.remove('on'); } });
  }

  SideBar.register('chat', render);
})();

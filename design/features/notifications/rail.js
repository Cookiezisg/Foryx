/* Foryx demo — 【通知 Inbox】侧栏接管内容（铃铛轴；薄组合）。
   外壳 core/sidebar.js 的铃铛点击 → SideBar.mount('notifications') 接管 #sidebody（镜像 settings 接管海面）。
   本文件经 SideBar.register('notifications', render) 挂载；只碰 render(host) 宿主 + 外壳暴露的 SideBar.exitNotif / setUnread。
   组合组件：StatusDot（行首 5 态点；未读黑、已读灰，靠字色明暗分主次——不上药丸、不全黑）。
   点普通行 → Intent.select 到被提及实体/run（跨海洋唯一前门，走行 data-ref）；拥有 kind=notification（Intent.on 兜底接管 Inbox）。
   依赖 mock/notifications.js（window.MOCK_NOTIFICATIONS，先于本文件加载；缺失则空态）。类名 ntf- 专属，只读令牌。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);
  const D = () => window.MOCK_NOTIFICATIONS || { needs: [], groups: [], read: [] };

  // 展示状态（filter 行 + 分组开关；跨重绘保留）
  let flt = 'all', group = true, readOpen = false;

  // 普通 FYI 行：状态点 + 单行文案 + 时间 + 悬浮 ⋯（实体引用并入行点击路由，不再独占药丸行——精简去 overwhelm）
  const row = n => `<div class="ntf-row${n.unread ? ' unread' : ''}" data-id="${n.id}" data-kind="${n.refKind || ''}" data-ref="${n.refId || ''}">
      <span class="ntf-st">${StatusDot.dot(n.st || 'idle')}</span>
      <span class="ntf-body"><span class="ntf-t">${n.title}</span></span>
      <span class="ntf-time">${n.time || ''}</span>
      <span class="ntf-more" title="更多">${icon('more', 16)}</span>
    </div>`;

  // Needs you 胖行：wait 橙点表「等你」+ 标题；展开后平铺 prompt + 就地拍板（中性浅底卡，非 warn 彩盒——不全黑、不报警色）
  const fatRow = a => {
    const inner = `<div class="ntf-prompt">${a.prompt || ''}</div>
      ${a.ddl ? `<div class="ntf-ddl">${icon('scheduler', 13)}${a.ddl}</div>` : ''}
      <div class="ntf-acts"><button class="ntf-btn go" data-act="approve">批准</button><button class="ntf-btn" data-act="deny">驳回</button>${a.refKind ? `<span class="ntf-open" data-act="open">在 Scheduler 打开</span>` : ''}</div>`;
    return `<div class="ntf-fat" data-id="${a.id}" data-kind="${a.refKind || ''}" data-ref="${a.refId || ''}">
      <div class="ntf-fat-top"><span class="ntf-st">${StatusDot.dot('wait')}</span><span class="ntf-fat-title">${a.title}</span><span class="ntf-time">${a.time || ''}</span><span class="ntf-chev">${icon('chevr', 14)}</span></div>
      <div class="ntf-approve">${inner}</div>
    </div>`;
  };

  const opt = (k, v, on, label) => `<button class="ntf-opt${on ? ' on' : ''}" data-${k}="${v}"><span class="ntf-ck">${icon('check', 14)}</span>${label}</button>`;
  const sec = (cls, head, body) => `<div class="ntf-sec ${cls}">${head}<div class="ntf-sec-body">${body}</div></div>`;

  function build() {
    const d = D();
    const needs = (d.needs && d.needs.length) ? sec('needs',
      `<div class="ntf-sec-h"><span class="ntf-sec-t">Needs you</span><span class="ntf-cnt">${d.needs.length}</span></div>`,
      d.needs.map(fatRow).join('')) : '';
    const timeline = sec('timeline',
      `<div class="ntf-sec-h"><span class="ntf-sec-t">时间线</span></div>`,
      (d.groups || []).map(g => `<div class="ntf-sub">${g.label}</div>${(g.items || []).map(row).join('')}`).join(''));
    const read = (d.read && d.read.length) ? sec('read collapsible' + (readOpen ? ' open' : ''),
      `<button class="ntf-sec-h tog"><span class="ntf-sec-t">已读</span><span class="ntf-cnt">${d.read.length}</span><span class="ntf-chev">${icon('chevr', 14)}</span></button>`,
      d.read.map(row).join('')) : '';
    const fltCls = flt === 'unread' ? ' only-unread' : flt === 'action' ? ' only-action' : '';
    return `
      <div class="ntf-head"><button class="ntf-back" title="返回">${icon('chevr', 18)}</button><span class="ntf-title">通知</span><button class="ntf-allread">全部已读</button></div>
      <div class="ntf-filter">${icon('search', 16)}<input type="text" placeholder="筛选通知…">
        <button class="ntf-mbtn" title="显示选项">${icon('sliders', 16)}</button>
        <div class="ntf-menu">
          <div class="ntf-mh">筛选</div>
          ${opt('flt', 'all', flt === 'all', '全部')}
          ${opt('flt', 'unread', flt === 'unread', '仅未读')}
          ${opt('flt', 'action', flt === 'action', '仅待决')}
          <div class="ntf-mh">显示</div>
          ${opt('toggle', 'group', group, '按时间分组')}
        </div>
      </div>
      <div class="ntf-list${group ? '' : ' no-group'}${fltCls}">${needs}${timeline}${read}</div>`;
  }

  function render(host) {
    host.innerHTML = build();
    const list = host.querySelector('.ntf-list');
    const refreshUnread = () => window.SideBar && SideBar.setUnread && SideBar.setUnread(!!host.querySelector('.ntf-row.unread, .ntf-fat'));

    // ← 返回 → 退出接管、回到来源海洋（外壳暴露；chevr 旋 180° 作返回箭头）
    host.querySelector('.ntf-back').onclick = () => window.SideBar && SideBar.exitNotif && SideBar.exitNotif();
    // 全部已读
    host.querySelector('.ntf-allread').onclick = () => { host.querySelectorAll('.ntf-row.unread').forEach(r => r.classList.remove('unread')); refreshUnread(); };

    // 折叠「已读」（记住开合态）
    host.querySelectorAll('.tog').forEach(h => h.onclick = () => { const s = h.closest('.collapsible'); readOpen = s.classList.toggle('open'); });

    // 普通行：点击 = 标记已读 + 跳被提及实体/run（引用走整行 data-ref，无独立药丸）
    host.querySelectorAll('.ntf-row').forEach(r => r.onclick = () => {
      r.classList.remove('unread'); refreshUnread();
      if (r.dataset.ref) Intent.select({ kind: r.dataset.kind, id: r.dataset.ref });
    });

    // Needs you 胖行：点 top 展开/收起拍板；批准/驳回 = 就地拍板；在 Scheduler 打开 = 深链
    host.querySelectorAll('.ntf-fat').forEach(fat => {
      fat.querySelector('.ntf-fat-top').onclick = () => fat.classList.toggle('open');
      fat.querySelectorAll('[data-act]').forEach(b => b.onclick = e => {
        e.stopPropagation();
        const act = b.dataset.act;
        if (act === 'open') { if (fat.dataset.ref) Intent.select({ kind: fat.dataset.kind, id: fat.dataset.ref }); return; }
        decide(host, fat, act === 'approve');
      });
    });

    // sliders 菜单：筛选单选 + 分组开关（实时改 .ntf-list 类，不重绘）
    const btn = host.querySelector('.ntf-mbtn'), menu = host.querySelector('.ntf-menu');
    btn.onclick = e => { e.stopPropagation(); const open = menu.classList.toggle('open'); btn.classList.toggle('on', open); };
    menu.addEventListener('click', e => e.stopPropagation());
    menu.querySelectorAll('[data-flt]').forEach(o => o.onclick = () => {
      menu.querySelectorAll('[data-flt]').forEach(x => x.classList.remove('on')); o.classList.add('on'); flt = o.dataset.flt;
      list.classList.toggle('only-unread', flt === 'unread'); list.classList.toggle('only-action', flt === 'action');
    });
    menu.querySelector('[data-toggle="group"]').onclick = function () { group = !group; this.classList.toggle('on', group); list.classList.toggle('no-group', !group); };

    // 标题快滤
    const fin = host.querySelector('.ntf-filter input');
    fin.oninput = () => {
      const q = fin.value.trim().toLowerCase();
      host.querySelectorAll('.ntf-row, .ntf-fat').forEach(it => { it.style.display = it.querySelector('.ntf-t, .ntf-fat-title').textContent.toLowerCase().includes(q) ? '' : 'none'; });
    };

    refreshUnread();
  }

  // 就地拍板：移出 Needs you、降级为一条 done FYI 进时间线（留痕，镜像后端「决定后该行降级」）
  function decide(host, fat, approve) {
    const wf = fat.querySelector('.ntf-fat-title').textContent;
    const s = fat.closest('.ntf-sec');
    fat.remove();
    const rest = s.querySelectorAll('.ntf-fat').length;
    if (!rest) s.remove(); else s.querySelector('.ntf-cnt').textContent = rest;
    const tl = host.querySelector('.ntf-sec.timeline .ntf-sec-body');
    if (tl) {
      const tmp = document.createElement('div');   // 已决 = 对自己动作的留痕，非未读（不重新点亮红点）
      tmp.innerHTML = row({ id: 'ntf_act_' + Date.now(), title: `${approve ? '已批准' : '已驳回'} · ${wf}`, st: 'done', time: '刚刚' });
      const firstSub = tl.querySelector('.ntf-sub');
      tl.insertBefore(tmp.firstElementChild, firstSub ? firstSub.nextSibling : tl.firstChild);
    }
    window.SideBar && SideBar.setUnread && SideBar.setUnread(!!host.querySelector('.ntf-row.unread, .ntf-fat'));
  }

  // 点菜单外收起（一次性常驻监听）
  document.addEventListener('click', () => {
    const m = document.querySelector('#sidebody .ntf-menu.open');
    if (m) { m.classList.remove('open'); const b = document.querySelector('#sidebody .ntf-mbtn'); if (b) b.classList.remove('on'); }
  });

  // 拥有 kind=notification：别处 Intent.select({kind:'notification'}) → 接管 Inbox（铃铛轴；兜底）
  Intent.on('notification', () => window.SideBar && SideBar.mount && SideBar.mount('notifications'));

  SideBar.register('notifications', render);
})();

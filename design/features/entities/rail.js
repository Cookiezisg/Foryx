/* Foryx demo — 实体海洋侧栏内容：竖向折叠树（分组 → 类型 → 实体行，薄）。
   形态对齐 design-lab：New + 搜索(含 sliders 排序菜单) → 分组(可折叠) → 类型(可展开) → 行首状态点 + 名 + meta。
   行点击 → Intent.select({kind:'entity'}) → sea.js morph 成该实体。状态点走组件 StatusDot；skill 无状态点（文件式），标 allowed-tools 数。
   分组/类型表静态（= 后端各域），实体行据 mock/entities.js 按 kind 归桶派生。类名全 ent- 前缀（NO fg-）。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);
  const D = () => window.MOCK_ENTITIES || {};

  // 分组 → [类型 kind, 标签, 图标]。approval 借 shield。
  const GROUPS = [
    ['Quadrinity', [['function', 'Functions', 'function'], ['handler', 'Handlers', 'handler'], ['agent', 'Agents', 'agent'], ['workflow', 'Workflows', 'workflow']]],
    ['Graph parts', [['trigger', 'Triggers', 'trigger'], ['control', 'Controls', 'control'], ['approval', 'Approvals', 'shield']]],
    ['Connections', [['mcp', 'MCP', 'mcp']]],
    ['Skills', [['skill', 'Skills', 'skill']]],
  ];
  const DEFAULT_OPEN = new Set(['function']);   // 默认展开 Functions（与 sea 默认开 process_invoice 同步）

  let sort = 'recent';
  // 显示开关（per-row 版本号/标记 meta + per-header 计数）——默认都显：靠右常驻，行 hover 时 meta 让位给 ⋯（Notion 式同槽，不再抢空间）；sliders 菜单可逐项关
  let showVersion = true, showCount = true;
  // 按 kind 取实体列表（保 mock 声明顺序）
  const ofKind = kind => Object.keys(D()).filter(name => D()[name].kind === kind);

  function rowHtml(name) {
    const e = D()[name];
    const dot = e.kind === 'skill' ? `<span class="ent-r-st-none"></span>` : StatusDot.dot(e.status || 'idle');
    const meta = e.kind === 'skill'
      ? `<span class="ent-r-meta">⚷ ${(e.allowed || []).length}</span>`
      : (e.version != null ? `<span class="ent-r-meta">v${e.version}</span>` : '');
    const on = name === 'process_invoice' ? ' on' : '';
    return `<div class="ent-r${on}" data-id="${name}">${dot}<span class="ent-r-t">${name}</span>${meta}<span class="ent-r-more">${icon('more', 16)}</span></div>`;
  }
  const sortOpt = (v, label) => `<button class="ent-opt${sort === v ? ' on' : ''}" data-sort="${v}"><span class="ent-ck">${icon('check', 14)}</span>${label}</button>`;
  const dispOpt = (v, on, label) => `<button class="ent-opt${on ? ' on' : ''}" data-disp="${v}"><span class="ent-ck">${icon('check', 14)}</span>${label}</button>`;

  const typeSec = ([kind, label, ic]) => {
    const items = ofKind(kind);
    return `<div class="ent-ty collapsible${DEFAULT_OPEN.has(kind) ? ' open' : ''}">
      <button class="ent-tog ent-ty-h"><span class="ent-ty-lead"><span class="ent-ty-ico">${icon(ic, 16)}</span><span class="ent-chev">${icon('chevr', 14)}</span></span><span class="ent-lbl">${label}</span><span class="ent-cnt">${items.length}</span></button>
      <div class="ent-cbody">${items.map(rowHtml).join('')}</div></div>`;
  };
  const groupSec = ([g, types]) => {
    const total = types.reduce((nn, [kind]) => nn + ofKind(kind).length, 0);
    return `<div class="ent-grp collapsible open">
      <button class="ent-tog ent-grp-h"><span class="ent-lbl">${g}</span><span class="ent-cnt">${total}</span><span class="ent-chev">${icon('chevr', 13)}</span></button>
      <div class="ent-cbody">${types.map(typeSec).join('')}</div></div>`;
  };

  function render(host) {
    host.innerHTML = `
      <button class="ent-new">${icon('plus', 18)}<span>New entity</span></button>
      <div class="ent-filter">${icon('search', 16)}<input type="text" placeholder="Search entities…">
        <button class="ent-disp" title="Sort & display">${icon('sliders', 16)}</button>
        <div class="ent-menu">
          <div class="ent-mh">Sort by</div>
          ${sortOpt('recent', 'Recent activity')}
          ${sortOpt('name', 'Name A–Z')}
          ${sortOpt('type', 'Type')}
          <div class="ent-mh">Display</div>
          ${dispOpt('version', showVersion, 'Version / badge')}
          ${dispOpt('count', showCount, 'Counts')}
        </div>
      </div>
      <div class="ent-tree${showVersion ? '' : ' hide-meta'}${showCount ? '' : ' hide-cnt'}">${GROUPS.map(groupSec).join('')}</div>`;

    // 折叠：每个 .ent-tog 切最近的 .collapsible（分组 / 类型两层通用）
    host.querySelectorAll('.ent-tog').forEach(h => h.onclick = () => h.closest('.collapsible').classList.toggle('open'));
    // 选中实体行 → 统一意图通道（sea.js Intent.on('entity') morph 详情）
    host.querySelectorAll('.ent-r').forEach(it => it.onclick = e => {
      if (e.target.closest('.ent-r-more')) return;
      host.querySelectorAll('.ent-r').forEach(x => x.classList.remove('on')); it.classList.add('on');
      Intent.select({ kind: 'entity', id: it.dataset.id });
    });
    // 排序菜单（sliders + 单选，示意；接后端 = List sort 参数）
    const disp = host.querySelector('.ent-disp'), menu = host.querySelector('.ent-menu');
    disp.onclick = e => { e.stopPropagation(); const o = menu.classList.toggle('open'); disp.classList.toggle('on', o); };
    menu.addEventListener('click', e => e.stopPropagation());
    menu.querySelectorAll('[data-sort]').forEach(o => o.onclick = () => {
      menu.querySelectorAll('[data-sort]').forEach(x => x.classList.remove('on')); o.classList.add('on'); sort = o.dataset.sort;
    });
    // 显示开关（独立多选，非单选）：实时切 .ent-tree 类、不重绘
    const tree = host.querySelector('.ent-tree');
    menu.querySelectorAll('[data-disp]').forEach(o => o.onclick = () => {
      const on = o.classList.toggle('on'), k = o.dataset.disp;
      if (k === 'version') { showVersion = on; tree.classList.toggle('hide-meta', !on); }
      else { showCount = on; tree.classList.toggle('hide-cnt', !on); }
    });
    // 标题快滤：隐藏未命中行；过滤期间有命中的类型自动展开
    const fin = host.querySelector('.ent-filter input');
    fin.oninput = () => {
      const q = fin.value.trim().toLowerCase();
      host.querySelectorAll('.ent-ty').forEach(sec => {
        let any = false;
        sec.querySelectorAll('.ent-r').forEach(en => { const hit = en.querySelector('.ent-r-t').textContent.toLowerCase().includes(q); en.style.display = hit ? '' : 'none'; if (hit) any = true; });
        if (q) sec.classList.toggle('open', any);
      });
    };

    // 点菜单外收起排序菜单
    document.addEventListener('click', () => {
      const m = host.querySelector('.ent-menu.open');
      if (m) { m.classList.remove('open'); host.querySelector('.ent-disp')?.classList.remove('on'); }
    });
  }

  SideBar.register('entities', render);
})();

/* Forgify design-lab — 【Documents 海洋】左侧栏内容（独立文件，一人负责；与外壳/别的海洋解耦）。
   外壳 sidebar.js 据四导航按需懒加载本文件；自注入 documents.css，经 SideBar.register('documents', render) 挂载。
   定位：文档库的【树导航器】——浏览/组织/选中打开。编辑器在海面、TOC/反链在右岛，侧栏一概不碰。
   结构对齐 chat.js（自注入 → build/render → sliders 菜单 → 标题过滤 → 外点收起）。类名全 dt- 专属，避让 doc-/da-/cv-/cm-。依赖 icon()。 */
(function () {
  // 自注入样式（自包含，只一次）
  const dir = new URL('.', document.currentScript.src).href;
  if (!document.querySelector('link[data-sb="documents"]')) {
    const l = document.createElement('link');
    l.rel = 'stylesheet'; l.href = dir + 'documents.css'; l.dataset.sb = 'documents';
    document.head.appendChild(l);
  }

  // 示意树（接后端 = GET /documents/tree 整树 metadata；折叠纯前端）。每个节点都是一篇 markdown 文档，有 children 即可当"文件夹"。u=更新新近度(示意)。
  const TREE = [
    { id: 'd1', name: 'Product', u: 4, children: [
      { id: 'd2', name: 'Frontend', u: 6, children: [
        { id: 'd3', name: '文档页设计', u: 9, on: true },
        { id: 'd4', name: 'Roadmap 2026', u: 7 },
      ] },
      { id: 'd5', name: '竞品列表', u: 3 },
    ] },
    { id: 'd6', name: 'Engineering', u: 8, children: [
      { id: 'd7', name: 'Backend 重构记录', u: 2 },
      { id: 'd8', name: 'API 契约', u: 8 },
    ] },
    { id: 'd9', name: '随手记', u: 5 },
  ];
  const RECENT = [{ id: 'd3', name: '文档页设计' }, { id: 'd8', name: 'API 契约' }, { id: 'd4', name: 'Roadmap 2026' }];

  // 排序/展示状态（filter 行 sliders 控制；跨重绘保留）
  let sort = 'manual', showRecent = true;

  // —— 片段 ——
  // 树节点（递归）。data-pos 原序(Manual 复位) · data-u 新近度(Recently edited)；行尾 hover 露 ＋/⋯。
  const node = (n, depth, pos) => {
    const branch = n.children && n.children.length;
    return `<div class="dt-node${branch ? ' branch open' : ''}">
      <div class="dt-row${n.on ? ' on' : ''}" data-id="${n.id}" data-pos="${pos}" data-u="${n.u || 0}" style="padding-left:${8 + depth * 15}px">
        <span class="dt-chev">${branch ? icon('chevr', 13) : ''}</span>
        <span class="dt-ico">${icon('doc', 15)}</span>
        <span class="dt-name">${n.name}</span>
        <button class="dt-act" data-act="add" title="New sub-page">${icon('plus', 15)}</button>
        <button class="dt-act" data-act="more" title="More">${icon('more', 15)}</button>
      </div>
      ${branch ? `<div class="dt-children">${n.children.map((c, i) => node(c, depth + 1, i)).join('')}</div>` : ''}
    </div>`;
  };
  const flatRow = r => `<div class="dt-row dt-flat" data-id="${r.id}"><span class="dt-ico">${icon('doc', 15)}</span><span class="dt-name">${r.name}</span></div>`;
  const opt = (k, v, on, label) => `<button class="dt-opt${on ? ' on' : ''}" data-${k}="${v}"><span class="dt-ck">${icon('check', 14)}</span>${label}</button>`;

  function build() {
    if (!TREE.length) return `
      <button class="dt-new">${icon('plus', 18)}<span>New document</span></button>
      <div class="dt-empty">${icon('doc', 30)}<p>No documents yet</p></div>`;
    const recent = showRecent ? `
        <div class="dt-sec open">
          <button class="dt-sec-h"><span class="dt-sec-t">Recent</span><span class="dt-chev">${icon('chevr', 13)}</span></button>
          <div class="dt-sec-body">${RECENT.map(flatRow).join('')}</div>
        </div>` : '';
    return `
      <button class="dt-new">${icon('plus', 18)}<span>New document</span></button>
      <div class="dt-filter">${icon('search', 16)}<input type="text" placeholder="Filter by name…">
        <button class="dt-mbtn" title="Sort & filter">${icon('sliders', 16)}</button>
        <div class="dt-menu">
          <div class="dt-mh">Sort by</div>
          ${opt('sort', 'manual', sort === 'manual', 'Manual order')}
          ${opt('sort', 'name', sort === 'name', 'Name A–Z')}
          ${opt('sort', 'recent', sort === 'recent', 'Recently edited')}
          <div class="dt-mh">Display</div>
          ${opt('show', 'recent', showRecent, 'Show recent')}
        </div>
      </div>
      <div class="dt-list">${recent}<div class="dt-tree">${TREE.map((n, i) => node(n, 0, i)).join('')}</div></div>`;
  }

  // 就地按 sort 重排树兄弟（递归；不重渲染，保留展开态与菜单态）
  function applySort(host, mode) {
    const sortIn = box => {
      const ns = [...box.children].filter(x => x.classList.contains('dt-node'));
      ns.sort((a, b) => {
        const ra = a.querySelector(':scope > .dt-row'), rb = b.querySelector(':scope > .dt-row');
        if (mode === 'name') return ra.querySelector('.dt-name').textContent.localeCompare(rb.querySelector('.dt-name').textContent, 'zh');
        if (mode === 'recent') return (+rb.dataset.u) - (+ra.dataset.u);
        return (+ra.dataset.pos) - (+rb.dataset.pos);   // manual = 原序
      });
      ns.forEach(n => { box.appendChild(n); const k = n.querySelector(':scope > .dt-children'); if (k) sortIn(k); });
    };
    const tree = host.querySelector('.dt-tree'); if (tree) sortIn(tree);
  }

  function render(host) {
    host.innerHTML = build();
    const tree = host.querySelector('.dt-tree');
    const sec = host.querySelector('.dt-sec');

    // 树展开/折叠（点 chevron，不冒泡到选中）
    host.querySelectorAll('.dt-node.branch > .dt-row .dt-chev').forEach(c => c.onclick = e => { e.stopPropagation(); c.closest('.dt-node').classList.toggle('open'); });
    // Recent 折叠
    if (sec) sec.querySelector('.dt-sec-h').onclick = () => sec.classList.toggle('open');
    // 选中 + 打开：高亮当前行 + 发 nav intent 给海面（外壳通道若已加；无则仅高亮）
    host.querySelectorAll('.dt-row').forEach(r => r.onclick = e => {
      if (e.target.closest('.dt-act') || e.target.closest('.dt-chev')) return;
      host.querySelectorAll('.dt-row').forEach(x => x.classList.remove('on')); r.classList.add('on');
      if (window.Shell && Shell.openDocument) Shell.openDocument(r.dataset.id);
    });
    // ＋/⋯ hover 占位入口（菜单/拖拽下一轮）：吃掉点击不误触选中
    host.querySelectorAll('.dt-act').forEach(b => b.onclick = e => e.stopPropagation());

    // 排序/筛选菜单（sliders，对齐 chat）：排序就地重排树(菜单留开) + Show recent 即时显隐
    const btn = host.querySelector('.dt-mbtn'), menu = host.querySelector('.dt-menu');
    btn.onclick = e => { e.stopPropagation(); const open = menu.classList.toggle('open'); btn.classList.toggle('on', open); };
    menu.addEventListener('click', e => e.stopPropagation());
    menu.querySelectorAll('[data-sort]').forEach(o => o.onclick = () => {
      menu.querySelectorAll('[data-sort]').forEach(x => x.classList.remove('on')); o.classList.add('on'); sort = o.dataset.sort; applySort(host, sort);
    });
    menu.querySelector('[data-show="recent"]').onclick = function () {
      showRecent = !showRecent; this.classList.toggle('on', showRecent); if (sec) sec.style.display = showRecent ? '' : 'none';
    };

    // 名字过滤：命中 + 祖先链可见 + 命中分支自动展开；Recent 同步过滤
    const fin = host.querySelector('.dt-filter input');
    fin.oninput = () => {
      const q = fin.value.trim().toLowerCase();
      const walk = nd => {
        let hit = !q || nd.querySelector(':scope > .dt-row .dt-name').textContent.toLowerCase().includes(q);
        nd.querySelectorAll(':scope > .dt-children > .dt-node').forEach(ch => { if (walk(ch)) hit = true; });
        nd.style.display = hit ? '' : 'none';
        if (q && hit && nd.classList.contains('branch')) nd.classList.add('open');
        return hit;
      };
      [...tree.children].forEach(walk);
      host.querySelectorAll('.dt-sec .dt-flat').forEach(r => { r.style.display = (!q || r.querySelector('.dt-name').textContent.toLowerCase().includes(q)) ? '' : 'none'; });
    };

    if (sort !== 'manual') applySort(host, sort);   // 重绘后应用当前排序
  }

  // 点菜单外收起（一次性，对齐 chat）
  document.addEventListener('click', () => {
    const m = document.querySelector('#sidebody .dt-menu.open');
    if (m) { m.classList.remove('open'); document.querySelector('#sidebody .dt-mbtn')?.classList.remove('on'); }
  });

  SideBar.register('documents', render);
})();

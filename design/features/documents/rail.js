/* Foryx demo — 文档海洋侧栏内容：文档库的【树导航器】（薄组合）。
   浏览/组织/选中打开：新建 + 名字过滤 + 排序/筛选菜单(经组件 Floating) + Recent(可折叠) + 嵌套文档树。
   选中通道：点行 → Intent.select({kind:'document'}) → sea.js Intent.on('document') 装载该文档。编辑器在海面、TOC/反链在右岛、侧栏一概不碰。
   依赖 mock/documents.js（树/recent/cur）。类名全 doc-rail- 专属（避让海面的 doc-）。注册 SideBar.register('documents', render)。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);
  const D = () => window.MOCK_DOCUMENTS || {};

  // 排序/展示状态（filter 行菜单控制；跨重绘保留）
  let sort = 'manual', showRecent = true;

  // —— 片段 ——
  // 树节点（递归）。data-pos 原序(Manual 复位) · data-u 新近度(Recently edited)；行尾 hover 露 ＋/⋯。
  // 行首槽 = icon ↔ chevron 共享一格（Notion 式）：默认显文档图标，hover 时图标淡出、折叠箭头淡入（仅 branch；leaf 永远图标）。
  const node = (n, depth, pos, curId) => {
    const branch = n.children && n.children.length;
    return `<div class="doc-rail-node${branch ? ' branch open' : ''}">
      <div class="doc-rail-row${n.id === curId ? ' on' : ''}" data-id="${n.id}" data-pos="${pos}" data-u="${n.u || 0}" style="padding-left:${8 + depth * 18}px">
        <span class="doc-rail-lead"><span class="doc-rail-ico">${icon('doc', 16)}</span>${branch ? `<span class="doc-rail-chev">${icon('chevr', 14)}</span>` : ''}</span>
        <span class="doc-rail-name">${n.name}</span>
        <span class="doc-rail-acts">
          <button class="doc-rail-act" data-act="add" title="New sub-page">${icon('plus', 15)}</button>
          <button class="doc-rail-act" data-act="more" title="More">${icon('more', 15)}</button>
        </span>
      </div>
      ${branch ? `<div class="doc-rail-children">${n.children.map((c, i) => node(c, depth + 1, i, curId)).join('')}</div>` : ''}
    </div>`;
  };
  const flatRow = r => `<div class="doc-rail-row doc-rail-flat" data-id="${r.id}"><span class="doc-rail-lead"><span class="doc-rail-ico">${icon('doc', 16)}</span></span><span class="doc-rail-name">${r.name}</span></div>`;
  const opt = (k, v, on, label) => `<button class="doc-rail-opt${on ? ' on' : ''}" data-${k}="${v}"><span class="doc-rail-ck">${icon('check', 14)}</span>${label}</button>`;

  function build() {
    const d = D(), tree = d.tree || [], recentArr = d.recent || [], curId = d.cur;
    if (!tree.length) return `
      <button class="doc-rail-new">${icon('plus', 18)}<span>New document</span></button>
      <div class="doc-rail-empty">${icon('doc', 30)}<p>No documents yet</p></div>`;
    const recent = showRecent && recentArr.length ? `
        <div class="doc-rail-sec open">
          <button class="doc-rail-sec-h"><span class="doc-rail-sec-t">Recent</span><span class="doc-rail-chev">${icon('chevr', 13)}</span></button>
          <div class="doc-rail-sec-body">${recentArr.map(flatRow).join('')}</div>
        </div>` : '';
    return `
      <button class="doc-rail-new">${icon('plus', 18)}<span>New document</span></button>
      <div class="doc-rail-filter">${icon('search', 16)}<input type="text" placeholder="Filter by name…">
        <button class="doc-rail-mbtn" title="Sort & filter">${icon('sliders', 16)}</button>
      </div>
      <div class="doc-rail-list">${recent}<div class="doc-rail-tree">${tree.map((n, i) => node(n, 0, i, curId)).join('')}</div></div>`;
  }

  // 就地按 sort 重排树兄弟（递归；不重渲染，保留展开态）
  function applySort(host, mode) {
    const sortIn = box => {
      const ns = [...box.children].filter(x => x.classList.contains('doc-rail-node'));
      ns.sort((a, b) => {
        const ra = a.querySelector(':scope > .doc-rail-row'), rb = b.querySelector(':scope > .doc-rail-row');
        if (mode === 'name') return ra.querySelector('.doc-rail-name').textContent.localeCompare(rb.querySelector('.doc-rail-name').textContent, 'zh');
        if (mode === 'recent') return (+rb.dataset.u) - (+ra.dataset.u);
        return (+ra.dataset.pos) - (+rb.dataset.pos);   // manual = 原序
      });
      ns.forEach(n => { box.appendChild(n); const k = n.querySelector(':scope > .doc-rail-children'); if (k) sortIn(k); });
    };
    const tree = host.querySelector('.doc-rail-tree'); if (tree) sortIn(tree);
  }

  // 排序/筛选菜单：内容节点经组件 Floating 贴 sliders 按钮弹出（收掉手摆位 + Escape 副本）。
  let sortMenu = null;
  function openSortMenu(host, btn) {
    if (sortMenu) { sortMenu.close(); sortMenu = null; return; }
    const c = tag('div.doc-rail-menu',
      `<div class="doc-rail-mh">Sort by</div>
       ${opt('sort', 'manual', sort === 'manual', 'Manual order')}
       ${opt('sort', 'name', sort === 'name', 'Name A–Z')}
       ${opt('sort', 'recent', sort === 'recent', 'Recently edited')}
       <div class="doc-rail-mh">Display</div>
       ${opt('show', 'recent', showRecent, 'Show recent')}`);
    c.querySelectorAll('[data-sort]').forEach(o => o.onclick = () => {
      c.querySelectorAll('[data-sort]').forEach(x => x.classList.remove('on')); o.classList.add('on');
      sort = o.dataset.sort; applySort(host, sort);
    });
    c.querySelector('[data-show="recent"]').onclick = function () {
      showRecent = !showRecent; this.classList.toggle('on', showRecent);
      const sec = host.querySelector('.doc-rail-sec'); if (sec) sec.style.display = showRecent ? '' : 'none';
    };
    btn.classList.add('on');
    sortMenu = Floating.open(btn.getBoundingClientRect(), c, { below: true, onClose: () => { btn.classList.remove('on'); sortMenu = null; } });
  }

  function render(host) {
    host.innerHTML = build();
    const d = D();
    const tree = host.querySelector('.doc-rail-tree');
    const sec = host.querySelector('.doc-rail-sec');

    // 树展开/折叠：点 branch 的行首槽（hover 时显折叠箭头），不冒泡到选中
    host.querySelectorAll('.doc-rail-node.branch > .doc-rail-row .doc-rail-lead').forEach(c => c.onclick = e => { e.stopPropagation(); c.closest('.doc-rail-node').classList.toggle('open'); });
    // Recent 折叠
    if (sec) sec.querySelector('.doc-rail-sec-h').onclick = () => sec.classList.toggle('open');

    // 选中 + 打开：高亮当前行 + 发 Intent.select → 海面装载该文档。
    host.querySelectorAll('.doc-rail-row').forEach(r => r.onclick = e => {
      if (e.target.closest('.doc-rail-act')) return;   // leaf 行首仍冒泡到选中；branch 行首由上面 stopPropagation 吃掉
      host.querySelectorAll('.doc-rail-row').forEach(x => x.classList.remove('on')); r.classList.add('on');
      if (d.docs) d.cur = r.dataset.id;
      Intent.select({ kind: 'document', id: r.dataset.id });
    });
    // ＋/⋯ hover 入口占位：吃掉点击不误触选中
    host.querySelectorAll('.doc-rail-act').forEach(b => b.onclick = e => e.stopPropagation());

    // 排序/筛选（sliders → Floating 弹层）
    const btn = host.querySelector('.doc-rail-mbtn');
    btn.onclick = e => { e.stopPropagation(); openSortMenu(host, btn); };

    // 名字过滤：命中 + 祖先链可见 + 命中分支自动展开；Recent 同步过滤
    const fin = host.querySelector('.doc-rail-filter input');
    fin.oninput = () => {
      const q = fin.value.trim().toLowerCase();
      const walk = nd => {
        let hit = !q || nd.querySelector(':scope > .doc-rail-row .doc-rail-name').textContent.toLowerCase().includes(q);
        nd.querySelectorAll(':scope > .doc-rail-children > .doc-rail-node').forEach(ch => { if (walk(ch)) hit = true; });
        nd.style.display = hit ? '' : 'none';
        if (q && hit && nd.classList.contains('branch')) nd.classList.add('open');
        return hit;
      };
      [...tree.children].forEach(walk);
      host.querySelectorAll('.doc-rail-sec .doc-rail-flat').forEach(r => { r.style.display = (!q || r.querySelector('.doc-rail-name').textContent.toLowerCase().includes(q)) ? '' : 'none'; });
    };

    if (sort !== 'manual') applySort(host, sort);   // 重绘后应用当前排序
  }

  SideBar.register('documents', render);

  // 侧栏高亮跟随海面装载（@提及/反链跨海洋切来时也对齐当前行）
  Intent.on('document', sel => {
    const host = document.querySelector('#sidebody'); if (!host) return;
    host.querySelectorAll('.doc-rail-row').forEach(r => r.classList.toggle('on', r.dataset.id === sel.id));
  });
})();

/* Foryx demo — 文档海洋海面：零-markdown WYSIWYG 文档页（薄组合）。
   海面 = 面包屑(path) + 标题 + WYSIWYG 正文 + 右岛元信息抽屉。自己几乎不画像素：
     · 编辑器四件套（斜杠/工具条/手柄/即输即渲）= 本海洋唯一独特逻辑，封在同目录 editor.js（DocEditor），浮层全走组件 Floating；
     · 右岛（大纲 TOC + 反链 + 信息）= 组件 RightIsland 基座 + 海洋自渲染轻量行（不套 KV/RefPill——侧栏面板求轻、不上药丸/黑粗字）；
     · 代码块高亮 = 组件 CodeEditor.highlight（DocEditor 内部调）。
   选中通道：侧栏文档行 / 正文 @提及·wikilink → Intent.select({kind:'document'/...}) → 本海洋 Intent.on('document') 切文档。
   依赖 mock/documents.js + 同目录 editor.js。注册 Shell.registerOcean('documents')。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);
  const D = () => window.MOCK_DOCUMENTS || {};
  const curDoc = () => { const d = D(); return (d.docs || {})[d.cur] || Object.values(d.docs || {})[0] || {}; };

  // editor.js 同目录懒加载（manifest 只拉 sea/rail，编辑器引擎由海面自取——薄组合不内联那坨独特逻辑）。
  const here = document.currentScript ? document.currentScript.src : '';
  const editorUrl = here.replace(/sea\.js(\?.*)?$/, 'editor.js');
  const ensureEditor = () => window.DocEditor ? Promise.resolve()
    : (window.loader ? loader.loadJs(editorUrl).catch(() => {}) : Promise.resolve());

  let sea, island, mounted = false;

  // ===== 主区头：保存状态徽 + 右岛开关 =====
  function setStatus(busy) {
    const s = sea && sea.querySelector('#docStatus'); if (!s) return;
    s.className = 'doc-status' + (busy ? ' busy' : '');
    s.innerHTML = busy ? `<span class="pulse"></span>AI 编辑中` : `<span class="ico">${icon('check', 14)}</span>已保存`;
  }
  function headExtra() {
    Shell.headExtra(`
      <span class="doc-status" id="docStatus"></span>
      <button class="ibtn" id="docPanel" title="大纲 / 反链 / 信息">${icon('panel')}</button>`);
    Shell.$('#docPanel').onclick = () => island && island.toggle();
    setStatus(false);
  }

  // ===== 文档头（面包屑 + 标题 + 元信息） =====
  function renderHead(doc) {
    const path = doc.path || [];
    const segs = path.map((p, i) => i === path.length - 1
      ? `<span class="cur">${p}</span>`
      : `<button class="doc-seg">${p}</button><span class="sep">/</span>`).join('');
    sea.querySelector('#docPath').innerHTML = `<span class="ico">${icon('folder', 13)}</span>${segs}`;
    sea.querySelector('#docTitle').textContent = doc.title || '';
    const tags = (doc.tags || []).map(t => `<span class="doc-tag"><span class="ico">${icon('tag', 11)}</span>${t}</span>`).join('');
    const nBack = (doc.backlinks || []).length;
    sea.querySelector('#docMeta').innerHTML = `<span>更新于 ${doc.updated || '—'}</span><span class="sep">·</span><span>${doc.words || ''}</span>` +
      (nBack ? `<span class="sep">·</span><button class="backref" id="docBackref">${nBack} 个反链</button>` : '') +
      (tags ? `<span class="doc-tags">${tags}</span>` : '');
    const br = sea.querySelector('#docBackref'); if (br) br.onclick = () => island && island.show();
  }

  // ===== 右岛 = 元信息抽屉（组件 RightIsland 基座 + 海洋填 body：TOC / 反链 / 信息） =====
  function ensureIsland() {
    if (island && island.el && island.el.isConnected) return island;   // 切走再回来时旧元素已被 Shell.mount 清掉 → 重建（修右岛打不开）
    // 不传 icon、不自定义 head：组件默认头隐空图标槽 + 标题 + 关（关已组件内置）——求轻，去掉标题前那个大图标方块
    island = RightIsland.create('documents', { title: '文档信息', width: 300 });
    return island;
  }
  // 大纲从正文 H2/H3 实时抽（scroll-spy 示意，当前节 = 黑字，不靠蓝线/灰块）。
  function renderIsland(doc) {
    ensureIsland();
    const b = island.body; b.innerHTML = '';
    const heads = [...sea.querySelectorAll('#docBody h2, #docBody h3')];

    // —— 大纲 TOC：缩进列表 + 当前节 accent 细线（文案走 textContent，不经 tag 第三参——避空文本 bug） ——
    const secToc = tag('div.da-sec'); secToc.appendChild(tag('div.da-h', '大纲'));
    const toc = tag('div.da-toc');
    if (heads.length) heads.forEach((h, i) => {
      const a = tag('a.da-toc-a' + (h.tagName === 'H3' ? '.h3' : '') + (i === 0 ? '.on' : ''));
      a.textContent = h.textContent;
      a.onclick = e => { e.preventDefault(); toc.querySelectorAll('.da-toc-a').forEach(x => x.classList.remove('on')); a.classList.add('on'); h.scrollIntoView({ behavior: 'smooth', block: 'start' }); };
      toc.appendChild(a);
    });
    else toc.appendChild(tag('div.da-empty', '暂无小节'));
    secToc.appendChild(toc); b.appendChild(secToc);

    // —— 反向链接（relation 入边消费视图）：轻行（链接图标 + 名 + 摘要），点 → Intent.select 切那篇 ——
    const back = doc.backlinks || [];
    if (back.length) {
      const secBack = tag('div.da-sec');
      secBack.appendChild(tag('div.da-h', `反向链接<span class="da-h-n">${back.length}</span>`));
      const list = tag('div.da-back');
      back.forEach(bl => {
        const row = tag('div.da-bl');
        row.innerHTML = `<span class="da-bl-ic">${icon('link', 13)}</span><span class="da-bl-main"><span class="da-bl-name"></span>${bl.snip ? '<span class="da-bl-snip"></span>' : ''}</span>`;
        row.querySelector('.da-bl-name').textContent = bl.name;
        if (bl.snip) row.querySelector('.da-bl-snip').textContent = bl.snip;
        row.onclick = () => Intent.select({ kind: 'document', id: bl.id });
        list.appendChild(row);
      });
      secBack.appendChild(list); b.appendChild(secBack);
    }

    // —— 信息：紧凑定义行（窄标签 + 值灰不撑黑；值走 textContent 防长路径撑破） ——
    const secMeta = tag('div.da-sec'); secMeta.appendChild(tag('div.da-h', '信息'));
    const info = tag('div.da-info');
    const irow = (k, v, mono) => {
      const r = tag('div.da-irow');
      r.innerHTML = `<span class="da-ik"></span><span class="da-iv${mono ? ' mono' : ''}"></span>`;
      r.querySelector('.da-ik').textContent = k; r.querySelector('.da-iv').textContent = v;
      return r;
    };
    info.appendChild(irow('路径', '/' + (doc.path || []).join('/'), true));
    info.appendChild(irow('标签', (doc.tags || []).join(' · ') || '—'));
    info.appendChild(irow('更新', doc.updated || '—'));
    info.appendChild(irow('大小', doc.size || '—'));
    secMeta.appendChild(info); b.appendChild(secMeta);
  }

  // ===== 装载一篇文档：头 + 正文(交 DocEditor) + 右岛 =====
  function load(id) {
    const d = D(); if (id != null && (d.docs || {})[id]) d.cur = id;
    const doc = curDoc();
    renderHead(doc);
    DocEditor.render(doc);
    renderIsland(doc);
  }

  Shell.registerOcean('documents', {
    crumb: '文档',
    build(sea_) {
      sea = sea_;
      sea.innerHTML = `
        <div class="doc-scroll scroll-fade" id="docScroll">
          <article class="doc" id="doc">
            <div class="doc-path" id="docPath"></div>
            <h1 class="doc-title" id="docTitle" contenteditable="true" spellcheck="false"></h1>
            <div class="doc-meta" id="docMeta"></div>
            <div class="doc-body" id="docBody" contenteditable="true" spellcheck="false"></div>
          </article>
        </div>`;
      headExtra();
      ensureIsland();
      ensureEditor().then(() => {
        if (!window.DocEditor) { sea.querySelector('#docBody').innerHTML = '<p style="color:var(--ink-3)">编辑器引擎加载失败</p>'; return; }
        DocEditor.mount(sea.querySelector('#docScroll'), {
          onStatus: setStatus,
          onChange: () => { if (island && island.isOpen()) renderIsland(curDoc()); },
        });
        mounted = true;
        load(D().cur);
      });
    },
  });

  // 选中通道：侧栏文档行 / 反链药丸 → Intent.select({kind:'document'}) → 切文档。
  Intent.on('document', sel => {
    if (!sea || !mounted) { const d = D(); if (sel && sel.id) d.cur = sel.id; return; }
    load(sel.id);
  });
})();

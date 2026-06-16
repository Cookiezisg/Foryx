/* Foryx demo — 文档海洋 · WYSIWYG 编辑器引擎（本地核心；零-markdown 心智）。
   这是本海洋唯一「自画像素」的地方——contenteditable 正文上的四件套：
     ① 斜杠命令窗(/)：空行敲 / 收纳所有「插入」；
     ② 选中工具条：选中即浮，点选格式 + 行内 AI 询问(对齐后端 :iterate)；
     ③ 块左侧手柄：+ 在此后插入 · ⋮⋮ 拖拽重排 / 点开块菜单；
     ④ markdown 即输即渲：行首 # / - / 1. / > / [] / --- + 空格 → 块变形。
   薄组合铁律：所有【浮层定位】一律走组件 Floating.open（不手摆 getBoundingClientRect / 不抄 Escape / 不抄 spring）；
              代码块【高亮】一律走组件 CodeEditor.highlight（唯一 tokenizer 事实源，不在此重抄正则）。
   暴露 window.DocEditor 供 sea.js 装配：mount(scrollEl, hooks) · render(doc) · close()。hooks = { onStatus(busy), onChange(), onPill() }。
   依赖：组件 Floating / CodeEditor / RefPill（只读）。样式在同目录 sea.css（doc- 前缀）。 */
window.DocEditor = (function () {
  const el = (t, c) => { const e = document.createElement(t); if (c) e.className = c; return e; };
  const sleep = ms => new Promise(r => setTimeout(r, ms));
  let runId = 0;
  const alive = id => id === runId;

  let scrollEl = null, hooks = {};
  const docEl = () => scrollEl && scrollEl.querySelector('.doc');
  const docBody = () => scrollEl && scrollEl.querySelector('#docBody');
  const setStatus = busy => hooks.onStatus && hooks.onStatus(busy);
  const fireChange = () => hooks.onChange && hooks.onChange();

  // 代码块着色：走组件 CodeEditor.highlight（唯一高亮事实源；本海洋不重抄 TOK/KW）。
  function highlightCode(scope) {
    scope.querySelectorAll('.doc-code pre').forEach(pre => {
      if (pre.dataset.hl) return; pre.dataset.hl = '1';
      pre.innerHTML = CodeEditor.highlight(pre.textContent);
    });
  }

  // —— 斜杠命令窗的块清单（接后端时即「插入」能力表）——
  const BLOCKS = [
    { grp: '基础' },
    { k: 'text', ic: 'text', nm: '文本', hint: '', html: '<p>新段落</p>' },
    { k: 'h2', ic: 'heading', nm: '标题 1', hint: '#', html: '<h2>新标题</h2>' },
    { k: 'h3', ic: 'heading', nm: '标题 2', hint: '##', html: '<h3>新小标题</h3>' },
    { k: 'todo', ic: 'check', nm: '待办清单', hint: '[ ]', html: '<ul class="doc-task"><li><span class="box"></span><span class="t">待办项</span></li></ul>' },
    { k: 'ul', ic: 'list', nm: '无序列表', hint: '-', html: '<ul><li>列表项</li></ul>' },
    { k: 'ol', ic: 'listol', nm: '有序列表', hint: '1.', html: '<ol><li>列表项</li></ol>' },
    { k: 'quote', ic: 'quote', nm: '引用', hint: '>', html: '<blockquote>引用内容</blockquote>' },
    { k: 'code', ic: 'code', nm: '代码块', hint: '```', html: '<div class="doc-code"><span class="lang">代码</span><pre>// 在此写代码</pre></div>' },
    { k: 'table', ic: 'table', nm: '表格', hint: '', html: '<table class="doc-table"><thead><tr><th>列 1</th><th>列 2</th></tr></thead><tbody><tr><td>—</td><td>—</td></tr></tbody></table>' },
    { k: 'callout', ic: 'spark', nm: '提示块', hint: '', html: '<div class="doc-callout"><span class="ico">' + icon('spark', 16, 1.6) + '</span><div class="c">提示内容</div></div>' },
    { k: 'hr', ic: 'divider', nm: '分隔线', hint: '---', html: '<hr>' },
    { k: 'img', ic: 'image', nm: '图片', hint: '', html: '<div class="doc-imgph">图片占位</div>' },
    { grp: '引用与 AI' },
    { k: 'wikilink', ic: 'link', nm: '链接到文档', hint: '[[', inline: () => RefPill.html('link', '某篇文档', 'd4') },
    { k: 'mention', ic: 'at', nm: '提及实体', hint: '@', inline: () => RefPill.html('agent', '某个 Agent', 'ag_research') },
    { k: 'ai', ic: 'spark', nm: '让 AI 写…', hint: '', ai: true },
  ];

  // ===== 渲染正文 =====
  function bindPills() {
    const b = docBody(); if (!b) return;
    // 委托点击（绑一次）：实体 @提及按真 kind 派发；document wikilink 的 data-kind='doc' 重路由成 owned 'document'。
    if (b.dataset.refWired) return; b.dataset.refWired = '1';
    b.addEventListener('click', e => {
      const p = e.target.closest && e.target.closest('.fg-ref[data-ref]');
      if (!p || !b.contains(p)) return;
      Intent.select({ kind: p.dataset.kind === 'doc' ? 'document' : p.dataset.kind, id: p.dataset.ref });
    });
  }
  function render(doc) {
    runId++;                                  // 打断进行中的 AI 流光
    const body = docBody(); if (!body) return;
    closeSlash(); hideToolbar(); closeBmenu();
    body.innerHTML = doc.body || '';
    highlightCode(body);
    body.classList.remove('doc-morph'); void body.offsetWidth; body.classList.add('doc-morph');
    bindPills(); setStatus(false); fireChange();
    if (scrollEl) scrollEl.scrollTop = 0;
  }

  // ===== 定位助手：取选区/光标的视口矩形（Floating 用视口坐标，无需减容器原点） =====
  function directBlock(n) {                  // node 所在的 #docBody 直接子元素 = 一个「块」
    if (!n) return null;
    let x = n.nodeType === 3 ? n.parentNode : n;
    const b = docBody();
    while (x && x.parentNode && x.parentNode !== b) x = x.parentNode;
    return x && x.parentNode === b ? x : null;
  }
  function caretRect() { const s = window.getSelection(); if (!s.rangeCount) return null; const r = s.getRangeAt(0).getBoundingClientRect(); return (r.width || r.height) ? r : null; }

  // ===== 选中工具条：点选格式化 + AI 询问 =====
  let bar = null;
  const FMT = [['bold', '<b>B</b>', '加粗'], ['italic', '<i>I</i>', '斜体'], ['strike', '<s>S</s>', '删除线'], ['mark', '<span class="hl">高</span>', '高亮'], ['code', icon('code', 14), '行内代码'], ['link', icon('link', 14), '链接']];
  function hideToolbar() { if (bar) { bar.close(); bar = null; } }
  function wireToolbar() {
    const b = docBody();
    b.addEventListener('mousedown', hideToolbar);
    b.addEventListener('mouseup', () => setTimeout(() => {
      if (slashOpen()) return;
      const s = window.getSelection();
      if (!s || s.isCollapsed || !s.rangeCount || !b.contains(s.anchorNode)) return;
      const rect = s.getRangeAt(0).getBoundingClientRect();
      if (rect.width < 2) return;
      showToolbar(rect, s.getRangeAt(0).cloneRange());
    }, 0));
  }
  function showToolbar(rect, range) {
    hideToolbar();
    const c = el('div', 'doc-tb');
    c.innerHTML = `<button class="ai" data-a="ai"><span class="ico">${icon('spark', 14, 1.7)}</span>AI</button><span class="sep"></span>` +
      FMT.map(([a, h, t]) => `<button data-a="${a}" title="${t}">${h}</button>`).join('');
    c.querySelectorAll('button').forEach(btn => btn.addEventListener('mousedown', e => {
      e.preventDefault();
      if (btn.dataset.a === 'ai') showAsk(rect, range); else applyFmt(btn.dataset.a, range);
    }));
    bar = Floating.open(rect, c, { below: false });   // 工具条落选区上方
  }
  function wrap(range, tag) { try { const e = el(tag); range.surroundContents(e); return e; } catch (x) { return null; } }
  function applyFmt(a, range) {
    const s = window.getSelection(); s.removeAllRanges(); s.addRange(range);
    if (a === 'bold' || a === 'italic') document.execCommand(a);
    else if (a === 'strike') document.execCommand('strikethrough');
    else if (a === 'mark' || a === 'code') wrap(range, a === 'mark' ? 'mark' : 'code');
    else if (a === 'link') { const e = wrap(range, 'a'); if (e) e.href = '#'; }
    hideToolbar(); fireChange();
  }
  // AI 询问：一句自然语言指令 + 快捷动作（对齐后端 :iterate）→ 选区流光改写
  function showAsk(rect, range) {
    hideToolbar();
    const c = el('div', 'doc-ask');
    c.innerHTML = `<div class="row"><span class="ico">${icon('spark', 16, 1.7)}</span><input placeholder="让 AI 改写选中内容…" autocomplete="off"></div>
      <div class="quick">${['改简洁', '续写', '翻译成英文', '更正式'].map(t => `<button>${t}</button>`).join('')}</div>`;
    const inp = c.querySelector('input');
    c.querySelectorAll('.quick button').forEach(b => b.addEventListener('mousedown', e => { e.preventDefault(); aiFlow(range); }));
    bar = Floating.open(rect, c, { below: true });
    setTimeout(() => inp.focus(), 0);
    inp.addEventListener('keydown', e => { if (e.key === 'Enter') { e.preventDefault(); aiFlow(range); } else if (e.key === 'Escape') hideToolbar(); });
  }
  async function aiFlow(range) {
    const id = ++runId; hideToolbar(); setStatus(true);
    const span = wrap(range, 'span'); if (span) span.className = 'doc-flow';
    window.getSelection()?.removeAllRanges();
    await sleep(1500);
    if (span) { span.classList.remove('doc-flow'); const p = span.parentNode; while (span.firstChild) p.insertBefore(span.firstChild, span); p.removeChild(span); p.normalize && p.normalize(); }
    if (alive(id)) { setStatus(false); fireChange(); }
  }

  // ===== 斜杠命令窗（经 Floating 定位）=====
  let menu = null, slash = null, onIdx = 0, flat = [];
  const slashOpen = () => !!menu;
  function wireSlash() {
    const b = docBody();
    b.addEventListener('input', () => { if (!detectSlash()) { closeSlash(); autoFormat(); } });
    document.addEventListener('keydown', onSlashKey, true);
  }
  function detectSlash() {
    const s = window.getSelection(); if (!s.rangeCount) return false;
    const r = s.getRangeAt(0); if (r.startContainer.nodeType !== 3) return false;
    const m = r.startContainer.textContent.slice(0, r.startOffset).match(/(?:^|\s)\/([^\s/]*)$/); if (!m) return false;
    slash = { mode: 'type', node: r.startContainer, start: r.startOffset - m[1].length - 1, end: r.startOffset, query: m[1] };
    openSlash(); return true;
  }
  function openSlashAfter(block, anchorRect) { slash = { mode: 'plus', host: block }; openSlash(anchorRect); }
  function openSlashTurn(block, anchorRect) { slash = { mode: 'turn', host: block }; openSlash(anchorRect); }
  function matched() {
    const q = (slash.query || '').toLowerCase(); const out = []; let grp = null;
    BLOCKS.forEach(it => {
      if (it.grp) { grp = it; return; }
      if (!q || it.nm.toLowerCase().includes(q) || it.k.includes(q) || (it.hint || '').includes(q)) { if (grp) { out.push(grp); grp = null; } out.push(it); }
    });
    return out;
  }
  // 斜杠窗内容随 query 实时重建。Floating 已开则原地换内容 + 重摆位（reposition），未开则新开。
  function openSlash(anchorRect) {
    const list = matched(); flat = list.filter(x => !x.grp);
    if (onIdx >= flat.length) onIdx = 0;
    const c = el('div', 'doc-slash');
    c.innerHTML = flat.length ? list.map(it => it.grp
      ? `<div class="grp">${it.grp}</div>`
      : `<div class="item${flat[onIdx] === it ? ' on' : ''}" data-k="${it.k}"><span class="ic">${icon(it.ic, 16)}</span><span class="nm">${it.nm}</span>${it.hint ? `<span class="hint">${it.hint}</span>` : ''}</div>`).join('')
      : `<div class="empty">没有匹配「${slash.query}」的块</div>`;
    c.querySelectorAll('.item').forEach(it => {
      it.addEventListener('mousedown', e => { e.preventDefault(); choose(flat.find(x => x.k === it.dataset.k)); });
      it.addEventListener('mousemove', () => { const i = flat.findIndex(x => x.k === it.dataset.k); if (i !== onIdx) { onIdx = i; paintOn(); } });
    });
    const rect = anchorRect || caretRect(); if (!rect) return;
    if (menu) { menu.el.firstChild && menu.el.firstChild.remove(); menu.el.appendChild(c); menu.reposition(rect); }
    else menu = Floating.open(rect, c, { below: true, onClose: () => { menu = null; slash = null; } });
  }
  function paintOn() { if (menu) menu.el.querySelectorAll('.item').forEach(it => it.classList.toggle('on', flat[onIdx] && it.dataset.k === flat[onIdx].k)); }
  function onSlashKey(e) {
    if (!menu) return;
    if (e.key === 'ArrowDown') { e.preventDefault(); onIdx = (onIdx + 1) % flat.length; paintOn(); seeOn(); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); onIdx = (onIdx - 1 + flat.length) % flat.length; paintOn(); seeOn(); }
    else if (e.key === 'Enter') { e.preventDefault(); choose(flat[onIdx]); }
    else if (e.key === 'Escape') { e.preventDefault(); closeSlash(); }
  }
  function seeOn() { const on = menu && menu.el.querySelector('.item.on'); on && on.scrollIntoView({ block: 'nearest' }); }
  function closeSlash() { if (menu) { menu.close(); menu = null; } slash = null; }
  function choose(block) {
    if (!block || !slash) return closeSlash();
    const mode = slash.mode;
    let host;
    if (mode === 'type') { const { node, start, end } = slash; try { node.textContent = node.textContent.slice(0, start) + node.textContent.slice(end); } catch (x) {} host = directBlock(node) || docBody().lastElementChild; }
    else host = slash.host;
    closeSlash(); if (!host) return;
    if (block.inline) { host.insertAdjacentHTML('beforeend', ' ' + block.inline()); bindPills(); }
    else if (block.ai) { host.insertAdjacentHTML('afterend', '<p></p>'); aiWrite(host.nextElementSibling); }
    else if (mode === 'turn') { host.insertAdjacentHTML('beforebegin', block.html); const f = host.previousElementSibling; host.remove(); f && f.classList.add('doc-morph'); bindPills(); }
    else { host.insertAdjacentHTML('afterend', block.html); const f = host.nextElementSibling; if (mode === 'type' && host.tagName === 'P' && !host.textContent.trim()) host.remove(); f && (f.classList.add('doc-morph'), f.scrollIntoView({ block: 'nearest' })); bindPills(); highlightCode(docBody()); }
    fireChange();
  }
  async function aiWrite(p) {
    if (!p) return; const id = ++runId; setStatus(true);
    const span = el('span', 'doc-flow'); p.appendChild(span); p.scrollIntoView({ block: 'nearest' });
    await typeInto(span, 'AI 根据上下文续写的一段内容，落定后从流光沉淀为正文。');
    span.classList.remove('doc-flow'); const par = span.parentNode; while (span.firstChild) par.insertBefore(span.firstChild, span); par.removeChild(span);
    if (alive(id)) { setStatus(false); fireChange(); }
  }
  function typeInto(node, text, cps = 58) {
    const id = runId;
    return new Promise(res => {
      const caret = el('span', 'doc-caret'); node.appendChild(caret); let i = 0;
      (function step() {
        if (!alive(id)) { caret.remove(); return res(); }
        caret.insertAdjacentText('beforebegin', text[i++] ?? '');
        if (i > text.length) { caret.remove(); return res(); }
        setTimeout(step, 1000 / cps + Math.random() * 14);
      })();
    });
  }

  // ===== markdown 即输即渲（行首 # / - / 1. / > / [] / --- + 空格 → 块变形） =====
  const MD = [
    [/^###\s/, 'h3'], [/^##\s/, 'h3'], [/^#\s/, 'h2'], [/^>\s/, 'blockquote'],
    [/^[-*]\s/, 'ul'], [/^\d+\.\s/, 'ol'], [/^\[[ xX]?\]\s/, 'todo'], [/^---$/, 'hr'],
  ];
  function autoFormat() {
    const s = window.getSelection(); if (!s.rangeCount) return;
    const blk = directBlock(s.anchorNode); if (!blk || blk.tagName !== 'P') return;
    const text = blk.textContent; const hit = MD.find(([re]) => re.test(text)); if (!hit) return;
    const [re, kind] = hit, rest = text.replace(re, '');
    let neo, tgt;
    if (kind === 'h2' || kind === 'h3' || kind === 'blockquote') { neo = el(kind); neo.textContent = rest; tgt = neo; }
    else if (kind === 'ul' || kind === 'ol') { neo = el(kind); const li = el('li'); li.textContent = rest; neo.appendChild(li); tgt = li; }
    else if (kind === 'todo') { neo = el('ul', 'doc-task'); neo.innerHTML = `<li><span class="box"></span><span class="t">${rest || ''}</span></li>`; tgt = neo.querySelector('.t'); }
    else if (kind === 'hr') { neo = el('hr'); }
    if (!neo) return;
    blk.replaceWith(neo); neo.classList && neo.classList.add('doc-morph');
    if (kind === 'hr') { const p = el('p'); p.innerHTML = '<br>'; neo.after(p); caretTo(p, true); }
    else if (tgt) caretTo(tgt, false);
    fireChange();
  }
  function caretTo(node, atStart) { try { const r = document.createRange(); r.selectNodeContents(node); r.collapse(atStart); const s = window.getSelection(); s.removeAllRanges(); s.addRange(r); } catch (x) {} }

  // ===== 块左侧悬浮手柄：+ 插入 · ⋮⋮ 拖拽重排 / 点开菜单 =====
  let gutter = null, gutBlk = null, dragging = false, dragBlk = null, dropBefore = null, dropEl = null, didDrag = false, bmenu = null;
  function wireGutter() {
    gutter = el('div', 'doc-gut');
    gutter.innerHTML = `<button class="add" title="在此后插入">${icon('plus', 16)}</button><button class="grip" title="拖动重排 · 点击菜单">${icon('grip', 16)}</button>`;
    docEl().appendChild(gutter);
    docBody().addEventListener('mousemove', e => { if (!dragging) { const b = directBlock(e.target); if (b) showGutter(b); } });
    docBody().addEventListener('mouseleave', () => { if (!dragging && !bmenu) gutter.classList.remove('show'); });
    gutter.addEventListener('mouseenter', () => gutter.classList.add('show'));
    gutter.querySelector('.add').addEventListener('click', () => { if (gutBlk) openSlashAfter(gutBlk, gutter.getBoundingClientRect()); });
    const grip = gutter.querySelector('.grip');
    grip.addEventListener('click', () => { if (!didDrag && gutBlk) openBmenu(gutBlk); });
    grip.addEventListener('pointerdown', startDrag);
  }
  // 手柄绝对定位在 #doc 内（随块走，非浮层；浮层只给斜杠/工具条/块菜单走 Floating）。
  function showGutter(b) { gutBlk = b; gutter.classList.add('show'); const dr = docEl().getBoundingClientRect(), br = b.getBoundingClientRect(); gutter.style.left = (br.left - dr.left - 44) + 'px'; gutter.style.top = (br.top - dr.top + 1) + 'px'; }
  function startDrag(e) { if (!gutBlk) return; e.preventDefault(); dragging = true; dragBlk = gutBlk; didDrag = false; dropBefore = null; window.addEventListener('pointermove', onDrag); window.addEventListener('pointerup', endDrag, { once: true }); }
  function onDrag(e) {
    if (!didDrag) { didDrag = true; dragBlk.classList.add('doc-dragging'); document.body.style.cursor = 'grabbing'; }
    dropBefore = [...docBody().children].filter(b => b !== dragBlk).find(b => { const r = b.getBoundingClientRect(); return e.clientY < r.top + r.height / 2; }) || null;
    if (!dropEl) dropEl = docEl().appendChild(el('div', 'doc-drop'));
    const dr = docEl().getBoundingClientRect(), bodyR = docBody().getBoundingClientRect();
    const y = dropBefore ? dropBefore.getBoundingClientRect().top : docBody().lastElementChild.getBoundingClientRect().bottom;
    dropEl.style.left = (bodyR.left - dr.left) + 'px'; dropEl.style.width = docBody().clientWidth + 'px'; dropEl.style.top = (y - dr.top - 1) + 'px';
  }
  function endDrag() {
    dragging = false; document.body.style.cursor = '';
    window.removeEventListener('pointermove', onDrag);
    if (didDrag && dragBlk) { dropBefore ? docBody().insertBefore(dragBlk, dropBefore) : docBody().appendChild(dragBlk); fireChange(); }
    dragBlk && dragBlk.classList.remove('doc-dragging');
    if (dropEl) { dropEl.remove(); dropEl = null; }
    dragBlk = null; dropBefore = null; setTimeout(() => { didDrag = false; }, 0);
  }
  function openBmenu(block) {
    closeBmenu();
    const c = el('div', 'doc-bmenu');
    c.innerHTML = `<button data-a="turn"><span class="ico">${icon('edit', 15)}</span>转换成…</button>
      <button data-a="dup"><span class="ico">${icon('copy', 15)}</span>复制</button>
      <button class="danger" data-a="del"><span class="ico">${icon('trash', 15)}</span>删除</button>`;
    c.querySelectorAll('button').forEach(b => b.addEventListener('mousedown', e => { e.preventDefault(); bmAct(b.dataset.a, block); }));
    bmenu = Floating.open(gutter.getBoundingClientRect(), c, { below: true, onClose: () => { bmenu = null; } });
  }
  function closeBmenu() { if (bmenu) { bmenu.close(); bmenu = null; } }
  function bmAct(a, block) {
    if (a === 'del') { block.remove(); closeBmenu(); gutter.classList.remove('show'); fireChange(); }
    else if (a === 'dup') { const c = block.cloneNode(true); block.after(c); c.classList.add('doc-morph'); bindPills(); closeBmenu(); fireChange(); }
    else if (a === 'turn') { const r = gutter.getBoundingClientRect(); closeBmenu(); openSlashTurn(block, r); }
  }

  // ===== 装配：sea.js 调一次，把已建好的 #docBody 接管为活体编辑器 =====
  function mount(scroll, h) {
    scrollEl = scroll; hooks = h || {};
    wireToolbar(); wireSlash(); wireGutter();
  }

  return { mount, render, highlightCode };
})();

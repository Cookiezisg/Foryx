/* Forgify design-lab — 文档海洋编排（重写 v2；单独，一人负责整个 oceans/documents/ 文件夹）。
   注册进外壳：Shell.registerOcean('documents', { crumb, build(sea) })，渲染文档页到 #sea；右岛交给同目录 right-island.js。
   零-markdown 心智：① 斜杠命令窗(/) 收纳所有「插入」；② 选中即浮工具条点选格式 + AI 询问；③ 块左侧 + / ⋮⋮ 手柄；④ 行首 markdown + 空格 即输即渲。
   依赖：shared/icons.js · shared/shell.js · ./right-island.js（DocAside）。
   注：mockup 用 contenteditable + execCommand 让格式化真生效；接后端时正文序列化回单块 markdown。 */
(function () {
  const $ = (s, r = document) => r.querySelector(s);
  const el = (t, c) => { const e = document.createElement(t); if (c) e.className = c; return e; };
  const sleep = ms => new Promise(r => setTimeout(r, ms));
  let runId = 0;
  const alive = id => id === runId;

  // —— 代码语法高亮（自包含轻量 tokenizer：注释 / 字符串 / 数字 / 关键字 / 函数名）——
  const KW = new Set('const let var function def class return if else elif for while do in of import from export default new await async try except catch finally raise throw with as lambda yield true false null None True False undefined this self and or not is'.split(' '));
  const escH = s => s.replace(/[&<>]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]));
  const TOK = /(\/\/[^\n]*|#[^\n]*|\/\*[\s\S]*?\*\/)|(`(?:\\.|[^`\\])*`|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')|(\b\d+(?:\.\d+)?\b)|([A-Za-z_$][\w$]*)/g;
  function highlight(code) {
    let out = '', last = 0, m; TOK.lastIndex = 0;
    while ((m = TOK.exec(code))) {
      out += escH(code.slice(last, m.index));
      if (m[1]) out += `<span class="hl-com">${escH(m[1])}</span>`;
      else if (m[2]) out += `<span class="hl-str">${escH(m[2])}</span>`;
      else if (m[3]) out += `<span class="hl-num">${escH(m[3])}</span>`;
      else { const w = m[4]; out += KW.has(w) ? `<span class="hl-kw">${w}</span>` : (/^\s*\(/.test(code.slice(m.index + w.length)) ? `<span class="hl-fn">${escH(w)}</span>` : escH(w)); }
      last = m.index + m[0].length;
    }
    return out + escH(code.slice(last));
  }
  function highlightCode(scope) { scope.querySelectorAll('.doc-code pre').forEach(pre => { if (pre.dataset.hl) return; pre.dataset.hl = '1'; pre.innerHTML = highlight(pre.textContent); }); }

  // —— 示意正文（markdown 全类型样张；渲染产物，接后端时换真 content）——
  const BODY_HTML = `
    <p>这是 Forgify 文档海洋的 <b>markdown 全类型样张</b>。内容区<b>放宽</b>了外壳禁横线——可有 <a href="#">下划线链接</a>、分隔线、表格细线，大范围参考 Notion；但正文 <b>不用灰色填充块</b>、行内代码 <code>like_this</code> <b>不学</b> Notion 的红。</p>
    <p><b>不会 markdown 也没关系：</b>空行敲 <code>/</code> 唤出命令窗挑块；选中文字浮出工具条点选格式；块左侧悬停有 <code>+</code> 和拖拽手柄。</p>

    <h2>标题层级</h2>
    <p>层级靠字号阶梯，分节靠留白、不靠编号或下划线。</p>
    <h3>这是一个三级标题</h3>
    <p>三级标题下的正文。</p>

    <h2>文字样式</h2>
    <p>支持 <b>粗体</b>、<em>斜体</em>、<del>删除线</del>、<mark>高亮</mark>、<code>行内代码</code>，以及 <a href="#">带下划线的链接</a>。行内代码是白底 + 细描边的等宽字。</p>

    <h2>列表</h2>
    <p>无序、有序、任务三种：</p>
    <ul>
      <li>无序列表用小圆点</li>
      <li>支持嵌套
        <ul><li>第二层换成空心环</li><li>靠缩进，不画连接线</li></ul>
      </li>
      <li>项与项之间留白呼吸</li>
    </ul>
    <ol>
      <li>有序列表用等宽数字</li>
      <li>序号即层级线索</li>
      <li>同样的紧凑节奏</li>
    </ol>
    <ul class="doc-task">
      <li class="done"><span class="box">${icon('check', 12)}</span><span class="t">已完成（中性近黑实底 + 白勾）</span></li>
      <li class="done"><span class="box">${icon('check', 12)}</span><span class="t">不用强调蓝——完成是事实、非「正在发生」</span></li>
      <li><span class="box"></span><span class="t">未完成只是一个细描边空框</span></li>
    </ul>

    <h2>引用</h2>
    <blockquote>引用用左侧一道细竖线 + 文字降一档灰，白底无填充。学 Notion 的经典引用，但去掉了灰块。</blockquote>

    <h2>提示块 Callout</h2>
    <div class="doc-callout"><span class="ico">${icon('spark', 16, 1.6)}</span><div class="c"><b>这是一个 Callout。</b>白底 + 一圈描边 + 左图标，强调一段话而不靠底色。</div></div>

    <h2>代码</h2>
    <p>行内是 <code>const x = 1</code>；多行代码块带语法高亮、白底、一圈描边、右上角标语言：</p>
    <div class="doc-code"><span class="lang">ts</span><pre>// 文档正文 = 单块 markdown 字符串，整篇覆盖
async function render(md) {
  const html = await parse(md)   // 无版本 diff
  return wrap(html, { theme: "light", toc: true })
}</pre></div>
    <div class="doc-code"><span class="lang">py</span><pre># 抓取竞品动态并归并去重（每日 08:00 触发）
def weekly_digest(sources, since):
    items = []
    for url in sources:
        items += fetch(url, since)
    return summarize(dedupe(items))</pre></div>

    <h2>表格</h2>
    <table class="doc-table">
      <thead><tr><th>构件</th><th>处理</th><th>底色</th></tr></thead>
      <tbody>
        <tr><td>引用</td><td>左竖线 + 灰字</td><td>白底</td></tr>
        <tr><td>代码</td><td>白底 + 描边 + 等宽 + 高亮</td><td>白底</td></tr>
        <tr><td>表格</td><td>细线分隔</td><td>白底</td></tr>
      </tbody>
    </table>

    <h2>链接与提及</h2>
    <p>外部 <a href="#">下划线链接</a>；文档间 <span class="doc-pill"><span class="ico">${icon('link', 13)}</span>另一篇文档</span> wikilink；提及实体 <span class="doc-pill"><span class="ico">${icon('at', 13)}</span>某个 Agent</span>。</p>

    <h2>分隔线</h2>
    <p>分隔线就是一条细线：</p>
    <hr>
    <p>用来分隔大段落。</p>

    <h2>图片</h2>
    <p>图片圆角裁切、可带说明：</p>
    <div class="doc-imgph">图片占位</div>
    <div class="doc-cap">图：示意配图（mockup 占位）</div>`;

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
    { k: 'wikilink', ic: 'link', nm: '链接到文档', hint: '[[', inline: '<span class="doc-pill"><span class="ico">' + icon('link', 13) + '</span>某篇文档</span>' },
    { k: 'mention', ic: 'at', nm: '提及实体', hint: '@', inline: '<span class="doc-pill"><span class="ico">' + icon('at', 13) + '</span>某个 Agent</span>' },
    { k: 'ai', ic: 'spark', nm: '让 AI 写…', hint: '', ai: true },
  ];

  // ===== 注册 + 装配 =====
  Shell.registerOcean('documents', {
    crumb: '文档',
    build(sea) {
      sea.innerHTML = `
        <div class="doc-scroll scroll-fade" id="docScroll">
          <article class="doc" id="doc">
            <div class="doc-path" id="docPath"></div>
            <h1 class="doc-title" id="docTitle" contenteditable="true" spellcheck="false"></h1>
            <div class="doc-meta" id="docMeta"></div>
            <div class="doc-body" id="docBody" contenteditable="true" spellcheck="false"></div>
          </article>
        </div>`;
      Shell.headExtra(`
        <span class="doc-status" id="docStatus"></span>
        <button class="ibtn" id="docPanel" title="大纲 / 反链 / 信息">${icon('panel')}</button>
        <button class="ibtn" id="docReset" title="重置样张">${icon('play', 16)}</button>`);
      $('#docPanel').onclick = () => DocAside.toggle();
      $('#docReset').onclick = render;
      DocAside.ensure();
      render();
      wireToolbar();
      wireSlash();
      wireGutter();
    },
  });

  // ===== 文档头 + 渲染 =====
  function setStatus(busy) {
    const s = $('#docStatus'); if (!s) return;
    s.className = 'doc-status' + (busy ? ' busy' : '');
    s.innerHTML = busy ? `<span class="pulse"></span>AI 编辑中` : `<span class="ico">${icon('check', 14)}</span>已保存`;
  }
  function renderHead() {
    $('#docPath').innerHTML = `<span class="ico">${icon('folder', 13)}</span>
      <button class="doc-seg">产品</button><span class="sep">/</span>
      <button class="doc-seg">前端</button><span class="sep">/</span><span class="cur">文档页设计</span>`;
    $('#docTitle').textContent = '文档页设计';
    $('#docMeta').innerHTML = `<span>更新于 2 小时前</span><span class="sep">·</span><span>1.2k 字</span><span class="sep">·</span>
      <button class="backref" id="docBackref">3 个反链</button>
      <span class="doc-tags"><span class="doc-tag"><span class="ico">${icon('tag', 11)}</span>design</span><span class="doc-tag"><span class="ico">${icon('tag', 11)}</span>markdown</span></span>`;
    $('#docBackref').onclick = () => DocAside.show();
  }
  function bindPills() { $('#docBody').querySelectorAll('.doc-pill').forEach(p => p.onclick = e => { e.preventDefault(); DocAside.show(); }); }
  function render() {
    runId++;                                  // 打断进行中的 AI 流光
    const body = $('#docBody'); if (!body) return;
    closeSlash(); hideToolbar(); closeBmenu();
    renderHead();
    body.innerHTML = BODY_HTML;
    highlightCode(body);
    body.classList.remove('doc-morph'); void body.offsetWidth; body.classList.add('doc-morph');
    bindPills(); setStatus(false); DocAside.render();
    $('#docScroll').scrollTop = 0;
  }

  // ===== 定位助手（浮层贴选区/光标，限制在 #doc 内）=====
  const docBody = () => $('#docBody'), doc = () => $('#doc');
  function directBlock(node) {                // node 所在的 #docBody 直接子元素 = 一个「块」
    if (!node) return null;
    let n = node.nodeType === 3 ? node.parentNode : node;
    while (n && n.parentNode && n.parentNode !== docBody()) n = n.parentNode;
    return n && n.parentNode === docBody() ? n : null;
  }
  function caretRect() { const s = window.getSelection(); if (!s.rangeCount) return null; const r = s.getRangeAt(0).getBoundingClientRect(); return (r.width || r.height) ? r : null; }
  function place(node, rect, below) {
    const dr = doc().getBoundingClientRect();
    node.style.left = Math.min(Math.max(8, rect.left - dr.left), doc().clientWidth - node.offsetWidth - 8) + 'px';
    node.style.top = (below ? rect.bottom - dr.top + 6 : rect.top - dr.top - node.offsetHeight - 8) + 'px';
  }

  // ===== 选中工具条：点选格式化 + AI 询问 =====
  let bar = null;
  const FMT = [['bold', '<b>B</b>', '加粗'], ['italic', '<i>I</i>', '斜体'], ['strike', '<s>S</s>', '删除线'], ['mark', '<span class="hl">高</span>', '高亮'], ['code', icon('code', 14), '行内代码'], ['link', icon('link', 14), '链接']];
  function hideToolbar() { if (bar) { bar.remove(); bar = null; } }
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
    bar = el('div', 'doc-tb');
    bar.innerHTML = `<button class="ai" data-a="ai"><span class="ico">${icon('spark', 14, 1.7)}</span>AI</button><span class="sep"></span>` +
      FMT.map(([a, h, t]) => `<button data-a="${a}" title="${t}">${h}</button>`).join('');
    doc().appendChild(bar); place(bar, rect, false);
    bar.querySelectorAll('button').forEach(btn => btn.addEventListener('mousedown', e => {
      e.preventDefault();
      if (btn.dataset.a === 'ai') showAsk(rect, range); else applyFmt(btn.dataset.a, range);
    }));
  }
  function wrap(range, tag) { try { const e = el(tag); range.surroundContents(e); return e; } catch (x) { return null; } }
  function applyFmt(a, range) {
    const s = window.getSelection(); s.removeAllRanges(); s.addRange(range);
    if (a === 'bold' || a === 'italic') document.execCommand(a);
    else if (a === 'strike') document.execCommand('strikethrough');
    else if (a === 'mark' || a === 'code') wrap(range, a === 'mark' ? 'mark' : 'code');
    else if (a === 'link') { const e = wrap(range, 'a'); if (e) e.href = '#'; }
    hideToolbar();
  }
  // AI 询问：一句自然语言指令 + 快捷动作（对齐后端 :iterate）→ 选区流光改写
  function showAsk(rect, range) {
    hideToolbar();
    bar = el('div', 'doc-ask');
    bar.innerHTML = `<div class="row"><span class="ico">${icon('spark', 16, 1.7)}</span><input placeholder="让 AI 改写选中内容…" autocomplete="off"></div>
      <div class="quick">${['改简洁', '续写', '翻译成英文', '更正式'].map(t => `<button>${t}</button>`).join('')}</div>`;
    doc().appendChild(bar); place(bar, rect, true);
    const inp = $('input', bar); setTimeout(() => inp.focus(), 0);
    inp.addEventListener('keydown', e => { if (e.key === 'Enter') { e.preventDefault(); aiFlow(range); } else if (e.key === 'Escape') hideToolbar(); });
    bar.querySelectorAll('.quick button').forEach(b => b.addEventListener('mousedown', e => { e.preventDefault(); aiFlow(range); }));
  }
  async function aiFlow(range) {
    const id = ++runId; hideToolbar(); setStatus(true);
    const span = wrap(range, 'span'); if (span) span.className = 'doc-flow';
    window.getSelection()?.removeAllRanges();
    await sleep(1500);
    if (span) { span.classList.remove('doc-flow'); const p = span.parentNode; while (span.firstChild) p.insertBefore(span.firstChild, span); p.removeChild(span); p.normalize && p.normalize(); }
    if (alive(id)) setStatus(false);
  }

  // ===== 斜杠命令窗 =====
  let menu = null, slash = null, onIdx = 0, flat = [];
  const slashOpen = () => !!menu;
  function wireSlash() {
    const b = docBody();
    b.addEventListener('input', () => { if (!detectSlash()) { closeSlash(); autoFormat(); } });
    document.addEventListener('keydown', onSlashKey, true);
    document.addEventListener('mousedown', e => { if (menu && !menu.contains(e.target)) closeSlash(); });
  }
  function detectSlash() {
    const s = window.getSelection(); if (!s.rangeCount) return false;
    const r = s.getRangeAt(0); if (r.startContainer.nodeType !== 3) return false;
    const m = r.startContainer.textContent.slice(0, r.startOffset).match(/(?:^|\s)\/([^\s/]*)$/); if (!m) return false;
    slash = { mode: 'type', node: r.startContainer, start: r.startOffset - m[1].length - 1, end: r.startOffset, query: m[1] };
    openSlash(); return true;
  }
  function openSlashAfter(block) { slash = { mode: 'plus', host: block }; openSlash(gutter.getBoundingClientRect()); }
  function openSlashTurn(block) { slash = { mode: 'turn', host: block }; openSlash(gutter.getBoundingClientRect()); }
  function matched() {
    const q = (slash.query || '').toLowerCase(); const out = []; let grp = null;
    BLOCKS.forEach(it => {
      if (it.grp) { grp = it; return; }
      if (!q || it.nm.toLowerCase().includes(q) || it.k.includes(q) || (it.hint || '').includes(q)) { if (grp) { out.push(grp); grp = null; } out.push(it); }
    });
    return out;
  }
  function openSlash(anchor) {
    const list = matched(); flat = list.filter(x => !x.grp);
    if (!menu) { menu = el('div', 'doc-slash'); doc().appendChild(menu); onIdx = 0; }
    if (onIdx >= flat.length) onIdx = 0;
    menu.innerHTML = flat.length ? list.map(it => it.grp
      ? `<div class="grp">${it.grp}</div>`
      : `<div class="item${flat[onIdx] === it ? ' on' : ''}" data-k="${it.k}"><span class="ic">${icon(it.ic, 16)}</span><span class="nm">${it.nm}</span>${it.hint ? `<span class="hint">${it.hint}</span>` : ''}</div>`).join('')
      : `<div class="empty">没有匹配「${slash.query}」的块</div>`;
    menu.querySelectorAll('.item').forEach(it => {
      it.addEventListener('mousedown', e => { e.preventDefault(); choose(flat.find(x => x.k === it.dataset.k)); });
      it.addEventListener('mousemove', () => { const i = flat.findIndex(x => x.k === it.dataset.k); if (i !== onIdx) { onIdx = i; paintOn(); } });
    });
    const rect = anchor || caretRect(); if (rect) place(menu, rect, true);
  }
  function paintOn() { menu.querySelectorAll('.item').forEach(it => it.classList.toggle('on', flat[onIdx] && it.dataset.k === flat[onIdx].k)); }
  function onSlashKey(e) {
    if (!menu) return;
    if (e.key === 'ArrowDown') { e.preventDefault(); onIdx = (onIdx + 1) % flat.length; paintOn(); seeOn(); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); onIdx = (onIdx - 1 + flat.length) % flat.length; paintOn(); seeOn(); }
    else if (e.key === 'Enter') { e.preventDefault(); choose(flat[onIdx]); }
    else if (e.key === 'Escape') { e.preventDefault(); closeSlash(); }
  }
  function seeOn() { const on = menu.querySelector('.item.on'); on && on.scrollIntoView({ block: 'nearest' }); }
  function closeSlash() { if (menu) { menu.remove(); menu = null; } slash = null; }
  function choose(block) {
    if (!block || !slash) return closeSlash();
    const mode = slash.mode;
    let host;
    if (mode === 'type') { const { node, start, end } = slash; try { node.textContent = node.textContent.slice(0, start) + node.textContent.slice(end); } catch (x) {} host = directBlock(node) || docBody().lastElementChild; }
    else host = slash.host;
    closeSlash(); if (!host) return;
    if (block.inline) { host.insertAdjacentHTML('beforeend', ' ' + block.inline); bindPills(); }
    else if (block.ai) { host.insertAdjacentHTML('afterend', '<p></p>'); aiWrite(host.nextElementSibling); }
    else if (mode === 'turn') { host.insertAdjacentHTML('beforebegin', block.html); const f = host.previousElementSibling; host.remove(); f && f.classList.add('doc-morph'); bindPills(); }
    else { host.insertAdjacentHTML('afterend', block.html); const f = host.nextElementSibling; if (mode === 'type' && host.tagName === 'P' && !host.textContent.trim()) host.remove(); f && (f.classList.add('doc-morph'), f.scrollIntoView({ block: 'nearest' })); bindPills(); highlightCode(docBody()); }
    DocAside.render();
  }
  async function aiWrite(p) {
    if (!p) return; const id = ++runId; setStatus(true);
    const span = el('span', 'doc-flow'); p.appendChild(span); p.scrollIntoView({ block: 'nearest' });
    await typeInto(span, 'AI 根据上下文续写的一段内容，落定后从流光沉淀为正文。');
    span.classList.remove('doc-flow'); const par = span.parentNode; while (span.firstChild) par.insertBefore(span.firstChild, span); par.removeChild(span);
    if (alive(id)) setStatus(false);
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
    else if (kind === 'todo') { neo = el('ul', 'doc-task'); neo.innerHTML = `<li><span class="box"></span><span class="t">${rest || ''}</span></li>`; tgt = $('.t', neo); }
    else if (kind === 'hr') { neo = el('hr'); }
    if (!neo) return;
    blk.replaceWith(neo); neo.classList && neo.classList.add('doc-morph');
    if (kind === 'hr') { const p = el('p'); p.innerHTML = '<br>'; neo.after(p); caretTo(p, true); }
    else if (tgt) caretTo(tgt, false);
    DocAside.render();
  }
  function caretTo(node, atStart) { try { const r = document.createRange(); r.selectNodeContents(node); r.collapse(atStart); const s = window.getSelection(); s.removeAllRanges(); s.addRange(r); } catch (x) {} }

  // ===== 块左侧悬浮手柄：+ 插入 · ⋮⋮ 拖拽重排 / 点开菜单 =====
  let gutter = null, gutBlk = null, dragging = false, dragBlk = null, dropBefore = null, dropEl = null, didDrag = false, bmenu = null;
  function wireGutter() {
    gutter = el('div', 'doc-gut');
    gutter.innerHTML = `<button class="add" title="在此后插入">${icon('plus', 16)}</button><button class="grip" title="拖动重排 · 点击菜单">${icon('grip', 16)}</button>`;
    doc().appendChild(gutter);
    docBody().addEventListener('mousemove', e => { if (!dragging) { const b = directBlock(e.target); if (b) showGutter(b); } });
    docBody().addEventListener('mouseleave', () => { if (!dragging && !bmenu) gutter.classList.remove('show'); });
    gutter.addEventListener('mouseenter', () => gutter.classList.add('show'));
    $('.add', gutter).addEventListener('click', () => { if (gutBlk) openSlashAfter(gutBlk); });
    const grip = $('.grip', gutter);
    grip.addEventListener('click', () => { if (!didDrag && gutBlk) openBmenu(gutBlk); });
    grip.addEventListener('pointerdown', startDrag);
  }
  function showGutter(b) { gutBlk = b; gutter.classList.add('show'); const dr = doc().getBoundingClientRect(), br = b.getBoundingClientRect(); gutter.style.left = (br.left - dr.left - 44) + 'px'; gutter.style.top = (br.top - dr.top + 1) + 'px'; }
  function startDrag(e) { if (!gutBlk) return; e.preventDefault(); dragging = true; dragBlk = gutBlk; didDrag = false; dropBefore = null; window.addEventListener('pointermove', onDrag); window.addEventListener('pointerup', endDrag, { once: true }); }
  function onDrag(e) {
    if (!didDrag) { didDrag = true; dragBlk.classList.add('doc-dragging'); document.body.style.cursor = 'grabbing'; }
    dropBefore = [...docBody().children].filter(b => b !== dragBlk).find(b => { const r = b.getBoundingClientRect(); return e.clientY < r.top + r.height / 2; }) || null;
    if (!dropEl) dropEl = doc().appendChild(el('div', 'doc-drop'));
    const dr = doc().getBoundingClientRect(), bodyR = docBody().getBoundingClientRect();
    const y = dropBefore ? dropBefore.getBoundingClientRect().top : docBody().lastElementChild.getBoundingClientRect().bottom;
    dropEl.style.left = (bodyR.left - dr.left) + 'px'; dropEl.style.width = docBody().clientWidth + 'px'; dropEl.style.top = (y - dr.top - 1) + 'px';
  }
  function endDrag() {
    dragging = false; document.body.style.cursor = '';
    window.removeEventListener('pointermove', onDrag);
    if (didDrag && dragBlk) { dropBefore ? docBody().insertBefore(dragBlk, dropBefore) : docBody().appendChild(dragBlk); DocAside.render(); }
    dragBlk && dragBlk.classList.remove('doc-dragging');
    if (dropEl) { dropEl.remove(); dropEl = null; }
    dragBlk = null; dropBefore = null; setTimeout(() => { didDrag = false; }, 0);
  }
  function openBmenu(block) {
    closeBmenu();
    bmenu = el('div', 'doc-bmenu');
    bmenu.innerHTML = `<button data-a="turn"><span class="ico">${icon('edit', 15)}</span>转换成…</button>
      <button data-a="dup"><span class="ico">${icon('copy', 15)}</span>复制</button>
      <button class="danger" data-a="del"><span class="ico">${icon('trash', 15)}</span>删除</button>`;
    doc().appendChild(bmenu);
    const dr = doc().getBoundingClientRect(), gr = gutter.getBoundingClientRect();
    bmenu.style.left = (gr.left - dr.left) + 'px'; bmenu.style.top = (gr.bottom - dr.top + 4) + 'px';
    bmenu.querySelectorAll('button').forEach(b => b.addEventListener('mousedown', e => { e.preventDefault(); bmAct(b.dataset.a, block); }));
    setTimeout(() => document.addEventListener('mousedown', bmOut), 0);
  }
  function bmOut(e) { if (bmenu && !bmenu.contains(e.target)) closeBmenu(); }
  function closeBmenu() { if (bmenu) { bmenu.remove(); bmenu = null; document.removeEventListener('mousedown', bmOut); } }
  function bmAct(a, block) {
    if (a === 'del') { block.remove(); closeBmenu(); gutter.classList.remove('show'); DocAside.render(); }
    else if (a === 'dup') { const c = block.cloneNode(true); block.after(c); c.classList.add('doc-morph'); bindPills(); closeBmenu(); DocAside.render(); }
    else if (a === 'turn') { closeBmenu(); openSlashTurn(block); }
  }
})();

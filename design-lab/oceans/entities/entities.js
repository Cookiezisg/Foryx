/* Forgify design-lab — 实体海洋编排（产品级重做 · 终极优化：同源 documents 美学）。
   静态「完整展示 + 必要信息 + 调试/修改平台」。窄阅读列 + 大留白 + 字号阶梯分层 + 零灰盒；
   字段=细线表格/定义行，代码=白底细描边块(可改+活体高亮)，标签=细描边药丸(可增删)，
   操作进 Shell.headExtra，版本=diff(绿增红删+高亮)，Workflow=图(拖拽/选中/检视/增删)，运行=账本。
   选中通道 Shell.openEntity(id)。代码高亮 --cd-* 同 documents。依赖 shared/icons.js + shared/shell.js。纯静态。 */
(function () {
  const $ = (s, r = document) => r.querySelector(s);
  const el = (t, c) => { const e = document.createElement(t); if (c) e.className = c; return e; };
  const esc = s => String(s == null ? '' : s).replace(/[&<>]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]));
  const sleep = ms => new Promise(r => setTimeout(r, ms));

  const KIND = {
    function: { icon: 'function', label: 'Function', verb: 'Run', vico: 'play' },
    handler:  { icon: 'handler',  label: 'Handler',  verb: 'Restart', vico: 'spin' },
    agent:    { icon: 'agent',    label: 'Agent',    verb: 'Invoke', vico: 'play' },
    workflow: { icon: 'workflow', label: 'Workflow', verb: 'Trigger', vico: 'zap' },
    trigger:  { icon: 'trigger',  label: 'Trigger',  verb: 'Fire', vico: 'zap' },
    control:  { icon: 'control',  label: 'Control',  verb: 'Probe', vico: 'play' },
    approval: { icon: 'shield',   label: 'Approval', verb: 'Render', vico: 'play' },
    mcp:      { icon: 'mcp',      label: 'MCP server', verb: 'Reconnect', vico: 'spin' },
    skill:    { icon: 'skill',    label: 'Skill',    verb: 'Render', vico: 'play' },
  };
  const ST = { done: '就绪', run: '运行中', wait: '需处理', err: '失败', idle: '闲置', listening: '监听中' };
  const PFX = { function: 'fn', handler: 'hd', agent: 'ag', workflow: 'wf', trigger: 'trg', control: 'ctl', approval: 'apr', mcp: 'mcp', skill: 'skl' };
  const fauxId = (kind, id) => { let h = 2166136261; for (const c of id) { h ^= c.charCodeAt(0); h = (h * 16777619) >>> 0; } return (PFX[kind] || 'en') + '_' + h.toString(16).padStart(8, '0'); };
  const NICON = { trigger: 'trigger', action: 'action', agent: 'agent', control: 'control', approval: 'shield' };

  // ===== 代码高亮（Atom One；tokenizer 同 documents；+ $n/${}/CEL arg）=====
  const KW = new Set('const let var function def class return if elif else for while do in of import from export default new await async try except catch finally raise throw with as lambda yield and or not is None True False true false null undefined self this match case pass break continue'.split(' '));
  const TOK = /(#[^\n]*|\/\/[^\n]*|\/\*[\s\S]*?\*\/)|(`(?:\\.|[^`\\])*`|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')|(\$\{[^}]*\}|\$\d+|\{\{[^}]*\}\})|(\b\d+(?:\.\d+)?\b)|([A-Za-z_$][\w$]*)/g;
  function hl(code) {
    let out = '', last = 0, m; TOK.lastIndex = 0;
    while ((m = TOK.exec(code))) {
      out += esc(code.slice(last, m.index));
      if (m[1]) out += `<span class="hl-com">${esc(m[1])}</span>`;
      else if (m[2]) out += `<span class="hl-str">${esc(m[2])}</span>`;
      else if (m[3]) out += `<span class="hl-arg">${esc(m[3])}</span>`;
      else if (m[4]) out += `<span class="hl-num">${esc(m[4])}</span>`;
      else { const w = m[5]; out += KW.has(w) ? `<span class="hl-kw">${w}</span>` : (/^\s*\(/.test(code.slice(m.index + w.length)) ? `<span class="hl-fn">${esc(w)}</span>` : esc(w)); }
      last = m.index + m[0].length;
    }
    return out + esc(code.slice(last));
  }

  // dirty / 保存态（保存指示在 headExtra）
  let dirtyEl = null, savedEl = null;
  function markDirty() { if (dirtyEl) dirtyEl.classList.add('dirty'); if (savedEl) savedEl.innerHTML = `<span class="ico">${icon('edit', 13)}</span>未保存`; }
  function wireEdit(scope) { scope.querySelectorAll('[contenteditable]').forEach(c => c.addEventListener('input', markDirty)); }

  // ===== 组件 =====
  function sec(host, label, cnt, addCb) {
    const s = el('div', 'eo-sec');
    s.innerHTML = `<div class="eo-h"><span>${esc(label)}</span>${cnt != null ? `<span class="cnt">${cnt}</span>` : ''}${addCb ? `<span class="add">${icon('plus', 13)}添加</span>` : ''}</div>`;
    if (addCb) s.querySelector('.add').onclick = addCb;
    host.appendChild(s); return s;
  }
  function codeEditor(host, { code = '', corner = '', readOnly = false } = {}) {
    const w = el('div', 'eo-code');
    w.innerHTML = `<div class="gut"></div><div class="area"><pre><code></code></pre><textarea spellcheck="false" ${readOnly ? 'readonly' : ''}></textarea></div>${corner ? `<span class="lang">${esc(corner)}</span>` : ''}`;
    const ta = w.querySelector('textarea'), codeEl = w.querySelector('code'), gut = w.querySelector('.gut'), pre = w.querySelector('pre');
    const paint = () => { codeEl.innerHTML = hl(ta.value); const n = ta.value.split('\n').length; gut.innerHTML = Array.from({ length: n }, (_, i) => `<i>${i + 1}</i>`).join(''); };
    ta.value = code; paint();
    ta.addEventListener('input', () => { paint(); markDirty(); });
    ta.addEventListener('scroll', () => { pre.scrollTop = ta.scrollTop; pre.scrollLeft = ta.scrollLeft; });
    ta.addEventListener('keydown', e => { if (e.key === 'Tab') { e.preventDefault(); const s = ta.selectionStart, en = ta.selectionEnd; ta.value = ta.value.slice(0, s) + '    ' + ta.value.slice(en); ta.selectionStart = ta.selectionEnd = s + 4; paint(); markDirty(); } });
    host.appendChild(w); return w;
  }
  function prose(host, { value = '', cls = 'eo-desc', ph } = {}) {
    const p = el('div', cls); p.contentEditable = 'true'; p.spellcheck = false; if (ph) p.dataset.ph = ph; p.textContent = value;
    p.addEventListener('input', markDirty); host.appendChild(p); return p;
  }
  function tags(host, { items = [], icon: ic, kind = 'multi', addLabel = 'add' } = {}) {
    const box = el('div', 'eo-tags');
    const mk = (t, health) => { const c = el('span', 'eo-tag'); c.innerHTML = `${ic ? `<span class="ico">${icon(ic, 12)}</span>` : ''}${health ? `<span class="mh ${health}"></span>` : ''}<span>${esc(t)}</span><span class="x">${icon('close', 11)}</span>`; c.querySelector('.x').onclick = () => { c.remove(); markDirty(); }; return c; };
    if (!items.length) box.appendChild(Object.assign(el('span', 'eo-none'), { textContent: '— 无 —' }));
    items.forEach(it => box.appendChild(typeof it === 'string' ? mk(it) : mk(it.label, it.health)));
    const add = el('span', 'eo-tag add'); add.innerHTML = `${icon('plus', 11)}<span>${addLabel}</span>`;
    add.onclick = () => { const n = (window.prompt && prompt(addLabel)) || ''; if (!n) return; if (kind === 'single') box.querySelectorAll('.eo-tag:not(.add)').forEach(x => x.remove()); const empty = box.querySelector('.eo-none'); if (empty) empty.remove(); box.insertBefore(mk(n), add); markDirty(); };
    box.appendChild(add); host.appendChild(box); return box;
  }
  // 细线表格
  function table(host, cols, rows) {
    const t = el('table', 'eo-tbl');
    t.innerHTML = `<thead><tr>${cols.map(c => `<th>${esc(c)}</th>`).join('')}</tr></thead><tbody>${rows.map(r => `<tr${r._cls ? ` class="${r._cls}"` : ''}>${r.map(c => `<td>${c}</td>`).join('')}</tr>`).join('')}</tbody>`;
    wireEdit(t); host.appendChild(t); return t;
  }
  // 定义行（k/v）
  function defs(host, rows) {
    const box = el('div');
    rows.forEach(([k, v, opt]) => { const r = el('div', 'eo-dl-row'); const o = opt || {}; r.innerHTML = `<span class="k">${esc(k)}</span><span class="v"${o.edit ? ' contenteditable="true"' : ''}>${o.html || esc(v)}</span>`; box.appendChild(r); });
    wireEdit(box); host.appendChild(box); return box;
  }
  const ENV = { pending: ['pending', '排队'], syncing: ['syncing', '物化中…'], ready: ['ready', '就绪'], failed: ['failed', '失败'] };
  const CFG = { ready: ['ready', '就绪'], partially_configured: ['syncing', '部分配置'], unconfigured: ['pending', '未配置'] };
  const CONN = { ready: ['ready', 'connected'], failed: ['failed', 'auth required'] };
  const badge = (map, k) => { const [c, t] = map[k] || Object.values(map)[0]; return `<span class="eo-badge ${c}"><span class="dot"></span>${t}</span>`; };

  // 行 diff（LCS）
  function lineDiff(a, b) {
    const A = a.split('\n'), B = b.split('\n'), m = A.length, n = B.length;
    const dp = Array.from({ length: m + 1 }, () => new Array(n + 1).fill(0));
    for (let i = m - 1; i >= 0; i--) for (let j = n - 1; j >= 0; j--) dp[i][j] = A[i] === B[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
    const o = []; let i = 0, j = 0;
    while (i < m && j < n) { if (A[i] === B[j]) { o.push(['ctx', A[i]]); i++; j++; } else if (dp[i + 1][j] >= dp[i][j + 1]) o.push(['del', A[i++]]); else o.push(['add', B[j++]]); }
    while (i < m) o.push(['del', A[i++]]); while (j < n) o.push(['add', B[j++]]); return o;
  }
  function versionDiff(host, { versions, field, cap }) {
    const w = el('div', 'eo-vers');
    w.innerHTML = `<div class="eo-vlist"></div><div class="eo-diff"><div class="eo-diff-cap"></div><div class="eo-diff-body"></div></div>`;
    const list = w.querySelector('.eo-vlist'), capE = w.querySelector('.eo-diff-cap'), body = w.querySelector('.eo-diff-body'); let sel = 0;
    function paint() {
      list.innerHTML = versions.map((v, i) => `<div class="eo-vrow${v.active ? ' cur' : ''}${i === sel ? ' on' : ''}" data-i="${i}"><span class="vn">v${v.n}</span><span class="vt">${esc(v.t || '')}</span><span class="vd">${esc(v.reason || '')}</span></div>`).join('');
      list.querySelectorAll('.eo-vrow').forEach(r => r.onclick = () => { sel = +r.dataset.i; paint(); });
      const nv = versions[sel], ov = versions[sel + 1];
      if (!ov) { capE.innerHTML = `<span class="mono">v${nv.n}</span><span>· 最早版本</span>`; body.innerHTML = field(nv).split('\n').map((l, i) => `<div class="eo-dl"><span class="ln">${i + 1}</span><span class="sg"></span><span class="ct">${hl(l)}</span></div>`).join(''); return; }
      const d = lineDiff(field(ov), field(nv)); let ln = 0, ad = 0, de = 0;
      body.innerHTML = d.map(([op, t]) => { const sg = op === 'add' ? '+' : op === 'del' ? '−' : ' '; if (op === 'add') ad++; if (op === 'del') de++; return `<div class="eo-dl ${op}"><span class="ln">${op === 'del' ? '' : ++ln}</span><span class="sg">${sg}</span><span class="ct">${hl(t)}</span></div>`; }).join('');
      capE.innerHTML = `<span class="mono">v${ov.n} → v${nv.n}</span><span>· ${esc(cap)}</span><span class="pm"><span class="a">+${ad}</span> <span class="d">−${de}</span></span>`;
    }
    paint(); host.appendChild(w); return w;
  }

  // 运行/调试面板（白底细描边块）
  function runDebug(host, { argsSeed = '', verb = 'Run', vico = 'play', gate = null, trace } = {}) {
    const w = el('div', 'eo-debug');
    if (gate) { w.innerHTML = `<div class="eo-gate"><span class="ico">${icon('shield', 15)}</span>${esc(gate)}</div>`; host.appendChild(w); return w; }
    w.innerHTML = `<div class="eo-dbg-args"></div><div class="eo-dbg-bar"><span class="tk">输入参数</span><span class="grow"></span><button class="eo-run" data-go>${icon(vico, 13)}${esc(verb)}</button></div><div class="eo-dbg-out"></div><div class="eo-dbg-res"></div>`;
    codeEditor(w.querySelector('.eo-dbg-args'), { code: argsSeed || '{}', corner: 'args' });
    const tk = w.querySelector('.tk'), btn = w.querySelector('[data-go]'), out = w.querySelector('.eo-dbg-out'), res = w.querySelector('.eo-dbg-res');
    btn.onclick = async () => {
      btn.style.pointerEvents = 'none'; res.classList.remove('show'); out.classList.add('show'); out.textContent = '';
      tk.textContent = '运行中…'; tk.classList.add('eo-shimmer');
      const lines = (trace && trace.lines) || ['→ spawn sandbox', '→ exec', 'stdout: ok'];
      for (const l of lines) { await sleep(240); out.textContent += l + '\n'; }
      await sleep(200); tk.textContent = '输入参数'; tk.classList.remove('eo-shimmer'); btn.style.pointerEvents = '';
      const r = (trace && trace.result) || { st: 'ok', out: 'done', ms: 100 };
      res.className = 'eo-dbg-res show';
      res.innerHTML = `<span class="eo-st ${r.st === 'ok' ? 'done' : 'err'}"><span class="dot"></span>${r.st}</span><span class="ev">${esc(r.out)}</span><span class="ms">${r.ms}ms</span>`;
    };
    host.appendChild(w); return w;
  }

  // 右岛抽屉
  let asideEl = null;
  function asideShow(title, ticon, fn) {
    if (!asideEl || !document.body.contains(asideEl)) { asideEl = el('aside', 'eo-aside'); asideEl.setAttribute('data-ocean-right', 'entities'); Shell.body.appendChild(asideEl); }
    asideEl.classList.add('show');
    asideEl.innerHTML = `<div class="eo-aside-h"><span class="gico">${icon(ticon, 16)}</span><b>${esc(title)}</b><button class="ibtn" data-x>${icon('close', 15)}</button></div><div class="eo-aside-b"></div>`;
    asideEl.querySelector('[data-x]').onclick = asideHide; fn(asideEl.querySelector('.eo-aside-b'));
  }
  function asideHide() { if (asideEl) asideEl.classList.remove('show'); }
  function relations(host, groups) {
    (groups || []).forEach(g => { const s = el('div', 'eo-rel-sec'); s.innerHTML = `<div class="eo-rel-h">${esc(g.title)}</div>` + (g.rows.length ? g.rows.map(r => `<div class="eo-rel-row"><span class="nico">${icon(KIND[r.kind] ? KIND[r.kind].icon : (r.kind || 'link'), 14)}</span><span class="rn">${esc(r.name)}</span><span class="rm">${esc(r.meta || '')}</span></div>`).join('') : `<div class="eo-none" style="padding:0 9px">— 无 —</div>`); host.appendChild(s); });
  }

  // Workflow 图编辑器
  function graphEditor(host, wf) {
    const w = el('div', 'eo-gwrap');
    w.innerHTML = `<div class="eo-gbar"><button class="eo-pal" data-add="action">${icon('action', 13)}节点</button><button class="eo-pal" data-add="control">${icon('control', 13)}分支</button><button class="eo-pal" data-add="approval">${icon('shield', 13)}审批</button><span class="grow"></span><button class="eo-pal" data-cap>${icon('check', 13)}能力校验</button></div>
      <div class="eo-canvas"><div class="eo-gc-inner"><svg class="edges"><defs><marker id="eoArrow" markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto"><path d="M0,0 L7,3.5 L0,7 z"/></marker></defs><g class="eo-edges"></g></svg></div></div>
      <div class="eo-ghint">拖动节点重排 · 点击选中并编辑 · 调色板加节点 · 能力校验标红不可解析引用</div>`;
    const canvas = w.querySelector('.eo-canvas'), inner = w.querySelector('.eo-gc-inner'), edgeG = w.querySelector('.eo-edges'), NW = 148, NH = 46, nodeEls = {};
    const sizeInner = () => { let mx = 0, my = 0; wf.nodes.forEach(n => { mx = Math.max(mx, n.x + NW); my = Math.max(my, n.y + NH); }); inner.style.width = (mx + 40) + 'px'; inner.style.height = (my + 40) + 'px'; };
    const drawEdges = () => { edgeG.innerHTML = wf.edges.map(([a, b]) => { const s = wf.nodes.find(n => n.id === a), t = wf.nodes.find(n => n.id === b); if (!s || !t) return ''; const x1 = s.x + NW, y1 = s.y + NH / 2, x2 = t.x, y2 = t.y + NH / 2, mx = (x1 + x2) / 2; return `<path d="M${x1},${y1} C${mx},${y1} ${mx},${y2} ${x2 - 7},${y2}" marker-end="url(#eoArrow)"/>`; }).join(''); sizeInner(); };
    const place = n => { const e = nodeEls[n.id]; e.style.left = n.x + 'px'; e.style.top = n.y + 'px'; };
    function select(n) {
      Object.values(nodeEls).forEach(e => e.classList.toggle('sel', n && e.dataset.id === n.id));
      if (!n) return asideHide();
      asideShow(n.id, NICON[n.kind] || 'action', b => {
        const f1 = el('div', 'eo-fld'); f1.innerHTML = `<label>类型</label><span class="eo-ref" style="cursor:default">${n.kind}</span>`; b.appendChild(f1);
        const f2 = el('div', 'eo-fld'); f2.innerHTML = `<label>引用 (ref)</label>`; b.appendChild(f2); tags(f2, { items: [n.ref], icon: NICON[n.kind] || 'action', kind: 'single', addLabel: '改引用' });
        const f3 = el('div', 'eo-fld'); f3.innerHTML = `<label>${n.kind === 'control' ? '分支条件 (CEL)' : 'Input 映射 (CEL)'}</label>`; b.appendChild(f3); codeEditor(f3, { code: n.cel || (n.kind === 'control' ? 'amount > 1000' : 'input: ctx.upstream.result'), corner: 'cel' });
        const del = el('button', 'eo-del'); del.innerHTML = `${icon('trash', 14)}删除节点`; del.onclick = () => { wf.nodes = wf.nodes.filter(x => x.id !== n.id); wf.edges = wf.edges.filter(([a, c]) => a !== n.id && c !== n.id); nodeEls[n.id].remove(); delete nodeEls[n.id]; drawEdges(); asideHide(); markDirty(); }; b.appendChild(del);
      });
    }
    function mk(n) {
      const e = el('div', 'eo-gn' + (n.id === wf.running ? ' run' : n.parked ? ' parked' : '')); e.dataset.id = n.id;
      e.innerHTML = `<span class="gico">${icon(NICON[n.kind] || 'action', 16)}</span><span class="gt"><b>${esc(n.id)}</b><span>${esc(n.ref || n.kind)}</span></span>`;
      nodeEls[n.id] = e; inner.appendChild(e); place(n);
      let sx, sy, ox, oy, moved = false, drag = false;
      e.addEventListener('pointerdown', ev => { drag = true; moved = false; sx = ev.clientX; sy = ev.clientY; ox = n.x; oy = n.y; e.classList.add('dragging'); try { e.setPointerCapture(ev.pointerId); } catch (x) { } ev.preventDefault(); });
      e.addEventListener('pointermove', ev => { if (!drag) return; const dx = ev.clientX - sx, dy = ev.clientY - sy; if (Math.abs(dx) + Math.abs(dy) > 3) moved = true; n.x = Math.max(0, ox + dx); n.y = Math.max(0, oy + dy); place(n); drawEdges(); });
      e.addEventListener('pointerup', () => { drag = false; e.classList.remove('dragging'); if (!moved) select(n); else markDirty(); });
      return e;
    }
    wf.nodes.forEach(mk); drawEdges();
    w.querySelectorAll('[data-add]').forEach(btn => btn.onclick = () => { const kind = btn.dataset.add, id = kind + '_' + (wf.nodes.length + 1), n = { id, kind, ref: '— 待设 —', x: 40 + (wf.nodes.length % 4) * 26, y: 40 + (wf.nodes.length % 5) * 22 }; wf.nodes.push(n); mk(n); markDirty(); select(n); });
    w.querySelector('[data-cap]').onclick = () => { let bad = 0; wf.nodes.forEach(n => { const ok = !/待设|—|undefined/.test(n.ref || ''); nodeEls[n.id].classList.toggle('problem', !ok); if (!ok) bad++; }); w.querySelector('.eo-ghint').textContent = bad ? `能力校验：${bad} 个节点引用不可解析（已标红）` : '能力校验：全部引用可解析 ✓'; };
    canvas.onclick = ev => { if (ev.target === canvas || ev.target === inner || ev.target.tagName === 'svg') select(null); };
    host.appendChild(w); return w;
  }

  // ===================== 数据 =====================
  const PY5 = `def process_invoice(file: bytes, currency: str = "USD") -> Invoice:
    doc = pdfplumber.open(io.BytesIO(file))
    text = "\\n".join(p.extract_text() for p in doc.pages)
    fields = extract_fields(text)          # 正则 + 版面启发式
    inv = Invoice.model_validate(fields)   # pydantic 校验
    inv.currency = currency
    return inv`;
  const PY4 = `def process_invoice(file: bytes) -> Invoice:
    doc = pdfplumber.open(io.BytesIO(file))
    text = "\\n".join(p.extract_text() for p in doc.pages)
    fields = extract_fields(text)
    return Invoice.model_validate(fields)`;
  const D = {
    process_invoice: { kind: 'function', version: 5, status: 'done', runs: 1284, desc: '解析上传的发票 PDF / 图片，抽取结构化字段（供应商、金额、税号、行项目），校验后返回 JSON。',
      code: PY5, lang: 'python 3.12', deps: ['pdfplumber', 'pydantic', 'regex'], inputs: [['file', 'bytes', true], ['currency', 'string', false]], output: [['Invoice', 'vendor·total·tax_id·line_items[]']], env: 'ready', lastRun: '2 小时前 · 成功',
      versions: [{ n: 5, t: '2 小时前', reason: '加 currency 入参 + 注释', active: true, code: PY5 }, { n: 4, t: '昨天', reason: '抽出 extract_fields 启发式', code: PY4 }, { n: 1, t: '上周', reason: '初版', code: 'def process_invoice(file):\n    return parse(file)' }],
      execs: [['ok', 'chat', '412ms', '2 小时前'], ['ok', 'workflow', '388ms', '今天 02:00'], ['failed', 'manual', '—', '昨天'], ['ok', 'agent', '455ms', '昨天']],
      rel: [{ title: 'Forged in', rows: [{ kind: 'chat', name: '前端设计 (fork)', meta: 'cv_3f' }] }, { title: 'Used by', rows: [{ kind: 'workflow', name: 'invoice_flow', meta: 'extract' }, { kind: 'agent', name: 'summarizer', meta: 'tool' }] }] },
    fetch_news: { kind: 'function', version: 2, status: 'run', runs: 96, desc: '按关键词拉取多源 RSS / 新闻，去重归并。', lang: 'python 3.12', env: 'syncing', code: 'def fetch_news(topics: list[str], since=None) -> list[NewsItem]:\n    items = []\n    for feed in resolve_feeds(topics):\n        items += parse_feed(feed, since)\n    return dedupe(items, key=lambda i: i.url)', deps: ['requests', 'feedparser'], inputs: [['topics', 'string[]', true], ['since', 'datetime', false]], output: [['NewsItem[]', 'title·url·source']] },
    parse_pdf: { kind: 'function', version: 1, status: 'err', runs: 7, desc: 'PDF 文本层抽成纯文本块。依赖物化失败。', lang: 'python 3.12', env: 'failed', code: 'def parse_pdf(file: bytes) -> list[TextBlock]:\n    return [TextBlock(page=i, text=p.extract_text())\n            for i, p in enumerate(pdfplumber.open(file).pages)]', deps: ['pdfplumber'], inputs: [['file', 'bytes', true]], output: [['TextBlock[]', 'page·text']] },
    slack_handler: { kind: 'handler', version: 3, status: 'done', runs: 512, desc: '常驻 Slack 连接：发消息、建频道、上传、监听事件，维护长连 socket 会话。', lang: 'python 3.12', life: 'active', env: 'ready', cfg: 'ready',
      code: 'class SlackHandler(Handler):\n    def __init__(self, bot_token, app_token, default_channel="#ops"):\n        self.client = WebClient(bot_token)\n        self.socket = SocketModeClient(app_token)\n\n    def shutdown(self):\n        self.socket.close()',
      methods: [['post_message', '(channel, text)'], ['create_channel', '(name)'], ['upload_file', '(path, channel)'], ['on_event', '(type, cb)']], initArgs: [['bot_token', null, true], ['app_token', null, true], ['default_channel', '#ops', false]] },
    db_pool: { kind: 'handler', version: 2, status: 'wait', runs: 0, desc: 'PostgreSQL 连接池。缺密码，未上线。', lang: 'python 3.12', life: 'inactive', env: 'pending', cfg: 'partially_configured', code: 'class DbPool(Handler):\n    def __init__(self, dsn, password, pool_size=10):\n        self.pool = create_pool(dsn, password, pool_size)', methods: [['query', '(sql, *p)'], ['execute', '(sql, *p)'], ['transaction', '()']], initArgs: [['dsn', 'postgres://…/forgify', false], ['password', null, true], ['pool_size', '10', false]] },
    research_agent: { kind: 'agent', version: 2, status: 'idle', runs: 38, life: 'active', desc: '深度调研代理：检索、交叉验证、带引用综述，写回记忆。', model: 'claude-opus-4-8', maxSteps: 40,
      system: '你是一名严谨的研究员。对每个论断都要交叉验证至少两个独立来源，输出带引用编号的综述。\n不确定时显式标注「待证实」，绝不杜撰来源。优先用 web_search 找一手资料，再用 fetch_url 取全文。',
      tools: [{ label: 'web_search', health: 'ok' }, { label: 'fetch_url', health: 'ok' }, { label: 'read_document', health: 'ok' }, { label: 'write_memory', health: 'ok' }, { label: 'cite', health: 'bad' }], skill: 'deep_research', knowledge: ['竞品库', '行业报告 2026'],
      rel: [{ title: 'Equips', rows: [{ kind: 'skill', name: 'deep_research', meta: 'skill' }, { kind: 'function', name: 'cite', meta: 'tool · ⚠' }] }, { title: 'Used by', rows: [{ kind: 'workflow', name: 'nightly_report', meta: 'summarize' }] }] },
    summarizer: { kind: 'agent', version: 4, status: 'idle', runs: 211, life: 'active', model: 'claude-sonnet-4-6', maxSteps: 12, desc: '把长文 / 会话 / 运行日志压成结构化摘要（要点 + 风险 + 下一步）。', system: '把输入压成三段：① 3–5 条要点 ② 风险或反对意见 ③ 建议的下一步。\n保留关键数字与专有名词，删冗余修饰。输出 markdown。', tools: [{ label: 'read_document', health: 'ok' }, { label: 'write_memory', health: 'ok' }], skill: null, knowledge: [] },
    nightly_report: { kind: 'workflow', version: 8, status: 'run', runs: 173, life: 'active', concurrency: 'serial', triggers: ['cron_2am'], running: 'summarize', desc: '每晚汇总仓库与议题动态，交研究代理综述，按规模路由后推送简报、必要时请负责人审批。',
      nodes: [{ id: 'cron', kind: 'trigger', ref: 'cron_2am', x: 22, y: 150 }, { id: 'fetch_repos', kind: 'action', ref: 'fetch_news', x: 210, y: 56 }, { id: 'fetch_issues', kind: 'action', ref: 'parse_pdf', x: 210, y: 244 }, { id: 'summarize', kind: 'agent', ref: 'research_agent', x: 398, y: 150 }, { id: 'route', kind: 'control', ref: 'route_by_amount', x: 586, y: 150 }, { id: 'publish', kind: 'action', ref: 'slack_handler', x: 398, y: 286 }, { id: 'notify', kind: 'approval', ref: 'manager_approval', x: 586, y: 286 }],
      edges: [['cron', 'fetch_repos'], ['cron', 'fetch_issues'], ['fetch_repos', 'summarize'], ['fetch_issues', 'summarize'], ['summarize', 'route'], ['route', 'publish'], ['route', 'notify']],
      rel: [{ title: 'Equips', rows: [{ kind: 'agent', name: 'research_agent', meta: 'summarize' }, { kind: 'handler', name: 'slack_handler', meta: 'publish' }, { kind: 'trigger', name: 'cron_2am', meta: 'bound' }] }] },
    invoice_flow: { kind: 'workflow', version: 3, status: 'wait', runs: 64, life: 'active', concurrency: 'serial', triggers: ['webhook_pr'], attention: '节点 <b>approve</b> 等待 manager_approval 审批（已等 3 小时）。', nodes: [{ id: 'in', kind: 'trigger', ref: 'webhook_pr', x: 28, y: 130 }, { id: 'extract', kind: 'action', ref: 'process_invoice', x: 240, y: 130 }, { id: 'approve', kind: 'approval', ref: 'manager_approval', x: 452, y: 130, parked: true }, { id: 'post', kind: 'action', ref: 'db_pool', x: 600, y: 130 }], edges: [['in', 'extract'], ['extract', 'approve'], ['approve', 'post']] },
    archive_cleanup: { kind: 'workflow', version: 1, status: 'idle', runs: 12, life: 'inactive', concurrency: 'serial', triggers: [], desc: '定期扫描过期对象并归档清理。', nodes: [{ id: 'cron', kind: 'trigger', ref: 'cron_weekly', x: 40, y: 130 }, { id: 'scan', kind: 'action', ref: 'list_stale', x: 280, y: 130 }, { id: 'purge', kind: 'action', ref: 'archive', x: 520, y: 130 }], edges: [['cron', 'scan'], ['scan', 'purge']] },
    cron_2am: { kind: 'trigger', status: 'listening', desc: '每天 02:00（本地时区）触发，启动 nightly_report。', cfg: [['源类型', 'cron'], ['表达式', '0 2 * * *'], ['时区', 'Asia/Shanghai'], ['绑定工作流', 'nightly_report'], ['上次 fire', '今天 02:00 · 7 节点全绿']], outputs: ['firedAt', 'tz'] },
    webhook_pr: { kind: 'trigger', status: 'idle', desc: 'GitHub PR webhook：opened / synchronize 时触发。', cfg: [['源类型', 'webhook'], ['路径', '/hooks/pr'], ['密钥', '••••', true], ['算法', 'hmac-sha256'], ['绑定工作流', 'invoice_flow'], ['上次 fire', '从未']], outputs: ['event', 'payload'] },
    route_by_amount: { kind: 'control', version: 2, status: 'idle', desc: 'CEL 路由：按金额分流到审批或自动入账。', branches: [['amount > 1000', 'approve'], ['amount <= 1000', 'auto_post']], inputs: [['amount', 'number', true]] },
    manager_approval: { kind: 'approval', version: 4, status: 'idle', desc: '人在环审批闸：金额超阈需经理放行。', template: '## 发票待审批\n\n供应商：{{input.vendor}}\n金额：**{{input.total}}** {{input.currency}}\n\n> 超过 ¥1000 阈值，需经理放行。', rules: [['allowReason', 'true'], ['timeout', '24h'], ['timeoutBehavior', 'reject']], inputs: [['vendor', 'string', true], ['total', 'number', true], ['currency', 'string', true]] },
    github_mcp: { kind: 'mcp', status: 'done', desc: 'GitHub MCP 连接器：仓库 / PR / issue 能力作为工具暴露给代理。', conn: 'ready', calls: 318, fails: 0, cfg: [['传输', 'stdio'], ['命令', 'npx -y @gh/mcp'], ['鉴权', 'PAT ••••', true]], tools: ['list_repos', 'get_pr', 'create_issue', 'merge_pr', 'search_code'] },
    linear_mcp: { kind: 'mcp', status: 'wait', desc: 'Linear MCP 连接器：任务 / 周期能力。需重新授权。', conn: 'failed', calls: 42, fails: 5, cfg: [['传输', 'http'], ['URL', 'https://mcp.linear.app'], ['鉴权', 'OAuth（已过期）']], tools: ['list_issues', 'create_issue', 'update_issue'] },
    deep_research: { kind: 'skill', desc: '深度调研 playbook：指导代理分解问题、检索、交叉验证、引用。', path: '.forgify/skills/deep_research.md', body: '# Deep research\n\n参数：$1 = 主题，${CLAUDE_SESSION_ID} = 会话\n\n1. 把问题拆成 3–5 个可独立检索的子问题。\n2. 每个子问题先 web_search 找一手来源，再 fetch_url 取全文。\n3. 交叉验证：同一论断至少两个独立来源，冲突则并列。\n4. 输出带引用编号 [n] 的综述，附来源清单。', allowed: ['web_search', 'fetch_url', 'cite'], frontmatter: [['name', 'deep_research'], ['context', 'inline'], ['arguments', '$1=主题']] },
    pdf_extract: { kind: 'skill', desc: '从 PDF 抽取并清洗文本的 playbook。', path: '.forgify/skills/pdf_extract.md', body: '# PDF extract\n\n1. 调 parse_pdf 取文本块。\n2. 合并跨页断句、去页眉页脚。\n3. 按标题层级重组为结构化大纲。', allowed: ['parse_pdf'], frontmatter: [['name', 'pdf_extract'], ['context', 'inline']] },
  };

  // ===================== 各类型概览（documents 风：标签 + 内容,无盒）=====================
  const ioRows = arr => arr.map(f => [`<span class="nm">${esc(f[0])}</span>${f[2] ? '<span class="req">*</span>' : ''}`, `<span class="ty">${esc(f[1])}</span>`]);
  const OVER = {
    function(b, a) {
      prose(b, { value: a.desc });
      codeEditor(sec(b, 'Code'), { code: a.code, corner: a.lang });
      runDebug(sec(b, '调试运行'), a.env === 'ready' ? { argsSeed: '{\n  "currency": "USD"\n}', verb: 'Run', trace: { lines: ['→ spawn sandbox (python 3.12)', '→ exec process_invoice()', 'stdout: parsed 14 line items', 'stdout: validated ✓'], result: { st: 'ok', out: '{ "vendor": "Acme Inc", "total": 1284.50, "tax_id": "US-99-1" }', ms: 412 } } } : { gate: '环境' + ENV[a.env][1] + ' — 就绪后可运行' });
      const g = el('div', 'eo-2col'); b.appendChild(g);
      table(sec(g, 'Inputs', a.inputs.length, () => { }), ['参数', '类型'], ioRows(a.inputs));
      table(sec(g, 'Output'), ['返回', '字段'], a.output.map(o => [`<span class="nm">${esc(o[0])}</span>`, `<span class="ty">${esc(o[1])}</span>`]));
      tags(sec(b, 'Dependencies', a.deps.length, () => { }), { items: a.deps });
      const env = sec(b, 'Environment'); const r = el('div'); r.style.cssText = 'display:flex;align-items:center;gap:12px'; r.innerHTML = `${badge(ENV, a.env)}<span class="eo-note">上次：${esc(a.lastRun || '—')}</span><button class="eo-mini" style="margin-left:auto">${icon('spin', 13)}Rebuild env</button>`; env.appendChild(r);
    },
    handler(b, a) {
      prose(b, { value: a.desc });
      defs(sec(b, 'Runtime'), [['Runtime', '', { html: badge({ active: ['ready', '运行中'], inactive: ['pending', '未上线'] }, a.life === 'active' ? 'active' : 'inactive') }], ['Config', '', { html: badge(CFG, a.cfg) }], ['Env', '', { html: badge(ENV, a.env) }]]);
      codeEditor(sec(b, 'Assembled class'), { code: a.code, corner: a.lang });
      table(sec(b, 'Methods', a.methods.length, () => { }), ['方法', '签名', ''], a.methods.map(m => [`<span class="nm">${esc(m[0])}</span>`, `<span class="ty">${esc(m[1])}</span>`, `<span class="act">Call ›</span>`]));
      const t = table(sec(b, 'Init args'), ['参数', '值'], a.initArgs.map(([k, v, s]) => [`<span class="nm">${esc(k)}</span>`, s ? `<span class="v mask" contenteditable="true">••••••••</span>` : `<span class="v" contenteditable="true">${esc(v || '')}</span>`]));
      t.insertAdjacentHTML('afterend', '<div class="eo-note" style="margin-top:8px">改 config 触发重启</div>');
    },
    agent(b, a) {
      if (a.tools.some(t => t.health === 'bad')) { const w = el('div', 'eo-attn'); w.innerHTML = `<span class="ico">${icon('shield', 16)}</span><span>挂载工具 <b style="font-family:var(--mono)">cite</b> 不可解析，invoke 时将跳过。</span>`; b.appendChild(w); }
      prose(b, { value: a.desc });
      prose(sec(b, 'System prompt'), { value: a.system, cls: 'eo-block' });
      tags(sec(b, 'Mounted tools', a.tools.length, () => { }), { items: a.tools, icon: 'code' });
      const g = el('div', 'eo-2col'); b.appendChild(g);
      defs(sec(g, 'Model'), [['model', a.model, { edit: true }], ['maxSteps', a.maxSteps, { edit: true }]]);
      tags(sec(g, 'Skill · 0–1', null, () => { }), { items: a.skill ? [a.skill] : [], icon: 'skill', kind: 'single', addLabel: '挂技能' });
      tags(sec(b, 'Knowledge', a.knowledge.length, () => { }), { items: a.knowledge });
      runDebug(sec(b, '调试调用'), { argsSeed: '{\n  "query": "竞品近况",\n  "scope": "2026"\n}', verb: 'Invoke', trace: { lines: ['mount-health: 4/5 ok（cite 跳过）', '⟐ reasoning…', '→ tool web_search("竞品近况 2026")', '→ tool fetch_url(...)', '⟐ 综述生成中…'], result: { st: 'ok', out: 'stopReason=end · 6 steps · 1.2k→3.4k tok', ms: 8800 } } });
    },
    workflow(b, a) {
      if (a.attention) { const w = el('div', 'eo-attn'); w.innerHTML = `<span class="ico">${icon('shield', 16)}</span><span>${a.attention}</span>`; b.appendChild(w); }
      if (a.desc) prose(b, { value: a.desc });
      graphEditor(sec(b, 'Graph', a.nodes.length + ' 节点'), a);
      const g = el('div', 'eo-2col'); b.appendChild(g);
      defs(sec(g, 'Run'), [['lifecycle', '', { html: `<span class="eo-st ${a.life === 'active' ? 'run' : 'idle'}"><span class="dot"></span>${a.life === 'active' ? '已激活' : '未上线'}</span>` }], ['concurrency', a.concurrency, { edit: true }]]);
      tags(sec(g, 'Triggers', a.triggers.length, () => { }), { items: a.triggers, icon: 'trigger' });
    },
    trigger(b, a) {
      prose(b, { value: a.desc });
      defs(sec(b, 'Signal source'), a.cfg.map(([k, v, mask]) => [k, '', { html: mask ? `<span class="v mask">${esc(v)}</span>` : esc(v), edit: !mask }]));
      tags(sec(b, 'Outputs', a.outputs.length, () => { }), { items: a.outputs });
      runDebug(sec(b, '调试 · Fire now'), { verb: 'Fire', vico: 'zap', trace: { lines: ['fanned to 1 workflow', 'trf_8a → nightly_report → started', 'flowrun fr_4c2 created'], result: { st: 'ok', out: 'fired · 1 firing · 0 skipped', ms: 42 } } });
    },
    control(b, a) {
      prose(b, { value: a.desc });
      const br = sec(b, 'Branches · 首个为真胜出', a.branches.length, () => { });
      a.branches.forEach((x, i) => { const r = el('div', 'eo-branch'); r.innerHTML = `<span class="bn">${i + 1}</span><span class="bwhen">${hl(x[0])}</span><span class="bport">→ ${esc(x[1])}</span>`; br.appendChild(r); });
      const ca = el('div', 'eo-branch catchall'); ca.innerHTML = `<span class="bn">·</span><span class="bwhen">true</span><span class="bport">→ default</span>`; br.appendChild(ca);
      table(sec(b, 'Inputs (CEL namespace)', a.inputs.length), ['参数', '类型'], ioRows(a.inputs));
      runDebug(sec(b, '调试 · Probe（内联求值,不落运行）'), { argsSeed: '{ "amount": 1500 }', verb: 'Evaluate', trace: { lines: ['branch 1: amount > 1000 → ✓ 命中', 'branch 2: 不再求值'], result: { st: 'ok', out: '→ port "approve"', ms: 3 } } });
    },
    approval(b, a) {
      prose(b, { value: a.desc });
      codeEditor(sec(b, 'Template · {{input.*}} 插值'), { code: a.template, corner: 'jinja+cel' });
      const g = el('div', 'eo-2col'); b.appendChild(g);
      defs(sec(g, 'Decision rules'), a.rules.map(([k, v]) => [k, v, { edit: true }]));
      table(sec(g, 'Input schema', a.inputs.length), ['字段', '类型'], ioRows(a.inputs));
      runDebug(sec(b, '调试 · Render & decide'), { argsSeed: '{ "vendor":"Acme", "total":1284.5, "currency":"USD" }', verb: 'Render', trace: { lines: ['解析 {{input.*}} → 渲染 markdown', 'emit parked · 待决策 · 24h 后 reject'], result: { st: 'ok', out: 'parked → 等待人工 通过/驳回', ms: 6 } } });
    },
    mcp(b, a) {
      prose(b, { value: a.desc });
      defs(sec(b, 'Connection'), [['status', '', { html: badge(CONN, a.conn) }], ...a.cfg.map(([k, v, mask]) => [k, '', { html: mask ? `<span class="v mask">${esc(v)}</span>` : esc(v), edit: !mask }]), ['calls / fails', `${a.calls} / ${a.fails}`]]);
      table(sec(b, 'Exposed tools', a.tools.length), ['工具', ''], a.tools.map(t => [`<span class="nm">${esc(t)}</span>`, `<span class="act">Invoke ›</span>`]));
    },
    skill(b, a) {
      prose(b, { value: a.desc });
      codeEditor(sec(b, 'Playbook · $n / ${...} 插值'), { code: a.body, corner: 'markdown' });
      const g = el('div', 'eo-2col'); b.appendChild(g);
      defs(sec(g, 'Frontmatter'), a.frontmatter.map(([k, v]) => [k, v, { edit: true }]));
      tags(sec(g, 'Allowed tools', a.allowed.length, () => { }), { items: a.allowed, icon: 'code' });
      runDebug(sec(b, '调试 · Render（技能是注入、非执行）'), { argsSeed: '$1 = "竞品调研"', verb: 'Render', trace: { lines: ['替换 $1 → "竞品调研"', '替换 ${CLAUDE_SESSION_ID}', '注入 agent system-prompt 的 ## Execution guide'], result: { st: 'ok', out: '已展开 · 注入 312 字', ms: 2 } } });
    },
  };

  function tabVersions(b, a) {
    if (a.kind === 'function') return versionDiff(b, { versions: a.versions, field: v => v.code, cap: 'Function 版本 diff · 非 git' });
    const cur = a.code || a.system || a.template || a.body || (a.branches ? a.branches.map(x => x.join(' → ')).join('\n') : JSON.stringify(a.cfg || {}, null, 2));
    const verLess = ['trigger', 'mcp', 'skill'].includes(a.kind);
    const cap = verLess ? (a.kind === 'skill' ? '保存文件 diff · 无版本' : (a.kind === 'mcp' ? '连接配置 diff · 无版本 · 外部 server' : '配置编辑 · 无版本 · Trigger 不入版本')) : KIND[a.kind].label + ' 版本 diff · 非 git';
    return versionDiff(b, { versions: [{ n: a.version || 1, t: '当前', reason: '当前', active: true, _: cur }, { n: (a.version || 2) - 1, t: '更早', reason: '上一次保存', _: cur.split('\n').slice(0, -1).join('\n') }], field: v => v._, cap });
  }
  function tabRuns(b, a) {
    const ex = a.execs || [['ok', 'manual', '120ms', '刚刚'], ['ok', 'workflow', '98ms', '今天'], ['failed', 'manual', '—', '昨天']];
    const ok = ex.filter(e => e[0] === 'ok').length;
    const agg = el('div', 'eo-agg'); agg.innerHTML = `<span><b>${ex.length}</b> 次</span><span><b style="color:var(--ok)">${ok}</b> 成功</span><span><b style="color:var(--danger)">${ex.length - ok}</b> 失败</span>`; b.appendChild(agg);
    const led = el('div', 'eo-led'); b.appendChild(led);
    ex.forEach((e, i) => { const r = el('div', 'eo-lrow'); r.innerHTML = `<span class="cst ${e[0]}"></span><span class="cid">${a.kind}e_${(i + 7).toString(16)}a${i}</span><span class="ctrig">${e[1]}</span><span class="cmeta">${e[3]}</span><span class="cdur">${e[2]}</span>`; led.appendChild(r); });
    const note = el('div', 'eo-sched'); note.innerHTML = `${icon('scheduler', 14)}深度运行历史在 <span class="lnk" data-s>Scheduler 海</span>`; b.appendChild(note);
    note.querySelector('[data-s]').onclick = () => Shell.toOcean && Shell.toOcean('scheduler');
  }

  function tabsFor(a) {
    const t = [{ key: 'o', label: '概览', render: b => OVER[a.kind](b, a) }];
    const verLess = ['trigger', 'mcp', 'skill'].includes(a.kind);
    if (a.kind === 'mcp') { /* mcp 无版本 tab */ }
    else t.push({ key: 'v', label: verLess ? (a.kind === 'skill' ? '历史' : '编辑历史') : '版本', render: b => tabVersions(b, a) });
    if (!['control', 'approval', 'skill'].includes(a.kind)) t.push({ key: 'r', label: a.kind === 'handler' ? '调用' : '运行', render: b => tabRuns(b, a) });
    t.push({ key: 'rel', label: '关系', render: b => relations(b, a.rel || [{ title: 'Referenced by', rows: [] }]) });
    t.push({ key: 'i', label: '迭代', render: b => { b.innerHTML = `<div class="eo-none">迭代 = 在对话里让 AI 改这个实体（:iterate → conversationId）。形态见 Chat 海洋右岛实体卡流式编辑。</div>`; } });
    return t;
  }

  // ===================== 渲染（文档头 + headExtra 操作 + tab）=====================
  function detail(stage, id) {
    const a = D[id]; if (!a) return empty(stage);
    asideHide();
    const k = KIND[a.kind] || KIND.function;
    const meta = [];
    if (a.version != null) meta.push(`v${a.version}`);
    meta.push(`<span class="eo-st ${a.status || 'idle'}"><span class="dot"></span>${ST[a.status] || '闲置'}</span>`);
    if (a.life) meta.push(`<span class="eo-life${a.life === 'active' ? ' active' : ''}">${a.life === 'active' ? '已激活' : '未上线'}</span>`);
    if (a.runs != null) meta.push(`${a.runs} runs`);
    meta.push(`<span class="mono">${fauxId(a.kind, id)}</span>`);
    if (a.path) meta.push(`<span class="mono">${esc(a.path)}</span>`);
    const tabs = tabsFor(a);
    stage.innerHTML = `<div class="eo-doc eo-morph">
      <div class="eo-path"><span class="ico">${icon(k.icon, 13)}</span><span>${k.label}</span><span class="sep">/</span><span>${esc(a.name || id)}</span></div>
      <div class="eo-title" contenteditable="true" spellcheck="false">${esc(a.name || id)}</div>
      <div class="eo-meta">${meta.join('<span class="sep">·</span>')}</div>
      <div class="eo-tabs">${tabs.map((t, i) => `<span class="eo-tab${i === 0 ? ' on' : ''}" data-i="${i}">${esc(t.label)}</span>`).join('')}</div>
      <div class="eo-tab-body"></div>
    </div>`;
    dirtyEl = $('.eo'); if (dirtyEl) dirtyEl.classList.remove('dirty');   // dirty 态挂 .eo 包裹
    // headExtra 操作（顶栏右上，对齐 documents）
    Shell.headExtra(`<span class="eo-saved"><span class="ico">${icon('check', 13)}</span>已保存</span>${k.verb ? `<button class="eo-run" data-run>${icon(k.vico, 13)}${k.verb}</button>` : ''}<button class="ibtn" data-iter title="迭代（AI 改）">${icon('spark', 16)}</button><button class="ibtn" data-more title="更多">${icon('more', 16)}</button>`);
    savedEl = $('#head-extra .eo-saved');
    stage.querySelector('.eo-title').addEventListener('input', markDirty);
    const body = stage.querySelector('.eo-tab-body');
    const render = i => { body.innerHTML = ''; tabs[i].render(body, a); };
    stage.querySelectorAll('.eo-tab').forEach((btn, i) => btn.onclick = () => { stage.querySelectorAll('.eo-tab').forEach(x => x.classList.remove('on')); btn.classList.add('on'); render(i); });
    render(0);
    const sc = $('#eoScroll'); if (sc) sc.scrollTop = 0;
  }

  function empty(stage) {
    asideHide(); if (Shell.headExtra) Shell.headExtra('');
    const cnt = k => Object.values(D).filter(e => e.kind === k).length;
    const stat = (k, l) => `<span class="eo-stat"><b>${cnt(k)}</b><span>${l}</span></span>`;
    stage.innerHTML = `<div class="eo-empty"><div class="in eo-morph"><div class="gico">${icon('entities', 24)}</div><h2>四项全能实体</h2><p>Function · Handler · Agent · Workflow——及组成图的触发器、控制、审批、连接器与技能。<br>从左侧选一个：看全貌、抓必要信息、就地调试与修改。</p><div class="eo-stats">${stat('function', 'Functions')}${stat('handler', 'Handlers')}${stat('agent', 'Agents')}${stat('workflow', 'Workflows')}</div></div></div>`;
  }

  Shell.registerOcean('entities', {
    crumb: '实体',
    build(sea) {
      sea.innerHTML = `<div class="eo"><div class="eo-scroll scroll-fade" id="eoScroll"><div id="eoStage"></div></div></div>`;
      const stage = $('#eoStage', sea);
      Shell.openEntity = id => detail(stage, id);
      detail(stage, 'process_invoice');
    },
  });
})();

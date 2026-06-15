/* Forgify demo — 组件 entity-card（九类全能右岛卡，单一事实源；合并 design-lab chat/entity-card 的流式锻造卡
   与 entities 的产品级九类字段集为一套外壳）。
   契约：组件 = 工厂 → handle；自载同名 .css；只读令牌 + 配置(ENTITY_KINDS/STATE_MODEL)；fg- 前缀；
        正文经【公开工厂】组合——RightIsland(外壳) · Tabs(四页签) · CodeEditor(code/class/system) · Tags(tools/deps/knowledge)
        · KV(init-args/meta) · StatusDot(env/cfg/conn 徽) · RunGraph(workflow 图) · RefPill(实体提及) · VersionDiff(版本 tab，缺则内联兜底)。
   为何流式 reveal/fill 不交给上层：锻造工具(create_/edit_*)的流式 args 经 entities 流镜像→逐字段填充是卡的固有职责，
        故 data-f 锚点【冻结】(function 的 code/inputs/deps/env、workflow 的 graph 等)——上层按 data-f 注入，不知内部结构。
   一套外壳 / 九种类型：只换 kind 图标 + 头部副行 + 四 tab 内容 + per-kind body；状态徽走统一 StatusDot.setSt。
   API：EntityCard.mount(host, entity) → {el, setLive, setSt, reveal, fill, show, hide, toggle, destroy}
     entity = {kind,name,version,live, ...kindFields}（kindFields 见 BODY 各分支；data-f key 冻结）。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  const esc = s => String(s == null ? '' : s).replace(/[&<>]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]));
  const ICO = window.icon || ((k, n) => '');
  const K = window.ENTITY_KINDS || {};
  const kindMeta = kind => K[kind] || K.function || { label: 'Entity', icon: 'function' };

  // CEL / wikilink 内联高亮（模板/文档里的插值；与 design-lab celHi/wikiHi 同源）
  const celHi = t => esc(t || '').replace(/\{\{([^}]*)\}\}/g, '<span class="fg-ec-cel">{{$1}}</span>');

  // ===== 头部副行（版本 vs 源 vs 路径，按 kind 分支；ver 锚点 data-ec-ver 供 setVersion）=====
  function headSub(a) {
    const k = kindMeta(a.kind);
    if (a.kind === 'mcp') return `${k.label} · <span class="fg-ec-src-tag">${esc(a.source || 'manual')}</span>`;
    if (a.kind === 'trigger') return `${k.label} · <span class="fg-ec-src-tag">${esc(a.source || 'webhook')} · ${a.listening ? '监听中' : '未上线'}</span>`;
    if (a.kind === 'skill') return `${k.label} · <span class="fg-ec-path">${esc(a.path || '/')}</span>`;
    let s = `${k.label} · <span class="fg-ec-ver" data-ec-ver>v${esc(a.version || 1)}</span>`;
    if (a.kind === 'handler') s += ' · 常驻进程';
    else if (a.kind === 'control') s += ' · 内联求值';
    else if (a.kind === 'approval') s += ' · 人在环闸';
    return s;
  }

  // live 徽（锻造中/编辑中/已保存）；脉冲点纯 CSS
  const liveBadge = v =>
    (v === true || v === 'forge') ? `<span class="fg-ec-live"><span class="fg-ec-pulse"></span>锻造中</span>`
    : v === 'edit' ? `<span class="fg-ec-live"><span class="fg-ec-pulse"></span>编辑中</span>`
    : '';

  // 带状态徽的字段：StatusDot.badge 渲点+文案；data-f 锚点 + data-ec-st 供 setSt 原位推进
  function stField(dataF, label, mapKey, state, note) {
    const badge = window.StatusDot ? window.StatusDot.badge(mapKey, state) : esc(state);
    return `<div class="fg-ec-field" data-f="${dataF}"><label>${esc(label)}</label>`
      + `<div class="fg-ec-stwrap"><span data-ec-st data-ec-map="${mapKey}">${badge}</span>${note ? `<span class="fg-ec-note">${esc(note)}</span>` : ''}</div></div>`;
  }

  // 字段壳（label + 可选计数 ct + body 槽，挂 data-f 锚点）；mode=val 给 .val 内嵌底盒，mode=raw 给裸槽
  function field(dataF, label, ct, mode) {
    const ctHtml = ct != null ? ` <span class="fg-ec-ct">${esc(ct)}</span>` : '';
    const body = mode === 'val' ? `<div class="fg-ec-val" data-ec-slot></div>` : `<div class="fg-ec-slot" data-ec-slot></div>`;
    return window.tag('div.fg-ec-field', { 'data-f': dataF }, `<label>${esc(label)}${ctHtml}</label>${body}`);
  }
  const slotOf = fieldEl => fieldEl.querySelector('[data-ec-slot]');

  // 行（图标 + id + 可选 health 点 + 右对齐 ref）：节点/工具/监听共用；ref 可点走 RefPill→Intent
  function entityRow(host, { ico, id, health, ref, refKind }) {
    const mh = health ? `<span class="fg-ec-mh ${health === 'bad' ? 'bad' : 'ok'}"></span>` : '';
    const refHtml = ref ? (window.RefPill ? window.RefPill.html(refKind || 'function', ref, refKind ? ref : '') : `<span class="fg-ec-ref">${esc(ref)}</span>`) : '';
    const r = window.tag('div.fg-ec-row', `<span class="fg-ec-rico">${ICO(ico, 16)}</span>${mh}<span class="fg-ec-rid">${esc(id)}</span><span class="fg-ec-rspacer"></span>${refHtml}`);
    host.appendChild(r);
    return r;
  }

  // ===== 概览正文（per-kind 字段集；data-f 冻结）。组件公开工厂填内嵌富件，富件不直接拼 HTML。 =====
  const NODE_KIND = { trigger: 'trigger', action: 'action', agent: 'agent', control: 'control', approval: 'approval' };

  function buildBody(host, a) {
    const add = node => host.appendChild(node);
    if (a.desc) add(window.tag('div.fg-ec-desc', esc(a.desc)));

    if (a.kind === 'function') {
      const code = field('code', 'Code', `python ${a.python || '3.12'}`);
      add(code); ceMount(slotOf(code), a.code, a.lang || `python ${a.python || '3.12'}`);
      const inputs = field('inputs', 'Inputs', null, 'val'); add(inputs);
      tagsMount(slotOf(inputs), (a.inputs || []).map(i => Array.isArray(i) ? i[0] : i));
      const deps = field('deps', 'Dependencies', (a.deps || []).length, 'val'); add(deps);
      tagsMount(slotOf(deps), a.deps || []);
      add(htmlField(stField('env', 'Env status', 'ENV', a.env || 'pending')));
      return;
    }
    if (a.kind === 'handler') {
      add(htmlField(stField('runtime', 'Runtime', 'CFG', a.runtime || 'unconfigured')));
      add(htmlField(stField('config', 'Config state', 'CFG', a.configState || 'unconfigured', '改 config 触发重启')));
      const init = field('init', 'Init args', null, 'val'); add(init);
      kvMount(slotOf(init), (a.initArgs || []).map(x => [x.name + (x.required ? ' *' : ''), x.value || x.default || '—', { mask: x.sensitive, mono: true }]));
      const methods = field('methods', 'Methods', 'catalog members', 'val'); add(methods);
      tagsMount(slotOf(methods), (a.methods || []).map(m => m + '()'));
      const cls = field('class', 'Assembled class'); add(cls); ceMount(slotOf(cls), a.classCode, a.lang || 'python');
      add(htmlField(stField('env', 'Env status', 'ENV', a.env || 'pending')));
      return;
    }
    if (a.kind === 'agent') {
      const sys = field('system', 'System prompt'); add(sys); ceMount(slotOf(sys), a.system, 'prompt');
      const model = field('model', 'Model', null, 'val'); add(model);
      kvMount(slotOf(model), [['model', a.model || '—', { mono: true }], ['maxSteps', a.maxSteps != null ? a.maxSteps : '—', { mono: true }]]);
      const tools = field('tools', 'Mounted tools', (a.tools || []).length); add(tools);
      const trows = window.tag('div.fg-ec-rows'); slotOf(tools).appendChild(trows);
      (a.tools || []).forEach(t => entityRow(trows, { ico: 'code', id: (t.ref || t.label || t), health: t.health, ref: t.ref, refKind: 'function' }));
      const skill = field('skill', 'Skill', '0–1', 'val'); add(skill);
      tagsMount(slotOf(skill), a.skill ? [a.skill] : [], 'skill', 'single');
      const know = field('knowledge', 'Knowledge', null, 'val'); add(know);
      tagsMount(slotOf(know), a.knowledge || []);
      return;
    }
    if (a.kind === 'workflow') {
      const graph = field('graph', 'Graph', `${(a.nodes || []).length} 节点`); add(graph);
      graphMount(slotOf(graph), a);
      add(htmlField(lifecycleField(a.lifecycle || 'inactive')));
      const conc = field('concurrency', 'Concurrency', '5 值', 'val'); add(conc);
      kvMount(slotOf(conc), [['concurrency', a.concurrency || 'serial', { mono: true }]]);
      if (a.attention) add(htmlField(`<div class="fg-ec-field" data-f="attn"><label>Attention</label><div class="fg-ec-attn">${celHi(a.attention)}</div></div>`));
      return;
    }
    if (a.kind === 'control') {
      const inputs = field('inputs', 'Inputs', null, 'val'); add(inputs);
      tagsMount(slotOf(inputs), (a.inputs || []).map(i => Array.isArray(i) ? i[0] : i));
      const br = field('branches', 'Branches', 'first-true-wins'); add(br);
      const brows = window.tag('div.fg-ec-rows'); slotOf(br).appendChild(brows);
      (a.branches || []).forEach((b, i) => branchRow(brows, b, i));
      branchRow(brows, { catchall: true, port: 'default' }, (a.branches || []).length);
      add(htmlField(stField('validation', 'Validation', 'CFG', a.validation || 'ready')));
      return;
    }
    if (a.kind === 'approval') {
      const tpl = field('template', 'Template', 'markdown + {{CEL}}'); add(tpl);
      slotOf(tpl).innerHTML = `<div class="fg-ec-tpl">${celHi(a.template)}</div>`;
      const rules = field('rules', 'Decision rules', null, 'val'); add(rules);
      kvMount(slotOf(rules), [['allowReason', a.allowReason ? '是' : '否'], ['timeout', a.timeout || '永不超时'], ['behavior', a.behavior || 'reject']]);
      add(htmlField(stField('validity', 'Form validity', 'CFG', a.validity || 'ready')));
      return;
    }
    if (a.kind === 'trigger') {
      const src = field('source', 'Source', null, 'val'); add(src);
      slotOf(src).innerHTML = `<span class="fg-ec-src-tag"><span class="fg-ec-srcico">${ICO('trigger', 13)}</span>${esc(a.source || 'webhook')}</span>`;
      const cfg = field('config', 'Config', null, 'val'); add(cfg);
      kvMount(slotOf(cfg), (a.config || []).map(c => [c[0], c[1], { mask: c[2], mono: true }]));
      const dedup = field('dedup', 'Dedup key', null, 'val'); add(dedup);
      kvMount(slotOf(dedup), [['key', a.dedup || '—', { mono: true }]]);
      const lst = field('listeners', 'Listeners', `refCount ${(a.listeners || []).length}`); add(lst);
      const lrows = window.tag('div.fg-ec-rows'); slotOf(lst).appendChild(lrows);
      if ((a.listeners || []).length) (a.listeners || []).forEach(l => entityRow(lrows, { ico: 'workflow', id: l, ref: l, refKind: 'workflow' }));
      else lrows.appendChild(window.tag('div.fg-ec-mask', '未上线 · 无监听'));
      return;
    }
    if (a.kind === 'mcp') {
      add(htmlField(stField('conn', 'Connection', 'CONN', a.conn || 'pending')));
      const tr = field('transport', 'Transport', a.transport || 'stdio', 'val'); add(tr);
      kvMount(slotOf(tr), (a.transportCfg || []).map(c => [c[0], c[1], { mask: c[2], mono: true }]));
      const sec = field('secrets', 'Secrets', null, 'val'); add(sec);
      kvMount(slotOf(sec), (a.secrets || []).map(s => [s, '', { mask: true, mono: true }]));
      const tools = field('tools', 'Tools', (a.tools || []).length, 'val'); add(tools);
      tagsMount(slotOf(tools), a.tools || [], null, 'multi', '连接后发现');
      return;
    }
    if (a.kind === 'skill') {
      const pb = field('body', 'Playbook', '$n / ${...} 插值'); add(pb); ceMount(slotOf(pb), a.body, 'markdown');
      const fm = field('frontmatter', 'Frontmatter', null, 'val'); add(fm);
      kvMount(slotOf(fm), (a.frontmatter || []).map(([k, v]) => [k, v, { mono: true }]));
      const allow = field('allowed', 'Allowed tools', (a.allowed || []).length, 'val'); add(allow);
      tagsMount(slotOf(allow), a.allowed || [], 'code');
    }
  }

  // 直接注入 HTML 的字段（状态徽/生命周期/attention 这类无富件子组件的）
  function htmlField(html) { const w = window.tag('div'); w.innerHTML = html; return w.firstElementChild; }

  // 生命周期徽（workflow；active 蓝脉冲 / draining 橙 / inactive 灰），data-ec-life 锚点供 setLifecycle
  function lifecycleField(life) {
    const txt = { active: '运行中', draining: '收尾中', inactive: '未上线' }[life] || '未上线';
    return `<div class="fg-ec-field" data-f="lifecycle"><label>Lifecycle</label>`
      + `<div class="fg-ec-stwrap"><span class="fg-ec-life ${esc(life)}" data-ec-life>${life === 'active' ? '<span class="fg-ec-life-dot"></span>' : ''}${esc(txt)}</span></div></div>`;
  }

  // control 分支行（first-true-wins；catchall 末条兜底；坏分支 danger 描边）
  function branchRow(host, b, i) {
    const cls = `fg-ec-branch${b.bad ? ' bad' : ''}${b.catchall ? ' catchall' : ''}`;
    const when = b.catchall ? 'true（兜底）' : celHi(Array.isArray(b) ? b[0] : b.when);
    const port = esc(b.catchall ? (b.port || 'default') : (Array.isArray(b) ? b[1] : b.port));
    host.appendChild(window.tag(`div.${cls.split(' ').join('.')}`,
      `<span class="fg-ec-bnum">${b.catchall ? '·' : i + 1}</span><div class="fg-ec-bbody"><div class="fg-ec-bwhen">${when}</div></div><span class="fg-ec-bport">→ ${port}</span>`));
  }

  // ===== 子组件挂载（全经公开工厂；缺工厂时极简兜底，不抄内部结构）=====
  function ceMount(host, code, corner) {
    if (window.CodeEditor && window.CodeEditor.mount) return window.CodeEditor.mount(host, { code: code || '', corner, readOnly: true });
    host.appendChild(window.tag('pre.fg-ec-pre', esc(code || '')));
  }
  function tagsMount(host, items, ic, mode, emptyAddless) {
    if (window.Tags && window.Tags.mount) return window.Tags.mount(host, { items: items || [], icon: ic, mode: mode || 'multi', addLabel: emptyAddless || 'add' });
    host.appendChild(window.tag('span.fg-ec-mask', (items && items.length) ? items.join(' · ') : '— 无 —'));
  }
  function kvMount(host, rows) {
    if (window.KV && window.KV.defs) return window.KV.defs(host, rows);
    host.appendChild(window.tag('span.fg-ec-mask', '— 无 —'));
  }
  function graphMount(host, a) {
    if (window.RunGraph && window.RunGraph.render) {
      // workflow 概览图：节点态据 running 标 / parked 标 → running/parked，余 completed（airy 图，全绿底）
      const nodes = (a.nodes || []).map(n => ({ id: n.id, kind: n.kind, ref: n.ref, x: n.x, y: n.y }));
      const state = {}; nodes.forEach(n => { state[n.id] = n.id === a.running ? 'running' : (a.parkedNode === n.id ? 'parked' : 'completed'); });
      const taken = (a.edges || []).map(([x, y]) => x + '>' + y);
      let mx = 0, my = 0; nodes.forEach(n => { mx = Math.max(mx, n.x + 164); my = Math.max(my, n.y + 60); });
      return window.RunGraph.render(host, { nodes, edges: a.edges || [], loopbacks: a.loopbacks || [], state, taken, live: a.running ? (taken.find(e => e.endsWith('>' + a.running)) || null) : null, vb: [mx + 24, my + 24] });
    }
    // 兜底：无图组件时落节点行列表（不抄图 SVG）
    const rows = window.tag('div.fg-ec-rows'); host.appendChild(rows);
    (a.nodes || []).forEach(n => entityRow(rows, { ico: NODE_KIND[n.kind] || n.kind, id: n.id, ref: n.ref, refKind: 'function' }));
  }

  // ===== 版本 tab：优先公开 VersionDiff；缺则内联兜底（镜像 design-lab versionDiff，CodeEditor.highlight 着色）=====
  function lineDiff(a, b) {
    const A = String(a).split('\n'), B = String(b).split('\n'), m = A.length, n = B.length;
    const dp = Array.from({ length: m + 1 }, () => new Array(n + 1).fill(0));
    for (let i = m - 1; i >= 0; i--) for (let j = n - 1; j >= 0; j--) dp[i][j] = A[i] === B[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
    const o = []; let i = 0, j = 0;
    while (i < m && j < n) { if (A[i] === B[j]) { o.push(['ctx', A[i]]); i++; j++; } else if (dp[i + 1][j] >= dp[i][j + 1]) o.push(['del', A[i++]]); else o.push(['add', B[j++]]); }
    while (i < m) o.push(['del', A[i++]]); while (j < n) o.push(['add', B[j++]]); return o;
  }
  const hl = code => (window.CodeEditor && window.CodeEditor.highlight) ? window.CodeEditor.highlight(code, 'code') : esc(code);
  function versionTab(host, a) {
    const verField = v => v.code != null ? v.code : (v._ != null ? v._ : '');
    const cur = a.code || a.system || a.template || a.body || (a.branches ? a.branches.map(x => Array.isArray(x) ? x.join(' → ') : (x.when + ' → ' + x.port)).join('\n') : JSON.stringify(a.config || {}, null, 2));
    const verLess = ['trigger', 'mcp', 'skill'].includes(a.kind);
    const caption = a.kind === 'function' ? 'Function 版本 diff · 非 git'
      : verLess ? (a.kind === 'skill' ? '保存文件 diff · 无版本' : (a.kind === 'mcp' ? '连接配置 diff · 无版本 · 外部 server' : '配置编辑 · 无版本 · Trigger 不入版本'))
      : kindMeta(a.kind).label + ' 版本 diff · 非 git';
    // VersionDiff 契约：每行键 v(版本号) + active/t/reason；a.versions 用 n 或 v 皆收，缺则合成「当前 vs 上一次保存」两版
    const versions = (a.versions || []).map(x => ({ v: x.n != null ? x.n : x.v, active: x.active, t: x.t, reason: x.reason, code: x.code, _: x._ }));
    if (!versions.length) versions.push(
      { v: a.version || 1, t: '当前', reason: '当前', active: true, _: cur },
      { v: (a.version || 2) - 1, t: '更早', reason: '上一次保存', _: String(cur).split('\n').slice(0, -1).join('\n') });
    // 公开 VersionDiff 优先（与 entities 海洋同源；版本列 + LCS 行级增删 + CodeEditor 着色）
    if (window.VersionDiff && window.VersionDiff.mount) { window.VersionDiff.mount(host, { versions, field: verField, caption }); return; }

    // 内联兜底：版本列 + diff 区（镜像 design-lab .eo-vers/.eo-diff/.eo-dl 度量）
    const w = window.tag('div.fg-ec-vers');
    w.innerHTML = `<div class="fg-ec-vlist"></div><div class="fg-ec-diff"><div class="fg-ec-diff-cap"></div><div class="fg-ec-diff-body"></div></div>`;
    const list = w.querySelector('.fg-ec-vlist'), capE = w.querySelector('.fg-ec-diff-cap'), body = w.querySelector('.fg-ec-diff-body');
    let sel = 0;
    function paint() {
      list.innerHTML = versions.map((v, i) => `<div class="fg-ec-vrow${v.active ? ' cur' : ''}${i === sel ? ' on' : ''}" data-i="${i}"><span class="vn">v${esc(v.v)}</span><span class="vt">${esc(v.t || '')}</span><span class="vd">${esc(v.reason || '')}</span></div>`).join('');
      list.querySelectorAll('.fg-ec-vrow').forEach(r => r.onclick = () => { sel = +r.dataset.i; paint(); });
      const nv = versions[sel], ov = versions[sel + 1];
      if (!ov) { capE.innerHTML = `<span class="mono">v${esc(nv.v)}</span><span>· 最早版本</span>`; body.innerHTML = String(verField(nv)).split('\n').map((l, i) => `<div class="fg-ec-dl"><span class="ln">${i + 1}</span><span class="sg"></span><span class="ct">${hl(l)}</span></div>`).join(''); return; }
      const d = lineDiff(verField(ov), verField(nv)); let ln = 0, ad = 0, de = 0;
      body.innerHTML = d.map(([op, t]) => { const sg = op === 'add' ? '+' : op === 'del' ? '−' : ' '; if (op === 'add') ad++; if (op === 'del') de++; return `<div class="fg-ec-dl ${op}"><span class="ln">${op === 'del' ? '' : ++ln}</span><span class="sg">${sg}</span><span class="ct">${hl(t)}</span></div>`; }).join('');
      capE.innerHTML = `<span class="mono">v${esc(ov.v)} → v${esc(nv.v)}</span><span>· ${esc(caption)}</span><span class="pm"><span class="a">+${ad}</span> <span class="d">−${de}</span></span>`;
    }
    paint(); host.appendChild(w);
  }

  // 运行 tab：聚合条 + 账本行（cst 点 + 触发源 + 时间 + 耗时）；mock 驱动，行属运行可点走 Intent(kind=run)
  function runsTab(host, a) {
    const ex = a.execs || [['ok', 'manual', '120ms', '刚刚'], ['ok', 'workflow', '98ms', '今天'], ['failed', 'manual', '—', '昨天']];
    const ok = ex.filter(e => e[0] === 'ok').length;
    host.appendChild(window.tag('div.fg-ec-agg',
      `<span><b>${ex.length}</b> 次</span><span><b class="ok">${ok}</b> 成功</span><span><b class="bad">${ex.length - ok}</b> 失败</span>`));
    const led = window.tag('div.fg-ec-led'); host.appendChild(led);
    ex.forEach((e, i) => {
      const dot = window.StatusDot ? window.StatusDot.dot(e[0] === 'ok' ? 'done' : 'err', { size: 7 }) : '';
      const id = `${(a.idPrefix || a.kind)}e_${(i + 7).toString(16)}a${i}`;
      const r = window.tag('div.fg-ec-lrow', `<span class="fg-ec-cst">${dot}</span><span class="fg-ec-cid">${esc(id)}</span><span class="fg-ec-ctrig">${esc(e[1])}</span><span class="fg-ec-cmeta">${esc(e[3])}</span><span class="fg-ec-cdur">${esc(e[2])}</span>`);
      r.onclick = () => window.Intent && Intent.select({ kind: 'run', id });
      led.appendChild(r);
    });
  }

  // 迭代 tab：迭代 = 在对话里让 AI 改实体（:iterate → conversationId）；此处为说明态
  function iterTab(host) {
    host.appendChild(window.tag('div.fg-ec-mask.fg-ec-iter',
      '迭代 = 在对话里让 AI 改这个实体（:iterate → conversationId）。流式编辑见对话海洋右岛卡。'));
  }

  // 四 tab 集（按 kind 调整运行/版本可见性，但 key 固定 o/v/r/i，与 PUBLIC API 的 概览/版本/运行/迭代 对齐）
  function tabsFor(a) {
    const items = [{ key: 'o', label: '概览', render: b => buildBody(b, a) }];
    const verLess = ['trigger', 'mcp', 'skill'].includes(a.kind);
    items.push({ key: 'v', label: verLess ? (a.kind === 'skill' ? '历史' : '编辑历史') : '版本', render: b => versionTab(b, a) });
    if (!['control', 'approval', 'skill'].includes(a.kind)) items.push({ key: 'r', label: a.kind === 'handler' ? '调用' : '运行', render: b => runsTab(b, a) });
    items.push({ key: 'i', label: '迭代', render: b => iterTab(b) });
    return items;
  }

  // ===== 工厂 =====
  function mount(host, entity) {
    const a = entity || {};
    const k = kindMeta(a.kind);

    // 外壳：传入 host 则就地渲染；否则用 RightIsland 抽屉（oceanId 取实体 kind 作槽键）
    let shell = null, root, body;
    if (host) {
      root = window.tag('div.fg-ec'); host.appendChild(root); body = root;
    } else if (window.RightIsland && window.RightIsland.create) {
      shell = window.RightIsland.create('entity-card', { title: a.name || '', icon: k.icon, width: 384 });
      shell.show();
      root = window.tag('div.fg-ec'); shell.body.appendChild(root); body = root;
    } else {
      root = window.tag('div.fg-ec'); document.body.appendChild(root); body = root;
    }

    // 头部（kind 图标 + 名 + 副行 + live 徽）——RightIsland 已含图标+标题，故此处头给副行+live；就地模式给完整头
    const headInner = `<span class="fg-ec-etype">${ICO(k.icon, 17)}</span>`
      + `<span class="fg-ec-ht"><b class="fg-ec-name">${esc(a.name || '')}</b><span class="fg-ec-sub">${headSub(a)}</span></span>`
      + `<span class="fg-ec-livewrap" data-ec-live>${liveBadge(a.live)}</span>`;
    body.appendChild(window.tag('div.fg-ec-head', headInner));

    // 四 tab（经公开 Tabs；懒渲染每 tab）
    const tabsHost = window.tag('div.fg-ec-tabs'); body.appendChild(tabsHost);
    if (window.Tabs && window.Tabs.mount) window.Tabs.mount(tabsHost, tabsFor(a), {});
    else { const o = window.tag('div'); tabsHost.appendChild(o); buildBody(o, a); }

    // 脚注（id + runs + 历史版本动作）
    const id = a.id || '';
    const runs = a.runs != null ? ` · ${esc(a.runs)} runs` : '';
    const versioned = !['trigger', 'mcp'].includes(a.kind);
    body.appendChild(window.tag('div.fg-ec-foot',
      `<span class="fg-ec-fid">${esc(id)}</span><span class="fg-ec-frun">${runs}</span><span class="fg-ec-fgap"></span>${versioned ? '<span class="fg-ec-act">历史版本</span>' : ''}`));

    // RefPill 委托点击（整片 root 内的实体提及药丸 → Intent.select）
    if (window.RefPill && window.RefPill.wire) window.RefPill.wire(root);

    // ----- handle 方法 -----
    const q = sel => root.querySelector(sel);

    // live 徽推进（false/null → 已保存；'forge'/'edit'/true → 脉冲徽）
    function setLive(v) {
      const w = q('[data-ec-live]'); if (!w) return;
      w.innerHTML = (v === false || v == null) ? `<span class="fg-ec-live saved">已保存</span>` : liveBadge(v);
    }
    // 状态徽原位推进（找 data-f 下 data-ec-st；mapKey 缺则取锚点记录的 map）
    function setSt(dataF, mapKey, state) {
      const anchor = q(`[data-f="${dataF}"] [data-ec-st]`); if (!anchor) return;
      const map = mapKey || anchor.dataset.ecMap || 'ENV';
      if (window.StatusDot) anchor.innerHTML = window.StatusDot.badge(map, state);
      anchor.dataset.ecMap = map;
    }
    // 版本副行原位改（锻造保存后 v++）
    function setVersion(v) { const e = q('[data-ec-ver]'); if (e) e.textContent = 'v' + v; }
    // 生命周期徽原位改（workflow 上线/收尾/未上线）
    function setLifecycle(life) {
      const e = q('[data-ec-life]'); if (!e) return;
      const txt = { active: '运行中', draining: '收尾中', inactive: '未上线' }[life] || '未上线';
      e.className = `fg-ec-life ${life}`;
      e.innerHTML = `${life === 'active' ? '<span class="fg-ec-life-dot"></span>' : ''}${esc(txt)}`;
    }
    // 流式揭示：闪一下该 data-f 字段（锻造逐字段落字时的高亮回执）；掩码项同时去掩码
    function reveal(dataF, key) {
      const f = q(`[data-f="${dataF}"]`); if (!f) return;
      // 含掩码键值行（KV）的，匹配 key 去掩码
      if (key) f.querySelectorAll('.fg-kv-row').forEach(row => {
        const kEl = row.querySelector('.fg-kv-k'), vEl = row.querySelector('.fg-kv-v');
        if (kEl && vEl && kEl.textContent.includes(key)) { vEl.classList.remove('mask'); vEl.classList.add('fg-ec-flash'); }
      });
      // 字段壳整体闪一下
      const target = f.querySelector('.fg-ec-val, .fg-ec-slot, .fg-kv, .fg-ce') || f;
      target.classList.remove('fg-ec-flash'); void target.offsetWidth; target.classList.add('fg-ec-flash');
    }
    // 流式填充：把任意 html 注入该 data-f 字段的内嵌槽（锻造时 code/desc 等逐字段成型）
    function fill(dataF, html) {
      const f = q(`[data-f="${dataF}"]`); if (!f) return;
      const slot = f.querySelector('[data-ec-slot]') || f.querySelector('.fg-ec-val') || f;
      slot.innerHTML = html == null ? '' : html;
      slot.classList.add('fg-ec-flash');
    }

    const show = () => { if (shell) shell.show(); else root.classList.remove('fg-ec-hidden'); return handle; };
    const hide = () => { if (shell) shell.hide(); else root.classList.add('fg-ec-hidden'); return handle; };
    const toggle = () => { if (shell) shell.toggle(); else root.classList.toggle('fg-ec-hidden'); return handle; };
    const destroy = () => { if (shell) { shell.hide(); shell.el.remove(); } else root.remove(); };

    const handle = { el: root, setLive, setSt, setVersion, setLifecycle, reveal, fill, show, hide, toggle, destroy };
    return handle;
  }

  window.EntityCard = { mount };
})();

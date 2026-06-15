/* Forgify demo — 实体海洋海面：薄组合（产品级编辑/调试平台）。
   心智：中间 = 编辑「它是什么」（单页竖滚的完整编辑器，静态信息全可就地改）；右岛 = 运行「它能做什么」（试运行/调试中心）。
   中间像素全在组件库——CodeEditor(代码/class/prompt/template/playbook) + Tags(deps/tools/triggers/outputs/knowledge/allowed)
   + KV(runtime/model/rules/frontmatter/connection) + ThinTable(IO/methods/init-args/exposed tools) + VersionDiff(版本) + RunGraph(workflow 图) + Attention/StatusDot/RefPill；
   右岛 = RightIsland 基座 + RunDebug(试运行) + 最近运行台账；workflow 点图节点 → 右岛检视记忆化结果。
   顶栏右上 = 保存态 + 「试运行」面板开关（对齐别的海洋开右岛）+ ⋯ 更多（迭代/重命名/复制/删除）。迭代不再单独占位、降为安静工具。
   选中通道：侧栏实体行 / 关系反链 → Intent.select({kind:'entity'}) → Intent.on('entity') morph。依赖 mock/entities.js + config/{entity-kinds,state-model}.js。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);
  const D = () => window.MOCK_ENTITIES || {};
  const K = () => window.ENTITY_KINDS || {};
  const esc = s => String(s == null ? '' : s).replace(/[&<>]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]));

  let stage, island, curId, savedEl, islandOpen = false;


  // ===== 薄分节：小标签 + 内容直接铺白底（无盒，靠留白）=====
  function sec(host, label, cnt) {
    const s = tag('div.ent-sec', `<div class="ent-h"><span>${esc(label)}</span>${cnt != null ? `<span class="ent-cnt">${cnt}</span>` : ''}</div>`);
    host.appendChild(s);
    return s;
  }
  function prose(host, value, cls) {
    const p = tag('div.' + (cls || 'ent-desc'));
    p.contentEditable = 'true'; p.spellcheck = false; p.textContent = value || '';
    p.addEventListener('input', markDirty);
    host.appendChild(p);
    return p;
  }
  // 细线表格 IO 行：名(+必填星) / 类型
  const ioRows = arr => arr.map(f => [`<span class="ent-nm">${esc(f[0])}</span>${f[2] ? '<span class="ent-req">*</span>' : ''}`, `<span class="ent-ty">${esc(f[1])}</span>`]);
  // 列表型分节尾的「+ 添加」（完整编辑：就地追加一行可编辑占位；纯演示）
  function addRow(secEl, label, onAdd) {
    const b = tag('button.ent-add', `${icon('plus', 13)}${esc(label)}`);
    b.onclick = () => { onAdd && onAdd(); markDirty(); };
    secEl.appendChild(b);
    return b;
  }

  // dirty / 保存态（保存指示在 headExtra；dirty 类染 accent）
  function markDirty() {
    if (savedEl) { savedEl.classList.add('dirty'); savedEl.innerHTML = `<span class="ent-saved-ic">${icon('edit', 13)}</span>未保存`; }
  }

  // 折叠分节（版本 / 关系放中间流末尾，可折叠）
  function foldSec(host, label, open) {
    const s = tag('div.ent-fold', `<button class="ent-fold-h"><span class="ent-fold-chev">${icon('chevr', 13)}</span><span>${esc(label)}</span></button><div class="ent-fold-b"></div>`);
    if (open) s.classList.add('open');   // 类经 classList 加——tag() 的类串以「.」分隔、不吃空格
    s.querySelector('.ent-fold-h').onclick = () => s.classList.toggle('open');
    host.appendChild(s);
    return s.querySelector('.ent-fold-b');
  }

  // 关系/反链渲染（中间末尾分节 body；行点击 → Intent.select 跳归属海洋）
  function relationsInto(host, groups) {
    (groups || []).forEach(g => {
      const s = tag('div.ent-rel-sec', `<div class="ent-rel-h">${esc(g.title)}</div>`);
      if (!g.rows.length) s.appendChild(tag('div.ent-none', '— 无 —'));
      g.rows.forEach(r => {
        const ico = (K()[r.kind] || {}).icon || r.kind || 'link';
        const row = tag('div.ent-rel-row', `<span class="ent-rel-ico">${icon(ico, 14)}</span><span class="ent-rel-n">${esc(r.name)}</span><span class="ent-rel-m">${esc(r.meta || '')}</span>`);
        if (K()[r.kind]) row.onclick = () => Intent.select({ kind: 'entity', id: r.name });
        else if (r.kind === 'conversation') row.onclick = () => Intent.select({ kind: 'conversation', id: r.meta || r.name });
        else if (r.kind === 'workflow') row.onclick = () => Intent.select({ kind: 'workflow', id: r.name });
        s.appendChild(row);
      });
      host.appendChild(s);
    });
  }

  // ===================== 中间：各类型静态定义（可编辑；无试运行——那在右岛）=====================
  const ENV_GATE = { pending: '排队', syncing: '物化中', failed: '失败' };
  const SPEC = {
    function(b, a) {
      prose(b, a.desc);
      CodeEditor.mount(sec(b, 'Code'), { code: a.code, corner: a.lang, onDirty: markDirty });
      const g = b;   // 单列流：原 2 栏并排失衡（对齐 documents 单列），分节直接堆叠到主体
      const inSec = sec(g, 'Inputs', a.inputs.length);
      ThinTable.table(inSec, ['参数', '类型'], ioRows(a.inputs));
      const addBtn = addRow(inSec, '添加入参', () => {
        const r = tag('div.ent-newrow', `<span class="ent-nm" contenteditable="true">name</span><span class="ent-ty" contenteditable="true">type</span>`);
        inSec.insertBefore(r, addBtn);
      });
      ThinTable.table(sec(g, 'Output'), ['返回', '字段'], a.output.map(o => [`<span class="ent-nm">${esc(o[0])}</span>`, `<span class="ent-ty">${esc(o[1])}</span>`]));
      Tags.mount(sec(b, 'Dependencies', a.deps.length), { items: a.deps, onChange: markDirty });
      const env = sec(b, 'Environment');
      env.appendChild(tag('div.ent-envrow', `${StatusDot.badge('ENV', a.env)}<span class="ent-note">上次：${esc(a.lastRun || '—')}</span><button class="ent-mini">${icon('spin', 13)}Rebuild env</button>`));
    },
    handler(b, a) {
      prose(b, a.desc);
      KV.defs(sec(b, 'Runtime'), [
        ['Runtime', '', { html: a.life === 'active' ? `${StatusDot.dot('done')}<span class="ent-inlabel">运行中</span>` : `${StatusDot.dot('idle')}<span class="ent-inlabel">未上线</span>` }],
        ['Config', '', { html: StatusDot.badge('CFG', a.cfg) }],
        ['Env', '', { html: StatusDot.badge('ENV', a.env) }],
      ]);
      CodeEditor.mount(sec(b, 'Assembled class'), { code: a.code, corner: a.lang, onDirty: markDirty });
      ThinTable.table(sec(b, 'Methods', a.methods.length), ['方法', '签名', ''], a.methods.map(m => [`<span class="ent-nm">${esc(m[0])}</span>`, `<span class="ent-ty">${esc(m[1])}</span>`, `<span class="ent-act" data-call="${esc(m[0])}">Call ›</span>`]));
      const t = ThinTable.table(sec(b, 'Init args'), ['参数', '值'], a.initArgs.map(([k, v, s]) => [`<span class="ent-nm">${esc(k)}</span>`, s ? { edit: true, html: '<span class="ent-mask">••••••••</span>' } : { edit: true, html: esc(v || '') }]));
      t.insertAdjacentHTML('afterend', '<div class="ent-note ent-note-mt">改 config 触发重启</div>');
      b.querySelectorAll('[data-call]').forEach(el => el.onclick = () => openDebug(a, el.dataset.call));
    },
    agent(b, a) {
      if (a.tools.some(t => t.health === 'bad')) {
        const w = tag('div', Attention.html('shield', '挂载工具 <b>cite</b> 不可解析，invoke 时将跳过。', { tone: 'warn' }));
        b.appendChild(w.firstElementChild);
      }
      prose(b, a.desc);
      prose(sec(b, 'System prompt'), a.system, 'ent-block');
      Tags.mount(sec(b, 'Mounted tools', a.tools.length), { items: a.tools, icon: 'code', onChange: markDirty });
      const g = b;   // 单列流：原 2 栏并排失衡（对齐 documents 单列），分节直接堆叠到主体
      KV.defs(sec(g, 'Model'), [['model', a.model, { edit: true, mono: true }], ['maxSteps', a.maxSteps, { edit: true, mono: true }]]);
      Tags.mount(sec(g, 'Skill · 0–1'), { items: a.skill ? [a.skill] : [], icon: 'skill', mode: 'single', addLabel: '挂技能', onChange: markDirty });
      Tags.mount(sec(b, 'Knowledge', a.knowledge.length), { items: a.knowledge, onChange: markDirty });
    },
    workflow(b, a) {
      if (a.attention) {
        const w = tag('div', Attention.html('shield', a.attention, { tone: 'warn' }));
        b.appendChild(w.firstElementChild);
      }
      if (a.desc) prose(b, a.desc);
      const graphSec = sec(b, 'Graph', a.nodes.length + ' 节点');
      const gwrap = tag('div.ent-gwrap');
      graphSec.appendChild(gwrap);
      RunGraph.render(gwrap, {
        nodes: a.nodes, edges: a.edges, loopbacks: a.loopbacks, vb: a.vb,
        state: a.state, taken: a.taken, live: a.live, iters: a.iters, ports: a.ports,
        onNode: id => openNode(a, id),
      });
      gwrap.insertAdjacentHTML('beforeend', '<div class="ent-ghint">点击节点 → 右岛检视引用与记忆化结果 · 深度运行历史在 Scheduler 海</div>');
      const g = b;   // 单列流：原 2 栏并排失衡（对齐 documents 单列），分节直接堆叠到主体
      KV.defs(sec(g, 'Run'), [
        ['lifecycle', '', { html: a.life === 'active' ? `${StatusDot.dot('run')}<span class="ent-inlabel">已激活</span>` : `${StatusDot.dot('idle')}<span class="ent-inlabel">未上线</span>` }],
        ['concurrency', a.concurrency, { edit: true, mono: true }],
      ]);
      Tags.mount(sec(g, 'Triggers', a.triggers.length), { items: a.triggers, icon: 'trigger', onChange: markDirty });
    },
    trigger(b, a) {
      prose(b, a.desc);
      KV.defs(sec(b, 'Signal source'), a.cfg.map(([k, v, mask]) => [k, v, mask ? { mask: true } : { edit: true, mono: true }]));
      Tags.mount(sec(b, 'Outputs', a.outputs.length), { items: a.outputs, onChange: markDirty });
    },
    control(b, a) {
      prose(b, a.desc);
      const br = sec(b, 'Branches · 首个为真胜出', a.branches.length);
      a.branches.forEach((x, i) => br.appendChild(tag('div.ent-branch', `<span class="ent-bn">${i + 1}</span><span class="ent-bwhen">${CodeEditor.highlight(x[0])}</span><span class="ent-bport">→ ${esc(x[1])}</span>`)));
      br.appendChild(tag('div.ent-branch.catchall', `<span class="ent-bn">·</span><span class="ent-bwhen">true</span><span class="ent-bport">→ default</span>`));
      ThinTable.table(sec(b, 'Inputs (CEL namespace)', a.inputs.length), ['参数', '类型'], ioRows(a.inputs));
    },
    approval(b, a) {
      prose(b, a.desc);
      CodeEditor.mount(sec(b, 'Template · {{input.*}} 插值'), { code: a.template, corner: 'jinja+cel', onDirty: markDirty });
      const g = b;   // 单列流：原 2 栏并排失衡（对齐 documents 单列），分节直接堆叠到主体
      KV.defs(sec(g, 'Decision rules'), a.rules.map(([k, v]) => [k, v, { edit: true, mono: true }]));
      ThinTable.table(sec(g, 'Input schema', a.inputs.length), ['字段', '类型'], ioRows(a.inputs));
    },
    mcp(b, a) {
      prose(b, a.desc);
      KV.defs(sec(b, 'Connection'), [
        ['status', '', { html: StatusDot.badge('CONN', a.conn) }],
        ...a.cfg.map(([k, v, mask]) => [k, v, mask ? { mask: true } : { edit: true, mono: true }]),
        ['calls / fails', `${a.calls} / ${a.fails}`, { mono: true }],
      ]);
      ThinTable.table(sec(b, 'Exposed tools', a.tools.length), ['工具', ''], a.tools.map(t => [`<span class="ent-nm">${esc(t)}</span>`, `<span class="ent-act" data-invoke="${esc(t)}">Invoke ›</span>`]));
      b.querySelectorAll('[data-invoke]').forEach(el => el.onclick = () => openDebug(a, el.dataset.invoke));
    },
    skill(b, a) {
      prose(b, a.desc);
      CodeEditor.mount(sec(b, 'Playbook · $n / ${...} 插值'), { code: a.body, corner: 'markdown', onDirty: markDirty });
      const g = b;   // 单列流：原 2 栏并排失衡（对齐 documents 单列），分节直接堆叠到主体
      KV.defs(sec(g, 'Frontmatter'), a.frontmatter.map(([k, v]) => [k, v, { edit: true, mono: true }]));
      Tags.mount(sec(g, 'Allowed tools', a.allowed.length), { items: a.allowed, icon: 'code', onChange: markDirty });
    },
  };

  // 版本（中间末尾折叠分节）；mcp 无版本号、标「编辑历史」
  function versionsInto(b, a) {
    if (a.kind === 'function') return VersionDiff.mount(b, { versions: a.versions, field: 'code', caption: 'Function 版本 diff · 非 git' });
    const cur = a.code || a.system || a.template || a.body || (a.branches ? a.branches.map(x => x.join(' → ')).join('\n') : JSON.stringify(a.cfg || {}, null, 2));
    const verLess = ['trigger', 'mcp', 'skill'].includes(a.kind);
    const cap = verLess
      ? (a.kind === 'skill' ? '保存文件 diff · 无版本' : (a.kind === 'mcp' ? '连接配置 diff · 无版本 · 外部 server' : '配置编辑 · 无版本 · Trigger 不入版本'))
      : (K()[a.kind].label + ' 版本 diff · 非 git');
    VersionDiff.mount(b, {
      versions: [
        { v: a.version || 1, active: true, t: '当前', reason: '当前', _: cur },
        { v: (a.version || 2) - 1, t: '更早', reason: '上一次保存', _: cur.split('\n').slice(0, -1).join('\n') },
      ],
      field: v => v._, caption: cap,
    });
  }

  // ===================== 右岛：试运行 / 调试中心 =====================
  // 各类型「试」配置（RunDebug.mount 参数）；handler/mcp 经选择器选目标，workflow 触发 + 节点检视
  const DBG = {
    function: a => a.env === 'ready'
      ? { argsSeed: '{\n  "currency": "USD"\n}', verb: 'Run', vico: 'play', trace: { lines: ['→ spawn sandbox (python 3.12)', '→ exec process_invoice()', 'stdout: parsed 14 line items', 'stdout: validated ✓'], result: { st: 'ok', out: '{ "vendor": "Acme Inc", "total": 1284.50, "tax_id": "US-99-1" }', ms: 412 } } }
      : { gate: '环境' + (ENV_GATE[a.env] || a.env) + ' — 就绪后可运行' },
    agent: () => ({ argsSeed: '{\n  "query": "竞品近况",\n  "scope": "2026"\n}', verb: 'Invoke', vico: 'play', trace: { lines: ['mount-health: 4/5 ok（cite 跳过）', '⟐ reasoning…', '→ tool web_search("竞品近况 2026")', '→ tool fetch_url(...)', '⟐ 综述生成中…'], result: { st: 'ok', out: 'stopReason=end · 6 steps · 1.2k→3.4k tok', ms: 8800 } } }),
    trigger: () => ({ verb: 'Fire', vico: 'zap', trace: { lines: ['fanned to 1 workflow', 'nightly_report → started', 'flowrun created'], result: { st: 'ok', out: 'fired · 1 firing · 0 skipped', ms: 42 } } }),
    control: () => ({ argsSeed: '{ "amount": 1500 }', verb: 'Evaluate', vico: 'play', trace: { lines: ['branch 1: amount > 1000 → ✓ 命中', 'branch 2: 不再求值'], result: { st: 'ok', out: '→ port "approve"', ms: 3 } } }),
    approval: () => ({ argsSeed: '{ "vendor":"Acme", "total":1284.5, "currency":"USD" }', verb: 'Render', vico: 'play', trace: { lines: ['解析 {{input.*}} → 渲染 markdown', 'emit parked · 待决策 · 24h 后 reject'], result: { st: 'ok', out: 'parked → 等待人工 通过/驳回', ms: 6 } } }),
    skill: () => ({ argsSeed: '$1 = "竞品调研"', verb: 'Render', vico: 'play', trace: { lines: ['替换 $1 → "竞品调研"', '替换 ${CLAUDE_SESSION_ID}', '注入 agent system-prompt 的 ## Execution guide'], result: { st: 'ok', out: '已展开 · 注入 312 字', ms: 2 } } }),
    workflow: () => ({ verb: 'Trigger', vico: 'zap', trace: { lines: ['flowrun 创建', '→ 首节点 started', '在 Scheduler 看实时进度'], result: { st: 'ok', out: 'run started', ms: 28 } } }),
  };
  const HAS_RUNS = a => !['control', 'approval', 'skill'].includes(a.kind);   // 这三种是内联求值/渲染、不产生耐久 run

  // 最近运行台账（右岛底；narrow 单列）
  function recentRuns(host, a) {
    const ex = a.execs || [['ok', 'manual', '120ms', '刚刚'], ['ok', 'workflow', '98ms', '今天'], ['failed', 'manual', '—', '昨天']];
    const ok = ex.filter(e => e[0] === 'ok').length;
    const s = tag('div.ent-dbg-sec', `<div class="ent-dbg-h">最近运行<span class="ent-agg-mini">${ex.length} 次 · <b class="ok">${ok}</b> ok · <b class="bad">${ex.length - ok}</b> 失败</span></div>`);
    const led = tag('div.ent-led'); s.appendChild(led);
    ex.forEach((e, i) => led.appendChild(tag('div.ent-lrow', `${StatusDot.dot(e[0] === 'ok' ? 'done' : 'err', { size: 7 })}<span class="ent-ctrig">${esc(e[1])}</span><span class="ent-cmeta">${esc(e[3])}</span><span class="ent-cdur">${esc(e[2])}</span>`)));
    const note = tag('div.ent-sched', `${icon('scheduler', 14)}深度历史在 <span class="ent-lnk" data-s>Scheduler 海</span>`);
    note.querySelector('[data-s]').onclick = () => Shell.toOcean && Shell.toOcean('scheduler');
    s.appendChild(note);
    host.appendChild(s);
  }

  function ensureIsland() {
    if (!island || !island.el || !island.el.isConnected) island = RightIsland.create('entities', { title: '试运行', icon: 'play', width: 396 });
    return island;
  }
  function islandHead(title, sub, ico) {
    island.setHead(`<span class="fg-island-ico">${icon(ico || 'play', 17)}</span>
      <span class="ent-ndh"><b>${esc(title)}</b>${sub ? `<span class="ent-ndsub">${sub}</span>` : ''}</span>
      <button class="fg-island-x" type="button">${icon('close', 16)}</button>`);
    island.head.querySelector('.fg-island-x').onclick = closeDebug;
  }

  // 调试中心主体（按类型）
  function fillDebug(a, target) {
    ensureIsland();
    const b = island.body; b.innerHTML = '';
    const k = K()[a.kind] || {};
    islandHead('试运行', `${esc(k.label || a.kind)} · ${esc((DBG[a.kind] ? DBG[a.kind](a).verb : k.verb) || 'Run')}`, k.vico || 'play');

    if (a.kind === 'handler' || a.kind === 'mcp') {
      const opts = a.kind === 'handler' ? a.methods.map(m => m[0]) : a.tools;
      const verb = a.kind === 'handler' ? 'Call' : 'Invoke';
      const pickWrap = tag('div.ent-dbg-pick', `<label>${a.kind === 'handler' ? '选方法' : '选工具'}</label>`);
      b.appendChild(pickWrap);
      const slot = tag('div'); b.appendChild(slot);
      const mountRD = name => { slot.innerHTML = ''; RunDebug.mount(slot, { argsSeed: '{\n  \n}', verb: verb + (name ? ' ' + name : ''), vico: 'play', trace: { lines: [`→ ${verb.toLowerCase()} ${name}(...)`, 'stdout: ok'], result: { st: 'ok', out: 'done', ms: 86 } } }); };
      Dropdown.mount(pickWrap, { options: opts.map(o => ({ value: o, label: o })), value: target || opts[0], onChange: mountRD });
      mountRD(target || opts[0]);
    } else if (a.kind === 'workflow') {
      RunDebug.mount(b, DBG.workflow(a));
      b.appendChild(tag('div.ent-dbg-hint', `点中间图上的节点 → 这里检视它的引用与记忆化结果`));
    } else {
      RunDebug.mount(b, DBG[a.kind] ? DBG[a.kind](a) : { verb: k.verb || 'Run', vico: k.vico || 'play' });
    }
    if (HAS_RUNS(a)) recentRuns(b, a);
  }

  // workflow 图节点检视（复用同一右岛；提供「← 试运行」返回）
  function openNode(wf, nodeId) {
    const nd = wf.nodes.find(x => x.id === nodeId) || {};
    const st = (wf.state || {})[nodeId] || 'future';
    const refKind = nd.kind === 'action' ? 'function' : nd.kind;
    ensureIsland();
    islandHead(nodeId, `${esc(nd.kind || '')} · ${StatusDot.dot(st)} ${StatusDot.label(st)}`, (window.NODE_ICON || {})[nd.kind] || 'action');
    const b = island.body; b.innerHTML = '';
    const back = tag('button.ent-dbg-back', `${icon('chevr', 13)}试运行`);
    back.onclick = () => fillDebug(wf);
    b.appendChild(back);
    if (nd.ref) { const r = tag('div.ent-ndref', `引用 ${RefPill.html(refKind, nd.ref, nd.ref)}`); RefPill.wire(r); b.appendChild(r); }
    const cel = tag('div.ent-fld', `<label>${nd.kind === 'control' ? '分支条件 (CEL)' : 'Input 映射 (CEL)'}</label>`);
    b.appendChild(cel);
    CodeEditor.mount(cel, { code: nd.kind === 'control' ? 'amount > 1000' : 'input: ctx.upstream.result', corner: 'cel' });
    showDebug();
  }

  // 右岛开/关（顶栏「试运行」开关 + 方法/工具行 Call/Invoke 都走这）
  function showDebug() { islandOpen = true; ensureIsland().show(); const btn = document.querySelector('#head-extra [data-dbg]'); if (btn) btn.classList.add('on'); }
  function closeDebug() { islandOpen = false; if (island) island.hide(); const btn = document.querySelector('#head-extra [data-dbg]'); if (btn) btn.classList.remove('on'); }
  function openDebug(a, target) { fillDebug(a, target); showDebug(); }
  function toggleDebug(a) { islandOpen ? closeDebug() : openDebug(a); }

  // ===================== 顶栏 ⋯ 更多菜单（迭代/重命名/复制/删除）=====================
  function openMore(a, btn) {
    const m = tag('div.ent-omenu',
      `<button class="ent-oitem" data-m="iterate"><span class="ent-oico">${icon('spark', 15)}</span>迭代（AI 改）</button>
       <button class="ent-oitem" data-m="rename"><span class="ent-oico">${icon('edit', 15)}</span>重命名</button>
       <button class="ent-oitem" data-m="dup"><span class="ent-oico">${icon('copy', 15)}</span>复制</button>
       <div class="ent-osep"></div>
       <button class="ent-oitem danger" data-m="del"><span class="ent-oico">${icon('trash', 15)}</span>删除</button>`);
    const f = Floating.open(btn.getBoundingClientRect(), m, { below: true });
    const act = {
      iterate: () => Intent.act({ verb: 'iterate', kind: 'entity', id: curId }),
      rename: () => { const t = stage.querySelector('.ent-title'); if (t) { t.focus(); document.getSelection().selectAllChildren(t); } },
      dup: () => markDirty(),
      del: () => { if (confirm('删除实体 ' + curId + '？（演示用，仅从视图移除）')) empty(); },
    };
    m.querySelectorAll('[data-m]').forEach(el => el.onclick = () => { f.close(); (act[el.dataset.m] || (() => {}))(); });
  }

  // ===================== 详情：单页竖滚编辑器 + 顶栏 + 右岛 =====================
  function detail(id) {
    const a = D()[id];
    if (!a) return empty();
    curId = id;
    const k = K()[a.kind] || K().function;

    const meta = [];
    if (a.version != null) meta.push(`v${a.version}`);
    meta.push(`<span class="ent-st-in">${StatusDot.dot(a.status || 'idle')}<span>${StatusDot.label(a.status || 'idle')}</span></span>`);
    if (a.life) meta.push(`<span class="ent-life${a.life === 'active' ? ' active' : ''}">${a.life === 'active' ? '已激活' : '未上线'}</span>`);
    if (a.runs != null) meta.push(`${a.runs} 次运行`);
    if (a.path) meta.push(`<span class="ent-mono">${esc(a.path)}</span>`);

    stage.innerHTML = `<div class="ent-doc ent-morph">
      <div class="ent-path"><span class="ent-path-ic">${icon(k.icon, 13)}</span><span>${esc(k.label)}</span><span class="ent-sep">/</span><span>${esc(id)}</span></div>
      <div class="ent-title" contenteditable="true" spellcheck="false">${esc(id)}</div>
      <div class="ent-meta"><span class="ent-mrow">${meta.join('<span class="ent-sep">·</span>')}</span><span class="ent-mact"><span class="ent-saved"><span class="ent-saved-ic">${icon('check', 13)}</span>已保存</span><button class="ibtn ent-more" data-more title="更多">${icon('more', 16)}</button></span></div>
      <div class="ent-body"></div>
    </div>`;
    stage.querySelector('.ent-title').addEventListener('input', markDirty);

    const body = stage.querySelector('.ent-body');
    SPEC[a.kind](body, a);                                   // 静态定义（可编辑）
    versionsInto(foldSec(body, a.kind === 'mcp' ? '编辑历史' : '版本', true), a);   // 版本比关系重要：排前、默认展开
    relationsInto(foldSec(body, '关系', false), a.rel || [{ title: 'Referenced by', rows: [] }]);

    // 顶栏右上只留「试运行」面板开关（icon panel，全海洋统一）；保存态与 ⋯ 已移入 meta 行
    Shell.headExtra(`<button class="ibtn ent-pan${islandOpen ? ' on' : ''}" data-dbg title="试运行 / 调试">${icon('panel', 18)}</button>`);
    document.querySelector('#head-extra [data-dbg]').onclick = () => toggleDebug(a);
    savedEl = stage.querySelector('.ent-saved');
    const moreBtn = stage.querySelector('.ent-more');
    moreBtn.onclick = () => openMore(a, moreBtn);

    // 右岛内容随选中实体重建；开合态跨切换保留
    fillDebug(a);
    if (islandOpen) ensureIsland().show(); else if (island) island.hide();

    const sc = document.querySelector('#entScroll'); if (sc) sc.scrollTop = 0;
  }

  function empty() {
    curId = null;
    closeDebug();
    if (Shell.headExtra) Shell.headExtra('');
    const cnt = kind => Object.values(D()).filter(e => e.kind === kind).length;
    const stat = (kind, l) => `<span class="ent-stat"><b>${cnt(kind)}</b><span>${l}</span></span>`;
    stage.innerHTML = `<div class="ent-empty"><div class="ent-empty-in ent-morph"><div class="ent-empty-ic">${icon('entities', 24)}</div><h2>四项全能实体</h2><p>Function · Handler · Agent · Workflow——及组成图的触发器、控制、审批、连接器与技能。<br>从左侧选一个：中间看 / 改它的定义，右岛试运行。</p><div class="ent-stats">${stat('function', 'Functions')}${stat('handler', 'Handlers')}${stat('agent', 'Agents')}${stat('workflow', 'Workflows')}</div></div></div>`;
  }

  Shell.registerOcean('entities', {
    crumb: '实体',
    build(sea) {
      sea.innerHTML = `<div class="ent"><div class="ent-scroll scroll-fade" id="entScroll"><div id="entStage"></div></div></div>`;
      stage = sea.querySelector('#entStage');
      detail('process_invoice');   // 默认开首个 function（与侧栏默认高亮同步）
    },
  });

  // 选中通道：侧栏实体行 / 关系反链 → Intent.select({kind:'entity'}) → morph 成该实体
  Intent.on('entity', sel => { if (stage) detail(sel.id); });
})();

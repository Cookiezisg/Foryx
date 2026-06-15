/* Forgify design-lab — 对话海洋 · 右岛块（实体卡，九类全能）。
   右岛是「本海洋的」：自己 append 到 Shell.body（作第三个 flex 子），自管显隐与流式编辑。
   信号交互 = 锻造工具（create_/edit_*）的流式 args 经 entities 流 forge 镜像 → 这里实时填充。
   一套外壳 / 九种类型：function/handler/agent/workflow/control/approval/trigger/mcp/document，
   只换 etype + 头部副行(HEADSUB) + tabs(TABS) + 字段集(body) + 脚注(FOOT)；状态徽章走统一 badge/setSt。
   ⚠ 回归铁律：function 的 [data-f=code/deps/env] 与 workflow 的 [data-f=graph] 的 data-f key 与内部结构勿改——
     oceans/chat/chat.js 的 live-fill 按这些选择器注入。
   依赖：shared/icons.js（icon）+ shared/shell.js（Shell.body）。样式在同目录 chat.css。 */
window.ChatEntityCard = (function () {
  const $ = (s, r) => r.querySelector(s);
  let el = null;

  const KIND = {
    function: { icon: 'function', label: 'Function' },
    handler:  { icon: 'handler',  label: 'Handler'  },
    agent:    { icon: 'agent',    label: 'Agent'    },
    workflow: { icon: 'workflow', label: 'Workflow' },
    control:  { icon: 'control',  label: 'Control'  },
    approval: { icon: 'shield',   label: 'Approval form' },
    trigger:  { icon: 'trigger',  label: 'Trigger'  },
    mcp:      { icon: 'mcp',      label: 'MCP server' },
    document: { icon: 'doc',      label: 'Document' },
  };

  function ensure() {
    if (el && document.body.contains(el)) return el;
    el = document.createElement('aside');
    el.className = 'aside';
    el.setAttribute('data-ocean-right', 'chat');   // 外壳 mount 时据此清理上个海洋的右岛
    Shell.body.appendChild(el);
    return el;
  }

  const liveBadge = v =>
    v === true || v === 'forge' ? '<span class="live"><span class="pulse"></span>锻造中</span>'
    : v === 'edit'             ? '<span class="live"><span class="pulse"></span>编辑中</span>'
    : '';

  // ===== 统一状态徽章：一套像素、多套 {state:[class,text]} 字典（class 复用 ready/syncing/pending/failed + 新轴重着色） =====
  const MAPS = {
    env:      { pending: ['pending', '排队'], syncing: ['syncing', '物化中…'], ready: ['ready', '就绪'], failed: ['failed', '失败'] },
    cfg:      { unconfigured: ['pending', '未配置'], partially_configured: ['syncing', '部分配置'], ready: ['ready', '就绪'] },
    rt:       { stopped: ['pending', '已停'], running: ['ready', '运行中'], crashed: ['crashed', '已崩溃'] },
    valid:    { invalid: ['invalid', '不通过'], compiling: ['compiling', '编译中…'], valid: ['valid', '通过'] },
    formval:  { invalid: ['invalid', '模板非法'], syncing: ['compiling', '校验中…'], valid: ['valid', '通过'] },
    conn:     { disconnected: ['pending', '未连接'], connecting: ['connecting', '连接中…'], ready: ['ready', '已连接'], degraded: ['degraded', '降级'], failed: ['failed', '失败'] },
  };
  const badge = (mapKey, state, anchor = false) => {
    const m = MAPS[mapKey]; const [c, t] = m[state] || Object.values(m)[0];
    return `<span class="statebadge ${c}"${anchor ? ' data-st' : ''}><span class="dot"></span>${t}</span>`;
  };
  // 带状态徽章的字段（data-f 锚点 + data-st，供 setSt 实时推进）
  const stField = (dataF, label, mapKey, state, note = '') =>
    `<div class="field" data-f="${dataF}"><label>${label}</label><div style="padding:2px 0">${badge(mapKey, state, true)}${note ? ` <span class="ec-note">${note}</span>` : ''}</div></div>`;

  const tags = (arr, empty = '— 无 —') => (arr && arr.length) ? `<div class="taglist">${arr.map(t => `<span class="tag">${t}</span>`).join('')}</div>` : `<span class="ec-mask">${empty}</span>`;
  const kv = rows => rows.map(r => `<div class="ec-kv"><span class="k">${r.k}</span><span class="v${r.mask ? ' ec-mask' : ''}">${r.mask ? '********' : (r.v ?? '')}</span></div>`).join('') || '<span class="ec-mask">— 无 —</span>';
  const NICON = { trigger: 'trigger', action: 'action', agent: 'agent', control: 'control', approval: 'shield' };
  const nodeRow = n => `<div class="ec-row"><span class="nico">${icon(NICON[n.kind] || n.kind, 15)}</span><span class="nid">${n.id}</span>${n.port ? `<span class="nport">${n.port}</span>` : ''}<span class="nref">${n.ref || ''}</span></div>`;
  const SRCICON = { webhook: 'webhook', cron: 'cron', fsnotify: 'fsnotify', sensor: 'sensor' };
  const celHi = t => (t || '').replace(/\{\{([^}]*)\}\}/g, '<span class="cel-tok">{{$1}}</span>');
  const wikiHi = t => (t || '').replace(/\[\[([^\]]*)\]\]/g, '<span class="wikilink-pill">$1</span>');
  // control 分支行（first-true-wins 有序；末条 catchall；坏分支 danger 描边）
  const branchRow = (b, i) => `<div class="branch-row${b.bad ? ' bad' : ''}${b.catchall ? ' catchall' : ''}"><span class="bnum">${i + 1}</span><div class="bbody"><div class="bwhen">${b.catchall ? 'true（兜底）' : celHi(b.when)}</div>${b.emit ? `<div class="bemit">emit → ${celHi(b.emit)}</div>` : ''}</div><span class="bport">${b.port}</span></div>`;

  // ===== 头部副行 / tabs / 脚注：按实体分支（版本 vs 源 vs 路径；runs vs 无；tab 名） =====
  function headSub(a) {
    const k = KIND[a.kind] || KIND.function;
    if (a.kind === 'mcp') return `${k.label} · <span class="ec-source">${a.source || 'manual'}</span>`;
    if (a.kind === 'trigger') return `${k.label} · <span class="src-badge">${a.source || 'webhook'} · ${a.listening ? '监听中' : '未上线'}</span>`;
    if (a.kind === 'document') return `${k.label} · <span class="ec-path">${a.path || '/'}</span>`;
    let s = `${k.label} · <span class="ver" data-ver>v${a.version || 1}</span>`;
    if (a.kind === 'handler') s += ' · 常驻进程';
    else if (a.kind === 'control') s += ' · 内联求值';
    else if (a.kind === 'approval') s += ' · 人在环闸';
    return s;
  }
  const TABS = { _: ['概览', '版本', '运行', '迭代'], document: ['概览', '正文', '反链', '迭代'], approval: ['概览', '版本', '收件箱', '迭代'], trigger: ['概览', 'Activations', 'Firings', '迭代'], mcp: ['概览', '工具', '调用', '迭代'] };
  function foot(a) {
    const id = `<span class="mono">${a.id || ''}</span>`;
    const runs = a.runs != null ? ` · ${a.runs} runs` : '';
    const versioned = !['trigger', 'mcp', 'document'].includes(a.kind);
    return `${id}${runs}<span class="gap"></span>${versioned ? '<span class="act">历史版本</span>' : ''}`;
  }

  // ===== 字段集（每实体一套；function/workflow 的 data-f 与结构是回归铁律，勿改） =====
  function body(a) {
    if (a.kind === 'function') return `
      ${a.desc ? `<div class="ec-desc">${a.desc}</div>` : ''}
      <div class="field" data-f="code"><label>Code <span class="ct">python ${a.python || '3.12'}</span></label><div class="val mono">${a.code || ''}</div></div>
      <div class="field" data-f="inputs"><label>Inputs</label><div class="val" style="padding:8px">${tags(a.inputs)}</div></div>
      <div class="field" data-f="deps"><label>Dependencies <span class="ct" data-dc>${(a.deps || []).length}</span></label><div class="val" style="padding:8px"><div class="taglist">${(a.deps || []).map(d => `<span class="tag">${d}</span>`).join('')}</div></div></div>
      ${stField('env', 'Env status', 'env', a.env || 'pending')}`;

    if (a.kind === 'handler') return `
      ${a.desc ? `<div class="ec-desc">${a.desc}</div>` : ''}
      ${stField('runtime', 'Runtime', 'rt', a.runtime || 'stopped')}
      ${stField('config', 'Config state', 'cfg', a.configState || 'unconfigured', '改 config 触发重启')}
      <div class="field" data-f="init"><label>Init args</label><div class="val">${(a.initArgs || []).map(x => `<div class="ec-kv"><span class="k">${x.name}${x.required ? ' *' : ''}</span><span class="v${x.sensitive ? ' ec-mask' : ''}">${x.sensitive ? '********' : (x.value || x.default || '—')}</span></div>`).join('') || '<span class="ec-mask">— 无 —</span>'}</div></div>
      <div class="field" data-f="methods"><label>Methods <span class="ct">catalog members</span></label><div class="val" style="padding:8px">${tags((a.methods || []).map(m => m + '()'))}</div></div>
      <div class="field" data-f="class"><label>Assembled class</label><div class="val mono">${a.classCode || ''}</div></div>
      ${stField('env', 'Env status', 'env', a.env || 'pending')}`;

    if (a.kind === 'agent') return `
      ${a.desc ? `<div class="ec-desc">${a.desc}</div>` : ''}
      <div class="field" data-f="system"><label>System prompt</label><div class="val">${a.system || ''}</div></div>
      <div class="field" data-f="model"><label>Model</label><div class="val mono">${a.model || ''}</div></div>
      <div class="field" data-f="tools"><label>Mounted tools <span class="ct" data-tc>${(a.tools || []).length}</span></label><div class="val" style="padding:4px"><div class="ec-rows">${(a.tools || []).map(t => `<div class="ec-row"><span class="mh-dot ${t.health || 'ok'}"></span><span class="nid">${t.ref || t}</span></div>`).join('')}</div></div></div>
      <div class="field" data-f="skill"><label>Skill <span class="ct">0–1</span></label><div class="val">${a.skill || '<span class="ec-mask">— 未挂 —</span>'}</div></div>
      <div class="field" data-f="knowledge"><label>Knowledge</label><div class="val" style="padding:8px">${tags(a.knowledge)}</div></div>`;

    if (a.kind === 'workflow') return `
      ${a.desc ? `<div class="ec-desc">${a.desc}</div>` : ''}
      <div class="field" data-f="graph"><label>Graph <span class="ct" data-nc>${(a.nodes || []).length} 节点</span></label><div class="val" style="padding:4px"><div class="ec-rows">${(a.nodes || []).map(nodeRow).join('')}</div></div></div>
      <div class="field" data-f="lifecycle"><label>Lifecycle</label><div style="padding:2px 0"><span class="lifecycle ${a.lifecycle || 'inactive'}" data-st>${a.lifecycle === 'active' ? '<span class="dot"></span>' : ''}${({ active: '运行中', draining: '收尾中', inactive: '未上线' })[a.lifecycle] || '未上线'}</span></div></div>
      <div class="field" data-f="concurrency"><label>Concurrency <span class="ct">5 值</span></label><div class="val mono">${a.concurrency || 'serial'}</div></div>
      ${a.attention ? `<div class="field" data-f="attn"><label>Attention</label><div class="val" style="color:var(--warn)">${a.attention}</div></div>` : ''}`;

    if (a.kind === 'control') return `
      ${a.desc ? `<div class="ec-desc">${a.desc}</div>` : ''}
      <div class="field" data-f="inputs"><label>Inputs</label><div class="val" style="padding:8px">${tags(a.inputs)}</div></div>
      <div class="field" data-f="branches"><label>Branches <span class="ct">first-true-wins</span></label><div class="val" style="padding:4px"><div class="ec-rows">${(a.branches || []).map(branchRow).join('')}</div></div></div>
      ${stField('validation', 'Validation', 'valid', a.validation || 'valid')}`;

    if (a.kind === 'approval') return `
      ${a.desc ? `<div class="ec-desc">${a.desc}</div>` : ''}
      <div class="field" data-f="template"><label>Template <span class="ct">markdown + {{CEL}}</span></label><div class="val" style="white-space:pre-wrap;line-height:1.6">${celHi(a.template)}</div></div>
      <div class="field" data-f="rules"><label>Decision rules</label><div class="val">${kv([{ k: 'allowReason', v: a.allowReason ? '是' : '否' }, { k: 'timeout', v: a.timeout || '永不超时' }, { k: 'behavior', v: a.behavior || 'reject' }])}</div></div>
      ${stField('validity', 'Form validity', 'formval', a.validity || 'valid')}`;

    if (a.kind === 'trigger') return `
      ${a.desc ? `<div class="ec-desc">${a.desc}</div>` : ''}
      <div class="field" data-f="source"><label>Source</label><div style="padding:2px 0"><span class="src-badge"><span class="ico">${icon(SRCICON[a.source] || 'trigger', 13)}</span>${a.source || 'webhook'}</span></div></div>
      <div class="field" data-f="config"><label>Config</label><div class="val">${kv(a.config || [])}</div></div>
      <div class="field" data-f="dedup"><label>Dedup key</label><div class="val mono" style="font-size:11.5px">${a.dedup || '—'}</div></div>
      <div class="field" data-f="listeners"><label>Listeners <span class="ct" data-lc>refCount ${(a.listeners || []).length}</span></label><div class="val" style="padding:4px"><div class="ec-rows">${(a.listeners || []).map(l => `<div class="ec-row"><span class="nico">${icon('workflow', 15)}</span><span class="nid">${l}</span></div>`).join('') || '<div class="ec-mask" style="padding:6px 8px">未上线 · 无监听</div>'}</div></div></div>`;

    if (a.kind === 'mcp') return `
      ${a.desc ? `<div class="ec-desc">${a.desc}</div>` : ''}
      ${stField('conn', 'Connection', 'conn', a.conn || 'connecting')}
      <div class="field" data-f="transport"><label>Transport <span class="ct">${a.transport || 'stdio'}</span></label><div class="val">${kv(a.transportCfg || [])}</div></div>
      <div class="field" data-f="secrets"><label>Secrets</label><div class="val">${kv((a.secrets || []).map(s => ({ k: s, mask: true })))}</div></div>
      <div class="field" data-f="tools"><label>Tools <span class="ct" data-tc>${(a.tools || []).length}</span></label><div class="val" style="padding:8px"><div class="taglist">${(a.tools || []).map(t => `<span class="tag">${t}</span>`).join('') || '<span class="ec-mask">连接后发现</span>'}</div></div></div>`;

    if (a.kind === 'document') return `
      ${a.desc ? `<div class="ec-desc">${a.desc}</div>` : ''}
      <div class="field" data-f="body"><label>Body <span class="ct" data-sz>${a.size || '0 B'}</span></label><div class="val" style="white-space:pre-wrap;line-height:1.6;font-size:12.5px">${wikiHi(a.body)}</div></div>
      <div class="field" data-f="wikilinks"><label>Wikilinks</label><div class="val" style="padding:8px">${tags(a.wikilinks)}</div></div>
      <div class="field" data-f="tags"><label>Tags</label><div class="val" style="padding:8px">${tags(a.tags)}</div></div>
      <div class="field" data-f="mounted"><label>Mounted as knowledge</label><div class="val">${(a.mountedAs || []).map(m => `<div class="ec-kv"><span class="k">${icon('agent', 13)} ${m}</span></div>`).join('') || '<span class="ec-mask">— 未挂 —</span>'}</div></div>`;

    return '';
  }

  function render(a) {
    ensure();
    el.classList.add('show');
    const k = KIND[a.kind] || KIND.function;
    const tabs = TABS[a.kind] || TABS._;
    el.innerHTML = `
      <div class="aside-head">
        <span class="etype">${icon(k.icon, 17)}</span>
        <span class="ht"><b>${a.name}</b><span class="sub">${headSub(a)}</span></span>
        <span data-live>${liveBadge(a.live)}</span>
        <button class="ibtn" data-close>${icon('close', 16)}</button>
      </div>
      <div class="aside-tabs">${tabs.map((t, i) => `<button class="${i === 0 ? 'on' : ''}">${t}</button>`).join('')}</div>
      <div class="aside-body">${body(a)}</div>
      <div class="ec-foot">${foot(a)}</div>`;
    $('[data-close]', el).onclick = hide;
    el.querySelectorAll('.aside-tabs button').forEach(b => b.onclick = () => {
      el.querySelectorAll('.aside-tabs button').forEach(x => x.classList.remove('on')); b.classList.add('on');
    });
    return el;
  }

  // ===== 锻造收尾 + 状态推进（统一 setSt；命名别名供编排可读调用） =====
  function setLive(v) {
    const s = el && $('[data-live]', el); if (!s) return;
    s.innerHTML = v === false || v == null ? '<span class="live" style="opacity:.65">已保存</span>' : liveBadge(v);
  }
  function setVersion(v) { const e = el && $('[data-ver]', el); if (e) e.textContent = 'v' + v; }
  function setSt(dataF, mapKey, state) {
    const e = el && $(`[data-f="${dataF}"] [data-st]`, el); if (!e) return;
    const m = MAPS[mapKey]; const [c, t] = m[state] || Object.values(m)[0];
    e.className = `statebadge ${c}`; e.innerHTML = `<span class="dot"></span>${t}`;
  }
  const setEnv = s => setSt('env', 'env', s);
  const setRuntime = s => setSt('runtime', 'rt', s);
  const setConfig = s => setSt('config', 'cfg', s);
  const setValidation = s => setSt('validation', 'valid', s);
  const setValidity = s => setSt('validity', 'formval', s);
  const setConn = s => setSt('conn', 'conn', s);
  function setLifecycle(s) {
    const e = el && $('[data-f="lifecycle"] [data-st]', el); if (!e) return;
    e.className = `lifecycle ${s}`; e.innerHTML = `${s === 'active' ? '<span class="dot"></span>' : ''}${({ active: '运行中', draining: '收尾中', inactive: '未上线' })[s] || '未上线'}`;
  }
  // 揭示掩码（handler/mcp 填密钥后）：找到该 data-f 下首个 ec-mask 改值
  function reveal(dataF, key) {
    const e = el && $(`[data-f="${dataF}"]`, el); if (!e) return;
    e.querySelectorAll('.ec-kv').forEach(row => { if (row.querySelector('.k')?.textContent.includes(key)) { const v = row.querySelector('.v'); v.classList.remove('ec-mask'); v.classList.add('flash'); } });
  }

  const show = () => { ensure().classList.add('show'); };
  const hide = () => { if (el) el.classList.remove('show'); };
  const toggle = () => { ensure().classList.toggle('show'); };

  return { ensure, render, setLive, setVersion, setSt, setEnv, setRuntime, setConfig, setValidation, setValidity, setConn, setLifecycle, reveal, show, hide, toggle, get el() { return el; }, $ };
})();

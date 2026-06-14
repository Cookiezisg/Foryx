/* Forgify design-lab — 对话海洋 · 右岛块（实体卡，四类全能）。
   右岛是「本海洋的」：自己 append 到 Shell.body（作第三个 flex 子），自管显隐与流式编辑。
   信号交互 = 锻造工具（create_/edit_*）的流式 args 经 entities 流 forge 镜像 → 这里实时填充。
   一套机制 / 四种类型：function / handler / agent / workflow 只换 etype 与字段集。
   依赖：shared/icons.js（icon）+ shared/shell.js（Shell.body）。样式在同目录 chat.css。 */
window.ChatEntityCard = (function () {
  const $ = (s, r) => r.querySelector(s);
  let el = null;

  const KIND = {
    function: { icon: 'function', label: 'Function' },
    handler:  { icon: 'handler',  label: 'Handler'  },
    agent:    { icon: 'agent',    label: 'Agent'    },
    workflow: { icon: 'workflow', label: 'Workflow' },
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

  // env 物化状态镜像（读 Version 行、非查 sandbox）：pending→syncing→ready/failed
  const ENV = { pending: ['pending', '排队'], syncing: ['syncing', '物化中…'], ready: ['ready', '就绪'], failed: ['failed', '失败'] };
  const envBadge = (s = 'pending') => { const [c, t] = ENV[s] || ENV.pending; return `<span class="statebadge ${c}" data-st><span class="dot"></span>${t}</span>`; };
  // handler config 状态：unconfigured / partially_configured / ready（独立于 env，二者并存两行）
  const CFG = { unconfigured: ['pending', '未配置'], partially_configured: ['syncing', '部分配置'], ready: ['ready', '就绪'] };
  const cfgBadge = (s = 'unconfigured') => { const [c, t] = CFG[s] || CFG.unconfigured; return `<span class="statebadge ${c}"><span class="dot"></span>${t}</span>`; };
  const LIFE = { active: '运行中', draining: '收尾中', inactive: '未上线' };
  const tags = (arr, empty = '— 无 —') => (arr && arr.length) ? `<div class="taglist">${arr.map(t => `<span class="tag">${t}</span>`).join('')}</div>` : `<span class="ec-mask">${empty}</span>`;
  // 图节点 5 类 → 图标 key（approval 复用 shield；其余 kind 即 key）
  const NICON = { trigger: 'trigger', action: 'action', agent: 'agent', control: 'control', approval: 'shield' };
  const nodeRow = n => `<div class="ec-row"><span class="nico">${icon(NICON[n.kind] || n.kind, 15)}</span><span class="nid">${n.id}</span><span class="nref">${n.ref}</span></div>`;

  function body(a) {
    if (a.kind === 'function') return `
      ${a.desc ? `<div class="ec-desc">${a.desc}</div>` : ''}
      <div class="field" data-f="code"><label>Code <span class="ct">python ${a.python || '3.12'}</span></label><div class="val mono">${a.code || ''}</div></div>
      <div class="field" data-f="inputs"><label>Inputs</label><div class="val" style="padding:8px">${tags(a.inputs)}</div></div>
      <div class="field" data-f="deps"><label>Dependencies <span class="ct" data-dc>${(a.deps || []).length}</span></label><div class="val" style="padding:8px"><div class="taglist">${(a.deps || []).map(d => `<span class="tag">${d}</span>`).join('')}</div></div></div>
      <div class="field" data-f="env"><label>Env status</label><div style="padding:2px 0">${envBadge(a.env)}</div></div>`;

    if (a.kind === 'handler') return `
      ${a.desc ? `<div class="ec-desc">${a.desc}</div>` : ''}
      <div class="field" data-f="class"><label>Assembled class</label><div class="val mono">${a.classCode || ''}</div></div>
      <div class="field" data-f="methods"><label>Methods <span class="ct">catalog</span></label><div class="val" style="padding:8px">${tags((a.methods || []).map(m => m + '()'))}</div></div>
      <div class="field" data-f="init"><label>Init args</label><div class="val">${(a.initArgs || []).map(x => `<div class="ec-kv"><span class="k">${x.name}</span><span class="v">${x.sensitive ? '<span class="ec-mask">********</span>' : (x.value || '')}</span></div>`).join('') || '<span class="ec-mask">— 无 —</span>'}</div></div>
      <div class="field" data-f="config"><label>Config state</label><div style="padding:2px 0">${cfgBadge(a.configState)} <span class="ec-note">改 config 触发重启</span></div></div>
      <div class="field" data-f="env"><label>Env status</label><div style="padding:2px 0">${envBadge(a.env)}</div></div>`;

    if (a.kind === 'agent') return `
      ${a.desc ? `<div class="ec-desc">${a.desc}</div>` : ''}
      <div class="field" data-f="system"><label>System prompt</label><div class="val">${a.system || ''}</div></div>
      <div class="field" data-f="model"><label>Model</label><div class="val mono">${a.model || ''}</div></div>
      <div class="field" data-f="tools"><label>Mounted tools <span class="ct" data-tc>${(a.tools || []).length}</span></label><div class="val" style="padding:8px"><div class="taglist">${(a.tools || []).map(t => `<span class="tag">${t}</span>`).join('')}</div></div></div>
      <div class="field" data-f="skill"><label>Skill <span class="ct">0–1</span></label><div class="val">${a.skill || '<span class="ec-mask">— 未挂 —</span>'}</div></div>
      <div class="field" data-f="knowledge"><label>Knowledge</label><div class="val" style="padding:8px">${tags(a.knowledge)}</div></div>`;

    if (a.kind === 'workflow') return `
      ${a.desc ? `<div class="ec-desc">${a.desc}</div>` : ''}
      <div class="field" data-f="graph"><label>Graph <span class="ct" data-nc>${(a.nodes || []).length} 节点</span></label><div class="val" style="padding:4px"><div class="ec-rows">${(a.nodes || []).map(nodeRow).join('')}</div></div></div>
      <div class="field" data-f="lifecycle"><label>Lifecycle</label><div style="padding:2px 0"><span class="lifecycle ${a.lifecycle || 'inactive'}">${a.lifecycle === 'active' ? '<span class="dot"></span>' : ''}${LIFE[a.lifecycle] || LIFE.inactive}</span></div></div>
      <div class="field" data-f="concurrency"><label>Concurrency <span class="ct">5 值</span></label><div class="val mono">${a.concurrency || 'serial'}</div></div>
      ${a.attention ? `<div class="field" data-f="attn"><label>Attention</label><div class="val" style="color:var(--warn)">${a.attention}</div></div>` : ''}`;

    return '';
  }

  function render(a) {
    ensure();
    el.classList.add('show');
    const k = KIND[a.kind] || KIND.function;
    const sub = [`${k.label} · <span class="ver" data-ver>v${a.version}</span>`];
    if (a.kind === 'handler') sub.push('常驻进程');
    el.innerHTML = `
      <div class="aside-head">
        <span class="etype">${icon(k.icon, 17)}</span>
        <span class="ht"><b>${a.name}</b><span class="sub">${sub.join(' · ')}</span></span>
        <span data-live>${liveBadge(a.live)}</span>
        <button class="ibtn" data-close>${icon('close', 16)}</button>
      </div>
      <div class="aside-tabs"><button class="on">概览</button><button>版本</button><button>运行</button><button>迭代</button></div>
      <div class="aside-body">${body(a)}</div>
      <div class="ec-foot"><span class="mono">${a.id || ''}</span>${a.runs != null ? ` · ${a.runs} runs` : ''}<span class="gap"></span><span class="act">历史版本</span></div>`;
    $('[data-close]', el).onclick = hide;
    el.querySelectorAll('.aside-tabs button').forEach(b => b.onclick = () => {
      el.querySelectorAll('.aside-tabs button').forEach(x => x.classList.remove('on')); b.classList.add('on');
    });
    return el;
  }

  // 锻造收尾：编辑中 → 已保存 · vN
  function setLive(v) {
    const s = el && $('[data-live]', el); if (!s) return;
    s.innerHTML = v === false || v == null
      ? '<span class="live" style="opacity:.65">已保存</span>'
      : liveBadge(v);
  }
  function setVersion(v) { const e = el && $('[data-ver]', el); if (e) e.textContent = 'v' + v; }
  // env 状态推进（pending→syncing→ready/failed）
  function setEnv(s) { const e = el && $('[data-f="env"] [data-st]', el); if (!e) return; const [c, t] = ENV[s] || ENV.pending; e.className = `statebadge ${c}`; e.innerHTML = `<span class="dot"></span>${t}`; }

  const show = () => { ensure().classList.add('show'); };
  const hide = () => { if (el) el.classList.remove('show'); };
  const toggle = () => { ensure().classList.toggle('show'); };

  return { ensure, render, setLive, setVersion, setEnv, show, hide, toggle, get el() { return el; }, $ };
})();

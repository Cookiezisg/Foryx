/* Forgify design-lab — 对话海洋编排（产品适配 v1；单独，一人负责整个 oceans/chat/ 文件夹）。
   注册进外壳：Shell.registerOcean('chat', { crumb, build(sea) })，渲染对话流 + composer 到 #sea；右岛(实体卡)交给同目录 entity-card.js。
   一条 teaching-spine 回合演 Forgify 真实对话引擎：说需求 → 实体诞生(锻造→右岛实时填充) → 跑通(progress 块) →
   求许可(人在环危险闸) → 接线(subagent 子树 + 锻造 workflow) → 看 durable run(flowrun 逐节点) → 诚实终止(max_steps)。
   依赖：shared/icons.js · shared/shell.js · ./entity-card.js（ChatEntityCard）。 */
(function () {
  const $ = (s, r = document) => r.querySelector(s);
  const el = (t, c) => { const e = document.createElement(t); if (c) e.className = c; return e; };
  const sleep = ms => new Promise(r => setTimeout(r, ms));
  let runId = 0;
  const alive = id => id === runId;
  const conv = () => $('#conv'), col = () => $('#col');
  const toBottom = () => { const c = conv(); if (c) c.scrollTop = c.scrollHeight; };
  const refPill = (kind, label, ent) => `<span class="ref-pill"${ent ? ` data-ent="${ent}"` : ''}><span class="ico">${icon(kind, 12)}</span>${label}</span>`;
  const setGen = on => { const c = $('#composer'); if (c) c.classList.toggle('gen', on); };
  // hold 开关（localStorage 'fg.chat.hold'='1'）：开启时人在环审批 / flowrun 决策等你点击、不自动放行（默认自动播放）
  const HOLD = () => localStorage.getItem('fg.chat.hold') === '1';
  // 图节点 5 类 → 图标 key（approval 复用 shield；其余 kind 即 key）
  const NICON = { trigger: 'trigger', action: 'action', agent: 'agent', control: 'control', approval: 'shield' };

  // —— 本回合锻造/触达的实体（点对话里的药丸即重新唤出对应卡：原型「per-tool-call anchor」）——
  const ENT = {
    weekly_digest: { kind: 'function', name: 'weekly_digest', version: 1, id: 'fn_7f3c2a91b04e8d52', runs: 1,
      desc: '抓取竞品 RSS / 变更，归并去重，产出结构化摘要列表。', python: '3.12',
      code: 'def weekly_digest(sources, since):\n    items = []\n    for url in sources:\n        items += fetch(url, since)\n    return summarize(dedupe(items))',
      inputs: ['sources: list[str]', 'since: str'], deps: ['feedparser', 'httpx', 'beautifulsoup4'], env: 'ready' },
    notion_writer: { kind: 'handler', name: 'notion_writer', version: 3, id: 'hd_5a2e9b13c7f04d68', runs: 42,
      desc: '常驻的 Notion 写手：保活连接，跨调用复用 client（真共享状态）。',
      classCode: 'class NotionWriter:\n    def __init__(self, api_key, db_default):\n        self.client = Notion(api_key)\n    def publish(self, database, title, blocks): ...\n    def shutdown(self): self.client.close()',
      methods: ['publish', 'append', 'search'], initArgs: [{ name: 'api_key', sensitive: true }, { name: 'db_default', value: '竞品追踪' }],
      configState: 'ready', env: 'ready' },
    polish: { kind: 'agent', name: '摘要润色', version: 2, id: 'ag_8d1f4c20a9e3b7f5', runs: 7,
      desc: '把抓取到的原始条目润色成中文要点，去营销腔、附来源。',
      system: '你是严谨的竞品分析助手。把条目压成 3–5 条中文要点，每条附来源链接，不夸大。',
      model: 'claude-opus-4-8', tools: ['fn_weekly_digest', 'hd_notion_writer.search', 'mcp:web/fetch'], skill: null, knowledge: ['竞品列表.md'] },
    weekly_report: { kind: 'workflow', name: 'weekly_report', version: 1, id: 'wf_2b91ac7740e8d3f1', runs: 1,
      desc: '每天 08:00 抓取 → 润色 → 人工过目 → 发布 的编排图。', lifecycle: 'inactive', concurrency: 'serial',
      nodes: [{ kind: 'trigger', id: 'daily', ref: 'trg_ cron 08:00' }, { kind: 'action', id: 'fetch', ref: 'fn_weekly_digest' },
        { kind: 'agent', id: 'polish', ref: 'ag_摘要润色' }, { kind: 'approval', id: 'review', ref: 'apf_过目' }, { kind: 'action', id: 'publish', ref: 'hd_notion_writer.publish' }] },
  };
  const openEntity = key => { const e = ENT[key]; if (e) { ChatEntityCard.render(e); ChatEntityCard.setLive(false); } };

  Shell.registerOcean('chat', {
    crumb: '每日竞品摘要',
    build(sea) {
      sea.innerHTML = `
        <div class="conv" id="conv"><div class="col" id="col"></div></div>
        <div class="todo-dock" id="todoDock"></div>
        <div class="composer" id="composer">
          <div class="cwrap">
            <div class="mention-pop" id="mpop">
              <div class="mh">提及一个实体（freeze-on-send · 发送时快照内容）</div>
              <div class="mention-row"><span class="ico">${icon('function', 16)}</span><span class="nm">weekly_digest</span><span class="kd">fn_</span></div>
              <div class="mention-row"><span class="ico">${icon('handler', 16)}</span><span class="nm">notion_writer</span><span class="kd">hd_</span></div>
              <div class="mention-row"><span class="ico">${icon('agent', 16)}</span><span class="nm">摘要润色</span><span class="kd">ag_</span></div>
              <div class="mention-row"><span class="ico">${icon('workflow', 16)}</span><span class="nm">weekly_report</span><span class="kd">wf_</span></div>
              <div class="mention-row"><span class="ico">${icon('doc', 16)}</span><span class="nm">竞品列表.md</span><span class="kd">doc</span></div>
            </div>
            <div class="box">
              <div class="field"><input id="ta" placeholder=""><span class="enter" data-i="enter"></span></div>
              <div class="bar">
                <button class="cbtn ic" id="b_at" title="@ 提及实体">${icon('at', 17)}</button>
                <button class="cbtn ic" id="b_plus" title="附件">${icon('plus', 17)}</button>
                <span class="right">
                  <button class="cbtn stop" id="b_stop">${icon('stop', 13)} 停止</button>
                  <span class="spin">${icon('spin', 14)}</span>
                  <button class="cbtn model"><b>claude-opus-4-8</b> ${icon('chevd', 13)}</button>
                </span>
              </div>
            </div>
          </div>
        </div>`;
      sea.querySelectorAll('[data-i]').forEach(e => e.innerHTML = icon(e.dataset.i, 16));

      // 主区头：本海洋按钮（重播 teaching-spine + 右岛面板切换）
      Shell.headExtra(`<button class="ibtn" id="i_replay" title="重播本回合">${icon('play', 16)}</button><button class="ibtn" id="i_panel" title="右岛">${icon('panel')}</button>`);
      $('#i_replay').onclick = run;
      $('#i_panel').onclick = () => ChatEntityCard.toggle();
      $('.enter', sea).onclick = run;
      $('#b_at').onclick = () => $('#mpop').classList.toggle('show');
      sea.querySelectorAll('.mention-row').forEach(r => r.onclick = () => $('#mpop').classList.remove('show'));
      $('#b_stop').onclick = () => { runId++; setGen(false); };   // 停止 = POST :cancel（中断当前回合）

      // 对话标题 → 主区头左上角 head-lead（与右上角控件对称的角落块；不跟会话列对齐）。标题 + 向下箭头快捷操作
      Shell.headLead.querySelectorAll('[data-ocean-head]').forEach(e => e.remove());   // 自清：重挂不重复
      const titlebar = el('span', 'chat-titlebar'); titlebar.setAttribute('data-ocean-head', 'chat');
      titlebar.innerHTML = `<button class="chat-title"><span class="tt">每日竞品摘要</span><span class="chev">${icon('chevd', 12)}</span></button>
        <div class="qa-menu"><div class="qa-item" data-a="rename">重命名</div><div class="qa-item" data-a="pin">置顶</div><div class="qa-item" data-a="archive">归档</div><div class="qa-item danger" data-a="delete">删除对话</div></div>`;
      Shell.headLead.appendChild(titlebar);
      const titleBtn = $('.chat-title', titlebar), qa = $('.qa-menu', titlebar);
      titleBtn.onclick = e => { e.stopPropagation(); const open = qa.classList.toggle('show'); titleBtn.classList.toggle('open', open); };
      document.addEventListener('click', () => { qa.classList.remove('show'); titleBtn.classList.remove('open'); });
      qa.querySelectorAll('.qa-item').forEach(it => it.onclick = e => {
        e.stopPropagation(); qa.classList.remove('show'); titleBtn.classList.remove('open');
        if (it.dataset.a !== 'rename') return;
        const tt = $('.tt', titleBtn), cur = tt.textContent;
        const inp = el('input', 'chat-title-edit'); inp.value = cur;
        ['click', 'mousedown'].forEach(ev => inp.addEventListener(ev, x => x.stopPropagation()));   // 编辑时别触发块的开关
        tt.replaceWith(inp); inp.focus(); inp.select();
        const fin = () => { const s = el('span', 'tt'); s.textContent = inp.value.trim() || cur; inp.replaceWith(s); };
        inp.onblur = fin; inp.onkeydown = ev => { if (ev.key === 'Enter') inp.blur(); else if (ev.key === 'Escape') { inp.value = cur; inp.blur(); } };
      });

      dock = buildDock();
      run();
    },
  });

  // —— 复用 v0 的对话原语 ——
  function typeInto(node, text, cps = 60) {
    const id = runId;
    return new Promise(res => {
      const caret = el('span', 'caret'); node.appendChild(caret);
      let i = 0;
      (function step() {
        if (!alive(id)) { caret.remove(); return res(); }
        caret.insertAdjacentText('beforebegin', text[i++] ?? '');
        toBottom();
        if (i > text.length) { caret.remove(); return res(); }
        setTimeout(step, 1000 / cps + Math.random() * 14);
      })();
    });
  }
  function userMsg(html) { const m = el('div', 'umsg'); m.innerHTML = `<div class="b">${html}</div>`; col().appendChild(m); toBottom(); }
  function aiTurn() { const t = el('div', 'turn'); t.innerHTML = `<div class="spark">${icon('spark', 16, 1.6)}</div><div class="amsg"></div>`; col().appendChild(t); toBottom(); return $('.amsg', t); }
  function para(b) { const p = el('p'); b.appendChild(p); return p; }

  // 工具调用组（复用 v0 视觉：运行流光 → 收敛 + 外框工具列表）
  function toolGroup(body) {
    const w = el('div', 'tg');
    w.innerHTML = `<div class="tg-sum run"><span class="tk"></span><span class="chev" style="display:none">${icon('chevr', 14)}</span></div>
      <div class="tg-list"><div class="w"><div class="tg-box"></div></div></div>`;
    body.appendChild(w); toBottom();
    const sum = $('.tg-sum', w), tk = $('.tk', w), chev = $('.chev', w), box = $('.tg-box', w);
    sum.onclick = () => { if (box.children.length) w.classList.toggle('open'); };
    return {
      el: w, box,
      status(t) { tk.textContent = t; toBottom(); },
      open() { w.classList.add('open'); toBottom(); },
      settle(s) { sum.classList.remove('run'); tk.textContent = s; chev.style.display = ''; toBottom(); },   // 默认折叠：只留摘要行，点击展开
    };
  }
  // 工具项（一行；danger 仅 cautious/dangerous 显徽章——safe 不显）
  function toolItem(box, o) {
    const ti = el('div', 'ti' + (o.open ? ' open' : ''));
    const badge = (o.danger && o.danger !== 'safe') ? ` <span class="badge ${o.danger}">${o.danger}</span>` : '';
    ti.innerHTML = `<div class="ti-sum"><span class="v">${o.verb || 'Used'}</span> <span class="nm">${o.name}</span>${badge}<span class="chev">${icon('chevr', 14)}</span></div>
      <div class="ti-det"><div class="w">${o.detailHTML || ''}</div></div>`;
    box.appendChild(ti);
    $('.ti-sum', ti).onclick = () => ti.classList.toggle('open');
    return ti;
  }
  // reasoning 块（默认折叠）
  function reasonBlock(body, text) {
    const r = el('div', 'reason');
    r.innerHTML = `<div class="reason-sum"><span class="chev">${icon('chevr', 13)}</span>思考</div>
      <div class="reason-body"><div class="w"><div class="tbox"><div class="out">${text}</div></div></div></div>`;
    body.appendChild(r); $('.reason-sum', r).onclick = () => r.classList.toggle('open'); toBottom(); return r;
  }
  // progress 块（六型之一：run_function/call_handler 下的实时 stderr/yield，与 result 分两框）
  function progressBox(parentW) {
    const pb = el('div', 'progress-box run');
    pb.innerHTML = `<div class="phead"><span>进度 · stderr</span><span class="lt"><span class="dot"></span>实时</span></div><div class="plines"></div>`;
    parentW.appendChild(pb); toBottom();
    const lines = $('.plines', pb);
    return { el: pb, add(t) { lines.textContent += (lines.textContent ? '\n' : '') + t; toBottom(); }, done() { pb.classList.remove('run'); pb.classList.add('done'); } };
  }
  function resultBox(parentW, json) {
    const cap = el('div', 'res-cap'); cap.textContent = '返回结果（单一 JSON）'; parentW.appendChild(cap);
    const b = el('div', 'tbox'); b.innerHTML = `<div class="out">${json}</div>`; parentW.appendChild(b); toBottom();
  }
  // todo 自管清单（整表替换、只读）→ 底部常驻进度坞（show 一次、随回合 set 更新进展；点头折叠）
  let dock = null;
  function buildDock() {
    const d = $('#todoDock'); if (!d) return null;
    d.innerHTML = `<div class="td-wrap"><div class="td-head"><span class="tt">Todo</span><span class="prog" data-prog></span><span class="cur" data-cur></span><span class="chev">${icon('chevr', 13)}</span></div><div class="td-body"><div class="w"><div data-rows></div></div></div></div>`;
    $('.td-head', d).onclick = () => d.classList.toggle('collapsed');
    return {
      show() { d.classList.add('show'); },
      hide() { d.classList.remove('show', 'collapsed'); },
      set(items) {
        const rows = $('[data-rows]', d); if (!rows) return;
        rows.innerHTML = items.map(([t, st]) => `<div class="todo-row ${st}"><span class="mk">${st === 'completed' ? icon('check', 13) : '<span class="circle"></span>'}</span><span class="t">${t}</span></div>`).join('');
        const done = items.filter(i => i[1] === 'completed').length;
        $('[data-prog]', d).textContent = `${done}/${items.length}`;
        const cur = items.find(i => i[1] === 'in-progress');
        $('[data-cur]', d).textContent = cur ? '· ' + cur[0] : (done === items.length ? '· 全部完成' : '');
      },
    };
  }
  // 人在环危险闸：内联审批卡（危险工具 / ask_user 两味）
  function approvalCard(body, o) {
    const c = el('div', 'approval-card' + (o.flavor === 'ask' ? ' ask' : ''));
    const ask = o.flavor === 'ask';
    c.innerHTML = `
      <span class="ac-pulse"></span>
      <div class="ac-head"><span class="ac-shield">${icon('shield', 16)}</span>
        <span class="ac-tt"><b>${ask ? '需要你的输入' : '需要你批准'}</b><span class="tool">${o.tool}</span></span>
        ${ask ? '' : `<span class="badge ${o.danger || 'dangerous'}">${o.danger || 'dangerous'}</span>`}</div>
      <div class="ac-body">
        <div class="ac-sum">${o.summary || ''}</div>
        ${ask ? `<input class="ac-answer" placeholder="${o.placeholder || '输入你的回答…'}">` : `<div class="ac-args">${o.args || ''}</div>`}
        <div class="ac-actions">${ask
          ? `<button class="acbtn primary" data-act="accept">${icon('check', 15)} 接受</button><button class="acbtn deny" data-act="decline">拒绝</button>`
          : `<button class="acbtn primary" data-act="approve">${icon('check', 15)} 批准</button><button class="acbtn ghost" data-act="approve_always">始终批准</button><span class="ac-note">本会话内预授权</span><button class="acbtn deny" data-act="deny">拒绝</button>`}</div>
      </div>
      <div class="ac-settled"><span class="ico">${icon('check', 15)}</span><span data-settled></span></div>`;
    body.appendChild(c); toBottom();
    return {
      el: c,
      settle(t) { $('[data-settled]', c).textContent = t; c.classList.add('settled'); toBottom(); },
      // 等用户决议；autoAct/ms 用于自动播放（模拟用户点选）
      wait(autoAct, ms) {
        return new Promise(res => {
          let done = false; const fin = (act, ans) => { if (done) return; done = true; res({ act, ans }); };
          c.querySelectorAll('[data-act]').forEach(b => b.onclick = () => fin(b.dataset.act, $('.ac-answer', c) && $('.ac-answer', c).value));
          if (autoAct) setTimeout(() => fin(autoAct), ms || 1600);
        });
      },
    };
  }
  // durable flowrun 条（逐节点 tick；行是真相、ephemeral tick 不入 replay 环）
  function flowrunStrip(body, frid) {
    const f = el('div', 'flowrun');
    f.innerHTML = `<div class="fr-head"><span class="fl">flowrun</span><span class="fid">${frid}</span><span class="pulse" data-pulse></span></div>
      <div data-nodes></div><div class="fr-foot">${refPill('search', 'get_flowrun ▸')}</div>`;
    body.appendChild(f); toBottom();
    const nodes = $('[data-nodes]', f);
    function addNode(kind, id) {
      const r = el('div', 'fr-node enter pending');
      r.innerHTML = `<span class="nico">${icon(NICON[kind] || kind, 15)}</span><span class="nid">${id}</span><span class="nkind">${kind}</span><span class="nstatus">排队</span>`;
      nodes.appendChild(r); toBottom();
      const st = () => $('.nstatus', r);
      return {
        el: r,
        running() { r.className = 'fr-node running'; st().innerHTML = '<span class="shimmer">运行中…</span>'; toBottom(); },
        ok(memo) { r.className = 'fr-node ok'; st().innerHTML = `${icon('check', 13)} ok${memo ? ' <span class="memo">已记忆化</span>' : ''}`; toBottom(); },
        park(autoMs) {
          r.className = 'fr-node parked';
          st().innerHTML = `<span class="fr-decide"><span class="lbl">等决策</span><button class="yes">通过</button><button class="no">驳回</button></span>`;
          toBottom();
          return new Promise(res => {
            let done = false; const fin = v => { if (done) return; done = true;
              st().innerHTML = v === 'yes' ? `${icon('check', 13)} 已通过` : '已驳回'; r.className = 'fr-node ' + (v === 'yes' ? 'ok' : 'parked'); res(v); };
            $('.yes', r).onclick = () => fin('yes'); $('.no', r).onclick = () => fin('no');
            if (autoMs) setTimeout(() => fin('yes'), autoMs);
          });
        },
      };
    }
    return { el: f, addNode, finish() { const p = $('[data-pulse]', f); if (p) p.classList.add('off'); } };
  }
  function subtree(body, label) {
    const s = el('div', 'subtree');
    s.innerHTML = `<div class="sublabel"><span class="ico">${icon('dispatch', 13)}</span>${label}</div>`;
    body.appendChild(s); toBottom(); return s;
  }
  function turnEnd(body, msg) {
    const t = el('div', 'turn-end');
    t.innerHTML = `<span class="te-ico">${icon('flag', 16)}</span><span class="te-msg">${msg}</span><span class="badge code">MAX_STEPS_REACHED</span><button class="cont">继续</button>`;
    body.appendChild(t); $('.cont', t).onclick = run; toBottom(); return t;
  }
  function compaction(body, n) { const c = el('div', 'compaction'); c.textContent = `· 上文已压缩 · earlier context summarized · seq ≤ ${n} ·`; body.appendChild(c); }

  // —— teaching-spine 编排 ——
  async function run() {
    const id = ++runId;
    col().innerHTML = ''; ChatEntityCard.hide(); if (ChatEntityCard.el) ChatEntityCard.el.innerHTML = '';
    setGen(true); if (dock) dock.hide();

    compaction(col(), 128);   // 第六型块：压缩标记（高处一处）

    // BEAT 1 — 用户消息（冻结 @提及）+ 计划 + todo 自管清单
    userMsg(`帮我搭一个每天早上自动汇总竞品动态、写进 Notion 的流程。背景看 ${refPill('doc', '竞品列表.md')}。`);
    await sleep(550); if (!alive(id)) return;
    const t = aiTurn();
    reasonBlock(t, '用户要一条「抓取→汇总→发布」的每日自动化。拆解：无状态抓取函数（function）+ 常驻 Notion 写手（handler，已有）+ 一个润色 LLM 员工（agent）+ 一张每天 08:00 触发的编排图（workflow）。先写待办、逐个锻造、跑通后编排。');
    await typeInto(para(t), '好的。我会先锻造一个抓取 + 汇总的函数，跑通后接上常驻的 Notion 写手，最后编排成每天触发的 workflow。先把任务拆出来：', 70);
    if (!alive(id)) return; await sleep(160);
    const tw = toolGroup(t); tw.status('todo_write…'); await sleep(620); if (!alive(id)) return;
    toolItem(tw.box, { name: 'todo_write', detailHTML: '<div class="tbox"><div class="out">整表替换写入 · 4 项 · LLM 自管、只读</div></div>' });
    tw.settle('已更新待办 · todo_write');
    if (dock) { dock.show(); dock.set([['抓取竞品动态并汇总（function）', 'in-progress'], ['接 Notion 写手（handler）', 'pending'], ['编排每日 workflow', 'pending'], ['上线 + 验证一次', 'pending']]); }
    await sleep(750); if (!alive(id)) return;

    // BEAT 2 — create_function → 右岛 function 卡实时填充（THE 签名交互，泛化到 Function）
    await typeInto(para(t), '开始锻造抓取函数。', 60); if (!alive(id)) return; await sleep(150);
    const cf = toolGroup(t); cf.status('Forging weekly_digest…'); await sleep(520); if (!alive(id)) return;
    ChatEntityCard.render({ kind: 'function', name: 'weekly_digest', version: 1, live: 'forge', id: 'fn_7f3c2a91b04e8d52',
      desc: '抓取竞品 RSS / 变更，归并去重，产出结构化摘要列表。', python: '3.12', code: '', inputs: ['sources: list[str]', 'since: str'], deps: [], env: 'pending' });
    const card = ChatEntityCard.el; await sleep(420); if (!alive(id)) return;
    await typeInto(ChatEntityCard.$('[data-f="code"] .val', card),
      'def weekly_digest(sources, since):\n    items = []\n    for url in sources:\n        items += fetch(url, since)\n    return summarize(dedupe(items))', 95);
    if (!alive(id)) return;
    ChatEntityCard.setEnv('syncing');
    for (const d of ['feedparser', 'httpx', 'beautifulsoup4']) {
      await sleep(300); if (!alive(id)) return;
      const tl = ChatEntityCard.$('[data-f="deps"] .taglist', card); const tag = el('span', 'tag new'); tag.textContent = d; tl.appendChild(tag);
      ChatEntityCard.$('[data-f="deps"] [data-dc]', card).textContent = tl.children.length;
    }
    await sleep(750); if (!alive(id)) return; ChatEntityCard.setEnv('ready'); ChatEntityCard.setLive(false);
    toolItem(cf.box, { verb: 'Forged', name: 'create_function(weekly_digest)',
      detailHTML: `<div class="diff-cap">实体版本 diff · v1（ops 锻造，非 git diff）</div><div class="tbox"><div class="diff">
        <div class="dline add"><span class="s">+</span><span class="c">def weekly_digest(sources, since): …</span></div>
        <div class="dline add"><span class="s">+</span><span class="c">deps += feedparser, httpx, beautifulsoup4</span></div>
        <div class="dline add"><span class="s">+</span><span class="c">env → ready</span></div></div></div>` });
    cf.settle('已锻造 · create_function');
    if (dock) dock.set([['抓取竞品动态并汇总（function）', 'completed'], ['接 Notion 写手（handler）', 'in-progress'], ['编排每日 workflow', 'pending'], ['上线 + 验证一次', 'pending']]);
    await sleep(550); if (!alive(id)) return;

    // BEAT 3 — run_function → progress 块（实时 stderr）+ 独立 result 框
    await typeInto(para(t), '跑一次看看输出。', 58); if (!alive(id)) return; await sleep(140);
    const rf = toolGroup(t); rf.status('run_function(weekly_digest)…');
    const ti = toolItem(rf.box, { name: 'run_function(weekly_digest)' });
    const det = $('.ti-det > .w', ti);
    const pb = progressBox(det);
    for (const ln of ['fetch arxiv.org/list … 12 条', 'fetch openai.com/blog … 3 条', 'fetch anthropic.com/news … 5 条', 'dedupe → 17 条', 'summarize via dialogue model …'])
      { await sleep(370); if (!alive(id)) return; pb.add(ln); }
    await sleep(420); if (!alive(id)) return; pb.done();
    resultBox(det, '{ "count": 17, "items": [ { "title": "…", "url": "…" }, … ] }');
    rf.settle('运行完成 · run_function'); await sleep(560); if (!alive(id)) return;

    // BEAT 4 — call_handler 危险 → 人在环审批卡（approve_always）
    await typeInto(para(t), '现在把摘要发到 Notion。这一步会写到外部空间，需要你确认。', 64); if (!alive(id)) return; await sleep(150);
    const ch = toolGroup(t); ch.status('call_handler(notion_writer.publish)…'); await sleep(560); if (!alive(id)) return;
    const ap = approvalCard(t, { flavor: 'danger', tool: 'notion_writer.publish', danger: 'dangerous',
      summary: '把今天的竞品摘要发布到 Notion「竞品追踪」数据库（新增 1 页、写 17 个块）。',
      args: '{\n  "database": "竞品追踪",\n  "title": "竞品摘要 · 06-14",\n  "blocks": 17\n}' });
    const { act } = await ap.wait(HOLD() ? null : 'approve_always', 1900); if (!alive(id)) return;
    ap.settle(act === 'deny' ? '已拒绝 · 反馈给模型' : '已批准 · 本会话内始终允许');
    await sleep(250); if (!alive(id)) return;
    if (act !== 'deny') {
      const ti2 = toolItem(ch.box, { name: 'call_handler(notion_writer.publish)', danger: 'dangerous' });
      const det2 = $('.ti-det > .w', ti2); const pb2 = progressBox(det2);
      for (const ln of ['connect notion … ok', 'create page 竞品摘要 · 06-14', 'append 17 blocks … done'])
        { await sleep(350); if (!alive(id)) return; pb2.add(ln); }
      pb2.done(); ch.settle('已发布 · call_handler');
    } else { ch.settle('已拒绝 · call_handler'); }
    if (dock) dock.set([['抓取竞品动态并汇总（function）', 'completed'], ['接 Notion 写手（handler）', 'completed'], ['编排每日 workflow', 'in-progress'], ['上线 + 验证一次', 'pending']]);
    await sleep(560); if (!alive(id)) return;

    // BEAT 5 — Subagent(Plan) 子树（E3 嵌套）+ create_workflow → 右岛 workflow 卡
    await typeInto(para(t), '接下来把这些接成每天触发的 workflow。我派一个 Plan 子 agent 先理清接线。', 64); if (!alive(id)) return; await sleep(450);
    const sub = subtree(t, 'Subagent · Plan');   // 子树即子 agent 标记（隔离 · 25 轮 · workspace dialogue · 深度 1）
    reasonBlock(sub, '触发用 cron（每天 08:00）；trigger → action(weekly_digest) → agent(摘要润色) → approval(人工过目) → action(notion_writer.publish)。回边只在 approval 上闭合（环纪律）。');
    await sleep(420); if (!alive(id)) return;
    const cw = toolGroup(sub); cw.status('Forging weekly_report…'); await sleep(520); if (!alive(id)) return;
    ChatEntityCard.render({ kind: 'workflow', name: 'weekly_report', version: 1, live: 'forge', id: 'wf_2b91ac7740e8d3f1',
      desc: '每天 08:00 抓取 → 润色 → 人工过目 → 发布 的编排图。', concurrency: 'serial', lifecycle: 'inactive', nodes: [] });
    const wcard = ChatEntityCard.el; await sleep(420); if (!alive(id)) return;
    const wnodes = ChatEntityCard.$('[data-f="graph"] .ec-rows', wcard);
    for (const n of ENT.weekly_report.nodes) {
      await sleep(280); if (!alive(id)) return;
      const r = el('div', 'ec-row new'); r.innerHTML = `<span class="nico">${icon(NICON[n.kind] || n.kind, 15)}</span><span class="nid">${n.id}</span><span class="nref">${n.ref}</span>`; wnodes.appendChild(r);
      ChatEntityCard.$('[data-f="graph"] [data-nc]', wcard).textContent = wnodes.children.length + ' 节点';
    }
    await sleep(550); if (!alive(id)) return; ChatEntityCard.setLive(false);
    toolItem(cw.box, { verb: 'Forged', name: 'create_workflow(weekly_report)',
      detailHTML: `<div class="diff-cap">实体版本 diff · 图 ops（add_node ×5）</div><div class="tbox"><div class="diff">
        <div class="dline add"><span class="s">+</span><span class="c">add_node daily (trigger · cron 08:00)</span></div>
        <div class="dline add"><span class="s">+</span><span class="c">add_node fetch (action · fn_weekly_digest)</span></div>
        <div class="dline add"><span class="s">+</span><span class="c">add_node polish (agent) + review (approval)</span></div>
        <div class="dline add"><span class="s">+</span><span class="c">add_node publish (action · hd_notion_writer.publish)</span></div></div></div>` });
    cw.settle('已锻造 · create_workflow');
    if (dock) dock.set([['抓取竞品动态并汇总（function）', 'completed'], ['接 Notion 写手（handler）', 'completed'], ['编排每日 workflow', 'completed'], ['上线 + 验证一次', 'in-progress']]);
    await sleep(560); if (!alive(id)) return;

    // BEAT 6 — trigger_workflow → durable flowrun，逐节点推进，approval 节点 park → 通过/驳回
    await typeInto(para(t), '先手动触发一次，看整条链路跑通没。', 60); if (!alive(id)) return; await sleep(150);
    const tg = toolGroup(t); tg.status('trigger_workflow(weekly_report)…'); await sleep(560); if (!alive(id)) return;
    toolItem(tg.box, { name: 'trigger_workflow(weekly_report)', detailHTML: '<div class="tbox"><div class="out">202 { id } · flowrun fr_9c4e… · 节点结果记忆化、逐节点推进（非事件日志）</div></div>' });
    tg.settle('已起 run · trigger_workflow');
    const fr = flowrunStrip(t, 'fr_9c4e1d77a0b3');
    const a1 = fr.addNode('trigger', 'daily'); await sleep(420); a1.running(); await sleep(620); if (!alive(id)) return; a1.ok(true);
    const a2 = fr.addNode('action', 'fetch'); await sleep(320); a2.running(); await sleep(820); if (!alive(id)) return; a2.ok(false);
    const a3 = fr.addNode('agent', 'polish'); await sleep(320); a3.running(); await sleep(820); if (!alive(id)) return; a3.ok(false);
    const a4 = fr.addNode('approval', 'review'); await sleep(360); if (!alive(id)) return;
    const dec = await a4.park(HOLD() ? 0 : 2200); if (!alive(id)) return;   // 与 chat 危险闸不同：flowrun :decide（通过/驳回）
    if (dec === 'yes') { const a5 = fr.addNode('action', 'publish'); await sleep(320); a5.running(); await sleep(720); if (!alive(id)) return; a5.ok(false); }
    fr.finish(); await sleep(500); if (!alive(id)) return;

    // BEAT 7 — 诚实终态（max_steps）+ 本回合实体（点药丸唤右岛）
    const t2 = aiTurn();
    await typeInto(para(t2), '链路跑通 ✅ 摘要已发布、workflow 已就绪。还差最后一步 activate 上线每日触发——不过这个回合已经到步数上限了。', 60); if (!alive(id)) return;
    await sleep(180);
    para(t2).innerHTML = `本回合锻造 / 触达的实体：${refPill('function', 'weekly_digest', 'weekly_digest')} ${refPill('handler', 'notion_writer', 'notion_writer')} ${refPill('agent', '摘要润色', 'polish')} ${refPill('workflow', 'weekly_report', 'weekly_report')} —— 点开看右岛详情。`;
    turnEnd(t2, '回合到达步数上限（25 步），诚实终止——<b>未失败</b>。');
    col().querySelectorAll('[data-ent]').forEach(p => p.onclick = () => openEntity(p.dataset.ent));
    setGen(false);
  }
})();

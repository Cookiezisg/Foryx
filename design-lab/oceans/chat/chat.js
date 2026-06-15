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
    // —— v2：另外八种实体（各自一个对话场景）——
    pdf_extract: { kind: 'function', name: 'pdf_extract', version: 2, id: 'fn_3c91a7b240e8f15d', runs: 6,
      desc: '从 PDF 抽取表格为结构化 JSON（无状态、每调用一进程）。', python: '3.12',
      code: 'def pdf_extract(url, pages):\n    doc = pdfplumber.open(fetch(url))\n    out = []\n    for p in pages:\n        out += [t.extract() for t in doc.pages[p].find_tables()]\n    return out',
      inputs: ['url: str', 'pages: list[int]'], deps: ['pdfplumber', 'pandas'], env: 'ready' },
    webhook_handler: { kind: 'handler', name: 'webhook_ingest', version: 2, id: 'hd_8b21fe04a9c7d350', runs: 128,
      desc: '常驻 webhook 入库：保活 DB 连接、跨调用复用 self.conn（真共享状态）。', runtime: 'running', configState: 'ready', env: 'ready',
      initArgs: [{ name: 'db_url', required: true, sensitive: true }, { name: 'table', value: 'events' }], methods: ['ingest', 'flush', 'stats'],
      classCode: 'class WebhookIngest:\n    def __init__(self, db_url, table="events"):\n        self.conn = connect(db_url)\n    def ingest(self, payload): ...\n    def shutdown(self): self.conn.close()' },
    researcher: { kind: 'agent', name: 'Researcher', version: 3, id: 'ag_5f2c8a10d4e3b7f9', runs: 24,
      desc: '深度调研员：检索、交叉验证、产出带引用的结构化综述。', system: '你是严谨的调研员。检索、交叉验证，每条结论必须附引用来源（行内链接）。',
      model: 'claude-opus-4-8', tools: [{ ref: 'fn_pdf_extract' }, { ref: 'hd_webhook_ingest.stats' }, { ref: 'mcp:web/fetch' }], skill: 'cite-sources', knowledge: ['竞品列表.md'] },
    cel_control: { kind: 'control', name: 'cel_validator', version: 4, id: 'ctl_7a3e1c90f24b6d8e',
      desc: '按金额 / 类目把请求路由到不同审批强度。', inputs: ['input.amount', 'input.category'], validation: 'valid',
      branches: [{ when: 'input.amount > 10000', port: 'strict', emit: '{level: "strict"}' }, { when: 'input.category == "refund"', port: 'review' }, { catchall: true, port: 'auto' }] },
    publish_gate: { kind: 'approval', name: 'publish_review', version: 1, id: 'apf_2d8f6b71a0c4e93f',
      desc: '发布前人工过目；2 天无人处理则自动驳回。', template: '## 发布确认\n标题：{{ input.title }}\n块数：{{ input.blocks }}\n是否发布到 {{ input.database }}？',
      allowReason: true, timeout: '2d', behavior: 'reject', validity: 'valid' },
    slack_trigger: { kind: 'trigger', name: 'slack_new_post', id: 'trg_9e1a4f80b2c6d57a', source: 'webhook', listening: true,
      desc: 'Slack 新帖 webhook → 扇给周报 workflow。', config: [{ k: 'path', v: '/hooks/slack/x9f2' }, { k: 'secret', mask: true }, { k: 'method', v: 'POST' }],
      dedup: 'sha256(body) + 分钟桶', listeners: ['weekly_report'] },
    notion_mcp: { kind: 'mcp', name: 'notion', id: 'mcp_notion', source: 'registry', conn: 'ready',
      desc: 'Notion MCP server：外部工具网桥（容器实体、非 Quadrinity）。', transport: 'stdio',
      transportCfg: [{ k: 'command', v: 'npx' }, { k: 'args', v: '@notion/mcp' }], secrets: ['NOTION_TOKEN'], tools: ['query-database', 'create-page', 'search', 'append-blocks', 'get-page'] },
    market_doc: { kind: 'document', name: '竞品列表.md', id: 'doc_4b7c2e91', path: '/研究/竞品', desc: '竞品清单与监控源（被检索 / 挂载，非执行实体）。', size: '4.2 KB',
      body: '# 竞品列表\n- OpenAI — [[openai-watch]]\n- Anthropic — [[anthropic-watch]]\n\n监控源见 [[rss-sources]]。', wikilinks: ['openai-watch', 'anthropic-watch', 'rss-sources'], tags: ['research', 'competitive'], mountedAs: ['Researcher', '文档问答 agent'] },
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
      $('#i_replay').onclick = () => open(curScenario, curTitle);
      $('#i_panel').onclick = () => ChatEntityCard.toggle();
      $('.enter', sea).onclick = () => open(curScenario, curTitle);
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
      open(DEFAULT);
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
        okPort(port) { r.className = 'fr-node ok'; st().innerHTML = `${icon('check', 13)} ok <span class="nport">→ port: ${port}</span>`; toBottom(); },   // control 节点：解析出口
        fail(msg) { r.className = 'fr-node failed'; st().innerHTML = `${icon('close', 13)} ${msg || 'failed'}`; toBottom(); },
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
    body.appendChild(t); $('.cont', t).onclick = () => open(curScenario, curTitle); toBottom(); return t;
  }
  function compaction(body, n) { const c = el('div', 'compaction'); c.textContent = `· 上文已压缩 · earlier context summarized · seq ≤ ${n} ·`; body.appendChild(c); }

  // —— 旗舰 spine：竞品日报 workflow（多实体一回合；默认场景） ——
  async function runWfWeekly() {
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
    wirePills();
    setGen(false);
  }

  // ======== v2 共享：场景前奏 + 中间卡 helper ========
  function begin() { const id = ++runId; col().innerHTML = ''; ChatEntityCard.hide(); if (ChatEntityCard.el) ChatEntityCard.el.innerHTML = ''; setGen(true); if (dock) dock.hide(); return id; }
  function wirePills() { col().querySelectorAll('[data-ent]').forEach(p => p.onclick = () => openEntity(p.dataset.ent)); }
  function entsLine(body, label, keys) { const p = para(body); p.innerHTML = `${label}：${keys.map(k => refPill(ENT[k].kind, ENT[k].name, k)).join(' ')} —— 点开看右岛详情。`; wirePills(); }
  function pcode(parentW, json, cap) { const c = el('div', 'res-cap'); c.textContent = cap || '返回结果（裸结果，不裹 {result}）'; parentW.appendChild(c); const b = el('div', 'tbox'); b.innerHTML = `<div class="out">${json}</div>`; parentW.appendChild(b); toBottom(); }
  // 执行/调用台账（共享：function executions / mcp calls / handler·agent calls）
  function ledger(body, head, rows) {
    const w = el('div', 'ledger');
    w.innerHTML = `<div class="ledger-head"><span>${head.title}</span><span class="agg">${head.agg || ''}</span></div>` +
      rows.map(r => `<div class="cl-row ${r.st}"><span class="cl-st"></span><span class="cl-name">${r.name}</span><span class="cl-meta">${r.meta || ''}</span><span class="chev">${icon('chevr', 13)}</span></div><div class="cl-logs">${r.logs || ''}</div>`).join('');
    body.appendChild(w); toBottom();
    w.querySelectorAll('.cl-row').forEach(row => row.onclick = () => row.classList.toggle('open'));
    return w;
  }
  // mcp 连接态机条（借 flowrun 皮肤；非耐久）
  function connStrip(body) {
    const f = el('div', 'conn-strip'); f.innerHTML = `<div class="seg-label">连接 · MCP server</div><div data-rows></div>`;
    body.appendChild(f); toBottom(); const rows = $('[data-rows]', f);
    return { el: f, add(cls, ic, text, reconnect) { const r = el('div', 'conn-row ' + cls); r.innerHTML = `<span class="ci">${icon(ic, 15)}</span><span${cls === 'connecting' ? ' class="shimmer"' : ''}>${text}</span>${reconnect ? '<button class="conn-reconnect">reconnect</button>' : ''}`; rows.appendChild(r); toBottom(); return r; } };
  }
  // trigger fire 信号条（flowrun 变体：ephemeral、无记忆化/replay；Activation 必写 + per-workflow firing）
  function fireStrip(body, traId) {
    const f = el('div', 'fire-strip'); f.innerHTML = `<div class="fr-head"><span class="fl">trigger fire</span><span class="fid">${traId}</span><span class="pulse" data-pulse></span></div>
      <div class="seg-label">Activation（触没触发都记）</div><div data-act></div><div class="seg-label">Firings（每监听 workflow 一条）</div><div data-fire></div>`;
    body.appendChild(f); toBottom();
    return {
      el: f,
      activation(text) { $('[data-act]', f).innerHTML = `<div class="firing-row ok"><span class="nid">${text}</span><span class="nstatus">${icon('check', 13)} fired=true</span></div>`; toBottom(); },
      firing(name) {
        const r = el('div', 'firing-row started'); r.innerHTML = `<span class="nid">${name}</span><span class="nstatus"><span class="shimmer">started · 起 fr_…</span></span>`; $('[data-fire]', f).appendChild(r); toBottom();
        return { ok(fr) { r.className = 'firing-row ok'; $('.nstatus', r).innerHTML = `${icon('check', 13)} started → ${fr}`; toBottom(); }, skip() { r.className = 'firing-row skipped'; $('.nstatus', r).textContent = 'skipped · overlap=serial'; toBottom(); } };
      },
      done() { const p = $('[data-pulse]', f); if (p) p.classList.add('off'); },
    };
  }
  // approval 收件箱决策卡（借审批骨架；shield+clock，通过/驳回——无「始终批准」，区别 chat 危险闸）
  function inboxCard(body, o) {
    const c = el('div', 'inbox-card');
    c.innerHTML = `<div class="ib-head"><span class="ib-shield">${icon('shield', 16)}</span>
        <span class="ib-tt"><b>审批收件箱 · ${o.node}</b><span class="sub">flowrun parked · 等人工决策</span></span>
        <span class="inbox-countdown">${icon('clock', 13)} ${o.countdown || ''}</span></div>
      <div class="ib-body"><div class="ec-rendered">${o.rendered || ''}</div>
        <div class="ac-actions"><button class="acbtn primary" data-act="yes">${icon('check', 15)} 通过</button><button class="acbtn deny" data-act="no">驳回</button></div>
        <div class="ib-foot">first-wins：人工决策与超时同源，谁先到算谁（输家 422）。</div></div>
      <div class="ac-settled"><span class="ico">${icon('check', 15)}</span><span data-settled></span></div>`;
    body.appendChild(c); toBottom();
    return {
      el: c,
      settle(t) { c.classList.add('settled'); $('[data-settled]', c).textContent = t; toBottom(); },
      wait(autoAct, ms) { return new Promise(res => { let d = false; const fin = v => { if (d) return; d = true; res(v); }; c.querySelectorAll('[data-act]').forEach(b => b.onclick = () => fin(b.dataset.act)); if (autoAct) setTimeout(() => fin(autoAct), ms || 2000); }); },
    };
  }
  // 文档树面板（借岛皮肤；缩进行、无 pulse/状态——非 run）
  function docTreePanel(body, rows) {
    const w = el('div', 'doc-tree');
    w.innerHTML = rows.map(r => `<div class="doc-tree-row${r.cur ? ' cur' : ''}" style="padding-left:${8 + r.depth * 16}px"><span class="ico">${icon(r.leaf ? 'doc' : 'folder', 14)}</span>${r.name}</div>`).join('');
    body.appendChild(w); toBottom(); return w;
  }
  const tdet = ti => $('.ti-det > .w', ti);   // 取工具项详情宿主

  // ======== 八个实体场景（各演一种实体的右岛卡 + 中间卡） ========

  // ① function —— PDF 提取（无状态、env-fix 修复循环、progress+result、版本 diff、台账）
  async function runFnPdf() {
    const id = begin();
    userMsg('帮我写个从 PDF 抽取表格的函数，按页码抽成结构化 JSON。');
    await sleep(450); if (!alive(id)) return;
    const t = aiTurn();
    reasonBlock(t, '一次性抽取、无跨调用状态 → 选 function（无状态、每调用一个全新沙箱进程）。');
    await typeInto(para(t), '锻造一个无状态抽取函数 pdf_extract。', 62); if (!alive(id)) return;
    if (dock) { dock.show(); dock.set([['锻造 pdf_extract（function）', 'in-progress'], ['物化 env + 跑通', 'pending']]); }
    await sleep(180);
    const cf = toolGroup(t); cf.status('Forging pdf_extract…'); await sleep(480); if (!alive(id)) return;
    ChatEntityCard.render({ kind: 'function', name: 'pdf_extract', version: 1, live: 'forge', id: 'fn_3c91a7b240e8f15d', desc: '从 PDF 抽取表格为结构化 JSON。', python: '3.12', code: '', inputs: ['url: str', 'pages: list[int]'], deps: [], env: 'pending' });
    const card = ChatEntityCard.el; await sleep(360); if (!alive(id)) return;
    await typeInto(ChatEntityCard.$('[data-f="code"] .val', card), 'def pdf_extract(url, pages):\n    doc = pdfplumber.open(fetch(url))\n    out = []\n    for p in pages:\n        out += [t.extract() for t in doc.pages[p].find_tables()]\n    return out', 105); if (!alive(id)) return;
    ChatEntityCard.setEnv('syncing');
    for (const d of ['pdfplumber==0.9', 'pandas']) { await sleep(240); if (!alive(id)) return; const tl = ChatEntityCard.$('[data-f="deps"] .taglist', card); const tag = el('span', 'tag new'); tag.textContent = d; tl.appendChild(tag); ChatEntityCard.$('[data-f="deps"] [data-dc]', card).textContent = tl.children.length; }
    await sleep(280); if (!alive(id)) return;
    const ev = toolGroup(t); ev.status('env 物化 · 修复循环…'); const evi = toolItem(ev.box, { name: 'ensureEnv(pdf_extract)' }); const evp = progressBox(tdet(evi));
    evp.add('pip install pdfplumber==0.9 … ✗ 无匹配版本'); await sleep(480); if (!alive(id)) return;
    evp.add('LLM 改依赖 → pdfplumber>=0.11（尝试 2/3）');
    const deps = ChatEntityCard.$('[data-f="deps"] .taglist', card); if (deps && deps.firstChild) { deps.firstChild.className = 'tag fixed'; deps.firstChild.textContent = 'pdfplumber>=0.11'; }
    await sleep(480); if (!alive(id)) return; evp.add('pip install pdfplumber>=0.11 … ✓'); evp.done(); ev.settle('env ready · ensureEnv');
    ChatEntityCard.setEnv('ready'); ChatEntityCard.setLive(false); cf.settle('已锻造 · create_function');
    if (dock) dock.set([['锻造 pdf_extract（function）', 'completed'], ['物化 env + 跑通', 'in-progress']]);
    await sleep(420); if (!alive(id)) return;
    await typeInto(para(t), '跑一页看看。', 54); if (!alive(id)) return; await sleep(140);
    const rf = toolGroup(t); rf.status('run_function(pdf_extract)…'); const rti = toolItem(rf.box, { name: 'run_function(pdf_extract)' }); const rp = progressBox(tdet(rti));
    for (const ln of ['open report.pdf … 14 页', 'find_tables p.3 … 2 表', 'extract → 38 行']) { await sleep(330); if (!alive(id)) return; rp.add(ln); }
    rp.done(); pcode(tdet(rti), '[ { "page": 3, "rows": 38, "cols": ["项目","金额"] }, … ]', '返回结果（单一 JSON · safe 无需确认）'); rf.settle('运行完成 · run_function');
    if (dock) dock.set([['锻造 pdf_extract（function）', 'completed'], ['物化 env + 跑通', 'completed']]);
    await sleep(420); if (!alive(id)) return;
    await typeInto(para(t), '加个超时保护，升个版本。', 54); if (!alive(id)) return; await sleep(140);
    const ed = toolGroup(t); ed.status('edit_function(pdf_extract)…'); await sleep(440); if (!alive(id)) return;
    toolItem(ed.box, { verb: 'Forged', name: 'edit_function(pdf_extract) → v2', detailHTML: `<div class="diff-cap">实体版本 diff · v1→v2（ops 差量；指针 active→v2，可 revert）</div><div class="tbox"><div class="diff"><div class="dline del"><span class="s">−</span><span class="c">doc = pdfplumber.open(fetch(url))</span></div><div class="dline add"><span class="s">+</span><span class="c">doc = pdfplumber.open(fetch(url, timeout=30))</span></div></div></div>` });
    ed.settle('已锻造 · edit_function → v2'); ChatEntityCard.setVersion(2);
    await sleep(440); if (!alive(id)) return;
    const t2 = aiTurn();
    await typeInto(para(t2), '执行台账在这（点行看 logs）：', 54); if (!alive(id)) return;
    ledger(t2, { title: 'function executions · list_function_executions', agg: '6 runs · 6 ok' }, [{ st: 'ok', name: 'run #6 · manual', meta: '120ms', logs: 'open report.pdf · 38 行 · ok' }, { st: 'ok', name: 'run #5 · workflow', meta: '98ms', logs: 'ok' }]);
    entsLine(t2, '本回合实体', ['pdf_extract']);
    turnEnd(t2, '回合到达步数上限（25 步），诚实终止——<b>未失败</b>。'); setGen(false);
  }

  // ② handler —— Webhook 入库（常驻、config 掩码、runtime 态机、yield、崩溃自愈）
  async function runHandler() {
    const id = begin();
    userMsg('帮我搭一个 webhook 入库的常驻服务，跨调用保活 DB 连接。');
    await sleep(450); if (!alive(id)) return;
    const t = aiTurn();
    reasonBlock(t, '需要跨调用保活状态（DB 连接复用）→ 选 handler（常驻进程、self.xxx 留存；区别无状态 function）。');
    await typeInto(para(t), '锻造常驻 handler webhook_ingest。', 60); if (!alive(id)) return;
    if (dock) { dock.show(); dock.set([['锻造 webhook_ingest（handler）', 'in-progress'], ['配 config（密钥）→ 起实例', 'pending'], ['调一次 ingest', 'pending']]); }
    await sleep(180);
    const cf = toolGroup(t); cf.status('Forging webhook_ingest…'); await sleep(480); if (!alive(id)) return;
    ChatEntityCard.render({ kind: 'handler', name: 'webhook_ingest', version: 1, live: 'forge', id: 'hd_8b21fe04a9c7d350', desc: '常驻 webhook 入库：保活 DB 连接、跨调用复用。', runtime: 'stopped', configState: 'unconfigured', env: 'syncing', initArgs: [{ name: 'db_url', required: true, sensitive: true }, { name: 'table', value: 'events' }], methods: ['ingest', 'flush', 'stats'], classCode: '' });
    const card = ChatEntityCard.el; await sleep(380); if (!alive(id)) return;
    await typeInto(ChatEntityCard.$('[data-f="class"] .val', card), 'class WebhookIngest:\n    def __init__(self, db_url, table="events"):\n        self.conn = connect(db_url)\n    def ingest(self, payload): ...\n    def shutdown(self): self.conn.close()', 95); if (!alive(id)) return;
    await sleep(400); ChatEntityCard.setEnv('ready'); ChatEntityCard.setLive(false);
    toolItem(cf.box, { verb: 'Forged', name: 'create_handler(webhook_ingest)', detailHTML: `<div class="diff-cap">实体版本 diff · v1（ops：set_init_args_schema / add_method ×3 / set_shutdown）</div><div class="tbox"><div class="out">create 不 spawn 实例 → Runtime=stopped · Config=unconfigured（等配齐 config / 首调）</div></div>` });
    cf.settle('已锻造 · create_handler');
    await sleep(420); if (!alive(id)) return;
    await typeInto(para(t), '配 DB 连接串（密钥由你填，加密存盘、读时掩码）。', 60); if (!alive(id)) return; await sleep(150);
    const cfgc = toolGroup(t); cfgc.status('update_handler_config…'); await sleep(420); if (!alive(id)) return;
    const ap = approvalCard(t, { flavor: 'ask', tool: 'update_handler_config(webhook_ingest)', summary: 'db_url 是敏感字段——请你填入（不进对话历史、AES-GCM 加密存盘）。', placeholder: 'postgres://…（仅你可见）' });
    const { act } = await ap.wait(HOLD() ? null : 'accept', 1800); if (!alive(id)) return;
    ap.settle(act === 'decline' ? '已取消 · 未配置' : '已写入 · db_url 已加密');
    await sleep(200); if (!alive(id)) return;
    if (act !== 'decline') {
      ChatEntityCard.reveal('init', 'db_url'); ChatEntityCard.setConfig('ready'); ChatEntityCard.setRuntime('running');
      cfgc.settle('已更新 config · 重启实例 → running（hdi_4f2a…）');
      if (dock) dock.set([['锻造 webhook_ingest（handler）', 'completed'], ['配 config（密钥）→ 起实例', 'completed'], ['调一次 ingest', 'in-progress']]);
      await sleep(450); if (!alive(id)) return;
      await typeInto(para(t), '调一次 ingest 验证。', 54); if (!alive(id)) return; await sleep(140);
      const cl = toolGroup(t); cl.status('call_handler(webhook_ingest.ingest)…'); const cli = toolItem(cl.box, { name: 'call_handler(webhook_ingest.ingest)' }); const cp = progressBox(tdet(cli));
      for (const ln of ['yield: validate payload … ok', 'yield: INSERT events … 1 行', 'self.conn 复用（实例保活）']) { await sleep(330); if (!alive(id)) return; cp.add(ln); }
      cp.done(); pcode(tdet(cli), '{ "inserted": 1, "id": "evt_9f31" }', '返回结果'); cl.settle('调用完成 · call_handler');
      if (dock) dock.set([['锻造 webhook_ingest（handler）', 'completed'], ['配 config（密钥）→ 起实例', 'completed'], ['调一次 ingest', 'completed']]);
    }
    await sleep(450); if (!alive(id)) return;
    const t2 = aiTurn();
    para(t2).innerHTML = '提示：edit / 改 config / crash 都会<b>重启常驻实例</b>（吃新代码 / 新 config）。';
    entsLine(t2, '本回合实体', ['webhook_handler']);
    turnEnd(t2, '回合到达步数上限（25 步），诚实终止——<b>未失败</b>。'); setGen(false);
  }

  // ③ agent —— Researcher 调优（@提及冻结、全量 Config 替换、挂载健康、嵌套 transcript、子运行危险闸）
  async function runAgentTune() {
    const id = begin();
    userMsg(`把 ${refPill('agent', 'Researcher', 'researcher')} 升到 Opus，并把离线的 web/search 换成 web/fetch、要求每条结论附引用。`);
    wirePills(); await sleep(500); if (!alive(id)) return;
    const t = aiTurn();
    reasonBlock(t, 'agent 编辑是「全量 Config 快照替换」（非 ops 增量）——声明式配置整体替换语义更清晰。@提及在发送时已冻结 Researcher v2 现状。');
    await typeInto(para(t), '读现状 → 全量替换出 v3。', 58); if (!alive(id)) return; await sleep(150);
    const g = toolGroup(t); g.status('get_agent(Researcher)…'); await sleep(420); if (!alive(id)) return;
    ChatEntityCard.render({ kind: 'agent', name: 'Researcher', version: 2, live: 'edit', id: 'ag_5f2c8a10d4e3b7f9', runs: 24, desc: '深度调研员：检索、交叉验证、产出综述。', system: '你是严谨的调研员。检索、交叉验证。', model: 'claude-sonnet-4-6', tools: [{ ref: 'fn_pdf_extract', health: 'ok' }, { ref: 'mcp:web/search', health: 'bad' }], skill: null, knowledge: ['竞品列表.md'] });
    const card = ChatEntityCard.el; toolItem(g.box, { name: 'get_agent(Researcher)', detailHTML: '<div class="tbox"><div class="out">挂载体检：mcp:web/search 离线（红点）→ invoke 会 fail-fast</div></div>' }); g.settle('已读 · get_agent v2');
    await sleep(450); if (!alive(id)) return;
    const e = toolGroup(t); e.status('edit_agent(Researcher)…'); await sleep(420); if (!alive(id)) return;
    const sv = ChatEntityCard.$('[data-f="system"] .val', card); sv.classList.add('flash'); sv.textContent = ''; await typeInto(sv, '你是严谨的调研员。检索、交叉验证，每条结论必须附引用来源（行内链接）。', 46); if (!alive(id)) return;
    const mv = ChatEntityCard.$('[data-f="model"] .val', card); mv.classList.add('flash'); mv.textContent = 'claude-opus-4-8';
    await sleep(350); if (!alive(id)) return;
    // 挂载工具：删 web/search、加 web/fetch（行预检 ok）
    const tl = ChatEntityCard.$('[data-f="tools"] .ec-rows', card); if (tl) { tl.innerHTML = '<div class="ec-row"><span class="mh-dot ok"></span><span class="nid">fn_pdf_extract</span></div><div class="ec-row new"><span class="mh-dot ok"></span><span class="nid">mcp:web/fetch</span></div><div class="ec-row new"><span class="mh-dot ok"></span><span class="nid">hd_webhook_ingest.stats</span></div>'; }
    ChatEntityCard.$('[data-f="tools"] [data-tc]', card).textContent = '3';
    await sleep(400); if (!alive(id)) return; ChatEntityCard.setVersion(3); ChatEntityCard.setLive(false);
    toolItem(e.box, { verb: 'Forged', name: 'edit_agent(Researcher) → v3', detailHTML: `<div class="diff-cap">实体版本 diff · v2→v3（全量 Config 快照替换；含 del + add）</div><div class="tbox"><div class="diff"><div class="dline del"><span class="s">−</span><span class="c">model: claude-sonnet-4-6 · tools: [web/search✗]</span></div><div class="dline add"><span class="s">+</span><span class="c">model: claude-opus-4-8</span></div><div class="dline add"><span class="s">+</span><span class="c">tools: [pdf_extract, web/fetch, webhook_ingest.stats]（5/5 健康）</span></div><div class="dline add"><span class="s">+</span><span class="c">system += 每条结论附引用来源</span></div></div></div>` });
    e.settle('已锻造 · edit_agent → v3');
    await sleep(450); if (!alive(id)) return;
    await typeInto(para(t), 'invoke 一次验证（嵌套 transcript）。', 56); if (!alive(id)) return; await sleep(150);
    const iv = toolGroup(t); iv.status('invoke_agent(Researcher)…'); await sleep(450); if (!alive(id)) return;
    iv.settle('员工运行完成 · invoke_agent');
    const sub = subtree(t, 'invoke_agent · Researcher（嵌套 ReAct，落 Execution.transcript）');
    reasonBlock(sub, '先用 web/fetch 取最新动态，再交叉验证、附引用。');
    const it = toolGroup(sub); it.status('mcp__web__fetch…'); await sleep(450); if (!alive(id)) return;
    toolItem(it.box, { name: 'mcp__web__fetch(openai.com/blog)', detailHTML: '<div class="tbox"><div class="out">200 · 3 条新动态</div></div>' }); it.settle('已取 · web/fetch');
    await sleep(300); if (!alive(id)) return;
    pcode(it.box, '{ "summary": "…", "citations": ["https://…"] }', 'outputs 硬约束：最终答案 = 恰含声明字段的单个 JSON');
    const t2 = aiTurn();
    await typeInto(para(t2), 'InvokeResult：steps 4 · tokens 3.1k · stopReason end_turn。子运行里若调危险工具，会在共享 loop 的危险门阻塞、唤你确认（与本回合同一 broker）。', 56); if (!alive(id)) return;
    entsLine(t2, '本回合实体', ['researcher', 'pdf_extract']);
    turnEnd(t2, '回合到达步数上限（25 步），诚实终止——<b>未失败</b>。'); setGen(false);
  }

  // ④ control —— 修复 CEL 校验器（分支组、双病灶、错误码、实时改、干跑 resolved-port）
  async function runControlFix() {
    const id = begin();
    compaction(col(), 64);
    userMsg(`${refPill('control', 'cel_validator', 'cel_control')} 报错跑不起来，帮我修一下。`);
    wirePills(); await sleep(500); if (!alive(id)) return;
    const t = aiTurn();
    await typeInto(para(t), '读回当前版本看看哪坏了。', 56); if (!alive(id)) return; await sleep(150);
    const g = toolGroup(t); g.status('get_control(cel_validator)…'); await sleep(420); if (!alive(id)) return;
    ChatEntityCard.render({ kind: 'control', name: 'cel_validator', version: 3, live: 'edit', id: 'ctl_7a3e1c90f24b6d8e', desc: '按金额 / 类目路由审批强度。', inputs: ['input.amount', 'input.category'], validation: 'invalid',
      branches: [{ when: 'input.amount > 10000', port: 'strict', emit: '{level: "strict"}' }, { when: 'input.catagory == "refund"', port: 'review', bad: true }, { when: 'input.amount < 100', port: 'auto' }] });
    const card = ChatEntityCard.el; g.settle('已读 · get_control v3（validation 不通过）');
    reasonBlock(t, '两处病灶：① branch[1] 的 when 把 category 拼成了 catagory（CEL 编译失败）；② 末条 when 不是 true，all-false 时无兜底路由。');
    para(t).insertAdjacentHTML('beforeend', '<div style="margin-top:8px"><span class="badge dangerous">CONTROL_INVALID_CEL</span> <span class="badge cautious">CONTROL_NO_CATCHALL</span></div>');
    if (dock) { dock.show(); dock.set([['修拼写 CEL', 'in-progress'], ['补 catchall 兜底', 'pending'], ['干跑验证 resolved-port', 'pending']]); }
    await sleep(500); if (!alive(id)) return;
    await typeInto(para(t), '改：修拼写 + 末条加 true 兜底。', 56); if (!alive(id)) return; await sleep(150);
    const e = toolGroup(t); e.status('edit_control(cel_validator)…'); ChatEntityCard.setValidation('compiling'); await sleep(500); if (!alive(id)) return;
    // 右岛实时改分支
    const br = ChatEntityCard.$('[data-f="branches"] .ec-rows', card);
    if (br) {
      br.children[1].classList.remove('bad'); br.children[1].querySelector('.bwhen').classList.remove('bad'); br.children[1].querySelector('.bwhen').innerHTML = 'input.category == "refund"';
      await sleep(450); if (!alive(id)) return;
      const cc = el('div', 'branch-row catchall new'); cc.innerHTML = '<span class="bnum">4</span><div class="bbody"><div class="bwhen">true（兜底）</div></div><span class="bport">auto</span>'; br.appendChild(cc);
    }
    await sleep(400); if (!alive(id)) return; ChatEntityCard.setValidation('valid'); ChatEntityCard.setVersion(4); ChatEntityCard.setLive(false);
    toolItem(e.box, { verb: 'Forged', name: 'edit_control(cel_validator) → v4', detailHTML: `<div class="diff-cap">实体版本 diff · v3→v4（branch ops）</div><div class="tbox"><div class="diff"><div class="dline del"><span class="s">−</span><span class="c">when: input.catagory == "refund"  ← 拼写</span></div><div class="dline add"><span class="s">+</span><span class="c">when: input.category == "refund"</span></div><div class="dline add"><span class="s">+</span><span class="c">branch[last]: when=true → port auto（兜底）</span></div></div></div>` });
    e.settle('已锻造 · edit_control → v4');
    if (dock) dock.set([['修拼写 CEL', 'completed'], ['补 catchall 兜底', 'completed'], ['干跑验证 resolved-port', 'in-progress']]);
    await sleep(450); if (!alive(id)) return;
    await typeInto(para(t), '干跑一次，看 control 节点解析出的出口。', 56); if (!alive(id)) return; await sleep(150);
    const tg = toolGroup(t); tg.status('trigger_workflow(干跑)…'); await sleep(450); if (!alive(id)) return; tg.settle('已起 run · trigger_workflow');
    const fr = flowrunStrip(t, 'fr_c4e1…(dry)');
    const a1 = fr.addNode('trigger', 'in'); await sleep(300); a1.running(); await sleep(450); if (!alive(id)) return; a1.ok(false);
    const a2 = fr.addNode('control', 'gate'); await sleep(300); a2.running(); await sleep(650); if (!alive(id)) return; a2.okPort('auto'); fr.finish();
    if (dock) dock.set([['修拼写 CEL', 'completed'], ['补 catchall 兜底', 'completed'], ['干跑验证 resolved-port', 'completed']]);
    await sleep(450); if (!alive(id)) return;
    const t2 = aiTurn();
    para(t2).innerHTML = '修好了 ✅ control 是<b>内联求值</b>（非 activity）；不通过会拦在 edit。revert 可纯指针回 v3。';
    entsLine(t2, '本回合实体', ['cel_control']);
    setGen(false);
  }

  // ⑤ approval —— 发布前过目（模板+决策规则、接进图、parked、收件箱决策、超时倒计时）
  async function runApprovalGate() {
    const id = begin();
    userMsg('发布前要人工过目，2 天没人理就当驳回。帮我做个审批表单接进周报流程。');
    await sleep(450); if (!alive(id)) return;
    const t = aiTurn();
    reasonBlock(t, 'approval 是图的「人在环闸」——固定 yes/no 两出口、parked 行即收件箱。区别 chat 内危险工具的内存确认（两个表面）。');
    await typeInto(para(t), '锻造审批表单 publish_review。', 60); if (!alive(id)) return;
    if (dock) { dock.show(); dock.set([['锻造 publish_review（approval）', 'in-progress'], ['接进 workflow 图', 'pending'], ['跑到 review 节点决策', 'pending']]); }
    await sleep(180);
    const cf = toolGroup(t); cf.status('Forging publish_review…'); await sleep(480); if (!alive(id)) return;
    ChatEntityCard.render({ kind: 'approval', name: 'publish_review', version: 1, live: 'forge', id: 'apf_2d8f6b71a0c4e93f', desc: '发布前人工过目；2 天无人处理则驳回。', template: '', allowReason: true, timeout: '2d', behavior: 'reject', validity: 'syncing' });
    const card = ChatEntityCard.el; await sleep(360); if (!alive(id)) return;
    await typeInto(ChatEntityCard.$('[data-f="template"] .val', card), '## 发布确认\n标题：{{ input.title }}\n块数：{{ input.blocks }}\n是否发布到 {{ input.database }}？', 70); if (!alive(id)) return;
    // 重渲染模板高亮 {{CEL}}
    const tv = ChatEntityCard.$('[data-f="template"] .val', card); if (tv) tv.innerHTML = tv.textContent.replace(/\{\{([^}]*)\}\}/g, '<span class="cel-tok">{{$1}}</span>');
    await sleep(380); if (!alive(id)) return; ChatEntityCard.setValidity('valid'); ChatEntityCard.setLive(false);
    toolItem(cf.box, { verb: 'Forged', name: 'create_approval(publish_review)', detailHTML: '<div class="tbox"><div class="out">ops：set_template · allowReason=true · timeout=2d · behavior=reject（非空 timeout 必配 behavior）</div></div>' }); cf.settle('已锻造 · create_approval');
    if (dock) dock.set([['锻造 publish_review（approval）', 'completed'], ['接进 workflow 图', 'in-progress'], ['跑到 review 节点决策', 'pending']]);
    await sleep(420); if (!alive(id)) return;
    const ew = toolGroup(t); ew.status('edit_workflow(weekly_report)…'); await sleep(420); if (!alive(id)) return;
    toolItem(ew.box, { verb: 'Forged', name: 'edit_workflow → 接 review 节点', detailHTML: '<div class="tbox"><div class="diff"><div class="dline add"><span class="s">+</span><span class="c">add_node review (approval · apf_publish_review)</span></div><div class="dline add"><span class="s">+</span><span class="c">add_edge polish → review → [yes] publish</span></div></div></div>' }); ew.settle('已锻造 · edit_workflow');
    if (dock) dock.set([['锻造 publish_review（approval）', 'completed'], ['接进 workflow 图', 'completed'], ['跑到 review 节点决策', 'in-progress']]);
    await sleep(420); if (!alive(id)) return;
    await typeInto(para(t), '触发一次，跑到 review 会 parked、唤你决策。', 58); if (!alive(id)) return; await sleep(150);
    const tg = toolGroup(t); tg.status('trigger_workflow(weekly_report)…'); await sleep(420); if (!alive(id)) return; tg.settle('已起 run · trigger_workflow');
    const fr = flowrunStrip(t, 'fr_7b2d…');
    const a1 = fr.addNode('action', 'fetch'); await sleep(300); a1.running(); await sleep(500); if (!alive(id)) return; a1.ok(true);
    const a2 = fr.addNode('agent', 'polish'); await sleep(280); a2.running(); await sleep(550); if (!alive(id)) return; a2.ok(false);
    const a3 = fr.addNode('approval', 'review'); await sleep(300); a3.park(0); fr.finish();   // 不在 flowrun 内联决策，改用收件箱卡
    await sleep(300); if (!alive(id)) return;
    const t2 = aiTurn();
    para(t2).innerHTML = `通知 <span class="badge cautious">workflow.approval_pending</span> 已唤人。收件箱里渲染了模板真值：`;
    const ib = inboxCard(t2, { node: 'review', countdown: '1d 22h 后自动 reject', rendered: '## 发布确认\n标题：竞品摘要 · 06-14\n块数：17\n是否发布到 竞品追踪？' });
    const dec = await ib.wait(HOLD() ? null : 'yes', 2400); if (!alive(id)) return;
    ib.settle(dec === 'yes' ? '已通过 · flowrun 续跑 publish（first-wins）' : '已驳回 · 反馈进 run');
    if (dock) dock.set([['锻造 publish_review（approval）', 'completed'], ['接进 workflow 图', 'completed'], ['跑到 review 节点决策', 'completed']]);
    await sleep(350); if (!alive(id)) return;
    const t3 = aiTurn();
    para(t3).innerHTML = '收件箱决策（通过/驳回 · yes|no）与 chat 危险闸（批准/始终批准/拒绝）是<b>两个表面</b>：前者 durable 带超时、后者内存即时。';
    entsLine(t3, '本回合实体', ['publish_gate']);
    setGen(false);
  }

  // ⑥ trigger —— Slack 通知（信号源、监听、fire 信号条、overlap skip、台账）
  async function runTrigger() {
    const id = begin();
    compaction(col(), 64);
    userMsg('Slack 有新帖时，自动扇给周报 workflow 跑一次。');
    await sleep(450); if (!alive(id)) return;
    const t = aiTurn();
    reasonBlock(t, '外部事件驱动 → trigger（四源之一 webhook，非 manual）。trigger 是信号源 + durable 收件箱，无版本模型。');
    await typeInto(para(t), '锻造一个 webhook trigger。', 58); if (!alive(id)) return;
    if (dock) { dock.show(); dock.set([['锻造 slack_new_post（trigger）', 'in-progress'], ['attach 上线监听', 'pending'], ['手动 fire 验证', 'pending']]); }
    await sleep(180);
    const cf = toolGroup(t); cf.status('Forging slack_new_post…'); await sleep(480); if (!alive(id)) return;
    ChatEntityCard.render({ kind: 'trigger', name: 'slack_new_post', live: 'forge', id: 'trg_9e1a4f80b2c6d57a', source: 'webhook', listening: false, desc: 'Slack 新帖 webhook → 扇给周报 workflow。', config: [{ k: 'path', v: '/hooks/slack/x9f2' }, { k: 'secret', mask: true }, { k: 'method', v: 'POST' }], dedup: 'sha256(body) + 分钟桶', listeners: [] });
    const card = ChatEntityCard.el; await sleep(500); if (!alive(id)) return; ChatEntityCard.setLive(false);
    toolItem(cf.box, { verb: 'Forged', name: 'create_trigger(slack_new_post)', detailHTML: '<div class="tbox"><div class="out">config ops · 无版本模型（trigger 不走方案 A）；dedup 防重复材化</div></div>' }); cf.settle('已锻造 · create_trigger');
    await sleep(420); if (!alive(id)) return;
    await typeInto(para(t), 'attach 到周报 workflow 并上线监听。', 56); if (!alive(id)) return; await sleep(150);
    const at = toolGroup(t); at.status('attach + activate…'); await sleep(450); if (!alive(id)) return;
    // 右岛：listeners 0→1 + 头部监听点亮（重渲染）
    ChatEntityCard.render({ kind: 'trigger', name: 'slack_new_post', id: 'trg_9e1a4f80b2c6d57a', source: 'webhook', listening: true, desc: 'Slack 新帖 webhook → 扇给周报 workflow。', config: [{ k: 'path', v: '/hooks/slack/x9f2' }, { k: 'secret', mask: true }, { k: 'method', v: 'POST' }], dedup: 'sha256(body) + 分钟桶', listeners: ['weekly_report'] }); ChatEntityCard.setLive(false);
    at.settle('已上线 · refCount 0→1 · listening');
    if (dock) dock.set([['锻造 slack_new_post（trigger）', 'completed'], ['attach 上线监听', 'completed'], ['手动 fire 验证', 'in-progress']]);
    await sleep(450); if (!alive(id)) return;
    await typeInto(para(t), '手动 fire 两次（看 Activation 必写 + overlap）。', 56); if (!alive(id)) return; await sleep(150);
    const fg = toolGroup(t); fg.status(':fire ×2…'); await sleep(420); if (!alive(id)) return; fg.settle('已 fire · 202 { id }');
    const fs = fireStrip(t, 'tra_5c8f… → tra_5c90…');
    fs.activation('fire #1 · activation 写入'); await sleep(400); const f1 = fs.firing('weekly_report'); await sleep(650); if (!alive(id)) return; f1.ok('fr_a1b2');
    await sleep(350); fs.activation('fire #2 · activation 写入（在途未结束）'); const f2 = fs.firing('weekly_report'); await sleep(550); if (!alive(id)) return; f2.skip(); fs.done();
    if (dock) dock.set([['锻造 slack_new_post（trigger）', 'completed'], ['attach 上线监听', 'completed'], ['手动 fire 验证', 'completed']]);
    await sleep(400); if (!alive(id)) return;
    const t2 = aiTurn();
    await typeInto(para(t2), '双台账（触没触发都查得到）：', 54); if (!alive(id)) return;
    ledger(t2, { title: 'activations / firings', agg: '2 activations · 2 fired · 1 started · 1 skipped' }, [{ st: 'ok', name: 'activation tra_5c8f', meta: 'fired=true', logs: 'firing weekly_report → started fr_a1b2' }, { st: 'failed', name: 'firing #2', meta: 'skipped', logs: 'overlap=serial · 在途 run 未结束 → 推迟/丢' }]);
    entsLine(t2, '本回合实体', ['slack_trigger']);
    setGen(false);
  }

  // ⑦ mcp —— Notion 同步实验（装 server、缺密钥 ask、连接态机、工具发现、读/写、台账）
  async function runMcp() {
    const id = begin();
    compaction(col(), 64);
    userMsg('装个 Notion 的 MCP，让我能从对话里直接读写 Notion。');
    await sleep(450); if (!alive(id)) return;
    const t = aiTurn();
    reasonBlock(t, 'MCP = 外部工具网桥（容器实体、非 Quadrinity）；动态工具名 mcp__server__tool。');
    await typeInto(para(t), '从市场装 notion server。', 58); if (!alive(id)) return;
    if (dock) { dock.show(); dock.set([['装 notion MCP server', 'in-progress'], ['补 NOTION_TOKEN + 连接', 'pending'], ['发现工具 + 调用', 'pending']]); }
    await sleep(180);
    const inst = toolGroup(t); inst.status('install_mcp_server(notion)…'); await sleep(480); if (!alive(id)) return;
    ChatEntityCard.render({ kind: 'mcp', name: 'notion', live: 'forge', id: 'mcp_notion', source: 'registry', conn: 'connecting', desc: 'Notion MCP server：外部工具网桥。', transport: 'stdio', transportCfg: [{ k: 'command', v: 'npx' }, { k: 'args', v: '@notion/mcp' }], secrets: ['NOTION_TOKEN'], tools: [] });
    const card = ChatEntityCard.el; await sleep(450); if (!alive(id)) return;
    toolItem(inst.box, { name: 'install_mcp_server(notion)', detailHTML: '<div class="tbox"><div class="out">MCP_ENV_MISSING：缺必填 NOTION_TOKEN</div></div>' }); inst.settle('需要密钥 · install_mcp_server');
    await sleep(300); if (!alive(id)) return;
    const ap = approvalCard(t, { flavor: 'ask', tool: 'NOTION_TOKEN', summary: '连接 Notion 需要 integration token（加密存盘、读时掩码）。', placeholder: 'secret_…（仅你可见）' });
    const { act } = await ap.wait(HOLD() ? null : 'accept', 1800); if (!alive(id)) return;
    ap.settle(act === 'decline' ? '已取消 · 未连接' : '已写入 · NOTION_TOKEN 已加密');
    await sleep(200); if (!alive(id)) return;
    if (act !== 'decline') {
      ChatEntityCard.reveal('secrets', 'NOTION_TOKEN');
      const cs = connStrip(t);
      cs.add('', 'close', 'disconnected'); await sleep(350); cs.add('connecting', 'spin', 'connecting · spawn npx @notion/mcp'); ChatEntityCard.setConn('connecting'); await sleep(700); if (!alive(id)) return;
      cs.add('ok', 'check', 'ready · 握手成功'); ChatEntityCard.setConn('ready');
      if (dock) dock.set([['装 notion MCP server', 'completed'], ['补 NOTION_TOKEN + 连接', 'completed'], ['发现工具 + 调用', 'in-progress']]);
      await sleep(450); if (!alive(id)) return;
      // tools/list → 右岛 tools popin
      const tl = ChatEntityCard.$('[data-f="tools"] .taglist', card);
      if (tl) for (const tn of ['query-database', 'create-page', 'search', 'append-blocks', 'get-page']) { await sleep(150); const tag = el('span', 'tag new'); tag.textContent = tn; tl.appendChild(tag); ChatEntityCard.$('[data-f="tools"] [data-tc]', card).textContent = tl.children.length; }
      await sleep(400); if (!alive(id)) return;
      await typeInto(para(t), '先调只读的 query-database。', 54); if (!alive(id)) return; await sleep(140);
      const rc = toolGroup(t); rc.status('mcp__notion__query-database…'); const rci = toolItem(rc.box, { name: 'mcp__notion__query-database' }); const rp = progressBox(tdet(rci));
      rp.add('progress: querying「竞品追踪」…'); await sleep(450); rp.done(); pcode(tdet(rci), '{ "results": [ { "id": "pg_1", "title": "06-13" }, … 12 ] }'); rc.settle('调用完成（safe）· mcp__notion__query-database');
      await sleep(400); if (!alive(id)) return;
      await typeInto(para(t), '再调写入的 create-page（危险，需确认）。', 54); if (!alive(id)) return; await sleep(140);
      const wc = toolGroup(t); wc.status('mcp__notion__create-page…'); await sleep(450); if (!alive(id)) return;
      const wp = approvalCard(t, { flavor: 'danger', tool: 'mcp__notion__create-page', danger: 'dangerous', summary: '在「竞品追踪」新建一页（写外部空间）。', args: '{ "database": "竞品追踪", "title": "06-14" }' });
      const { act: a2 } = await wp.wait(HOLD() ? null : 'approve_always', 1900); if (!alive(id)) return;
      wp.settle(a2 === 'deny' ? '已拒绝' : '已批准 · 本会话内始终允许'); if (a2 !== 'deny') wc.settle('已写入 · mcp__notion__create-page'); else wc.settle('已拒绝 · mcp__notion__create-page');
      if (dock) dock.set([['装 notion MCP server', 'completed'], ['补 NOTION_TOKEN + 连接', 'completed'], ['发现工具 + 调用', 'completed']]);
    }
    await sleep(420); if (!alive(id)) return;
    const t2 = aiTurn();
    await typeInto(para(t2), '调用台账（含失败附 server stderr 尾）：', 54); if (!alive(id)) return;
    ledger(t2, { title: 'mcp calls · search_mcp_calls', agg: '2 calls · 1 ok · 1 failed' }, [{ st: 'ok', name: 'query-database', meta: '210ms', logs: '12 results · ok' }, { st: 'failed', name: 'append-blocks', meta: 'timeout', logs: 'server stderr: rate limited (429)' }]);
    entsLine(t2, '本回合实体', ['notion_mcp']);
    setGen(false);
  }

  // ⑧ document —— 文档问答 agent · knowledge（检索、无版本卡、wikilink、文档树、挂载、RAG）
  async function runDocQa() {
    const id = begin();
    userMsg('给文档问答 agent 配个竞品知识库，挂上去能检索。');
    await sleep(450); if (!alive(id)) return;
    const t = aiTurn();
    reasonBlock(t, '文档是「被检索 / 挂载」的知识载体，非执行实体——无版本号、无 runs。');
    await typeInto(para(t), '先全文检索看有没有现成的。', 56); if (!alive(id)) return; await sleep(150);
    const s = toolGroup(t); s.status('search_documents…'); const sti = toolItem(s.box, { name: 'search_documents(竞品)' }); await sleep(450); if (!alive(id)) return;
    pcode(tdet(sti), '{ "count": 1, "documents": [ { "name": "竞品列表.md", "path": "/研究/竞品" } ] }', '检索命中'); s.settle('已检索 · 1 命中');
    await sleep(400); if (!alive(id)) return;
    await typeInto(para(t), '补一篇监控源文档。', 54); if (!alive(id)) return; await sleep(150);
    const cf = toolGroup(t); cf.status('create_document…'); await sleep(450); if (!alive(id)) return;
    ChatEntityCard.render({ kind: 'document', name: '竞品列表.md', live: 'forge', id: 'doc_4b7c2e91', path: '/研究/竞品', desc: '竞品清单与监控源。', size: '0 B', body: '', wikilinks: [], tags: ['research'] });
    const card = ChatEntityCard.el; await sleep(360); if (!alive(id)) return;
    await typeInto(ChatEntityCard.$('[data-f="body"] .val', card), '# 竞品列表\n- OpenAI — [[openai-watch]]\n- Anthropic — [[anthropic-watch]]\n\n监控源见 [[rss-sources]]。', 60); if (!alive(id)) return;
    const bv = ChatEntityCard.$('[data-f="body"] .val', card); if (bv) bv.innerHTML = bv.textContent.replace(/\[\[([^\]]*)\]\]/g, '<span class="wikilink-pill">$1</span>');
    const sz = ChatEntityCard.$('[data-f="body"] [data-sz]', card); if (sz) sz.textContent = '4.2 KB';
    const wl = ChatEntityCard.$('[data-f="wikilinks"] .val', card); if (wl) wl.innerHTML = `<div class="taglist">${['openai-watch', 'anthropic-watch', 'rss-sources'].map(w => `<span class="tag new">${w}</span>`).join('')}</div>`;
    await sleep(400); if (!alive(id)) return; ChatEntityCard.setLive(false);
    toolItem(cf.box, { verb: 'Forged', name: 'create_document(竞品列表.md)', detailHTML: '<div class="tbox"><div class="out">文档内容 diff · content ops（无版本模型）；[[wikilink]] 自动建反链</div></div>' }); cf.settle('已创建 · create_document');
    await sleep(420); if (!alive(id)) return;
    await typeInto(para(t), '归档到 /研究/竞品 下（防环）。', 54); if (!alive(id)) return; await sleep(150);
    const mv = toolGroup(t); mv.status('move_document…'); await sleep(420); if (!alive(id)) return; mv.settle('已移动 · move_document');
    docTreePanel(t, [{ depth: 0, name: '研究', leaf: false }, { depth: 1, name: '竞品', leaf: false }, { depth: 2, name: '竞品列表.md', leaf: true, cur: true }, { depth: 2, name: 'rss-sources.md', leaf: true }]);
    await sleep(420); if (!alive(id)) return;
    await typeInto(para(t), '挂为「文档问答 agent」的 knowledge（显式单篇、不拖子树），invoke 验证 RAG。', 56); if (!alive(id)) return; await sleep(150);
    const iv = toolGroup(t); iv.status('edit_agent（挂 knowledge）+ invoke_agent…'); await sleep(450); if (!alive(id)) return; iv.settle('已挂载 + 验证 · invoke_agent');
    const sub = subtree(t, 'invoke_agent · 文档问答（knowledge 前缀拼进 user 消息）');
    pcode(sub, '{ "answer": "OpenAI 见 openai-watch；Anthropic 见 anthropic-watch", "cited": ["竞品列表.md"] }', 'RAG 注入生效');
    const t2 = aiTurn();
    para(t2).innerHTML = '完成 ✅ 文档已建、已挂载、检索可用。';
    entsLine(t2, '本回合实体', ['market_doc', 'researcher']);
    setGen(false);
  }

  // ======== 场景注册表 + 切换入口（sidebar/chat.js 点会话调 window.ChatOcean.open(id)） ========
  const DEFAULT = 'wf-weekly-report';
  const SCENARIOS = {
    'wf-weekly-report': { title: '竞品动态日报流程', run: runWfWeekly },
    'fn-pdf-extract': { title: 'PDF 提取 function', run: runFnPdf },
    'webhook-handler': { title: 'Webhook 入库 handler', run: runHandler },
    'agent-researcher-tune': { title: 'Researcher agent 调优', run: runAgentTune },
    'control-cel-fix': { title: '修复 CEL 校验器 control', run: runControlFix },
    'approval-publish-gate': { title: '发布前过目 approval', run: runApprovalGate },
    'trigger-slack-webhook': { title: 'Slack 通知 trigger', run: runTrigger },
    'mcp-notion-sync': { title: 'Notion 同步实验 MCP', run: runMcp },
    'document-qa-agent': { title: '文档问答 agent · knowledge', run: runDocQa },
  };
  let curScenario = DEFAULT, curTitle = SCENARIOS[DEFAULT].title;
  function setTitle(t) { const tt = document.querySelector('.chat-titlebar[data-ocean-head="chat"] .tt'); if (tt) tt.textContent = t; }
  // 切换入口：sidebar/chat.js 点会话调它（title 可覆盖——多会话复用同场景时各显其名）
  function open(scenarioId, title) {
    const sid = SCENARIOS[scenarioId] ? scenarioId : DEFAULT;
    curScenario = sid; curTitle = title || SCENARIOS[sid].title;
    setTitle(curTitle);
    return SCENARIOS[sid].run();
  }
  window.ChatOcean = { open };
})();

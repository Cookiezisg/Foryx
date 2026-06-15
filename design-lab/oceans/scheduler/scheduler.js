/* Forgify design-lab — 运行海洋编排（Scheduler/Operate，单独，一人负责整个 oceans/scheduler/）。
   注册：Shell.registerOcean('scheduler',{crumb:'运行',build(sea)})。选中通道：Shell.openWorkflow(name)（沿用 documents/entities 的 Shell.openX 范式；侧栏 .wf 点击调它）。
   海面 = 选中 workflow 的驾驶舱：Zone0 attention rail + Zone1「Conducted Keynote」活运行图 + 运行头 + run-rail 历史。点节点 → 右岛看记忆化结果（loop → 逐迭代 stepper；parked → 内联 :decide）。
   纯静态示意：run 嵌冻结拓扑（镜像 flowruns.version_id）；状态枚举对齐后端（run=running/completed/failed/cancelled，node=completed/failed/parked/running/future）。
   依赖 shared/icons.js（icon）+ shared/shell.js（Shell.sea/body/crumb）。样式 oceans/scheduler/scheduler.css。 */
(function () {
  const $ = (s, r = document) => r.querySelector(s);
  const esc = s => String(s == null ? '' : s).replace(/[&<>]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]));
  const NICON = { trigger: 'trigger', action: 'action', agent: 'agent', control: 'control', approval: 'shield' };
  const NW = 164, NH = 60;

  // 节点几何（与 entities graphSVG 同契约：{x,y} 左上）
  const fwdD = (s, t) => { const x1 = s.x + NW, y1 = s.y + NH / 2, x2 = t.x, y2 = t.y + NH / 2, mx = (x1 + x2) / 2; return `M${x1},${y1} C${mx},${y1} ${mx},${y2} ${x2 - 7},${y2}`; };
  // 回边：从源顶心拱到目标顶心（上拱 → 读作「返回弧」而非前进）
  const loopD = (s, t) => { const x1 = s.x + NW / 2, y1 = s.y, x2 = t.x + NW / 2, y2 = t.y, cy = Math.min(y1, y2) - 46; return `M${x1},${y1} C${x1},${cy} ${x2},${cy} ${x2},${y2}`; };

  // ===== 节点拓扑构件（depth*200+24 手排）=====
  const n = (id, kind, ref, x, y) => ({ id, kind, ref, x, y });

  // 研报抓取流拓扑（7 节点 + 一条 route→summarize 重试回边）
  const REPORT_NODES = [
    n('cron', 'trigger', 'cron_2am', 24, 150),
    n('fetch_repos', 'action', 'fetch_news', 224, 40),
    n('fetch_issues', 'action', 'parse_pdf', 224, 260),
    n('summarize', 'agent', 'research_agent', 424, 150),
    n('route', 'control', 'route_by_amount', 624, 150),
    n('publish', 'action', 'slack_handler', 824, 40),
    n('notify_lead', 'approval', 'manager_approval', 824, 260),
  ];
  const REPORT_EDGES = [['cron', 'fetch_repos'], ['cron', 'fetch_issues'], ['fetch_repos', 'summarize'], ['fetch_issues', 'summarize'], ['summarize', 'route'], ['route', 'publish'], ['route', 'notify_lead']];
  const REPORT_LOOP = [['route', 'summarize']];
  const REPORT_VB = [1012, 340];

  // ===== 示意 run 数据（键 = workflow 名，与 sidebar/scheduler.js 的 WF 名对齐）=====
  // st: completed|failed|parked|running|future ; taken/ghost: 'a>b' 字符串集 ; live: 当前导电边 ; iters/ports/memo
  const WF = {
    '研报抓取流': { cur: 1, runs: [
      { id: 'fr_29f1a4', state: 'completed', version: 'v8', trigger: 'cron_2am', when: '昨天 02:00', pos: '7/7',
        nodes: REPORT_NODES, edges: REPORT_EDGES, loopbacks: REPORT_LOOP, vb: REPORT_VB,
        st: { cron: 'completed', fetch_repos: 'completed', fetch_issues: 'completed', summarize: 'completed', route: 'completed', publish: 'completed', notify_lead: 'completed' },
        taken: ['cron>fetch_repos', 'cron>fetch_issues', 'fetch_repos>summarize', 'fetch_issues>summarize', 'summarize>route', 'route>publish', 'route>notify_lead'],
        ghost: [], live: null, iters: { summarize: 2 }, ports: { route: 'done' },
        memo: { cron: { kind: 'trigger', out: 'fired @ 02:00 · cron_2am' }, fetch_repos: { kind: 'action', out: '42 commits · 6 repos' }, fetch_issues: { kind: 'action', out: '17 open issues' },
          summarize: { kind: 'agent', loop: [{ i: 0, status: 'completed', out: 'draft v1 · 320 词' }, { i: 1, status: 'completed', out: 'draft v2 · 286 词 · 质量 0.86' }] },
          route: { kind: 'control', out: '{ __port: "done" }   // 质量 0.86 ≥ 0.8' }, publish: { kind: 'action', out: '已发 #ops · ts 1718…' }, notify_lead: { kind: 'approval', decision: 'yes', reason: '看起来不错，发。' } } },
      { id: 'fr_3a9c2f', state: 'running', version: 'v8', trigger: 'cron_2am', when: '今天 02:00', pos: '4/7',
        nodes: REPORT_NODES, edges: REPORT_EDGES, loopbacks: REPORT_LOOP, vb: REPORT_VB,
        st: { cron: 'completed', fetch_repos: 'completed', fetch_issues: 'completed', summarize: 'running', route: 'completed', publish: 'future', notify_lead: 'future' },
        taken: ['cron>fetch_repos', 'cron>fetch_issues', 'fetch_repos>summarize', 'fetch_issues>summarize', 'summarize>route'],
        ghost: [], live: 'route>summarize', iters: { summarize: 3 }, ports: { route: 'retry' },
        memo: { cron: { kind: 'trigger', out: 'fired @ 02:00 · cron_2am' }, fetch_repos: { kind: 'action', out: '51 commits · 6 repos' }, fetch_issues: { kind: 'action', out: '23 open issues' },
          summarize: { kind: 'agent', loop: [{ i: 0, status: 'completed', out: 'draft v1 · 缺数据引用' }, { i: 1, status: 'completed', out: 'draft v2 · 质量 0.72 < 0.8' }, { i: 2, status: 'running', out: '生成中…' }] },
          route: { kind: 'control', out: '{ __port: "retry" }   // 质量 0.72 < 0.8 → 回炉' } } },
    ] },
    '竞品监控流': { cur: 0, runs: [
      { id: 'fr_71d0e8', state: 'running', version: 'v3', trigger: 'webhook_pr', when: '14:22', pos: '4/5',
        nodes: [n('web', 'trigger', 'webhook_pr', 24, 75), n('crawl', 'action', 'fetch_news', 224, 75), n('diff', 'agent', 'summarizer', 424, 75), n('gate', 'approval', 'manager_approval', 624, 75), n('alert', 'action', 'slack_handler', 824, 75)],
        edges: [['web', 'crawl'], ['crawl', 'diff'], ['diff', 'gate'], ['gate', 'alert']], loopbacks: [], vb: [1012, 200],
        st: { web: 'completed', crawl: 'completed', diff: 'completed', gate: 'parked', alert: 'future' },
        taken: ['web>crawl', 'crawl>diff', 'diff>gate'], ghost: [], live: null, iters: {}, ports: {},
        memo: { web: { kind: 'trigger', out: 'PR #482 opened' }, crawl: { kind: 'action', out: '竞品官网 3 处改动' }, diff: { kind: 'agent', out: '定价页降价 12%，新增企业版' },
          gate: { kind: 'approval', parked: true, prompt: '检测到竞品定价页改动（降价 12% + 新增企业版）。是否推送告警给负责人并触发应对评审？', ddl: '自动驳回 22h', form: 'manager_approval v4' } } },
    ] },
    '账单对账流': { cur: 0, runs: [
      { id: 'fr_5b3c10', state: 'failed', version: 'v3', trigger: null, when: '13:40', pos: '2/4', replay: 1,
        nodes: [n('in', 'trigger', 'webhook', 24, 75), n('extract', 'action', 'process_invoice', 224, 75), n('match', 'action', 'db_pool', 424, 75), n('post', 'action', 'db_pool', 624, 75)],
        edges: [['in', 'extract'], ['extract', 'match'], ['match', 'post']], loopbacks: [], vb: [800, 200],
        st: { in: 'completed', extract: 'failed', match: 'future', post: 'future' },
        taken: ['in>extract'], ghost: [], live: null, iters: {}, ports: {},
        memo: { in: { kind: 'trigger', out: '手动触发 · 12 张发票' }, extract: { kind: 'action', error: 'SandboxError: 依赖物化超时（pdfplumber 拉取 38s > 30s 限）' } } },
    ] },
    '日报汇总流': { cur: 0, runs: [
      { id: 'fr_18aa3c', state: 'completed', version: 'v5', trigger: 'cron_9am', when: '今天 09:02', pos: '4/4',
        nodes: [n('cron', 'trigger', 'cron_9am', 24, 75), n('gather', 'action', 'fetch_news', 224, 75), n('write', 'agent', 'summarizer', 424, 75), n('send', 'action', 'slack_handler', 624, 75)],
        edges: [['cron', 'gather'], ['gather', 'write'], ['write', 'send']], loopbacks: [], vb: [800, 200],
        st: { cron: 'completed', gather: 'completed', write: 'completed', send: 'completed' },
        taken: ['cron>gather', 'gather>write', 'write>send'], ghost: [], live: null, iters: {}, ports: {},
        memo: { cron: { kind: 'trigger', out: 'fired @ 09:00' }, gather: { kind: 'action', out: '昨日 8 条动态' }, write: { kind: 'agent', out: '日报「6 月 14 日」· 412 词' }, send: { kind: 'action', out: '已发 #daily' } } },
    ] },
  };
  // idle 类（无 live run）给一条最近完成的小 run，避免空点击
  const idleRun = (id, when) => ({ id, state: 'completed', version: 'v1', trigger: null, when, pos: '3/3',
    nodes: [n('start', 'trigger', 'manual', 24, 75), n('do', 'action', 'fn', 280, 75), n('done', 'action', 'fn', 536, 75)],
    edges: [['start', 'do'], ['do', 'done']], loopbacks: [], vb: [760, 200],
    st: { start: 'completed', do: 'completed', done: 'completed' }, taken: ['start>do', 'do>done'], ghost: [], live: null, iters: {}, ports: {},
    memo: { start: { kind: 'trigger', out: '手动' }, do: { kind: 'action', out: 'ok' }, done: { kind: 'action', out: 'ok' } } });
  WF['Slack 通知流'] = { cur: 0, runs: [idleRun('fr_0c91', '昨天')] };
  WF['PDF 批处理流'] = { cur: 0, runs: [idleRun('fr_0a47', 'Jun 8')] };
  WF['旧版迁移流'] = { cur: 0, runs: [idleRun('fr_0019', 'Jun 1')] };

  // ===== Conducted Keynote 图渲染 =====
  function nodeSVG(nd, run) {
    const st = run.st[nd.id] || 'future';
    const tier = (st === 'running' || st === 'parked' || st === 'failed') ? 'f0' : (st === 'future' ? 'f2' : 'f1');
    const its = run.iters[nd.id] || 1;
    const stack = its > 1 ? `<g class="rn-stack"><rect x="6" y="6" width="${NW}" height="${NH}" rx="12" opacity=".25"/><rect x="3" y="3" width="${NW}" height="${NH}" rx="12" opacity=".5"/></g>` : '';
    const rR = st === 'future' ? 3 : (st === 'completed' ? 3.5 : 4);
    const refW = Math.min(112, (nd.ref || '').length * 6.6 + 10);
    const refPill = nd.ref ? `<rect class="rn-refbg" x="42" y="34" width="${refW}" height="13" rx="5"/><text class="rn-ref" x="46" y="44">${esc(nd.ref)}</text>` : '';
    const port = run.ports[nd.id] ? `<g class="rn-port"><rect class="rn-port-bg" x="${NW - 64}" y="${NH - 18}" width="58" height="14" rx="4"/><text class="rn-port-tx" x="${NW - 60}" y="${NH - 8}">→ ${esc(run.ports[nd.id])}</text></g>` : '';
    const iter = its > 1 ? `<g class="rn-iter${run.iterFailed && run.iterFailed[nd.id] ? ' failed' : ''}"><rect class="rn-iter-bg" x="8" y="${NH - 18}" width="26" height="14" rx="7"/><text class="rn-iter-tx" x="21" y="${NH - 8}" text-anchor="middle">×${its}</text></g>` : '';
    return `<g class="rn-node ${nd.kind} ${st} ${tier}" data-id="${nd.id}" transform="translate(${nd.x},${nd.y})">
      ${stack}
      <rect class="rn-card" width="${NW}" height="${NH}" rx="12" filter="url(#rnLift)"/>
      <g class="rn-ic" transform="translate(14,21)">${icon(NICON[nd.kind] || nd.kind, 18)}</g>
      <text class="rn-id" x="44" y="27">${esc(nd.id)}</text>
      ${refPill}
      <circle class="rp" cx="${NW - 15}" cy="15" r="${rR}"/>
      ${port}${iter}</g>`;
  }

  function runGraphSVG(run) {
    const by = Object.fromEntries(run.nodes.map(x => [x.id, x]));
    let edges = '';
    run.edges.forEach(([a, b]) => {
      const key = a + '>' + b, cls = run.taken.includes(key) ? 'taken' : (run.ghost.includes(key) ? 'ghost' : 'future');
      const marker = cls === 'ghost' ? '' : (cls === 'future' ? ' marker-end="url(#schArrowFut)"' : ' marker-end="url(#schArrow)"');
      edges += `<path class="rn-edge ${cls}" d="${fwdD(by[a], by[b])}"${marker}/>`;
    });
    (run.loopbacks || []).forEach(([a, b]) => { edges += `<path class="rn-loopback" d="${loopD(by[a], by[b])}" marker-end="url(#schArrow)"/>`; });
    if (run.live) {
      const [a, b] = run.live.split('>');
      const isLoop = (run.loopbacks || []).some(([x, y]) => x === a && y === b);
      const d = isLoop ? loopD(by[a], by[b]) : fwdD(by[a], by[b]);
      edges += `<path class="rn-edge live" d="${d}"/><circle class="rn-comet" r="3"><animateMotion dur="0.9s" repeatCount="indefinite" path="${d}"/></circle>`;
    }
    return `<svg viewBox="0 0 ${run.vb[0]} ${run.vb[1]}" preserveAspectRatio="xMidYMid meet" role="img">
      <defs>
        <marker id="schArrow" markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto"><path d="M0,0 L7,3.5 L0,7 z"/></marker>
        <marker id="schArrowFut" markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto"><path d="M0,0 L7,3.5 L0,7 z"/></marker>
        <filter id="rnLift" x="-20%" y="-25%" width="140%" height="170%"><feDropShadow dx="0" dy="1" stdDeviation="2" flood-color="#000000" flood-opacity="0.08"/></filter>
      </defs>
      <g class="rn-edges">${edges}</g>
      ${run.nodes.map(nd => nodeSVG(nd, run)).join('')}</svg>`;
  }

  // ===== 驾驶舱渲染 =====
  const ST_TX = { running: '运行中', completed: '已完成', failed: '失败', cancelled: '已取消', waiting: '等审批' };
  function cockpit(host, name) {
    const wf = WF[name];
    if (!wf) { host.innerHTML = `<div class="sch-col" style="color:var(--ink-3);padding-top:80px;text-align:center">该 workflow 暂无运行记录</div>`; return; }
    const run = wf.runs[wf.cur];
    const stVals = Object.values(run.st);
    const hasParked = stVals.includes('parked'), hasFailed = run.state === 'failed';
    let badge = run.state; if (run.state === 'running' && hasParked) badge = 'waiting';

    // attention chips
    const chips = [];
    Object.entries(run.st).forEach(([id, s]) => {
      if (s === 'parked') { const m = run.memo[id] || {}; chips.push(`<div class="op-chip parked" data-node="${id}"><span class="dot"></span><span class="verb">等你审批</span><span>${esc(id)}</span><span class="ddl">${esc(m.ddl || '')}</span></div>`); }
      if (s === 'failed') { const m = run.memo[id] || {}; chips.push(`<div class="op-chip failed" data-node="${id}"><span class="dot"></span><span class="verb">失败</span><span>${esc(id)}</span><span class="ddl">${esc((m.error || '').split('：')[0].slice(0, 22))}</span><span class="verb">· Retry</span></div>`); }
    });
    const attn = chips.length ? `<div class="op-attn">${chips.join('')}</div>` : '';

    // stuck line
    let stuck = '';
    if (hasParked) { const id = Object.keys(run.st).find(k => run.st[k] === 'parked'); const m = run.memo[id] || {}; stuck = `<div class="sch-stuck"><span class="ico">${icon('shield', 16)}</span>停在 <b style="font-family:var(--mono);margin:0 4px">${esc(id)}</b> · 等待 ${esc(m.form || '审批')}（${esc(m.ddl || '')}）</div>`; }
    else if (hasFailed) { const id = Object.keys(run.st).find(k => run.st[k] === 'failed'); const m = run.memo[id] || {}; stuck = `<div class="sch-stuck fail"><span class="ico">${icon('close', 16)}</span><b style="font-family:var(--mono);margin:0 4px">${esc(id)}</b> 失败 · ${esc(m.error || '')}</div>`; }

    const src = run.trigger ? `<span class="src">${icon('trigger', 13)} 由 ${esc(run.trigger)} 触发</span>` : `<span class="src">${icon('play', 13)} 手动</span>`;
    const retry = run.replay ? `<span class="src">${icon('spin', 13)} Retry #${run.replay}</span>` : '';

    const rail = `<div class="run-rail"><span class="rl-label">运行历史</span>${wf.runs.map((r, i) => {
      const cls = r.state === 'running' ? (Object.values(r.st).includes('parked') ? 'wait' : 'run') : (r.state === 'failed' ? 'err' : (r.state === 'cancelled' ? '' : 'done'));
      const live = (i === wf.runs.length - 1 && r.state === 'running') ? `<span class="live-tag">LIVE</span>` : '';
      return `<button class="run-chip ${cls}${i === wf.cur ? ' on' : ''}" data-run="${i}"><span class="dot"></span><span class="rc-id">${esc(r.id)}</span><span class="rc-t">${esc(r.when)}</span>${live}</button>`;
    }).join('')}</div>`;

    host.innerHTML = `<div class="sch-col">
      ${attn}
      <div class="sch-cockpit">
        <div class="sch-runhead">
          <span class="sch-badge ${badge}"><span class="dot"></span>${ST_TX[badge]}</span>
          <span class="rid">${esc(run.id)}</span>
          <span class="pos">node <b>${esc(run.pos)}</b></span>
          <span class="ver">${esc(run.version)} pinned</span>
          ${src}${retry}
          <span class="gap"></span>
          <span class="when">${esc(run.when)}</span>
          <span class="sch-toggle"><button class="on">图</button><button>列表</button></span>
        </div>
        ${stuck}
        <div class="sch-graph" id="schGraph">${runGraphSVG(run)}</div>
        ${rail}
      </div>
    </div>`;

    // 节点点击 → 右岛
    host.querySelectorAll('.rn-node').forEach(g => g.onclick = () => SchedulerNode.open(run, g.dataset.id));
    // attention chip → 滚到/打开该节点
    host.querySelectorAll('.op-chip').forEach(c => c.onclick = () => SchedulerNode.open(run, c.dataset.node));
    // run-rail 切换
    host.querySelectorAll('.run-chip').forEach(b => b.onclick = () => { wf.cur = +b.dataset.run; cockpit(host, name); });
    // 图/列表 toggle（列表态本轮占位，主推图）
    host.querySelectorAll('.sch-toggle button').forEach(b => b.onclick = () => { host.querySelectorAll('.sch-toggle button').forEach(x => x.classList.remove('on')); b.classList.add('on'); });
  }

  // ===== 右岛 = 节点深读抽屉 =====
  const SchedulerNode = (function () {
    let el = null;
    function ensure() { if (el && document.body.contains(el)) return el; el = document.createElement('aside'); el.className = 'sch-aside'; el.setAttribute('data-ocean-right', 'scheduler'); Shell.body.appendChild(el); return el; }
    const ST = { completed: '已完成', failed: '失败', parked: '等审批', running: '运行中', future: '未运行' };
    function bodyFor(run, id) {
      const m = run.memo[id] || {}, st = run.st[id] || 'future';
      if (m.loop) {  // loop 节点 → 逐迭代 stepper（UNIQUE(node,iteration) n 行 n 结果）
        return `<div class="sa-sec"><label>逐迭代结果 · ×${m.loop.length}</label>${m.loop.map(it =>
          `<div class="sa-iter ${it.status === 'failed' ? 'failed' : 'ok'}${it.status === 'running' ? ' on' : ''}"><span class="n">iter ${it.i}</span><span class="mk">${it.status === 'failed' ? icon('close', 13) : (it.status === 'running' ? icon('spin', 13) : icon('check', 13))}</span><span class="ot">${esc(it.out)}</span></div>`).join('')}</div>`;
      }
      if (m.parked) {  // parked → 内联 :decide（retarget 自 chat 审批卡，绝非危险闸）
        return `<div class="sa-sec"><label>待你决定 · :decide</label><div class="sa-inbox">
          <div class="ib-prompt">${esc(m.prompt)}</div><div class="ib-ddl">${esc(m.ddl || '')} · 表单 ${esc(m.form || '')}</div>
          <textarea class="ib-reason" rows="2" placeholder="理由（可选）…"></textarea>
          <div class="ib-actions"><button class="ibtn-decide yes" data-d="yes">通过</button><button class="ibtn-decide no" data-d="no">驳回</button></div></div></div>`;
      }
      if (m.error) return `<div class="sa-sec"><label>错误</label><div class="sa-val mono" style="color:var(--danger)">${esc(m.error)}</div></div>`;
      if (m.decision) return `<div class="sa-sec"><label>审批结果</label><div class="sa-kv"><span class="k">decision</span><span class="v" style="color:${m.decision === 'yes' ? 'var(--ok)' : 'var(--danger)'}">${esc(m.decision)}</span></div><div class="sa-kv"><span class="k">reason</span><span class="v">${esc(m.reason || '—')}</span></div></div>`;
      if (st === 'future') return `<div class="sa-sec"><label>结果</label><div class="sa-val" style="color:var(--ink-3)">尚未运行</div></div>`;
      return `<div class="sa-sec"><label>记忆化结果 · result</label><div class="sa-val mono">${esc(m.out || '—')}</div></div>`;
    }
    function open(run, id) {
      ensure(); el.classList.add('show');
      const nd = run.nodes.find(x => x.id === id) || {}, st = run.st[id] || 'future';
      el.innerHTML = `<div class="sa-head">
        <span class="sa-ico">${icon(NICON[nd.kind] || 'action', 17)}</span>
        <span class="ht"><b>${esc(id)}</b><span class="sub">${esc(nd.kind || '')} · <span class="sa-state ${st}"><span class="dot"></span>${ST[st]}</span></span></span>
        <button class="sa-close">${icon('close', 16)}</button></div>
        <div class="sa-body">
          ${nd.ref ? `<div class="sa-sec"><label>引用实体</label><div class="sa-kv"><span class="k">ref</span><span class="v">${esc(nd.ref)} <span style="color:var(--ink-3)">@${esc(run.version)}</span></span></div></div>` : ''}
          ${bodyFor(run, id)}
        </div>`;
      el.querySelector('.sa-close').onclick = hide;
      const inbox = el.querySelector('.sa-inbox');
      if (inbox) inbox.querySelectorAll('.ibtn-decide').forEach(b => b.onclick = () => {
        inbox.innerHTML = `<div class="sa-settled"><span class="ico">${icon('check', 15)}</span>已${b.dataset.d === 'yes' ? '通过' : '驳回'} · 运行继续</div>`;
      });
    }
    const hide = () => { if (el) el.classList.remove('show'); };
    return { open, hide };
  })();
  window.SchedulerNode = SchedulerNode;

  // ===== 注册 + 装配 =====
  Shell.registerOcean('scheduler', {
    crumb: '运行',
    build(sea) {
      sea.innerHTML = `<div class="sch"><div class="sch-scroll scroll-fade" id="schScroll"><div id="schStage"></div></div></div>`;
      const stage = $('#schStage', sea);
      Shell.openWorkflow = name => { cockpit(stage, name); const sc = $('#schScroll'); if (sc) sc.scrollTop = 0; };
      cockpit(stage, '研报抓取流');   // 首屏开 live 英雄
    },
  });
})();

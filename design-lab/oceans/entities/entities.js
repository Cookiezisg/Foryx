/* Forgify design-lab — 实体海洋编排（单独，一人负责整个 oceans/entities/ 文件夹）。
   注册进外壳：Shell.registerOcean('entities', { crumb, build(sea) })，渲染实体工作台到 #sea。
   选中通道：暴露 Shell.openEntity(id)，侧栏行点击调它（沿用 documents 的 Shell.openDocument 范式；侧栏存在才调、优雅降级）。
   一套脊柱 / 四型分魂：function·handler·agent·workflow 各渲染各的体；graph parts/连接/技能给轻量但自洽的详情。
   依赖：shared/icons.js（icon）+ shared/shell.js（Shell.sea/crumb/headExtra）。样式在同目录 entities.css。
   注：纯静态示意数据，绝不连后端；字段集镜像 chat 海洋右岛实体卡 + 后端域契约。 */
(function () {
  const $ = (s, r = document) => r.querySelector(s);

  // 类型 → 图标 / 标签 / 执行动词（N5：:run/:call/:invoke/:trigger 是标准执行动词）
  const KIND = {
    function: { icon: 'function', label: 'Function', verb: 'Run',     vico: 'play' },
    handler:  { icon: 'handler',  label: 'Handler',  verb: 'Call',    vico: 'play' },
    agent:    { icon: 'agent',    label: 'Agent',    verb: 'Invoke',  vico: 'play' },
    workflow: { icon: 'workflow', label: 'Workflow', verb: 'Trigger', vico: 'zap'  },
    trigger:  { icon: 'trigger',  label: 'Trigger',  verb: 'Fire',    vico: 'play' },
    control:  { icon: 'control',  label: 'Control'  },
    approval: { icon: 'shield',   label: 'Approval' },
    mcp:      { icon: 'mcp',      label: 'MCP server', verb: 'Reconnect', vico: 'spin' },
    skill:    { icon: 'skill',    label: 'Skill'    },
  };
  const ST = { done: '就绪', run: '运行中', wait: '需处理', err: '失败', idle: '闲置' };
  // 图节点 5 类 kind → 图标 key（approval 复用 shield；action 是执行节点）
  const NICON = { trigger: 'trigger', action: 'action', agent: 'agent', control: 'control', approval: 'shield' };

  // ===== 示意数据（键 = 实体名，与侧栏 entities.js 的 ENTS 同名对齐）=====
  const D = {
    // —— Functions（确定性变换：代码 + I/O + deps）——
    process_invoice: {
      kind: 'function', version: 5, status: 'done', runs: 1284,
      desc: '解析上传的发票 PDF / 图片，抽取结构化字段（供应商、金额、税号、行项目），校验后返回 JSON。',
      lang: 'python 3.12',
      inputs: [['file', 'bytes'], ['currency', 'string?']],
      output: ['Invoice', 'vendor · total · tax_id · line_items[]'],
      code: `def process_invoice(file: bytes, currency: str = "USD") -> Invoice:
    doc = pdfplumber.open(io.BytesIO(file))
    text = "\\n".join(p.extract_text() for p in doc.pages)
    fields = extract_fields(text)          # 正则 + 版面启发式
    inv = Invoice.model_validate(fields)   # pydantic 校验
    inv.currency = currency
    return inv`,
      deps: ['pdfplumber', 'pydantic', 'regex'],
      env: 'ready', lastRun: '2 小时前 · 成功',
    },
    fetch_news: {
      kind: 'function', version: 2, status: 'run', runs: 96,
      desc: '按关键词拉取多源 RSS / 新闻，去重归并后返回结构化条目列表。',
      lang: 'python 3.12',
      inputs: [['topics', 'string[]'], ['since', 'datetime?']],
      output: ['NewsItem[]', 'title · url · source · published_at'],
      code: `def fetch_news(topics: list[str], since=None) -> list[NewsItem]:
    items = []
    for feed in resolve_feeds(topics):
        items += parse_feed(feed, since)
    return dedupe(items, key=lambda i: i.url)`,
      deps: ['requests', 'feedparser'],
      env: 'syncing', lastRun: '运行中…',
    },
    parse_pdf: {
      kind: 'function', version: 1, status: 'err', runs: 7,
      desc: '把 PDF 文本层抽成纯文本块（按段落 / 页码）。当前依赖物化失败。',
      lang: 'python 3.12',
      inputs: [['file', 'bytes']],
      output: ['TextBlock[]', 'page · text'],
      code: `def parse_pdf(file: bytes) -> list[TextBlock]:
    return [TextBlock(page=i, text=p.extract_text())
            for i, p in enumerate(pdfplumber.open(file).pages)]`,
      deps: ['pdfplumber'],
      env: 'failed', lastRun: '失败 · 沙箱依赖物化超时',
    },

    // —— Handlers（常驻有状态集成适配：类 + 方法目录 + config + 健康）——
    slack_handler: {
      kind: 'handler', version: 3, status: 'done', runs: 512,
      desc: '常驻 Slack 连接：发消息、建频道、上传文件、监听事件，维护长连 socket 会话。',
      lang: 'python 3.12',
      classCode: `class SlackHandler(Handler):
    def __init__(self, bot_token, app_token, default_channel="#ops"):
        self.client = WebClient(bot_token)
        self.socket = SocketModeClient(app_token)`,
      methods: [['post_message', '(channel, text)'], ['create_channel', '(name)'], ['upload_file', '(path, channel)'], ['on_event', '(type, cb)']],
      initArgs: [['bot_token', null, true], ['app_token', null, true], ['default_channel', '#ops', false]],
      configState: 'ready', life: 'active', env: 'ready',
    },
    db_pool: {
      kind: 'handler', version: 2, status: 'wait', runs: 0,
      desc: 'PostgreSQL 连接池：查询 / 事务 / 批量写。缺少密码，尚未上线。',
      lang: 'python 3.12',
      classCode: `class DbPool(Handler):
    def __init__(self, dsn, password, pool_size=10):
        self.pool = create_pool(dsn, password, pool_size)`,
      methods: [['query', '(sql, *params)'], ['execute', '(sql, *params)'], ['transaction', '()']],
      initArgs: [['dsn', 'postgres://…/forgify', false], ['password', null, true], ['pool_size', '10', false]],
      configState: 'partially_configured', life: 'inactive', env: 'pending',
    },

    // —— Agents（LLM 循环：system prompt + model + 工具 + skill + knowledge）——
    research_agent: {
      kind: 'agent', version: 2, status: 'idle', runs: 38,
      desc: '深度调研代理：检索、交叉验证、带引用综述，能写回记忆供后续会话复用。',
      model: 'claude-opus-4-8',
      system: `你是一名严谨的研究员。对每个论断都要交叉验证至少两个独立来源，输出带引用编号的综述。\n不确定时显式标注「待证实」，绝不杜撰来源。优先用 web_search 找一手资料，再用 fetch_url 取全文。`,
      tools: ['web_search', 'fetch_url', 'read_document', 'write_memory', 'cite'],
      skill: 'deep_research', knowledge: ['竞品库', '行业报告 2026'], maxSteps: 40,
    },
    summarizer: {
      kind: 'agent', version: 4, status: 'idle', runs: 211,
      desc: '把长文 / 会话 / 运行日志压成结构化摘要（要点 + 风险 + 下一步）。',
      model: 'claude-sonnet-4-6',
      system: `把输入压成三段：① 3–5 条要点 ② 风险或反对意见 ③ 建议的下一步。\n保留关键数字与专有名词，删冗余修饰。输出 markdown。`,
      tools: ['read_document', 'write_memory'],
      skill: null, knowledge: [], maxSteps: 12,
    },

    // —— Workflows（图编排 + durable 执行：节点图 + lifecycle + concurrency + triggers）——
    nightly_report: {
      kind: 'workflow', version: 8, status: 'run', runs: 173,
      desc: '每晚汇总仓库与议题动态，交研究代理综述，按规模路由后推送简报、必要时请负责人审批。',
      life: 'active', concurrency: 'serial', triggers: ['cron_2am'], running: 'summarize',
      nodes: [
        { id: 'cron', kind: 'trigger', ref: 'cron_2am', x: 20, y: 113 },
        { id: 'fetch_repos', kind: 'action', ref: 'fetch_news', x: 210, y: 30 },
        { id: 'fetch_issues', kind: 'action', ref: 'parse_pdf', x: 210, y: 196 },
        { id: 'summarize', kind: 'agent', ref: 'research_agent', x: 400, y: 113 },
        { id: 'route', kind: 'control', ref: 'route_by_amount', x: 590, y: 113 },
        { id: 'publish', kind: 'action', ref: 'slack_handler', x: 780, y: 30 },
        { id: 'notify_lead', kind: 'approval', ref: 'manager_approval', x: 780, y: 196 },
      ],
      edges: [['cron', 'fetch_repos'], ['cron', 'fetch_issues'], ['fetch_repos', 'summarize'], ['fetch_issues', 'summarize'], ['summarize', 'route'], ['route', 'publish'], ['route', 'notify_lead']],
      vb: [952, 280],
    },
    invoice_flow: {
      kind: 'workflow', version: 3, status: 'wait', runs: 64,
      desc: '发票入账流：抽取 → 金额超阈请经理审批 → 入账。当前一笔卡在审批。',
      life: 'active', concurrency: 'serial', triggers: ['webhook_pr'],
      attention: '节点 approve 等待 manager_approval 审批（已等 3 小时）',
      nodes: [
        { id: 'in', kind: 'trigger', ref: 'webhook', x: 20, y: 93 },
        { id: 'extract', kind: 'action', ref: 'process_invoice', x: 230, y: 93 },
        { id: 'approve', kind: 'approval', ref: 'manager_approval', x: 440, y: 93, parked: true },
        { id: 'post', kind: 'action', ref: 'db_pool', x: 650, y: 93 },
      ],
      edges: [['in', 'extract'], ['extract', 'approve'], ['approve', 'post']],
      vb: [822, 200],
    },
    archive_cleanup: {
      kind: 'workflow', version: 1, status: 'idle', runs: 12,
      desc: '定期扫描过期对象并归档清理。',
      life: 'inactive', concurrency: 'serial', triggers: [],
      nodes: [
        { id: 'cron', kind: 'trigger', ref: 'cron_weekly', x: 40, y: 73 },
        { id: 'scan', kind: 'action', ref: 'list_stale', x: 280, y: 73 },
        { id: 'purge', kind: 'action', ref: 'archive', x: 520, y: 73 },
      ],
      edges: [['cron', 'scan'], ['scan', 'purge']],
      vb: [712, 160],
    },

    // —— Graph parts / 连接 / 技能（轻量但自洽）——
    cron_2am: { kind: 'trigger', version: 1, status: 'run', desc: '每天 02:00（本地时区）触发，启动 nightly_report。',
      kv: [['源类型', 'cron'], ['表达式', '0 2 * * *'], ['绑定工作流', 'nightly_report'], ['上次 fire', '今天 02:00 · 7 节点全绿'], ['启用', 'true']] },
    webhook_pr: { kind: 'trigger', version: 1, status: 'idle', desc: 'GitHub PR webhook：收到 opened / synchronize 事件时触发。',
      kv: [['源类型', 'webhook'], ['路径', '/hooks/pr'], ['条件 (CEL)', 'event in ["opened","synchronize"]'], ['绑定工作流', '— 未绑定 —'], ['上次 fire', '从未']] },
    route_by_amount: { kind: 'control', version: 2, status: 'idle', desc: 'CEL 路由：按金额分流到审批或自动入账。',
      kv: [['类型', 'switch (CEL)'], ['分支 1', 'amount > 1000 → approve'], ['分支 2', 'amount <= 1000 → auto_post'], ['默认', 'auto_post']] },
    manager_approval: { kind: 'approval', version: 4, status: 'idle', desc: '人在环审批闸：金额超阈需经理放行。',
      kv: [['审批人', 'Sun Weilin · Finance Lead'], ['策略', 'any 1 通过'], ['超时', '24h → 驳回'], ['被引用', 'invoice_flow · nightly_report']] },
    github_mcp: { kind: 'mcp', status: 'done', desc: 'GitHub MCP 连接器：把仓库 / PR / issue 能力作为工具暴露给代理。',
      kv: [['传输', 'stdio'], ['状态', 'connected'], ['鉴权', 'PAT（已配置）']],
      tools: ['list_repos', 'get_pr', 'create_issue', 'merge_pr', 'search_code'] },
    linear_mcp: { kind: 'mcp', status: 'wait', desc: 'Linear MCP 连接器：任务 / 周期能力。需重新授权。',
      kv: [['传输', 'http'], ['状态', 'auth required'], ['鉴权', 'OAuth（已过期）']],
      tools: ['list_issues', 'create_issue', 'update_issue'] },
    deep_research: { kind: 'skill', desc: '深度调研 playbook：指导代理如何分解问题、检索、交叉验证、引用。',
      allowed: ['web_search', 'fetch_url', 'cite'],
      body: `1. 把问题拆成 3–5 个可独立检索的子问题。\n2. 每个子问题先 web_search 找一手来源，再 fetch_url 取全文。\n3. 交叉验证：同一论断至少两个独立来源，冲突则并列呈现。\n4. 输出带引用编号 [n] 的综述，附来源清单。` },
    pdf_extract: { kind: 'skill', desc: '从 PDF 抽取并清洗文本的 playbook。',
      allowed: ['parse_pdf'],
      body: `1. 调 parse_pdf 取文本块。\n2. 合并跨页断句、去页眉页脚。\n3. 按标题层级重组为结构化大纲。` },
  };

  const esc = s => String(s).replace(/[&<>]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]));
  const tags = (arr, empty = '— 无 —') => (arr && arr.length)
    ? `<div class="eo-tags">${arr.map(t => `<span class="eo-tag">${esc(t)}</span>`).join('')}</div>`
    : `<span class="eo-empty-note">${empty}</span>`;
  const ENV = { pending: ['pending', '排队'], syncing: ['syncing', '物化中…'], ready: ['ready', '就绪'], failed: ['failed', '失败'] };
  const CFG = { unconfigured: ['pending', '未配置'], partially_configured: ['syncing', '部分配置'], ready: ['ready', '就绪'] };
  const LIFE = { active: ['active', '运行中'], draining: ['draining', '收尾中'], inactive: ['inactive', '未上线'] };
  const badge = (map, s) => { const [c, t] = map[s] || Object.values(map)[0]; return `<span class="eo-badge ${c}"><span class="dot"></span>${t}</span>`; };
  const life = s => { const [c, t] = LIFE[s] || LIFE.inactive; return `<span class="eo-life ${c}">${c === 'active' ? '<span class="dot"></span>' : ''}${t}</span>`; };
  const kvRows = arr => arr.map(([k, v]) => `<div class="eo-kv"><span class="k">${esc(k)}</span><span class="v">${esc(v)}</span></div>`).join('');

  // ===== 四型分魂：体 =====
  function body(a) {
    if (a.kind === 'function') return `
      <div class="eo-desc">${esc(a.desc)}</div>
      <div class="eo-sec"><label>Code</label>
        <div class="eo-code"><span class="lang">${esc(a.lang)}</span>${esc(a.code)}</div></div>
      <div class="eo-grid2">
        <div class="eo-sec"><label>Inputs</label><div class="eo-box"><div class="eo-rows">
          ${a.inputs.map(([n, t]) => `<div class="eo-row"><span class="nm">${esc(n)}</span><span class="eo-io-t">${esc(t)}</span></div>`).join('')}</div></div></div>
        <div class="eo-sec"><label>Output</label><div class="eo-box"><div class="eo-rows">
          <div class="eo-row"><span class="nm">${esc(a.output[0])}</span></div>
          <div class="eo-row"><span class="args">${esc(a.output[1])}</span></div></div></div></div>
      </div>
      <div class="eo-grid2">
        <div class="eo-sec"><label>Dependencies <span class="ct">${a.deps.length}</span></label><div class="eo-box">${tags(a.deps)}</div></div>
        <div class="eo-sec"><label>Env status</label><div class="eo-box">${badge(ENV, a.env)}<span class="eo-note">上次：${esc(a.lastRun)}</span></div></div>
      </div>`;

    if (a.kind === 'handler') return `
      <div class="eo-desc">${esc(a.desc)}</div>
      <div class="eo-sec"><label>Assembled class</label>
        <div class="eo-code"><span class="lang">${esc(a.lang)}</span>${esc(a.classCode)}</div></div>
      <div class="eo-sec"><label>Methods <span class="ct">catalog · ${a.methods.length}</span></label><div class="eo-box"><div class="eo-rows">
        ${a.methods.map(([n, sig]) => `<div class="eo-row"><span class="nico">${icon('action', 15)}</span><span class="nm">${esc(n)}</span><span class="args">${esc(sig)}</span></div>`).join('')}</div></div></div>
      <div class="eo-sec"><label>Init args</label><div class="eo-box">
        ${a.initArgs.map(([n, v, sens]) => `<div class="eo-kv"><span class="k">${esc(n)}</span><span class="v">${sens ? '<span class="eo-mask">••••••••</span>' : esc(v || '—')}</span></div>`).join('')}</div></div>
      <div class="eo-grid2">
        <div class="eo-sec"><label>Config state</label><div class="eo-box">${badge(CFG, a.configState)}<span class="eo-note">改 config 触发重启</span></div></div>
        <div class="eo-sec"><label>Lifecycle / Env</label><div class="eo-box">${life(a.life)} &nbsp; ${badge(ENV, a.env)}</div></div>
      </div>`;

    if (a.kind === 'agent') return `
      <div class="eo-desc">${esc(a.desc)}</div>
      <div class="eo-sec"><label>System prompt</label><div class="eo-box"><div class="eo-prose">${esc(a.system)}</div></div></div>
      <div class="eo-grid2">
        <div class="eo-sec"><label>Model</label><div class="eo-box"><span class="eo-tag">${esc(a.model)}</span></div></div>
        <div class="eo-sec"><label>Max steps</label><div class="eo-box"><span class="eo-tag">${a.maxSteps}</span></div></div>
      </div>
      <div class="eo-sec"><label>Mounted tools <span class="ct">${a.tools.length}</span></label><div class="eo-box">${tags(a.tools)}</div></div>
      <div class="eo-grid2">
        <div class="eo-sec"><label>Skill <span class="ct">0–1</span></label><div class="eo-box">${a.skill ? `<span class="eo-tag"><span class="ico">${icon('skill', 13)}</span>${esc(a.skill)}</span>` : '<span class="eo-empty-note">— 未挂 —</span>'}</div></div>
        <div class="eo-sec"><label>Knowledge</label><div class="eo-box">${tags(a.knowledge)}</div></div>
      </div>`;

    if (a.kind === 'workflow') return `
      <div class="eo-desc">${esc(a.desc)}</div>
      ${a.attention ? `<div class="eo-attn"><span class="ico">${icon('shield', 16)}</span>${esc(a.attention)}</div>` : ''}
      <div class="eo-sec"><label>Graph <span class="ct">${a.nodes.length} 节点 · ${a.edges.length} 边</span></label>
        <div class="eo-graph">${graphSVG(a)}</div></div>
      <div class="eo-grid2">
        <div class="eo-sec"><label>Lifecycle</label><div class="eo-box">${life(a.life)}</div></div>
        <div class="eo-sec"><label>Concurrency</label><div class="eo-box"><span class="eo-tag">${esc(a.concurrency)}</span></div></div>
      </div>
      <div class="eo-sec"><label>Triggers <span class="ct">${a.triggers.length}</span></label><div class="eo-box">${a.triggers.length ? `<div class="eo-tags">${a.triggers.map(t => `<span class="eo-tag"><span class="ico">${icon('trigger', 13)}</span>${esc(t)}</span>`).join('')}</div>` : '<span class="eo-empty-note">— 无触发器，仅手动 / 被调用 —</span>'}</div></div>`;

    if (a.kind === 'mcp') return `
      <div class="eo-desc">${esc(a.desc)}</div>
      <div class="eo-sec"><label>Connection</label><div class="eo-box">${kvRows(a.kv)}</div></div>
      <div class="eo-sec"><label>Exposed tools <span class="ct">${a.tools.length}</span></label><div class="eo-box">${tags(a.tools)}</div></div>`;

    if (a.kind === 'skill') return `
      <div class="eo-desc">${esc(a.desc)}</div>
      <div class="eo-sec"><label>Allowed tools <span class="ct">${a.allowed.length}</span></label><div class="eo-box">${tags(a.allowed)}</div></div>
      <div class="eo-sec"><label>Playbook</label><div class="eo-box"><div class="eo-prose">${esc(a.body)}</div></div></div>`;

    // trigger / control / approval：键值详情
    return `
      <div class="eo-desc">${esc(a.desc)}</div>
      <div class="eo-sec"><label>Definition</label><div class="eo-box">${kvRows(a.kv)}</div></div>`;
  }

  // ===== Workflow 图：全 SVG（节点 + 边 + 自适应缩放）=====
  function graphSVG(wf) {
    const [W, H] = wf.vb, NW = 152, NH = 54;
    const by = Object.fromEntries(wf.nodes.map(n => [n.id, n]));
    const edge = ([a, b]) => {
      const s = by[a], t = by[b], x1 = s.x + NW, y1 = s.y + NH / 2, x2 = t.x, y2 = t.y + NH / 2, mx = (x1 + x2) / 2;
      return `<path d="M${x1},${y1} C${mx},${y1} ${mx},${y2} ${x2 - 7},${y2}" marker-end="url(#eoArrow)"/>`;
    };
    const node = n => {
      const cls = n.id === wf.running ? ' run' : (n.parked ? ' parked' : '');
      const live = (n.id === wf.running || n.parked) ? `<circle class="rp" cx="${NW - 14}" cy="15" r="4"/>` : '';
      return `<g class="eo-gn${cls}" transform="translate(${n.x},${n.y})">
        <rect width="${NW}" height="${NH}" rx="12"/>
        <g class="ic" transform="translate(13,${NH / 2 - 9})">${icon(NICON[n.kind] || n.kind, 18)}</g>
        <text class="nid" x="40" y="${NH / 2 - 2}">${esc(n.id)}</text>
        <text class="nkind" x="40" y="${NH / 2 + 13}">${esc(n.ref || n.kind)}</text>${live}</g>`;
    };
    return `<svg viewBox="0 0 ${W} ${H}" preserveAspectRatio="xMidYMid meet" role="img">
      <defs><marker id="eoArrow" markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto"><path d="M0,0 L7,3.5 L0,7 z"/></marker></defs>
      <g class="eo-edges">${wf.edges.map(edge).join('')}</g>
      ${wf.nodes.map(node).join('')}</svg>`;
  }

  // ===== 渲染：脊柱 + tab + 体 =====
  function detail(stage, id) {
    const a = D[id];
    if (!a) return empty(stage);
    const k = KIND[a.kind] || KIND.function;
    const sub = [`${k.label}`];
    if (a.version != null) sub.push(`<span class="eo-ver">v${a.version}</span>`);
    sub.push(`<span class="eo-st ${a.status || 'idle'}"><span class="dot"></span>${ST[a.status] || '闲置'}</span>`);
    const exec = k.verb ? `<button class="eo-btn primary">${icon(k.vico, 15)}${k.verb}</button>` : '';
    stage.innerHTML = `
      <div class="eo-col eo-morph">
        <div class="eo-head">
          <span class="eo-etype">${icon(k.icon, 22)}</span>
          <span class="eo-htext"><h1>${esc(a.name || id)}</h1><span class="eo-sub">${sub.join('<span class="sep">·</span>')}</span></span>
          <span class="eo-actions">${exec}
            <button class="eo-btn">${icon('spark', 14)}Iterate</button>
            <button class="eo-btn ic" title="更多">${icon('more', 16)}</button></span>
        </div>
        <div class="eo-tabs"><button class="on">概览</button><button>版本</button><button>运行</button><button>迭代</button></div>
        ${body(Object.assign({ name: id }, a))}
        <div class="eo-foot"><span class="mono">${a.kind}_${(id + '________').slice(0, 8)}</span>${a.runs != null ? ` · ${a.runs} runs` : ''}<span class="gap"></span><span class="lnk">历史版本</span></div>
      </div>`;
    stage.querySelectorAll('.eo-tabs button').forEach(b => b.onclick = () => {
      stage.querySelectorAll('.eo-tabs button').forEach(x => x.classList.remove('on')); b.classList.add('on');
    });
    const sc = $('#eoScroll'); if (sc) sc.scrollTop = 0;
  }

  function empty(stage) {
    const cnt = k => Object.values(D).filter(e => e.kind === k).length;
    const stat = (k, label) => `<span class="eo-stat"><span class="ico">${icon(KIND[k].icon, 16)}</span><b>${cnt(k)}</b><span>${label}</span></span>`;
    stage.innerHTML = `
      <div class="eo-empty"><div class="in eo-morph">
        <div class="gico">${icon('entities', 26)}</div>
        <h2>四项全能实体</h2>
        <p>Function · Handler · Agent · Workflow——以及它们组成图的触发器、控制、审批、连接器与技能。<br>从左侧选一个实体，在这片海面查看与编辑。</p>
        <div class="eo-stats">${stat('function', 'Functions')}${stat('handler', 'Handlers')}${stat('agent', 'Agents')}${stat('workflow', 'Workflows')}</div>
      </div></div>`;
  }

  // ===== 注册 + 装配 =====
  Shell.registerOcean('entities', {
    crumb: '实体',
    build(sea) {
      sea.innerHTML = `<div class="eo"><div class="eo-scroll scroll-fade" id="eoScroll"><div id="eoStage"></div></div></div>`;
      const stage = $('#eoStage', sea);
      // 选中通道：侧栏行点击调它（沿用 documents 的 Shell.openDocument 范式）
      Shell.openEntity = id => detail(stage, id);
      // 首屏自动开一个，立即有内容（与侧栏 process_invoice on:true 对齐）
      detail(stage, 'process_invoice');
    },
  });
})();

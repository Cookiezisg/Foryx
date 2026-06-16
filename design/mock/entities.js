/* Foryx demo — 实体示意数据层（DTO 镜像后端 references/，camelCase）。绝不连后端。
   9 类全覆盖：Quadrinity(function/handler/agent/workflow) + 图节点(trigger/control/approval) + 连接(mcp) + 技能(skill)。
   每条 = 一个实体的完整字段集（per-kind 字段差异即后端各域 DTO 差异）。键 = 实体名（= 侧栏 data-id / Intent.id）。
   消费者：features/entities（海面详情 + 侧栏树）。状态枚举对齐 STATE_MODEL：实体 5 态 done/run/wait/err/idle + trigger listening；
   env=ENV(pending/syncing/ready/failed) · cfg=CFG(ready/partially_configured/unconfigured) · conn=CONN(ready/failed)。 */
(function () {
  // ===== 代码/版本素材（function 真做版本 diff，故备 v5/v4/v1 三份源）=====
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
  const PY1 = `def process_invoice(file):
    return parse(file)`;

  // workflow 图拓扑（{x,y}=左上，与 RunGraph 节点契约一致；node.kind∈trigger/action/agent/control/approval）
  const n = (id, kind, ref, x, y) => ({ id, kind, ref, x, y });

  window.MOCK_ENTITIES = {
    // ============ Function ============
    process_invoice: {
      kind: 'function', version: 5, status: 'done', runs: 1284,
      desc: '解析上传的发票 PDF / 图片，抽取结构化字段（供应商、金额、税号、行项目），校验后返回 JSON。',
      code: PY5, lang: 'python 3.12', env: 'ready', lastRun: '2 小时前 · 成功',
      deps: ['pdfplumber', 'pydantic', 'regex'],
      inputs: [['file', 'bytes', true], ['currency', 'string', false]],
      output: [['Invoice', 'vendor·total·tax_id·line_items[]']],
      versions: [
        { v: 5, active: true, t: '2 小时前', reason: '加 currency 入参 + 注释', code: PY5 },
        { v: 4, t: '昨天', reason: '抽出 extract_fields 启发式', code: PY4 },
        { v: 1, t: '上周', reason: '初版', code: PY1 },
      ],
      execs: [['ok', 'chat', '412ms', '2 小时前'], ['ok', 'workflow', '388ms', '今天 02:00'], ['failed', 'manual', '—', '昨天'], ['ok', 'agent', '455ms', '昨天']],
      rel: [
        { title: 'Forged in', rows: [{ kind: 'conversation', name: '前端设计 (fork)' }] },
        { title: 'Used by', rows: [{ kind: 'workflow', name: 'invoice_flow', meta: 'extract' }, { kind: 'agent', name: 'summarizer', meta: 'tool' }] },
      ],
    },
    fetch_news: {
      kind: 'function', version: 2, status: 'run', runs: 96,
      desc: '按关键词拉取多源 RSS / 新闻，去重归并。', lang: 'python 3.12', env: 'syncing',
      code: 'def fetch_news(topics: list[str], since=None) -> list[NewsItem]:\n    items = []\n    for feed in resolve_feeds(topics):\n        items += parse_feed(feed, since)\n    return dedupe(items, key=lambda i: i.url)',
      deps: ['requests', 'feedparser'],
      inputs: [['topics', 'string[]', true], ['since', 'datetime', false]],
      output: [['NewsItem[]', 'title·url·source']],
    },
    parse_pdf: {
      kind: 'function', version: 1, status: 'err', runs: 7,
      desc: 'PDF 文本层抽成纯文本块。依赖物化失败。', lang: 'python 3.12', env: 'failed',
      code: 'def parse_pdf(file: bytes) -> list[TextBlock]:\n    return [TextBlock(page=i, text=p.extract_text())\n            for i, p in enumerate(pdfplumber.open(file).pages)]',
      deps: ['pdfplumber'],
      inputs: [['file', 'bytes', true]],
      output: [['TextBlock[]', 'page·text']],
    },

    // ============ Handler ============
    slack_handler: {
      kind: 'handler', version: 3, status: 'done', runs: 512,
      desc: '常驻 Slack 连接：发消息、建频道、上传、监听事件，维护长连 socket 会话。',
      lang: 'python 3.12', life: 'active', env: 'ready', cfg: 'ready',
      code: 'class SlackHandler(Handler):\n    def __init__(self, bot_token, app_token, default_channel="#ops"):\n        self.client = WebClient(bot_token)\n        self.socket = SocketModeClient(app_token)\n\n    def shutdown(self):\n        self.socket.close()',
      methods: [['post_message', '(channel, text)'], ['create_channel', '(name)'], ['upload_file', '(path, channel)'], ['on_event', '(type, cb)']],
      initArgs: [['bot_token', null, true], ['app_token', null, true], ['default_channel', '#ops', false]],
    },
    db_pool: {
      kind: 'handler', version: 2, status: 'wait', runs: 0,
      desc: 'PostgreSQL 连接池。缺密码，未上线。',
      lang: 'python 3.12', life: 'inactive', env: 'pending', cfg: 'partially_configured',
      code: 'class DbPool(Handler):\n    def __init__(self, dsn, password, pool_size=10):\n        self.pool = create_pool(dsn, password, pool_size)',
      methods: [['query', '(sql, *p)'], ['execute', '(sql, *p)'], ['transaction', '()']],
      initArgs: [['dsn', 'postgres://…/forgify', false], ['password', null, true], ['pool_size', '10', false]],
    },

    // ============ Agent ============
    research_agent: {
      kind: 'agent', version: 2, status: 'idle', runs: 38, life: 'active',
      desc: '深度调研代理：检索、交叉验证、带引用综述，写回记忆。',
      model: 'claude-opus-4-8', maxSteps: 40,
      system: '你是一名严谨的研究员。对每个论断都要交叉验证至少两个独立来源，输出带引用编号的综述。\n不确定时显式标注「待证实」，绝不杜撰来源。优先用 web_search 找一手资料，再用 fetch_url 取全文。',
      tools: [{ label: 'web_search', health: 'ok' }, { label: 'fetch_url', health: 'ok' }, { label: 'read_document', health: 'ok' }, { label: 'write_memory', health: 'ok' }, { label: 'cite', health: 'bad' }],
      skill: 'deep_research', knowledge: ['竞品库', '行业报告 2026'],
      rel: [
        { title: 'Equips', rows: [{ kind: 'skill', name: 'deep_research', meta: 'skill' }, { kind: 'function', name: 'cite', meta: 'tool · ⚠' }] },
        { title: 'Used by', rows: [{ kind: 'workflow', name: 'nightly_report', meta: 'summarize' }] },
      ],
    },
    summarizer: {
      kind: 'agent', version: 4, status: 'idle', runs: 211, life: 'active',
      model: 'claude-sonnet-4-6', maxSteps: 12,
      desc: '把长文 / 会话 / 运行日志压成结构化摘要（要点 + 风险 + 下一步）。',
      system: '把输入压成三段：① 3–5 条要点 ② 风险或反对意见 ③ 建议的下一步。\n保留关键数字与专有名词，删冗余修饰。输出 markdown。',
      tools: [{ label: 'read_document', health: 'ok' }, { label: 'write_memory', health: 'ok' }],
      skill: null, knowledge: [],
    },

    // ============ Workflow ============
    nightly_report: {
      kind: 'workflow', version: 8, status: 'run', runs: 173, life: 'active',
      concurrency: 'serial', triggers: ['cron_2am'],
      desc: '每晚汇总仓库与议题动态，交研究代理综述，按规模路由后推送简报、必要时请负责人审批。',
      nodes: [n('cron', 'trigger', 'cron_2am', 22, 150), n('fetch_repos', 'action', 'fetch_news', 210, 56), n('fetch_issues', 'action', 'parse_pdf', 210, 244), n('summarize', 'agent', 'research_agent', 398, 150), n('route', 'control', 'route_by_amount', 586, 150), n('publish', 'action', 'slack_handler', 398, 286), n('notify', 'approval', 'manager_approval', 586, 286)],
      edges: [['cron', 'fetch_repos'], ['cron', 'fetch_issues'], ['fetch_repos', 'summarize'], ['fetch_issues', 'summarize'], ['summarize', 'route'], ['route', 'publish'], ['route', 'notify']],
      vb: [780, 360],
      state: { cron: 'completed', fetch_repos: 'completed', fetch_issues: 'completed', summarize: 'running', route: 'future', publish: 'future', notify: 'future' },
      taken: ['cron>fetch_repos', 'cron>fetch_issues', 'fetch_repos>summarize', 'fetch_issues>summarize'],
      live: 'fetch_issues>summarize', iters: { summarize: 3 },
      rel: [{ title: 'Equips', rows: [{ kind: 'agent', name: 'research_agent', meta: 'summarize' }, { kind: 'handler', name: 'slack_handler', meta: 'publish' }, { kind: 'trigger', name: 'cron_2am', meta: 'bound' }] }],
    },
    invoice_flow: {
      kind: 'workflow', version: 3, status: 'wait', runs: 64, life: 'active',
      concurrency: 'serial', triggers: ['webhook_pr'],
      attention: '节点 <b>approve</b> 等待 manager_approval 审批（已等 3 小时）。',
      nodes: [n('in', 'trigger', 'webhook_pr', 28, 130), n('extract', 'action', 'process_invoice', 240, 130), n('approve', 'approval', 'manager_approval', 452, 130), n('post', 'action', 'db_pool', 600, 130)],
      edges: [['in', 'extract'], ['extract', 'approve'], ['approve', 'post']],
      vb: [800, 240],
      state: { in: 'completed', extract: 'completed', approve: 'parked', post: 'future' },
      taken: ['in>extract', 'extract>approve'],
      rel: [{ title: 'Equips', rows: [{ kind: 'function', name: 'process_invoice', meta: 'extract' }, { kind: 'approval', name: 'manager_approval', meta: 'gate' }, { kind: 'handler', name: 'db_pool', meta: 'post' }] }],
    },
    archive_cleanup: {
      kind: 'workflow', version: 1, status: 'idle', runs: 12, life: 'inactive',
      concurrency: 'serial', triggers: [],
      desc: '定期扫描过期对象并归档清理。',
      nodes: [n('cron', 'trigger', 'cron_weekly', 40, 130), n('scan', 'action', 'list_stale', 280, 130), n('purge', 'action', 'archive', 520, 130)],
      edges: [['cron', 'scan'], ['scan', 'purge']],
      vb: [760, 240],
      state: { cron: 'future', scan: 'future', purge: 'future' }, taken: [],
    },

    // ============ Trigger ============
    cron_2am: {
      kind: 'trigger', status: 'listening',
      desc: '每天 02:00（本地时区）触发，启动 nightly_report。',
      cfg: [['源类型', 'cron'], ['表达式', '0 2 * * *'], ['时区', 'Asia/Shanghai'], ['绑定工作流', 'nightly_report'], ['上次 fire', '今天 02:00 · 7 节点全绿']],
      outputs: ['firedAt', 'tz'],
      rel: [{ title: 'Fires', rows: [{ kind: 'workflow', name: 'nightly_report', meta: 'bound' }] }],
    },
    webhook_pr: {
      kind: 'trigger', status: 'idle',
      desc: 'GitHub PR webhook：opened / synchronize 时触发。',
      cfg: [['源类型', 'webhook'], ['路径', '/hooks/pr'], ['密钥', '••••', true], ['算法', 'hmac-sha256'], ['绑定工作流', 'invoice_flow'], ['上次 fire', '从未']],
      outputs: ['event', 'payload'],
      rel: [{ title: 'Fires', rows: [{ kind: 'workflow', name: 'invoice_flow', meta: 'bound' }] }],
    },

    // ============ Control ============
    route_by_amount: {
      kind: 'control', version: 2, status: 'idle',
      desc: 'CEL 路由：按金额分流到审批或自动入账。',
      branches: [['amount > 1000', 'approve'], ['amount <= 1000', 'auto_post']],
      inputs: [['amount', 'number', true]],
    },

    // ============ Approval ============
    manager_approval: {
      kind: 'approval', version: 4, status: 'idle',
      desc: '人在环审批闸：金额超阈需经理放行。',
      template: '## 发票待审批\n\n供应商：{{input.vendor}}\n金额：**{{input.total}}** {{input.currency}}\n\n> 超过 ¥1000 阈值，需经理放行。',
      rules: [['allowReason', 'true'], ['timeout', '24h'], ['timeoutBehavior', 'reject']],
      inputs: [['vendor', 'string', true], ['total', 'number', true], ['currency', 'string', true]],
    },

    // ============ MCP server ============
    github_mcp: {
      kind: 'mcp', status: 'done',
      desc: 'GitHub MCP 连接器：仓库 / PR / issue 能力作为工具暴露给代理。',
      conn: 'ready', calls: 318, fails: 0,
      cfg: [['传输', 'stdio'], ['命令', 'npx -y @gh/mcp'], ['鉴权', 'PAT ••••', true]],
      tools: ['list_repos', 'get_pr', 'create_issue', 'merge_pr', 'search_code'],
    },
    linear_mcp: {
      kind: 'mcp', status: 'wait',
      desc: 'Linear MCP 连接器：任务 / 周期能力。需重新授权。',
      conn: 'failed', calls: 42, fails: 5,
      cfg: [['传输', 'http'], ['URL', 'https://mcp.linear.app'], ['鉴权', 'OAuth（已过期）']],
      tools: ['list_issues', 'create_issue', 'update_issue'],
    },

    // ============ Skill ============
    deep_research: {
      kind: 'skill',
      desc: '深度调研 playbook：指导代理分解问题、检索、交叉验证、引用。',
      path: '.forgify/skills/deep_research.md',
      body: '# Deep research\n\n参数：$1 = 主题，${CLAUDE_SESSION_ID} = 会话\n\n1. 把问题拆成 3–5 个可独立检索的子问题。\n2. 每个子问题先 web_search 找一手来源，再 fetch_url 取全文。\n3. 交叉验证：同一论断至少两个独立来源，冲突则并列。\n4. 输出带引用编号 [n] 的综述，附来源清单。',
      allowed: ['web_search', 'fetch_url', 'cite'],
      frontmatter: [['name', 'deep_research'], ['context', 'inline'], ['arguments', '$1=主题']],
    },
    pdf_extract: {
      kind: 'skill',
      desc: '从 PDF 抽取并清洗文本的 playbook。',
      path: '.forgify/skills/pdf_extract.md',
      body: '# PDF extract\n\n1. 调 parse_pdf 取文本块。\n2. 合并跨页断句、去页眉页脚。\n3. 按标题层级重组为结构化大纲。',
      allowed: ['parse_pdf'],
      frontmatter: [['name', 'pdf_extract'], ['context', 'inline']],
    },
  };
})();

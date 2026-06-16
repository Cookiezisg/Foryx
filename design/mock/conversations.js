/* Foryx demo — 对话 / 会话示意数据层（DTO 镜像后端 references/messages 域）。绝不连后端。
   两层：sessions（侧栏会话史，镜像 conversations 行：id/title/state/group/time/pinned/archived）
        + scripts（每会话一条 teaching-spine 脚本：beats 数组，sea 解释器逐 beat 演 chat 引擎）。
   beat 是「块意图」声明式数据（type + 载荷），sea.js 调组件库渲染——脚本不含像素、不含 DOM。
   实体载荷（ENTITY）镜像 entities 域字段集，供锻造 beat 喂 EntityCard.mount + 流式 fill/reveal。
   消费者：features/chat（海面脚本播放 + 侧栏会话列）。状态枚举对齐通用 5 态（idle/run/wait/err/done）。 */
(function () {
  // —— 本演示触达的实体（镜像 entities DTO；点对话药丸 → Intent.select{kind} 唤右岛卡）——
  const ENTITY = {
    weekly_digest: {
      kind: 'function', name: 'weekly_digest', version: 1, id: 'fn_7f3c2a91b04e8d52', runs: 1,
      desc: '抓取竞品 RSS / 变更，归并去重，产出结构化摘要列表。', python: '3.12',
      code: 'def weekly_digest(sources, since):\n    items = []\n    for url in sources:\n        items += fetch(url, since)\n    return summarize(dedupe(items))',
      inputs: ['sources: list[str]', 'since: str'], deps: ['feedparser', 'httpx', 'beautifulsoup4'], env: 'ready',
    },
    notion_writer: {
      kind: 'handler', name: 'notion_writer', version: 3, id: 'hd_5a2e9b13c7f04d68', runs: 42,
      desc: '常驻的 Notion 写手：保活连接，跨调用复用 client（真共享状态）。',
      classCode: 'class NotionWriter:\n    def __init__(self, api_key, db_default):\n        self.client = Notion(api_key)\n    def publish(self, database, title, blocks): ...\n    def shutdown(self): self.client.close()',
      methods: ['publish', 'append', 'search'],
      initArgs: [{ name: 'api_key', sensitive: true }, { name: 'db_default', value: '竞品追踪' }],
      runtime: 'running', configState: 'ready', env: 'ready',
    },
    polish: {
      kind: 'agent', name: '摘要润色', version: 2, id: 'ag_8d1f4c20a9e3b7f5', runs: 7,
      desc: '把抓取到的原始条目润色成中文要点，去营销腔、附来源。',
      system: '你是严谨的竞品分析助手。把条目压成 3–5 条中文要点，每条附来源链接，不夸大。',
      model: 'claude-opus-4-8', maxSteps: 12,
      tools: [{ ref: 'fn_weekly_digest' }, { ref: 'hd_notion_writer.search' }, { ref: 'mcp:web/fetch' }],
      skill: null, knowledge: ['竞品列表.md'],
    },
    weekly_report: {
      kind: 'workflow', name: 'weekly_report', version: 1, id: 'wf_2b91ac7740e8d3f1', runs: 1,
      desc: '每天 08:00 抓取 → 润色 → 人工过目 → 发布 的编排图。', lifecycle: 'inactive', concurrency: 'serial',
      nodes: [
        { kind: 'trigger', id: 'daily', ref: 'trg_cron_0800', x: 24, y: 80 },
        { kind: 'action', id: 'fetch', ref: 'fn_weekly_digest', x: 224, y: 80 },
        { kind: 'agent', id: 'polish', ref: 'ag_摘要润色', x: 424, y: 80 },
        { kind: 'approval', id: 'review', ref: 'apr_过目', x: 624, y: 80 },
        { kind: 'action', id: 'publish', ref: 'hd_notion_writer.publish', x: 824, y: 80 },
      ],
      edges: [['daily', 'fetch'], ['fetch', 'polish'], ['polish', 'review'], ['review', 'publish']],
    },
    pdf_extract: {
      kind: 'function', name: 'pdf_extract', version: 2, id: 'fn_3c91a7b240e8f15d', runs: 6,
      desc: '从 PDF 抽取表格为结构化 JSON（无状态、每调用一进程）。', python: '3.12',
      code: 'def pdf_extract(url, pages):\n    doc = pdfplumber.open(fetch(url))\n    out = []\n    for p in pages:\n        out += [t.extract() for t in doc.pages[p].find_tables()]\n    return out',
      inputs: ['url: str', 'pages: list[int]'], deps: ['pdfplumber', 'pandas'], env: 'ready',
    },
    researcher: {
      kind: 'agent', name: 'Researcher', version: 3, id: 'ag_5f2c8a10d4e3b7f9', runs: 24,
      desc: '深度调研员：检索、交叉验证、产出带引用的结构化综述。',
      system: '你是严谨的调研员。检索、交叉验证，每条结论必须附引用来源（行内链接）。',
      model: 'claude-opus-4-8', maxSteps: 20,
      tools: [{ ref: 'fn_pdf_extract' }, { ref: 'hd_webhook_ingest.stats' }, { ref: 'mcp:web/fetch' }],
      skill: 'cite-sources', knowledge: ['竞品列表.md'],
    },
  };

  // —— 工具友好名：原始 tool id → {动词, 名}（用户读人话、不见 create_function(…) 这类内部名）。
  //    渲染：toolItem 的「动词 名」两槽；run/call/trigger 保留实体名（用户自己的实体），纯工具(todo)整句友好化。
  const TOOL_NAMES = {
    'todo_write': { verb: '更新', name: '任务清单' },
    'create_function(weekly_digest)': { verb: '锻造', name: 'weekly_digest' },
    'create_function(pdf_extract)': { verb: '锻造', name: 'pdf_extract' },
    'create_handler(webhook_ingest)': { verb: '锻造', name: 'webhook_ingest' },
    'create_workflow(weekly_report)': { verb: '锻造', name: 'weekly_report' },
    'edit_agent(Researcher)': { verb: '编辑', name: 'Researcher' },
    'run_function(weekly_digest)': { verb: '运行', name: 'weekly_digest' },
    'run_function(pdf_extract)': { verb: '运行', name: 'pdf_extract' },
    'call_handler(webhook_ingest.ingest)': { verb: '调用', name: 'webhook_ingest · ingest' },
    'trigger_workflow(weekly_report)': { verb: '触发', name: 'weekly_report' },
    'notion_writer.publish': { verb: '发布', name: 'Notion' },
  };

  // —— 脚本元语：每 beat 是声明式块意图（sea 解释器映射到组件调用）——
  // type ∈ user | ai | reason | tool | forge | run | approval | flowrun | subagent | todo | turnEnd | compaction | ents
  const SCRIPTS = {
    // 旗舰 spine：竞品日报 workflow（多实体一回合）
    'wf-weekly-report': [
      { type: 'compaction', text: '· 上文已压缩 · earlier context summarized ·' },
      { type: 'user', html: '帮我搭一个每天早上自动汇总竞品动态、写进 Notion 的流程。背景看 {{doc:竞品列表.md}}。' },
      { type: 'reason', text: '用户要一条「抓取→汇总→发布」的每日自动化。拆解：无状态抓取函数（function）+ 常驻 Notion 写手（handler，已有）+ 一个润色 LLM 员工（agent）+ 一张每天 08:00 触发的编排图（workflow）。先写待办、逐个锻造、跑通后编排。' },
      { type: 'ai', text: '好的。我会先锻造一个抓取 + 汇总的函数，跑通后接上常驻的 Notion 写手，最后编排成每天触发的 workflow。先把任务拆出来：' },
      { type: 'tool', status: '更新任务清单…', settle: '已更新待办', items: [{ name: 'todo_write', detail: '整表替换写入 · 4 项 · LLM 自管、只读' }] },
      { type: 'todo', rows: [['抓取竞品动态并汇总（function）', 'in-progress'], ['接 Notion 写手（handler）', 'pending'], ['编排每日 workflow', 'pending'], ['上线 + 验证一次', 'pending']] },
      { type: 'ai', text: '开始锻造抓取函数。' },
      {
        type: 'forge', status: '锻造 weekly_digest…', settle: '已锻造 · weekly_digest',
        verb: 'Forged', toolName: 'create_function(weekly_digest)',
        seed: { kind: 'function', name: 'weekly_digest', version: 1, live: 'forge', id: 'fn_7f3c2a91b04e8d52', desc: '抓取竞品 RSS / 变更，归并去重，产出结构化摘要列表。', python: '3.12', code: '', inputs: ['sources: list[str]', 'since: str'], deps: [], env: 'pending' },
        stream: [
          { f: 'code', code: 'def weekly_digest(sources, since):\n    items = []\n    for url in sources:\n        items += fetch(url, since)\n    return summarize(dedupe(items))' },
          { setSt: ['env', 'ENV', 'syncing'] },
          { f: 'deps', tags: ['feedparser', 'httpx', 'beautifulsoup4'] },
          { setSt: ['env', 'ENV', 'ready'] },
        ],
      },
      { type: 'todo', rows: [['抓取竞品动态并汇总（function）', 'completed'], ['接 Notion 写手（handler）', 'in-progress'], ['编排每日 workflow', 'pending'], ['上线 + 验证一次', 'pending']] },
      { type: 'ai', text: '跑一次看看输出。' },
      {
        type: 'run', status: '运行 weekly_digest…', settle: '运行完成 · weekly_digest', toolName: 'run_function(weekly_digest)',
        progress: ['fetch arxiv.org/list … 12 条', 'fetch openai.com/blog … 3 条', 'fetch anthropic.com/news … 5 条', 'dedupe → 17 条', 'summarize via dialogue model …'],
        result: '{ "count": 17, "items": [ { "title": "…", "url": "…" }, … ] }',
      },
      { type: 'ai', text: '现在把摘要发到 Notion。这一步会写到外部空间，需要你确认。' },
      {
        type: 'approval', status: '调用 notion_writer · publish…', tool: 'notion_writer.publish', danger: 'dangerous',
        summary: '把今天的竞品摘要发布到 Notion「竞品追踪」数据库（新增 1 页、写 17 个块）。',
        args: '{\n  "database": "竞品追踪",\n  "title": "竞品摘要 · 06-14",\n  "blocks": 17\n}',
        auto: 'approve_always', settleOk: '已批准 · 本会话内始终允许', settleNo: '已拒绝 · 反馈给模型',
        groupSettle: '已发布 · notion_writer', groupSettleNo: '已拒绝 · notion_writer',
        progress: ['connect notion … ok', 'create page 竞品摘要 · 06-14', 'append 17 blocks … done'],
      },
      { type: 'todo', rows: [['抓取竞品动态并汇总（function）', 'completed'], ['接 Notion 写手（handler）', 'completed'], ['编排每日 workflow', 'in-progress'], ['上线 + 验证一次', 'pending']] },
      { type: 'ai', text: '接下来把这些接成每天触发的 workflow。我派一个 Plan 子 agent 先理清接线。' },
      {
        type: 'subagent', label: 'Subagent · Plan',
        reason: '触发用 cron（每天 08:00）；trigger → action(weekly_digest) → agent(摘要润色) → approval(人工过目) → action(notion_writer.publish)。回边只在 approval 上闭合（环纪律）。',
        forge: {
          status: '锻造 weekly_report…', settle: '已锻造 · weekly_report', verb: 'Forged', toolName: 'create_workflow(weekly_report)',
          seed: { kind: 'workflow', name: 'weekly_report', version: 1, live: 'forge', id: 'wf_2b91ac7740e8d3f1', desc: '每天 08:00 抓取 → 润色 → 人工过目 → 发布 的编排图。', concurrency: 'serial', lifecycle: 'inactive', nodes: [] },
          fillGraph: 'weekly_report',
        },
      },
      { type: 'todo', rows: [['抓取竞品动态并汇总（function）', 'completed'], ['接 Notion 写手（handler）', 'completed'], ['编排每日 workflow', 'completed'], ['上线 + 验证一次', 'in-progress']] },
      { type: 'ai', text: '先手动触发一次，看整条链路跑通没。' },
      {
        type: 'tool', status: '触发 weekly_report…', settle: '已起 run · weekly_report',
        items: [{ name: 'trigger_workflow(weekly_report)', detail: '202 Accepted · flowrun 已起 · 节点结果记忆化、逐节点推进（非事件日志）' }],
      },
      {
        type: 'flowrun', frid: 'fr_9c4e1d77a0b3',
        nodes: [
          { kind: 'trigger', id: 'daily', act: 'ok', memo: true },
          { kind: 'action', id: 'fetch', act: 'ok' },
          { kind: 'agent', id: 'polish', act: 'ok' },
          { kind: 'approval', id: 'review', act: 'park' },
          { kind: 'action', id: 'publish', act: 'ok', gatedBy: 'review' },
        ],
      },
      { type: 'ai', text: '链路跑通 ✅ 摘要已发布、workflow 已就绪。还差最后一步 activate 上线每日触发——不过这个回合已经到步数上限了。' },
      { type: 'ents', label: '本回合锻造 / 触达的实体', keys: ['weekly_digest', 'notion_writer', 'polish', 'weekly_report'] },
      { type: 'turnEnd', code: 'MAX_STEPS_REACHED', msg: '回合到达步数上限（25 步），诚实终止——<b>未失败</b>。' },
    ],

    // PDF 提取 function（无状态、env 物化、progress+result）
    'fn-pdf-extract': [
      { type: 'user', html: '帮我写个从 PDF 抽取表格的函数，按页码抽成结构化 JSON。' },
      { type: 'reason', text: '一次性抽取、无跨调用状态 → 选 function（无状态、每调用一个全新沙箱进程）。' },
      { type: 'ai', text: '锻造一个无状态抽取函数 pdf_extract。' },
      { type: 'todo', rows: [['锻造 pdf_extract（function）', 'in-progress'], ['物化 env + 跑通', 'pending']] },
      {
        type: 'forge', status: '锻造 pdf_extract…', settle: '已锻造 · pdf_extract', verb: 'Forged', toolName: 'create_function(pdf_extract)',
        seed: { kind: 'function', name: 'pdf_extract', version: 1, live: 'forge', id: 'fn_3c91a7b240e8f15d', desc: '从 PDF 抽取表格为结构化 JSON。', python: '3.12', code: '', inputs: ['url: str', 'pages: list[int]'], deps: [], env: 'pending' },
        stream: [
          { f: 'code', code: 'def pdf_extract(url, pages):\n    doc = pdfplumber.open(fetch(url))\n    out = []\n    for p in pages:\n        out += [t.extract() for t in doc.pages[p].find_tables()]\n    return out' },
          { setSt: ['env', 'ENV', 'syncing'] },
          { f: 'deps', tags: ['pdfplumber==0.9', 'pandas'] },
          { setSt: ['env', 'ENV', 'ready'] },
        ],
      },
      { type: 'todo', rows: [['锻造 pdf_extract（function）', 'completed'], ['物化 env + 跑通', 'in-progress']] },
      { type: 'ai', text: 'env 已就绪，跑一次抽取验证。' },
      {
        type: 'run', status: '运行 pdf_extract…', settle: '运行完成 · pdf_extract', toolName: 'run_function(pdf_extract)',
        progress: ['open invoice.pdf … 4 页', 'page 0 · find_tables → 1', 'page 1 · find_tables → 2', 'extract → 3 张表'],
        result: '{ "tables": 3, "rows": 41 }',
      },
      { type: 'todo', rows: [['锻造 pdf_extract（function）', 'completed'], ['物化 env + 跑通', 'completed']] },
      { type: 'ai', text: '完成 ✅ 函数已锻造、env 物化、抽取跑通。' },
      { type: 'ents', label: '本回合实体', keys: ['pdf_extract'] },
    ],

    // Researcher agent 调优（agent 编辑 + 工具挂载）
    'agent-researcher-tune': [
      { type: 'user', html: '给 Researcher agent 换个更严谨的 system prompt，再挂上 pdf_extract 工具。' },
      { type: 'reason', text: 'agent 的能力 = system prompt + 挂载工具集；edit_agent 走版本（v3→v4），挂工具是引用边、非复制。' },
      { type: 'ai', text: '我来改 system prompt 并挂上 pdf_extract，改完是 v4。' },
      {
        type: 'forge', status: '编辑 Researcher…', settle: '已更新 · Researcher', verb: 'Edited', toolName: 'edit_agent(Researcher)', live: 'edit',
        seed: { kind: 'agent', name: 'Researcher', version: 3, live: 'edit', id: 'ag_5f2c8a10d4e3b7f9', desc: '深度调研员：检索、交叉验证、产出带引用的结构化综述。', system: '你是严谨的调研员。检索、交叉验证，每条结论必须附引用来源（行内链接）。', model: 'claude-opus-4-8', maxSteps: 20, tools: [{ ref: 'fn_pdf_extract' }, { ref: 'mcp:web/fetch' }], skill: 'cite-sources', knowledge: ['竞品列表.md'] },
        stream: [
          { f: 'system', code: '你是严谨的调研员。检索、交叉验证；每条结论必须附行内引用来源，无引用宁可不下结论。冲突来源需并列陈述并标注分歧。' },
          { reveal: 'tools' },
          { setVersion: 4 },
        ],
      },
      { type: 'ai', text: '调优完成。验证一次 invoke 看引用是否生效。' },
      {
        type: 'subagent', label: 'invoke_agent · Researcher（v4）',
        reason: '注入 cite-sources skill + pdf_extract；产出每条结论附行内引用。',
      },
      { type: 'ai', text: '完成 ✅ Researcher 已升到 v4、工具已挂、引用生效。' },
      { type: 'ents', label: '本回合实体', keys: ['researcher', 'pdf_extract'] },
    ],

    // Webhook 入库 handler（常驻 + config 重启）
    'webhook-handler': [
      { type: 'user', html: '搭一个常驻的 webhook 入库 handler，保活 DB 连接、跨调用复用。' },
      { type: 'reason', text: '需要跨调用复用 DB 连接（真共享状态）→ 选 handler（常驻进程，非无状态 function）。改 config 触发重启。' },
      { type: 'ai', text: '锻造常驻 handler webhook_ingest，init 持有 DB 连接。' },
      {
        type: 'forge', status: '锻造 webhook_ingest…', settle: '已锻造 · webhook_ingest', verb: 'Forged', toolName: 'create_handler(webhook_ingest)',
        seed: { kind: 'handler', name: 'webhook_ingest', version: 1, live: 'forge', id: 'hd_8b21fe04a9c7d350', desc: '常驻 webhook 入库：保活 DB 连接、跨调用复用 self.conn。', runtime: 'unconfigured', configState: 'unconfigured', env: 'pending', initArgs: [{ name: 'db_url', required: true, sensitive: true }, { name: 'table', value: 'events' }], methods: [], classCode: '' },
        stream: [
          { f: 'class', code: 'class WebhookIngest:\n    def __init__(self, db_url, table="events"):\n        self.conn = connect(db_url)\n    def ingest(self, payload): ...\n    def shutdown(self): self.conn.close()' },
          { setSt: ['config', 'CFG', 'ready'] },
          { setSt: ['env', 'ENV', 'ready'] },
        ],
      },
      { type: 'ai', text: '常驻进程已起、config 就绪。调一次 ingest 验证连接复用。' },
      {
        type: 'run', status: '调用 webhook_ingest · ingest…', settle: '调用完成 · webhook_ingest', toolName: 'call_handler(webhook_ingest.ingest)',
        progress: ['reuse self.conn (alive 4h12m)', 'INSERT events … 1 row', 'commit'],
        result: '{ "ingested": 1, "connReused": true }',
      },
      { type: 'ai', text: '完成 ✅ handler 常驻、连接跨调用复用。' },
      { type: 'ents', label: '本回合实体', keys: ['notion_writer'] },
    ],
  };

  // —— 侧栏会话史（镜像 conversations 行 DTO）。每行绑一脚本 id（→ 海面播放）。状态点 = isGenerating + humanloop。
  window.MOCK_CONVERSATIONS = {
    sessions: {
      pinned: [
        { id: 'wf-weekly-report', title: '竞品动态日报流程', st: 'idle', on: true },
        { id: 'agent-researcher-tune', title: 'Researcher agent 调优', st: 'idle' },
      ],
      groups: [
        ['Today', [
          { id: 'control-cel-fix', title: '修复 CEL 校验器', st: 'run', time: '14:32' },
          { id: 'webhook-handler', title: 'Webhook 入库 handler', st: 'wait', time: '11:08' },
        ]],
        ['Yesterday', [
          { id: 'wf-weekly-report', title: '周报自动汇总 workflow', st: 'done', time: 'Tue' },
          { id: 'document-qa-agent', title: '文档问答 agent', st: 'done', time: 'Tue' },
        ]],
        ['Previous 7 days', [
          { id: 'approval-publish-gate', title: '发布前过目 approval', st: 'idle', time: 'Jun 9' },
          { id: 'trigger-slack-webhook', title: 'Slack 通知 trigger', st: 'idle', time: 'Jun 8' },
          { id: 'fn-pdf-extract', title: 'PDF 提取 function', st: 'idle', time: 'Jun 7' },
        ]],
        ['Older', [
          { id: 'mcp-notion-sync', title: 'Notion 同步实验', st: 'idle', time: 'May 28' },
          { id: 'wf-weekly-report', title: '账单对账流程', st: 'err', time: 'Jun 9' },
          { id: '', title: '旧版迁移笔记', st: 'idle', time: 'May 20' },
        ]],
      ],
      archived: [
        { id: '', title: '临时调试 agent', st: 'idle' },
        { id: '', title: '废弃的爬虫流程', st: 'idle' },
        { id: '', title: '一次性数据清洗', st: 'idle' },
      ],
    },
    // 默认场景 + 每会话标题（行标签覆盖：多会话复用同脚本各显其名）
    default: 'wf-weekly-report',
    titles: {
      'wf-weekly-report': '竞品动态日报流程',
      'fn-pdf-extract': 'PDF 提取 function',
      'agent-researcher-tune': 'Researcher agent 调优',
      'webhook-handler': 'Webhook 入库 handler',
    },
    entities: ENTITY,
    toolNames: TOOL_NAMES,
    scripts: SCRIPTS,
  };
})();

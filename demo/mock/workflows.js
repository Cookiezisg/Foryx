/* Forgify demo — workflow / flowrun 示意数据层（DTO 镜像后端 references/）。绝不连后端。
   run 嵌冻结拓扑（镜像 flowruns.version_id），状态枚举对齐后端：run=running/completed/failed/cancelled，node=completed/failed/parked/running/future。
   消费者：features/scheduler（驾驶舱 + 侧栏列）。键 = workflow 名。 */
(function () {
  const n = (id, kind, ref, x, y) => ({ id, kind, ref, x, y });
  const REPORT = {
    nodes: [n('cron', 'trigger', '每日 02:00', 24, 150), n('fetch', 'action', 'fetch_news', 224, 40), n('parse', 'action', 'parse_pdf', 224, 260), n('summarize', 'agent', 'research_agent', 424, 150), n('route', 'control', 'route_by_amount', 624, 150), n('publish', 'action', 'slack_handler', 824, 40), n('notify', 'approval', 'manager_approval', 824, 260)],
    edges: [['cron', 'fetch'], ['cron', 'parse'], ['fetch', 'summarize'], ['parse', 'summarize'], ['summarize', 'route'], ['route', 'publish'], ['route', 'notify']],
    loopbacks: [['route', 'summarize']], vb: [1012, 340],
  };

  window.MOCK_WORKFLOWS = {
    '研报抓取流': { st: 'run', meta: '运行中 4/7', cur: 1, triggerLabel: '每日 02:00', next: '明天 02:00', runs: [
      { id: 'fr_29f1a4', runState: 'completed', version: 'v8', trigger: 'cron_2am', when: '昨天 02:00', dur: '4m 12s', pos: '7/7', ...REPORT,
        state: { cron: 'completed', fetch: 'completed', parse: 'completed', summarize: 'completed', route: 'completed', publish: 'completed', notify: 'completed' },
        taken: ['cron>fetch', 'cron>parse', 'fetch>summarize', 'parse>summarize', 'summarize>route', 'route>publish', 'route>notify'], ghost: [], live: null, iters: { summarize: 2 }, ports: { route: 'done' },
        memo: { cron: { out: 'fired 02:00' }, fetch: { out: '42 commits' }, parse: { out: '17 issues' }, summarize: { loop: [{ i: 0, status: 'completed', out: 'draft v1' }, { i: 1, status: 'completed', out: 'draft v2 · 0.86' }] }, route: { out: '{ __port: "done" }' }, publish: { out: '已发 #ops' }, notify: { decision: 'yes', reason: '看起来不错' } } },
      { id: 'fr_3a9c2f', runState: 'running', version: 'v8', trigger: 'cron_2am', when: '今天 02:00', dur: '运行 6m', pos: '4/7', ...REPORT,
        state: { cron: 'completed', fetch: 'completed', parse: 'completed', summarize: 'running', route: 'completed', publish: 'future', notify: 'future' },
        taken: ['cron>fetch', 'cron>parse', 'fetch>summarize', 'parse>summarize', 'summarize>route'], ghost: [], live: 'route>summarize', iters: { summarize: 3 }, ports: { route: 'retry' },
        memo: { cron: { out: 'fired 02:00' }, fetch: { out: '51 commits' }, parse: { out: '23 issues' }, summarize: { loop: [{ i: 0, status: 'completed', out: 'draft v1 · 缺引用' }, { i: 1, status: 'completed', out: 'draft v2 · 0.72 < 0.8' }, { i: 2, status: 'running', out: '生成中…' }] }, route: { out: '{ __port: "retry" }  // 0.72 < 0.8' } } },
    ] },
    '竞品监控流': { st: 'wait', meta: '等你审批', cur: 0, triggerLabel: 'PR Webhook', runs: [
      { id: 'fr_71d0e8', runState: 'running', version: 'v3', trigger: 'webhook_pr', when: '14:22', dur: '运行 3m', pos: '4/5',
        nodes: [n('web', 'trigger', 'PR Webhook', 24, 75), n('crawl', 'action', 'fetch_news', 224, 75), n('diff', 'agent', 'summarizer', 424, 75), n('gate', 'approval', 'manager_approval', 624, 75), n('alert', 'action', 'slack_handler', 824, 75)],
        edges: [['web', 'crawl'], ['crawl', 'diff'], ['diff', 'gate'], ['gate', 'alert']], loopbacks: [], vb: [1012, 200],
        state: { web: 'completed', crawl: 'completed', diff: 'completed', gate: 'parked', alert: 'future' },
        taken: ['web>crawl', 'crawl>diff', 'diff>gate'], ghost: [], live: null, iters: {}, ports: {},
        memo: { web: { out: 'PR #482' }, crawl: { out: '竞品 3 处改动' }, diff: { out: '降价 12%' }, gate: { parked: true, prompt: '检测到竞品定价页改动（降价 12%）。是否推送告警并触发应对评审？', ddl: '自动驳回 22h', form: 'manager_approval v4' } } },
    ] },
    '账单对账流': { st: 'err', meta: '失败 · 2m', cur: 0, triggerLabel: '手动', runs: [
      { id: 'fr_5b3c10', runState: 'failed', version: 'v3', trigger: null, when: '13:40', dur: '2m 失败', pos: '2/4', replay: 1,
        nodes: [n('in', 'trigger', 'Webhook', 24, 75), n('extract', 'action', 'process_invoice', 224, 75), n('match', 'action', 'db_pool', 424, 75), n('post', 'action', 'db_pool', 624, 75)],
        edges: [['in', 'extract'], ['extract', 'match'], ['match', 'post']], loopbacks: [], vb: [800, 200],
        state: { in: 'completed', extract: 'failed', match: 'future', post: 'future' },
        taken: ['in>extract'], ghost: [], live: null, iters: {}, ports: {},
        memo: { in: { out: '12 张发票' }, extract: { error: 'SandboxError: 依赖物化超时（pdfplumber 38s > 30s）' } } },
    ] },
    '日报汇总流': { st: 'done', meta: '今天 9:02', cur: 0, triggerLabel: '每日 09:00', next: '明天 09:00', runs: [
      { id: 'fr_18aa3c', runState: 'completed', version: 'v5', trigger: 'cron_9am', when: '今天 09:02', dur: '1m 48s', pos: '4/4',
        nodes: [n('cron', 'trigger', '每日 09:00', 24, 75), n('gather', 'action', 'fetch_news', 224, 75), n('write', 'agent', 'summarizer', 424, 75), n('send', 'action', 'slack_handler', 624, 75)],
        edges: [['cron', 'gather'], ['gather', 'write'], ['write', 'send']], loopbacks: [], vb: [800, 200],
        state: { cron: 'completed', gather: 'completed', write: 'completed', send: 'completed' },
        taken: ['cron>gather', 'gather>write', 'write>send'], ghost: [], live: null, iters: {}, ports: {},
        memo: { cron: { out: 'fired 09:00' }, gather: { out: '8 条动态' }, write: { out: '日报 412 词' }, send: { out: '已发 #daily' } } },
    ] },
    'Slack 通知流': { st: 'idle', meta: '昨天', cur: 0, runs: [] },
    'PDF 批处理流': { st: 'idle', meta: 'Jun 8', cur: 0, runs: [] },
  };
})();

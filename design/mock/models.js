/* Foryx demo — 设置 / 模型示意数据层（DTO 镜像后端 references/，camelCase 线缆名）。绝不连后端。
   一份工作区配置种子：身份 + 概览统计 + 活动洞察 + 最常用实体（带 kind/ref 供 Intent.select）+
   默认模型(对话/工具/Agent) + apiKeys(provider+status) + 嵌入引擎 + connectors(MCP) + runtimes(沙箱) + workspaces + 通知。
   status 词汇对齐 status-dot 的徽 map：apiKey/connector → CONN(ready/failed/pending)；runtime → ENV(ready/failed/pending)。
   消费者：features/settings（海面接管，经侧栏头像轴）。 */
(function () {
  window.MOCK_MODELS = {
    // ——— 身份（镜像 workspaces 行）———
    workspace: {
      id: 'ws_8f3a91c20d',
      name: 'Personal',
      initial: 'P',
      dataDir: '~/Library/Application Support/Foryx',
      createdAt: '2026-01-03',
      activeDays: 142,
    },

    // ——— 概览统计（聚合，仅示意）———
    stats: [
      { num: '28.3m', label: '累计 Token' },
      { num: '1,284', label: '对话' },
      { num: '47', label: '锻造实体' },
      { num: '6 天', label: '当前连续' },
      { num: '23 天', label: '最长连续' },
    ],

    // ——— 活动洞察（k/v）———
    insights: [
      ['最常用模型', 'claude-opus-4-8'],
      ['默认模式', '标准 · 92%'],
      ['探索 / 已用技能', '5 / 7'],
      ['平均对话时长', '12 分钟'],
      ['最长运行', '28 分 37 秒'],
    ],

    // ——— 最常用实体（kind/ref → Intent.select 可跳实体海洋）———
    topEntities: [
      { kind: 'agent',    name: 'Researcher',   ref: 'ag_4b1c9f',  count: 312 },
      { kind: 'function', name: 'fetch_news',   ref: 'fn_2a7d10',  count: 188 },
      { kind: 'workflow', name: 'daily-digest', ref: 'wf_9c33e1',  count: 142 },
      { kind: 'handler',  name: 'inbox',        ref: 'hd_71aa28',  count: 96 },
      { kind: 'trigger',  name: '每早 6 点',     ref: 'trg_05ef44', count: 61 },
    ],

    // ——— 默认模型（ModelRef = {apiKeyId, modelId}；这里只示意 label）———
    defaultModels: {
      chat:    [{ value: 'opus',   label: 'claude-opus-4-8',   meta: '最强' }, { value: 'sonnet', label: 'claude-sonnet-4-6', meta: '均衡' }, { value: 'gpt5', label: 'gpt-5.1', meta: 'OpenAI' }],
      utility: [{ value: 'haiku',  label: 'claude-haiku-4-5',  meta: '快' },   { value: 'sonnet', label: 'claude-sonnet-4-6', meta: '均衡' }],
      agent:   [{ value: 'sonnet', label: 'claude-sonnet-4-6', meta: '均衡' }, { value: 'opus',   label: 'claude-opus-4-8',   meta: '最强' }, { value: 'ds',  label: 'deepseek-v4', meta: 'DeepSeek' }],
    },
    defaultPick: { chat: 'opus', utility: 'haiku', agent: 'sonnet' },

    // ——— API 密钥（具名加密凭据；status ∈ ready/failed/pending → CONN 徽）———
    apiKeys: [
      { id: 'key_a1', provider: 'Anthropic', status: 'ready' },
      { id: 'key_b2', provider: 'OpenAI',    status: 'pending' },
      { id: 'key_c3', provider: 'DeepSeek',  status: 'ready' },
    ],

    // ——— 嵌入引擎 ———
    embedding: { engine: 0, status: 'ready', model: 'EmbeddingGemma-300m', ollamaAddr: '127.0.0.1:11434', ollamaModel: 'embeddinggemma' },

    // ——— 连接器（MCP servers；status → CONN 徽）———
    connectors: [
      { id: 'mcp_gh', name: 'GitHub',     status: 'ready' },
      { id: 'mcp_no', name: 'Notion',     status: 'ready' },
      { id: 'mcp_fs', name: 'Filesystem', status: 'failed' },
    ],

    // ——— 沙箱运行时（按需下载·钉死版本；status → ENV 徽）———
    runtimes: [
      { id: 'py',   chip: 'Py', name: 'Python', status: 'ready',   version: '3.12.4' },
      { id: 'node', chip: 'JS', name: 'Node',   status: 'ready',   version: '20.14' },
      { id: 'uv',   chip: 'uv', name: 'uv',     status: 'pending', version: '' },
      { id: 'net',  chip: '.N', name: '.NET',   status: 'pending', version: '' },
    ],
    diskUsage: '1.24 GB',

    // ——— 全部工作区 ———
    workspaces: [
      { id: 'ws_8f3a91c20d', name: 'Personal', current: true },
      { id: 'ws_11b2c3d4e5', name: '实验场',    current: false },
    ],

    // ——— 通知 + 并发 ———
    notif: { concurrency: 4, runComplete: 0, needsApproval: 0, entityChange: 1 },

    version: 'Foryx 0.3.0 · demo',
  };
})();

/* Mock data — realistic enough for daily-use feel. */

const now = Date.now();
const ago = (mins) => new Date(now - mins * 60 * 1000);
const fmt = (d) => d.toISOString();

// ────────────────────────────────────────────────────────────────────────────
// Conversations (sidebar list)
// ────────────────────────────────────────────────────────────────────────────
const conversations = [
  { id: "cv_a1", title: "CSV → Notion 同步脚本", updatedAt: fmt(ago(1)), pinned: true, archived: false, status: "streaming", model: "deepseek-chat" },
  { id: "cv_b2", title: "周报生成器 v2", updatedAt: fmt(ago(35)), pinned: true, archived: false, status: "idle", model: "deepseek-chat" },
  { id: "cv_c3", title: "Strava 训练数据汇总", updatedAt: fmt(ago(180)), pinned: false, archived: false, status: "idle", model: "claude-haiku-4-5" },
  { id: "cv_d4", title: "把发票 PDF 入库", updatedAt: fmt(ago(60 * 22)), pinned: false, archived: false, status: "idle", model: "qwen-max" },
  { id: "cv_e5", title: "API 重构梳理", updatedAt: fmt(ago(60 * 28)), pinned: false, archived: false, status: "idle", model: "deepseek-chat" },
  { id: "cv_f6", title: "Linear 周回顾自动化", updatedAt: fmt(ago(60 * 50)), pinned: false, archived: false, status: "approval", model: "deepseek-chat" },
  { id: "cv_g7", title: "RSS → Markdown 归档", updatedAt: fmt(ago(60 * 70)), pinned: false, archived: false, status: "idle", model: "deepseek-chat" },
  { id: "cv_h8", title: "客户邮件分类规则", updatedAt: fmt(ago(60 * 96)), pinned: false, archived: false, status: "idle", model: "deepseek-chat" },
  // ── 归档 ──
  { id: "cv_z1", title: "Bloomberg 数据 ingest 尝试", updatedAt: fmt(ago(60 * 24 * 12)), pinned: false, archived: true, status: "idle", model: "deepseek-chat" },
  { id: "cv_z2", title: "Twitter scrape 概念验证", updatedAt: fmt(ago(60 * 24 * 21)), pinned: false, archived: true, status: "idle", model: "deepseek-chat" },
  { id: "cv_z3", title: "失败的 PDF OCR 实验",     updatedAt: fmt(ago(60 * 24 * 30)), pinned: false, archived: true, status: "idle", model: "qwen-max" },
  { id: "cv_z4", title: "旧版 Trinity 重构讨论",   updatedAt: fmt(ago(60 * 24 * 45)), pinned: false, archived: true, status: "idle", model: "deepseek-chat" },
];

// ────────────────────────────────────────────────────────────────────────────
// Active conversation blocks (the main demo conversation)
// Demonstrates: text, reasoning, parallel tool_call w/ progress, tool_result,
//               subagent nested message, streaming tail.
// ────────────────────────────────────────────────────────────────────────────
const activeMessages = [
  // ─── User opener ─────────────────────────────────────────────
  {
    id: "msg_u1",
    role: "user",
    status: "completed",
    createdAt: fmt(ago(6)),
    blocks: [
      {
        id: "blk_u1_t1",
        type: "text",
        status: "completed",
        content:
          "帮我做一个工具：每天早上把昨天的 Strava 训练数据拉过来，按周汇总平均配速、爬升、心率，写到 Notion 里那个『训练日志』 database。\n\n顺便看下我以前有没有类似的工具，能改就改。",
      },
    ],
    attachments: [
      { id: "att_1", fileName: "training-log-schema.png", mimeType: "image/png", sizeBytes: 184320 },
    ],
  },

  // ─── Assistant turn 1: reasoning + parallel search ───────────
  {
    id: "msg_a1",
    role: "assistant",
    status: "completed",
    stopReason: "end_turn",
    createdAt: fmt(ago(5)),
    inputTokens: 1842,
    outputTokens: 612,
    model: "deepseek-chat",
    blocks: [
      {
        id: "blk_r1",
        type: "reasoning",
        status: "completed",
        durationMs: 1840,
        content:
          "先在工具库里看看有没有现成的 Strava 抓取或 Notion 写入 forge。两个查询可以并行——分别用 strava + notion 两个关键词搜，能拿到的就尽量复用。\n附件是 schema 截图，等下读一下确认字段。",
      },
      {
        id: "tc_search1",
        type: "tool_call",
        status: "completed",
        durationMs: 420,
        attrs: { tool: "search_forges", summary: "搜索 Strava 相关工具", executionGroup: 1 },
        content: '{"query": "strava workout daily", "limit": 5}',
        children: [
          {
            id: "blk_search1_r",
            type: "tool_result",
            status: "completed",
            content:
              '[\n  { "id": "fn_strava_002", "name": "strava_recent_activities", "score": 0.82 },\n  { "id": "fn_strava_005", "name": "strava_athlete_zones", "score": 0.51 }\n]',
          },
        ],
      },
      {
        id: "tc_search2",
        type: "tool_call",
        status: "completed",
        durationMs: 380,
        attrs: { tool: "search_forges", summary: "搜索 Notion database 写入工具", executionGroup: 1 },
        content: '{"query": "notion database insert page", "limit": 5}',
        children: [
          {
            id: "blk_search2_r",
            type: "tool_result",
            status: "completed",
            content:
              '[\n  { "id": "hd_notion_001", "name": "notion_db_writer", "score": 0.91 },\n  { "id": "fn_notion_004", "name": "notion_page_search", "score": 0.43 }\n]',
          },
        ],
      },
      {
        id: "tc_read1",
        type: "tool_call",
        status: "completed",
        durationMs: 220,
        attrs: { tool: "Read", summary: "读取附件确认 Notion schema", executionGroup: 2 },
        content: '{"path": "attachments/att_1/original.png"}',
        children: [
          {
            id: "blk_read1_r",
            type: "tool_result",
            status: "completed",
            content:
              "Image attached. Detected fields: Week (title), Avg Pace (rich_text), Total Climb (number), Avg HR (number), Notes (rich_text).",
          },
        ],
      },
      {
        id: "blk_a1_t",
        type: "text",
        status: "completed",
        content:
          "查到两个能直接复用的：\n\n- `strava_recent_activities` — 拉取最近活动\n- `notion_db_writer` — 写 Notion database\n\n按你的要求，我打算做一个新的 **workflow** 把两端拼起来，每天 7:30 触发一次，按周聚合。要不要我接着做？",
      },
    ],
  },

  // ─── User follow-up ──────────────────────────────────────────
  {
    id: "msg_u2",
    role: "user",
    status: "completed",
    createdAt: fmt(ago(4)),
    blocks: [
      { id: "blk_u2_t1", type: "text", status: "completed", content: "好，做吧。汇总粒度按周，先写到 Notion 草稿区，我点确认再正式发布。" },
    ],
  },

  // ─── Assistant turn 2: subagent + workflow forge + streaming text tail
  {
    id: "msg_a2",
    role: "assistant",
    status: "streaming",
    createdAt: fmt(ago(1)),
    inputTokens: 2418,
    outputTokens: 1147,
    model: "deepseek-chat",
    blocks: [
      {
        id: "blk_r2",
        type: "reasoning",
        status: "completed",
        durationMs: 920,
        content:
          "需要先确认两个 forge 的输入输出形状。让 sub-agent 去并行验证一下两个 handler，主线程同时起 workflow 草稿。",
      },
      {
        id: "blk_sub1",
        type: "message",
        status: "completed",
        attrs: { kind: "subagent_run", agentType: "Explore", title: "并行检查两个 forge 的契约" },
        durationMs: 4200,
        tokens: { in: 812, out: 244 },
        children: [
          {
            id: "msg_sub1",
            role: "subagent",
            blocks: [
              {
                id: "blk_sub1_t1",
                type: "text",
                status: "completed",
                content: "我会读两个 forge 的最新版本 schema，确认能拼起来。",
              },
              {
                id: "tc_sub1_a",
                type: "tool_call",
                status: "completed",
                durationMs: 180,
                attrs: { tool: "get_forge", summary: "查 strava_recent_activities 契约" },
                content: '{"id": "fn_strava_002"}',
                children: [
                  {
                    id: "blk_sub1_a_r",
                    type: "tool_result",
                    status: "completed",
                    content:
                      "{\n  inputs: { since: iso8601 },\n  outputs: { activities: Activity[] }\n}\nActivity = { id, type, distance_m, moving_time_s, avg_hr, total_elevation_gain }",
                  },
                ],
              },
              {
                id: "blk_sub1_t2",
                type: "text",
                status: "completed",
                content: "两边字段对得上，可以直接接。",
              },
            ],
          },
        ],
      },
      {
        id: "tc_create_wf",
        type: "tool_call",
        status: "streaming",
        attrs: { tool: "create_workflow", summary: "起草 weekly-training-summary 工作流", executionGroup: 3 },
        content: '{\n  "name": "weekly-training-summary",\n  "trigger": { "kind": "cron", "expr": "30 7 * * *" },\n  "nodes": [\n    { "id": "fetch", "kind": "function", "ref": "fn_strava_002" },\n    { "id": "agg", "kind": "function", "ref": "fn_aggregate_week" },\n    { "id": "wr',
        children: [
          {
            id: "blk_prog1",
            type: "progress",
            status: "streaming",
            attrs: { stage: "validating" },
            content: "verifying capability refs ✓ fn_strava_002\nverifying capability refs ✓ fn_aggregate_week\nverifying capability refs … hd_notion_001",
          },
        ],
      },
      {
        id: "blk_a2_tail",
        type: "text",
        status: "streaming",
        content: "已经起草好工作流的骨架，接下来我把周聚合那一步",
      },
    ],
  },
];

// ────────────────────────────────────────────────────────────────────────────
// Forges (functions / handlers / workflows)
// ────────────────────────────────────────────────────────────────────────────
const forges = [
  { id: "fn_strava_002", kind: "function", name: "strava_recent_activities", desc: "拉取 Strava 最近 N 天的活动数据", version: "v3", updatedAt: fmt(ago(60 * 14)), runs: 412, status: "ready" },
  { id: "fn_aggregate_week", kind: "function", name: "aggregate_week", desc: "按周聚合数值字段（avg / sum / max）", version: "v1", updatedAt: fmt(ago(60 * 24)), runs: 38, status: "pending" },
  { id: "hd_notion_001", kind: "handler", name: "notion_db_writer", desc: "向 Notion database 写入行（含 rate limit）", version: "v7", updatedAt: fmt(ago(60 * 48)), runs: 891, status: "ready" },
  { id: "fn_csv_parse", kind: "function", name: "csv_parse_typed", desc: "类型推断的 CSV 解析（datetime / number / bool）", version: "v2", updatedAt: fmt(ago(60 * 72)), runs: 209, status: "ready" },
  { id: "wf_weekly_training", kind: "workflow", name: "weekly-training-summary", desc: "每周一早 7:30 把训练数据写到 Notion", version: "v1", updatedAt: fmt(ago(2)), runs: 0, status: "draft" },
  { id: "wf_invoice_intake", kind: "workflow", name: "invoice-intake", desc: "OneDrive 新 PDF → OCR → Airtable", version: "v4", updatedAt: fmt(ago(60 * 96)), runs: 124, status: "ready" },
  { id: "hd_airtable_002", kind: "handler", name: "airtable_writer", desc: "Airtable 批量 upsert（带去重）", version: "v3", updatedAt: fmt(ago(60 * 130)), runs: 562, status: "ready" },
  { id: "fn_pdf_extract", kind: "function", name: "pdf_extract_text", desc: "PDF 文本抽取（保留段落）", version: "v5", updatedAt: fmt(ago(60 * 180)), runs: 1432, status: "ready" },
];

// ────────────────────────────────────────────────────────────────────────────
// Flow runs
// ────────────────────────────────────────────────────────────────────────────
// ────────────────────────────────────────────────────────────────────────────
// Flow runs (with rich node-level data for detail view)
// ────────────────────────────────────────────────────────────────────────────
const flowruns = [
  { id: "fr_8d2a", workflow: "invoice-intake", status: "running", startedAt: fmt(ago(2)), durationMs: 142000, trigger: "fsnotify", nodes: { total: 6, done: 3 } },
  { id: "fr_8d29", workflow: "weekly-training-summary", status: "waiting_approval", startedAt: fmt(ago(28)), durationMs: 18400, trigger: "cron", nodes: { total: 6, done: 4 } },
  { id: "fr_8d28", workflow: "linear-weekly-recap", status: "completed", startedAt: fmt(ago(120)), durationMs: 41200, trigger: "cron", nodes: { total: 5, done: 5 } },
  { id: "fr_8d27", workflow: "rss-archive", status: "completed", startedAt: fmt(ago(60 * 6)), durationMs: 8400, trigger: "manual", nodes: { total: 3, done: 3 } },
  { id: "fr_8d26", workflow: "invoice-intake", status: "failed", startedAt: fmt(ago(60 * 24)), durationMs: 24800, trigger: "fsnotify", nodes: { total: 6, done: 4 }, error: "notion_db_writer: 429 rate limited" },
  { id: "fr_8d25", workflow: "invoice-intake", status: "completed", startedAt: fmt(ago(60 * 26)), durationMs: 31200, trigger: "fsnotify", nodes: { total: 6, done: 6 } },
];

// Each workflow's last-30-runs status sequence for the heatmap
//   keys: ok | fail | running | wait | skip | none
const workflowHistory = {
  "invoice-intake":           "okokokokokokfaiokokokokokokfaiokokokokokokrunokokokokokokokfai".match(/.{1,3}/g).slice(0, 30).map(t => t === "fai" ? "fail" : t === "run" ? "running" : t === "wai" ? "wait" : "ok"),
  "weekly-training-summary":  ["ok","ok","ok","ok","ok","ok","ok","wait","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok"],
  "linear-weekly-recap":      ["ok","ok","ok","ok","ok","ok","ok","ok","ok","fail","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok"],
  "rss-archive":              Array(30).fill("ok"),
  "twitter-watcher":          ["fail","fail","ok","ok","fail","ok","ok","ok","ok","ok","fail","ok","ok","ok","ok","ok","ok","fail","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok"],
  "daily-digest":             Array(30).fill("ok").map((s, i) => (i === 9 || i === 23) ? "fail" : s),
  "github-issues-sync":       Array(30).fill("ok"),
  "slack-digest":             Array(30).fill("ok").map((s, i) => i === 4 ? "fail" : s),
  "email-classifier":         ["ok","ok","ok","ok","fail","ok","ok","fail","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok"],
  "calendar-block-finder":    Array(30).fill("ok"),
  "stripe-payouts-reconcile": Array(30).fill("ok").map((s, i) => i >= 27 ? "fail" : s),
  "ynab-transactions-ingest": Array(30).fill("ok"),
  "rss-to-todoist":           Array(30).fill("ok").map((s, i) => i === 15 ? "fail" : s),
  "obsidian-daily-build":     Array(30).fill("ok"),
  "ha-presence-watcher":      ["ok","ok","ok","ok","ok","ok","fail","ok","ok","ok","ok","ok","ok","ok","ok","fail","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok","ok"],
};

// Per-flowrun node-level detail (for drill-in DAG + node inspector + Gantt)
const flowrunDetails = {
  fr_8d2a: {
    // invoice-intake currently running, 6 nodes, 3 done, 1 running, 2 pending
    nodes: [
      { id: "n_trig",  kind: "trigger",  label: "fsnotify",        status: "ok",      startedMs: 0,     durationMs: 8,    x: 40,  y: 60 },
      { id: "n_ocr",   kind: "function", label: "pdf_extract_text", status: "ok",     startedMs: 12,    durationMs: 18400, x: 240, y: 60 },
      { id: "n_class", kind: "function", label: "classify_invoice",status: "ok",      startedMs: 18420, durationMs: 6200, x: 440, y: 60 },
      { id: "n_check", kind: "condition",label: "金额 > 1000?",   status: "running", startedMs: 24640, durationMs: 0,    x: 640, y: 60 },
      { id: "n_audit", kind: "handler",  label: "audit_log",       status: "pending", startedMs: null,  durationMs: null, x: 240, y: 220 },
      { id: "n_save",  kind: "handler",  label: "airtable_writer", status: "pending", startedMs: null,  durationMs: null, x: 640, y: 220 },
    ],
    edges: [
      { from: "n_trig", to: "n_ocr" },
      { from: "n_ocr",  to: "n_class" },
      { from: "n_class",to: "n_check" },
      { from: "n_check",to: "n_save" },
      { from: "n_check",to: "n_audit" },
    ],
    selectedNode: "n_class",
    nodeDetails: {
      n_class: {
        input:  { activities: "(elided 18 items)", file: "/Users/sun/Downloads/inv-2026-05-17.pdf" },
        output: { category: "office_supplies", confidence: 0.92, amount_cents: 84200 },
        log: [
          { time: "07:01:18.422", level: "info",  msg: "loaded text (18.4 KB)" },
          { time: "07:01:19.110", level: "info",  msg: "called fn_aggregate_week (cached)" },
          { time: "07:01:24.620", level: "info",  msg: "classified as office_supplies @ 0.92" },
        ],
        retries: 0,
      },
    },
  },
  fr_8d26: {
    // The failed one from yesterday
    nodes: [
      { id: "n_trig",  kind: "trigger",  label: "fsnotify",        status: "ok",   startedMs: 0,     durationMs: 5,    x: 40,  y: 60 },
      { id: "n_ocr",   kind: "function", label: "pdf_extract_text", status: "ok",  startedMs: 8,     durationMs: 14200, x: 240, y: 60 },
      { id: "n_class", kind: "function", label: "classify_invoice",status: "ok",   startedMs: 14210, durationMs: 5800, x: 440, y: 60 },
      { id: "n_check", kind: "condition",label: "金额 > 1000?",   status: "ok",   startedMs: 20020, durationMs: 12,   x: 640, y: 60 },
      { id: "n_save",  kind: "handler",  label: "airtable_writer", status: "fail", startedMs: 20040, durationMs: 4760, x: 640, y: 220, error: "429 rate limited (3 retries)" },
      { id: "n_audit", kind: "handler",  label: "audit_log",       status: "skip", startedMs: null,  durationMs: null, x: 240, y: 220 },
    ],
    edges: [
      { from: "n_trig", to: "n_ocr" },
      { from: "n_ocr",  to: "n_class" },
      { from: "n_class",to: "n_check" },
      { from: "n_check",to: "n_save" },
      { from: "n_check",to: "n_audit" },
    ],
    selectedNode: "n_save",
    nodeDetails: {
      n_save: {
        input:  { table: "invoices", rows: 1, payload_bytes: 412 },
        output: null,
        log: [
          { time: "07:08:40.040", level: "info",  msg: "attempt 1: POST https://api.airtable.com/v0/..." },
          { time: "07:08:42.110", level: "error", msg: "HTTP 429 — rate limited, retry in 1.2s" },
          { time: "07:08:43.350", level: "info",  msg: "attempt 2: POST ..." },
          { time: "07:08:43.620", level: "error", msg: "HTTP 429 — rate limited, retry in 2.4s" },
          { time: "07:08:46.040", level: "info",  msg: "attempt 3: POST ..." },
          { time: "07:08:46.310", level: "error", msg: "HTTP 429 — exhausted retries (3)" },
        ],
        retries: 3,
      },
    },
  },
};

// ────────────────────────────────────────────────────────────────────────────
// Resources (Skills / MCP / Memory / Knowledge — Phase 5)
// ────────────────────────────────────────────────────────────────────────────
const skills = [
  { id: "sk_csv", name: "csv-cleaner", description: "标准化 CSV：列重命名、类型推断、缺失值处理", source: "user" },
  { id: "sk_notion", name: "notion-formatter", description: "把 markdown 转成 Notion block 树", source: "user" },
  { id: "sk_pdf", name: "pdf-table-rip", description: "PDF 表格抽取（Camelot 风格）", source: "builtin" },
];

const mcpServers = [
  { id: "mcp_gh", name: "github", status: "healthy", tools: 12, lastSeen: fmt(ago(0.5)) },
  { id: "mcp_lin", name: "linear", status: "healthy", tools: 8, lastSeen: fmt(ago(1)) },
  { id: "mcp_fs", name: "filesystem", status: "healthy", tools: 5, lastSeen: fmt(ago(0.2)) },
  { id: "mcp_pg", name: "postgres-local", status: "degraded", tools: 4, lastSeen: fmt(ago(12)) },
];

const memories = [
  { id: "mem_1", kind: "user", text: "用户偏好：所有时间戳使用本地时区 Asia/Shanghai", pinned: true },
  { id: "mem_2", kind: "project", text: "weekly-training-summary 输出 Notion database id = 7c2a..." , pinned: true },
  { id: "mem_3", kind: "feedback", text: "用户反馈：不喜欢工具调用过于啰嗦的 reasoning，控制在 80 字以内", pinned: false },
  { id: "mem_4", kind: "reference", text: "Strava API rate limit: 100 reqs / 15 min, 1000 / day", pinned: false },
];

// ────────────────────────────────────────────────────────────────────────────
// API Keys / Models
// ────────────────────────────────────────────────────────────────────────────
const apiKeys = [
  { id: "aki_1", provider: "deepseek", displayName: "personal", masked: "sk-***...d4f2", verified: true, lastUsed: fmt(ago(0.3)) },
  { id: "aki_2", provider: "anthropic", displayName: "work", masked: "sk-ant-***...91xa", verified: true, lastUsed: fmt(ago(60 * 8)) },
  { id: "aki_3", provider: "qwen", displayName: "personal", masked: "sk-***...e9bc", verified: true, lastUsed: fmt(ago(60 * 48)) },
];

const modelConfigs = [
  { scenario: "chat", provider: "deepseek", modelId: "deepseek-chat" },
  { scenario: "title", provider: "deepseek", modelId: "deepseek-chat" },
  { scenario: "summary", provider: "deepseek", modelId: "deepseek-chat" },
  { scenario: "web_summary", provider: "qwen", modelId: "qwen-turbo" },
];

// ────────────────────────────────────────────────────────────────────────────
// Notifications
// ────────────────────────────────────────────────────────────────────────────
const notifications = [
  { id: "n1", type: "approval", title: "weekly-training-summary 等待批准", desc: "Notion 写入草稿已生成，需要确认后正式发布", time: ago(2), unread: true, icon: "Pause" },
  { id: "n2", type: "forge", title: "aggregate_week 创建草稿", desc: "function · v1 pending — 由 chat 锻造产生", time: ago(4), unread: true, icon: "Hammer" },
  { id: "n3", type: "flowrun", title: "invoice-intake 失败", desc: "notion_db_writer: 429 rate limited，已重试 3 次", time: ago(60 * 24), unread: false, icon: "AlertCircle" },
  { id: "n4", type: "mcp", title: "postgres-local 进入 degraded", desc: "连续 3 次心跳失败，将在下次调用时自愈", time: ago(60 * 26), unread: false, icon: "Server" },
  { id: "n5", type: "conversation", title: "对话标题已生成", desc: "『CSV → Notion 同步脚本』", time: ago(60 * 28), unread: false, icon: "MessageSquare" },
];

// ────────────────────────────────────────────────────────────────────────────
// Workflow DAG nodes (for the canvas demo)
// ────────────────────────────────────────────────────────────────────────────
const dagNodes = [
  { id: "n_trig",  kind: "trigger",  label: "Cron 触发",  sub: "30 7 * * 1",  x: 40,  y: 60,  status: "ok" },
  { id: "n_fetch", kind: "function", label: "Fetch Strava", sub: "fn_strava_002", x: 260, y: 60, status: "ok" },
  { id: "n_agg",   kind: "function", label: "Aggregate Week", sub: "fn_aggregate_week · pending", x: 480, y: 60, status: "warn" },
  { id: "n_apr",   kind: "approval", label: "等待确认", sub: "Notion 草稿区", x: 480, y: 200, status: "ok" },
  { id: "n_wr",    kind: "handler",  label: "Notion 写入", sub: "hd_notion_001", x: 260, y: 320, status: "ok" },
  { id: "n_done",  kind: "variable", label: "记录 last_run", sub: "Memory write", x: 40,  y: 320, status: "ok" },
];

const dagEdges = [
  { from: "n_trig",  to: "n_fetch" },
  { from: "n_fetch", to: "n_agg" },
  { from: "n_agg",   to: "n_apr" },
  { from: "n_apr",   to: "n_wr" },
  { from: "n_wr",    to: "n_done" },
];

// ── Workflow versions for wf_weekly_training (3 versions, has pending + deployed != current) ──
const workflowDetails = {
  wf_weekly_training: {
    versions: [
      {
        id: "v3", label: "v3", state: "pending",
        at: "8 分钟前", author: "ai · CSV → Notion 对话",
        summary: "在 Notion 写入前加 approval 节点 + Slack 失败通知",
        description: "每周一早 7:30 把训练数据写到 Notion（带审批 + Slack 通知）。",
        nodes: [
          { id: "n_trig",  kind: "trigger",  label: "Cron 触发",      sub: "30 7 * * 1",        x: 40,  y: 60, retry: 0, timeout: 5,  onError: "fail" },
          { id: "n_fetch", kind: "function", label: "Fetch Strava",   sub: "fn_strava_002",     x: 260, y: 60, retry: 3, timeout: 30, onError: "retry" },
          { id: "n_agg",   kind: "function", label: "Aggregate Week", sub: "fn_aggregate_week", x: 480, y: 60, retry: 1, timeout: 10, onError: "fail" },
          { id: "n_apr",   kind: "approval", label: "等待确认",        sub: "Notion 草稿区",      x: 480, y: 200, retry: 0, timeout: 86400, onError: "fail" },
          { id: "n_wr",    kind: "handler",  label: "Notion 写入",     sub: "hd_notion_001",     x: 260, y: 320, retry: 3, timeout: 30, onError: "fail" },
          { id: "n_done",  kind: "variable", label: "记录 last_run",   sub: "Memory write",      x: 40,  y: 320, retry: 0, timeout: 5,  onError: "skip" },
          { id: "n_slack", kind: "http",     label: "Slack 失败通知",  sub: "POST hooks.slack.com", x: 700, y: 320, retry: 0, timeout: 10, onError: "skip" },
        ],
        edges: [
          { from: "n_trig",  to: "n_fetch" },
          { from: "n_fetch", to: "n_agg" },
          { from: "n_agg",   to: "n_apr" },
          { from: "n_apr",   to: "n_wr" },
          { from: "n_wr",    to: "n_done" },
          { from: "n_wr",    to: "n_slack" },
        ],
      },
      {
        id: "v2", label: "v2", state: "current",
        at: "2 天前", author: "user · 手动",
        summary: "Notion 写入加 approval 节点",
        description: "每周一早 7:30 把训练数据写到 Notion，写之前需人工审批。",
        nodes: [
          { id: "n_trig",  kind: "trigger",  label: "Cron 触发",      sub: "30 7 * * 1",        x: 40,  y: 60, retry: 0, timeout: 5,  onError: "fail" },
          { id: "n_fetch", kind: "function", label: "Fetch Strava",   sub: "fn_strava_002",     x: 260, y: 60, retry: 3, timeout: 30, onError: "retry" },
          { id: "n_agg",   kind: "function", label: "Aggregate Week", sub: "fn_aggregate_week", x: 480, y: 60, retry: 1, timeout: 10, onError: "fail" },
          { id: "n_apr",   kind: "approval", label: "等待确认",        sub: "Notion 草稿区",      x: 480, y: 200, retry: 0, timeout: 86400, onError: "fail" },
          { id: "n_wr",    kind: "handler",  label: "Notion 写入",     sub: "hd_notion_001",     x: 260, y: 320, retry: 1, timeout: 30, onError: "fail" },
          { id: "n_done",  kind: "variable", label: "记录 last_run",   sub: "Memory write",      x: 40,  y: 320, retry: 0, timeout: 5,  onError: "skip" },
        ],
        edges: [
          { from: "n_trig",  to: "n_fetch" },
          { from: "n_fetch", to: "n_agg" },
          { from: "n_agg",   to: "n_apr" },
          { from: "n_apr",   to: "n_wr" },
          { from: "n_wr",    to: "n_done" },
        ],
      },
      {
        id: "v1", label: "v1", state: "deployed", // archived AND deployed in production
        at: "9 天前", author: "ai · 初版",
        summary: "初版 · 直接写 Notion，不经审批",
        description: "每周一早 7:30 把训练数据写到 Notion。",
        nodes: [
          { id: "n_trig",  kind: "trigger",  label: "Cron 触发",      sub: "30 7 * * 1",        x: 40,  y: 60, retry: 0, timeout: 5,  onError: "fail" },
          { id: "n_fetch", kind: "function", label: "Fetch Strava",   sub: "fn_strava_002",     x: 260, y: 60, retry: 3, timeout: 30, onError: "retry" },
          { id: "n_agg",   kind: "function", label: "Aggregate Week", sub: "fn_aggregate_week", x: 480, y: 60, retry: 1, timeout: 10, onError: "fail" },
          { id: "n_wr",    kind: "handler",  label: "Notion 写入",     sub: "hd_notion_001",     x: 260, y: 200, retry: 1, timeout: 30, onError: "fail" },
        ],
        edges: [
          { from: "n_trig",  to: "n_fetch" },
          { from: "n_fetch", to: "n_agg" },
          { from: "n_agg",   to: "n_wr" },
        ],
      },
    ],
  },
};


// ────────────────────────────────────────────────────────────────────────────
// Pending diff for forge detail demo
// ────────────────────────────────────────────────────────────────────────────
const pendingDiff = [
  { type: "hunk", text: "@@ aggregate_week.py @@ +12 / -3" },
  { type: "ctx", code: "def aggregate_week(activities):" },
  { type: "ctx", code: "    by_week = group_by_iso_week(activities)" },
  { type: "del", code: "    return {w: avg([a.pace for a in xs]) for w, xs in by_week.items()}" },
  { type: "add", code: "    out = {}" },
  { type: "add", code: "    for w, xs in by_week.items():" },
  { type: "add", code: "        out[w] = {" },
  { type: "add", code: "            'avg_pace_s_per_km': avg(a.moving_time_s / (a.distance_m/1000) for a in xs)," },
  { type: "add", code: "            'total_climb_m':     sum(a.total_elevation_gain for a in xs)," },
  { type: "add", code: "            'avg_hr':            avg(a.avg_hr for a in xs if a.avg_hr)," },
  { type: "add", code: "            'count':             len(xs)," },
  { type: "add", code: "        }" },
  { type: "add", code: "    return out" },
];

window.Forgify = {
  conversations, activeMessages, forges, flowruns, skills, mcpServers,
  memories, apiKeys, modelConfigs, notifications, dagNodes, dagEdges, pendingDiff,
  workflowDetails,
  workflowHistory, flowrunDetails,
};

// ────────────────────────────────────────────────────────────────────────────
// Documents (Notion-like tree)
// ────────────────────────────────────────────────────────────────────────────
const documents = [
  { id: "d_root", title: "我的文档", kind: "folder", children: [
    { id: "d_strava", title: "Strava 训练数据 schema", kind: "page", body:
`# Strava 训练数据 schema

> 用作 weekly-training-summary workflow 的输入契约。

## Fields

- **id** — Strava activity id
- **type** — \`Run\` / \`Ride\` / \`Swim\` / \`WeightTraining\` / 其他
- **distance_m** — 米
- **moving_time_s** — 秒
- **avg_hr** — 平均心率
- **total_elevation_gain** — 总爬升（米）

## 注意

- Strava 偶尔会缺 \`avg_hr\`，聚合时要跳过 None
- moving_time 不等于 elapsed_time（停表）
- 私密活动需要 OAuth scope \`activity:read_all\`
` },
    { id: "d_notion", title: "Notion database 设计", kind: "page", body:
`# 训练日志 database · Notion

数据库 id: \`7c2a...\`（写在 memory.project 里）

## Columns

| 字段 | 类型 | 来源 |
|---|---|---|
| Week | title | aggregate_week 输出的 isoweek |
| Avg Pace | rich_text | s/km，格式化为 m'ss" |
| Total Climb | number | 米 |
| Avg HR | number | bpm |
| Sessions | number | 训练次数 |
| Notes | rich_text | 由 LLM 补一句话点评 |

## 行为

- 同周已存在则 update，不是新增
- 草稿区先写 status="draft"，approval 后改 "published"
` },
    { id: "d_arch", title: "Forgify 自己的架构笔记", kind: "folder", children: [
      { id: "d_arch_sse", title: "三流 SSE 设计", kind: "page", body:
`# 三流 SSE 设计

后端只有三条 SSE 流：

1. **eventlog** (\`/api/v1/eventlog\`) — chat 流式内容
2. **notifications** (\`/api/v1/notifications\`) — entity 状态变更
3. **forge** (\`/api/v1/forge\`) — trinity 锻造进度

全部 per-user 订阅，client 按 payload 字段 demux。

> 永远不再加第四条流。所有未来需求走 forge 流 + filter，或 Wails native event。
` },
      { id: "d_arch_loop", title: "ReAct loop", kind: "page", body: "" },
      { id: "d_arch_sandbox", title: "Sandbox v2 (mise embed)", kind: "page", body: "" },
    ]},
    { id: "d_idea", title: "想法本", kind: "page", body:
`# 想法本

- 把 Inbox 邮件按主题打标，每周一汇总到 Notion
- 给 weekly-training-summary 加一个『心率区间分布』图表
- 录音转 markdown 笔记（要不要 attach 到对应对话？）
` },
  ]},
];

// ────────────────────────────────────────────────────────────────────────────
// Skill detail: full SKILL.md content
// ────────────────────────────────────────────────────────────────────────────
const skillBodies = {
  sk_csv: {
    frontmatter: {
      name: "csv-cleaner",
      description: "标准化 CSV：列重命名、类型推断、缺失值处理",
      version: "1.4.0",
      license: "MIT",
      scope: "user",
      requires: ["python:3.12", "pandas:>=2.0"],
    },
    body:
`# CSV Cleaner

把任意 CSV 标准化成统一 schema。

## Usage

\`\`\`
{{ csv-cleaner $1 [--columns=auto] [--strict] }}
\`\`\`

- \`$1\` — 输入 CSV 文件路径
- \`--columns\` — \`auto\` 推断 / 也可手写 mapping
- \`--strict\` — 任一行解析失败就抛错

## 关键行为

1. 列名规整成 snake_case
2. 数字列推断 int/float
3. 日期列尝试 ISO-8601 / RFC2822 / "YYYY-MM-DD HH:mm:ss" 三种
4. 缺失值填策略：number → median；text → "" ；bool → false

## 示例

\`\`\`bash
{{ csv-cleaner ./uploads/strava.csv }}
\`\`\`
`,
  },
};

// ────────────────────────────────────────────────────────────────────────────
// MCP server detail: command, args, env, tools, health
// ────────────────────────────────────────────────────────────────────────────
const mcpDetails = {
  mcp_gh: {
    command: "npx",
    args: ["-y", "@modelcontextprotocol/server-github"],
    env: { GITHUB_TOKEN: { masked: "ghp_***...91xa", required: true } },
    tools: [
      { name: "search_repositories", desc: "搜索 GitHub 仓库" },
      { name: "get_file_contents",   desc: "读取文件内容" },
      { name: "create_or_update_file", desc: "创建或更新文件" },
      { name: "list_issues",         desc: "列 issues" },
      { name: "create_issue",        desc: "新建 issue" },
      { name: "list_pull_requests",  desc: "列 PR" },
      { name: "get_pull_request",    desc: "PR 详情" },
      { name: "create_pull_request", desc: "新建 PR" },
      { name: "merge_pull_request",  desc: "合并 PR" },
      { name: "list_commits",        desc: "提交历史" },
      { name: "search_code",         desc: "搜代码" },
      { name: "search_users",        desc: "搜用户" },
    ],
    installLog: [
      { time: "07:01:12", level: "info", msg: "downloading @modelcontextprotocol/server-github@2.6.1" },
      { time: "07:01:14", level: "info", msg: "verifying signature" },
      { time: "07:01:15", level: "info", msg: "spawned pid=84012, capabilities ready" },
      { time: "07:01:16", level: "info", msg: "registered 12 tools" },
    ],
    health: [1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1],
  },
  mcp_pg: {
    command: "/opt/pg-mcp/bin/pg-mcp",
    args: ["--config", "~/.forgify/pg.toml"],
    env: { DATABASE_URL: { masked: "postgres://***", required: true } },
    tools: [
      { name: "query",   desc: "只读 SQL 查询" },
      { name: "explain", desc: "解释执行计划" },
      { name: "schema",  desc: "列出 schema" },
      { name: "tables",  desc: "列出表" },
    ],
    installLog: [
      { time: "00:00:01", level: "info",  msg: "binary cached at /opt/pg-mcp" },
      { time: "00:00:02", level: "info",  msg: "spawned pid=72104" },
      { time: "23:48:11", level: "warn",  msg: "heartbeat timeout (3000ms)" },
      { time: "23:48:14", level: "warn",  msg: "heartbeat timeout (3000ms) — strike 2" },
      { time: "23:48:17", level: "error", msg: "heartbeat timeout (3000ms) — entering degraded" },
    ],
    health: [1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,0,0],
  },
};

// ────────────────────────────────────────────────────────────────────────────
// Function detail: code + multiple versions for diff
// ────────────────────────────────────────────────────────────────────────────
const functionDetails = {
  fn_aggregate_week: {
    versions: [
      {
        id: "v2",
        label: "v2",
        state: "pending",
        at: "3 分钟前",
        author: "ai · CSV → Notion 对话",
        summary: "把按周聚合的返回值从单个 avg_pace 扩展为完整 5 字段（含 climb / hr / count）",
        description: "按周聚合训练数据。返回每周的平均配速、总爬升、平均心率、训练次数等 5 个聚合字段。",
        schema: { inputs: "activities: Activity[]", outputs: "by_week: dict[isoweek, AggregateRow]" },
        code:
`from collections import defaultdict
from isoweek import Week

def aggregate_week(activities):
    """按周聚合训练数据。"""
    by_week = defaultdict(list)
    for a in activities:
        wk = Week.withdate(a.start_date.date())
        by_week[str(wk)].append(a)

    out = {}
    for w, xs in by_week.items():
        out[w] = {
            'avg_pace_s_per_km': avg(a.moving_time_s / (a.distance_m/1000) for a in xs),
            'total_climb_m':     sum(a.total_elevation_gain for a in xs),
            'avg_hr':            avg(a.avg_hr for a in xs if a.avg_hr),
            'count':             len(xs),
        }
    return out


def avg(xs):
    xs = [x for x in xs if x is not None]
    return sum(xs) / len(xs) if xs else None
`,
      },
      {
        id: "v1",
        label: "v1",
        state: "current",
        at: "1 小时前",
        author: "ai · 周报对话",
        summary: "初版：按周聚合 + 单字段平均配速",
        description: "按周聚合训练数据，返回 isoweek → 平均配速 的字典。",
        schema: { inputs: "activities: Activity[]", outputs: "by_week: dict[isoweek, float]" },
        code:
`from collections import defaultdict
from isoweek import Week

def aggregate_week(activities):
    """按周聚合训练数据。"""
    by_week = defaultdict(list)
    for a in activities:
        wk = Week.withdate(a.start_date.date())
        by_week[str(wk)].append(a)
    return {w: avg([a.pace for a in xs]) for w, xs in by_week.items()}


def avg(xs):
    return sum(xs) / len(xs) if xs else None
`,
      },
      {
        id: "v0",
        label: "v0",
        state: "archived",
        at: "1 天前",
        author: "ai · 周报对话",
        summary: "首次实现 · 仅算配速",
        description: "返回 week → pace 的简单字典。",
        schema: { inputs: "activities: Activity[]", outputs: "dict[str, float]" },
        code:
`def aggregate_week(activities):
    return {a.week: a.pace for a in activities}
`,
      },
    ],
    sandbox: "python 3.12.13 · fnenv_a1b2c3",
    runs: [
      { at: "3 min ago", status: "ok",    duration: "230ms", input: "4 activities" },
      { at: "12 min ago", status: "ok",   duration: "218ms", input: "4 activities" },
      { at: "1 hour ago", status: "fail", duration: "12ms",  input: "0 activities — NameError on avg" },
    ],
  },

  // No pending — for previewing the "prod" view
  fn_strava_002: {
    versions: [
      {
        id: "v3", label: "v3", state: "current",
        at: "14 天前",
        author: "user · 手动",
        summary: "支持 since 参数 + 按距离/时间筛选",
        description: "拉取 Strava 最近 N 天的活动数据。支持 since (iso8601) 与 type 筛选。",
        schema: { inputs: "since: str, type?: str", outputs: "activities: Activity[]" },
        code:
`import requests
from datetime import datetime

def strava_recent_activities(since: str, type: str = None):
    """拉取 Strava 最近的活动数据。"""
    params = {"after": int(datetime.fromisoformat(since).timestamp())}
    r = requests.get("https://www.strava.com/api/v3/athlete/activities",
                     params=params,
                     headers={"Authorization": f"Bearer {TOKEN}"})
    items = r.json()
    if type:
        items = [a for a in items if a["type"] == type]
    return [Activity.from_api(a) for a in items]
`,
      },
      {
        id: "v2", label: "v2", state: "archived",
        at: "30 天前", author: "ai · API 调研对话",
        summary: "改用官方 API 替代第三方爬虫",
        description: "拉取 Strava 最近的活动数据。",
        schema: { inputs: "since: str", outputs: "activities: Activity[]" },
        code:
`import requests
from datetime import datetime

def strava_recent_activities(since):
    params = {"after": int(datetime.fromisoformat(since).timestamp())}
    r = requests.get("https://www.strava.com/api/v3/athlete/activities",
                     params=params,
                     headers={"Authorization": f"Bearer {TOKEN}"})
    return [Activity.from_api(a) for a in r.json()]
`,
      },
      {
        id: "v1", label: "v1", state: "archived",
        at: "45 天前", author: "ai · 初版",
        summary: "通过第三方爬虫拉数据（不稳定）",
        description: "scrape Strava activities.",
        schema: { inputs: "user_id: int", outputs: "activities: list" },
        code:
`def strava_recent_activities(user_id):
    # TODO: replace with official API
    return scrape_strava(user_id)
`,
      },
    ],
    sandbox: "python 3.12.13 · fnenv_d2e3f4",
    runs: [
      { at: "12 min ago", status: "ok", duration: "1.8s", input: "since=2026-05-13" },
      { at: "1 day ago",  status: "ok", duration: "1.6s", input: "since=2026-05-12" },
    ],
  },
};

// ────────────────────────────────────────────────────────────────────────────
// Handler detail: methods, config, calls
// ────────────────────────────────────────────────────────────────────────────
const handlerDetails = {
  hd_notion_001: {
    versions: [
      {
        id: "v8", label: "v8", state: "pending",
        at: "5 分钟前", author: "ai · API 重构对话",
        summary: "加 batch_upsert 批量写 + 修复 rate-limit 在大批量时的死锁",
        description: "Notion database writer 含 rate-limit + 批量写 + 自动重试。",
        methods: [
          { name: "__init__",   sig: "(database_id: str, draft_mode: bool = True)",  desc: "握住 Notion client 与目标 db",
            body: `def __init__(self, database_id: str, draft_mode: bool = True):
    self.db = database_id
    self.draft = draft_mode
    self.client = NotionClient(token=self.cfg.notion_token)
    self.limiter = RateLimiter(rps=3)
    self.batch_size = self.cfg.get('batch_size', 50)` },
          { name: "upsert_row", sig: "(key: str, fields: dict) -> str",               desc: "插入或更新；返回 page id",
            body: `def upsert_row(self, key: str, fields: dict) -> str:
    self.limiter.acquire()
    existing = self._find_by_key(key)
    if existing:
        return self.client.pages.update(existing.id, properties=fields).id
    return self.client.pages.create(database_id=self.db, properties=fields).id` },
          { name: "batch_upsert", sig: "(rows: list[dict]) -> list[str]",             desc: "批量 upsert，自动分批 + 限流",
            body: `def batch_upsert(self, rows: list[dict]) -> list[str]:
    ids = []
    for chunk in self._chunked(rows, self.batch_size):
        with self.limiter.batch():
            ids.extend(self._concurrent_upsert(chunk, max_workers=4))
    return ids` },
          { name: "publish",    sig: "(page_id: str) -> None",                        desc: "把 draft 改成 published（幂等）",
            body: `def publish(self, page_id: str) -> None:
    page = self.client.pages.retrieve(page_id)
    if page.properties.get('status') == 'published':
        return  # idempotent
    self.client.pages.update(page_id, properties={'status': 'published'})` },
          { name: "search",     sig: "(query: str, limit: int = 10, cursor: str = None) -> SearchPage", desc: "全文搜 + 分页",
            body: `def search(self, query: str, limit: int = 10, cursor: str = None) -> SearchPage:
    resp = self.client.search(query=query, page_size=limit, start_cursor=cursor)
    return SearchPage(items=resp.results, next_cursor=resp.next_cursor)` },
          { name: "delete",     sig: "(page_id: str) -> None",                        desc: "归档（不真删）",
            body: `def delete(self, page_id: str) -> None:
    self.client.pages.update(page_id, archived=True)` },
        ],
        config: {
          database_id:   { value: "7c2a***...***", masked: true,  secret: false },
          draft_mode:    { value: "true",          masked: false, secret: false },
          notion_token:  { value: "secret_***...d8a", masked: true, secret: true },
          batch_size:    { value: "50",            masked: false, secret: false },
        },
      },
      {
        id: "v7", label: "v7", state: "current",
        at: "2 天前", author: "ai · API 重构对话",
        summary: "把 publish 改成幂等 + 加 search 分页",
        description: "Notion database writer，含 rate-limit 重试。",
        methods: [
          { name: "__init__",   sig: "(database_id: str, draft_mode: bool = True)",  desc: "握住 Notion client 与目标 db",
            body: `def __init__(self, database_id: str, draft_mode: bool = True):
    self.db = database_id
    self.draft = draft_mode
    self.client = NotionClient(token=self.cfg.notion_token)
    self.limiter = RateLimiter(rps=3)` },
          { name: "upsert_row", sig: "(key: str, fields: dict) -> str",               desc: "插入或更新；返回 page id",
            body: `def upsert_row(self, key: str, fields: dict) -> str:
    self.limiter.acquire()
    existing = self._find_by_key(key)
    if existing:
        return self.client.pages.update(existing.id, properties=fields).id
    return self.client.pages.create(database_id=self.db, properties=fields).id` },
          { name: "publish",    sig: "(page_id: str) -> None",                        desc: "把 draft 改成 published（幂等）",
            body: `def publish(self, page_id: str) -> None:
    page = self.client.pages.retrieve(page_id)
    if page.properties.get('status') == 'published':
        return  # idempotent
    self.client.pages.update(page_id, properties={'status': 'published'})` },
          { name: "search",     sig: "(query: str, limit: int = 10, cursor: str = None) -> SearchPage", desc: "全文搜 + 分页",
            body: `def search(self, query: str, limit: int = 10, cursor: str = None) -> SearchPage:
    resp = self.client.search(query=query, page_size=limit, start_cursor=cursor)
    return SearchPage(items=resp.results, next_cursor=resp.next_cursor)` },
          { name: "delete",     sig: "(page_id: str) -> None",                        desc: "归档（不真删）",
            body: `def delete(self, page_id: str) -> None:
    self.client.pages.update(page_id, archived=True)` },
        ],
        config: {
          database_id:   { value: "7c2a***...***", masked: true,  secret: false },
          draft_mode:    { value: "true",          masked: false, secret: false },
          notion_token:  { value: "secret_***...d8a", masked: true, secret: true },
        },
      },
      {
        id: "v6", label: "v6", state: "archived",
        at: "8 天前", author: "ai · 用户反馈对话",
        summary: "支持 rich_text 字段类型",
        description: "Notion database writer.",
        methods: [
          { name: "__init__",   sig: "(database_id: str)",                            desc: "握住 Notion client",
            body: `def __init__(self, database_id: str):
    self.db = database_id
    self.client = NotionClient(token=self.cfg.notion_token)` },
          { name: "upsert_row", sig: "(key: str, fields: dict) -> str",               desc: "插入或更新",
            body: `def upsert_row(self, key: str, fields: dict) -> str:
    existing = self._find_by_key(key)
    if existing:
        return self.client.pages.update(existing.id, properties=fields).id
    return self.client.pages.create(database_id=self.db, properties=fields).id` },
          { name: "publish",    sig: "(page_id: str) -> None",                        desc: "发布草稿",
            body: `def publish(self, page_id: str) -> None:
    self.client.pages.update(page_id, properties={'status': 'published'})` },
          { name: "search",     sig: "(query: str, limit: int = 10) -> list[Page]",   desc: "全文搜（无分页）",
            body: `def search(self, query: str, limit: int = 10) -> list[Page]:
    return self.client.search(query=query, page_size=limit).results` },
          { name: "delete",     sig: "(page_id: str) -> None",                        desc: "归档",
            body: `def delete(self, page_id: str) -> None:
    self.client.pages.update(page_id, archived=True)` },
        ],
        config: {
          database_id:   { value: "7c2a***...***", masked: true,  secret: false },
          notion_token:  { value: "secret_***...d8a", masked: true, secret: true },
        },
      },
    ],
    callStats: { ok: 887, fail: 4, p50: 184, p95: 612, p99: 1840 },
    recentCalls: [
      { at: "1 分钟前", method: "upsert_row", status: "ok", ms: 184 },
      { at: "1 分钟前", method: "upsert_row", status: "ok", ms: 201 },
      { at: "2 分钟前", method: "publish",    status: "ok", ms: 92  },
      { at: "1 天前",   method: "upsert_row", status: "fail", ms: 30012, error: "429 rate limited" },
    ],
  },
};

window.Forgify = Object.assign(window.Forgify, {
  documents, skillBodies, mcpDetails, functionDetails, handlerDetails,
});

// ────────────────────────────────────────────────────────────────────────────
// Entity relationships — the graph of which things reference which.
// ────────────────────────────────────────────────────────────────────────────
// Each edge: { from, to, kind }
//   uses          — workflow uses function/handler/mcp/skill
//   forged_from   — forge produced by conversation (AI 锻造)
//   instance_of   — flowrun belongs to workflow
//   attached_to   — document attached to conversation
//   referenced_in — entity mentioned in document body
//   about         — memory about workflow/function
//   uses_doc      — workflow/function uses document as input/schema
const relations = [
  // Workflow weekly-training-summary uses these
  { from: "wf_weekly_training", to: "fn_strava_002",   kind: "uses" },
  { from: "wf_weekly_training", to: "fn_aggregate_week", kind: "uses" },
  { from: "wf_weekly_training", to: "hd_notion_001",  kind: "uses" },
  { from: "wf_weekly_training", to: "d_strava",       kind: "uses_doc" },
  { from: "wf_weekly_training", to: "d_notion",       kind: "uses_doc" },

  // invoice-intake workflow
  { from: "wf_invoice_intake",  to: "fn_pdf_extract",  kind: "uses" },
  { from: "wf_invoice_intake",  to: "fn_csv_parse",    kind: "uses" },
  { from: "wf_invoice_intake",  to: "hd_airtable_002", kind: "uses" },

  // Forging conversations
  { from: "cv_a1", to: "wf_weekly_training",  kind: "forged_from" },
  { from: "cv_a1", to: "fn_aggregate_week",   kind: "forged_from" },
  { from: "cv_b2", to: "wf_weekly_training",  kind: "discussed_in" },
  { from: "cv_d4", to: "wf_invoice_intake",   kind: "forged_from" },
  { from: "cv_d4", to: "fn_pdf_extract",      kind: "forged_from" },

  // Documents attached to conversations
  { from: "cv_a1", to: "d_strava",            kind: "attached_to" },
  { from: "cv_a1", to: "d_notion",            kind: "attached_to" },

  // Doc references inside doc bodies
  { from: "d_strava", to: "fn_strava_002",    kind: "referenced_in" },
  { from: "d_notion", to: "hd_notion_001",    kind: "referenced_in" },
  { from: "d_arch_sse", to: "wf_weekly_training", kind: "referenced_in" },

  // FlowRuns
  { from: "fr_8d2a", to: "wf_invoice_intake", kind: "instance_of" },
  { from: "fr_8d29", to: "wf_weekly_training", kind: "instance_of" },
  { from: "fr_8d28", to: "wf_weekly_training", kind: "instance_of" }, // changed for graph density
  { from: "fr_8d27", to: "wf_invoice_intake", kind: "instance_of" },
  { from: "fr_8d26", to: "wf_invoice_intake", kind: "instance_of" },
  { from: "fr_8d25", to: "wf_invoice_intake", kind: "instance_of" },

  // Memories about
  { from: "mem_1", to: "wf_weekly_training",  kind: "about" },
  { from: "mem_2", to: "wf_weekly_training",  kind: "about" },
  { from: "mem_4", to: "fn_strava_002",       kind: "about" },

  // Skills used
  { from: "wf_invoice_intake",  to: "sk_pdf",     kind: "uses" },
  { from: "fn_pdf_extract",     to: "sk_pdf",     kind: "uses" },

  // MCP usage
  { from: "wf_invoice_intake",  to: "mcp_fs",     kind: "uses" },
];

Forgify.relations = relations;

// ── Append more relations to make every entity kind interconnected ─────
const moreRelations = [
  // Conversation → Memory (which conv produced which memory)
  { from: "cv_a1", to: "mem_2",  kind: "produced" },
  { from: "cv_b2", to: "mem_3",  kind: "produced" },
  { from: "cv_c3", to: "mem_4",  kind: "produced" },

  // Conversations forging more entities
  { from: "cv_c3", to: "fn_strava_002", kind: "forged_from" },
  { from: "cv_e5", to: "hd_notion_001", kind: "discussed_in" },
  { from: "cv_h8", to: "fn_csv_parse",  kind: "discussed_in" },

  // Memory references between
  { from: "mem_1", to: "cv_a1",        kind: "referenced_in" },

  // Skill referenced in docs
  { from: "d_arch_loop", to: "sk_csv",    kind: "referenced_in" },
  { from: "d_arch_sandbox", to: "mcp_fs", kind: "referenced_in" },

  // MCP <-> conversation
  { from: "cv_e5", to: "mcp_gh",  kind: "uses" },
  { from: "cv_f6", to: "mcp_lin", kind: "uses" },

  // Workflow references doc for output schema
  { from: "wf_invoice_intake", to: "d_strava",   kind: "uses_doc" },
  { from: "wf_invoice_intake", to: "d_arch_sse", kind: "uses_doc" },

  // Cross handler/skill
  { from: "hd_notion_001",  to: "sk_notion", kind: "uses" },
  { from: "hd_airtable_002", to: "mcp_pg",   kind: "uses" },

  // Function → MCP
  { from: "fn_pdf_extract", to: "mcp_fs", kind: "uses" },
];

Forgify.relations = Forgify.relations.concat(moreRelations);

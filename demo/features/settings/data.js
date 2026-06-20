/* Anselm feature — settings 种子数据（mock）。设置六类页的展示数据，镜像后端真实形态（workspace/apikey/model/mcp/skill/sandbox/limits/free-tier）。 */
window.SETTINGS = {
  // ① 通用
  workspace: { name: "Personal", color: "海蓝", language: "中文 (zh-CN)" },

  // ② 模型与 Key
  freeTier: { label: "Anselm Free", model: "deepseek-v4-flash", quotaUsed: 1800, quotaLimit: 5000, resetAt: "07-01", enabled: false },
  defaults: [
    { scenario: "对话", hint: "用户对话主回合", model: "Claude Opus 4.8" },
    { scenario: "工具", hint: "标题 / 摘要 / 杂活", model: "GPT-5 mini" },
    { scenario: "Agent", hint: "Agent 实体调用", model: "Claude Sonnet 4.6" },
  ],
  keys: [
    { provider: "Anthropic", icon: "agent", name: "个人 key", masked: "sk-ant-•••• a91f", status: "ok", models: 4 },
    { provider: "OpenAI", icon: "web", name: "团队 key", masked: "sk-•••• 7c20", status: "ok", models: 6 },
    { provider: "Ollama", icon: "handler", name: "本地", masked: "127.0.0.1:11434", status: "error", models: 0, err: "连不上" },
  ],
  searchKey: "Brave · 个人 key",

  // ③ MCP 与市场
  mcpMarket: [
    { name: "GitHub", desc: "代码仓库 · issue · PR", auth: "token", authLabel: "需 token" },
    { name: "Notion", desc: "笔记 · 数据库", auth: "oauth", authLabel: "OAuth" },
    { name: "Linear", desc: "项目 · issue 跟踪", auth: "token", authLabel: "需 token" },
    { name: "Box", desc: "网盘 · 文件", auth: "byo", authLabel: "需自建应用" },
    { name: "Glean", desc: "企业搜索", auth: "oauth-url", authLabel: "OAuth · 填 URL" },
    { name: "Figma", desc: "设计稿 Dev Mode", auth: "local", authLabel: "本地" },
    { name: "Stripe", desc: "支付 · 账单", auth: "token", authLabel: "需 token" },
    { name: "Sentry", desc: "错误监控", auth: "oauth", authLabel: "OAuth" },
    { name: "Supabase", desc: "Postgres · 后端", auth: "token", authLabel: "需 token" },
  ],
  mcpInstalled: [
    { name: "github", status: "ready", tools: 28, source: "市场" },
    { name: "filesystem", status: "ready", tools: 11, source: "市场" },
    { name: "notion", status: "degraded", tools: 15, source: "市场" },
    { name: "postgres", status: "ready", tools: 6, source: "市场" },
    { name: "slack", status: "failed", tools: 0, source: "市场", err: "需重新授权" },
  ],

  // ④ 技能
  skills: [
    { name: "release-notes", desc: "从 PR 生成发布说明", source: "user" },
    { name: "triage-flowrun", desc: "诊断失败的 flowrun", source: "ai" },
    { name: "code-review", desc: "审查 diff 的正确性", source: "user" },
  ],

  // ⑤ 运行时与索引
  embedder: "builtin",
  embedderStatus: "就绪 · embeddinggemma-300m",
  runtimes: [
    { kind: "python", version: "3.12.13", size: "82 MB" },
    { kind: "node", version: "22.22.3", size: "64 MB" },
    { kind: "uv", version: "0.11.4", size: "31 MB" },
  ],
  diskUsage: "1.9 GB",
  bootstrap: "ok",

  // ⑥ 高级（运行上限：13 字段 / 5 段）
  limits: [
    ["Agent", [["最大步数", "25"], ["调用轮数", "10"]]],
    ["上下文", [["压缩触发比例", "0.80"]]],
    ["超时（秒）", [["LLM 空闲", "150"], ["MCP 调用", "180"], ["Bash 默认", "120"], ["Function 运行", "300"], ["Agent 调用", "900"]]],
    ["工具", [["Read 默认行数", "2000"], ["Bash 输出上限 (KB)", "256"], ["工具结果上限 (KB)", "256"]]],
    ["护栏", [["附件上限 (MB)", "50"], ["Webhook body (MB)", "10"]]],
  ],
};

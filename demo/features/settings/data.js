/* Anselm feature — settings 种子数据（mock）。设置六类页的展示数据，镜像后端真实形态（workspace/apikey/model/mcp/skill/sandbox/limits/free-tier）。 */
window.SETTINGS = {
  // ① 通用
  workspace: { name: "Personal", color: "海蓝", language: "中文 (zh-CN)" },

  // ② 模型与 Key —— 镜像后端：GET /providers + api_keys 单表 + model-capabilities + workspace 三场景默认（见 WRK-034）
  // provider 目录（ProviderMeta + 前端图标 glyph；图标后端不给、前端配）。12 LLM（mock 不入 UI）+ 4 search。
  providers: [
    { name: "openai", label: "OpenAI", glyph: "AI", base: "https://api.openai.com/v1", category: "llm" },
    { name: "anthropic", label: "Anthropic", glyph: "An", base: "https://api.anthropic.com", category: "llm" },
    { name: "google", label: "Google Gemini", glyph: "G", base: "https://generativelanguage.googleapis.com/v1beta", category: "llm" },
    { name: "deepseek", label: "DeepSeek", glyph: "DS", base: "https://api.deepseek.com", category: "llm" },
    { name: "anselm", label: "Anselm Free", glyph: "✦", base: "https://api.anselm.website/v1", category: "llm", managed: true },
    { name: "openrouter", label: "OpenRouter", glyph: "OR", base: "https://openrouter.ai/api/v1", category: "llm" },
    { name: "qwen", label: "通义千问", glyph: "通", base: "https://dashscope.aliyuncs.com/compatible-mode/v1", category: "llm" },
    { name: "zhipu", label: "智谱 GLM", glyph: "智", base: "https://open.bigmodel.cn/api/paas/v4", category: "llm" },
    { name: "moonshot", label: "Moonshot Kimi", glyph: "K", base: "https://api.moonshot.cn/v1", category: "llm" },
    { name: "doubao", label: "字节豆包", glyph: "豆", base: "https://ark.cn-beijing.volces.com/api/v3", category: "llm" },
    { name: "ollama", label: "Ollama", glyph: "L", base: "", baseReq: true, category: "llm" },
    { name: "custom", label: "Custom 兼容", glyph: "⚙", base: "", baseReq: true, apiFormat: true, category: "llm" },
    { name: "brave", label: "Brave Search", glyph: "B", base: "https://api.search.brave.com/res/v1", category: "search" },
    { name: "serper", label: "Serper.dev", glyph: "S", base: "https://google.serper.dev", category: "search" },
    { name: "tavily", label: "Tavily", glyph: "T", base: "https://api.tavily.com", category: "search" },
    { name: "bocha", label: "博查 Bocha", glyph: "博", base: "https://api.bochaai.com/v1", category: "search" },
  ],
  // 已配 key（api_keys 行，含 anselm managed 免费档）。id=aki_*；status=test_status；managed 行不可改删。
  keys: [
    { id: "aki_anselm", provider: "anselm", name: "免费额度", masked: "gwk_•••• 8c0a", status: "ok", managed: true, quota: { used: 1800, limit: 5000, resetAt: "07-01" } },
    { id: "aki_anthropic", provider: "anthropic", name: "个人 key", masked: "sk-ant-•••• a91f", status: "ok" },
    { id: "aki_openai", provider: "openai", name: "团队 key", masked: "sk-•••• 7c20", status: "ok" },
    { id: "aki_ollama", provider: "ollama", name: "本地", masked: "127.0.0.1:11434", status: "error", err: "连不上" },
    { id: "aki_brave", provider: "brave", name: "个人 key", masked: "BSA•••• f3d1", status: "ok" },
  ],
  // model-capabilities（GET /api/v1/model-capabilities）：每把 key → 可用 model + 每 model 的 knobs（per-model，换 model 换一套）。
  modelCaps: {
    aki_anselm: [
      { modelId: "deepseek-v4-flash", label: "DeepSeek V4 Flash", ctx: 1000000, knobs: [] },
    ],
    aki_anthropic: [
      { modelId: "claude-opus-4-8", label: "Claude Opus 4.8", ctx: 1000000, knobs: [
        { key: "thinking", label: "思考", type: "enum", values: ["adaptive", "disabled"], default: "adaptive" },
        { key: "effort", label: "强度", type: "enum", values: ["low", "medium", "high", "xhigh", "max"], default: "high" },
      ] },
      { modelId: "claude-sonnet-4-6", label: "Claude Sonnet 4.6", ctx: 200000, knobs: [
        { key: "thinking", label: "思考", type: "enum", values: ["adaptive", "enabled", "disabled"], default: "adaptive" },
        { key: "effort", label: "强度", type: "enum", values: ["low", "medium", "high", "xhigh", "max"], default: "high" },
      ] },
      { modelId: "claude-haiku-4-5", label: "Claude Haiku 4.5", ctx: 200000, knobs: [] },
    ],
    aki_openai: [
      { modelId: "gpt-5.5", label: "GPT-5.5", ctx: 400000, knobs: [
        { key: "reasoning_effort", label: "推理强度", type: "enum", values: ["none", "low", "medium", "high", "xhigh"], default: "medium" },
        { key: "verbosity", label: "详尽度", type: "enum", values: ["low", "medium", "high"], default: "medium" },
      ] },
      { modelId: "gpt-5-mini", label: "GPT-5 mini", ctx: 400000, knobs: [
        { key: "reasoning_effort", label: "推理强度", type: "enum", values: ["minimal", "low", "medium", "high"], default: "medium" },
      ] },
      { modelId: "o3", label: "o3", ctx: 200000, knobs: [
        { key: "reasoning_effort", label: "推理强度", type: "enum", values: ["low", "medium", "high"], default: "medium" },
      ] },
    ],
    aki_ollama: [],
  },
  // 三场景默认（workspace 三列）：ModelRef {apiKeyId, modelId, options}
  defaults: [
    { scenario: "dialogue", label: "对话", hint: "用户对话主回合 · subagent", ref: { apiKeyId: "aki_anthropic", modelId: "claude-opus-4-8", options: { thinking: "adaptive", effort: "high" } } },
    { scenario: "utility", label: "工具", hint: "标题 · 摘要 · 压缩 · 搜索精选", ref: { apiKeyId: "aki_openai", modelId: "gpt-5-mini", options: { reasoning_effort: "low" } } },
    { scenario: "agent", label: "Agent", hint: "Agent 实体调用", ref: { apiKeyId: "aki_anthropic", modelId: "claude-sonnet-4-6", options: {} } },
  ],
  defaultSearchKeyId: "aki_brave",

  // ③ MCP —— 镜像后端：catalog 白名单(96 server)经 GitHub MCP Registry(api.mcp.github.com) 解析；icon/stars/lang 来自 registry，auth 来自 catalog overlay（见 WRK-035）。市场取 34 代表（覆盖全 auth 类型 + prereq）。
  // auth: direct(免配置·47+2) | token(填 token·24) | oauth(浏览器授权·18) | byo(自建应用·3) | oauth-url(填实例URL·1) | local(本地·figma)
  mcpMarket: [
    {slug:"microsoft/markitdown",name:"Markitdown",icon:"https://avatars.githubusercontent.com/u/6154722?v=4",desc:"Convert various file formats (PDF, Word, Excel, images, audio) to Mark",stars:"157k",lang:"Python",auth:"direct"},
    {slug:"io.github.netdata/mcp-server",name:"Netdata",icon:"https://avatars.githubusercontent.com/u/43390781?v=4",desc:"Real-time infrastructure monitoring with metrics, logs, alerts, and ML",stars:"79k",lang:"Go",auth:"token"},
    {slug:"io.github.upstash/context7",name:"Context7",icon:"https://avatars.githubusercontent.com/u/74989412?v=4",desc:"Up-to-date code docs for any prompt",stars:"58k",lang:"TypeScript",auth:"direct"},
    {slug:"io.github.ChromeDevTools/chrome-devtools-mcp",name:"Chrome DevTools MCP",icon:"https://avatars.githubusercontent.com/u/11260967?v=4",desc:"MCP server for Chrome DevTools",stars:"44k",lang:"TypeScript",auth:"direct"},
    {slug:"microsoft/playwright-mcp",name:"Playwright",icon:"https://avatars.githubusercontent.com/u/6154722?v=4",desc:"Automate web browsers using accessibility trees for testing and data e",stars:"34k",lang:"TypeScript",auth:"direct"},
    {slug:"io.github.github/github-mcp-server",name:"GitHub",icon:"https://avatars.githubusercontent.com/u/9919?v=4",desc:"Connect AI assistants to GitHub - manage repos, issues, PRs, and workf",stars:"31k",lang:"Go",auth:"token"},
    {slug:"oraios/serena",name:"Serena",icon:"https://avatars.githubusercontent.com/u/181485370?v=4",desc:"Semantic code retrieval & editing tools for coding agents.",stars:"26k",lang:"Python",auth:"direct"},
    {slug:"coplaydev/unity-mcp",name:"Unity",icon:"https://avatars.githubusercontent.com/u/188132522?v=4",desc:"Control the Unity Editor from MCP clients via a Unity bridge + local P",stars:"11k",lang:"C#",auth:"direct",prereq:true},
    {slug:"firecrawl/firecrawl-mcp-server",name:"Firecrawl",icon:"https://avatars.githubusercontent.com/u/135057108?v=4",desc:"Extract web data with Firecrawl",stars:"7k",lang:"JavaScript",auth:"token"},
    {slug:"io.github.wonderwhy-er/desktop-commander",name:"Desktop Commander",icon:"https://avatars.githubusercontent.com/u/1150639?u=a63bab27280afb38b44ceea84590bb8ad9321c92&v=4",desc:"MCP server for terminal commands, file operations, and process managem",stars:"6k",lang:"TypeScript",auth:"direct"},
    {slug:"makenotion/notion-mcp-server",name:"Notion",icon:"https://avatars.githubusercontent.com/u/4792552?v=4",desc:"Official MCP server for Notion API",stars:"4k",lang:"TypeScript",auth:"oauth"},
    {slug:"com.microsoft/azure",name:"Azure MCP Server",icon:"https://avatars.githubusercontent.com/u/6154722?v=4",desc:"All Azure MCP tools to create a seamless connection between AI agents ",stars:"3k",lang:"C#",auth:"direct"},
    {slug:"com.microsoft/microsoft-fabric",name:"Microsoft Fabric MCP Server",icon:"https://avatars.githubusercontent.com/u/6154722?v=4",desc:"MCP tools for interacting with Microsoft Fabric",stars:"3k",lang:"C#",auth:"direct"},
    {slug:"io.github.bytebase/dbhub",name:"DBHub",icon:"https://avatars.githubusercontent.com/u/74386897?v=4",desc:"Minimal, token-efficient Database MCP Server for PostgreSQL, MySQL, SQ",stars:"3k",lang:"TypeScript",auth:"direct",prereq:true},
    {slug:"com.supabase/mcp",name:"Supabase",icon:"https://supabase.com/favicon/favicon-16x16.png",desc:"MCP server for interacting with the Supabase platform",stars:"3k",lang:"TypeScript",auth:"oauth"},
    {slug:"io.github.brightdata/brightdata-mcp",name:"Brightdata",icon:"https://avatars.githubusercontent.com/u/213028976?v=4",desc:"Bright Data's Web MCP server enabling AI agents to search, extract & n",stars:"2k",lang:"JavaScript",auth:"direct"},
    {slug:"io.github.tavily-ai/tavily-mcp",name:"Tavily",icon:"https://avatars.githubusercontent.com/u/170207473?v=4",desc:"MCP server for advanced web search using Tavily",stars:"2k",lang:"JavaScript",auth:"direct"},
    {slug:"microsoft/azure-devops-mcp",name:"Azure DevOps",icon:"https://avatars.githubusercontent.com/u/6154722?v=4",desc:"Interact with Azure DevOps services like repositories, work items, bui",stars:"2k",lang:"TypeScript",auth:"direct"},
    {slug:"microsoftdocs/mcp",name:"Microsoft Learn",icon:"https://avatars.githubusercontent.com/u/22479449?v=4",desc:"Enables clients like GitHub Copilot and other AI agents to bring trust",stars:"2k",lang:"TypeScript",auth:"direct"},
    {slug:"com.stripe/mcp",name:"Stripe",icon:"https://avatars.githubusercontent.com/u/856813?v=4",desc:"MCP server integrating with Stripe - tools for customers, products, pa",stars:"2k",lang:"TypeScript",auth:"token"},
    {slug:"com.microsoft/nuget",name:"Microsoft Nuget",icon:"https://avatars.githubusercontent.com/u/968310?v=4",desc:"A Model Context Protocol (MCP) server for NuGet.",stars:"2k",lang:"HTML",auth:"direct"},
    {slug:"io.github.hashicorp/terraform-mcp-server",name:"Terraform",icon:"https://avatars.githubusercontent.com/u/761456?v=4",desc:"Generate more accurate Terraform and automate workflows for HCP Terraf",stars:"1k",lang:"Go",auth:"direct"},
    {slug:"com.apify/apify-mcp-server",name:"Apify",icon:"https://avatars.githubusercontent.com/u/24586296?v=4",desc:"Extract data from any website with thousands of scrapers, crawlers, an",stars:"1k",lang:"TypeScript",auth:"token"},
    {slug:"io.github.mongodb-js/mongodb-mcp-server",name:"Mongodb",icon:"https://avatars.githubusercontent.com/u/11214950?v=4",desc:"MongoDB Model Context Protocol Server",stars:"1k",lang:"TypeScript",auth:"direct"},
    {slug:"com.atlassian/atlassian-mcp-server",name:"Atlassian",icon:"https://avatars.githubusercontent.com/u/168166?v=4",desc:"Atlassian Rovo MCP Server",stars:"792",lang:"JavaScript",auth:"oauth"},
    {slug:"io.github.vercel/next-devtools-mcp",name:"Vercel Next Dev Tools",icon:"https://avatars.githubusercontent.com/u/14985020?v=4",desc:"Next.js development tools MCP server with stdio transport",stars:"769",lang:"TypeScript",auth:"direct"},
    {slug:"io.github.getsentry/sentry-mcp",name:"Getsentry Sentry",icon:"https://avatars.githubusercontent.com/u/1396951?v=4",desc:"MCP server for Sentry - error monitoring, issue tracking, and debuggin",stars:"734",lang:"TypeScript",auth:"oauth"},
    {slug:"elastic/mcp-server-elasticsearch",name:"Elasticsearch",icon:"https://avatars.githubusercontent.com/u/6764390?v=4",desc:"MCP server for connecting to Elasticsearch data and indices. Supports ",stars:"675",lang:"Rust",auth:"direct"},
    {slug:"neondatabase/mcp-server-neon",name:"Neon",icon:"https://avatars.githubusercontent.com/u/77690634?v=4",desc:"MCP server for interacting with Neon Management API and databases",stars:"611",lang:"TypeScript",auth:"oauth"},
    {slug:"io.github.SonarSource/sonarqube-mcp-server",name:"SonarSource Sonarqube",icon:"https://avatars.githubusercontent.com/u/545988?v=4",desc:"An MCP server that enables integration with SonarQube Server or Cloud ",stars:"580",lang:"Java",auth:"direct"},
    {slug:"chroma-core/chroma-mcp",name:"Chroma",icon:"https://avatars.githubusercontent.com/u/105881770?v=4",desc:"Provides data retrieval capabilities powered by Chroma, enabling AI mo",stars:"568",lang:"Python",auth:"direct"},
    {slug:"doist/todoist-ai",name:"Todoist",icon:"https://avatars.githubusercontent.com/u/2565372?v=4",desc:"A set of tools to connect to AI agents, to allow them to use Todoist o",stars:"518",lang:"TypeScript",auth:"oauth"},
    {slug:"com.glean/mcp",name:"Glean Remote MCP Server",icon:"https://developers.glean.com/img/glean-logo.svg",desc:"Remote MCP Server that securely connects Glean Enterprise Knowledge wi",stars:"163",lang:"",auth:"oauth-url"},
    {slug:"io.github.microsoft/EnterpriseMCP",name:"Microsoft MCP Server for Enterprise",icon:"https://avatars.githubusercontent.com/u/6154722?v=4",desc:"Official Microsoft MCP Server to query Microsoft Entra data using natu",stars:"44",lang:"",auth:"byo"},
  ],
  // 已装：ServerStatus（status 5 态 / tools / calls / fails / connectedAt / lastError）+ transport（后端 ServerStatus 缺、demo 自带）
  mcpInstalled: [
    {slug:"io.github.github/github-mcp-server",name:"GitHub",icon:"https://avatars.githubusercontent.com/u/9919?v=4",status:"ready",tools:28,calls:1240,fails:3,transport:"stdio",connectedAt:"2 小时前"},
    {slug:"io.github.upstash/context7",name:"Context7",icon:"https://avatars.githubusercontent.com/u/74989412?v=4",status:"ready",tools:2,calls:890,fails:0,transport:"remote",connectedAt:"5 小时前"},
    {slug:"microsoft/playwright-mcp",name:"Playwright",icon:"https://avatars.githubusercontent.com/u/6154722?v=4",status:"ready",tools:21,calls:156,fails:1,transport:"stdio",connectedAt:"1 天前"},
    {slug:"microsoft/markitdown",name:"Markitdown",icon:"https://avatars.githubusercontent.com/u/6154722?v=4",status:"degraded",tools:15,calls:432,fails:7,transport:"remote",connectedAt:"3 天前",err:"上游 503 · 连续失败 7 次"},
    {slug:"io.github.netdata/mcp-server",name:"Netdata",icon:"https://avatars.githubusercontent.com/u/43390781?v=4",status:"failed",tools:0,calls:88,fails:88,transport:"remote",connectedAt:"—",err:"OAuth 授权过期 · 需重新授权"},
    {slug:"io.github.ChromeDevTools/chrome-devtools-mcp",name:"Chrome DevTools MCP",icon:"https://avatars.githubusercontent.com/u/11260967?v=4",status:"connecting",tools:0,calls:0,fails:0,transport:"stdio",connectedAt:"—"},
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

# MCP — V1.2 详设计

**Phase**：Phase 4 准备件（提前到位，本周交付）
**状态**：🔄 D5 已交付（domain types + V1 内置 6 marketplace + ~/.forgify/mcp.json I/O，2026-05-06），D6 待实施（stdio Client + Service runtime + system tools + HTTP + pipeline）
**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../service-contract-documents/database-design.md`](../service-contract-documents/database-design.md) — 无新表（mcp.json 是 source）
- [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md) — mcp ×4（待加）
- [`../service-contract-documents/events-design.md`](../service-contract-documents/events-design.md) — `mcp` entity-state 事件（待加）
- 关联设计：[`subagent.md`](./subagent.md) / [`skill.md`](./skill.md) / [`catalog.md`](./catalog.md)
- 外部 spec：[MCP 2025-11-25](https://modelcontextprotocol.io/specification/2025-11-25)
- 依赖库：[`modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk) v1.x（**官方** SDK，不用 mark3labs/mcp-go——v1 stability + Anthropic+Google 共维）

---

## 1. 一句话

把外部 MCP server 当成"动态 tool 来源"接进来——但**不 flat 注册**到 LLM 的 tool registry，而是暴露 **`search_mcp(query)` + `call_mcp(server, tool, args)` 两个 system tool**，让 LLM 按需召回 + 调用，避免 70+ server × 5 tool × 200 token = 70k 启动开销。

---

## 2. 端到端推演

### 启动期（main.go DI）

```
main.go → mcpapp.NewService(deps)
  → 读 ~/.forgify/mcp.json + 项目 .forgify/mcp.json（merge）
  → 对每个 server（mcp.json 在 = 启用），并发 Connect：
      → infra/mcp.NewStdioClient(cmd, args, env)  # 用官方 go-sdk
      → 子进程启动 + JSON-RPC initialize 握手
      → tools/list RPC → 缓存 ToolDef[] 在 server 状态里
      → 失败 → 标记 server.status=failed + log Warn + 不阻塞其他 server
  → 返回 Service（含已 connect server 的状态 map）
  → 注册 2 个 system tool：mcptool.SearchMCP(svc) + mcptool.CallMCP(svc)
```

### 运行期 — search

```
LLM → tool_use{name="search_mcp", args={query:"github pr"}}
  → mcptool.SearchMCP.Execute(ctx, args)
    → mcpapp.Service.Search(ctx, query)
        → 拉所有 connected server 的所有 ToolDef
        → 拼一段 prompt 让一个 ranking LLM 选 top 5（同 forge search 模式 A）
        → 返回 [{server, name, description, schema}, ...]
    → 序列化成 LLM 可读 JSON 字符串
  → tool_result 给 LLM：top 5 候选
  → LLM 决定调哪个，下一 turn 调 call_mcp
```

### 运行期 — call

```
LLM → tool_use{name="call_mcp", args={server:"github", tool:"create_pr", args:{...}}}
  → mcptool.CallMCP.Execute(ctx, args)
    → mcpapp.Service.CallTool(ctx, server, tool, args)
        → 取 server 的 client，调 tools/call RPC（JSON-RPC over stdio）
        → server 子进程返 content blocks（text/image array）
        → 序列化成 string（image 转 base64 inline 或 ref）
    → 返回给 LLM 当 tool_result
  → LLM 继续
```

### 子进程生命周期

```
Connect 成功 → 启 monitor goroutine：
  - 监听子进程 Wait → 退出立即 mark status=disconnected + Warn 日志
  - 监听 stdin EOF（父进程退出信号）
  - 转发 stderr → zap.L().Named("mcp."+server).Warn(line)
启动 fail → mark failed + 不重试（fail-loud）
主进程退出 → SIGTERM 所有子；超时 5s 后 SIGKILL
```

**关键**：**不静默 auto-restart**——Claude Code 自己都不做，silent restart 会掩盖真正 bug（spec 漂移、子进程 crash 循环等）。

---

## 3. 设计原则

| 原则 | 落地 |
|---|---|
| **Search 模式不 flat** | 每个 server 不暴露 N 个 tool 给 LLM；只 search_mcp + call_mcp 两个，按需召回 |
| **stdio only（v1）** | 不实现 Streamable HTTP；本地单用户场景够用，远程未来 Phase 5+ 再说 |
| **官方 SDK** | `modelcontextprotocol/go-sdk` v1.x，不用 mark3labs（pre-v1 + 民间）|
| **Stdout 污染零容忍** | 首条非 valid JSON-RPC 立即 fail（防子进程 fmt.Println 污染）|
| **stderr → zap** | 子进程 stderr 全转 zap 日志，便于排查 |
| **No OAuth v1** | stdio 模式 spec 明确不该用 OAuth；env 传 secrets 即可 |
| **mcp.json schema 兼容（不共享文件）** | schema 完全照抄 Claude Desktop，**用户可手拷或拖拽进来一份**，但 Forgify 拥有 `~/.forgify/mcp.json` 副本——**不读 Claude Desktop 等外部 app 目录**（自包含原则）|
| **配置即启用，无 enable/disable** | Catalog 已治 token 爆炸，无需"装而不起"。mcp.json 在 → 启动时连；不想要 → 从文件删。简单直接 |
| **仅用户级 mcp.json** | 只有 `~/.forgify/mcp.json` 一份，**没有项目级**——避免 merge 逻辑复杂度，单用户场景下用户级足够 |

---

## 4. 领域模型

### ServerConfig（`internal/domain/mcp/mcp.go`）

```go
type ServerConfig struct {
    Name    string            `json:"name"`              // unique within file
    Command string            `json:"command"`           // 可执行路径
    Args    []string          `json:"args,omitempty"`
    Env     map[string]string `json:"env,omitempty"`     // secrets 走这里
}

type ServerStatus struct {
    Name                string     `json:"name"`
    Status              string     `json:"status"`           // disconnected / connecting / ready / degraded / failed
    PID                 int        `json:"pid,omitempty"`
    ConnectedAt         *time.Time `json:"connectedAt,omitempty"`
    LastError           string     `json:"lastError,omitempty"`
    LastErrorAt         *time.Time `json:"lastErrorAt,omitempty"`
    LastSuccessAt       *time.Time `json:"lastSuccessAt,omitempty"`   // 上次 tools/call 成功时间
    ConsecutiveFailures int        `json:"consecutiveFailures"`       // 连续失败计数（成功一次重置）
    TotalCalls          int64      `json:"totalCalls"`                // 累计 tools/call 次数
    TotalFailures       int64      `json:"totalFailures"`             // 累计失败次数
    Tools               []ToolDef  `json:"tools"`                      // tools/list 缓存
}
```

### ToolDef

```go
type ToolDef struct {
    ServerName  string          `json:"serverName"`
    Name        string          `json:"name"`         // 不带 mcp__ 前缀
    Description string          `json:"description"`
    InputSchema json.RawMessage `json:"inputSchema"`  // 直接转发给 LLM
}
```

### Status 5 值

| 值 | 含义 |
|---|---|
| `disconnected` | 未启动 / 已断开 |
| `connecting` | 子进程已 spawn，正在 initialize 握手 |
| `ready` | 握手成功，正常服务 |
| `degraded` | 子进程还在但近期 tools/call 连续失败 ≥ 3 次（仍可调，但 UI 显示警示）|
| `failed` | 子进程退出 / 握手失败 / stdout 污染检测失败（不可调）|

### Sentinel 错误

```go
var (
    ErrServerNotFound        = errors.New("mcp: server not found")
    ErrServerNotConnected    = errors.New("mcp: server not connected")
    ErrToolNotFound          = errors.New("mcp: tool not found on server")
    ErrToolCallFailed        = errors.New("mcp: tool call failed")
    ErrToolCallTimeout       = errors.New("mcp: tool call timeout")
    // Registry 相关（详 §5.5）
    ErrRegistryEntryNotFound = errors.New("mcp: registry entry not found")
    ErrRuntimeMissing        = errors.New("mcp: runtime (node/python) not available")
    ErrRequiredEnvMissing    = errors.New("mcp: required env variables not provided")
    ErrRequiredArgsMissing   = errors.New("mcp: required args not provided")
    ErrInstallFailed         = errors.New("mcp: install command failed")
)
```

`ErrToolCallFailed` 用 `%w` wrap 上游错误（per §S16），保留 server 自报的失败信息。

---

## 5. mcp.json schema（Claude Desktop 兼容）

```json
{
  "mcpServers": {
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_..."
      }
    },
    "filesystem-extra": {
      "command": "/usr/local/bin/mcp-fs-extra",
      "args": ["--root", "/Users/me/projects"]
    }
  }
}
```

**与 Claude Desktop 的差异**：
- 文件位置：**仅** `~/.forgify/mcp.json`（用户级），**无项目级**
- schema 100% 兼容 Claude Desktop（Claude Desktop 配置直接复制粘贴可用）
- **无 `enabled` 字段**：在文件里 = 启动时连；要禁用某 server → 从文件删（或 UI 调 DELETE）

**为什么没 `enabled` / 没项目级**：
- search_mcp 模式不 flat 注册，token 成本可控，不需要"装而不启"
- 单用户本地场景，用户级一份足够；项目级会引入 merge / override 复杂度而无明显收益

### 自包含原则（重要）

**Forgify 永远不读外部 app 目录**（`~/Library/Application Support/Claude/` / `~/.config/Cursor/` 等）。所有 MCP 配置 source of truth 在 **`~/.forgify/mcp.json`** 一处。

**用户从 Claude Desktop 迁移配置**有两条路径，**全是显式动作**（不是后台映射）：

1. **拖拽导入**（推荐）：UI / testend 拖拽 zone 接收 mcp.json 文件 → 后端 `POST /api/v1/mcp-servers:import` 解析 + merge 到 `~/.forgify/mcp.json` → Forgify 拥有副本
2. **手动复制**：`cp ~/Library/Application\ Support/Claude/claude_desktop_config.json ~/.forgify/mcp.json`

**导入后 Forgify 完全拥有副本**：之后改 Claude Desktop 配置不影响 Forgify；卸载 Claude Desktop 不影响 Forgify；备份 Forgify 只需要 `~/.forgify/`。这才是 desktop app 该有的边界。

---

## 5.5. 内置 Registry — Marketplace UX

### 概念

为解决"用户得知道 npm 包名、要 vim mcp.json、要手填 env"的初次体验问题，Forgify 内置一个**静态 server registry**——告诉用户"这些 server 你可以一键装"，UI 提供安装向导。点 install 后**通过 sandbox 服务 lazy 装 runtime + 包**（详 [`sandbox.md`](./sandbox.md)），不需要预装。

**关键设计**：不 bundle server 二进制（包体积爆炸 + 维护噩梦），只 bundle 元数据（~5-10 KB 编译进 Forgify binary）。真正的 server 由 `npx -y` / `uvx` 等按需下载到用户机器（首次安装联网，之后系统缓存）。

### Registry 数据结构（`internal/domain/mcp/registry.go`）

```go
type RegistryEntry struct {
    Name              string             `json:"name"`         // 用作 mcp.json 的 server name
    DisplayName       string             `json:"displayName"`  // UI 显示
    Description       string             `json:"description"`
    Category          string             `json:"category"`     // "data" / "web" / "doc" / "browser" / "demo" / ...
    Homepage          string             `json:"homepage"`     // 项目主页 URL
    License           string             `json:"license"`      // MIT / Apache-2.0 / ...
    Runtime           string             `json:"runtime"`      // "node" / "python" / "binary"
    Bundled           bool               `json:"bundled"`      // true 时表示 v1 默认 marketplace 推荐项（在 UI 列出）；用户也可加自定义 server，Bundled=false。所有装机一律 lazy via sandbox
    Hidden            bool               `json:"hidden"`       // ⚡ true 时 marketplace UI 不展示（用于 dev/test server）
    InstallCmd        InstallCmd         `json:"installCmd"`
    PostInstallSteps  []PostInstallStep  `json:"postInstallSteps,omitempty"`  // ⚡ Playwright 等需要后续步骤（下载 Chromium 等）
    RequiredEnv       []EnvRequirement   `json:"requiredEnv,omitempty"`
    RequiredArgs      []ArgRequirement   `json:"requiredArgs,omitempty"`
    DefaultTimeoutSec int                `json:"defaultTimeoutSec,omitempty"`  // 0 = 用全局默认 30s
    OnlineOnly        bool               `json:"onlineOnly,omitempty"`         // ⚡ 标记需要持续联网（如 Context7）
    UnsupportedPlatforms []string        `json:"unsupportedPlatforms,omitempty"` // ⚡ 不支持的 GOOS（如 ["windows"]）；marketplace UI 在该 OS 隐藏；空数组 = 全平台支持
    Notes             string             `json:"notes,omitempty"`              // UI 显示的注意事项（如 "扫描件 PDF 不支持"）
}

type InstallCmd struct {
    Command string   `json:"command"`  // "npx" / "uvx"
    Args    []string `json:"args"`     // 含 -y 等；user-provided args 通过模板替换合并进来
}

type PostInstallStep struct {
    Description    string   `json:"description"`     // UI 显示，如 "Downloading Chromium browser (~150MB)"
    Command        string   `json:"command"`
    Args           []string `json:"args"`
    StreamProgress bool     `json:"streamProgress"`  // UI 显示进度条
}

type EnvRequirement struct {
    Name        string `json:"name"`        // "GITHUB_PERSONAL_ACCESS_TOKEN"
    Description string `json:"description"` // UI 提示
    SetupURL    string `json:"setupUrl,omitempty"` // 获取的链接（如 GitHub 设置页）
    Secret      bool   `json:"secret"`      // UI 是否 mask 显示
}

type ArgRequirement struct {
    Name        string `json:"name"`        // "rootPath"
    Description string `json:"description"`
    Type        string `json:"type"`        // "path" / "url" / "string"
    Default     string `json:"default,omitempty"`
}
```

### v1 内置 5 个 server（4 用户可见 + 1 hidden test）

> **安装机制**：通过 [`sandbox.md`](./sandbox.md) 的统一 PluginRuntime 服务**lazy install**——用户点 install → mcpapp 调 `sandboxapp.EnsureEnv(Owner{Kind:"mcp", ID:<server>}, EnvSpec{...})` → sandbox 内部按需拉 runtime + 装包。**没有"预装"概念**——首次安装时联网下载（progress 通过 chat.message tool_call 流推前端）。

筛选原则（基于 web research + 项目情况）：
- **避开与内置 system tool 重复**——不收 fetch / filesystem / git / time（分别已有 WebFetch / Read+Write+Edit+Glob+Grep / Bash 覆盖）
- **避开 OAuth / API key**——开箱体验 体验不能破，github/notion/slack 等 v2+
- **砍跟原生计划撞车的**——不收 memory（Forgify 计划做原生 memory，避免迁移债）
- **保留有 wow factor 的**——演示给非 tech 用户看必须有"卧槽"瞬间

按 demo wow factor 排序：

| Server | Category | Runtime | 需 args | 安装大小 | demo 一句话 |
|---|---|---|---|---|---|
| `playwright` | browser | Node + Chromium | 无（`useSystemChrome` 可选）| ~10 MB + ~150 MB Chromium | "AI 开浏览器、点东西、抓截图给我看"——非 tech 杀手锏 |
| `markitdown` | doc | Python（Forgify 自带 uv）| 无 | ~50 MB | "拖 PDF/PPT/Word，AI 读懂跟我聊"——文档处理杀手锏 |
| `context7` | docs | Node | 无 | ~5 MB | "AI 知道某库**这周** release 的最新 API"——技术用户瞳孔放大 |
| `duckduckgo-search` | web | Python（Forgify 自带 uv）| 无 | ~10 MB | "查今天新闻 / 总结某文章"——0 setup 立即 work，无 API key |
| `sqlite` | data | Python（Forgify 自带 uv）| DB 路径 | ~10 MB | "AI 操作我自己的 SQLite 文件" |
| `everything` | demo | Node | 无 | ~5 MB | **`hidden:true`**——marketplace 不展示；MCP 协议 pipeline test 用 |

**总安装预算**：~245 MB（Chromium 占 150 MB 大头）。在 250 MB 预算内。

### v2+ 候选（不在 v1 开箱体验）

| Server | 推迟原因 |
|---|---|
| `github` | 需 PAT，开箱体验 体验破 |
| `notion` / `slack` / `gmail` / `google-drive` | OAuth 2.1 + DCR 当前业界普遍 broken |
| `obsidian` | 需用户装 Local REST API 插件 |
| `serena` | 每语言 LSP 30-200 MB，磁盘代价过大 |
| `desktop-commander` | 跟内置 Read/Write/Edit/Bash 高度重叠 |
| `firecrawl` / `tavily` | 免费 tier 受限 |
| `memory` | Forgify 计划原生 memory 系统 |
| `sequential-thinking` | 跟主 LLM reasoning block 价值重叠，按用户反馈再加 |
| `postgres` | DSN 配置复杂 |
| `brave-search` | 需注册 API key；DuckDuckGo 已覆盖该需求 |

### Windows 平台过滤

Windows 上 mise 的 Ruby/PHP/Erlang/Elixir 等语言走 bash plugin，跑不起来。mcpapp 启动时按当前 GOOS 过滤 RegistryEntry：

```go
func (s *Service) ListRegistry() []RegistryEntry {
    out := []RegistryEntry{}
    for _, e := range bundledRegistry {
        if slices.Contains(e.UnsupportedPlatforms, runtime.GOOS) {
            continue  // 当前平台不支持 → marketplace 隐藏
        }
        out = append(out, e)
    }
    return out
}
```

**v1 内置 5 个 server 都跨平台**（Python/Node 类）——`UnsupportedPlatforms` 字段全空。
**v2+ 加 Ruby/PHP 等长尾语言 server 时**才标 `["windows"]`。例：
```go
{
    Name: "some-ruby-server",
    Runtime: "ruby",
    UnsupportedPlatforms: []string{"windows"},  // ← 仅 Mac/Linux
    ...
}
```

详见 [`sandbox.md`](./sandbox.md) §17 跨平台支持矩阵。

### Registry 数据来源 — v1 静态，预留远程

**v1 实现**：编译进 binary 的静态 `[]RegistryEntry`（go embed 一份 JSON 也行，便于编辑）。

**预留 future 接口**：`Service.LoadRegistry(ctx)` 内部走 strategy pattern：
```go
type RegistryProvider interface {
    Load(ctx context.Context) ([]RegistryEntry, error)
}
// v1: type embedRegistryProvider struct{}     // 读 embed.FS
// v2: type remoteRegistryProvider struct{ URL string }  // GET 远程 JSON + 24h cache
```

**好处**：未来想增减 server 不用重发 binary——加远程 provider 即可，registry 数据可独立更新。**v1 强制 embed**，远程留接口不实现。

### v1 内置 server 的 RegistryEntry 例子

```go
{
    Name: "playwright",
    DisplayName: "Playwright",
    Description: "Headless browser automation. Open URLs, click elements, fill forms, take screenshots.",
    Category: "browser",
    Homepage: "https://github.com/microsoft/playwright-mcp",
    License: "Apache-2.0",
    Runtime: "node",
    Bundled: true,                              // v1 marketplace 推荐项
    InstallCmd: InstallCmd{
        Command: "npx",
        Args: []string{"-y", "@playwright/mcp"},
    },
    PostInstallSteps: []PostInstallStep{
        {
            Description: "Downloading Chromium browser (~150MB, one-time)",
            Command: "npx", Args: []string{"-y", "playwright", "install", "chromium"},
            StreamProgress: true,
        },
    },
    DefaultTimeoutSec: 60,                      // 浏览器操作天然慢
    Notes: "Optionally use system Chrome via 'useSystemChrome' arg to skip download (advanced)",
},
{
    Name: "markitdown",
    Description: "Convert PDF / DOCX / PPTX / XLSX / images / audio / YouTube to markdown for LLM consumption.",
    Category: "doc",
    Homepage: "https://github.com/microsoft/markitdown",
    License: "MIT",
    Runtime: "python",
    Bundled: true,
    InstallCmd: InstallCmd{Command: "uvx", Args: []string{"markitdown-mcp"}},
    Notes: "Best on text-based PDFs/Office docs. Complex layouts (scanned docs) may extract poorly.",
},
{
    Name: "context7",
    Description: "Up-to-date library docs from Context7.",
    Category: "docs", Runtime: "node", Bundled: true,
    Homepage: "https://github.com/upstash/context7", License: "MIT",
    InstallCmd: InstallCmd{Command: "npx", Args: []string{"-y", "@upstash/context7-mcp"}},
    OnlineOnly: true,                          // 持续需要联网
    Notes: "Calls Context7 service; requires internet. Free tier rate-limited.",
},
{
    Name: "duckduckgo-search",
    Description: "Free web search via DuckDuckGo — no API key required.",
    Category: "web", Runtime: "python", Bundled: true,
    Homepage: "https://github.com/nickclyde/duckduckgo-mcp-server", License: "MIT",
    InstallCmd: InstallCmd{Command: "uvx", Args: []string{"duckduckgo-mcp-server"}},
},
{
    Name: "sqlite",
    Description: "Query and modify a user-specified SQLite database.",
    Category: "data", Runtime: "python", Bundled: true,
    License: "MIT",
    InstallCmd: InstallCmd{Command: "uvx", Args: []string{"mcp-server-sqlite", "--db-path", "${dbPath}"}},
    RequiredArgs: []ArgRequirement{
        {Name: "dbPath", Description: "Absolute path to a .sqlite/.db file", Type: "path"},
    },
},
{
    Name: "everything",
    Description: "MCP protocol reference test server.",
    Category: "demo", Runtime: "node", Bundled: true,
    Hidden: true,                              // marketplace UI 不展示
    InstallCmd: InstallCmd{Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-everything"}},
    Notes: "For Forgify pipeline test only.",
},
```

### Install 机制 — 委托给 sandbox 服务（lazy install）

**所有 MCP server 的 runtime 装机 + 包装 + spawn 都委托给 `sandboxapp` 服务**，详见 [`sandbox.md`](./sandbox.md)。MCP service 只关心"用户要装哪个 server / server connect 状态如何 / tool 调用怎么路由"——runtime 这层 **0 关心**。

**典型 install 流程**：

```
用户点 "Install Playwright MCP"
  → POST /api/v1/mcp-registry/playwright:install
    → mcpapp.InstallFromRegistry(ctx, "playwright", args, env)
      → 校验 RequiredEnv / RequiredArgs 完整
      → 写 ~/.forgify/mcp.json
      → owner := sandboxdomain.Owner{Kind: "mcp", ID: "playwright", Name: "Playwright"}
      → spec := sandboxdomain.EnvSpec{
          Runtime: sandboxdomain.RuntimeSpec{Kind: "node"},
          Deps:    []string{"@playwright/mcp"},
          Extras:  []string{"browsers/chromium"},
        }
      → env, err := sandboxapp.EnsureEnv(ctx, owner, spec, progressFn)
        ↓ sandbox 内部按需 lazy 装：
        ↓   - 没 Node runtime → mise install node@22  (~50MB 下载)
        ↓   - mkdir envs/mcp/playwright/
        ↓   - npm install @playwright/mcp --prefix=envs/mcp/playwright
        ↓   - playwright install chromium  (~150MB 下载)
        ↓   - 写 sandbox_envs DB 行 status=ready
      → mcpapp.Connect("playwright")
        → 用 envs/mcp/playwright/.runtime/node bin → spawn server stdio 子进程
        → tools/list → ready
  ← 返 ServerStatus
```

**首次安装慢，之后秒开**：
- Node runtime 装一次后所有 Node 类 MCP server 共享（fastpath：第二个 npm-only server 装机 ~10s）
- Chromium 装一次后所有 Playwright 类共享
- Python runtime 同理（共享 mise-installed python+uv）

**MCP service 不再有 runtime 检查代码**——`checkRuntime` 那段全部删掉，由 sandbox 负责。MCP layer 只 catch sandbox 返的 `ErrRuntimeMissing` 等错误转友好消息。

### Playwright 系统 Chrome 选项

Playwright 默认下自己 fork 的 Chromium（150 MB）。**用户已装 Chrome 时可选跳过**：

`POST /api/v1/mcp-registry/playwright:install` body：
```json
{
  "args": {
    "useSystemChrome": true,           // 跳过 Chromium 下载
    "userDataDir": "/Users/me/Library/Application Support/Google/Chrome/Default"  // 可选
  }
}
```

后端 `args.useSystemChrome=true` 时：
- 跳过 PostInstallSteps 里的 Chromium 下载
- mcp.json 的 args 加 `--channel chrome`
- 可选 `--user-data-dir <path>`——LLM 继承用户已登录 session（**安全警告**：AI 拿到所有 cookie，UI 必须明确告知）

### MCP 实现工作量

| 工作 | 时间 |
|---|---|
| RegistryEntry struct + 5 个 v1 内置 entry | 2h |
| Install endpoint（委托 sandbox 服务）| 2-3h |
| Server 生命周期（subprocess monitor + restart + health check）| 1 天 |
| search_mcp / call_mcp 两个 system tool | 半天 |
| **MCP 后端总工作量** | **~3 天** |

依赖前置：sandbox v2 已就位（详 [`sandbox.md`](./sandbox.md)，~3-4 天独立工作）。

### Runtime 装机 — 委托 sandbox 服务

**MCP service 不再有 runtime 检查代码**——所有 runtime（Node/Python/Browsers/.NET）的装机、版本管理、env 隔离、spawn 都委托给 `sandboxapp.Service`。

详细机制见 [`sandbox.md`](./sandbox.md)：
- §6 `EnsureRuntime` — lazy install（mise 通配 + Playwright + dotnet + static）
- §6 `EnsureEnv` — 每个 mcp server 一个独立 env（owner=mcp/<name>）
- §8 `SpawnLongLived` — server 子进程 stdio 接 JSON-RPC

MCP layer 仅 catch sandbox 返的 `ErrRuntimeMissing` 等 sentinel 转友好消息——不实现任何装机逻辑。

### License / 合规

| 项 | 状态 |
|---|---|
| MCP 协议 | Anthropic 开源，无须 ask permission |
| modelcontextprotocol/servers 各 server | 全 MIT，自由 bundle 元数据 |
| 我们 bundle 的 | **元数据 + 安装命令**，不是 server 二进制；server 由 npm/pypi 用户运行时下载 |
| Attribution | About 页 + docs："Forgify supports the open Model Context Protocol by Anthropic"，每个 server 链接原项目 |
| 商标边界 | 描述用 "compatible with" / "supports"，**不**僭称 "official" / "endorsed by" |

**结论**：完全合规，无需找任何人。

---

## 5.6. 失效检测与恢复（health monitoring）

### 设计原则

| 原则 | 落地 |
|---|---|
| **被动 health 优先于主动 ping** | 不引入定期 heartbeat（MCP spec 没规定 ping）；用真实 tools/call 的成功/失败累计判断健康度 |
| **Per-call 硬超时** | tools/call 默认 30s，超时即失败（防止 server 卡住整个 chat）|
| **连续失败 → degraded** | ≥ 3 次 tools/call 连续失败标 `degraded`（仍可调，但 UI 警示）|
| **不静默 auto-restart** | 子进程退出 → 标 `failed`，**不**自动重启；用户手动 `:reconnect` |
| **可主动触发健康检查** | UI 提供 "Test Connection" 按钮 → 后端调 tools/list 验证 |

### 失败累计逻辑

```go
// 每次 CallTool 完成后
func (s *Service) recordCallResult(name string, err error) {
    state := s.states[name]
    state.TotalCalls++
    if err != nil {
        state.TotalFailures++
        state.ConsecutiveFailures++
        state.LastError = err.Error()
        state.LastErrorAt = ptr(time.Now())
        if state.ConsecutiveFailures >= 3 && state.Status == "ready" {
            state.Status = "degraded"
            s.bridge.Publish(ctx, "", eventsdomain.MCP{Servers: s.snapshot()})
        }
    } else {
        wasDownGraded := state.Status == "degraded"
        state.ConsecutiveFailures = 0
        state.LastSuccessAt = ptr(time.Now())
        if wasDownGraded {
            state.Status = "ready"  // 自愈
            s.bridge.Publish(ctx, "", eventsdomain.MCP{Servers: s.snapshot()})
        }
    }
}
```

**自愈**：degraded 状态下任何一次 tools/call 成功 → 自动回 ready。

### 主动健康检查方法

```go
// app/mcp/mcp.go 新增
func (s *Service) HealthCheck(ctx context.Context, name string) (*HealthResult, error)

type HealthResult struct {
    ServerName  string        `json:"serverName"`
    Healthy     bool          `json:"healthy"`
    LatencyMs   int           `json:"latencyMs"`        // tools/list RTT
    ToolCount   int           `json:"toolCount"`
    Error       string        `json:"error,omitempty"`
    CheckedAt   time.Time     `json:"checkedAt"`
}
```

**实现**：调 `tools/list` 当探针，10s 超时；记录 RTT。**不**改任何 ServerStatus 字段（避免 UI 测试触发 degraded 误判）。

### 子进程异常分类

| 现象 | 检测方式 | Status 转换 | 是否发 SSE |
|---|---|---|---|
| 子进程退出（exit code）| Wait goroutine | ready → failed | ✅ |
| stdin/stdout 管道 broken | RPC 调用 EOF/PIPE | ready → failed | ✅ |
| Initialize 握手失败（首条非 valid JSON-RPC）| 启动时检测 | connecting → failed | ✅ |
| tools/call 超时 | per-call 30s timer | 视情况 → degraded | ✅（degraded 转换时）|
| tools/call 业务错误（server 返 isError=true）| RPC 解析 | 视情况 → degraded | ✅（同上）|
| stderr 输出（warning）| stderr → zap | 不影响 status | ❌ |

### Stderr ring buffer 防内存爆

子进程长跑可能输出大量 stderr 日志。每 server 维护一个 **256 KB ring buffer**：
- 写入超 256 KB 自动丢最早数据 + Warn 一次（避免日志撑爆内存）
- ring buffer 存最近内容供 UI"查看 server 日志"端点拉取
- 同时全量也走 zap（受 zap 自己的 rotation 控制）

---

## 5.7. 失败/取消传播与 In-flight RPC 处理

### Cancellation 必须级联到子进程

**问题**：LLM 主动 cancel 主对话 → MCP `tools/call` RPC 半路被丢 → 子进程仍在算（孤儿请求）。

**设计**：
- `Service.CallTool(ctx, ...)` 必须传 ctx 进 client
- ctx.Done 时 client 主动给子进程发 MCP spec 标准的 `notifications/cancelled` 通知（含 requestId）
- 同时**本地立即返回** `ErrToolCallTimeout`（不等子进程响应 cancel）
- 子进程是否真停由它自己决定（spec 允许 server 选择忽略 cancel）

```go
func (c *mcpClient) CallTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
    reqID := c.nextID()
    
    // 注册 in-flight Promise
    ch := c.registerInFlight(reqID)
    defer c.unregisterInFlight(reqID)
    
    if err := c.sendRequest(reqID, "tools/call", ...); err != nil {
        return "", err
    }
    
    select {
    case resp := <-ch:
        return resp.Result, resp.Err
    case <-ctx.Done():
        // 通知 server 取消（best-effort，server 可忽略）
        c.sendNotification("notifications/cancelled", map[string]any{"requestId": reqID})
        return "", mcpdomain.ErrToolCallTimeout
    case <-time.After(c.callTimeout):  // 默认 30s，可被 RegistryEntry.DefaultTimeoutSec / mcp.json override
        c.sendNotification("notifications/cancelled", map[string]any{"requestId": reqID})
        return "", mcpdomain.ErrToolCallTimeout
    }
}
```

**Per-server timeout 解析顺序**（高优先级覆盖低）：
1. **mcp.json 里 server 配置**显式 `"timeoutSec": N`（用户最终控制权）
2. **RegistryEntry.DefaultTimeoutSec**（Registry 装的 server 自带 default）
3. **全局默认 30s**（兜底）

`ServerConfig` 加可选字段：
```go
type ServerConfig struct {
    Name       string            `json:"name"`
    Command    string            `json:"command"`
    Args       []string          `json:"args,omitempty"`
    Env        map[string]string `json:"env,omitempty"`
    TimeoutSec int               `json:"timeoutSec,omitempty"`  // 0 = 用 Registry 或全局默认
}
```

### Disconnect 时清理所有 in-flight

**问题**：用户调 DELETE / `:reconnect` 时，正在跑的 `tools/call` Promise 仍然挂着 → goroutine 泄漏。

**设计**：每个 client 维护 `inFlight map[reqID]chan response`。Close() 时遍历全部 reject：

```go
func (c *mcpClient) Close() error {
    c.mu.Lock()
    for reqID, ch := range c.inFlight {
        close(ch)  // reader goroutine 自动得到 ErrServerNotConnected
        delete(c.inFlight, reqID)
    }
    c.mu.Unlock()
    
    return c.subprocess.Close()
}
```

### Tool 名碰撞防护

**问题**：两个 MCP server 都暴露 `search` tool；或 MCP tool 名字撞内置 `Read`。

**设计**：MCP tool 永远走 `mcp__<server>__<name>` 命名空间，**不进全局 tool registry 平铺**——LLM 通过 `call_mcp(server, tool, args)` 间接调用，dispatch 在 mcpapp 内部按 server 路由。这就**结构性免疫**所有命名碰撞：
- `mcp__github__search` 和 `mcp__brave__search` 各自独立路由
- `mcp__filesystem__read` 不会撞内置 `Read`（命名空间不同）
- 内部 catalog source 上报给 catalog 时也带前缀，避免 routing 提示混淆

### mcp.json 损坏处理

**问题**：用户手编时漏个逗号 → JSON parse fail → Service.Start panic？

**设计**：`Service.Start` 加载 mcp.json 失败 → log Error + 当作空 mcp.json 启动（无 server）+ UI 显示提示"mcp.json 损坏，请检查"。**不 panic，不静默重写**——给用户机会自己修。

```go
type Service struct {
    configs map[string]mcpdomain.ServerConfig   // name → config（来自 ~/.forgify/mcp.json）
    states  map[string]*mcpdomain.ServerStatus  // name → live state
    clients map[string]*mcpClient               // name → live client（含 subprocess handle）
    bridge  eventsdomain.Bridge
    log     *zap.Logger
    llm     llmclientpkg.Resolver               // 用于 search ranking
    mu      sync.RWMutex
}

func (s *Service) Start(ctx context.Context) error                // 读 mcp.json + 并发 Connect 全部
func (s *Service) Stop(ctx context.Context) error                 // SIGTERM 所有子进程
func (s *Service) AddServer(ctx context.Context, c ServerConfig) error      // 写 mcp.json + 立即 Connect
func (s *Service) RemoveServer(ctx context.Context, name string) error      // Disconnect + 从 mcp.json 删
func (s *Service) Reconnect(ctx context.Context, name string) error         // 强制重启子进程（debug / 失败恢复）
func (s *Service) ListServers(ctx context.Context) []ServerStatus
func (s *Service) ListTools(ctx context.Context) []ToolDef        // 全 server 拉平
func (s *Service) Search(ctx context.Context, query string, topK int) ([]ToolDef, error)
func (s *Service) CallTool(ctx context.Context, server, tool string, args json.RawMessage) (string, error)  // 默认 30s 超时

// 内置 Registry（详 §5.5）
func (s *Service) ListRegistry() []RegistryEntry
func (s *Service) GetRegistryEntry(name string) (*RegistryEntry, error)
func (s *Service) InstallFromRegistry(ctx context.Context, name string, env map[string]string, args map[string]string) (*ServerStatus, error)

// 健康检查（详 §5.6）
func (s *Service) HealthCheck(ctx context.Context, name string) (*HealthResult, error)
```

### Search 实现（同 forge 模式 A）

```go
func (s *Service) Search(ctx context.Context, query string, topK int) ([]ToolDef, error) {
    all := s.ListTools(ctx)
    if len(all) <= topK {
        return all, nil  // 少时直接全返
    }
    // 拼提示让 LLM 选 top K
    prompt := buildRankingPrompt(query, all)
    bundle, err := s.llm.ResolveForChat(ctx)
    if err != nil { return nil, err }
    response, err := llm.Generate(ctx, bundle.Client, prompt, ...)
    if err != nil { return nil, err }
    indices := parseRankedIndices(response)  // 解析 LLM 返的 [3, 7, 1, 12, 5]
    return pickByIndices(all, indices), nil
}
```

**LLM 排序错也无妨**——LLM 拿 top K 后还会自己判断要不要 call。差的排序结果就是"多搜一次或选错一个 tool 调用浪费 1-2k token"，不致命。

---

## 7. 子进程 wrapper（`internal/infra/mcp/`）

```go
// internal/infra/mcp/mcp.go
type Client interface {
    Initialize(ctx context.Context) error
    ListTools(ctx context.Context) ([]ToolDef, error)
    CallTool(ctx context.Context, name string, args json.RawMessage) (string, error)
    Close() error
}

func NewStdioClient(cmd string, args []string, env map[string]string, log *zap.Logger) (Client, error)
```

**实现**：thin wrapper around `modelcontextprotocol/go-sdk`，主要责任：
1. 起子进程 + 把 stderr pipe 接到 zap
2. 监控子进程 Wait（goroutine），退出立即 emit disconnect 事件给 Service
3. Initialize 时校验首条响应是 valid JSON-RPC，否则立即 fail（防 stdout 污染）
4. Close 优雅关停：SIGTERM → 5s timeout → SIGKILL

---

## 8. 2 个 System Tool

### 8.1 `search_mcp`（`internal/app/tool/mcp/search.go`）

```go
type SearchMCP struct{ svc *mcpapp.Service }

func (t *SearchMCP) Name() string { return "search_mcp" }

func (t *SearchMCP) Description() string {
    return "Search across all connected MCP servers for tools matching a query. " +
           "Returns top 5 candidate tools with their schemas. " +
           "Use when you need an external integration (GitHub, Slack, Postgres, etc.) " +
           "and want to discover what's available before calling."
}

func (t *SearchMCP) Parameters() json.RawMessage {
    return json.RawMessage(`{
      "type":"object",
      "properties":{
        "query":{"type":"string","description":"Natural language description of what tool you need"}
      },
      "required":["query"]
    }`)
}

// 9 方法...
```

`Execute` 主体调 `svc.Search` 返结果 JSON。

### 8.2 `call_mcp`（`internal/app/tool/mcp/call.go`）

```go
func (t *CallMCP) Description() string {
    return "Invoke a specific tool on a specific MCP server. " +
           "Find candidates first via search_mcp. " +
           "args must conform to the tool's inputSchema (returned by search_mcp)."
}

func (t *CallMCP) Parameters() json.RawMessage {
    return json.RawMessage(`{
      "type":"object",
      "properties":{
        "server":{"type":"string","description":"Server name (e.g. 'github')"},
        "tool":{"type":"string","description":"Tool name (no mcp__ prefix)"},
        "args":{"type":"object","description":"Tool args matching the inputSchema"}
      },
      "required":["server","tool","args"]
    }`)
}
```

`Execute` 调 `svc.CallTool` 返字符串结果。

**为什么不在 search 直接 call**：分两步 LLM 显式确认调用意图，**也便于 catalog 路由提示影响"调哪个"决策**——先看候选再选择，比"一键调用"更可控。

---

## 9. SSE 事件

```go
// internal/domain/events/types.go
type MCP struct {
    Servers []mcpdomain.ServerStatus `json:"servers"`
}

func (MCP) EventName() string { return "mcp" }
```

**触发点**：`Service.Connect` / `Disconnect` / `Enable` / `Disable` / 子进程退出 monitor 检测到 disconnect 时，发布**全 server 状态快照**（不是单 server 增量——前端拿快照重渲染最简单）。

**过滤 key**：无（全局事件，前端订阅 user-level）。

---

## 10. HTTP API

### Server 配置 / 生命周期

| Method + Path | 用途 | 响应 |
|---|---|---|
| `GET /api/v1/mcp-servers` | 列所有配置（含 status + tools + health 字段）| `{data: [ServerStatus...]}` |
| `GET /api/v1/mcp-servers/{name}` | 单 server 详情 + tools | `{data: ServerStatus}` |
| `PUT /api/v1/mcp-servers/{name}` | 增/改配置（写 mcp.json + Connect）| `{data: ServerStatus}` (200) |
| `DELETE /api/v1/mcp-servers/{name}` | 删配置 + disconnect | 204 |
| `POST /api/v1/mcp-servers:import` | **拖拽导入**（multipart 上传 mcp.json 文件 / 文本 fragment）| `{data: {imported: [...], conflicts: [...]}}` |
| `POST /api/v1/mcp-servers/{name}:reconnect` | 强制重启子进程（degraded / failed 恢复用）| `{data: ServerStatus}` |
| `POST /api/v1/mcp-servers/{name}:health-check` | 主动健康检查（调 tools/list 验证）| `{data: HealthResult}` |

### Registry（marketplace 体验）

| Method + Path | 用途 | 响应 |
|---|---|---|
| `GET /api/v1/mcp-registry` | 列所有可装 server entries（含元数据）| `{data: [RegistryEntry...]}` |
| `GET /api/v1/mcp-registry/{name}` | 单 entry 详情 | `{data: RegistryEntry}` |
| `POST /api/v1/mcp-registry/{name}:install` | 安装：填 env + args → 写 mcp.json + Connect | `{data: ServerStatus}` (201) |

`:install` 端点 body：
```json
{
  "env":  { "GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_..." },
  "args": { "rootPath": "/Users/me/projects" }
}
```

**没有 `:enable` / `:disable` 端点**——配置在 mcp.json 即启用，删除即禁用，无中间态。

### 拖拽导入端点详细

**`POST /api/v1/mcp-servers:import`**

接收两种 input：
1. **multipart file**：上传 `mcp.json` 文件
2. **JSON body**：`{"mcpServers": {...}}` fragment

行为：
- 解析后检查每个 server 是否已存在
- 不存在 → 加入 `~/.forgify/mcp.json` + 立即 Connect
- 已存在 → 不覆盖，加入响应 `conflicts` 列表，由前端弹确认
- 强制覆盖：query param `?overwrite=true`

响应：
```json
{ "data": {
  "imported": ["github", "postgres"],
  "conflicts": ["slack"]   // 已存在未导入
}}
```

**用户拖一个完整 Claude Desktop 配置进来**，一次性把所有 server 装进 Forgify——之后两边互不影响。

---

## 11. 错误码

| Sentinel | HTTP | Wire Code |
|---|---|---|
| `mcpdomain.ErrServerNotFound` | 404 | `MCP_SERVER_NOT_FOUND` |
| `mcpdomain.ErrServerNotConnected` | 409 | `MCP_SERVER_NOT_CONNECTED` |
| `mcpdomain.ErrToolNotFound` | 404 | `MCP_TOOL_NOT_FOUND` |
| `mcpdomain.ErrToolCallFailed` | 502 | `MCP_TOOL_CALL_FAILED` |
| `mcpdomain.ErrToolCallTimeout` | 504 | `MCP_TOOL_CALL_TIMEOUT` |
| `mcpdomain.ErrRegistryEntryNotFound` | 404 | `MCP_REGISTRY_ENTRY_NOT_FOUND` |
| `mcpdomain.ErrRuntimeMissing` | 422 | `MCP_RUNTIME_MISSING` |
| `mcpdomain.ErrRequiredEnvMissing` | 422 | `MCP_REQUIRED_ENV_MISSING` |
| `mcpdomain.ErrRequiredArgsMissing` | 422 | `MCP_REQUIRED_ARGS_MISSING` |
| `mcpdomain.ErrInstallFailed` | 502 | `MCP_INSTALL_FAILED` |

`ErrToolCallFailed` / `ErrInstallFailed` 用 502（外部 server / 包管理器错），消息含原始失败文本。

---

## 12. CatalogSource 实现

```go
// internal/app/mcp/catalogsource.go
type catalogSource struct{ svc *Service }

func (c *catalogSource) Name() string                                    { return "mcp" }
func (c *catalogSource) Granularity() catalogdomain.Granularity          { return catalogdomain.PerServer }
func (c *catalogSource) EventTopics() []string                          { return []string{"mcp"} }

func (c *catalogSource) ListItems(ctx context.Context) ([]catalogdomain.Item, error) {
    items := []catalogdomain.Item{}
    for _, server := range c.svc.ListServers(ctx) {
        if server.Status != "ready" { continue }
        items = append(items, catalogdomain.Item{
            Source:      "mcp",
            ID:          server.Name,
            Name:        server.Name,
            Description: fmt.Sprintf("%s (%d tools): %s", server.Name, len(server.Tools), summarizeTools(server.Tools)),
        })
    }
    return items, nil
}

func (s *Service) AsCatalogSource() catalogdomain.CatalogSource {
    return &catalogSource{svc: s}
}
```

`Granularity = PerServer` 让 catalog generator **不合并 server**——每个 MCP server 是独立外部系统，应单独提及。`summarizeTools` 拼几个代表 tool 名称做 hint（"github (21 tools): create_pr, list_issues, ...")。

---

## 13. 测试覆盖（计划）

| 层 | 文件 | 测试数 | 覆盖 |
|---|---|---|---|
| domain | `internal/domain/mcp/mcp_test.go` | 6 | ServerConfig JSON / RegistryEntry JSON / Sentinel 一致性 / Status 5 值校验 |
| infra/mcp | `internal/infra/mcp/mcp_test.go` | 10 | stdio handshake / tools/list / call / 子进程退出 / stdout 污染检测 / per-call 30s timeout |
| app/mcp | `internal/app/mcp/mcp_test.go` | 18 | Connect / Disconnect / Search ranking / CallTool / 多 server 并发 / 健康累计 / degraded 触发 / 自愈 / Registry install 流程 / runtime 缺失拒绝 |
| app/tool/mcp | `internal/app/tool/mcp/mcp_test.go` | 10 | search/call 9 方法 + happy + error 分支 |
| pipeline | `test/mcp/mcp_test.go` | 5 | fake stdio server：tools/list + search + call 闭环 / 子进程崩溃 fail 状态 / 连续失败触发 degraded / 自愈回 ready / Registry 装 `everything` server 端到端（要求 CI 有 node，否则 skip）|

总计 ~50 测 + 5 pipeline 场景。

**fake MCP server**（pipeline 默认用）：用 Go 写一个最小 stdio MCP server（~100 行），暴露 2-3 个 echo tool；放 `test/mcp/fakeserver/`。**离线可跑**。

**真 server 测试**（require gate）：`TestPipeline_MCP_RegistryInstallEverything` 装公开的 `@modelcontextprotocol/server-everything`，验证 Registry 端到端；要求 CI 装 node，否则 `t.Skip`（per §T6）。

---

## 14. 与其他 domain 的关系

| 关系 | 说明 |
|---|---|
| **chat** | 主对话 LLM 通过 search_mcp / call_mcp 间接使用；mcpapp 不直接 import chat |
| **catalog** | mcpapp 实现 CatalogSource，catalog 通过接口拉数据 |
| **events** | 复用 events bridge，发 mcp 全局快照事件 |
| **logger** | 子进程 stderr → zap.L().Named("mcp.<server>")，便于过滤排查 |
| **llmclient** | Search ranking 调 llm（用 chat 场景模型，简化）|

---

## 15. 演化方向

- **Streamable HTTP transport**（v2）：远程 MCP server，OAuth 2.1 + PKCE。前提是用户场景出现"多机共享 MCP server"或"接 SaaS MCP"
- **Tool 单独 enable/disable**：当前粒度是 server 级；未来允许"启用 github server 但禁用其 delete_repo tool"
- **调用日志持久化**：当前调 LLM-search 排序但不存历史；可加 `mcp_call_history` 表用于 audit
- **远程注册表自动发现**：扫 `registry.modelcontextprotocol.io` 帮用户装 server（当前手编 mcp.json）
- **Resources / Prompts 支持**：v1 只用 Tools；spec 还有 Resources（数据源）和 Prompts（模板）primitive，几乎没人用，按需再加
- **Sampling 协议**：spec 允许 server 主动调 LLM；v1 不支持，几乎没 server 用

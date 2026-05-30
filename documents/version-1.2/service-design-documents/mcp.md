# MCP — V1.2 详设计

**Phase**：Phase 4 准备件（提前到位）

> **🔧 限制优化（2026-05-31，limits-optimization）**：`defaultCallTimeout` 30→180s（agent-recovery 高默认；MCP 工具可能自调 LLM / 爬虫，超时把控制权还给 agent 而非挂死）。详 [`../adhoc-topic-documents/limits-optimization/`](../adhoc-topic-documents/limits-optimization/)。
**状态**：✅ Marketplace V3 — curated（2026-05-08 curated 化 / 2026-05-09 search→list 化）：domain types + 14 sentinels（10 mcp.go 核心 + 4 registry.go marketplace）+ **21 条 hand-picked RegistrySource**（npm + pypi only）+ ~/.forgify/mcp.json I/O + stdio Client wrapper（go-sdk v1.6）+ Service lifecycle/Search/CallTool/Health/Install/Import/Stderr + 5 system tools (search_mcp_tools / call_mcp_tool / list_mcp_marketplace / install_mcp_server / uninstall_mcp_server) + 11 HTTP endpoints + 4 离线 pipeline 场景 + 1 Live_ 装 everything 场景门控 + `mcp_server` per-name notification（不发全量快照）
**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../service-contract-documents/database-design.md`](../service-contract-documents/database-design.md) — 无新表（mcp.json 是 source）
- [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md) — mcp ×14（已接 errmap）
- [`../service-contract-documents/events-design.md`](../service-contract-documents/events-design.md) — `mcp_server` per-name entity-state 通知（不再发全量快照）
- 关联设计：[`subagent.md`](./subagent.md) / [`skill.md`](./skill.md) / [`catalog.md`](./catalog.md) / [`sandbox.md`](./sandbox.md)
- 外部 spec：[MCP 2025-11-25](https://modelcontextprotocol.io/specification/2025-11-25)
- 依赖库：[`modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk) v1.x（**官方** SDK，不用 mark3labs/mcp-go——v1 stability + Anthropic+Google 共维）

---

## 1. 一句话

把外部 MCP server 当成"动态 tool 来源"接进来——但**不 flat 注册**到 LLM 的 tool registry，而是暴露 **5 个 system tool**（`search_mcp_tools` / `call_mcp_tool` / `list_mcp_marketplace` / `install_mcp_server` / `uninstall_mcp_server`），让 LLM 按需 search + call + 装/卸，避免 70+ server × 5 tool × 200 token = 70k 启动开销。

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
  → 注册 5 个 system tool：search_mcp_tools / call_mcp_tool / list_mcp_marketplace / install_mcp_server / uninstall_mcp_server
```

### 运行期 — search

```
LLM → tool_use{name="search_mcp_tools", args={query:"github pr"}}
  → mcptool.SearchMCPTools.Execute(ctx, args)
    → mcpapp.Service.Search(ctx, query, topK)
        → 拉所有 connected server 的所有 ToolDef
        → 拼一段 prompt 让一个 ranking LLM 选 top 5（同 forge search 模式 A）
        → 返回 [{server, name, description, schema}, ...]
    → 序列化成 LLM 可读 JSON 字符串
  → tool_result 给 LLM：top 5 候选
  → LLM 决定调哪个，下一 turn 调 call_mcp_tool
```

### 运行期 — call

```
LLM → tool_use{name="call_mcp_tool", args={server:"github", tool:"create_pr", args:{...}}}
  → mcptool.CallMCPTool.Execute(ctx, args)
    → mcpapp.Service.CallTool(ctx, server, tool, args)
        → 取 server 的 client，调 tools/call RPC（JSON-RPC over stdio）
        → server 子进程返 content blocks（text/image array）
        → 序列化成 string（image 转 base64 inline 或 ref）
    → 返回给 LLM 当 tool_result
  → LLM 继续
```

### 运行期 — marketplace 装 / 卸

```
LLM → tool_use{name="list_mcp_marketplace", args={}}
  → 直接 passthrough svc.ListRegistry() → 返 21 条 RegistryEntry JSON

LLM → tool_use{name="install_mcp_server", args={name:"github"}}
  → 阶段 1：返 phase1Envelope (needsConfirmation + requiredEnv + Notes)
  → 用户填好 env / args 后 LLM 再调 install_mcp_server({name, env, args, confirmed:true})
  → 阶段 2：mcpapp.InstallFromRegistry → 写 mcp.json + sandbox.EnsureEnv + Connect

LLM → tool_use{name="uninstall_mcp_server", args={name:"github"}}
  → mcpapp.RemoveServer → Disconnect + 从 mcp.json 删
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
| **Search 模式不 flat** | 每个 server 不暴露 N 个 tool 给 LLM；只 5 个 system tool（search_mcp_tools / call_mcp_tool / list_mcp_marketplace / install_mcp_server / uninstall_mcp_server），按需召回 |
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

### Sentinel 错误（14 个：10 核心 + 4 marketplace V3）

`internal/domain/mcp/mcp.go`：

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

`internal/domain/mcp/registry.go`（marketplace V3 加，2026-05-08）：

```go
var (
    ErrAlreadyInstalled   = errors.New("mcp: server already installed")
    ErrUnsupportedRuntime = errors.New("mcp: unsupported runtime")
)
```

> 历史 `ErrMarketplaceUnavailable` / `ErrHandshakeFailed` 已删——curated registry source 同步从内存返，永不失败；handshake 失败由 `ErrServerNotConnected` 覆盖。

`ErrToolCallFailed` / `ErrInstallFailed` 用 `%w` wrap 上游错误（per §S16），保留 server / 包管理器自报的失败信息。

`ErrAlreadyInstalled` 由 `Service.InstallFromRegistry` 收口——同名 server 已在 mcp.json 时返该 sentinel，避免重复装机。

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
- search_mcp_tools 模式不 flat 注册，token 成本可控，不需要"装而不启"
- 单用户本地场景，用户级一份足够；项目级会引入 merge / override 复杂度而无明显收益

### 自包含原则（重要）

**Forgify 永远不读外部 app 目录**（`~/Library/Application Support/Claude/` / `~/.config/Cursor/` 等）。所有 MCP 配置 source of truth 在 **`~/.forgify/mcp.json`** 一处。

**用户从 Claude Desktop 迁移配置**有两条路径，**全是显式动作**（不是后台映射）：

1. **拖拽导入**（推荐）：UI / testend 拖拽 zone 接收 mcp.json 文件 → 后端 `POST /api/v1/mcp-servers:import` 解析 + merge 到 `~/.forgify/mcp.json` → Forgify 拥有副本
2. **手动复制**：`cp ~/Library/Application\ Support/Claude/claude_desktop_config.json ~/.forgify/mcp.json`

**导入后 Forgify 完全拥有副本**：之后改 Claude Desktop 配置不影响 Forgify；卸载 Claude Desktop 不影响 Forgify；备份 Forgify 只需要 `~/.forgify/`。这才是 desktop app 该有的边界。

---

## 5.5. Curated Marketplace — V3

### 概念

Forgify 内置一份 **21 条 hand-picked 的 MCP marketplace**，由 `internal/infra/mcp/curated_registry.go::CuratedRegistrySource` 提供，覆盖**生产力域**（DB / VCS / 错误监控 / 项目管理 / 文档 / 设计 / 邮件 / 浏览器 / 沙箱 / 网搜 / 内存）。

**为什么 hardcoded curated**：上游 `registry.modelcontextprotocol.io` 5000+ 条目，质量参差——大量 broken / abandoned / 需企业 OAuth；端到端用户不需要"全 5000 条搜得到"，需要"我能立刻装、立刻 work 的 21 条"。

**为什么单 Name 字段**：每条目 `Name` 是我们写的短 kebab-case slug（`playwright` / `notion` / `ms365`）——同时是 LLM `install_mcp_server` 的 lookup id 和 `mcp.json` key，**没有独立 alias 概念**。真正的 npm/pypi 包名只放 `InstallCmd.Args`。

**为什么 npm + pypi only**：curated 列表全部用 `npx -y <pkg>` 或 `uvx <pkg>` 起 stdio server——sandbox 端只需保留 `python` + `node` runtime 与 `uv` 工具（其他 7 个 EnvManager 已删，详 [`sandbox.md`](./sandbox.md)）。

### Tier — 上手摩擦分级

| Tier | 含义 | 列表中数量 | 安装 UX |
|---|---|---|---|
| **0** | 零配置，npx/uvx 起就用 | 5 (playwright, chrome-devtools, duckduckgo, context7, memory) | 直接 install |
| **1** | 一个 API key（free / easy signup） | 11 (tavily, firecrawl, github, gitlab, sentry, linear, atlassian, notion, slack, figma, e2b) | 填 token 后 install；entry 带 `SetupURL` |
| **2** | OAuth 流（subprocess 印登录 URL 到 stderr） | 2 (google-workspace, ms365) | install 后 testend **自动开 stderr modal + 短轮询**抓登录 URL；google-workspace 还要先去 Google Cloud Console 建 OAuth client（Google verification 政策禁止 ship 共享凭证） |
| **3** | DB / 云 credential（DSN / connection string） | 3 (dbhub, mongodb, supabase) | 填 DSN/PAT 后 install |

### Registry 数据结构（`internal/domain/mcp/registry.go`）

```go
type RegistryEntry struct {
    Name         string             `json:"name"`         // short kebab-case slug; 同时是 mcp.json key
    Description  string             `json:"description"`
    Homepage     string             `json:"homepage,omitempty"`
    Runtime      string             `json:"runtime"`      // "node" / "python"（仅这俩）
    Version      string             `json:"version,omitempty"`
    InstallCmd   InstallCmd         `json:"installCmd"`
    RequiredEnv  []EnvRequirement   `json:"requiredEnv,omitempty"`
    RequiredArgs []ArgRequirement   `json:"requiredArgs,omitempty"`
    Category     string             `json:"category,omitempty"`  // browser / web-data / code / vcs / error / db / pm / docs / design / memory / sandbox / email
    Tier         int                `json:"tier"`                // 0 / 1 / 2 / 3
    Notes        string             `json:"notes,omitempty"`     // 首跑提示（如 "first run downloads Chromium ~150MB"）
}
```

**砍掉的字段（V2 → V3）**：`DisplayName`（Name 自身就是 short slug）/ `License` / `Bundled` / `Hidden` / `OnlineOnly` / `UnsupportedPlatforms` / `PostInstallSteps` / `DefaultTimeoutSec`——这些要么由 `Notes` 一句覆盖、要么由全局默认覆盖、要么 curated 列表全平台 + 全 npm/pypi 不需要。

### 21 条 curated 列表

| Tier | Name | Category | Runtime | 用途一句 |
|---|---|---|---|---|
| 0 | `playwright` | browser | node | AI 操作浏览器（导航/点击/截图） |
| 0 | `chrome-devtools` | browser | node | Chrome DevTools 协议 — DOM/网络/性能 |
| 0 | `duckduckgo` | web-data | node | 免费 web 搜索 — 无需 API key |
| 0 | `context7` | docs | node | 库文档实时检索（持续联网） |
| 0 | `memory` | memory | node | 知识图谱 — 跨对话记忆 |
| 1 | `tavily` | web-data | node | LLM 级 web 搜索 — 一个免费 key |
| 1 | `firecrawl` | web-data | node | 站点抓取 / 转 markdown |
| 1 | `github` | vcs | node | GitHub repo / issue / PR / Action |
| 1 | `gitlab` | vcs | node | GitLab repo / MR / pipeline |
| 1 | `sentry` | error | python | Sentry 错误调查 |
| 1 | `linear` | pm | node | Linear issues |
| 1 | `atlassian` | pm | node | Jira + Confluence |
| 1 | `notion` | docs | node | Notion pages / databases |
| 1 | `slack` | docs | node | Slack messaging / search |
| 1 | `figma` | design | python | Figma 文件 / frame / asset |
| 1 | `e2b` | sandbox | python | E2B 远程沙箱执行 |
| 2 | `google-workspace` | email | python | Gmail + Drive + Calendar + Docs + Sheets + Slides + Forms + Tasks + Contacts + Chat 全套（**OAuth** — 用户须先 Cloud Console 建 client + 设两个 env） |
| 2 | `ms365` | email | node | Microsoft 365（Outlook / Teams / OneDrive — 同 OAuth 设备码） |
| 3 | `dbhub` | db | node | PostgreSQL / MySQL / SQLite — 一个 DSN 通吃 |
| 3 | `mongodb` | db | node | MongoDB |
| 3 | `supabase` | db | node | Supabase（PAT + project ref） |

实际 entries 见 `backend/internal/infra/mcp/curated_registry.go::curatedEntries`（21 个 RegistryEntry literal，包含 npm/pypi 包名、required env 描述、Notes）。

### List 接口（V3 / 2026-05-09：全量直返）

```go
type RegistrySource interface {
    List(ctx context.Context) ([]RegistryEntry, error)            // 全量，按 tier asc + name asc 稳排
    Get(ctx context.Context, name string) (*RegistryEntry, error) // 不存在返 ErrRegistryEntryNotFound
}
```

**为什么 V2 search → V3 list**：实测 V2 的 4-token AND-match（`playwright` / `browser` / `github` / `slack`）在 21 条里只命中 1 条；curated 已 hand-picked 一遍，再做关键词过滤等于二次筛选高质量结果——召回掉的"明显匹配项"反让 LLM 误以为"没有"。21 条全列入 LLM context ~15-20KB token 完全 OK，比"LLM rerank 4-token search"性价比高。

`/api/v1/mcp-registry` HTTP 端点直接 passthrough List；testend / LLM `list_mcp_marketplace` tool 都走它。Tier 2/3 entries 由 Notes 字段警告 LLM 用前必须 ask 确认（OAuth flow / DB credential）。

### Install 流程（两阶段，LLM 触发）

LLM tool `install_mcp_server` 走两阶段：

```
阶段 1（needs_confirmation）：
  LLM 调 install_mcp_server({name: "github"})
    → 后端读 RegistryEntry → 返 phase1Envelope:
       {needsConfirmation: true, name, runtime, tier, requiredEnv, requiredArgs, notes, summary}
    → LLM 把这个 envelope 渲染给用户："要装 github MCP，需要 GITHUB_PERSONAL_ACCESS_TOKEN（链接 …），确认装吗？"
阶段 2（confirmed）：
  用户填好 env / args 后 LLM 再调 install_mcp_server({name: "github", env: {...}, args: {...}, confirmed: true})
    → mcpapp.InstallFromRegistry(ctx, name, env, args)
       → 校验 RequiredEnv / RequiredArgs 完整
       → 写 ~/.forgify/mcp.json (owner.Name = entry.Name)
       → sandboxapp.EnsureEnv(Owner{Kind:"mcp", ID:name}, EnvSpec{Runtime:{Kind:entry.Runtime}, Deps:[entry.InstallCmd.Args[1]]})
       → mcpapp.Connect(name) → tools/list → ready
    ← 返 ServerStatus
```

testend `/mcp-registry/{name}:install` 直接走阶段 2（UI 自己采集 env/args 后 POST，不需要 needs_confirmation 来回）。

### Tier 2 OAuth UX

google-workspace / ms365 子进程首跑把 OAuth 登录 URL 印到 stderr（ms365 是真设备码；google-workspace 是 OAuth callback URL，因为 Google 政策不让 ship 共享 client）。

- testend tab-mcp.js `install()`：检测到 `tier === 2` 时，install 成功后**自动开 stderr modal** + 短轮询 6×1s（每秒重读 stderr，直到出现 `https://`）——用户立刻看到登录链接。
- LLM 走 `install_mcp_server` 时，phase1 envelope 自带 entry.Notes（如 "first run prints device-code URL to stderr"），LLM 渲染时会提醒用户安装后看 stderr。

### Sandbox 委托

所有 MCP server 的 runtime 装机 / 包装 / spawn **委托给 sandboxapp**（详 [`sandbox.md`](./sandbox.md)）：
- 仅注册 `python` + `node` runtime + `uv` 工具
- 仅注册 `python` + `node` EnvManager（其他 7 个 EnvManager 已删）
- 每个 server 一个独立 env（owner=mcp/`<name>`）

MCP layer 不实现任何装机逻辑——只 catch sandbox 返的 `ErrRuntimeMissing` 等 sentinel 转友好消息。

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
// 每次 CallTool 完成后；仅更新内存状态，不发通知
// （通知已在 AddServer / RemoveServer / connectOne 等"显式生命周期事件"上 publish；
//  per-call 成功 / 失败累计若也推 notification 会刷屏）
func (s *Service) recordCallResult(name string, err error) {
    state := s.states[name]
    state.TotalCalls++
    if err != nil {
        state.TotalFailures++
        state.ConsecutiveFailures++
        state.LastError = err.Error()
        state.LastErrorAt = ptr(time.Now())
        if state.ConsecutiveFailures >= degradedThreshold && state.Status == "ready" {
            state.Status = "degraded"
            // 内部状态翻 degraded，不主动 publish——下次 ListServers / Health 端点拉时即时看到
        }
    } else {
        state.ConsecutiveFailures = 0
        state.LastSuccessAt = ptr(time.Now())
        if state.Status == "degraded" {
            state.Status = "ready"  // 自愈
        }
    }
}
```

**自愈**：degraded 状态下任何一次 tools/call 成功 → 自动回 ready。

**通知边界**（与 §9 SSE 一致）：仅 `AddServer` / `RemoveServer` / `connectOne` / `Reconnect` 等显式生命周期事件 publish `mcp_server` 通知；per-call 失败累计触发的 ready→degraded 转换**不**主动推。前端如果需要看 degraded 自动转换，靠定期 poll `GET /mcp-servers` 或 `:health-check`。

### 主动健康检查方法

```go
// app/mcp/calltool.go
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

**实现**：调 `tools/list` 当探针，10s 超时；记录 RTT。**不**改任何 ServerStatus 字段（避免 UI 测试触发 degraded 误判）。每次调用后自动写一条 `HealthSnapshot` 行（best-effort，失败只 log-warn）。

### §5.7 健康历史（HealthSnapshot）

每次 `HealthCheck` 调用（手动或定期）会追加一条快照，供前端绘制健康折线图。

```go
// domain/mcp/health_history.go
type HealthSnapshot struct {
    ID         string    `json:"id"`          // mch_<16hex>
    UserID     string    `json:"-"`
    ServerName string    `json:"serverName"`
    Healthy    bool      `json:"healthy"`
    LatencyMs  int       `json:"latencyMs"`
    ToolCount  int       `json:"toolCount"`
    ErrorMsg   string    `json:"errorMsg,omitempty"`
    CheckedAt  time.Time `json:"checkedAt"`
}

type HealthHistoryRepository interface {
    Insert(ctx context.Context, snap *HealthSnapshot) error
    ListSince(ctx context.Context, userID, serverName string, since time.Time) ([]*HealthSnapshot, error)
}
```

查询接口：`Service.ListHealthHistory(ctx, name, since)` — `healthRepo` 为 nil 时返空列表（V1 优雅降级）。

**表**：`mcp_health_history`（见 `service-contract-documents/database-design.md`）— 无软删，按 `(user_id, server_name, checked_at)` 索引；id 前缀 `mch_`（§S15）。

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
    case <-time.After(c.callTimeout):  // 默认 30s，可被 mcp.json 里的 ServerConfig.TimeoutSec override
        c.sendNotification("notifications/cancelled", map[string]any{"requestId": reqID})
        return "", mcpdomain.ErrToolCallTimeout
    }
}
```

**Per-server timeout 解析顺序**（高优先级覆盖低）：
1. **mcp.json 里 server 配置**显式 `"timeoutSec": N`（用户最终控制权）
2. **全局默认 30s**（兜底，`defaultCallTimeout` 常量）

> 注：早期设计想用 `RegistryEntry.DefaultTimeoutSec` 让 marketplace 入口自带 default，但 V3 砍掉了——curated 21 条都跑得快，没必要为单 entry 调 timeout。RegistryEntry 不含 timeout 字段。

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

**设计**：MCP tool 永远走 `mcp__<server>__<name>` 命名空间，**不进全局 tool registry 平铺**——LLM 通过 `call_mcp_tool(server, tool, args)` 间接调用，dispatch 在 mcpapp 内部按 server 路由。这就**结构性免疫**所有命名碰撞：
- `mcp__github__search` 和 `mcp__brave__search` 各自独立路由
- `mcp__filesystem__read` 不会撞内置 `Read`（命名空间不同）
- 内部 catalog source 上报给 catalog 时也带前缀，避免 routing 提示混淆

### mcp.json 损坏处理

**问题**：用户手编时漏个逗号 → JSON parse fail → Service.Start panic？

**设计**：`Service.Start` 加载 mcp.json 失败 → log Error + 当作空 mcp.json 启动（无 server）+ UI 显示提示"mcp.json 损坏，请检查"。**不 panic，不静默重写**——给用户机会自己修。

```go
type Service struct {
    configs       map[string]mcpdomain.ServerConfig   // name → config（来自 ~/.forgify/mcp.json）
    states        map[string]*mcpdomain.ServerStatus  // name → live state
    clients       map[string]*mcpClient               // name → live client（含 subprocess handle）
    notif         notificationspkg.Publisher          // 单 server 状态变化推 `mcp_server` 通知（不发全量快照）
    log           *zap.Logger
    modelPicker   modelpickerport.Picker              // search ranking — model 选择
    keyProvider   apikeydomain.KeyProvider            //   ─ apikey 提供
    llmFactory    llmclientpkg.Factory                //   ─ build client
    sandboxPort   sandboxdomain.PluginSandbox         // V3：装 server 时调 EnsureEnv（详 §5.5）
    registrySrc   mcpdomain.RegistrySource            // 21 条 curated marketplace（详 §5.5）
    clientFactory func(...) Client                    // 测试注入点（SetClientFactory）
    mu            sync.RWMutex
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
func (s *Service) Stderr(name string) (string, error)             // 拉某 server 的 stderr ring buffer（256 KB）
func (s *Service) Import(ctx context.Context, incoming []ServerConfig, overwrite bool) (*MergeResult, error)  // 拖拽导入

// 内置 Registry（详 §5.5）
func (s *Service) ListRegistry() []RegistryEntry
func (s *Service) GetRegistryEntry(name string) (*RegistryEntry, error)
func (s *Service) InstallFromRegistry(ctx context.Context, name string, env map[string]string, args map[string]string) (*ServerStatus, error)

// 健康检查（详 §5.6）
func (s *Service) HealthCheck(ctx context.Context, name string) (*HealthResult, error)

// 测试 / web 路由
func (s *Service) SetClientFactory(f func(...) Client)            // 测试注入 stdio Client（fake 走它）
```

**关键时间常量**（`internal/app/mcp/mcp.go`）：
- `defaultCallTimeout = 30 * time.Second`（tools/call 默认上限）
- `addServerTimeout   = 3 * time.Minute`（Connect + initialize + tools/list 整轮）
- `initializeTimeout  = 30 * time.Second`（initialize handshake 单步）
- `degradedThreshold  = 3`（连续失败次数阈值，触发 ready → degraded）

**SearchRouter port**（给 `app/tool/web/WebSearch` 用）：
mcpapp 暴露 `SearchRouter` 接口（`internal/app/mcp/searchrouter.go`），`WebSearch` 工具按需把搜索请求路由到已装的 duckduckgo / tavily MCP server——避免 `web` 包反向依赖 mcp 具体实现。

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

## 8. 5 个 System Tool

`internal/app/tool/mcp/` 子包，§S12 例外位置（按 tool 家族嵌套）；调用方按 §S13 别名 `mcptool`。

### 8.1 `search_mcp_tools`（`search.go`）

跨所有 connected server 搜索匹配 query 的 tool；返 top 5 候选（含 server / name / description / inputSchema）。`Execute` 主体调 `svc.Search(ctx, query, 5)` 返 JSON。

```go
func (t *SearchMCPTools) Description() string {
    return "Search across all connected MCP servers for tools matching a query. " +
           "Returns top candidate tools with their schemas. " +
           "Use when you need an external integration (GitHub, Slack, Postgres, etc.) " +
           "and want to discover what's available before calling."
}
```

### 8.2 `call_mcp_tool`（`call.go`）

调用特定 server 的特定 tool；args 必须符合该 tool 的 inputSchema（由 `search_mcp_tools` 返）。`Execute` 调 `svc.CallTool` 返字符串结果。

```go
func (t *CallMCPTool) Description() string {
    return "Invoke a specific tool on a specific MCP server. " +
           "Find candidates first via search_mcp_tools. " +
           "args must conform to the tool's inputSchema (returned by search_mcp_tools)."
}
```

### 8.3 `list_mcp_marketplace`（`list_marketplace.go`）

返回 21 条 curated RegistryEntry JSON（含 tier / category / requiredEnv / Notes）；LLM 看后能跟用户讨论"装哪个"。`Execute` 直接 passthrough `svc.ListRegistry()` 返序列化结果。Tier 2/3 entry 的 `Notes` 字段警告 LLM 安装前需 ask 用户确认。

### 8.4 `install_mcp_server`（`install_server.go`）

两阶段流程（详 §5.5 阶段 1/2）：

- **阶段 1**（`{name}`）：返 `phase1Envelope` `{needsConfirmation:true, requiredEnv, requiredArgs, notes, ...}`，LLM 渲染给用户征求 env / args
- **阶段 2**（`{name, env, args, confirmed:true}`）：调 `mcpapp.InstallFromRegistry` → 写 mcp.json + sandbox.EnsureEnv + Connect → 返 ServerStatus

集成 `pkg/installprogress` 的 Run helper：sandbox install 进度通过 ctx eventlog Emitter 推 `progress` block 到该 tool_call 父下，前端实时看到 "Installing @playwright/mcp..." / "Downloading Chromium ~150MB..." 等。

### 8.5 `uninstall_mcp_server`（`uninstall_server.go`）

调用 `mcpapp.RemoveServer` → Disconnect 子进程 + 从 mcp.json 删 + sandbox env 保留（`docker image prune` 类资源由用户主动清）。

**为什么 search → call 分两步**：让 LLM 显式确认调用意图，**也便于 catalog 路由提示影响"调哪个"决策**——先看候选再选择，比"一键调用"更可控。

### 8.6 LLM-facing return shapes（Phase C 清理后约定）

5 个 tool 返不同 shape 但每个 shape 在自己的语义内自洽——LLM 按 tool 选择解 JSON 或读 text：

| Tool | Success | 错误（friendly path）|
|---|---|---|
| `search_mcp_tools` | JSON array of `ToolDef`（indented）| plain string: `Search failed: <reason>` / `No MCP tools found. ...` |
| `call_mcp_tool` | server passthrough string | plain string: `MCP server X is not connected.` / `MCP tool X does not exist on server Y. ...` |
| `list_mcp_marketplace` | JSON array of slim registry entries | 仅走 Go err → framework boundary 清洗后到 LLM |
| `install_mcp_server` | JSON envelope `{status:"installed", name, server}` / `{status:"needs_confirmation", suggested_question, required_env, required_args, notes, tier}` | JSON envelope `{status:"error", error:<code>, message}` 含 code: `not_in_registry` / `already_installed` / `missing_required_args` / `install_failed` |
| `uninstall_mcp_server` | JSON envelope `{status:"uninstalled", name}` | JSON envelope `{status:"error", error:"not_installed", message}` |

**Phase C 清理纪要**：
- 删了 install / uninstall 成功 envelope 里的 "human message"（如 `"Server X installed and connected"`）——envelope 自有 `status` / `name` / `server.Status` 字段，message 重复
- 删了 friendly 字符串里的 UI / 文件路径泄漏（`~/.forgify/mcp.json` / "MCP servers UI" / "click 'Reconnect'"）
- 错误的 `%v err` 改用 `%s err.Error()`——framework boundary（`loop/tools.go`）会清 §S16 wrap 链

---

## 9. Notifications（per-server，不发全量快照）

V3 改用 `notificationspkg.Publisher` 推**单 server** 状态变化，不再发全 server 快照——前端按 server name 局部更新，避免快照覆盖正在打字的别处。

```json
// 通用 envelope（详 events-design.md notifications 协议章）
{
  "type": "mcp_server",
  "id":   "<server-name>",   // 如 "github" / "playwright"；不是 "*"
  "data": {                   // ServerStatus payload
    "name":               "github",
    "status":             "ready",       // disconnected / connecting / ready / degraded / failed
    "pid":                12345,
    "connectedAt":        "2026-05-09T13:42:00Z",
    "consecutiveFailures": 0,
    "totalCalls":         42,
    "totalFailures":      0,
    "tools":              [ /* ToolDef[] */ ]
  }
}
```

**触发点**：
- `AddServer` / `RemoveServer`（写 mcp.json + Connect/Disconnect 后）
- `connectOne` 子进程握手成功 / 失败时
- `Reconnect` 强制重启
- 子进程退出 monitor 检测 disconnect

**不触发点**：per-call 失败累计触发的 ready→degraded（详 §5.6 通知边界）；per-call 成功打回 ready。前端要看 degraded 转换靠定期 poll。

**Wire path**：`/api/v1/notifications` 全局通道 + 客户端按 `type=mcp_server` 过滤；`id` 是 server name 用于增量更新。详 [`../service-contract-documents/events-design.md`](../service-contract-documents/events-design.md) notifications 协议章。

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
| `POST /api/v1/mcp-servers/{name}:health-check` | 主动健康检查（调 tools/list 验证）+ 写 HealthSnapshot | `{data: HealthResult}` |
| `GET /api/v1/mcp-servers/{name}/health-history` | 健康快照历史（`?sinceMinutes=N`，默 1440，max 10080）| `{data: [HealthSnapshot...]}` |
| `GET /api/v1/mcp-servers/{name}/stderr` | 拉某 server 的 stderr ring buffer（256 KB，给 testend / Tier 2 OAuth modal 看 device-code URL）| `{data: {stderr: "..."}}` |
| `POST /api/v1/mcp-servers/{name}/tools/{tool}:invoke` | 直接调用工具（绕 chat/LLM）；详情页"试调用"用 | `{data: {result: "..."}}` |

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

## 11. 错误码（14 个）

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
| `mcpdomain.ErrAlreadyInstalled` | 409 | `MCP_ALREADY_INSTALLED` |
| `mcpdomain.ErrUnsupportedRuntime` | 422 | `MCP_UNSUPPORTED_RUNTIME` |

`ErrToolCallFailed` / `ErrInstallFailed` 用 502（外部 server / 包管理器错），消息含原始失败文本。`ErrAlreadyInstalled` 409 表达"server 名已占用"。

---

## 12. CatalogSource 实现

```go
// internal/app/mcp/catalogsource.go
type catalogSource struct{ svc *Service }

func (c *catalogSource) Name() string                                    { return "mcp" }
func (c *catalogSource) Granularity() catalogdomain.Granularity          { return catalogdomain.PerServer }

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

## 13. 测试覆盖 ✅

| 层 | 文件 | 测试数 | 覆盖 |
|---|---|---|---|
| domain | `internal/domain/mcp/mcp_test.go` + `registry_test.go` | 实测 | ServerConfig / RegistryEntry JSON / IsCallable 5 状态 / 10 sentinel 一致性 |
| infra/mcp | `internal/infra/mcp/{client,config}_test.go` | 实测 | stdio handshake fixture / tools/list / call / 子进程退出 / Load/Save/Merge atomic+0600 |
| app/mcp | `internal/app/mcp/{mcp,registry}_test.go` | 实测 | Connect/Disconnect/Reconnect / CallTool / 健康累计 / degraded 触发 / 自愈 / Registry install / runtime 缺失拒绝 |
| app/tool/mcp | （tool 实测覆盖在 transport handler 集成测试 + pipeline 闭环里） | — | search/call 行为通过 HTTP + pipeline 端到端验证 |
| transport/handlers | `internal/transport/httpapi/handlers/mcp_test.go` | 20 | 11 端点 happy + error 分支 + import multipart/JSON + conflict overwrite + stderr ring buffer |
| pipeline | `test/mcp/mcp_test.go` | 4 + 1 gated | (1) tools/list+search+call 闭环 / (2) BadCommand→failed / (3+4) 连续失败→degraded→自愈 / (5) Live_ 装 everything（双门控：sandbox.IsReady() + `FORGIFY_LIVE_MCP_INSTALL=1`）|

**fake MCP server**：`backend/test/mcp/fakeserver/main.go` ~70 行；3 tool（echo / fail / crash）；TestMain 一次性 build。**离线可跑**。

**Live 装 everything**：`TestMCP_Live_RegistryInstallEverything` 装公开的 `@modelcontextprotocol/server-everything` 验证 Registry 端到端；双门控（per §T6 + 装 npm 包成本控制）。

---

## 14. 与其他 domain 的关系

| 关系 | 说明 |
|---|---|
| **chat** | 主对话 LLM 通过 5 个 system tool 间接使用；mcpapp 不直接 import chat。`pkg/installprogress` 把 sandbox install 进度推到当前 ctx 的 eventlog Emitter（tool_call 父下的 progress block）|
| **catalog** | mcpapp 实现 CatalogSource，catalog 1s 轮询拉数据 |
| **notifications** | 经 `notificationspkg.Publisher` 推 `mcp_server` per-name 通知（详 §9）；不再走 events bridge |
| **sandbox** | `Service.InstallFromRegistry` 调 `sandboxapp.EnsureEnv(owner=mcp/<name>)` 装 runtime + deps；`Service.Connect` 调 `SpawnLongLived` 拿 stdio handle |
| **logger** | 子进程 stderr → zap.L().Named("mcp.<server>")，便于过滤排查；同时存进 256 KB ring buffer 给 `/stderr` 端点 |
| **llmclient** | Search ranking 调 LLM（modelPicker + keyProvider + llmFactory 三件套，每次 attempt 内部 resolve）|

---

## 15. 演化方向

- **Streamable HTTP transport**（v2）：远程 MCP server，OAuth 2.1 + PKCE。前提是用户场景出现"多机共享 MCP server"或"接 SaaS MCP"
- **Tool 单独 enable/disable**：当前粒度是 server 级；未来允许"启用 github server 但禁用其 delete_repo tool"
- **调用日志持久化**：当前调 LLM-search 排序但不存历史；可加 `mcp_call_history` 表用于 audit
- **远程注册表自动发现**：扫 `registry.modelcontextprotocol.io` 帮用户装 server（当前手编 mcp.json）
- **Resources / Prompts 支持**：v1 只用 Tools；spec 还有 Resources（数据源）和 Prompts（模板）primitive，几乎没人用，按需再加
- **Sampling 协议**：spec 允许 server 主动调 LLM；v1 不支持，几乎没 server 用

---

## Relations Integration（2026-05-19）

mcp_server 在 relgraph 中作为节点（含孤儿）；name 是主键，不参与 wikilink。

| 方法 | 触发的 relation 操作 |
|---|---|
| `Service.RemoveServer` | `PurgeEntity("mcp", name)` 级联清边 |

mcp 不直接写出向边；它通过 `workflow_uses_mcp` 入向边被引用。reader 实现 `ListAllMeta` 给 relgraph 拉 label（mcp name + status）。详 [`./relation.md`](./relation.md) §9.3。

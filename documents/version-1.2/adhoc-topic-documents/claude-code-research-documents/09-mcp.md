# 09 — Claude Code MCP 集成

## 信息来源与局限

主要参考：
- https://code.claude.com/docs/en/mcp (官方完整)
- https://github.com/anthropics/claude-code/issues/11364 / #23787 / #21545 (lazy load 历史)
- https://claudefa.st/blog/tools/mcp-extensions/mcp-tool-search
- https://jpcaparas.medium.com/claude-code-finally-gets-lazy-loading-for-mcp-tools-explained-39b613d1d5cc
- https://www.mindstudio.ai/blog/claude-code-mcp-server-token-overhead
- https://modelcontextprotocol.io/introduction (MCP 协议本身)

---

## 1. MCP 工具延迟加载 ⭐ (v2.1.7+)

### 1.1 问题：context 爆炸

✅ MCP 之前的痛：装 5 个 MCP server（github + filesystem + slack + sentry + jira）→ 每个 server 暴露 10-30 tool → **session start 一次性把 100+ tool schema 注入 system prompt**，吃 ~25K tokens——这部分还会污染每次 turn 的输入。

### 1.2 Lazy Loading 实现

✅ v2.1.7 起的 **MCP Tool Search**：

```
session start:
  收到 MCP server 报告的 (toolName, shortDescription) 列表
  ↓
  只把 tool 名 + 描述（约 5-10 字/tool）放 system prompt 的 "tool index"
  ↓
  完整的 tool schema (parameters, longDescription) 不加载
```

调用时：

```
LLM 想用 mcp__github__create_pr
  ↓
内部 tool_search resolver:
  1. 根据 tool 名向对应 MCP server 请求完整 schema
  2. schema 缓存到 in-memory map (sessionId, toolName) -> schema
  3. 注入到当前 turn 的 tool definitions 列表
  4. LLM 重新看到带完整 schema 的 tool，正常调用
```

✅ 触发自动启用条件：
- MCP tool descriptions **总量超过 context 10%** 时自动启用 tool search
- 否则一次性全部加载（少量 server 时不必复杂化）
- `enable_tool_search: false` 强制走 legacy 全量加载

### 1.3 节省效果

✅ 官方数据（来自 mindstudio 文章）：
- 5 个 MCP server / 100+ tool 场景：从 25K tokens 降到 ~1.5K tokens（**95% 减少**）
- 新增 3-5K tokens 当 LLM 第一次调每个 tool（schema 拉取）——但只为实际用到的工具买单

### 1.4 缓存策略

✅ schema 缓存在 in-memory map，**session 级**——session 结束清空。新 session 重新拉。

⚠️ MCP server 用 `list_changed` notification 主动通知 schema 变化时，缓存失效。

---

## 2. MCP Server 配置与发现

### 2.1 多 scope 配置

✅ 5 个 scope，优先级（自上而下）：

| Scope | 路径 | 共享？ |
|---|---|---|
| Enterprise / Managed | `/etc/claude-code/mcp.json` 等 | 是（管理员） |
| User | `~/.claude/mcp.json` | 否（per-user） |
| Project | `<proj>/.mcp.json` | 是（commit） |
| Local | `<proj>/.mcp.local.json` | 否（gitignore） |
| Plugin | `<plugin_root>/.mcp.json` | plugin 启用时 |

✅ 合并：union of servers across scopes；同名 server 高优 scope 赢。

### 2.2 配置文件 schema

```json
{
  "mcpServers": {
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": { "GITHUB_TOKEN": "${GITHUB_TOKEN}" },
      "transport": "stdio"
    },
    "notion": {
      "url": "https://mcp.notion.com/mcp",
      "transport": "http",
      "headers": { "Authorization": "Bearer ${NOTION_TOKEN}" }
    },
    "asana": {
      "url": "https://mcp.asana.com/sse",
      "transport": "sse"
    }
  }
}
```

✅ 三种 transport：
- `stdio`：本地子进程，stdin/stdout JSON-RPC
- `http`：流式 HTTP（推荐，最广泛支持）
- `sse`（Server-Sent Events）：旧、已 deprecated 但仍兼容

### 2.3 启动方式

✅ stdio：Claude Code spawn 子进程（`command + args` + `env`），保持 stdin/stdout 长连接。
✅ http：建立 streamable-http 长连接（HTTP/2 multiplexed）。
✅ sse：HTTP GET 长连，server push events。

### 2.4 自动发现

⚠️ 不是真正"自动发现"——server 都需要手动配置。但 `claude mcp add` 命令简化了这件事：

```bash
claude mcp add notion --transport http https://mcp.notion.com/mcp
claude mcp add github --transport stdio --env GITHUB_TOKEN=xxx -- npx -y @modelcontextprotocol/server-github
```

✅ 写入 scope 由 `--scope` 标志决定（`local` 默认 / `project` / `user`）。

### 2.5 自动重连

✅ HTTP/SSE server 中途断开：exponential backoff，最多 5 次（1s → 2s → 4s → 8s → 16s）。重连期间 server 状态显示为 pending。5 次失败后 mark failed，用户从 `/mcp` 手动 retry。

✅ 启动时初次连接也重试（v2.1.121+）：transient error（5xx / 拒接 / timeout）最多 3 次；auth / 404 不 retry（必须改配置）。

✅ stdio server **不自动重连**（子进程 crash 就 dead）——用户重启 session。

---

## 3. MCPTool → 统一 Tool 接口映射

### 3.1 命名约定

✅ MCP tool 注册到 Claude Code Tool registry 时，名字加前缀：

```
原 MCP server tool: search_repositories  (server: github)
                ↓
在 Claude Code: mcp__github__search_repositories
```

格式：`mcp__<sanitized_server_name>__<tool_name>`。server name 里特殊字符 → `_`。

### 3.2 Schema 转换

✅ MCP 协议的 `tools/list` 返回每个 tool 的：
- `name`
- `description`
- `inputSchema`（标准 JSON Schema）

✅ Claude Code 适配层把这 3 项直接映射到内部 Tool interface 的 `name`/`description`/`inputSchema`。
**额外字段处理**：
- `readOnlyHint`（MCP server 可在 tool annotation 里声明）→ 复制到 Claude Code Tool 的 `readOnlyHint`，参与并行决策
- `destructiveHint` 同样

### 3.3 调用流程

```
LLM 调 mcp__github__create_pr({title:"...", body:"..."})
  ↓
Claude Code Tool dispatch:
  1. 识别前缀 mcp__github__
  2. 找到对应 MCP client 实例
  3. 发送 JSON-RPC: tools/call {name: "create_pr", arguments: {...}}
  4. 等响应
  5. MCP server 返回 content blocks (text/image/...)
  6. Claude Code 把 content 序列化成 string 作为 tool_result
```

### 3.4 Tool result 格式转换

✅ MCP `tools/call` 响应可以是 array of content blocks：

```json
{
  "content": [
    { "type": "text", "text": "PR #42 created" },
    { "type": "image", "data": "base64...", "mimeType": "image/png" }
  ]
}
```

Claude Code 把它转成 tool_result block 的内容（多 content 时拼成多 block 的 tool_result）。

### 3.5 错误处理

✅ MCP server 断开：tool call 报 `MCP server <name> is disconnected`，触发自动重连（§2.5）。重连成功前所有 tool call 失败。
✅ Tool 不存在：返回 `Tool <name> not found in MCP server <server>`。

---

## 4. Transport 实现

### 4.1 stdio

✅ JSON-RPC 2.0 over newline-delimited JSON：

```
client → server (stdin):
{"jsonrpc":"2.0","id":1,"method":"tools/list"}\n

server → client (stdout):
{"jsonrpc":"2.0","id":1,"result":{"tools":[...]}}\n
```

✅ 子进程管理：
- `child_process.spawn` 起进程
- 保持 stdin/stdout 流
- stderr 流到 `~/.claude/logs/mcp-<server>.log`
- 进程退出 → mark server failed
- session end → SIGTERM → 等 5s → SIGKILL

### 4.2 HTTP（streamable-http）

✅ 基于 HTTP/2 multiplex 的长连接：
- 单个 long-running request
- server 用 chunked transfer encoding 推送事件
- 支持双向 streaming（client 也可以 mid-request 推消息）

### 4.3 OAuth 2.0 认证（HTTP server）

✅ 当 HTTP server 要求 OAuth：
1. 首次连接 → 401 + `WWW-Authenticate: Bearer realm="..."`
2. Claude Code 通过 `/mcp` 命令引导用户走 OAuth flow（在浏览器打开 authorization URL）
3. 拿到 access_token → 加到 `Authorization: Bearer <token>` header
4. token 存到 `~/.claude/mcp-tokens/<server>.json`（加密）
5. 后续启动自动用；过期时刷新（refresh_token）

✅ Protected Resource Metadata discovery (RFC 8414): client 在 401 后请求 `/.well-known/oauth-protected-resource` 拿到 OAuth server URL，再请求 `/.well-known/oauth-authorization-server` 拿 endpoints。

⚠️ 完整 OAuth flow 实现在 `src/mcp/oauth.ts` 但 minified 后细节难还原。

---

## 5. 权限与 Hook 集成

### 5.1 MCP tool 权限

✅ MCP tool 完全走标准权限流（详见报告 08），唯一区别是匹配规则：

```json
{
  "permissions": {
    "allow": [
      "mcp__github",                    // github server 全部
      "mcp__github__*",                 // 同义（regex 通配）
      "mcp__github__search_repositories" // 单个 tool
    ],
    "deny": [
      "mcp__filesystem__delete_*"       // filesystem 的所有 delete tool
    ]
  }
}
```

### 5.2 Hook 集成

✅ MCP tool 触发标准 hook（PreToolUse/PostToolUse），matcher 用 `mcp__` 前缀：

```json
{
  "hooks": {
    "PreToolUse": [{
      "matcher": "mcp__github__.*",   // 所有 github MCP tool
      "hooks": [{
        "type": "command",
        "command": "echo 'Calling GitHub MCP tool'"
      }]
    }]
  }
}
```

### 5.3 Trust model

✅ MCP server 配置在 project scope（`.mcp.json` commit 进 git）→ 第一次启动 session 会**弹一次 trust 弹框**（"This project wants to start MCP servers: github, notion. Trust?"）。
✅ 用户级（`~/.claude/mcp.json`）默认信任。

✅ 管理员可用 managed settings：
- `allowManagedMcpServersOnly: true` → 只用 managed 配置的 server（除 deny merge）
- `deniedMcpServers: ["xxx"]` → 黑名单永远 merge

---

## 6. MCP Resources / Prompts

### 6.1 Resources

✅ MCP 协议除了 tool 外还支持 **resources**（数据源）：
- 暴露：`resources/list` 列出 URI
- 读取：`resources/read` 拿内容
- Claude Code 把 resource 暴露成可在 conversation 里 `@mention` 的对象（例：`@notion://page/abc123`）
- mention 到的 resource 内容自动被读、注入到 user message

✅ 与 tool 的区别：
- tool 是动作（"做什么"），resource 是数据（"是什么"）
- resource 是 pull-based（用户/Claude 显式 mention 才读），tool 是 push-based（LLM 主动调）

### 6.2 Prompts

✅ MCP server 可暴露**预设 prompt template**：
- 暴露：`prompts/list`
- 在 Claude Code 里以 slash command 形式出现：`/mcp__notion__summarize_page`
- 用户调用时填参数 → server 返回完整 prompt → 作为 user message 提交

⚠️ Resources 和 Prompts 在公开分析里讨论比 tools 少；具体 UI 接入实现细节未深查。

---

## 7. Channels（v2.x 新）

✅ MCP server 还可声明 `claude/channel` capability，**主动 push 消息到 session**（不是被动响应）。
启动时：`claude --channels notion-updates,sentry-alerts`

✅ 用途：
- CI/CD pipeline 完成 → push 通知
- Sentry 报警 → push
- Telegram/Slack 收到 @ → push

✅ 实现：channel 消息进 session h2A queue → Claude 在下个 turn 看到，决定如何回应。

---

## 8. 对 Forgify 的改进建议

> 现状：Phase 5 计划接 MCP；尚无任何 MCP 代码；Forgify Tool 接口已统一，MCP tool 应注册到同一接口。
> 推荐库：`mark3labs/mcp-go`（已在 backend-design.md）。

### 8.1 整体架构建议

```
Forgify Service
  ├─ tools []agent.Tool  (内置 13 个)
  └─ mcpRegistry         (新)
        ├─ servers map[string]*mcpClient
        ├─ schemaCache  (lazy schema)
        └─ adapter func -> agent.Tool
```

### 8.2 实现步骤（优先级）

| # | 步骤 | 优先级 | 实施 |
|---|---|---|---|
| 1 | **stdio transport 最小可用** | P0 | 用 `mark3labs/mcp-go` 的 client：`mcp.NewStdioClient(cmd, args, env)`。在 service 启动时读 `~/.forgify/mcp.json`（schema 同 §2.2）；为每个 server 起 client；调 `tools/list` 拿到 tool 列表 |
| 2 | **MCPToolAdapter** | P0 | 新文件 `agent/mcp.go`：`type mcpTool struct { client *mcp.Client; name, desc string; schema json.RawMessage }`<br>实现 Tool 接口：Name() = `"mcp__"+server+"__"+name`；Description() / Parameters() 直返；Execute() 调 `client.CallTool(ctx, name, args)`<br>注册到 service.tools 和内置 tool 同列表 |
| 3 | **延迟加载** | P1 | session start 仅调 `tools/list` 拿 (name, shortDescription)；schema 用 `sync.Once`-style lazy。Tool.Parameters() 第一次被调用时同步阻塞拉完整 schema。**简化版**：先全量加载，等 MCP tool 数量超 50 个再上 lazy |
| 4 | **HTTP transport** | P2 | `mark3labs/mcp-go` 也支持 HTTP；同样接进来。OAuth 先不做（手动配 token） |
| 5 | **OAuth 2.0** | P3 | 复杂，先支持手动 token。OAuth flow 涉及打开浏览器、回调 server、token 存储、自动刷新 — 工作量大 |
| 6 | **权限 / Hook 集成** | P0 | mcpTool.Name() 用 `mcp__` 前缀 → 自动被报告 06 / 08 改进里的 hook + 权限规则匹配（"mcp__github__*"）。**这条几乎 0 额外工作**，是 #2 设计正确的副作用 |
| 7 | **自动重连** | P2 | mcp-go 内置；配置好 retry 参数即可。Forgify 后端 daemon 适合做长连重连 |
| 8 | **list_changed 通知** | P3 | mcp-go 提供；收到时清 schemaCache 即可 |

### 8.3 最小可行版本（MVP）代码草图

```go
// internal/app/agent/mcp.go
package agent

import (
    "context"
    "encoding/json"
    "fmt"
    "github.com/mark3labs/mcp-go/mcp"
)

type MCPClient interface {
    ListTools(ctx context.Context) ([]mcp.Tool, error)
    CallTool(ctx context.Context, name string, args map[string]any) (string, error)
    Close() error
}

type MCPTool struct {
    serverName string
    client     MCPClient
    info       mcp.Tool   // name, description, inputSchema
}

func (t *MCPTool) Name() string {
    return fmt.Sprintf("mcp__%s__%s", t.serverName, t.info.Name)
}
func (t *MCPTool) Description() string { return t.info.Description }
func (t *MCPTool) Parameters() json.RawMessage {
    b, _ := json.Marshal(t.info.InputSchema)
    return b
}
func (t *MCPTool) Execute(ctx context.Context, argsJSON string) (string, error) {
    var args map[string]any
    if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
        return "", fmt.Errorf("mcp: bad args: %w", err)
    }
    return t.client.CallTool(ctx, t.info.Name, args)
}
func (t *MCPTool) IsReadOnly() bool {  // 配合报告 02 改进 #5
    return t.info.Annotations.ReadOnlyHint
}

// 注册：
// service.go 启动时
func (s *Service) registerMCPServers(ctx context.Context, configPath string) error {
    cfg := loadMCPConfig(configPath)
    for name, server := range cfg.Servers {
        client, err := newMCPClient(server)  // 按 transport 分支
        if err != nil { return err }
        tools, err := client.ListTools(ctx)
        if err != nil { return err }
        for _, t := range tools {
            s.tools = append(s.tools, &MCPTool{serverName: name, client: client, info: t})
        }
    }
    return nil
}
```

最先做：**#1 + #2 + #6**（stdio + adapter + 命名前缀使权限/hook 自动适用）。约 2-3 天工作量，做完就能接生态里大量现成 MCP server（github、filesystem、postgres、puppeteer 等）。


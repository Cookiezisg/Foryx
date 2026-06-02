---
id: DOC-112
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# MCP Domain — Model Context Protocol 基础设施

> **核心地位**：MCP 是 Forgify 与外部现成生态（如 Google Maps, Slack, GitHub）对接的 **“协议网桥”**。它遵循 Anthropic 发布的标准协议，通过 JSON-RPC 2.0 驱动外部子进程。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `MCPServer` (配置清单)
此数据持久化在 `~/.forgify/users/<uid>/mcp.json` 物理文件中。
```json
{
  "servers": {
    "google-maps": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-google-maps"],
      "env": { "MAPS_API_KEY": "..." }
    }
  }
}
```

### 1.2 `Call` (调用审计表 - D22)
```go
type Call struct {
    ID            string         `gorm:"primaryKey;type:text" json:"id"` // mcl_<16hex>
    UserID        string         `gorm:"not null;index" json:"-"`
    ServerName    string         `gorm:"not null;index" json:"serverName"`
    ToolName      string         `gorm:"not null" json:"toolName"`
    ServerVersion string         `json:"serverVersion"`
    
    // D22 字段
    Status        string         `json:"status"` // ok|failed|timeout
    Input         string         `json:"input"`  // JSON-RPC Arguments
    Output        string         `json:"output"` // Tool result
    ElapsedMs     int64          `json:"elapsedMs"`
    CreatedAt     time.Time      `json:"createdAt"`
}
```

### 1.3 `HealthSnapshot` (心跳记录)
```go
type HealthSnapshot struct {
    ID          string    `gorm:"primaryKey" json:"id"` // mch_<16hex>
    ServerName  string    `gorm:"index" json:"serverName"`
    Healthy     bool      `json:"healthy"`
    LatencyMs   int64     `json:"latencyMs"`
    ErrorMsg    string    `json:"errorMsg"`
    CheckedAt   time.Time `json:"checkedAt"`
}
```

---

## 2. 核心原理 (Principles)

### 2.1 JSON-RPC StdIO 网桥
每个激活的 MCP Server 对应一个宿主机子进程：
- **生命周期**：Service 维护进程池。首次调用工具时 Lazy-load 进程。
- **协议握手**：发送 `initialize` 方法，协商版本和能力（Capability）。
- **StdIO 劫持**：后端监听进程的 `stdout` 作为响应流，`stderr` 作为实时日志缓冲区。

### 2.2 Marketplace 一键安装
Forgify 内置了官方 Registry：
- **原理**：利用 `npm/npx` 或 `python` 环境动态拉取远端包。
- **配置固化**：安装成功后，后端自动向 `mcp.json` 写入对应的 `command` 和 `args`。

### 2.3 自动发现机制 (Reflection)
后端通过 `listTools` 定期刷新可用工具：
- **映射逻辑**：MCP 工具在 LLM 侧的名称会被前缀化为 `mcp:server:tool`，彻底消除与本地 `fn_` 的冲突隐患。

---

## 3. 生命周期 (Lifecycle)

1. **安装 (Installing)**：用户调 `:install` 或 PUT 端点。
2. **连接 (Connecting)**：子进程拉起，StdIO Pipe 建立。
3. **就绪 (Ready)**：`initialize` 握手成功，心跳检查通过。
4. **调用 (Calling)**：LLM 发起 tool_call -> RPC `callTool`。
5. **熔断 (Circuit Breaking)**：若子进程 stderr 输出异常，自动标记为 `not_connected` 触发自愈重连。

---

## 4. 跨域集成 (Interactions)

- **Workflow**：节点类型 `mcp` 的底层实现。
- **Agent**：`agent_versions.tools` 列表中可配置 `mcp:` 引用。
- **Metrics**：通过 `mch_` 表渲染服务器的“健康心电图”。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 备注 |
|---|---|---|---|
| `ErrServerNotFound` | 404 | `MCP_SERVER_NOT_FOUND` | |
| `ErrServerNotConnected` | 409 | `MCP_SERVER_DOWN` | 子进程崩溃或未拉起。 |
| `ErrToolNotFound` | 404 | `MCP_TOOL_NOT_FOUND` | 协议握手中未发现此工具。 |
| `ErrToolCallFailed` | 502 | `MCP_RPC_ERROR` | 上游 Server 返回错误 JSON。 |
| `ErrInstallFailed` | 502 | `MCP_INSTALL_FAILED` | npm/pip 报错。 |
| `ErrAlreadyInstalled` | 409 | `MCP_NAME_CONFLICT` | |

---
id: DOC-127
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Web Tools — 网络抓取、搜索路由与安全网关原理

> **核心职责**：Web 域是 Forgify 赋予 Agent 的 **“网络感知器”**。它包含 `WebFetch` (精准抓取) 和 `WebSearch` (分布式搜索) 两大引擎。核心目标是平衡 LLM 对新鲜知识的需求与宿主机的网络安全（防 SSRF）。

---

## 1. 核心原理 (Principles)

### 1.1 SSRF Two-Layer Guard (SSRF 双层防线)
为了防止 LLM 诱导后端访问本地网络（如 `http://localhost/`）：
- **Layer 1: Hostname Guard**：物理拦截 `localhost`, `127.0.0.1` 及其十六进制变体。
- **Layer 2: Redirect Check**：系统强制开启 `CheckRedirect` 钩子。即便公网 URL `302` 重定向到内网 IP，后端也会在重定向发生的瞬间将其切断。
- **DNS Rebinding 防御**：每次物理连接前都会强制执行一次物理 DNS 解析，并验证解析出的所有 IP 段（拒私有 A/B/C 类地址）。

### 1.2 Multi-Tier Search Routing (多阶搜索路由)
`WebSearch` 不绑定单一供应商，它采用了 **“分级探测”** 机制：
1. **BYOK Tier**：检查用户是否配置了 `Brave`, `Serper`, `Tavily` 或 `Bocha` (国产) 的 API Key。
2. **MCP Tier**：若无 Key，则检查是否安装了 `duckduckgo-search` MCP Server。
3. **Actionable Failure**：若以上皆空，系统不会返回“0 结果”，而是返回一段具体的引导文案，告诉 LLM 指导用户去配 Key。

### 1.3 LLM-Aided Summarization (摘要先行)
针对 `WebFetch`：
- **Jina Reader 优先**：默认通过 `r.jina.ai` 将网页转为干净的 Markdown。
- **极速摘要**：抓取到的 HTML/MD 不直接塞给主对话（防止撑爆窗口），而是先分发给 **Utility 档** 模型进行初步提炼。

---

## 2. 生命周期 (Lifecycle)

1. **寻址 (Parsing)**：LLM 给出 URL 或搜索 Query。
2. **鉴权 (Guarding)**：网络网关验证目标地址安全性。
3. **抓取 (Fetching)**：后端 HttpClient 拉取内容（30s 硬超时）。
4. **提炼 (Processing)**：通过 Utility 模型将 1MB+ 的原始 HTML 压缩为 2KB 的核心结论。
5. **合流 (Returning)**：结果作为 `tool_result` 注入消息流。

---

## 3. 跨域集成 (Interactions)

- **APIKey**：为搜索供应商提供 BYOK 凭证。
- **Model**：决定摘要提炼所使用的低成本模型（Utility 档）。
- **MCP**：作为搜索的备选物理引擎。
- **Relation**：记录对话产生的 `web_reference` 关系。

---

## 4. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 备注 |
|---|---|---|---|
| `ErrAuthFailed` | 401 | `WEBSEARCH_AUTH_FAIL` | BYOK Key 填错了，同步触发 MarkInvalid。 |
| `ErrRateLimited` | 429 | `WEBSEARCH_LIMIT` | 搜索供应商限流。 |
| `ErrRedirectBlocked` | 403 | `PERM_DENIED` | 安全拦截：重定向到了内网地址。 |
| `ErrUpstreamHTTP` | 502 | `WEB_GATEWAY_ERROR` | 对方网站返回 5xx。 |
| `ErrEmptyURL` | 400 | `INVALID_REQUEST` | |

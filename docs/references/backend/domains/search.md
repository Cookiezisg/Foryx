---
id: DOC-120
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Search Domain — 本地文件扫描、Grep 与 Glob 原理

> **核心职责**：Search 域是 Forgify 的 **“文件雷达”**。它不负责网络搜索，而是专注于宿主机物理文件的高性能检索。通过实现 `Ripgrep` 级别的扫描能力，为 Agent 提供快速定位代码和文档的手段。

---

## 1. 核心原理 (Principles)

### 1.1 Ripgrep-Style Performance
后端不直接调 shell grep，而是集成了高性能的搜索算法：
- **物理路径映射**：搜索始终限定在用户的 Workspace 根目录下。
- **并发扫描**：利用 Go 协程池遍历文件树。
- **内存安全**：限制单次 Grep 结果的大小（默认 100 条），防止超大文件拖垮内存。

### 1.2 Binary-Safe Globbing
`Glob` 工具实现了标准的 Unix 通配符匹配：
- **原理**：基于 `path/filepath` 的 Glob 扩展。
- **递归支持**：支持 `**/*.ts` 这种深层扫描。
- **物理校验**：系统会自动过滤掉 `.git`, `node_modules` 等二进制干扰项（通过内置的 Ignore 清单）。

### 1.3 Result Chunking (分块投影)
为了适配 LLM 的上下文窗口：
- **智能截断**：搜索结果不仅仅返回行号，还会携带命合点前后 2 行的上下文（Context lines）。
- **去重逻辑**：在多文件搜索时，自动按文件物理路径进行结果聚合展示。

---

## 2. 工具规约 (The Tools)

### 2.1 `grep_search`
| 参数 | 类型 | 说明 |
|---|---|---|
| `pattern` | string | 核心正则。 |
| `include_pattern`| string | 文件过滤器（如 `*.go`）。 |
| `context` | number | 前后关联行数。 |

### 2.2 `glob`
- **用途**：文件树结构盘点。
- **特性**：返回物理路径的有序列表。

---

## 3. 生命周期 (Lifecycle)

1. **请求 (Requesting)**：LLM 发起搜索指令。
2. **鉴权 (Guarding)**：`Permissions` 模块检查路径是否在 `protectedPaths` 中。
3. **物理扫描 (Scanning)**：后端开启多线程读磁盘。
4. **格式化 (Formatting)**：将物理结果转化为结构化的 `SearchBlock` 推入 SSE 流。
5. **返回 (Responding)**：LLM 收到结果，继续执行逻辑。

---

## 4. 跨域集成 (Interactions)

- **Chat**：作为主要的工具集注入。
- **Sandbox**：搜索操作受沙箱物理根目录的限制。
- **Permissions**：受 `protectedPaths` 约束。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 备注 |
|---|---|---|---|
| `ErrPatternInvalid`| 400 | `INVALID_REQUEST` | 正则表达式语法错误。 |
| `ErrPathForbidden` | 403 | `PERM_DENIED` | 命中路径保护规则。 |
| `ErrSearchTimeout` | 504 | `REQUEST_TIMEOUT` | 扫超大磁盘超过 10s。 |
| `ErrTooManyMatches`| 422 | `SEARCH_RESULT_TOO_LARGE` | 命中了过万条记录，请细化搜索。 |
| `ErrNotFound` | 404 | `NOT_FOUND` | 搜索路径不存在。 |

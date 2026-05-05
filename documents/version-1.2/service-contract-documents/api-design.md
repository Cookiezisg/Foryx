# API Design — V1.2 REST API 一眼索引

**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../service-design-documents/`](../service-design-documents/) — 每个 domain 的详设计

**定位**：**一眼能看到谁提供了什么**。详细的 request/response schema、错误细节、边界 case，**去 service-design-documents 看**。

**遵守标准**：N1（envelope）/ N2（状态码）/ N3（camelCase）/ N4（分页）/ N5（RESTful）

---

## 全局约定

### 路径前缀
所有 API 统一前缀 `/api/v1/`。

### 响应 envelope

```typescript
// 成功
type Success<T> = { data: T }

// 列表（分页）
type Paged<T> = {
  data: T[]
  nextCursor: string | null
  hasMore: boolean
}

// 失败
type Error = {
  error: {
    code: string        // 如 "API_KEY_NOT_FOUND"
    message: string     // 人类可读
    details?: object    // 可选上下文
  }
}
```

### 状态码语义（N2）

| 码 | 场景 |
|---|---|
| 200 | 读取成功 / 更新成功（有响应体） |
| 201 | 创建成功（返回新资源）|
| 202 | 异步任务已接受（如启动流式响应）|
| 204 | 删除成功 / 操作成功（无响应体） |
| 400 | 请求参数错误 |
| 401 | 未认证（Phase N 引入 auth 后）|
| 403 | 已认证但无权限 |
| 404 | 资源不存在 |
| 409 | 业务冲突（如重名）|
| 422 | 参数合法但业务拒绝（如 API Key 测试失败）|
| 500 | 内部错误（bug）|

### 字段命名（N3）
- 请求/响应字段：`camelCase`
- DB 列名：`snake_case`（repo 层转换）
- 错误码：`SCREAMING_SNAKE_CASE`

### 分页（N4）
列表端点支持 `?cursor=xxx&limit=50`，默认 50，最大 200。配置类（如 `/model-configs`）无分页。

### 业务动作命名（N5）
- 状态变更：用 `PATCH` + 状态字段（不用 `/archive`、`/restore` 子路径）
- 不能用 RESTful 表达的动作：`:action` 后缀（如 `POST /api-keys/{id}:test`）

---

## API 清单

> **状态**：⬜ 未设计 | 🔄 设计中 | ✅ 已实现

### 通用

| Method | Path | 用途 | 状态 |
|---|---|---|---|
| GET | `/api/v1/health` | 存活探针（Electron 启动后读）| ✅ |
| GET | `/api/v1/events?conversationId=xxx` | SSE 事件流（keep-alive ping + Last-Event-ID）| ✅ |

---

### Phase 2：基础对话能力

#### apikey ✅
详见 [`../service-design-documents/apikey.md`](../service-design-documents/apikey.md) §10。

| Method | Path | 用途 |
|---|---|---|
| POST | `/api/v1/api-keys` | 创建 |
| GET | `/api/v1/api-keys` | 列表（分页 + `?provider=` 过滤）|
| PATCH | `/api/v1/api-keys/{id}` | 更新 displayName / baseUrl |
| DELETE | `/api/v1/api-keys/{id}` | 软删 |
| POST | `/api/v1/api-keys/{id}:test` | 连通性测试 |

#### model ✅
详见 [`../service-design-documents/model.md`](../service-design-documents/model.md)。用户给每个 scenario 选定 `(provider, modelID)`；Phase 2 仅 `scenario=chat`。

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/v1/model-configs` | 列出当前用户所有 scenario 的配置（不分页，最多 ~6 条）|
| PUT | `/api/v1/model-configs/{scenario}` | upsert 指定 scenario 的配置（200，无论创建或更新）|

#### conversation ✅
详见 [`../service-design-documents/conversation.md`](../service-design-documents/conversation.md)。对话线程容器的 CRUD；消息历史由 chat domain 管理。

| Method | Path | 用途 |
|---|---|---|
| POST | `/api/v1/conversations` | 创建对话（201）；title 可为空 |
| GET | `/api/v1/conversations` | 列表（200，cursor 分页，最新优先）|
| PATCH | `/api/v1/conversations/{id}` | 改名（200）|
| DELETE | `/api/v1/conversations/{id}` | 软删（204）|

#### chat ✅
详见 [`../service-design-documents/chat.md`](../service-design-documents/chat.md)。自有 `infra/llm` 驱动（Eino 已移除），Block 模型，Phase 2 tools=nil（纯流式对话），Phase 3+ 注入 System Tools。

| Method | Path | 用途 |
|---|---|---|
| POST | `/api/v1/attachments` | 上传附件（multipart，50MB 限制）→ 201 返回 attachment_id |
| POST | `/api/v1/conversations/{id}/messages` | 发送消息（202，队列化异步 Agent 运行）；body 含 `attachmentIds[]` |
| DELETE | `/api/v1/conversations/{id}/stream` | 取消正在运行的 Agent（204）；404 STREAM_NOT_FOUND |
| GET | `/api/v1/conversations/{id}/messages` | 消息历史（cursor 分页，ASC 时序）；每条消息含 `blocks[]`（text/reasoning/tool_call/tool_result/attachment_ref）+ `inputTokens` + `outputTokens` |

---

### Phase 3：工具锻造能力

#### forge ✅
详见 [`../service-design-documents/forge.md`](../service-design-documents/forge.md) §11–12。

| Method | Path | 用途 |
|---|---|---|
| POST | `/api/v1/forges` | 创建工具（直接传 code）|
| GET | `/api/v1/forges` | 列表（分页）|
| GET | `/api/v1/forges/{id}` | 详情 |
| PATCH | `/api/v1/forges/{id}` | 更新（直接生效）|
| DELETE | `/api/v1/forges/{id}` | 软删 |
| POST | `/api/v1/forges/{id}:run` | 执行工具 |
| POST | `/api/v1/forges/{id}:export` | 导出 JSON |
| POST | `/api/v1/forges:import` | 导入 JSON |
| GET | `/api/v1/forges/{id}/versions` | 版本列表 |
| GET | `/api/v1/forges/{id}/versions/{version}` | 单版本详情 |
| POST | `/api/v1/forges/{id}:revert` | 回滚版本 |
| GET | `/api/v1/forges/{id}/pending` | 当前 pending |
| POST | `/api/v1/forges/{id}/pending:accept` | 接受 pending |
| POST | `/api/v1/forges/{id}/pending:reject` | 拒绝 pending |
| GET | `/api/v1/forges/{id}/test-cases` | 测试用例列表 |
| POST | `/api/v1/forges/{id}/test-cases` | 创建测试用例 |
| DELETE | `/api/v1/forges/{id}/test-cases/{tcId}` | 删除测试用例 |
| POST | `/api/v1/forges/{id}/test-cases/{tcId}:run` | 运行单个测试 |
| POST | `/api/v1/forges/{id}:test` | 运行全部测试 |
| POST | `/api/v1/forges/{id}:generate-test-cases` | LLM 生成测试用例（一次性返回 JSON 批量）|
| GET | `/api/v1/forges/{id}/executions` | 执行历史（统一端点，?kind=run\|test &batchId=&cursor=&limit= 过滤；替代旧的 run-history + test-history）✅ Phase 5 |
| GET | `/api/v1/forges/{id}` | 详情（响应含 `pending` 字段，存在时为完整 ForgeVersion 对象，否则缺省）✅ Phase 5 |

> Phase 5 整合（2026-05-02）：原 `forge_run_history` + `forge_test_history` 两表合并为 `forge_executions`（kind 字段区分），HTTP 端点对应合并为单 `:executions`。响应是分页 envelope（`{data, nextCursor, hasMore}`），与其他列表端点一致。

> **沙箱迭代 1（2026-05-03）**——`POST /forges` body 加 **`dependencies` (PEP 508 string array, optional)** + **`pythonVersion` (PEP 440 spec, optional)**；service 同步等 venv sync 完成才返，响应的 forge 对象计算字段含 `envStatus` / `envError` / `envSyncedAt` / `envSyncStage` / `envSyncDetail` / `activeVersionId`（由 `attachActiveEnv` 填充）。`PATCH /forges/{id}` 不接 deps 改动——deps 改走 `edit_forge` LLM tool / pending → accept 流程。`POST /forges/{id}/pending:accept` 守卫 pending 的 `envStatus`：仅 `ready` 才放行；`failed` 返 422 `FORGE_ENV_FAILED`，其他态返 422 `FORGE_ENV_NOT_READY`。`POST /forges/{id}:revert` 自动检测目标版本 `envStatus="evicted"` 并触发同步 sync 重建。LLM tool args（`create_forge` / `edit_forge`）schema 同样含 `dependencies` 与 `python_version` 字段，并在 `tool_result` 返回 `env_status` / `env_error` 让 LLM 据此决定下一步（详 forge.md §10）。

#### chat（Phase 3 升级）✅
Forge System Tools 注入（search/get/create/edit/run，5 个）。SSE 见 events-design.md。无新 HTTP 端点，见 Phase 2 chat 端点。

> Phase 3 后优化轮（2026-05-02）删除了原 Phase 3 装的 8 个通用 system tool（read_file/write_file/list_dir/run_shell/run_python/datetime/web_search/fetch_url）。新一代 system tools（Read/Write/Edit/Bash/Glob/Grep/LS）将在 Phase 5 重建。

---

### Phase 5：System Tool 第二代 + 任务追踪 + 用户问询（2026-05-04）

#### 系统工具家族（注入 chat agent，无新 HTTP 端点）

| 家族 | 工具 | 说明 |
|---|---|---|
| filesystem | `Read` / `Write` / `Edit` | 文件读写编辑；PathGuard 守敏感路径；Edit 走 must-Read-first 守卫 + 原子写 |
| search | `Grep` / `Glob` | rg 优先 + stdlib 兜底；Glob 输出 JSON 含 type/size/mtime（决策 D3：替代 LS）|
| web | `WebFetch` / `WebSearch` | Jina r.jina.ai 摘要 + 直 GET fallback；3 层搜索 fallback（SearXNG 池 → Bing → Bing CN）；SSRF 守卫拒私网 / loopback / link-local |
| shell | `Bash` / `BashOutput` / `KillShell` | 前后台双模式；cwd 状态机（AgentState.Cwd）；后台子进程 ProcessManager 注册 256 KB 环形缓冲；KillShell SIGKILL 幂等 |
| todo | `TodoCreate` / `TodoList` / `TodoGet` / `TodoUpdate` | 对话级 TODO 追踪（mini-domain，详见 [`../service-design-documents/todo.md`](../service-design-documents/todo.md)）|
| ask | `AskUserQuestion` | 暂停 agent loop 等用户回答；问题坐 chat.message SSE，答案走下方 answers endpoint |

详细工具集在 chat agent 注入清单见 [`../service-design-documents/chat.md`](../service-design-documents/chat.md)。

#### chat（Phase 5 新增端点）✅

| Method | Path | 用途 |
|---|---|---|
| POST | `/api/v1/conversations/{id}/answers` | 投递 AskUserQuestion 工具的用户答案 → 204；body: `{toolCallId, answer}`；404 ASK_NO_PENDING_QUESTION 表 toolCallId 未在 Wait |

> 决策 D11：问题本身坐 `chat.message` SSE 流（AskUserQuestion tool_call block 含 `question` + `options`），不新建事件家族；answers endpoint 仅闭合答案投递路径。

---

### Phase 4 准备件（2026-05-05 提前交付）

提前完成以下 4 个 domain 作为 Phase 4-5 工作流 / 智能化的基础设施。设计完成、待实施。

#### subagent 📐
详见 [`../service-design-documents/subagent.md`](../service-design-documents/subagent.md)。LLM 通过 `Subagent` system tool spawn 子 LLM loop（避开 `todo` domain 撞车而改名）；独立 context、过滤后 tool registry；复用 chat runner。

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/v1/conversations/{id}/subagent-runs` | 列对话下所有 subagent run（cost analysis）|
| GET | `/api/v1/subagent-runs/{id}` | 单 run 详情（prompt + result）|
| GET | `/api/v1/subagent-runs/{id}/messages` | run 内全部 messages（流式小窗回放用）|
| GET | `/api/v1/subagent-types` | 列所有可用 subagent 类型（Explore / Plan / general-purpose）|

#### mcp 📐
详见 [`../service-design-documents/mcp.md`](../service-design-documents/mcp.md)。官方 `modelcontextprotocol/go-sdk` v1.x；stdio only；search/call 模式不 flat 注册（避 token 爆炸）；自包含原则（只读 `~/.forgify/mcp.json`）。

##### Server 配置 / 生命周期

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/v1/mcp-servers` | 列所有配置（含 status + tools + 健康字段）|
| GET | `/api/v1/mcp-servers/{name}` | 单 server 详情 + tools |
| PUT | `/api/v1/mcp-servers/{name}` | 增/改配置（写 mcp.json + 立即 Connect）|
| DELETE | `/api/v1/mcp-servers/{name}` | 删配置 + disconnect（204）|
| POST | `/api/v1/mcp-servers:import` | **拖拽导入**（multipart 上传 mcp.json 文件 / 文本 fragment）|
| POST | `/api/v1/mcp-servers/{name}:reconnect` | 强制重启子进程（degraded / failed 恢复用）|
| POST | `/api/v1/mcp-servers/{name}:health-check` | 主动健康检查（调 tools/list 验证）|

##### Registry — 内置 Marketplace

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/v1/mcp-registry` | 列所有可装 server entries（v1 内置 5 个：playwright / markitdown / context7 / duckduckgo-search / sqlite，Bundled=true 表 marketplace 默认推荐项；外加 hidden 的 everything 仅 pipeline test 用。装机一律 lazy via sandbox）|
| GET | `/api/v1/mcp-registry/{name}` | 单 entry 详情（含 RequiredEnv / RequiredArgs）|
| POST | `/api/v1/mcp-registry/{name}:install` | 安装：填 env + args → 写 mcp.json + Connect |

**没有 `:enable` / `:disable`**——配置在 mcp.json 即启用，删除即禁用，无中间态。

#### skill 📐
详见 [`../service-design-documents/skill.md`](../service-design-documents/skill.md)。`SKILL.md` 跨厂兼容（Anthropic spec）；progressive disclosure 三层加载；`context: fork` 可组合到 subagent；自包含（仅 `~/.forgify/skills/`，无项目级）。

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/v1/skills` | 列所有 skills（含 frontmatter，**不**含 body）|
| GET | `/api/v1/skills/{name}` | 单 skill 详情 |
| GET | `/api/v1/skills/{name}/body` | 拿 SKILL.md body 内容（编辑用）|
| POST | `/api/v1/skills` | 创建新 skill（写 SKILL.md 到 user 目录，201）|
| PUT | `/api/v1/skills/{name}` | 整体替换 skill 内容（200）|
| DELETE | `/api/v1/skills/{name}` | 删除 skill 目录（204）|
| POST | `/api/v1/skills:import` | **拖拽导入**（folder / zip / tar / 单 SKILL.md）|
| POST | `/api/v1/skills:refresh` | 手动 Rescan（绕过 fsnotify，debug 用）|
| POST | `/api/v1/skills/{name}:invoke` | 手动调用（slash command 路径用）|

#### catalog 📐
详见 [`../service-design-documents/catalog.md`](../service-design-documents/catalog.md)。统一能力目录（forge + skill + mcp）。LLM-gen summary + 自动跨类目路由观察。**1s polling + atomic 单 flight + fingerprint dedup**。**不发 SSE**（内部组件）。

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/v1/catalog` | 当前 catalog cache 内容（debug / UI 显示）|
| POST | `/api/v1/catalog:refresh` | 强制立即 refresh（绕过 1s polling 间隔）|

**没有 routing-hints 端点**——路由提示由 generator LLM-gen 时直接写进 summary，用户想影响路由 → 编辑源头 forge/skill/mcp 的 description。

#### chat 同步改动 📐

`chat.message` SSE 事件 schema 加可选字段 `subagentRunId`：当 subagent 内部 sub-runner 推消息时带此字段，前端按是否携带分流到主对话区 / 流式小窗（详 [`../service-design-documents/subagent.md`](../service-design-documents/subagent.md) §10 / [`../service-design-documents/chat.md`](../service-design-documents/chat.md)）。**不影响主对话 wire format**（omitempty）。

---

### Phase 4：工作流能力

#### workflow ⬜
#### flowrun ⬜
#### scheduler ⬜
#### trigger ⬜

---

### Phase 5：智能化能力

#### knowledge ⬜
#### document ⬜
#### intent ⬜
#### mcpserver — 已提前交付 ✅ 见上方"Phase 4 准备件 / mcp"
#### skill — 已提前交付 ✅ 见上方"Phase 4 准备件 / skill"
#### chat（终极智能版）⬜

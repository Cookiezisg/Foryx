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
| 410 | 历史已淘汰（事件日志 SSE 重连超 buffer）— 客户端 refetch 全态 |
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
| GET | `/api/v1/eventlog?conversationId=xxx` | **per-conversation** SSE 事件流（递归事件日志协议；`Last-Event-ID` 重连；超 buffer 返 410 Gone SEQ_TOO_OLD）— 详 [`../event-log-protocol.md`](../event-log-protocol.md) | ✅ |
| GET | `/api/v1/notifications` | **global broadcast** SSE entity 状态总线（单 envelope `{type,id,data,conversationId?}` 覆盖 conversation / todo / mcp_server / skill / catalog / sandbox_env；`Last-Event-ID` 重连）— 详 [`events-design.md §11`](./events-design.md) | ✅ |
| GET | `/api/v1/conversations/{id}/eventlog?from=N` | 历史回放：DB 重构 block 事件流（client 收 410 后用此 refetch；返 `{events, tailSeq, count}`）| ✅ |

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
| GET | `/api/v1/providers` | 列 ProviderMeta 注册表（`?category=llm` 或 `?category=search` 过滤）；前端用以替代客户端硬编码 provider 列表（屎山拯救计划 #4 收尾）|

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
| GET | `/api/v1/conversations/{id}` | 单对话详情（200，含 systemPrompt / autoTitled / metadata）|
| PATCH | `/api/v1/conversations/{id}` | partial update（200）；body `{title?, systemPrompt?}`——两个字段可任意组合改 |
| DELETE | `/api/v1/conversations/{id}` | 软删（204）|

#### chat ✅
详见 [`../service-design-documents/chat.md`](../service-design-documents/chat.md)。自有 `infra/llm` 驱动（Eino 已移除），Block 模型，Phase 2 tools=nil（纯流式对话），Phase 3+ 注入 System Tools。

| Method | Path | 用途 |
|---|---|---|
| POST | `/api/v1/attachments` | 上传附件（multipart，50MB 限制）→ 201 返回 attachment_id |
| POST | `/api/v1/conversations/{id}/messages` | 发送消息（202，队列化异步 Agent 运行）；body 含 `attachmentIds[]` |
| DELETE | `/api/v1/conversations/{id}/stream` | 取消正在运行的 Agent（204）；404 STREAM_NOT_FOUND |
| GET | `/api/v1/conversations/{id}/messages` | 消息历史（cursor 分页，ASC 时序）；每条消息含 `blocks[]`（**6 类型**：text/reasoning/tool_call/tool_result/progress/message）+ `attrs`（user msg 含 `attachments[]` 引用、subagent sub-message 含 `kind=subagent_run`）+ `inputTokens` + `outputTokens`。**注**：附件不是 block 类型，是 `attrs.attachments[]` 引用 `attachments` 表 |

---

### Phase 3：工具锻造能力 — function trinity (forge_redesign Plan 01)

#### function ✅
详见 [`../service-design-documents/function.md`](../service-design-documents/function.md) §6 + redesign topic [`../adhoc-topic-documents/forge_redesign/02-function.md`](../adhoc-topic-documents/forge_redesign/02-function.md)。

| Method | Path | 用途 |
|---|---|---|
| POST | `/api/v1/functions` | 创建 function(扁平 definition: name/description/code/parameters/dependencies/...);后台起 env sync。HTTP 走扁平,LLM 走 ops。|
| GET | `/api/v1/functions` | 列表(分页 cursor) |
| GET | `/api/v1/functions/{id}` | 详情(含 pending + active version env 状态镜像字段) |
| PATCH | `/api/v1/functions/{id}` | 改元数据(name/description/tags;code/deps 走 LLM ops 流) |
| DELETE | `/api/v1/functions/{id}` | 软删(D20 级联:workflow domain 收到 notification 标 needs_attention) |
| POST | `/api/v1/functions/{id}:run` | 执行(body `{args, version?}`);取消走 caller ctx,无 per-call timeout knob |
| POST | `/api/v1/functions/{id}:resync` | 强制重建 active version 的 venv(后台);幂等 |
| POST | `/api/v1/functions/{id}:revert` | 回滚到指定 accepted 版本号 |
| GET | `/api/v1/functions/{id}/versions` | 版本分页(?status= filter) |
| GET | `/api/v1/functions/{id}/versions/{v}` | 单版本(integer→ByNumber, fnv_*→ById) |
| GET | `/api/v1/functions/{id}/pending` | 当前 pending(无则 404 FUNCTION_PENDING_NOT_FOUND) |
| POST | `/api/v1/functions/{id}/pending:accept` | accept → 新 accepted 版本 + 翻 ActiveVersionID;应用 AcceptedVersionCap=50 |
| POST | `/api/v1/functions/{id}/pending:reject` | reject(状态 → rejected,不动 ActiveVersion) |
| GET | `/api/v1/functions/{id}/executions` | 执行日志列表 ✅ D22(? versionId/status/conversationId/flowrunId &cursor &limit);返 previews + aggregates |
| GET | `/api/v1/function-executions/{execId}` | 全局执行详情 ✅ D22;返完整行 + machine-computed hints(outputEmpty / significantlySlower) |

> forge_redesign Plan 01(2026-05-11):整套 forge 代码路径在 Phase 7 删除,trinity domain function 替代。POST 走扁平 definition(curl/UI/script 友好),LLM 走 ops 增量编辑(create_function / edit_function 工具单 op emit 1 progress delta)。env sync 是 caller-owns lifetime(D3):创建/edit/accept 后 SyncEnvForVersion 后台起 goroutine,UI 经 GET /functions/{id} 看 envStatus 翻 ready/failed。D22:每次 RunFunction 终态写 1 行 function_executions(detached ctx §S9);9 LLM tools 含 search_function_executions / get_function_execution 让 LLM 自诊断。

#### chat（Phase 3 升级）✅
Function System Tools 注入（7 CRUD/exec + 2 D22 execution log,共 9 个）。SSE 见 events-design.md。无新 HTTP 端点,见 Phase 2 chat 端点。

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

#### subagent ✅
详见 [`../service-design-documents/subagent.md`](../service-design-documents/subagent.md)。LLM 通过 `Subagent` system tool spawn 子 LLM loop（避开 `todo` domain 撞车而改名）；独立 context、过滤后 tool registry；与 chat 都通过 `internal/app/loop` 共享 ReAct 引擎。V1 内置 3 类型（Explore / Plan / general-purpose）。**V1.2 D3-D4 完成 2026-05-06**。

**无独立 HTTP 端点**——2026-05 schema 统一后 sub-run 不再有专表，sub-run 数据是 `messages` 行（`attrs.kind=subagent_run`），sub-run transcript 是该 message 在 `message_blocks` 的 blocks。前端经 `GET /api/v1/conversations/{id}/messages` 读 sub-run 状态；type registry 由 `Subagent` 系统工具进程内消费，不暴露 HTTP。

#### mcp ✅
详见 [`../service-design-documents/mcp.md`](../service-design-documents/mcp.md)。官方 `modelcontextprotocol/go-sdk` v1.6；stdio only；search/call 模式不 flat 注册（避 token 爆炸）；自包含原则（只读 `~/.forgify/mcp.json`）。**V1.2 D5+D6（2026-05-06）全部落地**：domain types + 10 sentinels + 内置 6 marketplace（Playwright/MarkItDown/Context7/DuckDuckGo/SQLite/everything）+ ~/.forgify/mcp.json Load/Save/Merge + stdio Client wrapper（stderr→zap+256KB ring / SDK CommandTransport 处理 SIGTERM→5s→SIGKILL）+ Service lifecycle/Search/CallTool/Health/Install + 2 system tools (search_mcp/call_mcp) + 10 HTTP endpoints + 4 离线 pipeline 场景 + 1 Live_ 装 everything 场景门控。

##### Server 配置 / 生命周期

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/v1/mcp-servers` | 列所有配置（含 status + tools + 健康字段）|
| GET | `/api/v1/mcp-servers/{name}` | 单 server 详情 + tools |
| GET | `/api/v1/mcp-servers/{name}/stderr` | 取 server stderr 256KB ring buffer（debug 用）；返 JSON envelope `{name, stderr, size}`（不是 raw text）|
| PUT | `/api/v1/mcp-servers/{name}` | 增/改配置（写 mcp.json + 立即 Connect）。**注**：返 200 + ServerStatus **无论 connect 是否成功**——caller 看 status 字段判断（per mcp.md §10 设计；handler log Error 级让 observability 捞到 connect failure）|
| DELETE | `/api/v1/mcp-servers/{name}` | 删配置 + disconnect（204）|
| POST | `/api/v1/mcp-servers:import` | **拖拽导入**（multipart 上传 mcp.json 文件 / 文本 fragment）|
| POST | `/api/v1/mcp-servers/{name}:reconnect` | 强制重启子进程（degraded / failed 恢复用）|
| POST | `/api/v1/mcp-servers/{name}:health-check` | 主动健康检查（调 tools/list 验证）|

##### Registry — 内置 Marketplace

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/v1/mcp-registry` | 列 curated marketplace 全部条目（V3 / 2026-05-09：~21 条精选，tier asc + name asc 稳排，无 query 参数）|
| GET | `/api/v1/mcp-registry/{name}` | 单 entry 详情（含 RequiredEnv / RequiredArgs）|
| POST | `/api/v1/mcp-registry/{name}:install` | 安装：填 env + args → 写 mcp.json + Connect；返 **201** Created + 新 ServerStatus；body `{env: {...}, args: {...}}`，空 body OK（entry 可无 RequiredEnv/Args）|

**没有 `:enable` / `:disable`**——配置在 mcp.json 即启用，删除即禁用，无中间态。

#### skill ✅
详见 [`../service-design-documents/skill.md`](../service-design-documents/skill.md)。`SKILL.md` 跨厂兼容（Anthropic spec）；progressive disclosure 三层加载；`context: fork` 可组合到 subagent；自包含（仅 `~/.forgify/skills/`，无项目级）。**V1.2 D7（2026-05-06）全部交付**：domain types + 5 sentinels + agentstate ActiveSkill 旁路 + Service{Scan/Get/List/Search/Activate/Body/Create/Replace/Delete/Import} + fsnotify watcher（debounce 500ms + symlink loop guard + Linux fd-limit fail-soft + 5min poll backstop）+ 2 system tools (search_skills/activate_skill) + framework permission integration（活动 skill 的 allowed-tools 在 loop dispatch 短路 CheckPermissions）+ 9 HTTP endpoints + 3 离线 pipeline 场景（Activate inline / Search→Activate / Bash 预授权端到端）。

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
| POST | `/api/v1/skills/{name}:invoke` | 手动调用（slash command 路径用）；body `{arguments: string[]}`（位置参数），返 200 `{result: out}` |

#### catalog ✅
详见 [`../service-design-documents/catalog.md`](../service-design-documents/catalog.md)。统一能力目录（function + skill + mcp）。LLM-gen summary + 自动跨类目路由观察。**1s polling + atomic.Bool 单 flight + fingerprint dedup**。**不发 SSE**（内部组件）。**V1.2 D8（2026-05-06）全部交付**：domain types + 2 sentinels + Service + LLMGenerator（3-attempt retry + coverage 校验 + mechanical fallback）+ atomic disk cache + 3 CatalogSource（function/skill/mcp）+ chat runner SystemPromptProvider 注入 + 2 HTTP endpoints + 3 离线 pipeline 场景。

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/v1/catalog` | 当前 catalog cache 内容（debug / UI 显示）；**未 Refresh 时返 envelope 内 `null`**——UI 需 null-guard |
| POST | `/api/v1/catalog:refresh` | 强制立即 refresh（绕过 1s polling 间隔）|

**没有 routing-hints 端点**——路由提示由 generator LLM-gen 时直接写进 summary，用户想影响路由 → 编辑源头 function/skill/mcp 的 description。

#### sandbox ✅
详见 [`../service-design-documents/sandbox.md`](../service-design-documents/sandbox.md)。统一 PluginSandbox v2（mise embed + per-plugin 隔离 env，4 类 owner：function / mcp / skill / conversation）。Bootstrap 自启 + lazy install runtime。

##### Read 端点

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/v1/sandbox/runtimes` | 列所有已装 runtime（kind/version/path/sizeBytes/isDefault）|
| GET | `/api/v1/sandbox/envs?ownerKind=function\|mcp\|skill\|conversation` | 按 ownerKind 列 envs（**ownerKind 必填**，否则 400 OWNER_KIND_REQUIRED）|
| GET | `/api/v1/sandbox/envs/{id}` | 单 env 详情 |
| GET | `/api/v1/sandbox/disk-usage` | 全 sandbox 磁盘占用 `{totalBytes}` |
| GET | `/api/v1/sandbox/bootstrap-status` | Bootstrap 状态 `{ok, miseBin?, error?}` |
| GET | `/api/v1/conversations/{id}/sandbox-envs` | 列对话作用域所有 conversation-kind env |

##### :action 端点（POST）

| Method | Path | 用途 |
|---|---|---|
| POST | `/api/v1/sandbox/envs/{id}:destroy` | 销毁单 env（rm -rf 目录 + DB 行；in-use 返 409 SANDBOX_ENV_IN_USE）|
| POST | `/api/v1/sandbox/runtimes/{id}:destroy` | 销毁单 runtime（被任何 env 引用时拒）|
| POST | `/api/v1/sandbox/:gc` | 触发 GC（清理孤儿 env / runtime）|
| POST | `/api/v1/sandbox/:retry-bootstrap` | 重试 Bootstrap（mise binary 自检 + 装失败的核心 runtime）|
| POST | `/api/v1/sandbox/runtimes:install` | 显式装 runtime（kind+version；body：`{kind, version}`）|
| POST | `/api/v1/conversations/{id}/sandbox-envs/{kind}:reset` | 重置对话内单 kind 的 conversation env |
| POST | `/api/v1/conversations/{id}/sandbox-envs:reset-all` | 重置对话内全部 conversation env |

> 路由实现注：`POST /sandbox/{action}` 单 mux 入口分派 3 个 action（`:gc` / `:retry-bootstrap` / `runtimes:install`）；`POST /sandbox/envs/{idAction}` / `runtimes/{idAction}` 用 `strings.Cut` 拆 id 与 action。详 `handlers/sandbox.go::Register`。

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

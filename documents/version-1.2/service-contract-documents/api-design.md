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

### 多用户 session（§20）

每个请求需 `X-Forgify-User-ID: <userID>` header 标识当前 profile；缺省回退至 DB 首个 user，再回退 `local-user`。SSE EventSource API 不能自定义 header → 端点 URL append `?userID=<uid>` 兜底。详 [`../service-design-documents/user.md`](../service-design-documents/user.md) §9。

### 多语言（§21）

每个请求附 `Accept-Language: zh-CN | en` header（按 active user.language 自动设）；后端 ctx 经 `InjectLocale` middleware 注入，LLM system prompt 自动按 locale 拼 hint 段。

---

## API 清单

> **状态**：⬜ 未设计 | 🔄 设计中 | ✅ 已实现

### 通用

| Method | Path | 用途 | 状态 |
|---|---|---|---|
| GET | `/api/v1/health` | 存活探针(Electron 启动后读)| ✅ |
| GET | `/api/v1/eventlog` | **per-user** SSE 事件流(D-redo-2;Bridge 按 user_id key,payload 带 `conversationId`,client 按 payload demux);递归事件日志协议(5 events × 6 block types);`Last-Event-ID` 重连;超 buffer 返 410 Gone SEQ_TOO_OLD — 详 [`../event-log-protocol.md`](../event-log-protocol.md) | 🔄 |
| GET | `/api/v1/notifications` | **per-user** SSE 或 REST 快照：无 `Accept: text/event-stream` header 时返 JSON 分页列表（`?cursor=<seq>&limit=<N>`，max 200，每条含 `{seq,type,id,data,conversationId?}`）；有 SSE header 时走 entity 状态总线（Bridge 按 user_id key；`Last-Event-ID` 重连）— 详 [`events-design.md`](./events-design.md) | ✅ |
| GET | `/api/v1/forge` | **per-user** SSE 锻造进度流(D-redo-4);trinity entity(function / handler / workflow)的 create/edit/revert/delete 流式;4 event types: `forge_started` / `forge_op_applied` / `forge_env_attempt` / `forge_completed`;payload 含 `scope:{kind,id}` + `operation` + `conversationId?` + `toolCallId?`;`Last-Event-ID` 重连 | ⬜ |
| GET | `/api/v1/conversations/{id}/eventlog?from=N` | 历史回放:DB 重构 block 事件流(client 收 410 后用此 refetch;返 `{events, tailSeq, count}`)| ✅ |

> **SSE 上限三条**(D-redo-5):eventlog / notifications / forge,**永远不再加**。所有未来"entity 详情面板独立流"需求一律走 forge 流 + client filter,或经 Wails native event 机制(打包阶段实施,绕过 HTTP)。Plan 03 原 Phase 5 TLS + HTTP/2 永久搁置(D-redo-1)。

---

### Phase 2：基础对话能力

#### apikey ✅
详见 [`../service-design-documents/apikey.md`](../service-design-documents/apikey.md) §10。

| Method | Path | 用途 |
|---|---|---|
| POST | `/api/v1/api-keys` | 创建 |
| GET | `/api/v1/api-keys` | 列表（分页 + `?provider=` 过滤）|
| PATCH | `/api/v1/api-keys/{id}` | 更新 displayName / baseUrl / key（旋转）/ isDefault（单选默认，同 category 其他 key 自动取消）|
| DELETE | `/api/v1/api-keys/{id}` | 软删；**2026-05-28 redesign：被 model_configs / conv.modelOverride / node.modelOverride 引用时拒删，返 422 `API_KEY_IN_USE`** |
| POST | `/api/v1/api-keys/{id}:test` | 连通性测试 |
| GET | `/api/v1/providers` | 列 ProviderMeta 注册表（`?category=llm` 或 `?category=search` 过滤）；前端用以替代客户端硬编码 provider 列表（屎山拯救计划 #4 收尾）|

#### model ✅
详见 [`../service-design-documents/model.md`](../service-design-documents/model.md)。用户给每个 scenario 选定 `(apiKeyId, modelID)`（provider 由 apiKey 隐含）；2026-05-28 redesign 后 3 个 scenario：`dialogue` / `utility` / `agent`。

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/v1/model-configs` | 列出当前用户所有 scenario 的配置（不分页，最多 ~6 条）；row shape 含 `apiKeyId`（2026-05-28 redesign：原 `provider` 字段已删）|
| PUT | `/api/v1/model-configs/{scenario}` | upsert 指定 scenario 的配置；body `{apiKeyId, modelId, thinking?}`（200，无论创建或更新）；F1 校验 `apiKeyId` 存在 + 跨用户隔离（404 `API_KEY_NOT_FOUND`）；**2026-05-30：body 新增可选 `thinking` 字段（`ThinkingSpec`，nil=未设定）** |
| GET | `/api/v1/scenarios` | 列 scenario 白名单（静态 metadata，3 项 `dialogue` / `utility` / `agent`；onboarding 前可读，不需 user header）|
| GET | `/api/v1/model-capabilities` | 当前用户已配置 provider/model 的 resolved capability 列表（静态规则 ⊕ 用户 override）；供前端 ThinkingControl 渲染，详 [`../service-design-documents/model.md`](../service-design-documents/model.md) §10.4（**2026-05-30 新增**）|
| PUT | `/api/v1/model-capabilities` | 设置用户 per-model capability override；body `{provider, modelId, thinkingShape?, contextWindow?, maxOutput?}`；400 `INVALID_THINKING_SHAPE`（handler 内联，不进 errmap）（**2026-05-30 新增**）|
| DELETE | `/api/v1/model-capabilities` | 清除用户对 `?provider=xxx&modelId=yyy` 的 override，回退静态规则（**2026-05-30 新增**）|

#### conversation ✅
详见 [`../service-design-documents/conversation.md`](../service-design-documents/conversation.md)。对话线程容器的 CRUD；消息历史由 chat domain 管理。

| Method | Path | 用途 |
|---|---|---|
| POST | `/api/v1/conversations` | 创建对话（201）；title 可为空 |
| GET | `/api/v1/conversations` | 列表（200，cursor 分页，ORDER BY `pinned DESC, created_at DESC, id DESC`）；query `?search=` title LIKE、`?archived=true/false` 过滤归档（缺省排除已归档，§17.12）|
| GET | `/api/v1/conversations/{id}` | 单对话详情（200，含 systemPrompt / autoTitled / archived / pinned / metadata）|
| PATCH | `/api/v1/conversations/{id}` | partial update（200）；body `{title?, systemPrompt?, attachedDocuments?, archived?, pinned?, modelOverride?}`——六个字段可任意组合改；归档/置顶/override 切换发 slim notif `action: archived/unarchived/pinned/unpinned/model_override`（§17.12 + §15.6 + §12.3）。**modelOverride 形状（2026-05-28 redesign）**：三态 absent=不变 / `null`=清除 / `{apiKeyId, modelId}`=设置；F1 校验 apiKeyId 存在 + 跨用户隔离 → 404 `API_KEY_NOT_FOUND`；缺字段 → 400 `API_KEY_ID_REQUIRED` / `MODEL_ID_REQUIRED`。conv override 自动经 ctx 传播到 subagent spawn（subagent 跑同一 (apiKeyId, modelId)）|
| DELETE | `/api/v1/conversations/{id}` | 软删（204）|

#### chat ✅
详见 [`../service-design-documents/chat.md`](../service-design-documents/chat.md)。自有 `infra/llm` 驱动（Eino 已移除），Block 模型，Phase 2 tools=nil（纯流式对话），Phase 3+ 注入 System Tools。

| Method | Path | 用途 |
|---|---|---|
| POST | `/api/v1/attachments` | 上传附件（multipart，50MB 限制）→ 201 返回 attachment_id |
| POST | `/api/v1/conversations/{id}/messages` | 发送消息（202，队列化异步 Agent 运行）；body 含 `attachmentIds[]` + `mentions[]`（`{type,id}`，@ 引用实体内容快照进消息，type ∈ document/function/handler/workflow；详见 service-design-documents/mention.md）|
| DELETE | `/api/v1/conversations/{id}/stream` | 取消正在运行的 Agent（204）；404 STREAM_NOT_FOUND |
| GET | `/api/v1/conversations/{id}/messages` | 消息历史（cursor 分页，ASC 时序）；每条消息含 `blocks[]`（**7 类型**：text/reasoning/tool_call/tool_result/progress/message/compaction）+ `attrs`（user msg 含 `attachments[]` 引用、subagent sub-message 含 `kind=subagent_run`）+ `inputTokens` + `outputTokens`。**注**：附件不是 block 类型，是 `attrs.attachments[]` 引用 `attachments` 表 |

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
| POST | `/api/v1/functions/{id}:run` | 执行(body `{args, version?}`);取消走 caller ctx,无 per-call timeout knob || POST | `/api/v1/functions/{id}:revert` | 回滚到指定 accepted 版本号 |
| GET | `/api/v1/functions/{id}/versions` | 版本分页(?status= filter) |
| GET | `/api/v1/functions/{id}/versions/{v}` | 单版本(integer→ByNumber, fnv_*→ById) |
| GET | `/api/v1/functions/{id}/pending` | 当前 pending(无则 404 FUNCTION_PENDING_NOT_FOUND) |
| POST | `/api/v1/functions/{id}/pending:accept` | accept → 新 accepted 版本 + 翻 ActiveVersionID;应用 AcceptedVersionCap=50 |
| POST | `/api/v1/functions/{id}/pending:reject` | reject(状态 → rejected,不动 ActiveVersion) |
| GET | `/api/v1/functions/{id}/executions` | 执行日志列表 ✅ D22(? versionId/status/conversationId/flowrunId &cursor &limit);返 previews + aggregates |
| GET | `/api/v1/function-executions/{execId}` | 全局执行详情 ✅ D22;返完整行 + machine-computed hints(outputEmpty / significantlySlower) |

> forge_redesign Plan 01(2026-05-11):整套 forge 代码路径在 Phase 7 删除,trinity domain function 替代。POST 走扁平 definition(curl/UI/script 友好),LLM 走 ops 增量编辑(create_function / edit_function 工具单 op emit 1 progress delta)。env sync 是 caller-owns lifetime(D3):创建/edit/accept 后 SyncEnvForVersion 后台起 goroutine,UI 经 GET /functions/{id} 看 envStatus 翻 ready/failed。D22:每次 RunFunction 终态写 1 行 function_executions(detached ctx §S9);9 LLM tools 含 search_function_executions / get_function_execution 让 LLM 自诊断。

#### handler ✅ (forge_redesign Plan 02)
详见 [`../service-design-documents/handler.md`](../service-design-documents/handler.md) §9 + redesign topic [`../adhoc-topic-documents/forge_redesign/03-handler.md`](../adhoc-topic-documents/forge_redesign/03-handler.md)。

| Method | Path | 用途 |
|---|---|---|
| POST   | `/api/v1/handlers`                              | 创建(扁平 definition)|
| GET    | `/api/v1/handlers`                              | 列表(分页) |
| GET    | `/api/v1/handlers/{id}`                         | 详情(含 pending + env + configState + liveInstances) |
| PATCH  | `/api/v1/handlers/{id}`                         | 改 meta(name/desc/tags;不改 code 走 LLM ops 流) |
| DELETE | `/api/v1/handlers/{id}`                         | 软删 + 级联销毁所有 owner 持有的 instance |
| POST   | `/api/v1/handlers/{id}:call`                    | 调用 method(per-call lifetime;返 `{result}`)|
| POST   | `/api/v1/handlers/{id}:revert`                  | 回滚到 accepted 版本号 |
| GET    | `/api/v1/handlers/{id}/versions`                | 版本分页 |
| GET    | `/api/v1/handlers/{id}/versions/{v}`            | 单版本 |
| GET    | `/api/v1/handlers/{id}/pending`                 | 当前 pending |
| POST   | `/api/v1/handlers/{id}/pending:accept`          | accept pending |
| POST   | `/api/v1/handlers/{id}/pending:reject`          | reject pending |
| GET    | `/api/v1/handlers/{id}/config`                  | masked config + configState |
| POST   | `/api/v1/handlers/{id}/config`                  | merge patch 写 config |
| DELETE | `/api/v1/handlers/{id}/config`                  | 清回 unconfigured |
| GET    | `/api/v1/handlers/{id}/calls`                   | 调用日志列表(D22)|
| GET    | `/api/v1/handler-calls/{callId}`                | 全局调用详情 + hints(D22)|

> forge_redesign Plan 02(2026-05-12):Handler 是 trinity 第二条腿 — 有状态 Python class。**Caller-owns lifetime**(D3 + 2026-05-12 用户细化):chat = per-call(spawn-method-destroy 一气呵成),workflow/test/session = persistent via instanceRegistry。无 idle GC。**ConfigState**(D-handler):per-Definition 整个 init_args JSON 经 AES-GCM 加密存 `handlers.config_encrypted`;sensitive 字段在 GET/list/LLM 工具结果 mask。stdio JSON-line RPC client 跟 Python subprocess 沟通,自己写一份(不复用 MCP)。D22:每次 Service.Call 终态写 1 行 handler_calls。

#### workflow ✅ (forge_redesign Plan 04)
详见 [`../service-design-documents/workflow.md`](../service-design-documents/workflow.md) §9 + redesign topic [`../adhoc-topic-documents/forge_redesign/04-workflow.md`](../adhoc-topic-documents/forge_redesign/04-workflow.md)。

| Method | Path | 用途 |
|---|---|---|
| POST   | `/api/v1/workflows`                              | 创建(收 `{ops, changeReason}`;ParseOps + ApplyOps + ValidateGraph + auto-accept v1)|
| GET    | `/api/v1/workflows`                              | 列表(?enabled=true 过滤)|
| GET    | `/api/v1/workflows/{id}`                         | 详情(含 pending + LiveRuns/LastFiredAt/NextFireAt 计算字段 — 后三个 Plan 05 territory,响应形状预留)|
| PATCH  | `/api/v1/workflows/{id}`                         | 改 meta(name/description/tags/enabled/concurrency/needsAttention/attentionReason)|
| DELETE | `/api/v1/workflows/{id}`                         | 软删 |
| POST   | `/api/v1/workflows/{id}:revert`                  | 回滚到 accepted 版本号 |
| POST   | `/api/v1/workflows/{id}:edit`                    | 应用 ops 产/迭代 pending 版本(`{ops, changeReason}`);ParseOps + ApplyOps + ValidateGraph;**ops 支持 `set_node_model_override` (2026-05-28 redesign,详下)**;iterate-same-pending(D-redo-11);拒 `ops=[]` |
| GET    | `/api/v1/workflows/{id}/versions`                | 版本分页(?status= filter)|
| GET    | `/api/v1/workflows/{id}/versions/{v}`            | 单版本(integer→ByNumber, wfv_*→ById)|
| GET    | `/api/v1/workflows/{id}/pending`                 | 当前 pending(无返 404 WORKFLOW_PENDING_NOT_FOUND)|
| POST   | `/api/v1/workflows/{id}/pending:accept`          | accept(纯指针翻转;trim 到 AcceptedVersionCap=50)|
| POST   | `/api/v1/workflows/{id}/pending:reject`          | reject(HardDeleteVersion pending 行,D-redo-12)|

> forge_redesign Plan 04(2026-05-12):Workflow 是 trinity 第三条腿 — **用户命名的有向无环图(DAG)**。锻造 vs 执行分离(D6):本端点集只管"图怎么样",不管"图怎么跑"(`:trigger` action + flowrun endpoints + execution log endpoints 在 Plan 05)。Edit 走 iterate-same-pending(D-redo-11);拒绝 `ops=[]`(workflow 无 env 要"force-rebuild")。CapabilityChecker 真接 function/handler/skill/mcp 服务,validation 期返 `WORKFLOW_CAPABILITY_NOT_FOUND` / `WORKFLOW_MCP_SERVER_NOT_INSTALLED`。

> **2026-05-28 model selection redesign**:`:edit` ops 联合新增第 10 个 `set_node_model_override` op,payload `{nodeId, modelOverride:{apiKeyId, modelId}?}`(modelOverride 字段缺失或 null = 清除)。F1 校验:任一 `apiKeyId`/`modelId` 缺失 → 400 `INVALID_NODE_MODEL_OVERRIDE`;`apiKeyId` 不存在 / 跨用户 → 404 `API_KEY_NOT_FOUND`。详 [`../service-design-documents/workflow.md`](../service-design-documents/workflow.md) §5。

#### flowrun + trigger + scheduler ✅ (forge_redesign Plan 05)
详见 [`../service-design-documents/{flowrun,trigger,scheduler}.md`](../service-design-documents/) + redesign topic [`../adhoc-topic-documents/forge_redesign/05-execution-plane.md`](../adhoc-topic-documents/forge_redesign/05-execution-plane.md)。

| Method | Path | 用途 |
|---|---|---|
| POST   | `/api/v1/workflows/{id}:trigger`                       | 手动触发 (转 scheduler.StartRun;disabled → 422);query `?dryRun=true` 走 preview 模式,side-effect 节点(function/handler/mcp/skill/llm/agent/http/approval/wait)返 mock outputs(§19 dry-run)|
| GET    | `/api/v1/workflows/{id}/triggers`                      | trigger 状态 §6.12 (含 cron `nextFireAt`)|
| GET    | `/api/v1/flowruns`                                     | 列表 (?workflowId / ?status / ?triggerKind)|
| GET    | `/api/v1/flowruns/{id}`                                | 单 FlowRun |
| GET    | `/api/v1/flowruns/{id}/nodes`                          | per-node 执行记录分页 |
| DELETE | `/api/v1/flowruns/{id}`                                | 取消 (scheduler.Cancel;in-flight 204 或 已终态 422)|
| POST   | `/api/v1/flowruns/{id}/approvals/{nodeId}`             | approval 签收 (body `{decision, reason?}`)|
| POST   | `/api/v1/webhooks/{wfId}/{path}`                       | webhook trigger 入口 (trigger.webhook listener 直接挂 ServeMux,secret 校验) |

> Plan 05(2026-05-13):执行 plane 三 domain — flowrun(记录簿)/ trigger(4 种 listener:cron/fsnotify/webhook/manual)/ scheduler(编排器 + 13 dispatcher + retry/timeout/onError + pause/resume)。`:trigger` action + `/triggers` state 由 WorkflowHandler 持(共享 `:revert` 的 `{idAction}` mux dispatcher),委派 FlowRunHandler 薄 helper。webhook 端点由 trigger.webhook listener 直接挂主 ServeMux 子路径,跟主 router 共享。14 hardening item 全覆盖(详 spec §6 + scheduler.md §11)。

#### chat（Phase 3 升级）✅
Function + Handler + Workflow System Tools 注入(9 function + 10 handler + 6 workflow,共 25 个 trinity tool)。SSE 见 events-design.md。无新 HTTP 端点,见 Phase 2 chat 端点。

> Phase 3 后优化轮（2026-05-02）删除了原 Phase 3 装的 8 个通用 system tool（read_file/write_file/list_dir/run_shell/run_python/datetime/web_search/fetch_url）。新一代 system tools（Read/Write/Edit/Bash/Glob/Grep/LS）将在 Phase 5 重建。

---

### Phase 5：System Tool 第二代 + 任务追踪 + 用户问询（2026-05-04）

#### 系统工具家族（注入 chat agent，无新 HTTP 端点）

**Resident 工具（28 把，每轮始终在 `tools` 参数中）**：

| 家族 | 工具 |
|---|---|
| filesystem | `Read` / `Write` / `Edit` / `Grep` / `Glob` |
| shell | `Bash` / `BashOutput` / `KillShell` |
| web | `WebFetch` / `WebSearch` |
| ask | `AskUserQuestion` |
| todo | `TodoCreate` / `TodoList` / `TodoGet` / `TodoUpdate` |
| memory | `read_memory` / `write_memory` / `forget_memory` |
| discovery | `search_function` / `search_handler` / `search_workflow` / `search_skills` / `search_mcp_tools` |
| execution | `run_function` / `call_handler` / `activate_skill` / `Subagent` |
| meta | `activate_tools` |

**`activate_tools` 工具契约**（RESIDENT，IsReadOnly=true）：
- 参数：`{ category: enum["function","handler","workflow","mcp","document","skill"] }` （必填）
- Execute：把 category 写入 `AgentState.ActivatedGroups`；返回该组工具名清单字符串，格式 `"Activated <category>: <name>, ..."`。
- 效果：step N 调 `activate_tools("function")` → step N+1 起 `host.Tools(ctx)` 含 function 组（`create_function` / `edit_function` / `delete_function` / `revert_function` / `get_function` / `get_function_execution` / `search_function_executions`）。

**Lazy 工具组（6 组，按需 `activate_tools(category)` 激活）**：function / handler / workflow / mcp / document / skill。详细成员见 [`capability-disclosure-design.md` §4.3](../capability-disclosure-design.md)。

**实测常驻 context**：system ~2.9k bytes + 28 常驻 tools slim schemas ~17.5k bytes ≈ 20.4k bytes ≈ **5.1k token**（vs 重构前 ~28k token）。

| 家族（Phase 5 基础）| 工具 | 说明 |
|---|---|---|
| filesystem | `Read` / `Write` / `Edit` | PathGuard 守敏感路径；Edit 走 must-Read-first 守卫 + 原子写 |
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

#### memory（V1.2 §2 final-sweep）✅

跨对话长期事实 CRUD + pin/unpin。详见 [`../service-design-documents/memory.md`](../service-design-documents/memory.md)。

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/v1/memories?type=&pinned=` | 列出活跃 memory；可选按 type / pinned 过滤 → 200 |
| POST | `/api/v1/memories` | 创建（source=user 服务端硬定）→ 201；name 冲突返 409 MEMORY_NAME_CONFLICT |
| GET | `/api/v1/memories/{name}` | 按 name 取（同时 bump access stats）→ 200；缺失 404 MEMORY_NOT_FOUND |
| PATCH | `/api/v1/memories/{name}` | 部分更新；source 永不变（保留原作者身份）→ 200 |
| DELETE | `/api/v1/memories/{name}` | 软删 → 204 |
| POST | `/api/v1/memories/{name}:pin` | 标 pinned=true → 200 |
| POST | `/api/v1/memories/{name}:unpin` | 标 pinned=false → 200 |

3 个 system tool（`read_memory` / `write_memory` / `forget_memory`）无独立 HTTP 端点；write_memory 内部走 Upsert 语义（不报 NAME_CONFLICT），source 硬写为 `ai`。Pin 字段对 LLM 不可见——pinning 是用户控件（pinned 全文进每次 system prompt，只用户能决定该不该）。

#### document (Phase 5 §14) ✅ §14.2

Notion-style 树状文档库 CRUD + move。详见 [`../service-design-documents/document.md`](../service-design-documents/document.md)。

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/v1/documents?parentId=` | 列指定层(参数空 = root),轻字段不含 content → 200 |
| GET | `/api/v1/documents/tree` | 整树 metadata(含 path,不含 content),侧边栏一次拉满 → 200 |
| POST | `/api/v1/documents` | 创建(可指定 parentId);body `{name, parentId?, content?, description?, tags?}` → 201 |
| GET | `/api/v1/documents/{id}` | 详情含 content → 200 / 404 |
| PATCH | `/api/v1/documents/{id}` | 部分更新 name / content / description / tags(改 name 触发整子树 path 级联)→ 200 / 404 / 422 |
| DELETE | `/api/v1/documents/{id}` | 软删整子树;返 `{id, deletedCount}`(给 testend 显示"X 个一起删") → 200 / 404 |
| POST | `/api/v1/documents/{id}:move` | 改 parentId + position(防成环 + 整子树 path 级联);body `{parentId?, position?}` → 200 / 404 / 422 |

未来 7 个 system tool(`search_documents` / `list_documents` / `read_document` / `create_document` / `edit_document` / `move_document` / `delete_document`)无独立 HTTP 端点(§14.3);workflow LLM 节点 + Conversation 挂载在 §14.5。

#### compaction（V1.2 §1 final-sweep）

**无新 HTTP 端点**。压缩由 `app/contextmgr.Manager` 在每轮 AI turn 完成后同步触发（chat runner 内部）。状态可通过：
- `GET /api/v1/conversations/{id}` 看 `summary` + `summaryCoversUpToSeq` 字段
- `GET /api/v1/conversations/{id}/messages` 看 blocks 的 `contextRole`（`hot`/`warm`/`cold`/`archived`）+ type=`compaction` 行（attrs 含 `coversFromSeq`/`coversToSeq`/`blocksArchived`）
- testend `/current/compaction` 面板可视化展示

未来扩展：`POST /api/v1/conversations/{id}:compact` 手动强制（Manager.ForceCompact 已实现，端点未接）。

#### usage（V1.2 §4 final-sweep）✅

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/v1/conversations/{id}` | 响应附 `tokensUsed: {input, output, total}` 聚合（§4.1）|
| GET | `/api/v1/usage?conversationId={id}` | per-conv totals（无 cost）|
| GET | `/api/v1/usage?period=day\|week\|month\|all` | per-period 聚合 + cost 估算按 (provider, modelId) 拆 |

`/api/v1/usage` 响应 shape：`{scope, conversationId?, period?:{since,until}, inputTokens, outputTokens, totalTokens, costEstimateUsd, byModel: [{provider, modelId, inputTokens, outputTokens, totalTokens, costEstimateUsd, costKnown}], note}`。cost 基于 `pkg/llmcost` 静态 16-model registry（DeepSeek / Anthropic / OpenAI），未知模型 `costKnown=false`、cost=0；note 字段提示 "rough ballpark"。

#### permissions + settings（V1.2 §3 final-sweep）✅

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/v1/settings` | 当前 `~/.forgify/settings.json` 解析后快照 |
| PUT | `/api/v1/settings` | 替换整个 settings.json（atomic tmp+rename + reload）|
| POST | `/api/v1/settings:reload` | 强制从磁盘重读（不依赖 fsnotify watcher）|
| GET | `/api/v1/settings/limits` | 当前运行上限（settings.json `limits` 块叠加高 ceiling 默认）|
| PUT | `/api/v1/settings/limits` | upsert `limits` 块（read-modify-write 保 permissions/hooks）+ reload，返新 limits |
| GET | `/api/v1/permissions/tools` | 列所有已注册 tool + dangerLevel(`read_only`/`workspace_write`/`danger_full_access`) |
| POST | `/api/v1/permissions/test` | body `{toolName, args, destructive?}` → 返 `{action, reason}` 预测当前规则下结果，无副作用 |

settings.json 顶层 schema：`{permissions:{defaultMode:ask\|allow\|deny\|bypass, deny:[], ask:[], allow:[]}, hooks:{PreToolUse:[], PostToolUse:[], Stop:[]}, protectedPaths:{denyWrite:[]}}`。规则形态 `"Verb(pattern)"`（如 `"Bash(rm -rf *)"`、`"Edit(./src/**)"`、`"WebFetch(domain:github.com)"`）；详 [`../service-design-documents/permissions.md`](../service-design-documents/permissions.md) §5.1。求值：deny→ask→allow→defaultMode 第一匹配赢；session ask-once 缓存让用户答过的同 (tool, args) 不再问。Hook 形态当前仅 shell exec（stdin/stdout JSON 协议），exit 0/2/其他 三态语义；详 §6。

settings.json 另含可选 `limits` 块（运行上限：agent 步数 / 输出 token / 超时 / 工具结果 / workflow agent 上限）——默认 = 高 ceiling，缺失键保默认（`json.Unmarshal` 叠加在 `limits.Default()` 上），热重载；经 `GET/PUT /api/v1/settings/limits` 或前端「高级能力」设置区编辑；后端经 `internal/pkg/limits.Current()` 单点读取。详 [`../adhoc-topic-documents/limits-optimization/`](../adhoc-topic-documents/limits-optimization/)。

#### user（V1.2 §20 multi-user）✅
详见 [`../service-design-documents/user.md`](../service-design-documents/user.md)。本地多 profile（无 auth、无密码）；DB 自动 user_id scope；前端 X-Forgify-User-ID header 注入。

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/v1/users` | 列表（无分页，量小）|
| POST | `/api/v1/users` | 创建（body `{username, displayName?, avatarColor?, language?}`；username 1-32 [a-z0-9_-] 自动 lowercase）→ 201 |
| GET | `/api/v1/users/{id}` | 单查 → 200 / 404 |
| PATCH | `/api/v1/users/{id}` | partial update（`displayName?` / `avatarColor?` / `language?`）→ 200 |
| DELETE | `/api/v1/users/{id}` | 软删（拒最后一个 → 422 CANNOT_DELETE_LAST_USER）→ 204 |
| POST | `/api/v1/users/{id}:activate` | touch last_used_at + 返 User → 200 |

#### dev prompts inventory（V1.2 §18 prompt governance）✅
详见 [`../service-design-documents/../prompt-principles.md`](../prompt-principles.md)。dev-only。

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/v1/dev/prompts` | 41 条 prompt 总览：33 tool descriptions + 2 chat-system 静态段 + 3 internal-llm + 3 subagent；每条 `{name, category, content, length, tokensEst, source}` → 200 |
| GET | `/api/v1/conversations/{id}/system-prompt-preview` | 当前 conv 实际拼装的 system prompt + section 拆解 → 200 |

#### dev infra 端点（`--dev` 模式，`/dev/` 前缀）

仅 `--dev` 模式注册，不走 errmap，直接返回 JSON / SSE。**注意**：`/dev/*` 与 `/api/v1/dev/*` 是两个不同前缀。

| Method | Path | 用途 |
|---|---|---|
| GET | `/dev/logs` | 后端日志 SSE 流（ring buffer 回放 + 持续推送） |
| POST | `/dev/sql` | 只读 SQL 查询（`SELECT` 前缀强制） |
| GET | `/dev/info` | 实例信息（build / port / git / uptime） |
| GET | `/dev/runtime` | Go 运行时指标（goroutines / heap / GC） |
| GET | `/dev/forgify-home` | `~/.forgify/` 目录快照 |
| GET | `/dev/bash-processes` | 活跃 bash shell 进程列表 |
| GET | `/dev/mock-llm` | mock LLM 状态 + 请求历史 |
| GET | `/dev/llm/trace` | LLM 调用 trace（请求/响应 payload） |
| GET | `/dev/routes` | **reflection-based** via `router.Recorder`；自动收录 HandleFunc/Handle；drift-proof（2026-05-27 根治） |

**V3 清理记录（2026-05-27）**：删除 `/dev/collections`（YAML 集合列举）、`/dev/tools`（system tool 列表）、`/dev/invoke`（直接调 tool），同步删除 `--collections-dir` flag 和 `Deps.Tools` 字段；`--integration-dir` 重命名为 `--testend-dir`。

详见 [`../adhoc-topic-documents/testend/testend-design.md`](../adhoc-topic-documents/testend/testend-design.md)。

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
详见 [`../service-design-documents/catalog.md`](../service-design-documents/catalog.md)。统一能力目录（function + handler + skill + mcp）。**懒生成 + mechanical**：开聊时按需现查四源拼结构化清单注入 system prompt，无轮询 / 无 LLM / 无缓存 / 无磁盘。**不发 SSE**（内部组件）。**2026-05-25 重构**：移除 1s 轮询 / LLM Generator / 磁盘 cache / version history；document 移出 catalog（走 @-mention，独立功能）。

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/v1/catalog` | 按需现查并返当前用户能力清单（summary + coverage，巡检用）；全源失败 503 `CATALOG_ALL_SOURCES_FAILED` |

（移除 `POST /catalog:refresh` / `GET /catalog/history` / `GET /catalog/diff`——懒生成下 refresh 等价 get，无版本故无 history/diff。）

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

#### relation ✅（2026-05-19）

跨实体关系图（relgraph 数据底座）。**只读**——无 POST/PATCH/DELETE 端点；边由 source domain hook 隐式维护。

| Endpoint | 用途 |
|---|---|
| `GET /api/v1/relations?fromKind&fromId&toKind&toId&kind&cursor&limit` | 按任意组合过滤的分页查询。limit 默认 200，最大 500 |
| `GET /api/v1/relations/neighborhood?kind&id&depth=1-3` | 中心实体 N 跳邻域 BFS；返所有边 |
| `GET /api/v1/relgraph` | 洞察 tab 全图快照（`{nodes, edges}`）；conversation 实体只在有边时入图，其他实体类型含孤儿 |

详 [`../service-design-documents/relation.md`](../service-design-documents/relation.md)。

#### askai + capability check + mcp health ✅（2026-05-19）

V1.2 §17 — 前端洞察 / 编辑器 / mcp 屏需要的最后一批端点。

| Endpoint | 用途 |
|---|---|
| `POST /api/v1/workflows/{id}:capability-check` | 跑 ValidateGraph 返报告（OK + issues 列表），不拒绝；前端编辑器试跑前预检 |
| `POST /api/v1/functions/{id}:iterate` | AI 改 function：起内部对话 + system prompt（含当前 code）+ 用户提示，返 conversationId；前端订阅 eventlog + forge stream 看 AI 推理 + pending 落地 |
| `POST /api/v1/handlers/{id}:iterate` | 同上，针对 handler |
| `POST /api/v1/workflows/{id}:iterate` | 同上，针对 workflow |
| `POST /api/v1/documents/{id}:iterate` | 同上，针对 document |
| `POST /api/v1/flowruns/{id}:triage` | AI 失败排查：flowrun 全状态 + workflow graph + 可选 user hint 做 system prompt，LLM 分析失败 → 可能调 edit_forge → pending 让用户 review |
| `GET /api/v1/mcp-servers/{name}/health-history?sinceMinutes=N` | 返时间窗内的 health 快照；HealthCheck 每次调用自动写一行 |
| `POST /api/v1/mcp-servers/{name}/tools/{toolName}:invoke` | 直接调 MCP 工具（绕 chat/LLM）；mcp 详情页"试调用"按钮用 |

详 `service-design-documents/{workflow,function,handler,document,flowrun,mcp}.md`。askai 共享编排见 `app/askai/`。

---

**覆盖矩阵自动生成**：每个 endpoint 在 `backend/test/README.md` 的覆盖矩阵段中按 axis 列出哪些 pipeline 测试覆盖；矩阵由 `make matrix`（消费测试 `// covers:` annotation）维护。新增 endpoint 需补 `api/<domain>/<domain>_pipeline_test.go` 测试 + `// covers: METHOD /path` 注释（§S14 触发表已强制）。

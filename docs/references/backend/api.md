---
id: DOC-008
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-14
review-due: 2026-09-14
audience: [human, ai]
---

# HTTP API —— 端点登记

> 全部端点的单一事实源（method · path · 语义一行）。
> 通则（N 系列）：统一 Envelope `{"data":...}` / `{"error":{code,message,details}}`；线缆 camelCase；List 全部 `?cursor&limit` 分页；非 CRUD 动作 `:action`；执行动词 `:run`(fn) `:call`(hd) `:invoke`(ag) `:trigger`(wf)；`:iterate` = 开 AI 编辑对话（全实体共享 aispawn）。
> **响应形状铁律**：`data` 内层一律**裸实体**——`POST`(Create) / `GET` 单读 / `PATCH` 同形,前端一套解构到底;**绝不**裹 `{"<entity>": ..., "version": ...}` 外层 key。版本实体(function/handler/agent/workflow/control/approval)的当前版本经实体内嵌 `activeVersion` 字段透出(Create 即附新版本,与 GET 单读完全同形)。复合读(一次返多个并列实体,如 `GET /flowruns/{id}` → `{flowrun, nodes, nextCursor}`,nodes 为 N4 keyset 一页)才用具名多 key。
> **异步动作返 id 铁律**：返回新建资源 id 的异步动作(`POST /{id}:trigger`→flowrun、chat `POST /{id}/messages`→message、`:iterate`/`:triage`→conversation、`:fire`→activation)一律 `202 {data:{"id": <newId>}}`——前端一条规则取新资源 id。**同步执行**(`:run`/`:invoke`/`:call`,阻塞返完整结果)不在此列、返**裸结果**(不裹 `{result}`/`{output}`)。
> **状态变更动作铁律**：改实体状态的动作(`:stage`/`:kill`/`:activate`/`:deactivate`/`:restart`/`:edit`/`:revert`)一律返**动作后实体完整快照**(`{data:<entity>}`),不发 `{staged:true}`/`{killed:N}` 等临时裸键(附加计数等并入实体字段或由相关列表端点查)。**无新产物的变更**(resolve-interaction、search `:reindex`、DELETE)一律 `204 No Content`,绝不返 `{data:null}`。

## function（`/api/v1/functions`）

| Method · Path | 语义 |
|---|---|
| `POST /functions` | 创建（扁平 payload → 反推 ops 走构建管线），201 |
| `GET /functions` | 分页列表 |
| `GET /functions/{id}` | 单读（附 activeVersion：代码+env 状态一趟拿全） |
| `PATCH /functions/{id}` | 改 meta（name/description/tags，不升版本） |
| `DELETE /functions/{id}` | 软删 + 销毁 env + 清边，204 |
| `POST /functions/{id}:run` | 执行（TriggeredBy=manual），body `{args, version?}` |
| `POST /functions/{id}:revert` | active 指针移到指定版本号 |
| `POST /functions/{id}:edit` | ops 构建新版本（空 ops = 仅重建 env） |
| `POST /functions/{id}:iterate` | 开 AI 编辑对话，返 `conversationId` |
| `GET /functions/{id}/versions` | 版本分页 |
| `GET /functions/{id}/versions/{version}` | 单版本（接受版本号或 fnv_ id） |
| `GET /functions/{id}/executions` | 执行日志分页（`?status&triggeredBy&conversationId&flowrunId`）；返 `{data:{executions, aggregates}, nextCursor, hasMore}`——分页坐标顶层、聚合在 data 子对象(与 handler/agent/mcp 执行·调用日志同形) |
| `GET /function-executions/{id}` | 单执行详情（含 `logs`——print/调试输出；列表端点不带） |

## handler（`/api/v1/handlers`）

| Method · Path | 语义 |
|---|---|
| `POST /handlers` | 创建（扁平 → ops），201；**不 spawn 实例**（等 config 配齐/Boot/首调） |
| `GET /handlers` | 分页列表 |
| `GET /handlers/{id}` | 单读（附 activeVersion + configState + missingConfig + runtimeState） |
| `PATCH /handlers/{id}` | 改 meta |
| `DELETE /handlers/{id}` | 停实例 + 软删 + 销毁 env + 清边，204 |
| `POST /handlers/{id}:call` | 调方法（manual），body `{method, args}` |
| `POST /handlers/{id}:restart` | 手动重启常驻实例，返新 runtimeState |
| `POST /handlers/{id}:revert` | 移 active 指针 + 重启实例 |
| `POST /handlers/{id}:edit` | ops 构建新版本 + 重启实例（空 ops = 重建 env + 重启） |
| `POST /handlers/{id}:iterate` | 开 AI 编辑对话 |
| `GET /handlers/{id}/versions` · `GET /handlers/{id}/versions/{version}` | 版本（号或 hdv_ id） |
| `GET /handlers/{id}/config` | 读 config（sensitive 字段掩码 `********`） |
| `PUT /handlers/{id}/config` | JSON Merge Patch 更新（null 删 key）→ 整 blob 重加密 → **重启实例重跑 `__init__`** |
| `DELETE /handlers/{id}/config` | 清空 config + 停实例 |
| `GET /handlers/{id}/calls` | 调用日志分页（`?method&status&triggeredBy&conversationId&flowrunId`）；返 `{data:{calls, aggregates}, nextCursor, hasMore}`(同 function/agent/mcp 同形) |
| `GET /handler-calls/{id}` | 单调用详情（含 `logs`——yield + 调用窗口 stderr；列表端点不带） |

## agent（`/api/v1/agents`）

| Method · Path | 语义 |
|---|---|
| `POST /agents` | 创建（identity + 全量 Config 快照 = v1），201 |
| `GET /agents` | 分页列表 |
| `GET /agents/{id}` | 单读（附 activeVersion） |
| `PATCH /agents/{id}` | 改 meta |
| `DELETE /agents/{id}` | 软删 + 清边，204 |
| `POST /agents/{id}:invoke` | 跑 ReAct loop（manual），body `{input, version?}` |
| `POST /agents/{id}:revert` | 移 active 指针 |
| `POST /agents/{id}:edit` | 全量 Config 替换 → 新版本（**非** ops、非合并） |
| `POST /agents/{id}:iterate` | 开 AI 编辑对话 |
| `GET /agents/{id}/versions` · `/versions/{version}` | 版本分页 · 单版本（接受版本号或 agv_ id） |
| `GET /agents/{id}/mount-health` | 按需预检 active 版本各挂载（fn/hd/mcp）是否仍可解析（被删/离线/坏 ref），返 `{data:{mounts:[{ref,name?,healthy,error?}], allHealthy}}`——与 invoke 同解析路径、不 fail-fast。供 invoke 前红点预警（无 active 版本/无挂载 = 平凡健康） |
| `GET /agents/{id}/executions` | 执行日志分页（同款过滤）；返 `{data:{executions, aggregates}, nextCursor, hasMore}`(同 function/handler/mcp 同形) |
| `GET /agent-executions/{id}` | 单执行详情（含完整 transcript） |

## workflow（`/api/v1/workflows`）

| Method · Path | 语义 |
|---|---|
| `POST /workflows` · `GET /workflows` · `GET /workflows/{id}` · `PATCH /workflows/{id}` · `DELETE /workflows/{id}` | CRUD（PATCH=meta 不升版本）（含 `concurrency`: serial\|skip\|buffer_one\|replace\|allow_all——overlap 政策，下一次 drain 生效） |
| `POST /workflows/{id}:trigger` | 立即跑一次（任何 lifecycle 下可跑），body `{payload?}`（只读 payload），返 flowrun id |
| `POST /workflows/{id}:stage` | 待命恰一次真实触发后自动撤防（已 active → 409） |
| `POST /workflows/{id}:activate` / `:deactivate` | 上线（挂监听+active）/ 优雅下线（摘监听+inactive 或 draining） |
| `POST /workflows/{id}:kill` | 硬停：摘监听 + 取消全部在途 run + inactive，返被杀数 |
| `POST /workflows/{id}:edit` / `:revert` | 图 ops 构建新版本 / 移 active 指针 |
| `POST /workflows/{id}:capability-check` | ref 解析体检（实体在吗/kind 对吗/port·method 在吗）；返 `problems`（阻断）+ `warnings`（建议——含 F156 未声明输出读：读 `producer.field` 而 producer 声明输出不含 field） |
| `POST /workflows/{id}:iterate` | 开 AI 编辑对话 |
| `GET /workflows/{id}/versions[/{version}]` | 版本 |

## flowrun（`/api/v1/flowruns`）

| Method · Path | 语义 |
|---|---|
| `GET /flowruns` | 运行历史分页（`?workflowId&status=running\|completed\|failed\|cancelled`） |
| `POST /flowruns` | 手动起 run（= workflow `:trigger` 的等价入口），body `{workflowId, entryNode?, payload?}`（`entryNode` 消歧多 trigger 图——唯一接受 entryNode 的端点） |
| `GET /flowruns/{id}` | run 头 + **一页节点行**（N4 分页 `?cursor&limit`、最新在前、返 `nextCursor`；长 loop run 数千行不再一次倾倒，F168-M7。完整记忆化全集是解释器内部的、非线缆的） |
| `POST /flowruns/{id}:replay` | 修复失败 run：清 failed 行 + 重走（completed 复用） |
| `GET /flowrun-inbox` | 审批收件箱（= 全部 parked 节点行） |
| `POST /flowruns/{id}/approvals/{node}:decide` | 人工审批决策 `{decision: yes|no, reason?}`（first-wins，输家 422） |

## trigger（`/api/v1/triggers`）

| Method · Path | 语义 |
|---|---|
| `POST /triggers` · `GET /triggers` · `GET /triggers/{id}` · `PATCH /triggers/{id}` · `DELETE /triggers/{id}` | CRUD（PATCH=Edit，热更监听中的 listener）。List/Get 每条带派生 `refCount`/`listening` + **`lastFiredAt`**（最近一次 fire 的时间，nil=从未；行可显示「N 前 fire」，读时从 activation 日志投影） |
| `POST /triggers/{id}:fire` | 手动催一次（扇给当前监听者），202 返 `{data:{id}}`——新产物 activation 的单 id（triggerId 在 URL、fired 被 202 蕴含）；拿 id 直查 activation 闭环 |
| `POST /triggers/{id}:iterate` | 开 AI 编辑对话 |
| `GET /triggers/{id}/activations` · `GET /trigger-activations/{id}` | 活动审计（触没触发都有记录） |
| `GET /triggers/{id}/firings` | firing 收件箱分页（`?status=pending\|started\|skipped\|superseded\|shed`）——「触发了为什么没跑」的处置面 |

## control / approval（`/api/v1/controls` · `/api/v1/approvals`）

两域同构：CRUD + `POST {id}:edit / :revert / :iterate` + `GET {id}/versions[/{version}]`。approval 的运行时决策端点在 flowrun 侧（见上）。

## skill（`/api/v1/skills`，name 即 id）

CRUD（`POST` 严格冲突 / `PUT {name}` 覆盖 / `DELETE {name}`）+ `POST /skills/{name}:activate`（inline 渲染注入 / fork 派 subagent）。

## mcp（`/api/v1/mcp-servers` · `/api/v1/mcp-registry`）

servers（name 即键，workspace 唯一）：`GET /mcp-servers`（实时状态列表）· `PUT /mcp-servers/{name}`（手动装/同名替换：stdio `{command, args, env, runtime?, timeoutSec?}`（runtime 缺省按 command 推断：npx→node、uvx→python…）或 remote `{url, transport?, headers}`；**连接失败仍落盘 `status=failed`+`lastError`**，reconnect 可救）· `GET /mcp-servers/{name}`（状态+tools 缓存）· `DELETE /mcp-servers/{name}`（204）· `POST /mcp-servers/{name}:reconnect`（重置按钮）· `GET /mcp-servers/{name}/stderr`（stdio stderr ring 尾，返 `{name, stderr, size}`）· `POST /mcp-servers/{name}/tools/{tool}:invoke`（`{args}` 直接试调、绕过 chat/LLM，返**裸结果**——与 L17 同步执行铁律一致、不裹 `{result}`）· `POST /mcp-servers:import?overwrite=`（Claude Desktop mcp.json 片段，返 `{imported, skipped}`）。
调用台账：`GET /mcp-servers/{name}/calls`（`?tool&status&triggeredBy&conversationId&flowrunId`；返 `{data:{calls, aggregates:{okCount,failedCount}}, nextCursor, hasMore}`——分页坐标顶层、聚合在 data 子对象，与 handler/function/agent 执行日志同形）+ `GET /mcp-calls/{id}`（含 `logs`——progress 通知 + 失败附 server stderr 尾；列表端点不带）。
市场：`GET /mcp-registry`（curated 全列）· `POST /mcp-registry:install`（`{name, env}`——完整 slug 在 body 因含 `/`，无 per-name 详情端点（列表即全量）；缺必填 env 422 `MCP_ENV_MISSING`、无可跑 package 422 `MCP_NO_RUNNABLE_PACKAGE`）。

## document（`/api/v1/documents`）

CRUD + `POST {id}:move`（防环；nil parent=根）+ `POST {id}:duplicate`（深拷整子树，可选 body `{parentId}`：nil/缺省=落为源的兄弟；新根名自动去重；201 返新根裸实体）+ `POST {id}:iterate`（开 AI 编辑对话）+ `GET /documents?parentId=`（直接子节点；空=根级）+ `GET /documents/tree`（整树 metadata，无 content，侧栏一趟拿全）。全文检索走统一 `/search` 与 `search_documents` 工具，无独立 HTTP 端点。

## conversation / chat（`/api/v1/conversations`）

| Method · Path | 语义 |
|---|---|
| conversation CRUD | `POST` · `GET`(list：`?search&archived&sort`) · `GET/{id}` · `PATCH/{id}`（含 ModelOverride 三态）· `DELETE/{id}`。**`?sort`** = `activity`(默认，置顶优先再 `last_message_at` 降序——最近聊过) \| `created`(置顶优先再创建序)；切换 sort 须重置分页（游标随排序列走、跨 sort 无意义）。List/Get 每条带 `lastMessageAt` + **`isGenerating`**（派生只读：chat 是否有在途回合，供冷启动活动圆点；不入 PATCH） |
| `POST /{id}/messages` | **Send**：落 user 回合 + 开 assistant 回合 + 入队，返 assistant msg id |
| `GET /{id}/messages` | 回合历史 keyset 分页（含 blocks） |
| `POST /{id}:cancel` | **Cancel** 在途生成（动作语法,非删子资源） |
| `GET /{id}/interactions` · `POST /{id}/interactions/{toolCallId}` | 待决人机交互重同步 / 决议（body `{action, answer?}`：action ∈ approve\|approve_always\|deny\|accept\|decline；answer 仅 ask accept 用），204 |
| `GET /{id}/system-prompt-preview` · `GET /{id}/usage` | 调试预览 / token 用量 |
| `GET /{conversationId}/todos` | 对话工作清单 |

## attachment / memory（`/api/v1/...`）

attachment：`POST /attachments`（上传）· `GET /{id}` · `GET /{id}/content` · `DELETE /{id}`。
memory：`GET /memories` · `GET/PUT/DELETE /memories/{name}` · `POST /{name}/pin|unpin`（name 即 id）。

## search（`/api/v1/search`，统一搜索）

| Method · Path | 语义 |
|---|---|
| `GET /search` | 综搜/垂搜同端点：`?q`(必填) `&types`(csv，空=综搜) `&tags`(csv) `&updatedAfter/Before`(RFC3339) `&includeArchived`(默认 true) `&cursor&limit`(默认 20 上限 50,走 ParsePageBounded;非数字/<1 → 400)。返 `{data:{hits, total}, nextCursor, hasMore}`——分页坐标顶层、total 在 data 子对象;hit 含 entityType/entityId/name/snippet(`<mark>`)/anchor/tags/archived/score/matchedChunks/refHint（仅积木六类） |
| `POST /search:reindex` | **就地**重建 ctx workspace 索引，204（fire-and-forget、无可轮询产物；force-reconcile 覆盖每个实体词法行、**不** purge-then-rebuild——词法索引从不清空，故并发 Search 返完整结果而非不全/空且无信号，F168-M8/F175-M2；向量缓存仍 invalidate + 重嵌，词法主检索保持完整；**同一 workspace** 运行中再调 409 `SEARCH_REINDEX_RUNNING`——单飞锁 per-workspace、不阻塞别的 workspace 的 reindex，F175-M3） |
| `GET /search/settings` | 机器级搜索设置 + 引擎实时状态 `{embedder, ollamaBaseUrl, ollamaModel, engine:{status: ready\|downloading\|absent\|error\|off, model, lastError}}`（Ollama 字段恒回显生效值） |
| `PATCH /search/settings` | 修补设置：`{embedder?: builtin\|ollama\|off, ollamaBaseUrl?, ollamaModel?}`（缺省字段不动；Ollama 参数空串重置默认）；非法 embedder 400 `SEARCH_EMBEDDER_INVALID`；改 model 即旧模型向量按 model 列失效、后台重嵌 |

LLM 工具面（非 HTTP）：`search_blocks`（积木面板：六类可接线单元，返 ref 直填 workflow 节点）；8 个 `search_<entity>` 垂搜工具保 schema 换引擎（非空 query 走内容引擎、引擎错误回退原子串路径）。

## P6 支撑域

workspace：CRUD（守最后一个；PATCH 含 `webFetchMode`: local|jina）+ `PUT/DELETE {id}/default-models/{scenario}`（dialogue|utility|agent 三场景模型；DELETE 清该场景默认回未配；**写时校 apiKeyId 存在性**——引用不存在的 key 即 404 `API_KEY_NOT_FOUND`，非只 invoke 时失败，与「删被引用 key 挡 `API_KEY_IN_USE`」对称，F153；modelId 拼写不校、留 invoke 时 fail-loud）+ `PUT/DELETE {id}/default-search`（搜索 key）+ `POST {id}:activate`（刷 lastUsedAt）。apikey：CRUD（受管 provider 行 PATCH 返 `API_KEY_IMMUTABLE`）+ `:test`（probe）+ `GET /providers`（provider 白名单列表，每项带 `managed` 标记——内置免费档 `anselm` 为 true）。freetier：`GET /freetier/quota`（免费档本月配额代理——后端解出受管 anselm key 的 `gwk_` install token、Bearer 调网关 `GET /v1/quota`，返 `{limit,used,remaining,resetAt,available}`；客户端无法直读——token AES-GCM 加密存后端、永不出明文；无受管行 404 `FREETIER_NOT_PROVISIONED`，网关自身失败原样冒泡 `LLM_AUTH_FAILED`/`LLM_RATE_LIMITED`/`LLM_PROVIDER_ERROR`）。model：`GET /model-capabilities` · `GET /scenarios`。sandbox：`GET/POST /sandbox/runtimes` + `GET /sandbox/runtimes/available`（用户可装语言运行时 + 默认/钉死版本，UI 据此渲染、免硬编 pin map；引擎产物 llamasrv/embedmodel 与 docker 不列）+ `DELETE /sandbox/runtimes/{id}` · `GET /sandbox/envs[/{id}]` + `DELETE /sandbox/envs/{id}` · `GET /sandbox/disk-usage` · `GET /sandbox/bootstrap-status` · `POST /sandbox:gc` · `POST /sandbox:retry-bootstrap`；对话级 scratch env：`GET /conversations/{id}/sandbox-envs` · `POST .../sandbox-envs/{kind}:reset` · `POST .../sandbox-envs:reset-all`。relation：list / `GET /relations/neighborhood` / `GET /relgraph`。catalog：`GET /catalog`。limits（**机器级全局单设置**——落 `<dataDir>/settings.json`、与 workspace 无关；统一 auth 门要求 workspace header 仅作身份、对 limits 值无隔离作用，任一 workspace 改的都是这台机器的同一份上限。本地单用户语义下「全局」即正确，非 per-workspace bug）：`GET /limits`（活动运行上限）+ `GET /limits/schema`（逐字段 default/min/max/unit/desc 元数据，UI 据此渲染范围、免复刻 Go 常量）+ `PATCH /limits`（部分 JSON 合并、校验后持久化 `<dataDir>/settings.json` 并热换——消费方下次读取即生效；越界 400 `SETTINGS_LIMITS_INVALID`）+ `POST /limits:reset`（无 body，恢复 `Default()`、持久化并热换——默认由服务端持有，客户端不硬编）。notification：list / `POST /notifications/{id}:mark-read` / `POST /notifications:mark-all-read` / `GET /notifications/unread-count`。aispawn：`POST /<entity>/{id}:iterate` 分布于各实体 + `POST /executions/{id}:triage`（按 execId 前缀 function/handler/agent/flowrun 分发）。

## 系统 / 可观测性

`GET /api/v1/health`（liveness，N1 envelope，免 workspace；**但不免 bearer**——见下 loopback 加固）· `GET /api/v1/system/data-dir`（返 `{dataDir}`——解析后的数据目录 = 本地优先存储位置，供桌面端「显示 / 在文件管理器打开」；guarded，与 `/limits` 同走 workspace 门——同为**机器级**端点，header 仅作身份、非隔离轴）。**仅 dev**（`ANSELM_DEV=1`，`bootstrap.registerDebug`，非 /api/v1 路径故免 workspace 中间件、出货 sidecar 不挂——pprof 是信息泄露/DoS 面）：`GET /debug/pprof/*`（Go pprof——`goroutine`(`?debug=2` 列卡住的栈)/`heap`/`allocs`/`profile`(cpu)/`trace`，抓 goroutine 泄漏=数只涨、内存泄漏=堆只涨、CPU 失控）+ `GET /debug/stats`（运行时快照 JSON：`goroutines`/`heapAllocMB`/`heapObjects`/`stackInuseMB`/`numGC`/`gomaxprocs`/`cgoCalls`）。

**loopback 加固**（本地 HTTP 服务的安全姿态，sidecar 模型）：server 默认绑 **`127.0.0.1`**（env `ANSELM_ADDR` 覆盖；原 `:8080` 全网卡）。中间件栈在日志之后、业务之前加两道门（`router.Chain`，覆盖含 SSE GET 与 /health 的全部 /api/v1）：① **`RequireLoopbackHost`**（**常开**，仅放行 Host=`127.0.0.1`/`::1`/`localhost`，防 DNS rebinding；否则 403 `FORBIDDEN_BAD_HOST`）；② **`RequireBearerToken`**（强制 `Authorization: Bearer <ANSELM_AUTH_TOKEN>`，常时比较；否则 401 `UNAUTH_BAD_TOKEN`。桌面父进程每次启动铸随机 token 注入子进程 env——本机其它进程/网页够到端口也无 token 无法动手。**仅当 server 设了 token 时强制**：dev `make server` / `testend` 不设 `ANSELM_AUTH_TOKEN` 即关闭、零鉴权可用。豁免：`OPTIONS`（CORS 预检无 Authorization）+ `/api/v1/webhooks/`（外部调用方不知 token、自带 HMAC）；**`/api/v1/health` 不豁免**——门控它的是铸 token 的同一进程）。env：`ANSELM_AUTH_TOKEN`（空=关 bearer 强制）。

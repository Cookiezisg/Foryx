---
id: DOC-011
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# API Design — 全量物理路由契约 (167/167 Routes)

> **法律级声明**：本文档通过物理扫描 `backend/internal/transport/httpapi/handlers/` 下所有 39 个 Go 文件生成。包含 100% 的注册端点。

---

## 1. 核心交互 (Conversation & Chat)

| Method | Path | 文件源 | 备注 |
|---|---|---|---|
| POST | `/api/v1/conversations` | `conversation.go` | M5.1 ✅ 建线程容器 |
| GET | `/api/v1/conversations` | `conversation.go` | M5.1 ✅ `?cursor&limit&search&archived`；置顶优先 |
| GET | `/api/v1/conversations/{id}` | `conversation.go` | M5.1 ✅（`tokensUsed` 富化延后 M5.2） |
| PATCH | `/api/v1/conversations/{id}` | `conversation.go` | M5.1 ✅ title/systemPrompt/attachedDocuments/archived/pinned/modelOverride（三态） |
| DELETE | `/api/v1/conversations/{id}` | `conversation.go` | M5.1 ✅ 软删 + 清 relation 边 |
| POST | `/api/v1/conversations/{id}/messages` | `chat.go` | **R0056 ✅** **202** `{messageId}`；body `{content, attachmentIds?, mentions?}`；回合经 messages SSE 流式 |
| GET | `/api/v1/conversations/{id}/messages` | `chat.go` | **R0056 ✅** Paged（最新在前 + blocks）；`?cursor&limit`（N4） |
| DELETE | `/api/v1/conversations/{id}/stream` | `chat.go` | **R0056 ✅** **204** 停运行回合 + drain 积压 |
| GET | `/api/v1/conversations/{id}/system-prompt-preview` | `chat.go` | **R0057 ✅** `{systemPrompt}`（复用 buildSystemPrompt，不解析模型） |
| GET | `/api/v1/conversations/{id}/usage` | `chat.go` | **R0057 ✅** `{inputTokens, outputTokens, totalTokens}`（tokensUsed，解耦 conversation←messages） |
| GET | `/api/v1/conversations/{id}/context-stats` | `context_stats.go` | |
| GET | `/api/v1/conversations/{id}/eventlog` | `eventlog.go` | |
| POST | `/api/v1/conversations/{id}/answers` | `answers.go` | |
| GET | `/api/v1/conversations/{id}/todos` | `todo.go` | 任务看板只读（`?subagentId=` 可选）；写入是 LLM `TodoWrite` 工具（波次 2/3）|
| POST | `/api/v1/attachments` | `attachment.go` | R0051 ✅ multipart 上传（单 `file` 字段）→ CAS 存 → 返 att_ |
| GET | `/api/v1/attachments/{id}` | `attachment.go` | R0051 ✅ 元数据 |
| GET | `/api/v1/attachments/{id}/content` | `attachment.go` | R0051 ✅ 原始字节（按存储 mime）|
| DELETE | `/api/v1/attachments/{id}` | `attachment.go` | R0051 ✅ 软删（blob 由 GC 回收）|

---

## 2. 四项全能锻造 (The Quadrinity)

### 2.1 Functions (fn_)
> **I/O**：create/`:edit` 走 ops——`set_inputs`/`set_outputs`（各取 `[]schema.Field`：`[{name,type,description}]`，取代旧 `set_parameters`/`set_return_schema`）。

| Method | Path | 文件源 |
|---|---|---|
| POST | `/api/v1/functions` | `function.go` |
| GET | `/api/v1/functions` | `function.go` |
| GET | `/api/v1/functions/{id}` | `function.go` |
| PATCH | `/api/v1/functions/{id}` | `function.go` |
| DELETE | `/api/v1/functions/{id}` | `function.go` |
| POST | `/api/v1/functions/{idAction}` | `function.go` | (:run, :revert, :edit；:iterate 随 askai 波次 6) |
| GET | `/api/v1/functions/{id}/versions` | `function.go` |
| GET | `/api/v1/functions/{id}/versions/{version}` | `function.go` | (整数号或 version id) |
| GET | `/api/v1/functions/{id}/executions` | `function.go` |
| GET | `/api/v1/function-executions/{execId}` | `function.go` |

### 2.2 Handlers (hd_)
> **I/O**：方法 I/O 走 forge op `add_method`——`method.inputs`（必） + `method.outputs`（可选），各 `[]schema.Field`（取代旧 `method.args`/`method.returnSchema`）。`__init__` 配置 `set_init_args_schema`（`InitArgSpec`，带 sensitive/required/default）不变。

| Method | Path | 文件源 |
|---|---|---|
| POST | `/api/v1/handlers` | `handler.go` |
| GET | `/api/v1/handlers` | `handler.go` |
| GET | `/api/v1/handlers/{id}` | `handler.go` |
| PATCH | `/api/v1/handlers/{id}` | `handler.go` |
| DELETE | `/api/v1/handlers/{id}` | `handler.go` |
| POST | `/api/v1/handlers/{idAction}` | `handler.go` | (:call, :restart, :revert, :edit；:iterate 随 askai 波次 6) |
| GET | `/api/v1/handlers/{id}/versions` | `handler.go` |
| GET | `/api/v1/handlers/{id}/versions/{version}` | `handler.go` | (整数号或 version id) |
| GET | `/api/v1/handlers/{id}/config` | `handler.go` | (masked + configState + missing) |
| PUT | `/api/v1/handlers/{id}/config` | `handler.go` | (merge patch + 重启实例) |
| DELETE | `/api/v1/handlers/{id}/config` | `handler.go` |
| GET | `/api/v1/handlers/{id}/calls` | `handler.go` |
| GET | `/api/v1/handler-calls/{callId}` | `handler.go` |

### 2.3 Workflows (wf_)
> Quadrinity 的编排者：静态「DAG + 回边」typed 图，按 id 引用 trg_/fn_·hd_·mcp_/ag_/ctl_/apf_，每节点 CEL 接线 I/O。线性版本 + 自由 active 指针，无 pending/accept。**只 STORE+VALIDATE+PIN，不执行**（执行 = scheduler/flowrun 后续波次）。详 domains/workflow.md。
> **I/O**：图经 **ops** 编辑（`set_meta`/`add_node`/`update_node`/`delete_node`〔级联删边〕/`add_edge`/`update_edge`/`delete_edge`）。节点 `input` 是 `field → 裸 CEL`，**按 node id 读上游结果**（`reviewer.score`，对全图 node id + ctx 的 ScopedEnv 编译）；边只携控制（`fromPort`：control 源=Branch.Port / approval 源=yes\|no）。create 起始 deactivated（`active=false`/`lifecycle=inactive`），图无误后 `:activate`。

| Method | Path | 文件源 | 备注 |
|---|---|---|---|
| POST | `/api/v1/workflows` | `workflow.go` | 创建（name/description/tags + ops 建 v1，≥1 trigger，立即 active 指针但 deactivated）；返 `{workflow,version}` |
| GET | `/api/v1/workflows` | `workflow.go` | 列表（分页）|
| GET | `/api/v1/workflows/{id}` | `workflow.go` | 含 activeVersion + 解码图（nodes+edges）|
| PATCH | `/api/v1/workflows/{id}` | `workflow.go` | UpdateMeta（name/description/tags，不升版本）|
| DELETE | `/api/v1/workflows/{id}` | `workflow.go` | 软删（清边）|
| POST | `/api/v1/workflows/{idAction}` | `workflow.go` | (:edit 套 ops 写 max+1〔非空 ops〕、:revert 移指针、:activate/:deactivate 切 lifecycle、:capability-check 结构+ref 报告；**无 :trigger**；:iterate 随 askai 波次 6) |
| GET | `/api/v1/workflows/{id}/versions` | `workflow.go` | 分页 |
| GET | `/api/v1/workflows/{id}/versions/{version}` | `workflow.go` | 整数号或 version id |

> **无 `:trigger` / execution-history 端点**（消费 durable scheduler，后续波次）；**无 pending 端点**（无 accept 状态机）；`draining` 是 scheduler 设的系统态，无专门用户动词。

### 2.4 Agents (ag_)
> 第四元「配置好的 LLM worker」：不写代码，按引用挂载（skill / 文档 / fn·hd·mcp 工具 / model 覆盖 + 声明 `inputs`/`outputs`），跑 ReAct loop。线性版本 + 自由 active 指针，无 pending/accept。`:invoke` 是唯一执行入口（落 `agent_executions`）。
> **I/O**：Config 携 `inputs`/`outputs`（均 `[]schema.Field`，取代旧三态 `outputSchema`）。`outputs` 非空 ⇒ agent 被指示以含这些字段的 JSON object 作答；空 = 自由文本。enum 硬约束/coercion 已删。

| Method | Path | 文件源 | 备注 |
|---|---|---|---|
| POST | `/api/v1/agents` | `agent.go` | 扁平创建（name/description/tags + v1 Config，含 inputs/outputs），立即生效 |
| GET | `/api/v1/agents` | `agent.go` | 列表（分页）|
| GET | `/api/v1/agents/{id}` | `agent.go` | 含 activeVersion |
| PATCH | `/api/v1/agents/{id}` | `agent.go` | UpdateMeta（name/description/tags，不升版本）|
| DELETE | `/api/v1/agents/{id}` | `agent.go` | 软删（清边）|
| POST | `/api/v1/agents/{idAction}` | `agent.go` | (:edit 全量替换写 max+1, :invoke 真跑 ReAct, :revert 移指针；:iterate 随 askai 波次 6) |
| GET | `/api/v1/agents/{id}/versions` | `agent.go` | 全版本（新→旧）|
| GET | `/api/v1/agents/{id}/versions/{version}` | `agent.go` | 单版本（整数号或 version id）|
| GET | `/api/v1/agents/{id}/executions` | `agent.go` | 执行日志（?versionId/status/triggeredBy/conversationId/flowrunId）|
| GET | `/api/v1/agent-executions/{execId}` | `agent.go` | 单条执行详情 |

### 2.5 Triggers (trg_)
> 独立信号源实体（cron / webhook / fsnotify / sensor），无版本。引用计数生命周期由 workflow 激活/停用驱动（Attach/Detach，波次 4）。
> **I/O**：create/edit body 含 `outputs`（`[]schema.Field`）——声明 trigger 扇给监听 workflow 的 payload 字段（下游读这些）。
| Method | Path | 文件源 | 备注 |
|---|---|---|---|
| POST | `/api/v1/triggers` | `trigger.go` | 创建（kind + config + outputs）|
| GET | `/api/v1/triggers` | `trigger.go` | 列表（分页）|
| GET | `/api/v1/triggers/{id}` | `trigger.go` | 含 refCount/listening |
| PATCH | `/api/v1/triggers/{id}` | `trigger.go` | 改 name/description/config/outputs（kind 不可变、config 立即生效）|
| DELETE | `/api/v1/triggers/{id}` | `trigger.go` | 软删（停 listener + 清边）|
| POST | `/api/v1/triggers/{idAction}` | `trigger.go` | (:fire 手动触发一次→202；:iterate 随 askai 波次 6) |
| GET | `/api/v1/triggers/{id}/activations` | `trigger.go` | 动作日志（?firedOnly，"为什么没触发"）|
| GET | `/api/v1/trigger-activations/{actId}` | `trigger.go` | 单条 activation |
| (动态) | `/api/v1/webhooks/{triggerId}/{path}` | webhook listener | webhook 入口（由 listener 挂载）|

### 2.6 Controls (ctl_)
> workflow `control` 节点引用的路由逻辑实体（when/emit 分支组；详 domains/control.md）。AI 工作实体，有版本、无 `:run`。
> **I/O**：create/`:edit` body 含 `inputs`（`[]schema.Field`，与 fn/hd/ag/trg 对齐）——声明 workflow 节点喂入的字段（`branches[].when`/`emit` 的 CEL 读 `input.*`）。**无 `outputs`**：输出已被各分支 `emit` 的 keys 描述（下游按 port 读）。Port 是 workflow 路由的具名结局（`fromPort==port` 的边把本臂 emit 输出带到下游，control 不知道连哪个节点）。

| Method | Path | 文件源 | 备注 |
|---|---|---|---|
| POST | `/api/v1/controls` | `control.go` | 创建（name + inputs + branches）|
| GET | `/api/v1/controls` | `control.go` | 列表（分页）|
| GET | `/api/v1/controls/{id}` | `control.go` | 含 active 版 inputs + 分支 |
| PATCH | `/api/v1/controls/{id}` | `control.go` | 改 name/description（不动版本）|
| DELETE | `/api/v1/controls/{id}` | `control.go` | 软删 + 清边 |
| POST | `/api/v1/controls/{idAction}` | `control.go` | (:edit 整组替换写新版本〔inputs + branches〕、:revert 移指针；**无 :run**) |
| GET | `/api/v1/controls/{id}/versions` | `control.go` | 分页 |
| GET | `/api/v1/controls/{id}/versions/{version}` | `control.go` | 整数号或 version id |

### 2.7 Approvals (apf_)
> workflow `approval` 节点引用的审批渲染实体（prompt 模板 + 决策规则；详 domains/approval.md）。AI 工作实体，前缀 `apf_`（≠ `apv_`=运行时），无 `:run`。
> **I/O**：create/`:edit` body 含 `inputs`（`[]schema.Field`，与 fn/hd/ag/trg 对齐）——声明 workflow 节点喂入的字段（`template` markdown 用 `{{ input.* }}` 插值）。**无 `outputs`**：审批节点向下游固定吐出 `{decision, reason}`（常量，落 `approvals` 运行时表）。

| Method | Path | 文件源 | 备注 |
|---|---|---|---|
| POST | `/api/v1/approvals` | `approval.go` | 创建（name + inputs + template + 规则）|
| GET | `/api/v1/approvals` | `approval.go` | 列表（分页）|
| GET | `/api/v1/approvals/{id}` | `approval.go` | 含 active 版 inputs + template + 规则 |
| PATCH | `/api/v1/approvals/{id}` | `approval.go` | 改 name/description（不动版本）|
| DELETE | `/api/v1/approvals/{id}` | `approval.go` | 软删 + 清边 |
| POST | `/api/v1/approvals/{idAction}` | `approval.go` | (:edit 整组替换〔inputs + template + 规则〕、:revert 移指针；**无 :run**) |
| GET | `/api/v1/approvals/{id}/versions` | `approval.go` | 分页 |
| GET | `/api/v1/approvals/{id}/versions/{version}` | `approval.go` | 整数号或 version id |

---

## 3. 执行引擎 (Execution Plane) — flowrun + scheduler（M4.2/M4.3 落地）

> durable 图解释器：节点结果记忆化、崩溃可恢复（重走图、completed 行抄不重跑）。flowrun 是运行时记录（无版本/无锻造）。详 domains/flowrun.md + domains/scheduler.md + workflow-revamp/21。

| Method | Path | 文件源 | 说明 |
|---|---|---|---|
| GET | `/api/v1/flowruns` | `flowrun.go` | 分页列出（`?workflowId=` 限定单 workflow；N4 cursor/limit）|
| POST | `/api/v1/flowruns` | `flowrun.go` | **手动起 run（「Run now」）**；body `{workflowId, entryNode?, payload}`，payload 形如入口 trigger 的 `Outputs`；201 返 `{flowrun, nodes}`（v1 advance 同步，故可能已 completed/failed/running-parked）|
| GET | `/api/v1/flowruns/{id}` | `flowrun.go` | run 头 + 全部节点行（`{flowrun, nodes}`，完整记忆化）|
| GET | `/api/v1/flowrun-inbox` | `flowrun.go` | 审批收件箱 = 所有 parked 节点行（无 `apv_` 投影表）|
| POST | `/api/v1/flowruns/{idAction}` | `flowrun.go` | `:replay`（清 failed 行重走；非 failed → `FLOWRUN_NOT_REPLAYABLE`）|
| POST | `/api/v1/flowruns/{id}/approvals/{nodeId}:decide` | `flowrun.go` | 人工决策 parked 审批；body `{decision: "yes"\|"no", reason?}`；first-wins（输家 → `FLOWRUN_APPROVAL_NOT_PARKED`）|

> **firing 驱动的自动 run** 不经 HTTP（scheduler 排空 `trigger_firings` 收件箱、单事务 claim，ADR-021）。`trigger_workflow` LLM 工具随 M7 装配。删旧虚构端点 `/nodes`·`/failures`·`/trace`·`DELETE /flowruns/{id}`·`GET /approvals`（旧事件溯源/取消模型残留）。

---

## 4. MCP & Sandbox 治理

### 4.1 MCP
> N5：server 用短名（工作区唯一）作 path key；registry 用完整 slug（含 `/`，放 body）。

| Method | Path | 文件源 | 备注 |
|---|---|---|---|
| GET | `/api/v1/mcp-servers` | `mcp.go` | 列已装 server（实时 status）|
| GET | `/api/v1/mcp-servers/{name}` | `mcp.go` | 单个 server status |
| GET | `/api/v1/mcp-servers/{name}/stderr` | `mcp.go` | stderr 尾部 |
| PUT | `/api/v1/mcp-servers/{name}` | `mcp.go` | 手动 upsert（command/args/env/url/transport/runtime/timeoutSec）|
| DELETE | `/api/v1/mcp-servers/{name}` | `mcp.go` | 软删 |
| POST | `/api/v1/mcp-servers/{nameAction}` | `mcp.go` | (:reconnect) 重置闸 |
| POST | `/api/v1/mcp-servers/{name}/tools/{toolNameAction}` | `mcp.go` | (:invoke) 直接试调用，绕过 LLM（body: args）|
| POST | `/api/v1/mcp-servers:import` | `mcp.go` | Claude Desktop mcp.json 片段（`?overwrite=true`）|
| GET | `/api/v1/mcp-registry` | `mcp.go` | 列市场全量（99 个）|
| POST | `/api/v1/mcp-registry:install` | `mcp.go` | (:install) body: 完整 slug + env |

### 4.2 Sandbox
| Method | Path | 说明 |
|---|---|---|
| GET | `/api/v1/sandbox/runtimes` | 列出已装 runtime |
| POST | `/api/v1/sandbox/runtimes` | 懒装 runtime（body `{kind, version}`）→ 201 |
| DELETE | `/api/v1/sandbox/runtimes/{id}` | 删 runtime（有 env 引用 → 409） |
| GET | `/api/v1/sandbox/envs?ownerKind=` | 列出某 ownerKind 的 env（ownerKind 必填） |
| GET | `/api/v1/sandbox/envs/{id}` | 单个 env |
| DELETE | `/api/v1/sandbox/envs/{id}` | 销毁 env（DB 行 + 磁盘目录） |
| GET | `/api/v1/sandbox/disk-usage` | 磁盘占用审计 |
| GET | `/api/v1/sandbox/bootstrap-status` | mise bootstrap 状态 |
| POST | `/api/v1/sandbox:gc` | GC 超期 env（`?olderThanDays=30`） |
| POST | `/api/v1/sandbox:retry-bootstrap` | 重试 bootstrap |
| GET | `/api/v1/conversations/{id}/sandbox-envs` | 对话 scratch env 列表 |
| POST | `/api/v1/conversations/{id}/sandbox-envs/{kind}:reset` | 重置对话某 kind env |
| POST | `/api/v1/conversations/{id}/sandbox-envs:reset-all` | 重置对话所有 env |

---

## 5. 知识、关系与技能 (Knowledge & Skills)

### 5.1 Skills
| Method | Path | 文件源 |
|---|---|---|
| GET | `/api/v1/skills` | `skill.go` |
| POST | `/api/v1/skills` | `skill.go` |
| GET | `/api/v1/skills/{name}` | `skill.go` |
| PUT | `/api/v1/skills/{name}` | `skill.go` |
| DELETE | `/api/v1/skills/{name}` | `skill.go` |
| POST | `/api/v1/skills/{nameAction}` | `skill.go` | (:activate) |

> R0040：skill 重写为文件式。删 `:import`/`:refresh`（无市场、纯按需扫描）+ `/{name}/body`（GET 已含 body）；`:invoke`→`:activate`。List 返全集（文件式，不分页）。

### 5.2 Documents
| Method | Path | 文件源 |
|---|---|---|
| GET | `/api/v1/documents` | `document.go` |
| GET | `/api/v1/documents/tree` | `document.go` |
| POST | `/api/v1/documents` | `document.go` |
| GET | `/api/v1/documents/{id}` | `document.go` |
| PATCH | `/api/v1/documents/{id}` | `document.go` |
| DELETE | `/api/v1/documents/{id}` | `document.go` |
| POST | `/api/v1/documents/{idAction}` | `document.go` | (:move；:iterate 留波次 6 askai) |

### 5.3 Relations & Graph
| Method | Path | 文件源 |
|---|---|---|
| GET | `/api/v1/relations` | `relation.go` |
| GET | `/api/v1/relations/neighborhood` | `relation.go` |
| GET | `/api/v1/relgraph` | `relation.go` |

### 5.4 Memory（按 workspace 文件式 `~/.forgify/workspaces/<wsID>/memories/`）
| Method | Path | 文件源 |
|---|---|---|
| GET | `/api/v1/memories` | `memory.go` |
| GET | `/api/v1/memories/{name}` | `memory.go` |
| PUT | `/api/v1/memories/{name}` | `memory.go` |
| DELETE | `/api/v1/memories/{name}` | `memory.go` |
| POST | `/api/v1/memories/{name}/pin` | `memory.go` |
| POST | `/api/v1/memories/{name}/unpin` | `memory.go` |

---

## 6. 全局设置、用户与监控 (System)

### 6.1 API Keys & Auth
| Method | Path | 文件源 |
|---|---|---|
| POST | `/api/v1/api-keys` | `apikey.go` |
| GET | `/api/v1/api-keys` | `apikey.go` |
| PATCH | `/api/v1/api-keys/{id}` | `apikey.go` |
| DELETE | `/api/v1/api-keys/{id}` | `apikey.go` |
| POST | `/api/v1/api-keys/{idAction}` | `apikey.go` | (:test) |
| GET | `/api/v1/workspaces` | `workspaces.go` |
| POST | `/api/v1/workspaces` | `workspaces.go` |
| GET | `/api/v1/workspaces/{id}` | `workspaces.go` |
| PATCH | `/api/v1/workspaces/{id}` | `workspaces.go` |
| DELETE | `/api/v1/workspaces/{id}` | `workspaces.go` |
| POST | `/api/v1/workspaces/{idAction}` | `workspaces.go` | (:activate) |
| PUT | `/api/v1/workspaces/{id}/default-models/{scenario}` | `workspaces.go` |
| PUT | `/api/v1/workspaces/{id}/default-search` | `workspaces.go` |
| DELETE | `/api/v1/workspaces/{id}/default-search` | `workspaces.go` |

### 6.2 Utility & Metrics
| Method | Path | 文件源 |
|---|---|---|
| GET | `/api/v1/health` | `health.go` |
| GET | `/api/v1/providers` | `apikey.go` |
| GET | `/api/v1/scenarios` | `model.go` |
| GET | `/api/v1/model-capabilities` | `model.go` |
| GET | `/api/v1/usage` | `usage.go` |
| GET | `/api/v1/catalog` | `catalog.go` |
| GET | `/api/v1/metrics/tools` | `metrics.go` |
| GET | `/api/v1/eventlog` | `eventlog.go` (SSE) |
| GET | `/api/v1/notifications` | `notification.go` |
| GET | `/api/v1/notifications/unread-count` | `notification.go` |
| PUT | `/api/v1/notifications/{id}/read` | `notification.go` |
| POST | `/api/v1/notifications/read-all` | `notification.go` |
| GET | `/api/v1/notifications/stream` | `notification.go` (SSE) |
| GET | `/api/v1/forge` | `forge.go` (SSE) |

### 6.3 模型面契约 (Model Surface)

模型选择不再有独立 store：默认选择是 workspace 行的列、override 是各实体字段（详见 `domains/model.md`）。

**`GET /api/v1/scenarios`** — 固定 scenario 白名单（豁免 `RequireWorkspace`，onboarding 前可读）。
```jsonc
{ "data": [{ "name": "dialogue" }, { "name": "utility" }, { "name": "agent" }] }
```

**`GET /api/v1/model-capabilities`** — 当前 workspace 每个可用的 `(key, model)` 对，由 model 模块聚合各 key 探测档案（`test_response`）经各家 provider 自描述解析而成。探测失败 / 解析不出的 key 不贡献。
```jsonc
{ "data": [{
  "apiKeyId": "aki_…", "keyName": "我的 Claude", "provider": "anthropic",
  "modelId": "claude-opus-4-8", "displayName": "claude-opus-4-8",
  "contextWindow": 1000000, "maxOutput": 128000,
  "vision": true, "nativeDocs": true,
  "knobs": [{ "key": "thinking", "label": "Thinking", "type": "enum",
              "values": ["adaptive", "disabled"], "default": "adaptive" }]
}] }
```
- `vision`/`nativeDocs`（M7 model-caps）：模型是否原生接受图片输入 / 内联 PDF。与 ctx/out 同源于各 provider 静态 spec 表（provider 自描述）；chat 据此决定附件原生渲染 vs 文本抽取降级。现表：anthropic/gemini = vision+docs；openai = vision；kimi-k2.5/k2.6 = vision；deepseek/qwen/zhipu/doubao 列出的文本旗舰 = 否（其 vision 在独立 -vl/-V/-vision SKU，未入目录——之后加 spec 条目即启用）。
- `knobs[]` 是「容器统一、内容全原生」的可渲染描述符：`type ∈ enum|bool|int`；`key`/`values` 是各家 wire 词表，绝不归一。各家原生旋钮：openai `reasoning_effort`+`verbosity`；anthropic `thinking`(adaptive/disabled…)+`effort`(low..max,xhigh)；gemini `thinkingLevel`(Gemini-3 枚举) 或 `thinkingBudget`(Gemini-2.5 整数)；deepseek `thinking`+`reasoning_effort`(high/max)；qwen `enable_thinking`+`thinking_budget`；ollama `think`+`num_ctx`。

**`PUT /api/v1/workspaces/{id}/default-models/{scenario}`** — 设 workspace 某 scenario（`dialogue`/`utility`/`agent`）默认模型；body 是 ModelRef，`options` 为原生旋钮 k-v。返回更新后的 workspace。

**`PUT /api/v1/workspaces/{id}/default-search`** — 设 workspace 默认搜索 api-key（body `{apiKeyId}`）——WebSearch 用的唯一显式 key，provider 由 key 隐含（单选、防乱烧钱）。`DELETE` 同路径清除。返回更新后的 workspace。详见 `domains/websearch.md`。
```jsonc
// request
{ "apiKeyId": "aki_…", "modelId": "claude-opus-4-8", "options": { "thinking": "adaptive" } }
// response: 完整 Workspace 实体，含 defaultDialogue/defaultUtility/defaultAgent（ModelRef 或 null）
```
- 错误：scenario 非白名单 → `MODEL_SCENARIO_INVALID`(400)；ModelRef 缺 `apiKeyId`/`modelId` → `MODEL_REF_INVALID`(400)。

---

## 7. 开发模式专属 (Developer / --dev)

| Method | Path | 文件源 |
|---|---|---|
| GET | `/dev/logs` | `dev.go` |
| POST | `/dev/sql` | `dev.go` |
| GET | `/dev/schema` | `dev.go` |
| GET | `/dev/info` | `dev.go` |
| GET | `/dev/forgify-home` | `dev.go` |
| GET | `/dev/runtime` | `dev.go` |
| GET | `/dev/routes` | `dev.go` |
| GET | `/dev/bash-processes` | `dev.go` |
| POST | `/dev/mock-llm/scripts` | `dev.go` |
| GET | `/dev/mock-llm/queue` | `dev.go` |
| DELETE | `/dev/mock-llm/scripts` | `dev.go` |
| GET | `/dev/mock-llm/last-prompt` | `dev.go` |
| GET | `/dev/llm-trace` | `dev.go` |
| GET | `/api/v1/dev/prompts` | `prompts.go` |
| ANY | `/dev/` | `dev.go` |

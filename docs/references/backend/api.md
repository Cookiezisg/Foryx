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
| POST | `/api/v1/conversations` | `conversation.go` | |
| GET | `/api/v1/conversations` | `conversation.go` | |
| GET | `/api/v1/conversations/{id}` | `conversation.go` | |
| PATCH | `/api/v1/conversations/{id}` | `conversation.go` | |
| DELETE | `/api/v1/conversations/{id}` | `conversation.go` | |
| GET | `/api/v1/conversations/{id}/system-prompt-preview` | `conversation.go` | |
| POST | `/api/v1/conversations/{id}/messages` | `chat.go` | |
| GET | `/api/v1/conversations/{id}/messages` | `chat.go` | |
| DELETE | `/api/v1/conversations/{id}/stream` | `chat.go` | |
| GET | `/api/v1/conversations/{id}/export` | `chat.go` | |
| GET | `/api/v1/conversations/{id}/llm-trace` | `chat.go` | |
| GET | `/api/v1/conversations/{id}/context-stats` | `context_stats.go` | |
| GET | `/api/v1/conversations/{id}/eventlog` | `eventlog.go` | |
| POST | `/api/v1/conversations/{id}/answers` | `answers.go` | |
| GET | `/api/v1/conversations/{id}/todos` | `todo.go` | 任务看板只读（`?subagentId=` 可选）；写入是 LLM `TodoWrite` 工具（波次 2/3）|
| POST | `/api/v1/attachments` | `chat.go` | |

---

## 2. 四项全能锻造 (The Quadrinity)

### 2.1 Functions (fn_)
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
| Method | Path | 文件源 |
|---|---|---|
| POST | `/api/v1/workflows` | `workflow.go` |
| GET | `/api/v1/workflows` | `workflow.go` |
| GET | `/api/v1/workflows/{id}` | `workflow.go` |
| PATCH | `/api/v1/workflows/{id}` | `workflow.go` |
| DELETE | `/api/v1/workflows/{id}` | `workflow.go` |
| POST | `/api/v1/workflows/{idAction}` | `workflow.go` | (:trigger, :activate, :deactivate, :revert, :iterate) |
| GET | `/api/v1/workflows/{id}/triggers` | `workflow.go` |
| GET | `/api/v1/workflows/{id}/versions` | `workflow.go` |
| GET | `/api/v1/workflows/{id}/versions/{version}` | `workflow.go` |
| GET | `/api/v1/workflows/{id}/pending` | `workflow.go` |
| POST | `/api/v1/workflows/{id}/pending:accept` | `workflow.go` |
| POST | `/api/v1/workflows/{id}/pending:reject` | `workflow.go` |

### 2.4 Agents (ag_)
> 第四元「配置好的 LLM worker」：不写代码，按引用挂载六件（skill / 文档 / fn·hd·mcp 工具 / model 覆盖），跑 ReAct loop。线性版本 + 自由 active 指针，无 pending/accept。`:invoke` 是唯一执行入口（落 `agent_executions`）。

| Method | Path | 文件源 | 备注 |
|---|---|---|---|
| POST | `/api/v1/agents` | `agent.go` | 扁平创建（name/description/tags + v1 Config），立即生效 |
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
| Method | Path | 文件源 | 备注 |
|---|---|---|---|
| POST | `/api/v1/triggers` | `trigger.go` | 创建（kind + config）|
| GET | `/api/v1/triggers` | `trigger.go` | 列表（分页）|
| GET | `/api/v1/triggers/{id}` | `trigger.go` | 含 refCount/listening |
| PATCH | `/api/v1/triggers/{id}` | `trigger.go` | 改 name/description/config（kind 不可变、config 立即生效）|
| DELETE | `/api/v1/triggers/{id}` | `trigger.go` | 软删（停 listener + 清边）|
| POST | `/api/v1/triggers/{idAction}` | `trigger.go` | (:fire 手动触发一次→202；:iterate 随 askai 波次 6) |
| GET | `/api/v1/triggers/{id}/activations` | `trigger.go` | 动作日志（?firedOnly，"为什么没触发"）|
| GET | `/api/v1/trigger-activations/{actId}` | `trigger.go` | 单条 activation |
| (动态) | `/api/v1/webhooks/{triggerId}/{path}` | webhook listener | webhook 入口（由 listener 挂载）|

### 2.6 Controls (ctl_)
> workflow `control` 节点引用的路由逻辑实体（when/emit 分支组；详 domains/control.md）。AI 工作实体，有版本、无 `:run`。

| Method | Path | 文件源 | 备注 |
|---|---|---|---|
| POST | `/api/v1/controls` | `control.go` | 创建（name + branches）|
| GET | `/api/v1/controls` | `control.go` | 列表（分页）|
| GET | `/api/v1/controls/{id}` | `control.go` | 含 active 版分支 |
| PATCH | `/api/v1/controls/{id}` | `control.go` | 改 name/description（不动版本）|
| DELETE | `/api/v1/controls/{id}` | `control.go` | 软删 + 清边 |
| POST | `/api/v1/controls/{idAction}` | `control.go` | (:edit 整组替换写新版本、:revert 移指针；**无 :run**) |
| GET | `/api/v1/controls/{id}/versions` | `control.go` | 分页 |
| GET | `/api/v1/controls/{id}/versions/{version}` | `control.go` | 整数号或 version id |

### 2.7 Approvals (apf_)
> workflow `approval` 节点引用的审批渲染实体（prompt 模板 + 决策规则；详 domains/approval.md）。AI 工作实体，前缀 `apf_`（≠ `apv_`=运行时），无 `:run`。

| Method | Path | 文件源 | 备注 |
|---|---|---|---|
| POST | `/api/v1/approvals` | `approval.go` | 创建（name + template + 规则）|
| GET | `/api/v1/approvals` | `approval.go` | 列表（分页）|
| GET | `/api/v1/approvals/{id}` | `approval.go` | 含 active 版 template + 规则 |
| PATCH | `/api/v1/approvals/{id}` | `approval.go` | 改 name/description（不动版本）|
| DELETE | `/api/v1/approvals/{id}` | `approval.go` | 软删 + 清边 |
| POST | `/api/v1/approvals/{idAction}` | `approval.go` | (:edit 整组替换、:revert 移指针；**无 :run**) |
| GET | `/api/v1/approvals/{id}/versions` | `approval.go` | 分页 |
| GET | `/api/v1/approvals/{id}/versions/{version}` | `approval.go` | 整数号或 version id |

---

## 3. 执行引擎 (Execution Plane)

| Method | Path | 文件源 |
|---|---|---|
| GET | `/api/v1/flowruns` | `flowrun.go` |
| GET | `/api/v1/flowruns/{id}` | `flowrun.go` |
| GET | `/api/v1/flowruns/{id}/nodes` | `flowrun.go` |
| GET | `/api/v1/flowruns/{id}/failures` | `flowrun.go` |
| GET | `/api/v1/flowruns/{id}/trace` | `flowrun.go` |
| DELETE | `/api/v1/flowruns/{id}` | `flowrun.go` |
| GET | `/api/v1/approvals` | `flowrun.go` |
| POST | `/api/v1/flowruns/{id}/approvals/{nodeId}` | `flowrun.go` |
| POST | `/api/v1/flowruns/{idAction}` | `flowrun.go` | (:replay, :triage) |

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
  "knobs": [{ "key": "thinking", "label": "Thinking", "type": "enum",
              "values": ["adaptive", "disabled"], "default": "adaptive" }]
}] }
```
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

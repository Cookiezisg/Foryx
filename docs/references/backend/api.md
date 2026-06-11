---
id: DOC-008
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
audience: [human, ai]
---

# HTTP API —— 端点登记

> 全部端点的单一事实源（method · path · 语义一行）。随评审逐域填入；当前已落：function · handler · agent · workflow · flowrun · trigger · control · approval。
> 通则（N 系列）：统一 Envelope `{"data":...}` / `{"error":{code,message,details}}`；线缆 camelCase；List 全部 `?cursor&limit` 分页；非 CRUD 动作 `:action`；执行动词 `:run`(fn) `:call`(hd) `:invoke`(ag) `:trigger`(wf)；`:iterate` = 开 AI 编辑对话（全实体共享 aispawn）。

## function（`/api/v1/functions`）

| Method · Path | 语义 |
|---|---|
| `POST /functions` | 创建（扁平 payload → 反推 ops 走锻造管线），201 |
| `GET /functions` | 分页列表 |
| `GET /functions/{id}` | 单读（附 activeVersion：代码+env 状态一趟拿全） |
| `PATCH /functions/{id}` | 改 meta（name/description/tags，不升版本） |
| `DELETE /functions/{id}` | 软删 + 销毁 env + 清边，204 |
| `POST /functions/{id}:run` | 执行（TriggeredBy=manual），body `{input, version?}` |
| `POST /functions/{id}:revert` | active 指针移到指定版本号 |
| `POST /functions/{id}:edit` | ops 锻造新版本（空 ops = 仅重建 env） |
| `POST /functions/{id}:iterate` | 开 AI 编辑对话，返 `conversationId` |
| `GET /functions/{id}/versions` | 版本分页 |
| `GET /functions/{id}/versions/{version}` | 单版本（接受版本号或 fnv_ id） |
| `GET /functions/{id}/executions` | 执行日志分页（`?status&triggeredBy&conversationId&flowrunId`） |
| `GET /function-executions/{execId}` | 单执行详情 |

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
| `POST /handlers/{id}:edit` | ops 锻造新版本 + 重启实例（空 ops = 重建 env + 重启） |
| `POST /handlers/{id}:iterate` | 开 AI 编辑对话 |
| `GET /handlers/{id}/versions` · `GET /handlers/{id}/versions/{version}` | 版本（号或 hdv_ id） |
| `GET /handlers/{id}/config` | 读 config（sensitive 字段掩码 `********`） |
| `PUT /handlers/{id}/config` | JSON Merge Patch 更新（null 删 key）→ 整 blob 重加密 → **重启实例重跑 `__init__`** |
| `DELETE /handlers/{id}/config` | 清空 config + 停实例 |
| `GET /handlers/{id}/calls` | 调用日志分页（`?method&status&triggeredBy&conversationId&flowrunId`） |
| `GET /handler-calls/{callId}` | 单调用详情 |

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
| `GET /agents/{id}/versions` · `/versions/{version}` | 版本 |
| `GET /agents/{id}/executions` | 执行日志分页（同款过滤） |
| `GET /agent-executions/{execId}` | 单执行详情（含完整 transcript） |

## workflow（`/api/v1/workflows`）

| Method · Path | 语义 |
|---|---|
| `POST /workflows` · `GET /workflows` · `GET /workflows/{id}` · `PATCH /workflows/{id}` · `DELETE /workflows/{id}` | CRUD（PATCH=meta 不升版本） |
| `POST /workflows/{id}:trigger` | 立即跑一次（任何 lifecycle 下可跑），body `{payload, entryNode?}`，返 flowrun id |
| `POST /workflows/{id}:stage` | 待命恰一次真实触发后自动撤防（已 active → 409） |
| `POST /workflows/{id}:activate` / `:deactivate` | 上线（挂监听+active）/ 优雅下线（摘监听+inactive 或 draining） |
| `POST /workflows/{id}:kill` | 硬停：摘监听 + 取消全部在途 run + inactive，返被杀数 |
| `POST /workflows/{id}:edit` / `:revert` | 图 ops 锻造新版本 / 移 active 指针 |
| `POST /workflows/{id}:capability-check` | ref 解析体检（实体在吗/kind 对吗/port·method 在吗） |
| `POST /workflows/{id}:iterate` | 开 AI 编辑对话 |
| `GET /workflows/{id}/versions[/{version}]` | 版本 |

## flowrun（`/api/v1/flowruns`）

| Method · Path | 语义 |
|---|---|
| `GET /flowruns` | 运行历史分页（`?workflowId`） |
| `POST /flowruns` | 手动起 run（= workflow `:trigger` 的等价入口） |
| `GET /flowruns/{id}` | run 头 + 全部节点行（完整记忆化） |
| `POST /flowruns/{id}:replay` | 修复失败 run：清 failed 行 + 重走（completed 复用） |
| `GET /flowrun-inbox` | 审批收件箱（= 全部 parked 节点行） |
| `POST /flowruns/{id}/approvals/{node}:decide` | 人工审批决策 `{decision: yes|no, reason?}`（first-wins，输家 422） |

## trigger（`/api/v1/triggers`）

| Method · Path | 语义 |
|---|---|
| `POST /triggers` · `GET /triggers` · `GET /triggers/{id}` · `PATCH /triggers/{id}` · `DELETE /triggers/{id}` | CRUD（PATCH=Edit，热更监听中的 listener） |
| `POST /triggers/{id}:fire` | 手动催一次（扇给当前监听者） |
| `POST /triggers/{id}:iterate` | 开 AI 编辑对话 |
| `GET /triggers/{id}/activations` · `GET /trigger-activations/{actId}` | 活动审计（触没触发都有记录） |

## control / approval（`/api/v1/controls` · `/api/v1/approvals`）

两域同构：CRUD + `POST {id}:edit / :revert / :iterate` + `GET {id}/versions[/{version}]`。approval 的运行时决策端点在 flowrun 侧（见上）。

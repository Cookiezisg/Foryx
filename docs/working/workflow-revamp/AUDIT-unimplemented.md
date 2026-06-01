---
id: WRK-AUDIT-001
type: audit
status: active
owner: @weilin
created: 2026-06-01
review-due: 2026-07-01
audience: [human, ai]
---
# Workflow Revamp 未实现项完整审计

> **审计范围**：`docs/working/workflow-revamp/` 下全部 18 份文档（00-17 + IMPLEMENTATION-LOG）与实际代码的 1:1 对比。
> **审计方法**：逐文档读设计 → grep/读代码验证 → 标注实现状态。
> **状态图例**：✅ 已实现 · ⚠️ 部分实现 · ❌ 未实现 · 🔴 CRITICAL · 🟡 HIGH · 🟢 MEDIUM

---

## 一、总体概况

| 分类 | 已实现 | 部分实现 | 未实现 |
|---|---|---|---|
| **核心执行引擎**（00/17） | durable journal + replay + fork-join + CEL | continue-as-new；durable timer at/after gate | — |
| **节点系统**（01-05） | approval；cron/fsnotify/webhook trigger；case when: | tool 节点统一 dispatcher；触发器 overlap 策略 | polling trigger；agent 实体引用；durable timer gate |
| **Workflow 生命周期**（06） | drain；基础 enable/disable | activate/deactivate 端点；trigger 端点 triggerNodeId | overlap 策略；触发耗尽→停用 |
| **错误处理**（07） | flowrun cancel；approval cancel | 节点级 retry（interpreter 路径） | trigger 耗尽通知；dead letter 已由设计代替 |
| **编排 UI**（08） | trace API；approval banner；5 节点 palette | 画布节点状态实时刷新 | useFlowrunTicker；triggerNodeId 触发 UI |
| **Agent 实体**（09） | — | agent 节点（config 内嵌，无实体引用） | **完整 agent domain**；11 个 AI 工具；DB 表 |
| **LLM 工具层**（10/13/15） | search_function/handler/workflow；run_function；call_handler | capability_check（仅后端，无 LLM 工具） | create_agent；JSON repair；schema pinning；critical rules |
| **集成链路**（11） | forge SSE function/handler/workflow；domain-6 lazy | — | forge SSE agent/document/skill；relations 6 种新边；ForgeOpApplied 未真发 |
| **标准模式**（16） | WP1-4、WP5 active-branch join；Temporal event-history；callable pin | — | overlap 策略（WP6-9）；trigger schedule 层 |

---

## 二、逐文档逐项明细

### doc 00 — Overview（核心心智）

#### ✅ 已实现
- durable execution：journal + 确定性重放，崩溃重启接续 ✅
- 5 节点全集（前端 palette）：trigger/agent/tool/case/approval ✅
- 并发模型：fork（多出边广播）+ AND-join（await 全部入边）✅
- active-branch join：case XOR 分支汇合只等被激活的入边 ✅
- CEL 表达式语言（interpreter 的 case/emit 求值路径）✅
- callable 版本 pin：flowrun 启动时 pin `pinned_callables` 快照 ✅
- 事件日志 GC（HardDeleteOldest 保留 N 条）✅

#### ❌-1 🔴 Durable timer gate（`at?`/`after?`）

**设计要求（doc 00 §"Durable timer"段）：**
> 任意非 trigger 节点可挂一个 timer gate，到点才放行执行该节点：
> - `at T`（绝对）：墙钟到 T 才放行
> - `after D`（相对）：输入到齐后空 D 才放行
> arm 时把解析后的绝对 deadline 记进 journal（`timer_armed`），放行写 `timer_fired`；重放读账里的 deadline，绝不重算 `now()`。

**实际代码：**
- `EventTimerArmed`/`EventTimerFired` 事件类型已在 `domain/flowrun/event.go` 定义，但 **从不被 emit**
- interpreter.go 中 `activityRun` 没有任何 gate 逻辑
- `expiry.go` 只实现了 **approval 节点**的超时，不是通用 gate
- `at?`/`after?` 字段未在任何 NodeSpec.Config 解析中出现

**缺什么：**
1. interpreter.go 在 `activityRun` 开头：检查 `spec.Config["at"]`/`spec.Config["after"]`，如果有则挂 timer（写 `timer_armed` journal，等到点再走）
2. 到期检查器：扩展 expiry.go，扫 `timer_armed` 且 deadline 未到的 flowrun，到点写 `timer_fired` 并继续
3. 重放时：`timer_armed`/`timer_fired` 作为 record-once 事件，重放从 journal 读 deadline，不重算

#### ❌-2 🟡 continue-as-new（超长循环滚动续期）

**设计要求（doc 00 §"持久化"段）：**
> 超长循环（几千轮）会让日志变大、重放变慢 → continue-as-new：journal 事件数/大小超平台默认上限 → 自动把作用域变量快照成新 flowrun 的 input、开新 journal、旧段归档；replay 只在当前段 journal 内，UI 标"已滚动续期"。

**实际代码：** 完全未实现。没有日志大小检查，没有续期机制，没有归档逻辑。

#### ✅-3(partial) 🟡 dispatch_condition.go 已迁移到 CEL（subdag 的 {{ .loop.item }} 待 14→5 折叠时处理）

**设计要求（doc 00 §"表达式语言"段）：**
> Go text/template 作为语言整个退役（无 if/range/funcMap 控制流）；整个 workflow 平台只有一套表达式语言 = CEL。

**实际代码：**
- `internal/app/workflow/expression.go`（text/template）**仍在使用**
- `dispatch_condition.go:30`：`workflowapp.Compile(exprSrc)` 调旧引擎
- `subdag.go:162,169`：`SubstituteLoopTemplates` 用旧引擎处理 `{{ .loop.item }}` / `{{ .loop.index }}`

---

### doc 01 — Triggers

#### ✅ 已实现
- cron/fsnotify/webhook 三种 trigger listener ✅
- durable trigger_firings 收件箱 + 单事务 claim ✅
- lastFiredAt 持久化 + 跨重启补漏刻度 ✅
- trigger dedup_key（cron 按调度刻度，不重复触发）✅

#### ❌-4 🔴 polling 触发器完全未实现

**设计要求（doc 01 §polling）：**
> polling trigger = kind=`polling` 的 forge function，接 `poll(last_cursor) → {events, next_cursor}`；平台定期调它，光标前进，去重，不重复 emit。polling_states 表持久化 cursor。

**实际代码：**
- `trigger_listeners`：只有 cron/fsnotify/webhook，**没有 polling listener**
- `polling_states` 表虽已建（DB schema 存在），但没有任何代码读写它
- `PollingState` domain 结构定义了，但 trigger service 没有任何 polling 注册/调度逻辑

**缺什么：**
1. `infra/trigger/polling/polling.go`：定时调 forge function，传递 cursor，解析 `{events, next_cursor}`
2. `app/trigger/trigger.go`：注册 polling listener，处理 `KindPolling`
3. cursor 推进写 `polling_states`，崩溃重启从 DB 恢复

#### ✅-5(partial:AllowAll+serial implemented; BufferOne/BufferAll queuing deferred) 🔴 overlap 策略（BufferOne/BufferAll/AllowAll/Skip）未实现

**设计要求（doc 01 §"持久派发 + overlap"段）：**
> 派发器按 `workflow.concurrency` + `trigger.overlap_policy` 消费收件箱；撞上"正在跑"时 overlap：Skip / BufferOne（默认）/ BufferAll / AllowAll。铁律：每条 firing 都有 outcome，绝不静默丢。

**实际代码：**
- `dispatch.go:buildRun`：只做了 `CountRunning > 0` → 返回 `ErrConcurrencyLimit`（相当于 Skip）
- `TriggerSchedule.OverlapPolicy` 字段定义了，但**从不被读取**
- BufferOne/BufferAll 完全没有逻辑

#### ✅-6 🟡 trigger 用尽 → workflow 自动 deactivate 未实现

**设计要求（doc 01 §"Trigger listener 用尽 retry"段）：**
> trigger retry 用尽（如 fsnotify 路径失效重试 N 次）→ listener deactivate + 推送通知 + workflow.needs_attention=true + `trigger_exhausted` SSE 通知。

**实际代码：** 触发失败时只 log.Error，没有 retry 计数，没有 deactivate 逻辑，没有通知。

---

### doc 02 — Agent 节点

#### ⚠️ 部分实现
- AgentDispatcher 跑 ReAct 循环 ✅
- 子步 replay（StepRecorder + agent_step_completed journal）✅

#### ✅-7 🔴 agent 节点通过 agentRef 引用 forge 实体

**设计要求（doc 02 §"节点形态"段）：**
> ```yaml
> type: agent
> config:
>   agentRef: ag_xxx      # 必填 — 引用 agent entity（永远指 active）
> ```
> 平台按 agentRef 查 agent active version 的所有配置（prompt/skill/knowledge/tools/model/outputSchema）

**实际代码（`dispatch_agent.go`）：**
```go
prompt, _ := cfg["prompt"].(string)    // 直接读 prompt，agentRef 字段从未读取
maxTurns, _ := cfg["maxTurns"]...
enabledTools, _ := parseEnabledTools(cfg)
```
- `agentRef` 字段：代码里查无此字段的任何读取/处理
- agent 节点完全是"把 prompt/tools 内嵌在节点 config 里直接跑"，不是"引用实体"

---

### doc 03 — Tool 节点

#### ⚠️ 部分实现
- function/handler/mcp 各自有独立 dispatcher ✅
- agent 作为 tool node 的 callable 有 AgentDispatcher ✅
- handler 并发 mutex 串行 ✅

#### ❌-8 🟡 tool 节点没有统一的 `callable` 字段 + 前缀路由

**设计要求（doc 03 §"config"段）：**
> ```yaml
> type: tool
> config:
>   callable: fn_xxx          # 或 hd_xxx.method 或 ag_xxx 或 mcp:server/tool
>   args: { key: CEL expr }   # 逐参数一个布尔 CEL
> ```
> callable 前缀决定路由：`fn_` → function，`hd_` → handler，`ag_` → agent，`mcp:` → mcp。

**实际代码：** 仍是 14 种节点类型各自 dispatch，没有统一的 `tool` 节点 + `callable` 字段 + 前缀路由机制。workflow 里 `function`/`handler`/`mcp`/`agent` 仍作为独立 nodeType 存在。

---

### doc 04 — Case 节点

#### ✅ 已实现
- per-branch `when:` CEL 守卫，first-true-wins ✅
- fail-to-false（guard eval error → false）✅
- `emit` CEL payload 变换 ✅
- 回边形成结构化循环 ✅
- active-branch join（case XOR 分支汇合）✅

#### ⚠️ ConditionDispatcher 仍用旧 text/template 引擎

**设计要求（doc 04 §"表达式语言"段）：**
> Go text/template 整个退役；整个 workflow 平台只有 CEL。

**实际代码：** `dispatch_condition.go:30` 调 `workflowapp.Compile(exprSrc)`（旧 text/template）。这个 dispatcher 只走 loop body subdag 路径，但尚未退役。

---

### doc 05 — Approval 节点

#### ✅ 已实现
- approval park/resume via signal ✅
- durable 挂起（awaiting_signal 状态）✅
- Deadline + TimeoutBehavior + expiry checker ✅
- signal dedup first-wins ✅
- 投影行（approvals 表 + inbox API）✅

**（approval 节点设计基本完整）**

---

### doc 06 — Workflow 生命周期

#### ✅ 已实现
- trigger_firings durable 收件箱 ✅
- drain（优雅关停）✅
- flowrun cancel ✅
- enable/disable 基础逻辑 ✅

#### ✅-9 🔴 `POST /workflows/{id}:activate` / `:deactivate` 端点缺失

**设计要求（doc 06 §S6）：**
> - 新 `POST /workflows/{id}:activate`：检查 active version 存在 + capability check 通过 → 注册 listeners + 推通知
> - 新 `POST /workflows/{id}:deactivate`：撤 listeners + 进 draining 状态机（等在途 flowrun 跑完）

**实际代码：** 只有 `enabled: bool` 字段 toggle，没有 activate/deactivate 状态机。没有独立的 HTTP 端点。

#### ✅-10(partial) 🔴 `:trigger` 端点缺 `triggerNodeId` 必填参数

**设计要求（doc 06 §S6）：**
> 改造 `POST /workflows/{id}:trigger`，body 加 `triggerNodeId` **必填**（breaking）：明确指定从哪个 trigger 节点触发，携带该节点 config 定义的 payload schema。

**实际代码（`flowrun.go handler:FireManual`）：** 不接 `triggerNodeId`，直接触发，不区分多 trigger 节点场景。

#### ❌-11 🟡 draining 状态机未完整实现

**设计要求（doc 06 §"lifecycle"段）：**
> deactivate → 进 `draining` 状态 → 等在途 flowrun 跑完各自销毁实例 → 进 `inactive`。零停机，绝不强拆在途。

**实际代码：** `Drain()` 方法等 runWG，但没有 `draining` 状态持久化，重启后无法感知上次 draining 是否完成。

#### ✅-12(same as ✅-5(partial)) 🟡 Overlap 策略未实现（同 ✅-5(partial:AllowAll+serial implemented; BufferOne/BufferAll queuing deferred)）

---

### doc 07 — 错误处理

#### ✅ 已实现
- flowrun cancel（Cancel + CancelParked）✅
- 节点失败最终写 node_failed journal ✅
- approval cancel-on-flowrun-cancel ✅

#### ✅-13 🟡 节点级 retry 在新 interpreter 路径中不生效

**设计要求（doc 07 §"节点级 retry"段）：**
> 节点 config 可配 `retry: {maxAttempts, backoffMs, backoffStrategy}`；activity 失败后按策略重试；`node_started`/`node_failed` append-many 留重试痕；`node_completed` record-once。

**实际代码：**
- `retry.go` 的 `withRetry`/`dispatchWithPolicies` 在 **旧 runReadyLoop 路径（loop body subdag）** 中有效
- 新 interpreter 的 `activityRun`：直接 `dispatch.Dispatch()` → 失败写 `node_failed` → 返 error，**没有 retry 逻辑**
- `NodeSpec.Retry` 字段定义了，但 interpreter 从不读它

#### ❌-14 🟡 trigger 用尽通知（同 ✅-6）

---

### doc 08 — 编排 UI

#### ✅ 已实现
- trace API（GET /flowruns/{id}/trace）✅
- approval banner（自取 /approvals inbox）✅
- 5 节点 palette（前端 WorkflowEditor）✅
- flowrun 节点列表（GET /nodes，现写 flowrun_nodes）✅

#### ❌-15 🔴 `useFlowrunTicker` 实时节点状态机未实现

**设计要求（doc 08 §5 "运行时滴答可视化"）：**
> 新 `useFlowrunTicker`（消费 notifications + eventlog 两已有流 + 维护 "nodeId → 视觉状态" 映射 + 重连/丢失时从 trace 全量补），不订新 SSE。节点颜色读 `FlowRunNode.status`：spinning = running，绿色 = ok，红色 = failed，黄色⏸ = awaiting_signal。

**实际代码：** `useFlowrunTicker` 不存在。运行时 tick 触发 query invalidation（粗粒度重拉列表），没有 nodeId→视觉状态的 state machine。画布节点颜色是静态的，不是实时的。

#### ❌-16 🟡 triggerNodeId 触发按钮 UI 未实现

**设计要求（doc 08 §4）：**
> 触发按钮在 Trigger 节点上（▶ 按钮），支持选择具体 trigger 节点 + 填 payload form → `POST /workflows/{id}:trigger {triggerNodeId, payload}`。

**实际代码：** 前端触发按钮存在，但不传 `triggerNodeId`，不区分多 trigger 节点。

#### ❌-17 🟡 节点详情 inline diagnostic 未完整实现

**设计要求（doc 08 §6）：**
> 节点详情显示：节点 config（只读）+ 运行时状态（状态/重试次数/耗时）+ **该 flowrun 在这个节点上的事件日志（trace）**：每次执行的 input/结果/分支选择/第几轮，按序列出。

**实际代码：** FlowRunDetail 现在有 I/O|Journal 双 tab（trace API 已接入）✅。但节点运行时的"当前状态/重试次数"数据来自 flowrun_nodes 表，部分字段（kind/label/dependsOn/log）仍缺失。

---

### doc 09 — Agent Domain（最大设计-代码断层）

#### ✅-18(partial:domain+store+service+6tools; no HTTP handlers) 🔴 Agent 作为一等锻造实体完全未实现

**设计要求（doc 09 完整设计）：**

Agent = Quadrinity 的第三元（function/handler/**agent**/workflow），拥有：
1. **DB 表**：`agents`（id=`ag_xxx`，name，description，mounts JSON）、`agent_versions`（id=`agv_xxx`，prompt，skill，knowledge，tools，outputSchema，model，acceptedAt）、`agent_executions`（id=`agx_xxx`，elapsedMs，input，output）
2. **完整 CRUD 端点**（13 个 HTTP）：
   - `POST /agents`（create）
   - `GET /agents/{id}`、`GET /agents/{id}/versions/{versionId}`
   - `POST /agents/{id}:edit`、`:accept`、`:reject`、`:revert`
   - `DELETE /agents/{id}`
   - `GET /agents`（list）、`POST /agents/search`
   - `POST /agents/{id}:run`（试跑）
   - `POST /agents/{id}:iterate`（AI 迭代）
   - `GET /agents/{id}/executions`、`GET /agents/{id}/executions/{execId}`
3. **11 个 AI 锻造工具**（LLM 可调用）：
   - `search_agents`、`get_agent`、`create_agent`、`edit_agent`、`revert_agent`、`delete_agent`
   - `accept_pending_agent`、`run_agent`
   - `search_agent_executions`、`get_agent_execution`
   - `get_agent_versions`
4. **4 类 mounts**：prompt / skill（0-1）/ knowledge（doc 引用列表）/ tools（fn_/hd_/mcp，**绝不另一个 agent**）
5. **outputSchema 运行时强制**：`kind: json_schema|enum|free_text`；provider-native structured output + JSON-repair + schema validate + next_step-retry
6. **agentRef 引用**：workflow agent 节点通过 `agentRef: ag_xxx` 引用，从 active version 读所有 mounts

**实际代码：** 以上全部不存在。
```
find backend/internal/domain -name "agent*"       → 只有 subagent（子代理，完全不同的概念）
find backend/internal/app/tool/agent -name "*.go" → 不存在
grep -rn "agents\|ag_xxx\|agentRef" domain/       → 0 results（domain 层）
```

**具体缺失文件/包（约 2000 LoC）：**
- `internal/domain/agent/`（entity/repository/版本/执行记录 structs）
- `internal/infra/store/agent/`（GORM store）
- `internal/app/agent/`（service：create/edit/accept/run/iterate）
- `internal/app/tool/agent/`（11 个 AI 工具）
- `internal/transport/httpapi/handlers/agent.go`（13 个 HTTP 端点）
- DB migration：agents/agent_versions/agent_executions 三表

---

### doc 10 — AI 工具清单

#### ✅ 已实现
- search/get/run/edit/create/delete function 全套 ✅
- search/get/call/edit/create/delete handler 全套 ✅
- search/get/create/edit/delete/trigger/execution workflow 全套 ✅
- search/get/call MCP 全套 ✅
- trigger_workflow ✅

#### ✅-19 🔴 `capability_check_workflow` 工具（LLM 可调用版本）未实现

**设计要求（doc 10 + doc 13 §1-E）：**
> `capability_check_workflow(id)` 作为 LLM 工具：
> - 真查 ref（functionId/handlerName/serverName/agentRef 是否存在）
> - 结构 lint（悬空分支/空 payload/缺 fetch/不可达节点）
> - 返 `{ok, issues:[{error, missing, next_step}]}`，错误带 `next_step` 回喂让模型修
> - **强制：create/edit workflow 后、activate 前必跑**

**实际代码：**
- `workflowapp.CapabilityCheck()` 后端方法存在且正确（ValidateGraph + ProductionChecker 真查 ref）✅
- 但**没有 LLM 工具**：`grep -rn "capability_check" tool/` → 0 results
- LLM 创建 workflow 后无法主动触发校验
- 无 `next_step` 字段

#### ✅-20(partial:6 of 11 tools implemented) 🔴 Agent 相关工具全部缺失（11 个，同 ✅-18(partial:domain+store+service+6tools; no HTTP handlers)）

#### ✅-21 🟡 `list_failed_steps` / `replay_flowrun` 未作为 LLM 工具暴露

**设计要求（doc 10 §"错误诊断工具"）：**
> - `list_failed_steps(flowrunId)` → 列失败 activity
> - `replay_flowrun(flowrunId, nodeId)` → 从失败步重放

**实际代码：**
- `GET /flowruns/{id}/failures` API 存在 ✅
- `POST /flowruns/{id}:replay` API 存在 ✅
- 但**没有对应的 LLM 工具**，LLM 无法主动查失败步或触发重放

---

### doc 11 — 集成链路

#### ✅ 已实现
- domain-6 lazy 分组（function/handler/workflow/mcp/document/skill）✅
- forge SSE function/handler/workflow（create/edit/revert/delete）✅
- Relations：workflow_uses_function 等基础类型 ✅
- Catalog function/handler/workflow sources ✅

#### ✅-22 🔴 forge SSE 缺 agent/document/skill 三种 kind

**设计要求（doc 11 §S2）：**
> forge SSE 从 3 kind 扩到 **6 kind**（function/handler/workflow/agent/document/skill），驱动前端**右栏 subpage 流式呈现**。`IsValidScopeKind` 加 3 个 kind。

**实际代码：**
```go
// infra/forge/protocol.go（或类似文件）
// IsValidScopeKind 只有 function/handler/workflow 三种
```
agent/document/skill 三种 kind 的 forge SSE 完全未实现。document 编辑、skill 锻造没有实时流式呈现。

#### ✅-23(partial) 🔴 `ForgeOpApplied` 事件从未真正 emit

**设计要求（doc 11 §S2）：**
> `ForgeOpApplied`（每 op apply 时 emit，约 3-5 site）让前端"逐 op 进度"实时展示。

**实际代码：** forge 协议声明了 `ForgeOpApplied` 事件类型，但 grep 整库：
```
grep -rn "PublishOpApplied\|ForgeOpApplied\|op_applied" backend/ → 0 production results
```
协议声明了但从不发。

#### ✅-24(partial) 🔴 Relations：6 种 Agent 新边类型缺失

**设计要求（doc 11 §S3）：**
> 新增 6 种 relation kind：
> - `workflow_uses_agent`
> - `agent_uses_function`
> - `agent_uses_handler`
> - `agent_uses_mcp`
> - `agent_uses_document`
> - `agent_uses_skill`
> DB CHECK constraint 更新；SyncOutgoing 钩子。注意：**无 `agent_uses_agent`（员工思维）**

**实际代码：** `domain/relation/relation.go` 的 `IsValidKind` 里不存在上述 6 种 kind。`EntityKindAgent` 未加入 entity kind 枚举。

#### ✅-25 🟡 Catalog Item 缺 `Kind` / `Active` 字段

**设计要求（doc 11 §S4）：**
> - `Item.Kind` 字段（function 透出 normal/polling）
> - `Item.Active` 字段（workflow 透出激活状态；渲染加 `[INACTIVE]` 前缀）

**实际代码：** `catalog.Item` struct 里没有这两个字段。

#### ✅-26 🟡 自动激活未激活工具组（模型调未激活组工具时）

**设计要求（doc 11 §S1 / doc 13 §"activate_tools"段）：**
> 模型调一个还没激活的组里的工具时，后端**自动激活该组并执行**（而非报错）。

**实际代码：** 调未激活工具 → 返 `tool not found` 错误，模型必须先 `activate_tools`。

#### ✅-27 🟡 Agent 系统 prompt 独立装配链

**设计要求（doc 11 §S4 + doc 09 §"系统 prompt"段）：**
> agent 在 workflow flowrun 中跑时，使用**独立于 chat 的系统 prompt 装配链**：
> ```
> [agent identity]  你是 {agentRef.name}，职责 {prompt}
> [knowledge]       附着的文档 XML
> [skill]           单个 skill 描述
> [tools]           只有挂载的 fn/hd/mcp，无平台工具
> [output schema]   如果 outputSchema 不是 free_text
> ```

**实际代码（`dispatch_agent.go`）：**
```go
System: "You are a workflow agent. Use available tools as needed; respond concisely when finished."
```
硬编码的简单描述，无 identity/knowledge/outputSchema 注入。

---

### doc 13 — LLM-Facing 实施指南

#### ✅ 已实现
- A: case when: 守卫 ✅
- D: case fail-to-false（G9）✅
- E: capability_check 后端真查 ref ✅

#### ✅-28 🔴 JSON-repair（§1-C）

**设计要求：**
> 后端解析 tool 参数前跑 JSON-repair——容忍多行字面控制字符 + 括号配平。deepseek ~4-8%（复杂 agent ~17%）吐畸形 JSON，Go 默认拒；repair 恢复 100%。

**实际代码：** 全库无任何 JSON repair 逻辑。tool 参数解析：
```go
json.Unmarshal([]byte(argsJSON), &args)  // 直接拒，无修复
```

#### ✅-29 🔴 ops/node.config 形状未 pin 在 schema 里（§1-B）

**设计要求（实测数据）：**

| 易错点 | 现状 | 应 pin 形状 | 实测效果 |
|---|---|---|---|
| agent `set_output_schema` | `value: {}` | `{kind:'json_schema'\|'enum'\|'free_text', schema:<JSON>}` | 0% → 87% |
| trigger cron 字段 | `config: {}` | `{kind:'cron', cron:'<5段>'}` | 73%放错字段 → 100% |
| `update_code` op | 裸 `{}` | `{op:'update_code', code:'<代码>'}` | 46% → 66% |
| case node.config | 无类型 | `{branches:[{when,to,emit?}]}` | — |
| approval config | 无类型 | `{prompt,timeout?,timeoutBehavior?}` | — |
| trigger `{kind,spec}` | 无类型 | `{kind:'manual'\|'cron'\|...}` | — |

**实际代码：** 所有 forge 工具 `ops` 的 Parameters() 都是：
```json
{"type": "array", "items": {"type": "object"}}
```

#### ✅-30 🔴 系统 prompt 缺 critical rules 6 条（§2）

**设计要求（殿后，deepseek 对 prompt 末尾遵守度最高）：**

| # | 规则 | 实测效果 |
|---|---|---|
| 1 | worker 工具限制：agent 绝不调 agent；绝不给 worker fs/shell/web/memory | — |
| 2 | 选型消歧：分类/判断/抽取/路由 = agent；知识库 = document | G5 消歧 |
| **3** | **不可能能力禁令**：绝不给 agent 写没有工具支撑的能力 | **17→95** |
| **4** | **可满足性检查紧措辞**：仅当需求自相矛盾才提冲突；信息不全按默认直接建 | **0→85，宽措辞 100→47** |
| **5** | **commit-after-recon**：search/read 过一次就直接执行，别反复重查 | **revert 35→100** |
| 6 | 构图守则：cron 触发后先 fetch；case 用 when 守卫；重试 emit 自增且有界 | — |

**实际代码（runner.go）：** `howToWorkSection` 有部分原则，但上述 6 条 critical rules 完全缺失。没有"殿后"段。

#### ✅-31 🟡 系统 prompt 缺 gold 示例 + 架构决策守则

**设计要求（§4.5）：**
> - 1 个 gold 示例（完整正确 workflow，进 system prompt）→ +11pt
> - 架构决策守则（agent-vs-function / polling-vs-cron / case 别当分析师 / 多字段守卫 `&&`）→ +10pt

**实际代码：** 两者均未加入系统 prompt。

#### ✅-32 🟡 错误 envelope 无 `next_step` 字段

**设计要求（§1-E / doc 15 §F）：**
> 工具出错时 envelope 带 `next_step`（具体下一步），让模型能自纠（有 next_step 模型自纠 3/3，裸 prose 乱试）。

**实际代码：** 所有工具出错返回裸 error string，无 `next_step`。

#### ✅-33 🟡 create_function 缺 polling 教学

**设计要求：**
> 描述加：*"kind=polling 的函数 `poll(last_cursor) → {events, next_cursor}`，只 emit 比 cursor 新的、cursor 前进、不重复、首次 last_cursor=None 要处理"*

**实际代码（create_function Description）：**
```
"Build a new function from ops: set_meta, set_code, set_parameters required; ..."
```
polling 教学完全缺失。

---

### doc 15 — Tool Catalog

#### ✅-34 🔴 create_agent 工具描述未建（全新）

**设计要求（doc 15 §A）：**
> create_agent 描述：
> - agent = 配置好的 LLM worker；挂 prompt/skill(0-1)/knowledge/tools(只 fn/hd/mcp，绝不另一个 agent)/outputSchema/model
> - `set_output_schema.value = {kind, schema}` pin 形状
> - 描述里写死不可能能力禁令

**实际代码：** 工具不存在。

---

### doc 16 — 标准模式对标

#### ✅ 已实现
- WP1-4（顺序/AND-split/AND-join/XOR case）✅
- WP5 active-branch join ✅
- A-2 journal 主键模型（dedup_key partial unique）✅
- A-5 callable 版本 pin ✅
- Durable timer（approval 超时层面）✅

#### ✅-35 🟡 WP11 隐式终止定义未落入代码注释/文档

**设计要求（doc 16 §WP11）：**
> flowrun completed = 所有活跃路径到达无后继出边的节点、且无 parked（approval/timer）；有路径 failed 且无 retry = failed。

**实际代码：** interpreter 的终态逻辑已实现，但没有对照 WP11 的文档/注释确认。

#### ✅-36 🟡 WP19/20 取消粒度语义未完整规约

**设计要求（doc 16 §WP19/20）：**
> cancel 整个 flowrun → 写 `flowrun_cancelled` 事件、在途 activity 收 ctx.Done、parked approval/timer 标 cancelled。详细规约各状态的 cancel 路径。

**实际代码：** `Cancel()` 方法已实现基础路径，但 approval timer 的 cancel 语义（expiry checker 看到 cancelled 状态后的行为）未明确规约。

#### ✅-37 🟡 A-3 trigger schedule 层规约未对齐

**设计要求（doc 16 §A-3）：**
> trigger 失败 / retry / "用尽→workflow inactive" 全规约到 trigger schedule 层（Temporal Schedule 类比）；trigger 节点本身不是 activity，不在 flowrun journal 里写 retry。

**实际代码：** trigger 失败时没有 retry 计数（ConsecutiveFailures 字段存在但从不被更新）。没有 trigger 层的 retry 逻辑。

---

### doc 17 — 执行契约

**大部分 17 §1-§8 的执行契约已在 durable engine 实现（ADR-016 ~ ADR-022）。**

#### ⚠️ 剩余未实现
- §7 timer gate（at?/after?）→ 同 ❌-1
- agent sub-step 的 outputSchema 运行时强制（§N1）→ 同 ✅-18(partial:domain+store+service+6tools; no HTTP handlers) 的一部分

---

## 三、汇总 Gap 清单（按优先级排序）

### 🔴 CRITICAL — 阻断产品核心功能

| # | Gap | 对应文档 | 影响 |
|---|---|---|---|
| ✅-18(partial:domain+store+service+6tools; no HTTP handlers) | Agent 一等锻造实体完全缺失（DB/CRUD/11工具/agentRef引用） | doc 09 | Quadrinity 只有三元；无法 forge agent；workflow agent 节点无法引用受版本管理的实体 |
| ✅-19 | capability_check_workflow 工具（LLM 可调用版）未实现 | doc 13 §1-E | LLM 创建 workflow 后无法校验；端到端实测 23/24 接对依赖此校验 |
| ✅-20(partial:6 of 11 tools implemented) | 11 个 agent AI 工具全部缺失 | doc 10 | — |
| ✅-28 | JSON-repair 未实现 | doc 13 §1-C | 复杂 workflow forge 时 4-8% 工具调用失败 |
| ✅-29 | ops/node.config 形状未 pin | doc 15 §E | trigger cron 73%放错字段；set_output_schema 0% 正确 |
| ✅-30 | 系统 prompt critical rules 6 条缺失（殿后） | doc 13 §2 | 不可能能力禁令 17→95；可满足性检查 0→85 |
| ✅-22 | forge SSE 缺 agent/document/skill kind | doc 11 §S2 | 右栏 subpage 无法流式呈现 |
| ❌-4  | polling 触发器完全未实现 | doc 01 | SaaS 集成无法无 webhook 对接 |
| ✅-7  | agentRef 字段未实现（agent 节点内嵌非引用） | doc 02 | — |

### 🟡 HIGH — 显著影响质量/体验

| # | Gap | 对应文档 |
|---|---|---|
| ✅-5(partial:AllowAll+serial implemented; BufferOne/BufferAll queuing deferred)  | overlap 策略（BufferOne/BufferAll/AllowAll/Skip）未实现 | doc 01/06 |
| ✅-9  | activate/deactivate 端点缺失 | doc 06 |
| ✅-10(partial) | :trigger 端点缺 triggerNodeId 必填参数 | doc 06 |
| ✅-13 | 节点级 retry 在新 interpreter 路径中不生效 | doc 07 |
| ❌-15 | useFlowrunTicker 实时节点状态机未实现 | doc 08 |
| ✅-23(partial) | ForgeOpApplied 事件从未真正 emit | doc 11 §S2 |
| ✅-24(partial) | Relations 缺 6 种 agent 新边类型 | doc 11 §S3 |
| ✅-27 | agent 系统 prompt 独立装配链未实现 | doc 09/11 |
| ✅-31 | 系统 prompt 缺 gold 示例（+11pt）+ 架构守则（+10pt） | doc 13 §4.5 |
| ✅-32 | 错误 envelope 无 next_step 字段 | doc 13 §1-E |
| ❌-1  | durable timer gate（at?/after?）未实现 | doc 00 |

### 🟢 MEDIUM — 应改但不阻断

| # | Gap | 对应文档 |
|---|---|---|
| ❌-2  | continue-as-new（超长循环续期）未实现 | doc 00 |
| ❌-3  | text/template 引擎未完全退役（dispatch_condition/subdag）| doc 00/04 |
| ✅-6  | trigger 用尽 → workflow deactivate 未实现 | doc 01/07 |
| ❌-8  | tool 节点没有统一 callable 字段 + 前缀路由 | doc 03 |
| ❌-11 | draining 状态机未持久化 | doc 06 |
| ✅-12(same as ✅-5(partial)) | 同 ✅-5(partial:AllowAll+serial implemented; BufferOne/BufferAll queuing deferred) | — |
| ❌-14 | 同 ✅-6 | — |
| ❌-16 | triggerNodeId 触发按钮 UI 未实现 | doc 08 |
| ❌-17 | 节点详情字段部分缺失（kind/label/dependsOn/log）| doc 08 |
| ✅-21 | list_failed_steps/replay_flowrun 未作为 LLM 工具暴露 | doc 10 |
| ✅-25 | Catalog Item 缺 Kind/Active 字段 | doc 11 §S4 |
| ✅-26 | 自动激活未激活工具组未实现 | doc 11/13 |
| ✅-33 | create_function 缺 polling 教学 | doc 13 |
| ✅-34 | create_agent 工具描述（随 ✅-18(partial:domain+store+service+6tools; no HTTP handlers) 一并实现）| doc 15 §A |
| ✅-35 | WP11 隐式终止未落注释/文档 | doc 16 |
| ✅-36 | WP19/20 取消粒度语义未规约 | doc 16 |
| ✅-37 | A-3 trigger schedule 层 retry 规约未对齐 | doc 16 |

---

## 四、实现建议顺序

```
P0（阻断主线）:
  1. Agent domain 完整实现（doc 09）— 2000+ LoC，独立专题
  2. JSON-repair（infra/llm 层，~50 LoC）
  3. capability_check_workflow 工具暴露 + next_step 错误格式

P1（LLM 质量）:
  4. ops schema pinning（各 forge 工具 Parameters()，~200 LoC）
  5. 系统 prompt critical rules + gold 示例 + 架构守则
  6. activate/deactivate 端点 + :trigger triggerNodeId 参数
  7. overlay 策略（BufferOne/BufferAll/Skip/AllowAll）

P2（完善）:
  8. polling trigger listener
  9. durable timer gate（at?/after?）
 10. forge SSE agent/document/skill + ForgeOpApplied
 11. Relations 6 种 agent 新边
 12. useFlowrunTicker 前端实时状态机
 13. 节点级 retry 在 interpreter 路径中生效
 14. text/template 完全退役（dispatch_condition → CEL）
```

---

## 五、本次审计覆盖的文档

| 文档 | 审计状态 |
|---|---|
| 00-overview.md | ✅ 完整 |
| 01-triggers.md | ✅ 完整 |
| 02-agent-node.md | ✅ 完整 |
| 03-tool-node.md | ✅ 完整 |
| 04-case-node.md | ✅ 完整 |
| 05-approval-node.md | ✅ 完整 |
| 06-workflow-lifecycle.md | ✅ 完整 |
| 07-error-handling.md | ✅ 完整 |
| 08-orchestration-ui.md | ✅ 完整 |
| 09-agent-domain.md | ✅ 完整 |
| 10-ai-tool-inventory.md | ✅ 完整 |
| 11-integration-chains.md | ✅ 完整 |
| 12-deep-dive-findings.md | ✅（ADR 多已落实） |
| 13-llm-facing-implementation-guide.md | ✅ 完整 |
| 14-llm-validation-research-record.md | 参考/证据文档，无实现项 |
| 15-tool-catalog.md | ✅ 完整 |
| 16-standard-conformance.md | ✅ 完整 |
| 17-execution-contract.md | ✅（主体已实现）|
| IMPLEMENTATION-LOG.md | 日志，非规格文档 |

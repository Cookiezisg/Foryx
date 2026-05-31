# 11 — 全链路改造盘点

脑爆结论笔记(2026-05-29)。
2026-05-31 改向 durable execution(详 [`00-overview.md`](./00-overview.md))。

依赖:00-10 全部子设计。

> **核心提醒**:00-10 定的新设计要"全链路通"。按需加载 / prompt 系统 / catalog / 锻造工具 / lifecycle / SSE / DB schema 等链路都得跟着改一遍。本 doc 盘点每条链路的现状、改动、依赖顺序 — 改完才能 demo 闭环。
>
> **执行底盘已改向**:本 revamp 曾用 **message-queue + actor** 模型(节点=actor、边=持久化消息队列、控制流从消息涌现),端到端推演发现它对汇合/循环/并发持续冒窟窿,已**整体改向 durable execution**(workflow=一段结构化程序,一次 flowrun=确定性跑一遍 + 事件日志 journal + 崩溃重放)。详 [`00-overview.md`](./00-overview.md)。本 doc 已按 durable 模型重写;凡涉及"消息队列 / 节点 actor / 死信 / 复制消息进 queue / messages+node_state 表"的旧盘点项一律换成 durable 等价物。

---

## 一图速览

| # | 链路 | 现状 | 改动 | 阻塞性 |
|---|---|---|---|---|
| 1 | Lazy/Resident Toolset | 6 lazy group(function/handler/workflow/mcp/document/skill) | agent 作为第 4 个 forge 实体并入 forge 域分组(保持 domain-6 原则,详 §C1) | 🔴 强 |
| 2 | Forge 教学 prompt | runner.go `categoryLabels` 6 项 | 加 agent 标签 + trinity→quadrinity + 三条总纲 + durable 心智 | 🔴 强 |
| 3 | Catalog source 注册 | 6 readers | 加 agent reader + function kind 字段 | 🔴 强 |
| 4 | search 工具 kind 过滤 | 无 kind 概念 | search_functions 加 `kind?` 参数,按上下文默认 | 🟡 中 |
| 5 | Agent forge domain | 不存在 | 全新 domain(详 09)+ 11 锻造工具 | 🔴 强 |
| 6 | Function `kind` 字段 | 无 | version 级 enum (normal/polling) + capability check | 🔴 强 |
| 7 | Workflow.active 字段 | 仅 `active_version_id` | 加 `active bool` 列 + 6 字段 | 🔴 强 |
| 8 | trigger_workflow 工具签名 | hardcoded `"manual"` | 必填 `triggerNodeId` | 🔴 强 |
| 9 | activate/deactivate 工具 + HTTP action | 不存在 | 新增 2 工具 + 2 HTTP action | 🔴 强 |
| 10 | AcceptPending 联动 | 改 active_version_id 完事 | 加:active workflow 撤旧 listener + 重 register | 🔴 强 |
| 11 | RehydrateOnBoot 扩展 | 只扫 paused flowrun | 加:扫 `active=true` workflow 重 register listener + 重放未完成 flowrun | 🔴 强 |
| 12 | Trigger Service onFire | `isFromListener` 概念不存在 | 先写 `trigger_firings` 收件箱 → 派发器调 `StartRun(workflowId, nodeId, payload)` | 🔴 强 |
| 13 | Handler/agent instance Owner | 已有 `Owner{Kind, ID}` ✅ | owner 恒为 `{Kind:"flowrun", ID:flowrunId}`,实例 per-flowrun 隔离,无双模、无 `IsFromListener` 分支 | 🟢 弱(infra 已就绪) |
| 14 | FlowRun 字段扩展 | 无 `trigger_node_id` | 加 `trigger_node_id` 1 列(记哪个 trigger 节点起的;触发来源由其 kind 可知,无需 `is_from_listener`) | 🔴 强 |
| 15 | Durable 执行引擎 | 不存在 | 全新 durable 引擎:解释器照图走 + 事件日志 journal + 确定性重放(详 [`00-overview.md`](./00-overview.md) 持久化段) | 🔴 强 |
| 16 | 节点执行模型 | `driveLoop` 拓扑驱动 | 重构为 **5 节点结构化解释器**(fork-join 并行汇合、结构化循环、case 选路、approval durable 信号),每步往 journal 记账 | 🔴 强 |
| 17 | SSE forge 协议 | 3 kind(function/handler/workflow) | 加 agent/document/skill = **6 kind**(右栏 subpage 流式) | 🟡 中 |
| 18 | 失败诊断 API | 不存在 | 新 `GET /events?type=...` + 查 journal 的失败步视图(替代旧 dead_letter store) | 🔴 强(配 5 错诊工具) |
| 19 | flowrun-trace SSE / API | 不存在 | 新增 get_flowrun_trace(读 journal 因果链)/ nodes 数据源 | 🟡 中 |
| 20 | 前端 WorkflowEditor 节点面板 | 14 节点 | 5 节点 + 滴答可视化(详 08) | 🟡 中 |
| 21 | Durable 调度器 + 触发收件箱 + 单派发器 | 内存 lastFire + onFire 直起 flowrun(撞并发上限静默 log+return 丢) | `trigger_schedules`(持久化 listener 注册 + `last_fired_at`)+ `trigger_firings`(durable 收件箱,每条触发先持久化再动作,每条有 outcome)+ 单派发器按 overlap_policy 消费(详 §D3/E1) | 🔴 强 |
| 22 | 优雅 drain 生命周期 | deactivate/accept 即时 `DestroyOwner`(抽在途) | `workflows.lifecycle_state`(active/draining/inactive)+ 停新→等在途 flowrun 各自跑完(每个结束时自销其独占实例)→inactive,无 refcount、无共享 handler,零停机(详 §D1/D5) | 🔴 强 |

---

## 详细盘点(按依赖顺序)

### A. 底盘:DB schema + 新 entity

> **CANON-MIGRATION:不迁移,清空重建**。项目未上线,存量 `wf_` 图(旧 14 节点)/ 旧引擎 flowrun **直接清空重建,不做数据迁移**;durable 新 schema(flowruns / flowrun_events / approvals / trigger_schedules / trigger_firings 等)**全新建,无 ALTER-only 兼容包袱**。这是一次重写,不背历史数据。下文 `ALTER TABLE` 表述按"在全新建表里就含这些列"理解,不是对存量数据做迁移。

**A1. Workflow / FlowRun 字段扩展**

```sql
ALTER TABLE workflows ADD COLUMN active BOOL DEFAULT 0;
ALTER TABLE flowruns ADD COLUMN trigger_node_id TEXT;
ALTER TABLE flowruns ADD COLUMN is_from_listener BOOL DEFAULT 0;
```

`internal/domain/workflow/workflow.go` + `internal/domain/flowrun/flowrun.go` 加字段。

**A2. Function `kind` 字段(version 级)**

```sql
ALTER TABLE function_versions ADD COLUMN kind TEXT DEFAULT 'normal';
ALTER TABLE function_versions ADD COLUMN polling_interval TEXT;   -- duration string
```

DB CHECK `kind IN ('normal', 'polling')`(D3:稳定白名单)。

**A3. Agent forge domain 全新**(详 [09-agent-domain.md](./09-agent-domain.md))

- `internal/domain/agent/` 新建(entity + version + execution)
- `internal/app/agent/` 新建(service: CRUD + accept + revert + run)
- `internal/infra/store/agent/` 新建
- 路由 `/api/v1/agents`(对齐 functions / handlers)
- ID 前缀:`ag_` / `agv_` / `agx_`

**A4. Durable 执行 schema 全新**(详 [`00-overview.md`](./00-overview.md) 持久化段)

durable execution 的唯一真相是**每个 flowrun 一本 append-only 事件日志(journal)**,塌缩成三张表(替代旧的 `messages` + `node_state`):

```sql
-- 一次执行
flowruns        ( id, workflow_id, version_id,        -- version_id 启动时钉(图拓扑稳定)
                  input,                               -- payload + ctx(JSON)
                  status,                              -- running / awaiting_signal / completed / failed / cancelled
                  trigger_node_id, is_from_listener,   -- A1 加的两列
                  started_at, ended_at )

-- 唯一真相:journal(append-only)
flowrun_events  ( id, flowrun_id, seq,                -- append 顺序
                  type,                                -- node_started / node_completed / branch_taken / signal_awaited / signal_received / ...
                  node_id, iteration_key,              -- iteration_key = 区分循环不同轮的结果(内部重放键,非用户可见版本)
                  result )                             -- JSON(activity 的输出 / 分支选择 / 信号)

-- durable 等待(approval)
approvals       ( id, flowrun_id, node_id, prompt, payload,
                  status,                              -- parked / approved / rejected
                  reason, created_at, decided_at )
```

App 层 API(围绕 journal,不是队列):`AppendEvent` / `ReplayFrom(flowrunId)` / `LoadJournal(flowrunId)` / `RecordApproval`。

**`messages` / `node_state` / 消息 version / 前沿 / 空票 —— 全部不建。** 旧设计里的"消息队列 infra ~300 行"不再存在;durable 引擎自身就是执行+持久化的全部底盘(见 §H)。

**A5. 失败步视图(替代旧 dead-letter store)**

durable 模型下**没有独立 dead-letter 表 / store**:节点失败 = 那个 activity 没记账成功,失败信息(retry 用尽、错误、stack trace)作为事件记进同一本 `flowrun_events` journal。

- "死信"概念塌缩成 **journal 里 retry 用尽的失败步**(`type=node_failed` 且达 retry 上限);查询走一个 view / 谓词过滤,不开新表。
- "replay 死信"塌缩成 **从失败步重放重跑**:重放命中已记账步骤抄结果、停在那个未记账(失败)的步骤真跑一次(详 §H + [`00-overview.md`](./00-overview.md) 崩溃重放段)。

**A6. Durable 触发收件箱 + 持久调度 + drain schema 全新**(Theme 3 边界 durable)

Theme 1 让 flowrun **内部** durable(journal + 重放);Theme 3 让 `trigger → dispatch → lifecycle` 这一**边界**层 durable。根原则:**先持久化再动作 + 受管生命周期,不许有 fire-and-forget**。单进程 / SQLite,落地三处:

```sql
-- 持久化 listener 注册,取代内存里的 lastFire
-- (原 workflow.LastFiredAt 是 gorm:"-" 不入库,正是 E1 根因)
trigger_schedules ( workflow_id, trigger_node_id,
                    kind, spec,                       -- cron / fsnotify / webhook / polling / manual + 其 spec
                    last_fired_at,                    -- 持久化,开机据此算 catchup
                    catchup_window,                   -- 不补 / 补最近一次(默认)/ 补窗口内全部
                    overlap_policy )                  -- Skip / BufferOne(默认)/ BufferAll / AllowAll

-- durable 触发收件箱:每条触发一行,先持久化再动作,每条都有 outcome
trigger_firings  ( id, workflow_id, trigger_node_id,
                  payload,
                  scheduled_at, enqueued_at,
                  status,                             -- pending / claimed / started / skipped / superseded
                  flowrun_id, outcome )               -- 派发后回填

-- drain 用:生命周期状态机;停新后等在途 flowrun 各自跑完(每个 flowrun 自销其独占实例),无实例级 refcount
ALTER TABLE workflows ADD COLUMN lifecycle_state TEXT DEFAULT 'inactive';  -- active / draining / inactive
```

DB CHECK `lifecycle_state IN ('active','draining','inactive')`、`trigger_firings.status` / `overlap_policy` / `catchup_window` 各按枚举(D3:稳定白名单)。`trigger_firings` 也走**事件日志 GC 默认 retention**(见风险表)。

> **统一框架**:任何触发(cron/fsnotify/webhook/polling/manual)在尝试跑之前先写一条 `trigger_firings`(先持久化再动作)→ 统一走收件箱 → 单派发器 → flowrun;**已落库 firing 不丢**(落库前内存窗口:webhook 靠 200-after-persist + 重试、fsnotify best-effort,详 C8/C9)。manual 默认 `overlap=AllowAll`(显式动作,立即跑)。**Mechanism**(平台保证):触发绝不静默丢失 / 每条 firing 有 outcome / 在途绝不被强拆;**Policy**(编排者拍):`catchup_window`(补多少)+ `overlap_policy`(撞车怎么办),给显式选项 + sane 默认。

---

### B. 锻造工具 + 教学 prompt

**B1. Agent 11 锻造工具**(详 [09](./09-agent-domain.md) + [10](./10-ai-tool-inventory.md))

`internal/app/tool/agent/` 新建 11 个 tool 文件:`search.go` / `get.go` / `get_versions.go` / `create.go` / `edit.go` / `accept.go` / `revert.go` / `delete.go` / `run.go` / `search_executions.go` / `get_execution.go`。

**B2. Function 工具加 `kind` 参数**

- `create_function`:必填 `kind: "normal" | "polling"`,polling 时必填 `pollingInterval`
- `edit_function`:ops 数组支持 `update_kind` / `update_polling_interval`
- `run_function`:kind=polling 时平台模拟 `lastCursor` 试跑
- `search_functions`:加 `kind?` 过滤,**按上下文默认**(配 tool 节点 → kind=normal;配 polling trigger → kind=polling)

**B3. Workflow lifecycle 3 工具**

- `activate_workflow(id)`:`internal/app/tool/workflow/activate.go` 新建
- `deactivate_workflow(id)`:同上
- `trigger_workflow(id, triggerNodeId, payload)`:**改造现有** `internal/app/tool/workflow/trigger.go`(签名加 `triggerNodeId` 必填)

**B4. 运行时观察 5 工具**

`internal/app/tool/workflow/` 加 `search_flowruns.go`(已有重构)/ `get_flowrun.go`(已有)/ `get_flowrun_trace.go` 新建 / `get_flowrun_nodes.go` 新建 / `cancel_flowrun.go` 新建。

> `get_flowrun_trace` 读 **journal 的因果链**(node_started/node_completed/branch_taken 的有序序列),不是"消息队列历史";`get_flowrun_nodes` 从 journal 折算每节点最新状态。

**B5. 错误诊断 5 工具(全新)**

`internal/app/tool/diagnosis/`(新子包 — §S12 允许 `app/tool/` 下按家族嵌套):`query_events.go` / `list_failed_steps.go` / `get_failed_step.go` / `replay_flowrun.go` / `clear_failed_steps.go`。

> 这 5 个工具全部围绕 **journal** 工作(不是死信队列):`query_events` 查事件流;`list_failed_steps` / `get_failed_step` 过滤 journal 里 retry 用尽的失败步(payload + ctx + 错误 + stack trace);`replay_flowrun` 触发**从失败步重放重跑**;`clear_failed_steps` 把失败步标记为放弃(不再可 replay)。语义对齐 [`07-error-handling.md`](./07-error-handling.md)(读到 07 里"死信 / 复制消息进 queue"等旧表述,按"journal 失败步 / 重放重跑"理解)。

**B6. Forge 教学 prompt 改 4 处**

`internal/app/chat/runner.go`:

- `categoryLabels` map:加 `"agent": "agent (LLM ReAct loop configuration)"`
- `toolsSection` const:lazy 组列举里加 agent(保持 domain-6 分组原则,详 §C1)
- `identitySection` / `howToWorkSection`:trinity → quadrinity,加员工思维 / 永远 prod / 能力源自 forge 三条总纲(详 [`00-overview.md`](./00-overview.md));并把执行心智讲清——workflow=结构化程序、一次 flowrun=确定性跑一遍 + 崩了照 journal 重放(让 LLM 理解"为何 case 只读已记账值、为何工具要幂等")
- 新加 "polling cursor 模板" 段(高风险工具 LLM 兜底,详 [10](./10-ai-tool-inventory.md))

---

### C. Toolset 装配 + Resident/Lazy 划分

**C1. `toolapp.Toolset` Lazy 加 agent 域(保持 domain-6 原则)**

`backend/cmd/server/main.go` `lazyGroups` 加 `agent` category。

> **🆕 分组原则已被研究收口为 domain-6**(详 [`14-llm-validation-research-record.md`](./14-llm-validation-research-record.md) §3.6 + [`13-llm-facing-implementation-guide.md`](./13-llm-facing-implementation-guide.md) §3):lazy 分组**按 forge/资产域分,不按 edit/use 细分**——A/B 实测 domain 分组激活对组 62% 显著优于 11-组细分的 46%(细分让模型搞混 edit 组和 use 组)。当年脑爆稿写的"6 组不行要拆 7/11 组"**已被推翻**(真变量是 search_* 工具的摆放位置,不是组粒度)。
>
> 因此:agent 作为**第 4 个 forge 实体**,自然得到一个与 function/handler/workflow 对称的 forge 域 lazy 组,**不拆 edit/use 子组**。资产域(mcp/document/skill)分组不动。

**Resident vs Lazy 划分提案**(全 ~89 工具):

| 分类 | 工具数 | Resident? | Lazy 组 |
|---|---|---|---|
| 主对话基础(file/shell/web/task/ask) | ~14 | ✅ | — |
| activate_tools(meta) | 1 | ✅ | — |
| Forge function | 11 | ❌ | `function` |
| Forge handler | 12 | ❌ | `handler` |
| Forge agent(新) | 11 | ❌ | `agent`(新,第 4 个 forge 域) |
| Forge workflow | 9 | ❌ | `workflow` |
| Workflow lifecycle(activate/deactivate/trigger) | 3 | ❌ | `workflow`(并入) |
| 运行时观察 | 5 | ❌ | `workflow`(并入) |
| 错误诊断(journal 失败步) | 5 | ❌ | `workflow`(并入,**workflow 组膨胀到 ~22 工具**) |
| MCP | 5 | ❌ | `mcp` |
| Document | 7 | ❌ | `document` |
| Skill | 3 | ❌ | `skill` |
| Memory | 3 | ✅(跨对话基础) | — |

**结论:7 个 lazy 域**(function / handler / agent / workflow(膨胀) / mcp / document / skill)—— 这是 **domain-6 原则**(按域分、不按 edit/use 细分)在 quadrinity(agent 加入)后的自然结果,**不是回到被推翻的"拆 11 组"**。`activate_tools` enum 加 `agent` 候选。

> **激活摩擦兜底**(详 [13](./13-llm-facing-implementation-guide.md) §3):① skill 命名撞车(用户说"激活技能"模型想直接 `activate_skill` 却够不着未激活组);② 模型 search 完想直接 edit/run 够不着 lazy 工具。**最省心修法:模型调一个还没激活的组里的工具时,后端自动激活该组并执行**(而非报错)。这是后端兜的小机制,与 domain-6 分组正交。

**C2. `host.Tools(ctx)` 无改**(逻辑通用)。durable 引擎的 `loop.Run` 仍每步调 `host.Tools(ctx)` 重算(Resident + 已激活 Lazy 组),与现状一致。

---

### D. Lifecycle hooks 联动

**D1. AcceptPending 联动(走优雅 drain,不即时抽在途)**(详 [06-workflow-lifecycle.md](./06-workflow-lifecycle.md))

**不再即时 `DestroyOwner`**(那会抽掉在途 flowrun)。`internal/app/workflow/crud.go` 末尾走 drain 状态机(见 D5):

```go
if workflow.Active {
    triggerService.UnregisterByWorkflow(id)   // (1) 停新:撤 listener
    workflow.lifecycle_state = "draining"      //     派发器不再为该 wf 起新 flowrun
    // (2) 排空:在途 flowrun(durable、靠 journal 活着)在老版本跑完,
    //     各自结束时 DestroyOwner({Kind:"flowrun", ID}) 自清其独占实例;不在此处即时 Destroy,无共享实例、无 refcount
    drainCoordinator.OnDrained(id, func() {
        // (3) in-flight=0 后:挂新版本 listener → 新 active
        scanGraphAndRegister(...)              // 重做 activate(新 active_version_id 的图)
        workflow.lifecycle_state = "active"
    })
}
```

正在跑的 flowrun 继续用老版本逻辑跑完(其 `FlowRun.version_id` 已钉住老图拓扑);排空后挂的新 listener 触发的 flowrun 用新版本。**零停机、绝不抽在途**。

**D2. RehydrateOnBoot 扩展(CANON-BOOT 四步)**

`internal/app/scheduler/rehydrate.go` —— 重启 = 四步,Theme 3 边界恢复(a/b/d)+ Theme 1 内部重放(c)合起来端到端无 fire-and-forget:

```go
// (a) 从 trigger_schedules 重挂 listener(取代旧的"扫 active workflow + 内存 lastFire")
for _, sch := range listTriggerSchedules() {   // lifecycle_state = active
    triggerService.Register(sch)
}

// (b) 材化漏的 firing:按 cron 表达式算 last_fired_at → now 漏了哪几次,
//     按 catchup_window 策略写进收件箱(catchup;诚实边界见下)
for _, sch := range listTriggerSchedules() {
    for _, t := range missedFirings(sch.spec, sch.lastFiredAt, now, sch.catchupWindow) {
        appendFiring(sch, t)   // 先持久化(pending),不直接起 flowrun
    }
}

// (c) durable 重放:扫未完成 flowrun,从头确定性重放(命中 journal 抄结果,停在第一个未记账步续跑)
for _, fr := range listRunningOrAwaitingFlowruns() {   // status ∈ {running, awaiting_signal}
    durableEngine.Resume(fr)   // 详 §H + 00 崩溃重放段
}

// (d) 派发器继续消费收件箱(pending firing → 按 overlap_policy 起 flowrun,见 D3)
dispatcher.Start()
```

**Catchup 诚实边界**:`last_fired_at` 落库,cron 靠 cron-math 补、polling 靠 cursor 自愈;**webhook/fsnotify 的停机期事件是外部 ephemeral 客观找不回**(可选 fsnotify 开机扫现状兜底),明说不假装兜住。catchup_window 策略由编排者拍(对接 [`desktop-packaging-notes.md`](../desktop-packaging-notes/desktop-packaging-notes.md) 早标的"错过任务策略":不补 / 补最近一次(默认)/ 补窗口内全部)。

(c) 替代旧设计的"扫 paused flowrun + 重建内存态 + 重新 drive DAG"——**唯一真相是 journal,重放即恢复**,没有 PausedState / ExecutionContext 内存快照要重建。`awaiting_signal`(approval 挂着)由 journal 里的 `signal_awaited` 事件表达,重放到该点自然停下等信号,**不依赖任何进程内 cancel handle**。

**D3. Trigger `onFire` → 收件箱 → 单派发器(CANON-INBOX / CANON-DISPATCH)**

`internal/app/trigger/trigger.go` + 新 `internal/app/scheduler/dispatch.go`。**不再 onFire 直起 flowrun**(那是 fire-and-forget):任何触发**先写一条 `trigger_firings`(先持久化再动作)**,单派发器再消费:

```go
// onFire:先持久化一条 firing(pending),绝不在此处直接 StartRun
func onFire(workflowID, nodeID, payload) {
    appendFiring(workflowID, nodeID, payload)   // 落库后即 durable(落库前窗口见 C9)
    bumpLastFiredAt(workflowID, nodeID)         // last_fired_at 落库(给 catchup)
}

// 单派发器:按 workflow.concurrency + trigger.overlap_policy 消费收件箱
func dispatch(firing) {
    if hasRunningFlowrun(firing.workflowID) {   // 撞上"正在跑"
        switch overlapPolicy {
        case Skip:       markOutcome(firing, "skipped")     // 跳过但记 outcome,绝不静默(+可通知)
        case BufferOne:  supersedeOlderBuffered(firing)     // 默认:只留最新一个排队
        case BufferAll:  /* 全排队按序跑(留 pending,空了再跑) */
        case AllowAll:   startRun(firing)                   // 无视上限并发(manual 默认)
        }
        return
    }
    fr := scheduler.StartRun(firing.workflowID, firing.nodeID, firing.payload)
    markStarted(firing, fr)   // 回填 flowrun_id + status=started
}
```

`StartRun` 起一个 flowrun(写 `flowruns` 行 + 首条 journal 事件),durable 引擎接手照图走。**铁律:每条 firing 都有 outcome**(started/skipped/superseded/...)—— 取代现状 onFire 对 `ErrConcurrencyLimit` 只 log+return 的静默丢(E2 根治)。排队的 firing 留收件箱(pending/buffered),空了再跑。`overlap_policy` 默认 `BufferOne`,manual 默认 `AllowAll`。

**D4. Handler instance Owner 调用方**

`internal/infra/handler/dispatch_handler.go`:Owner 恒为 `{Kind:"flowrun", ID:flowrun.ID}`(无条件、无 `IsFromListener` 分支):

```go
owner := handler.Owner{Kind: "flowrun", ID: flowrun.ID}
inst := handlerRegistry.Acquire(ctx, owner, name, spawnFn)
```

handler/agent 实例 per-flowrun 隔离:首次调用时 lazy spawn,flowrun 结束时 `DestroyOwner({Kind:"flowrun", ID:flowrun.ID})` 自清。`IsFromListener` 如保留仅记录触发来源,与实例归属无关。

(owner 恒为 flowrun,实例 per-flowrun 独占;不存在 workflow 级共享实例 / 双模。现状 dispatch_handler.go 已无条件这么干。)

**D5. 优雅 drain 生命周期(CANON-DRAIN,解 E6)**

deactivate / accept **不即时 `DestroyOwner`**(会抽在途 flowrun)。走三段状态机(`workflows.lifecycle_state`,A6 加的;在途 flowrun 各自结束时自销其独占实例,无实例级 refcount):

```
(1) 停新  —— 撤 listener + 派发器不再起新 flowrun;workflow 进 draining
(2) 排空  —— 在途 flowrun(durable、靠 journal 活着)在老版本跑完,
             各自结束时 DestroyOwner({Kind:"flowrun", ID}) 销毁其独占实例;无共享实例、无 refcount
(3) 收口  —— in-flight=0 后(accept)挂新版本 listener → inactive / 新 active(无 workflow 级实例可销毁,各 flowrun 已自销)
```

- **deactivate**:走 (1)(2)(3) → 终态 `inactive`。
- **accept(改 version)**:走 (1)(2)(3) → (3) 末挂新版本 listener → 终态 `active`(D1 调的就是这条)。
- flowrun 结束时调 `DestroyOwner({Kind:"flowrun", ID})` 销毁该 flowrun 独占的全部实例;`internal/app/scheduler/` 的 drainCoordinator 只跟在途 flowrun 计数(归 0 = 排空完成),不做实例级 refcount。

**零停机、绝不抽在途**——任何 durable 系统的受管生命周期前提。

**E1. Polling kind=polling function 系统**(详 [01-triggers.md](./01-triggers.md))

- Trigger Service 加 polling listener(新 `internal/infra/trigger/polling/`)
- polling listener tick interval = function.pollingInterval
- **listener 注册与 `last_fired_at` 一律落 `trigger_schedules`(A6),取代内存里的 lastFire**(原 `workflow.LastFiredAt` 是 `gorm:"-"` 不入库,正是 E1/E2 静默丢的根因);触发先写 `trigger_firings`(先持久化再动作),不在 tick 里直起
- 平台另持久化业务 cursor:`polling_states (workflow_id, node_id, cursor TEXT)`(给 polling 函数读取增量;停机期 polling 靠 cursor **自愈**,无需 catchup 补 firing)
- 失败 retry 用尽 → workflow drain → `lifecycle_state=inactive`(走 D5,不即时抽在途)+ SSE 通知(详 [07](./07-error-handling.md))

**E2. Trigger 节点 payloadSchema**(详 [01](./01-triggers.md))

- 节点 config 加 `payloadSchema` JSON schema 字段
- listener 类型节点的 payloadSchema 由 kind 固定(cron `{firedAt}` / webhook `{method, headers, body}` 等)
- manual 节点的 payloadSchema 编排者拍

**E3. Capability check on accept(CANON-X2:no-pin 下唯一护栏,查深)**

no-pin(永远 prod)下没有 version 钉,**capability-check 是唯一护栏,要查深**。`internal/app/workflow/crud.go` `:accept` 前 + agent.tools 挂载时:

- **检查项**(4 条):存在 + kind 匹配(polling/normal 等)+ handler 的 `.method` 在 active version 还在 + 必填参数齐(node/agent 给了值 → 查有无给值,**不查类型**)。**砍掉 full payload 类型流**(payload 动态无类型,太难;运行时由 N1 + case 的 fail-to-false(G9)兜)。
- **报全 + next_step**:遍历各节点引用的 fn_/hd_/ag_,**报全部问题、每条带 `next_step`,不首违规就短路**(详 [13](./13-llm-facing-implementation-guide.md) §1-E / [14](./14-llm-validation-research-record.md) G8;端到端实测真查-ref 版 capability_check 0/24 → 23/24 接对)。
- **两个触发点**:(a) **accept 时 gate**——workflow 节点引用 + `agent.tools` 挂载引用都查;(b) **被引用实体改了 active version(kind/签名)时反向重查依赖方**——标 `needs_attention` + 通知,**复用现有 capability-deletion listener**(从"删"扩到"改")。

> 这跟执行底盘无关,durable 改向不影响这条。

---

### F. Catalog + 能力披露

**F1. Catalog source 注册**

`backend/cmd/server/main.go` `catalog.RegisterSource` 加 agent reader(对齐 function/handler reader 接口)。

**F2. Function reader kind 字段透出**

`internal/app/function/catalog_source.go`:在 catalog 项里加 `kind` 字段(让 LLM `search_functions` 看到 normal 还是 polling)。

**F3. Workflow trigger 节点暴露**

`get_workflow(id)` 返回时,把 trigger 节点 list(`{nodeId, kind, payloadSchema}`)显式拎出来,**方便 LLM 调 `trigger_workflow` 时填 triggerNodeId**。

---

### G. SSE + 协议

**G1. forge SSE 扩到 6 kind = +agent / document / skill(CANON-X1:扩共享 scope 枚举)**

`internal/infra/forge/bridge.go` + `infra/forge/protocol.go`:加 `agent` / `document` / `skill` 到 kind 枚举(→ 6)。

> **CANON-X1**:给 forge SSE 加 agent / document / skill kind,本质是**扩 eventlog 的共享 scope-kind 闭枚举**——`forge.IsValidScopeKind` 复用 `eventlogdomain.Kind*`,两条 SSE 共用同一套 scope 词表。这是项目重构,**扩这个共享枚举是接受的、不另立 forge 私有枚举**;按 **E2**(闭枚举先改协议文档再加 code)**先更新 [`event-log-protocol.md`](../eventlog-redesign/event-log-protocol.md) 再加 kind**。(**forge SSE = 6 kind**:function/handler/workflow/agent/document/skill —— 前端 subpage 右栏要对这 6 大锻造工具**流式呈现编辑/锻造**,核心产品体验;`forge SSE 的 6 kind ≠ forge 实体分类 quadrinity 4`,两个 axis;mcp 不进。)

**G2. eventlog SSE 事件本体不动,但共享 scope 枚举要扩(澄清旧说)**

消息流本体 = 主对话 block,新 domain agent 锻造对齐 chat 既有 5 events × 7 block types,**事件/block 类型不动**。但 G1 已点明:eventlog 与 forge **共用同一套 scope-kind 闭枚举**,加 agent / document / skill kind 时这套**共享 scope 枚举要扩**(走 E2;forge SSE = 6 kind,右栏 subpage 流式)。旧表述里"eventlog SSE 不动"应理解为"eventlog 的 5×7 事件本体不动",**不等于共享 scope 枚举不动**。

**G3. notifications SSE 加新 type**

- `workflow_activated` / `workflow_deactivated` / `flowrun_started` / `flowrun_completed` / `flowrun_failed` / `trigger_exhausted` / `handler_crash` / `step_failed`
- 协议是开放词表(E2),加字符串即可
- (`step_failed` = durable 模型下"某 activity retry 用尽",取代旧的 `dead_letter_created`)

**G4. flowrun-progress 进度 tick(新,CANON-X4:best-effort 不背压)**

用于 UI 实时画布滴答:每节点状态变化(node_started / node_completed / branch_taken / signal_awaited)推一条 lightweight 提示——数据源就是 journal 的 append 事件。

> **CANON-X4**:flowrun 进度 tick(画布滴答)= **best-effort、可丢、不进 notifications 的 replay buffer、绝不背压执行引擎**(限流 / 合并:每节点状态变化或每 N ms 一条)。**真相在 journal(flowrun_events)**;UI 重连或丢了事件就从 `get_flowrun_trace` 拉一次全量补。这跟 durable 一致——journal 是 durable 真相,tick 只是实时提示,丢了无所谓。

- **不开第 4 条 SSE**(E1 铁律:上限 3 条);进度走 **notifications 的一个可丢 best-effort 子类**,与会阻塞的实体变更事件**隔离对待**(进度子类不进 replay buffer、不背压;实体变更事件照旧)。

**结论:不开第 4 条 SSE,flowrun-progress 作为 notifications 的 best-effort 子类并入,可丢、不背压、真相在 journal、重连从 trace 补**。

---

### H. 节点执行引擎(durable 解释器)

**H1. driveLoop → durable 结构化解释器**(详 [`00-overview.md`](./00-overview.md) 持久化段)

`internal/app/scheduler/`:从拓扑驱动重构为 **durable execution 解释器**——一个 goroutine 从 trigger 节点出发照图的结构往下走,每步往 SQLite 的 `flowrun_events` journal 记账;遇并行就再开几个 goroutine + `WaitGroup` 等齐;遇 approval 就 durable 挂起等信号。**没有分布式队列、没有消息 version、没有前沿计算、没有 actor 盯邮箱。**

核心循环(`Run` + `Resume` 同一套):

```
解释器照图走(从 trigger 节点 / 重放时从头)
   ├ 走到 agent/tool 节点(activity)
   │     ├ journal 已有该步结果(命中 iteration_key)→ 直接抄,不重跑 LLM/工具
   │     └ 没记账 → 真跑一次 → 把结果 append 进 journal
   ├ 走到 case → 读已记账的 payload 求 CEL → 选一个分支(branch_taken 记 journal)/ 绕回边
   ├ 走到并行分叉 → 同时开几条分支 goroutine,汇合处 await 全部(fork-join)
   └ 走到 approval → 记 signal_awaited、durable 挂起(status=awaiting_signal),信号到了 append signal_received 继续
终止:程序返回(completed)/ workflow timeout 强杀 / 用户 cancel / activity retry 用尽(failed)
```

**这是最大单点改造,但规模比旧 message-queue 引擎小**(无需写队列 infra + actor 调度 + 消息状态机 + 原子认领):**~1000-1800 行**(解释器 + journal 记账 + 重放 + fork-join + CEL 求值接 [04](./04-case-node.md))。

> **表达式语言(CANON-N2)**:全平台一套表达式语言 = CEL。`internal/app/workflow/expression.go`(Go text/template)**整个退役**(无 if/range/funcMap 控制流)→ 换成**一个 CEL 求值核心**(求值/布尔字段裸 CEL,产出类型化值)+ **一个薄的 `{{ CEL }}` 插值 pass**(文本文档字段如 agent.prompt / approval.prompt,`{{ }}` 里是 CEL、求值后字符串化插入)。`{{ }}` 不是第二种语言、只是 CEL 的插值定界符;列表拼字符串用 CEL 函数一行(如 `payload.items.map(i,i.name).join`)。详 [04](./04-case-node.md)。

旧脑爆稿的这些机制**由构造消失,不要再写**:

| 旧机制(作废) | durable 下取代物 |
|---|---|
| 节点 = actor 盯入口 queue dequeue | 节点 = activity,解释器照图主动走到它 |
| case 回边 = 复制消息进上游 queue | case 回边 = 程序里的结构化循环(只重跑循环体,`iteration_key` 区分轮次) |
| 终止 = 无新消息 | 终止 = 程序返回 / timeout / cancel / retry 用尽 |
| 消息 version / 前沿 / 空票 配对 | 删除——循环用作用域变量、没走的分支不执行、join=await 全部只跑一次 |
| consume-emit-processed 状态机 + 原子认领 | journal append 记账 + 确定性重放(命中已记账抄结果) |

**H2. 5 节点的解释器处理**

| 节点 | 解释器怎么处理 |
|---|---|
| `trigger` | 程序入口:用 trigger 的 payload + ctx 起跑(不是 activity),append 首条事件 |
| `agent` | **activity**:调 agent domain `Run(prompt, tools, knowledge, model)`(详 [02](./02-agent-node.md) + [09](./09-agent-domain.md))→ 结果记 journal。**forged agent run 强制其声明的 `outputSchema`**(CANON-N1:agent-run 薄层做 JSON-repair → 按 schema 校验 → 回喂带 next_step 的结构化错误 → 重试 ~2 轮 → 用尽=该 activity 失败;只校 enum/json_schema、free_text 不校;详 [09](./09-agent-domain.md))|
| `tool` | **activity**:解 ref(永远 active version)→ 调 callable(fn/hd/mcp/agent)→ 结果记 journal |
| `case` | **纯控制流**:读已记账 payload 求 CEL → 选分支 / 绕回边,记 `branch_taken`(详 [04](./04-case-node.md)) |
| `approval` | **durable 等信号**:记 `signal_awaited` + 写 `approvals` 行 + 挂起;用户决策 = 一条 `signal_received` 事件,重放/在线都从此处继续(详 [05](./05-approval-node.md)) |

> **确定性硬约束**(durable 正确性前提,Forgify 天生满足):所有不确定性(LLM 输出 / 工具结果 / 时间 / 随机)都在 activity 里、结果记进 journal;case/loop 的判断只读已记账的 payload(纯 CEL、无副作用)。所以重放每次走同一条路。详 [`00-overview.md`](./00-overview.md) 确定性段。
>
> **exactly-once 边界**:平台保证每个 activity 结果只记一次账、重放读缓存不重复调 LLM/工具;但 activity 崩在"外部副作用已发生、结果未记账"之间 → 重跑会重复那个副作用(任何 durable 系统含 Temporal 的固有 at-least-once 边界)。**编排者选 retry + 把工具写成幂等 = 业务层 exactly-once**。这是命名清楚的责任线,归锻造,不是窟窿。

---

## 改造顺序(因果链)

按依赖严格顺序,7 大块:

```
块 1: DB schema 改完(A1 + A2 + Workflow.active A7 + Agent domain A3 + durable schema A4 + 触发/drain schema A6)— 1.5 天
   ↓ (entity + 数据底盘:flowruns / flowrun_events / approvals / trigger_schedules / trigger_firings + lifecycle_state)
块 2: Agent domain + 11 锻造工具(B1)— 2 天
   ↓ (forge entity 就位,跟 function/handler 同 lift)
块 3: Durable 执行引擎(H1 + H2)— 3-4 天
   ↓ (最大单点,核心:解释器 + journal 记账 + 重放 + fork-join;无独立队列 infra 块,比旧模型省一块)
块 4: Lifecycle / 触发边界 durable(activate/deactivate/trigger 工具 + durable 调度器 + 触发收件箱 + 单派发器 overlap D3 + 优雅 drain D5 + AcceptPending 走 drain D1 + RehydrateOnBoot CANON-BOOT 四步 D2)— 2.5 天
   ↓ (上线 / 触发抽象 / boot 恢复;触发绝不静默丢、在途绝不被抽)
块 5: Polling 系统 + capability check(E1 + E3)— 1.5 天
   ↓ (trigger 体系闭环)
块 6: 运行时观察 + 错诊工具(B4 + B5,全部读 journal)+ 教学 prompt 全改(B6)+ catalog(F1-F3)+ toolset(C1)+ SSE(G1-G4)— 2 天
   ↓ (AI 工程师能用)

总:12.5-13.5 天纯写,加测试 ~17-19 天
```

> 与旧盘点对比:旧版把"message queue infra"(块 3)和"节点执行引擎重构"(块 4)分两块共 ~5 天;durable 模型把两者合成**一块 durable 引擎**(块 3,3-4 天)——执行+持久化是同一套解释器+journal,不需要单独的队列底盘,**总工期略降**。

**前端 WorkflowEditor 改造**(详 [08-orchestration-ui.md](./08-orchestration-ui.md))平行 durable 引擎块后开工 — 2-3 天。

---

## 闭环验收(全链路通的判据)

| 场景 | 必须通 |
|---|---|
| **AI 锻造 agent**:`create_agent` → `accept_pending_agent` → `run_agent` 试跑 | ✅ Agent forge 通 |
| **AI 造 polling**:`create_function(kind=polling)` → workflow 引用 → `activate` → cursor 持久化 + 真触发 | ✅ Polling 闭环通 |
| **AI 编排 + 试跑**:`create_workflow` + 几个节点 → `trigger_workflow(wf, manualNode, payload)` 跑通 | ✅ 编排核心通 |
| **AI 上线**:`activate_workflow` → cron 自动触发 → 每次触发起一个 flowrun,实例 per-flowrun 隔离,flowrun 结束自销 | ✅ Lifecycle 通 |
| **跨触发累积**:active workflow 内 cron 跑 N 次,计数累积在外部 store / journaled 作用域变量(非进程内存),每个 flowrun 独立实例,重放结果一致 | ✅ State discipline 通 |
| **改 entity 自动跟新**:edit_function → accept → 所有 workflow 引用自动用新版 | ✅ 永远 prod 通 |
| **结构化循环**:case 回边重试到成功,只重跑循环体、循环外值不重算,`iteration_key` 区分轮次 | ✅ durable 循环语义通 |
| **fork-join 并行**:节点多条出边并发跑、汇合处 await 全部只继续一次(无双点火) | ✅ 并发模型通 |
| **trigger 用尽 inactive**:polling 跑挂 → retry 用尽 → workflow 走 drain → `lifecycle_state=inactive` + 通知 | ✅ 错诊 + lifecycle 联动通 |
| **失败步 replay 重跑**:handler crash → activity retry 用尽记入 journal → AI 调 `replay_flowrun` 从失败步重放重跑 | ✅ 错诊 + durable 重放通 |
| **boot 恢复**:Forgify 重启 → active workflow 自动重 register listener + 未完成 flowrun 照 journal 确定性重放接着跑 | ✅ Rehydrate + 重放通 |
| **approval durable 挂起跨重启**:approval 挂着时重启 → 重放到 `signal_awaited` 继续等,信号到了从此处续跑 | ✅ approval durable 通 |
| **AI 反馈循环**:用户"跑一下" → AI 发现缺 manual 节点 → edit_workflow 加 → trigger 成功 | ✅ chat/workflow 互通,产品 narrative 落地 |

每个场景都过 = 全链路通 = demo 可以摆。

---

## 风险点(改造期间踩坑预警)

| 风险 | 触发场景 | 缓解 |
|---|---|---|
| **durable 引擎重写阻塞太久** | 块 3 ~3-4 天纯改写,影响并发其他块 | 块 1-2 + 块 4-6 都可平行;块 3 单独排长档期 |
| **重放确定性破坏** | 解释器在 activity 外引入了不确定性(读时钟 / 随机 / 直接调外部),重放走岔路 | 铁律:一切不确定性塞进 activity 并记 journal;case/loop 只读已记账 payload(纯 CEL);加重放一致性单测(同 journal 重放两次结果一致) |
| **AcceptPending 联动漏点** | active workflow 改 version 时旧 listener 没撤干净,出"幽灵触发" | 联动写完单独 E2E:edit + accept + 校验旧 listener 不再 fire |
| **Polling cursor race** | LLM 写的 polling function 漏存 cursor / 重复触发(详 [10](./10-ai-tool-inventory.md) 🔴 风险) | 教学 prompt 强约束 + 提供 cursor 模板库 |
| **Handler/agent 实例隔离** | 不同 flowrun 串了实例 / flowrun 结束没清干净 | 单测:owner 恒为 per-flowrun、不同 flowrun 不共享实例;`DestroyOwner({Kind:"flowrun", ID})` 在 flowrun 结束清干净 |
| **journal retention** | 平台给兜底 GC 默认(资源安全,对齐 sandbox 30 天先例),但超长循环(几千轮)让 journal 变大、重放变慢 | continue-as-new(快照 + 新日志);"N 塞进工具"哲学让循环天然短(详 [04](./04-case-node.md) / 00 持久化段) |
| **trigger_firings retention** | 高频触发(秒级 cron / polling)让收件箱表无限长 | **沿用事件日志 GC 默认 retention**(A6;与 journal 同一套兜底,不另设策略);outcome 已回填的老 firing 可 GC |
| **agent 节点空 tool 退化 single-shot** | 节点配错或 LLM 忘挂 tool → 默认变 LLM 一发(详 02) | run_agent 试跑时返回 tokens / tool calls count,LLM 能自检 |

---

## 砍掉 / 已确认无需动的

| 项目 | 理由 |
|---|---|
| Message queue infra / 节点 actor / 消息 version / 前沿 / 空票 / 死信 store | **整体作废**——改向 durable execution(解释器 + journal + 重放),这些窟窿由构造消失(详 [`00-overview.md`](./00-overview.md) 设计演进段) |
| `messages` 表 / `node_state` 表 | 塌缩成 `flowrun_events`(journal);不建 |
| Variable 节点 / 全局状态 | 砍(00 列表)——跨节点状态用 journaled 作用域变量 / payload(或外部 store);handler 实例进程内存只放 ephemeral、不影响结果的资源(连接池 / 缓存),不放可累积的业务状态 |
| Loop 节点 | 砍(case 回边 = 结构化循环) |
| Parallel 节点 | 砍(普通节点多出边 = fork,汇合处 = join,程序结构原生表达) |
| Wait 节点 | 砍(延迟 = durable timer,记"睡到 T"、重放按 journal 判断) |
| HTTP 节点 | 砍(forge function 包装) |
| LLM 节点 | 砍(agent 节点空 tool 退化 single-shot) |
| Skill 节点 | 砍(agent 挂载) |
| `domain/events` 包 | 已删(CLAUDE.md 已注明) |
| Handler/agent instance owner | 恒为 `{Kind:"flowrun", ID:flowrunId}`,per-flowrun 隔离,调用方无需拍 Kind;无双模、无 workflow 级共享实例 |
| Subagent 数据表 | 已统一进 messages 行(attrs.kind=subagent_run),不动 |
| Sandbox v2 | 已就绪(CLAUDE.md),agent 跑也走它 |
| eventlog SSE 协议 | 5 events × 7 block types 不动(agent 跑也走它,作为 message 流的特殊形态) |
| Lazy 拆 11 组("6 组不行"假说) | **已被推翻**(详 [14](./14-llm-validation-research-record.md) §3.6)——保持 domain 分组,agent 加入后是 7 个 forge/资产域,不是 edit/use 细分 |

---

## 待用户确认

1. **agent 节点试跑能力**:`run_agent` 试跑直接调真 LLM 烧 token,还是支持 mock LLM 试跑?
2. **错诊工具 Resident 还是 Lazy**:本 doc 提议并入 `workflow` lazy 组,LLM 处理 workflow 问题时 activate workflow 组就全有。是否合理?
3. **flowrun-progress 流**:并入 notifications SSE(不开第 4 条),还是定独立 subprotocol(notifications 内部的"progress"子 type)?
4. **polling listener tick 实现**:Forgify 主进程内启 N 个 goroutine 各管一个 polling trigger,还是统一 ticker 调度器?(N 多时影响)
5. **journal continue-as-new 阈值**:超长循环触发快照的默认轮数 / 日志大小阈值定多少?(资源安全兜底,本地单用户可先设宽松默认)

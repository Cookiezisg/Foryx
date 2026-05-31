# 06 — Workflow Lifecycle

脑爆结论笔记(2026-05-27)。
2026-05-31 改向 durable execution(详 [`00-overview.md`](./00-overview.md))。

依赖纲领:[`00-overview.md`](./00-overview.md) 的 durable-execution 模型(事件日志 journal + 确定性重放)+ `Workflow.active` 字段。`FlowRun.IsFromListener`(如保留)仅记录触发来源,与实例归属无关 —— handler/agent 实例 Owner 恒为 `{Kind:"flowrun", ID:flowrunId}`。本篇只讲 lifecycle(上线/下线/触发/恢复);执行底盘细节见 00 与 [`11-integration-chains.md`](./11-integration-chains.md)。

---

## 没有"Deployment"抽象层

行业里 K8s / n8n / Airflow / GitHub Actions / Lambda 都用 **workflow 自身的 active 状态**表达"上线",**不引入额外的 Deployment 实体**。Forgify 跟进这个标准做法。

数据模型:2 个 entity + 2 个 flag(外加 drain 期的 `lifecycle_state` 枚举 active/draining/inactive,详 CANON-DRAIN;drain 只跟在途 flowrun,不做实例级 refcount):

```
Workflow {
  id, active_version_id,
  active: bool                ← 新加,上线开关
}

FlowRun {
  id, workflow_id, workflow_version_id, status,
  trigger_node_id,            ← 新加,触发的 trigger 节点 ID
  is_from_listener: bool      ← 新加,触发来源
}

Handler Instance Registry(in-memory,不入 DB)
  └── 按 Owner{Kind, ID} 索引
        Kind 恒为 "flowrun"(无 "workflow" 共享实例)
```

---

## Active 状态的产品语义

| 状态 | 行为 |
|---|---|
| `active = true` | 扫 workflow graph 中所有 **listener 类型** 的 trigger 节点(cron / fsnotify / webhook / polling),调 `triggerService.RegisterTrigger`(写 `trigger_schedules` 持久化)。listener 开始监听 |
| `active = false` | 撤所有 listener + 派发器不起新 flowrun;走 drain 状态机(active → draining → inactive)等在途 flowrun 各自跑完,每个结束时 `DestroyOwner({Kind:"flowrun", ID:flowrunId})` 自销其独占实例(见 Deactivate 流程)。无 workflow 级共享实例需销毁 |

`lifecycle_state`(active/draining/inactive)是 drain 期的中间态:`active` 正常运行,`draining` 停新但在途 flowrun 仍在老实例跑完,in-flight 归 0 后转 `inactive`。

**Active / Inactive 只管 listener,不管 manual trigger 节点**。Manual 节点本来就没 listener,任何时候只要 workflow 存在就能被显式触发(`:trigger` HTTP / `trigger_workflow` 工具 / UI 点节点)。

## 谁能改 workflow.active

| 谁 | 怎么改 | `last_action_by` |
|---|---|---|
| 用户(UI) | 点 Active toggle | `"user"` |
| AI(chat) | 调 `activate_workflow` / `deactivate_workflow` 工具 | `"user"`(代用户) |
| **平台**(特例) | **Trigger 节点 retry 用尽 → 自动设 false** + `attention_reason="trigger_exhausted: ..."` + 推 SSE 通知 | `"system"` |

平台**不会**在其他失败场景下动 active(普通节点失败只通知)。Trigger 是入口,入口废了 workflow 客观不能工作,平台必须诚实标记。详 [`07-error-handling.md`](./07-error-handling.md) 的 Trigger 特例段。

---

## 触发的统一抽象

不管谁触发,都走同一个底层入口:

```go
scheduler.StartRun(workflowId, triggerNodeId, payload) → runID
```

3 套入口汇聚(触发来源**不影响实例归属** —— owner 恒为 `{Kind:"flowrun"}`):

| 入口 | 谁组装 `(triggerNodeId, payload)` |
|---|---|
| Listener 自动触发(cron / fsnotify / webhook / polling) | listener |
| UI 用户点 trigger 节点 + 填表单 → `POST /workflows/{id}:trigger` | 用户 |
| AI 调 `trigger_workflow(workflowId, triggerNodeId, payload)` 工具 | LLM(按节点 payloadSchema)|

**handler/agent 实例 Owner 恒为 `{Kind: "flowrun", ID: flowrunId}`**:三套入口无差别,每个 flowrun 独占自己的实例,首次调用时 lazy spawn,flowrun 结束时自销。`IsFromListener` 不决定 owner(无 `{Kind:"workflow"}` 共享实例、无跨触发复用)。暖实例复用如未来需要 = per-handler 的 ephemeral 资源池(Temporal 式),非共享有状态实例,V1 不做。

详 [`03-tool-node.md`](./03-tool-node.md) handler 生命周期段。

---

## Activate 流程(持久化注册)

```
POST /workflows/{id}:activate
   │
   ├── workflow.active = true + lifecycle_state = active
   ├── 读 active version 的 graph
   ├── 扫所有 listener 类型 trigger 节点(cron / fsnotify / webhook / polling)
   ├── 对每个调 triggerService.RegisterTrigger({
   │      WorkflowID, NodeID, Kind, Config, UserID
   │   })
   │     └── **写一行 trigger_schedules**
   │         (workflow_id, trigger_node_id, kind, spec,
   │          last_fired_at[持久化], catchup_window, overlap_policy)
   └── listener 开始监听
```

handler instance 此时**不创建**——lazy 等首次触发时 acquire 时再 spawn。

**注册是持久化的(CANON-SCHEDULE)**:listener 注册落 `trigger_schedules` 表,`last_fired_at` 入库;**取代旧设计里只活在内存的 `lastFire`**(原 `workflow.LastFiredAt` 是 `gorm:"-"` 不入库)。这直接修了 [`00-overview.md`](./00-overview.md) 列的 **E1「重启丢补跑」**——进程崩了重启,从 `trigger_schedules` 里读回 `last_fired_at`,按 cron 表达式算 `last_fired_at → now` 漏了哪几次,再按 Catchup Window 策略补(见 Boot 恢复段)。`trigger_schedules` 三处数据模型详 [`11-integration-chains.md`](./11-integration-chains.md)(CANON-DATA)。

---

## Deactivate 流程(优雅 drain)

```
POST /workflows/{id}:deactivate
   │
   ├── (1) 停新:workflow.active = false + lifecycle_state = draining
   │        triggerService.UnregisterByWorkflow(id)  ← 撤所有 listener
   │        删/停 trigger_schedules row + 派发器不再为本 workflow 起新 flowrun
   │
   ├── (2) 排空:在途 flowrun(durable、靠 journal 活着)在老实例跑完
   │        每个 flowrun 结束时 DestroyOwner({Kind:"flowrun", ID:flowrunId}) 自销其独占实例
   │        (无 refcount、无共享 handler)
   │
   └── (3) in-flight = 0 后:lifecycle_state = inactive
            (无 workflow 级共享实例需销毁 —— 各 flowrun 已自销)
```

**不再即时 `DestroyOwner`(CANON-DRAIN)**:deactivate 走 active → draining → inactive 状态机,**绝不抽在途的 handler**。在途 flowrun 是 durable 的(journal + 重放),在老实例跑完;每个 flowrun 结束时各自销毁自己 `{Kind:"flowrun"}` 的独占实例,无 refcount、无共享 handler。这直接修了 [`00-overview.md`](./00-overview.md) 列的 **E6「抽在途 handler」**——旧设计 deactivate 即时 `DestroyOwner({workflow})`,会把正在用该 handler 的在途 flowrun 拆掉。`lifecycle_state`(active/draining/inactive)数据模型详 [`11-integration-chains.md`](./11-integration-chains.md)(CANON-DATA / CANON-DRAIN)。

> **挂起的 approval 不阻塞 drain(C3)**:drain 只等**正在跑 activity** 的在途 flowrun 清零;`awaiting_signal`(挂起等人、可能等 30d)的 flowrun **不计为在途、不钉老实例**——它没在用 handler,只是 parked 在 journal 里。所以一个长挂 approval **不会让 drain 卡 30 天**:drain 等的是"正在跑的活儿"清零,parked 的等信号 run 继续 durable 地挂着;信号到了照样按 **active 版本**逻辑续(它本就要换到新版本——符合"永远 prod")。这避免了"长挂 approval 饿死 drain + 钉死老实例一个月"。

---

## AcceptPending 联动(优雅 drain 换版)

```
AcceptPending(workflowId, pendingVersionId)
   │
   ├── 现有逻辑:翻 active_version_id / 清 needs_attention / trim 老版本
   │
   └── 新加:如果 workflow.active,走 drain 换版(CANON-DRAIN):
       ├── (1) 停新:lifecycle_state = draining
       │        triggerService.UnregisterByWorkflow(id)  ← 撤老 listener
       │        停老 trigger_schedules + 派发器不为老版本起新 flowrun
       ├── (2) 排空:老版本在途 flowrun 跑完,各自结束时 DestroyOwner({Kind:"flowrun", ID:flowrunId}) 自销其独占实例(无 refcount、无共享 handler)
       └── (3) in-flight = 0 后:重做 activate 流程(扫新 graph,写新 trigger_schedules,重挂 listener)
                lifecycle_state = active
                (无 workflow 级共享老实例需销毁 —— 各 flowrun 已自销)
```

**换版不即时拆老实例(CANON-DRAIN)**:走 active → draining → active 状态机。正在跑的 flowrun 用老版本的逻辑跑完(durable、靠 journal 活着),各自结束时销毁自己 `{Kind:"flowrun"}` 的独占实例(无 refcount、无共享 handler);in-flight 清零后才挂新版本 listener。新 listener 触发的 flowrun 用新版本。**用户感知零停机、绝不抽在途**(同样修 [`00-overview.md`](./00-overview.md) **E6**)。

---

## Boot 恢复(CANON-BOOT)

重启要恢复四件事(durable 调度器 + durable 触发收件箱 + journal 重放,合起来端到端无 fire-and-forget):

```
Process boot
   │
   ├── (a) 从 trigger_schedules 重挂 listener
   │       └── 对每行(active workflow 的 trigger 节点)重做 activate 注册
   │             webhook 重挂 mux / fsnotify 重新 watch / polling 重启 tick / cron 重挂
   │
   ├── (b) cron-math + catchup:把停机期漏的 firing 材化进收件箱
   │       └── 按 cron 表达式算 last_fired_at → now 漏了哪几次,
   │           按 catchup_window 策略(不补 / 补最近一次[默认] / 补窗口内全部)
   │           写 trigger_firings(status=pending)
   │       └── 诚实边界:cron 靠 cron-math 补、polling 靠 cursor 自愈;
   │           webhook/fsnotify 的停机期事件是外部 ephemeral 客观找不回
   │           (可选 fsnotify 开机扫现状兜底),不假装兜住
   │
   ├── (c) 从 journal 重放在途 flowrun(Theme 1)
   │       └── 扫 status ∈ {running, awaiting_signal} 的 flowrun,从头确定性重放
   │             ├ 命中事件日志的 activity → 直接抄日志里的结果(不重跑 LLM/工具)
   │             ├ 走到第一个没记账的步骤 → 真跑一次、记账,接着往下走
   │             └ awaiting_signal(approval 挂着)→ 重放回到挂起点继续等信号
   │                 (信号到达也是一条日志事件;无需在内存里持有 cancel handle —
   │                  挂起点就是日志里的 signal_awaited,重放天然恢复)
   │
   └── (d) 派发器继续消费收件箱
           └── 按 workflow.concurrency + trigger.overlap_policy 消费 trigger_firings
               把 pending firing 派成 flowrun(撞上限按 overlap 策略,每条都有 outcome)
```

**(a) 持久重挂取代旧的内存 `lastFire`(CANON-SCHEDULE)**:listener 注册的真相是 `trigger_schedules`(`last_fired_at` 入库),不再依赖只活在内存、随进程消失的 `lastFire`。这跟 Activate 段一起修了 [`00-overview.md`](./00-overview.md) **E1「重启丢补跑」**。

**(b) catchup 材化进收件箱**:停机期漏的 cron firing 不再当场补跑,而是按 catchup_window 策略写进 `trigger_firings` 收件箱(先持久化再动作),由派发器 (d) 统一消费——已落库 firing 不丢(cron firing 由 cron-math 幂等可重算,崩在 catchup 中途下次开机重算补上)。补多少是编排者拍的 policy(CANON-MP),平台只保证不静默丢。

**(c) 替代旧设计的"扫 paused flowrun + 重建内存态 + 重新 drive DAG"**——不再有 PausedState / ExecutionContext 的内存快照需要重建,**唯一真相是事件日志**,重放即恢复(Theme 1)。approval 的"挂着等人"由 `awaiting_signal` 状态 + 日志里的 `signal_awaited` 事件表达,重放到该点自然停下等信号,**不依赖任何进程内的 cancel handle**。详 [`05-approval-node.md`](./05-approval-node.md)。

**(c) 必须先于 (d):boot 阶段串行(C7)**——派发器 (d) **等 (c) 重放全部完成才开闸**消费收件箱。否则 catchup 的 firing 会给一个"正在被重放"的 workflow 起新 flowrun,而 overlap-policy 此刻还看不到那个重放中的 run、可能误判"没在跑"从而违反 `BufferOne`。规则:**重放中的 run 对 overlap 计为 "running"**;(d) 在 (c) 完成后再按 overlap_policy 消费,看到的"在跑集合"才完整。

**draining 在 boot 时自然续上**——`lifecycle_state=draining` 持久;boot 时对每个 draining 的 workflow,从 journal 重放它老版本的在途 `running` flowrun(`awaiting_signal` 的不算,见 C3),drain 等这些 flowrun 各自跑完(每个结束时自销其独占实例)。**无 refcount 可丢、无 boot 时 refcount 重建**:drain 完成的判据就是"在途 running flowrun 清零",崩在 drain 中途也能续上,不会"要么永不销毁、要么提前抽实例"。

收件箱 / 派发器 / catchup 策略详 [`11-integration-chains.md`](./11-integration-chains.md)(CANON-INBOX / CANON-DISPATCH / CANON-SCHEDULE)。

进程崩溃 → 重启后 listener 自动重挂、停机期漏的触发按策略补、未完成的 flowrun 接着跑、收件箱继续派发,**用户感知 ≤ 进程启动时间**。

---

## Inactive workflow 仍可被显式触发

```
workflow.active = false
   │
   ├── listener 不跑(cron 不到点跑 / webhook URL 返 404 / fsnotify 不 watch / polling 不 tick)
   │
   └── 但用户/AI 仍可:
       ├── UI 上点画布上任何 trigger 节点 + 填表单
       ├── HTTP POST /workflows/{id}:trigger {triggerNodeId, payload}
       └── AI 调 trigger_workflow(workflowId, triggerNodeId, payload)
       
   → scheduler.StartRun(...)
   → handler/agent 实例 Owner 恒为 {Kind:"flowrun"},per-flowrun 独立
     (owner 与触发来源无关 —— 不因 listener / 显式触发而不同)
```

适合**调试 / 开发 / 偶尔跑一次**场景——不用 activate 也能跑。

---

## 跟 AI 的产品反馈循环

```
用户:"帮我跑一下邮件总结 workflow"
   ↓
AI 调 trigger_workflow(wf_xxx)
   ↓
LLM 看到该 workflow 的所有 trigger 节点 schema:
   - cron_node:{firedAt}        (kind=cron)
   - 没有 manual 节点
   ↓
AI 自然反应:"这个 workflow 设计成 cron 自动跑的,没留手动入口。
              我加一个 manual trigger 节点吧?"
   ↓
用户:"好"
   ↓
AI 调 edit_workflow:加 manual trigger 节点,payloadSchema 按业务推断
   ↓
AI 调 :accept_pending → workflow 新版本生效
   ↓
AI 调 trigger_workflow(wf_xxx, "new_manual_node", payload) → 成功跑
```

这是 Forgify "chat 老板 / workflow 员工" narrative 的具体落地——chat AI **不只触发员工**,还会**发现员工缺接口然后帮你改员工**。Dify / n8n / Coze 抄不动这个反馈循环,因为它们的 workflow 编辑跟 chat 是分开的产品。

---

## 跟 Forgify 现状的 gap + 改动清单

| 改动 | 代码量 |
|---|---|
| `Workflow.active bool` + `Workflow.lifecycle_state`(active/draining/inactive)字段 | DB migration 两列 |
| `FlowRun.trigger_node_id TEXT` + `FlowRun.is_from_listener bool` | DB migration 两列 |
| `trigger_schedules` 表(持久化 listener 注册 + `last_fired_at` 入库,取代内存 `lastFire`)| 见 [`11-integration-chains.md`](./11-integration-chains.md) CANON-DATA |
| drain 跟在途 flowrun 计数(归 0 = 排空完成;无实例级 refcount,flowrun 结束自销其独占实例) | 见 [`11-integration-chains.md`](./11-integration-chains.md) CANON-DRAIN |
| `POST /workflows/{id}:activate` / `:deactivate` HTTP action | `handlers/workflow.go` + `app/workflow/lifecycle.go` 新 ~50 行 |
| activate:扫 graph 提取 trigger 节点 → `RegisterTrigger` + 写 `trigger_schedules` | ~40 行 |
| deactivate:drain 状态机(停新 → 排空在途 flowrun(各自销毁自有实例)→ inactive,取代即时 `DestroyOwner`)| ~25 行 |
| trigger Service `onFire` → 先写 `trigger_firings` 收件箱 → 派发器 `StartRun(workflowId, triggerNodeId, payload)` | 见 [`11-integration-chains.md`](./11-integration-chains.md) CANON-INBOX |
| `dispatch_handler.go` Owner 恒为 `{Kind:"flowrun"}`(现状已无条件如此,无 IsFromListener 分支) | 0 行(已就绪) |
| AcceptPending 末尾:active 时走 drain 换版(停新 → 排空在途老版本 flowrun(各自销毁自有实例,无 refcount)→ 重 register 新 graph) | ~30 行 |
| `RehydrateOnBoot`(CANON-BOOT a/b):从 `trigger_schedules` 重挂 + cron-math+catchup 材化漏的 firing | ~40 行 |
| Boot 重放(CANON-BOOT c):扫 running/awaiting_signal flowrun → 照事件日志确定性重放接着跑(替代旧的扫 paused + 重建 PausedState/ExecutionContext)| 属执行引擎,见 [`11-integration-chains.md`](./11-integration-chains.md) |
| `:trigger` HTTP body 加 `triggerNodeId` 字段(替代隐式 manual) | ~10 行 |
| `trigger_workflow` LLM 工具描述加 `triggerNodeId` 必填参数 + 暴露 workflow 的 trigger 节点 list | ~30 行 |
| 砍 `local-user` magic / 默认 manual:必须显式指定 triggerNodeId | 0 行(自然 by-product) |

drain / 收件箱 / 持久调度的数据模型 + 派发器细节落在 [`11-integration-chains.md`](./11-integration-chains.md)(CANON-DATA / DRAIN / INBOX / DISPATCH / SCHEDULE);drain 状态只是 `lifecycle_state` + 在途 flowrun 集合(从 flowrun status 推导),不含实例级 refcount。本篇行数只算 lifecycle 入口侧改动。

---

## 总结的产品 narrative

```
设计阶段:
  AI 在 chat 帮用户画 workflow,加各种 trigger 节点
  每个 trigger 节点有 kind + payloadSchema

调试 / 试跑:
  AI / 用户点画布上任意 trigger 节点 + 填 payload → flowrun 跑
  Owner=flowrun,跑完销毁
  不需要 activate

上线:
  AI 调 :activate → listener 开始跑
  cron 自动每天 9 点触发 / webhook 接外部 / polling 启动...
  每次触发起一个 flowrun,Owner=flowrun,实例 per-flowrun 隔离(首调 lazy spawn,flowrun 结束自销)
  无跨触发复用;durable/业务状态进 journaled 作用域变量/payload 或外部 store,不放进程内存

日常使用:
  AI 调 trigger_workflow(wfId, manualNodeId, payload)
    → 触发 manual trigger 节点 → Owner=flowrun,独立 instance
    → 跟其他 flowrun(含 listener-触发)同样 per-flowrun 互相隔离
      (没有共享的 listener 实例可供"隔离"—— 所有 flowrun 一律独占自己的实例)

下线:
  AI 调 :deactivate → listener 停(停新)→ 在途 flowrun drain 跑完
    → 每个 flowrun 结束时各自 DestroyOwner({Kind:"flowrun"}) 自销其独占实例(绝不抽在途,无 refcount)→ inactive
  Manual 触发仍可用(workflow 还在就能调)
  workflow 不删,inactive 状态保存

进程重启(CANON-BOOT):
  从 trigger_schedules 重挂 listener(last_fired_at 入库,不靠内存 lastFire)
  cron-math + catchup 把停机期漏的 firing 材化进收件箱(按 catchup_window 策略)
  从 journal 重放未完成的 flowrun(running / awaiting_signal)接着跑
    (awaiting_signal 的 approval 重放到挂起点继续等信号,无需内存 cancel handle)
  派发器继续消费收件箱
  整体感知 ≤ 进程启动时间
```

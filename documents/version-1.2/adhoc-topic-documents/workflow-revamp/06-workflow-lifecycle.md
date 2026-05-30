# 06 — Workflow Lifecycle

脑爆结论笔记(2026-05-27)。
2026-05-31 改向 durable execution(详 [`00-overview.md`](./00-overview.md))。

依赖纲领:[`00-overview.md`](./00-overview.md) 的 durable-execution 模型(事件日志 journal + 确定性重放)+ `Workflow.active` / `FlowRun.IsFromListener` 字段。本篇只讲 lifecycle(上线/下线/触发/恢复);执行底盘细节见 00 与 [`11-integration-chains.md`](./11-integration-chains.md)。

---

## 没有"Deployment"抽象层

行业里 K8s / n8n / Airflow / GitHub Actions / Lambda 都用 **workflow 自身的 active 状态**表达"上线",**不引入额外的 Deployment 实体**。Forgify 跟进这个标准做法。

数据模型就 2 个 entity + 2 个 flag:

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
        Kind ∈ {"workflow", "flowrun"}
```

---

## Active 状态的产品语义

| 状态 | 行为 |
|---|---|
| `active = true` | 扫 workflow graph 中所有 **listener 类型** 的 trigger 节点(cron / fsnotify / webhook / polling),调 `triggerService.RegisterTrigger`。listener 开始监听 |
| `active = false` | 撤所有 listener;销毁 `Owner={Kind:"workflow"}` 的所有 handler instance |

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
scheduler.StartRun(workflowId, triggerNodeId, payload, isFromListener) → runID
```

3 套入口汇聚:

| 入口 | `isFromListener` | 谁组装 `(triggerNodeId, payload)` |
|---|---|---|
| Listener 自动触发(cron / fsnotify / webhook / polling) | `true` | listener |
| UI 用户点 trigger 节点 + 填表单 → `POST /workflows/{id}:trigger` | `false` | 用户 |
| AI 调 `trigger_workflow(workflowId, triggerNodeId, payload)` 工具 | `false` | LLM(按节点 payloadSchema)|

**`IsFromListener` 决定 handler Owner**:
- `true` → `Owner = {Kind: "workflow", ID: workflowId}` —— active workflow 内跨触发复用 handler instance
- `false` → `Owner = {Kind: "flowrun", ID: flowrunId}` —— per-flowrun 独立 instance

详 [`03-tool-node.md`](./03-tool-node.md) handler 生命周期段。

---

## Activate 流程

```
POST /workflows/{id}:activate
   │
   ├── workflow.active = true
   ├── 读 active version 的 graph
   ├── 扫所有 listener 类型 trigger 节点(cron / fsnotify / webhook / polling)
   ├── 对每个调 triggerService.RegisterTrigger({
   │      WorkflowID, NodeID, Kind, Config, UserID
   │   })
   └── listener 开始监听
```

handler instance 此时**不创建**——lazy 等首次触发时 acquire 时再 spawn。

---

## Deactivate 流程

```
POST /workflows/{id}:deactivate
   │
   ├── workflow.active = false
   ├── triggerService.UnregisterByWorkflow(id)   ← 撤所有 listener
   └── handlerRegistry.DestroyOwner({Kind:"workflow", ID: id})
       ↑ 销毁 active workflow 内的所有 handler instance
```

正在跑的 flowrun(从 listener 触发的)继续跑完——deactivate 不强杀,只是不接新 listener 触发。

---

## AcceptPending 联动

```
AcceptPending(workflowId, pendingVersionId)
   │
   ├── 现有逻辑:翻 active_version_id / 清 needs_attention / trim 老版本
   │
   └── 新加:如果 workflow.active:
       ├── triggerService.UnregisterByWorkflow(id)         ← 撤老 listener
       ├── handlerRegistry.DestroyOwner({Kind:"workflow", ID})  ← 撤老 instance
       └── 重做 activate 流程(扫新 graph,重新注册 listener)
```

正在跑的 flowrun 继续跑完(用老版本的逻辑);新 listener 触发的 flowrun 用新版本。**用户感知零停机**。

---

## Boot 恢复

Boot 要恢复两件正交的东西:**(A) 没跑完的 flowrun**(durable execution 的重放)和 **(B) active workflow 的 listener**(入口重挂)。

```
Process boot
   │
   ├── (A) 扫 status ∈ {running, awaiting_signal} 的 flowrun
   │       └── 对每个:从头确定性重放程序(详 00-overview「崩溃重放」段)
   │             ├ 命中事件日志的 activity → 直接抄日志里的结果(不重跑 LLM/工具)
   │             ├ 走到第一个没记账的步骤 → 真跑一次、记账,接着往下走
   │             └ awaiting_signal(approval 挂着)→ 重放回到挂起点继续等信号
   │                 (信号到达也是一条日志事件;无需在内存里持有 cancel handle —
   │                  挂起点就是日志里的 signal_awaited,重放天然恢复)
   │
   └── (B) 扫所有 workflow.active = true 的 row
           └── 对每个重做 activate 流程
               └── listener 重新注册
```

(A) 替代旧设计里"扫 paused flowrun + 重建内存态 + 重新 drive DAG"——不再有 PausedState / ExecutionContext 的内存快照需要重建,**唯一真相是事件日志**,重放即恢复。approval 的"挂着等人"由 `awaiting_signal` 状态 + 日志里的 `signal_awaited` 事件表达,重放到该点自然停下等信号,**不依赖任何进程内的 cancel handle**。详 [`05-approval-node.md`](./05-approval-node.md)。

(B) 的 listener 重挂照旧:cron 用 `lastFire` 重新算补跑(现有机制),webhook listener 重挂 mux,fsnotify 重新 watch,polling 重启 tick。

进程崩溃 → 重启后未完成的 flowrun 接着跑、active workflow 的 listener 自动恢复,**用户感知 ≤ 进程启动时间**。

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
       
   → scheduler.StartRun(..., isFromListener=false)
   → handler instance Owner = {Kind:"flowrun"},per-flowrun 独立
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
| `Workflow.active bool` 字段 | DB migration 一列 |
| `FlowRun.trigger_node_id TEXT` + `FlowRun.is_from_listener bool` | DB migration 两列 |
| `POST /workflows/{id}:activate` / `:deactivate` HTTP action | `handlers/workflow.go` + `app/workflow/lifecycle.go` 新 ~50 行 |
| activate:扫 graph 提取 trigger 节点 → `RegisterTrigger` | ~30 行 |
| deactivate:`UnregisterByWorkflow` + `DestroyOwner({workflow})` | ~5 行 |
| trigger Service `onFire` → `scheduler.StartRun(..., isFromListener=true)` | ~5 行 |
| `dispatch_handler.go` Owner 双模式(根据 IsFromListener) | 4 行 if |
| AcceptPending 末尾:active 时撤 + 重 register | ~15 行 |
| `RehydrateOnBoot` 扩展(B):扫 active workflow 重 register listener | ~20 行 |
| Boot 重放(A):扫 running/awaiting_signal flowrun → 照事件日志确定性重放接着跑(替代旧的扫 paused + 重建 PausedState/ExecutionContext)| 属执行引擎,见 [`11-integration-chains.md`](./11-integration-chains.md) |
| `:trigger` HTTP body 加 `triggerNodeId` 字段(替代隐式 manual) | ~10 行 |
| `trigger_workflow` LLM 工具描述加 `triggerNodeId` 必填参数 + 暴露 workflow 的 trigger 节点 list | ~30 行 |
| 砍 `local-user` magic / 默认 manual:必须显式指定 triggerNodeId | 0 行(自然 by-product) |

**总 ~170 行代码,估 1.5-2 天**(含测试)。

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
  Owner=workflow,handler instance 跨触发复用,state 持续

日常使用:
  AI 调 trigger_workflow(wfId, manualNodeId, payload)
    → 触发 manual trigger 节点 → Owner=flowrun,独立 instance
    → 跟 active workflow 内的 listener-触发 完全隔离

下线:
  AI 调 :deactivate → listener 停 + workflow-Owner instance 销毁
  Manual 触发仍可用(workflow 还在就能调)
  workflow 不删,inactive 状态保存

进程重启:
  Boot 扫所有 active workflow → 自动重 register listener
  未完成的 flowrun(running / awaiting_signal)→ 照事件日志确定性重放接着跑
    (awaiting_signal 的 approval 重放到挂起点继续等信号,无需内存 cancel handle)
  整体感知 ≤ 进程启动时间
```

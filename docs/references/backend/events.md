---
id: DOC-010
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
audience: [human, ai]
---

# 事件 —— SSE 流挂载 / 通知类型登记

> 流式产出的单一事实源。随评审逐域填入；当前已落：function · handler · agent · workflow · flowrun · trigger · control · approval。
> 通则（E 系列）：全系统**仅三条 SSE 流**（messages / entities / notifications，E1 永不再加）；workspace 级、后端不过滤；delta/tick 标 `seq=0` ephemeral（E2）；messages 流 `parentBlockId` 嵌套（E3）。任何实体**不开新流**——只把内容挂上三条流。

## notifications 流（生命周期通知，`<domain>.<action>`）

| 域 | 事件 |
|---|---|
| function | `function.{created, edited, reverted, updated, deleted, env_rebuilt}` |
| handler | `handler.{created, edited, reverted, updated, deleted, env_rebuilt, restarted, config_updated, config_cleared}` |
| agent | `agent.{created, edited, reverted, updated, deleted}` |

> `updated` = meta 变更（不升版本）；`edited` = 新版本生效；`env_rebuilt` = 空 ops 的 edit 重建了 active env。

## entities 流挂载（实体面板实时呈现，SSE-C）

| 域 | 挂载 |
|---|---|
| function | **run 终端**：每次执行的实时 stderr（= 函数自己的 `print()`，driver 引流）→ function scope；**forge 镜像**：create/edit_function 的流式 code args → 面板实时填充 |
| handler | **run 终端**：流式 method 的每个 yield → handler scope（不论谁触发）；**forge 镜像**：create/edit_handler 的类代码 |
| agent | **run 轨迹**：invoke 的完整 ReAct block 流（text/reasoning/tool_call/tool_result）→ agent scope（不论 chat/REST/workflow 触发）；**forge 镜像**：create/edit_agent 的 config |

## messages 流挂载（对话内呈现）

| 域 | 挂载 |
|---|---|
| function | `run_function` tool_call 下的 progress 块 = 执行的实时 stderr；create/edit 的 env-fix 尝试逐步流出 |
| handler | `call_handler` tool_call 下的 progress 块 = 流式 method 的 yield |
| agent | `invoke_agent` tool_call 下**嵌套** agent 的全部流式 block（E3 `parentBlockId`）——仅流式呈现，耐久记录是 Execution.transcript |

## P3 五域挂载

**notifications**：workflow/trigger/control/approval 的 `<域>.{created, edited, reverted, updated, deleted}` 生命周期族（workflow 另有 lifecycle 流转随 activate/deactivate/kill 的状态变更通知）。

**entities 流**：
| 域 | 挂载 |
|---|---|
| workflow | **flowrun 节点进度**：advance 每节点终态发一条 Signal（`{flowrunId, nodeId, iteration, status}`）→ workflow scope——面板实时看 run 逐节点推进；forge 镜像（create/edit_workflow 的图 ops） |
| trigger | **fire 信号**：每次扇出（全 4 源 + manual）发 `{activationId, kind, fired, firingCount, error}` → trigger scope；durable 记录 = Activation/Firing 行 |
| control / approval | forge 镜像（create/edit 的 branches/template） |

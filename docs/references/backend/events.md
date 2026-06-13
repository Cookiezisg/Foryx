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

> 流式产出的单一事实源。随评审逐域填入；当前已落：P0-P6 全部 32 域。
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
| function | **run 终端**：每次执行的实时 stderr（= 函数自己的 `print()`，driver 引流）→ function scope；**forge 镜像**：create/edit_function 的流式 code args → 面板实时填充；**env 物化终端**：每次 ensureEnv 的尝试/修复行（不分入口——HTTP 编辑器/chat 锻造/run 重建）→ forge 节点 |
| handler | **run 终端**：流式 method 的每个 yield → handler scope（不论谁触发）；**forge 镜像**：create/edit_handler 的类代码；**env 物化终端**：同 function |
| agent | **run 轨迹**：invoke 的完整 ReAct block 流（text/reasoning/tool_call/tool_result）→ agent scope（不论 chat/REST/workflow 触发）；**forge 镜像**：create/edit_agent 的 config |

## messages 流挂载（对话内呈现）

| 域 | 挂载 |
|---|---|
| function | `run_function` tool_call 下的 progress 块 = 执行的实时 stderr；create/edit 的 env-fix 尝试逐步流出 |
| handler | `call_handler` tool_call 下的 progress 块 = 流式 method 的 yield |
| agent | `invoke_agent` tool_call 下**嵌套** agent 的全部流式 block（E3 `parentBlockId`）——仅流式呈现，耐久记录是 Execution.transcript |

## P3 五域挂载

**notifications**：workflow/control/approval 的 `<域>.{created, edited, reverted, updated, deleted}` 生命周期族；workflow 另有 `workflow.lifecycle_changed`（activate/deactivate/kill 的状态流转，payload {lifecycleState, active}）、`workflow.attention_changed`（payload {needsAttention, attentionReason}——调度器自愈语义：run 失败点亮、completed 熄灭，无 acknowledge 端点）、`workflow.run_failed`（payload {workflowId, flowrunId, error}）与 `workflow.approval_pending`（payload {workflowId, flowrunId, nodeId}，at-least-once——唤人决策）。trigger **无**生命周期通知（其活动经 activations 行 + entities 流 fire 信号呈现）。

**entities 流**：
| 域 | 挂载 |
|---|---|
| workflow | **flowrun 节点进度**：advance 每节点终态发一条 Signal（`{flowrunId, nodeId, iteration, status}`）→ workflow scope——面板实时看 run 逐节点推进；forge 镜像（create/edit_workflow 的图 ops） |
| trigger | **fire 信号**：每次扇出（全 4 源 + manual）发 `{activationId, kind, fired, firingCount, error}` → trigger scope；durable 记录 = Activation/Firing 行 |
| control / approval | forge 镜像（create/edit 的 branches/template） |

## P4 三域挂载

**notifications**：`skill.{created,updated,deleted}` · `mcp.{installed,updated,removed,reconnected}` 族 · `document.{created,updated,moved,deleted}`。

**entities 流**：mcp = CallTool 的进度通知 tee 到 server scope 的 run 终端（per-call token 关联）；skill/document = forge 镜像（create/edit 的 body/content）。

**messages 流**：mcp 动态工具（`mcp__*__*`）的进度作为 tool_call 下 progress 块。

## P5 对话运行时族挂载

**messages 流（主战场）**：message_start/stop（durable，close 带快照）· 块级 open/delta/close（text/reasoning/tool_call/tool_result/progress 实时流，E2 delta=ephemeral）· **interaction 信号**（ephemeral——broker pending 表是真相、重连走 REST 重同步）· todo 信号 · subagent 子树经 `Open.ParentID` 嵌套（E3）。

**notifications**：`conversation.auto_titled` · `memory.{created, updated, deleted}` · `sandbox.env_status_changed`（payload 含 env/状态）· `sandbox.env_deleted` · 上传/删除类生命周期。

## P6 支撑域挂载

**notifications 流本体**：notification.Emit = DB 行 + durable 信号（scope=notification:<id>，node.type=事件类型）——流是推送、行是真相。
**entities 流本体**：entitystream 是全部实体面板活动的唯一生产原语（open→delta*→close / Signal）。
**messages 流**：humanloop 的 interaction ephemeral 信号（chat 注入 Surface）。

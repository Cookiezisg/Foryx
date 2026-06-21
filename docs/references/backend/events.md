---
id: DOC-010
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-14
review-due: 2026-09-14
audience: [human, ai]
---

# 事件 —— SSE 流挂载 / 通知类型登记

> 流式产出的单一事实源。
> 通则（E 系列）：全系统**仅三条 SSE 流**（messages / entities / notifications，E1 永不再加）；workspace 级、后端不过滤；delta/tick 标 `seq=0` ephemeral（E2）；messages 流 `parentBlockId` 嵌套（E3）。任何实体**不开新流**——只把内容挂上三条流。

## Frame 协议与 node.type 词表（非穷举）

三流共用统一帧 envelope `{seq, scope:{kind,id}, id, frame}`，`frame` 是四动词封闭联合。`durable` 报告该帧是否进 replay 环（重连可重建）；`seq=0` = ephemeral、不入 buffer、不产生背压。**durable 帧入订阅者 buffer（`bufSize+256`）；buffer 满 = 客户端卡死（durable 低频、满即真卡）→ 发布方断开该订阅者（关 `done`、幂等）、它重连并从环重放（缺口超环走 REST 重取）——绝不让一个卡死客户端永久卡住整工作区扇出 + 堆积所有 producer（R5）。**

| 动词 | durable | 说明 |
|---|---|---|
| `Open` | 恒 durable | 建节点（`parentId` 空=顶层，非空=嵌套挂载点，E3） |
| `Delta` | 恒 ephemeral | 给开着的节点追加流式 chunk（token 文本 / 终端输出） |
| `Close` | 恒 durable | 结束节点；`result` 携最终快照——流式节点的重连真相（delta 可丢） |
| `Signal` | **由 `Ephemeral` 字段定** | 不建树的点状广播；`Ephemeral` 不上线缆，仅定投递语义 |

**Signal 的 durable/ephemeral 硬规则**："DB 行才是真相、流只为实时呈现"的点状广播 MUST 置 `Ephemeral:true`：**flowrun 节点 tick**（`run`，flowrun_nodes 行是真相）、**trigger fire**（`fire`，Activation/Firing 行是真相）、**chat interaction**（broker pending 表是真相、重连走 REST 重同步）。必达的 **notification 信号**置 `Ephemeral:false`（durable——流是推送、行是真相但不可丢）。

`Node.Type` 词表由 **producer 定**（domain 不枚举类型），下表登记**当前全集**、非穷举：

| 流 | node.type 当前全集 |
|---|---|
| entities | `build`（create/edit 内容镜像）· `run`（执行中间产出 / flowrun tick）· `fire`（trigger 扇出）· `status`（ephemeral：mcp 连接态转移） |
| messages | `message`（start/stop，durable 带快照）· `text` · `reasoning` · `tool_call` · `tool_result` · `progress`（块级 open/delta/close）· `interaction`（ephemeral 信号）· `todo`（信号） |
| notifications | node.type = 事件类型字符串 `<domain>.<action>`（见下方各域登记） |

## notifications 流（生命周期通知，`<domain>.<action>`）

| 域 | 事件 |
|---|---|
| function | `function.{created, edited, reverted, updated, deleted, env_rebuilt}` |
| handler | `handler.{created, edited, reverted, updated, deleted, env_rebuilt, restarted, config_updated, config_cleared, crashed}` |

> `crashed` = 常驻进程在某次 `:call` 时被发现已死（manager 下次调用回收+重启）——让 handler 行此刻亮红点，而非等下个 :call 才暴露。payload `{handlerId}`。
| agent | `agent.{created, edited, reverted, updated, deleted}` |

> `updated` = meta 变更（不升版本）；`edited` = 新版本生效。`env_rebuilt`（空 ops 的 edit 重建了 active env）只在 **function / handler** 发，agent 不发。

## entities 流挂载（实体面板实时呈现，SSE-C）

| 域 | 挂载 |
|---|---|
| function | **run 终端**：每次执行的实时 stderr（= 函数自己的 `print()`，driver 引流）→ function scope；**build 镜像**：create/edit_function 的流式 code args → 面板实时填充；**env 物化终端**：每次 ensureEnv 的尝试/修复行（不分入口——HTTP 编辑器/chat 构建/run 重建）→ build 节点 |
| handler | **run 终端**：流式 method 的每个 yield → handler scope（不论谁触发）；**build 镜像**：create/edit_handler 的类代码；**env 物化终端**：同 function |
| agent | **run 轨迹**：invoke 的完整 ReAct block 流（text/reasoning/tool_call/tool_result）→ agent scope（不论 chat/REST/workflow 触发）；**build 镜像**：create/edit_agent 的 config |

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
| workflow | **flowrun 节点进度**：advance 每节点终态发一条 **ephemeral** Signal（`{flowrunId, nodeId, iteration, status}`）→ workflow scope——面板实时看 run 逐节点推进；flowrun_nodes 行是真相、tick 不占 replay 环（E2）；build 镜像（create/edit_workflow 的图 ops） |
| trigger | **fire 信号**：每次扇出（全 4 源 + manual）发 **ephemeral** Signal `{activationId, kind, fired, firingCount, error}` → trigger scope；durable 记录 = Activation/Firing 行（信号丢弃无妨） |
| control / approval | build 镜像（create/edit 的 branches/template） |

## P4 三域挂载

**notifications**：`skill.{created,updated,deleted}` · `mcp.{installed,updated,removed,reconnected}` 族 · `document.{created,updated,moved,deleted}`。

**entities 流**：mcp = CallTool 的进度通知 tee 到 server scope 的 run 终端（per-call token 关联）+ **`status` 信号**（**ephemeral**：连接态转移 connecting→ready / ready↔degraded / →failed，发 `{status, prevStatus, lastError}` → server scope，使 MCP 行状态点实时变色；mcp_servers 行是重连真相、信号丢弃无妨，只在真变化时发，不入 buffer E2）；skill/document = build 镜像（create/edit 的 body/content）。

**messages 流**：mcp 动态工具（`mcp__*__*`）的进度作为 tool_call 下 progress 块。

## P5 对话运行时族挂载

**messages 流（主战场）**：message_start/stop（durable，close 带快照）· 块级 open/delta/close（text/reasoning/tool_call/tool_result/progress 实时流，E2 delta=ephemeral）· **interaction 信号**（ephemeral——broker pending 表是真相、重连走 REST 重同步）· todo 信号 · subagent 子树经 `Open.ParentID` 嵌套（E3）。

**notifications**：`conversation.{created, updated, deleted, archived, unarchived, pinned, unpinned, auto_titled, model_override, compacted}`（`updated` = 仅改 title/systemPrompt/attachedDocuments 的默认动作；archived/unarchived·pinned/unpinned 为 toggle 动作；`compacted` payload {coversUpToSeq, summaryBytes}——压缩器写）· `memory.{created, updated, deleted}` · `sandbox.env_status_changed`（payload 含 env/状态）· `sandbox.env_deleted` · 上传/删除类生命周期。

## P6 支撑域挂载

**notifications 流本体**：notification.Emit = DB 行 + durable 信号（scope=notification:<id>，node.type=事件类型）——流是推送、行是真相。
**relation**：`relation.dependency_broken`（payload `{deletedKind, deletedId, dependents:[{kind,id,name,edge}]}`）——删一个被依赖的实体时，`PurgeEntity` 在 purge 抹边**前**快照其入向 equip/link 依赖、purge 后发 **ONE 聚合**通知点名（hydrate + 去重）这些被留下悬空挂载的实体。是 F160 瞬时 delete-tool 提示的**持久**对应物：经通知中心在任意删除路径（HTTP 或 LLM 工具）触达、跨重启留存（F161）。刻意用通知、非实体 attention 标志（agent 无 attention 列、workflow run-attention 仅在 run 完成时清会永久点亮）。无依赖 / nil emitter → 不发；hydrate/emit 失败只记录、绝不让删除失败。
**entities 流本体**：entitystream 是全部实体面板活动的唯一生产原语（open→delta*→close / Signal）。
**messages 流**：humanloop 的 interaction ephemeral 信号（chat 注入 Surface）。

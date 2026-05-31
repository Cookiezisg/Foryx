# 10 — AI 工具清单

脑爆结论笔记(2026-05-29)。
2026-05-31 改向 durable execution(详 00-overview)。

依赖:[00-overview](./00-overview.md) / [02-agent-node](./02-agent-node.md) / [03-tool-node](./03-tool-node.md) / [06-workflow-lifecycle](./06-workflow-lifecycle.md) / [07-error-handling](./07-error-handling.md) / [09-agent-domain](./09-agent-domain.md)。

---

## 总览

| 类别 | 工具数 | 现状已有 | 全新 | 改造 |
|---|---|---|---|---|
| Forge 实体锻造(function/handler/agent/workflow) | ~44 | 32 | 11(agent 全新) | ~3(kind 参数) |
| Workflow lifecycle | 3 | 1(改造) | 2 | 1 |
| 运行时观察 | 5 | 2 | 3 | 0 |
| 错误诊断 + 修复 | 5 | 0 | 5 | 0 |
| 资产管理(MCP/skill/document/memory) | 18 | 18 | 0 | 0 |
| 主对话基础 | ~14 | 14 | 0 | 0 |
| **总** | **~89** | **67** | **21** | **~5** |

---

## Forge 实体锻造(Quadrinity 对齐,4 套对称)

### Function(11 个,现状 11 已有 + kind 参数改造)

| 工具 | 解决问题 |
|---|---|
| `search_functions(query, kind?)` | 找 function(按 name / tags / desc,可按 kind 过滤) |
| `get_function(id)` | 看详情(active version 代码 + signature) |
| `get_function_versions(id)` | 看历史版本 |
| `create_function(name, kind, code, pollingInterval?)` | 造新 function — **kind 必填**:`normal` 或 `polling` |
| `edit_function(id, ops)` | 改 function(ops 数组,可改 kind) |
| `accept_pending_function(id)` | pending → active |
| `revert_function(id, targetVersion)` | 退回老版本 |
| `delete_function(id)` | 删 |
| `run_function(id, args)` | 试跑(polling 时平台模拟 lastCursor) |
| `search_function_executions(id, since)` | 看历史调用 |
| `get_function_execution(executionId)` | 看某次详情 |

### Handler(12 个,现状已全有)

| 工具 | 解决问题 |
|---|---|
| `search_handlers(query)` | 找 |
| `get_handler(id)` | 看 class 定义 + init schema + methods schema |
| `get_handler_versions(id)` | 看历史版本 |
| `create_handler(name, code, initSchema, methodsSchema)` | 造 |
| `edit_handler(id, ops)` | 改 |
| `accept_pending_handler(id)` | pending → active |
| `revert_handler(id, targetVersion)` | 退回 |
| `delete_handler(id)` | 删 |
| `call_handler(id, method, args)` | 试调 method(init 一次 instance,然后 call) |
| `update_handler_config(id, config)` | 更新 init args / secrets(AES-GCM 加密存) |
| `search_handler_calls(id, since)` | 看历史调用 |
| `get_handler_call(callId)` | 看某次详情 |

### Agent(11 个全新)

详 [`09-agent-domain.md`](./09-agent-domain.md)。

| 工具 | 解决问题 |
|---|---|
| `search_agents(query)` | 找 |
| `get_agent(id)` | 看 prompt / skill / knowledge / tools / model / outputSchema |
| `get_agent_versions(id)` | 看历史版本 |
| `create_agent(name, prompt, skill?, knowledge[], tools[], model?, outputSchema?)` | 造 |
| `edit_agent(id, ops)` | 改 |
| `accept_pending_agent(id)` | pending → active |
| `revert_agent(id, targetVersion)` | 退回 |
| `delete_agent(id)` | 删 |
| `run_agent(id, payload)` | 试跑(给输入看输出 + tokens / 耗时) |
| `search_agent_executions(id, since)` | 看历史调用 |
| `get_agent_execution(executionId)` | 看某次详情 |

### Workflow(10 个,现状 9 已有 + 改造)

| 工具 | 解决问题 |
|---|---|
| `search_workflows(query, active?)` | 找(可按 active 过滤) |
| `get_workflow(id)` | 看 graph(nodes + edges) |
| `get_workflow_versions(id)` | 看历史 |
| `create_workflow(name, graph)` | 造(初始 v1 auto-accept) |
| `edit_workflow(id, ops)` | 改 graph(add_node / remove_node / connect / disconnect / update_config 等 ops) |
| `accept_pending_workflow(id)` | pending → active |
| `revert_workflow(id, targetVersion)` | 退回 |
| `delete_workflow(id)` | 删 |
| `capability_check_workflow(id)` | 预校验 callable 存在 + kind 匹配 + handler `.method` 在 active version 还在 + 必填参数给了值(**不查类型**;full payload 类型流**已砍**——payload 动态无类型,运行时由 N1 + case fail-to-false(G9)兜)+ 图良构/可归约(并行分支自包含、循环单入口)。深度详 [`11`](./11-integration-chains.md) §G |

---

## Workflow Lifecycle(3 个)

| 工具 | 解决问题 | 现状 |
|---|---|---|
| `activate_workflow(id)` | 上线 — 注册 listener,workflow.active=true | ❌ 新增 |
| `deactivate_workflow(id)` | 下线 — 撤 listener(停起新 flowrun);在途 flowrun 跑完时各自 `DestroyOwner({Kind:"flowrun", ID:flowrunId})` 自清独占实例。无 workflow 级共享实例需销毁、无 refcount | ❌ 新增 |
| `trigger_workflow(id, triggerNodeId, payload)` | **统一触发**:产品调用(manual 节点) + 调试(cron / webhook 等其他节点 + mock payload);Owner 恒为 `{Kind:"flowrun", ID:flowrunId}`(每次触发独占隔离实例,无 IsFromListener 分支、无跨触发共享实例) | ⚠️ 改造(加 triggerNodeId 必填) |

---

## 运行时观察(5 个)

一次 flowrun = 把图当结构化程序确定性跑一遍,每步结果记进事件日志(journal);下列工具读这本日志(详 [00-overview](./00-overview.md) 持久化段)。

| 工具 | 解决问题 | 现状 |
|---|---|---|
| `search_flowruns(workflowId, status?, since?)` | 列 flowrun 历史(状态 / 时间过滤;status ∈ running / awaiting_signal / completed / failed / cancelled) | ✅ 已有 |
| `get_flowrun(id)` | flowrun 概况(status / trigger / 耗时) | ✅ 已有 |
| `get_flowrun_trace(id)` | **看事件流**(读事件日志:按 seq 的因果序列 —— 每步开始/结果、分支选择、信号;画布滴答的数据源) | ❌ 新增 |
| `get_flowrun_nodes(id)` | 看每节点状态(running / completed / failed / approval pending) | ❌ 新增 |
| `cancel_flowrun(id)` | 取消一次正在跑或挂起的 flowrun | ❌ 新增 |

---

## 错误诊断 + 修复(5 个全新,AI 工程师能力核心)

durable-execution 下没有"死信消息"——失败的是**某个 activity(节点 / 那一轮)**,它的"开始没记成结果"留在事件日志里;诊断 = 读日志找到失败步,修复 = `replay`(重放跳过已记账步、重跑未完成的)。

| 工具 | 解决问题 | 现状 |
|---|---|---|
| `query_events(workflowId, type?, since?)` | 查事件流(跨 flowrun 查事件日志 type:`handler_crash` / `node_failed` / `trigger_exhausted`) | ❌ 新增 |
| `list_failed_steps(workflowId, since?)` | 列失败步(retry 用尽、永久失败的 activity / 节点) | ❌ 新增 |
| `get_failed_step(flowrunId, nodeId, iterationKey?)` | 看失败步详情(该步输入 + ctx + 失败原因 + stack trace;iterationKey 区分循环不同轮) | ❌ 新增 |
| `replay_flowrun(flowrunId, fromNode?)` | replay — 重放该 flowrun:命中日志的步直接抄结果,从失败步(或 `fromNode` 指定节点)起真跑;不重复已成功的 LLM/工具调用 | ❌ 新增 |
| `clear_failed_steps(workflowId)` | 批量清失败步标记(放弃重跑、归档) | ❌ 新增 |

> 命名变更:旧设计称这五个为 `list_dead_letters` / `get_dead_letter(messageId)` / `replay_message(messageId, fromNode?)` / `clear_dead_letters`,把失败建模成"死信 message"。durable 模型下失败建模成"失败的记账步",签名改用 `flowrunId + nodeId`(而非 messageId)。语义对齐 [`07-error-handling.md`](./07-error-handling.md):列失败步 / replay 重跑 / 读事件日志。

---

## 资产管理(现有,不动)

| 套 | 工具 | 用途 |
|---|---|---|
| **MCP**(5) | search_mcp_tools / call_mcp_tool / list_mcp_servers / install_mcp_from_registry / health_check_mcp | 第三方能力接入 |
| **Skill**(3) | search_skills / get_skill / activate_skill | 主对话 + agent entity 引用 |
| **Document**(7) | search / list / read / create / edit / move / delete | knowledge 挂载 / 知识库 |
| **Memory**(3) | read_memory / write_memory / forget_memory | 跨对话长记忆(chat 老板用) |

---

## 主对话基础(现有,跟 workflow 无关)

| 套 | 工具 |
|---|---|
| **文件** | Read / Write / Edit / Glob / Grep |
| **Shell** | Bash / BashOutput / KillShell |
| **Web** | WebFetch / WebSearch |
| **任务** | TodoCreate / List / Get / Update |
| **交互** | AskUserQuestion / Subagent |
| **元** | activate_tools(lazy 加载) |

---

## 风险等级标注(LLM Best Practice 重点研究方向)

按"LLM 用得好不好"排,不是按"功能复杂度":

| 等级 | 工具 | 风险点 |
|---|---|---|
| 🔴 高 | `create_function(kind=polling)` / `edit_function`(改 polling code) | LLM 写 cursor 容易出 race / 漏事件 / 重复;需要平台提供 cursor 模板库 + 详细教学 prompt |
| 🔴 高 | `edit_workflow`(复杂 ops 数组) | LLM 长程推理弱,5+ 节点 workflow 容易 ops 顺序乱 / 漏 edge / payload schema 不对齐 / 画出不可归约回边 |
| 🟡 中 | `get_flowrun_trace` + `query_events` + `get_failed_step`(多步诊断 chain) | LLM 多次工具调用时"短期记忆"容易丢;需要 TodoCreate 辅助记录 |
| 🟡 中 | `edit_handler`(改 state 持久化代码) | thread safety / state persistence LLM 容易疏漏;需要 forge 教学 prompt 强约束 |
| 🟡 中 | `edit_workflow` 改 case 节点(CEL expression) | CEL 不是 LLM 主流训练数据,复杂 expression(null safety / has() / 嵌套)易错 |
| 🟢 低 | search / get / 简单 create / version 管理 / activate / replay | LLM 做得很顺,已在 Phase 3-4 验证 |

---

## 待验证:Best Practice 研究(2026-05-29 晚上起)

用户提供 LLM API key 后,真跑 Claude / GPT 测每个工具,产出每个工具的 Best Practice:

1. **工具 description prompt** 怎么写,让 LLM 一眼看懂用途 / 参数语义 / 何时调
2. **参数 schema 设计** — 必填 / 可选 / enum 候选 / 默认行为
3. **错误信息怎么 actionable** — LLM 看到错就知道下一步怎么做
4. **跟其他工具的 chain 模式** — 多工具组合时的最佳调用顺序
5. **特别针对高风险工具的 LLM 兜底**:
   - polling cursor 模板库
   - CEL 表达式辅助 validator
   - 复杂 workflow ops 的拆分指引
   - 多步诊断的 TodoCreate 配套

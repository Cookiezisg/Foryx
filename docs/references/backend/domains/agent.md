---
id: DOC-007
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-14
review-due: 2026-09-14
audience: [human, ai]
---

# agent —— 配置好的 LLM worker（Quadrinity 第四元）

## 1. 定位

Agent **自己不写代码**：它是一份"LLM 员工配置"——提示词 + **按引用挂载**的能力（fn_/hd_/mcp 工具、skill、文档、模型覆盖），运行时跑共享的 ReAct 引擎（`app/loop.Run`）。代码层级：`domain/agent`（2 文件）→ `app/agent`（8 文件 + invoke 是核心）→ `infra/store/agent` + `app/tool/mount`（挂载合成，agent 专属机制）+ `app/tool/agent`（10 工具）。

## 2. 心智模型

**三个对象**：`Agent`（身份 + active 指针）→ `Version`（不可变快照：**prompt · skill(0-1) · knowledge(docIDs) · tools(ToolRef[]) · inputs/outputs 声明 · modelOverride**——可变配置全在版本上）→ `Execution`（一次运行的审计行 + **transcript**）。

**版本模型与 function/handler 同构**（方案 A，见 [function.md](function.md)#2），但有两个**有意分化**：
- **编辑是全量 Config 快照替换、非 ops**：agent 配置是声明式字段（无代码体），整体替换语义清晰；ops 增量是为代码体设计的。
- **name 不强制 slug**：function/handler 的 name 是代码标识符（Python 入口/类名，强制 `^[a-z][a-z0-9_-]{0,63}$`）；agent 的 name 是展示身份（"You are <Name>"），可中文/空格。

**`ToolRef{Ref, Name}`**：Ref 合法集 = `fn_<id>` / `hd_<id>.<method>` / `mcp:<server>/<tool>`；**禁 `ag_`**（员工不调员工，domain `ValidateTools` 在 create/edit 时拒；create_agent 的 tools 描述指路：**串联 agent 改用 workflow agent 节点**）。Name 是挂载时存的展示名——运行时**一律按现名重新解析**（实体改名后工具自动用新名）。

**transcript 是核心持久化决策**：agent 运行的完整 block 序列（text/reasoning/tool_call/tool_result 跨步）序列化存进 `Execution.transcript`——**自包含的耐久记录，不进共享 message_blocks 表**。chat 内实时呈现走流（嵌套在 invoke_agent tool_call 下），reload 后前端从 transcript 重水合。

## 3. 五类挂载的运行时语义（invoke 时逐项生效）

| 挂载 | 运行时形态 | 机制 |
|---|---|---|
| `fn_<id>` | 以 **function 现名**命名的绑定工具 | desc/inputs schema 来自活实体；Execute → `RunFunction`(TriggeredBy=agent) |
| `hd_<id>.<method>` | `<handlerName>__<method>` 绑定工具 | method spec → schema；Execute → `handler.Call`(agent)，yield 流进 progress |
| `mcp:<server>/<tool>` | `mcp__server__tool` 绑定工具 | 经**在线** server 解析（离线即失败）；Execute → `mcp.CallTool`(agent) |
| skill 名 | **执行指南**注入 system prompt（`## Execution guide` 段） | `skillapp.Guide`：渲染正文；**不**设 active-skill（防预授权泄漏父对话）、**不** fork；**create/edit 期 eager 校验存在**（同一 `Guide` 解析、不存在 → `AGENT_SKILL_NOT_FOUND`，免 dangling 名建出 dead-on-arrival agent，F96） |
| knowledge docIDs | 知识前缀拼进 user 消息 | `BuildKnowledgePrefix`：缺失 doc **大声失败** `AGENT_KNOWLEDGE_NOT_FOUND`（不再 GetBatch WhereIn 静默丢——免 dangling/已删 doc 静默丢 grounding 却报 ok，F98） |

**核心设计**：agent **永不**见通用系统工具表（无 `run_function`/`Read`/`Bash`）——工具宇宙**恰是其挂载**，每个工具预绑定目标（LLM 没有自由 id 参数可乱走）。合成在 `app/tool/mount`：`Resolver` 持三个窄端口（FunctionPort/HandlerPort/MCPPort，DIP、测试可 fake），按 ref 前缀分流出三种绑定工具。

**fail-fast**：目标被删（冒具体码如 `FUNCTION_NOT_FOUND`）/ method 不存在（`HANDLER_METHOD_NOT_FOUND`）/ MCP server 离线 / ref 格式坏 / 两挂载合成同名（撞名检测）→ **invoke 失败**（mount 自身问题 = `AGENT_MOUNT_INVALID`）。worker 缺声明能力**绝不静默降级跑**。**create/edit 期 eager 校验全挂载 ref**（skill F96 · knowledge/tool F98——经 invoke 的**同一** resolver：skill→`Guide`、knowledge→`BuildKnowledgePrefix`、tool→`CheckHealth`，不存在即在 build 期拒，免 dangling ref 建出 dead-on-arrival/静默降级的 agent；domain `ValidateTools` 只校格式不校存在）。

**挂载健康预检**（`Resolver.CheckHealth` + `GET /agents/{id}/mount-health`）：Resolve 的按需、**非 fail-fast** 对应物——逐挂载独立解析、收集每条状态（`MountHealth{ref,name?,healthy,error?}` + `allHealthy`），用同一批 per-ref 解析器**且同样做撞名检测**（两挂载合成同名时第二个标 unhealthy——与 Resolve 对称，故此处坏的正是 invoke 会拒的那个；否则单独可解析、合起来撞名的挂载会过 eager 校验却每次 invoke 0 步即败=DOA agent）。给 UI 在 invoke 前红点预警；List 不投影（逐 agent 逐挂载 N+1 不划算，按需单 agent 才对）。

## 4. Invoke 生命周期（所有路径唯一漏斗，对标 RunFunction）

```
InvokeAgent(in)
  ├─ 取 version（空→active；无 active → AGENT_NO_ACTIVE_VERSION）
  ├─ runLoop:
  │   ├─ knowledge 前缀 + v.Prompt + Input(JSON 块) → user 消息
  │   ├─ mounts.Resolve(v.Tools) → 绑定工具（fail-fast）
  │   ├─ skill.Guide(v.Skill) → system prompt 的执行指南段
  │   ├─ Resolver.ResolveAgent(modelOverride) → LLM bundle（nil=默认 agent 场景模型）
  │   ├─ ctx 装饰：tool_call 嵌套（E3）+ entities 流 agent scope 镜像（SSE-C）
  │   ├─ ctx WithTimeout(limits.Timeout.AgentInvokeSec) — 整次运行墙钟封顶
  │   └─ loop.Run(agentHost, maxTurns 默认 10 − 已重放步数)
  └─ recordExecution（Detached ctx，best-effort；status 按运行 ctx.Err() 区分 timeout/cancelled）
```

- **InvokeDeps**（DIP 后注入：Resolver/Mounts/Skill/Knowledge/EntitiesBridge）——"挂载了某能力却 nil 对应依赖" = 装配 bug，invoke **大声失败**（不静默跳过）。
- **agentHost 实现 loop.Host**：LoadHistory = prompt + 重放步；Tools = 预合成挂载；**WriteFinalize = no-op**（agent 经 Execution 落账、非消息历史）；RecordStep 在装了 recorder 时按绝对回合下标记账。不实现 AutoActivator/ReminderProvider（无 search_tools 扩张、无 todo reminder——worker 聚焦）。
- **system prompt 组装**：身份（"You are <Name>… Your role: <Description>"）+ worker 纪律（只用给的工具）+ skill 指南段 + **outputs 硬约束**（声明了 Outputs → "最终答案必须是恰含这些字段的单个 JSON"；未声明 → 自由作答）。
- **outputs 回解析（声明输出非 advisory，F40）**：invoke 后把终答**解析回命名字段 map**（容忍 ```json 围栏 + jsonrepair），使下游 workflow 节点读 `node.<字段>` 而非整段塞进 `node.text`。终答非对象时：**恰 1 声明** → 裹进该名（自由文本→单输出便利）；**2+ 声明** → 无法拆成多字段、报 `AGENT_OUTPUT_NOT_STRUCTURED` 大声失败（节点行写 failed，非静默交废 text）。未声明 → 原样 `node.text`。**与 fn/hd 非对称**：function/handler 的 dispatch 侧 `toResultMap` 不接声明输出（标量→`.text`、声明输出对其为 advisory——返回 dict 才得 `node.<字段>`），唯 agent 在 invoke 处回解析。
- **三条触发路径**：chat 的 `invoke_agent` 工具（TriggeredBy=chat；toolCallID 设进 ctx 使流式 block 嵌套其下）/ HTTP `:invoke`（manual）/ workflow agent 节点 `dispatch.RunAgent`（workflow；**粗粒度 activity**——只记忆化最终 result，`ReplaySteps/Recorder/FlowrunID` 等 InvokeInput 字段是子步重放的预留，调度器 v1 留空）。
- **运行墙钟（与 fn/hd/mcp 同款）**：`runLoop` 给 `loop.Run` 套外层 `WithTimeout(limits.Timeout.AgentInvokeSec)`（默认 900s，`PATCH /limits` 可调、校验 >0）。`InvokeMaxTurns` 只封轮数、不封时间，慢 agent（轮数 ×（LLM idle + 每工具等待））在单条 workflow drain 协程上同步跑会饿死所有 workspace 的排空 + 审批超时——墙钟是补位安全帽。**status 映射**：超时为权威信号、压过 loop 自报终态（ctx 取消时 loop 报 cancelled 非 error），运行 ctx `DeadlineExceeded` → `ExecutionStatusTimeout`（durable、可 `:replay`），`Canceled` → `ExecutionStatusCancelled`；记录仍落 Detached ctx。
- **溯源**：conversation/message/toolCall 从 ctx；flowrun **InvokeInput 显式字段优先、ctx 注入兜底**（调度器派发前 `reqctx.SetFlowrunID`）。
- **人在环**：ctx 带 humanloop broker（chat 回合的 broker 自然流进子运行）时，自报 dangerous 的工具在共享 loop 的 danger 门**阻塞**至用户 resolve——嵌套不冒泡，阻塞的 goroutine 天然 hold 整个栈。
- **状态判定**：runErr → failed；loop 结果 StatusError → failed；其余 ok。tokens/steps/stopReason 在 `InvokeResult` 同步返回（**不持久化**——留全局观测议题；transcript 已含全过程）。

## 5. 关键设计决策

- **挂载合成 vs 过滤注册表**：不是"从全局工具里挑"，是"为每个挂载造一个绑定工具"——这使 agent 的能力面=配置面，且系统工具物理上进不来。
- **skill 作指南而非激活**：`Guide` 渲染正文（展开 `${CLAUDE_SESSION_ID}`，不接 `$ARGUMENTS`/位置参数）注入 system prompt；不写 AgentState 的 active-skill（那会把 allowed-tools 预授权泄漏给父对话）、不触发 fork（指南就是给本次运行的文本）。
- **无 sandbox 依赖**：agent 不写代码——是唯一没有 env/物化链路的执行实体。
- **TriggeredBy 无 "agent"**：员工不调员工（与 ToolRef 禁 ag_ 同一条公理的两面）。
- **subagent 与 agent 实体无关**：subagent 是 chat 内 spawn 的隔离 loop 运行（固定动词工具白名单、落 sub-message）；agent 是持久化实体。两者只共享 loop 引擎。故 trace 查回也分两路：`get_agent_execution` 读 `agent_executions` 表（实体 run），`get_subagent_trace` 读父对话的 sub-message（inline subagent 无表行，见 [messages.md](messages.md)）——别混。

## 6. 契约（引用）

端点 → [api.md](../api.md)#agent · 表 → [database.md](../database.md)#agent · 码 → [error-codes.md](../error-codes.md)（domain `AGENT_*` 11 + 工具校验 6）· 事件 → [events.md](../events.md)。LLM 工具 10 个：search/get/create/edit/revert/delete_agent + invoke_agent + 执行日志查询（search_agent_executions + get_agent_execution）+ **`update_agent_meta`**（只改 row 的 name/desc/tags、不铸版本——name/desc/tags 不在版本化 config 内，edit_agent 改不了它们，纯改名/改述用它）；create/edit 是 build 工具（config 镜像 entities 流）。

## 7. 跨域集成

- **invoke 被谁调**：chat loop / HTTP / workflow 调度器（`AgentInvoker` 窄接口）。
- **mount 端口指向**：functionapp / handlerapp / mcpapp 的具体服务（bootstrap 装配 `mount.NewResolver(fn, hd, mcp)`）。
- **relation 双向同步**（出 + 入；workflow/skill/trigger 同样双向，function/handler 仅入向）：**出边** equip（挂载的 fn/hd/mcp/doc/skill，每次 active 变更重算——hd 剥 `.method`、mcp 剥 `/tool` 归到容器实体）+ **入边** create/edit（构建对话，create 和 edit 分 kind-scope 共存）。
- **catalog 非容器**：只报 name+desc，**不报 Members**——挂载是内部白名单、非 agent 的可调子单元（对比 handler/mcp 的容器形态）。
- **@ 提及**：快照 name+description（这个员工是干什么的——供模型谈及/转交）。

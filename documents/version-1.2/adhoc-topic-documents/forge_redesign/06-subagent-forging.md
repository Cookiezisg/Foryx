# 多 Agent 并行锻造

**关联**:
- [`00-overview.md`](./00-overview.md) — 顶层愿景(D8 多 agent / D21 sub-agent 不控 workflow)
- [`01-shared-tool-interface.md`](./01-shared-tool-interface.md) — LLM 工具接口
- 现状 — 复用现有 subagent 基础设施(D3-D4 已落地)

---

## 1. 决策回顾

**topic 2(多 agent 并行锻造)收成"配置 + prompt 工程",不是新 architecture**(D8)。基础设施已就位:

- `app/subagent/` Service.Spawn 一站式 ✓
- 内置 SubagentType 注册表(目前 3 类:Explore / Plan / general-purpose)✓
- 5min 总超时 + panic recover + 双保险防递归 ✓
- `app/loop` ReAct 引擎共享 ✓
- chat.message SSE 携带 `subagentRunId`,前端按 ID 分流到流式小窗 ✓

**本次重做要做的就 3 件事**(全是配置 / prompt,不引入新 SubagentType):

1. **`general-purpose` subagent 的 filterTools 加 strip workflow ops**(D21)— ~10 行 Go
2. 主 agent system prompt 教学(经 catalog generator 注入)— ~30 行 prompt
3. 协作模式约定("主 agent 负责 wire,sub-agent 负责零件")

> **不引入新 SubagentType** — 用现有 `general-purpose`(继承父 tool registry,minus Subagent + workflow ops)+ 主 agent 写 prompt instruction 控制行为。比之前推的"4 forger types"简化得多。

---

## 2. 不加 SubagentType — 纯 prompt-driven 角色

复杂多模块需求时,主 agent spawn 多个现有 subagent type,每个通过 **prompt instructions** 拿到不同任务:

| 角色(prompt 描述,**不是** type)| 用哪个现有 type | 任务 |
|---|---|---|
| "decomposer" | **`Explore`**(纯只读,已存在)| 分析需求 → 出 forging plan → 只用 search_* 工具 |
| "function-forger" | `general-purpose` | 锻造一个 Function;完成后 run_function 自测;返 functionId |
| "handler-forger" | `general-purpose` | 锻造一个 Handler;完成后 call_handler 自测;返 handlerId |
| "workflow 装配" | **不用 sub-agent 做** | 主 agent 自己干(D21 — workflow ops 主 agent 独享)|

**好处**:
- `app/subagent/registry.go` **0 行改动**
- LLM 心智:跟现有 Subagent 工具一致,只是 prompt 内容因任务而异
- 灵活 — 后续加新角色不需要 spec 改 / 代码改

---

## 3. 主 Agent System Prompt 教学

`app/catalog/generator.go` 生成的 catalog summary 加一段(或在 chat 主 agent 的 system prompt 模板里直接加):

```text
You have multi-agent forging capabilities via the Subagent tool.

When the user requests something involving 3+ independent forgeable modules
(e.g., "build a workflow that does X, Y, Z, each needing its own Function
or Handler"), CONSIDER spawning subagents in parallel:

1. (Optional) Spawn `Subagent(type="Explore", prompt="analyze + produce a
   forging plan; use search_* tools only, do NOT forge anything")` — returns
   a structured plan listing what Functions / Handlers are needed.

2. Spawn N `Subagent(type="general-purpose", prompt="forge ONE specific
   atom: ...")` IN PARALLEL (LLM-self-reported execution_group=1 to get
   them parallel-batched). Each subagent forges a Function or Handler,
   runs self-test (run_function / call_handler), returns the entity ID.

3. Wait for all subagents to return.

4. CHECK CONFIG GATE: get_handler / get_function for each new entity, check
   configState. If unconfigured / partially_configured → use AskUserQuestion
   to collect missing init_args, then call update_handler_config to persist.
   Only proceed when all references show configState="ready".

5. **YOU YOURSELF assemble the workflow** — call create_workflow + apply ops
   directly. Sub-agents have NO workflow ops by design (D21); they can't
   create / edit / trigger workflows. Workflow assembly is your job.

6. trigger_workflow to dry-run, report results to user.

For SIMPLE requests (single Function edit, one-line Handler tweak), DO IT
YOURSELF. Don't spawn subagents for trivial work — token cost is N× higher.
```

放进 catalog generator template,自动喂主 agent。

---

## 4. 协作模式 — "主 Agent 负责 wire,sub-agent 负责零件"

类比 OS 进程模型:**子进程产零件,父进程负责装配**。**且父进程独享高级 orchestration ops**(workflow domain — D21)。

### 4.1 协作示例(邮件 workflow)

```
用户:"做个监听邮箱 workflow,猎头存 DB,发 WhatsApp"

主 agent:
  ① spawn Subagent(type="Explore", prompt="分析需求 + 出 forging plan,只用 search_*")
     → 返 plan: [
         "需要 handler: gmail-listener (init: api_key)",
         "需要 handler: pg-recruiter (init: dsn)",
         "需要 function: send-whatsapp (HTTP)"
       ]
  
  ② 并行 spawn 三个 general-purpose subagent(同 batch,execution_group=1):
     subA prompt: "锻造一个 Handler 叫 gmail-listener,init=api_key,
                   methods: list_unread/mark_read,call_handler 自测,返 handlerId"
     subB prompt: "锻造一个 Handler 叫 pg-recruiter,init=dsn,
                   methods: query/insert,自测,返 handlerId"
     subC prompt: "锻造一个 Function 叫 send-whatsapp,HTTP POST 到 WhatsApp Business API,
                   run_function 自测,返 functionId"
     
  ③ 等三 sub 完成,各返 ID
  
  ④ Config 引导(D16 / D20):
     get_handler(gmail-listener / pg-recruiter)看 configState
     ↓ 各缺 api_key / dsn(unconfigured)
     ↓ 主 agent 用 AskUserQuestion 收集
     ↓ update_handler_config 写回
     ↓ 全 ready 后进 ⑤
  
  ⑤ **主 agent 自己** create_workflow + apply ops 装配:
     - trigger 节点(cron 0 */1 * * *)
     - handler 节点 gmail-listener.list_unread
     - loop 节点(items=emails)
       - 子图:llm 判断 + condition + handler 写 DB
     - http 节点(WhatsApp 通知)
     
  ⑥ **主 agent 自己** trigger_workflow 试跑 → 报告用户
```

注意 ⑤ 和 ⑥ —— **主 agent 直接调 workflow ops**,不再有 "workflow-forger" sub-agent。Sub-agent 物理上看不到 `create_workflow` / `trigger_workflow` 等(D21 strip)。

### 4.2 视觉端 — 现有基础设施已支持

- `chat.message` SSE 已携带 `subagentRunId`(D4 落地)
- 前端 chat.js 已按 subagentRunId 分流到**流式小窗**
- 三个 forger 并行 → 三个流式小窗同时呼啦呼啦地造各自的部分
- 主对话区显示主 agent 的协调内容 + 主 agent 装 workflow 时的 ops 流(在主 conv 直接看,挂 tool_call 父下)

**零新协议** — 现有事件日志协议 + ops-driven 流式 + chat.message subagentRunId 路由就够了。

---

## 5. 防御机制

### 5.1 防递归(已有)

- **结构性 filterTools 剥离**:sub-runner 的 tool registry 物理不含 SubagentTool 自身
- **运行时 ctx depth check**:depth ≥ 1 时 SubagentTool.CheckPermissions 拒

Sub-agent 不能再 spawn sub-agent — 锻造场景只需一层。

### 5.2 防 workflow 越权(D21,新加)

`general-purpose` subagent 继承父 tool registry 时,**filterTools 额外 strip workflow ops**:

```go
// app/loop/tools.go 或 wherever filterTools lives
var subagentStrippedTools = []string{
    "Subagent",            // 防递归(已有)
    "create_workflow",     // D21 — workflow 装配主 agent 独享
    "edit_workflow",
    "delete_workflow",
    "revert_workflow",
    "trigger_workflow",    // D21 — workflow 触发主 agent 独享(避副作用)
}

func filterTools(tools []toolapp.Tool) []toolapp.Tool {
    return slices.DeleteFunc(slices.Clone(tools), func(t toolapp.Tool) bool {
        return slices.Contains(subagentStrippedTools, t.Name())
    })
}
```

**保留**:`search_workflow` / `get_workflow`(只读,sub-agent 可参考现有 workflow context)。
**保留**:`call_handler` / `run_function`(sub-agent 锻造完自测必需)。

### 5.3 哲学层面 — 跟 D7 同根

| 决策 | 限制 | 原因 |
|---|---|---|
| **D7** | Function 不调 Handler | Function 是叶子,组合靠 workflow 编排 |
| **D21** | sub-agent 不控 workflow | sub-agent 是工具人(锻造 / 测试零件),装配靠主 agent |

**"显式 domain 分工 > 默认全权限"** 是这套设计的统一精神。

---

## 6. Caveats

### 6.1 Token 成本

每个 sub 是独立 LLM 调用,N 个并行 = N 倍 token。
- 简单需求(单 Function 改一行 / 单 Handler 加一个 method)主 agent 直接干
- 中等需求(2-3 个零件)考虑 spawn
- 大需求(3+ 个零件 + workflow 装配)spawn 收益最大

### 6.2 失败级联

任一 sub 失败 → 主 agent 决定:
- 重试该 sub(同样的 spec)
- 跳过 + 调整 plan(主 agent 自己改 wire 方式)
- 整体失败 + 报告用户

现有 SubagentRun.Status (`completed` / `failed` / `max_turns` / `cancelled`)足够表达。

### 6.3 Catalog 自然同步

多 sub 同时锻造产物,catalog 1s polling 自然感知所有新增。主 agent 下一轮 LLM 调用时通过 catalog summary 看到新建的 Function / Handler ID。

### 6.4 跨 sub 共享上下文 — V1 不做

主 agent → sub 通过 `SpawnOpts.Prompt` 传递 spec。
sub → 主 agent 通过 `SpawnResult.Output` 返回 ID + 测试结果。
**不支持跨 sub 互发消息** —— 各 sub 隔离 context,V1 不打算引入 sub 间通讯。

---

## 7. 实施清单

简单,**只 2 处改动**:

### 7.1 filterTools 加 workflow ops strip(D21)

`app/loop/tools.go`(或 sub-agent 调用 filterTools 处)加 `subagentStrippedTools` slice + 校验逻辑。**~10 行 Go**。

### 7.2 主 Agent system prompt 注入

`app/catalog/generator.go` system prompt template 加 §3 的 multi-agent 教学段。**~30 行**(prompt 内容)。

### 7.3 测试

| 测试 | 覆盖 |
|---|---|
| `app/loop/tools_test.go::TestFilterTools_StripsWorkflowOps` | sub-agent 看不到 workflow mutation / execution ops |
| `app/loop/tools_test.go::TestFilterTools_KeepsSearchAndGetWorkflow` | 只读 workflow ops 保留 |
| `app/loop/tools_test.go::TestFilterTools_KeepsCallHandlerAndRunFunction` | sub-agent 仍能自测 |
| `test/subagent/parallel_forging_test.go` | E2E:多模块需求 → Explore decomposer + parallel forger + 主 agent 自装 workflow |

---

## 8. 主要风险

| 风险 | 缓解 |
|---|---|
| 主 agent 无脑 spawn 浪费 token | system prompt 明确"简单需求自己干";LLM 自我节制 |
| sub-agent prompt 中漏说 "self-test" | LLM 自然倾向自测,且 §3 prompt 模板明示要求 |
| 三个 forger 改同一 entity 冲突 | V1 假设各 forger 锻造**独立** entity(不同 name);冲突时后到的报 `NAME_DUPLICATE` |
| forger sub 跑超时(maxTurns) | 现有 5min 总超时 + maxTurns 限制;主 agent 收 `max_turns` status 决定补 |
| forger 写出的 Function / Handler 测试失败 | forger 内部 run_function / call_handler 自测;失败 sub 返 `status=failed`,主 agent retry / 调整 spec |
| sub-agent 试图调 workflow ops | filterTools 物理 strip 后 LLM 调时 "tool not found",自然回路;主 agent prompt 明示"workflow 我自己干" |

---

## 9. 实现工作量估

- filterTools strip workflow ops:~10 行 Go + ~30 行单元测试
- 主 system prompt 注入(catalog generator template 改):~30 行 prompt
- pipeline test(parallel forging):~150 行

**总 ~200 LOC** —— 比原设计 ~600 LOC 大幅简化(0 新 SubagentType,纯 prompt + 一行 strip list)。

---

## 10. 文档同步

实施时:
- `service-design-documents/subagent.md` § 加 "Forger collaboration mode + workflow ops strip(D21)"
- `progress-record.md` 加 dev log
- backend-design.md domain/subagent 树注明扩展(无新 type 注册;只是 filterTools 加 strip + system prompt 加教学段)

---

(本文档完)

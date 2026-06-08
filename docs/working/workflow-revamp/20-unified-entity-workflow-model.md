---
id: WRK-001-20
type: working
status: draft
owner: @weilin
created: 2026-06-08
reviewed: 2026-06-08
review-due: 2026-09-08
audience: [human, ai]
supersedes: [18-graph-model-redesign, 19-workflow-module-design]
---
# 20 — 统一实体模型 + Workflow（大统一设计 · 终稿）

> **本文是 workflow 这一块的唯一事实源**，收编并取代 `18`（图模型）与 `19`（workflow 模块设计）。
> 它把「每个实体怎么定义」「每个节点怎么跑」「workflow 怎么编排」「已建的东西怎么回炉」一次说清，供细致审核。
> durable 执行底盘（journal / replay / pin）仍以 `17` 为准，本文 §6.6 给执行模型概览 + 引用 `17`。

---

## 0. 一句话总纲

> **一切皆「带声明 I/O 的纯调用」。** 每个**实体** = `output = f(input)`，有声明的输入、声明的输出、一坨逻辑，且**不认识 workflow**。每个 workflow **节点** = 「调一个实体：把命名空间里祖先节点的 result 按名喂成它的 input，拿回 output 挂自己名下」。**边只画控制骨架，数据靠按名引用。**

五条铁律：

1. **节点 = 一次调用**：`result = entity(input)`。5 种节点同一个形状，只在「出口」不同。
2. **数据按名引用祖先**：下游写 `reviewer.score`、`getOrder.total`，从 journal 已记账的祖先 result 取。无隐式 merge、无 payload 穿透、无工作流变量。
3. **边 = 控制骨架**：激活 / 先后 / 分支 / 循环。**边不搬数据。**
4. **实体不认识 workflow**：实体只认自己的 `input`/`output`，因此可复用、可独立锻造、可版本化 + pin。
5. **控制 / 审批 = 结果带路由标签**：control 输出 `(枚举, 数据)`、approval 输出 `(yes/no, 数据)`；**标签驱动走哪条边、由 workflow 决定连去哪**，数据照常按名引用。

---

## 1. 通用实体契约

不管 function、handler、agent、control、approval，每个实体长同一个样：

```
实体 = {
  ID, Name, Description          // 身份（catalog 可见）
  inputSchema  : [FieldSpec]     // 我要哪几个输入、叫什么、什么类型
  outputSchema : [FieldSpec]     // 我产出哪几个字段、叫什么（下游据此接线）
  逻辑                            // input → output 的具体实现（各实体不同）
  版本: 单调版本号 + active 指针   // 同 function；pin 所必需（在途 run 不漂移）
}

type FieldSpec struct {
    Name        string  // 字段名
    Type        string  // string|number|integer|boolean|object|array
    Description string
    Required    bool
}
```

- **workspace 隔离、软删、errorsdomain** 同既有实体准则。
- **可引用永远指 active**（编排语义=永远 prod）；**在途执行 pin 具体版本**（17 §1 / ADR-020）。
- **catalog**：进（name + description）。**relation**：进（`workflow → 实体` 引用边 + 引用计数 + 改 active 反查）。

---

## 2. 通用节点契约

**节点 = 对实体的一次调用。** 三件事：按名取祖先 result → 拼成实体 input → 拿回 output 挂自己名下。

```go
type Node struct {
    ID    string            // 图内局部 id（如 "reviewer"）—— 也是它 result 的引用名
    Kind  string            // trigger | action | agent | control | approval
    Ref   string            // 引用的实体：trg_ / fn_|hd_.method|mcp: / ag_ / ctl_ / apf_
    Input map[string]string // field → 裸 CEL（从命名空间取数喂 input）；trigger 空（外部注入）
    Retry *RetryConfig      // 仅 action：平台 activity retry（非业务循环）
    Pos   *Position         // UI 布局
    Notes string
}
```

**两个轴，别混：**

| 轴 | 是什么 | 谁有 |
|---|---|---|
| **数据轴** | `Input`（喂进去）+ `result` 的具名字段（吐出来），下游按名引用 | **所有节点** |
| **控制轴** | 出口（决定接下来走哪条边） | action/agent/trigger=单出口；**control=N 枚举出口**；**approval=yes/no 出口** |

> control 一个节点：左边吃 `{score, attempt}`（数据 in）、右边吐 `{draft, feedback}`（数据 out）、**另外**选一个枚举决定走哪条边（控制）。数据和控制分开。

---

## 3. 边 + 作用域

- **边 = 控制骨架**：`A → B` 表示「B 在 A 之后才有资格跑 + B 能看见 A」。control 的边带枚举名（`fromPort=enum`），approval 的边带 `yes`/`no`，回边表示循环。**边不携带数据。**
- **数据 = 按名引用祖先**：节点的 `Input` CEL 里 `A.x` 按上游节点名取 result 字段。
- **可见性 = 传递祖先**：B 能引用 A ⟺ A 是 B 的（传递）控制祖先。所以 `trigger.x` 全图可读，不用一层层穿。
- **自动推出「数据依赖 ⟹ 先后」**：想用 A 的数据 → A 必是祖先 → 必有边 → B 排在 A 后。想只跟在 A 后（先扣款再发货，无数据）→ 画边不引用即可。A、B 真并行 → 无边 → 互相看不见（正确）。
- **作用域内容**：求值节点 N 的 Input 时，scope = `{ <祖先节点id>: 其 result, … } + { ctx: { runId } }`。`ctx` 只剩真·环境（runId）；trigger 进了命名空间，无 `ctx.trigger` 特例。
- **循环里的可见性**：同循环内祖先 → 当前轮（iteration）的 result；循环外祖先 → 其固定 result（17 §5）。

```go
type Edge struct {
    ID       string
    From     string // 源节点 id
    FromPort string // control: 枚举名；approval: yes|no；其余空
    To       string // 目标节点 id
}
```

---

## 4. 数据轴 vs 控制轴：5 节点全景

| 节点 | ref 实体 | input（数据 in） | result（数据 out） | 出口（控制） |
|---|---|---|---|---|
| **trigger** | `trg_` | 无（外部注入） | payload（如 `{orderId}`） | 单 |
| **action** | `fn_`/`hd_.m`/`mcp:` | callable 入参 | callable 返回 | 单 |
| **agent** | `ag_` | task + 结构输入 | agent outputSchema | 单 |
| **control** | `ctl_` | 判定输入（如 `{score, attempt}`） | **选中枚举的携带数据** | **N 枚举**（选一） |
| **approval** | `apf_` | 表单展示字段（如 `{amount, payee}`） | **`{decision, reason}`** | **yes/no**（选一） |

---

## 5. 逐个实体的具体逻辑

### 5.1 trigger（`trg_`，已建 R0039 · 轻确认）

- **角色**：程序入口（整次执行起点，非 activity）。图外是 `trg_` 信号源实体（cron/webhook/fsnotify/sensor 的监听 + firing 收件箱 + 引用计数）；图内是一个**入口节点引用它**。
- **input**：无 scope 输入；payload 由触发方按 `trg_` 的 `payloadSchema` 注入。**唯一输入不来自命名空间的节点**（它本就是源头）。
- **output**：`payload`（= `payloadSchema` 声明的字段）。它就是 trigger 节点的 result，下游按 `trigger.orderId` 读。
- **出口**：单。
- **回炉**：**无代码改动**。`payloadSchema` 已是它的 outputSchema；概念上确认「trigger 节点 result = 注入 payload」即可。

### 5.2 action（`fn_` / `hd_.method` / `mcp:`，已建 · 范本，不改）

- **角色**：调一个确定性 callable（activity，整步记账）。
- **引用**：`fn_`（函数）/ `hd_.method`（handler 方法）/ `mcp:server/tool`（MCP 工具）。**一个节点，三种子类型**——执行机制相同。
- **input**：`Input` map 每值裸 CEL，喂 callable 的入参。`fn_` 的 `parameters` **就是** inputSchema。
- **output**：callable 返回。`fn_` 的 `returnSchema` **就是** outputSchema。
- **出口**：单；可选 `retry`（平台 activity 级，扛瞬时故障，不带业务反馈）。
- **回炉**：**无**。`fn_`/`hd_`/`mcp` 天生就是 `input(params) → output(return)`，是全模型的范本。

### 5.3 agent（`ag_`，已建 R0043 · 轻确认）

- **角色**：调一个 LLM（activity，子步记账：每个 ReAct turn 记 `agent_step`）。
- **引用**：`ag_`。
- **input**：`Input` map 喂 agent 的任务 + 结构输入（agent 的 system prompt / 任务模板读 `input.*`）。
- **output**：agent 的 `outputSchema` 约束的结构结果。
- **出口**：单。
- **回炉**：**轻**。确认「agent 节点喂 input、产出 outputSchema 结果」。多半无代码改动（input→task 在 scheduler 轮接）；若要显式 `inputSchema` 声明可轻加。

### 5.4 control（`ctl_`，已建 R0045 · 回炉为「枚举选择器」）

- **角色**：确定性路由 + 重塑（纯控制，不调外部，不记 activity；选择记 `branch_taken`）。
- **本质**：一张 `(条件 → 枚举, 携带数据)` 的有序表。**算出该走哪个枚举，吐出那行的数据。**

```go
type ControlVersion struct {  // ctlv_
    Version     int
    InputSchema []FieldSpec    // 我要哪些判定输入（如 score, attempt）
    Branches    []Branch       // 有序：first-true-wins，末条 When="true" 兜底
}
type Branch struct {
    When string             // 布尔 CEL over input.*（选哪一行）
    Enum string             // 这一行的枚举名（workflow 据此路由；= 边的 fromPort）
    Emit map[string]string  // field → CEL over input.* → 这一行的携带数据（空=透传 input）
}
```

- **input**：节点喂 `{score: reviewer.score, attempt: prevGate.attempt}` → 实体读 `input.score` / `input.attempt`。
- **output**：选中行的 `(Enum, Emit 求值后的数据)`。**Enum 驱动路由**、**Emit 数据是本节点 result**（下游按名读 `gate.feedback`）。
- **出口**：N 个枚举，**只激活选中的那一个**（XOR）。**workflow 负责「枚举 → 连去哪个节点」，control 完全不知道下游节点**。
- **能力边界**：when/emit 仅确定性 CEL（读 input、禁 now()、禁外部）——不是轻量 function，不做任意计算。
- **回炉清单**（revision，非 rebuild —— 结构基本在）：
  1. domain：`Branch.Port` → `Branch.Enum`（语义=枚举，明确「workflow 路由它」）；`When`/`Emit` 的 CEL 环境从「读 scope（payload/ctx）」→「读 `input.*`」；`Version` 加 `InputSchema`。
  2. store：`control_logic_versions` 加 `input_schema`(json) 列。
  3. app：`validateBranches` 编译 when/emit 的环境变量改成 `input`；`Resolve` 返 `[]Branch` 签名不变。
  4. tool：`create/edit_control` 参数加 `inputSchema`。
  5. 契约：`domains/control.md` + database/api 更新。

### 5.5 approval（`apf_`，已建 R0046 · 回炉为 input 化 + 产出 result）

- **角色**：durable 挂起等人工信号 + 二元路由（park；记 `signal_awaited`/`signal_received`）。
- **本质**：一个**可复用的审批表单**——把数据渲染成人能看懂的审批点。

```go
type ApprovalVersion struct {  // apfv_
    Version         int
    InputSchema     []FieldSpec  // 我展示哪些字段（如 amount, payee）
    Template        string       // markdown，{{ input.* }} 插值
    AllowReason     bool
    Timeout         string       // duration；空=永不超时（唯一保留的 durable timer 用途）
    TimeoutBehavior string       // reject | approve | fail
}
```

- **input**：节点喂 `{amount: order.total, payee: order.payee}` → 模板渲染 `{{ input.amount }}`。
- **output**：`{decision: yes|no, reason: string}`。**decision 驱动路由**、`{decision, reason}` 是本节点 result（下游按名读 `approve.reason`）。**不透传上游数据**——下游要原数据直接读那个祖先（如 `order.total`），不经 approval。
- **出口**：固定 `yes` / `no`，**只激活选中的**。workflow 负责「yes/no → 连去哪」。
- **回炉清单**：
  1. domain：`Template` 的 `{{ CEL }}` 环境 → `input.*`；`Version` 加 `InputSchema`；明确 output=`{decision, reason}`。
  2. store：`approval_form_versions` 加 `input_schema`(json) 列。
  3. app：`validateForm` 编译 template 环境改 `input`；`Resolve` 返 `*Version` 签名不变。
  4. tool：`create/edit_approval` 加 `inputSchema`。
  5. 契约：`domains/approval.md` + database/api 更新。

---

## 6. Workflow 本体逻辑

### 6.1 数据模型

镜像 function：header（`wf_`）+ 单调版本（`wfv_`）+ active 指针；图存进 version 的 JSON blob。

```go
type Workflow struct {  // wf_
    ID, WorkspaceID, Name, Description string
    Tags            []string
    Active          bool    // 是否参与调度
    LifecycleState  string  // active | draining | inactive（17 §1）
    Concurrency     string  // serial | Skip | BufferOne | BufferAll | AllowAll
    NeedsAttention  bool
    AttentionReason string
    LastActionBy    string  // user | system（区分自动 deactivate，ADR-022）
    ActiveVersionID string
}
type WorkflowVersion struct {  // wfv_
    Version                int
    Graph                  string  // JSON（Graph 序列化）
    ChangeReason           string
    ForgedInConversationID *string
}
type Graph struct {
    Nodes []Node  // §2
    Edges []Edge  // §3
}
```

### 6.2 节点 config（per kind）

| kind | Ref | Input（接线） | 出边 fromPort |
|---|---|---|---|
| trigger | `trg_` | 空（外部注入） | 空 |
| action | `fn_`/`hd_.m`/`mcp:` | callable 入参 map | 空 |
| agent | `ag_` | task/结构输入 map | 空 |
| control | `ctl_` | 判定输入 map | **枚举名**（命中 ctl_ 的某 Enum） |
| approval | `apf_` | 展示字段 map | **yes / no** |

### 6.3 ops 编辑（AI 锻造）

编辑 = 一串 ops（AI 友好、可逐 op 推 forge SSE）：`set_meta` / `add_node` / `update_node`(merge patch) / `delete_node`(级联删边) / `add_edge` / `update_edge` / `delete_edge`。删了旧的 `set_variable`/`set_node_model_override`（统一模型无变量、模型归 ag_）。

### 6.4 校验（capability_check · 17 §8 的 ①②）

- **形状**：kind ∈ 5 枚举；ref 前缀匹配 kind；action 的 Input 值非空 CEL。
- **引用存在 + kind 对**：每个 ref 解析得到且 kind 匹配；`hd_.method` 的 method 在 active version 还在。
- **图良构**：节点/边 id 唯一、边两端存在、无自环、**≥1 trigger**、可达（无孤儿）。
- **环 = 仅可归约回边**（control 回边、单入口循环；`BackEdges()` 与解释器共用）。
- **出口对账**：control 出边的 `fromPort` 命中 ctl_ 的某个 `Enum`；approval 的 ∈ `{yes,no}`。
- **CEL 可编译 + 引用合法**：每个 Input/emit/when/template 能 `cel.Compile`；**且引用的节点 id 是本节点的祖先**（按名引用的可见性 lint）。

### 6.5 版本（无 pending）+ pin 闭包

- **无 pending**（同 function）：`Create`→v1 active；`Edit`(ops)→vN+1 **立即生效**；`Revert`→active 指针移历史版本。版本上限 50，trim 最旧（护 active）。
- **在途 run 不漂移**：靠 pin（§6.6），不靠 pending。**编辑期 workflow 设 `inactive`，改完显式 `:activate`**——安全靠 lifecycle，不靠暂存态。
- **pin 闭包**（`BuildPinClosure(graph)`，归 workflow 模块——它最懂图+引用解析）：走图收集所有 `node.Ref`，递归解析每个实体的 callable 依赖（agent 挂的 fn/hd），快照 `{entity_id: active_version_id}`（含 trg_/fn_/hd_/mcp_/ag_/**ctl_/apf_**；闭包深度 ≤2，ADR-020）。scheduler `StartRun` 单事务内调它。

### 6.6 执行模型（概览 · 深设计见 scheduler 文 + 17）

一次执行 = durable 解释器照**钉死的图**走一遍，全程 journal 记账、可崩溃重放：

1. **启动**：firing → 建 flowrun，钉 `version_id`（图拓扑）+ pin 闭包（所有引用实体版本）。
2. **走图**：从 trigger 起，agenda 驱动「ready 的 (节点, 轮次)」。每个节点：**从 journal 按名取祖先 result 拼 scope → 求值 Input → 调实体 → result 记 journal**。
3. **各节点机制**（执行轴三类）：
   - **action/agent（activity）**：dispatch → 记 `node_completed`（result）。重放命中 `node_completed` 抄结果、不重跑。
   - **control（纯控制）**：评 branches 选枚举 → 记 `branch_taken`（枚举 + emit 数据）→ 激活该枚举的出边。
   - **approval（park）**：渲染 template → 记 `signal_awaited` + 写 approvals 投影行 → 挂起（`status=awaiting_signal`）；人决策 → 记 `signal_received`（yes/no）→ 重驱解释器、激活该出边。
4. **join**：fork 汇合 = AND-join（等全部入边）；control 分支汇合 = active-branch join（只等被激活的枚举出边）——由上游 split 类型结构推导（17 §3）。
5. **循环**：control 回边 → 下游 `iteration+1`；回边的 emit 携带循环状态（attempt+1、feedback）。
6. **完成**：所有活跃路径到无后继出边节点、且无 parked → `completed`；活动耗尽 retry 仍失败 → `failed`，修好 `:replay`。

> journal schema、record-once（dedup_key）、replay 代数、approvals 表、firing 单事务 claim —— **全照 `17`**，本文不复制。

---

## 7. 执行机制轴（3 flavor · 与数据轴正交）

数据轴全统一（`result = entity(input)`），但**执行机制**故意保持三类，**别动**：

| 执行类 | 谁 | 机制 | journal |
|---|---|---|---|
| **activity**（记账、可重放） | action、agent | 调外部 | `node_started`→`node_completed`/`node_failed` |
| **纯控制**（确定性、不记账外部） | control | 选枚举 + 重塑 | `branch_taken` |
| **park**（挂起等信号） | approval | 渲染 + 等人 | `signal_awaited`/`signal_received` |

trigger = 入口（非 activity）。**数据轴统一、执行轴分三类，正交。**

---

## 8. 重做清单（每个东西怎么改 + 顺序）

| 东西 | 现状 | 改动 | 轮次 |
|---|---|---|---|
| **fn_** | `params→returnSchema` | **无**（范本） | — |
| **hd_.method** | `params→return` | **无** | — |
| **mcp tool** | `args→output` | **无** | — |
| **ag_**（R0043） | `task→outputSchema` | 轻确认（input→task；可选加 inputSchema） | 并入 workflow 轮 |
| **trg_**（R0039） | `→payload` | 轻确认（payload=result） | 并入 workflow 轮 |
| **ctl_**（R0045） | when/emit 读 scope | **回炉**：读 `input.*` + `InputSchema` + `Port→Enum` 语义（§5.4） | 回炉轮 A |
| **apf_**（R0046） | template 读 scope | **回炉**：读 `input.*` + `InputSchema` + output=`{decision,reason}`（§5.5） | 回炉轮 B |
| **workflow** | — | **新建**（§6 全套，relation 第 12 类 `wf_`） | R0047 |

**好消息**：fn/hd/mcp/ag/trg 这 5 个调用类实体**天生符合**（fn 是范本）。只有最后建的两个新实体 ctl_/apf_ 要回炉——根因是当初让它们直读 scope、绑死了图；改成读 input 后它俩就和 fn_ 一样 workflow-agnostic、可复用。

**顺序**（每步 verify + commit + push）：
1. 本文（doc 20）定稿。
2. **ctl_ 回炉**（轮 A）：domain/store/app/tool/测试/契约。
3. **apf_ 回炉**（轮 B）：同上。
4. **workflow 新建**（R0047）：domain → store → app（CRUD + ops + 校验 + BuildPinClosure + WorkflowReader）→ tool（7 个，无 :trigger/执行类）→ handler → relation 第 12 类 → 测试 → 契约。ag_/trg_ 轻确认并入此轮。
5. 之后：**flowrun**（journal/approvals 表）+ **scheduler**（durable 解释器 + Dispatcher 端口 + firing 消费）——另文设计。

---

## 9. 对已定契约的回写（doc-fix）

- **18-graph-model-redesign**：control 改「(枚举,数据) 选择器、路由在 workflow」；边=控制不搬数据；approval 产 `{decision,reason}`；§3 两个新实体加 inputSchema/outputSchema。
- **19-workflow-module-design**：§1 节点 `Args/Prompt` 合并为 `Input` map；§3 作用域 = 按名引用祖先（模型 B）。
- **17-execution-contract**：§5 作用域——trigger 进命名空间（删 `ctx.trigger`）、可见性=传递祖先、scope 从 journal 按 node_id 重建；§7 节点 config = `{kind, ref, input}`（内联 branches/prompt 删）；`branch_taken` result = `{enum, data}`。
- **`database.md` S15**：登记 `ctl_/ctlv_/apf_/apfv_/wf_/wfv_/fr_/fre_/apv_`。

---

## 端到端例子（写 → 审 → 改循环，一图打包全部概念）

```
trigger(start, trg_manual)              result {orderId}
  → action(draft, fn_writeDraft)        input {topic: start.topic}        result {text}
  → agent(reviewer, ag_review)          input {task: "审 "+draft.text}    result {score, reason}
  → control(gate, ctl_qualityGate)      input {score: reviewer.score, attempt: gate.attempt}
        枚举 pass  : when input.score>=0.9            emit {text: draft.text}
        枚举 retry : when input.attempt<3             emit {text: draft.text, attempt: input.attempt+1, feedback: reviewer.reason}
        枚举 else  : when "true"                      emit {text: draft.text}

  workflow 路由（边）：
     gate --pass--> action(publish, fn_publish)   input {text: gate.text}
     gate --retry-> [回边] action(draft)           input {topic: start.topic, feedback: gate.feedback}
     gate --else--> approval(human, apf_review)    input {text: gate.text}
        human --yes--> action(publish)
        human --no---> （结束）
```

- 每个节点 = **调实体 + 喂 input**；下游 **按名引用祖先**（`reviewer.score`、`gate.text`、`start.topic`）。
- `gate` 输出 `(枚举, 数据)`；**workflow 决定 pass/retry/else 各连去哪**，`gate` 不知道有 publish/draft/human。
- `retry` 回边的 emit 把 `attempt+1` + `feedback` 塞进数据，回到 draft——循环带状态。
- `human` 输出 `{decision, reason}`，workflow 按 yes/no 路由。

---

> **一句话定位**：从「CEL 要不要做实体」一路推到这里——**一切皆带声明 I/O 的纯调用，workflow 只是把这些调用按命名空间接起来 + 用控制骨架画分支循环。** fn/hd/mcp/ag/trg 已经是了，ctl/apf 回炉两轮，workflow 新建一轮，干净。

---
id: WRK-001-18
type: working
status: active
owner: @weilin
created: 2026-06-08
reviewed: 2026-06-08
review-due: 2026-09-08
audience: [human, ai]
landed-into:
---
# 18 — Workflow 图模型重定型（纯编排 · 节点引用实体）

> **本文是波次 4 workflow 图模型的新事实源**。它演进 / 局部推翻 00-05 的节点模型，统一 `00`/`17` 与 `DOC-125`（trigger 实体）之间关于「trigger 是不是节点」的冲突。durable 执行底盘（`17` 的 journal / replay / join / pin）**原样保留**，变的只是「图怎么表达」这一层。
>
> **为什么现在做**：波次 4（workflow / flowrun / scheduler）在 backend-new **一行没写**，改设计成本最低。旧文档（00-05/17/08/16/DOC-125）的系统收口见 §7 清单，本文先成总纲。

---

## 0. 为什么重定型（动机）

旧 5 节点模型（00-05）有三个真实的不一致，端到端推演时浮出来：

1. **控制流脚踩两条船**。00 同时引用 Temporal（「workflow = 程序，控制流是程序结构」）和 Step Functions（「可视化画布、可强制结构」），但在「控制流到底是不是节点」上选了 Temporal 的**下沉**——把 `parallel`/`loop`/`wait`/`variable` 砍掉，控制能力藏进**边的多寡**（多出边=并发）、**case 回边**（=循环）、**节点角上的 gate**（=延迟）。可画布需要**看图就懂**——控制藏在边和 gate 里，用户在图上**看不出控制流**。这是熵的来源。

2. **trigger 已经出图，文档没跟上**。backend-new 在波次 3（R0039）把 trigger 提成一等实体（`DOC-125`：`trg_`、catalog、relation 第 9 类、引用计数生命周期），16 的 A-3 也把它规约到「schedule 层、非图内 activity」。但 00/17 §7 仍写「trigger 是节点，config `{kind,spec}`」。**文档内部打架。**

3. **图既是编排又是逻辑容器**。control（CEL 选路+重塑）和 approval（把数据渲染成人能读的审批点）的逻辑**内联**在图里，AI 编排时「连拓扑」和「写逻辑」两种关注点混在一起，心智不解耦。

**重定型目标**：workflow 图回归**纯编排**（只连拓扑 + 数据流），所有逻辑/能力**物化成实体**，节点只**引用**实体。一套心智，零例外。

---

## 1. 心智总纲

> **workflow = 一张纯编排的数据依赖图。每个节点 = 引用一个实体 + 数据接线；边 = payload 数据管道。一次执行 = durable 解释器照图确定性走一遍 + journal 重放。**

### 5 节点 × 5 类引用实体

| 节点 | 引用的实体 | 执行时是 | 改名自 |
|---|---|---|---|
| **trigger** | `trg_` 信号源（图外，可共享） | 入口（非 activity） | 同名 |
| **action** | callable `fn_` / `hd_.method` / `mcp:` | activity（整步记账） | 原 `tool` |
| **agent** | `ag_` | activity（子步记账） | 同名 |
| **control** | `ctl_` 路由逻辑实体（新） | 纯控制（确定性，不记 activity） | 原 `case` |
| **approval** | `apf_` 审批渲染实体（新） | 等信号（durable park） | 同名 |

### 五条铁律

1. **节点 = 结构角色（图里干什么）；实体 = 可锻造逻辑（节点引用什么）。** 五节点全部「引用实体 + 接线」，无例外。
2. **接线 CEL 留节点，逻辑 CEL 进实体。** `action` 的 args（把上游数据映射成入参）是**接线**=线，内联；`control` 的整套 when/emit 决策、`approval` 的整套渲染规则是**逻辑坨**=东西，进实体。
3. **并发 = 拓扑涌现（免费，无 parallel 节点）；时间 = 归图外 trigger（无 wait 节点）。**
4. **判据**：图内执行机制相同 → 一个节点 + 子类型（`action` 的 fn/hd/mcp、`trigger` 的 4 kind）；机制根本不同 → 分立节点（`control` ≠ `approval`）。
5. **`control` 能力边界**：确定性路由 + 重塑，**不膨胀成任意计算**（那是 `function`）——物化只是把逻辑命名独立，不放大能力。

### 端到端例子（写 → 审 → 改循环，一图打包全部概念）

```
trigger(trg_manual)
  → action(fn_draft 写初稿)            ← 引用 function callable
  → agent(ag_reviewer 审核)            ← 引用 agent，出 {score, reason}
  → control(ctl_gate 判合格?)          ← 引用 control 逻辑实体
       ├ port=pass  : score>=0.9               → action(fn_publish 发布)
       ├ port=retry : score<0.9 && attempt<3   →[回边]→ action(fn_draft)
       │              emit: {draft, attempt: attempt+1, feedback: reviewer.reason}
       └ port=else  : "true"                    → approval(apf_humancheck 人工兜底)
```

- 每个节点**引用一个实体**；边传 **payload**；`control` 的 `retry` 出口是**回边**，emit **把审核反馈 + attempt+1 塞进回边**（数据管道携带循环状态）。
- `ctl_gate` 实体定义「有哪些出口 + 每个出口的 when/emit」；图只定义「每个出口连哪个下游」。**逻辑在实体，拓扑在图。**

---

## 2. 五节点逐个契约

每个节点 = `{ 引用的实体 ref, 输入接线（CEL）, 出口→下游 }`。

### 2.1 trigger
- **角色**：程序入口（整次执行起点，非 activity）。
- **引用**：`trg_` 信号源实体（图外，可被 N 个 workflow 共享）。
- **输入**：无（payload 由触发方按 `trg_` 的 payloadSchema 注入）。
- **出口**：单出口（把入口 payload 喂给后继）。
- **双身份**（化解 00 vs DOC-125 冲突）：图外是 `trg_` 实体（cron/webhook/fsnotify/sensor 的监听、firing 收件箱、引用计数）；图内是一个**入口节点引用它**——与 action/agent 引用实体**同构**。一个 workflow 可有多个 trigger 入口节点。

### 2.2 action（原 tool）
- **角色**：调一个确定性 callable（activity，整步记账）。
- **引用**：callable ref `fn_` / `hd_.method` / `mcp:`（**一个节点，fn/hd/mcp 是子类型，不拆三个节点**——执行机制 100% 相同，判据铁律 4）。`ag_` 不在此列——agent 单列（2.3）。
- **输入接线**：`args`，每个值是**裸 CEL**（接线），读上游 result 求值出类型化值（`count: A.x + 1` → 数字）。
- **出口**：单出口（结果广播给所有下游）；可选 `retry` / `timeout`（平台 activity 级，见 §5）。

### 2.3 agent
- **角色**：调一个 LLM（activity，**子步记账**：每个 ReAct turn/tool-call 记 `agent_step`）。
- **引用**：`ag_`。
- **输入接线**：prompt 走 `{{ CEL }}` 模板插值（文本字段）。
- **出口**：单出口（按 agent 的 outputSchema 约束的结果，广播）。
- 与 action 的分工：**action 调确定性能力（fn/hd/mcp），agent 调 LLM（ag_）**，不重叠。「用 action 当普通调用方式调 agent」的老用法**取消**（agent 只能是 agent 节点）——消除「两种方式调 agent」的复杂。

### 2.4 control（原 case）
- **角色**：确定性路由 + 重塑（纯控制，不调外部，不记 activity；选择记 `branch_taken`）。
- **引用**：`ctl_` 路由逻辑实体（§3.1）。**逻辑在实体**：一组有序出口，每个 = `{ when（布尔 CEL）, emit（重塑 CEL，可选）, port（出口名）}`；first-true-wins，末条 `when:"true"` 兜底。
- **图只定义**：每个 `port` 连哪个下游节点（含**回边**=连回上游=循环）。
- **CEL 控制输出的两层**：`when` 控制「数据往哪个出口流」（选路）；`emit` 控制「那个出口流什么数据」（重塑，含回边带 attempt+1/feedback）。
- **确定性**：when/emit 只读已记账值、禁墙钟 → `branch_taken` 记 emit 后的 payload，重放抄账不分叉。

### 2.5 approval
- **角色**：durable 挂起等人工信号 + 二元路由（非 activity；signal_awaited/received 记账）。
- **引用**：`apf_` 审批渲染实体（§3.2）。**逻辑在实体**：渲染规则（markdown 模板 + `{{ CEL }}` 插值）、`allowReason`、`timeout` / `timeoutBehavior`。
- **图只定义**：`yes` / `no` 两个固定出口各连哪个下游。
- **不改 payload**（透传）；决策只决定走哪个出口，reason 只进审计（不进数据流，对齐 05）。

---

## 3. 两个新实体的 domain 设计（「AI 工作实体」）

control 与 approval 的逻辑物化成实体。它们是一类**新的可见性级别 —— AI 工作实体**：进 catalog / relation / pin，但**不在用户主导航**（用户通过「AI 编排的结果」间接接触）。遵循既有实体准则（workspace 隔离、软删、`errorsdomain`）。

**共性规格：**
- **版本模型**：有 linear 单调版本 + active 指针（同 function 的版本模型，§ `function.md`），**这是 pin 所必需**（在途 flowrun 不漂移）；**无 sandbox / env / executions 日志**（它们不是 activity，不产生执行记录——control 的选择记 flowrun journal 的 `branch_taken`、approval 记 `approvals` 表）。比 function 轻、比 trigger（无版本）重。
- **catalog**：进（name + description；强制每个节点都引用实体 ⇒ catalog 会比 function 多很多一次性小实体，**靠 AI 写清楚 name/description 区分**，不设特殊过滤机制——决策 B）。
- **relation**：进，新增 2 类 EntityKind（`control` / `approval`），relation 实体 **9 → 11**；`workflow → ctl_/apf_` 记引用边（引用计数 + 改了反查影响 + needs_attention）。
- **pin**：执行时进 flowrun 的 pin 闭包——`pinned_callables` 扩展为「pin 所有引用的版本化实体（fn/hd/ag/**ctl/apv**）」，机制与 callable pin 完全统一（17 §1，ADR-020）。
- **生命周期**：**独立孤儿**（决策 C）——删 workflow **不级联删**它引用的 ctl_/apf_，与 function/agent 一致；孤儿（relation `refCount=0`）可被识别、AI/用户按需清理。
- **可引用永远指 active**（永远 prod，编排语义）；在途执行 pin 版本（同 callable）。

### 3.1 control 逻辑实体（`ctl_`）

```go
type ControlLogic struct {              // ctl_<16hex>
    ID, WorkspaceID, Name, Description string
    ActiveVersionID string
}
type ControlLogicVersion struct {       // ctlv_<16hex>
    Version int                          // 单调
    Branches []ControlBranch             // 有序
}
type ControlBranch struct {
    Port string                          // 出口名（图把它连到下游）
    When string                          // 布尔 CEL（选路；末条 "true" 兜底）
    Emit map[string]string               // 字段→CEL（重塑下游 payload；空=透传）
}
```
- **能力边界**：when/emit 仅确定性 CEL（读 payload/ctx、禁 now()、禁外部）——**不是轻量 function**，不做任意计算。
- **复用现实（诚实）**：when/emit 与具体 workflow 的 payload 形状**强耦合**，跨 workflow 复用少；价值在「编排-锻造分离 + 统一心智」，非建共享库。

### 3.2 approval 渲染实体（`apf_`）

```go
type ApprovalForm struct {              // apf_<16hex>
    ID, WorkspaceID, Name, Description string
    ActiveVersionID string
}
type ApprovalFormVersion struct {       // apfv_<16hex>
    Version int
    Template        string               // markdown，含 {{ CEL }} 插值（"把数据变成人能看懂的审批点"）
    AllowReason     bool
    Timeout         string               // duration；空=永不超时
    TimeoutBehavior string               // reject | approve | fail（填了 timeout 必填）
}
```
- 出口固定 `yes` / `no`（approval 的本质，不由实体定义）。
- `timeout` 是**唯一保留的 durable timer 用途**（§5 砍 wait 后，timer 原语缩到这里）。
- 复用潜力高于 control（审批界面更通用，如「邮件发送审批」可跨 workflow）。

> **ID 前缀**（`ctl_`/`ctlv_`/`apf_`/`apfv_`）为建议值，落地时进 `database.md` S15 登记。

---

## 4. 边 = payload 数据流

- **边携带 payload**（数据依赖：下游读上游已记账的 `node_completed.result`）。与 `17 §5` 的「作用域变量」契约**完全一致 → 不改 17**，只是表达从「画端口线」退回「CEL 引用上游」（决策 A：payload 是更简单的心智）。
- **顺序 = 数据依赖涌现**：B 读 A 的 result → B 在 A 之后；B 不读 A → 并行。**无纯控制边**（伪顺序被正确消解为并行）。唯一例外：副作用必须有序但数据无关（先扣款再发货）→ 让下游读上游的 **completion（一种数据）** 强制顺序。
- **不是 token 驱动**（关键，别撞回 00 否决的 message-queue）：数据依赖是**图的语义**，执行是 **durable 解释器照图判 readiness + journal 记账**——不是 actor 等 token。一个节点 ready ⟺ 控制前驱完成 + 需要的上游 result 已记账。
- **多输入**：节点 CEL 引用多个上游 result，**合并显式写**（`{user: A.user, order: B.order}`），**不搞隐式字段 merge**（避免覆盖歧义 + 守确定性）。
- **多输出**：普通节点（action/agent）result 广播给所有下游（fork）；control 按 when 只发选中出口（XOR）。
- **回边带信息**：control 的回边 emit 携带循环状态（attempt+1、feedback）；每轮 `iteration_key` 区分，重放抄已记账轮次（17 §4，ADR-017）。

---

## 5. 控制流哲学修订（推翻 revamp「控制下沉」的一半）

revamp「14→5」做对了**调用类合并**（llm/function/handler/mcp/skill 5 个调用节点 → action+agent 2 个）——**保留**。做过头的是**控制类下沉**（parallel/loop/wait 塞进边/拓扑/gate）——本次修订：

| 控制能力 | revamp（下沉） | 本次（控制即节点 / 拓扑 / 实体） |
|---|---|---|
| 分支 / 循环 | case 回边（藏在边里） | **control 节点**（引用 ctl_ 实体；回边=连回上游） |
| 并发 fork | 多出边（隐式） | **拓扑涌现**（一个出口连多下游=广播并行，免费，无 parallel 节点） |
| 汇合 join | await 全部入边 | **拓扑涌现**：AND-join（fork 汇合，等全部）/ active-branch（control 分支汇合，等被激活）——由「上游是 fork 还是 control」**结构推导**（17 §3），不可混 |
| 延迟 / 定时 | 任意节点 `at?`/`after?` gate | **砍**。时间归图外 trigger（cron/sensor）；超时归 approval `timeout`；retry 间隔归 action `retry.backoff`；限流归 callable 内部 |

**砍 wait（决策）**：「等待节点正常业务没有」——它的真实用途全被吸收（见上表右列末行）。durable timer **原语本身没消失**，缩到 approval `timeout`（§3.2）。这是对 **16 A-4** 的修订：durable timer 从「三用途（at gate / after gate / approval timeout）」缩到「一用途（approval timeout）」，是 owned trade-off。

**两种 retry**（别混）：
- **平台 activity retry**（action 的 `retry` 字段）：平台自动重跑同一步、同样输入，**不带业务反馈**，扛瞬时故障。
- **图层业务循环**（control 回边 + emit）：编排者**显式画**，**回边带反馈**（attempt/不合格原因），业务迭代到合格。
「不合格了带着反馈重做」走后者（control 回边），不是 action.retry。

---

## 6. 与已定契约的兼容性

| 契约 | 兼容性 |
|---|---|
| **17 durable 执行契约** | journal / record-once / replay / join 语义 / pin **全不变**；`pinned_callables` 扩展含 ctl_/apf_（§3）；**§7 节点 config 要改**（从内联 `{branches}`/`{prompt}` 改为 `{ ref, args/出口连线 }`）；§5 作用域变量不变（payload 模型对齐） |
| **08 编排 UI** | ✅ 印证「边不画流动、节点是唯一滴答载体」；palette 5 节点改名（tool→action / case→control）；**inspector 改为「引用哪个实体 + 接线」**（不再内联 branches/prompt）；运行时滴答 / trace 不受影响 |
| **16 标准符合性** | WP1-5/10/16 符合性**不变**（control 实体化不改 WP4 XOR / approval 不改 WP16 deferred choice，只改逻辑存放位置）；**A-4 durable timer 修订**（缩到 approval timeout）；**A-3 trigger 出图**被本模型印证（trg_ 实体）；WP11 implicit termination / WP19-20 cancellation 仍 GAP（继承，不恶化） |
| **DOC-125 trigger 实体** | ✅ 一致；本文补「图内 trigger **入口节点**引用 trg_ 实体」这一层（DOC-125 §6 已铺垫「payload 成 flowrun 入口输入」） |

---

## 7. Changelog + 待收口清单

**相对 workflow-revamp 改了什么：**
- trigger：节点 → 出图信号源实体（trg_）+ 图内入口节点引用它
- `tool` → **`action`**（改名；只调 fn/hd/mcp，agent 单列）
- `case` → **`control`** + 逻辑物化成 `ctl_` 实体
- `approval` 渲染逻辑物化成 `apf_` 实体
- **砍 wait 节点**（时间归 trigger / approval timeout / retry backoff / callable 内部）
- 控制下沉 → **控制即节点**（引用实体）；并发 = 拓扑涌现；边 = payload 数据管道
- 新增 2 类「AI 工作实体」（ctl_ / apf_）；relation 实体 **9 → 11**

**待收口旧文档**（本文先成总纲，后续逐份对齐）：
- `00-overview`：控制流哲学（下沉→即节点）、节点全集（5 节点改名）、砍 wait、表达式段
- `01-triggers`：trigger 出图（与 DOC-125 合并口径）
- `02→agent` / `03→action`：节点名 + 「引用实体」表述
- `04→control` / `05→approval`：逻辑搬进实体，文档改成「实体 domain + 节点引用」
- `08`：palette/inspector（5 节点改名 + inspector 引用实体）
- `16`：A-4 durable timer 缩到 approval timeout
- `17 §7`：节点 config schema（内联→引用实体）
- `DOC-125`：补图内入口节点引用层

---

## 8. 待落地决策（留实现期 / 波次 4）

- **WP11 implicit termination**：flowrun `completed` 定义（所有活跃路径到无后继出边节点、且无 parked）——16 GAP，波次 4 落实。
- **WP19/20 cancellation**：单 activity / parked approval / drain 期取消语义——16 GAP，波次 4 落实。
- **ID 前缀** `ctl_`/`ctlv_`/`apf_`/`apfv_` 进 `database.md` S15 登记。
- **catalog 可见性**（决策 B）：靠 AI 写清楚 name/description；若实测噪音过大，再考虑按类型/workflow 分组展示（UI 细节，非阻塞）。
- **两个新实体的 LLM 工具组**：create/edit/revert/search/get/delete（同 function 工具形态，去掉 run/executions）——波次 4 或随 workflow 锻造一起。

---

> **一句话定位**：这次重定型没动 durable 内核（17），动的是「图怎么表达」——从「内联逻辑的 5 节点」到「纯编排 + 节点引用实体」。从「CEL 要不要做成实体」的纠结出发，绕一圈落在比起点干净得多的地方：**CEL 是 control 节点的语言，control 的逻辑坨是实体，节点只是编排。**

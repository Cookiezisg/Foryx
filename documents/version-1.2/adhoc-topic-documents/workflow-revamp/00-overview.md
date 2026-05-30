# 00 — Overview

脑爆纲领(2026-05-27 立;**2026-05-31 执行模型大改向**)。本文件统领 01-12 各份子设计,是整个 workflow-revamp 的**核心心智事实源**。

> **设计演进(必读)**:本 revamp 的执行底盘曾用 **message-queue + actor** 模型(节点=actor、边=持久化消息队列、控制流从消息涌现)。端到端推演发现该模型对**汇合(join)/ 循环 / 并发**会持续冒窟窿(版本配对、空票、并发双点火),靠打补丁堵不完——这是**选错抽象**的信号。改向后:**workflow = 一段结构化程序,一次 flowrun = 把它确定性地跑一遍,崩了照"事件日志"重放接着跑**。这条路线工业界成熟(Temporal / AWS Step Functions / DBOS,统称 **Durable Execution / 持久化执行**),那些窟窿**由构造消失**。本文档及 01-12 已按此重写;**凡提到"消息队列作为边 / 版本号 / 前沿 / 空票 / 复制消息进 queue"的旧表述一律作废**。

---

## 总原则:Mechanism vs Policy 分离

> **平台只提供机制(mechanism),策略(policy)由 workflow 编排者(AI / 用户)在编排时决定。**
>
> 平台不知道业务,**永远不猜业务默认值,永远不替用户做业务决定**。

派生:
- ❌ 无 timeout / retry / 错误分类 等任何**业务**默认值;不填 = 不做。
- ✅ **安全 / 资源兜底**(防平台自己崩,跟业务无关 — 如 CEL 评估超时 / sandbox 内存上限 / 事件日志 GC 默认值)平台必须保留。这条豁免覆盖"资源安全",不算"替业务做决定"。
- ✅ **通知是 mechanism**(平台保证)— 节点 retry 用尽时平台必推 SSE 通知;**Trigger 节点 retry 用尽时平台自动 deactivate workflow**——这不是"替用户暂停",是诚实呈现"入口已废"。详 [`07-error-handling.md`](./07-error-handling.md)。

跟 Dify / n8n 差异化:他们靠平台 hardcode 默认值弥补"用户拖拽时不会想细节",Forgify **AI 编排时主动问 / 显式画**,不需要平台兜底业务。

---

## 产品定位 — Durable Execution(持久化执行),不是 message-queue,也不是纯 DAG engine

| 业界类别 | 代表 | 执行模型 | Forgify |
|---|---|---|---|
| 数据 pipeline | Airflow / Prefect / Dagster | 严格 DAG,一次性拓扑跑完 | ❌ |
| No-code 自动化 | n8n / Dify / Coze | 严格 DAG | ❌ |
| **持久化执行** | **Temporal / AWS Step Functions / DBOS / Restate** | **workflow=程序,跑一遍,事件日志+确定性重放** | **✅ 同类** |

Forgify 是 **嵌入式、跑在自带 SQLite 上的 durable execution 引擎**(DBOS 那种"一个库、启动时发现没跑完的执行就续上"的形状,**不是**架一个 Temporal 集群):

- **一次 flowrun = 把这张图当一段结构化程序、确定性地跑一遍**;执行间靠 trigger 携带的 ctx(`flowrunId` + 哪次触发)区分,互不干扰。
- **节点 = 一个"记账步骤"(activity)**:跑之前先记"我要调它",跑完把**结果记进事件日志**。
- **控制流(顺序 / 并行汇合 / 分支 / 循环)是程序原生结构**,不是从消息到达里涌现的。
- **崩了 = 从头重放程序,凡日志里已有结果的步骤直接抄结果(不重跑 LLM/工具),停在第一个没记账的步骤继续**。
- 跟"人照菜谱做事"心智同源:照流程走、随手记笔记、被打断了照笔记接着做。

---

## 核心心智:一个执行器 + 一本事件日志

把 workflow 重新建模成:**一张被校验为"结构化"的图 + 一个解释器照着它走 + 一本 append-only 事件日志(journal)记每步结果**。

- **解释器(执行器)** 从 trigger 节点出发,照图的结构往下走:走到 agent/tool 节点 = 跑一个 activity;走到 case = 按 CEL 选分支 / 绕回边;走到并行分叉 = 同时开几条分支、汇合处等齐;走到 approval = durable 地挂起等信号。
- **事件日志(`flowrun_events`)= 唯一真相**:每个 activity 的"开始 / 结果"、每次分支选择、每次信号都按序 append。
- **物理形态**:一个 goroutine 照图走、调工具;遇并行就再开几个 goroutine、`WaitGroup` 等齐;每步往 SQLite 写一行日志。**没有分布式队列、没有版本号、没有前沿计算。**

### 确定性(durable execution 的硬约束,Forgify 天生满足)

重放要正确,**控制流必须确定性**:给定相同输入 + 相同的已记账结果,解释器每次都走同一条路。
- 所有**不确定性**(LLM 输出、工具结果、时间、随机)都在 **activity** 里,其结果被记进日志 → 重放时读日志、不重算。
- **case / loop 的判断只读 payload(= 已记账的结果),是纯 CEL、无副作用**(doc 04:100ms、禁 LLM/HTTP)。
- 因此 **Forgify 的 5 节点结构天生满足确定性**:所有"会变"的东西都在 activity,控制流只读已记账的值。

---

## 5 个节点全集

砍 14 → 留 5。每个节点要么是 activity(记账步骤),要么是纯控制流:

| 节点 | 在执行模型里是什么 | 详设计 |
|---|---|---|
| `trigger` | 程序入口;输入 = 该 trigger 的 payload + ctx(整次执行起点,不是 activity) | [01-triggers.md](./01-triggers.md) |
| `agent` | **activity**:引用 agent entity,跑一次 LLM ReAct loop,结果记账 | [02-agent-node.md](./02-agent-node.md) + [09-agent-domain.md](./09-agent-domain.md) |
| `tool` | **activity**:调 forge callable(function/handler/mcp/**agent**),结果记账 | [03-tool-node.md](./03-tool-node.md) |
| `case` | **纯控制流**:多路 switch 选分支 + 回边形成结构化循环 | [04-case-node.md](./04-case-node.md) |
| `approval` | **durable 等信号**:挂起,记"等信号",用户决策后从此处继续 | [05-approval-node.md](./05-approval-node.md) |

### 砍掉的 9 个 + 原因(durable-execution 视角)

| 砍 | 原因 |
|---|---|
| `llm` | 合到 agent(空 tool 自动 single-shot 退化) |
| `function` / `handler` / `mcp` | 合到 tool(都是"调一个 callable"的 activity) |
| `skill`(独立节点) | 改 agent 的挂载 |
| `condition` | 合到 case |
| `loop` | 合到 case + 回边(= 程序里的结构化循环) |
| `variable` | 跨节点状态本就是**程序作用域里的变量**(循环外算的值在循环里直接读);真要持久化跨执行状态用 handler。不需要节点表达 |
| `parallel` | 并发是 infra 行为:**普通节点多条出边 = fork,汇合处 = join(await 全部)**,程序结构原生表达,不需要节点 |
| `wait` | 延迟 = 程序里的一个 durable timer(记"睡到 T"、重放时按日志判断),不是节点类型 |
| `http` | 用 forge function 包装,跟"能力源自 forge"原则一致 |

---

## 表达式语言:全平台一套 CEL

整个 workflow 平台只有**一套表达式语言 = CEL**。按字段输出类型分两种用法,**由字段类型定死,作者不用选**:

- **求值 / 布尔字段**(`case.when`、`case.emit` 的字段值、`tool.args` 的字段值)→ **裸 CEL**,产出类型化值(如 `payload.x + 1` 出数字 `6`)。
- **文本文档字段**(`agent.prompt`、`approval.prompt`)→ **模板串 `{{ CEL }}`**(`{{ }}` 里是 CEL,求值后字符串化插入)。
- `{{ }}` 不是第二种语言,**只是 CEL 的插值定界符**。Go `text/template` 作为语言整个退役(无 `if`/`range`/`funcMap` 控制流);列表拼字符串用 CEL 函数一行(如 `payload.items.map(i, i.name).join(...)` 出逗号串)。
- **实现**:`backend/internal/app/workflow/expression.go`(原 `text/template`)退役 → 一个 CEL 求值核心 + 一个薄的 `{{ CEL }}` 插值 pass。详 [`04-case-node.md`](./04-case-node.md)。

---

## 并发模型:fork-join(结构化并行)

- **fan-out = 普通节点多条出边 = 把同一份输出广播给每条下游分支,分支并发跑**(进程内 goroutine)。
- **fan-in = 汇合节点 = `await 全部入边分支完成`,然后只继续一次**。这是结构化的 fork-join,不是"消息到了再判断点不点火"——**所以不存在并发双点火**。
- **静态 join**:汇合的分支数 = 设计时图里画了几条入边,运行时不变。**"对 N 个东西 map 再聚合"这类动态扇出不在图层做,塞进一把 forge function 内部**(它内部爱怎么并发怎么并发,返回一个结果)——连"并行处理"都是锻造出来的能力,不是平台编排原语。
- **平台保完整性、业务并发归锻造**:平台保证 activity 不会被重复跑(日志记一次)、给 handler 发指令的管道不串字节(见下文 handler 段);但**两条并行分支同时改同一处外部状态、谁先谁后影响结果 = 锻造的人写成幂等 / 设计成不撞**(这也是产品乐趣的一部分)。

---

## 循环与汇合:结构化 + 作用域变量(Theme 1 的核心结论)

把"散开→循环→再汇合"建模成**程序里的并行块 + 结构化循环 + 作用域变量**,而不是消息配对。三类历史窟窿**由构造消失**:

| 历史窟窿(message-queue 模型下) | durable-execution 下为何不存在 |
|---|---|
| **汇合配对**(循环外的旧值要跟循环内的新值配,需版本/前缀匹配,否则死等) | 循环外算的值就是**作用域里的一个变量**,循环体每轮直接读。没有版本、没有匹配、没有死等。 |
| **空票**(没走的分支怎么不让下游死等) | **没走的分支就是没执行**。控制流去哪是程序定的,没有"消息在等没来的分支"。 |
| **并发双点火**(join 被两个输入各触发一次) | join = `await 全部`,后续**只跑一次**;运行时调度它一次。 |

**唯一约束:图必须"良构 / 可归约"**——并行分支自包含(分支里的状态不跳到分支外,外部也不跳进分支中间)、循环单入口(一个明确的循环头,大家从头进、绕回头)。AWS Step Functions 也是这么强制的。accept 时校验器检查;**鬼画符式的乱回边(交叉 / 不可归约)会被拒并说清原因**。对"AI 画规整工作流 + 画布可强制结构"的 Forgify,这反而更干净——真实的回环(重试到成功 / 改到合格 / 轮询到就绪 / 修一部分留另一部分)全是良构循环。

**"聪明地循环"**:循环体里只重跑循环体内的节点;循环外的值白拿不重算;崩溃重放只补没做完的;绝不从头傻跑。

> 一个内部实现细节:循环里要用"节点 + 第几轮"当**日志查找键**区分同一节点不同轮次的结果——但它只是重放去重键,**不是用户可见的版本、没有偏序匹配、没有前沿**,跟历史方案天壤之别。

---

## 持久化:事件日志 + 确定性重放

执行的可靠性靠**每个 flowrun 一本 append-only 事件日志 + 崩溃重放**(替代旧的"消息状态机 + 原子认领")。

### Schema(对比旧设计,塌缩成三张)

```sql
-- 一次执行
flowruns        ( id, workflow_id, version_id,        -- version_id 启动时钉(图拓扑稳定)
                  input,                               -- payload + ctx(JSON)
                  status,                              -- running / awaiting_signal / completed / failed / cancelled
                  started_at, ended_at )

-- 唯一真相:journal
flowrun_events  ( id, flowrun_id, seq,                -- append 顺序
                  type,                                -- node_started / node_completed / branch_taken / signal_awaited / signal_received / ...
                  node_id, iteration_key,              -- iteration_key = 区分循环不同轮的结果(内部重放键)
                  result )                             -- JSON

-- durable 等待(approval)
approvals       ( id, flowrun_id, node_id, prompt, payload,
                  status,                              -- parked / approved / rejected
                  reason, created_at, decided_at )
```

**`messages` / `node_state` / 版本列 / 前沿 / 空票 —— 全部不存在了。**

### 崩溃重放

```
Forgify 重启
  ↓
扫 status=running / awaiting_signal 的 flowrun
  ↓
对每个:从头重放程序
    ├ 命中日志的 activity → 直接返回日志里的结果(不重跑 LLM/工具)
    ├ 走到第一个没记账的步骤 → 真跑一次、记账
    └ awaiting_signal → 等信号(信号到了也是一条日志事件)
  ↓
跑到程序返回 = flowrun done
```

### exactly-once 与幂等边界(平台 vs 锻造)

- **平台保证**:每个 activity 的结果**记一次账**,重放读缓存 → **不会重复调 LLM/工具**;控制流确定性。
- **明确的边界(归锻造,任何 durable 系统含 Temporal 都一样)**:activity 崩在"外部副作用已发生、结果还没记账"之间 → 重跑会重复那个副作用。这是 at-least-once 的固有边界。**编排者选 retry + 把工具写成幂等 = 业务层达成 exactly-once 效果**。这是一条命名清楚的责任线,不是 whack-a-mole 窟窿。

### 规模提示

超长循环(几千轮)会让日志变大、重放变慢 → 需要 **continue-as-new**(快照 + 新日志)。但"N 塞进工具"的哲学让循环天然短(都是有限重试,不是数据迭代),本地单用户不急。

---

## 三条总纲

### 1. 员工思维
> **workflow 节点 = 员工**:接固定任务 + 用配好的方法和工具 + 执行 + 输出。**不改流程结构,不调度其他人**。
- agent 节点不能 spawn subagent / 不能调其他 workflow;
- skill 编排时预激活,不让 LLM 临场 search/activate;
- tool 必须 forge,不挂平台黑盒(fs / shell / web / memory / ask 一律不挂)。

### 2. 能力源自 forge
所有外部能力接入**只有一个来源**——forge,无平台黑盒 escape hatch。

| 层 | 来自 forge 的能力 |
|---|---|
| trigger 层 | polling function(AI 帮造,对接 SaaS / 复杂判断 / 第三方无 webhook) |
| tool 层 | function / handler / agent 都是用户/AI 锻造;mcp 是 marketplace 装 |
| 状态层 | 跨节点持久状态用 handler stateful class(循环内临时状态用程序作用域变量) |

### 3. 永远 prod
> 所有"X 引用 Y"的关系,**Y 永远是 active version**。无 version pinning,没有 `@v3`。
- 改 Y → 所有引用 X 自动跟新;revert Y → 跟着回滚。
- forge entity 加 kind 字段(如 function 的 `normal`/`polling`)——**kind 是 version 级**。
- Workflow accept 时 capability check 校验"引用需要 vs active version 实际 kind",不匹配 → accept 失败 / 标 needs_attention。**capability-check 不只查存在,还查 kind/method/必填参数齐(handler 的 `.method` 在 active version 还在、node/agent 给了必填参数值);报全部问题 + 每条带 next_step,不首违规即短路;被引用实体改了 active version(kind/签名)时反向重查依赖方、标 needs_attention + 通知。** 详 [`11-integration-chains.md`](./11-integration-chains.md) §E3(CANON-X2)。
- AI 工程师角色:改 entity 前主动告诉用户"这影响 workflow A/B/C"。
- 跟 K8s deployment "所有 pod 用同一 image" 心智一致 —— 简单 > 灵活,本地单用户无 SaaS 级 pin 必要。
- (注:`FlowRun.version_id` 钉住**图拓扑**的版本,保证一次执行内图结构稳定;但被引用的 callable(fn_/hd_/ag_)按"永远 prod"在调用时解析到 active version。长跑/挂起重启后可能跑到改过的 callable——这是"永远 prod"刻意的修复回路属性,编排者改 callable 时应保证幂等。)

### Quadrinity — Forgify 的 4 类 forge 实体

| | function | handler | **agent** | workflow |
|---|---|---|---|---|
| 性质 | 纯函数 | stateful class | **LLM ReAct loop 配置** | 编排 |
| 版本管理 + pending/accept | ✅ | ✅ | ✅ | ✅ |
| AI 锻造工具 | 9 | 10 | **11** | 9 |
| 可作 callable 被引用 | ✅ `fn_xxx` | ✅ `hd_xxx.method` | ✅ `ag_xxx` | ❌(员工思维) |
| ID 前缀 | fn_/fnv_/fne_ | hd_/hdv_/hcl_ | **ag_/agv_/agx_** | wf_/wfv_/fr_ |

mcp 从 marketplace 装,不算 forge。Quadrinity 严格指 forge 四元 = **function / handler / agent / workflow**。

---

## handler 并发(单管道安全 — 平台必兜的完整性)

handler 是单 subprocess、单 stdin/stdout 管道的 JSON-RPC。**对同一个 handler 实例的并发调用(同 flowrun 的并行分支 或 跨 flowrun 共享实例)在实例处串行(保留 mutex)**——这是平台的完整性保证。**绝不"砍 mutex 让真并发"**(单管道并发写会撕裂帧、读会抢错响应)。真并发只发生在**不同能力之间**(不同 function/handler 独立并发);打同一个有状态 handler 实例本就该串行(共享可变状态的天然串行点)。若某 handler 成瓶颈,锻造者可写成无状态(让每次调用走独立实例/函数并行)。详 [`03-tool-node.md`](./03-tool-node.md)。

---

## 资源 / 安全兜底(防平台自己崩,跟业务无关)

| 兜底 | 谁定 |
|---|---|
| Workflow timeout(整体跑多久强杀) | **用户/AI 编排时拍**;不填 = 永不超时 |
| Sandbox 内存 / fd 上限 | 平台保留 |
| CEL 评估超时(100ms) | 平台保留(防恶意表达式卡死) |
| **事件日志 retention / GC 默认值** | **平台给兜底默认值**(资源安全,非业务决定;对齐 sandbox 30 天 GC 先例),用户/AI 可覆盖。详 [`07-error-handling.md`](./07-error-handling.md) |
| 死循环 | **编排者责任**(case 写合理终止 + workflow timeout 兜底) |

平台**不**设"hard cap 100 次循环"这种业务相关硬阈值。

---

## Workflow lifecycle

**没有独立 "Deployment" 抽象层。**用 `Workflow.active: bool` 表达"上线/下线",`FlowRun.IsFromListener: bool` 表达"触发来自 listener 自动还是用户/AI 显式"。`active=true` 扫 graph 中 listener 类 trigger 节点(cron/fsnotify/webhook/polling)注册监听;`active=false` 撤监听 + 销毁 `Owner={Kind:"workflow"}` 的 handler 实例。`IsFromListener` 决定 handler Owner(`true→{workflow}` 跨触发复用;`false→{flowrun}` 隔离)。详 [`06-workflow-lifecycle.md`](./06-workflow-lifecycle.md) + [`03-tool-node.md`](./03-tool-node.md)。触发统一入口 `scheduler.StartRun(workflowId, triggerNodeId, payload, isFromListener)`,起一个 flowrun。

---

## 跟 chat 的产品对照

| | chat agent | workflow agent 节点 |
|---|---|---|
| 角色 | **老板** | **员工** |
| 任务来源 | 用户对话 / 探索 | 程序走到这一步喂给它的输入 |
| skill | 自己 search + activate | 编排时配死 |
| tool | 自己挑 + 临场 forge | 编排时配死 |
| subagent | 可 spawn | 不能 |
| 改流程 | 自由探索 | 不能 |

**narrative**:**chat 是探索/设计/锻造的地方;workflow 是沉淀/自动化/规模化的地方**。锻造完的能力 → 沉淀进 workflow,员工无人值守干活。

---

## 待继续脑爆的大块(均已 settle)

1. ~~Approval~~ ✅ [`05`](./05-approval-node.md)(durable 等信号)
2. ~~Workflow lifecycle~~ ✅ [`06`](./06-workflow-lifecycle.md)
3. ~~编排 UI~~ ✅ [`08`](./08-orchestration-ui.md)
4. ~~错误处理 / retry / 死信~~ ✅ [`07`](./07-error-handling.md)
5. ~~case 表达式语言~~ ✅ CEL,详 [`04`](./04-case-node.md)
6. ~~循环 / 汇合 / 并发的执行语义~~ ✅ **durable execution(本文 + 04 + 07)**,端到端推演零窟窿
7. ~~handler crash + state~~ ✅ [`03`](./03-tool-node.md)
8. ~~持久化 / exactly-once~~ ✅ 本文"持久化"段(事件日志 + 重放)

---

## 术语映射(旧 message-queue 表述 → 新 durable-execution 表述)

01-12 已按下表重写;读到旧库的任何残留按此理解:

| 旧表述(作废) | 新表述 |
|---|---|
| 边 = 持久化 message queue / 邮箱 | 边 = 流程图箭头(谁接谁);数据当作值传递并记进日志 |
| 节点 = actor,盯邮箱消费 | 节点 = activity(记账步骤);执行器照图走 |
| 消息 `version` / `iterationIdx` / 前沿 | 删除;循环用作用域变量 + 内部重放键 |
| 空票 / void token | 删除;没走的分支不执行 |
| 消息原子性 / consume-emit-processed 状态机 / 原子认领 | 事件日志记账 + 确定性重放 |
| `复制消息进 queue`(retry/回边/replay 统一抽象) | retry/replay = 重放跳过已记账、重跑未完成;回边 = 程序循环 |
| `messages` 表 / `node_state` 表 | `flowrun_events`(journal) |
| `{{ nodes.X.out }}` 跨节点引用 | 节点读其前驱的输出(程序数据流;记进日志) |

---
id: WRK-001-04
type: working
status: active
owner: @weilin
created: 2026-05-20
reviewed: 2026-05-31
review-due: 2026-08-31
audience: [human, ai]
landed-into:
---
# 03 — Tool 节点

脑爆结论笔记(2026-05-27)。
2026-05-31 改向 durable execution(详 [`00-overview.md`](./00-overview.md))。

---

## tool 节点 = 一个 activity

在 durable-execution 模型里(详 [`00-overview.md`](./00-overview.md)),**tool 节点 = 一个 activity(记账步骤)**:执行器走到它 → 先记"我要调这个 callable" → 调 forge callable(function / handler / mcp / **agent**)+ 传 args → 把结果记进事件日志(journal)。崩溃重放时命中日志的步骤直接抄结果、不重跑工具。

---

## 3 → 1 合并

废弃 `function` / `handler` / `mcp` 三个独立节点,合并成一种 **tool 节点**:统一"调用一个被命名的可执行能力 + 传 args + 拿结果"。

跟 [`02-agent-node.md`](./02-agent-node.md) 里 agent 节点的 tool 挂载 **完全同源** — 同一个 callable 注册表,两种调用方式:

| 调用方式 | args 谁组装 | 谁决定调用 |
|---|---|---|
| **tool 节点(流程直接调)** | 编排时静态填(每个 arg 值是裸 CEL 表达式) | workflow 流程 |
| **agent 节点 tool 挂载(LLM 调)** | LLM 临场组装 | LLM 自治 |

---

## 节点结构

```
type: tool
config:
  callable: <ref>          # 见下方 ref 语法
  args: {...}               # 每个 arg 值是一个裸 CEL 表达式,读 payload / ctx 求值出类型化值(count: payload.x + 1 → 数字 6,不是字符串 "6")
  retry: { maxAttempts: 3, backoff: "exponential" }  # 可选,不填 = 0 次,失败立即通知
  timeout: <duration>                                # 可选,不填 = 永不超时
```

**args 数据流(durable 语义)**:每个 arg 的值是一个**裸 CEL 表达式**,读其前驱节点已记账的输出(以及该 flowrun 的 payload / ctx)**求值出类型化值** —— 如 `count: payload.x + 1` 产出数字 `6`,不是字符串 `"6"`。这是程序数据流——值在节点间传递、记进日志,不是"从邮箱里取一条消息";求值字段用裸 CEL(不走 `{{ }}` 模板插值),详 [`04-case-node.md`](./04-case-node.md) 表达式语言段。

**retry 行为**(跟 [`07-error-handling.md`](./07-error-handling.md) 一致):
- 这是一个 activity;失败按 `retry` 配置**重跑该 activity**(重放跳过已记账的步骤,只重跑这一个未完成的)。
- retry 次数内失败 → 只记录,不通知
- retry 用尽 → 平台主动推 SSE 通知;**workflow.active 不变**(tool 节点不是入口,不像 trigger 节点 retry 用尽会 deactivate)
- `retry` 字段不填 = 0 次重试,失败立即通知

> **at-least-once 与幂等边界(跟 00 持久化段一致)**:activity 崩在"外部副作用已发生、结果还没记进日志"之间 → 重跑会重复那个副作用。这是任何 durable 系统(含 Temporal)的固有边界,**不是窟窿**。平台保证 activity 结果只记一次账、重放读缓存不重复调;**编排者选 retry + 把 callable 写成幂等 = 业务层达成 exactly-once 效果**。幂等边界归锻造。

---

## Callable ref 语法

跟 [`00-overview.md`](./00-overview.md) 总纲 3 "永远 prod" 一致:**ref 永远指 active version**,无 pin 语法。

| Callable | ref 形式 |
|---|---|
| function | `fn_xxx`(永远 active version) |
| handler 方法 | `hd_xxx.methodName`(永远 active version) |
| mcp 工具 | `mcp:serverName/toolName`(MCP 无版本概念) |
| **agent** | **`ag_xxx`**(永远 active version)— 详 [`09-agent-domain.md`](./09-agent-domain.md) |

引用 entity 的 active version 改了 / revert 了,**所有 tool 节点自动跟新 / 跟着回滚**。Workflow accept 时 capability check 校验 active version 是否符合引用上下文(如 trigger 节点要求 function kind=polling)。

> 注(版本语义,A-5):"引用永远指 active、改了自动跟新 / revert 跟着回滚" 是**编排 / 引用**语义。**执行另说** —— flowrun **启动时 pin 住图拓扑(`version_id`)+ 它要调的 callable 版本**(`pinned_callables` 快照),整个生命周期(含崩溃重放 / parked resume)用这份快照,**callable 不漂移**(= Temporal versioning,采纳标准)。新 flowrun 才用新 active。详 [`00`](./00-overview.md) §3 / 确定性段。

---

## Handler 生命周期跟 flowrun 走

handler(及 agent)实例 **per-flowrun 隔离,恒为** `Owner = {Kind: "flowrun", ID: flowrunId}` —— 不论触发来源(listener / 用户 UI / AI)。首次调用时 lazy spawn,flowrun 结束时 `DestroyOwner({Kind: "flowrun", ID: flowrunId})` 自清。**没有 active-vs-其他的区分**,没有 `{Kind: "workflow"}` 共享实例、没有跨触发复用。

| 触发来源 | Handler Owner | instance 生命周期 |
|---|---|---|
| Active workflow 的 listener 自动触发(cron / fsnotify / webhook / polling) | `{Kind: "flowrun", ID: flowrun.id}` | 跟 flowrun 同寿,跑完销毁 |
| 用户 UI 点 trigger 节点 / AI `trigger_workflow` 工具 / inactive workflow 的任何触发 | `{Kind: "flowrun", ID: flowrun.id}` | 跟 flowrun 同寿,跑完销毁 |

`IsFromListener` 不再决定 owner(如保留仅记录触发来源,与实例归属无关)。意思是:
- cron 每小时触发 active workflow → 每次触发**各起独立 flowrun-隔离实例**;connection pool / cache 是 ephemeral、随 respawn 重建;counter / 累积业务态进 journaled 作用域变量 / payload 或外部 store(DB 经 handler 方法,或 Forgify document / memory 实体),**绝不放进程内存跨触发**。
- 用户在 UI 上点 manual trigger 节点测试 → **独立 instance**,跑完销毁,不污染其他 flowrun
- AI 调 `trigger_workflow` 跑一次 → **独立 instance**,同上

Handler 作为 **stateful object** 的对象能力 ✓ 保留,但进程内存只放 ephemeral、可重建、不影响结果的资源(连接池 / 缓存 / 客户端);durable / 影响结果的状态进 journal 或外部 store,**不跨触发持续**。暖实例复用如未来需要 = per-handler 的 ephemeral 资源池(Temporal 式),非共享有状态实例,V1 不做。

### Crash 处理

**Handler 子进程死了**(Python OOM / 未捕获异常 / 外部依赖死):

- 平台 detect(stdio EOF / pipe broken)
- 平台**自动 respawn 新 instance**(`handlerRegistry.Acquire` 现状已经如此)
- **无"几次后放弃"的业务上限** — 跟 Mechanism vs Policy 一致,平台不替用户决定"重试几次算失败"。**但有资源安全帽(C11,"防平台崩"豁免)**:respawn 走**速率限制**,持续 crash-loop(短时间反复崩,如 init 必崩)→ 标 `needs_attention` + 通知(对位 trigger 失败 → workflow inactive + 通知的诚实模式),不无限高频 respawn 烧 CPU/PID。速率/窗口走 `pkg/limits` 高默认。**这是资源兜底,不是替用户放弃业务重试。**

**activity 崩在"副作用已发生、结果未记账"之间怎么办** — tool 节点 config 拍。在 durable 模型里,handler 子进程死掉就是这个 activity 没记账成功;执行器重放时会停在这个未完成的 activity,按下面策略处理:

```yaml
type: tool
config:
  callable: hd_xxx.method
  retry: { maxAttempts: 3, backoff: "exponential" }   # activity 失败/崩溃 → 重跑该 activity
```

- **activity 失败(callable 返业务错误 或 子进程崩溃)** → 按 `retry` 重跑这个 activity(执行器重放、命中已记账的前序步骤抄结果,停在此 activity 重跑一次)。
- **retry 用尽仍失败** → 平台推 SSE 通知,该 flowrun 走 [`07-error-handling.md`](./07-error-handling.md) 的失败路径。
- 至于"重跑会不会重复已发生的外部副作用"——这就是上面那条 **at-least-once 幂等边界**:归锻造,handler 写成幂等即可。一个 activity 是"子进程崩"还是"method 返错误",对 durable 重放是同一件事(都是"这步没记账成功,要重跑")。

### State 持久化 — handler 作者完全责任

**平台不提供 state 持久化 helper API**。按"能力源自 forge"原则,handler 作者自治。注意 durable execution 的事件日志只记 activity 的**输入/输出结果**,**不接管 handler 进程内的 in-memory state**——跨 crash 的业务状态仍由作者负责:

| handler 类型 | 怎么做 |
|---|---|
| 完全无状态 | crash 无影响 |
| in-memory state + 丢了不要紧(连接池 / 缓存) | crash 接受丢,新 instance 重建 |
| **durable / 影响结果的状态(counter / 累积业务态)** | 进 journaled 作用域变量 / payload,或外部 store(DB 经 handler 方法,或 Forgify document / memory 实体),**按 flowrun 作用域**;绝不放进程内存、不写跨触发存活的全局 per-handler-id 文件(重放会分叉、并发 flowrun 会撞) |

forge 系统在锻造 handler 时,**教学 prompt 必须明示**:

> handler 是 stateful Python class。
> **in-memory state 在 crash 时会丢**。
> 业务状态需要 survive crash 时,自己写到 file / SQLite。
> 平台不提供 state API。

跟 trigger function / function / mcp 的模式一致 — **作者完全自治,平台不当保姆**。

### Workflow 改 / handler config 改时

- 用户改 workflow version 后 `:accept` → 如果 workflow active,撤旧 listener + 注册新 listener,然后 drain:停新 flowrun + 派发器不起新 flowrun,等在途 flowrun 各自跑完(每个结束时经 `DestroyOwner({Kind: "flowrun", ID: flowrunId})` 自销其独占实例)。**没有 `{Kind: "workflow"}` 共享实例需要撤,没有 refcount / 实例级记账。**
- 实例 **lazy** 等 flowrun 内首次调用时 spawn
- 详见 [`06-workflow-lifecycle.md`](./06-workflow-lifecycle.md)

### Forgify 本体重启

handler 这一侧的重启跟 flowrun 重放是两件互补的事:

```
Forgify 启动
  ↓
扫所有 workflow.active = true 的 row(详 06-workflow-lifecycle.md)
  ↓
re-register 所有 listener(不预先 spawn、不重建任何 handler 实例、无 refcount 重建)
  ↓
实例 lazy:等每个 flowrun 内首次调用时才 spawn(per-flowrun 独占)
```

同时,执行器扫 `status=running / awaiting_signal` 的 flowrun 并从头确定性重放(命中日志的 activity 抄结果、停在第一个未记账步骤续跑,详 [`00-overview.md`](./00-overview.md) 持久化段)。flowrun 的 durable 状态靠确定性 journal 重放恢复,**不靠从 file/SQLite 回灌 handler 进程内存**。第一次触发延迟略高(handler 启动 ~5s),本地单用户场景可接受。

### 通知 / 监控

平台**不主动通知**(跟 [`07-error-handling.md`](./07-error-handling.md) 一致)。平台暴露 events API:

```
GET /events?type=handler_crash&workflowId=wf_xxx&since=24h
GET /flowruns/{id}/failures
```

用户/AI 在 chat 里查 + 主动聚合分析:

```
用户:"昨天 cron 跑挂了"
   ↓
AI 调 events / 失败记录 API → 查到 handler crash 5 次 + OOM 痕迹
   ↓
AI:"handler 调 Gmail API 时 OOM 了,你的 cache 没限制大小。要改吗?"
   ↓
用户:"好"
   ↓
AI:edit_handler → :accept → 重跑失败的 flowrun(从日志续放)
```

主动聚合 / 诊断 / 修复是 **AI 工程师**的事,不是平台的事。

---

## Handler 并发(单管道安全 — 平台必兜的完整性)

> 这一段对齐 [`00-overview.md`](./00-overview.md) 的 "handler 并发" 段。**结论已反转旧脑爆稿**:旧稿曾写"砍 `infra/handler/client.go` 的 per-instance `sync.Mutex`、让 method 调用真并发"——**作废**。durable 模型下,同一实例的并发调用来自 fork-join 的并行分支(同一 flowrun 内),这恰恰要求实例处串行。

handler 是单 subprocess、单 stdin/stdout 管道的 JSON-RPC。**对同一个 handler 实例的并发调用(同一 flowrun 的并行分支)在实例处串行——保留 per-instance `sync.Mutex`。绝不砍 mutex。** 跨 flowrun 不共享实例:不同 flowrun 各有独立实例,对单实例不存在跨 flowrun 的并发调用。

为什么绝不砍:
- **单管道并发写会撕裂帧、并发读会抢错响应** — 单 stdin/stdout 上多个 in-flight 请求没有帧隔离,锁是正确性前提,不是性能调优开关。
- **这是平台的完整性保证(mechanism)** — 跟"平台保 activity 不被重复跑、给 handler 发指令的管道不串字节"同一类承诺(详 [`00-overview.md`](./00-overview.md) 并发模型段)。

真并发来自**更多实例**:不同 flowrun 调同一 handler 各起独立实例并发(per-flowrun 隔离),以及不同 callable 之间(不同 function / 不同 handler)独立并发(fork 出的并行分支各打各的 callable,互不阻塞)。**只有对同一实例的并发调用(同一 flowrun 内)才串行**——共享可变状态的天然串行点,串行是语义上正确的行为,不是退化。

若某 handler 成瓶颈:锻造者把它**写成无状态**,让每次调用走独立实例 / 独立 function 并行(回到"不同能力之间真并发"那条路)。这是锻造者的设计选择,不是平台砍锁。

> 与 00 并发模型段呼应:**平台保完整性(管道不串字节 + activity 不重复记账)、业务并发归锻造**。两条并行分支同时改同一处外部状态、谁先谁后影响结果 = 锻造的人写成幂等 / 设计成不撞。forge 系统在锻造 handler 时,template / 教学 prompt 应明示:"handler 是 stateful subprocess,进程内存只放 ephemeral、per-flowrun 的状态;同一实例的调用平台会串行化(per-instance mutex,正确);任何 durable / 影响结果的状态进 journaled 作用域变量 / payload 或外部 store,不在内存里跨调用共享。要更高的跨实例并发请锻造成无状态。"

---

## 累计节点数减负

跟前两份共识合算:

| 现状 | 重设计后 |
|---|---|
| llm + agent(2) | agent(1) |
| function + handler + mcp(3) | tool(1) |
| skill(独立节点) | 砍掉(改 agent 挂载) |

**6 个节点 → 2 个节点**。剩 `trigger` / `condition` / `loop` / `approval` / `wait` / `variable` / `parallel` / `http` 待审 —— 后续已 settle 为 5 节点全集(`trigger` / `agent` / `tool` / `case` / `approval`),`condition`+`loop` 合 case、`parallel` 由 fork-join 程序结构表达、`wait` 为 durable timer、`variable` 用程序作用域变量、`http` 用 forge function 包装(详 [`00-overview.md`](./00-overview.md) 5 节点段)。

# 16 — 标准模式对标(workflow patterns + Temporal-4 conformance)

2026-05-31 建。动机:前两轮评审靠抽查找洞,A-1(case 后菱形 join 死等)这类"标准模式没规约"的洞是抽查漏出来的。
本文拿**已知标准全集**逐条核 00-15,把"采纳 / 刻意偏离 / 还没覆盖(GAP)"一次性扫清 —— 不是我临场想,是对着教科书清单走。

两套标准:
- **控制流 workflow patterns**(van der Aalst / ter Hofstede,WP1-WP43 的相关子集)—— 图式编排引擎的公认模式目录。
- **Temporal 四原语**(event history / schedules / durable timer / versioning)—— durable execution 工业基线。

状态图例:✅ 采纳 · ⚖️ 刻意偏离(owned trade-off)· 🔴/🟡 GAP(没规约,要修)。

---

## A. 控制流 workflow patterns

| WP | 模式 | Forgify 怎么处理 | 状态 |
|---|---|---|---|
| WP1 | Sequence 顺序 | 边 | ✅ |
| WP2 | Parallel Split(AND-split,扇出) | 普通节点多出边 = 广播并发 | ✅ |
| WP3 | Synchronization(AND-join) | 汇合 = `await 全部入边` | ✅(但须与 WP5 区分,见 A-1) |
| WP4 | Exclusive Choice(XOR-split) | `case`(逐分支 `when:`,first-true-wins) | ✅ |
| **WP5** | **Simple Merge(XOR-join)** | **case 分支重新汇合的 join** | **🔴 GAP = A-1**:现写"join = await 全部入边",对 XOR 分支汇合会死等没走的那条。需 **active-branch join**(只等被激活的入边;case 给未走分支下发 skip 信号,join 据此不等) |
| WP6 | Multi-Choice(OR-split,多分支同时激活) | case 是 XOR(只走第一个 true) | ⚖️ 刻意不支持 OR-split(要并发多路 = 用 AND-split 扇出 + 各自 `when:` 守卫) |
| WP7 | Synchronizing Merge(OR-join) | 因无 OR-split,不需要 full OR-join;只需 WP5 的 active-branch join | ✅(随 A-1 修)|
| WP8 | Multi-Merge | — | ⚖️ N/A(结构化 / 可归约,无多次触发合并)|
| WP9 | Discriminator / N-of-M join | — | ⚖️ N/A(静态 join)|
| WP10 | Arbitrary Cycles 任意环 | 只允许**可归约**回边(`case` 回边、单入口);乱 goto 环 accept 拒 | ⚖️ 刻意约束(换可解性 + 重放确定性)|
| **WP11** | **Implicit Termination 隐式终止** | flowrun 何时算 `completed`? | **🟡 GAP**:没明写。定义:**所有活跃路径都到达无后继出边的节点、且无 parked(approval/timer)= flowrun completed**;有路径 failed 且无 retry = failed |
| WP12-15 | Multiple Instances(对 N 个实例)| "map over N" 下沉 forge 工具内部 | ⚖️ 刻意(动态扇出不在图层)|
| WP16 | Deferred Choice(外部信号选边) | `approval`(挂起等信号再选 yes/no) | ✅ |
| WP17 | Interleaved Parallel Routing | — | ⚖️ N/A |
| WP18 | Milestone | 用 case + 作用域变量表达 | ✅(组合)|
| **WP19/20** | **Cancel Activity / Cancel Case** | `cancel_flowrun` 取消整 run | **🟡 GAP**:取消**整个 flowrun** 已有(`cancel_flowrun`);但单 activity 取消 / parked approval 被取消 / drain 期取消的语义(怎么落 journal、终态、副作用)没规约。至少定:cancel → 写 `flowrun_cancelled` 事件、在途 activity 收 ctx.Done、parked approval/timer 标 cancelled |

---

## B. Temporal 四原语

| 原语 | Forgify 怎么处理 | 状态 |
|---|---|---|
| **Event history(append-only 日志)** | `flowrun_events` journal | **🔴 = A-2**:我上轮把 record-once 写成"`UNIQUE(...,type)` 套所有 type",与 retry 多次 `node_failed`/`node_started` 撞键。标准做法:**log 纯 append-only(只 seq 唯一)**;**record-once 只作用于"结果事件"(`node_completed`)** —— 重放取该步第一条 completed 当缓存结果;attempts/failures 全进 history(带 `attempt#`),不设 type 级 UNIQUE |
| **Schedules(定时 / 触发)** | Theme 3:`trigger_schedules` + `trigger_firings` 收件箱 + 派发器 | **🔴 = A-3**:trigger 是程序入口、**不是 flowrun 内 activity**;它的失败发生在 **listener / 收件箱 / pre-flowrun polling-function 层**,不能写成"trigger 节点 retry"。标准(Temporal Schedule)= schedule 触发**创建** execution,schedule 自身的失败 / overlap / catchup 是 **schedule 级**。trigger 失败 / retry / "用尽→workflow inactive" 全规约到这一层 |
| **Durable timer** | A-4:节点级 gate(`at` 绝对 / `after` 相对)+ approval `timeout`,deadline 记 journal | ✅ 采纳(同一原语三用途)|
| **Versioning / patching** | callable 版本 | **⚖️→✅ 改判(A-5)**:原"永远 prod(callable 调用时解析 active)"会让长跑 / parked run 撞版本漂移。**采纳标准 = 在途 flowrun pin 住启动时解析的 callable 版本**(整生命周期用这份快照,含 resume);**新 flowrun 用新 active**。"永远 prod" 缩回到**编排 / 引用语义**(引用永远指 active、改了自动跟、无 `@v3`),不再作用于在途执行 |

---

## C. 本对标的净产出(行动清单)

对标确认 / 新挖出的,合并第二轮 reviewer findings,完整待修集:

**标准模式没规约(对标新挖 / 确认)**
- **A-1 / WP5** active-branch join(case 汇合不能 await-all)— 🔴
- **WP11** implicit termination 定义 — 🟡(新挖)
- **WP19/20** cancellation 粒度语义 — 🟡(新挖)
- **A-2 / event-history** journal 主键模型(append-only + completed-record-once)— 🔴
- **A-3 / schedules** trigger 失败规约到 schedule 层 — 🔴
- **A-5 / versioning** 改为 pin callable 版本(标准)— ⚖️→✅

**reviewer 其余(非标准目录,工程 / 一致性)**
- A-4 agent retry 契约对齐 · A-6 approval fail 终态 · A-7 删 sub-workflow(workflow 不可调,只 forge 工具)
- B-1/B-2/B-3/B-4 实现量 / 协议同步 / JSON-repair 放对层(已知,排期 + 实现纪律)
- C-1 ephemeral tick 放 notifications(极简,seq-less、不参与 Last-Event-ID)· C-2 continue-as-new 阈值 · C-3 清 `agent_uses_agent` 残留

> 结论:**没有 whack-a-mole** —— 抽象(durable execution + 可归约图)选对了,剩下全落进标准模式,修法是"采纳标准"。两处刻意偏离(WP6 无 OR-split、WP10 仅可归约环)是 owned trade-off;A-5 从偏离改回标准(pin)。

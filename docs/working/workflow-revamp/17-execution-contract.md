# 17 — 执行契约(单一事实源:schema · join · replay · 6 大机制)

2026-05-31 建。2026-05-31 M0 收口(实现 take-over):补全 typed schema、收口 record-once（ADR-018）、replay 代数（ADR-019）、pin 闭包（ADR-020）、单事务 claim（ADR-021）、trigger retry（ADR-022），删 `00`/`11` 残留 schema 副本。**定位**:durable 执行**可实现契约的唯一事实源**。`00` 讲心智(why),本文讲精确 schema / 状态机 / 重放规则(how,照着建)。

> **DRY 铁律(根治 00/11 反复漂移)**:journal/approval/trigger **schema**、**join 语义**、**节点 config 字段名**、**状态枚举** —— **只在本文定义一次**;`00`/`11`/`02-05`/`13`/`15` 一律**引用本文**,不再各存一份。改契约 = 改本文一处。
>
> **schema 落地**:下方列出的是**契约**(列 / 类型 / 约束 / 索引);实现为 `domain/flowrun` + 新 store 的 GORM struct（`serializer:json;type:text`、`check:` 枚举、`gorm.DeletedAt` + `CreatedAt`/`UpdatedAt`），struct tag 必须满足本表。带 `WHERE` 的 partial 索引进 `schema_extras.go`(D7)。

---

## 1. Schema(唯一定义处 · typed)

所有 ID 列 `TEXT PK`，前缀见 §S15。JSON 列 `TEXT`（GORM `serializer:json`）。除非注明，每表带 D2 `created_at`/`updated_at` + D1 `deleted_at`。

### flowruns

| 列 | 类型 | 约束 / 说明 |
|---|---|---|
| `id` | TEXT | PK，`fr_<16hex>` |
| `user_id` | TEXT | NOT NULL，index |
| `workflow_id` | TEXT | NOT NULL，复合 index `(workflow_id,status,started_at desc)` |
| `version_id` | TEXT | NOT NULL — 启动时钉:图拓扑版本 |
| `pinned_callables` | TEXT(JSON) | NOT NULL DEFAULT `'{}'` — `{callable_id: version_id}` **传递闭包**快照（ADR-020）；整 run 用它，无漂移(A-5) |
| `input` | TEXT(JSON) | NOT NULL DEFAULT `'{}'` — payload + ctx |
| `status` | TEXT | NOT NULL，CHECK IN `(running, awaiting_signal, completed, failed, cancelled)` |
| `trigger_node_id` | TEXT | NOT NULL — 哪个 trigger 起的(来源由其 kind 得知，无 `is_from_listener`) |
| `generation` | INTEGER | NOT NULL DEFAULT 0 — replay 代数(§4) |
| `started_at`/`ended_at` | DATETIME | |

> 旧 `paused_state` 列**删除**（journal 取代；ADR-016）。`workflows.concurrency`（旧有列，enum）被派发器读取（§6）——不在本表。

### flowrun_events（append-only journal）

| 列 | 类型 | 约束 / 说明 |
|---|---|---|
| `id` | TEXT | PK |
| `flowrun_id` | TEXT | NOT NULL，index |
| `seq` | INTEGER | NOT NULL，**UNIQUE(flowrun_id, seq)**；写入事务内分配 → per-flowrun 严格单调 |
| `type` | TEXT | NOT NULL，CHECK IN（event type 全集，见下） |
| `node_id` | TEXT | nullable（trigger/控制事件可空） |
| `iteration_key` | INTEGER | NOT NULL DEFAULT 0 — 循环轮次（ADR-017）；0 = 循环外 |
| `generation` | INTEGER | NOT NULL DEFAULT 0 — 属哪一代（§4） |
| `attempt` | INTEGER | NOT NULL DEFAULT 0 — 仅 attempt 类，append-many 序号 |
| `turn` | INTEGER | nullable — 仅 agent 子事件 |
| `tool_call_id` | TEXT | nullable — 仅 agent 子事件 |
| `dedup_key` | TEXT | NOT NULL — record-once 幂等键（ADR-018），app 计算；attempt 类 = `''` |
| `result` | TEXT(JSON) | activity 输出 / 分支选择 / 信号 / timer deadline |

- **append-only**：本表**无** `updated_at`/`deleted_at`（显式豁免 D1/D2 的 update 维度；GC 走整 flowrun 软删 + retention，见 07）。`created_at` 保留。
- **索引**：`UNIQUE(flowrun_id, seq)`；查询索引 `(flowrun_id, node_id, iteration_key, generation)`（replay copy-hit + failures，§4）。
- **record-once partial unique（→ schema_extras.go，D7，ADR-018）**：
  `CREATE UNIQUE INDEX idx_flowrun_events_record_once ON flowrun_events(flowrun_id, dedup_key) WHERE type NOT IN ('node_started','node_failed')`

### approvals

`id`、`user_id`(NOT NULL，inbox 按用户 scope)、`flowrun_id`、`node_id`、`prompt`、`payload`(JSON)、`reason`、`allow_reason`(BOOL)、`decided_at`，+ D1/D2。
`status` TEXT NOT NULL，CHECK IN `(parked, approved, rejected, timed_out, failed, cancelled)`（**+cancelled** — flowrun 取消时 parked approval 标 cancelled，07）。
**UNIQUE(flowrun_id, node_id)** — interpreter park 时 upsert(DoNothing)写本行,重放幂等。**执行真相是 journal**(signal_awaited/received);本行是 UI inbox + 审计投影。读出口:`GET /api/v1/approvals`(当前用户所有 parked,前端 banner/inbox 数据源)。

### trigger_schedules

`id`、`workflow_id`、`trigger_node_id`、`kind`、`spec`(JSON)、`last_fired_at`、`catchup_window`、`overlap_policy`，+ D1/D2。
`retry_policy` TEXT(JSON) — `{maxAttempts, backoff}`（ADR-022）。
`consecutive_failures` INTEGER NOT NULL DEFAULT 0 — 失败 firing 增、成功重置；`≥ maxAttempts` → 自动 deactivate（ADR-022）。
枚举 CHECK：`overlap_policy IN (Skip,BufferOne,BufferAll,AllowAll)`、`catchup_window IN (none,latest,window)`。

### trigger_firings（durable 收件箱）

`id`、`workflow_id`、`trigger_node_id`、`payload`(JSON)、`scheduled_at`、`enqueued_at`、`flowrun_id`(nullable，claim 时回填)，+ D1/D2。
`dedup_key` TEXT NOT NULL — **UNIQUE(workflow_id, trigger_node_id, dedup_key)**（§6 幂等）。
`status` TEXT NOT NULL — **单一生命周期+处置枚举**，CHECK IN `(pending, claimed, started, skipped, superseded, shed)`。无独立 `outcome` 列：终态 status 即 outcome（“每条 firing 有 outcome”= 每条 firing 必达终态）；`01`/`07`/`11` 旧文 “落 outcome” 读作 “置终态 status”。无 `claimed_at` lease（单事务 claim 无中间态，ADR-021）。

### polling_states

`workflow_id`、`node_id`（复合 PK）、`cursor` TEXT，+ D1/D2 — polling 函数读取增量的业务游标；停机靠 cursor 自愈（§6）。

### workflows（旧表加列）

`active` BOOL NOT NULL DEFAULT 0；`lifecycle_state` TEXT NOT NULL DEFAULT `'inactive'`，CHECK IN `(active, draining, inactive)`；`attention_reason` TEXT；`last_action_by` TEXT（`user`/`system`，区分自动 deactivate，ADR-022）。`concurrency`（旧有，派发器读）保留。

### function_versions（旧表加列）

`kind` TEXT NOT NULL DEFAULT `'normal'`，CHECK IN `(normal, polling)`；`polling_interval` TEXT（duration 串，仅 polling）——**polling 间隔的 canon 位置**（§7 trigger spec 不再放 `intervalSeconds`）。

### event type 全集(closed enum)

| 类 | type | record-once? |
|---|---|---|
| 结果 | `node_completed` · `branch_taken` · `signal_received` · `timer_fired` | **是**（partial unique on `dedup_key`） |
| attempt | `node_started` · `node_failed` | 否 — append-many，带 `attempt`（被 partial WHERE 排除） |
| 等待 | `signal_awaited` · `timer_armed` | 是（`signal_awaited` 是 journal **事件类型**；勿与 `flowruns.status='awaiting_signal'` 混） |
| agent 子步 | `agent_step_started` · `agent_step_completed` | completed 是（`dedup_key` 含 `turn`/`tool_call_id`，ADR-018） |
| 控制 | `flowrun_cancelled` · `replay_started` | 是 |

> **作用域变量不另立事件**(= 节点 `result`，见 §5)。

---

## 2. Journal 写入契约

- **per-flowrun 串行写**(单写入者 / 写锁);`seq` 写入事务内分配 → 严格单调（并行分支 goroutine 收束到单写入者，不裸写）。
- **record-once（ADR-018）**：`AppendEvent` = INSERT；命中 partial unique（`dedup_key`）= 已记账 → 取回既有行（compare-and-insert / first-wins）。`dedup_key` 由 app 一处计算：scalar = `node_id|iteration_key|type|generation`；agent 子步 = `…|turn|tool_call_id`；attempt 类 = `''`（被 WHERE 排除，自由 append）。
- **first-wins**：`signal_received` / `timer_fired` 同 record-once 路径 —— 第一条胜、撞键 no-op。**approval 的 timeout 也写 `signal_received`**（来源=timeout，见 §9/05），与用户决策同一 `dedup_key` 桶 → 双信号天然 first-wins，不会双端口点火。
- **attempt 类**(node_started/failed) append-many（带 `attempt`），不进 partial unique —— retry 多次失败都留痕。

---

## 3. 控制流激活 / join(契约 —— 根治 a1)

两种汇合，由 **split 类型**决定，**不可混用**：

- **AND-join**(并行扇出 AND-split 的汇合):`await 全部入边`。
- **active-branch join / simple-merge**(`case` XOR-split 分支的汇合，WP5):`await 被激活的入边`。

**激活如何确定 + 重放**:case 选边记 `branch_taken`(结果事件，已 journal)。**join “哪些入边激活” 由重放控制流从 `branch_taken` 推导，不需独立 edge/skip 事件** —— 重放走到 case → 读 `branch_taken` → 只激活该出边 → 未走分支的下游不执行 → 其汇合入边视为 skip。**无 skip 信号传播**（删 04 旧的“下发 skip 信号顺图传到汇合点”说法；精确算法 M3 落地）。

> **实现铁律**:join 等的是“**上游已激活且未被 skip 的入边**”，**不是静态全部**。任何解释器伪码/盘点(含 `11`)写“汇合 await 全部”**仅指 AND-split 扇出**;case 分支汇合必须写 active-branch。崩在 case 后 / join 前 → 重放从 `branch_taken` 重新推导，确定。

---

## 4. 重放 + replay-reset(契约 —— 根治 a2;ADR-019)

- **崩溃重放**:从头 replay，命中结果事件抄结果(不重跑 LLM/工具)，停在第一个未记账步真跑。`iteration_key` 由重放控制流确定地推导（ADR-017，循环 back-edge 序数）。
- **agent 子步**:每 tool-call/turn 记 `agent_step_*`;重放命中已记账子步抄、停在第一个未记账子步真跑（host 接口 `loop.Run(ctx, replayed []AgentStep)` —— 见 02 + ADR-010/M7）。
- **承重原则（ADR-019）**:一个步骤 `(flowrun_id, node_id, iteration_key)` 的**当前态 = 其最高 generation 的 record-once 事件**。
  - **copy-hit**:查该步最高 generation 结果事件 —— `node_completed`→抄（不重跑、不重写）;`node_failed` 为最高且当前 replay generation 更新→重跑 + 写 `node_completed@当前代`;无→首跑。
  - **`GET /flowruns/{id}/failures`**:最高 generation 事件为 `node_failed` 的步（未被后代 completion 覆盖）。
- **replay-reset(`:replay` 一个 `failed` flowrun)**：
  1. 写一条 `replay_started` → **`flowruns.generation += 1`**;`status` 回 `running`。
  2. 之后所有事件带**新 generation**;`seq` 继续递增(不回绕)。
  3. 终态 / 失败步只看最高 generation（上）—— 旧代 `node_failed` 是历史、不算当前态。
  4. record-once `dedup_key` 含 generation，故失败步在新代可重记 completed(不撞旧代)。

---

## 5. 作用域变量(契约 —— 根治 c2)

- **“作用域变量” = 上游节点的已记账输出(`node_completed.result`)，下游按数据流读** —— **不是独立 var store，不另立事件**。
- **可见性**:节点按其**入边来源**读上游输出;循环体读循环外节点的输出(那些在循环外已记账，重放白拿、不重算)。
- **continue-as-new**:journal 超 `pkg/limits` 高默认上限 → 把“当前仍被下游引用的最高-generation 节点输出”快照进新 flowrun 的 `input`，新段**继承**旧 `pinned_callables`（不重 pin active），旧 journal 归档（归档段不作 `:replay` 目标）。精确选取算法 / 阈值 M6 落地。

---

## 6. 触发 dispatch(契约 —— 根治 a4 / a5;ADR-021/022)

- **claim 原子性（单事务，ADR-021）**:dispatcher 消费一条 firing = **单 SQLite 事务内** `claim(pending→claimed)` + 建 flowrun（写 `flowruns` + 首条 journal + pin 闭包）+ 回填 `flowrun_id` + `status=started`。崩在 commit 前→回滚→firing 仍 `pending`→boot 重消费;崩在 commit 后→`started` + flowrun 在→boot 重放该 flowrun、派发器跳过(非 pending)。**无 “claimed 但无 flowrun” 中间态，无 lease，无 stale-claim 回收**。`StartRun` 须 tx-aware（接事务句柄）。
- **dedup_key(幂等材化，不丢且不重)**:`UNIQUE(workflow_id, trigger_node_id, dedup_key)`。
  - cron = `scheduled_at`(cron 表达式 + 时刻纯确定);webhook = 请求体 hash / 幂等头;**polling = `(cursor_in, 段内 event-index)`** —— `cursor_in` 是本次 poll 的**输入**游标（不是返回的 nextCursor），段内序号区分同批事件。游标必须前进（锻造契约，教学）;平台**检测游标不前进 + 报错**（诚实），绝不静默丢/误判重复。删 `11` 的 `source-event-id` 说法。
- **cursor 推进**:事件材化进收件箱(已落库)时推进 `polling_states.cursor`;失败 firing 走 replay(已 durable)，不靠 cursor 回退。
- **overlap / 并发**:派发器读 `workflows.concurrency` + `trigger_schedules.overlap_policy`;撞“正在跑”按 overlap(Skip/BufferOne[默认]/BufferAll/AllowAll)；超资源帽 → `status=shed` + 通知（C10）。manual 默认 AllowAll。
- **trigger 失败 → 自动 deactivate（ADR-022）**:failed firing → `consecutive_failures += 1`（成功置 0）;`≥ retry_policy.maxAttempts` → workflow 走 drain → `inactive` + `attention_reason` + `last_action_by='system'` + 通知。

---

## 7. 节点 config schema(契约 —— 根治 c1，字段名定死)

| 节点 | config(canon 字段名) |
|---|---|
| `trigger` | `{ kind, spec, payloadSchema? }`（spec 按 kind:cron→`{cron}`、webhook→`{path}`、**polling→`{functionRef}`**；polling 间隔在 `function_versions.polling_interval`，**不在 spec**）|
| `agent` | `{ agentRef }`(**canon=`agentRef`**，非 `ref`)；**无 retry/timeout 旋钮**(见 07) |
| `tool` | `{ callable, args, retry?, timeout? }`(**canon=`callable`**，非 `ref`) |
| `case` | `{ branches: [ { when, to, emit? } ] }`(有序列表，首个 `when` 真者胜，末条 `when:"true"` 兜底)|
| `approval` | `{ prompt, timeout?, timeoutBehavior?, allowReason }`，端口固定 `yes`/`no` |

> **节点级 timer gate（durable timer，00 §99-106）**:`at?`（绝对）/ `after?`（相对）是**任意非 trigger 节点**(agent/tool/case/approval) 的可选字段，与上面 config 并列。arm 写 `timer_armed`（含解析后绝对 deadline），放行写 `timer_fired`；重放读账里 deadline、不重算 now()。`02`/`03` 节点结构块须列出 `at?`/`after?`。
>
> **端口名 canon:approval = `yes` / `no`**（非 `approved`/`rejected`;`approved`/`rejected` 仅 `approvals.status` 值）。`02`/`03`/`04`/`05`/`13`/`15` 的字段名一律以本表为准。

---

## 8. 校验三层职责(契约 —— 根治 c3)

| 层 | 管什么 | 何时 |
|---|---|---|
| **① JSON schema validation** | config **形状**(字段名 / 必填 / 枚举 / ops·node.config pin 形状)| 工具调用 / accept 前 |
| **② capability_check** | 引用**存在** + kind 匹配 + handler `.method` 在 active version 还在 + 必填参数**给了值**(**不查类型**)| accept gate + 被引用实体改 active 时反向重查 |
| **③ 运行时** | 实际类型 / 值 / **outputSchema 强制(N1)** | activity 执行时(JSON-repair → validate → next_step retry)|

> ops / node.config **pin 形状**属 ①;**outputSchema 强制**属 ③;**capability_check 不碰类型**。三层不重叠。
> agent 节点 config 仅 `agentRef`、无参数槽 → ② 的“必填参数给了值”对 agent 节点不适用（只查 agentRef 存在 + kind=agent）；tool 节点“给了值” = arg 的 CEL 表达式非空（不求值、不查类型）。

---

## 9. approval 状态机 + tick(根治 a3 / a6)

- 端口 `yes`/`no`(§7)。状态:`parked` → `approved`(yes) / `rejected`(no 或 timeout-reject) / `timed_out` / `failed`(timeout-fail) / `cancelled`(flowrun 取消)。
- 挂起写一条 **`signal_awaited`** 事件（journal 事件类型；deadline 记其 result）+ `approvals` 行(parked)；`flowruns.status='awaiting_signal'`。
- timeout:`reject`→`rejected` 走 no;`approve`→`approved` 走 yes;`fail`→`failed` + flowrun `failed`。**timeout 与用户决策都写 `signal_received`（来源不同），同 `dedup_key` 桶 first-wins（§2）** —— 不会双端口点火。
- **progress tick**:走 notifications，**有 seq(uniform envelope，守 N7)但不进 replay buffer**;Last-Event-ID 重连跳过未缓存的 tick gap(**不是 seq-less** —— 详 08)。

---

## 引用本文的文档(DRY 出口)

| 文档 | 不再复制、改为引用本文的部分 |
|---|---|
| `00-overview.md` | schema(§1)、journal 写入(§2)、join 语义(§3)、replay(§4)—— **schema 块已删，指本文** |
| `11-integration-chains.md` | flowrun_events / approvals / trigger schema、解释器 join 伪码 —— **schema 副本已删，指本文 §1/§3**;`outcome`→终态 status |
| `02`/`03`/`04`/`05` | 节点 config 字段名 + timer gate —— 指本文 §7;`02` agent pin 见 ADR-020;`05` 等信号事件名 = `signal_awaited` |
| `13`/`15` | node.config pin 形状 / 字段名 —— 指本文 §7/§8 |
| `07` | replay-reset(§4)、校验分层(§8)、trigger retry/deactivate(§6 + ADR-022) |
| `01` | trigger dispatch / dedup / claim(§6 + ADR-021);polling 间隔在 function_versions |
| `16` | A-2 record-once = ADR-018;WP9 N-of-M = 刻意偏离(下沉 forge function) |

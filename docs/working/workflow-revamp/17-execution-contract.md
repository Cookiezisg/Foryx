# 17 — 执行契约(单一事实源:schema · join · replay · 6 大机制)

2026-05-31 建。**定位**:durable 执行**可实现契约的唯一事实源**。`00` 讲心智(why),本文讲精确 schema / 状态机 / 重放规则(how,照着建)。

> **DRY 铁律(根治 00/11 反复漂移)**:journal/approval/trigger **schema**、**join 语义**、**节点 config 字段名**、**状态枚举** —— **只在本文定义一次**;`00`/`11`/`02-05`/`13`/`15` 一律**引用本文**,不再各存一份。改契约 = 改本文一处。

---

## 1. Schema(唯一定义处)

```sql
flowruns ( id, workflow_id,
           version_id,            -- 启动时钉:图拓扑版本
           pinned_callables,      -- 启动时解析并 pin 的 callable→版本 快照(JSON);整 run 用它,无漂移(A-5)
           input,                 -- payload + ctx
           status,                -- running / awaiting_signal / completed / failed / cancelled
           trigger_node_id,       -- 哪个 trigger 起的(来源由其 kind 得知,无 is_from_listener)
           generation,            -- replay 代数(§4);初始 0
           started_at, ended_at )

flowrun_events ( id, flowrun_id, seq,    -- seq 写入事务内分配;UNIQUE(flowrun_id, seq)
                 type,                    -- 见下"event type 全集"
                 node_id, iteration_key,  -- replay key
                 attempt,                 -- 仅 attempt 类,append-many 留 retry 痕
                 turn, tool_call_id,      -- 仅 agent 子事件
                 generation,              -- 属哪一代(§4 replay)
                 result )                 -- JSON

approvals ( id, flowrun_id, node_id, prompt, payload,
            status,    -- parked / approved / rejected / timed_out / failed
            reason, created_at, decided_at )

trigger_schedules ( workflow_id, trigger_node_id, kind, spec,
                    last_fired_at, catchup_window, overlap_policy )

trigger_firings ( id, workflow_id, trigger_node_id, payload,
                  dedup_key,    -- §6 幂等键;UNIQUE(workflow_id, trigger_node_id, dedup_key)
                  status,       -- pending / claimed / started / skipped / superseded
                  claimed_at,   -- claim lease 时戳(§6 stale-claim 回收)
                  scheduled_at, enqueued_at, flowrun_id, outcome )

-- workflows 加列 lifecycle_state TEXT  -- active / draining / inactive
```

### event type 全集(closed enum)

| 类 | type | record-once? |
|---|---|---|
| 结果 | `node_completed` · `branch_taken` · `signal_received` · `timer_fired` | **是** — `UNIQUE(flowrun_id, node_id, iteration_key, type, generation)` |
| attempt | `node_started` · `node_failed` | 否 — append-many,带 `attempt` 序号 |
| 等待 | `signal_awaited` · `timer_armed` | 是 |
| agent 子步 | `agent_step_started` · `agent_step_completed` | completed 记一次(键含 `turn`/`tool_call_id`) |
| 控制 | `flowrun_cancelled` · `replay_started` | 一次 |

> **作用域变量不另立事件**(= 节点 `result`,见 §5)。

---

## 2. Journal 写入契约

- **per-flowrun 串行写**(单写入者 / 写锁);`seq` 写入事务内分配 → 严格单调。
- **record-once**:UNIQUE 只对**结果类**(上表),撞键 = 已记账丢弃。
- **first-wins**:`signal_received` / `timer_fired` compare-and-insert,第一条胜、第二条 no-op。
- **attempt 类**(node_started/failed)**append-many**(带 `attempt`),不设 type 级 UNIQUE —— retry 多次失败都留痕。

---

## 3. 控制流激活 / join(契约 —— 根治 a1)

两种汇合,由 **split 类型**决定,**不可混用**:

- **AND-join**(并行扇出 AND-split 的汇合):`await 全部入边`。
- **active-branch join / simple-merge**(`case` XOR-split 分支的汇合,WP5):`await 被激活的入边`。

**激活如何确定 + 重放**:case 选边记 `branch_taken`(结果事件,已 journal)。**join "哪些入边激活" 由重放控制流从 `branch_taken` 推导,不需独立 edge 事件** —— 重放走到 case → 读 `branch_taken` → 只激活该出边 → 未走分支的下游不执行 → 其汇合入边视为 skip。

> **实现铁律**:join 等的是"**上游已激活且未被 skip 的入边**",**不是静态全部**。任何解释器伪码/盘点(含 `11`)写"汇合 await 全部"**仅指 AND-split 扇出**;case 分支汇合必须写 active-branch。崩在 case 后 / join 前 → 重放从 `branch_taken` 重新推导,确定。

---

## 4. 重放 + replay-reset(契约 —— 根治 a2)

- **崩溃重放**:从头 replay,命中结果事件抄结果(不重跑 LLM/工具),停在第一个未记账步真跑。
- **agent 子步**:每 tool-call/turn 记 `agent_step_*`;重放命中已记账子步抄、停在第一个未记账子步真跑(host 接口:`loop.Run` 接 replayed-steps、跳过已完成 tool-call —— 见 02/§实现风险)。
- **replay-reset(`:replay` 一个 `failed` flowrun)**:
  1. 写一条 `replay_started` → **`flowruns.generation += 1`**;
  2. 之后所有事件带**新 generation**;`flowruns.status` 回 `running`;失败步在新代重跑。
  3. **flowrun 终态 / 失败步列表只看"最高 generation"的事件** —— 旧代 `node_failed` 是历史、不算当前态;新代 `node_completed` 即"已恢复"。
  4. record-once UNIQUE **含 `generation`**,故失败步在新代可重新记 completed(不撞旧代)。

---

## 5. 作用域变量(契约 —— 根治 c2)

- **"作用域变量" = 上游节点的已记账输出(`node_completed.result`),下游按数据流读** —— **不是独立 var store,不另立事件**。
- **可见性**:节点按其**入边来源**读上游输出;循环体读循环外节点的输出(那些在循环外已记账,重放白拿、不重算)。
- **continue-as-new**:滚动续期(§…/00 规模段)把"当前仍被下游引用的节点输出"快照进新 flowrun 的 `input`,旧 journal 归档。

---

## 6. 触发 dispatch(契约 —— 根治 a4 / a5)

- **claim 原子性(无卡死)**:dispatcher 消费一条 firing = **单事务内** `claim(pending→claimed)` + 建 flowrun + 回填 `flowrun_id` + `status=started`。**没有"claimed 但无 flowrun"的中间态**(原子)。
  - 退路(若实现拆两步):`claimed` 带 `claimed_at` lease;boot 时 **stale claimed**(超 lease 且无 flowrun_id)→ 回滚 `pending`。
- **dedup_key(幂等材化,不丢且不重)**:`UNIQUE(workflow_id, trigger_node_id, dedup_key)`。
  - cron = `scheduled_at`;webhook = 请求体 hash / 幂等头;**polling = `(nextCursor, 段内 event-index)`** —— **不要求 poll 函数返事件 ID**(用 cursor 段 + 段内序号,稳定可推导;a5 据此闭合)。
- **cursor 推进**:事件**材化进收件箱(已落库)时**推进,不等 flowrun 成功;失败 firing 走 replay(已 durable),不靠 cursor 回退。

---

## 7. 节点 config schema(契约 —— 根治 c1,字段名定死)

| 节点 | config(canon 字段名) |
|---|---|
| `trigger` | `{ kind, spec, payloadSchema? }`(spec 按 kind:cron→`{cron}`、webhook→`{path}`、polling→`{functionRef, intervalSeconds}`)|
| `agent` | `{ agentRef }`(**canon=`agentRef`**,非 `ref`)+ 节点级 timer gate `at?`/`after?`;**无 retry/timeout 旋钮**(见 07)|
| `tool` | `{ callable, args, retry?, timeout? }`(**canon=`callable`**,非 `ref`)+ timer gate `at?`/`after?` |
| `case` | `{ branches: [ { when, to, emit? } ] }`(首个 when 真者胜,末条 `when:"true"` 兜底)|
| `approval` | `{ prompt, timeout?, timeoutBehavior? }`,端口固定 `yes`/`no` |

> **端口名 canon:approval = `yes` / `no`**(非 `approved`/`rejected`;现代码用 `approved`/`rejected`,实现时改名)。`02`/`03`/`04`/`05`/`13`/`15` 的字段名一律以本表为准。

---

## 8. 校验三层职责(契约 —— 根治 c3)

| 层 | 管什么 | 何时 |
|---|---|---|
| **① JSON schema validation** | config **形状**(字段名 / 必填 / 枚举 / ops·node.config pin 形状)| 工具调用 / accept 前 |
| **② capability_check** | 引用**存在** + kind 匹配 + handler `.method` 在 active version 还在 + 必填参数**给了值**(**不查类型**)| accept gate + 被引用实体改 active 时反向重查 |
| **③ 运行时** | 实际类型 / 值 / **outputSchema 强制(N1)** | activity 执行时(JSON-repair → validate → next_step retry)|

> ops / node.config **pin 形状**属 ①;**outputSchema 强制**属 ③;**capability_check 不碰类型**。三层不重叠。

---

## 9. approval 状态机 + tick(根治 a3 / a6)

- 端口 `yes`/`no`(§7)。状态:`parked` → `approved`(yes) / `rejected`(no 或 timeout-reject) / `timed_out` / `failed`(timeout-fail)。
- timeout:`reject`→`rejected` 走 no;`approve`→`approved` 走 yes;`fail`→`failed` + flowrun `failed`。双信号 first-wins(§2)。
- **progress tick**:走 notifications,**有 seq(uniform envelope,守 N7)但不进 replay buffer**;Last-Event-ID 重连跳过未缓存的 tick gap(**不是 seq-less** —— 详 08)。

---

## 引用本文的文档(DRY 出口)

| 文档 | 不再复制、改为引用本文的部分 |
|---|---|
| `00-overview.md` | schema(§1)、journal 写入(§2)、join 语义(§3)—— 保留心智散文,schema 块指本文 |
| `11-integration-chains.md` | flowrun_events / approvals / trigger schema、解释器 join 伪码 —— 指本文 §1/§3 |
| `02`/`03`/`04`/`05` | 节点 config 字段名 —— 指本文 §7 |
| `13`/`15` | node.config pin 形状 / 字段名 —— 指本文 §7/§8 |
| `07` | replay-reset(§4)、校验分层(§8)|
| `01` | trigger dispatch / dedup / claim(§6)|

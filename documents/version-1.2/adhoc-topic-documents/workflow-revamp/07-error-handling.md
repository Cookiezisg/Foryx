# 07 — 错误处理 / 重试 / 通知 / 失败步与重跑

脑爆结论笔记(2026-05-27 起 / 2026-05-29 重大修正)。
2026-05-31 改向 durable execution(详 [`00-overview.md`](./00-overview.md))。

> 执行底盘已从 message-queue + actor 改为 **durable execution(持久化执行)**:workflow = 一段结构化程序,一次 flowrun = 把它确定性跑一遍,崩了照**事件日志(`flowrun_events` journal)**重放接着跑。本文的错误处理建在这套底盘上——**节点 = activity(记账步骤)**,失败 / retry / replay 都用"重放跳过已记账、重跑未完成"语义,不再有"消息进死信队列 / 复制消息进 queue"那套。底盘细节见 00,这里只讲错误路径。

---

## 核心规则

| 状态 | 行为 |
|---|---|
| 节点(activity)失败,**retry 次数内** | 只往事件日志记一笔失败,**不通知** |
| 节点失败,**retry 用尽** | **平台主动推 SSE 通知**——告诉用户哪个 workflow / 哪个节点失败 |
| **Trigger 节点** retry 用尽(特例) | **workflow 自动 inactive** + 通知(入口废了,workflow 客观失效) |
| 其他节点 retry 用尽 | flowrun 标 `failed` + 通知,**workflow.active 不变** |

派生:
- **通知是 mechanism**(平台保证)— retry 用尽必通知,用户始终知道
- **retry 次数是 policy**(用户/AI 编排时拍)— 不填 = 0 次,失败立即通知
- **Trigger 失败让 workflow inactive 不是"替用户暂停",是诚实**——入口废了,active 是欺骗

---

## Mechanism vs Policy(修正后的分配)

| 维度 | 谁定 |
|---|---|
| retry 次数 / backoff | **Policy** — 用户/AI 在节点 config 拍 |
| retry 用尽后是否通知 | **Mechanism** — 平台强制通知 |
| 通知载体 | **Mechanism** — SSE notifications 流(已有) |
| 错误分类(transient vs business) | **Policy** — case 节点显式判断 |
| 通知聚合规则(连续 N 次只报 1 次) | **Policy** — AI 在 chat 帮聚合;平台保证推每次重要事件 |
| Workflow.active 自动变 inactive | **Mechanism**(仅 trigger 失败例外)— 入口失效平台必须诚实 |
| 事件日志 retention / GC 默认值 | **Mechanism 兜底默认 + Policy 可覆盖** — 资源安全豁免,见下文 |

---

## retry 在 durable 模型里怎么跑

retry **不是**"复制一条消息再投递",而是 activity 在重放路径上的就地重跑:

```
某 activity(agent / tool 节点)Execute 报错
    ↓
解释器查该节点的 retry policy(maxAttempts / backoff)
    ↓ 还有次数 → 按 backoff 等待后重跑这一个 activity
        （重放语义:日志里已记账的上游 activity 直接抄结果,
          不重跑 LLM/工具；只有这个失败步真跑）
    ↓ 用尽 → 把"此 activity 永久失败"记进事件日志
        ├ Trigger 节点 → workflow 自动 inactive(下文特例)
        └ 其他节点    → flowrun 标 failed + 推通知
```

要点:
- 重跑只发生在**失败的那个 activity** 上;它的兄弟并行分支、上游结果都已在事件日志里,**不重算、不重复触发**。
- 进程崩在 retry 等待中间 → 重启重放时,该 activity 仍是"没记账完成"的步骤,自然接着试(retry 计数也记在日志里,不会丢)。
- 没有"retryAttempt 消息""死信队列"——只有日志上一笔笔的失败记录 + 一个永久失败标记。

---

## Trigger 节点的特例 — workflow 自动 inactive

入口失败是不同性质——workflow 客观上不能继续工作:

```
trigger 节点(例:cron / webhook / polling)失败
    ↓
平台按节点 retry 配置重试
    ↓ N 次后仍失败
    ↓
平台自动 deactivate workflow:
  workflow.active           = false
  workflow.attention_reason = "trigger_exhausted: <details>"
  workflow.last_action_by   = "system"        # 跟用户主动 deactivate 区分
  → 推 SSE notification:"workflow X 入口失效,需要人工干预"
    ↓
用户/AI 在 chat 看到 → 诊断 + 修 → 再 :activate
```

不影响:
- 其他节点(tool / agent)失败:只通知,workflow.active 不变
- Approval 节点超时拒绝(business 结果,不算节点失败;详 [`05-approval-node.md`](./05-approval-node.md))
- 用户主动 deactivate(`last_action_by = "user"`)

---

## 平台提供什么(机制层)

| 机制 | 干什么 |
|---|---|
| **事件日志记账**(`flowrun_events` journal) | 每个 activity 的开始 / 结果 / 失败按序 append,因果链 = seq 顺序,可 trace |
| **失败计数 + retry 编排** | 按节点 config retry 就地重跑失败 activity(重放跳过已记账),用尽后判定永久失败 |
| **永久失败标记** | activity 用尽 retry → 事件日志记一笔 `node_failed`;flowrun 标 `failed` |
| **通知 SSE** | retry 用尽 → 立即推 `notifications` 流 |
| **Trigger 失败 → workflow inactive** | 自动设 workflow.active=false + attention_reason + 推通知 |
| **确定性重放** | 崩溃 / replay 时从头重放程序,已记账步骤抄结果,停在第一个未完成处续跑(retry / 回边 / replay 共用这一条语义,见下文) |
| **Cancel 机制** | `:cancel` flowrun 时停掉解释器 goroutine,flowrun 标 cancelled |
| **超时杀进程** | workflow / 节点超时时强杀(用户配的超时值) |
| **失败步 / replay API** | 列失败步、看详情、`:replay` 重跑、清理(下文) |
| **事件日志 GC** | 资源安全兜底默认 retention,可被覆盖(下文) |

---

## 策略由 workflow 编排者拍

| 策略 | 怎么实现 |
|---|---|
| **失败重试次数 / backoff** | tool / agent / trigger 节点 config 填 `retry: {maxAttempts, backoff}`;不填 = 0 次,失败立即通知 |
| **错误分类** | 上游节点输出 error → **case 节点显式判断** + 路由(详 [`04-case-node.md`](./04-case-node.md)) |
| **通知后做什么** | workflow 内部画(连一个 notify tool 节点 / 走一条补偿分支 / 等) |
| **超时** | 节点 / workflow config 填 `timeout`;不填 = 永不超时 |
| **失败后要不要重跑** | 用户/AI 调 `:replay` 决定(下文) |
| **事件日志保留多久** | flowrun / workflow config 可覆盖平台兜底 retention(下文) |

---

## 失败步与重跑(replay)

durable 模型下,"某节点失败"不是一条进了死信队列的消息,而是**事件日志里一笔 `node_failed` + flowrun 的 `failed` 状态**。"再跑一次"就是**带着已有事件日志重新跑这个 flowrun**——这跟崩溃重放是同一条确定性重放语义(retry、case 回边、replay 三者共用,见 00"持久化"段)。

### replay 的语义(为什么零分叉、零重复)

```
flowrun 在某 activity 失败 → status=failed
    ↓ 用户/AI 调 :replay
解释器带着这本事件日志从头重放:
    ├ 已记账完成的 activity → 直接抄日志里的结果(不重跑 LLM/工具)
    ├ 失败的那个 activity   → 从这里真跑一次、记账
    └ 它的下游            → 本来就没记过账(没跑过)→ 自然首次执行
```

两个"由构造保证"的性质:
- **零分叉**:下游节点从没记过账 = 从没执行过,replay 不会"再点一次已经走过的下游"。控制流去哪是程序结构定的,不存在悬在队列里的旧消息被重复消费。
- **零重复**:失败步的兄弟并行分支(同一 join 的其他入边)早已把结果写进事件日志 / 程序作用域,replay 重放时直接抄,**不重跑也不会拿到两份**。循环外算的值同理是作用域变量,重放白拿。

### 幂等边界(明确归锻造)

replay 重跑失败 activity 时,**若该 activity 在崩溃前已经发生了外部副作用(发了邮件 / 写了外部库)但结果还没记账,重跑会重复那个副作用**。这是 **at-least-once 的固有边界,任何 durable 系统(含 Temporal)都一样**,不是 Forgify 的窟窿:

- **平台保证**:每个 activity 的结果在事件日志里**记一次账**,重放读账 → 不会重复调 LLM/工具。
- **锻造侧责任**:外部副作用的幂等性归**锻造**——编排者选 retry/replay + 把 tool(forge function/handler)写成幂等(带幂等键 / upsert / 先查后写),业务层即达成 exactly-once 效果。这是一条**命名清楚的责任线**,跟"能力源自 forge"一致(详 [`00-overview.md`](./00-overview.md) 持久化段 + [`03-tool-node.md`](./03-tool-node.md))。

### API(列失败步 / 看详情 / replay / 清)

| 操作 | API | 说明 |
|---|---|---|
| 列某 flowrun 的失败步 | `GET /flowruns/{id}/failures` | 返回事件日志里的 `node_failed` 项(node_id / iteration_key / error / attempts) |
| 看单条失败详情 | `GET /flowruns/{id}/failures/{nodeId}` | 该 activity 的输入、最后一次错误、retry 轨迹 |
| 重跑 flowrun | `POST /flowruns/{id}:replay` | 带现有事件日志重放(语义见上;语义由用户/AI 决定要不要跑) |
| 清失败 flowrun | `DELETE /flowruns/{id}` | 软删 flowrun + 其事件日志(也由 GC 兜底,见下文) |

> 对位 N5:`:replay` 是挂在 flowrun 上的标准 action,返 `{flowrunId}`(重放复用同一 flowrun,不新建)。`failures` 是 flowrun 的子资源,强制分页(N4)。

---

## 事件日志 retention / GC(资源安全兜底,修正旧"不填=无上限"footgun)

事件日志是 append-only,长期堆积会撑爆本地 SQLite。**这是资源安全问题,不是业务决定**,所以平台**必须给兜底默认值**(Mechanism vs Policy 的"安全/资源兜底"豁免,详 [`00-overview.md`](./00-overview.md) "资源 / 安全兜底"段):

| 项 | 谁定 | 默认 |
|---|---|---|
| 已完成 / 失败 flowrun 的事件日志保留期 | **平台兜底默认 + 用户/AI 可覆盖** | 对齐 sandbox 30 天 GC 先例(可在 flowrun/workflow config 覆盖) |
| `running` / `awaiting_signal` 的活跃 flowrun | 永不 GC(还在跑 / 还在等信号) | — |

要点:
- 这条**修正了之前"不填=无上限"的 footgun**——retention 默认不再是空,而是平台给的安全兜底值。
- 它**不违反** "平台永不替业务做默认值":GC 只删过期的**已结束** flowrun 历史,跟业务语义无关,纯粹防平台自己被日志撑死(与 CEL 100ms 超时、sandbox 内存上限同性质)。
- 用户/AI 想长留某些 flowrun 做审计 → config 覆盖更长 retention(显式 policy 压过兜底默认)。

---

## AI 在错误处理中的角色

通知到达 chat 后,AI 主动诊断 + 帮修:

```
平台推 SSE 通知:"workflow X 的 tool 节点重试 3 次都失败 (handler crash, OOM)"
    ↓
AI 在 chat 主动起话题(订阅 notifications 流):
  "workflow X 的 Gmail handler 重试 3 次都 crash 了。
   看了下代码,cache 没限制大小导致 OOM。
   要我帮你改 + replay 这个 flowrun 吗?"
    ↓
用户 "好"
    ↓
AI 调 edit_handler → :accept → POST /flowruns/{id}:replay
```

如果是 trigger 失败导致 workflow inactive:

```
平台推通知:"workflow X 入口 polling 失败 3 次,workflow 已 inactive"
    ↓
AI:"polling function 调 Gmail API 时 429,频率太高了。
   我看了下你设的 interval 是 5s,要不改成 60s 再 :activate?"
    ↓
用户 yes → AI edit polling function / 改 trigger interval → :activate
```

> 注:replay 会带着现有事件日志重放,已成功的步骤(如失败步之前已发出的邮件)**不会重跑**——只补失败步及其下游。若失败步本身有外部副作用,幂等性归锻造(见上文幂等边界)。

---

## 决策总览

```
1. retry 次数                   → tool / agent / trigger 节点 config(用户/AI 拍,不填=0)
2. retry 内失败                 → 只往事件日志记一笔,不通知
3. retry 用尽                   → 平台强制通知 SSE
4. Trigger retry 用尽           → workflow 自动 inactive + needs_attention + 通知
5. 其他节点 retry 用尽           → 通知 + flowrun 标 failed,workflow.active 不变
6. 通知载体                     → SSE notifications 流(已有)
7. 错误分类                     → case 节点显式判断
8. 失败步                       → 事件日志 node_failed + flowrun.status=failed(无死信队列)
9. 重跑(replay)               → :replay API,带现有日志重放(零分叉/零重复),语义由用户/AI 决定
10. 幂等边界                    → activity 外部副作用幂等归锻造(at-least-once 固有,Temporal 亦然)
11. 事件日志 retention          → 平台安全兜底默认(对齐 sandbox 30 天 GC),用户/AI 可覆盖
```

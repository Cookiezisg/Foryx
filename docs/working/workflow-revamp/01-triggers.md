---
id: WRK-001-02
type: working
status: active
owner: @weilin
created: 2026-05-20
reviewed: 2026-05-31
review-due: 2026-08-31
audience: [human, ai]
landed-into:
---
# 01 — Triggers

脑爆结论笔记(2026-05-27)。
2026-05-31 改向 durable execution(详 [`00-overview.md`](./00-overview.md))。
2026-05-31 补 Theme 3:trigger→dispatch→lifecycle 这一层做成 **durable 调度器 + durable 触发收件箱 + 优雅 drain 生命周期**。一条根原则:**先持久化再动作 + 受管生命周期,不许有 fire-and-forget**。本文是该层的主设计文档(数据模型 / 收件箱 / 持久调度 / 派发 / drain 见下方 CANON 段)。

---

## 触发器全集 — 5 种

| Kind | 信号源 | push/pull |
|---|---|---|
| `cron` | 时钟 | push |
| `fsnotify` | 本地文件系统 | push |
| `webhook` | 外部 HTTP | push |
| `polling` | 用户写的判断逻辑 | **pull** |
| `manual` | 用户 / AI 显式调用 | push |

push / pull 维度穷举,无第 6 种。

---

## polling = Function entity 的 kind=polling

polling trigger **本质是 function entity 加 `kind=polling` 字段**,**不开新 domain**。复用 function 全套基础设施(版本 / pending / sandbox / 锻造工具 / 试跑 / catalog),Quadrinity 不破。

### Function entity 加 kind 字段

```go
type Function struct {
    ID  string         // fn_xxx(共享 ID 前缀)
    // ❌ 不在 entity 级放 kind
}

type FunctionVersion struct {
    Kind            "normal" | "polling"   // ← version 级(每版可不同)
    Code            string                  // Python
    PollingInterval *Duration               // 仅 polling 时填(如 "60s")
    ...
}
```

**Kind 是 version 级**(跟 00 总纲 3 "永远 prod" 一致):
- 用户/AI 可在新 pending version 改 kind(normal → polling 或反之),平台 accept 时校验签名匹配
- active version 决定当前实际生效的 kind
- 引用方永远跟 active(无 pin),改 / revert active version 时引用方自动跟新

### 约束差(per polling version)

| 维度 | normal | polling |
|---|---|---|
| 签名 | 用户自定义 | 固定:`def poll(lastCursor) -> {"events": [...], "nextCursor": ...}` |
| 执行 | workflow tool 节点按需调 | 系统按 `PollingInterval` 反复跑 |
| 错误 | 冒泡到节点 | 冒泡到 trigger 系统 |
| 副作用 | 无约束 | 应只读 |
| 状态 | 无状态 | cursor 由系统持久化(每个 polling trigger 实例独立 cursor) |
| 输出 | 返一个值 | 返事件列表,N 个 → N 次执行(N 个 flowrun) |
| 被谁引用 | tool 节点 / agent.tools | trigger 节点 kind=polling |
| Catalog 视角 | callable catalog | trigger catalog |

输出必须是 **event-list + cursor**,不是 1/0 boolean——一次 polling 可能多事件、payload 跟触发耦合、cursor 必需。

### 跨用保护:Capability check

- trigger 节点 polling kind 引用 `fn_xxx`(默认 active)
- 用户 revert / edit fn_xxx,新 active version `kind=normal`
- **workflow accept 时 capability check**:trigger 节点要求 kind=polling,active 实际是 normal → check 失败
- workflow `:accept` 拒绝 / 标 needs_attention,推 AI / 用户
- AI 在 chat 里:"你把 fn_xxx 改成 normal 了,workflow Y 的 polling trigger 不能用了,要怎么办?"

### AI 锻造工具

不需要新加工具,复用现有 function 9 个工具:

- `create_function(kind: "normal" | "polling", ...)` — 必填 kind
- `edit_function(fn_xxx, ops=[..., update_kind: "polling", update_pollingInterval: "60s"])` — 可改 kind
- `run_function` — Kind=polling 时,平台模拟提供 lastCursor 试跑
- 其他工具(search / get / get_versions / revert / delete / accept / executions)无变化

### Catalog 自动按上下文过滤

UX 上 polling 跟 normal 不会混:

- 用户/AI 配 tool 节点 → search_functions 默认 `kind=normal`,polling **不出现**
- 用户/AI 配 polling trigger → search_functions 默认 `kind=polling`,normal **不出现**

ID 前缀仍 `fn_`(都是 function entity),分组在视图层。

### 失败处理(跟 07 doc 一致)

```
polling 跑一次 → 抛 exception(直接报错,不是返空 events)
    ↓
平台按 trigger 节点 retry config 重试
    ↓ retry 次数内,只记录,不通知
    ↓ retry 用尽仍失败
    ↓
平台自动:
  workflow.active           = false
  workflow.attention_reason = "trigger_exhausted: polling fn_xxx repeatedly failed"
  推 SSE notification → 用户/AI 在 chat 看到
    ↓
AI 主动诊断 + 帮修:"polling 调 Gmail 时 429,interval 太密。
                     要改成 60s 再 :activate 吗?"
```

**Trigger 失败让 workflow inactive 不是"替用户暂停",是诚实**——入口废了 workflow 客观不能跑。详 [`07-error-handling.md`](./07-error-handling.md) 的 Trigger 特例段。

---

## 对外契约统一

5 种内部实现不同,对外统一:**每个触发事件起一个独立 flowrun**(`scheduler.StartRun`),把 payload 作为该 flowrun 的程序入口输入喂给 trigger 节点。trigger 节点 = 程序入口(整次执行的起点,不是 activity),它的输出就是后继节点读到的输入(程序数据流;记进事件日志 — 见 [`00-overview.md`](./00-overview.md))。

| Kind | 一次几个事件 | 事件 payload |
|---|---|---|
| cron | 1 | `{firedAt}` |
| fsnotify | 1 / 文件事件 | `{firedAt, path, eventKind}` |
| webhook | 1 / 请求 | `{firedAt, method, headers, body}` |
| polling | 0~N | function 自定义 |
| manual | 1 | 用户传 |

下游 workflow 不需要知道 trigger kind,只面对统一的 payload(整次执行的入口输入)。

---

## 触发的统一抽象 = `(triggerNodeId, payload)`

不管谁触发,系统看到的都是同一个二元组:**指定哪个 trigger 节点 + 提供 payload**。

```
触发 = (triggerNodeId, payload)
       ↓
scheduler.StartRun(workflowId, triggerNodeId, payload)
       ↓
起一个 flowrun(确定性地把这张图跑一遍)
```

| 触发来源 | `(triggerNodeId, payload)` 谁提供 |
|---|---|
| cron / fsnotify / webhook / polling listener 自动 | listener 生成 |
| UI 用户点画布上某个 trigger 节点 + 填表单 | 用户 |
| AI 调 `trigger_workflow` 工具 | LLM(按 trigger 节点的 payloadSchema) |
| HTTP `POST /workflows/{id}:trigger { triggerNodeId, payload }` | 调用方 |

**三套入口汇聚到一个底层 API**。`StartRun` 是唯一入口。触发来源不影响实例归属:handler / agent 实例 Owner 恒为 `{Kind:"flowrun", ID:flowrunId}`,每个 flowrun 独占自己的实例、首次调用时 lazy spawn、flowrun 结束时 `DestroyOwner({Kind:"flowrun", ID:flowrunId})` 自清,无 `{Kind:"workflow"}` 共享实例、无跨触发复用。详 [`03-tool-node.md`](./03-tool-node.md) + [`06-workflow-lifecycle.md`](./06-workflow-lifecycle.md)。(暖复用如未来需要 = per-handler 的 ephemeral 资源池,非共享有状态实例,V1 不做。)

### Trigger 节点的 payloadSchema

每个 trigger 节点 config 声明自己的 payloadSchema(JSON schema),让调用方知道要塞什么:

```yaml
type: trigger
config:
  kind: manual
  payloadSchema:                 # 调用方必须按这个 schema 提供 payload
    date: string
    userId: string
```

listener 类型的 trigger 节点(cron 等),payloadSchema 由 kind 固定:cron 是 `{firedAt}`,webhook 是 `{method, headers, body}`,等等。

### UI 形态

Workflow 编辑器画布上,每个 trigger 节点角上有 `▶ 触发` 按钮:

- 点该按钮 → 弹**该节点 payloadSchema** 的表单
- 填完触发 → workflow 从这个 trigger 节点开始跑(起一个 flowrun)

**没有"workflow 顶部统一运行按钮"**——所有触发都是"指定哪个 trigger 节点"。

### Manual 节点 vs 其他 kind 显式触发

理论上,**任何 trigger 节点都可以被用户/AI 显式触发**(点 UI 按钮 / 调 `trigger_workflow`)。但语义不同:

| 节点 kind | 显式触发的语义 |
|---|---|
| cron / fsnotify / webhook / polling | "**调试 / 测试**"——平时由 listener 自动触发,显式触发 = 强制立即跑一次模拟 |
| **manual** | "**产品功能**"——这个 trigger 节点设计就是给用户/AI 显式调的,没 listener |

Manual 节点的存在意义 = **编排者明确声明"这个 workflow 接受手动调用"**。AI 在 chat 里发现 workflow 缺 manual 节点会自然反应"我加一个"——产品反馈循环的关键(详见 [`06-workflow-lifecycle.md`](./06-workflow-lifecycle.md))。

---

## trigger 的角色 = 任务分发器(经收件箱 → 派发器)

trigger 永远是**分发任务**:每个触发事件先落 `trigger_firings` 收件箱,再由**单派发器**按 `overlap_policy` + workflow `concurrency` 决定材化成几个 flowrun。常态是 **1 个事件 = 1 个 flowrun**;polling 的事件列表在收件箱层拆成 N 条 firing,**经派发器 → N flowrun**(不是 onFire 直起、不再是 trigger Service 的隐式直发)。

- polling 返 `[e1, e2, e3]` → 写 **3 条 trigger_firings**(每条一行、各一份 payload)→ 派发器按 overlap/concurrency **逐条材化 flowrun**(`scheduler.StartRun`,各自那份 payload 作入口输入)
- 每个 flowrun 内,trigger 节点把它那一份 payload 喂给后继节点
- workflow 内部永远是"单次执行单份输入",不存在 list 在节点间流动

**Fan-out 走"收件箱 → 派发器 → N flowrun",不在 workflow 图里画**——不需要 splitter 节点。早先把 fan-out 说成"trigger Service 隐式直发"的表述按本节口径作废:N 条事件 = N 条 firing,由派发器按并发/overlap 落地(详下方 CANON-DISPATCH)。

> **claim 原子性**(`pending→claimed` + 建 flowrun + 回填 `flowrun_id` 单事务,无"claimed 但无 flowrun"卡死态)**+ dedup_key 幂等键**(cron=`scheduled_at` / webhook=请求 hash / polling=`(cursor, 段内 index)`,**不要求 poll 函数返事件 ID**)的精确契约见 [`17`](./17-execution-contract.md) §6。

理由:

| | flowrun 层 fan-out(本设计) | workflow 内 splitter 节点 |
|---|---|---|
| 错误隔离 | 挂 1 个 flowrun 不影响其他 | 同 flowrun 内一挂全挂语义模糊 |
| 取消粒度 | cancel 单 flowrun 直接 | cancel 部分子图复杂 |
| 历史统计 | "trigger 跑了 N 次" 清晰 | 1 次还是 N 次?语义模糊 |
| 节点 API | trigger 节点入口永远单份输入 | 用户每次都要记"我在面对 list" |
| 并发控制 | 复用 flowrun 层的 Concurrency gate | 节点级并发要重做 |

> 注:这里的"对 N 个事件各起一个 flowrun"是 **trigger 层** 的扇出(N 次独立执行),跟 **图内** 的并行无关。图内并行是同一次执行里的 fork-join(普通节点多出边 = fork,汇合处 = await 全部),静态、自包含——详 [`00-overview.md`](./00-overview.md) 并发模型段。两者不要混。

铁律:**一个 trigger 事件永远对应一条 firing**(polling 的事件列表在收件箱层拆成 N 条 firing),firing 是否、何时材化成 flowrun 由派发器按 overlap/concurrency 定,而不是把 list 塞进一次执行。

---

## Theme 3 — durable 调度器 + 触发收件箱 + 优雅 drain

把上面"统一抽象 `(triggerNodeId, payload)` → `StartRun`"这条线在**边界处**做成 durable:触发不再 fire-and-forget,而是**先持久化(收件箱)→ 受管派发 → 受管生命周期**。一条根原则贯穿本节:**先持久化再动作 + 受管生命周期,不许有 fire-and-forget**。

> 与 Theme 1 的分工:Theme 1 = flowrun **内部** durable(journal + 重放);Theme 3 = trigger→dispatch→lifecycle **边界** durable(收件箱 + 持久调度 + drain)。两层合起来端到端无 fire-and-forget。

### CANON-DATA — 数据模型(单进程 / SQLite,3 处)

| 表 / 列 | 字段 | 作用 |
|---|---|---|
| `trigger_schedules` | `workflow_id, trigger_node_id, kind, spec, last_fired_at, catchup_window, overlap_policy` | **持久化 listener 注册**,取代内存里的 lastFire。原 `workflow.LastFiredAt` 是 `gorm:"-"` 不入库,正是 E1(重启漏触发)根因 —— `last_fired_at` 落库根治。 |
| `trigger_firings` | `id, workflow_id, trigger_node_id, payload, scheduled_at, enqueued_at, status(pending/claimed/started/skipped/superseded), flowrun_id, outcome` | **durable 触发收件箱**,每条触发一行、每条都有 outcome。也走事件日志 GC 默认 retention。 |
| `workflows.lifecycle_state` | `active / draining / inactive` | 给 drain 用(见 CANON-DRAIN)。停新后等在途 flowrun 各自跑完(每个结束时自销其独占实例),无实例级 refcount。 |

### CANON-INBOX — 收件箱

任何触发(cron / fsnotify / webhook / polling / manual)在**尝试跑之前先写一条 `trigger_firings`**(先持久化再动作);所有触发统一走 **收件箱 → 派发器 → flowrun**。**已落库 firing 不丢**:崩在"firing 落库 → flowrun 起"之间,重放从收件箱续。**落库前的内存窗口**("事件到达 → 写 firing",C9):webhook **落库后才返 200**(崩在落库前 → 无 200 → 发送方按 webhook 标准重试,at-least-once 覆盖该窗口);fsnotify 无重试、该窗口 best-effort(开机扫现状兜底)。所以"不丢"精确指**已落库 firing**,不假装兜住外部 ephemeral 的落库前内存窗口。

- 这正是上面"`StartRun` 是唯一入口"的 durable 化:listener / UI / AI / HTTP 四套入口都先落 firing,派发器再调 `StartRun`。
- **manual 默认 `overlap=AllowAll`**(显式动作,立即跑)。

### CANON-SCHEDULE — 持久调度 + Catchup

`last_fired_at` 落库;开机按 cron 表达式算 `last_fired_at → now` 漏了哪几次,按 **Catchup Window 策略**材化进收件箱。

策略(编排者拍,对接 [`desktop-packaging-notes.md`](../desktop-packaging-notes/desktop-packaging-notes.md) 早标的"错过任务策略"三档):

| catchup_window | 行为 |
|---|---|
| 不补 | 停机期漏的全丢,只从现在起 |
| 补最近一次(**默认**) | 只补一条,代表"该跑而没跑" |
| 补窗口内全部 | 漏几次补几次 |

**诚实边界**:cron 靠 cron-math 补、polling 靠 cursor 自愈(下次 poll 从 lastCursor 续;**cursor 在事件材化进收件箱[已落库 firing]时推进、不等 flowrun 成功 —— 某条 firing 的 flowrun 失败由失败步 / replay 兜,firing 已 durable、有 outcome,不靠 cursor 回退**,C8);**webhook / fsnotify 的停机期事件是外部 ephemeral,客观找不回**(可选 fsnotify 开机扫现状兜底),明说**不假装兜住**。

### CANON-DISPATCH — 单派发器 + Overlap

派发器按 `workflow.concurrency` + `trigger.overlap_policy` 消费收件箱;撞上"正在跑"时按 `overlap_policy`:

| overlap_policy | 行为 |
|---|---|
| `Skip` | 跳过但记 `outcome=skipped`(+ 可通知),**绝不静默** |
| `BufferOne`(**默认**) | 只留最新一个排队(新来 supersede 旧排队项,旧记 `superseded`) |
| `BufferAll` | 全排队、按序跑 |
| `AllowAll` | 无视上限并发 |

排队的 firing 留在收件箱(`pending` / buffered)空了再跑。

**铁律:每条 firing 都有 outcome,绝不静默丢。** 这是 E2 的根治 —— 取代现状 `onFire` 对 `ErrConcurrencyLimit` 只 `log+return` 的静默丢。

**资源安全帽(C10,属"防平台崩"豁免,非业务 policy)**:`BufferAll` 的收件箱深度、`AllowAll` 的并发各给一个**高默认上限**(走 `pkg/limits` 配置,见 limits-optimization)——抖动的高频 webhook + `AllowAll` 不会无界 spawn 撑爆平台。超帽**不静默丢**:落 `outcome=shed` + 通知(守"每条 firing 有 outcome"铁律)。这是资源兜底(00 的 mechanism-vs-policy 明确豁免"防平台自己崩"),**不是替用户拍业务并发**。

### CANON-DRAIN — 优雅 drain(解 E6)

`:deactivate` / `:accept` **不即时 `DestroyOwner`**(那会抽在途),走状态机:

1. **停新** —— 撤 listener + 派发器不再起新 flowrun,workflow 进 `draining`
2. **排空** —— 在途 flowrun(durable、靠 journal 活着)在老版本跑完,各自结束时 `DestroyOwner({Kind:"flowrun", ID:flowrunId})` 自销其独占实例;无 refcount、无共享 handler
3. **`in-flight=0` 后** —— (`:accept`)挂新版本 listener → `inactive` / 新 `active`;无 workflow 级共享实例需销毁(各 flowrun 已自清)

零停机、**绝不抽在途**。Owner 恒为 `{Kind:"flowrun", ID:flowrunId}` —— 实例 per-flowrun 隔离,绝不跨触发共享或复用。详 [`03-tool-node.md`](./03-tool-node.md) + [`06-workflow-lifecycle.md`](./06-workflow-lifecycle.md)。

### CANON-MP — Mechanism vs Policy

| | 内容 |
|---|---|
| **平台保证(mechanism)** | 触发绝不静默丢失 / 每条 firing 有 outcome / 在途绝不被强拆 |
| **编排者拍(policy)** | `catchup_window`(补多少)、`overlap_policy`(撞车怎么办) |

平台不替业务猜,给**显式选项 + sane 默认**:`overlap` 默认 `BufferOne`、`catchup` 默认补最近一次。

### CANON-BOOT — 启动序列

重启 = (a) 从 `trigger_schedules` 重挂 listener;(b) 按 cron-math + catchup 把漏的 firing 材化进收件箱;(c) 从 journal 重放在途 flowrun(Theme 1,详 [`00-overview.md`](./00-overview.md));(d) 派发器继续消费收件箱。

---

## 差异化锚点

Forgify vs Zapier/n8n 的差别:**polling function 由 AI(forge)帮造**,而非平台预集成 / 用户手写。

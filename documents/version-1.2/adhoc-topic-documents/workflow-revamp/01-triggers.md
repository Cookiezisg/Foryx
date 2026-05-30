# 01 — Triggers

脑爆结论笔记(2026-05-27)。
2026-05-31 改向 durable execution(详 [`00-overview.md`](./00-overview.md))。

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
scheduler.StartRun(workflowId, triggerNodeId, payload, isFromListener)
       ↓
起一个 flowrun(确定性地把这张图跑一遍)
```

| 触发来源 | `(triggerNodeId, payload)` 谁提供 | isFromListener |
|---|---|---|
| cron / fsnotify / webhook / polling listener 自动 | listener 生成 | `true` |
| UI 用户点画布上某个 trigger 节点 + 填表单 | 用户 | `false` |
| AI 调 `trigger_workflow` 工具 | LLM(按 trigger 节点的 payloadSchema) | `false` |
| HTTP `POST /workflows/{id}:trigger { triggerNodeId, payload }` | 调用方 | `false` |

**三套入口汇聚到一个底层 API**。`StartRun` 是唯一入口。`isFromListener` 决定本次执行内 handler 实例的 Owner(`true→{workflow}` 跨触发复用;`false→{flowrun}` 隔离),详 [`03-tool-node.md`](./03-tool-node.md) + [`06-workflow-lifecycle.md`](./06-workflow-lifecycle.md)。

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

## trigger 的角色 = 任务分发器

trigger 永远是**分发任务**:**1 个事件 = 1 个 flowrun**,每次给下游 workflow 一次独立执行。

- polling 返 `[e1, e2, e3]` → trigger Service **起 3 个独立 flowrun**(各调一次 `scheduler.StartRun`,各自一份 payload 作入口输入)
- 每个 flowrun 内,trigger 节点把它那一份 payload 喂给后继节点
- workflow 内部永远是"单次执行单份输入",不存在 list 在节点间流动

**Fan-out 是 trigger Service 的隐式行为,不在 workflow 图里画**——不需要 splitter 节点。

理由:

| | flowrun 层 fan-out(本设计) | workflow 内 splitter 节点 |
|---|---|---|
| 错误隔离 | 挂 1 个 flowrun 不影响其他 | 同 flowrun 内一挂全挂语义模糊 |
| 取消粒度 | cancel 单 flowrun 直接 | cancel 部分子图复杂 |
| 历史统计 | "trigger 跑了 N 次" 清晰 | 1 次还是 N 次?语义模糊 |
| 节点 API | trigger 节点入口永远单份输入 | 用户每次都要记"我在面对 list" |
| 并发控制 | 复用 flowrun 层的 Concurrency gate | 节点级并发要重做 |

> 注:这里的"对 N 个事件各起一个 flowrun"是 **trigger 层** 的扇出(N 次独立执行),跟 **图内** 的并行无关。图内并行是同一次执行里的 fork-join(普通节点多出边 = fork,汇合处 = await 全部),静态、自包含——详 [`00-overview.md`](./00-overview.md) 并发模型段。两者不要混。

铁律:**一个 trigger 事件永远对应一次 flowrun**(polling 的事件列表在 trigger Service 层拆成 N 次 StartRun,而不是把 list 塞进一次执行)。

---

## 差异化锚点

Forgify vs Zapier/n8n 的差别:**polling function 由 AI(forge)帮造**,而非平台预集成 / 用户手写。

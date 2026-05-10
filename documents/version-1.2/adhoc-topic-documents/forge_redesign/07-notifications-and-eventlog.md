# Notifications & Eventlog Multi-Scope — 通知与流式协议总览

**关联**:
- [`00-overview.md`](./00-overview.md) — 顶层愿景(D18 Transport / D19 entity-level scope)
- [`05-execution-plane.md`](./05-execution-plane.md) — Transport Layer + Eventlog 协议泛化

**定位**:V1.2 + forge_redesign V1 全套通知 type + eventlog scope 总表。**前端按当前 view 选择订阅哪些**。

---

## 1. 两条管道分工

| 管道 | 用途 | 内容形态 |
|---|---|---|
| `/api/v1/eventlog?scope=...` | **实时流式细节**(LLM 输出 / ops apply / run 进度 / 锻造过程) | 5 events × 6 block types,parentId 嵌套 |
| `/api/v1/notifications` | **全局 entity 状态变化**(create / delete / status change 等) | 一律 `{type, id, data, conversationId?}` envelope |

简记:**eventlog = "正在发生的过程",notifications = "发生了什么大事"**。

---

## 2. Eventlog Scope 清单(V1 = 5 种)

```
conversation:cv_xxx     主对话流(chat 内 LLM 输出 / tool calls)
flowrun:fr_xxx          workflow 执行实例的完整事件流
function:fn_xxx         该 function 锻造期 ops 流(D19 — 用户在 Function 详情页订)
handler:hd_xxx          该 handler 锻造期 + instance 生命周期事件流(D19)
workflow:wf_xxx         该 workflow 锻造期 + edit ops 流(D19)
```

### 2.1 Multi-scope 单连接(可选,HTTP/2 落地后随意)

HTTP/2 + TLS 落地(D18)后,**前端订阅策略自由**:
- **方式 a**:一条 SSE,query 带多个 scope `?scope=conversation:cv_xxx&scope=workflow:wf_yyy`
- **方式 b**:多 SSE,每 scope 一条

两种都没有 HTTP/1.1 那样的 6-connection 限制问题。后端 Bridge `Subscribe([]Scope)` 接受多 scope 一条订阅(实施细节见 §4 章节)。

### 2.2 多 Scope 双写策略(LLM 锻造期)

LLM 在 chat 里调 `edit_function` / `edit_handler` / `edit_workflow` 时,系统**同时把事件写两个 scope**:

| Scope | 内容 | 用途 |
|---|---|---|
| `conversation:<convId>` | 挂 chat 的 `tool_call` 父下 | chat 用户实时看锻造过程 |
| `function:<fnId>` / `handler:<hdId>` / `workflow:<wfId>` | 直接挂在 entity 的 scope | Function/Handler/Workflow 详情页直接订看 |

非 chat 触发的事件:
- HTTP / UI 直触发(用户点 Run / Resync / PATCH config):**单写 entity scope**
- 内部触发(scheduler / watcher / catalog refresh):**单写 entity scope**

---

## 3. Notifications Type 总表

### 3.1 V1.2 现状(继承,不动)

| Type | 触发 | data 主要字段 |
|---|---|---|
| `conversation` | autoTitle / Create / Update / Delete | `{convId, title, ...}` |
| `todo` | CRUD | `{todoId, status, ...}` |
| `mcp_server` | Connect / Disconnect / Add / Remove / health change | `{name, status, error?}` |
| `skill` | Scan / Create / Replace / Delete | `{name, ...}` |
| `catalog` | Refresh | `{fingerprint, version, generatedAt}` |

### 3.2 forge_redesign V1 加

| Type | 触发 | data 主要字段 |
|---|---|---|
| `function` | created / pending / accepted / rejected / deleted | `{id, name, action, ...}` |
| `handler` | created / pending / accepted / rejected / deleted / config_updated | `{id, name, action, configState?, ...}` |
| `handler_instance` | spawned / destroyed | `{handlerId, instanceId, ownerKind, ownerId, reason}` |
| `workflow` | created / pending / accepted / rejected / deleted / enabled_changed / needs_attention / capability_removed | `{id, name, action, attentionReason?, ...}` |
| `flowrun` | started / paused / resumed / completed / failed / cancelled / timeout | `{runId, workflowId, status, errorCode?, ...}` |
| `trigger` | registered / unregistered / fired / error | `{workflowId, triggerKind, ...}` |

---

## 4. 前端订阅策略(每页推荐)

| 当前页 | eventlog 订 | notifications 过滤 |
|---|---|---|
| 主对话(chat conv) | `scope=conversation:cv_xxx` | 全部(显示 toast)|
| Function 详情页 | `scope=function:fn_xxx` | `type=function` |
| Handler 详情页 | `scope=handler:hd_xxx` | `type IN (handler, handler_instance)` |
| Workflow 编辑器 | `scope=workflow:wf_xxx` | `type=workflow` |
| FlowRun 监控 | `scope=flowrun:fr_xxx` | `type=flowrun` |
| 列表页(Function / Handler / Workflow / FlowRun) | 不订 eventlog | 过滤对应 type — 列表项实时更新 |
| 仪表盘(全局视图)| 不订 eventlog | 全部 |

---

## 5. Capability 删除引发的级联通知(扩 D12)

V1 决策:删除 function / handler / workflow / mcp_server / skill 时,系统发对应通知,**引用此 capability 的其他 entity 自动标 `needs_attention`**(workflow 域 entity 上加这个状态字段)。

| 被删 | 发通知 type | 级联效果 |
|---|---|---|
| `delete_mcp_server` | `mcp_server_uninstalled` (现有) | 引用此 server 的 workflow → `needs_attention` |
| `delete_function` (新)| `function` (action=deleted) | 引用此 function 的 workflow → `needs_attention` |
| `delete_handler` (新)| `handler` (action=deleted) | 引用此 handler 的 workflow → `needs_attention` |
| `delete_skill` (V1 加级联)| `skill` (action=deleted) | 引用此 skill 的 workflow → `needs_attention` |
| `delete_workflow` | `workflow` (action=deleted) | 仅通知;别的 workflow 不能引用它(V1 不做 nested workflow) |

### 5.1 `needs_attention` workflow 行为

- **trigger 仍 register** 但触发时 fail-fast(`WORKFLOW_CAPABILITY_REMOVED` 422 / 502)
- **UI 列表项显示警告标识** + 建议用户 `edit_workflow` 替换或 `delete_workflow`
- **enabled** 字段不变(用户主动 disable 才停 trigger;系统不擅自停)

### 5.2 实施扇出

`workflow domain` 监听 5 类 entity 的 deletion notification → 扫所有 workflow graph → 找引用 → 标 needs_attention + 单独发 `workflow.action=needs_attention` 通知。

---

## 6. 实施成本

| 改动 | LOC |
|---|---|
| `domain/eventlog` Scope multi-subscribe | ~80 |
| `domain/notifications` type 枚举完整化 + helpers | ~120 |
| 各 service publish notification(function/handler/workflow/flowrun/trigger 5 个 service) | ~200 |
| workflow `needs_attention` 检测器 + 跨域 deletion listener | ~100 |
| HTTP `?scope=` 多参解析(eventlog handler) | ~50 |

总 ~550 行 backend(分散到各 domain)。前端订阅模式留 Wails 实施期,~300 行。

---

## 7. 不在范围

- **跨 user 共享 notification**(单用户场景)
- **Notification 持久化**(V1 全是 in-memory + SSE 推送;用户开 app 之前的事件丢失,不补)— V1.5 看需要加 `notifications_log` 表
- **Notification 优先级 / 严重度**(全部 info-level)— V1.5 加 severity:info / warn / error
- **批量 notification 节流**(短时间内大量同 type 通知)— V1.5 加 BurstCoalescer

---

(本文档完)

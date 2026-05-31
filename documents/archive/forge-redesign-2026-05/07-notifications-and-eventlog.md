# Notifications & Eventlog & Forge — 三协议 SSE 总览

**关联**:
- [`00-overview.md`](./00-overview.md) — 顶层愿景
- [`discussions/2026-05-12-env-and-sse-rework.md`](./discussions/2026-05-12-env-and-sse-rework.md) §A-C — 本文档的事实源(三流统一 + payload 瘦身 + forge 流新建)
- [`../../service-contract-documents/events-design.md`](../../service-contract-documents/events-design.md) — wire 契约 + producer 责任分配 + Bridge 实现
- [`../../event-log-protocol.md`](../../event-log-protocol.md) — eventlog 5 events × 6 block types 完整 schema

**定位**:V1.2 + forge_redesign V1 全套 SSE 流形态 + Notification type 总表 + Capability 删除级联策略。

---

## 1. 三条 SSE 流分工(+ 1 张持久化表系列)

| 管道 / 数据 | 订阅 key | 用途 | 内容形态 |
|---|---|---|---|
| `GET /api/v1/eventlog` | **user_id** | **实时 chat 内容流**(LLM 输出 / tool calls / subagent 嵌套 / progress)| 5 events × 6 block types,parentId 递归 |
| `GET /api/v1/notifications` | **user_id** | **entity 状态变更**(create / delete / status change 等)| 1 通用 envelope `{type, id, data, conversationId?}`,**data 瘦身只送 ID + 必要小字段** |
| `GET /api/v1/forge` | **user_id** | **trinity 锻造进度流**(function / handler / workflow 的 create/edit/revert/delete + env-fix attempts)| 4 events × 3 kinds 封闭 |
| **execution log 表系列**(D22) | DB(非 SSE)| 持久化执行历史 | 5 张 per-entity 表 |

简记:
- **eventlog** = "chat 内正在发生的过程"(实时,SSE,per-user)
- **notifications** = "entity 发生了什么大事"(实时,SSE,per-user,**瘦身**)
- **forge** = "trinity 锻造的实时进度"(实时,SSE,per-user)
- **execution log** = "干完了的事的账本"(持久化,DB,debug / 审计 / LLM 诊断用)

四者独立 data plane,互不重复。execution log 不发 SSE(高频低价值),notification 不持久化(in-memory broadcast)。

**SSE 上限三条**(D-redo-5,2026-05-12):**永远不再加新 SSE 流**。所有未来"entity 详情面板想看实时事件"需求 → 走 forge 流(锻造)或 eventlog 流(chat 内容,client 按 conversationId demux)+ client filter,或经 Wails native event 机制(打包阶段实施,绕过 HTTP)。

---

## 2. 为何按 user_id 订(而非 per-conversation / per-entity)

**问题历史**:V1.2 初期 SSE 按 per-conversation 订(eventlog `?conversationId=`)+ global broadcast(notifications)+ 拟加 per-entity scope(forge_redesign D19 早期提案)。testend 多 panel + 详情页同时活跃 → 撞 HTTP/1.1 浏览器 **6 connection per origin** 限制。

**解决路径权衡**(讨论:[`discussions/2026-05-12-env-and-sse-rework.md`](./discussions/2026-05-12-env-and-sse-rework.md) §A-B):
- 选项 A:HTTP/2 + TLS(原 D18)— mkcert binary 25MB / 首次 sudo 装 CA / 18 月证书轮换,负担太重
- 选项 B:**3 条 SSE 流按 user_id 订,client 自己 demux**(选定)— 永远只占 3 个连接,远低于 6
- 选项 C(打包阶段):Wails native event 机制完全绕过 HTTP — V1.5 / 打包时实施,届时连 SSE 都可考虑撤掉

**选 B 的本质收益**:
- 不引入 TLS / mkcert / HTTP/2 任何东西
- backend Bridge 改 key 即可(`user_id` 替 `conversation_id` / "global")
- payload 字段(conversationId / scope.kind / scope.id 等)给 client demux 用,wire 协议没有破坏性变化
- 单用户场景下,user_id = "local-user"(per CLAUDE.md project specialty),wire 上等同 broadcast

---

## 3. 三流 wire 形态速查

详细 schema 见 [`../../service-contract-documents/events-design.md`](../../service-contract-documents/events-design.md)。这里只列**关键 wire 字段** + payload 示例。

### 3.1 eventlog wire(5 events,无 query)

```
GET /api/v1/eventlog HTTP/1.1
Accept: text/event-stream
Last-Event-ID: 42      ← optional, 重连时

event: block_delta
id: 43
data: {"conversationId":"cv_abc","id":"blk_xyz","delta":"hello"}

```

5 事件:`message_start` / `message_stop` / `block_start` / `block_delta` / `block_stop`。6 block 类型:`text` / `reasoning` / `tool_call` / `tool_result` / `progress` / `message`。

**Client demux**:按 `payload.conversationId` 把事件 dispatch 到对应 panel(主对话 / 历史 conv 子 panel / testend 多 conv)。

### 3.2 notifications wire(瘦身 payload,无 query)

```
GET /api/v1/notifications HTTP/1.1
Last-Event-ID: 87

event: notification        ← 硬码字面量
id: 88
data: {"type":"function","id":"fn_abc","data":{"action":"pending_created","versionId":"fnv_xyz"},"conversationId":"cv_aaa"}

```

**Payload 瘦身**(D-redo-6):`data` 字段**只送 ID + 必要小字段**(`{action, versionId?, versionNumber?}` 等),**禁止塞完整 entity**。UI 拿通知 → 主动 GET 详情。带宽 / 心智 / 一致性都更好。

### 3.3 forge wire(4 events,无 query)

```
GET /api/v1/forge HTTP/1.1
Last-Event-ID: 12

event: forge_env_attempt
id: 13
data: {"scope":{"kind":"function","id":"fn_abc"},"attempt":2,"status":"failed","error":"No matching distribution"}

```

4 events:`forge_started` / `forge_op_applied` / `forge_env_attempt` / `forge_completed`。3 kinds:`function` / `handler` / `workflow`(封闭)。

**Payload 嵌套 Scope struct**(D-redo-23):`{"scope":{"kind":"function","id":"fn_x"}, ...}` — 复用 `domain/eventlog/scope.go` 的 Scope 类型,不平铺 `{kind, entityId}`(更清晰 + 未来若 entity id 复合也能装)。

---

## 4. Notifications Type 总表

V1.2 现有 + forge_redesign V1 新增。**所有 type 的 data 字段都瘦身**(per §3.2 / D-redo-6)。

### 4.1 V1.2 现有(瘦身后)

| Type | producer | 触发场景 | data 字段(瘦身) |
|---|---|---|---|
| `conversation` | `app/conversation/Service` + chat runner autoTitle | 创建 / 改 title / 软删 / autoTitle | `{action}` |
| `todo` | `app/todo/Service.publish` | 任意 CRUD | `{action, status?}` |
| `mcp_server` | `app/mcp/Service` + `publishStatus` | server 增删改 / 重连 / 健康检查 | `{action, status?, error?}` |
| `skill` | `app/skill/Service.Scan` 轮询 | 添 / 改 / 删 SKILL.md | `{action}`(client 全部重读)|
| `catalog` | `app/catalog/Service.applyRefresh` | poll fingerprint 变化 | `{action, fingerprint}` |
| `sandbox_env` | `app/sandbox/Service` | env 状态变 / env 软删 | `{action}` |

### 4.2 forge_redesign 新增

| Type | producer | 触发 action | data 字段(瘦身) |
|---|---|---|---|
| `function` | `app/function/Service` 各 CRUD | `created` / `updated` / `pending_created` / `version_accepted` / `pending_rejected` / `reverted` / `deleted` | `{action, versionId?, versionNumber?}` |
| `handler` | `app/handler/Service` 各 CRUD | 同 function 7 个 + `config_updated` / `config_cleared` | `{action, versionId?, versionNumber?}` |

**已删除的 action**(D-redo-7):
- `env_synced` / `env_failed`(function + handler 各一对)— env 终态信息走 LLM tool_result 返,UI 经 GET 拉,**不需要异步推**
- 原本拟加的 `handler_instance` type — Instance 是运行时对象 in-memory,LLM 通过 GET `/handlers/{id}` 看 `liveInstances` 计算字段即可

**workflow / flowrun / trigger** 等 type 留待 Plan 04+ 实施,届时按瘦身规则定 data 字段。

---

## 5. 前端订阅策略(每页推荐)

V1.2 后端期不动前端(per CLAUDE.md 设计原则 #4),本节给 Wails 实施期参考。

| 当前 view | eventlog | notifications | forge |
|---|---|---|---|
| 主对话(chat conv) | 订(按 `payload.conversationId == 当前 convId` filter)| 全订 + filter(显示 toast)| 订 + filter `scope.conversationId == 当前 convId`(锻造期看进度)|
| Function 详情页 | 不订 / 订(若想看历史 chat 流的 tool_call)| 订 + filter `type=function && id=fn_x` | 订 + filter `scope.kind=function && scope.id=fn_x` |
| Handler 详情页 | 不订 / 订 | 订 + filter `type=handler && id=hd_x` | 订 + filter `scope.kind=handler && scope.id=hd_x` |
| Workflow 编辑器 | 不订 | 订 + filter `type=workflow` | 订 + filter `scope.kind=workflow && scope.id=wf_x` |
| 列表页(Function / Handler / Workflow / FlowRun) | 不订 | 订 + filter 对应 type — 列表项实时更新 | 不订(列表不需要锻造期细节)|
| 仪表盘(全局视图)| 不订 | 全订 | 全订 |

**单用户场景**(per CLAUDE.md 项目特殊性):3 个 SSE 流是常驻连接,filter 在 client 端做,wire 流量小心智低。

---

## 6. Capability 删除引发的级联通知(D20)

删除 function / handler / workflow / mcp_server / skill 时,系统发对应通知,**引用此 capability 的其他 entity 自动标 `needs_attention`**(workflow 域 entity 上加这个状态字段)。

| 被删 | 发通知 type | data | 级联效果 |
|---|---|---|---|
| `delete_mcp_server` | `mcp_server` | `{action: "deleted"}` | 引用此 server 的 workflow → `needs_attention` |
| `delete_function` | `function` | `{action: "deleted"}` | 引用此 function 的 workflow → `needs_attention` |
| `delete_handler` | `handler` | `{action: "deleted"}` | 引用此 handler 的 workflow → `needs_attention` |
| `delete_skill` | `skill` | `{action: "deleted"}` | 引用此 skill 的 workflow → `needs_attention` |
| `delete_workflow` | `workflow` | `{action: "deleted"}` | 仅通知;别的 workflow 不能引用它(V1 不做 nested workflow) |

### 6.1 `needs_attention` workflow 行为

- **trigger 仍 register** 但触发时 fail-fast(`WORKFLOW_CAPABILITY_REMOVED` 422 / 502)
- **UI 列表项显示警告** + 建议用户 `edit_workflow` 替换或 `delete_workflow`
- **enabled** 字段不变(用户主动 disable 才停 trigger;系统不擅自停)

### 6.2 实施扇出

`workflow domain` 监听 5 类 entity 的 deletion notification → 扫所有 workflow graph → 找引用 → 标 `needs_attention` + 单独发 `workflow.action=needs_attention` 通知。

---

## 7. 实施成本(2026-05-12 后)

trinity 锻造期 backend 改动:

| 改动 | LOC 估 | Commit |
|---|---|---|
| `domain/eventlog` Bridge 改 user_id key | ~40 | Commit 3 |
| `infra/eventlog` 同 | ~80 | Commit 3 |
| `transport/httpapi/handlers/eventlog.go` 去 `?conversationId=` | ~20 | Commit 3 |
| `domain/notifications` Bridge 加 user_id key | ~40 | Commit 3 |
| `infra/notifications` 同 | ~80 | Commit 3 |
| `pkg/notifications` Publisher 从 ctx 抽 user_id | ~30 | Commit 3 |
| function/handler service 删 `env_synced` / `env_failed` publish | ~50 | Commit 3 |
| notification payload 瘦身(去 inline entity) | ~80 | Commit 3 |
| `domain/forge` 新建(events + Scope + Bridge) | ~150 | Commit 4 |
| `infra/forge` Bridge 新建 | ~120 | Commit 4 |
| `transport/httpapi/handlers/forge.go` GET endpoint | ~80 | Commit 4 |
| `pkg/forge` Publisher 给 LLM tool 用 | ~80 | Commit 4 |
| 4 LLM tool 双写(forge bus + chat progress block) | ~100 | Commit 4 |
| testend 三 bus listener 改造 | ~200 | Commit 5 |

backend 约 ~1000 行 + testend ~200 行。前端正式实施留 Wails 迁移期。详 [`discussions/2026-05-12-env-and-sse-rework.md`](./discussions/2026-05-12-env-and-sse-rework.md) §G 的 6-commit 切分。

---

## 8. 不在范围

- **跨 user 共享 SSE**(单用户场景,user_id 恒为 "local-user")
- **Notification 持久化**(V1 全 in-memory + SSE 推送;用户开 app 前的事件丢失,不补)— V1.5 看需要加 `notifications_log` 表
- **Notification 优先级 / 严重度**(V1 全 info-level)— V1.5 加 severity:info / warn / error
- **批量通知节流**(短时间内大量同 type)— V1.5 加 BurstCoalescer
- **Wails native events**(取代 SSE)— V1.5 / 打包阶段实施

---

(本文档完)

# Events Design — V1.2 SSE 事件契约(三协议)

**关联**:
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../event-log-protocol.md`](../event-log-protocol.md) — 完整 eventlog 协议设计文档(事件流示例 / 后端架构 / migration SQL)
- [`../adhoc-topic-documents/forge_redesign/discussions/2026-05-12-env-and-sse-rework.md`](../adhoc-topic-documents/forge_redesign/discussions/2026-05-12-env-and-sse-rework.md) §B-C — SSE 三流统一 + payload 瘦身 的事实源
- **配套实现**(eventlog):
  - `domain/eventlog/` — Event 接口 + 5 events + 7 block types + Bridge 接口 + ValidateEvent
  - `infra/eventlog/` — in-process Bridge(per-user 单调 seq + replay buffer + 慢订阅者阻塞)
  - `pkg/eventlog/` — Emitter (auto-mint ID + ctx-injected) + ctx helpers
- **配套实现**(notifications):
  - `domain/notifications/` — 1 通用 Event envelope + Bridge 接口 + ValidateEvent
  - `infra/notifications/` — per-user Bridge(per-user 单调 seq + replay buffer + Last-Event-ID 重连)
  - `pkg/notifications/` — Publisher(从 ctx 自动抽 user_id)
- **配套实现**(forge):
  - `domain/forge/` — 4 events(started / op_applied / env_attempt / completed)+ Scope struct + Bridge 接口 + ValidateEvent
  - `infra/forge/` — per-user Bridge(同模式)
  - `pkg/forge/` — Publisher(从 ctx 自动抽 user_id)
- **SSE 端点**:
  - `GET /api/v1/eventlog` — per-user 流式 chat 内容(eventlog 协议)
  - `GET /api/v1/notifications` — per-user entity 状态变更(notifications 协议)
  - `GET /api/v1/forge` — per-user trinity 锻造进度(forge 协议)
- **历史 refetch**:`GET /api/v1/conversations/{id}/eventlog?from=<seq>`(eventlog 协议 — 410 Gone 时的全态刷新)

**三协议(CLAUDE.md §E1)**:本契约覆盖后端**三条** SSE 流,**永远不再加**(D-redo-5):
1. **eventlog**(per-user;payload 带 `conversationId`,client demux)— recursive event log 协议(5 events × 7 block types),流式 chat 内容
2. **notifications**(per-user)— 1 通用 envelope,entity 状态变更;`data` 字段瘦身只送 ID + 必要小字段,完整 entity 走 GET 拉取
3. **forge**(per-user)— 4 events,trinity(function / handler / workflow)的 create/edit/revert/delete 流式锻造

三流共享 Bridge pattern(per-user seq + replay buffer + Last-Event-ID 重连),订阅一律按 user_id key,**没有** `?conversationId=` / `?entityId=` 之类 query 参 — 客户端按 payload 自己 demux / filter。§1-§10 是 eventlog 主体;§11 是 notifications 协议;§12 是 forge 协议;§13 是测试参考。

**遵守标准**:§E1(三协议;eventlog 5 events + 7 block types 封闭;forge 4 events 封闭;notifications 开放词表)/ §E2(eventlog parentId 路由;notifications + forge 按 payload 字段过滤)/ §N7(SSE wire format)/ §S21(事件流 invariants)

---

## 1. 事件总览

| Event Type | 用途 | 触发频率 | DB 写入 |
|---|---|---|---|
| `message_start` | 开新 message（user / assistant / subagent） | 每 message 1 次 | ✅ → `messages` 行（终态时 SaveMessage） |
| `message_stop` | 关 message（终态） | 每 message 1 次 | ✅ → 同上 |
| `block_start` | 开新 block | 每 block 1 次 | ✅ → `message_blocks` |
| `block_delta` | 给 block append 内容 | 每 token / chunk | ✅ → AppendBlockContent |
| `block_stop` | 关 block | 每 block 1 次 | ✅ → FinalizeBlock |

## 2. Block 类型枚举（7 种穷举）

| Block Type | 含义 | content 形态 | attrs | 子 block 允许？ |
|---|---|---|---|---|
| `text` | LLM 主文本（含 tool_call 间叙述） | string，append | — | ❌ |
| `reasoning` | LLM 思考（extended thinking） | string，append | — | ❌ |
| `tool_call` | LLM 发起的工具调用 | args JSON 流式拼 | `{tool: string}` | ✅（progress / nested / tool_result） |
| `tool_result` | 工具最终返回 | result string | — | ❌ |
| `progress` | 工具进度文字（sandbox 装包 / 网络拉块） | string，append | `{stage?: string}` 自由文本 | ❌ |
| `message` | 嵌套消息占位（subagent 等） | — | `{messageId: string, ...}` | ✅（递归到下层） |
| `compaction` | conversation-level 摘要（V1.2 §1 final-sweep） | markdown summary，append（一次性 delta）| `{coversFromSeq, coversToSeq, blocksArchived, generatedBy}` | ❌ |

新增 block 类型必须先改 [`event-log-protocol.md`](../event-log-protocol.md) + DB CHECK + 前端 renderer，同 PR。

`compaction` 块由 `app/contextmgr.Manager.fullCompact` 服务端 emit（不源于 LLM），挂在虚拟 system message 下（`{kind: "compaction"}`），承载 anchored-append summary 文本。DB block 行的 `context_role` 字段（V1.2 §1）按 `hot`/`warm`/`cold`/`archived` 控制投影到 LLM history 时的形态——见 [`../service-design-documents/compaction.md`](../service-design-documents/compaction.md) §6。

## 3. Status 枚举（4 种穷举）

`streaming` → 终态 (`completed` | `error` | `cancelled`)，单向不回退。

## 4. 完整事件 schema

```typescript
type Envelope = { seq: int64; event: Event }

type Event =
  | { type: "message_start"
      conversationId: string
      id: string                 // msg_<16hex>
      parentBlockId?: string     // 嵌套 message 才填（subagent 场景）
      role: "user" | "assistant" | "system"
      attrs?: object             // subagent: {kind:"subagent_run", type, runId, maxTurns}
    }
  | { type: "message_stop"
      conversationId: string
      id: string
      status: "completed" | "error" | "cancelled"
      stopReason?: string
      errorCode?: string
      errorMessage?: string
      inputTokens?: int
      outputTokens?: int
    }
  | { type: "block_start"
      conversationId: string
      id: string                 // blk_<16hex> (text/reasoning/result/progress/message)
                                 //  或 LLM 自带 tc_<id> (tool_call 复用)
      parentId: string           // 父 block ID 或 message ID（顶层 block 此处填 message ID）
      messageId: string          // 顶层归属 message ID（冗余但前端方便）
      blockType: BlockType
      attrs?: object
    }
  | { type: "block_delta"
      conversationId: string
      id: string
      delta: string              // append 字符串
    }
  | { type: "block_stop"
      conversationId: string
      id: string
      status: Status
      error?: string
    }
```

## 5. SSE wire format（§N7）

```
event: <type>
id: <seq>
data: <event JSON, 不重复 type/seq>

```

例：
```
event: block_delta
id: 42
data: {"conversationId":"cv_abc","id":"blk_xyz","delta":"hello"}

```

**重连**：`Last-Event-ID: <seq>` header → server replay buffer 内 seq > N 的事件，再接实时；超 buffer → 410 Gone + `code=SEQ_TOO_OLD` → 客户端 `GET /api/v1/conversations/{id}/eventlog?from=<seq>` refetch 全态。

**History refetch endpoint** —— `GET /api/v1/conversations/{id}/eventlog?from=<seq>` 返 JSON envelope（**不是 SSE**）：

```json
{
  "events": [ ...event JSONs from DB replay... ],
  "tailSeq": 1234,
  "count": 5
}
```

`tailSeq` 是关键——客户端拿它作下次 SSE 重订的 `Last-Event-ID` header，无缝衔接历史 + 实时。`count` 帮 UI 显示加载进度。

## 6. 路由与嵌套

- 客户端**按 user_id 订一条 SSE**(无 query 参,后端从 ctx 抽 user_id)
- 该 user 的**所有 conversation 的所有事件**(含主对话 + 嵌套 subagent / 嵌套 message)走同一个 SSE 流
- 客户端**按 `payload.conversationId` demux** — 主对话 panel / 历史 conv 子 panel / testend 多 conv 同时活跃皆按此字段分派
- 路由靠 `parentId` 字段递归 — 不靠事件名分层
- 前端维护两个 Map:`state.messages: Map<id, Message>` + `state.blocks: Map<id, Block>`,每 block 用 `parent` 字段挂树
- 同 user_id 内 wire `seq` 全局单调递增(跨所有 conversation 共享一个 seq 序列;Last-Event-ID 重连按此 seq);DB `message_blocks.seq` 与 wire seq 同源,`UNIQUE(conversation_id, seq)` 仍成立(seq 在 user 内单调即在 conversation 子集内也唯一)

## 7. 嵌套示例

```
Conversation (cv_xx)
└─ Message (msg_main, role=assistant)
   ├─ Block (blk_text_1, type=text)
   ├─ Block (tc_abc, type=tool_call, attrs.tool="spawn_subagent")
   │   ├─ Block (blk_msg_placeholder, type=message, attrs.messageId=msg_sub)
   │   │   └─ Message (msg_sub, role=assistant, parentBlockId=blk_msg_placeholder)
   │   │      ├─ Block (blk_text_2, type=text)
   │   │      ├─ Block (tc_xyz, type=tool_call, attrs.tool="Read")
   │   │      │   └─ Block (blk_result, type=tool_result)
   │   │      └─ Block (blk_text_3, type=text)
   │   └─ Block (blk_summary, type=tool_result)  ← spawn_subagent 返主 LLM 的 summary
   └─ Block (blk_text_4, type=text)
```

## 8. Producer 责任分配

| Producer | 推什么 |
|---|---|
| `app/chat/Service.Send` | user message 5 类事件 burst（user message_start → 每 block 的 BlockStart/Delta/Stop → message_stop） |
| `app/chat/runner.processTask` | assistant message_start（顶层） |
| `app/chat/chatHost.WriteFinalize` | assistant message_stop |
| `app/loop/streamLLM` | 流式期间每 LLM 事件推 text/reasoning/tool_call block_start/delta/stop（共享给主对话 + subagent） |
| `app/loop/runOneTool` | tool_result block_start/delta/stop（每 tool 结束后） + WithParentBlockID(tc.ID) 给 tool 内部 emit 自动挂父 |
| `app/subagent/Service.Spawn` | message-block 占位（type=message） + sub message_start ；loop.Run 返后 message-block stop |
| `app/subagent/subagentHost.WriteFinalize` | sub message_stop |
| Tool.Execute 内部（progress / 嵌套 LLM） | 经 ctx 拿 emitter 自由 emit progress block / 嵌套 text block |

## 9. DB 写入表

`message_blocks`（事件日志协议主表）：

| 列 | 类型 | 说明 |
|---|---|---|
| id | text PK | `blk_<16hex>` 或 LLM tc_<id> |
| conversation_id | text NOT NULL UNIQUE(conv_id, seq) idx 1 | per-conv 路由 + UNIQUE |
| message_id | text NOT NULL idx | 顶层归属 |
| parent_block_id | text idx | 嵌套用；顶层 block 此列空 |
| seq | int NOT NULL UNIQUE(conv_id, seq) idx 2 | per-conv 单调（Bridge 分配） |
| type | text NOT NULL CHECK in 7 值 | block 类型（含 compaction）|
| attrs | text | JSON |
| content | text NOT NULL DEFAULT '' | append-only 累积 |
| status | text NOT NULL CHECK in 4 值 | streaming → 终态 |
| error | text | block_stop 时填 |
| created_at / updated_at | datetime | GORM 自动 |

**Message 不双写** — `messages` 表走 chat repo（`infra/store/chat/SaveMessage`，含 user_id / role / status / token 字段），块内容走 eventlog Emitter 写 `message_blocks`。两表 schema 统一后协作而非竞争（详 [`../service-design-documents/chat.md`](../service-design-documents/chat.md) §3）。

## 10. Invariants（§S21）

- `block_start.parentId` 必须先于本事件出现过（dangling = producer bug）
- `block.status` / `message.status` 单向流转 streaming → 终态
- 同 conv `seq` 严格全局单调（DB UNIQUE 强制）
- 同 block 的 deltas 按 seq append-only，前端不重写不重排
- `tool_call` block ID = LLM 自带 tc_id（不走 §S15 prefix）；其他 block ID 走 idgen `blk_`

## 11. Notifications 协议(per-user SSE)

与 §1-§10 的 eventlog 协议独立 — 共享 Bridge pattern(per-user seq + replay buffer + Last-Event-ID 重连),envelope shape / 演化规则不同。

### 11.1 Envelope shape

```typescript
type Envelope = { seq: int64; event: Event }

type Event = {
  type: string                  // 实体种类判别字符串(开放词表)
  id: string                    // 实体 ID(type 内唯一)
  data: SlimPayload             // **瘦身**:只送 ID + 必要小字段,完整 entity 走 GET 拉取(D-redo-6)
  conversationId?: string       // 仅 conversation-scoped 实体填(如 todo)
}

type SlimPayload = {
  action: string                // 如 "created" / "updated" / "deleted" / "version_accepted" / ...
  // 加点小字段(per entity type 不同):
  versionId?: string            // function/handler/workflow 版本相关 action
  versionNumber?: int
  envStatus?: string            // 仅 trinity 锻造期终态通知带(可省)
  envError?: string
  // ...
}
```

**为何瘦身**(D-redo-6):
- 旧设计 data 字段塞完整 entity(几 KB - 几十 KB),违背"通知 = 轻量状态变更"
- 带宽 / 心智 / 一致性都更好 — UI 拿通知 → 主动 GET 详情(避免"通知里 entity 已 stale,GET 才是 source of truth"的双源问题)
- 多 panel 并存场景下,只有"关心此 entity"的 panel 触发 GET,流量按需

### 11.2 现有 entity types

| type | producer | 触发场景 | data 字段 |
|---|---|---|---|
| `conversation` | `app/conversation/Service` + `app/chat/runner.autoTitle` | 创建 / 改 title 或 systemPrompt / 软删 / autoTitle | `{action}`(标题等改动需 GET refetch)|
| `todo` | `app/todo/Service` | 任意 CRUD | `{action, status?}` |
| `mcp_server` | `app/mcp/Service` | server 增删改 / 重连 / 健康检查 | `{action, status?, error?}` |
| `skill` | `app/skill/Service.Scan` 轮询 | 添 / 改 / 删 SKILL.md | `{action}`(client 全部重读 skill 库)|
| `sandbox_env` | `app/sandbox/Service` | env 状态变 / env 软删 | `{action}` |
| `function` | `app/function/Service` 各 CRUD 端点 | created / updated / pending_created / version_accepted / pending_rejected / reverted / deleted | `{action, versionId?, versionNumber?}`(D-redo-7 删除 `env_synced`/`env_failed`;但 `env_rebuilt` 仍在 venv 重建后 emit — `function/crud.go`;env 详情仍经 GET 拉)|
| `handler` | `app/handler/Service` 各 CRUD 端点 | 同 function 7 个 + `config_updated` / `config_cleared` | `{action, versionId?, versionNumber?}`(同 D-redo-7 删除 env_synced/env_failed;但 `env_rebuilt` 仍 emit — `handler/crud.go`)|
| `workflow` | `app/workflow/Service` 各 CRUD 端点 | created / updated / pending_created / version_accepted / pending_rejected / reverted / deleted | `{action, versionId?, versionNumber?}`(slim payload D-redo-6;无 env action,workflow 域无 sandbox)|
| `document` | `app/document/Service` 各 CRUD 路径 | created / updated / deleted 等（开放词表）| `{action}`(slim;UI 经 `GET /api/v1/documents/{id}` 拉详情)|
| `flowrun` | `app/scheduler/Service` StartRun/finalize/pauseRun/ResumeApproval | started / paused / resumed / completed / failed / cancelled | `{action, workflowId, triggerKind?, nodeID?, decision?, elapsedMs?}`(slim;UI 经 `GET /flowruns/{id}` 拉详情 D-redo-6)|
| `memory` | `app/memory/Service` 各 mutation 路径 | created / updated / pinned / unpinned / deleted | `{action, name, memType, source}` (slim; UI 经 `GET /api/v1/memories/{name}` 拉全文 — V1.2 §2 final-sweep)|
| `compaction` | `app/contextmgr/Manager.fullCompact` | completed（暂仅一种 action — 失败仅 log，不推通知）| `{action: "completed", coversToSeq, blocksArchived, summaryBytes}` (slim; UI 经 `GET /api/v1/conversations/{id}` 拉 summary — V1.2 §1 final-sweep)|
| `ask` | `app/ask/Service.Wait` + `Resolve` | pending（AskUserQuestion 工具注册时推）/ resolved（用户答完或超时/取消后推）| `{toolCallId}` 无 action 时 = pending；`{toolCallId, action: "resolved"}` 已答完。`id` = `conversationId`。前端用此控制 sidebar 红点 |

新增 type 字符串即可(**开放词表** — E2 演化规则)。前端不需协议升级。**已删除**:`handler_instance` / `trigger` 等 forge_redesign 早期表上拟加但未实施的 type。**Plan 05 (2026-05-13)**:加 `flowrun` entity type;scheduler.StartRun 推 `started`,driveLoop 终态推 `completed`/`failed`/`cancelled`,pauseRun 推 `paused`,ResumeApproval 推 `resumed`。**V1.2 §1/§2 final-sweep (2026-05-16)**:加 `memory` + `compaction` entity types(slim payload + GET 拉详情)。

### 11.3 HTTP 端点

`GET /api/v1/notifications` — 双模式：有 `Accept: text/event-stream` header → per-user SSE 流（`Last-Event-ID` 重连）；否则 → REST 快照（`?cursor=<seq>&limit=<N>`，max 200，每条 `{seq,type,id,data,conversationId?}`）。后端从 ctx 抽 user_id，无 query 参决定 user。

**Wire format**:

```
event: notification          ← 硬码字面量"notification",不是动态实体类型
id: <seq>
data: {"type":"<entityType>","id":"<id>","data":{"action":"...",...},"conversationId":"<convId or empty>"}

```

**前端 dispatch**:所有通知 SSE event name **永远是 `"notification"`** — 前端 `es.addEventListener("notification", ...)` 单点订阅,然后按 `JSON.parse(e.data).type` 分派给不同 entity 处理器。**不要**写 `addEventListener("conversation", ...)` 之类按实体类型订阅 — SSE event name 不是动态的。这是设计决策:开放词表协议(每加新 entity type 不需前端扩展 SSE 路由表)。

**重连**:`Last-Event-ID: <seq>` header → server replay buffer 内 seq > N 的事件,再接实时;超 buffer → 410 Gone + `code=SEQ_TOO_OLD` → 客户端清缓存重订(无 fromSeq)+ 经 REST refetch 关心的实体。

### 11.4 Publisher API

`pkg/notifications.Publisher` — 从 ctx 自动抽 user_id:

```go
import notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"

// Service 构造期注入(cmd/server 主装配 + service 持作 struct 字段):
type Service struct {
    notif notificationspkg.Publisher
}

// 内部消费(传入 ctx,wrapper 从 ctx 抽 user_id):
s.notif.Publish(ctx, "function", fnID, slimPayload, convID)
// payload 是 SlimPayload(仅 {action, versionId?, ...} 这种轻字段),禁止塞完整 entity
// 第 5 参 conversationID:conversation-scoped 实体传对应 ID;非 scoped 传 ""
```

`New(bridge, log)` 是唯一构造器;bridge nil 时返 noop Publisher,service 构造器可安全 fallback `notificationspkg.New(nil, log)` 用于测试 / 未接线场景。**failure log 不上抛** — 通知是可观测性,不是业务。

### 11.5 与 eventlog / forge 协议的对比

| 维度 | eventlog | notifications | forge |
|---|---|---|---|
| 订阅域 | per-user(无 query)| per-user(无 query)| per-user(无 query)|
| envelope | 5 封闭事件 × 7 封闭 block type | 1 通用 envelope(type 自由) | 4 封闭事件 × 3 封闭 kind(function/handler/workflow)|
| 路由 / demux | `parentId` 递归 + client 按 payload.conversationId 分派 | client 按 type / conversationId 过滤 | client 按 scope.kind / scope.id 过滤 |
| 演化 | 加事件 / block type 必须改 [`../event-log-protocol.md`](../event-log-protocol.md) | 加 type 字符串即可,无协议升级 | 加 kind 必须改 [`../adhoc-topic-documents/forge_redesign/07-notifications-and-eventlog.md`](../adhoc-topic-documents/forge_redesign/07-notifications-and-eventlog.md);加 event type 同样封闭演化 |
| 用途 | 流式 chat 内容(含 subagent 嵌套)| entity 状态变更(CRUD action)| trinity entity 锻造流式(ops apply + env attempts)|
| Bridge | per-user seq + replay buffer | per-user seq + replay buffer | per-user seq + replay buffer |
| Producer | 紧密耦合(5 类型固定 schema)| 松散耦合(任意 type 字符串)| 紧密耦合(4 事件固定 schema)|
| payload 大小 | event-level(<1KB 每事件)| 瘦身(<200B 每通知)| event-level(<2KB 每事件)|

## 12. Forge 协议(per-user SSE)

trinity 锻造进度流(D-redo-4)。`create_function` / `edit_function` / `create_handler` / `edit_handler` / `revert_*` / `delete_*` 等 LLM tool 推流走 forge bus(+ 主对话 progress block 经 eventlog 双写,给 chat UI 实时看)。

### 12.1 Envelope shape

```typescript
type Envelope = { seq: int64; event: Event }

type Event = ForgeStarted | ForgeOpApplied | ForgeEnvAttempt | ForgeCompleted

type Scope = {
  kind: "function" | "handler" | "workflow"   // 封闭枚举(D-redo-23 嵌套用 Scope struct)
  id: string                                   // fn_/hd_/wf_<16hex>
}

type Operation = "create" | "edit" | "revert" | "delete"   // 封闭枚举

type ForgeStarted = {
  type: "forge_started"
  scope: Scope
  operation: Operation
  conversationId?: string       // chat-driven 锻造填;HTTP 直触发可空
  toolCallId?: string           // LLM 锻造时填(关联 chat 的 tool_call block)
}

type ForgeOpApplied = {
  type: "forge_op_applied"
  scope: Scope
  index: int                    // 第几个 op
  op: string                    // op 类型("set_code" / "set_dependencies" / ...)
}

type ForgeEnvAttempt = {
  type: "forge_env_attempt"
  scope: Scope
  attempt: int                  // 第几次(1..maxAttempts)
  status: "installing" | "fixing" | "ok" | "failed"
  stage?: string                // "resolving deps" / "downloading wheels" / ...(uv stderr 分段)
  detail?: string               // 当前 stage 一行 detail
  error?: string                // status=failed 时填
}

type ForgeCompleted = {
  type: "forge_completed"
  scope: Scope
  status: "ok" | "failed" | "cancelled"
  versionId?: string            // 成功时填
  envStatus?: string            // 成功时填(ready / failed)
  attemptsUsed?: int            // env-fix loop 累计 attempts
  error?: string
}
```

### 12.2 HTTP 端点

`GET /api/v1/forge` — per-user SSE 流(无 query 参,后端从 ctx 抽 user_id)。客户端按 `payload.scope` / `payload.conversationId` 字段 demux/filter。

**Wire format**:

```
event: <type>            ← forge_started / forge_op_applied / forge_env_attempt / forge_completed
id: <seq>
data: <event JSON, 不重复 type/seq>

```

例:
```
event: forge_env_attempt
id: 87
data: {"scope":{"kind":"function","id":"fn_abc"},"attempt":2,"status":"failed","error":"No matching distribution"}

```

**重连**:`Last-Event-ID: <seq>` → server replay → 超 buffer 返 410 Gone + `code=SEQ_TOO_OLD` → 客户端重订(无 fromSeq)。

### 12.3 双写

LLM tool 推流时,**每个 forge_env_attempt 同时**:
1. **forge bus** publish `forge_env_attempt`(给 entity 详情页订)
2. **eventlog bus** 在 chat tool_call block 下 emit `progress` block delta(给 chat panel 看)

主对话 LLM context 只看到最终 tool_result(env-fix loop 是 tool 内自治),中间 attempts 是 UI 实时观测层。详 [`../adhoc-topic-documents/forge_redesign/discussions/2026-05-12-env-and-sse-rework.md`](../adhoc-topic-documents/forge_redesign/discussions/2026-05-12-env-and-sse-rework.md) §E。

### 12.4 Publisher API

`pkg/forge.Publisher` — 从 ctx 自动抽 user_id(同 notifications 模式):

```go
import forgepkg "github.com/sunweilin/forgify/backend/internal/pkg/forge"

type Service struct {
    forge forgepkg.Publisher
}

// LLM tool 调用(create_function/edit_handler 等):
s.forge.PublishStarted(ctx, forgepkg.Scope{Kind: "function", ID: fnID}, "create", convID, toolCallID)
s.forge.PublishOpApplied(ctx, scope, idx, opType)
s.forge.PublishEnvAttempt(ctx, scope, attempt, "failed", "resolving deps", "...", err)
s.forge.PublishCompleted(ctx, scope, "ok", versionID, "ready", attemptsUsed, nil)
```

`New(bridge, log)` 是唯一构造器;bridge nil 时返 noop。

### 12.5 Invariants

- `scope.kind` 必须 ∈ {function, handler, workflow} — 其他值 Bridge publish panic
- `operation` 必须 ∈ {create, edit, revert, delete}
- 一个 entity 的一次锻造(`scope` 固定)按 wire 顺序:`forge_started` → N 个 `forge_op_applied` → 0..maxAttempts 个 `forge_env_attempt` → 1 个 `forge_completed`
- 同 user_id 内 wire `seq` 全局单调
- delete 操作只有 `forge_started` + `forge_completed`(无 ops / env attempts)

---

## 13. 测试覆盖

事件协议层单测:

- `infra/eventlog/bridge_test.go` — 单调 seq / 慢订阅阻塞 / Last-Event-ID 重连 / ErrSeqTooOld
- `infra/notifications/bridge_test.go` — 同模式,per-user key
- `infra/forge/bridge_test.go` — 同模式,per-user key + Scope 校验
- `pkg/eventlog/eventlog_test.go` — Emitter 父链 / DB dual-write(顶层/嵌套/append/finalize/error/attrs JSON)
- `pkg/notifications/publisher_test.go` — 从 ctx 抽 user_id / 瘦身 payload 校验
- `pkg/forge/publisher_test.go` — 4 events + Scope enum 校验
- `transport/httpapi/handlers/eventlog_test.go` — SSE 端到端 / Last-Event-ID / 410
- `transport/httpapi/handlers/notifications_test.go` — 同上
- `transport/httpapi/handlers/forge_test.go` — 同上

集成端到端测试(多 producer + 真 stream)走 `backend/test/` pipeline test(§T5)。

---

**覆盖矩阵自动生成**：3 流（eventlog / notifications / forge）每个事件在 `backend/test/README.md` 矩阵段列出覆盖测试；SSE truth 列表 hardcode 在 `backend/cmd/coverage-matrix/sse_truth.go`（封闭枚举）。新增 SSE event type 需改 `sse_truth.go` + `sse/<stream>_pipeline_test.go` 加测试 + `// covers: sse:<stream>:<event>` 注释（§S14 触发表已强制）。

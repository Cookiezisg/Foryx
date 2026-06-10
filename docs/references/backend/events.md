---
id: DOC-013
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Events Design — SSE 物理发射全量契约 (100% Coverage)

> **as-built（2026-06-10 改名对账）**：SSE 三流 2026-06-03 改名 `eventlog→messages`、`forge→entities`、`notifications` 不变（CLAUDE.md E1）。**订阅端点统一在 `StreamHandler`**：`GET /api/v1/{messages,entities,notifications}/stream`——**workspace 级、后端不过滤**（始终发完整 delta，前端常驻全连 + 按对话/实体自滤）、`Last-Event-ID`/`?fromSeq` 续传、`410 SEQ_TOO_OLD`。下表事件/载荷是**目标设计**：§1 block 生命周期生产侧 ✅ 已在 `app/loop` 落地；§3 forge **双写 entities 流**的生产侧待建（B/C 层，逐 tool）。物理源路径列含旧 backend 残留，随覆盖阶段 events.md 全量重写校准。

---

## 1. Messages 流 (`/api/v1/messages/stream`)

完整对话流，消息树实时渲染：assistant **文本** + **reasoning（thinking）** + **tool_call**（请求 / 中间过程 / result）逐 block 流式。由 `app/loop` 经 messages bus 实时推（`loop.WithBridge` 埋 ctx；tool 中间过程在 tool 内部经 ctx-bridge 自发——B 层逐 tool）。

| Event | 触发位置 | 载荷关键字段 (TS) |
|---|---|---|
| `message_start` | `chat/chat.go`, `subagent/spawn.go` | `{ id, conversationId, role, parentBlockId?, attrs }` |
| `block_start` | `chat/chat.go`, `loop/stream.go`, `loop/tools.go` | `{ id, conversationId, messageId, parentId, blockType, attrs }` |
| `block_delta` | `loop/stream.go` | `{ id, conversationId, delta }` |
| `block_stop` | `loop/stream.go`, `loop/tools.go` | `{ id, conversationId, status, error? }` |
| `message_stop` | `loop/stream.go` | `{ id, conversationId, status, inputTokens, outputTokens }` |

**BlockType 物理全集**：`text`, `reasoning`, `tool_call`, `tool_result`, `progress`, `message`, `compaction`。

**Todo 看板信号（M1.11）**：todo 写入时本流额外推一条 `signal` 帧承载任务看板快照——`scope={kind:"conversation", id:<convId>}` + `signal` + `node{type:"todo", content:{conversationId, subagentId?, todos:[{content,activeForm,status}]}}`。锚定对话（查看该对话的前端即收到）；subagent 清单的 `subagentId` 入 payload、前端据此嵌到对应子树。durable（重连 replay 最后看板态）。**写入是 LLM 专属**（`TodoWrite` 工具，波次 2/3），前端只读不写（REST 初值见 `api.md`）。

---

## 2. Notifications 流 (`/api/v1/notifications/stream` + 通知中心 REST)

**持久化通知中心**：每条通知是一个 `Notification` 实体（存 DB，见 `domains/notification.md`），由 notification 模块统一产生，关机重开仍在。任何 producer 经 `Emitter.Emit(type, payload)` 发；notification 模块存 DB + 在本流推一条 **durable signal**。

线缆形态：`scope={kind:"notification", id:"noti_x"}` + `signal` 帧 + `node{type, content}`。
- **事件类型 = `node.type` = `<域>.<动作>`**（下表 `Entity Type`.`Action`，如 `memory.updated`）；payload = `node.content`。
- **workspace 不在 scope**——它是 Bus 从 ctx 取的分流轴（前端按当前 workspace 订阅、防多窗口串台）。
- 通知中心 REST：`GET /notifications`（列表）、`/unread-count`（badge）、`PUT /{id}/read`、`POST /read-all`。
- `(Ephemeral)` 标记的（如 `flowrun.tick`）是实时进度，**不入通知中心 DB**、可丢。

| Entity Type | Action | 物理源文件（规划） | 载荷 Data (JSON) |
|---|---|---|---|
| `conversation` | `created` | `conversation.go` | `{ id }` |
| `conversation` | `deleted` | `conversation.go` | `{ id }` |
| `conversation` | `auto_title` | `conversation.go` | `{ id, title }` |
| `handler` | `config_updated` | `handler/config.go` | `{ id, status: "ok" }` |
| `function` | `version_accepted` | `function/crud.go` | `{ id, versionId }` |
| `workflow` | `version_accepted` | `workflow/crud.go` | `{ id, versionId }` |
| `flowrun` | `started` | `app/scheduler` | `{ flowrunId, workflowId, triggerId? }` |
| `flowrun` | `completed` | `app/scheduler` | `{ flowrunId, workflowId, status: "completed" }` |
| `flowrun` | `failed` | `app/scheduler` | `{ flowrunId, workflowId, status: "failed", error }` |
| `flowrun` | `tick` (Ephemeral) | `app/scheduler` | `{ flowrunId, nodeId, iteration, status }` |
| `sandbox` | `env_status_changed` | `app/sandbox` | `{ envId, status, ownerKind, ownerId, errorMsg? }` |
| `sandbox` | `env_deleted` | `app/sandbox` | `{ envId, ownerKind, ownerId }` |
| `mcp_server` | `connected` | `mcp/mcp.go` | `{ name, status: "ok" }` |
| `mcp_server` | `error` | `mcp/mcp.go` | `{ name, status: "error", lastError }` |
| `ask` | `pending` | `ask/ask.go` | `{ toolCallId, conversationId }` |
| `ask` | `resolved` | `ask/ask.go` | `{ toolCallId, status: "resolved" }` |
| `ask` | `timeout` | `ask/ask.go` | `{ toolCallId, status: "timeout" }` |
| `memory` | `created`/`updated`/`deleted` | `app/memory` | `{ name }` |
| `document` | `created`/`updated`/`moved`/`deleted` | `app/document` | `{ documentId, path, parentId? }` |
| `compaction` | `completed` | `contextmgr/compact.go` | `{ convID, coversToSeq }` |
| `skill` | `scanned` | `skill/scan.go` | `{ count }` |

---

## 3. Entities 流 (`/api/v1/entities/stream`)

锻造流水线进度（全实体流式总线）。**双写**：forge 工具把进度同时写 messages 流（tool_call 下的中间过程，§1）+ 本流（独立锻造流，给前端实体面板）。**生产侧待建**（Layer C，逐 tool）。

### 3.1 物理事件序列 (By Code Path)
| Event | 常用物理路径 | 载荷详情 |
|---|---|---|
| `forge_started` | `tool/handler/create.go` | `{ scope, operation, conversationId, toolCallId }` |
| `forge_op_applied` | `tool/workflow/edit.go` | `{ scope, index, op }` |
| `forge_env_attempt` | `tool/function/edit.go` | `{ scope, attempt, status: "installing"\|"ok"\|"failed", stage, detail }` |
| `forge_completed` | `tool/handler/edit.go` | `{ scope, status: "ok"\|"failed", versionId, envStatus, attemptsUsed, error }` |

### 3.2 物理覆盖范围
全量覆盖 `Quadrinity` (fn/hd/wf/ag) 以及 `document`, `skill` 实体。

---

## 4. 传输规范重申
1. **线缆分隔**：每条消息后紧跟 `\n\n`。
2. **Buffer 限制**：每 **workspace** 缓存最近 `durable` 事件（`seq > 0`，`stream.New(bufSize)`，当前 256）；续传游标越出环 → `410 SEQ_TOO_OLD`，客户端重取历史后重连。
3. **Tick 吞吐**：`ephemeral` (seq=0) 消息不进入 Buffer，高频发射（最高 100Hz），前端应使用 `requestAnimationFrame` 节流。

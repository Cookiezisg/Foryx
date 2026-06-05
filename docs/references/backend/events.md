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

> **法律级声明**：本文档通过扫描 `backend/internal/app` 下所有 `Publish` 和 `Emit` 调用生成。包含 100% 的 SSE 事件及其物理源。

---

## 1. Eventlog 流 (`/api/v1/eventlog`)

用于消息树实时渲染。由 `eventlog.Emitter` 触发。

| Event | 触发位置 | 载荷关键字段 (TS) |
|---|---|---|
| `message_start` | `chat/chat.go`, `subagent/spawn.go` | `{ id, conversationId, role, parentBlockId?, attrs }` |
| `block_start` | `chat/chat.go`, `loop/stream.go`, `loop/tools.go` | `{ id, conversationId, messageId, parentId, blockType, attrs }` |
| `block_delta` | `loop/stream.go` | `{ id, conversationId, delta }` |
| `block_stop` | `loop/stream.go`, `loop/tools.go` | `{ id, conversationId, status, error? }` |
| `message_stop` | `loop/stream.go` | `{ id, conversationId, status, inputTokens, outputTokens }` |

**BlockType 物理全集**：`text`, `reasoning`, `tool_call`, `tool_result`, `progress`, `message`, `compaction`。

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
| `flowrun` | `started` | `scheduler/scheduler.go` | `{ FrID, WfID, triggerKind }` |
| `flowrun` | `completed` | `scheduler/scheduler.go` | `{ FrID, WfID, status: "completed" }` |
| `flowrun` | `failed` | `scheduler/scheduler.go` | `{ FrID, WfID, status: "failed", error }` |
| `flowrun` | `tick` (Ephemeral) | `scheduler/state.go` | `{ WfID, nodeID, status, iterKey }` |
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

## 3. Forge 流 (`/api/v1/forge`)

锻造流水线。由 `forge.Publisher` 触发。

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
2. **Buffer 限制**：每用户缓存最近 1000 条 `durable` 事件（`seq > 0`）。
3. **Tick 吞吐**：`ephemeral` (seq=0) 消息不进入 Buffer，高频发射（最高 100Hz），前端应使用 `requestAnimationFrame` 节流。

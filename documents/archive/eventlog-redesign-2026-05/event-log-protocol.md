# Event Log Protocol — Forgify SSE 龙骨设计文档

> **状态**:✅ 已落地 + 2026-05-12 订阅模型修订
> **类型**:递归事件日志 SSE 协议(5 events × 7 block types,V1.2 §1 final-sweep 起 6→7)
> **当前现实**:本文 §0-§6 关于事件 schema / block 类型 / 嵌套 / DB 持久化的设计仍然成立(已实施)。**§3-§4 中 Bridge "per-conversation" 订阅模型已改为 per-user**(D-redo-2,2026-05-12 SSE 三流统一)。订阅端不再传 `?conversationId=`,后端 Bridge 按 user_id key,payload 仍带 `conversationId`,client 按 payload 字段 demux。详 [`adhoc-topic-documents/forge_redesign/discussions/2026-05-12-env-and-sse-rework.md`](./adhoc-topic-documents/forge_redesign/discussions/2026-05-12-env-and-sse-rework.md) §B。

**关联文档**:
- [`backend-design.md`](./backend-design.md) — 总体设计(本文档不替代)
- [`service-contract-documents/events-design.md`](./service-contract-documents/events-design.md) — wire 契约 + 三协议总览 + Producer 责任表
- [`service-contract-documents/database-design.md`](./service-contract-documents/database-design.md) — message_blocks schema
- [`service-design-documents/chat.md` / `subagent.md`](./service-design-documents/) — domain 详设计
- [`progress-record.md`](./progress-record.md) — dev log
- [`adhoc-topic-documents/forge_redesign/discussions/2026-05-12-env-and-sse-rework.md`](./adhoc-topic-documents/forge_redesign/discussions/2026-05-12-env-and-sse-rework.md) — 2026-05-12 三流统一修订

**本文档结构**：
1. §0 为什么做（背景 + 痛点）
2. §1 6 条设计原则（宪法）
3. §2 数据模型（Message / Block / 类型枚举）
4. §3 事件协议（5 种事件完整 schema + wire format）
5. §4 后端架构（Bridge / Emitter / Tool 框架 / LLM 客户端 / Sandbox / reqctx）
6. §5 前端架构（事件循环 + 6 种 block renderer）
7. §6 持久化（DB schema + migration SQL + 历史回放）
8. §7 规范修订（CLAUDE.md / 设计文档同步清单）
9. §8 Phase 路线图（5 phase × acceptance criteria）
10. §9 方法论（开发者决策框架）
11. §10 风险与缓解
12. §11 dogfood 验证清单
13. §12 决定一览

---

## §0 为什么做

### 现状痛点（审计结论）

1. **复杂**：6 个 entity-snapshot 事件类型 + 60fps 节流 + rAF batch + 嵌套快照借壳 + 事件名混乱（chat.message 双层 vs forge 单层）
2. **信息没推全**：tool call 间 LLM 文字可能被节流盖掉；MCP 装时 30s-5min 黑屏；subagent 内 LLM 流式 token 不传到主对话；forge run stdout 不实时；tool 内嵌 LLM 调用全部不可见
3. **subagent 借壳 chat.message**：跨 domain 共用一个事件 type，加新 domain 就得继续塞字段
4. **forge "用 entity 传 delta"**：每 token 推全 Forge entity（KB 级）传几十字节增量信息

### 现状根因

**只有一种事件语义（snapshot），没区分快照 vs 增量**。所有"复杂"都是这个根因的衍生：节流是为了救 snapshot 太重；rAF batch 是为了合并重复推；borrowed shell 是因为没有 delta 类型可表达"流式中间产物"。

### 行业现状

Anthropic Claude API + Vercel AI SDK 两个独立设计的协议**收敛到同一个抽象**：**recursive event log**。

```
Anthropic：message_start → content_block_start → content_block_delta* → content_block_stop → message_delta → message_stop
Vercel：   message-start → text-start → text-delta* → text-end → tool-input-start → tool-input-delta* → ... → finish
```

两个 SDK 共同特征：
- 一个统一事件流，不是多个事件类型
- block-based：message 含多个 content block
- 流式 delta：内容增量 append，不覆盖
- 工具结果 / 进度 / 文本 / 思考 都是 block，前端不区分逻辑

**这是 SSE-for-AI 行业事实标准**。

### 选这条路的本质收益

- **前端不再"拼东西"** —— 单一 30 行事件循环替代当前一坨拼装逻辑
- **删除全部当前症状**：60fps 节流、rAF batch、覆盖检测、借壳模式、6 种事件类型 → 全部消失
- **一个 primitive 解所有未来类似需求**：tool 流式进度 / 嵌套 LLM / 并行 subagent / 长任务推进 全免费
- **跟行业 SDK 同模式**：未来对接 Vercel AI SDK / Anthropic native 友好

### Scope: in / out

**In**：
- SSE 推流模型从 entity-snapshot → recursive event log
- DB schema 改造（message_blocks 加 5 列 + 删 subagent 两表）
- chat loop / subagent / 所有 sandbox 接 emit
- 前端 chat.js 重写 renderer
- §E / §S15 / §D / 加 §N7 / §S21 等规范修订

**Out**（明确不做）：
- forge 模块整体（用户即将单独重写）
- catalog polling 改事件驱动（独立轮，事件系统升级再说）
- skill wildcard 引擎（保留灵活性）
- subagent activeRuns Cancel registry（留作未来）
- tool emit helper 抽象（违 §S18，本轮坚持 in-line emit）
- LLM 客户端二元路径整合（独立轮）
- Permission 系统（用户决定保留现状）

---

## §1 设计原则（宪法）

落地时反复参照。任何 PR 违这 6 条退回重写。

### 原则 1：一个协议，递归覆盖一切
没有针对 subagent / 并行 / 嵌套的特殊事件。任何"嵌套"用 `parentId` 字段表达，没有第二种机制。Subagent 是 nested message，并行是同 parent 的多个 sibling block，全部同一套事件。

### 原则 2：Append-only，无覆盖
事件流单向流入。前端**永远 append，永远不重写**。没有"后到覆盖前到"的丢字符问题。Block 的 content 字段在 DB 里是累积值，不是覆盖。

### 原则 3：前端纯播放
前端只有一个事件循环。**不重建 entity、不计算 diff、不批合帧**。每事件 → 状态变更 → 渲染（React/Alpine 自然 diff）。

### 原则 4：后端纯发射
后端服务 emit 事件，**不管 UI 怎么渲染**。新功能 = 新 emit 调用，不需要协调任何 UI 逻辑。

### 原则 5：持久化与实时分离
DB 存最终 block 树（给历史用），SSE 推事件流（给实时 UI 用）。两者用同一个数据模型，**历史回放 = DB 转事件流再 emit 一遍**。前端 renderer 不区分实时 vs 回放。

### 原则 6：Block ID 即身份
事件用 block ID 路由。所有引用都通过 block ID。entity ID（messageID / forgeID / runID）只在 DB / 业务层有意义，**事件层只关心 block**。

---

## §2 数据模型

### 三层嵌套

```
Conversation
  └─ Message  (一次完整发言：user 输入 / assistant 回复 / subagent 内消息 都是 Message)
      └─ Block  (发言里一个内容单元)
          └─ Block  (递归嵌套，靠 parent_block_id 串)
              └─ ...
```

### Block 类型枚举（6 种，穷举）

| 类型 | 含义 | content 形态 | attrs 字段 | 子 block 允许？ |
|---|---|---|---|---|
| `text` | LLM 主文本（含 tool_call 间的 narration） | string，流式 append | — | ❌ |
| `reasoning` | LLM 思考（extended thinking） | string，流式 append | — | ❌ |
| `tool_call` | LLM 发起的工具调用 | args JSON 字符串，流式 append | `{tool: string}` | ✅（progress / nested / tool_result 都是子 block） |
| `tool_result` | 工具最终返回值 | result 字符串 | — | ❌ |
| `progress` | 工具执行期进度文字 | string，流式 append | `{stage?: string}` | ❌ |
| `message` | 嵌套消息（subagent / 任何"它在跑独立对话"） | — | `{messageId: string}` | ✅（递归到下层 Block；实际 message 数据在 messages 表） |

**就这 6 种**。新需求若不能套到这 6 种里，必须先讨论改协议——不能私加。

### Message 字段

| 字段 | 类型 | 说明 |
|---|---|---|
| ID | string (`msg_<16hex>`) | 全局唯一 |
| ConversationID | string (`cv_<16hex>`) | 所属对话 |
| ParentBlockID | string (`blk_<16hex>` 可空) | 触发该 message 的 block（subagent 场景）；顶层为空 |
| Role | string | `user` / `assistant` / `system` |
| Status | string | `streaming` / `completed` / `error` / `cancelled` |
| Attrs | JSON | 元数据（subagent 场景塞 `{kind:"subagent_run", type, tokens_in, tokens_out, max_turns}`） |
| StopReason | string | LLM 完成原因 |
| ErrorCode / ErrorMessage | string | terminal 错误时填 |
| InputTokens / OutputTokens | int | LLM 消耗 |
| CreatedAt / UpdatedAt | datetime | GORM 自动 |
| DeletedAt | gorm.DeletedAt | 软删（沿用 §D1） |

### Block 字段（重点：新 schema）

| 字段 | 类型 | 说明 |
|---|---|---|
| ID | string (`blk_<16hex>`) | 全局唯一 |
| **ConversationID** | string (`cv_<16hex>`) | **★ 新加** — 直接对话过滤，省 join messages |
| MessageID | string (`msg_<16hex>`) | 顶层归属 message |
| **ParentBlockID** | string (`blk_<16hex>` 可空) | **★ 新加** — 递归嵌套指针，可空表示顶层 block |
| **Seq** | int64 | **★ 改语义** — per-conversation 全局单调（事件序号） |
| Type | string | **7 种** block 类型枚举之一（含 CHECK 约束；V1.2 §1 final-sweep 加 `compaction`） |
| **Attrs** | JSON | **★ 改名（原 data）** — 仅 type 元数据 |
| **Content** | string | **★ 新加** — 累积流式内容 |
| **Status** | string | **★ 新加** — `streaming` / `completed` / `error` / `cancelled` |
| **Error** | string | **★ 新加** — block_stop 时填 |
| CreatedAt / UpdatedAt | datetime | GORM 自动 |

### 身份规则

- **ID 用 §S15 prefix 格式**：`msg_<16hex>` / `blk_<16hex>` / `cv_<16hex>`
- **Seq 是 conversation 级全局单调** — 不同 conversation 各自一套 seq
- **ParentBlockID 必须在同一 conversation 内**，且必须 block_start 已发生
- **顶层 message 的 ParentBlockID 为空**；嵌套 message（subagent）的 ParentBlockID 指向触发它的 tool_call block

---

## §3 事件协议

### 5 种事件完整 schema

```typescript
// Common envelope (所有事件必带)
type EventBase = {
  conversationId: string  // 路由 key
  seq: int64              // 全局单调，重连用
}

// 5 种事件
type Event =
  | (EventBase & {
      type: "message_start"
      id: string                  // msg_<16hex>
      parentBlockId?: string      // 触发该 message 的 block（subagent 场景）
      role: "user" | "assistant" | "system"
      attrs?: object              // subagent 场景：{kind:"subagent_run", type, ...}
    })
  | (EventBase & {
      type: "message_stop"
      id: string                  // msg_<16hex>
      status: "completed" | "error" | "cancelled"
      stopReason?: string
      errorCode?: string
      errorMessage?: string
      inputTokens?: int
      outputTokens?: int
    })
  | (EventBase & {
      type: "block_start"
      id: string                  // blk_<16hex>
      parentId: string            // 父 block ID（必填，因为 block 必属某个 message-or-block）
      messageId: string           // 顶层归属 message（冗余，前端方便）
      blockType: "text" | "reasoning" | "tool_call" | "tool_result" | "progress" | "message"
      attrs?: object              // tool_call: {tool: string}; progress: {stage?: string}; message: {messageId: string}
    })
  | (EventBase & {
      type: "block_delta"
      id: string                  // blk_<16hex>
      delta: string               // append 字符串（tool_call 也用，args JSON 流式拼）
    })
  | (EventBase & {
      type: "block_stop"
      id: string                  // blk_<16hex>
      status: "completed" | "error" | "cancelled"
      error?: string
    })
```

### Wire format（SSE）

每事件按 SSE 标准发送：

```
event: <type>
id: <seq>
data: <JSON of event without `type` field; type 在 event: 行了>

```

例：
```
event: block_delta
id: 42
data: {"conversationId":"cv_abc","seq":42,"id":"blk_xyz","delta":"hello world"}

```

**Last-Event-ID 重连**:客户端断线重连时携带 `Last-Event-ID: 42` header,后端从 `seq=43` 开始重发(buffer 30 秒内事件)。超出 buffer → 提示客户端 refetch full state。

> **订阅模型**(2026-05-12 修订,D-redo-2):客户端不传 `?conversationId=`,后端 Bridge 按 user_id key,wire seq 在 user 内全局单调。前端按 `payload.conversationId` 把不同对话的事件分派到对应 panel。

### 完整事件流示例（兼顾并行 + subagent + install）

用户问"开两个 subagent 分别研究 + 装个 ddg MCP"：

```jsonc
// === 用户消息 ===
{type:"message_start", conversationId:"cv_1", seq:1, id:"m1", role:"user"}
{type:"block_start", conversationId:"cv_1", seq:2, id:"b1", parentId:"m1", messageId:"m1", blockType:"text"}
{type:"block_delta", conversationId:"cv_1", seq:3, id:"b1", delta:"开两个 subagent..."}
{type:"block_stop", conversationId:"cv_1", seq:4, id:"b1", status:"completed"}
{type:"message_stop", conversationId:"cv_1", seq:5, id:"m1", status:"completed"}

// === 主 LLM 回复 ===
{type:"message_start", conversationId:"cv_1", seq:6, id:"m2", role:"assistant"}

// 思考
{type:"block_start", conversationId:"cv_1", seq:7, id:"b2", parentId:"m2", messageId:"m2", blockType:"reasoning"}
{type:"block_delta", conversationId:"cv_1", seq:8, id:"b2", delta:"用户想..."}
{type:"block_stop", conversationId:"cv_1", seq:9, id:"b2", status:"completed"}

// 同 turn 发起 3 个并行 tool call
{type:"block_start", conversationId:"cv_1", seq:10, id:"b3", parentId:"m2", messageId:"m2", blockType:"tool_call", attrs:{tool:"spawn_subagent"}}
{type:"block_start", conversationId:"cv_1", seq:11, id:"b4", parentId:"m2", messageId:"m2", blockType:"tool_call", attrs:{tool:"spawn_subagent"}}
{type:"block_start", conversationId:"cv_1", seq:12, id:"b5", parentId:"m2", messageId:"m2", blockType:"tool_call", attrs:{tool:"install_mcp_server"}}

// args 流式拼
{type:"block_delta", conversationId:"cv_1", seq:13, id:"b3", delta:"{\"role\":\"researcher\"}"}
{type:"block_delta", conversationId:"cv_1", seq:14, id:"b4", delta:"{\"role\":\"reviewer\"}"}
{type:"block_delta", conversationId:"cv_1", seq:15, id:"b5", delta:"{\"name\":\"io.github.x/ddg\",\"confirmed\":true}"}
{type:"block_stop", conversationId:"cv_1", seq:16, id:"b3", status:"completed"}
{type:"block_stop", conversationId:"cv_1", seq:17, id:"b4", status:"completed"}
{type:"block_stop", conversationId:"cv_1", seq:18, id:"b5", status:"completed"}

// === 三个 tool 并行执行，事件交叉 ===

// install_mcp_server 的 progress block（b5 的子）
{type:"block_start", conversationId:"cv_1", seq:19, id:"b6", parentId:"b5", messageId:"m2", blockType:"progress", attrs:{stage:"installing"}}
{type:"block_delta", conversationId:"cv_1", seq:20, id:"b6", delta:"Pulling docker image..."}

// subagent A 启动（b3 的子，是 message 类型 block）
{type:"block_start", conversationId:"cv_1", seq:21, id:"b7", parentId:"b3", messageId:"m2", blockType:"message", attrs:{messageId:"m_sub_a"}}
{type:"message_start", conversationId:"cv_1", seq:22, id:"m_sub_a", parentBlockId:"b7", role:"assistant", attrs:{kind:"subagent_run", type:"researcher", maxTurns:20}}
{type:"block_start", conversationId:"cv_1", seq:23, id:"b8", parentId:"m_sub_a", messageId:"m_sub_a", blockType:"text"}
{type:"block_delta", conversationId:"cv_1", seq:24, id:"b8", delta:"Let me research..."}

// subagent B 启动（同模式）
{type:"block_start", conversationId:"cv_1", seq:25, id:"b9", parentId:"b4", messageId:"m2", blockType:"message", attrs:{messageId:"m_sub_b"}}
{type:"message_start", conversationId:"cv_1", seq:26, id:"m_sub_b", parentBlockId:"b9", role:"assistant", attrs:{kind:"subagent_run", type:"reviewer"}}
{type:"block_start", conversationId:"cv_1", seq:27, id:"b10", parentId:"m_sub_b", messageId:"m_sub_b", blockType:"text"}
{type:"block_delta", conversationId:"cv_1", seq:28, id:"b10", delta:"I'll review..."}

// 进度 + subagent text 持续交叉到达
{type:"block_delta", conversationId:"cv_1", seq:29, id:"b6", delta:" 234MB / 1.2GB"}
{type:"block_delta", conversationId:"cv_1", seq:30, id:"b8", delta:" the codebase"}
{type:"block_delta", conversationId:"cv_1", seq:31, id:"b6", delta:" 600MB / 1.2GB"}
// ... 数百事件交叉 ...

// 各自结束
{type:"block_stop", conversationId:"cv_1", seq:200, id:"b6", status:"completed"}
{type:"block_start", conversationId:"cv_1", seq:201, id:"b11", parentId:"b5", messageId:"m2", blockType:"tool_result"}
{type:"block_delta", conversationId:"cv_1", seq:202, id:"b11", delta:"{\"installed\":true}"}
{type:"block_stop", conversationId:"cv_1", seq:203, id:"b11", status:"completed"}
{type:"block_stop", conversationId:"cv_1", seq:204, id:"b5", status:"completed"}

// subagent A 结束（自己的 message_stop + 父 message block stop + 父 tool_call stop）
{type:"message_stop", conversationId:"cv_1", seq:300, id:"m_sub_a", status:"completed", inputTokens:1234, outputTokens:567}
{type:"block_stop", conversationId:"cv_1", seq:301, id:"b7", status:"completed"}
{type:"block_start", conversationId:"cv_1", seq:302, id:"b12", parentId:"b3", messageId:"m2", blockType:"tool_result"}
{type:"block_delta", conversationId:"cv_1", seq:303, id:"b12", delta:"<subagent A summary>"}
{type:"block_stop", conversationId:"cv_1", seq:304, id:"b12", status:"completed"}
{type:"block_stop", conversationId:"cv_1", seq:305, id:"b3", status:"completed"}

// subagent B 同模式结束
// ...

// 主 LLM 收到 3 个 tool_result，继续输出最终回答
{type:"block_start", conversationId:"cv_1", seq:400, id:"b15", parentId:"m2", messageId:"m2", blockType:"text"}
{type:"block_delta", conversationId:"cv_1", seq:401, id:"b15", delta:"装好了，研究和review都完成..."}
{type:"block_stop", conversationId:"cv_1", seq:402, id:"b15", status:"completed"}

// 主 message 结束
{type:"message_stop", conversationId:"cv_1", seq:403, id:"m2", status:"completed", inputTokens:5000, outputTokens:2000}
```

**注意几个亮点**：
- 所有"嵌套"靠 parentId / parentBlockId，**无特殊 type** 区分 subagent 或并行
- 事件在线上交叉到达，前端按 ID 路由
- subagent 的 message 是嵌套 block 里 type=message 的**承载**——`b7` 是个"占位 block"，attrs.messageId 指向真正的 message id `m_sub_a`，message 数据走标准 message_start/stop

---

## §4 后端架构

### 三层模型

```
┌──────────────────────────────────────┐
│  Service / Tool 层                    │
│   - 业务逻辑                          │
│   - 调 emit() 推事件                  │
└──────────────┬───────────────────────┘
               │ ctx.emit
               ▼
┌──────────────────────────────────────┐
│  pkg/eventlog (Emitter)               │
│   - ctx-injected publisher            │
│   - 维护 messageId / parentBlockId    │
│   - 自动分配 seq + ID                 │
│   - 调 Bridge.Publish                 │
└──────────────┬───────────────────────┘
               │
               ▼
┌──────────────────────────────────────┐
│  infra/eventlog (EventLog Bridge)     │
│   - 替换 events.Bridge                │
│   - per-conversation seq 单调递增     │
│   - 短期 buffer 支持断线重连           │
│   - 派发 SSE handler                  │
└──────────────────────────────────────┘
```

### Emitter API（service / tool 层用这个）

```go
// pkg/eventlog/emitter.go

type Emitter interface {
    // Message lifecycle
    StartMessage(ctx context.Context, role string, parentBlockID string, attrs map[string]any) (msgID string)
    StopMessage(ctx context.Context, msgID string, status string, stopReason string, errCode, errMsg string, inputTokens, outputTokens int)

    // Block lifecycle
    StartBlock(ctx context.Context, parentID string, blockType string, attrs map[string]any) (blockID string)
    DeltaBlock(ctx context.Context, blockID string, delta string)
    StopBlock(ctx context.Context, blockID string, status string, err error)

    // Convenience: open a child emitter scoped under a parent block
    Child(parentBlockID string) Emitter
}

// ctx-injected
ctx = eventlog.With(ctx, publisher)
em := eventlog.From(ctx)

// 用例：tool 内推 progress
prog := em.StartBlock(ctx, currentToolCallBlockID, "progress", map[string]any{"stage": "installing"})
em.DeltaBlock(ctx, prog, "Pulling 234 MB / 1.2 GB\n")
em.DeltaBlock(ctx, prog, "Pulling 600 MB / 1.2 GB\n")
em.StopBlock(ctx, prog, "completed", nil)
```

### Bridge 接口契约(2026-05-12 后:per-user key)

```go
// infra/eventlog/bridge.go

type Bridge interface {
    // Publish 把事件按 user_id(从 ctx 抽)路由到订阅者;事件 payload 仍携带 conversationId,
    // client 自己按 payload.conversationId demux。
    // subscriber 慢时**阻塞 publisher**,不再 drop(delta 不能丢)。
    Publish(ctx context.Context, event Event) error

    // Subscribe 按 user_id 订阅;fromSeq 给 Last-Event-ID 重连用。返 channel + cancel。
    Subscribe(ctx context.Context, userID string, fromSeq int64) (<-chan Event, func())

    // Buffer 30s 内的事件供重连用;超出 buffer 客户端必须 refetch full state。
}
```

> 旧 `Publish(ctx, conversationID, event)` / `Subscribe(ctx, conversationID, fromSeq)` 已淘汰(D-redo-2)。Bridge 内部 key 从 `conversation_id` 改为 `user_id`,wire seq 在 user 内单调。**理由**:per-conversation 订阅在 testend 多 panel + 详情页同时活跃场景下会撞 HTTP/1.1 6-conn 限制;per-user 订一条解决,加 payload demux 心智低。

**§A2 关键变化** — buffer 语义反转：

```go
// 当前 events/memory/bridge.go: 默认 buffer=2048, drop on slow
//   → snapshot 模型下丢一帧无所谓（下个 snapshot 自带最新状态）
//
// 新 bridge: buffer=256 per-conversation, BLOCK on slow
//   → delta 模型下丢一个 delta = 前端缺字符
//   → subscriber 慢就让 publisher 阻塞（chat loop 是单消费者）
//   → 加 subscriber 死亡检测（Closed() 方法），死了让 publisher abort
```

**这是 day-1 必须做对**——上线后改这条语义会破整个 producer 端假设。

### Tool 框架自动注入 emitter

```go
// app/loop/tools.go::runOneTool

func runOneTool(ctx context.Context, tool Tool, call ToolCallData, ...) {
    em := eventlog.From(ctx)
    parentBlockID := eventlog.CurrentMessageID(ctx)  // 当前 LLM message

    // 自动开 tool_call block（args 流式拼，由 LLM stream 触发）
    blockID := em.StartBlock(ctx, parentBlockID, "tool_call", map[string]any{"tool": call.Name})
    em.DeltaBlock(ctx, blockID, call.ArgsJSON)
    em.StopBlock(ctx, blockID, "completed", nil)

    // 给 tool Execute 注入子 emitter，parent = blockID
    childCtx := reqctxpkg.WithParentBlockID(ctx, blockID)

    // Tool 执行期间所有 emit 都会自动 parentId=blockID
    result, err := tool.Execute(childCtx, call.ArgsJSON)

    // 自动开 tool_result block 作为 tool_call 的子 block
    resultBlockID := em.StartBlock(ctx, blockID, "tool_result", nil)
    em.DeltaBlock(ctx, resultBlockID, result)
    em.StopBlock(ctx, resultBlockID, statusFromErr(err), err)
}
```

**Tool 实现内部不用做任何事**——`eventlog.From(ctx)` 拿 emitter 直接 emit 就行。Parent ID 自动传递。

### LLM 客户端集成

```go
// infra/llm/client.go

func Generate(ctx context.Context, req Request) (*Response, error) {
    em := eventlog.From(ctx)
    parentID := eventlog.Parent(ctx)  // 通常是当前 message id 或 tool_call block id

    // Stream 期间 emit text/reasoning blocks
    var textBlockID, reasoningBlockID string
    for event := range stream {
        switch event.Type {
        case TextStart:
            textBlockID = em.StartBlock(ctx, parentID, "text", nil)
        case TextDelta:
            em.DeltaBlock(ctx, textBlockID, event.Delta)
        case TextStop:
            em.StopBlock(ctx, textBlockID, "completed", nil)
        case ReasoningStart:
            reasoningBlockID = em.StartBlock(ctx, parentID, "reasoning", nil)
        // ...
        }
    }
    return resp, nil
}
```

**所有调 LLM 的地方自动有流式 UI**——主对话、search rerank、catalog generator、subagent 内部 LLM 全部统一。

### Sandbox progress 集成

```go
// infra/sandbox/exec_helper.go

// RunWithStderrCapture 接收 emitter 替代 ProgressFunc
func RunWithStderrCapture(ctx context.Context, cmd *exec.Cmd, em Emitter, blockID string, sentinel error, msgPrefix string) error {
    scanner := bufio.NewScanner(stderrPipe)
    for scanner.Scan() {
        line := scanner.Text()
        em.DeltaBlock(ctx, blockID, line+"\n")  // 实时推每行
    }
    // ...
}

// 调用方（mcp install / 等）
func (s *Service) InstallFromRegistry(ctx context.Context, ...) {
    em := eventlog.From(ctx)
    parentID := eventlog.Parent(ctx)  // 当前 tool_call block id

    progressBlockID := em.StartBlock(ctx, parentID, "progress", map[string]any{"stage": "installing"})
    err := sandbox.EnsureEnv(ctx, ..., em, progressBlockID)
    em.StopBlock(ctx, progressBlockID, statusFromErr(err), err)
}
```

### reqctx 加 ParentBlockID（§A5）

```go
// pkg/reqctx/blocks.go (新)

type ctxKeyParentBlock struct{}

func WithParent(ctx context.Context, blockID string) context.Context {
    return context.WithValue(ctx, ctxKeyParentBlock{}, blockID)
}

func GetParentBlockID(ctx context.Context) (string, bool) {
    v, ok := ctx.Value(ctxKeyParentBlock{}).(string)
    return v, ok
}
```

Tool framework 在 Execute 前自动 push；递归 emit 自然继承父链。

### Subagent 改递归 emit（拆借壳）

旧（borrowed shell）：
```go
// subagent/host.go::publishChatMessage
// 把 SubagentRun snapshot 塞进 ChatMessage 事件的可选字段，前端按 SubagentRunID 路由
```

新（recursive emit）：
```go
// subagent.Spawn
em := eventlog.From(ctx)
parentToolCallBlockID := eventlog.Parent(ctx)  // 触发 subagent 的 tool_call

// 开 message block 占位
msgBlockID := em.StartBlock(ctx, parentToolCallBlockID, "message", map[string]any{})

// 开嵌套 message（caller 已在上游铸 subMsgID，emit 用 EmitMessageStart）
em.EmitMessageStart(ctx, subMsgID, "assistant", msgBlockID, map[string]any{
    "kind": "subagent_run",
    "type": typeName,
    "maxTurns": maxSteps,
})

// 把 subMsgID 写回 attrs
em.UpdateBlockAttrs(ctx, msgBlockID, map[string]any{"messageId": subMsgID})

// 跑 subagent 的 loop（内部用 childCtx 继承 emitter，parent = subMsgID）
childCtx := reqctxpkg.WithParentBlockID(ctx, subMsgID)
result := loop.Run(childCtx, ...)

em.StopMessage(ctx, subMsgID, statusFromResult(result), ...)
em.StopBlock(ctx, msgBlockID, "completed", nil)
```

**subagent 不再借 chat.message 壳**。

---

## §5 前端架构

### 状态：一棵 block 树

```js
// testend/js/event-log.js (新文件，替代 chat.js 的 message 处理)

const state = {
  messages: new Map(),  // msgId → { id, role, status, blocks: [], attrs }
  blocks: new Map(),    // blockId → { id, type, parent, content, status, children: [], attrs }
}

function getParent(parentId) {
  return state.blocks.get(parentId) ?? state.messages.get(parentId)
}
```

### 事件循环（30 行核心）

```js
function onEvent(event) {
  switch (event.type) {
    case 'message_start': {
      state.messages.set(event.id, {
        id: event.id,
        role: event.role,
        parentBlockId: event.parentBlockId,
        status: 'streaming',
        attrs: event.attrs ?? {},
        blocks: [],
      })
      // 如果是嵌套 message，挂到父 block 的 children
      if (event.parentBlockId) {
        getParent(event.parentBlockId)?.children?.push(event.id)
      }
      break
    }
    case 'message_stop': {
      const m = state.messages.get(event.id)
      if (m) {
        m.status = event.status
        m.stopReason = event.stopReason
        m.tokens = { input: event.inputTokens, output: event.outputTokens }
      }
      break
    }
    case 'block_start': {
      state.blocks.set(event.id, {
        id: event.id,
        type: event.blockType,
        parent: event.parentId,
        messageId: event.messageId,
        attrs: event.attrs ?? {},
        content: '',
        status: 'streaming',
        children: [],
      })
      const p = getParent(event.parentId)
      if (p) {
        (p.blocks ?? p.children).push(event.id)
      }
      break
    }
    case 'block_delta': {
      const b = state.blocks.get(event.id)
      if (b) b.content += event.delta
      break
    }
    case 'block_stop': {
      const b = state.blocks.get(event.id)
      if (b) {
        b.status = event.status
        b.error = event.error
      }
      break
    }
  }
  scheduleRender()  // Alpine 自动 diff
}
```

### 6 种 block renderer 草图

```jsx
function Block({ id }) {
  const b = state.blocks.get(id)
  if (!b) return null

  switch (b.type) {
    case 'text':       return <TextBlock content={b.content} status={b.status} />
    case 'reasoning':  return <CollapsibleReasoning content={b.content} />
    case 'tool_call':  return (
      <ToolCallBox
        tool={b.attrs.tool}
        args={b.content}
        status={b.status}
        children={b.children.map(cid => <Block id={cid} />)}
      />
    )
    case 'tool_result':return <ToolResultPanel content={b.content} status={b.status} error={b.error} />
    case 'progress':   return <ProgressLine stage={b.attrs.stage} text={b.content} />
    case 'message':    return <NestedMessage msgId={b.attrs.messageId} />
  }
}

function NestedMessage({ msgId }) {
  const m = state.messages.get(msgId)
  if (!m) return null
  return (
    <div className={`nested-message role-${m.role}`}>
      <div className="header">{m.attrs.type ?? m.role}</div>
      {m.blocks.map(bid => <Block id={bid} />)}
      {m.status === 'streaming' && <Spinner />}
    </div>
  )
}
```

**6 种 block × 各 30-50 行 renderer**。无特殊场景（subagent / parallel / 嵌套都自然渲染）。

### 删除前端旧逻辑

- 删 `_pendingMsgs` Map + rAF batch
- 删"同 messageId 后到覆盖前到"逻辑
- 删 reasoning / tool step 的折叠逻辑（替成 block-type-driven 渲染）
- 删 chat.message 事件特殊处理（统一走 block_* 事件）

---

## §6 持久化

### 新 message_blocks schema

```sql
CREATE TABLE message_blocks (
  id              TEXT PRIMARY KEY,                     -- blk_<16hex>
  conversation_id TEXT NOT NULL,                        -- ★ 新加
  message_id      TEXT NOT NULL,
  parent_block_id TEXT,                                 -- ★ 新加，可空（顶层）
  seq             INTEGER NOT NULL,                     -- ★ 改语义：per-conversation 全局
  type            TEXT NOT NULL CHECK(type IN ('text','reasoning','tool_call','tool_result','progress','message')),
  attrs           TEXT,                                 -- JSON（原 data 改名）
  content         TEXT NOT NULL DEFAULT '',             -- ★ 新加：累积流式内容
  status          TEXT NOT NULL CHECK(status IN ('streaming','completed','error','cancelled')),
  error           TEXT,                                 -- ★ 新加：block_stop 时填
  created_at      DATETIME,
  updated_at      DATETIME
);

CREATE UNIQUE INDEX idx_blocks_conv_seq ON message_blocks(conversation_id, seq);
CREATE INDEX idx_blocks_message ON message_blocks(message_id);
CREATE INDEX idx_blocks_parent ON message_blocks(parent_block_id);
```

走 GORM tag（普通索引）+ schema_extras（CHECK）混合（§D7）。

### 新 messages schema

```sql
ALTER TABLE messages ADD COLUMN parent_block_id TEXT;        -- ★ 新加（subagent 场景指向父 tool_call block）
ALTER TABLE messages ADD COLUMN attrs TEXT;                  -- ★ 新加 JSON（subagent 元数据等）
-- 其他字段沿用：id / conversation_id / role / status / stop_reason / err* / tokens / created_at / updated_at / deleted_at
```

### 删 subagent 两表

```sql
DROP TABLE subagent_runs;
DROP TABLE subagent_messages;
```

数据迁移到 messages + message_blocks（见 §Migration SQL）。

### Migration SQL（完整可执行）

SQLite 不支持复杂 ALTER，走 §D6 schema_extras 模式：

```sql
-- ===== STEP 1: 新 message_blocks 表 =====
CREATE TABLE message_blocks_v2 (
  id              TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL,
  message_id      TEXT NOT NULL,
  parent_block_id TEXT,
  seq             INTEGER NOT NULL,
  type            TEXT NOT NULL CHECK(type IN ('text','reasoning','tool_call','tool_result','progress','message')),
  attrs           TEXT,
  content         TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL CHECK(status IN ('streaming','completed','error','cancelled')),
  error           TEXT,
  created_at      DATETIME,
  updated_at      DATETIME
);

-- ===== STEP 2: 老 message_blocks 数据 backfill =====
INSERT INTO message_blocks_v2 (
  id, conversation_id, message_id, parent_block_id, seq,
  type, attrs, content, status, error, created_at, updated_at
)
SELECT
  mb.id,
  m.conversation_id,
  mb.message_id,
  NULL,                                                    -- 顶层（旧数据无嵌套）
  ROW_NUMBER() OVER (PARTITION BY m.conversation_id ORDER BY m.created_at, mb.seq),
  mb.type,
  json_object('legacy', 1),                                -- 标记历史
  COALESCE(
    json_extract(mb.data, '$.text'),
    json_extract(mb.data, '$.content'),
    json_extract(mb.data, '$.result'),
    ''
  ),
  'completed',                                              -- 历史一律视为完成态
  NULL,
  mb.created_at,
  mb.created_at
FROM message_blocks mb
JOIN messages m ON m.id = mb.message_id
WHERE mb.deleted_at IS NULL;

-- ===== STEP 3: subagent_runs → messages =====
ALTER TABLE messages ADD COLUMN parent_block_id TEXT;
ALTER TABLE messages ADD COLUMN attrs TEXT;

INSERT INTO messages (
  id, conversation_id, parent_block_id, role, status, stop_reason,
  attrs, input_tokens, output_tokens, created_at, updated_at
)
SELECT
  sr.id,                                                    -- subagent run id 复用为 message id
  sr.parent_conversation_id,
  sr.parent_tool_call_id,                                   -- 父 tool_call block 直接连
  'assistant',
  CASE sr.status
    WHEN 'completed' THEN 'completed'
    WHEN 'failed' THEN 'error'
    WHEN 'cancelled' THEN 'cancelled'
    ELSE 'completed'
  END,
  COALESCE(sr.stop_reason, ''),
  json_object(
    'kind', 'subagent_run',
    'type', sr.type,
    'maxTurns', sr.max_turns,
    'model', sr.model
  ),
  sr.total_tokens_in,
  sr.total_tokens_out,
  sr.created_at,
  sr.updated_at
FROM subagent_runs sr;

-- ===== STEP 4: subagent_messages → message_blocks_v2 =====
-- subagent_messages.blocks 是 JSON serialized [Block...]
-- 解开成 row-per-block，串 parent_block_id 到 subagent run 的根
INSERT INTO message_blocks_v2 (...)
SELECT
  json_extract(value, '$.id'),
  sr.parent_conversation_id,
  sm.subagent_run_id,                                       -- = subagent message id
  NULL,                                                      -- 老数据无嵌套
  ROW_NUMBER() OVER (
    PARTITION BY sr.parent_conversation_id
    ORDER BY sm.created_at, sm.seq, key
  ),
  json_extract(value, '$.type'),
  json_extract(value, '$.attrs'),
  COALESCE(json_extract(value, '$.text'), json_extract(value, '$.content'), ''),
  'completed',
  NULL,
  sm.created_at,
  sm.created_at
FROM subagent_messages sm
JOIN subagent_runs sr ON sr.id = sm.subagent_run_id,
     json_each(sm.blocks);

-- ===== STEP 5: 重命名 + 删旧表 =====
DROP TABLE message_blocks;
ALTER TABLE message_blocks_v2 RENAME TO message_blocks;
DROP TABLE subagent_messages;
DROP TABLE subagent_runs;

-- ===== STEP 6: 索引 =====
CREATE UNIQUE INDEX idx_blocks_conv_seq ON message_blocks(conversation_id, seq);
CREATE INDEX idx_blocks_message ON message_blocks(message_id);
CREATE INDEX idx_blocks_parent ON message_blocks(parent_block_id);
```

走 `infra/db/schema_extras.go` 注册 + `db.Migrator().HasTable("message_blocks_v2")` 守卫幂等。

### 历史回放

```go
// infra/store/blocks/replay.go

func ReplayConversation(ctx context.Context, db *gorm.DB, convID string, fromSeq int64) ([]Event, error) {
    // 1. Replay messages
    var messages []Message
    db.Where("conversation_id = ?", convID).Order("created_at").Find(&messages)

    // 2. Replay blocks (按 seq)
    var blocks []Block
    db.Where("conversation_id = ? AND seq > ?", convID, fromSeq).Order("seq").Find(&blocks)

    events := []Event{}

    // For each block, emit start + delta(完整 content) + stop
    for _, b := range blocks {
        events = append(events,
            BlockStart{ID: b.ID, ParentID: b.ParentBlockID, MessageID: b.MessageID, BlockType: b.Type, Attrs: b.Attrs, Seq: b.Seq},
            BlockDelta{ID: b.ID, Delta: b.Content, Seq: b.Seq},
            BlockStop{ID: b.ID, Status: b.Status, Error: b.Error, Seq: b.Seq},
        )
    }

    // Message lifecycle 插入对应位置（按 message.created_at + 第一个 block.seq）
    // ...

    return events, nil
}
```

**前端 renderer 不知道是实时还是回放**——同一个事件循环。

---

## §7 规范修订

### CLAUDE.md 必改

#### §E1 现状(2026-05-12 三协议)

CLAUDE.md §E1 已实施 + 2026-05-12 修订为**三协议**(eventlog + notifications + forge),全部按 user_id 订。详细文本见 CLAUDE.md 当前 §E1。本文档不再镜像 CLAUDE.md 文本(单源原则)。

#### §E2 重写

```markdown
**E2 事件路由 + 命名** —— 通过 parentId 字段路由（不再靠事件名分层）。事件类型固定 5 种，无 `chat.token` / `tool.code_updated` 等；block 类型固定 6 种。新增 block 子类型必须先改协议文档，再改 code。
```

#### §S15 ID 前缀更新

```markdown
**S15 ID 生成统一**（更新条目）：
- 废弃：`sar_`（subagent run）/ `smm_`（subagent message）—— 合并到 `msg_` / `blk_`
- 沿用：`msg_` / `blk_` 给所有 message + block（含 subagent 内的）
- 不新增：`evt_` 不需要（事件不持久化原始流，从 message_blocks 反推）
```

#### §S18 加 emit 规约

```markdown
**S18 Tool 接口规约**（新加段）：
- Tool.Execute 内可调 `eventlog.From(ctx).StartBlock/DeltaBlock/StopBlock` 推 progress / nested 调用
- parent_block_id 由 framework 通过 ctx 自动注入，tool 不必手算
- 推 progress 是惯例不强制；纯快任务（filesystem Read 等）可不推
```

#### §D3 加 block.type / status CHECK

```markdown
**D3 枚举 CHECK 约束**（新加条）：
- `message_blocks.type` 6 值 CHECK：text / reasoning / tool_call / tool_result / progress / message
- `message_blocks.status` 4 值 CHECK：streaming / completed / error / cancelled
- `messages.status` 沿用既有 4 值
```

#### 加 §N7 SSE wire format

```markdown
**N7 SSE wire format** —— 事件 wire 格式：
- `event: <type>` / `id: <seq>` / `data: <JSON>` 标准 SSE
- 客户端断线重连用 `Last-Event-ID: <seq>` header，后端 buffer 30s 内事件
- 超出 buffer → 客户端 refetch full state（HTTP `/conversations/{id}/events?from=<seq>`）
- 详见 [`event-log-protocol.md` §3](./event-log-protocol.md)
```

#### 加 §S21 事件流契约 invariant

```markdown
**S21 事件流契约 invariants**（必守）：
- block_start 的 parentId 必须先于本事件出现过（顶层 message 例外）
- block 的 status 流转单向：streaming → {completed | error | cancelled}
- message 的 status 同上
- 同 block 内 deltas 按 seq 严格有序，前端按 seq append
- 同一 conversation 内 seq 全局单调递增（DB UNIQUE 约束）
```

### 设计文档级改动清单

| 文档 | 改动 |
|---|---|
| `events-design.md` | **整篇重写** —— 替换 entity-state 模型为 recursive event log |
| `chat.md` | Block 结构 + Service.Save 契约改 |
| `subagent.md` | 删 SubagentRun / SubagentMessage 节，改成"subagent 是 nested message" |
| `forge.md` | Run 推流改走 progress block（标注待 forge 重写时落地）|
| `database-design.md` | message_blocks schema 大改 + 删 subagent 两表 |
| `api-design.md` | `/api/v1/eventlog` SSE 端点契约(per-user 订;`Last-Event-ID` 重连)|
| `error-codes.md` | 检查是否有借用 chat.message 的错误码需要重命名 |

### 受影响的 §S 系列规则审计

走完 4 轮屎山拯救计划后状态：
- §S5 长度参考线 —— 仍守
- §S11 双语注释 —— 仍守
- §S12 包结构平铺 —— 仍守，新加 `pkg/eventlog` / `infra/eventlog` 遵守
- §S13 别名规范 —— 新包 `eventlogpkg` / `eventloginfra` 遵守
- §S14 文档同步纪律 —— 本重构本身就是巨型 §S14 测试
- §S20 "no defer without reason" —— 仍守

---

## §8 Phase 路线图

### Phase 1：基础设施（~930 行 / 3 天）

**Acceptance criteria**：
- [ ] `pkg/eventlog` 包：Emitter 接口 + ctx 注入 helpers + 6 种 block type 常量
- [ ] `infra/eventlog/bridge.go`：替换 events.Bridge，per-conversation seq 单调递增，buffer block-on-slow（**§A2 关键**）
- [ ] `infra/store/blocks`：新 message_blocks repo（含 ConversationID / ParentBlockID / Seq / Status / Error / Content / Attrs 字段）
- [ ] `pkg/reqctx`：加 `WithParent` / `GetParentBlockID`（**§A5**）
- [ ] migration SQL：跑 schema_extras 完成新表 + 索引
- [ ] `transport/httpapi/handlers/eventstream.go`：SSE handler 改用新 Bridge，支持 Last-Event-ID 重连
- [ ] 全套单测：Emitter / Bridge buffer 满阻塞 / Last-Event-ID 重连
- [ ] go build + staticcheck + GOOS=windows/linux 干净

**完工产出**：基础设施可独立跑，但 chat / tool 还没接入（旧 SSE 流仍在跑）。

### Phase 2：生产者迁移（~600 行 / 5 天）

**Acceptance criteria**：
- [ ] chat loop emit events（替代 publishMessage entity）
- [ ] subagent 改递归 emit（删借壳 chat.message 路径）
- [ ] Tool framework `runOneTool` 自动注入 emitter + parent_block_id
- [ ] LLM 客户端（infra/llm）emit text/reasoning blocks
- [ ] subagent.go / host.go 拆文件（§B4，反正要重写）
- [ ] subagent_runs / subagent_messages 表删（migration 跑 backfill）
- [ ] 旧 publishChatMessage / publishForge / publishMCP 等保留兼容期（dual-write）
- [ ] 单测：chat 一轮对话事件流正确性 / subagent 嵌套事件流 / 并行 tool 事件交叉

**完工产出**：后端两套并行跑（旧 entity-snapshot + 新 event log），前端尚未切换。

### Phase 3：生态接入（~300 行 / 2 天）

**Acceptance criteria**：
- [ ] sandbox progress 接 emit（替代 ProgressFunc）
- [ ] mcp install / web / shell / search / 等 8 个沉默工具补 progress block emit
- [ ] errAssistImports 反模式删（§B1）
- [ ] V2 placeholder TODO 三连清（§B3）
- [ ] 历史回放：DB → events 转换器（`ReplayConversation`）
- [ ] 集成测试：完整 install_mcp_server flow 端到端事件流验证

**完工产出**：所有 producer 全用新 emit；旧 publishX entity 兼容层仍在。

### Phase 4：前端 + 清理（~580 行 / 4 天）

**Acceptance criteria**：
- [ ] `testend/js/event-log.js` 新文件：30 行事件循环 + state.messages / state.blocks 维护
- [ ] 6 种 block renderer 实现（text / reasoning / tool_call / tool_result / progress / message）
- [ ] 删 `chat.js` 旧 message / rAF batch / 折叠规则等所有路径
- [ ] subagent 4 个 GET HTTP endpoints 删（§A4），testend subagent tab 改读 block tree
- [ ] events/types.go 删 8 个旧 entity-snapshot event types（§B5）
- [ ] 后端兼容层删（旧 publishChatMessage 等）
- [ ] 删 60fps 节流 + rAF batch
- [ ] dogfood：起 chat 跑 install_mcp_server / spawn_subagent / 并行 tool 验证 UI 正确

**完工产出**：flag-day 切换完成，旧路径全删，新路径单一事实源。

### Phase 5：文档 + 验证（~280 行 / 1-2 天）

**Acceptance criteria**：
- [ ] CLAUDE.md §E1/§E2/§S15/§S18/§D3 改 + 加 §N7/§S21
- [ ] `documents/version-1.2/service-contract-documents/events-design.md` 整篇重写
- [ ] `documents/version-1.2/service-design-documents/chat.md` / `subagent.md` 改章节
- [ ] `documents/version-1.2/service-contract-documents/database-design.md` 改 + `api-design.md` SSE 端点重写
- [ ] `progress-record.md` 加重构 dev log
- [ ] contract test 全套：事件 invariant / 重连 / 历史回放
- [ ] 跨平台 GOOS=windows/linux 干净
- [ ] 整套 dogfood checklist 跑过（见 §11）

### 总规模 + 时间表

| Phase | 估行 | 天 | 累计 |
|---|---|---|---|
| 1 基础设施 | 930 | 3 | 3 |
| 2 生产者迁移 | 600 | 5 | 8 |
| 3 生态接入 | 300 | 2 | 10 |
| 4 前端 + 清理 | 580 | 4 | 14 |
| 5 文档 + 验证 | 280 | 2 | 16 |
| **总** | **~2700** | **~16 天** | **2-2.5 周** |

---

## §9 方法论：决策框架

实施期遇到具体决策时**翻这一节**。

### "我要推一个东西，应该用什么 block 类型？"

```
显示给用户的吗？
├─ 否 → 不推，用 zap.Log 即可
└─ 是 → 是结构化数据还是文本？
        ├─ 结构化（如 tool_call args / tool_result）→ tool_call / tool_result
        └─ 文本流
            ├─ LLM 思考 → reasoning
            ├─ LLM 主答 / 中间叙述 → text
            ├─ 工具进度 / 状态 → progress
            └─ 嵌套对话（subagent 等）→ message
```

### "我要嵌套别的东西，parent 是谁？"

ctx-injected emitter 自动维护，开发者不必手算。但要理解：

```
当前在哪个 block 上下文？
├─ 主对话 LLM 流期 → parent = 当前 message id
├─ 在 tool Execute 内 → parent = 当前 tool_call block id
└─ 在 subagent run 内 → parent = subagent 自己的 message id（递归）
```

### "什么时候开新 block，什么时候 append delta？"

```
是同一种内容的延续吗？
├─ 是（如 LLM 继续吐 text token）→ delta 到当前 block
└─ 否（如切到 tool_call / 切到不同 reasoning 段）→ 开新 block
```

LLM SDK 通常给"事件"（TextEvent / ToolEvent / ReasoningEvent / 等）——**事件类型变化 = 切 block**。

### "并行的子流要不要特殊处理？"

不要。同 parent 的多个 sibling block 自然并行。前端 renderer 按 children 列表渲染——并排或堆叠依 UI 设计。

### "我的 service 调 LLM，要不要自己 emit？"

不要。LLM 客户端（`infra/llm`）原生 emit text / reasoning blocks——caller 不重复。tool args / tool result 由 `runOneTool` 框架自动 emit。**service 只需 emit progress block**（如 sandbox stderr）。

### "DB 写盘什么时候发生？"

emit 即写 DB。Emitter 内部 dual-write：
1. Bridge.Publish (实时 SSE)
2. blocks.Save (持久化)

两者用同一事务保证一致性。

---

## §10 风险与缓解

| 风险 | 缓解 |
|---|---|
| 事件序号实现 bug 导致 UI 错位 | 严格的单调递增 + 客户端校验 + dev mode 报错；Bridge 单元测试覆盖序号生成 |
| Bridge buffer 阻塞导致 LLM 流变慢 | SSE handler flush 频率每 16ms，订阅端 channel 容量 256（够 16ms 内的事件量） |
| Bridge buffer 溢出 | 超时让客户端 refetch full state；不无限内存 |
| 历史回放与实时模型不一致 | 同一个 renderer + 同一组事件类型；contract test 双向验证 |
| 前端旧逻辑残留导致渲染冲突 | flag-day 切换，不双跑（但内部 staging 用 feature flag 跑一周） |
| forge / catalog 等已有 entity-snapshot 消费者 | Phase 2 期间保留兼容层（dual-write 旧 publishForge 等）；Phase 4 一次性清理 |
| migration 数据丢失 | migration 跑前自动备份 `~/.forgify/forgify.db.bak.<timestamp>`；migration 失败回滚到备份 |
| dogfood 期间发现协议设计漏洞 | 每个 phase 完工 dogfood，发现就 §S20 当场修不留下次 |

---

## §11 dogfood 验证清单

flag-day 切换前必须手动跑通：

### 主对话基础
- [ ] 用户发短消息 → assistant 流式回复 → message_stop 完整
- [ ] reasoning 内容流式可见，可折叠
- [ ] 长回复（多 paragraph）正常 append 不丢字

### Tool calls
- [ ] 单 tool call（Read 文件）→ tool_call block + tool_result block 嵌套展示
- [ ] 多 tool call 串行 → 顺序展示
- [ ] 多 tool call 并行（同 turn 多个）→ 并排展示，事件交叉到达不混乱
- [ ] tool call 间 LLM 文字流式可见（之前痛点）

### Progress
- [ ] mcp install 流式进度可见（Pulling 234MB / 1.2GB...）
- [ ] sandbox 进度（uv install 等）流式可见
- [ ] 长任务 cancel 后 status=cancelled 正确

### Subagent
- [ ] spawn_subagent 内部 LLM 流式 token 可见（之前痛点）
- [ ] subagent 内嵌套 tool call 可见
- [ ] 主对话 + subagent 同时活跃，UI 不冲突
- [ ] 两个 subagent 并行启动，各自子窗独立流

### LLM-inside-tool
- [ ] search_mcp_marketplace 的 LLM rerank 流式可见
- [ ] forge.search 的 LLM rerank 流式可见
- [ ] catalog generator 的 LLM 调用流式可见

### 重连 / 历史
- [ ] 关闭浏览器再开，对话历史正确渲染
- [ ] SSE 断线（kill 后端 reload）→ 重连恢复，不丢字符
- [ ] 历史回放：打开 7 天前的对话 → block tree 正确渲染

### 数据库
- [ ] migration 跑过，message_blocks 表新 schema 生效
- [ ] subagent_runs / subagent_messages 表已删
- [ ] 旧对话 block 用 attrs.legacy=1 标记，能正常显示

---

## §12 决定一览

### 已拍

| 决定 | 选择 |
|---|---|
| 路线选型 | recursive event log（参考 Anthropic + Vercel SDK） |
| 嵌套机制 | parentId 字段递归（无特殊事件类型） |
| 流式语义 | append-only delta，无覆盖 |
| 兼容策略 | flag-day 切换（无外部 SSE 消费者） |
| message 嵌套（block.type=message） | 要 |
| tool_call args 流式 delta | 要 |
| 断点重连 | 要做（buffer 30s + Last-Event-ID） |
| progress block 的 attrs.stage | 自由文本，不规范化 |
| 持久化模型 | message_blocks 是事实源，不另加 events 表 |
| 历史回放 | DB → events 转换器，前端用同 renderer |
| Bridge buffer 语义 | 反转：drop → block on slow（§A2） |
| reqctx 加 ParentBlockID | 是（§A5） |
| subagent 4 个 HTTP endpoints | 删（§A4） |
| events/types.go 8 个旧事件 | 删（§B5） |
| errAssistImports 反模式 | 删（§B1） |
| V2 placeholder 三连 | 清（§B3） |
| subagent.go / host.go 拆文件 | 拆（§B4） |
| Permission 系统 | **保留现状**（B2 跳过，用户决定） |
| forge 模块 | 不动（用户即将单独重写） |
| catalog polling | 不动（独立轮再说） |
| skill wildcard | 不动（保留灵活性） |
| subagent Cancel registry | 不动（留作未来） |
| Tool emit helper 抽象 | 不抽（违 §S18 心智统一原则） |

### 待拍（实施期遇到）

- migration 跑哪一刻：每次 backend 启动自动跑 vs 单独 admin 命令
- staging 跑 feature flag 一周还是直接 prod 切
- 旧 publishX 兼容层在 Phase 2 完结时还是 Phase 4 完结时删

---

## §结语

这是 Forgify 项目的龙骨级重构。完成后整个 SSE 模型与行业 SDK 同质，痛点系统性消失。本文档是实施期的"宪法" —— **一切代码改动以本文档为准**。文档与代码不一致 = bug，按 §S14 立刻修文档。

**下一步**：用户拍版后，按 Phase 1 acceptance criteria 开 task 清单，开干。

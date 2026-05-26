# entities/conversation — 前端 slice 详细设计

**所属层**：entities（对位后端 domain/conversation + domain/chat + domain/eventlog）
**状态**：✅ 已实现（FSD Revamp 阶段 0–4 完成）
**职责**：管理对话线程元数据（CRUD）+ 消息/块树的 SSE 实时构建（chatStore）。两个独立职责通过同一 slice 暴露，是 entities 中最复杂的 slice。

**关联文档**：
- [`../frontend-design.md`](../frontend-design.md) — FSD 总规范
- [`../frontend-contract-documents/`](../frontend-contract-documents/) — API 契约索引
- 后端 [`../service-design-documents/conversation.md`](../service-design-documents/conversation.md)

---

## 1. 职责边界

| 子职责 | 说明 |
|---|---|
| 对话元数据 CRUD | 列表 / 创建 / PATCH 更新（rename / archive / pin / modelOverride）/ 软删 |
| 消息历史 REST 加载 | GET /conversations/{id}/messages → chatStore.hydrateConv |
| 消息发送 / 取消 | POST messages / DELETE stream |
| SSE 事件处理 | eventlog 5 类事件 → chatStore 树增量更新 |
| chatStore 公开 API | selector 稳定引用 + rAF 合并 → React 组件按 block id memo |

---

## 2. 类型（`model/types.ts`）

### 对话元数据

```ts
interface AttachedDocument { documentId: string; includeSubtree: boolean }
interface ModelRef { provider: string; modelId: string }

interface Conversation {
  id: string;           // cv_<16hex>
  title: string;
  autoTitled: boolean;
  systemPrompt?: string;
  summary?: string;
  summaryCoversUpToSeq?: number;
  attachedDocuments?: AttachedDocument[];
  archived: boolean;
  pinned: boolean;
  modelOverride?: ModelRef | null;
  createdAt: string;
  updatedAt: string;
}

interface CreateConversationBody { title?: string }
interface UpdateConversationPatch {
  title?: string; systemPrompt?: string;
  attachedDocuments?: AttachedDocument[];
  archived?: boolean; pinned?: boolean;
  modelOverride?: ModelRef | null;
}
```

### 消息 / 块

```ts
type BlockType = "text" | "reasoning" | "tool_call" | "tool_result" | "progress" | "message" | "compaction";
type BlockStatus = "streaming" | "completed" | "error" | "cancelled";
type MessageRole = "user" | "assistant";
type MessageStatus = "pending" | "streaming" | "completed" | "error" | "cancelled";

interface Block { id; messageId; parentId; type: BlockType; attrs; content; status: BlockStatus; durationMs; error; children: string[]; version }
interface Message { id; conversationId; role; status; parentBlockId; stopReason?; errorCode?; inputTokens?; outputTokens?; modelId?; provider?; attrs?; blocks: Block[]; attachments; createdAt }
interface SendMessageBody { content: string; attachmentIds?: string[] }
```

字段与后端 domain/chat + domain/eventlog 的 json tag 一一对应。REST 的 `Block.children` 是嵌套对象；chatStore 运行时把 children 拆成 ID 数组以支持稳定引用。

---

## 3. API hooks（`api/conversation.ts`）

| Hook | 方法 + 端点 | 说明 |
|---|---|---|
| `useConversations()` | GET `/conversations?limit=100` | 侧边栏列表；select pickList |
| `useConversation(id)` | GET `/conversations/{id}` | 详情（conv header 用）|
| `useConversationMessages(convId)` | GET `/conversations/{convId}/messages?limit=200` | 历史消息；chatStore.hydrateConv 消费 |
| `useCreateConversation()` | POST `/conversations` | 创建后 invalidate conversations |
| `useUpdateConversation(id)` | PATCH `/conversations/{id}` | rename / archive / pin / modelOverride；invalidate conversations + conversation(id) |
| `useDeleteConversation()` | DELETE `/conversations/{id}` | invalidate conversations |
| `useSendMessage(convId)` | POST `/conversations/{convId}/messages` | 不 invalidate；SSE 驱动 UI |
| `useCancelStream(convId)` | DELETE `/conversations/{convId}/stream` | meta.suppressGlobal=true，错误由 feature 层处理 |

Query key 工厂：`qk.conversations()` / `qk.conversation(id)` / `qk.messages(convId)`（定义在 `shared/api/queryKeys.ts`）。

---

## 4. chatStore（`model/chatStore.ts`）

### 状态形状

```ts
interface ChatConvState {
  messages: Map<string, ChatMessage>;   // 全部消息（含嵌套 subagent）
  blocks:   Map<string, ChatBlock>;     // 全部块；children 字段是 ID[]
  topMsgIds: string[];                  // 顶级消息 id 按到达顺序
  lastSeq: number;
}
interface ChatState {
  convs: Record<string, ChatConvState>;
  hydratedConvs: Set<string>;           // 已 REST 撒种子的 conv id
  // ... actions
}
```

`ChatMessage` / `ChatBlock` 是 store-internal 形状，不等于 REST `Message` / `Block`——blocks 字段一律 ID 数组，而非嵌套对象。

### hydrateConv — REST 撒种子

- 入参：REST `Message[]`（每条消息内嵌 `blocks?: RestBlock[]`，blocks 含 `parentBlockId`）
- 每个 conv **只执行一次**；`hydratedConvs` 集合防止 SSE 积累后被 cache refetch 覆盖
- 恢复路径（SSE 410 SEQ_TOO_OLD）：`resetConv(id)` 清除标记 → 下次 hydrateConv 重新撒种子
- 树重建算法：
  1. 对每条 message 调 `installMessage(m, parentBlockId)`
  2. 所有 blocks 先 `blocks.set(b.id, ...)` 建索引，再第二遍 wire children（保证父块先于子块查找时存在）
  3. 若 block 的 parentId 指向一个已知块 → `parent.children.push(b.id)`；否则 → `message.blocks.push(b.id)`
  4. 若 block.type === "message" 且含 `innerMessage` → 递归 `installMessage(b.innerMessage, b.id)`（subagent 嵌套）

### SSE 事件处理器

| 事件 | 处理器 | 关键逻辑 |
|---|---|---|
| `message.start` | `onMessageStart` | 插入新 ChatMessage；若有 parentBlockId 则写入对应块的 `attrs.messageId`（subagent 锚点）；否则追加 topMsgIds |
| `message.stop` | `onMessageStop` | 先 `flushNow()`（冲洗 delta buffer）；再设 status / stopReason / token counts |
| `block.start` | `onBlockStart` | 插入新 ChatBlock；attach 到父消息 blocks[] 或父块 children[] |
| `block.delta` | `onBlockDelta` | 只追加 `pendingDeltas`，调 `scheduleFlush()`（rAF 批合并） |
| `block.stop` | `onBlockStop` | 先 `flushNow()`；再设 status / error / durationMs |

### rAF delta 合并

- 问题：后端可达 30–100 delta/s；每条直接 setState → 满帧重渲染 → 主线程卡死
- 方案：`onBlockDelta` 把 `{convId, blockId, delta}` 推入 `pendingDeltas[]`，调 `scheduleFlush()`
  - `scheduleFlush` 用 `requestAnimationFrame`（SSR fallback：`setTimeout 16ms`）注册单次 flush
  - `flushDeltas`：把同 conv+block 的 delta 拼合后 **一次** `setState`，所有脏 block `version++`
  - `block.stop` / `message.stop` 调 `flushNow()`（cancel rAF + 立即执行）保证终态先于 stop 事件
- 效果：render 频率上限约 60 fps，长代码块不再锁 tab

### selector 稳定引用

```ts
const EMPTY_IDS: string[] = Object.freeze([]) as unknown as string[];

export function selectTopMessageIds(convId, state): string[]
export function selectBlock(convId, blockId, state): ChatBlock | null
export function selectChildIds(convId, parentId, state): string[]
```

- 返 ID 数组（不返对象）：store 只在真正变更时才分配新数组引用；zustand `useSyncExternalStore` 依赖引用等价判断，返对象会触发无限循环
- 消费方按 id 调 `selectBlock` 拿具体块，配合 `React.memo` 实现 per-block 精细重渲染

---

## 5. 端到端数据流

### 5.1 发送消息（SSE 驱动）

```
用户输入 → features/send-message → useSendMessage(convId).mutate(body)
  → POST /conversations/{convId}/messages  (202 Accepted)
  → 后端启动 chat 流，向 SSE eventlog 推事件
  → app 层 SSEProvider 监听 GET /eventlog
      → message.start  → chatStore.onMessageStart(convId, e)
      → block.start    → chatStore.onBlockStart(convId, e)
      → block.delta    → chatStore.onBlockDelta(convId, e)  (rAF 批量)
      → block.stop     → chatStore.onBlockStop(convId, e)   (flushNow 先)
      → message.stop   → chatStore.onMessageStop(convId, e) (flushNow 先)
  → React 组件通过 selectTopMessageIds / selectBlock 响应更新
```

### 5.2 切换对话（REST hydrate）

```
用户点击侧边栏 conv → pages/chat → useConversationMessages(convId)
  → TanStack Query: GET /conversations/{convId}/messages?limit=200
  → useEffect: 若 query.data 就绪且 convId 有效
      → chatStore.hydrateConv(convId, messages)
          → 若已 hydrated：no-op（保护 SSE 积累态）
          → 否则：installMessage 递归重建树 → setState
```

### 5.3 auto-title SSE 更新

```
后端 auto-titling goroutine → push notifications SSE "conversation" event
  → app/SSEProvider → qc.invalidateQueries(qk.conversations())
  → useConversations() 重取 → 侧边栏标题刷新
```

---

## 6. 实现清单

| 文件 | 说明 |
|---|---|
| `frontend/src/entities/conversation/model/types.ts` | Conversation / Message / Block / 请求体类型定义 |
| `frontend/src/entities/conversation/model/chatStore.ts` | useChatStore + hydrateConv + SSE handlers + rAF batcher + selectors |
| `frontend/src/entities/conversation/api/conversation.ts` | 8 个 TanStack Query / Mutation hooks |
| `frontend/src/entities/conversation/index.ts` | public API（类型 + hooks + store exports） |

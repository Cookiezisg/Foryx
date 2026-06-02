---
id: DOC-104
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Conversation & Chat — 核心消息引擎深度审计全书

> **核心地位**：这是 Forgify 的心脏。它不仅管理对话线程（Conversation），更通过一个极其复杂的 **ReAct 递归循环**，实现了“一切皆工具”的交互哲学。

---

## 1. 物理存储架构 (Data Persistence)

### 1.1 `Conversation` — 线程主表
```go
type Conversation struct {
    ID                   string    `gorm:"primaryKey;type:text" json:"id"` // cv_<16hex>
    UserID               string    `gorm:"not null;index" json:"-"`
    Title                string    `gorm:"not null;type:text;default:''" json:"title"`
    AutoTitled           bool      `gorm:"not null;default:false" json:"autoTitled"`
    SystemPrompt         string    `gorm:"type:text;default:''" json:"systemPrompt,omitempty"`
    Summary              string    `gorm:"type:text;default:''" json:"summary,omitempty"`
    SummaryCoversUpToSeq int64     `gorm:"not null;default:0" json:"summaryCoversUpToSeq,omitempty"`
    AttachedDocuments    []AttachedDocument `gorm:"serializer:json" json:"attachedDocuments,omitempty"`
    Archived             bool      `gorm:"not null;default:false;index" json:"archived"`
    Pinned               bool      `gorm:"not null;default:false" json:"pinned"`
    ModelOverride        *ModelRef `gorm:"serializer:json" json:"modelOverride,omitempty"`
}
```
- **排序逻辑**：`List` 接口采用 `pinned DESC, created_at DESC` 复合排序，确保置顶对话永远浮顶。

### 1.2 `Message` — 逻辑回合 (Turn)
一个 Message 代表 LLM 或用户的一次发言。
- **Token 记账**：`InputTokens`, `OutputTokens` 字段固化了每次生成的消耗，直接支撑 `/api/v1/usage` 统计。
- **溯源**：`Provider`, `ModelID` 字段记录了产出该消息的具体模型，解决了多模型混合对话下的成本核算难题。

### 1.3 `Block` — 物理内容树 (The Content Journal)
这是 Forgify V1.2 最重大的架构升级：内容不再是纯文本，而是 **`message_blocks`** 中的结构化记录。
- **Seq (序列号)**：对话内全局单调递增，是 SSE 重连（`from=seq`）的唯一索引。
- **ContextRole**：控制该 Block 如何参与 LLM 上下文。
  - `hot`: 活跃，完整送入。
  - `warm`: 被压缩后的摘要。
  - `archived`: 已过期，不参与生成。
- **物理校验**：数据库通过 `CHECK (type IN ('text','reasoning','tool_call',...))` 确保内容类型的严格闭合。

---

## 2. ReAct 循环原理 (The Engine)

### 2.1 任务队列 (The Queue)
- **并发控制**：每个 Conversation 拥有一个独立的 `convQueue`（容量 5）。同一时间只允许一个 AI 协程为该对话工作。
- **空闲回收**：Queue 协程在 5 分钟无任务后自动销毁 (`time.NewTimer`)。

### 2.2 循环算法 (`loop.Run`)
ReAct 引擎采用 `for step := range maxSteps` 结构：
1. **采样 (Sampling)**：调用 LLM 获取生成的 Block 流。
2. **熔断 (Circuit Breaker)**：
   - **TOOL_ERROR_STORM**：若连续 3 回合的所有工具调用全部失败，立即熔断，防止 Token 空转。
   - **MAX_STEPS_REACHED**：达到步数上限（默认 25-30）时停止，返回“继续”建议。
3. **工具派发 (Dispatching)**：
   - **Auto-Activation**：若 LLM 调用了一个尚未加载的工具组（Lazy Group），系统自动调用 `TryActivateForTool` 加载对应的 AgentState 并执行。
4. **状态写回**：每步完成后，通过 `RecordStep` 将历史增量（Assistant Blocks + Tool Results）持久化。

---

## 3. SSE 发射协议 (Live Wire)

SSE 不仅仅是推流，它在物理上驱动了前端的状态机。

### 3.1 严格发射时序
一个完整的 AI 生成回合遵循以下原子序列：
1. `message_start` (msg_xxx)
2. `block_start` (blk_text_1) -> `block_delta` (N 次) -> `block_stop`
3. `block_start` (tc_call_1) -> `block_stop` (带 tool name)
4. (后端执行工具...)
5. `block_start` (tr_result_1, parent=tc_call_1) -> `block_stop`
6. `message_stop` (带 Token 统计)

### 3.2 鲁棒性设计
- **Detached Context**：`StopMessage` 和 `FinalizeStop` 必须在剥离原有 Cancel 链的 Context 下执行（`context.Background()`）。
- **目的**：即便用户在 AI 生成的最后一毫秒关闭浏览器，后端也必须确保该 Block 的“终态”被标记为 `completed` 或 `cancelled`，禁止在数据库中留下永久的 `streaming` 孤儿块。

---

## 4. 上下文拼装 (System Prompt Build)

系统提示词采用 **Section 容器化** 架构，各部分通过 `<section name="...">` 标签隔离。

### 4.1 优先级顺序 (Cache-Friendly)
1. `identity`: 谁是 Forgify。
2. `how_to_work`: 核心指令（reuse first, verify before claiming）。
3. `tools`: 当前可用工具索引。
4. `memory`: 长期记忆（Memory 域注入）。
5. `documents`: 挂载文档（Notion 树展开后的 XML）。
6. **`architecture_rules`**: 架构决策（如：分类任务用 Agent 节点）。
7. **`critical_rules`**: 殿后指令（DeepSeek 等模型对末尾指令遵守度最高）。

### 4.2 语言注入 (Locale)
`InjectLocale` 中间件读 User 语言，将 `lang` 变量（"Chinese"/"English"）注入 `environment` 段，强制要求 LLM 遵循对应的回复语言。

---

## 5. 跨域联动详情 (Interactions)

- **Mention (@引用)**：`RegisterMentionResolver` 允许 `document`, `function`, `handler`, `workflow` 各域注册解析器。在消息发送时，系统会自动抓取这些实体的“快照”并存储在 Message 的 `attrs` 字段中。
- **Compaction (压缩)**：回合结束后，同步触发 `compactor.MaybeCompact`。如果检测到 Token 溢出，会自动生成摘要并切掉旧的 Blocks。
- **Auto-Title**：对于第一个回合，系统会异步启动一个 **Utility 档** 模型，根据 Assistant 的回答总结一个 5-10 字的标题。

---

## 6. 错误矩阵 (Failures)

| Wire Code | 物理起因 | 处理逻辑 |
|---|---|---|
| `STREAM_IN_PROGRESS` | `select q.ch <- task` default 分支命中 | 告知用户 AI 还在忙。 |
| `TOOL_ERROR_STORM` | `consecAllFail >= 3` | 彻底停止 Loop。 |
| `MAX_STEPS_REACHED` | `step == maxSteps` | 引导用户点击 UI 的“继续”。 |
| `LLM_STREAM_ERROR` | `streamLLM` 捕获到网络异常 | 标记 Message 为 `error`，停止 SSE。 |
| `EMPTY_CONTENT` | 用户输入为空且无附件 | `handlers` 层拦截。 |
| `UNAUTH_NO_USER` | `RequireUser` 找不到 UserID | 触发前端 self-heal。 |

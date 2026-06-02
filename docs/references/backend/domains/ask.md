---
id: DOC-102
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Ask Domain — 人工干预会合服务原理

> **核心职责**：Ask 解决了 Agent 交互中的 **“环境缺失”** 问题。它允许 LLM 在遇到模棱两可的指令、缺失的配置或高风险操作时，主动发起一个阻塞式请求，等待人类用户的实时输入。

---

## 1. 物理模型 (Data Anatomy)

Ask 属于 **“纯内存会合”** 领域，不设物理数据库表（除非未来需要支持跨重启恢复）。

### 1.1 `AskSession` (内存状态)
```go
type AskSession struct {
    ToolCallID     string        // 关联的 LLM 工具调用 ID
    ConversationID string        
    Signal         chan string   // 用于唤醒阻塞协程的通道
    CreatedAt      time.Time
}
```

---

## 2. 核心原理 (Principles)

### 2.1 Tool-Level Blocking (工具级阻塞)
Ask 不是一个简单的通知，它在 Go 协程层面实现了 **“物理暂停”**：
1. **发起**：LLM 调 `AskUserQuestion(question)`。
2. **挂起**：后端工具执行函数进入 `Service.Wait()`，创建一个带 `ToolCallID` 的信道并 `select` 阻塞。
3. **通知**：同步发布 `ask:pending` 类型的 **Notifications SSE** 指示前端显示输入框。
4. **唤醒**：前端调 `POST /api/v1/conversations/{id}/answers`，后端查找到信道，写入答案。
5. **继续**：工具函数收到信道信号，将答案作为 `tool_result` 返回给 LLM。

### 2.2 First-Wins 竞争保护
为了防止重复提交或旧请求干扰：
- **原子解约**：一旦信道被写入（或超时），对应的 Session 会立即从全局 Map 中移除。
- **关联锁定**：提交答案时必须携带正确的 `ToolCallID`，确保“问对人，答对事”。

### 2.3 自动清理 (GC)
- **硬超时**：默认阻塞上限为 **5 分钟**。
- **超时回退**：若时间到用户未答，信道自动返回 `(skipped by timeout)`，让 LLM 知道用户不在电脑前。

---

## 3. 生命周期 (Lifecycle)

1. **提问 (Asking)**：LLM 调工具 -> `Service.Wait` 开启。
2. **等待 (Awaiting)**：前端通过 `Notifications` 监听到请求，弹出 Modal 或 Input。
3. **回答 (Answering)**：用户输入内容 -> `POST /answers`。
4. **合流 (Resolving)**：后端 `Resolve` 唤醒阻塞协程。
5. **反馈 (Returning)**：LLM 收到答案，继续其 ReAct 循环。

---

## 4. 跨域集成 (Interactions)

- **Chat**：作为 `AskUserQuestion` 的底层宿主。
- **Notifications**：负责将提问信号准时送达前端。
- **Workflow**：在 Workflow 中暂不支持此同步 Ask（Workflow 采用异步的 `Approval` 域）。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 备注 |
|---|---|---|---|
| `ErrNoPendingQuestion`| 404 | `ASK_NOT_PENDING` | 尝试回答一个不存在或已结束的问题。 |
| `ErrTimeout` | 504 | `ASK_TIMEOUT` | 用户 5 分钟未理，工具自行返回。 |
| `ErrMissingToolCallID`| 400 | `INVALID_REQUEST` | 提交参数缺失核心标识。 |
| `ErrConvMismatch` | 403 | `UNAUTHORIZED` | 尝试在 A 对话回答 B 对话的问题。 |

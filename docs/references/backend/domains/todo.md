---
id: DOC-124
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Todo Domain — 对话级任务追踪与管理

> **核心地位**：Todo 是 Agent 的 **“交互式工作清单”**。它允许 LLM 在执行长跨度、多步骤的任务时，显式记录进度、管理依赖，并向用户展示“我正在做什么”。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `Todo` 实体
```go
type Todo struct {
    ID             string         `gorm:"primaryKey;type:text" json:"id"` // td_<16hex>
    ConversationID string         `gorm:"not null;index:idx_td_conv_status,priority:1" json:"conversationId"`
    
    Subject        string         `gorm:"not null;type:text" json:"subject"` // "运行测试"
    Description    string         `gorm:"type:text" json:"description"`
    
    // UI 动效支持: 当前状态的进行时描述
    ActiveForm     string         `gorm:"type:text" json:"activeForm"` // "正在运行测试..."
    
    Status         string         `gorm:"not null;default:'pending'" json:"status"` // pending|in_progress|completed|deleted
    
    // 协作与依赖
    Owner          string         `gorm:"type:text" json:"owner"`     // 负责该 Todo 的 Agent 名
    BlockedBy      []string       `gorm:"serializer:json" json:"blockedBy"` // 依赖的 td_ID 列表
    
    CreatedAt      time.Time      `json:"createdAt"`
    UpdatedAt      time.Time      `json:"updatedAt"`
    DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}
```

---

## 2. 核心原理 (Principles)

### 2.1 Conversation-Scoped (对话作用域隔离)
Todo 与 Conversation 物理绑定：
- **隔离性**：Agent A 在对话 1 中创建的 Todo，Agent B 在对话 2 中不可见。
- **防止泄漏**：Repository 强制校验 `conversation_id`，禁止通过 ID 跨对话修改任务。

### 2.2 LLM-facing Interaction (工具化操作)
Todo 不通过前端 REST API 进行 CRUD，而是完全由 LLM 驱动：
- **Create**：LLM 规划步骤。
- **Update**：LLM 完成一个步骤后，手动更新状态。
- **Observe**：系统每轮会自动将“当前未完成任务清单”注入上下文，辅助 LLM 维持一致性。

### 2.3 Status-Driven UI (实时同步)
后端在 `Service.Update` 成功后，会自动发布 `todo` 类型的 **Notifications SSE**：
- **Payload**：完整的 Todo 实体快照。
- **效果**：前端对话侧栏的“任务看板”会实时根据 SSE 事件跳动，无需用户刷新。

---

## 3. 生命周期 (Lifecycle)

1. **规划 (Planning)**：LLM 收到复杂指令，调 `create_todo` 拆解步骤。
2. **执行 (Action)**：LLM 开始执行某一步，调 `update_todo` 设置 `status="in_progress"`。
3. **完成 (Closing)**：步骤结束，调 `update_todo` 设置 `status="completed"`。
4. **清理 (Discarding)**：若步骤作废，设置 `status="deleted"`（触发软删）。

---

## 4. 跨域集成 (Interactions)

- **Chat**：作为系统工具集（Task 家族）注入。
- **Eventlog**：Todo 的状态变化会触发 Notifications 流。
- **Subagent**：当派生子智能体时，子任务可持有父对话的 Todo 句柄（视配置而定）。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 备注 |
|---|---|---|---|
| `ErrNotFound` | 404 | `TODO_NOT_FOUND` | 物理不存在或跨对话越权。 |
| `ErrSubjectRequired`| 400 | `TODO_SUBJECT_MISSING` | 标题不能为空。 |
| `ErrInvalidStatus` | 400 | `TODO_INVALID_STATUS` | 状态不符合枚举。 |
| `ErrMissingConvID` | 500 | `INTERNAL_ERROR` | 上下文中丢失会话 ID（接线 Bug）。 |

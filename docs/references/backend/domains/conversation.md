---
id: DOC-106
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Conversation Domain — 线程管理与属性治理

> **核心职责**：Conversation 域负责 Forgify 对话线程的 **“全生命周期管理”**。它不关心具体的消息内容（那是 Chat 域的事），而是作为容器，管理标题、摘要、归档状态以及该对话专属的模型覆盖设置。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `Conversation` 实体
```go
type Conversation struct {
    ID                   string    `gorm:"primaryKey;type:text" json:"id"` // cv_<16hex>
    UserID               string    `gorm:"not null;index" json:"-"`
    
    // 语义属性
    Title                string    `gorm:"not null;type:text;default:''" json:"title"`
    AutoTitled           bool      `gorm:"not null;default:false" json:"autoTitled"`
    
    // 背景活与压缩属性
    SystemPrompt         string    `gorm:"type:text;default:''" json:"systemPrompt,omitempty"`
    Summary              string    `gorm:"type:text;default:''" json:"summary,omitempty"`
    SummaryCoversUpToSeq int64     `gorm:"not null;default:0" json:"summaryCoversUpToSeq,omitempty"`
    
    // 挂载资源
    AttachedDocuments    []AttachedDocument `gorm:"serializer:json" json:"attachedDocuments,omitempty"`
    
    // 交互状态
    Archived             bool      `gorm:"not null;default:false;index" json:"archived"`
    Pinned               bool      `gorm:"not null;default:false" json:"pinned"`
    
    // 局部模型路由: 覆盖全局 Settings
    ModelOverride        *ModelRef `gorm:"serializer:json;type:text" json:"modelOverride,omitempty"`
    
    CreatedAt            time.Time `json:"createdAt"`
    UpdatedAt            time.Time `json:"updatedAt"`
    DeletedAt            gorm.DeletedAt `gorm:"index" json:"-"`
}
```

---

## 2. 核心原理 (Principles)

### 2.1 The "Bubble" Indexing (置顶气泡索引)
对话列表采用 **“混合排序”** 策略实现：
- **物理 SQL**：`ORDER BY pinned DESC, created_at DESC`。
- **效果**：置顶（Pinned）的对话永远停留在左侧列表最上方，其余按时间倒序。这避免了因单个长对话频繁交互导致的 UI 列表大幅度抖动。

### 2.2 Auto-Titling Lifecycle (自动命名周期)
对话标题不是用户手动填写的（虽然支持手动改）：
1. **触发**：第一轮 Assistant 回复结束后，系统识别 `AutoTitled == false`。
2. **异步执行**：后台开启 goroutine 调用 Utility 模型。
3. **静默更新**：标题生成后直接更新 DB，并通过 **Notifications SSE** 通知前端。前端无感自动刷新侧边栏。

### 2.3 Per-Conversation Overrides (隔离重载)
每个对话可以拥有完全独立的“智力水平”：
- **原理**：`ModelOverride` 字段。
- **联动**：如果该字段非空，`chat.Service` 在解析模型时会优先忽略 `/api/v1/model-configs` 中的全局默认值。
- **用处**：用户可以在对话 A 用 GPT-4o 进行重度代码编写，而在对话 B 用 Haiku 进行快速问答，互不干扰。

---

## 3. 生命周期 (Lifecycle)

1. **新建 (Creating)**：`POST /conversations` 创建物理容器。
2. **活跃 (Interacting)**：Chat 域不断向此 ID 挂载消息。
3. **命名 (Titling)**：第一回合后自动赋予语义标题。
4. **归档 (Archiving)**：用户点击归档。对话在默认列表中隐藏，但不物理删除。
5. **销毁 (Purging)**：调 DELETE。系统执行级联操作，彻底清理所属的所有 `messages`, `blocks` 和 `attachments`。

---

## 4. 跨域集成 (Interactions)

- **Chat**：作为消息的根容器。
- **Document**：解析 `AttachedDocuments` 列表。
- **Model**：提供 `ModelRef` 校验支持。
- **Relation**：作为 RelGraph 的节点，记录 `forged_in` 关系。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 备注 |
|---|---|---|---|
| `ErrNotFound` | 404 | `CONVERSATION_NOT_FOUND` | ID 拼错或属于另一个用户。 |
| `ErrTitleTooLong` | 400 | `INVALID_REQUEST` | 标题超过 100 字符限制。 |
| `ErrDeleteFailed` | 500 | `INTERNAL_ERROR` | 数据库级联删除超时。 |

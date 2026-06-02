---
id: DOC-113
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Memory Domain — 跨对话长期记忆与事实索引

> **核心地位**：Memory 是 Forgify 的 **“长期语义存储”**。它打破了对话（Conversation）之间的物理隔离，通过存储关键事实、用户偏好和项目背景，赋予 Agent “跨越时空的记忆”。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `Memory` 实体
```go
type Memory struct {
    ID          string         `gorm:"primaryKey;type:text" json:"id"` // mem_<16hex>
    UserID      string         `gorm:"not null;type:text;uniqueIndex:idx_memory_user_name,priority:1" json:"userId"`
    
    // 唯一标识 (LLM-facing ID)
    Name        string         `gorm:"not null;type:text;uniqueIndex:idx_memory_user_name,priority:2" json:"name"`
    
    // 类型: user (个人习惯) | project (项目事实) | feedback (纠偏)
    Type        string         `gorm:"not null;type:text;index" json:"type"`
    
    Source      string         `gorm:"not null;type:text" json:"source"` // user|ai
    Content     string         `gorm:"not null;type:text" json:"content"`
    
    // 状态
    Pinned      bool           `gorm:"not null;default:false;index" json:"pinned"`
    
    // 热度统计
    AccessedAt  time.Time      `gorm:"index" json:"accessedAt"`
    AccessCount int            `gorm:"not null;default:0;index" json:"accessCount"`
    
    CreatedAt   time.Time      `json:"createdAt"`
    UpdatedAt   time.Time      `json:"updatedAt"`
    DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}
```

---

## 2. 核心原理 (Principles)

### 2.1 Explicit Pining (置顶注入)
Memory 不走向量检索（V1.2 决策），而是走 **“显示置顶”**：
- **Pinned 策略**：用户或 AI 标记为 `pinned=true` 的记忆，会 100% 注入到该用户所有对话的 System Prompt 中。
- **限制**：每个用户建议 Pinned 数量不超过 20 条，以防止 Context 溢出。

### 2.2 LRU-Style Index (热度索引)
对于非 Pinned 的记忆：
- **原理**：后端维护一个基于 `AccessCount` 和 `AccessedAt` 的排行榜。
- **注入逻辑**：Catalog 模块会自动选取前 N 条（通常为 5-10 条）热点记忆注入上下文，确保 LLM 总是能感知到最活跃的背景。

### 2.3 LLM-Driven Lifecycle (自进化记忆)
- **写入**：LLM 调 `write_memory` 记录新事实。
- **更新**：LLM 调 `write_memory` (upsert) 修正旧事实。
- **清理**：LLM 调 `delete_memory` 移除过时信息。

---

## 3. 生命周期 (Lifecycle)

1. **学习 (Learning)**：LLM 在对话中捕捉到“我不喜欢 Python 3.8”这种偏好，调工具写入。
2. **持久化 (Persisting)**：写入 `memories` 表，计算 `name` 唯一性。
3. **分发 (Distributing)**：用户开启新对话，System Prompt 构建模块通过 `ListForIndex` 抓取 Pinned 和热点记忆。
4. **衰减 (Decaying)**：长时间未访问的记忆其 `AccessedAt` 变旧，逐渐淡出非 Pinned 索引。

---

## 4. 跨域集成 (Interactions)

- **Chat**：作为系统提示词的 `memory` 段来源。
- **User**：严格按 `user_id` 物理隔离，A 用户的习惯不会影响 B 用户的 Agent。
- **Relation**：建立 `memory_about_document` 等语义关联。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 备注 |
|---|---|---|---|
| `ErrNameConflict` | 409 | `MEMORY_NAME_CONFLICT` | ID 碰撞。 |
| `ErrInvalidName` | 400 | `MEMORY_INVALID_NAME` | 含有非法字符或空格。 |
| `ErrNotFound` | 404 | `MEMORY_NOT_FOUND` | |

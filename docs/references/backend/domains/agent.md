---
id: DOC-129
type: reference
status: active
owner: @weilin
created: 2026-06-01
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Agent Domain — 实体化 AI Worker 与 Quadrinity 规格

> **核心地位**：Agent 是 Forgify 的“第四支柱” (Quadrinity)。与临时生成的 Chat Agent 不同，本域定义的 Agent 是 **“持久化、版本化、可重用”** 的专业 AI Worker。它可以独立存在，也可以作为 Workflow 节点被引用。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `Agent` (实体主表)
```go
type Agent struct {
    ID              string         `gorm:"primaryKey;type:text" json:"id"` // ag_<16hex>
    UserID          string         `gorm:"not null;index" json:"-"`
    Name            string         `gorm:"not null;type:text" json:"name"`
    Description     string         `gorm:"type:text;default:''" json:"description"`
    Tags            []string       `gorm:"serializer:json;type:text;default:'[]'" json:"tags"`
    
    NeedsAttention  bool           `gorm:"not null;default:false" json:"needsAttention"`
    AttentionReason string         `gorm:"type:text;default:''" json:"attentionReason,omitempty"`
    
    ActiveVersionID string         `gorm:"type:text;default:''" json:"activeVersionId"`
    CreatedAt       time.Time      `json:"createdAt"`
    UpdatedAt       time.Time      `json:"updatedAt"`
    DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}
```

### 1.2 `AgentVersion` (配置快照)
```go
type AgentVersion struct {
    ID            string         `gorm:"primaryKey;type:text" json:"id"` // agv_<16hex>
    AgentID       string         `gorm:"not null;index" json:"agentId"`
    Status        string         `gorm:"not null;default:'pending'" json:"status"` // pending|accepted
    Version       *int           `gorm:"type:integer" json:"version,omitempty"`
    
    // 挂载件 (Mounts)
    Prompt        string         `gorm:"type:text;default:''" json:"prompt"` // System Prompt
    Skill         string         `gorm:"type:text;default:''" json:"skill"`  // 引用的 Skill 名
    Knowledge     []string       `gorm:"serializer:json;type:text" json:"knowledge"` // doc_ID 列表
    Tools         []ToolRef      `gorm:"serializer:json;type:text" json:"tools"`     // 引用的实体列表
    
    // 约束
    OutputSchema  *OutputSchema  `gorm:"serializer:json;type:text" json:"outputSchema"`
    ModelOverride *ModelRef      `gorm:"serializer:json;type:text" json:"modelOverride"`
    
    CreatedAt     time.Time      `json:"createdAt"`
    UpdatedAt     time.Time      `json:"updatedAt"`
}
```

---

## 2. 核心原理 (Principles)

### 2.1 挂载件架构 (Mounts Architecture)
Agent 不直接编写逻辑代码，而是通过 **“挂载”** 其它领域的实体来定义能力：
- **Knowledge Mount**：挂载 `document` 实体。系统在执行该 Agent 时，会自动将这些文档的内容展开为 XML 注入 Context。
- **Tool Mount**：显式授权该 Agent 可用的工具（`fn_`, `hd_`, `mcp:`）。禁止 Agent 递归引用另一个 Agent ID（ADR-010）。
- **Output Schema 强制**：若配置了 Schema，系统在 LLM 生成结束后会执行强制 JSON 校验或重试。

### 2.2 Sub-step Replay (子步重放 - ADR-010)
当 Agent 作为 Workflow 的一个节点执行时：
- **问题**：一个 Agent 回回合可能包含 5 次工具调用。如果 Workflow 在第 3 次调用后崩溃，重启后不应重新消耗前 2 次 Token。
- **方案**：解释器通过 `AgentSubSteps` 句柄，将 Agent 内部的每一轮 LLM 响应和工具结果都记入 `flowrun_events`。
- **效果**：重放时，Agent 会“快进”到最后一个未完成的子步。

---

## 3. 生命周期 (Lifecycle)

1. **锻造 (Forging)**：用户或 AI 调 `create_agent` 工具，填入 Prompt 和 Mounts。
2. **待审 (Pending)**：生成 `agv_` 记录。此时该 Agent 尚不可被 Workflow 引用。
3. **转正 (Accepting)**：用户确认配置，Pending -> Accepted。
4. **嵌入 (Embedding)**：在 Workflow 图中通过 `agentRef: "ag_xxx"` 进行引用。
5. **执行 (Execution)**：Scheduler 唤起 `chatHost`，加载 Agent 配置，启动 ReAct 循环。

---

## 4. 跨域集成 (Interactions)

- **Workflow**：通过 `agent` 节点类型引用。
- **Document**：解析 `Knowledge` 列表。
- **Capability Catalog**：Agent 实体会作为一类特殊的“能力”出现在系统的全局 Catalog 中，供主对话 Agent 发现。
- **Relation**：建立 `agent_uses_document` 和 `workflow_uses_agent` 关联。

---

## 5. 错误字典 (Sentinels)

| Sentinel | Wire Code | 备注 |
|---|---|---|
| `ErrNotFound` | `AGENT_NOT_FOUND` | |
| `ErrNoActiveVersion`| `AGENT_NO_ACTIVE_VERSION` | 尝试运行一个未转正的 Agent。 |
| `ErrToolsAgentRef` | `AGENT_TOOLS_AGENT_REF_FORBIDDEN` | 安全红线：禁止 Agent 互相调用。 |
| `ErrNoPending` | `AGENT_NO_PENDING` | accept 动作前提不符。 |

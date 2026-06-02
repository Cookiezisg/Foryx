---
id: DOC-128
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Workflow Domain — Durable Authoring 可视化编排原理

> **核心地位**：Workflow 是 Forgify 的“四项全能” (Quadrinity) 核心。它不持有执行代码，而是作为 **“逻辑编排器”**，通过 DAG (有向图) 定义节点间的拓扑关系与数据流。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `Workflow` (实体主表)
```go
type Workflow struct {
    ID              string         `gorm:"primaryKey;type:text" json:"id"` // wf_<16hex>
    UserID          string         `gorm:"not null;index" json:"-"`
    Name            string         `gorm:"not null;type:text" json:"name"`
    Description     string         `gorm:"type:text;default:''" json:"description"`
    
    // 运行策略
    Enabled         bool           `gorm:"not null;default:true" json:"enabled"`
    Concurrency     string         `gorm:"type:text;not null;default:'serial'" json:"concurrency"` // serial|AllowAll
    TimeoutSec      int            `gorm:"not null;default:0" json:"timeoutSec"`
    
    // 自动下线保护: 若关键引用 (fn/hd) 被删，此位置 true
    NeedsAttention  bool           `gorm:"not null;default:false" json:"needsAttention"`
    
    ActiveVersionID string         `gorm:"type:text;default:''" json:"activeVersionId"`
    CreatedAt       time.Time      `json:"createdAt"`
    UpdatedAt       time.Time      `json:"updatedAt"`
    DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}
```

### 1.2 `Version` (冻结快照表)
```go
type Version struct {
    ID           string    `gorm:"primaryKey;type:text" json:"id"` // wfv_<16hex>
    WorkflowID   string    `gorm:"not null;index" json:"workflowId"`
    Status       string    `gorm:"not null;type:text" json:"status"` // pending|accepted|rejected
    Version      *int      `gorm:"type:integer" json:"version,omitempty"`
    
    // 核心图定义: 存储为物理 JSON 字符串
    Graph        string    `gorm:"type:text;not null;default:'{}'" json:"-"` 
    
    ChangeReason string    `gorm:"type:text;default:''" json:"changeReason"`
    CreatedAt    time.Time `json:"createdAt"`
}
```

---

## 2. 核心原理 (Principles)

### 2.1 5-节点收敛模型
为了降低编排熵增，V1.2 强制所有能力通过 5 种核心节点实现：
1. **Trigger**：流量入口（Cron/Webhook/Polling）。
2. **Agent**：LLM 决策节点。
3. **Tool**：能力派发点（前缀路由 `fn_`, `hd_`, `mcp:`, `skill:`）。
4. **Case**：基于 CEL 的 XOR 多路径分支。
5. **Approval**：人工干预暂停点。

### 2.2 Durable Authoring (持久化编写)
- **原理**：编辑操作不直接改写 Active 行。
- **流程**：`edit_workflow` 产生一个 `pending` 版本。用户在 UI 侧反复调整、测试。
- **原子切换**：只有在调 `:accept` 时，系统才将 `active_version_id` 原子更新。这保证了生产环境的 Workflow 永远跑在稳定的快照上。

### 2.3 严格校验 (ValidateGraph)
创建或编辑时，系统强制运行物理校验器：
- **DFS 回边检测**：仅支持 **Reducible Loops** (单入口循环)。对于无法确定终结点的 Irreducible 循环，物理层直接拒绝。
- **CEL 预编译**：图中所有的 `when` 分支和 `emit` 表达式必须在保存前成功编译，防止运行期出现语法崩溃。

---

## 3. 生命周期 (Lifecycle)

1. **锻造 (Forging)**：LLM 调 `create_workflow` Ops。
2. **校验 (Validating)**：后端检查 Trigger 节点是否存在、是否有悬空边。
3. **试运行 (Dry-Run)**：Scheduler 在 mock 环境下走一遍图逻辑。
4. **版本转正 (Accepting)**：Pending -> Accepted。
5. **调度运行 (Running)**：Trigger 触发，Interpreter 按图索骥。

---

## 4. 跨域集成 (Interactions)

- **Relation**：在 AcceptVersion 时，系统扫描图引用，自动向 `relations` 表写入 `workflow_uses_function` 等边，供全仓引用计数。
- **Mention**：Workflow 详情可作为 `@mention` 注入对话，LLM 会抓取图定义进行分析。
- **Scheduler**：作为图定义的唯一“读者”。

---

## 5. 错误字典 (Sentinels)

| Sentinel | Wire Code | 备注 |
|---|---|---|
| `ErrNoTrigger` | `WORKFLOW_NO_TRIGGER` | 图中没有设置入口。 |
| `ErrDAGCycle` | `WORKFLOW_DAG_CYCLE` | 存在非法的 Irreducible 循环。 |
| `ErrInvalidReference`| `WORKFLOW_INVALID_REF` | 引用了不存在的 fn_/hd_。 |
| `ErrNoActiveVersion` | `NO_ACTIVE_VERSION` | 该 Workflow 还没 Accept 过任何图。 |
| `ErrWorkflowDisabled`| `WORKFLOW_DISABLED` | 手动下线状态。 |

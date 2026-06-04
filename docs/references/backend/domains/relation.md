---
id: DOC-117
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Relation Domain — 跨实体关系网 (RelGraph) 数据底座

> **核心地位**：Relation 是 Forgify 的 **“实体粘合剂”**。它将孤立的 Function, Handler, Workflow, Agent, Document 等实体连接成一张巨大的拓扑图，支撑着全仓搜索、引用计数和血缘分析。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `Relation` 实体 (物理边)
```go
type Relation struct {
    ID       string         `gorm:"primaryKey;type:text" json:"id"` // rel_<16hex>
    UserID   string         `gorm:"not null;index" json:"-"`
    
    // 源节点 (From)
    FromKind string         `gorm:"not null;index:idx_rel_fwd,priority:2" json:"fromKind"`
    FromID   string         `gorm:"not null;index:idx_rel_fwd,priority:3" json:"fromId"`
    
    // 目标节点 (To)
    ToKind   string         `gorm:"not null;index:idx_rel_rev,priority:2" json:"toKind"`
    ToID     string         `gorm:"not null;index:idx_rel_rev,priority:3" json:"toId"`
    
    // 关系类型 (Enum)
    Kind     string         `gorm:"not null;uniqueIndex:uq_rel,priority:6" json:"kind"`
    
    // 附加属性: 如引用次数、行号等
    Attrs    map[string]any `gorm:"serializer:json;type:text" json:"attrs,omitempty"`
    
    CreatedAt time.Time      `json:"createdAt"`
}
```

### 1.2 关系类型详表 (The Schema)
| Kind | 含义 | 触发场景 |
|---|---|---|
| `uses_function` | 引用了 Function | Workflow/Agent 锻造 |
| `uses_handler` | 引用了 Handler | Workflow/Agent 锻造 |
| `links_to` | 内容外链 | Document [[wikilink]] |
| `forged_in` | 产生于某对话 | 实体创建时带上 convID |
| `runs_in` | 执行于某对话 | FlowRun 启动 |
| `mentions` | 消息中引用 | 发送带 @ 的消息 |

---

## 2. 核心原理 (Principles)

### 2.1 Sync-Incoming (主动同步)
Forgify 采用 **“显式写入”** 而非“动态扫描”来维护关系。
- **逻辑**：当一个 Workflow 被 Accept 时，Workflow 服务会提取图中所有引用，并调用 `Relation.SyncIncoming(fromEntity, toRefs)`。
- **原子覆盖**：Sync 操作会先物理删除该 `fromEntity` 的所有旧边，再批量插入新边。这保证了关系网永远与实体的 Active 版本同步。

### 2.2 RelGraph 拓扑快照
后端提供 `GET /api/v1/relgraph` 端点：
- **原理**：将 `relations` 表全量扫描（按用户隔离）。
- **格式**：返回 D3.js 兼容的 `nodes` 和 `links` 结构。
- **应用**：前端“全景图”模块以此为数据源。

### 2.3 级联删除保护 (Reverse Lookup)
- **原理**：当用户尝试删除一个实体（如 Function）时，删除服务会反向查找 `relations` 表中以该实体为 `ToID` 的边。
- **策略**：若发现有 Workflow 正在 `uses_function` 该实体，则物理上阻断删除（ErrInUse），直到用户先修改 Workflow 图。

---

## 3. 生命周期 (Lifecycle)

1. **识别 (Detecting)**：实体发生变更（Accept Version / Update Content）。
2. **重整 (Re-syncing)**：调用 `PurgeEntity` 清理旧关联。
3. **沉淀 (Persisting)**：批量写入新关系记录。
4. **消费 (Querying)**：前端拉取 neighborhood 或全量图。
5. **销毁 (Cleanup)**：实体彻底被硬删时，触发全表级联清理。

---

## 4. 跨域集成 (Interactions)

- **Workflow/Agent**：锻造成功后的“第一读者”。
- **Document**：解析 Wikilink 的驱动方。
- **APIKey**：通过 workspace 默认模型列与实体 model override 关联，防止 Key 被误删。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 备注 |
|---|---|---|---|
| `ErrInvalidEntityRef` | 400 | `REL_INVALID_REF` | 源或目标实体类型不支持。 |
| `ErrSelfLoop` | 400 | `REL_SELF_LOOP` | 物理约束：实体不能指向自己。 |
| `ErrDepthOutOfRange` | 400 | `REL_DEPTH_LIMIT` | neighborhood 查询深度超限。 |

---
id: DOC-107
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Document Domain — Notion-style 树状知识库与 RAG 引擎

> **核心地位**：Document 是 Forgify 的 **“本地知识脑库”**。它支持无限层级的文件夹结构，不仅作为用户的 Wiki，更作为 RAG (检索增强生成) 的核心数据源，直接注入到 Agent 的 Context 中。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `Document` 实体
```go
type Document struct {
    ID          string         `gorm:"primaryKey;type:text" json:"id"` // doc_<16hex>
    UserID      string         `gorm:"not null;index" json:"userId"`
    
    // 树状结构
    ParentID    *string        `gorm:"index;type:text" json:"parentId,omitempty"`
    Name        string         `gorm:"not null;type:text" json:"name"`
    Path        string         `gorm:"index;not null;type:text" json:"path"` // 全路径如 /work/spec.md
    
    Description string         `gorm:"type:text;default:''" json:"description"`
    Content     string         `gorm:"type:text;default:''" json:"content"` // 1MB 限制
    
    // 渲染参数
    Position    int            `gorm:"not null;default:0" json:"position"`
    Tags        []string       `gorm:"serializer:json;type:text" json:"tags"`
    
    SizeBytes   int64          `gorm:"not null;default:0" json:"sizeBytes"`
    CreatedAt   time.Time      `json:"createdAt"`
    UpdatedAt   time.Time      `json:"updatedAt"`
    DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}
```

### 1.2 `AttachedDocument` (引用协议)
这是在 `Conversation` 或 `Agent` 中持久化的 DTO。
```typescript
interface AttachedDocument {
    documentId: string;
    includeSubtree: boolean; // 是否包含该文件夹下所有后代
}
```

---

## 2. 核心原理 (Principles)

### 2.1 Virtual Path (虚拟路径同步)
后端在每次 `Move` 或 `Rename` 操作后，会自动计算并递归更新所有子节点的 `Path` 字段。
- **优点**：支持通过 `LIKE '/work/%'` 进行极速的子树检索，无需递归 DB 查询。
- **物理约束**：同一父节点下禁止重名。

### 2.2 XML-Style Context Injection
当 Document 被挂载到对话时：
1. **即时展开**：根据 `includeSubtree` 标志，系统在运行时扫描并抓取所有相关的 `doc_` 内容。
2. **XML 封装**：
   ```xml
   <document path="/work/prd.md" id="doc_123">
     [内容明文]
   </document>
   ```
3. **注入位置**：注入在 System Prompt 的 `documents` 段，利用 XML 标签辅助模型精确定位。

### 2.3 Wikilink 解析
系统在保存 Document 内容时，会自动正则匹配 `[[doc_<id>]]` 或 `[[fn_<id>]]`：
- **原理**：将解析结果同步到 `relations` 表。
- **效果**：在 UI 侧可以实时看到“谁引用了这篇文章”的反向链接。

---

## 3. 生命周期 (Lifecycle)

1. **创建 (Creating)**：用户调 POST API，指定 `parentId`。
2. **编辑 (Editing)**：支持实时保存草稿。
3. **挂载 (Attaching)**：用户在对话侧栏选择文档。
4. **注入 (Hydrating)**：LLM 每次生成前，系统动态拼装文档 XML。
5. **归档 (Archiving)**：软删除，保留数据以便已发送的消息可以解引用。

---

## 4. 跨域集成 (Interactions)

- **Agent**：`Knowledge` 挂载点的物理载体。
- **Chat**：作为 RAG 源注入。
- **Relation**：作为 RelGraph 的中心节点，维护 `links_to` 关系。
- **Search**：支持对文档全内容的词频检索。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 场景 |
|---|---|---|---|
| `ErrNameConflict` | 409 | `DOCUMENT_NAME_CONFLICT` | 同目录下重名。 |
| `ErrInvalidParent`| 422 | `DOCUMENT_INVALID_PARENT` | 尝试将父文件夹移入自己的子文件夹。 |
| `ErrContentTooLarge`| 413 | `DOCUMENT_TOO_LARGE` | 超过 1MB 安全上限。 |
| `ErrParentNotFound`| 422 | `DOCUMENT_PARENT_MISSING`| `parentId` 指向了虚空。 |
| `ErrNotFound` | 404 | `DOCUMENT_NOT_FOUND` | |

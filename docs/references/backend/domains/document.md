---
id: DOC-107
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-06
review-due: 2026-09-01
audience: [human, ai]
---
# Document Domain — Notion-style 树状知识库

> **核心地位**：Document 是 Forgify 的**本地知识脑库**——无限层级的 markdown 文档树，既是用户的 Wiki，也能被 @ 引用、显式挂载到对话/workflow 当背景资料、用 wikilink 互链。**无 RAG / 无向量检索**：注入靠用户显式挑选，可控、有界。

---

## 1. 物理模型

### 1.1 `Document` 实体（按 workspace、软删）
markdown 树的一个节点，`ParentID nil = 根级`。去 GORM——纯 struct + 轻量 db tag，workspace 隔离由 orm 自动施加。
```go
type Document struct {
    ID          string     `db:"id,pk"`              // doc_<16hex>
    WorkspaceID string     `db:"workspace_id,ws"`
    ParentID    *string    `db:"parent_id"`
    Name        string     `db:"name"`
    Description string     `db:"description"`
    Content     string     `db:"content"`           // markdown, ≤ 1 MB
    Tags        []string   `db:"tags,json"`
    Position    int        `db:"position"`
    Path        string     `db:"path"`              // "/Parent/Child"
    SizeBytes   int64      `db:"size_bytes"`
    CreatedAt   time.Time  `db:"created_at,created"`
    UpdatedAt   time.Time  `db:"updated_at,updated"`
    DeletedAt   *time.Time `db:"deleted_at,deleted"`
}
```
`UNIQUE(workspace_id, COALESCE(parent_id,''), name) WHERE deleted_at IS NULL` — 同父名唯一（根级 NULL→'' 兜住）、软删后名复用。

### 1.2 `AttachedDocument`（引用协议）
持久化在 `Conversation` / workflow 节点的 DTO。**只引用单篇，不含子树**——子树自动注入已砍（挂载必须显式有界，不能"挂一篇拖出一整棵树"炸 context）。
```go
type AttachedDocument struct {
    DocumentID string `json:"documentId"`
}
```

---

## 2. 核心原理

### 2.1 Virtual Path（虚拟路径级联）
每次 `Move` / `Rename` 后，后端 BFS 递归重算所有子节点的 `Path`。优点：`LIKE '/work/%'` 极速子树检索；同父禁止重名（重名自动加后缀 `foo`→`foo 2`，Notion 风格）；防环（`IsAncestor` 拒绝把节点移入自己的子树）。

### 2.2 XML Context Injection（无子树）
挂载文档注入对话/workflow 节点时：`ResolveAttached` 取**显式挂的那几篇**（不展开子树）→ `RenderAttachedAsXML` 拼成 `<documents>` 段：
```xml
<document path="/work/prd.md" id="doc_123">
  [内容明文]
</document>
```
注入 system prompt 的 documents 段。

### 2.3 Wikilink → relation 边
保存内容时正则抓 `[[<prefix>_<16hex>]]`：`wikilink.Parse` 取 id（**无 kind**）→ `relation.KindForID` 据前缀解析 kind + 过滤未知前缀 + 跳自链 → 写一条 `link` 边（relation 4 动词之一）到拓扑图。UI 侧据此显示反向链接。

---

## 3. 四个适配器（接入前三模块）

document 是**第一个接通 catalog / relation / mention 的实体**，实现 4 个端口（注入在 M7）：

| 适配器 | 接入 | 作用 |
|---|---|---|
| `AsCatalogSource` | catalog | 把文档名录（name = path + description）报给能力概览，让 LLM 知道有哪些文档 |
| `AsMentionResolver` | mention | `@文档` 时抓 description+content 快照 |
| `syncRelationsForDocumentBody` / `purgeRelationsForDocuments` | relation（消费 `SyncOutgoing`/`PurgeEntity`） | wikilink → `link` 边；删除级联清边 |
| `NamesByIDs` | relation（提供 `Namer`） | relation 读时 hydrate 给 document 节点贴名 |

> 这些端口对齐的是前三模块**收窄后**的新接口（catalog 去 Granularity/InvokeTool/Category、relation 4 动词边、wikilink 去 Kind）——document 是验证那些设计的试金石。

---

## 4. 生命周期
创建（指定 parentId）→ 编辑（实时保存；改 content 重 sync wikilink）→ 挂载（用户挑文档）→ 注入（每轮拼 XML，仅显式那几篇）→ 软删（保留墓碑，已发消息可解引用；级联软删子树 + 清边）。

---

## 5. 跨域集成
- **Chat / Scheduler**：消费 `ResolveAttached` + `RenderAttachedAsXML` 注入挂载文档（波次 4/5）。
- **Catalog / Mention / Relation**：经上述 4 适配器（注入 M7）。
- **AI `:iterate`**：askai 编辑（波次 6）。

---

## 6. 错误字典

| Sentinel | Kind | Wire Code | HTTP |
|---|---|---|---|
| `ErrNotFound` | NotFound | `DOCUMENT_NOT_FOUND` | 404 |
| `ErrInvalidParent` | Unprocessable | `DOCUMENT_INVALID_PARENT` | 422 |
| `ErrNameConflict` | Conflict | `DOCUMENT_NAME_CONFLICT` | 409 |
| `ErrContentTooLarge` | TooLarge | `DOCUMENT_CONTENT_TOO_LARGE` | 413 |
| `ErrInvalidName` | Invalid | `DOCUMENT_INVALID_NAME` | 400 |
| `ErrParentNotFound` | Unprocessable | `DOCUMENT_PARENT_NOT_FOUND` | 422 |

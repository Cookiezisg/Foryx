---
id: DOC-019
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-14
review-due: 2026-09-14
audience: [human, ai]
---

# document —— Notion-style 树状文档库

## 1. 定位 + 心智模型

按 workspace 的 **markdown 树**：父子有序（`position`）、**path 寻址**（`/a/b/c`，物化列）、可被 @ 引用、挂载到对话/workflow、wikilink 互链。单实体单表（软删——删除的子树留墓碑）；上限：单篇 1MB（超出拆子文档）、标题 256 字符（不含 `/`——path 分隔符）。

## 2. 关键行为（树不变式都在 app 层）

- **Create 自动加后缀**（"foo"→"foo 2"…重试 cap 100）：POST 不该因重名失败；显式改名（PATCH）保留严格 `DOCUMENT_NAME_CONFLICT`——同一约束两种出口对应两种用户意图。
- **改名 → 子树 path 级联**（批量重写后裔 path）；**Move 防环**（`IsAncestor` 拒把节点挂到自己后裔下，`DOCUMENT_INVALID_PARENT`）、nil parent=移根、nil position=追加末尾。
- **Delete = 软删整子树**（`SoftDeleteSubtree`）+ 清全部后裔的 relation 边（`ListSubtreeIDs` BFS）。
- **attach 单篇不拖子树**（`AttachedDocument`）：挂载必须显式有界——子树自动注入刻意不做（防"挂一篇拖出一整棵树"炸 context）。
- **wikilink 出边**：body 每次写入后解析 `[[...]]`（`pkg/wikilink`）重 sync `link` 出边——文档间引用进 relation 图。
- **`Service.Search` 走 DB LIKE**（name/description，updated_at DESC limit 50）是 `search_documents` 工具的**回退**路径——主路径走统一全文内容引擎（覆盖 name + markdown 正文、带 heading snippet）；DB LIKE 这条与 fn/hd/agent 的内存子串过滤不同：文档行数可能大、且有 content 大列，DB 侧过滤是对的（有意分化）。

## 3. 契约（引用）

端点（CRUD + `POST {id}:move` 防环 + `POST {id}:duplicate` 深拷子树（BFS 自顶向下铸新 id、重映射 parent/path、复制 content、新根名去重、逐节点 Insert 非原子；可选 `{parentId}` 默认落为兄弟）+ `POST {id}:iterate` AI 编辑 + `GET ?parentId=` 直接子节点 + `GET /tree` 整树 metadata）→ [api.md](../api.md) · 表 `documents`（path/position/size_bytes 物化列）→ [database.md](../database.md) · 码 `DOCUMENT_*` 6+3 → [error-codes.md](../error-codes.md) · ID：`doc_` · 通知 `document.*`。LLM 工具 7 个（薄适配、domain 错误转软失败串供自纠）。消费方：@ 提及（快照内容）、agent knowledge 挂载（注入正文）、workflow 节点 attach、catalog。

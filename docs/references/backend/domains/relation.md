---
id: DOC-029
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
audience: [human, ai]
---

# relation —— 实体拓扑图

## 1. 定位 + 心智模型

全 workspace 的**实体关系图**：11 种 `EntityKind` 节点（Quadrinity + trigger/control/approval/skill/mcp/document/conversation）× 边 kind（equip/create/edit/link…）。**写侧是 diff-sync**：实体在 create/edit/revert 时声明"我的某 kind-scope 出/入边应是这个集合"（`SyncOutgoing`/`SyncIncoming`），relation 对照现存做增删——调用方永远声明终态、不做增量管理；`PurgeEntity` 删除时级联清边。**读侧 hydrate 显示名**：`Namers` 注册表（**11 种全注册**，bootstrap）按需批量 id→name——图存 id、名字读时取，实体改名图自动跟随。守卫：自环禁止、ref 校验、邻域深度限制。

## 2. 契约（引用）

LLM 工具：`get_relations`（neighborhood 的工具孪生——kind+id+depth(1-3)，编辑/删除前自查影响面）。

表 `relations` → [database.md](../database.md) · 码 `REL_*` 5 → [error-codes.md](../error-codes.md)。端点：list / neighborhood / relgraph（全景快照）。写方：每个实体的 relations.go 适配器（nil 容忍——relation 不在场实体照常工作）。

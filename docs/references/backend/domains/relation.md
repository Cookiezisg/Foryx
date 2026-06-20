---
id: DOC-029
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-14
review-due: 2026-09-14
audience: [human, ai]
---

# relation —— 实体拓扑图

## 1. 定位 + 心智模型

全 workspace 的**实体关系图**：11 种 `EntityKind` 节点（Quadrinity + trigger/control/approval/skill/mcp/document/conversation）× 边 kind（4 动词封闭集：create/edit/equip/link）。**写侧是 diff-sync**：实体在 create/edit/revert 时声明"我的某 kind-scope 出/入边应是这个集合"（`SyncOutgoing`/`SyncIncoming`），relation 对照现存做增删——调用方永远声明终态、不做增量管理；`PurgeEntity` 删除时级联清边。**读侧 hydrate 显示名**：`Namers` 注册表（**11 种全注册**，bootstrap）按需批量 id→name——图存 id、名字读时取，实体改名图自动跟随。`CountDependents(kind,id)` 报「删了它什么会坏」的诚实计数=**入向 equip/link 边**（挂载/外链它的实体），排除 create/edit 溯源与本实体出边（复用 `ListByToAndKinds` 的 {equip,link} 过滤、无新 repo 方法）；8 个单实体 delete 工具（function/handler/agent/workflow/trigger/control/approval/skill）在删**前**读它、把依赖数折进结果（经共享 `toolapp.DependentCounter` 端口 + `AnnotateDependents`/`DependentSuffix` helper，nil 容忍），使 agent 知道删后多少引用可能失效（F48；delete_document 因递归删 + 字符串结果不纳入）。守卫：自环禁止、ref 校验、邻域深度限制。

## 2. 契约（引用）

LLM 工具：`get_relations`（neighborhood 的工具孪生——kind+id+depth(1-3)，编辑/删除前自查影响面）。

表 `relations` → [database.md](../database.md) · 码 `REL_*` 5 → [error-codes.md](../error-codes.md)。端点：list / neighborhood / relgraph（全景快照）。写方：每个实体的 relations.go 适配器（nil 容忍——relation 不在场实体照常工作）。

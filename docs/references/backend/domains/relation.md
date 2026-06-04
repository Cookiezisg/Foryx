---
id: DOC-117
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-05
review-due: 2026-09-01
audience: [human, ai]
---
# Relation Domain — 跨实体关系网 (RelGraph) 数据底座

> **核心地位**：Relation 是 Forgify 的**实体血缘网**——把孤立的 Function / Handler / Workflow / Agent / Document / Conversation 等实体连成一张有向图，支撑全景图、邻域查询与引用血缘。它还从 idgen 收编了 **id 前缀 → 实体类型** 的路由（`KindForID`）。

---

## 1. 物理模型

### 1.1 `Relation` 实体（一行一条有向边）
`(from_kind, from_id) --kind--> (to_kind, to_id)` + 可选 `attrs`。

```go
type Relation struct {
    ID          string         `db:"id,pk"`              // rel_<16hex>
    WorkspaceID string         `db:"workspace_id,ws"`    // orm 自动隔离
    Kind        string         `db:"kind"`               // CHECK IN ('create','edit','equip','link')
    FromKind    string         `db:"from_kind"`
    FromID      string         `db:"from_id"`
    ToKind      string         `db:"to_kind"`
    ToID        string         `db:"to_id"`
    Attrs       map[string]any `db:"attrs,json"`         // 如 edit 边的版本号
    CreatedAt   time.Time      `db:"created_at,created"`
    UpdatedAt   time.Time      `db:"updated_at,updated"`
}
```

**两个有意的缺席**：
- **无 name 列**：显示名读时在内存现查（见 §4），从不入库——故改名后永远显示最新、无 stale。
- **无 deleted_at**：边是派生数据，随实体硬删（仅 Journal/Log 禁删，边不属此列）。

**索引**：`idx_rel_dedup` UNIQUE(workspace_id, from_id, to_id, kind) 保幂等；`idx_rel_from` / `idx_rel_to` 支撑邻域遍历。

### 1.2 节点类型（8 种 EntityKind）
`function` `handler` `workflow` `agent`（Quadrinity）+ `document` `conversation` + `skill` `mcp`（外部能力）。

### 1.3 边类型（4 个动词）
两端类型已在 from_kind/to_kind 列里，故 kind 只需**动词**——无论实体增加多少，边类型恒为 4 个，DB CHECK 恒为 4 值集。

| 动词 | 含义 | 谁 → 谁 | 维护方 |
|---|---|---|---|
| `create` | 对话创造实体（v1） | conversation → 实体 | 实体（SyncIncoming）|
| `edit` | 对话编辑实体（新版本） | conversation → 实体 | 实体（SyncIncoming）|
| `equip` | 挂载工具/知识 | workflow/agent → fn/hd/mcp/skill/doc | 自身（SyncOutgoing）|
| `link` | 文本性外链（仅提及） | document → 实体 | 自身（SyncOutgoing）|

> `equip` 允许 workflow → agent，但 **agent 不 equip agent**（员工不挂员工）——靠业务层 sync 保证，非 DB CHECK。

---

## 2. id 前缀 → 实体类型（`KindForID`，收编自 idgen）
从一个 id 的前缀认出它是哪种实体。**6 条现役 + 2 条已定规矩**：

| 前缀 | 实体 | · | 前缀 | 实体 |
|---|---|---|---|---|
| `fn_` | function | · | `doc_` | document |
| `hd_` | handler | · | `cv_` | conversation |
| `wf_` | workflow | · | `sk_` | skill ※ |
| `ag_` | agent | · | `mcp_` | mcp ※ |

> ※ `sk_` / `mcp_`：skill/mcp 当前未独立建表，但前缀作为规矩已定死、`KindForID` 已识别——故 document 现在就能经 wikilink `[[sk_…]]` / `[[mcp_…]]` 标记它们。`skills` / `mcps` 表与生成器接入是**波次 3** 工作，届时零改动接入。
>
> 未知前缀（含执行流水 `fne_` / `mcl_` / `ske_` 等非实体前缀）返 `ok=false`。

---

## 3. 写侧：显式写入，而非扫描
边由各 source domain 在 forge/equip/link 后**主动声明**，diff-sync 整组替换（幂等）。

### 3.1 SyncOutgoing — “我用了谁”
`workflow` / `agent` 改版后扫自己的引用算出 equip 边，整组替换自己的出边；`document` 同理写 link 边。

### 3.2 SyncIncoming — “谁产生了我”
`create` / `edit` 边方向是 `对话 → 实体`，但由**实体侧反向声明**（对话不知道自己锻造了什么，实体才知道出身）。`kindScope=[create]` / `[edit]` 配合整组替换，天然实现“一个实体至多被 1 个对话 create + 1 个对话 edit”。

### 3.3 PurgeEntity — 级联硬删
实体删除时，硬删所有 from 或 to 触及它的边。

> **best-effort**：sync 失败只 log、不阻断主流程（图是派生数据）。**无删除时引用保护**——删一个被引用的实体不会被阻止（对齐 model override 的弱引用方针），全景图下次自然反映。

---

## 4. 读侧：3 个只读端点 + 读时取名
边是派生数据，**无 POST/PATCH/DELETE**。

| 端点 | 返回 |
|---|---|
| `GET /api/v1/relations` | 按 from/to/kind 过滤、keyset 分页的边 |
| `GET /api/v1/relations/neighborhood?kind=&id=&depth=` | 中心实体 depth(1–3) 跳内的边（BFS）|
| `GET /api/v1/relgraph` | 全图快照 `{nodes, edges}` |

### 4.1 hydrateNames — 读时内存取名
边只存 id；返回前在内存按 kind 批量查显示名（注入的 `Namer.NamesByIDs`），拼成 `RelationView{…, fromName, toName}`。
- **不落库**：每次读现查源表，故改名即刻反映、无 stale，也无需任何写时刷新。
- **回退**：名字查不到（实体已删，或 skill/mcp 暂无表）→ 显示 id。

### 4.2 relgraph 节点
`nodes` 由边端点去重而来（`{kind, id, name}`）。**孤立实体（无任何边）不出现**——图展示关系，不是清单。

---

## 5. 跨域集成（消费者，波次 2/3/5）
- **workflow / agent**：锻造后 SyncOutgoing 写 equip 边、SyncIncoming 写 create/edit 边；删除时 PurgeEntity。
- **document**：解析 `[[wikilink]]` → `KindForID` 认类型 → SyncOutgoing 写 link 边。
- **各实体域**：各实现一个 `Namer.NamesByIDs`（一句 `WHERE id IN …` 取 name），装配时注入 relation Service（波次 3）。

---

## 6. 错误字典

| Sentinel | Wire Code | HTTP | 场景 |
|---|---|---|---|
| `ErrInvalidRef` | `REL_INVALID_REF` | 400 | 源/目标 ref 空 id 或未知实体类型 |
| `ErrInvalidKind` | `REL_INVALID_KIND` | 400 | 边类型非 create/edit/equip/link |
| `ErrSelfLoop` | `REL_SELF_LOOP` | 400 | 禁止自环（from == to）|
| `ErrDepthOutOfRange` | `REL_DEPTH_LIMIT` | 400 | neighborhood 深度超 [1,3] |
| `ErrIncompleteFilter` | `REL_INCOMPLETE_FILTER` | 400 | filter 的 kind/id 未成对 |

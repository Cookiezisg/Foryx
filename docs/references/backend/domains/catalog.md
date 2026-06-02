---
id: DOC-103
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Capability Catalog Domain — 工具全态目录与 RAG 索引

> **核心地位**：Catalog 是 Forgify 的 **“实体地图”**。它的职责是统一收纳 Function, Handler, Workflow, Agent, Skill, MCP 六大来源的所有能力，并将其转化为 LLM 可理解的语义描述，解决“LLM 如何知道它有哪些工具”的问题。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `CatalogItem` (逻辑 DTO)
Catalog 是纯内存派生出的虚表，不持久化（除非未来规模过大需要做本地索引）。
```typescript
interface CatalogItem {
    kind: "function" | "handler" | "workflow" | "agent" | "skill" | "mcp";
    id: string;          // 物理主键或 Skill Name
    name: string;        // 语义名称
    description: string; // 描述文案
    source: string;      // 来源说明
}
```

---

## 2. 核心原理 (Principles)

### 2.1 Multi-Source Aggregation (多源归集)
Catalog 实现了 `CatalogSource` 接口，定期或在对话开启时扫描全系统：
- **Forge 域**：抓取所有 `Accepted` 状态的实体元数据。
- **Skill 域**：扫描 `~/.forgify/skills/` 下的 Frontmatter。
- **MCP 域**：通过 RPC `listTools` 发现外部工具。

### 2.2 System-Prompt Rendering (提示词渲染)
后端在构建 System Prompt 时：
1. **全量扫描**：拉取所有活跃能力。
2. **格式化**：
   ```text
   - [function] fn_hello: 这是一个问候函数。
   - [workflow] wf_deploy: 执行自动化部署。
   ```
3. **注入**：填入 System Prompt 的 `capabilities` 段落。

### 2.3 On-demand Loading (按需激活策略)
为了防止 System Prompt 被数千个工具撑爆：
- **V1.2 现状**：全量注入（针对单机场景，工具数通常 < 100）。
- **预留机制**：支持 `activate_tools(category)` 手动扩充。LLM 如果发现 Catalog 中有该分类但没加载，会主动调工具加载。

---

## 3. 生命周期 (Lifecycle)

1. **注册 (Registration)**：实体被 Accept 或 MCP Server 被安装。
2. **同步 (Indexing)**：后端 `CatalogManager` 内存哈希表更新。
3. **消费 (Discovery)**：
   - 用户调 `GET /api/v1/catalog` 预览。
   - 对话开启，注入提示词。
4. **失效 (Invalidation)**：实体删除，内存记录同步抹除。

---

## 4. 跨域集成 (Interactions)

- **Chat**：最重要的消费者，通过 `SystemPromptProvider` 接口调用。
- **Relation**：依赖 RelGraph 数据判定实体活跃度。
- **User**：严格按用户隔离（A 用户的私有工具不会出现在 B 用户的 Catalog 中）。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 场景 |
|---|---|---|---|
| `ErrAllSourcesFailed`| 500 | `CATALOG_SCAN_ERROR` | 极其严重的错误：所有领域 Repository 均无法连接。 |
| `ErrItemNotFound` | 404 | `NOT_FOUND` | 查询特定能力详情失败。 |

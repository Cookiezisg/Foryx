---
id: WRK-007
type: working
status: active
owner: @weilin
created: 2026-06-12
reviewed: 2026-06-12
review-due: 2026-09-12
expires: 2026-09-12
landed-into: ""
audience: [human, ai]
---

# 统一搜索服务（BM25 + RAG）—— 现状与完整技术方案

> **状态：方案待审，未动工。** 批准后按 §10 分期实施；落地后结论提取进 `concepts/architecture.md` + references 四件套，本文档填 `landed-into` 移 `archive/`。

---

## 1. 现状（2026-06-12，逐处核实）

### 1.1 现有搜索面

| 搜索面 | 实现 | 局限 |
|---|---|---|
| HTTP List `?q=` | `LIKE '%x%'`（document 的 name/description、conversation 的 title 等） | 只搜元数据、无相关性排序、无正文 |
| 实体垂搜 tool ×8（`search_function` / `search_handler` / `search_agent` / `search_workflow` / `search_triggers` / `search_control` / `search_approval` / `search_documents`） | 各 Service.Search：大小写不敏感**子串匹配** name / description / tags | 同上；互相孤立、无跨实体入口 |
| 执行日志 tool ×4（`search_function_executions` / `search_handler_calls` / `search_agent_executions` / `search_activations`） | 结构化过滤（状态/时间窗），非全文 | 非本方案范畴（见 §3.3） |
| `search_tools`（工具发现） | 内存里 query 词与 name+description 重叠计数 | 独立小宇宙、规模小、**不并入** |
| 内容正文 | **完全不可搜** | 最大缺口（见下） |

不可搜的正文清单：document `content`、message_blocks 对话正文、function/handler **代码**、workflow 图（节点/CEL）、skill `SKILL.md`、memory 文件、trigger 配置语义、control branches（CEL 分支）、approval template。

### 1.2 物理事实（本机探针实测）

- 驱动 `glebarez/go-sqlite v1.21.2`（modernc 1.23.1）= **SQLite 3.41.2**：
  - ✅ FTS5 可用、`bm25()` 排序可用、`tokenize='trigram'` 可用（中文「工作流」命中「持久化工作流执行引擎」）、external-content 表可用——**BM25 零新依赖**。
  - ⚠️ trigram 对 **<3 字符查询零命中**（实测 2 字「引擎」0 hit）——必须有短词回退。
  - ❌ 纯 Go 驱动**不能加载 C 扩展**（sqlite-vec/sqlite-vss 排除）——向量必须自有表。
- **全部目标实体均 workspace 隔离**：DB 表带 `workspace_id`；skill/memory 文件态也按 `~/.forgify/workspaces/<wsID>/{skills,memories}/` 分桶。索引无需全局行特例。
- 相关表形态（节选）：trigger（`kind IN (cron,webhook,fsnotify,sensor)` + `config` JSON + `outputs` JSON，无版本表）；control（`control_logics` + `control_logic_versions.branches`(CEL) + `inputs`）；approval（`approval_forms` + `approval_form_versions.template/inputs/timeout`）；message_blocks（`type IN (text,reasoning,tool_call,tool_result,compaction,progress)`）。

### 1.3 缺口一句话

全系统只有「元数据子串匹配」：没有正文索引、没有相关性排序、没有跨实体统一入口、没有 RAG 取数口。

---

## 2. 方案总览

### 2.1 目标能力（四面一引擎）

1. **Tool call 检索**：现有 8 个垂搜 tool 保契约换引擎；新增跨实体 `search_workspace`（LLM 搓工作流找积木的主入口）。
2. **综搜**：`GET /api/v1/search?q=...`（前端 Cmd+K 全局搜索框）。
3. **垂搜**：同端点 `&types=function,document`。
4. **RAG 取数**：app 内部 `Retrieve(ctx, q, opts)`，返回 chunk 粒度 + 分数（agent 上下文注入 / 未来知识挂载）。

### 2.2 选型裁决（主流方案对照，全部已决）

| 维度 | 裁决 | 落选项与理由 |
|---|---|---|
| 词法检索 | **SQLite FTS5 + bm25()**（驱动内置，已实测） | Bleve：第二份索引文件 + 新依赖，FTS5 在场即重复轮子（原则 #8）；自研倒排：造轮子 |
| 中文分词 | **trigram**（免词典、中英文/代码统一子串语义）+ **短词 LIKE 回退** | gse（纯 Go jieba）：新依赖 + 词典常驻 ~50MB 内存，本地规模 trigram 召回已够；不满意时可日后叠加第二索引列，地基不变 |
| 向量存储 | **自有表 float32 BLOB + Go 内暴力余弦**（本地 ≤10 万 chunk 毫秒级，FAISS 在此规模也推荐 Flat） | sqlite-vec：C 扩展不可加载；chromem-go 等：新依赖且内部同样是暴力余弦 |
| Embedder | **可选端口**：workspace 配置 `searchSemantic: off\|ollama`，默认 off；Ollama 适配（推荐 bge-m3，中英双语）；缺席/失败**无声降级纯 BM25** | 云端 embedding API：违反「不依赖 embedding API」约束（架构上留适配器位，未来要 BYOK 只加枚举值） |
| 混合融合 | **RRF（k=60）**——Elasticsearch/Weaviate/LanceDB 默认 | 线性加权：要调权、对分数尺度敏感 |
| 索引同步 | **写后 Enqueue + 单 worker 异步 + boot 对账**（索引=纯派生数据，永远可重建） | 事务内同步写：耦合业务事务、慢热路径；复用 entitystream：那是 SSE 直播帧原语非 CRUD 变更流 |

### 2.3 架构图

```
                ┌─ GET /api/v1/search?q=&types=        ← 综搜（types 空）/ 垂搜
三个出口         ├─ tool: search_workspace（新）+ 8 个 search_<entity> 换引擎
                └─ app 内部 Retrieve(ctx, q, opts)      ← RAG 取数
                         │
                 app/search.Service
                 ├─ Search():  FTS5 BM25 ──┐
                 │             向量(可选) ───┴─ RRF → boost → 折叠 → 分页/snippet
                 ├─ Retrieve(): 同管线，chunk 粒度
                 └─ Indexer:   Enqueue 队列 + 单 worker + boot 对账
                         │ DIP：12 个 SearchSource 端口（各实体 app Service 实现）
                 infra/search
                 ├─ search_docs 投影表 + search_fts(FTS5 external-content) + 触发器
                 ├─ search_meta（fts schema 版本）
                 └─ search_embeddings（M3 可选）
```

---

## 3. 实体覆盖与内容映射（12 类）

### 3.1 覆盖全集

用户钦定 11 类（对话/function/handler/agent/mcp/skill/document/workflow/**trigger/control/approval**）+ 产品维度评估补 1 类（**memory**——agent 长期记忆正是「我记得存过关于 X」要找的东西）。

| type | 来源 | title | body | anchor | 分块 |
|---|---|---|---|---|---|
| `conversation` | conversations + message_blocks | 会话标题 | **仅 `text` 块**正文（排除 tool_result / reasoning / tool_call / compaction / progress——噪声与体积） | message_id（前端直达） | 每条 message 一行（chunk_no = message 序） |
| `document` | documents | 名 + 路径 | `content` 列 | 标题链 | markdown 标题感知切 ~512 token、10% 重叠 |
| `function` | functions + 活跃版本 | 名 + 描述 + tags | **代码** + 入出参 schema 文本化 | — | 代码按 ~512 token 行对齐切 |
| `handler` | handlers + 活跃版本 | 名 + 描述 + tags | **类代码** + 方法签名清单 | 方法名 | 同上 |
| `agent` | agents + 活跃版本 | 名 + 描述 + tags | prompt + 挂载（skill/知识/工具）说明 | — | 整体（超长再切） |
| `workflow` | workflows + 活跃版本 | 名 + 描述 | 图文本化：节点名 + ref + CEL 条件/映射表达式拼接 | 节点 id | 整体 |
| `mcp` | mcp_servers | server 名 | 工具名 + 工具描述清单（缓存的 tool list） | 工具名 | 整体 |
| `skill` | skillfs（文件态，ws 分桶） | 名 + frontmatter 描述 | SKILL.md 正文 | 标题链 | 按标题切 |
| `memory` | memoryfs（文件态，ws 分桶） | slug 名 | markdown 正文 | — | 整体 |
| `trigger` | triggers | 名 + 描述 | **kind + outputs 字段名**（`config` 整体不入索——webhook/sensor 配置可能含 secret，安全红线优先于 cron 表达式的可检索性） | — | 整体 |
| `control` | control_logics + 活跃版本 | 名 + 描述 | inputs 字段名 + branches 的 CEL 表达式文本 | 分支序 | 整体 |
| `approval` | approval_forms + 活跃版本 | 名 + 描述 | inputs 字段名 + template 正文 | — | 整体 |

版本语义统一：**只索活跃版本**（pin 闭包/历史版本不入索——搜索面向「现在可用的积木」）；Edit 产生新活跃版本 → 重索；Revert 同理。

### 3.2 已裁决取舍（用户确认 + 本方案固化）

- ✅ 代码入索（fn/hd 的活跃版本代码、wf 的 CEL、control 的 branches）——trigram 搜代码子串恰好是强项。
- ❌ tool_result / reasoning 不入索。
- ❌ **密文红线**：api_keys 表整体不入索；mcp `config`（密文落盘）不入索（只 name + tools）；trigger `config` 不入索。**任何经 Encryptor 落盘的字段永不进索引**——索引明文落盘，违者即泄密通道。
- ✅ 归档对话**仍可搜**（hit 带 `archived: true`，前端可滤）——归档+可搜正是归档的意义。

### 3.3 产品维度评估——本轮查漏结论

| 候选 | 结论 | 理由 |
|---|---|---|
| memory | **纳入**（12 类之一） | agent 记忆是高价值检索目标，文件态接入成本低 |
| 执行日志全文（executions / calls / firings / flowrun_nodes） | **明确排除，列为未来独立轴** | 体量无界（D1 永不删）、产品语义是「执行历史调查」非「找积木」；现有 4 个结构化过滤 tool 已覆盖主场景 |
| todo / attachment / notification | 暂缓 | todo 转瞬即逝；attachment 需文本抽取（PDF 等）是独立工程；notification 有自己的收件箱 UI |
| workspace / api_key / sandbox | 不索 | workspace 个位数靠 List；api_key 密文红线；sandbox 是 infra |
| LLM 搓工作流的结果形状 | **`search_workspace` 返回可直接接线的 ref**（§7.4） | 这是该 tool 的产品本质：搜到即可填进 workflow 节点 |
| 排序产品手感 | **exact-name 命中必须第一**（§6.3 boost 公式） | 搜「天气预报」时同名 function 排第一是底线直觉 |
| 索引时机 | message **完成时**即索（streaming 中不索）；标题改名、版本激活、文件写入均触发 | 搜索新鲜度秒级，列表过滤仍走实时 LIKE 不受影响 |

---

## 4. 架构落位（四层，依赖单向）

| 层 | 包 | 职责 |
|---|---|---|
| domain | `domain/search` | 纯类型：`EntityType`（12 值枚举）、`Hit` / `Chunk` / `Query` / `RetrieveOpts`、**`Notifier` 端口**（`Changed(ctx, EntityType, id, anchor)`，实体 Service 写后调用，nil 安全 no-op）、`SEARCH_*` sentinel（S20）。零外部 import。 |
| app | `app/search` | `Service`（Search / Retrieve / Reindex）+ `Indexer`（队列 + worker + 对账）+ **`Source` 端口**（拉取式：`Type()`、`Docs(ctx,id)`、`ListSince(ctx,since)`；conversation 额外实现 `DocAt(ctx,id,anchor)` 增量单 message）。**只依赖端口**，不 import 12 个实体包。 |
| app（各实体） | 12 个实体 app 包 | 各加一个 `searchsource.go` 实现 `Source`；Service 持可选 `searchdomain.Notifier`，Create/Update/Delete/Revert/message 完成时 `Changed(...)`。 |
| infra | `infra/search` | FTS5 物理层：DDL + raw SQL（MATCH / LIKE / snippet / 余弦）。**虚表不走 pkg/orm**——workspace 谓词手写，这是 D2 在 orm 自动隔离之外的唯一豁免点，`database.md` 登记 + 隔离测试钉死。 |
| transport | handlers + tool | `GET /api/v1/search`、`POST /api/v1/search:reindex`、`search_workspace` tool、8 个垂搜 tool 换引擎。 |

ID：投影行 `sd_<16hex>`（S15 登记进 `database.md` 前缀全集）。

---

## 5. 索引子系统

### 5.1 DDL（标准 FTS5 external-content 模式）

```sql
CREATE TABLE IF NOT EXISTS search_docs (
  id           TEXT PRIMARY KEY,            -- sd_<16hex>
  workspace_id TEXT NOT NULL,
  entity_type  TEXT NOT NULL CHECK (entity_type IN
    ('conversation','function','handler','agent','mcp','skill',
     'document','workflow','trigger','control','approval','memory')),
  entity_id    TEXT NOT NULL,
  chunk_no     INTEGER NOT NULL DEFAULT 0,
  anchor       TEXT NOT NULL DEFAULT '',    -- message_id / 方法名 / 标题链 / 节点 id
  title        TEXT NOT NULL,
  body         TEXT NOT NULL,
  tags         TEXT NOT NULL DEFAULT '[]',  -- 过滤 + 展示
  archived     INTEGER NOT NULL DEFAULT 0,  -- conversation 归档标记
  updated_at   DATETIME NOT NULL,
  UNIQUE(workspace_id, entity_type, entity_id, chunk_no)
);
CREATE INDEX IF NOT EXISTS idx_sd_ws_entity ON search_docs(workspace_id, entity_type, entity_id);

CREATE VIRTUAL TABLE IF NOT EXISTS search_fts USING fts5(
  title, body, content='search_docs', content_rowid='rowid', tokenize='trigram');

-- external-content 标准配套三触发器：投影表任何写法都不可能漏同步 FTS
CREATE TRIGGER IF NOT EXISTS search_docs_ai AFTER INSERT ON search_docs BEGIN
  INSERT INTO search_fts(rowid, title, body) VALUES (new.rowid, new.title, new.body);
END;
CREATE TRIGGER IF NOT EXISTS search_docs_ad AFTER DELETE ON search_docs BEGIN
  INSERT INTO search_fts(search_fts, rowid, title, body) VALUES ('delete', old.rowid, old.title, old.body);
END;
CREATE TRIGGER IF NOT EXISTS search_docs_au AFTER UPDATE ON search_docs BEGIN
  INSERT INTO search_fts(search_fts, rowid, title, body) VALUES ('delete', old.rowid, old.title, old.body);
  INSERT INTO search_fts(rowid, title, body) VALUES (new.rowid, new.title, new.body);
END;

CREATE TABLE IF NOT EXISTS search_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
-- 行：fts_schema_version = "1"（变更 → boot 全量重建）

-- M3 可选：
CREATE TABLE IF NOT EXISTS search_embeddings (
  doc_id TEXT PRIMARY KEY,                  -- = search_docs.id
  model  TEXT NOT NULL,
  dims   INTEGER NOT NULL,
  vector BLOB NOT NULL                      -- float32 小端序列
);
```

### 5.2 同步机制

1. **增量**：实体 Service 写成功（事务提交后）→ `Notifier.Changed(type, id, anchor)`（非阻塞投递，队列满则丢弃——boot 对账兜底）。**单 worker** 消费：经 `Source.Docs/DocAt` 重读实体 → 一个事务内 diff-upsert 该实体的投影行（多余 chunk 删、变更 chunk 改、新增 chunk 插）。conversation 用 `DocAt(id, messageID)` 单 message 增量，避免长会话 O(n²) 重索。
2. **删除**：实体软删/硬删 → `Changed` → `Docs` 返回空 → worker 删尽该 `entity_id` 全部行（+ embeddings）。**索引是派生数据，D1 不适用**。workspace 级联销毁（PD-1）顺带 `DELETE FROM search_docs WHERE workspace_id=?`。
3. **boot 对账**：起服时（detached context）逐 Source `ListSince(max(updated_at) per type)` 补漏 + 反向扫孤儿行删除；`search_meta.fts_schema_version` 不匹配 → 清空全量重建。
4. **手动重建**：`POST /api/v1/search:reindex`（202 Accepted，异步；进度走既有 notifications 流——**不加 SSE，E1 铁律**）。

### 5.3 体积与边界

- trigram 索引约为正文 3–5 倍；对话是大头，靠「只索 text 块」压制。单 chunk body 硬上限 8 KiB 字符（超长截断 + 续 chunk）。
- 分块 ~512 token（用 `pkg/tokencount` 估算）、10% 重叠、代码行对齐。
- 索引 worker 全程 detached context（S9：保 workspace 种子、脱请求取消）。

---

## 6. 查询子系统

### 6.1 管线

```
query → 清洗（去 FTS5 运算符，按空白切 token，每 token 双引号包裹防注入）
  → token 路由：
      长 token(≥3 字符) 存在 → FTS5 MATCH（隐式 AND）→ 候选集
                                短 token 作为 LIKE 谓词叠在 JOIN 上（候选集已窄，代价小）
      全部 token <3 字符     → 纯 LIKE 路径（title 优先，LIMIT 截断）
  → [语义开启且向量就绪] 余弦 top-100 ──┐
    BM25 top-100 ────────────────────┴─ RRF(k=60)
  → boost（§6.3）→ 同实体多 chunk 折叠（取最高分，附命中 chunk 数）
  → types/tags/时间/归档过滤 → 窗口分页 → snippet() 高亮片段
```

主查询形态（infra/search 内 raw SQL）：

```sql
SELECT d.id, d.entity_type, d.entity_id, d.chunk_no, d.anchor, d.title, d.tags, d.archived,
       snippet(search_fts, 1, '<mark>', '</mark>', '…', 16) AS snip,
       bm25(search_fts, 4.0, 1.0) AS score          -- title:body = 4:1，越负越相关，ASC
FROM search_fts f JOIN search_docs d ON d.rowid = f.rowid
WHERE search_fts MATCH ?1 AND d.workspace_id = ?2
  AND (?3 = '' OR d.entity_type IN (/* types */))
ORDER BY score LIMIT 100;
```

### 6.2 分页（融合结果上的 cursor）

RRF 融合分数跨页不稳定 → **物化窗口**：单次查询融合 top-200 为窗口，cursor = opaque base64 `{queryHash, offset}`；窗口内偏移翻页，越界返回空 `nextCursor`。产品上没人翻过 200 条；N4 形制不破。

### 6.3 排序（产品手感的硬规则）

```
final = rrf（或纯 BM25 归一）           — 基底
      + 3.0  × exact-name 命中          — title 与 query 全等（大小写不敏感）必须第一
      + 1.5  × name-prefix 命中
      + 0.3  × 实体类（fn/hd/ag/wf/mcp/skill/trigger/control/approval）
               对内容类（conversation/document/memory chunk）的 type boost
      tie → updated_at DESC → id
```

常量为初始值可调；测试断言**相对序**（exact > prefix > 正文命中）不断言绝对分。

---

## 7. API 与工具契约

### 7.1 `GET /api/v1/search`

参数：`q`（必填）、`types`（csv，空=综搜）、`tags`（csv）、`updatedAfter/updatedBefore`、`includeArchived`（默认 true）、`cursor`、`limit`（默认 20，上限 50）。

```jsonc
{ "data": {
    "hits": [{
      "entityType": "function", "entityId": "fn_a1b2…", "name": "天气预报",
      "snippet": "…查询<mark>天气</mark>并格式化…", "anchor": "",
      "tags": ["weather"], "archived": false, "score": 12.3,
      "matchedChunks": 3, "updatedAt": "2026-06-12T08:00:00Z",
      "refHint": "fn_a1b2…"                       // 按 ref 语法可直填 workflow 节点
    }],
    "nextCursor": "…", "total": 47
} }
```

### 7.2 `POST /api/v1/search:reindex`

202 Accepted（N2/N5）；运行中再触发 → 409 `SEARCH_REINDEX_RUNNING`；完成发 notifications 流通知。

### 7.3 错误码（S20，登记 error-codes.md）

| code | Kind |
|---|---|
| `SEARCH_QUERY_REQUIRED` | Invalid |
| `SEARCH_TYPE_INVALID` | Invalid |
| `SEARCH_CURSOR_INVALID` | Invalid |
| `SEARCH_REINDEX_RUNNING` | Conflict |
| `WORKSPACE_SEARCH_SEMANTIC_INVALID`（M3，workspace 域） | Invalid |

### 7.4 tool：`search_workspace`（LLM 搓工作流主入口）

- 入参：`query`（必填）+ `types`（可选）+ `limit`（默认 8）。
- 返回（面向接线，不止"找到了"）：每 hit 附 **`ref`**（fn → `fn_<id>`；hd → `hd_<id>.<method>` + `methods` 清单；mcp → `mcp:<serverId>/<tool>` + `tools` 清单；control/approval/trigger → 各自 id + kind），LLM 拿到即可填 workflow 节点 / 直接调用，无需二跳 `get_*`（要完整 schema 仍走 get）。
- S18 五方法接口；空 query 拒绝（`ValidateInput`）。

### 7.5 既有 tool 与 List

- 8 个 `search_<entity>` **保 schema 换引擎**：内部改调 `searchapp.Service.Search(types=[自身])`，对 agent 无感知；新增正文召回是纯增益。
- List `?q=` 保持 LIKE 不动——「边打边滤」的名字过滤与内容检索是两种产品行为，不混。
- `search_tools` 不并入（职责不同：工具发现，内存小全集）。

### 7.6 RAG 内部口

`Retrieve(ctx, q, RetrieveOpts{Types, TopK, MaxChars}) []Chunk{entityType, entityId, anchor, title, body, score}`——chunk 粒度不折叠，给 agent 上下文注入/知识挂载；M3 接通向量后此口自动 hybrid，调用方零改动。

---

## 8. 语义层（M3，可选增强，hybrid-ready）

- `domain/search.EmbeddingProvider` 端口：`Embed(ctx, texts []string) ([][]float32, error)`。
- 适配器：**Ollama**（`localhost:11434/api/embed`，推荐 `bge-m3`，1024 维中英双语；URL/模型名进 server 启动配置，非 workspace）。
- 开关：workspaces 表加 `search_semantic TEXT NOT NULL DEFAULT '' CHECK (search_semantic IN ('','off','ollama'))`，`''`→off——与 `web_fetch_mode` 同款裁决形态（显式选择、保守默认、读失败降级）。
- 嵌入由索引 worker 顺带批算；失败仅记日志、下次对账重试；**缺向量的行无声降级纯 BM25**（开关开着但 Ollama 没起，搜索照常工作）。
- 查询侧：ws 级向量内存缓存（懒加载、upsert/delete 失效；20k chunk × 4KB ≈ 80MB 上界，单活跃 ws 常驻可受）；暴力余弦 top-100 → RRF。
- 未来 BYOK 云端 embedding = 新适配器 + 枚举值，地基不动。

---

## 9. 测试与门禁

| 轴 | 钉死内容 |
|---|---|
| 中文/混排 | trigram 中文命中；中英混合 query；**2 字短词走 LIKE 回退命中**（探针已证盲区） |
| 隔离 | ws-A 索引内容 ws-B 搜不到（D2 豁免点的专项测试） |
| 一致性 | 触发器同步（直写投影表后 MATCH 即命中）；实体删/软删/级联后索引零残留；schema 版本变更全量重建 |
| 增量 | message 完成单条入索；版本激活重索；conversation 改名重索 title |
| 排序 | exact-name > prefix > 正文命中的相对序 |
| 注入 | query 含 FTS5 运算符（`" ( ) * OR NEAR`）不报语法错 |
| 降级 | 语义开但 Ollama 缺席 → 纯 BM25 正常返回 |
| 安全 | mcp config / trigger config / api key 内容在索引中 **0 命中**（红线测试） |
| 门禁 | `make verify` 全绿；索引 worker 并发路径 `-race`；fake_llm（T6）零 token |

---

## 10. 分期与工作量

| 期 | 内容 | 量级 |
|---|---|---|
| **M1 地基** | domain/app/infra 三层 + DDL/触发器 + 12 个 Source + Notifier 接线 + 队列/对账 + HTTP 两端点 + `search_workspace` + 8 tool 换引擎 + §9 测试 + 文档四件套（api/database/error-codes/domains）| ~3 天，发版即 12 类全量可搜 |
| **M2 RAG** | document/conversation 分块精修 + `Retrieve` 口 + agent 取数接入点 | ~1 天 |
| **M3 语义** | EmbeddingProvider + Ollama 适配 + embeddings 表 + RRF + workspace 配置 | ~1.5 天 |
| **M4 前端** | Cmd+K 综搜框 + 垂搜过滤 + anchor 跳转 + 重建索引按钮 | 随前端重建 |

## 11. 风险与边界

- **trigram <3 字符盲区**：LIKE 回退是硬要求（已入测试矩阵）。
- **索引体积**：正文 3–5 倍，对话靠 text-only 压制；上限可控（单用户本地）。
- **新鲜度**：异步秒级延迟；列表过滤走实时 LIKE 不受影响。
- **执行日志检索**是未来独立轴（体量无界、产品语义不同），本方案明确不背。
- **精度天花板**：trigram 召回宽、精度靠 BM25+boost；若实际使用中中文精度不满意，升级路径是叠加 gse 分词列（地基不变）。

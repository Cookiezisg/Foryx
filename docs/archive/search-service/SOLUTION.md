---
id: WRK-007
type: working
status: active
owner: @weilin
created: 2026-06-12
reviewed: 2026-06-12
review-due: 2026-09-12
expires: 2026-09-12
landed-into: "references/backend/domains/search.md"
audience: [human, ai]
---

# 统一搜索服务（BM25 + RAG）—— 现状与完整技术方案

> **状态：已实施落地（2026-06-12，M1+M2+M3 全量）。** 权威源已迁移：设计结论在 [`domains/search.md`](../../references/backend/domains/search.md) + `concepts/architecture.md` §3.5/§6/§8 + api/database/error-codes 四索引；本文档仅存档设计过程。

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
- **全部目标实体均 workspace 隔离**：DB 表带 `workspace_id`；skill/memory 文件态也按 `~/.anselm/workspaces/<wsID>/{skills,memories}/` 分桶。索引无需全局行特例。
- 相关表形态（节选）：trigger（`kind IN (cron,webhook,fsnotify,sensor)` + `config` JSON + `outputs` JSON，无版本表）；control（`control_logics` + `control_logic_versions.branches`(CEL) + `inputs`）；approval（`approval_forms` + `approval_form_versions.template/inputs/timeout`）；message_blocks（`type IN (text,reasoning,tool_call,tool_result,compaction,progress)`）。

### 1.3 缺口一句话

全系统只有「元数据子串匹配」：没有正文索引、没有相关性排序、没有跨实体统一入口、没有 RAG 取数口。

---

## 2. 方案总览

### 2.1 目标能力（四面一引擎）

1. **Tool call 检索**：现有 8 个垂搜 tool 保契约换引擎（LLM 仍可按域搜 document/workflow/trigger 等）；新增 **`search_blocks`（搜积木）**——**只搜能直接塞进 workflow 的可调用单元**（function / handler 的方法 / mcp 的工具 / agent / control / approval），LLM 搓工作流时心智负担最小化。**LLM 没有跨实体全域入口，对话内容不暴露给任何 LLM 工具**；全域综搜是人的入口（综搜框）。
2. **综搜**：`GET /api/v1/search?q=...`（前端 Cmd+K 全局搜索框）。
3. **垂搜**：同端点 `&types=function,document`。
4. **RAG 取数**：app 内部 `Retrieve(ctx, q, opts)`，返回 chunk 粒度 + 分数（agent 上下文注入 / 未来知识挂载）。

### 2.2 选型裁决（主流方案对照，全部已决）

| 维度 | 裁决 | 落选项与理由 |
|---|---|---|
| 词法检索 | **SQLite FTS5 + bm25()**（驱动内置，已实测） | Bleve：第二份索引文件 + 新依赖，FTS5 在场即重复轮子（原则 #8）；自研倒排：造轮子 |
| 中文分词 | **trigram**（免词典、中英文/代码统一子串语义）+ **短词 LIKE 回退** | gse（纯 Go jieba）：新依赖 + 词典常驻 ~50MB 内存，本地规模 trigram 召回已够；不满意时可日后叠加第二索引列，地基不变 |
| 向量存储 | **自有表 float32 BLOB + Go 内暴力余弦**（本地 ≤10 万 chunk 毫秒级，FAISS 在此规模也推荐 Flat）。**不抄 AnythingLLM 的向量库可换**（Chroma/Pinecone/Milvus）——那是服务端规模化的逃生门，单用户本地外接向量库只引入服务器与网络面；真要换只动 infra/search 一层 | sqlite-vec：C 扩展不可加载；chromem-go 等：新依赖且内部同样是暴力余弦 |
| Embedder | **内置默认开**（对标 AnythingLLM Native Embedder 的产品语义：零配置、拖入即向量化、全程透明）：directInstaller 首用下载 **llama-server 官方预编译二进制 + EmbeddingGemma-300m QAT GGUF**（量化后 <200MB RAM、100+ 语言含中文、MTEB 500M 以下开源第一），常驻子进程出本机 `/v1/embeddings`；**机器级配置可换** `builtin\|ollama\|off`；引擎缺席/下载中/失败**无声降级纯 BM25**（§8） | 照搬 AnythingLLM 技术栈（Node 进程内 ONNX Runtime）：纯 Go 无 CGO 进不了程内，且其默认 all-MiniLM-L6-v2 **主要是英文模型**、中文场景不合格；hugot/gomlx 纯 Go 进程内推理：experimental、~5x 慢、把 ML 框架拖进 go.mod——列为未来替换路径；云端 embedding API：违反约束（适配器位留好） |
| 检索模式 | **无配置、恒混合**：hybrid 是默认且唯一模式，向量未就绪自动退纯 BM25——降级自动化，不让用户选 | per-workspace 的 bm25/rag/hybrid 三选一开关：用户已裁决不做（自动降级优于配置化） |
| 混合融合 | **RRF（k=60）**——Elasticsearch/Weaviate/LanceDB 默认 | 线性加权：要调权、对分数尺度敏感 |
| 索引同步 | **写后 Enqueue + 单 worker 异步 + boot 对账**（索引=纯派生数据，永远可重建） | 事务内同步写：耦合业务事务、慢热路径；复用 entitystream：那是 SSE 直播帧原语非 CRUD 变更流 |

### 2.3 架构图

```
                ┌─ GET /api/v1/search?q=&types=        ← 综搜（types 空）/ 垂搜
三个出口         ├─ tool: search_blocks（搜积木，新）+ 8 个 search_<entity> 换引擎
                └─ app 内部 Retrieve(ctx, q, opts)      ← RAG 取数
                         │
                 app/search.Service
                 ├─ Search():  FTS5 BM25 ────────┐
                 │             向量(默认开,自动降级) ┴─ RRF → boost → 折叠 → 分页/snippet
                 ├─ Retrieve(): 同管线，chunk 粒度
                 └─ Indexer:   Enqueue 队列 + 单 worker + boot 对账 + 嵌入补算
                         │ DIP：12 个 SearchSource 端口（各实体 app Service 实现）
                         │      EmbeddingProvider 端口（builtin 子进程 / ollama）
                 infra/search
                 ├─ search_docs 投影表 + search_fts(FTS5 external-content) + 触发器
                 ├─ search_meta（fts schema 版本 + embedder 机器级配置）
                 ├─ search_embeddings（向量，模型/维度逐行记账）
                 └─ embedded engine：directInstaller 下发 llama-server + GGUF，常驻子进程（§8）
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
| `handler` | handlers + 活跃版本 | 名 + 描述 + tags | **类代码** + 方法签名 | 方法名 | **每方法一行**（anchor=方法名，body=签名+docstring+方法代码段，撑起 §7.4 按方法命中）+ 类级元数据一行 |
| `agent` | agents + 活跃版本 | 名 + 描述 + tags | prompt + 挂载（skill/知识/工具）说明 | — | 整体（超长再切） |
| `workflow` | workflows + 活跃版本 | 名 + 描述 | 图文本化：节点名 + ref + CEL 条件/映射表达式拼接 | 节点 id | 整体 |
| `mcp` | mcp_servers | server 名 | 工具名 + 工具描述（缓存的 tool list） | 工具名 | **每工具一行**（anchor=工具名，撑起 §7.4 按工具命中）+ server 级元数据一行 |
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
| LLM 工具的检索范围 | **`search_blocks` 只搜 workflow 节点面板**（fn/hd 方法/mcp 工具/ag/control/approval），返回可直接接线的 ref（§7.4）；**无跨实体全域工具、对话不暴露给任何 LLM 工具**（按域的 8 个垂搜 tool 保留） | 该 tool 的产品本质是搜积木：减轻 LLM 心智负担，搜到即可填进 workflow 节点；全域综搜是人的入口 |
| 排序产品手感 | **exact-name 命中必须第一**（§6.3 boost 公式） | 搜「天气预报」时同名 function 排第一是底线直觉 |
| 索引时机 | message **完成时**即索（streaming 中不索）；标题改名、版本激活、文件写入均触发 | 搜索新鲜度秒级，列表过滤仍走实时 LIKE 不受影响 |

---

## 4. 架构落位（四层，依赖单向）

| 层 | 包 | 职责 |
|---|---|---|
| domain | `domain/search` | 纯类型：`EntityType`（12 值枚举）、`Hit` / `Chunk` / `Query` / `RetrieveOpts`、**`Notifier` 端口**（`Changed(ctx, EntityType, id, anchor)`，实体 Service 写后调用，nil 安全 no-op）、**`EmbeddingProvider` 端口**（§8.2）、`SEARCH_*` sentinel（S20）。零外部 import。 |
| app | `app/search` | `Service`（Search / SearchBlocks / Retrieve / Reindex / Settings）+ `Indexer`（队列 + worker + 对账 + 嵌入补算）+ **`Source` 端口**（拉取式：`Type()`、`Docs(ctx,id)`、`ListSince(ctx,since)`；conversation 额外实现 `DocAt(ctx,id,anchor)` 增量单 message）。**只依赖端口**，不 import 12 个实体包；utility 精选经 ModelPicker（§7.4）。 |
| app（各实体） | 12 个实体 app 包 | 各加一个 `searchsource.go` 实现 `Source`；Service 持可选 `searchdomain.Notifier`，Create/Update/Delete/Revert/message 完成时 `Changed(...)`。 |
| infra | `infra/search` | FTS5 物理层：DDL + raw SQL（MATCH / LIKE / snippet / 余弦）+ **`engine` 子包**（builtin/ollama 两个 EmbeddingProvider 适配器、llama-server 子进程与安装，§8）。**虚表不走 pkg/orm**——workspace 谓词手写，这是 D2 在 orm 自动隔离之外的唯一豁免点，`database.md` 登记 + 隔离测试钉死。 |
| transport | handlers + tool | `GET /api/v1/search`、`POST /api/v1/search:reindex`、`GET/PATCH /api/v1/search/settings`、`search_blocks` tool、8 个垂搜 tool 换引擎。 |

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
  anchor       TEXT NOT NULL DEFAULT '',    -- message_id / 方法名 / 工具名 / 标题链 / 节点 id
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
--     embedder = builtin|ollama|off（机器级配置，缺省视为 builtin，§8）

-- 语义层（M2，默认在场）：
CREATE TABLE IF NOT EXISTS search_embeddings (
  doc_id TEXT PRIMARY KEY,                  -- = search_docs.id
  model  TEXT NOT NULL,                     -- 逐行记账：换 embedder 后旧向量直接失效可辨
  dims   INTEGER NOT NULL,
  vector BLOB NOT NULL                      -- float32 小端序列
);
```

### 5.2 同步机制

1. **增量**：实体 Service 写成功（事务提交后）→ `Notifier.Changed(type, id, anchor)`（非阻塞投递，队列满则丢弃——boot 对账兜底）。**单 worker** 消费：经 `Source.Docs/DocAt` 重读实体 → 一个事务内 diff-upsert 该实体的投影行（多余 chunk 删、变更 chunk 改、新增 chunk 插）。conversation 用 `DocAt(id, messageID)` 单 message 增量，避免长会话 O(n²) 重索。
2. **删除**：实体软删/硬删 → `Changed` → `Docs` 返回空 → worker 删尽该 `entity_id` 全部行（+ embeddings）。**索引是派生数据，D1 不适用**。workspace 级联销毁（PD-1）顺带 `DELETE FROM search_docs WHERE workspace_id=?`。
3. **boot 对账**：起服时（detached context）逐 Source `ListSince(max(updated_at) per type)` 补漏 + 反向扫孤儿行删除；`search_meta.fts_schema_version` 不匹配 → 清空全量重建；顺带扫缺当前模型向量的行入嵌入补算队列（引擎 absent 时触发安装重试，§8.3）。
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
  → [向量就绪] 余弦 top-100 ──────────┐   （引擎缺席/下载中/向量未追平 → 自动跳过，纯 BM25）
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
      "refHint": "fn_a1b2…"                       // 按 ref 语法可直填 workflow 节点；仅积木六类带，内容类命中为空
    }],
    "nextCursor": "…", "total": 47
} }
```

### 7.2 管理面：`POST /api/v1/search:reindex` 与 `GET/PATCH /api/v1/search/settings`

- `search:reindex`：202 Accepted（N2/N5）；运行中再触发 → 409 `SEARCH_REINDEX_RUNNING`；完成发 notifications 流通知。
- `search/settings`（**机器级**，存 `search_meta`——embedder 是装机资源、引擎与模型全 workspace 共用，不进 workspaces 表）：

```jsonc
{ "data": {
    "embedder": "builtin",                       // builtin | ollama | off，PATCH 可改
    "engine": { "status": "ready",               // ready | downloading | absent | error
                "model": "embeddinggemma-300m-qat-q8_0", "dims": 768 }
} }
```

PATCH 非法值 → `SEARCH_EMBEDDER_INVALID`；切换 embedder 后后台对当前模型缺向量的行重新补算（旧模型向量按 `model` 列识别、直接失效）。

### 7.3 错误码（S20，登记 error-codes.md）

| code | Kind |
|---|---|
| `SEARCH_QUERY_REQUIRED` | Invalid |
| `SEARCH_TYPE_INVALID` | Invalid |
| `SEARCH_CURSOR_INVALID` | Invalid |
| `SEARCH_REINDEX_RUNNING` | Conflict |
| `SEARCH_EMBEDDER_INVALID` | Invalid |

### 7.4 tool：`search_blocks`（搜积木——workflow 节点面板检索，LLM 专用）

**范围铁律**：只搜六类**可直接塞进 workflow 的积木**——function / handler / mcp / agent / control / approval。对话、document、skill、memory、workflow、trigger 一律不出现在结果里（搜索服务能搜 ≠ 该给 LLM 搜：全域检索是人的综搜框；trigger 不是图内节点、workflow 自身用 `search_workflow`）。目的只有一个：把 LLM 的心智负担压到「描述能力 → 拿到可接线的积木」。

- 入参：`query`（必填，描述需要的能力）+ `kinds`（可选，六类子集）+ `limit`（默认 8）。
- **命中粒度 = 可调用单元**，不是实体壳：handler 按**方法**出 hit（投影行 anchor=方法名）、mcp 按**工具**出 hit（anchor=工具名）——「搜'发邮件'命中 hd_x 的 sendMail 方法」而非「命中整个 handler」。同实体多方法命中时各自成行、不折叠（与综搜的实体折叠相反，这正是积木语义）。
- 返回（面向接线）：每 hit 附 **`ref`**（fn → `fn_<id>`；hd → `hd_<id>.<method>`；mcp → `mcp:<serverId>/<tool>`；ag/control/approval → 各自 id）+ name + 一句话描述 + 入参摘要（inputs 字段名），LLM 拿到即可填 workflow 节点，无需二跳 `get_*`（要完整 schema 仍走 get）。
- S18 五方法接口；空 query 拒绝（`ValidateInput`）。
- 实现即 `Service.Search` 的受限投影：`types` 钉死六类 + 折叠策略按 (entity_id, anchor)——同一引擎，无第二套索引。

**三段精度链**（`SearchBlocks` 内部策略，对调用方 LLM 完全透明——三档返回形状一致）：

```
目录 ≤ T token        → utility 模型全量精选（目录直喂，无损，最大精度）
目录 > T              → 索引检索 top-50 → utility 模型精选 top-N（token 有界，精度≈）
utility 不可用/失败    → 索引检索 top-N（不经精选，永远可用的兜底）
```

「索引检索」即 `Service.Search` 现役管线——M1 期纯 BM25，M2 起自动 hybrid，精度链本身不感知。

- 判据用**序列化 token 数**（`pkg/tokencount`）非条数：一条积木 ≈ 30–60 token，T 初值 **4k**（常量不做配置）——百条以内的目录全程处在无损直喂区，单用户本地绝大多数时间即最大精度；索引检索是规模兜底。
- 「目录」即 `search_docs` 六类积木行（entity+anchor 粒度）——投影表同时是索引源与目录源，无第二条数据路径。
- utility 调用复用 WebFetch 既有范式（ModelPicker 取 utility 模型，tool 内一次短补全，严格只回 ref 列表）；失败/未配置无声降级下一档（与全方案「降级自动化」同律）。
- 代价：前两档在 tool call 内嵌一次 utility 调用（~1–3s + utility token）——搓工作流找积木是低频高价值动作，换无损精度划算；测试走 fake_llm（T6）零消耗。
- 同一策略层后续可顺手赋能 8 个垂搜 tool（同一 Service 方法），不限于积木。

### 7.5 既有 tool 与 List

- 8 个 `search_<entity>` **保 schema 换引擎**：内部改调 `searchapp.Service.Search(types=[自身])`，对 agent 无感知；新增正文召回是纯增益。
- List `?q=` 保持 LIKE 不动——「边打边滤」的名字过滤与内容检索是两种产品行为，不混。
- `search_tools` 不并入（职责不同：工具发现，内存小全集）。

### 7.6 RAG 内部口

`Retrieve(ctx, q, RetrieveOpts{Types, TopK, MaxChars}) []Chunk{entityType, entityId, anchor, title, body, score}`——chunk 粒度不折叠，给 agent 上下文注入/知识挂载；向量默认在场（M2 起）即天然 hybrid，引擎缺席自动纯 BM25，调用方零改动零感知。

---

## 8. 语义层（M2，内置默认开——对标 AnythingLLM Native Embedder）

### 8.1 借鉴判定（调研结论）

AnythingLLM 桌面版的「核心秘密」= 安装包内封 ONNX Runtime + all-MiniLM-L6-v2（几十 MB、CPU 毫秒级），拖入文档自动向量化、全程透明，设置里可换 Ollama/HF/云端。**产品语义全盘借鉴，技术栈三处不照搬**：

1. **进程内 ONNX Runtime 进不来**：它是 Node/Electron 生态；Go 侧 ONNX Runtime 绑定要 CGO（地基铁律禁）。纯 Go 推理（hugot/gomlx simplego）2025 年才出、官方自称 experimental、~5x 慢、把整个 ML 框架拖进 go.mod——列为未来替换路径、暂不选。
2. **Anselm 的等价物是子进程**：directInstaller（decisions/0001：无内嵌、首用按需下）下发 **llama.cpp `llama-server` 官方预编译二进制**（macOS arm64/x64、Windows x64/arm64、Linux x64 全覆盖，MIT），以 `--embeddings` 常驻子进程暴露本机 OpenAI 兼容 `/v1/embeddings`——与 handler 常驻进程、MCP 子进程**同一套已有范式**，两块地基直接复用。
3. **默认模型不抄**：all-MiniLM-L6-v2 主要以英文训练（AnythingLLM 自己有 multilingual 的 FEAT issue 挂着），中文场景不合格。选 **EmbeddingGemma-300m QAT Q8 GGUF**（Google 2025-09，official ggml-org 仓库）：100+ 语言含中文、MTEB 500M 以下开源第一、量化后 **<200MB RAM**、768 维（Matryoshka 可截 512/256）、CPU 毫秒级。

### 8.2 设计

- **端口**：`domain/search.EmbeddingProvider`：`Embed(ctx, texts []string) ([][]float32, error)` + `Info() (model, dims)`。
- **适配器 builtin（默认）**：`infra/search/engine`——directInstaller recipe 下 `llama-server` 二进制 + EmbeddingGemma GGUF（共约 350MB，一次性，§8.3）；惰性启动（首次需要嵌入时 spawn）、随 app 优雅关停、crash 重启（复用常驻进程管理范式）；本机随机端口、仅监听 127.0.0.1。
- **适配器 ollama**：`localhost:11434/api/embed`，模型名经 settings 配置（默认 `embeddinggemma`）——给已有 Ollama 的用户复用其模型库。
- **配置**：机器级 `search_meta.embedder = builtin|ollama|off`（§7.2 settings 端点），默认 builtin。**检索模式无配置**：恒 hybrid，降级自动。
- **生命周期（全程透明，抄 AnythingLLM 的产品手感）**：嵌入由索引 worker 写完 FTS 后**异步批算**（批 ≤32 文本），永不阻塞索引与搜索；引擎 absent/downloading/error 期间查询自动纯 BM25，就绪后向量逐步追平、搜索无感变 hybrid；失败记日志、boot 对账重试。
- **查询侧**：ws 级向量内存缓存（懒加载、upsert/delete 失效；20k chunk × 768d×4B ≈ 60MB 上界，单活跃 ws 常驻可受）；暴力余弦 top-100 → RRF。
- **换模型/换 provider**：`search_embeddings.model` 逐行记账——切换后旧向量自动失效、后台重算，查询期间混存不混用。
- **未来扩展**：BYOK 云端 embedding = 新适配器 + settings 枚举值；外接向量库**明确不做**（§2.2）。

### 8.3 引擎下载与安装生命周期（复用 directInstaller，decisions/0001）

**何时下**——惰性、需求驱动，与 python/node 运行时同律（无内嵌、无预拉、首用按需下）：

- 触发点：`embedder=builtin`（默认）且引擎 absent 时**第一次产生嵌入需求**即触发——实践中就是 M2 上线后的首次启动（boot 对账发现 search_docs 有行缺当前模型向量 → 索引 worker 要嵌入 → 引擎 absent → detached goroutine 后台安装）。
- 三个不阻塞：不阻塞 boot、不阻塞索引（FTS 先行写完）、不阻塞搜索（纯 BM25 全程可用）。
- 失败语义：status=error（settings 可见原因）+ notifications 流一条；**不自动死循环重试**（离线环境不反复打网络）——下次 boot 对账重试，或 settings `PATCH embedder=builtin` 手动重触发。
- 防抖：索引 worker 单线程天然串行；settings 手动触发与 worker 之间加安装单飞锁。

**怎么下**——directInstaller 既有五步流水线（钉版本 → 平台选 asset → 流式下载 → 校验和 → staging 解压 + 原子 rename），新增两个 recipe：

| recipe | 来源（钉死版本） | 落点 | 体积 |
|---|---|---|---|
| `llamasrv` | llama.cpp 官方 release 固定 tag，按 GOOS/GOARCH 选 zip（macos-arm64/x64、win-x64/arm64、linux-x64）；sha256 钉死在 recipe（钉版本即钉 hash，与 uv sidecar 同级保证） | `<sandboxRoot>/runtimes/llamasrv/<tag>/`（llama-server + 动态库树） | ~几十 MB |
| `embedmodel` | EmbeddingGemma-300m QAT Q8 单 GGUF（ggml-org 官方仓库，HF LFS 自带 sha256）；**URL 主备链**：huggingface.co 主、hf-mirror.com 备（国内网络现实），逐个尝试 | `<sandboxRoot>/models/embedding/<name>/` | ~300 MB |

- 原子性/幂等：temp 下载 + staging 解压 + rename——目录要么完整要么不存在，崩溃残留可清，重入先 `Locate` 已装即跳过。
- 落账：`sandbox_runtimes` 既有表记 kind/version/path/size/installed_at；进度经既有 ProgressFunc → settings `engine.status=downloading`（含百分比），完成/失败发 notifications 流。
- 就绪后：进程**仍是惰性 spawn**（首次 `Embed` 调用才起 llama-server，127.0.0.1 随机端口）；`embedder=off` 只停进程不删文件。
- 隐私边界：下载是静态产物拉取（GitHub/HF），不携带任何用户数据——与 python/node 运行时下载同一信任面。

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
| 降级 | 引擎 absent/downloading/crash、Ollama 没起 → 纯 BM25 正常返回；引擎就绪向量追平后同一查询自动变 hybrid |
| 精度链 | 小目录走 utility 全量精选（fake_llm 断言直喂路径）；超阈值走检索+精选；utility 失败/未配置降纯索引检索——三档返回形状一致 |
| 安全 | mcp config / trigger config / api key 内容在索引中 **0 命中**（红线测试） |
| 门禁 | `make verify` 全绿；索引 worker 并发路径 `-race`；fake_llm（T6）零 token |

---

## 10. 分期与工作量

| 期 | 内容 | 量级 |
|---|---|---|
| **M1 地基（BM25 全量）** | domain/app/infra 三层 + DDL/触发器 + 12 个 Source + Notifier 接线 + 队列/对账 + HTTP 端点 + `search_blocks`（含三段精度链，utility 接线复用 WebFetch 范式）+ 8 tool 换引擎 + §9 测试 + 文档四件套（api/database/error-codes/domains）| ~3.5 天，发版即 12 类全量可搜（综搜面向人）、积木可搜（面向 LLM） |
| **M2 内置语义（默认混合）** | builtin 引擎 recipe（llama-server + EmbeddingGemma GGUF）+ 常驻进程管理 + EmbeddingProvider 双适配器 + embeddings 表/缓存 + RRF + settings 端点 + 降级链测试 | ~2.5 天 |
| **M3 RAG 精修** | document/conversation 分块精修 + `Retrieve` 口 + agent 取数接入点 | ~1 天 |
| **M4 前端** | Cmd+K 综搜框 + 垂搜过滤 + anchor 跳转 + 设置页（embedder/重建索引）| 随前端重建 |

## 11. 风险与边界

- **trigram <3 字符盲区**：LIKE 回退是硬要求（已入测试矩阵）。
- **索引体积**：正文 3–5 倍，对话靠 text-only 压制；上限可控（单用户本地）。
- **新鲜度**：异步秒级延迟；列表过滤走实时 LIKE 不受影响。
- **执行日志检索**是未来独立轴（体量无界、产品语义不同），本方案明确不背。
- **首启下载共约 350MB**（llama-server 二进制 ~几十 MB + GGUF ~300MB，§8.3）：与 directInstaller 既有体验一致（python/node 同样首用下）；下载失败/离线 → builtin 长期 absent，搜索以纯 BM25 持续可用，settings 可重试。
- **常驻子进程**：引擎 RSS <200MB、惰性启动；crash 重启与优雅关停复用 handler 常驻进程范式，不新发明生命周期。
- **精度天花板**：trigram 召回宽、词法精度靠 BM25+boost、语义召回靠默认混合；若中文词法精度仍不满意，升级路径是叠加 gse 分词列（地基不变）。

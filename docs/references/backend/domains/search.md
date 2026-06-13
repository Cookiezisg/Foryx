---
id: DOC-036
type: reference
status: active
owner: @weilin
created: 2026-06-12
reviewed: 2026-06-12
review-due: 2026-09-12
audience: [human, ai]
---

# search —— 统一搜索服务（BM25 + 语义混合）

> 一套索引、四个出口：人的综搜/垂搜（HTTP）、LLM 搜积木（`search_blocks`）、RAG 取数（`Retrieve`）。设计全文与裁决见 working 方案（落地后归档）；本文是代码的精确投影。

## 心智模型

- **索引 = 实体内容的纯投影**：12 类实体（conversation/function/handler/agent/mcp/skill/document/workflow/trigger/control/approval/memory）各自实现 `Source` 端口，把自己投影成 `search_docs` 行（title/body/anchor/chunk）。投影永远可重建——物理删、无软删、D1 不适用。
- **词法层**：SQLite FTS5（驱动内置）+ `trigram` 分词（中英文/代码统一子串语义）+ `bm25(title:body=4:1)`。trigram 对 <3 rune 的查询零命中（实测）→ **短词 LIKE 回退**；长短混合 token 时长 token 走 MATCH、短 token 以 LIKE 谓词叠加（隐式 AND）。
- **同步 = 写后通知 + 单 worker + 对账自愈**：实体 Service 写成功后调 `searchdomain.Notifier.Changed(type, id, anchor)`（非阻塞，队满即丢）；单 worker 在 detached ctx（S9）下重读实体并 diff 投影；boot 对账（stamps 比对 + 孤儿清理）是丢事件/崩溃/schema 重建背后的唯一自愈机制。conversation 走 `DocAt` 单 message 增量（anchor=message_id，chunk_no=块 seq——稳定键），避免长会话 O(n²) 重索。
- **排序**（§产品手感硬规则）：基底分归一到 [0,1] → exact-name +3.0 > name-prefix +1.5 > 正文命中，积木类对内容类 +0.3；tie → updated_at DESC → entity_id。测试只断言相对序。
- **分页**：融合分跨查询不稳定 → 物化 top-200 窗口，cursor = base64{queryHash, offset}；异查询 cursor 被 `SEARCH_CURSOR_INVALID` 拒绝而非切错窗口。
- **折叠**：综搜按实体折叠（最高分 chunk 胜出 + matchedChunks）；积木面板按 (entity, anchor)——每个 handler 方法 / mcp 工具本身就是结果单元。

## 代码层级

`domain/search`（类型 + `Notifier`/`EmbeddingProvider`/`Repository` 端口 + query 路由/分块纯函数 + 5 sentinel）→ `app/search`（`Service`：Search/SearchBlocks/Reindex/PurgeWorkspace + `Indexer`：队列/worker/对账；只依赖端口，不 import 实体包）→ `infra/search`（raw SQL 物理层——**D2 唯一豁免点**，见 [database.md](../database.md)）→ transport（`GET /search` + `POST /search:reindex`，见 [api.md](../api.md)）+ `app/tool/blocks`（`search_blocks`）。

四个出口：HTTP 综搜/垂搜（人）· `search_blocks`（LLM 积木面板：六类可接线单元、(entity,anchor) 粒度、ref 直填节点、无 ref 命中丢弃）· 8 个 `search_<entity>` 垂搜工具（保 schema 换引擎——全部渲染 `{count, list}` JSON：其中 7 个经 `toolapp.ContentSearch` 渲染共享 `searchdomain.EntitySlim`（`{id,name,description}`；trigger/workflow 内嵌它再加 kind·refCount·listening / lifecycleState·active）；`search_documents` 引擎同源、自有 JSON 渲染器（`{count, documents:[{id,name,path?,description?,snippet?}]}`，多带 path/snippet）。非空 query 走内容引擎、引擎缺席/出错回退原子串路径）· `Retrieve(ctx, q, RetrieveOpts{Types, TopK, MaxChars})` RAG 内部口（chunk 粒度不折叠、补全文 body、MaxChars 预算截断；与 Search 同一条混合管线。**当前零生产消费方**——为未来 agent 上下文注入/知识挂载预留的休眠口，单测覆盖管线、黑盒不可达，见 acceptance AC-25）。

**投影身份键**：各 Source 的 entity_id 用实体的**公开寻址键**——多数实体是行 id；skill 与 **mcp 用 name**（两者 HTTP 即按 name 寻址；mcp 曾按行 id 键控，refHint 发出 `mcp:msv_…/tool` 而挂载只解析 `mcp:<name>/<tool>`，可接线 ref 物理死亡，AC-27 修复）。

**search_blocks 三段精度链**（对调用方透明）：①目录序列化 ≤4k token（`pkg/tokencount`，常量非配置）→ **整目录直喂 utility 模型精选**（`Sifter` 端口，bootstrap 的 `llmSifter` 走 utility resolve→credentials→build→Generate 链，严格只回编号 JSON）；②超阈 → 索引检索 top-50 → utility 精选；③sifter 缺席/出错 → 纯索引排序。

## 语义层（默认混合）

- **EmbeddingProvider 双适配器**（`infra/search/engine`）：`Builtin`（默认）= directInstaller 首用经 sandbox `EnsureTool` 拉钉死的 llama-server 二进制（tag b9601，五平台 sha256 焙进 recipe）+ EmbeddingGemma-300m QAT Q8 GGUF（HF LFS sha256，hf-mirror 备链），常驻子进程出 127.0.0.1 OpenAI 兼容 `/v1/embeddings`——惰性安装、惰性 spawn、crash 重拉、Close 优雅停；`Ollama` = 本机 `/api/embed` 复用其模型库。
- **配置**：机器级 search_meta 三键——`embedder = builtin|ollama|off`（空=builtin）+ `ollama_base_url`/`ollama_model`（空=域默认 `127.0.0.1:11434`/`embeddinggemma`，权威在 `searchdomain.DefaultOllama*`），经 `GET/PATCH /search/settings`；Ollama 适配器由 bootstrap 注入工厂、参数变化即重建（app 不 import engine）；**检索模式无配置**——恒混合、降级自动。
- **补算**：独立 embed worker（与索引 worker 分离，下载/嵌入绝不阻塞 FTS）；索引写成功与 boot 对账后 kick；缺生效模型向量的行批 ≤32 补嵌（title+body，CapRunes）；provider 出错停本轮等下次 kick。
- **融合**：查询时 provider 在场且向量就绪 → 余弦 top-100 与词法 top-100 做 RRF(k=60)，纯向量命中补行后**重过查询过滤器**；任何失败原样返回词法列表。向量 ws 级内存缓存，upsert/purge/切换失效。
- **换 embedder**：`search_embeddings.model` 逐行记账——旧模型行对新模型即「缺向量」，自动重嵌，绝不混用。

## 关键不变量

1. `infra/search` 每条查询必须带显式 `workspace_id = ?` 谓词（隔离测试钉死）。
2. 密文红线：经 Encryptor 落盘的字段（api key 密文、mcp config、trigger config）**永不进投影**——索引明文落盘即泄密通道（红线测试）。
3. `fts_schema_version` 不匹配 → boot 清空全量重建——索引从不原地迁移。
4. 索引器永不阻塞业务写：Changed 非阻塞投递，溢出丢弃由对账兜底。

## 边界

- 执行日志（executions/calls/firings/flowrun_nodes）不入索——体量无界，是未来独立轴。
- `search_tools`（工具发现）独立小宇宙，不并入。
- List `?q=` 的 LIKE 名字过滤保持原样——「边打边滤」与内容检索是两种产品行为。

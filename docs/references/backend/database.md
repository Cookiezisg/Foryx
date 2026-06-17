---
id: DOC-009
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-14
review-due: 2026-09-14
audience: [human, ai]
---

# 数据库 —— 表 / ID 前缀登记

> 物理 schema 的单一事实源（表 · 关键列 · 索引/约束 · ID 前缀），覆盖全部 32 域。DDL 全文在各 `infra/store/<域>` 的 `Schema`（搜索域在 `infra/search`），幂等 `CREATE IF NOT EXISTS`，启动时 `db.Migrate` 单事务应用。
> 通则（D 系列）：业务表软删 `deleted_at`；Log 表（executions/calls）**只增不删**（D1）；全表带 `workspace_id`（orm 据 ctx 自动隔离，D2）；name 用 partial-UNIQUE `WHERE deleted_at IS NULL`（软删释放名字）；版本表 `UNIQUE(<entity>_id, version)`。
> **时间戳约定**：实体表与版本表统一带 `created_at` + `updated_at`（orm `,created`/`,updated` tag 自动戳，写时刷新 updated_at）；**Log 表（executions/calls/activations/notifications 等只增审计行）只带 `created_at`**——行写一次不改，updated_at 无意义（D1）。下方各表列清单省略这套标准时间戳，不逐张列。

## 三实体共同形状

每实体三张表：**主表**（身份 + `active_version_id` 指针，软删）· **版本表**（不可变快照，只增，cap 50 裁剪但放过 active）· **执行/调用 Log 表**（终态审计，只增）。Log 表统一溯源列：`conversation_id / message_id / tool_call_id`（chat 路径，ctx 注入）+ `flowrun_id / flowrun_node_id`（workflow 路径，调度器 ctx 注入）；CHECK 约束 `status IN (ok,failed,cancelled,timeout)` + `triggered_by`。

## function

| 表 | 关键列 | 约束/索引 |
|---|---|---|
| `functions` | name · description · tags(json) · active_version_id | partial-UNIQUE(ws,name)；ws+created 游标索引 |
| `function_versions` | version(int) · code · inputs/outputs(json) · dependencies(json) · python_version · **env_id/env_status/env_error/env_synced_at**（env 镜像）· change_reason · built_in_conversation_id | UNIQUE(function_id,version) |
| `function_executions` | version_id · status · triggered_by(chat/agent/workflow/manual) · input/output(json) · error_message · **logs**（print/调试输出，logtail 头尾限长 64KiB；List 置空、仅单条 Get 携带） · elapsed_ms · started/ended_at · 溯源 5 列 | CHECK ×2；ws+function / ws+conversation / ws+flowrun 偏索引 |

ID：`fn_` `fnv_` `fne_` · env：`fnenv_`（infra 侧自有前缀）

## handler

| 表 | 关键列 | 约束/索引 |
|---|---|---|
| `handlers` | （同上）+ **config_encrypted**（init-args 值，AES-GCM 加密存盘） | 同上 |
| `handler_versions` | version · **imports / init_body / shutdown_body / methods(json MethodSpec[]) / init_args_schema(json InitArgSpec[])** · dependencies · python_version · env 镜像 4 列 | UNIQUE(handler_id,version) |
| `handler_calls` | method · status · triggered_by(含 agent) · input/output · **logs**（yield + 调用窗口内 stderr，logtail 限长；List 置空） · **instance_id** · 溯源 5 列 | 同款 CHECK + 索引 |

ID：`hd_` `hdv_` `hcl_` · env：`hdenv_` · 实例（内存态，不落库）：`hdi_`

## agent

| 表 | 关键列 | 约束/索引 |
|---|---|---|
| `agents` | （同上，无 config） | 同上 |
| `agent_versions` | version · **prompt · skill(0-1 名) · knowledge(json docIDs) · tools(json ToolRef[]) · inputs/outputs(json) · model_override(json)** · change_reason · built_in_conversation_id | UNIQUE(agent_id,version) |
| `agent_executions` | model_id（实际跑的模型）· status · triggered_by(chat/workflow/manual，**无 agent**——员工不调员工) · input/output · **transcript(json，完整 block 序列——运行的自包含耐久记录，不入 message_blocks)** · 溯源 5 列 | 同款 |

ID：`ag_` `agv_` `agx_`（agent 无 env——不写代码无 sandbox）

## workflow / control / approval（同构对：主表 + 版本表）

| 表 | 特有列 | 约束 |
|---|---|---|
| `workflows` | active(bool) · **lifecycle_state**(CHECK active/draining/inactive) · **concurrency**(CHECK serial/skip/buffer_one/buffer_all/allow_all，DEFAULT serial) · needs_attention/attention_reason/last_action_by | partial-UNIQUE(ws,name) |
| `workflow_versions` | **graph**(JSON blob：nodes+edges) | UNIQUE(workflow_id,version) |
| `control_logics` / `control_logic_versions` | versions：inputs(json) · **branches**(json Branch[]：port/when/emit) | 同构 |
| `approval_forms` / `approval_form_versions` | versions：inputs · **template**(markdown+{{CEL}}) · allow_reason(bool：是否允许填备注) · timeout("30d"/"2w"/""=永不) · timeout_behavior(reject/approve/fail) | 同构 |

ID：`wf_`/`wfv_` · `ctl_`/`ctlv_` · `apf_`/`apfv_`

## trigger（配置实体 + 两张 Log）

| 表 | 关键列 | 约束/索引 |
|---|---|---|
| `triggers` | kind(CHECK cron/webhook/fsnotify/sensor) · **config**(自由 json map) · outputs(json) | partial-UNIQUE(ws,name) |
| `trigger_activations`（Log） | kind · fired(bool) · return_value/payload(json) · error · detail · firing_count | 按 trigger+created 索引 |
| `trigger_firings`（Log，durable 收件箱） | trigger_id · workflow_id · activation_id · payload(json) · **dedup_key** · status(pending/claimed/started/skipped/superseded/shed) · flowrun_id | **`idx_trf_dedup` UNIQUE(workflow_id,trigger_id,dedup_key)**（D3）+ pending 偏索引 |

ID：`trg_`/`tra_`/`trf_`

## flowrun（两张 Log——引擎的全部状态）

| 表 | 关键列 | 约束/索引 |
|---|---|---|
| `flowruns` | workflow_id · **version_id**(钉死拓扑) · **pinned_refs**(json pin 闭包) · trigger_id/firing_id · status(CHECK running/completed/failed/cancelled) · replay_count · error | running 偏索引（跨 ws boot 恢复）|
| `flowrun_nodes` | flowrun_id · **node_id**(图内名) · **iteration**(循环轮次) · kind · ref · status(CHECK completed/failed/parked) · **result**(json 记忆化) · error | **`idx_frn_once` UNIQUE(flowrun_id,node_id,iteration)**（D3 record-once）+ parked 偏索引（收件箱）|

ID：`fr_`/`frn_`。两张无 deleted_at（D1）；唯一物理删 = `:replay` 清 failed 行（非结果）。

## skill / mcp / document

| 表 | 关键列 | 说明 |
|---|---|---|
| **skill：无表** | — | 文件式：`~/.anselm/workspaces/<ws>/skills/<name>/SKILL.md`（目录/条，纯按需扫描） |
| `mcp_servers` | transport(stdio/sse/streamable-http) · runtime(node/python/docker/dotnet) · command/args · url · **config_enc**（加密的 {env,headers}，Env/Headers 非列）· timeout_sec · source(registry/manual/import) · registry_id | 软删；partial-UNIQUE(ws,name) |
| `mcp_calls`（Log） | server_id · tool · status/triggered_by(CHECK) · input/output · **logs**（progress 通知 + 失败附 server stderr 尾，logtail 限长；List 置空） · elapsed_ms · 溯源 5 列（含 flowrun 2 列）| ws+server 索引 + flowrun 偏索引 |
| `documents` | parent_id(nullable=根) · name · content · **path**(物化全路径) · **position**(同级序) · size_bytes · tags | 软删；同父名唯一（应用层重试加后缀） |

ID：`mcp_`/`mcl_` · `doc_`（skill 无 id——slug 即身份）

## 对话运行时族

| 表 | 关键列 | 说明 |
|---|---|---|
| `conversations` | title · auto_titled · system_prompt · **summary / summary_covers_up_to_seq**（压缩器写）· attached_documents(json) · archived/pinned · model_override(json) · **last_message_at**（最近活跃排序键：创建时=now、chat 每用户回合刷；列表索引 `(ws, pinned DESC, last_message_at DESC, id DESC)` + keyset 游标键此列） | 软删；`cv_` |
| `messages` | conversation_id · **subagent_id**（≠'' = subagent 产出）· role/status(CHECK) · stop_reason · error_code/message · input/output_tokens · provider/model_id（溯源）· attrs(json：附件/提及快照) | **append-only**（D1）；`msg_` |
| `message_blocks` | message_id · parent_block_id · **seq**（落盘分配）· type(CHECK 六型含 progress/compaction) · attrs/content · status · **context_role**(CHECK hot/warm/cold/archived——压缩投影) | append-only；`blk_` |
| `attachments` | sha256(内容寻址，非唯一) · filename · mime_type · kind(image/document/text/audio/video/other) · size_bytes · blob 字节在 infra/fs/blob 按 sha256 寻址 | 软删；`att_`；≤50MB |
| `todos` | **`scope_id`**(pk = subagent id ?? conv id) · conversation_id · subagent_id · items(json ≤64) | 整表替换写 |
| **memory / subagent：无表** | — | memory=文件式（`workspaces/<ws>/memories/<name>.md`）；subagent=运行时机制（回合落父对话 messages） |

## search（统一搜索索引——派生数据）

| 表 | 关键列 | 约束/索引 |
|---|---|---|
| `search_docs` | workspace_id · entity_type(CHECK 12 类) · entity_id · chunk_no · anchor（message_id/方法名/工具名/标题链/节点 id）· title · body · tags(json) · archived | UNIQUE(ws,entity_type,entity_id,chunk_no)；ws+entity 索引 |
| `search_fts` | FTS5 **external-content 虚表**（content=search_docs，`tokenize='trigram'`，title/body 两列）+ 三触发器（AI/AD/AU）构造性同步 | bm25 权重 title:body=4:1 在查询侧 |
| `search_meta` | key/value：`fts_schema_version`（不匹配→boot 清空重建）· `embedder`（builtin\|ollama\|off，空=builtin，机器级） | PK(key) |
| `search_embeddings` | doc_id(=search_docs.id) · model · dims · vector(BLOB float32 LE)——model 逐行记账，换 embedder 旧向量直接可辨失效 | PK(doc_id) |

ID：`sd_`。**派生数据**：物理删（实体删/级联/重建即删行），D1 不适用、无软删。**D2 豁免点（全库唯一）**：FTS5 虚表在 pkg/orm 之外，`infra/search` 手写 raw SQL——每条查询显式 `workspace_id = ?` 谓词，隔离由专项测试钉死（`infra/search/search_test.go::TestSearch_WorkspaceIsolation`）。

## 支撑域

| 表 | 说明 |
|---|---|
| `workspaces` | **全局表（无 ws 列——它即 workspace）**；语言/三场景模型默认/默认搜索 key/`web_fetch_mode`（local\|jina，CHECK，空=local）；`ws_` |
| `api_keys` | 密文整列加密；probe 归档；软删；`aki_` |
| `relations` | from/to (kind,id) × edge kind；硬删（PurgeEntity 级联）；`rel_` |
| `notifications` | type(`<domain>.<action>`) · payload · read_at；`noti_` |
| `sandbox_runtimes` | runtime manifest（全机解释器/镜像：kind+version · path · size_bytes，`UNIQUE(kind,version)`）——**系统级**（无 ws 列、owner 无关）、**硬删**（盘上镜像是实体，墓碑无意义）；`sr_` |
| `sandbox_envs` | env manifest（owner kind+id · runtime · status）——**系统级**（owner-id 全局隔离、无 ws 列）、**硬删**（盘上目录是实体，墓碑无意义）；`se_` |
| catalog / mention / model / websearch / aispawn / humanloop / contextmgr / entitystream：**无表** | 派生/契约/运行时机制 |

> **运行时/infra ID 前缀（无表，S15 仍登记）**：`sig_`（entitystream 信号帧 id）· `bsh_`（shell 工具的 bash 进程句柄）· `subagt_`（subagent run id）· `hdi_`（handler 实例，见 handler 节）。infra 侧 ID 一律用自己前缀、不从消费实体 id 派生。

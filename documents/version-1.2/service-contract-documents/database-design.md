# Database Design — V1.2 表一眼索引

**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../service-design-documents/`](../service-design-documents/) — 每个 domain 的详设计（含完整 struct + 业务规则）

**定位**：**一眼看到全仓有哪些表 + 关键约束**。struct / 索引细节 / CHECK 约束原文、schema_extras 补丁，**去 service-design-documents 看**。

**遵守标准**：D1（软删 deleted_at）/ D2（时间戳）/ D3（枚举 CHECK）/ D4（FK + `PRAGMA foreign_keys=on`）/ D5（业务 UNIQUE）

---

## 全局约定

### 数据库
- **SQLite**（本地）+ GORM
- 驱动：`modernc.org/sqlite`（纯 Go，无 CGO，2026-05-01 从 `mattn/go-sqlite3` 迁移）→ 经 `github.com/glebarez/sqlite` 接入 GORM
- WAL、FK、PrepareStmt、UTC 全开（见 `infra/db/db.go`）；DSN PRAGMA 用 modernc 语法 `_pragma=journal_mode(WAL)` 等

### 类型策略
- **一份到底**：domain 类型直接带 GORM tag，不分两套
- **DB 列名**：`snake_case`
- **主键**：文本 ID（带 domain 前缀，如 `aki_<16hex>`、`mc_<16hex>`）

### 时间戳 + 软删除（D1 + D2）
每表标配：
```go
CreatedAt time.Time      // GORM 自动
UpdatedAt time.Time      // GORM 自动
DeletedAt gorm.DeletedAt // 软删（写入 deleted_at 列）
```
废弃 `status='deleted'` / `archived_at` 等变体。

### 枚举（D3）
- **稳定白名单**（`role`、`content_type`、`test_status` 等）在 DB 层 CHECK
- **会随 Phase 扩张的白名单**（如 `scenario`）在 app 层校验，DB 不 CHECK

### 高级 schema（`infra/db/schema_extras.go`）
GORM tag 表达不了的 SQL 都在这里：
- 部分 UNIQUE 索引（`WHERE deleted_at IS NULL`，例如 functions `UNIQUE(user_id, name)`）
- 触发器
- FTS5 虚拟表（**当前未使用**——chat 重构 2026-04-27 时移除了原基于 messages.content 的 FTS5；modernc 驱动内置 FTS5，未来按 message_blocks.data 重建时无需编译标志）

---

## 表清单

> **状态**：⬜ 未设计 | 🔄 讨论中 | ✅ 已实现

### Phase 2

#### `users` ✅（V1.2 §20 multi-user）
详见 [`../service-design-documents/user.md`](../service-design-documents/user.md) §8。
主键文本（默认 user 固定 `"local-user"` 匹配 `reqctxpkg.DefaultLocalUserID`；新建用户 `u_<16hex>`）；UNIQUE `username`；软删。字段：`username`（1-32 [a-z0-9_-]，强制 lowercase）/ `display_name` / `avatar_color`（hex #4f46e5）/ **`language TEXT default 'zh-CN' CHECK(language IN ('zh-CN','en'))`**（§21 i18n）/ `last_used_at`（activate 时刷新；UserPicker 高亮"最近用"）/ 时间戳。**唯一不按 user_id scope 的表**——它自己就是身份。`EnsureDefault` 启动时空表创默认 user 让现有数据自然 surface（DB 已 user_id-scoped）。

#### `api_keys` ✅
详见 [`../service-design-documents/apikey.md`](../service-design-documents/apikey.md) §11。
主键 `aki_<16hex>`；软删（`DeletedAt`）；全索引 `(user_id)` + `(user_id, provider)` + `(deleted_at)`（目前未走部分索引 `WHERE deleted_at IS NULL`，见 backlog）。**注**：`idx_api_keys_user_id` 单列与复合索引前导列重合可视为冗余，保留是为匹配 List filter 写法清晰度（单用户本地写入罕见，冗余成本可忽略）。敏感字段 `key_encrypted`（AES-GCM `v1:` 前缀，`json:"-"` 守护永不上线）+ `key_masked` 冗余展示。**feature 列**：`display_name`（UI 展示用）/ `base_url`（ollama / custom 必填）/ `api_format`（custom 必填，openai-compatible / anthropic-compatible 二选一）/ `test_status`（pending / ok / error）/ `test_error`（连通性测试失败原因）/ `last_tested_at`（最近一次测试时间，nullable）/ `models_found TEXT`（GORM `serializer:json`，存 JSON 字符串如 `["deepseek-chat","deepseek-reasoner"]`；测试成功后由 `UpdateTestResult` 写入，测试前为 `[]`）。不加 `UNIQUE(user_id, provider)`，允许同 provider 多 key。Provider / TestStatus 的 DB 层 CHECK 约束**未加**，由 app 层校验。

#### `model_configs` ✅
详见 [`../service-design-documents/model.md`](../service-design-documents/model.md) §11。
主键 `mc_<16hex>`；软删（`deleted_at`）；GORM 全唯一索引 `UNIQUE(user_id, scenario)`（partial UNIQUE 暂缓，见 §17 决定）。Scenario 白名单 app 层校验，DB 不 CHECK。

#### `conversations` ✅
详见 [`../service-design-documents/conversation.md`](../service-design-documents/conversation.md) §8。
主键 `cv_<16hex>`；软删（`deleted_at`）；`user_id` 索引 + **`archived` 索引（§17.12，2026-05-17）**。字段：`system_prompt TEXT`（对话级自定义系统提示词，可为空）/ `auto_titled BOOLEAN`（标记标题是 AI 自动生成的还是用户手动改的）/ **`summary TEXT default ''`**（V1.2 §1 final-sweep；compaction running summary，anchored-append。空 = 尚未发生压缩）/ **`summary_covers_up_to_seq INT64 default 0`**（V1.2 §1；该 summary 覆盖到哪个 block seq；下次压缩从此 seq+1 起算）/ **`attached_documents TEXT default '[]'`**（Phase 5 §14.5c；GORM serializer:json，挂载文档 ID + includeSubtree refs）/ **`archived BOOLEAN default false`**（§17.12，2026-05-17；归档隐藏；List 默认 `WHERE archived = false`，UI 切到归档面板用 `?archived=true`）/ **`pinned BOOLEAN default false`**（§15.6，2026-05-17；List `ORDER BY pinned DESC, created_at DESC, id DESC`；testend sidebar 客户端同 sort + 📌 标识）/ **`model_override TEXT`**（§12.3，2026-05-17；GORM serializer:json，存 `{provider, modelId}`；nil = 用全局 chat scenario，`llmclient.ResolveWithOverride` 优先 conv override；PATCH 时 422 PROVIDER_HAS_NO_KEY 校验）。Title 允许空字符串，首轮完成后 auto-titling goroutine 回写。

#### `messages` ✅（chat infra 重构后精简；Phase 5 加错误信息字段；schema 统一后加 subagent 嵌套字段）
chat domain 所有；主键 `msg_<16hex>`；字段：`conversation_id`（索引）/ `user_id` / `role`（**user\|assistant**，`tool` 角色已移除——tool result 变为 message_blocks 的 block）/ `status`（pending\|streaming\|completed\|error\|cancelled）/ `stop_reason` / **`error_code`**（status="error" 时填，如 `LLM_STREAM_ERROR` / `HISTORY_EXTEND_FAILED` / `MODEL_NOT_CONFIGURED` / `INTERNAL_ERROR` 等；其他 status 时为空）/ **`error_message`**（结构化错误原因人类可读文本）/ `input_tokens INT` / `output_tokens INT` / **`parent_block_id TEXT`**（嵌套 subagent run 时指向父 message-block 占位；非嵌套时空）/ **`attrs TEXT`**（JSON 自由形状——user message 含 `attachments[]` 引用；subagent sub-message 含 `kind=subagent_run` + type / runId / maxTurns）/ `created_at` / **`updated_at`**（GORM 自动维护）/ 软删 `deleted_at`。**内容字段已移除**：`content`、`reasoning_content`、`tool_calls`、`tool_call_id`、`attachment_ids`、`token_usage` 全部转移到 `message_blocks` 表。FTS5 已移除（原基于 `content` 列，后续按 message_blocks 重建）。

#### `message_blocks` ✅（事件日志协议 + 递归嵌套，2026-05 schema 统一）
chat domain 所有；主键 `blk_<16hex>` 或 LLM 自带 `tc_<id>`（tool_call 复用 LLM 给的 ID，方便 tool_result 用 parent_block_id 直挂）。

**字段**：
- `conversation_id TEXT NOT NULL` — UNIQUE(conv_id, seq) 复合索引第 1 列；replay 查询不需 join messages
- `message_id TEXT NOT NULL idx` — 顶层归属的 message
- `parent_block_id TEXT NULL idx` — 嵌套指针（如 progress 挂 tool_call 下）；顶层 block 此列空
- `seq INT64 NOT NULL` — UNIQUE 复合第 2 列；**per-conversation 全局**单调（Bridge 分配，非 per-message）
- `type TEXT NOT NULL CHECK(type IN ('text','reasoning','tool_call','tool_result','progress','message','compaction'))` — **7 值封闭枚举**（V1.2 §1 final-sweep 新增 `compaction`）
- `attrs TEXT` — JSON 元数据（如 `{tool: name}` / `{stage: installing}` / `{messageId: subId}` / compaction 块用 `{coversFromSeq, coversToSeq, blocksArchived, generatedBy}`）
- `content TEXT NOT NULL DEFAULT ''` — append-only 流式正文（DeltaBlock SQL `content || ?` 原子拼）
- `status TEXT NOT NULL CHECK(status IN ('streaming','completed','error','cancelled'))` — 终态机
- `error TEXT` — block_stop 时填（status='error' 时）
- **`context_role TEXT NOT NULL DEFAULT 'hot' CHECK(context_role IN ('hot','warm','cold','archived'))`**（V1.2 §1 final-sweep）—— 投影角色，由 app/contextmgr 维护。hot=完整 / warm=200B preview / cold=元数据占位 / archived=完全跳过（已被 conversation.summary 覆盖）。**DB Content 永不改写**，只 context_role 一字段移动。
- `created_at` / `updated_at` — GORM 自动维护

索引：UNIQUE(conv_id, seq) + (message_id) + (parent_block_id) + `idx_mb_conv_role (conversation_id, context_role)`（V1.2 §1，给 contextmgr 扫描 archived 候选用）。**无软删**——历史 block 不会被改名/删除。完整设计 [`../event-log-protocol.md`](../event-log-protocol.md) §6。

#### `attachments` ✅（Phase 5 重命名 chat_attachments → attachments + 加软删）
chat domain 所有；主键 `att_<16hex>`；字段：`user_id`（索引）/ `file_name` / `mime_type` / `size_bytes` / `storage_path`（相对 dataDir，json:"-" 不对外暴露）/ `created_at` / **`updated_at`** / **`deleted_at`** GORM 软删。文件实体存 `{dataDir}/attachments/{att_id}/original.{ext}`，50MB 限制。**支持软删**——用户删附件后旧对话仍能解引用（不再硬删导致 dangling reference）。

---

### Phase 3 — function trinity (forge_redesign Plan 01)

#### `functions` ✅
详见 [`../service-design-documents/function.md`](../service-design-documents/function.md) §5.1 + redesign topic [`02-function.md`](../adhoc-topic-documents/forge_redesign/02-function.md) §5.1。
主键 `fn_<16hex>`；软删（`deleted_at`）；`user_id` 索引；partial UNIQUE `UNIQUE(user_id, name) WHERE deleted_at IS NULL`（在 `schema_extras.go`）。
字段：`name` / `description` / `tags`（JSON 数组,serializer:json）/ **`active_version_id`**（指向当前活跃 FunctionVersion.ID；草稿期空字符串）/ `created_at` / `updated_at` / `deleted_at`。
**计算字段（非列,`gorm:"-"`）**：`Pending *Version` 由 service 层 `attachComputed` 填；`EnvStatus` / `EnvError` / `EnvSyncedAt` / `EnvSyncStage` / `EnvSyncDetail` 镜像 active version 的环境字段（ActiveVersionID=="" 时全空）。`function` notification 载荷依赖这些计算字段。
function 搜索通过 LLM 排序实现（SearchFunction 把全量发给 LLM），无独立向量索引。

#### `function_versions` ✅
详见 [`../service-design-documents/function.md`](../service-design-documents/function.md) §5.2 + redesign topic [`02-function.md`](../adhoc-topic-documents/forge_redesign/02-function.md) §5.2。
主键 `fnv_<16hex>`；**兼作 pending 变更存储**：`status` 字段（DB CHECK `IN ('pending','accepted','rejected')`）区分,pending/rejected 时 `version` 为 NULL。
完整快照字段：`function_id`（索引）/ `code` / `parameters`（JSON 数组,serializer:json）/ `return_schema`（JSON 对象,serializer:json）/ **`change_reason`** / `created_at` / `updated_at`。
**Sandbox 字段**：
- **`dependencies`** TEXT default `'[]'`（PEP 508 specifier JSON,serializer:json,由 LLM 在 create_function / edit_function 时申报）
- **`python_version`** TEXT default `''`（PEP 440 spec；空回退到 `functiondomain.DefaultPythonVersion=">=3.12"`）
- **`env_id`** TEXT 索引（`fnenv_<16hex>`,每 Version 行独立生成,跟 version_id 1:1 但**解耦**;D-redo-8 每版本独立 venv,sandbox 跨域共用时各家命名空间隔离。详 [`../adhoc-topic-documents/forge_redesign/discussions/2026-05-12-env-and-sse-rework.md`](../adhoc-topic-documents/forge_redesign/discussions/2026-05-12-env-and-sse-rework.md) §D）
- **环境运行时状态**：`env_status` TEXT default `'pending'`（5 值：`pending`/`syncing`/`ready`/`failed`/`evicted`,白名单 service 层校验）/ `env_error` TEXT / `env_synced_at` DATETIME / `env_sync_stage` TEXT（含 `fixing` 表示 LLM env-fix loop 中）/ `env_sync_detail` TEXT

每 Version 独立 venv(D-redo-8),env_status 字段对当前 version 状态零歧义。env 装配同步发生在 `create_function` / `edit_function` 工具内(D-redo-9),失败由 LLM tool 走内部 env-fix loop(maxAttempts=3,详 [`../adhoc-topic-documents/forge_redesign/02-function.md`](../adhoc-topic-documents/forge_redesign/02-function.md) §10)。accepted 版本上限 `functiondomain.AcceptedVersionCap = 50`,超限硬删最旧。

#### `function_executions` ✅ (D22)
详见 [`../service-design-documents/function.md`](../service-design-documents/function.md) §5.3 + redesign topic [`08-executions.md`](../adhoc-topic-documents/forge_redesign/08-executions.md) §4.1。
主键 `fne_<16hex>`；软删（`deleted_at`）；`user_id` 索引。每次 `Service.RunFunction` 终态写一行（detached ctx §S9）。
**通用 16 字段**（per spec/08-executions.md §2,5 个 per-entity 表共享 schema）：`status`（CHECK `IN ('ok','failed','cancelled','timeout')`）/ `triggered_by`（CHECK `IN ('chat','workflow','http','test')`）/ `input` JSON / `output` JSON / `error_code` / `error_message` / `elapsed_ms` / `started_at`（索引 DESC）/ `ended_at` / `conversation_id` 索引 / `message_id` / `tool_call_id` / `flowrun_id` 索引 / `flowrun_node_id`。
**Function 专属字段**：`function_id`（索引：`idx_fne_function` 复合 priority:1 跟 created_at:2 倒序）/ `version_id` / `python_version`。
复合索引：`(function_id, created_at DESC)` 单 function 历史；`(conversation_id, message_id)` chat 追溯；`(flowrun_id, started_at)` workflow 追溯（V1.2 阶段未启用）。
HTTP 端点：`GET /api/v1/functions/{id}/executions`（per-function 分页 + aggregates）+ `GET /api/v1/function-executions/{execId}`（全局详情 + hints）。
LLM 工具 `search_function_executions` / `get_function_execution` 透过 Service 包装的 `SearchExecutions` / `GetExecutionDetail`,返 200B preview + aggregates / 4KB truncated + machine-computed hints（outputEmpty / significantlySlower）。

### Phase 3 — handler trinity (forge_redesign Plan 02)

#### `handlers` ✅
详见 [`../service-design-documents/handler.md`](../service-design-documents/handler.md) §8.1。
主键 `hd_<16hex>`;软删;用户作用域;partial UNIQUE `(user_id, name) WHERE deleted_at IS NULL`(schema_extras)。
字段:`name` / `description` / `tags` JSON / `active_version_id` / `config_encrypted`(AES-GCM 密文 blob 含全部 init_args 值,json:"-" 永不返序列化)/ 时间戳。
**计算字段**(`gorm:"-"`):`Pending *Version`,active version 的 5 个 env 字段镜像,`ConfigState`(unconfigured/partially_configured/ready),`LiveInstances` 跨 owner 数。`attachComputed` 在 Get/List 时填。

#### `handler_versions` ✅
详见 [`../service-design-documents/handler.md`](../service-design-documents/handler.md) §8.2。
主键 `hdv_<16hex>`;`status` CHECK in (pending/accepted/rejected),pending/rejected 时 `version` NULL。
class code parts:`imports` / `init_body` / `shutdown_body` / `methods` JSON / `init_args_schema` JSON。Sandbox: `dependencies` JSON / `python_version` / `env_id` 索引(`hdenv_<16hex>`,每 Version 行独立生成,D-redo-8 跟 version_id 1:1 但解耦)/ `env_status`(5 值 service-level 校验)/ `env_error` / `env_synced_at` / `env_sync_stage`(含 `fixing` 状态)/ `env_sync_detail`。
AcceptedVersionCap = 50/handler。Env 装配跟 function 同模式 — 同步发生在 `create_handler` / `edit_handler` 工具内,失败走内部 env-fix loop(详 [`../adhoc-topic-documents/forge_redesign/03-handler.md`](../adhoc-topic-documents/forge_redesign/03-handler.md) §11)。

#### `handler_calls` ✅ (D22)
详见 [`../service-design-documents/handler.md`](../service-design-documents/handler.md) §8.3 + [`08-executions.md`](../adhoc-topic-documents/forge_redesign/08-executions.md) §4.2。
主键 `hcl_<16hex>`;软删;用户作用域。每次 `Service.Call` 终态写一行(detached ctx §S9)。
通用 16 字段(跟 function_executions 一致)+ handler 专属 6 字段:`handler_id`(索引 priority:1) / `version_id` / `method`(索引) / `instance_id` / `owner_kind` / `owner_id`。
复合索引:`(handler_id, created_at DESC)` 单 handler 历史;`(conversation_id, message_id)` chat 追溯;`(flowrun_id, started_at)` workflow 追溯。
HTTP 端点:`GET /api/v1/handlers/{id}/calls` + `GET /api/v1/handler-calls/{callId}`。LLM 工具 `search_handler_calls` / `get_handler_call` 接 Service 的 `SearchCalls` / `GetCallDetail`。

### Phase 3 — workflow trinity (forge_redesign Plan 04)

#### `workflows` ✅
详见 [`../service-design-documents/workflow.md`](../service-design-documents/workflow.md) §8.1。
主键 `wf_<16hex>`;软删;用户作用域;partial UNIQUE `(user_id, name) WHERE deleted_at IS NULL`(schema_extras `idx_workflows_user_name_active`)。
字段:`name` / `description` / `tags` JSON / `enabled`(无 GORM `default:` tag — Service 层显式赋值,防 GORM 用 column 默认值静默覆盖零值)/ `concurrency`("serial",V1.5 加 "parallel(N)")/ `needs_attention` / `attention_reason`(Plan 05 D20 listener 写)/ `active_version_id` / **`timeout_sec INT default 0`**(§5.7 run-level timeout，0 = unlimited，>0 时 scheduler 用 `context.WithTimeout`，超时 → status=failed + RUN_TIMEOUT)/ 时间戳。
**计算字段**(`gorm:"-"`):`Pending *Version` / `LiveRuns int` / `LastFiredAt *time.Time` / `NextFireAt *time.Time`(后三个 Plan 05 territory,响应形状预留)。`attachComputed` 在 Get 时填 Pending。

#### `workflow_versions` ✅
详见 [`../service-design-documents/workflow.md`](../service-design-documents/workflow.md) §8.2。
主键 `wfv_<16hex>`;`status` 约束 `IN ('pending','accepted','rejected')`(app 层 + DB level — store.UpdateVersionStatus 接受任一,Service 严格三值),pending/rejected 时 `version` 为 NULL。
字段:`workflow_id` 索引 / `version` INT / `status` / `graph` TEXT(整图 JSON,Service 层 `attachGraph` 在 GET 时解为 `*Graph` 填 `Version.GraphParsed`)/ `change_reason` / 时间戳。
AcceptedVersionCap = 50/workflow;RejectPending 不留 rejected 行,直接 HardDeleteVersion(D-redo-12 mirror)。

> Plan 05 territory(本 plan 不实施):`flowruns` 表(per-trigger 一行,记录 trigger source / status / aggregates)+ `flowrun_nodes` 表(per-node execution log,继承 `flowrun_id` + `flowrun_node_id` 两个通用字段穿到 function_executions / handler_calls,D22 already-wired)。

### Phase 3 — execution plane trinity (forge_redesign Plan 05)

#### `flowruns` ✅
详见 [`../service-design-documents/flowrun.md`](../service-design-documents/flowrun.md) §2.1。
主键 `fr_<16hex>`;软删;用户作用域;复合索引 `(workflow_id, status, started_at DESC)`。
字段:`user_id` / `workflow_id` (FK) / `version_id` (锁起跑版本) / `trigger_kind` CHECK cron|fsnotify|webhook|manual / `trigger_input` JSON / `status` CHECK running|paused|completed|failed|cancelled / `started_at` / `ended_at` / `elapsed_ms` / `output` JSON / `error_code`（含 `RUN_TIMEOUT` for §5.7）/ `error_message` / `paused_state` JSON (approval/wait 暂停时持久化 ExecutionContext 快照)/ **`dry_run BOOLEAN default false`**（§19 dry-run preview run，side-effect dispatchers 返 mock outputs）。
**保留策略 (§6.7)**:每 workflow 默认保留最近 200 行,`HardDeleteOldest` 在 `scheduler.finalizeRun` 后异步剪。

#### `flowrun_nodes` ✅
详见 [`../service-design-documents/flowrun.md`](../service-design-documents/flowrun.md) §2.2 + [`08-executions.md`](../adhoc-topic-documents/forge_redesign/08-executions.md) §4.5。
主键 `frn_<16hex>`;软删;用户作用域。
**通用 16 字段**(spec 08 §2 模板) + **flowrun-specific 4 字段**:`flowrun_id` 索引 / `node_id` (graph 内 ID,如 "filter_cond") / `node_type` (function/handler/mcp/skill/llm/http/condition/loop/parallel/approval/wait/variable/trigger) / `attempts` (retry 次数 default 1) + **§5.1 loop body 2 字段**:`parent_loop_node TEXT`(外层 loop 节点 ID，body 子图迭代时填）+ `iteration_index INT default 0`(0-起 item 序号)。
索引 `(flowrun_id, started_at DESC)`(D22 spec 08 §2.5)。
**Cross-table linking**(spec 08 §4.5):capability 节点 (function/handler/mcp/skill) dispatch 时**同时写两条** — 一条到 flowrun_nodes (workflow 视角) + 一条到对应 entity 表 (function_executions / handler_calls / mcp_calls / skill_executions),经 `flowrun_node_id` 字段交叉引用。

#### `mcp_calls` ✅ (D22)
详见 [`../service-design-documents/mcp.md`](../service-design-documents/mcp.md) + [`08-executions.md`](../adhoc-topic-documents/forge_redesign/08-executions.md) §4.3。
主键 `mcl_<16hex>`;软删;用户作用域。每次 `Service.CallTool` 终态写一行(detached ctx §S9)。
通用 16 + mcp 专属 3 字段:`server_name`(索引) / `tool_name` / `server_version`(V1 留空,mcpinfra Client 暴露 initialize-response 后填)。
HTTP 端点 + LLM 工具 `search_mcp_calls` / `get_mcp_call`(D22 spec 08 §7,E13)。

#### `skill_executions` ✅ (D22)
详见 [`../service-design-documents/skill.md`](../service-design-documents/skill.md) + [`08-executions.md`](../adhoc-topic-documents/forge_redesign/08-executions.md) §4.4。
主键 `ske_<16hex>`;软删;用户作用域。每次 `Service.Activate` 终态写一行(defer-wrapped from outer Activate)。
通用 16 + skill 专属 4 字段:`skill_name`(索引) / `skill_version`(SHA256 of SKILL.md,V1 留空 hook) / `fork_depth`(0 = inline,≥1 = fork 嵌套深度,从 `reqctxpkg.GetSubagentDepth` 取) / `substitutions` JSON(\$1..\$N 替换值)。
LLM 工具 `search_skill_executions` / `get_skill_execution`(E13)。

> Plan 05 引入 4 张新表(flowruns / flowrun_nodes / mcp_calls / skill_executions);所有 D22 表共享通用 16 字段模板;capability 节点 dispatch 时跨表写两条经 `flowrun_node_id` 交叉引用。无新 schema_extras 行(D22 表索引全 GORM tag 表达得了)。

---

### Phase 5：System Tool 第二代（2026-05-04）

#### `todos` ✅
详见 [`../service-design-documents/todo.md`](../service-design-documents/todo.md)。
chat 内 todo 追踪 mini-domain；主键 `td_<16hex>`；GORM 软删（`deleted_at`）；复合索引 `idx_td_conv_status (conversation_id, status)`。表名走 GORM 默认复数化（`Todo` struct → `todos`）。
字段：`conversation_id`（作用域键）/ `subject`（imperative 标题）/ `description`（可选长文）/ `active_form`（present-continuous，UI 显示用）/ `status`（pending\|in_progress\|completed\|deleted，**白名单 app 层校验，DB 不 CHECK**——同 model_configs 模式）/ `owner`（可选 agent 名）/ `blocked_by JSON`（依赖 todo ID 列表）/ `metadata JSON`（自由扩展）/ 时间戳 + 软删。
Service 跨 conversation 操作返 `ErrNotFound`（防泄漏存在性）；每次变更 publish entity-state SSE `todo` 事件（详 events-design.md）。

> 2026-05-05 改名：原 `tasks` 表（`tk_` 前缀）改为 `todos`（`td_` 前缀），LLM-facing 工具同步 `Task*` → `Todo*`。原因：项目内"task"概念太宽泛，与计划中的 `Subagent` 工具语义易混；`Todo` 单义。

> 不持久化的兄弟域：**ask**（AskUserQuestion 会合）—— `app/ask` 持有 in-memory `pending` map（toolCallID → channel），无 DB 表，无 entity；详见 [`../service-design-documents/ask.md`](../service-design-documents/ask.md)。

#### `memories` ✅（V1.2 §2 final-sweep）
详见 [`../service-design-documents/memory.md`](../service-design-documents/memory.md)。
跨对话长期事实表；主键 `mem_<16hex>`；GORM 软删（`deleted_at`）；partial UNIQUE `(name) WHERE deleted_at IS NULL`（schema_extras）。**全局作用域**——无 `user_id` 字段（V1.2 单用户本地）。
字段：`name TEXT NOT NULL`（lowercase + digit + underscore，正则 `^[a-z][a-z0-9_]{0,63}$`，service 层校验）/ `type TEXT NOT NULL CHECK(type IN ('user','feedback','project','reference'))`（4 类 CoALA 分类）/ `description TEXT NOT NULL`（一行 summary，进 memory index 段）/ `content TEXT NOT NULL`（markdown 全文，pinned 时进 system prompt）/ `pinned BOOLEAN NOT NULL DEFAULT false`（pinned=全文 system prompt；非 pinned 只入 index）/ `source TEXT NOT NULL CHECK(source IN ('user','ai'))`（HTTP create=user / write_memory tool=ai；**Upsert 时不变**——保留原作者身份）/ `metadata TEXT`（GORM serializer:json，自由 map）/ `accessed_at DATETIME` / `access_count INT default 0`（Get + read_memory 时 bump，给 ListForIndex 排序用）/ 时间戳 + 软删。
复合索引：`idx_mem_type_pinned (type, pinned)` + `idx_mem_accessed (accessed_at DESC, access_count DESC)`（ListForIndex 排名）。每次变更 publish slim `memory` notification（`{action, name, memType, source}`）。

#### `documents` ✅ (Phase 5 §14.1)
详见 [`../service-design-documents/document.md`](../service-design-documents/document.md).
Notion-style 树状文档表;主键 `doc_<16hex>`;**自引用** `parent_id` (nil=root level);GORM 软删 (`deleted_at`);partial UNIQUE `(user_id, COALESCE(parent_id, ''), name) WHERE deleted_at IS NULL`(schema_extras——`COALESCE` 让根级两条同名也撞 UNIQUE,SQLite 默认视 NULL!=NULL 会漏)。**user-scoped**(V1 = local-user)。
字段:`user_id TEXT NOT NULL`(index) / `parent_id TEXT`(index,nil=root) / `name TEXT NOT NULL`(标题,Service 层校验:非空 / ≤256 字符 / 不含 `/`) / `description TEXT NOT NULL DEFAULT ''`(一行 summary,catalog 给 LLM 看) / `content TEXT NOT NULL DEFAULT ''`(markdown body,Service 层 1 MB 上限) / `tags TEXT NOT NULL DEFAULT '[]'`(GORM `serializer:json`) / `position INT NOT NULL DEFAULT 0`(同级排序,拖拽时改) / `path TEXT NOT NULL`(冗余字段如 `/Projects/2026/Q1`,Insert/Update name/Move 时由 Service 重算,move/rename 时整子树级联) / `size_bytes INT NOT NULL` / 时间戳 + 软删。
索引:`idx_documents_user_id` / `idx_documents_parent_id` / `idx_documents_path` / `idx_documents_deleted_at`(均 GORM tag);schema_extras `idx_documents_parent_name_active` partial UNIQUE。每次 Create/Update/Move/Delete publish slim `document` notification(`{action, parentId?, path}`)。**树操作语义**:Move 防成环靠 Service `IsAncestor` walk (depth bound 10k);删除是软删整子树(BFS 收集后裔 → 单 GORM Delete 走 deleted_at) — 详见 [`../service-design-documents/document.md`](../service-design-documents/document.md) §11。

---

### Phase 4 准备件（2026-05-05 设计完成 / 待实施）

> Subagent 数据模型 (2026-05 schema 统一)：sub-run 不再有独立表。一次 spawn 是 `messages` 表里的一行（attrs.kind=subagent_run + type/runId/maxTurns；parent_block_id 指向父 message 的 message-block 占位），sub-run 内部 transcript 是该 message 在 `message_blocks` 的 blocks（经 eventlog Emitter 实时写）。无 `subagent_runs` / `subagent_messages` 表——递归事件日志一处管全部。详见 [`../service-design-documents/subagent.md`](../service-design-documents/subagent.md)。

#### `sandbox_runtimes` ✅
详见 [`../service-design-documents/sandbox.md`](../service-design-documents/sandbox.md) §5。
统一 PluginSandbox 的 runtime 元数据表（已装的 Python/Node/Rust/Java/...）。主键 `sr_<16hex>`；UNIQUE `(kind, version)`。
字段：`kind`（"python"/"node"/"uv"/任何 mise 支持的）/ `version`（"3.12.5"/"22.5.0"/"stable"）/ `path`（相对 `~/.forgify/sandbox/runtimes/`）/ `size_bytes` / `installed_at` / `updated_at`。
默认版本由 `RuntimeInstaller.ResolveDefault` 拥有（构造时固化的常量）；无 `is_default` 列 / `FindDefaultRuntime` 查询。

#### `sandbox_envs` ✅
详见 [`../service-design-documents/sandbox.md`](../service-design-documents/sandbox.md) §5。
每个 plugin 实例（function/mcp/skill/conversation/...）的隔离 env。主键 `se_<16hex>`；UNIQUE `(owner_kind, owner_id)`；复合索引 `idx_se_owner (owner_kind, owner_id)`；索引 `last_used_at`（GC）。**`runtime_id` 引用 `sandbox_runtimes.id`**（无 DB FK 约束——§D4 实施现实见 CLAUDE.md；lifecycle 由 app 层 service 管：删 runtime 前确保无 env 引用）。
字段：`owner_kind`（"function"/"mcp"/"skill"/"conversation"，**DB CHECK** `IN ('function','mcp','skill','conversation')`）/ `owner_id`（function "<fnID>_<envID>" / mcp server name / skill name / "<conv_id>_<runtime_kind>" 用 `_` 不用 `:`）/ `owner_name`（UI display）/ `runtime_id`（引用 sandbox_runtimes.id，软引用）/ `deps JSON` / `path`（相对 `~/.forgify/sandbox/envs/`）/ `size_bytes` / `status`（"installing"/"ready"/"failed"，**DB CHECK** `IN ('installing','ready','failed')`，default `'ready'`）/ `error_msg` / **`running_pid INT default 0; index`**（Layer-B leak prevention：Bootstrap 时 kill 残留进程）/ `created_at` / `last_used_at`（索引，GC 用）/ `updated_at`。

**Conversation owner 特殊**：conversation 软删保留 env（恢复对话仍可用）；硬删时立即 Destroy；**v1 全 owner 默认手动 GC**（uv/pnpm 包管理器原生共享让多 conv 磁盘开销极小，自动 GC 价值低）。

#### mcp / skill / catalog（不进 DB）

- **mcp 配置**：`~/.forgify/mcp.json` 文件 source of truth（Claude Desktop schema 兼容）；server runtime 状态在内存。详见 [`../service-design-documents/mcp.md`](../service-design-documents/mcp.md) §5。
- **skill 文件**：`~/.forgify/skills/<name>/SKILL.md` 文件系统 source of truth；fsnotify 维护内存 cache。详见 [`../service-design-documents/skill.md`](../service-design-documents/skill.md) §5。
- **catalog**：`~/.forgify/.catalog.json` 派生 cache（删了能重建）；进程内 cache 热路径。详见 [`../service-design-documents/catalog.md`](../service-design-documents/catalog.md) §5。

---

### Phase 4

#### `workflows` ⬜
#### `flowruns` ⬜
#### `nodes` ⬜（如节点独立成表）
#### `schedulers` ⬜
#### `triggers` ⬜

---

### Phase 5

#### `knowledge_bases` ⬜
#### `documents` ⬜
#### `document_chunks` ⬜
#### `embeddings` ⬜（向量存储，本地 sqlite-vec）
#### `mcp_servers` — 已提前交付（不进 DB，配置文件存储）✅ 见上方"Phase 4 准备件"
#### `skills` — 已提前交付（不进 DB，文件系统存储）✅ 见上方"Phase 4 准备件"

---

## 跨表关系图

> 每完成一个 Phase 更新一次。

**当前（Phase 3 + chat infra 重构 + Phase 5 schema 统一，2026-05-02）**：
```
api_keys    model_configs   conversations
    │             │               │
    └─────────────┴───── local-user ──────┘
                                   │ conversation_id
                               messages ────────── message_blocks
                               (含 error_code/                (text/reasoning/
                                error_message/                 tool_call/tool_result
                                updated_at)                    含 errorMsg+elapsedMs/
                                   │                           attachment_ref)
                                   │ att_id
                              attachments (软删)

functions ──── function_versions (status: pending/accepted/rejected;
  │             change_reason 为变更意图;含 dependencies/python_version/env_*)
  │
  └ ─────────  function_executions (D22;status: ok/failed/cancelled/timeout;
                triggered_by + conversation_id/message_id/tool_call_id
                把 chat 触发的执行串回去;LLM 经 search_function_executions /
                get_function_execution 自诊断)

todos (Phase 5 新增；conversation_id 作用域，status app 层白名单)

subagent (无独立表；sub-run = messages.attrs.kind=subagent_run，
  sub-blocks 进 message_blocks 经 eventlog Emitter 实时写；
  parent_block_id 串接父 message 的占位块——递归事件树一处管全部)

sandbox_runtimes ─── sandbox_envs (Phase 4 准备件，2026-05-05；
  统一 PluginSandbox 数据：runtime 共享 + env 隔离；
  owner_kind 含 "function"/"mcp"/"skill"/"conversation"；
  v1 全 owner 默认手动 GC（uv/pnpm 共享让多 conv 磁盘 ≈ 1×）)
```

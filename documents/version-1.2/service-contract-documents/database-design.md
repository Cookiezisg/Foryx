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

#### `api_keys` ✅
详见 [`../service-design-documents/apikey.md`](../service-design-documents/apikey.md) §11。
主键 `aki_<16hex>`；软删（`DeletedAt`）；全索引 `(user_id)` + `(user_id, provider)` + `(deleted_at)`（目前未走部分索引 `WHERE deleted_at IS NULL`，见 backlog）。**注**：`idx_api_keys_user_id` 单列与复合索引前导列重合可视为冗余，保留是为匹配 List filter 写法清晰度（单用户本地写入罕见，冗余成本可忽略）。敏感字段 `key_encrypted`（AES-GCM `v1:` 前缀，`json:"-"` 守护永不上线）+ `key_masked` 冗余展示。**feature 列**：`display_name`（UI 展示用）/ `base_url`（ollama / custom 必填）/ `api_format`（custom 必填，openai-compatible / anthropic-compatible 二选一）/ `test_status`（pending / ok / error）/ `test_error`（连通性测试失败原因）/ `last_tested_at`（最近一次测试时间，nullable）/ `models_found TEXT`（GORM `serializer:json`，存 JSON 字符串如 `["deepseek-chat","deepseek-reasoner"]`；测试成功后由 `UpdateTestResult` 写入，测试前为 `[]`）。不加 `UNIQUE(user_id, provider)`，允许同 provider 多 key。Provider / TestStatus 的 DB 层 CHECK 约束**未加**，由 app 层校验。

#### `model_configs` ✅
详见 [`../service-design-documents/model.md`](../service-design-documents/model.md) §11。
主键 `mc_<16hex>`；软删（`deleted_at`）；GORM 全唯一索引 `UNIQUE(user_id, scenario)`（partial UNIQUE 暂缓，见 §17 决定）。Scenario 白名单 app 层校验，DB 不 CHECK。

#### `conversations` ✅
详见 [`../service-design-documents/conversation.md`](../service-design-documents/conversation.md) §8。
主键 `cv_<16hex>`；软删（`deleted_at`）；`user_id` 索引。新增字段：`system_prompt TEXT`（对话级自定义系统提示词，可为空）/ `auto_titled BOOLEAN`（标记标题是 AI 自动生成的还是用户手动改的）。Title 允许空字符串，首轮完成后 auto-titling goroutine 回写。

#### `messages` ✅（chat infra 重构后精简；Phase 5 加错误信息字段；schema 统一后加 subagent 嵌套字段）
chat domain 所有；主键 `msg_<16hex>`；字段：`conversation_id`（索引）/ `user_id` / `role`（**user\|assistant**，`tool` 角色已移除——tool result 变为 message_blocks 的 block）/ `status`（pending\|streaming\|completed\|error\|cancelled）/ `stop_reason` / **`error_code`**（status="error" 时填，如 `LLM_STREAM_ERROR` / `HISTORY_EXTEND_FAILED` / `MODEL_NOT_CONFIGURED` / `INTERNAL_ERROR` 等；其他 status 时为空）/ **`error_message`**（结构化错误原因人类可读文本）/ `input_tokens INT` / `output_tokens INT` / **`parent_block_id TEXT`**（嵌套 subagent run 时指向父 message-block 占位；非嵌套时空）/ **`attrs TEXT`**（JSON 自由形状——user message 含 `attachments[]` 引用；subagent sub-message 含 `kind=subagent_run` + type / runId / maxTurns）/ `created_at` / **`updated_at`**（GORM 自动维护）/ 软删 `deleted_at`。**内容字段已移除**：`content`、`reasoning_content`、`tool_calls`、`tool_call_id`、`attachment_ids`、`token_usage` 全部转移到 `message_blocks` 表。FTS5 已移除（原基于 `content` 列，后续按 message_blocks 重建）。

#### `message_blocks` ✅（事件日志协议 + 递归嵌套，2026-05 schema 统一）
chat domain 所有；主键 `blk_<16hex>` 或 LLM 自带 `tc_<id>`（tool_call 复用 LLM 给的 ID，方便 tool_result 用 parent_block_id 直挂）。

**字段**：
- `conversation_id TEXT NOT NULL` — UNIQUE(conv_id, seq) 复合索引第 1 列；replay 查询不需 join messages
- `message_id TEXT NOT NULL idx` — 顶层归属的 message
- `parent_block_id TEXT NULL idx` — 嵌套指针（如 progress 挂 tool_call 下）；顶层 block 此列空
- `seq INT64 NOT NULL` — UNIQUE 复合第 2 列；**per-conversation 全局**单调（Bridge 分配，非 per-message）
- `type TEXT NOT NULL CHECK(type IN ('text','reasoning','tool_call','tool_result','progress','message'))` — 6 值封闭枚举
- `attrs TEXT` — JSON 元数据（如 `{tool: name}` / `{stage: installing}` / `{messageId: subId}`）
- `content TEXT NOT NULL DEFAULT ''` — append-only 流式正文（DeltaBlock SQL `content || ?` 原子拼）
- `status TEXT NOT NULL CHECK(status IN ('streaming','completed','error','cancelled'))` — 终态机
- `error TEXT` — block_stop 时填（status='error' 时）
- `created_at` / `updated_at` — GORM 自动维护

索引：UNIQUE(conv_id, seq) + (message_id) + (parent_block_id)。**无软删**——历史 block 不会被改名/删除。完整设计 [`../event-log-protocol.md`](../event-log-protocol.md) §6。

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

---

### Phase 5：System Tool 第二代（2026-05-04）

#### `todos` ✅
详见 [`../service-design-documents/todo.md`](../service-design-documents/todo.md)。
chat 内 todo 追踪 mini-domain；主键 `td_<16hex>`；GORM 软删（`deleted_at`）；复合索引 `idx_td_conv_status (conversation_id, status)`。表名走 GORM 默认复数化（`Todo` struct → `todos`）。
字段：`conversation_id`（作用域键）/ `subject`（imperative 标题）/ `description`（可选长文）/ `active_form`（present-continuous，UI 显示用）/ `status`（pending\|in_progress\|completed\|deleted，**白名单 app 层校验，DB 不 CHECK**——同 model_configs 模式）/ `owner`（可选 agent 名）/ `blocked_by JSON`（依赖 todo ID 列表）/ `metadata JSON`（自由扩展）/ 时间戳 + 软删。
Service 跨 conversation 操作返 `ErrNotFound`（防泄漏存在性）；每次变更 publish entity-state SSE `todo` 事件（详 events-design.md）。

> 2026-05-05 改名：原 `tasks` 表（`tk_` 前缀）改为 `todos`（`td_` 前缀），LLM-facing 工具同步 `Task*` → `Todo*`。原因：项目内"task"概念太宽泛，与计划中的 `Subagent` 工具语义易混；`Todo` 单义。

> 不持久化的兄弟域：**ask**（AskUserQuestion 会合）—— `app/ask` 持有 in-memory `pending` map（toolCallID → channel），无 DB 表，无 entity；详见 [`../service-design-documents/ask.md`](../service-design-documents/ask.md)。

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

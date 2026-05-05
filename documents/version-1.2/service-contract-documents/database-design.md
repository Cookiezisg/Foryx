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
- 部分 UNIQUE 索引（`WHERE deleted_at IS NULL`，例如 tools `UNIQUE(user_id, name)`）
- 触发器
- FTS5 虚拟表（**当前未使用**——chat 重构 2026-04-27 时移除了原基于 messages.content 的 FTS5；modernc 驱动内置 FTS5，未来按 message_blocks.data 重建时无需编译标志）

---

## 表清单

> **状态**：⬜ 未设计 | 🔄 讨论中 | ✅ 已实现

### Phase 2

#### `api_keys` ✅
详见 [`../service-design-documents/apikey.md`](../service-design-documents/apikey.md) §11。
主键 `aki_<16hex>`；软删（`DeletedAt`）；全索引 `(user_id)` + `(user_id, provider)` + `(deleted_at)`（目前未走部分索引 `WHERE deleted_at IS NULL`，见 backlog）。敏感字段 `key_encrypted`（AES-GCM `v1:` 前缀，`json:"-"` 守护永不上线）+ `key_masked` 冗余展示。不加 `UNIQUE(user_id, provider)`，允许同 provider 多 key。Provider / TestStatus 的 DB 层 CHECK 约束**未加**，由 app 层校验。新增 `models_found TEXT`（GORM `serializer:json`，存 JSON 字符串如 `["deepseek-chat","deepseek-reasoner"]`；测试成功后由 `UpdateTestResult` 写入，测试前为 `[]`）。

#### `model_configs` ✅
详见 [`../service-design-documents/model.md`](../service-design-documents/model.md) §11。
主键 `mc_<16hex>`；软删（`deleted_at`）；GORM 全唯一索引 `UNIQUE(user_id, scenario)`（partial UNIQUE 暂缓，见 §17 决定）。Scenario 白名单 app 层校验，DB 不 CHECK。

#### `conversations` ✅
详见 [`../service-design-documents/conversation.md`](../service-design-documents/conversation.md) §8。
主键 `cv_<16hex>`；软删（`deleted_at`）；`user_id` 索引。新增字段：`system_prompt TEXT`（对话级自定义系统提示词，可为空）/ `auto_titled BOOLEAN`（标记标题是 AI 自动生成的还是用户手动改的）。Title 允许空字符串，首轮完成后 auto-titling goroutine 回写。

#### `messages` ✅（chat infra 重构后精简；Phase 5 加错误信息字段）
chat domain 所有；主键 `msg_<16hex>`；字段：`conversation_id`（索引）/ `user_id` / `role`（**user\|assistant**，`tool` 角色已移除——tool result 变为 message_blocks 的 block）/ `status`（pending\|streaming\|completed\|error\|cancelled）/ `stop_reason` / **`error_code`**（status="error" 时填，如 `LLM_STREAM_ERROR` / `HISTORY_EXTEND_FAILED` / `MODEL_NOT_CONFIGURED` / `INTERNAL_ERROR` 等；其他 status 时为空）/ **`error_message`**（结构化错误原因人类可读文本）/ `input_tokens INT` / `output_tokens INT` / `created_at` / **`updated_at`**（GORM 自动维护）/ 软删 `deleted_at`。**内容字段已移除**：`content`、`reasoning_content`、`tool_calls`、`tool_call_id`、`attachment_ids`、`token_usage` 全部转移到 `message_blocks` 表。FTS5 已移除（原基于 `content` 列，后续按 message_blocks 重建）。

#### `message_blocks` ✅（chat infra 重构新增；Phase 5 给 tool_result 加 errorMsg/elapsedMs）
chat domain 所有；主键 `blk_<16hex>`；外键 `message_id → messages.id`；字段：`seq INT`（消息内顺序）/ `type`（text\|reasoning\|tool_call\|tool_result\|attachment_ref）/ `data TEXT`（JSON，格式随 type 变化）。复合索引 `(message_id, seq)`。无软删（随 message 一起管理）。block type 的 data JSON 结构：`text/reasoning → {text}`；`tool_call → {id, name, summary, destructive, arguments}`（`destructive` 由 LLM 自报、framework 剥除参数后存入；UI 据此显示警示徽章）；`tool_result → {toolCallId, ok, result, errorMsg, elapsedMs}`（`errorMsg` 仅 ok=false 时填；`elapsedMs` 是工具 wall time）；`attachment_ref → {attachmentId, fileName, mimeType}`。

#### `attachments` ✅（Phase 5 重命名 chat_attachments → attachments + 加软删）
chat domain 所有；主键 `att_<16hex>`；字段：`user_id`（索引）/ `file_name` / `mime_type` / `size_bytes` / `storage_path`（相对 dataDir，json:"-" 不对外暴露）/ `created_at` / **`updated_at`** / **`deleted_at`** GORM 软删。文件实体存 `{dataDir}/attachments/{att_id}/original.{ext}`，50MB 限制。**支持软删**——用户删附件后旧对话仍能解引用（不再硬删导致 dangling reference）。

---

### Phase 3

#### `forges` ✅
详见 [`../service-design-documents/forge.md`](../service-design-documents/forge.md) §3.1。
主键 `f_<16hex>`；软删（`deleted_at`）；`user_id` 索引；partial UNIQUE `UNIQUE(user_id, name) WHERE deleted_at IS NULL`（在 `schema_extras.go`）。
字段：`name` / `description` / `code`（当前活跃代码）/ `parameters`（JSON 数组）/ `return_schema`（JSON 对象）/ `tags`（JSON 数组）/ `version_count`（最大已接受版本号，0=未保存）/ **`active_version_id`**（沙箱迭代 1：指向当前活跃 ForgeVersion.ID；草稿期空字符串）/ `created_at` / `updated_at` / `deleted_at`。
**计算字段（非列）**：`Pending *ForgeVersion`（`gorm:"-"`），由 service 层 `attachPending` 在 GET / List 后填充。**沙箱迭代 1 新增**：`EnvStatus` / `EnvError` / `EnvSyncedAt` / `EnvSyncStage` / `EnvSyncDetail`（`gorm:"-"`），由 `attachActiveEnv` 从 ActiveVersion 拷过来——草稿期 ActiveVersionID="" 时全空。entity-state SSE `forge` 事件载荷依赖这些计算字段。
forge 搜索通过 LLM 排序实现（SearchForge 把全量 forge 发给 LLM），无独立向量索引。

#### `forge_versions` ✅
详见 [`../service-design-documents/forge.md`](../service-design-documents/forge.md) §3.2。
主键 `fv_<16hex>`；**兼作 pending 变更存储**：`status` 字段区分 `pending`/`accepted`/`rejected`，pending/rejected 时 `version` 为 NULL。
完整快照字段：`name` / `description` / `code` / `parameters` / `return_schema` / `tags` / **`change_reason`**（Phase 5 改名 from `message`：LLM 指令 | "manual edit" | "reverted to v{N}" | "initial"）/ `created_at` / `updated_at`。
**沙箱迭代 1 新增字段**：
- **`dependencies`** TEXT default `'[]'`（PEP 508 specifier JSON 数组，由 LLM 在 create_forge / edit_forge 时申报）
- **`python_version`** TEXT default `''`（PEP 440 spec；空回退到 `forgedomain.DefaultPythonVersion=">=3.12"`）
- **`env_id`** TEXT 索引（沙箱按此键管 venv 目录；同 deps + python 的多版本共享同 EnvID 进而共享 venv）
- **环境运行时状态**：`env_status` TEXT default `'pending'`（5 值：`pending`/`syncing`/`ready`/`failed`/`evicted`，白名单 service 层校验）/ `env_error` TEXT（uv stderr 在失败时）/ `env_synced_at` DATETIME（成功时戳，状态转 ready 时设，其他状态置 nil）/ `env_sync_stage` TEXT（`resolving`/`preparing`/`installing`，sync 期间）/ `env_sync_detail` TEXT（uv stderr 当前 stage 行）

每版本独立的 env 状态——pending 自带自己的 sync 历史，跟 active 不串。EnvID 相同的多版本共用 venv，但 env_status 各自独立。
accepted 版本上限 50 条/forge，超限硬删最旧。

#### `forge_test_cases` ✅
详见 [`../service-design-documents/forge.md`](../service-design-documents/forge.md) §3.3。
主键 `tc_<16hex>`；`forge_id` 索引。字段：`name` / `input_data`（JSON）/ `expected_output`（JSON，空=不断言）。

#### `forge_executions` ✅（Phase 5 统一表，替代 forge_run_history + forge_test_history）
详见 [`../service-design-documents/forge.md`](../service-design-documents/forge.md) §3.4。
主键 `fe_<16hex>`；无软删；保留最近 300 条/forge（合并上限，原 100+200）。
字段：
- `forge_id`（索引：`idx_fe_forge_created` 复合 priority:1）/ `user_id` / `forge_version`（执行时版本号）
- **`kind`**（**"run"** 临时运行 / LLM 调用，**"test"** 测试用例执行）—— 区分两类历史
- `input`（JSON）/ `output` / `ok` / `error_msg` / `elapsed_ms`
- **test 专属可空字段**（kind="run" 时为空字符串）：`test_case_id`（索引）/ `batch_id`（索引；批跑共享，单跑为空）/ `pass *bool`（nil=无断言）
- **触发上下文**：**`triggered_by`**（**"chat"** LLM 在 chat 中调用 / **"http"** 用户直接调）/ `conversation_id` / `message_id` / `tool_call_id`（chat 触发时填，HTTP 触发时空）
- `created_at`（索引：`idx_fe_forge_created` 复合 priority:2，`idx_fe_msg` 复合 with conversation_id+message_id）

复合索引 2 个：`idx_fe_forge_created (forge_id, created_at)` 单 forge 历史按时间倒序检索；`idx_fe_msg (conversation_id, message_id)` 一次 chat 消息触发的所有 forge 调用追溯。
HTTP 端点：`GET /api/v1/forges/{id}/executions?kind=&batchId=&cursor=&limit=`（cursor 分页 envelope）。
内部使用 `ExecutionFilter` struct 支持 forge / kind / batch_id / test_case_id / chat 上下文 / cursor / limit 任意组合查询。

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

#### `subagent_runs` 📐
详见 [`../service-design-documents/subagent.md`](../service-design-documents/subagent.md) §3。
每次 `Task` system tool spawn 一条 run 总账。主键 `sar_<16hex>`；无软删（仅历史记录）；`parent_conversation_id` 索引（`ListByConversation` 查询）。
字段：`parent_conversation_id` / `parent_message_id`（索引）/ `parent_tool_call_id` / `type`（subagent 类型名 Explore/Plan/general-purpose）/ `prompt` / `result` / `status`（5 值：`running`/`completed`/`cancelled`/`max_turns`/`failed`，**白名单 app 层校验**）/ `total_tokens_in INT` / `total_tokens_out INT` / `steps_used INT` / `model` / `started_at` / `ended_at *DATETIME` / `error_msg` / 时间戳。

**流式 UI 瞬时字段不落库**（`LastToolCalled` / `LastToolArgsBrief` / `LastToolResultBrief` / `LastStepDurationMs` / `LastStepAt`）—— `gorm:"-"` 仅 in-memory 维护，run 跑完即过期，重启丢失无所谓。通过 chat.message 嵌套 subagentRun 推前端做小窗状态条。

每 step 末由 sub-runner 更新（持久化字段 + 瞬时字段同时刷）；终态时 ended_at 写盘。**Conversation 删除时不级联删**——保留独立审计；UI list 通过 conversation_id 自然过滤掉孤儿。

#### `subagent_messages` 📐
详见 [`../service-design-documents/subagent.md`](../service-design-documents/subagent.md) §3。
subagent 内部消息细粒度持久化（流式小窗回放用）。主键 `smm_<16hex>`；无软删；复合索引 `idx_smm_run_seq (subagent_run_id, seq)` 单 query 命中"按 run 拉全部"+按 seq 排序。
字段：`subagent_run_id`（索引 priority:1）/ `seq INT`（run 内顺序，0 起，AppendMessage 自增分配）/ `role`（user\|assistant\|tool\|system）/ `blocks JSON`（**复用 chatdomain.Block 类型**——text/reasoning/tool_call/tool_result/attachment_ref，前端渲染零成本）/ `prompt_tokens INT` / `completion_tokens INT`（仅 assistant role）/ `created_at`。

**不级联** subagent_runs 删除——独立生命周期，方便回放历史 subagent 行为。

#### `sandbox_runtimes` 📐
详见 [`../service-design-documents/sandbox.md`](../service-design-documents/sandbox.md) §5。
统一 PluginSandbox 的 runtime 元数据表（已装的 Python/Node/Rust/Java/...）。主键 `sr_<16hex>`；UNIQUE `(kind, version)`；复合索引 `idx_sr_kind_def (kind, is_default)`。
字段：`kind`（"python"/"node"/"rust"/"java"/"go"/"ruby"/"php"/"browsers"/"dotnet"/"static"/...）/ `version`（"3.12.5"/"22.5.0"/"stable"/"chromium-1234"）/ `path`（相对 `~/.forgify/sandbox/runtimes/`）/ `size_bytes` / `is_default`（该 kind 的默认那个）/ `installed_at` / `updated_at`。

#### `sandbox_envs` 📐
详见 [`../service-design-documents/sandbox.md`](../service-design-documents/sandbox.md) §5。
每个 plugin 实例（forge/mcp/skill/conversation/...）的隔离 env。主键 `se_<16hex>`；UNIQUE `(owner_kind, owner_id)`；FK `runtime_id → sandbox_runtimes.id`；复合索引 `idx_se_owner (owner_kind, owner_id)`；索引 `last_used_at`（GC）。
字段：`owner_kind`（"forge"/"mcp"/"skill"/"conversation"）/ `owner_id`（forge envID / mcp server name / skill name / "<conv_id>:<runtime_kind>"）/ `owner_name`（UI display）/ `runtime_id`（FK）/ `deps JSON` / `extras JSON` / `path`（相对 `~/.forgify/sandbox/envs/`）/ `size_bytes` / `status`（"installing"/"ready"/"failed"）/ `error_msg` / `created_at` / `last_used_at`（索引，GC 用）/ `updated_at`。

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

forges ──────── forge_versions (status: pending/accepted/rejected;
  │             change_reason 为变更意图)
  │
  │ ─────────  forge_test_cases
  │
  └ ─────────  forge_executions (kind=run|test; batch_id 串联批跑;
                triggered_by + conversation_id/message_id/tool_call_id
                把 chat 触发的执行串回去)

todos (Phase 5 新增；conversation_id 作用域，status app 层白名单)

subagent_runs ─── subagent_messages (Phase 4 准备件，2026-05-05；
  parent_conversation_id 反向引用 conversations；
  subagent_messages 复用 chatdomain.Block 类型；
  conversation 删除时不级联——独立审计)

sandbox_runtimes ─── sandbox_envs (Phase 4 准备件，2026-05-05；
  统一 PluginSandbox 数据：runtime 共享 + env 隔离；
  owner_kind 含 "forge"/"mcp"/"skill"/"conversation"；
  v1 全 owner 默认手动 GC（uv/pnpm 共享让多 conv 磁盘 ≈ 1×）)
```

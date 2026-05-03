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
#### `mcp_servers` ⬜
#### `skills` ⬜

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
```

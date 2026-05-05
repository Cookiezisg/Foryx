# Events Design — V1.2 SSE 事件一眼索引

**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- **配套实现**：`domain/events.Bridge` 接口 + `infra/events/memory.Bridge`（已就绪，72 测试）
- **SSE 订阅端点**：`GET /api/v1/events?conversationId=xxx`（Phase 2 chat 落地时接）

**定位**：**全仓所有 SSE 事件一眼索引**。每个事件的完整 struct / 触发时机 / 详细载荷，**去对应 domain 的 `service-design-documents/<domain>.md` 看**。

**遵守标准**：E1（强类型，禁止 `map[string]any`）/ E2（snake_case 分层，必带过滤上下文）

---

## 全局约定

### 事件传输
- 客户端通过 `GET /api/v1/events?conversationId=xxx` 订阅 SSE 流
- 后端 `domain/events.Bridge` 接口 + `infra/events/memory.Bridge` 内存实现
- 未来 SaaS 可换 `infra/events/redis` 实现，业务代码零改

### 事件命名（E2）
- 全部 snake_case，按 domain 加点号前缀：`chat.token`、`tool.code_updated`
- 每个事件必带 `conversationId` 或其他过滤上下文（subscriber 的 filter key）
- **死事件禁止**：每个事件必须有真实发布点

### 事件 struct（E1）
所有事件必须有具体 Go struct，定义在 `domain/events/types.go` 或按 domain 分文件。
**禁止** 发布或订阅时使用 `map[string]any`。

### 字段规范
- 字段命名：`camelCase` JSON tag（前端友好）
- 每个事件必带 `conversationId` 或其他明确过滤上下文

### Bridge 行为（已实现）
- 慢订阅者 buffer 满 → **丢弃 + warn log**，不阻塞 publisher
- ctx 结束 → 自动 unsubscribe
- 多次 cancel → sync.Once 保证幂等

---

## 事件清单

> **状态**：⬜ 未设计 | 🔄 struct 定义中 | ✅ 已实现（struct + 真实发布点）

### entity-state 模型（Phase 6 重构 · 2026-05-02；Phase 5 加 todo · 2026-05-04，原 task 改名于 2026-05-05；Phase 4 准备件加 mcp/skill + chat.message 加 subagent 字段 · 2026-05-05）

**6 个事件**（4 已实现 + 2 新设计待实施 mcp/skill）+ chat.message 在 subagent 上下文额外承载 SubagentRun 快照。订阅方按 entity ID 替换本地拷贝即可渲染。**subagent 不发独立 SSE 事件**——所有信息（消息内容 + run 元数据 + lifecycle）全合到 chat.message 一条流，subagent 上下文额外携带 `subagentRunId` + `parentConversationId` + `subagentRun` 三字段。

每个事件 struct 嵌入 `*<domain>.Entity` 指针 + 自定义 `MarshalJSON` → wire 形状 = `GET /api/v1/<entities>/{id}` 的响应（无 wrapper key，entity 字段直接出现在顶层）。

| 事件名 | 载荷 = | 过滤 key | 触发点 | 状态 |
|---|---|---|---|---|
| `chat.message` | 完整 `Message`（含 `blocks`/`status`/`stopReason`/`errorCode`/`errorMessage`/`inputTokens`/`outputTokens`/`updatedAt`）；subagent 上下文额外携带 **`subagentRunId`** + **`parentConversationId`** + **`subagentRun`**（完整 SubagentRun 快照含 token/status/lastTool/等，全部 omitempty 主对话消息向后兼容）| `conversationId`（主对话）/ `parentConversationId`（subagent 消息）| message slot 创建、每个 LLM token、tool_call 出现、tool_call args 完整、每个 tool result 完成、最终写、pre-LLM 失败的 stub 错误消息；Phase 5 起 AskUserQuestion 工具的提问通过此事件；V1.2 D4 起 subagent 内部 sub-runner 推消息也通过此事件——载荷加 subagent 三字段，前端按 subagentRunId 分流到主对话区 / 流式小窗，subagentRun 子对象提供 lifecycle 状态条 | ✅ |
| `forge` | 完整 `Forge`（含 `pending` 子对象/`code`/`parameters`/`returnSchema`/`tags`/`versionCount`/`activeVersionId`/`envStatus`/`envError`/`envSyncedAt`/`envSyncStage`/`envSyncDetail`）| `conversationId` | create_forge / edit_forge 期间逐 token（draft 在内存增长）、最终 save 后定型；**沙箱迭代 1 新增**：EnvStatus 状态转换（pending→syncing→ready/failed/evicted）、每行 uv stderr 解析（envSyncStage / envSyncDetail 变化）；HTTP CRUD 暂不广播（MVP 单用户单窗口）| ✅ |
| `conversation` | 完整 `Conversation`（含 `title`/`autoTitled`/`systemPrompt` 等）| `conversationId` | auto-title 回写、未来归档/系统 prompt 更新等 | ✅ |
| `todo` | 完整 `Todo`（含 `id`/`conversationId`/`subject`/`description`/`activeForm`/`status`/`owner`/`blockedBy`/`metadata`/时间戳）| `conversationId` | TodoCreate / TodoUpdate / TodoDelete 任意时点；删除时最后一帧 status="deleted" 让订阅方丢本地拷贝 | ✅ |
| `mcp` | `{servers: [ServerStatus...]}` 全 server 状态快照（含 `name`/`status`(disconnected/connecting/ready/degraded/failed)/`pid`/`connectedAt`/`lastError`/`lastErrorAt`/`lastSuccessAt`/`consecutiveFailures`/`totalCalls`/`totalFailures`/`tools[]`）| `global`（无过滤——所有用户都看自己的）| Connect / Disconnect / Enable / Disable / 子进程退出 monitor 检测到 disconnect / degraded 状态转换（连续失败≥3）/ 自愈回 ready；推全 server 快照（前端拿全量重渲染最简单，未来 server 数多时再考虑 B 方案瘦载荷）| 📐 |
| `skill` | `{skills: [Skill...]}` 全 skill 快照（每条含 `name`/`source`/`dirPath`/`bodyPath`/`description`/`frontmatter`/`loadedAt`，**不含 body**——body 是 L2 按需加载）| `global` | fsnotify 抓到 SKILL.md 改动 + Service.Scan 完成 / 手动 `:refresh` / `:import` 后重扫 | 📐 |

**配套实现细节**：

- create_forge 进入即预分配 `forgeID = forgeapp.NewForgeID()`，构建内存 stub Forge，逐 token 更新 `Forge.Code` 并发快照；末尾才走 `svc.Create(ID=forgeID)` 真正落库。失败干净丢弃 draft，无 DB 污染。
- edit_forge 进入预分配 `pendingID = forgeapp.NewVersionID()`，构建 `draftPending` 挂在 `Forge.Pending` 上，逐 token 更新 `Pending.Code` 并发快照；末尾走 `svc.CreatePending(ID=pendingID)`。仅元数据路径不流，但仍发一次最终快照。
- chat 层 `runner.go` 是 `chat.message` 的唯一发布事实源（`publishMessageSnapshot` / `writeAndPublish` / `emitFatalError`）；`stream.go` 与 `tools.go` 通过 closure 调它，从不自己 `bridge.Publish`。
- pre-LLM 错误（MODEL_NOT_CONFIGURED / API_KEY_PROVIDER_NOT_FOUND / LLM_PROVIDER_ERROR）也走 stub Message 路径——`status="error"` + `errorCode/errorMessage` 填好。所有错误现都装进 `chat.message` 快照里。

**已删除事件（Phase 6 之前 12 个）**：

- chat 域：`chat.reasoning_token` / `chat.token` / `chat.tool_call_start` / `chat.tool_call` / `chat.tool_result` / `chat.done` / `chat.error`
- conversation 域：`conversation.title_updated`
- forge 域：`forge.code_streaming` / `forge.created` / `forge.pending_created` / `forge.metadata_updated`

旧事件的所有信息都在新 entity 快照里：tokens 体现为 text/reasoning block 内容生长；tool_call_start vs tool_call vs tool_result 体现为 block 序列演化；done/error 体现为 message.status + stopReason + errorCode/errorMessage；conversation.title_updated 体现为 conversation.title + autoTitled 字段。

---

### Phase 4：工作流能力

| 事件名 | 用途 | 过滤 key | 状态 |
|---|---|---|---|
| `workflow.run_started` | 工作流开始运行 | `flowrunId` | ⬜ |
| `workflow.node_started` | 某节点开始执行 | `flowrunId` | ⬜ |
| `workflow.node_completed` | 某节点完成 | `flowrunId` | ⬜ |
| `workflow.run_completed` | 工作流运行完成 | `flowrunId` | ⬜ |
| `workflow.run_failed` | 工作流运行失败 | `flowrunId` | ⬜ |

---

### Phase 5：智能化能力

| 事件名 | 用途 | 过滤 key | 状态 |
|---|---|---|---|
| `intent.identified` | 意图识别结果 | `conversationId` | ⬜ |
| `knowledge.indexing_progress` | 知识库索引进度 | `knowledgeBaseId` | ⬜ |
| ~~`mcp.server_connected`~~ | 已被 entity-state `mcp` 事件取代（详见上方 Phase 4 准备件）| — | ✅ |

**Catalog 不发 SSE**：catalog 是内部组件，对前端透明。源数据变化（forge/skill/mcp/subagent）由各自 SSE 通知 UI；catalog 是后台派生 cache，前端不需要知道它何时刷新。详见 [`../service-design-documents/catalog.md`](../service-design-documents/catalog.md) §13。

# Entity Types 契约 — 一眼索引

**关联**：
- [`./fsd-layers.md`](./fsd-layers.md) — 层定义 / slice 清单
- [`./cross-cutting.md`](./cross-cutting.md) — DIP / SSE / queryKeys
- [`../service-contract-documents/api-design.md`](../service-contract-documents/api-design.md) — 后端 REST API 权威（§N3：响应字段 camelCase）

**定位**：**13 个 REST entity 的 TS 接口 ↔ 后端端点一眼对齐**（apikey / conversation / document / flowrun / function / handler / mcp / memory / model-config / relation / skill / user / workflow）；另有 **2 个非 REST entity**（session / settings，本地持久化，无后端端点）单独列在末节。字段细节 / mutation 参数类型看各 `entities/<name>/model/types.ts`。

**§N3 遵守**：前端所有字段名 = 后端 API json tag（camelCase）。DB snake_case 由后端 repo 层转换，前端不感知。

**协议变更同步点（§F1）**：后端加字段 / 改端点 → 同步更新本文件对应行。

---

## 1. apikey

**文件**：`entities/apikey/model/types.ts`
**后端 domain**：`internal/domain/apikey/`
**ID 前缀**：`aki_`

| 主类型 | 关键字段 | 对应端点 |
|---|---|---|
| `ApiKey` | `id / userId / provider / displayName / keyMasked / baseUrl / apiFormat / testStatus("pending"\|"ok"\|"error") / testError / lastTestedAt / modelsFound / isDefault` | `GET /api/v1/api-keys` / `GET /api/v1/api-keys/{id}` |
| `CreateApiKeyBody` | `provider / displayName / key / baseUrl? / apiFormat?` | `POST /api/v1/api-keys` |
| `UpdateApiKeyPatch` | `displayName? / baseUrl? / key? / isDefault?` | `PATCH /api/v1/api-keys/{id}` |
| `TestApiKeyResult` | `ok / message / latencyMs / modelsFound` | `POST /api/v1/api-keys/{id}:test` |

---

## 2. conversation（含 message / block）

**文件**：`entities/conversation/model/types.ts`
**后端 domain**：`internal/domain/chat/` + `internal/domain/eventlog/`
**ID 前缀**：`cv_`（conversation）/ `msg_`（message）/ `blk_`（block）

| 主类型 | 关键字段 | 对应端点 |
|---|---|---|
| `Conversation` | `id / title / autoTitled / systemPrompt? / summary? / attachedDocuments? / archived / pinned / modelOverride?` | `GET /api/v1/conversations` / `GET /api/v1/conversations/{id}` |
| `CreateConversationBody` | `title?` | `POST /api/v1/conversations` |
| `UpdateConversationPatch` | `title? / systemPrompt? / attachedDocuments? / archived? / pinned? / modelOverride?` | `PATCH /api/v1/conversations/{id}` |
| `Message` | `id / conversationId / role / status / parentBlockId / blocks / attachments / inputTokens? / outputTokens? / modelId?` | `GET /api/v1/conversations/{id}/messages` |
| `SendMessageBody` | `content / attachmentIds?` | `POST /api/v1/conversations/{id}/messages:send` |
| `Block` | `id / messageId / parentId / type(BlockType) / attrs / content / status(BlockStatus) / durationMs / children` | SSE 增量（非独立 REST 端点）|

**BlockType**（7 封闭枚举，后端 `eventlog.go` 权威）：`text` / `reasoning` / `tool_call` / `tool_result` / `progress` / `message` / `compaction`

**BlockStatus**（4 封闭枚举，单向流转）：`streaming → completed | error | cancelled`

---

## 3. document

**文件**：`entities/document/model/types.ts`
**后端 domain**：`internal/domain/document/`
**ID 前缀**：`doc_`

| 主类型 | 关键字段 | 对应端点 |
|---|---|---|
| `Document` | `id / userId / parentId / name / description / content / tags / position / path / sizeBytes` | `GET /api/v1/documents/{id}` |
| `DocTreeNode` | `id / userId / parentId / name / description / tags / position / path / sizeBytes`（无 content）| `GET /api/v1/documents/tree` |
| `CreateDocumentBody` | `name / parentId? / content? / description? / tags?` | `POST /api/v1/documents` |
| `UpdateDocumentPatch` | `name? / content? / description? / tags?` | `PATCH /api/v1/documents/{id}` |
| `MoveDocumentVars` | `id / parentId / position?` | `PATCH /api/v1/documents/{id}` |

---

## 4. flowrun

**文件**：`entities/flowrun/model/types.ts`
**后端 domain**：`internal/domain/flowrun/`
**ID 前缀**：`fr_`（flowrun）/ `frn_`（node）

| 主类型 | 关键字段 | 对应端点 |
|---|---|---|
| `FlowRun` | `id / userId / workflowId / versionId / triggerKind / status(FlowRunStatus) / startedAt / endedAt? / elapsedMs / pausedState? / dryRun` | `GET /api/v1/flowruns` / `GET /api/v1/flowruns/{id}` |
| `FlowRunNode` | `id / status(FlowRunNodeStatus) / nodeId / nodeType / input / output? / elapsedMs / attempts / conversationId?` | `GET /api/v1/flowruns/{id}/nodes` |
| `FlowRunsParams` | `workflowId? / status? / triggerKind? / cursor? / limit?` | query params |
| `ApproveNodeVars` | `runId / nodeId / decision? / reason?` | `POST /api/v1/flowruns/{id}/nodes/{nodeId}:approve` |

**FlowRunStatus**：`running` / `paused` / `completed` / `failed` / `cancelled`

**FlowRunNodeStatus**：`pending` / `running` / `ok` / `failed` / `cancelled` / `timeout` / `skipped`

---

## 5. function

**文件**：`entities/function/model/types.ts`
**后端 domain**：`internal/domain/function/`
**ID 前缀**：`fn_`（function）/ `fnv_`（version）

**注意**：TS 类型名 `FunctionEntity`（避让 JS 内置 `Function`）。

| 主类型 | 关键字段 | 对应端点 |
|---|---|---|
| `FunctionEntity` | `id / userId / name / description / tags / activeVersionId / envStatus? / pending?` | `GET /api/v1/functions` / `GET /api/v1/functions/{id}` |
| `FunctionVersion` | `id / functionId / status / code / parameters / returnSchema / dependencies / pythonVersion / envId / envStatus / envSyncStage` | `GET /api/v1/functions/{id}/versions` |
| `RunFunctionVars` | `id / inputs` | `POST /api/v1/functions/{id}:run` |
| `RunFunctionResult` | `output / elapsedMs` | 同上响应 |

**EnvStatus**：`pending` / `syncing` / `ready` / `failed` / `evicted`

**VersionStatus**：`pending` / `accepted` / `rejected`

---

## 6. handler

**文件**：`entities/handler/model/types.ts`
**后端 domain**：`internal/domain/handler/`
**ID 前缀**：`hd_`（handler）/ `hdv_`（version）

| 主类型 | 关键字段 | 对应端点 |
|---|---|---|
| `Handler` | `id / userId / name / description / tags / activeVersionId / envStatus? / configState? / liveInstances?` | `GET /api/v1/handlers` / `GET /api/v1/handlers/{id}` |
| `HandlerVersion` | `id / handlerId / status / imports / initBody / methods / initArgsSchema / dependencies / envId / envStatus` | `GET /api/v1/handlers/{id}/versions` |
| `HandlerConfig` | `configState / config` | `GET /api/v1/handlers/{id}/config` |
| `CallHandlerVars` | `id / method / args` | `POST /api/v1/handlers/{id}:call` |

**ConfigState**：`unconfigured` / `partially_configured` / `ready`

---

## 7. mcp

**文件**：`entities/mcp/model/types.ts`
**后端 domain**：`internal/domain/mcp/`
**主键**：`name`（字符串，无 ID 前缀）

| 主类型 | 关键字段 | 对应端点 |
|---|---|---|
| `McpServer` | `name / status("disconnected"\|"connecting"\|"ready"\|"degraded"\|"failed") / pid? / connectedAt? / consecutiveFailures / totalCalls / tools` | `GET /api/v1/mcp-servers` |
| `ToolDef` | `serverName / name / description / inputSchema` | 嵌入 McpServer.tools |
| `ReconnectMcpResult` | `name / status` | `POST /api/v1/mcp-servers/{name}:reconnect` |

---

## 8. memory

**文件**：`entities/memory/model/types.ts`
**后端 domain**：`internal/domain/memory/`
**ID 前缀**：`mem_`
**主键**：`name`（字符串）

| 主类型 | 关键字段 | 对应端点 |
|---|---|---|
| `Memory` | `id / name / type("user"\|"feedback"\|"project"\|"reference") / description / content / pinned / source("user"\|"ai") / accessCount` | `GET /api/v1/memories` / `GET /api/v1/memories/{name}` |
| `CreateMemoryBody` | `name / type / description / content / pinned? / source?` | `POST /api/v1/memories` |
| `UpdateMemoryBody` | `description? / content? / type? / pinned?` | `PATCH /api/v1/memories/{name}` |
| `PinMemoryVars` | `name / pinned` | `PATCH /api/v1/memories/{name}` |

---

## 9. model-config

**文件**：`entities/model-config/model/types.ts`
**后端 domain**：`internal/domain/model/`
**ID 前缀**：`mc_`

| 主类型 | 关键字段 | 对应端点 |
|---|---|---|
| `ModelConfig` | `id / scenario / provider / modelId` | `GET /api/v1/model-configs` |
| `Provider` | `name / displayName / category / defaultBaseUrl? / baseUrlRequired` | `GET /api/v1/providers`（静态白名单）|
| `Scenario` | `name` | `GET /api/v1/scenarios`（后端权威白名单）|
| `UpsertModelConfigBody` | `provider / modelId` | `PUT /api/v1/model-configs/{scenario}`（N6：无论新建/更新返 200）|

---

## 10. relation

**文件**：`entities/relation/model/types.ts`
**后端 domain**：`internal/domain/relation/`
**ID 前缀**：`rel_`

| 主类型 | 关键字段 | 对应端点 |
|---|---|---|
| `Relation` | `id / userId / fromKind / fromId / toKind / toId / kind(RelationKind) / attrs?` | `GET /api/v1/relations` |
| `Neighborhood` | `nodes(GraphNode[]) / edges(Relation[])` | `GET /api/v1/relations/neighborhood?kind=&id=` |
| `GraphNode` | `kind / id / label / sub?` | 嵌入 Neighborhood |

**RelationKind**（封闭枚举）：`conversation_forged_entity` / `conversation_edited_entity` / `workflow_uses_function` / `workflow_uses_handler` / `workflow_uses_mcp` / `workflow_uses_skill` / `workflow_uses_document` / `document_links_entity`

---

## 11. skill

**文件**：`entities/skill/model/types.ts`
**后端 domain**：`internal/domain/skill/`
**主键**：`name`（字符串，无 ID 前缀）

| 主类型 | 关键字段 | 对应端点 |
|---|---|---|
| `Skill` | `name / source / dirPath / bodyPath / description / frontmatter / loadedAt` | `GET /api/v1/skills` / `GET /api/v1/skills/{name}` |
| `SkillFrontmatter` | `name / description / whenToUse? / allowedTools? / disableModelInvocation? / userInvocable? / paths? / agent? / arguments?` | 嵌入 Skill |

---

## 12. user

**文件**：`entities/user/model/types.ts`
**后端 domain**：`internal/domain/user/`
**ID 前缀**：`u_`

| 主类型 | 关键字段 | 对应端点 |
|---|---|---|
| `User` | `id / username / displayName / avatarColor / language / lastUsedAt` | `GET /api/v1/users` / `GET /api/v1/users/{id}` |
| `CreateUserBody` | `username / displayName? / avatarColor? / language?` | `POST /api/v1/users` |
| `UpdateUserPatch` | `displayName? / avatarColor? / language?` | `PATCH /api/v1/users/{id}` |

---

## 13. workflow

**文件**：`entities/workflow/model/types.ts`
**后端 domain**：`internal/domain/workflow/`
**ID 前缀**：`wf_`（workflow）/ `wfv_`（version）

| 主类型 | 关键字段 | 对应端点 |
|---|---|---|
| `Workflow` | `id / userId / name / description / tags / enabled / concurrency / needsAttention / activeVersionId / liveRuns? / lastFiredAt?` | `GET /api/v1/workflows` / `GET /api/v1/workflows/{id}` |
| `WorkflowVersion` | `id / workflowId / status / graph(JSON) / graphParsed? / changeReason / forgedInConversationId?` | `GET /api/v1/workflows/{id}/versions` |
| `EditWorkflowVars` | `ops(WorkflowEditOp[]) / changeReason?` | `POST /api/v1/workflows/{id}:edit` |
| `RunWorkflowVars` | `id / input?` | `POST /api/v1/workflows/{id}:run` |
| `CapabilityCheckResult` | `ok / issues(CapabilityIssue[])` | `POST /api/v1/workflows/{id}:check-capabilities` |

---

## 非 REST entities（本地持久化）

### entities/session

**文件**：`entities/session/model/sessionStore.ts`（zustand + persist，key `"forgify-session"`）

| 字段 | 类型 | 说明 |
|---|---|---|
| `currentUserId` | `string \| null` | localStorage 持久化；`null` = 未登录 |
| `status` | `"loading" \| "onboarding" \| "ready"` | boot gate 驱动；不持久化 |

`resolveSession()`（`entities/session/model/resolve.ts`）：基于 fresh `/users` 响应重置 currentUserId + status，是唯一合法 writer。

### entities/settings

**文件**：`entities/settings/model/settingsStore.ts`（zustand + persist，key `"forgify-settings"` v1）

| 字段 | 类型 | 默认 |
|---|---|---|
| `theme` | `"system" \| "light" \| "dark"` | `"system"` |
| `accent` | `"claude" \| "blue" \| "ink" \| "green" \| "purple"` | `"claude"` |
| `density` | `"compact" \| "cozy" \| "comfortable"` | `"cozy"` |
| `lang` | `"zh" \| "en"` | `navigator.language` 检测 |
| `reasoningDefault` | `"collapsed" \| "expanded"` | `"collapsed"` |

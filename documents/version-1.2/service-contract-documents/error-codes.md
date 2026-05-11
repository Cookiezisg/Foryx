# Error Codes — V1.2 错误码一眼索引

**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- **配套实现**：`internal/transport/httpapi/response/errmap.go`

**定位**：**全仓所有错误码、HTTP 状态、sentinel 一眼索引**。每个 code 的详细触发条件、details 字段，**去对应 domain 的 `service-design-documents/<domain>.md` 看**。

---

## 全局约定

### 错误码命名
- 全部大写 + 下划线：`SCREAMING_SNAKE_CASE`
- 按 domain 加前缀（除非通用）：`FUNCTION_NOT_FOUND`、`API_KEY_INVALID`

### 三层错误模型

```
┌─────────────────────────────────────────────┐
│ domain/<name>/*.go                            │
│   var ErrNotFound = errors.New("...")         │  ← Sentinel
└──────────────────┬───────────────────────────┘
                   │ errors.Is 匹配
                   ↓
┌─────────────────────────────────────────────┐
│ transport/httpapi/response/errmap.go          │
│   errTable: sentinel → (HTTP, code)           │  ← 翻译表
└──────────────────┬───────────────────────────┘
                   │
                   ↓
            { "error": { "code", "message", "details" }}
```

### 添加新错误码的流程（三步缺一不可）

1. 在 `domain/<name>/*.go` 声明 sentinel：`var ErrNotFound = errors.New("<domain>: not found")`
2. 在 `response/errmap.go` 加映射行：`<domain>.ErrNotFound: {http.StatusNotFound, "<DOMAIN>_NOT_FOUND"}`
3. 在本文档对应 domain 段加一行

handler 侧调 `response.FromDomainError(w, log, err)` 自动翻译。

### 兜底
未注册到 `errTable` 的错误自动降级为 `500 INTERNAL_ERROR`，原始 message **不**暴露给客户端（防泄漏实现细节）。

---

## 错误码清单

> **状态**：⬜ 未定义 | ✅ 已实现

### 通用（Phase 1）

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `INVALID_REQUEST` | 400 | `errorsdomain.ErrInvalidRequest` | JSON 坏 / 字段缺 / cursor 格式错 | ✅ |
| `INTERNAL_ERROR` | 500 | (未映射 fallback) | errmap 兜底；未匹配 sentinel 自动降级到此（不需要专门 sentinel）| ✅ |
| `INTERNAL_ERROR` | 500 | `reqctxpkg.ErrMissingUserID` | auth middleware 未跑（接线 bug）。显式登记以抑制 "unmapped" 警告 | ✅ |
| `INTERNAL_ERROR` | 500 | `reqctxpkg.ErrMissingConversationID` | chat-runner 未在 ctx 印 conversation ID（接线 bug）。todo / ask 工具依赖此 ID | ✅ |
| `INTERNAL_ERROR` | 500 | `cryptoinfra.ErrUnsupportedVersion` | DB 中密文版本前缀（如 `v2:`）超出当前 encryptor 支持范围（升降级 / 数据损坏）| ✅ |
| `NOT_FOUND` | 404 | (middleware 直接发，不走 errmap) | 路由未匹配 | ✅ |
| `SEQ_TOO_OLD` | 410 | (handler inline，不走 errmap) | SSE replay buffer 超时——client 经 `/api/v1/conversations/{id}/eventlog?from=<seq>` refetch（事件日志）或重订（通知）；handler `/eventlog`+`/notifications` 直写 wire | ✅ |
| `CLIENT_CLOSED` | 499 | `context.Canceled` (stdlib) | 客户端断开（浏览器 hard refresh / 关 tab）；登记仅为抑制 unmapped warning，响应反正没人收 | ✅ |
| `REQUEST_TIMEOUT` | 504 | `context.DeadlineExceeded` (stdlib) | 请求超时；同样登记仅为抑制 unmapped warning | ✅ |

---

### Phase 2：基础对话能力

#### apikey ✅
详见 [`../service-design-documents/apikey.md`](../service-design-documents/apikey.md) §13。

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `API_KEY_NOT_FOUND` | 404 | `apikeydomain.ErrNotFound` | id 查不到 | ✅ |
| `API_KEY_PROVIDER_NOT_FOUND` | 404 | `apikeydomain.ErrNotFoundForProvider` | 当前用户 该 provider 无活跃 key | ✅ |
| `INVALID_PROVIDER` | 400 | `apikeydomain.ErrInvalidProvider` | provider 不在 11 白名单 | ✅ |
| `BASE_URL_REQUIRED` | 400 | `apikeydomain.ErrBaseURLRequired` | ollama / custom 没填 baseURL | ✅ |
| `API_FORMAT_REQUIRED` | 400 | `apikeydomain.ErrAPIFormatRequired` | custom 没填 apiFormat | ✅ |
| `KEY_REQUIRED` | 400 | `apikeydomain.ErrKeyRequired` | 创建时 key 空 | ✅ |

#### model ✅
详见 [`../service-design-documents/model.md`](../service-design-documents/model.md)。

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `MODEL_NOT_CONFIGURED` | 422 | `modeldomain.ErrNotConfigured` | chat 调 PickForChat 时用户未配过 | ✅ |
| `INVALID_SCENARIO` | 400 | `modeldomain.ErrInvalidScenario` | PUT path 的 scenario 不在白名单 | ✅ |
| `PROVIDER_REQUIRED` | 400 | `modeldomain.ErrProviderRequired` | PUT body provider 空 | ✅ |
| `MODEL_ID_REQUIRED` | 400 | `modeldomain.ErrModelIDRequired` | PUT body modelId 空 | ✅ |

#### conversation ✅
详见 [`../service-design-documents/conversation.md`](../service-design-documents/conversation.md)。

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `CONVERSATION_NOT_FOUND` | 404 | `convdomain.ErrNotFound` | id 查不到（Get/Rename/Delete）| ✅ |

#### chat ✅

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `MESSAGE_NOT_FOUND` | 404 | `chatdomain.ErrMessageNotFound` | 消息 id 不存在 | ✅ |
| `STREAM_NOT_FOUND` | 404 | `chatdomain.ErrStreamNotFound` | 取消不存在的流 | ✅ |
| `STREAM_IN_PROGRESS` | 409 | `chatdomain.ErrStreamInProgress` | 同一对话已有流在跑 | ✅ |
| `LLM_PROVIDER_ERROR` | 502 | `llminfra.ErrProviderError` | 上游 LLM 故障——infra/llm classifyHTTPError 兜底所有非 401/429/400/404 的 5xx | ✅ |
| `ATTACHMENT_TOO_LARGE` | 413 | `chatdomain.ErrAttachmentTooLarge` | 附件超过 50MB | ✅ |
| `ATTACHMENT_TYPE_UNSUPPORTED` | 415 | `chatdomain.ErrAttachmentTypeUnsupported` | 无法处理的文件格式 | ✅ |
| `ATTACHMENT_PARSE_FAILED` | 422 | `chatdomain.ErrAttachmentParseFailed` | 文件损坏或解析失败 | ✅ |
| `LLM_AUTH_FAILED` | 401 | `llminfra.ErrAuthFailed` | LLM provider 返 401（API key 失效）；errors.Is 触发 apikey.MarkInvalid | ✅ |
| `LLM_RATE_LIMITED` | 429 | `llminfra.ErrRateLimited` | LLM provider 返 429（速率限制）| ✅ |
| `LLM_BAD_REQUEST` | 400 | `llminfra.ErrBadRequest` | LLM provider 返 400（请求体非法）| ✅ |
| `LLM_MODEL_NOT_FOUND` | 404 | `llminfra.ErrModelNotFound` | 指定 modelID 在 provider 不存在 | ✅ |

**Message.errorCode 字段值**（Phase 5 新增字段，仅 status="error" 时填；不走 HTTP 路径，由 SSE `chat.message` 事件携带，前端按 code 解释失败原因）：

| Code | 触发点 | 含义 |
|---|---|---|
| `MODEL_NOT_CONFIGURED` | `processTask` PickForChat 失败（`llmclient.ErrPickModel`）| 用户尚未配置 chat scenario 模型 |
| `API_KEY_PROVIDER_NOT_FOUND` | `processTask` ResolveCredentials 失败（`llmclient.ErrResolveCreds`）| 当前 provider 无活跃 key |
| `LLM_BUILD_FAILED` | `processTask` LLMFactory.Build 失败（`llmclient.ErrBuildClient`）| 上游 LLM 客户端构造失败（如 ollama / custom 缺 BaseURL）|
| `LLM_PROVIDER_ERROR` | `processTask` 三段解析其他失败（fallback）| Resolve 阶段未匹配三个 sentinel 的兜底 |
| `LLM_STREAM_ERROR` | `streamLLM` 收到 EventError（非取消）| LLM 流式响应中途出错（401 / 网络等）|
| `HISTORY_EXTEND_FAILED` | `agentRun` extendHistory 失败 | tool result 注入历史时 JSON 序列化失败（极罕见）|
| `INTERNAL_ERROR` | writeAndPublish 写库 fatal 失败 | 终态 message 落库失败——message 已无法持久化 |

---

### Phase 3：工具锻造能力 — function trinity (forge_redesign Plan 01)

#### function ✅
详见 [`../service-design-documents/function.md`](../service-design-documents/function.md) §10 + redesign topic [`../adhoc-topic-documents/forge_redesign/02-function.md`](../adhoc-topic-documents/forge_redesign/02-function.md)。

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `FUNCTION_NOT_FOUND` | 404 | `functiondomain.ErrNotFound` | id 查不到 | ✅ |
| `FUNCTION_NAME_DUPLICATE` | 409 | `functiondomain.ErrDuplicateName` | 创建/改名时撞名(partial UNIQUE 兜底) | ✅ |
| `FUNCTION_VERSION_NOT_FOUND` | 404 | `functiondomain.ErrVersionNotFound` | revert/get version 版本号或 id 不存在 | ✅ |
| `FUNCTION_PENDING_NOT_FOUND` | 404 | `functiondomain.ErrPendingNotFound` | accept/reject 时无 pending | ✅ |
| `FUNCTION_PENDING_CONFLICT` | 409 | `functiondomain.ErrPendingConflict` | edit_function 时已有未处理 pending | ✅ |
| `FUNCTION_RUN_FAILED` | 422 | `functiondomain.ErrRunFailed` | sandbox 基础设施错误(≠ ok=false 的用户代码失败,后者经 ExecutionResult.OK=false + ErrorMsg 返) | ✅ |
| `FUNCTION_AST_PARSE_FAILED` | 422 | `functiondomain.ErrASTParseError` | final validation 失败(无 top-level def / D7 handler-import 黑名单 / 签名一致性) | ✅ |
| `FUNCTION_OP_INVALID` | 400 | `functiondomain.ErrOpInvalid` | 单 op apply 失败(未知 op 类型 / payload 形状错 / incremental 校验破规则) | ✅ |
| `FUNCTION_NO_ACTIVE_VERSION` | 422 | `functiondomain.ErrNoActiveVersion` | RunFunction 时 Function.ActiveVersionID == "" (Create 自动 accept v1,该错主要给手动构造 entity 的边角 case) | ✅ |
| `FUNCTION_ENV_NOT_READY` | 422 | `functiondomain.ErrEnvNotReady` | ActiveVersion 的 venv 处于非-ready;等 entity-state 翻转或 :resync | ✅ |
| `FUNCTION_ENV_FAILED` | 422 | `functiondomain.ErrEnvFailed` | ActiveVersion 的 env=failed(venv 装包失败);EnvError 含 sandbox stderr | ✅ |
| `FUNCTION_DEPENDENCY_RESOLUTION` | 422 | `functiondomain.ErrDependencyResolution` | uv 无法解析依赖(包名错 / 版本冲突 / 网络);EnvError 含完整 stderr | ✅ |
| `FUNCTION_SANDBOX_UNAVAILABLE` | 503 | `functiondomain.ErrSandboxUnavailable` | sandbox v2 Bootstrap 失败(mise binary 缺) | ✅ |
| `FUNCTION_EXECUTION_NOT_FOUND` | 404 | `functiondomain.ErrExecutionNotFound` | get_function_execution / GET /function-executions/{id} 查不到 | ✅ |

> 历史 `TOOL_*` / `FORGE_*` wire codes 已随 forge 代码路径在 Plan 01 Phase 7 同步移除。trinity domain 统一用 `FUNCTION_*` 前缀。

---

### Phase 5：System Tool 第二代（2026-05-04）

> **NB：filesystem / search / shell 工具家族不向 errmap 注册**——所有失败以友好字符串返 LLM（吃在 chat.message 的 tool_result block 里），不到 handler。详见各家族 design doc 的 §6 安全边界 + §8 错误返回模式：[`filesystem.md`](../service-design-documents/filesystem.md) / [`search.md`](../service-design-documents/search.md) / [`shell.md`](../service-design-documents/shell.md)。**例外**：web 家族的 BYOK provider HTTP 状态分类 sentinel **登记**（让 `errors.Is` 触发 `apikey.MarkInvalid`，UI 自动翻 "error"）；下方 todo / ask / web 三类有独立 HTTP 端点或显式 errmap 行。

#### web 🔁（控制流 sentinel，handler 不可达）
BYOK web search providers（Brave / Serper / Tavily / Bocha）的 HTTP 状态分类 sentinel **不进 errmap**——`tool/web/search.go::tryBYOKProvider` 内部 catch 后 fallback 到下一 provider / MCP tier，永不冒泡到 handler。仅 `errors.Is` 内部判定用（`markInvalidIfAuthErr` 触发 `apikey.MarkInvalid` 替代 string match）。

| Sentinel | 状态 | 用途 |
|---|---|---|
| `webtool.ErrAuthFailed` | 🔁 | provider 401（API key 失效）→ 触发 apikey.MarkInvalid + 落到下一 provider |
| `webtool.ErrRateLimited` | 🔁 | provider 429（速率限制）→ 落到下一 provider |
| `webtool.ErrUpstreamHTTP` | 🔁 | provider 其他 5xx → 落到下一 provider |

#### todo ✅
详见 [`../service-design-documents/todo.md`](../service-design-documents/todo.md)。

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `TODO_NOT_FOUND` | 404 | `tododomain.ErrNotFound` | TodoGet/Update/Delete 时 ID 不存在；**也覆盖跨 conversation 访问场景**（防存在性泄漏，统一返 NotFound 而非 mismatch）| ✅ |
| `TODO_SUBJECT_REQUIRED` | 400 | `tododomain.ErrSubjectRequired` | TodoCreate / TodoUpdate 的 subject 字段为空 | ✅ |
| `TODO_INVALID_STATUS` | 400 | `tododomain.ErrInvalidStatus` | TodoUpdate status 不在 4 值白名单（pending/in_progress/completed/deleted）| ✅ |

#### ask ✅
AskUserQuestion 的答案投递端点 `POST /api/v1/conversations/{id}/answers` 错误。

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `ASK_NO_PENDING_QUESTION` | 404 | `ask.ErrNoPendingQuestion` | 投递的 toolCallId 未在 Service.Wait 注册（已超时 / 已答 / 拼错 / 二次答均走此）| ✅ |
| `ASK_TIMEOUT` | 504 | `ask.ErrTimeout` | （Service 内部）AskUserQuestion 工具 Wait 超过 5 分钟；当前实现工具内部转为友好字符串而非上抛，因此实际不到 handler——保留登记便于将来若改语义 | ✅ |

> ASK_* 端点错误不属于任一 domain entity，归属 app/ask 服务（in-memory 会合，无持久化）。

---

### Phase 4：工作流能力

#### workflow ⬜

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `WORKFLOW_NOT_FOUND` | 404 | `workflow.ErrNotFound` | | ⬜ |
| `WORKFLOW_INVALID_DEFINITION` | 400 | `workflow.ErrInvalidDefinition` | DAG 校验失败（环 / 孤儿节点）| ⬜ |
| `WORKFLOW_NODE_NOT_FOUND` | 404 | `workflow.ErrNodeNotFound` | | ⬜ |

---

### Phase 4 准备件（2026-05-05 设计完成 / 待实施）

#### subagent ✅

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `SUBAGENT_TYPE_NOT_FOUND` | 404 | `subagentdomain.ErrTypeNotFound` | spawn 时 subagent_type 不在注册表 | ✅ |
| `SUBAGENT_RECURSION` | 422 | `subagentdomain.ErrRecursionAttempt` | subagent 内尝试再 spawn（防嵌套）| ✅ |

> 注：`subagentdomain.ErrMaxTurnsExceeded` / `ErrCancelled` **不上抛 handler**，由 SubagentTool.Execute 转友好字符串返 LLM。
> 注：原 `SUBAGENT_RUN_NOT_FOUND` 行已删——schema 统一后 `/subagent-runs/{id}` 端点不存在，sub-run 数据走 `/conversations/{id}/messages`（attrs.kind=subagent_run 过滤）。

#### mcp ✅

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `MCP_SERVER_NOT_FOUND` | 404 | `mcpdomain.ErrServerNotFound` | server 名不在 mcp.json | ✅ |
| `MCP_SERVER_NOT_CONNECTED` | 409 | `mcpdomain.ErrServerNotConnected` | 调用未 connect 的 server | ✅ |
| `MCP_TOOL_NOT_FOUND` | 404 | `mcpdomain.ErrToolNotFound` | tool 名不在 server 的 tools/list | ✅ |
| `MCP_TOOL_CALL_FAILED` | 502 | `mcpdomain.ErrToolCallFailed` | server 自报失败（含 isError=true）| ✅ |
| `MCP_TOOL_CALL_TIMEOUT` | 504 | `mcpdomain.ErrToolCallTimeout` | per-call 超时（默认 30s，可 per-server override）| ✅ |
| `MCP_REGISTRY_ENTRY_NOT_FOUND` | 404 | `mcpdomain.ErrRegistryEntryNotFound` | install 时 registry name 不存在 | ✅ |
| `MCP_REQUIRED_ENV_MISSING` | 422 | `mcpdomain.ErrRequiredEnvMissing` | install 时 required env 未填全 | ✅ |
| `MCP_REQUIRED_ARGS_MISSING` | 422 | `mcpdomain.ErrRequiredArgsMissing` | install 时 required args 未填全 | ✅ |
| `MCP_INSTALL_FAILED` | 502 | `mcpdomain.ErrInstallFailed` | npm install / uvx 安装命令失败 | ✅ |
| `MCP_ALREADY_INSTALLED` | 409 | `mcpdomain.ErrAlreadyInstalled` | install 时 server name 已存在 mcp.json（先卸再装）| ✅ |

> 注：Marketplace V3（2026-05-08 curated 化 / 2026-05-09 search→list 化）。`MCP_ALIAS_COLLISION`（无 alias 概念）+ `MCP_QUERY_REQUIRED`（V3 list 不再要 query）+ `MCP_MARKETPLACE_UNAVAILABLE`（curated 同步源永不失败）+ `MCP_HANDSHAKE_FAILED`（被 `MCP_SERVER_NOT_CONNECTED` 覆盖）相继移除。所有 sentinel + errmap 已对齐。

#### skill ✅

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `SKILL_NOT_FOUND` | 404 | `skilldomain.ErrSkillNotFound` | skill 名不在 ~/.forgify/skills/ | ✅ |
| `SKILL_INVALID_FRONTMATTER` | 422 | `skilldomain.ErrInvalidFrontmatter` | YAML 解析失败 / 必填缺 / fork 模式缺 agent | ✅ |
| `SKILL_BODY_TOO_LARGE` | 422 | `skilldomain.ErrBodyTooLarge` | SKILL.md body > 32 KB | ✅ |
| `SKILL_NAME_CONFLICT` | 409 | `skilldomain.ErrNameConflict` | POST /skills 创建同名（PUT 改 200 替换）| ✅ |
| `SKILL_INVALID_NAME` | 422 | `skilldomain.ErrInvalidName` | name 不符 `[a-z0-9][a-z0-9-]{0,63}` | ✅ |

> 注：5 个 sentinel + errmap 行 D7-1 全接（2026-05-06）。runtime 触发点全部接通（D7-3 Activate / D7-7 mutate / D7-7 Import）。allowed-tools 校验未注册 tool 设计上推迟到 V2（boot 顺序 race：skill scan 早于 tool 注册）。

#### catalog ✅

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `CATALOG_ALL_SOURCES_FAILED` | 503 | `catalogdomain.ErrAllSourcesFailed` | 全部 source（function/skill/mcp）同时挂；`POST /catalog:refresh` 时上抛 | ✅ |

> `ErrCoverageIncomplete` / `ErrGenerationFailed` 内部消化（3 次 retry + mechanical fallback），不上抛 handler。仅 `ErrAllSourcesFailed` 在所有 source 同时失败时透出 503。

#### sandbox ✅

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `SANDBOX_RUNTIME_NOT_SUPPORTED` | 422 | `sandboxdomain.ErrRuntimeNotSupported` | 没有 installer 注册该 kind | ✅ |
| `SANDBOX_RUNTIME_INSTALL_FAILED` | 502 | `sandboxdomain.ErrRuntimeInstallFailed` | mise install / playwright install 等失败 | ✅ |
| `SANDBOX_ENV_NOT_FOUND` | 404 | `sandboxdomain.ErrEnvNotFound` | 通过 owner / id 查不到 | ✅ |
| `SANDBOX_ENV_CREATE_FAILED` | 502 | `sandboxdomain.ErrEnvCreateFailed` | venv / node_modules / etc. 建失败 | ✅ |
| `SANDBOX_DEP_INSTALL_FAILED` | 502 | `sandboxdomain.ErrDepInstallFailed` | uv pip install / npm install 失败 | ✅ |
| `SANDBOX_SPAWN_FAILED` | 502 | `sandboxdomain.ErrSpawnFailed` | 子进程起不来 | ✅ |
| `SANDBOX_SPAWN_TIMEOUT` | 504 | `sandboxdomain.ErrSpawnTimeout` | once-spawn 超时 | ✅ |
| `SANDBOX_ENV_IN_USE` | 409 | `sandboxdomain.ErrEnvInUse` | Destroy 时 env 还在跑 | ✅ |
| `SANDBOX_INVALID_OWNER_ID` | 400 | `sandboxdomain.ErrInvalidOwnerID` | ownerID 格式不合法（D2 收紧）| ✅ |
| `SANDBOX_CMD_REQUIRED` | 400 | `sandboxdomain.ErrCmdRequired` | spawn 命令 cmd 字段必填 | ✅ |
| `SANDBOX_DOCKER_NOT_INSTALLED` | 422 | `sandboxdomain.ErrDockerNotInstalled` | docker CLI 不在 PATH；Forgify 不替用户装 Docker（系统服务）| ✅ |
| `SANDBOX_DOCKER_DAEMON_DOWN` | 503 | `sandboxdomain.ErrDockerDaemonDown` | docker CLI 在但 daemon 不响应（Mac/Win 没启 Docker Desktop / Linux dockerd inactive）| ✅ |

#### flowrun ⬜

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `FLOWRUN_NOT_FOUND` | 404 | `flowrun.ErrNotFound` | | ⬜ |
| `FLOWRUN_ALREADY_FINISHED` | 409 | `flowrun.ErrAlreadyFinished` | 取消已结束的运行 | ⬜ |

#### scheduler / trigger ⬜

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `TRIGGER_INVALID_CRON` | 400 | `scheduler.ErrInvalidCron` | cron 表达式错 | ⬜ |
| `TRIGGER_DUPLICATE` | 409 | `scheduler.ErrDuplicate` | 同一触发器重复注册 | ⬜ |

---

### Phase 5：智能化能力

#### knowledge ⬜

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `KNOWLEDGE_NOT_FOUND` | 404 | `knowledge.ErrNotFound` | | ⬜ |
| `DOCUMENT_NOT_FOUND` | 404 | `knowledge.ErrDocumentNotFound` | | ⬜ |
| `EMBEDDING_FAILED` | 502 | `knowledge.ErrEmbeddingFailed` | 向量化失败 | ⬜ |

#### mcp — 已提前交付 ✅ 见上方"Phase 4 准备件 / mcp"

#### skill — 已提前交付 ✅ 见上方"Phase 4 准备件 / skill"

#### intent ⬜

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `INTENT_AMBIGUOUS` | 422 | `intent.ErrAmbiguous` | 意图无法明确识别 | ⬜（待定）|

# Error Codes — V1.2 错误码一眼索引

**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- **配套实现**：`internal/transport/httpapi/response/errmap.go`

**定位**：**全仓所有错误码、HTTP 状态、sentinel 一眼索引**。每个 code 的详细触发条件、details 字段，**去对应 domain 的 `service-design-documents/<domain>.md` 看**。

---

## 全局约定

### 错误码命名
- 全部大写 + 下划线：`SCREAMING_SNAKE_CASE`
- 按 domain 加前缀（除非通用）：`TOOL_NOT_FOUND`、`API_KEY_INVALID`

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
| `INVALID_REQUEST` | 400 | `derrors.ErrInvalidRequest` | JSON 坏 / 字段缺 / cursor 格式错 | ✅ |
| `INTERNAL_ERROR` | 500 | `derrors.ErrInternal` | 兜底；未映射错误降级到此 | ✅ |
| `INTERNAL_ERROR` | 500 | `reqctxpkg.ErrMissingUserID` | auth middleware 未跑（接线 bug）。显式登记以抑制 "unmapped" 警告 | ✅ |
| `INTERNAL_ERROR` | 500 | `reqctxpkg.ErrMissingConversationID` | chat-runner 未在 ctx 印 conversation ID（接线 bug）。todo / ask 工具依赖此 ID | ✅ |
| `INTERNAL_ERROR` | 500 | `cryptoinfra.ErrUnsupportedVersion` | DB 中密文版本前缀（如 `v2:`）超出当前 encryptor 支持范围（升降级 / 数据损坏）| ✅ |
| `NOT_FOUND` | 404 | (middleware 直接发，不走 errmap) | 路由未匹配 | ✅ |

---

### Phase 2：基础对话能力

#### apikey ✅
详见 [`../service-design-documents/apikey.md`](../service-design-documents/apikey.md) §13。

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `API_KEY_NOT_FOUND` | 404 | `apikey.ErrNotFound` | id 查不到 | ✅ |
| `API_KEY_PROVIDER_NOT_FOUND` | 404 | `apikey.ErrNotFoundForProvider` | 当前用户 该 provider 无活跃 key | ✅ |
| `INVALID_PROVIDER` | 400 | `apikey.ErrInvalidProvider` | provider 不在 11 白名单 | ✅ |
| `BASE_URL_REQUIRED` | 400 | `apikey.ErrBaseURLRequired` | ollama / custom 没填 baseURL | ✅ |
| `API_FORMAT_REQUIRED` | 400 | `apikey.ErrAPIFormatRequired` | custom 没填 apiFormat | ✅ |
| `KEY_REQUIRED` | 400 | `apikey.ErrKeyRequired` | 创建时 key 空 | ✅ |
| `API_KEY_TEST_FAILED` | 422 | `apikey.ErrTestFailed` | 连通性测试失败 | ✅ |
| `API_KEY_INVALID` | 401 | `apikey.ErrInvalid` | 使用时 provider 返回 401 | ✅ |

#### model ✅
详见 [`../service-design-documents/model.md`](../service-design-documents/model.md)。

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `MODEL_NOT_CONFIGURED` | 422 | `model.ErrNotConfigured` | chat 调 PickForChat 时用户未配过 | ✅ |
| `INVALID_SCENARIO` | 400 | `model.ErrInvalidScenario` | PUT path 的 scenario 不在白名单 | ✅ |
| `PROVIDER_REQUIRED` | 400 | `model.ErrProviderRequired` | PUT body provider 空 | ✅ |
| `MODEL_ID_REQUIRED` | 400 | `model.ErrModelIDRequired` | PUT body modelId 空 | ✅ |

#### conversation ✅
详见 [`../service-design-documents/conversation.md`](../service-design-documents/conversation.md)。

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `CONVERSATION_NOT_FOUND` | 404 | `conversation.ErrNotFound` | id 查不到（Get/Rename/Delete）| ✅ |

#### chat ✅

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `MESSAGE_NOT_FOUND` | 404 | `chat.ErrMessageNotFound` | 消息 id 不存在 | ✅ |
| `STREAM_NOT_FOUND` | 404 | `chat.ErrStreamNotFound` | 取消不存在的流 | ✅ |
| `STREAM_IN_PROGRESS` | 409 | `chat.ErrStreamInProgress` | 同一对话已有流在跑 | ✅ |
| `LLM_PROVIDER_ERROR` | 502 | `chat.ErrProviderUnavailable` | 上游 LLM 故障（非 401）| ✅ |
| `ATTACHMENT_TOO_LARGE` | 413 | `chat.ErrAttachmentTooLarge` | 附件超过 50MB | ✅ |
| `ATTACHMENT_TYPE_UNSUPPORTED` | 415 | `chat.ErrAttachmentTypeUnsupported` | 无法处理的文件格式 | ✅ |
| `ATTACHMENT_PARSE_FAILED` | 422 | `chat.ErrAttachmentParseFailed` | 文件损坏或解析失败 | ✅ |
| `VISION_NOT_SUPPORTED` | 422 | `chat.ErrVisionNotSupported` | 当前 provider 不支持图片 | ✅ |

**Message.errorCode 字段值**（Phase 5 新增字段，仅 status="error" 时填；不走 HTTP 路径，由 SSE `chat.message` 事件携带，前端按 code 解释失败原因）：

| Code | 触发点 | 含义 |
|---|---|---|
| `MODEL_NOT_CONFIGURED` | `processTask` PickForChat 失败 | 用户尚未配置 chat scenario 模型 |
| `API_KEY_PROVIDER_NOT_FOUND` | `processTask` ResolveCredentials 失败 | 当前 provider 无活跃 key |
| `LLM_PROVIDER_ERROR` | `processTask` LLMFactory.Build 失败 | 上游 LLM 客户端构造失败 |
| `LLM_STREAM_ERROR` | `streamLLM` 收到 EventError（非取消）| LLM 流式响应中途出错（401 / 网络等）|
| `HISTORY_EXTEND_FAILED` | `agentRun` extendHistory 失败 | tool result 注入历史时 JSON 序列化失败（极罕见）|
| `INTERNAL_ERROR` | writeAndPublish 写库 fatal 失败 | 终态 message 落库失败——message 已无法持久化 |

---

### Phase 3：工具锻造能力

#### tool ✅
详见 [`../service-design-documents/forge.md`](../service-design-documents/forge.md) §12。

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `TOOL_NOT_FOUND` | 404 | `tool.ErrNotFound` | id 查不到 | ✅ |
| `TOOL_NAME_DUPLICATE` | 409 | `tool.ErrDuplicateName` | 创建/改名时撞名 | ✅ |
| `TOOL_VERSION_NOT_FOUND` | 404 | `tool.ErrVersionNotFound` | revert/get version 版本不存在 | ✅ |
| `TOOL_PENDING_NOT_FOUND` | 404 | `tool.ErrPendingNotFound` | accept/reject 时无 pending | ✅ |
| `TOOL_PENDING_CONFLICT` | 409 | `tool.ErrPendingConflict` | edit_forge 时已有未处理 pending | ✅ |
| `TOOL_TEST_CASE_NOT_FOUND` | 404 | `tool.ErrTestCaseNotFound` | 测试用例找不到 | ✅ |
| `TOOL_RUN_FAILED` | 422 | `tool.ErrRunFailed` | sandbox 内部错误（≠ ok=false 的执行失败）| ✅ |
| `TOOL_AST_PARSE_FAILED` | 422 | `tool.ErrASTParseError` | 代码无法被 Python AST 解析 | ✅ |
| `TOOL_IMPORT_INVALID` | 400 | `tool.ErrImportInvalid` | 导入 JSON 格式错误 | ✅ |
| `TOOL_IMPORT_CONFLICT` | 409 | `tool.ErrImportConflict` | 导入名字冲突需用户决策 | ⬜ |
| `FORGE_ENV_NOT_READY` | 422 | `forge.ErrEnvNotReady` | Run 时 ActiveVersion 的 EnvStatus≠ready / Accept 时 pending 还在 syncing 或 evicted | ✅ |
| `FORGE_ENV_FAILED` | 422 | `forge.ErrEnvFailed` | Sync 失败（含 deps 解析失败 / Python 包冲突）；EnvError 含 uv stderr 全文 | ✅ |
| `FORGE_SANDBOX_UNAVAILABLE` | 503 | `forge.ErrSandboxUnavailable` | 启动期 Bootstrap 失败（uv / python-build-standalone 资源缺失） | ✅ |
| `FORGE_DEPENDENCY_RESOLUTION` | 422 | `forge.ErrDependencyResolution` | uv 无法解析依赖（包名拼错 / 版本约束冲突 / 网络错误）；EnvError 含 uv 完整 stderr | ✅ |

> 现有 `TOOL_*` wire codes 是 Phase 1 大重命名时为客户端兼容保留——sentinel 自身已是 `forgedomain.Err*`。新增的沙箱迭代 sentinel 用 `FORGE_*` 前缀；客户端兼容性清理留到未来独立任务。

---

### Phase 5：System Tool 第二代（2026-05-04）

> **NB：filesystem / search / web / shell 工具家族不向 errmap 注册**——所有失败以友好字符串返 LLM（吃在 chat.message 的 tool_result block 里），不到 handler。详见各家族 design doc 的 §6 安全边界 + §8 错误返回模式：[`filesystem.md`](../service-design-documents/filesystem.md) / [`search.md`](../service-design-documents/search.md) / [`web.md`](../service-design-documents/web.md) / [`shell.md`](../service-design-documents/shell.md)。下面仅 todo / ask 因为有独立 HTTP 端点（todo SSE 推 entity 事件 / ask 走 `POST /answers`），错误才会到 handler 进 errmap。

#### todo ✅
详见 [`../service-design-documents/todo.md`](../service-design-documents/todo.md)。

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `TODO_NOT_FOUND` | 404 | `todo.ErrNotFound` | TodoGet/Update/Delete 时 ID 不存在；**也覆盖跨 conversation 访问场景**（防存在性泄漏，统一返 NotFound 而非 mismatch）| ✅ |
| `TODO_SUBJECT_REQUIRED` | 400 | `todo.ErrSubjectRequired` | TodoCreate / TodoUpdate 的 subject 字段为空 | ✅ |
| `TODO_INVALID_STATUS` | 400 | `todo.ErrInvalidStatus` | TodoUpdate status 不在 4 值白名单（pending/in_progress/completed/deleted）| ✅ |

#### ask ✅
AskUserQuestion 的答案投递端点 `POST /api/v1/conversations/{id}/answers` 错误。

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `ASK_NO_PENDING_QUESTION` | 404 | `ask.ErrNoPendingQuestion` | 投递的 toolCallId 未在 Service.Wait 注册（已超时 / 已答 / 拼错）| ✅ |
| `ASK_ALREADY_ANSWERED` | 409 | `ask.ErrAlreadyAnswered` | （保留）历史 sentinel；当前实现 Resolve 原子摘条目，二次答总走 `ASK_NO_PENDING_QUESTION` | ✅ |
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
| `SUBAGENT_RUN_NOT_FOUND` | 404 | `gorm.ErrRecordNotFound`（handler 内映射） | GET `/subagent-runs/{id}` 找不到 | ✅ |

> 注：`subagentdomain.ErrMaxTurnsExceeded` / `ErrCancelled` **不上抛 handler**，由 SubagentTool.Execute 转友好字符串返 LLM。

#### mcp 📐

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `MCP_SERVER_NOT_FOUND` | 404 | `mcpdomain.ErrServerNotFound` | server 名不在 mcp.json | 📐 |
| `MCP_SERVER_NOT_CONNECTED` | 409 | `mcpdomain.ErrServerNotConnected` | 调用未 connect 的 server | 📐 |
| `MCP_TOOL_NOT_FOUND` | 404 | `mcpdomain.ErrToolNotFound` | tool 名不在 server 的 tools/list | 📐 |
| `MCP_TOOL_CALL_FAILED` | 502 | `mcpdomain.ErrToolCallFailed` | server 自报失败（含 isError=true）| 📐 |
| `MCP_TOOL_CALL_TIMEOUT` | 504 | `mcpdomain.ErrToolCallTimeout` | per-call 超时（默认 30s，可 per-server override）| 📐 |
| `MCP_REGISTRY_ENTRY_NOT_FOUND` | 404 | `mcpdomain.ErrRegistryEntryNotFound` | install 时 registry name 不存在 | 📐 |
| `MCP_RUNTIME_MISSING` | 422 | `mcpdomain.ErrRuntimeMissing` | 系统未装 node / python | 📐 |
| `MCP_REQUIRED_ENV_MISSING` | 422 | `mcpdomain.ErrRequiredEnvMissing` | install 时 required env 未填全 | 📐 |
| `MCP_REQUIRED_ARGS_MISSING` | 422 | `mcpdomain.ErrRequiredArgsMissing` | install 时 required args 未填全 | 📐 |
| `MCP_INSTALL_FAILED` | 502 | `mcpdomain.ErrInstallFailed` | npm install / uvx 安装命令失败 | 📐 |

#### skill 📐

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `SKILL_NOT_FOUND` | 404 | `skilldomain.ErrSkillNotFound` | skill 名不在 ~/.forgify/skills/ | 📐 |
| `SKILL_INVALID_FRONTMATTER` | 422 | `skilldomain.ErrInvalidFrontmatter` | YAML 解析失败 / 必填缺 / allowed-tools 引用未注册 tool | 📐 |
| `SKILL_BODY_TOO_LARGE` | 422 | `skilldomain.ErrBodyTooLarge` | SKILL.md body > 32 KB | 📐 |
| `SKILL_NAME_CONFLICT` | 409 | `skilldomain.ErrNameConflict` | POST /skills 创建同名 | 📐 |
| `SKILL_INVALID_NAME` | 422 | `skilldomain.ErrInvalidName` | name 不符 `[a-z0-9-]{1,64}` | 📐 |

#### catalog 📐

无对外 sentinel——`ErrCoverageIncomplete` / `ErrGenerationFailed` 内部消化（重试 + mechanical fallback），不上抛 handler。`POST /catalog:refresh` 失败返通用 500 + 日志详情。

#### sandbox 📐

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `SANDBOX_RUNTIME_NOT_SUPPORTED` | 422 | `sandboxdomain.ErrRuntimeNotSupported` | 没有 installer 注册该 kind | 📐 |
| `SANDBOX_RUNTIME_INSTALL_FAILED` | 502 | `sandboxdomain.ErrRuntimeInstallFailed` | mise install / playwright install 等失败 | 📐 |
| `SANDBOX_ENV_NOT_FOUND` | 404 | `sandboxdomain.ErrEnvNotFound` | 通过 owner / id 查不到 | 📐 |
| `SANDBOX_ENV_CREATE_FAILED` | 502 | `sandboxdomain.ErrEnvCreateFailed` | venv / node_modules / etc. 建失败 | 📐 |
| `SANDBOX_DEP_INSTALL_FAILED` | 502 | `sandboxdomain.ErrDepInstallFailed` | uv pip install / npm install 失败 | 📐 |
| `SANDBOX_SPAWN_FAILED` | 502 | `sandboxdomain.ErrSpawnFailed` | 子进程起不来 | 📐 |
| `SANDBOX_SPAWN_TIMEOUT` | 504 | `sandboxdomain.ErrSpawnTimeout` | once-spawn 超时 | 📐 |
| `SANDBOX_ENV_IN_USE` | 409 | `sandboxdomain.ErrEnvInUse` | Destroy 时 env 还在跑 | 📐 |

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

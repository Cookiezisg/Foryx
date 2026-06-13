---
id: DOC-004
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
audience: [human, ai]
---

# 错误码 —— 错误系统 + 全量 wire code 登记

> 后端错误的单一事实源：框架 / 规约 + **全 256 个 wire code 完整登记**（按域）。机械守卫保证「全用 `errorspkg.New`」+「码全库唯一」——`pkg/errors/standard_test.go`，进 `make verify`。

## 框架（`pkg/errors`）

`errorspkg.Error{Kind, Code, Message, Details, cause}`，`New(kind, code, msg)` 构造。**类型在 `pkg/errors`（地基、全层可用、纯机制）**——所有命名 sentinel 一律 `errorspkg.New`，无「是否冒泡 HTTP」之分（见 [`decisions/0002`](../../decisions/0002-unified-error-type.md)）。

- `Is` **按 Code 匹配** → sentinel 与其 `WithCause`/`WithDetails` 副本在 `errors.Is` 下相等（故 **Code 必须全库唯一**，否则两 sentinel 被混淆）。
- 两种出口：**HTTP** 读 Kind(→状态码)/Code/Details 走 N1 Envelope；**LLM tool** 读 Message。
- 包裹用 `fmt.Errorf("…: %w", err)`（保留链）；`errors.Is/As` 用标准库。
- 泛型原语（`ORM_*` / `FSPATH_*` / `MISSING_*` / `HANDLER_CLIENT_*` 等）带兜底码，domain 仍 `errors.Is` 后翻成具体码（如 `orm.ErrNotFound` → `FUNCTION_NOT_FOUND`）。

## Kind → HTTP（15，封闭集；零值 `KindInternal` 安全兜底）

唯一映射表 = `transport/httpapi/response/errmap.go::statusForKind`（transport 不持逐错误表、不 import 业务 domain）。

| Kind | HTTP | Kind | HTTP | Kind | HTTP |
|---|---|---|---|---|---|
| Internal | 500 | Unprocessable | 422 | GatewayTimeout | 504 |
| Invalid | 400 | TooLarge | 413 | Accepted | 202 |
| Unauthorized | 401 | UnsupportedMedia | 415 | ClientClosed | 499 |
| NotFound | 404 | RateLimited | 429 | Gone | 410 |
| Conflict | 409 | BadGateway | 502 | Unavailable | 503 |

## 命名规约 + 守卫

**`<ENTITY>_<REASON>`，SCREAMING_SNAKE，按实体命名空间，全库唯一。** infra 原语用独立命名空间避免与 domain 撞码（如 `HANDLER_CLIENT_*` 区别于 domain 的 `HANDLER_*`）。

`pkg/errors/standard_test.go`（`make verify` 内）守两条：① 任何包级 `var Err… = errors.New/fmt.Errorf`（绕过 errorspkg）→ build 失败；② wire code 重复 → 失败。

---

## 全量登记（264 码，按域）

> `errorspkg.New` 机械抽取（259，不含 `*_test.go` 测试 sentinel 如 DUP/THING_NOT_FOUND）+ `pkg/errors` 自身 bare `New` 的跨域 sentinel（5）。每条：code · HTTP（Kind 映射）· message。`(dynamic)` = 消息含运行时格式化。

### `pkg/errors`（跨域 sentinel）

| code | HTTP | message |
|---|---|---|
| `INVALID_REQUEST` | 400 | invalid request（domain 逻辑前的格式/语义无效） |
| `UNAUTH_NO_WORKSPACE` | 401 | unauthorized: no valid workspace id（隔离路由缺 ws；中间件 `RequireWorkspace` 兜、前端清 workspace 重选） |
| `NOT_FOUND` | 404 | not found（路由 / 未知 :action / handler 派发未命中的统一兜底，S6/MD-err） |
| `INTERNAL_ERROR` | 500 | internal error（recover 的 panic；原始细节记日志、不上线缆） |
| `STREAMING_UNSUPPORTED` | 500 | streaming not supported（SSE 端点遇非流式 ResponseWriter） |

### `app/aispawn`

| code | HTTP | message |
|---|---|---|
| `EMPTY_ITERATE_REQUEST` | 400 | iterate needs a request describing what to change |

### `app/attachment`

| code | HTTP | message |
|---|---|---|
| `ATTACHMENT_EXTRACTION_UNSUPPORTED` | 415 | extraction unsupported for this mime |

### `app/chat`

| code | HTTP | message |
|---|---|---|
| `EMPTY_CONTENT` | 400 | message has no text and no attachments |
| `NO_PENDING_INTERACTION` | 404 | no pending interaction with that tool call id in this conversation |
| `STREAM_IN_PROGRESS` | 409 | this conversation already has an assistant turn running |

### `app/settings`

| code | HTTP | message |
|---|---|---|
| `SETTINGS_LIMITS_INVALID` | 400 | limits values out of range |

### `app/tool/agent`

| code | HTTP | message |
|---|---|---|
| `AGENT_EXECUTION_ID_REQUIRED` | 400 | executionId is required |
| `AGENT_ID_PROMPT_REQUIRED` | 400 | agentId and prompt are required |
| `AGENT_ID_REQUIRED` | 400 | agentId is required |
| `AGENT_NAME_PROMPT_REQUIRED` | 400 | name and prompt are required |
| `AGENT_REVERT_ARGS_REQUIRED` | 400 | agentId and a positive version are required |

### `app/tool/approval`

| code | HTTP | message |
|---|---|---|
| `APPROVAL_ID_REQUIRED` | 400 | approvalId is required |
| `APPROVAL_NAME_REQUIRED` | 400 | name is required |
| `APPROVAL_TEMPLATE_REQUIRED` | 400 | template is required |
| `APPROVAL_VERSION_POSITIVE` | 400 | version must be a positive integer |

### `app/tool/ask`

| code | HTTP | message |
|---|---|---|
| `ASK_MESSAGE_REQUIRED` | 400 | message is required |
| `ASK_NO_INTERACTIVE_USER` | 503 | ask_user is only available in an interactive conversation; proceed without asking |

### `app/tool/control`

| code | HTTP | message |
|---|---|---|
| `CONTROL_BRANCHES_REQUIRED` | 400 | branches is required (at least one) |
| `CONTROL_ID_REQUIRED` | 400 | controlId is required |
| `CONTROL_NAME_REQUIRED` | 400 | name is required |
| `CONTROL_VERSION_POSITIVE` | 400 | version must be a positive integer |

### `app/tool/document`

| code | HTTP | message |
|---|---|---|
| `DOCUMENT_ID_REQUIRED` | 400 | id is required |
| `DOCUMENT_NAME_REQUIRED` | 400 | name is required |
| `DOCUMENT_QUERY_REQUIRED` | 400 | query is required |

### `app/tool/filesystem`

| code | HTTP | message |
|---|---|---|
| `FS_CONTENT_REQUIRED` | 400 | content field is required (use empty string to create an empty file) |
| `FS_EDIT_NOOP` | 400 | old_string and new_string must be different |
| `FS_EMPTY_FILE_PATH` | 400 | file_path is required |
| `FS_EMPTY_OLD_STRING` | 400 | old_string is required and must be non-empty |
| `FS_NEGATIVE_LIMIT` | 400 | limit must be non-negative |
| `FS_NEGATIVE_OFFSET` | 400 | offset must be non-negative |
| `FS_NEW_STRING_REQUIRED` | 400 | new_string field is required (use empty string to delete the matched text) |

### `app/tool/function`

| code | HTTP | message |
|---|---|---|
| `FUNCTION_EXECUTION_ID_REQUIRED` | 400 | executionId is required |
| `FUNCTION_ID_REQUIRED` | 400 | functionId is required |
| `FUNCTION_OPS_REQUIRED` | 400 | ops is required (non-empty) |
| `FUNCTION_VERSION_POSITIVE` | 400 | version must be a positive integer |

### `app/tool/handler`

| code | HTTP | message |
|---|---|---|
| `HANDLER_CALL_ID_REQUIRED` | 400 | callId is required |
| `HANDLER_ID_REQUIRED` | 400 | handlerId is required |
| `HANDLER_METHOD_REQUIRED` | 400 | method is required |
| `HANDLER_OPS_REQUIRED` | 400 | ops is required (non-empty) |
| `HANDLER_VERSION_POSITIVE` | 400 | version must be a positive integer |

### `app/tool/mcp`

| code | HTTP | message |
|---|---|---|
| `MCP_CALL_ID_REQUIRED` | 400 | callId is required |
| `MCP_NAME_REQUIRED` | 400 | name is required |
| `MCP_SERVER_ID_REQUIRED` | 400 | serverId is required |

### `app/tool/memory`

| code | HTTP | message |
|---|---|---|
| `MEMORY_EMPTY_CONTENT` | 400 | content is required |
| `MEMORY_EMPTY_DESCRIPTION` | 400 | description is required |
| `MEMORY_EMPTY_NAME` | 400 | name is required |

### `app/tool/search`

| code | HTTP | message |
|---|---|---|
| `SEARCH_EMPTY_PATTERN` | 400 | pattern is required and must be non-empty |
| `SEARCH_INVALID_OUTPUT_MODE` | 400 | output_mode must be one of "content", "files_with_matches", "count" |
| `SEARCH_NEGATIVE_LIMIT` | 400 | a numeric limit must be non-negative |
| `SEARCH_PATH_REQUIRED` | 400 | path is required (absolute or ~; the agent has no current directory) |

### `app/tool/shell`

| code | HTTP | message |
|---|---|---|
| `SHELL_EMPTY_BASH_ID` | 400 | bash_id is required |
| `SHELL_EMPTY_COMMAND` | 400 | command is required and must be non-empty |
| `SHELL_INVALID_TIMEOUT` | 400 | timeout must be between 0 and %d ms |
| `SHELL_PROCESS_NOT_FOUND` | 404 | background shell process not found |

### `app/tool/skill`

| code | HTTP | message |
|---|---|---|
| `SKILL_NAME_REQUIRED` | 400 | name is required |

### `app/tool/toolset`

| code | HTTP | message |
|---|---|---|
| `TOOLSET_EMPTY_QUERY` | 400 | query is required and must be non-empty |

### `app/tool/trigger`

| code | HTTP | message |
|---|---|---|
| `TRIGGER_ACTIVATION_ID_REQUIRED` | 400 | activationId is required |
| `TRIGGER_ID_REQUIRED` | 400 | triggerId is required |
| `TRIGGER_NAME_REQUIRED` | 400 | name is required |

### `app/tool/web`

| code | HTTP | message |
|---|---|---|
| `WEBSEARCH_AUTH_FAILED` | 502 | search provider authentication failed |
| `WEBSEARCH_EMPTY_QUERY` | 400 | query is required and must be non-empty |
| `WEBSEARCH_NEGATIVE_LIMIT` | 400 | limit must be non-negative |
| `WEBSEARCH_RATE_LIMITED` | 429 | search provider rate limited |
| `WEBSEARCH_UPSTREAM_HTTP` | 502 | search provider upstream error |
| `WEB_EMPTY_PROMPT` | 400 | prompt is required and must be non-empty |
| `WEB_EMPTY_URL` | 400 | url is required and must be non-empty |
| `WEB_UNSUPPORTED_SCHEME` | 400 | url must use http or https scheme |

### `app/tool/workflow`

| code | HTTP | message |
|---|---|---|
| `FLOWRUN_ID_REQUIRED` | 400 | flowrunId is required |
| `WORKFLOW_ID_REQUIRED` | 400 | workflowId is required |
| `WORKFLOW_NAME_REQUIRED` | 400 | name is required |
| `WORKFLOW_OPS_REQUIRED` | 400 | ops is required (non-empty) |
| `WORKFLOW_VERSION_POSITIVE` | 400 | version must be a positive integer |

### `app/workflow`

| code | HTTP | message |
|---|---|---|
| `WORKFLOW_EXECUTION_UNAVAILABLE` | 500 | workflow execution lifecycle is unavailable (engine not wired) |

### `bootstrap`

| code | HTTP | message |
|---|---|---|
| `UNTRIAGEABLE_EXECUTION` | 400 | (dynamic) |

### `domain/agent`

| code | HTTP | message |
|---|---|---|
| `AGENT_EXECUTION_NOT_FOUND` | 404 | agent execution not found |
| `AGENT_INVALID_MODEL_OVERRIDE` | 422 | invalid modelOverride (apiKeyId and modelId both required) |
| `AGENT_MOUNT_INVALID` | 422 | agent mounted tool ref is invalid or unresolvable |
| `AGENT_NAME_CONFLICT` | 409 | agent name already exists |
| `AGENT_NOT_FOUND` | 404 | agent not found |
| `AGENT_NO_ACTIVE_VERSION` | 422 | agent has no active version to invoke |
| `AGENT_TOOLS_AGENT_REF` | 422 | agent tools cannot reference another agent (ag_ forbidden) |
| `AGENT_TOOL_REF_BLANK` | 422 | agent tool ref must not be blank |
| `AGENT_VERSION_NOT_FOUND` | 404 | agent version not found |

### `domain/apikey`

| code | HTTP | message |
|---|---|---|
| `API_KEY_API_FORMAT_REQUIRED` | 400 | api format is required for custom provider |
| `API_KEY_BASE_URL_REQUIRED` | 400 | base url is required for this provider |
| `API_KEY_DISPLAY_NAME_CONFLICT` | 409 | display name already in use |
| `API_KEY_INVALID_PROVIDER` | 400 | unknown provider |
| `API_KEY_IN_USE` | 422 | api key is referenced and cannot be deleted |
| `API_KEY_TEST_FAILED` | 422 | api key probe failed（details: latencyMs + reason） |
| `API_KEY_NOT_FOUND` | 404 | api key not found |
| `API_KEY_VALUE_REQUIRED` | 400 | key value is required |

### `domain/approval`

| code | HTTP | message |
|---|---|---|
| `APPROVAL_INVALID_NAME` | 422 | invalid approval form name |
| `APPROVAL_INVALID_TEMPLATE` | 422 | approval template empty or its {{ CEL }} failed to compile |
| `APPROVAL_INVALID_TIMEOUT` | 422 | invalid timeout duration or missing/invalid timeoutBehavior |
| `APPROVAL_NAME_DUPLICATE` | 409 | approval form name already exists |
| `APPROVAL_NOT_FOUND` | 404 | approval form not found |
| `APPROVAL_NO_ACTIVE_VERSION` | 422 | approval form has no active version |
| `APPROVAL_VERSION_NOT_FOUND` | 404 | approval form version not found |

### `domain/attachment`

| code | HTTP | message |
|---|---|---|
| `ATTACHMENT_BAD_UPLOAD` | 400 | malformed multipart upload or missing 'file' field |
| `ATTACHMENT_EMPTY` | 400 | empty file |
| `ATTACHMENT_NOT_FOUND` | 404 | attachment not found |
| `ATTACHMENT_TOO_LARGE` | 413 | file exceeds the 50 MB limit |

### `domain/catalog`

| code | HTTP | message |
|---|---|---|
| `CATALOG_ALL_SOURCES_FAILED` | 503 | all catalog sources failed |

### `domain/control`

| code | HTTP | message |
|---|---|---|
| `CONTROL_INVALID_BRANCHES` | 422 | branches empty, or port empty/duplicate |
| `CONTROL_INVALID_CEL` | 422 | branch when/emit failed to compile |
| `CONTROL_INVALID_NAME` | 422 | invalid control logic name |
| `CONTROL_NAME_DUPLICATE` | 409 | control logic name already exists |
| `CONTROL_NOT_FOUND` | 404 | control logic not found |
| `CONTROL_NO_ACTIVE_VERSION` | 422 | control logic has no active version |
| `CONTROL_NO_CATCHALL` | 422 | last branch must be when:"true" |
| `CONTROL_VERSION_NOT_FOUND` | 404 | control logic version not found |

### `domain/conversation`

| code | HTTP | message |
|---|---|---|
| `CONVERSATION_INVALID_MODEL_OVERRIDE` | 422 | invalid modelOverride (apiKeyId and modelId both required) |
| `CONVERSATION_NOT_FOUND` | 404 | conversation not found |

### `domain/document`

| code | HTTP | message |
|---|---|---|
| `DOCUMENT_CONTENT_TOO_LARGE` | 413 | content exceeds 1 MB limit |
| `DOCUMENT_INVALID_NAME` | 400 | invalid name (empty, too long, or contains '/') |
| `DOCUMENT_INVALID_PARENT` | 422 | invalid parent (cycle or self) |
| `DOCUMENT_NAME_CONFLICT` | 409 | name already exists under same parent |
| `DOCUMENT_NOT_FOUND` | 404 | document not found |
| `DOCUMENT_PARENT_NOT_FOUND` | 422 | parent not found |

### `domain/flowrun`

| code | HTTP | message |
|---|---|---|
| `FLOWRUN_APPROVAL_NOT_PARKED` | 422 | approval node is not awaiting a decision |
| `FLOWRUN_INVALID_DECISION` | 422 | approval decision must be 'yes' or 'no' |
| `FLOWRUN_INVALID_ENTRY` | 422 | invalid or ambiguous trigger entry node |
| `FLOWRUN_NOT_FOUND` | 404 | flowrun not found |
| `FLOWRUN_NOT_REPLAYABLE` | 422 | flowrun is not in a replayable (failed) state |

### `domain/function`

| code | HTTP | message |
|---|---|---|
| `FUNCTION_ENV_NOT_READY` | 422 | function env not ready |
| `FUNCTION_EXECUTION_NOT_FOUND` | 404 | function execution not found |
| `FUNCTION_INVALID_CODE` | 422 | function code invalid |
| `FUNCTION_INVALID_NAME` | 400 | invalid function name (lowercase alphanumeric + dashes/underscores, 1-64 chars) |
| `FUNCTION_NAME_DUPLICATE` | 409 | function name already exists |
| `FUNCTION_NOT_FOUND` | 404 | function not found |
| `FUNCTION_NO_ACTIVE_VERSION` | 422 | function has no active version |
| `FUNCTION_OP_INVALID` | 422 | invalid forge op |
| `FUNCTION_SANDBOX_UNAVAILABLE` | 503 | sandbox runtime unavailable |
| `FUNCTION_VERSION_NOT_FOUND` | 404 | function version not found |

### `domain/handler`

| code | HTTP | message |
|---|---|---|
| `HANDLER_CALL_NOT_FOUND` | 404 | handler call not found |
| `HANDLER_CONFIG_DECRYPT_FAILED` | 500 | handler config decrypt failed |
| `HANDLER_CONFIG_INCOMPLETE` | 422 | handler config incomplete (required init args unset) |
| `HANDLER_CRASHED` | 502 | handler instance crashed |
| `HANDLER_ENV_NOT_READY` | 422 | handler env not ready |
| `HANDLER_INSTANCE_SPAWN_FAILED` | 502 | handler instance spawn failed |
| `HANDLER_INVALID_CODE` | 422 | handler class code invalid |
| `HANDLER_INVALID_NAME` | 400 | invalid handler name (lowercase alphanumeric + dashes/underscores, 1-64 chars) |
| `HANDLER_METHOD_NOT_FOUND` | 404 | handler method not found |
| `HANDLER_NAME_DUPLICATE` | 409 | handler name already exists |
| `HANDLER_NOT_FOUND` | 404 | handler not found |
| `HANDLER_NO_ACTIVE_VERSION` | 422 | handler has no active version |
| `HANDLER_OP_INVALID` | 422 | invalid forge op |
| `HANDLER_RPC_TIMEOUT` | 504 | handler instance RPC timeout |
| `HANDLER_SANDBOX_UNAVAILABLE` | 503 | sandbox runtime unavailable |
| `HANDLER_VERSION_NOT_FOUND` | 404 | handler version not found |

### `domain/mcp`

| code | HTTP | message |
|---|---|---|
| `MCP_CALL_NOT_FOUND` | 404 | mcp call not found |
| `MCP_ENV_MISSING` | 422 | required environment variables missing |
| `MCP_INSTALL_FAILED` | 502 | mcp server install failed |
| `MCP_NAME_CONFLICT` | 409 | mcp server name already exists |
| `MCP_NO_RUNNABLE_PACKAGE` | 422 | no package with a supported runtime (node/python/docker/dotnet) and no remote endpoint |
| `MCP_REGISTRY_NOT_FOUND` | 404 | mcp registry entry not found |
| `MCP_RPC_ERROR` | 502 | mcp tool call failed |
| `MCP_SERVER_DOWN` | 503 | mcp server not connected |
| `MCP_SERVER_NOT_FOUND` | 404 | mcp server not found |
| `MCP_TOOL_NOT_FOUND` | 404 | mcp tool not found on server |
| `MCP_TOOL_TIMEOUT` | 504 | mcp tool call timed out |

### `domain/memory`

| code | HTTP | message |
|---|---|---|
| `MEMORY_INVALID_INPUT` | 400 | memory description and content required |
| `MEMORY_INVALID_NAME` | 400 | invalid memory name (must be a lowercase slug) |
| `MEMORY_INVALID_SOURCE` | 400 | invalid memory source (must be user or ai) |
| `MEMORY_NOT_FOUND` | 404 | memory not found |

### `domain/messages`

| code | HTTP | message |
|---|---|---|
| `MESSAGE_NOT_FOUND` | 404 | message not found |

### `domain/model`

| code | HTTP | message |
|---|---|---|
| `MODEL_NOT_CONFIGURED` | 422 | no model configured for scenario |
| `MODEL_REF_INVALID` | 400 | model selection requires both apiKeyId and modelId |
| `MODEL_SCENARIO_INVALID` | 400 | unknown model scenario |

### `domain/notification`

| code | HTTP | message |
|---|---|---|
| `NOTIFICATION_INVALID_TYPE` | 400 | notification type required (<domain>.<action>) |
| `NOTIFICATION_NOT_FOUND` | 404 | notification not found |

### `domain/relation`

| code | HTTP | message |
|---|---|---|
| `REL_DEPTH_LIMIT` | 400 | neighborhood depth out of range |
| `REL_INCOMPLETE_FILTER` | 400 | incomplete filter (kind without id, or vice versa) |
| `REL_INVALID_KIND` | 400 | invalid relation kind |
| `REL_INVALID_REF` | 400 | invalid entity ref (unknown kind or empty id) |
| `REL_SELF_LOOP` | 400 | self-loop forbidden (from == to) |

### `domain/sandbox`

| code | HTTP | message |
|---|---|---|
| `SANDBOX_CMD_REQUIRED` | 400 | spawn cmd is required |
| `SANDBOX_DEP_INSTALL_FAILED` | 502 | dependency install failed |
| `SANDBOX_DOCKER_DAEMON_DOWN` | 503 | docker daemon not responding |
| `SANDBOX_DOCKER_NOT_INSTALLED` | 422 | docker not installed |
| `SANDBOX_ENV_CREATE_FAILED` | 502 | env create failed |
| `SANDBOX_ENV_IN_USE` | 409 | env in use; cannot destroy |
| `SANDBOX_ENV_NOT_FOUND` | 404 | env not found |
| `SANDBOX_INVALID_OWNER_ID` | 400 | owner id contains PATH-meta or whitespace character |
| `SANDBOX_INVALID_OWNER_KIND` | 400 | ownerKind must be one of: function, handler, mcp, skill, conversation |
| `SANDBOX_OWNER_KIND_REQUIRED` | 400 | ownerKind query parameter is required |
| `SANDBOX_RUNTIME_INSTALL_FAILED` | 502 | runtime install failed |
| `SANDBOX_RUNTIME_NOT_FOUND` | 404 | runtime not found |
| `SANDBOX_RUNTIME_NOT_SUPPORTED` | 422 | runtime kind not registered |
| `SANDBOX_SPAWN_FAILED` | 502 | spawn process failed |
| `SANDBOX_SPAWN_TIMEOUT` | 504 | spawn process timeout |

### `domain/search`

| code | HTTP | message |
|---|---|---|
| `SEARCH_QUERY_REQUIRED` | 400 | search query is required |
| `SEARCH_TYPE_INVALID` | 400 | unknown search entity type |
| `SEARCH_CURSOR_INVALID` | 400 | search cursor is invalid or stale |
| `SEARCH_REINDEX_RUNNING` | 409 | a reindex is already running |
| `SEARCH_EMBEDDER_INVALID` | 400 | embedder must be one of builtin, ollama, off |

### `domain/skill`

| code | HTTP | message |
|---|---|---|
| `SKILL_BODY_TOO_LARGE` | 422 | skill body exceeds size limit |
| `SKILL_FORK_REQUIRES_AGENT` | 422 | context=fork requires an agent type |
| `SKILL_INVALID_FRONTMATTER` | 422 | invalid skill frontmatter |
| `SKILL_INVALID_NAME` | 400 | invalid skill name (must be a lowercase slug) |
| `SKILL_NAME_CONFLICT` | 409 | skill name already exists |
| `SKILL_NOT_FOUND` | 404 | skill not found |
| `SKILL_SUBAGENT_UNAVAILABLE` | 503 | fork skill requires a subagent runner (not wired) |

### `domain/stream`

| code | HTTP | message |
|---|---|---|
| `SEQ_TOO_OLD` | 410 | requested seq too old (evicted from replay buffer) |
| `STREAM_INVALID_EVENT` | 500 | stream: invalid event |

### `domain/todo`

| code | HTTP | message |
|---|---|---|
| `TODO_EMPTY_CONTENT` | 400 | todo item content is required |
| `TODO_ITEMS_REQUIRED` | 400 | items is required (send the full checklist; [] clears) |
| `TODO_INVALID_STATUS` | 400 | invalid todo item status |
| `TODO_TOO_MANY_ITEMS` | 400 | too many todo items |

### `domain/trigger`

| code | HTTP | message |
|---|---|---|
| `TRIGGER_ACTIVATION_NOT_FOUND` | 404 | activation not found |
| `TRIGGER_FIRING_NOT_PENDING` | 409 | firing already claimed |
| `TRIGGER_INVALID_CEL` | 422 | invalid CEL expression |
| `TRIGGER_INVALID_CONFIG` | 422 | invalid trigger config |
| `TRIGGER_INVALID_CRON` | 422 | invalid cron expression |
| `TRIGGER_INVALID_INTERVAL` | 422 | sensor interval below minimum |
| `TRIGGER_INVALID_KIND` | 422 | unknown trigger kind |
| `TRIGGER_LISTENER_UNAVAILABLE` | 503 | trigger listener not available |
| `TRIGGER_NAME_DUPLICATE` | 409 | trigger name already exists |
| `TRIGGER_NOT_FOUND` | 404 | trigger not found |
| `TRIGGER_SENSOR_TARGET_REQUIRED` | 422 | sensor requires a function or handler target |
| `TRIGGER_WEBHOOK_SECRET_MISMATCH` | 401 | webhook secret mismatch |

### `domain/workflow`

| code | HTTP | message |
|---|---|---|
| `WORKFLOW_ALREADY_ACTIVE` | 409 | workflow is already active; deactivate before staging a one-shot run |
| `WORKFLOW_INVALID_GRAPH` | 422 | workflow graph is invalid |
| `WORKFLOW_INVALID_LIFECYCLE` | 422 | invalid workflow lifecycle state or transition |
| `WORKFLOW_INVALID_OPS` | 422 | invalid workflow ops |
| `WORKFLOW_NAME_DUPLICATE` | 409 | workflow name already exists |
| `WORKFLOW_NOT_FOUND` | 404 | workflow not found |
| `WORKFLOW_NO_ACTIVE_VERSION` | 422 | workflow has no active version |
| `WORKFLOW_NO_TRIGGER_ENTRY` | 422 | workflow has no entry trigger node to listen on |
| `WORKFLOW_REF_NOT_FOUND` | 422 | workflow node ref not found or mismatched |
| `WORKFLOW_VERSION_NOT_FOUND` | 404 | workflow version not found |

### `domain/workspace`

| code | HTTP | message |
|---|---|---|
| `CANNOT_DELETE_LAST_WORKSPACE` | 422 | cannot delete the last workspace |
| `WORKSPACE_LANGUAGE_INVALID` | 400 | language must be one of zh-CN, en |
| `WORKSPACE_WEB_FETCH_MODE_INVALID` | 400 | webFetchMode must be one of local, jina |
| `WORKSPACE_NAME_CONFLICT` | 409 | workspace name already exists |
| `WORKSPACE_NAME_REQUIRED` | 400 | workspace name is required |
| `WORKSPACE_NAME_TOO_LONG` | 400 | workspace name exceeds the length limit |
| `WORKSPACE_NOT_FOUND` | 404 | workspace not found |

### `infra/crypto`

| code | HTTP | message |
|---|---|---|
| `CRYPTO_NO_FINGERPRINT` | 500 | cannot determine machine fingerprint |
| `CRYPTO_UNSUPPORTED_VERSION` | 500 | aesgcm: unsupported ciphertext version |

### `infra/handler`

| code | HTTP | message |
|---|---|---|
| `HANDLER_CLIENT_ALREADY_SHUTDOWN` | 500 | handler.Client: already shut down |
| `HANDLER_CLIENT_CALL_FAILED` | 502 | handler.Client: call failed |
| `HANDLER_CLIENT_CRASHED` | 502 | handler.Client: subprocess crashed |
| `HANDLER_CLIENT_INIT_FAILED` | 502 | handler.Client: init failed |
| `HANDLER_CLIENT_PROTOCOL` | 502 | handler.Client: protocol error |

### `infra/llm`

| code | HTTP | message |
|---|---|---|
| `LLM_AUTH_FAILED` | 401 | llm: authentication failed |
| `LLM_BAD_REQUEST` | 400 | llm: bad request |
| `LLM_MODEL_NOT_FOUND` | 404 | llm: model not found |
| `LLM_PROVIDER_ERROR` | 502 | llm: provider error |
| `LLM_RATE_LIMITED` | 429 | llm: rate limited |
| `MOCK_QUEUE_EMPTY` | 500 | mock-llm: script queue empty — push a script before sending |

### `pkg/fspath`

| code | HTTP | message |
|---|---|---|
| `FSPATH_EMPTY_PATH` | 400 | path is required |
| `FSPATH_NOT_ABSOLUTE` | 400 | path must be absolute (the agent has no working directory; pass an absolute path or one starting with ~) |
| `FSPATH_NO_HOME` | 500 | cannot expand ~: home directory is unknown |

### `pkg/orm`

| code | HTTP | message |
|---|---|---|
| `ORM_CONFLICT` | 409 | orm: unique constraint conflict |
| `ORM_NOT_FOUND` | 404 | orm: record not found |

### `pkg/pagination`

| code | HTTP | message |
|---|---|---|
| `MALFORMED_CURSOR` | 400 | pagination: malformed cursor |

### `pkg/reqctx`

| code | HTTP | message |
|---|---|---|
| `MISSING_CONVERSATION_ID` | 500 | reqctx: missing conversation id in context |
| `MISSING_WORKSPACE_ID` | 500 | reqctx: missing workspace id in context（接线 bug；客户端的 401 是 UNAUTH_NO_WORKSPACE） |

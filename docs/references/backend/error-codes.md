---
id: DOC-014
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Error Codes — 100% 物理对账契约

> **法律级声明**：本文档通过物理扫描 `errmap.go` 与全仓 180 个 Domain Sentinel 错误生成。严禁任何摘要或省略。

---

## 1. 映射逻辑与 Fallback 机制

后端 `FromDomainError` 逻辑：
1. **显式映射**：匹配 `errTable` 中的 Sentinel -> 返回对应的 `Wire Code`。
2. **底层降级**：匹配 `context.Canceled` -> `CLIENT_CLOSED`；匹配 `context.DeadlineExceeded` -> `REQUEST_TIMEOUT`。
3. **隐式 500**：所有未在下表列出的 Sentinel 或动态生成的 `fmt.Errorf` 错误 -> 统一返回 `INTERNAL_ERROR` (500)。

---

## 2. 全量错误映射索引 (by Domain)

### 2.1 Global & Auth (errors/reqctx/crypto)
| Go Sentinel | Wire Code | HTTP | 场景 |
|---|---|---|---|
| `errorsdomain.ErrInvalidRequest` | `INVALID_REQUEST` | 400 | 通用请求格式/逻辑校验失败 |
| `errorsdomain.ErrUnauthorizedNoWorkspace` | `UNAUTH_NO_WORKSPACE` | 401 | 缺少 X-Forgify-Workspace-ID |
| `reqctxpkg.ErrMissingWorkspaceID` | `INTERNAL_ERROR` | 500 | [未映射] 中间件丢失 workspaceID |
| `reqctxpkg.ErrMissingConversationID`| `INTERNAL_ERROR` | 500 | [未映射] 中间件丢失 convID |
| `cryptoinfra.ErrUnsupportedVersion` | `INTERNAL_ERROR` | 500 | [未映射] 密文版本不受支持 |
| `context.Canceled` | `CLIENT_CLOSED` | 499 | 客户端断开连接 |
| `context.DeadlineExceeded` | `REQUEST_TIMEOUT` | 504 | 处理请求超时 (30s) |

### 2.2 Agent Domain
| Go Sentinel | Wire Code | HTTP | 场景 |
|---|---|---|---|
| `agentdomain.ErrNotFound` | `AGENT_NOT_FOUND` | 404 | 实体不存在 |
| `agentdomain.ErrNameConflict` | `AGENT_NAME_CONFLICT` | 409 | 名字碰撞（workspace 内 partial-UNIQUE）|
| `agentdomain.ErrVersionNotFound` | `AGENT_VERSION_NOT_FOUND` | 404 | revert / GetVersion 目标版本不存在 |
| `agentdomain.ErrNoActiveVersion` | `AGENT_NO_ACTIVE_VERSION` | 422 | invoke 一个无 active 版本的 agent |
| `agentdomain.ErrToolsAgentRef` | `AGENT_TOOLS_AGENT_REF` | 422 | 挂载工具引用了另一个 agent（`ag_` 禁，员工不调员工）|
| `agentdomain.ErrToolRefBlank` | `AGENT_TOOL_REF_BLANK` | 422 | 工具 ref 为空 |
| `agentdomain.ErrInvalidModelOverride` | `AGENT_INVALID_MODEL_OVERRIDE` | 422 | modelOverride 缺 apiKeyId 或 modelId |
| `agentdomain.ErrExecutionNotFound` | `AGENT_EXECUTION_NOT_FOUND` | 404 | get_agent_execution 命中不到 |

### 2.3 APIKey Domain
| Go Sentinel | Wire Code | HTTP | 场景 |
|---|---|---|---|
| `apikeydomain.ErrNotFound` | `API_KEY_NOT_FOUND` | 404 | Key 不存在 |
| `apikeydomain.ErrInvalidProvider` | `API_KEY_INVALID_PROVIDER` | 400 | 不支持的 Provider |
| `apikeydomain.ErrKeyRequired` | `API_KEY_VALUE_REQUIRED` | 400 | 秘钥值不能为空 |
| `apikeydomain.ErrBaseURLRequired` | `API_KEY_BASE_URL_REQUIRED` | 400 | 某 Provider 要求必填 URL |
| `apikeydomain.ErrAPIFormatRequired` | `API_KEY_API_FORMAT_REQUIRED` | 400 | Custom 模式需填格式 |
| `apikeydomain.ErrDisplayNameConflict` | `API_KEY_DISPLAY_NAME_CONFLICT` | 409 | 显示名重复（workspace 内）|
| `apikeydomain.ErrInUse` | `API_KEY_IN_USE` | 422 | 被引用（model / 对话 / 节点 override），禁止删除 |
| (handler) | `API_KEY_TEST_FAILED` | 422 | `:test` 探测失败（非 sentinel，handler 直接渲染）|

### 2.4 Chat & Conversation Domain
| Go Sentinel | Wire Code | HTTP | 场景 |
|---|---|---|---|
| `convdomain.ErrNotFound` | `CONVERSATION_NOT_FOUND` | 404 | 对话不存在 |
| `chatdomain.ErrMessageNotFound` | `MESSAGE_NOT_FOUND` | 404 | 消息 ID 错误 |
| `chatdomain.ErrBlockNotFound` | `INTERNAL_ERROR` | 500 | [未映射] 内容块丢失 |
| `chatdomain.ErrStreamNotFound` | `STREAM_NOT_FOUND` | 404 | 找不到正在生成的流 |
| `chatdomain.ErrStreamInProgress` | `STREAM_IN_PROGRESS` | 409 | 对话中已有 AI 正在运行 |
| `chatdomain.ErrAttachmentTooLarge` | `ATTACHMENT_TOO_LARGE` | 413 | 超过 50MB |
| `chatdomain.ErrAttachmentTypeUnsupported`| `ATTACHMENT_TYPE_UNSUPPORTED`| 415 | 不支持的文件 MIME |
| `chatdomain.ErrAttachmentParseFailed` | `ATTACHMENT_PARSE_FAILED` | 422 | 内容提取失败 |
| `chatdomain.ErrAttachmentNotFound` | `ATTACHMENT_NOT_FOUND` | 404 | 附件物理丢失 |
| `chatdomain.ErrEmptyContent` | `EMPTY_CONTENT` | 400 | 发送了空消息 |

### 2.5 Function Domain
| Go Sentinel | Wire Code | HTTP | 场景 |
|---|---|---|---|
| `functiondomain.ErrNotFound` | `FUNCTION_NOT_FOUND` | 404 | |
| `functiondomain.ErrDuplicateName` | `FUNCTION_NAME_DUPLICATE` | 409 | |
| `functiondomain.ErrVersionNotFound` | `FUNCTION_VERSION_NOT_FOUND` | 404 | |
| `functiondomain.ErrExecutionNotFound` | `FUNCTION_EXECUTION_NOT_FOUND` | 404 | 历史记录查不到 |
| `functiondomain.ErrNoActiveVersion` | `FUNCTION_NO_ACTIVE_VERSION` | 422 | |
| `functiondomain.ErrEnvNotReady` | `FUNCTION_ENV_NOT_READY` | 422 | env 建不起来（fix 后仍失败） |
| `functiondomain.ErrOpInvalid` | `FUNCTION_OP_INVALID` | 422 | 锻造 op 畸形 / 草稿非法 |
| `functiondomain.ErrInvalidCode` | `FUNCTION_INVALID_CODE` | 422 | 代码终校验失败（无 def / D7 黑名单） |
| `functiondomain.ErrSandboxUnavailable` | `FUNCTION_SANDBOX_UNAVAILABLE` | 503 | sandbox runtime 未就绪 |

### 2.6 Handler Domain
| Go Sentinel | Wire Code | HTTP | 场景 |
|---|---|---|---|
| `handlerdomain.ErrNotFound` | `HANDLER_NOT_FOUND` | 404 | |
| `handlerdomain.ErrDuplicateName` | `HANDLER_NAME_DUPLICATE` | 409 | |
| `handlerdomain.ErrVersionNotFound` | `HANDLER_VERSION_NOT_FOUND` | 404 | |
| `handlerdomain.ErrCallNotFound` | `HANDLER_CALL_NOT_FOUND` | 404 | 调用日志查不到 |
| `handlerdomain.ErrMethodNotFound` | `HANDLER_METHOD_NOT_FOUND` | 404 | 调用了不存在的方法 |
| `handlerdomain.ErrNoActiveVersion` | `HANDLER_NO_ACTIVE_VERSION` | 422 | |
| `handlerdomain.ErrEnvNotReady` | `HANDLER_ENV_NOT_READY` | 422 | env 建不起来 |
| `handlerdomain.ErrConfigIncomplete` | `HANDLER_CONFIG_INCOMPLETE` | 422 | 缺必填初始化参数 |
| `handlerdomain.ErrOpInvalid` | `HANDLER_OP_INVALID` | 422 | 锻造 op 畸形 |
| `handlerdomain.ErrInvalidCode` | `HANDLER_INVALID_CODE` | 422 | 类草稿校验失败（无名/无方法）|
| `handlerdomain.ErrSandboxUnavailable` | `HANDLER_SANDBOX_UNAVAILABLE` | 503 | sandbox runtime 未就绪 |
| `handlerdomain.ErrInstanceSpawnFailed` | `HANDLER_INSTANCE_SPAWN_FAILED` | 502 | 常驻进程拉起失败 |
| `handlerdomain.ErrInstanceCrashed` | `HANDLER_CRASHED` | 502 | 常驻进程崩溃（下次调用重生）|
| `handlerdomain.ErrInstanceRPCTimeout` | `HANDLER_RPC_TIMEOUT` | 504 | 子进程通信超时 |
| `handlerdomain.ErrConfigDecryptFailed` | `HANDLER_CONFIG_DECRYPT_FAILED` | 500 | 密钥无法解密 DB 记录 |
| `handlerinfra.ErrCallFailed` | `HANDLER_CALL_FAILED` | 422 | 底层派发失败 |
| `handlerinfra.ErrInitFailed` | `HANDLER_INIT_FAILED` | 422 | __init__ 挂了 |
| `handlerinfra.ErrCrashed` | `HANDLER_INSTANCE_CRASHED_INFRA` | 422 | |
| `handlerinfra.ErrProtocol` | `HANDLER_PROTOCOL_ERROR` | 500 | RPC 协议错 |
| `handlerinfra.ErrShutdownAlready` | `HANDLER_SHUTDOWN_ALREADY` | 422 | 已关闭 |

### 2.6.1 Trigger Domain (trg_ / trf_ / tra_)
| Go Sentinel | Wire Code | HTTP | 场景 |
|---|---|---|---|
| `triggerdomain.ErrNotFound` | `TRIGGER_NOT_FOUND` | 404 | |
| `triggerdomain.ErrDuplicateName` | `TRIGGER_NAME_DUPLICATE` | 409 | |
| `triggerdomain.ErrInvalidKind` | `TRIGGER_INVALID_KIND` | 422 | 非 cron/webhook/fsnotify/sensor |
| `triggerdomain.ErrInvalidConfig` | `TRIGGER_INVALID_CONFIG` | 422 | config 结构缺字段 |
| `triggerdomain.ErrInvalidCron` | `TRIGGER_INVALID_CRON` | 422 | cron 表达式语法错 |
| `triggerdomain.ErrInvalidCEL` | `TRIGGER_INVALID_CEL` | 422 | sensor condition/output CEL 编译失败 |
| `triggerdomain.ErrInvalidInterval` | `TRIGGER_INVALID_INTERVAL` | 422 | sensor interval < 5s |
| `triggerdomain.ErrSensorTargetRequired` | `TRIGGER_SENSOR_TARGET_REQUIRED` | 422 | sensor 缺 function/handler 目标 |
| `triggerdomain.ErrWebhookSecretMismatch` | `TRIGGER_WEBHOOK_SECRET_MISMATCH` | 401 | HMAC/secret 验签失败 |
| `triggerdomain.ErrActivationNotFound` | `TRIGGER_ACTIVATION_NOT_FOUND` | 404 | |
| `triggerdomain.ErrListenerUnavailable` | `TRIGGER_LISTENER_UNAVAILABLE` | 503 | listener 未就绪 |
| `triggerdomain.ErrFiringNotPending` | `TRIGGER_FIRING_NOT_PENDING` | 409 | claim 竞争失败（scheduler 波次 4 消费）|

### 2.6b Control Logic（control 逻辑实体，ctl_）
| Go Sentinel | Wire Code | HTTP | 场景 |
|---|---|---|---|
| `controldomain.ErrNotFound` | `CONTROL_NOT_FOUND` | 404 | |
| `controldomain.ErrDuplicateName` | `CONTROL_NAME_DUPLICATE` | 409 | workspace 内同名 |
| `controldomain.ErrVersionNotFound` | `CONTROL_VERSION_NOT_FOUND` | 404 | |
| `controldomain.ErrNoActiveVersion` | `CONTROL_NO_ACTIVE_VERSION` | 422 | |
| `controldomain.ErrInvalidName` | `CONTROL_INVALID_NAME` | 422 | name 空 / 畸形 |
| `controldomain.ErrInvalidBranches` | `CONTROL_INVALID_BRANCHES` | 422 | branches 空 / port 空或重复 |
| `controldomain.ErrNoCatchAll` | `CONTROL_NO_CATCHALL` | 422 | 末条非 when:"true" 兜底 |
| `controldomain.ErrInvalidCEL` | `CONTROL_INVALID_CEL` | 422 | branch when/emit CEL 编译失败 |

### 2.6c Approval Form（审批渲染实体，apf_）
| Go Sentinel | Wire Code | HTTP | 场景 |
|---|---|---|---|
| `approvaldomain.ErrNotFound` | `APPROVAL_NOT_FOUND` | 404 | |
| `approvaldomain.ErrDuplicateName` | `APPROVAL_NAME_DUPLICATE` | 409 | workspace 内同名 |
| `approvaldomain.ErrVersionNotFound` | `APPROVAL_VERSION_NOT_FOUND` | 404 | |
| `approvaldomain.ErrNoActiveVersion` | `APPROVAL_NO_ACTIVE_VERSION` | 422 | |
| `approvaldomain.ErrInvalidName` | `APPROVAL_INVALID_NAME` | 422 | name 空 / 畸形 |
| `approvaldomain.ErrInvalidTemplate` | `APPROVAL_INVALID_TEMPLATE` | 422 | template 空 / {{ CEL }} 编译失败 |
| `approvaldomain.ErrInvalidTimeout` | `APPROVAL_INVALID_TIMEOUT` | 422 | timeout 非法 duration / behavior 缺或非法 |

### 2.7 Workflow & Execution Domain
| Go Sentinel | Wire Code | HTTP | 场景 |
|---|---|---|---|
| `workflowdomain.ErrNotFound` | `WORKFLOW_NOT_FOUND` | 404 | |
| `workflowdomain.ErrDuplicateName` | `WORKFLOW_NAME_DUPLICATE` | 409 | |
| `workflowdomain.ErrVersionNotFound` | `WORKFLOW_VERSION_NOT_FOUND` | 404 | |
| `workflowdomain.ErrPendingNotFound` | `WORKFLOW_PENDING_NOT_FOUND` | 404 | |
| `workflowdomain.ErrNoActiveVersion` | `WORKFLOW_NO_ACTIVE_VERSION` | 422 | |
| `workflowdomain.ErrDAGCycle` | `WORKFLOW_DAG_CYCLE` | 422 | 环路检测失败 |
| `workflowdomain.ErrInvalidReference` | `WORKFLOW_INVALID_REFERENCE` | 422 | 节点引用了已删除资源 |
| `workflowdomain.ErrNoTrigger` | `WORKFLOW_NO_TRIGGER` | 422 | 无启动入口 |
| `workflowdomain.ErrOpInvalid` | `WORKFLOW_OP_INVALID` | 400 | |
| `workflowdomain.ErrCapabilityNotFound` | `WORKFLOW_CAPABILITY_NOT_FOUND` | 422 | 运行期引用丢失 |
| `workflowdomain.ErrMCPServerNotInstalled` | `WORKFLOW_MCP_SERVER_NOT_INSTALLED`| 422 | |
| `workflowdomain.ErrInvalidNodeModelOverride`| `INVALID_NODE_MODEL_OVERRIDE` | 400 | Override 字段格式错 |
| `flowrundomain.ErrNotFound` | `FLOWRUN_NOT_FOUND` | 404 | |
| `flowrundomain.ErrNotCancellable` | `FLOWRUN_NOT_CANCELLABLE` | 422 | 已经结束了 |
| `flowrundomain.ErrNotPaused` | `FLOWRUN_NOT_PAUSED` | 422 | 尝试 Resume 未暂停的任务 |
| `flowrundomain.ErrApprovalNodeNotFound` | `FLOWRUN_APPROVAL_NODE_NOT_FOUND`| 404 | 节点 ID 匹配错 |
| `flowrundomain.ErrApprovalDecisionInvalid` | `FLOWRUN_APPROVAL_DECISION_INVALID`| 400 | decision 只能是 yes/no |
| `flowrundomain.ErrNodeNotFound` | `FLOWRUN_NODE_NOT_FOUND` | 404 | 原子节点历史查不到 |
| `schedulerapp.ErrWorkflowDisabled` | `WORKFLOW_DISABLED` | 422 | 尝试触发禁用流 |
| `schedulerapp.ErrWorkflowNeedsAttention` | `WORKFLOW_NEEDS_ATTENTION` | 422 | 自动下线的流 |
| `schedulerapp.ErrConcurrencyLimit` | `FLOWRUN_CONCURRENCY_LIMIT` | 409 | Serial 已满 |
| `schedulerapp.ErrNotReplayable` | `FLOWRUN_NOT_REPLAYABLE` | 422 | 只有 Failed 且 Generation < Max 可重跑 |
| `schedulerapp.ErrApprovalRequired` | `APPROVAL_REQUIRED` | 202 | 正常暂停，需前端切审批页 |
| `schedulerapp.ErrLoopBodyNotSupported` | `LOOP_BODY_NOT_SUPPORTED` | 422 | 仅限 V1.5+ |
| `schedulerapp.ErrParallelBranchNotSupported`| `PARALLEL_BRANCH_NOT_SUPPORTED`| 422 | |
| `schedulerapp.ErrSubDAGContainsApproval` | `SUBDAG_CONTAINS_APPROVAL` | 422 | 嵌套图禁止再等审批 |

### 2.8 Sandbox & Infrastructure Domain
| Go Sentinel | Wire Code | HTTP | 场景 |
|---|---|---|---|
| `sandboxdomain.ErrRuntimeNotSupported` | `SANDBOX_RUNTIME_NOT_SUPPORTED` | 422 | runtime kind 未注册 |
| `sandboxdomain.ErrRuntimeInstallFailed` | `SANDBOX_RUNTIME_INSTALL_FAILED`| 502 | mise/docker 安装失败 |
| `sandboxdomain.ErrRuntimeNotFound` | `SANDBOX_RUNTIME_NOT_FOUND` | 404 | 内部查找未命中（EnsureRuntime 消化，通常不冒泡）|
| `sandboxdomain.ErrEnvNotFound` | `SANDBOX_ENV_NOT_FOUND` | 404 | |
| `sandboxdomain.ErrEnvCreateFailed` | `SANDBOX_ENV_CREATE_FAILED` | 502 | |
| `sandboxdomain.ErrDepInstallFailed` | `SANDBOX_DEP_INSTALL_FAILED` | 502 | pip 失败 |
| `sandboxdomain.ErrSpawnFailed` | `SANDBOX_SPAWN_FAILED` | 502 | |
| `sandboxdomain.ErrSpawnTimeout` | `SANDBOX_SPAWN_TIMEOUT` | 504 | |
| `sandboxdomain.ErrEnvInUse` | `SANDBOX_ENV_IN_USE` | 409 | |
| `sandboxdomain.ErrInvalidOwnerID` | `SANDBOX_INVALID_OWNER_ID` | 400 | ID 含非法字符 |
| `sandboxdomain.ErrCmdRequired` | `SANDBOX_CMD_REQUIRED` | 400 | |
| `sandboxdomain.ErrDockerNotInstalled` | `SANDBOX_DOCKER_NOT_INSTALLED` | 422 | |
| `sandboxdomain.ErrDockerDaemonDown` | `SANDBOX_DOCKER_DAEMON_DOWN` | 503 | |

### 2.9 MCP Domain
| Go Sentinel | Wire Code | HTTP | 场景 |
|---|---|---|---|
| `mcpdomain.ErrServerNotFound` | `MCP_SERVER_NOT_FOUND` | 404 | 短名查不到已装 server |
| `mcpdomain.ErrServerNotConnected` | `MCP_SERVER_DOWN` | 503 | 连接断 / 子进程崩，暂不可用 |
| `mcpdomain.ErrToolNotFound` | `MCP_TOOL_NOT_FOUND` | 404 | server 未自报此工具 |
| `mcpdomain.ErrToolCallFailed` | `MCP_RPC_ERROR` | 502 | 上游 server 返回错误 JSON-RPC |
| `mcpdomain.ErrToolCallTimeout` | `MCP_TOOL_TIMEOUT` | 504 | CallTool 超 `timeout_sec` |
| `mcpdomain.ErrNameConflict` | `MCP_NAME_CONFLICT` | 409 | 短名在工作区内已占用 |
| `mcpdomain.ErrInstallFailed` | `MCP_INSTALL_FAILED` | 502 | 装包 / 连接失败 |
| `mcpdomain.ErrEnvMissing` | `MCP_ENV_MISSING` | 422 | 缺必填 env |
| `mcpdomain.ErrRegistryEntryNotFound` | `MCP_REGISTRY_NOT_FOUND` | 404 | slug 不在 registry |
| `mcpdomain.ErrNoRunnablePackage` | `MCP_NO_RUNNABLE_PACKAGE` | 422 | registry entry 无可装 package |

### 2.10 Knowledge & Skills Domain
| Go Sentinel | Wire Code | HTTP | 场景 |
|---|---|---|---|
| `skilldomain.ErrNotFound` | `SKILL_NOT_FOUND` | 404 | SKILL.md 不存在 |
| `skilldomain.ErrInvalidName` | `SKILL_INVALID_NAME` | 400 | name 非 slug / 路径穿越 |
| `skilldomain.ErrInvalidFrontmatter` | `SKILL_INVALID_FRONTMATTER` | 422 | YAML 坏 / description 缺或超长 / source 非法 |
| `skilldomain.ErrBodyTooLarge` | `SKILL_BODY_TOO_LARGE` | 422 | body > 32 KiB |
| `skilldomain.ErrNameConflict` | `SKILL_NAME_CONFLICT` | 409 | create 同名已存在 |
| `skilldomain.ErrForkRequiresAgent` | `SKILL_FORK_REQUIRES_AGENT` | 422 | context=fork 缺 agent |
| `skilldomain.ErrSubagentUnavailable` | `SKILL_SUBAGENT_UNAVAILABLE` | 503 | fork 但 subagent runner 未装（波次 5 前） |
| `memorydomain.ErrNotFound` | `MEMORY_NOT_FOUND` | 404 | 记忆文件不存在 |
| `memorydomain.ErrInvalidName` | `MEMORY_INVALID_NAME` | 400 | name 非小写 slug |
| `memorydomain.ErrInvalidSource` | `MEMORY_INVALID_SOURCE` | 400 | source 非 user/ai |
| `memorydomain.ErrInvalidInput` | `MEMORY_INVALID_INPUT` | 400 | description/content 缺 |
| `documentdomain.ErrNotFound` | `DOCUMENT_NOT_FOUND` | 404 | |
| `documentdomain.ErrInvalidParent` | `DOCUMENT_INVALID_PARENT` | 422 | 自引或循环引 |
| `documentdomain.ErrNameConflict` | `DOCUMENT_NAME_CONFLICT` | 409 | |
| `documentdomain.ErrContentTooLarge` | `DOCUMENT_CONTENT_TOO_LARGE` | 413 | |
| `documentdomain.ErrInvalidName` | `DOCUMENT_INVALID_NAME` | 400 | |
| `documentdomain.ErrParentNotFound` | `DOCUMENT_PARENT_NOT_FOUND` | 422 | |

### 2.11 Other Domains (Model/Workspace/Rel/Catalog)
| Go Sentinel | Wire Code | HTTP | 场景 |
|---|---|---|---|
| `modeldomain.ErrScenarioInvalid` | `MODEL_SCENARIO_INVALID` | 400 | 非 dialogue/utility/agent |
| `modeldomain.ErrNotConfigured` | `MODEL_NOT_CONFIGURED` | 422 | 该 scenario 无默认模型，提示去配置 |
| `modeldomain.ErrRefInvalid` | `MODEL_REF_INVALID` | 400 | ModelRef 缺 apiKeyId 或 modelId |
| `workspacedomain.ErrNotFound` | `WORKSPACE_NOT_FOUND` | 404 | |
| `workspacedomain.ErrNameRequired` | `WORKSPACE_NAME_REQUIRED` | 400 | |
| `workspacedomain.ErrNameTooLong` | `WORKSPACE_NAME_TOO_LONG` | 400 | 超过 64 字符 |
| `workspacedomain.ErrNameConflict` | `WORKSPACE_NAME_CONFLICT` | 409 | |
| `workspacedomain.ErrCannotDeleteLast` | `CANNOT_DELETE_LAST_WORKSPACE` | 422 | |
| `workspacedomain.ErrLanguageInvalid` | `WORKSPACE_LANGUAGE_INVALID` | 400 | |
| `relationdomain.ErrInvalidRef` | `REL_INVALID_REF` | 400 | 源/目标 ref 空 id 或未知实体类型 |
| `relationdomain.ErrInvalidKind` | `REL_INVALID_KIND` | 400 | 边类型非 create/edit/equip/link |
| `relationdomain.ErrSelfLoop` | `REL_SELF_LOOP` | 400 | 禁止自环（from == to）|
| `relationdomain.ErrDepthOutOfRange` | `REL_DEPTH_LIMIT` | 400 | neighborhood 深度超 [1,3] |
| `relationdomain.ErrIncompleteFilter` | `REL_INCOMPLETE_FILTER` | 400 | filter 的 kind/id 未成对 |
| `catalogdomain.ErrAllSourcesFailed` | `CATALOG_ALL_SOURCES_FAILED` | 503 | 所有 source 失败（系统故障，如 DB 不可达）|
| `triggerdomain.ErrPathNotExist` | `TRIGGER_PATH_NOT_EXIST` | 422 | |
| `triggerdomain.ErrPathConflict` | `TRIGGER_PATH_CONFLICT` | 409 | |
| `triggerdomain.ErrWebhookSecretMismatch` | `TRIGGER_WEBHOOK_SECRET_MISMATCH`| 401 | |
| `triggerdomain.ErrInvalidCronExpression` | `TRIGGER_INVALID_CRON_EXPRESSION`| 400 | |
| `triggerdomain.ErrFiringNotPending` | `INTERNAL_ERROR` | 500 | [未映射] 并发冲突 |
| `notificationdomain.ErrNotFound` | `NOTIFICATION_NOT_FOUND` | 404 | MarkRead 未知 id |
| `notificationdomain.ErrInvalidType` | `NOTIFICATION_INVALID_TYPE` | 400 | Emit 空 type |

### 2.12 LLM Upstream Classifications
| Go Sentinel | Wire Code | HTTP |
|---|---|---|
| `llminfra.ErrAuthFailed` | `LLM_AUTH_FAILED` | 401 |
| `llminfra.ErrRateLimited` | `LLM_RATE_LIMITED` | 429 |
| `llminfra.ErrBadRequest` | `LLM_BAD_REQUEST` | 400 |
| `llminfra.ErrModelNotFound` | `LLM_MODEL_NOT_FOUND` | 404 |
| `llminfra.ErrProviderError` | `LLM_PROVIDER_ERROR` | 502 |

---

## 3. 未映射 (Fallback 500) 审计清单

以下 Sentinel 目前尚未在 `errmap.go` 登记，前端收到时 Code 均为 `INTERNAL_ERROR`：
- `reqctxpkg.ErrMissingUserID`
- `reqctxpkg.ErrMissingConversationID`
- `cryptoinfra.ErrUnsupportedVersion`
- `triggerdomain.ErrFiringNotPending`
- `chatdomain.ErrBlockNotFound`
- `subagentdomain.ErrRecursionAttempt`
- `askapp.ErrNoPendingQuestion` (注：API 直接处理了 ask，此处为 app 层兜底)
- ...以及所有 Go 内部 `fmt.Errorf` 产生的动态错误。

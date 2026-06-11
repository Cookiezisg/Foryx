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

# 错误码 —— 错误系统 + wire code 注册

> 后端错误的单一事实源。框架/规约在此；**逐域的码枚举随各模块评审填**（见 `domains/<域>.md` + 本表的命名空间）。

## 框架（`pkg/errors`）

`errorspkg.Error{Kind, Code, Message, Details, cause}`，`New(kind, code, msg)` 构造。**类型在 `pkg/errors`（地基、全层可用、纯机制）**——所有命名 sentinel 一律 `errorspkg.New`，无"是否冒泡 HTTP"之分（见 [`decisions/0002`](../../decisions/0002-unified-error-type.md)）。

- `Is` **按 Code 匹配** → sentinel 与其 `WithCause`/`WithDetails` 副本在 `errors.Is` 下相等。
- 两种出口：**HTTP** 读 Kind(→状态码)/Code/Details 走 N1 Envelope；**LLM tool** 读 Message。
- 包裹用 `fmt.Errorf("…: %w", err)`（保留链）；`errors.Is/As` 用标准库。

## Kind → HTTP（15，封闭集；零值 `KindInternal` 安全兜底）

唯一映射表 = `transport/httpapi/response/errmap.go::statusForKind`（transport 不持逐错误表、不 import 业务 domain）。

| Kind | HTTP | Kind | HTTP | Kind | HTTP |
|---|---|---|---|---|---|
| Internal | 500 | Unprocessable | 422 | GatewayTimeout | 504 |
| Invalid | 400 | TooLarge | 413 | Accepted | 202 |
| Unauthorized | 401 | UnsupportedMedia | 415 | ClientClosed | 499 |
| NotFound | 404 | RateLimited | 429 | Gone | 410 |
| Conflict | 409 | BadGateway | 502 | Unavailable | 503 |

## wire code 命名规约

**`<ENTITY>_<REASON>`，SCREAMING_SNAKE，按实体命名空间，全库唯一。** 范例（function 模板）：`FUNCTION_NOT_FOUND` · `FUNCTION_NAME_DUPLICATE` · `FUNCTION_NO_ACTIVE_VERSION`。

## 命名空间注册（前缀 → 拥有模块；逐域码在各域评审时枚举）

| 前缀 | 模块 | 前缀 | 模块 |
|---|---|---|---|
| `FUNCTION_` | function | `WEB_` / `WEBSEARCH_` | web fetch / websearch tool |
| `HANDLER_` | handler | `FS_` | filesystem tool |
| `AGENT_` | agent | `SEARCH_` | grep/search tool |
| `WORKFLOW_` / `FLOWRUN_` | workflow / flowrun | `SHELL_` | shell tool |
| `TRIGGER_` / `CONTROL_` / `APPROVAL_` | 图节点 | `TOOLSET_` | search_tools |
| `SKILL_` / `MCP_` / `DOCUMENT_` | 挂载/协议 | `TODO_` | todo |
| `CONVERSATION_` / `CHAT_` / `MESSAGE_` | 对话运行时 | `MEMORY_` / `ASK_` | memory / ask 工具 |
| `ORM_` | pkg/orm（兜底，domain 翻译成具体码） | `CRYPTO_` / `HANDLER_`（infra） | infra 原语 |
| `MALFORMED_CURSOR` / `MISSING_WORKSPACE_ID` / `MISSING_CONVERSATION_ID` / `FSPATH_*` | pkg 原语 | | |

## 跨域 sentinel（`pkg/errors/sentinel.go`）

| Code | Kind | HTTP | 场景 |
|---|---|---|---|
| `INVALID_REQUEST` | Invalid | 400 | domain 逻辑前的格式/语义无效 |
| `UNAUTH_NO_WORKSPACE` | Unauthorized | 401 | workspace 隔离路由缺有效 workspace id；前端清 workspace 重选 |

> 逐域完整码表（每个 sentinel 的 code/kind/场景）随对应 `domains/<域>.md` 评审填入本文件。

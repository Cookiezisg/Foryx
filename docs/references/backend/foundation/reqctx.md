---
id: DOC-006
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
audience: [human, ai]
---

# reqctx —— 请求作用域 ctx 载体（地基）

## 1. 定位

`pkg/reqctx` 通过 `context.Context` 携带请求作用域的值——纯 stdlib、无上层依赖、私有 empty-struct key 防冲突。它是**横切接线层**：workspace id 由 HTTP 中间件在任何业务包存在前写入 ctx（放进 workspace 业务模块会倒置层级）。S9：每个跨层调用强制传 `ctx`。

## 2. 携带的值

| 值 | API | 注入者 | 读取者 |
|---|---|---|---|
| workspace id | `Set`/`Get`/`RequireWorkspaceID` | auth 中间件（唯一的源）+ ~15 detached 重埋 | **orm 自动隔离** + ~20 包 |
| conversation id | `Set`/`Get`/`RequireConversationID` | chat / subagent | loop + 多 app |
| subagent / message / toolCall id | `Set`/`Get*` | loop / chat / subagent | 流式嵌套（E3）/ 归属 |
| flowrun / flowrunNode id | `Set`/`Get*`（只 Get、无 Require——缺席=非 workflow 派发，非错误） | **workflow 调度器**（runNode 派发前） | function/handler/agent 执行记账填 flowrun 审计列 |
| locale | `Set`/`GetLocale`（总返可用值，默认 zh-CN） | `InjectLocale`（Accept-Language，pre-workspace 兜底）→ `IdentifyWorkspace`（**workspace.language 权威**，识别到 workspace 即覆盖）+ chat detached 重埋 | AI 生成内容语言 |
| agentState | `With`/`GetAgentState` | chat / subagent runner | loop / tool / skill |

## 3. 横切链路（单看包看不见，必须全项目看）

```
入口注入：auth 中间件 SetWorkspaceID（从 session）—— 唯一的"源"
   ↓ ctx 一路下传（S9：每跨层调用带 ctx）
读取：orm.whereClause 自动 ws 过滤（隔离安全网）+ RequireWorkspaceID（~20 包）
   ↓ 工作脱离请求（异步 / 比请求活得久）时
detached 重播种：reqctx.Detached(wsID)[+SetConversationID]（~15 站点）
```

## 4. workspace 隔离的两个错误（别混 —— 同 §[error-codes](../error-codes.md)）

| 错误 | Kind/HTTP | 谁的错 | 何时 |
|---|---|---|---|
| `UNAUTH_NO_WORKSPACE`（`pkg/errors/sentinel.go`） | Unauthorized / **401** | **客户端** | 未带有效 workspace 命中隔离路由 → `RequireWorkspace` 中间件在边界拒、前端清 workspace 重选 |
| `MISSING_WORKSPACE_ID`（本包） | Internal / **500** | **接线 bug** | 中间件已过 / detached 已埋的前提下 `RequireWorkspaceID` 仍空 = 中间件被跳过或 detached 忘重埋 |

> 401 是客户端的事、500 是我们的。`ErrMissingConversationID` 同理 500（对称）。

## 5. Detached Context 惯例（S9）

异步工作（finalize / best-effort 后台写 / 自动标题）必须**比派生它的请求活得久** → 用 `reqctx.Detached(wsID)`：从 `context.Background()` 起、重埋 workspace（orm 隔离最低要求），按需链 `SetConversationID`。~15 站点统一走它。

- **为何 `Background()` 而非 `WithoutCancel(ctx)`**：要的就是脱离**已取消**的请求——回合取消正是 finalize 触发时机（被取消的 subagent 仍须落终态，防孤儿）。
- **为何重埋而非沿用 ctx**：trigger / scheduler 起的异步**无请求 ctx**；workspace 来自实体行，不取已死的请求 ctx。
- 每站点只重埋它需要的子集（finalize 类 ws-only；对话延续类 ws+conv）——精确、不多埋。

## 6. 边界 / 集成

- **其它 ctx 能力**（stream bridge / humanloop broker / progress sink）各自包持私有 key 经 ctx 注入（DIP——reqctx 不依赖 `streamdomain` 等上层），**不并入 reqctx**：正确分层、非 bypass。
- 错误经 [`pkg/errors`](../error-codes.md)：`MISSING_WORKSPACE_ID`(500) · `MISSING_CONVERSATION_ID`(500)，见 §4。
- 隔离的下游消费者是 [`orm`](orm.md)（`whereClause` 读 workspace id 自动过滤）。

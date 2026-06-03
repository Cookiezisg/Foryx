# 契约变更日志（驱动覆盖后的前端 / testend 兼容）

> 重写时每改一处对外契约（REST envelope / 路由 / SSE 事件 / error code），在此**追加一条**。
> 覆盖回 `backend/` 后，前端 + testend 照此表逐条施工——这是兼容工作的**唯一施工图**，记糙了兼容就会乱。
> 现有契约有 AI 瞎改的部分，把它改对**也要记**（"为什么"注明"修正 AI 瞎改"）。

## 记录格式

每条：模块/波次 · 类型 · 原契约 · 新契约 · 为什么 · 前端&testend 受影响点。

| # | 模块/波次 | 类型 | 原契约 | 新契约 | 为什么 | 前端/testend 受影响点 |
|---|---|---|---|---|---|---|
| 1 | errors（M0.4 R0012） | error code + auth header | code `UNAUTH_NO_USER`；header `X-Forgify-User-ID` | code `UNAUTH_NO_WORKSPACE`；header `X-Forgify-Workspace-ID` | user→workspace 全局正名 | 前端 401 拦截/重选逻辑判的 code；`localStorage.activeUserId`→`activeWorkspaceId`；请求头改名（header 实际在 M0.7 middleware 落地，code 已在 errors 定型） |
| 2 | SSE 三流（M0.4/M0.5） | **端点改名 + 数据结构全量重构** | 端点 `/api/v1/{eventlog,forge,notifications}`；三流异构事件（eventlog 5 强类型 message/block · forge 4 强类型 forge_* · notifications 通用 `{type,id,data}`） | 端点 `/api/v1/{messages,entities,notifications}`；**统一「流式树」协议**：`Envelope{seq,scope,id,frame}`，frame ∈ open/delta/close/signal，node 为 `{type,...}` 判别联合（见 `stream-protocol.md`） | eventlog→messages、forge→**entities**（全实体流式总线：创建/编辑流式内容 + 运行流式输出 + 双输出）；三流数据结构标准化统一、scope/id 升信封层 | 前端 SSE 消费层全改：EventSource URL、事件解析（按 `frame` + `node.type` 判别联合）、消息树递归渲染（open→delta→close）、entities 面板消费、双输出按共享 node id 关联；testend 三流 SSE 断言全改 |

## 覆盖后兼容清单（从上表自动汇总）

- 前端（`frontend/src/shared/api` + entities）：
- testend（`testend/src`）：

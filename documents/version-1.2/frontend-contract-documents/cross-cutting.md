# 横切机制契约 — 一眼索引

**关联**：
- [`../frontend-design.md`](../frontend-design.md) — 设计理由（为什么这样切）
- [`./fsd-layers.md`](./fsd-layers.md) — 层定义 / slice 清单
- [`./entity-types.md`](./entity-types.md) — entity TS 类型表
- 后端：[`../service-contract-documents/events-design.md`](../service-contract-documents/events-design.md)（SSE 协议权威）

**定位**：**一眼看到 DIP / errorMap / SSE / queryKeys / toastStore / i18n 的接口和数据流**。理由和演进历史去 `frontend-design.md`。

---

## 1. DIP 注入点（解 shared→上层反向依赖）

`shared` 层不可依赖上层（FSD 铁律）。下表是两个 DIP port，结构与后端 `domain` 定 port / `main.go` wire 同构。

### 1.1 authProvider（身份 DIP）

**文件**：`frontend/src/shared/api/authProvider.ts`

| 导出 | 类型 | 说明 |
|---|---|---|
| `setUserIdProvider(fn)` | `(fn: () => string \| null) => void` | 注册 userId 读取函数；app 启动注入 |
| `setOnAuthFailure(fn)` | `(fn: () => void) => void` | 注册 401 响应回调；app 启动注入 |
| `getUserId()` | `() => string \| null` | httpClient / sse 调用，不知 session 存在 |
| `notifyAuthFailure()` | `() => void` | httpClient / sse 永久断连时调用 |

**app 注入点**：`app/model/useSessionBootstrap.ts`

```ts
setUserIdProvider(() => useSessionStore.getState().currentUserId);
setOnAuthFailure(() => { void resolveSession().catch(() => {}); });
```

默认 provider 返回 `null`（无 DI 时不发请求）；默认 failure 是 noop。

### 1.2 navigation（导航 DIP）

**文件**：`frontend/src/shared/lib/navigation.ts`

```ts
interface Navigator {
  openConv(id: string): void;
  openEntity(pane: string, id: string): void;
  openPane(pane: string): void;
  setActiveDocument(id: string): void;
}
```

| 导出 | 类型 | 说明 |
|---|---|---|
| `setNavigator(n)` | `(n: Navigator) => void` | app 启动注入真实实现（操作 paneStore）|
| `navigate` | `Navigator` | widgets/features 调用的代理对象 |

**app 注入点**：`app/AppShell.tsx`（`useEffect` 调 `setNavigator`）。

widgets / features 调 `navigate.openConv(id)` 等，不 import `app/model`（反向禁）。

---

## 2. errorMap（错误码 → i18n key）

**文件**：`frontend/src/shared/api/errorMap.ts`

对位后端 `internal/transport/httpapi/<domain>/errmap.go`，保持同步（§F1）。

### code → key 映射表

| 后端错误码 | i18n key（errors namespace）| kind |
|---|---|---|
| `UNAUTH_NO_USER` | `errors:UNAUTH_NO_USER` | error |
| `CONVERSATION_NOT_FOUND` | `errors:CONVERSATION_NOT_FOUND` | **warn** |
| `STREAM_IN_PROGRESS` | `errors:STREAM_IN_PROGRESS` | error |
| `LLM_PROVIDER_ERROR` | `errors:LLM_PROVIDER_ERROR` | error |
| `LLM_AUTH_FAILED` | `errors:LLM_AUTH_FAILED` | error |
| `LLM_RATE_LIMITED` | `errors:LLM_RATE_LIMITED` | error |
| `LLM_BAD_REQUEST` | `errors:LLM_BAD_REQUEST` | error |
| `LLM_MODEL_NOT_FOUND` | `errors:LLM_MODEL_NOT_FOUND` | error |
| `MODEL_NOT_CONFIGURED` | `errors:MODEL_NOT_CONFIGURED` | error |
| `API_KEY_NOT_FOUND` | `errors:API_KEY_NOT_FOUND` | error |
| `API_KEY_PROVIDER_NOT_FOUND` | `errors:API_KEY_PROVIDER_NOT_FOUND` | error |
| `FUNCTION_NOT_FOUND` | `errors:FUNCTION_NOT_FOUND` | error |
| `FUNCTION_RUN_FAILED` | `errors:FUNCTION_RUN_FAILED` | error |
| `HANDLER_NOT_FOUND` | `errors:HANDLER_NOT_FOUND` | error |
| `WORKFLOW_NOT_FOUND` | `errors:WORKFLOW_NOT_FOUND` | error |
| `INTERNAL_ERROR` | `errors:INTERNAL_ERROR` | error |
| `NETWORK`（client-side）| `errors:NETWORK` | error |
| *(未知码)* | `errors:fallback` | error |

### API

```ts
errorKey(code: string): string      // → i18n key; 调用方 t(key)
kindForCode(code: string): "error" | "warn"
```

### 全局 onError 流程

TanStack QueryClient 全局 `onQueryError` / `onMutationError`：

```
API 响应 4xx/5xx
  → httpClient 抛 ApiError{ code, message }
    → QueryClient.onError
      → errorKey(code) → t(key)
        → useToastStore.pushToast({ kind: kindForCode(code), desc: t(key) })
```

feature hook 只需 `throw` 或让 mutation 自然失败，不手写 `pushToast`。

---

## 3. SSE 三流

后端协议权威：`service-contract-documents/events-design.md`（E1）。本节记录**前端消费侧**接口。

**上限 3 条，永不再加。** 三流均在 `app/sse/SSEProvider.tsx` 单例挂载（每 userId 一套连接）。

### 3.1 eventlog — `/api/v1/eventlog`

**hook**：`app/sse/useEventLog.ts`
**分发目标**：`entities/conversation/model/chatStore`（消息树增量更新）

| SSE 事件名 | payload 关键字段 | chatStore 方法 |
|---|---|---|
| `message_start` | `conversationId` / `id` / `role` / `parentBlockId` | `ensureConv` + `onMessageStart` |
| `message_stop` | `conversationId` / `id` / `status` / token counts | `onMessageStop` |
| `block_start` | `conversationId` / `id` / `messageId` / `type` / `parentId` | `ensureConv` + `onBlockStart` |
| `block_delta` | `conversationId` / `id` / `delta` | `onBlockDelta` |
| `block_stop` | `conversationId` / `id` / `status` / `durationMs` | `onBlockStop` |

**重连机制**：`useEffect` keyed on `activeUserId`（切账号时 tear-down 旧 EventSource）；`EventSource` 内置 Last-Event-ID 断线重连。

### 3.2 forge — `/api/v1/forge`

**hook**：`app/sse/useForge.ts`
**分发目标**：`shared/model/forgeProgress`（zustand，scopeKey = `"{kind}:{id}"`）

| SSE 事件名 | payload 关键字段 | forgeProgress 动作 |
|---|---|---|
| `forge_started` | `scope{kind,id}` / `operation` / `conversationId` / `toolCallId` | `put(key, { status:"running", ops:[], envAttempts:[] })` |
| `forge_op_applied` | `scope` / `index` / `op` | `put(key, { ...cur, ops:[...ops, {index,op}] })` |
| `forge_env_attempt` | `scope` / `attempt` / `status` / `stage?` / `detail?` / `error?` | `put(key, { ...cur, envAttempts:[...] })` |
| `forge_completed` | `scope` / `status` / `versionId?` / `envStatus?` / `attemptsUsed?` / `error?` | `put(key, { status })` + `qc.invalidateQueries(...)` |

**完成后 invalidate 映射**（`forge_completed` 时）：

| scope.kind | 失效的 queryKey |
|---|---|
| `function` | `qk.functions()` / `qk.function(id)` / `qk.functionVersions(id)` |
| `handler` | `qk.handlers()` / `qk.handler(id)` / `qk.handlerVersions(id)` |
| `workflow` | `qk.workflows()` / `qk.workflow(id)` / `qk.workflowVersions(id)` |

### 3.3 notifications — `/api/v1/notifications`

**hook**：`app/sse/useNotifications.ts`
**分发目标**：TanStack Query invalidation + `overlayStore.setPendingAsk`
**`PendingAsk` 类型**：`shared/api/types.ts`（features/widgets 直接从 `@shared/api` 导入，不走 `@app/model`）

SSE 事件名固定为 `notification`；dispatch 由 `payload.type` 字段驱动。

| notification type | 触发动作 |
|---|---|
| `ask` (action=pending) | `overlayStore.setPendingAsk({ id, conversationId, toolCallId, ...data })` |
| `ask` (action=resolved) | `overlayStore.setPendingAsk(null)` |
| `conversation` | invalidate `qk.conversations()` + `qk.conversation(id)` |
| `function` | invalidate functions + function(id) + functionVersions(id) |
| `handler` | invalidate handlers + handler(id) + handlerVersions(id) + handlerConfig(id) |
| `workflow` | invalidate workflows + workflow(id) + workflowVersions(id) |
| `flowrun` | invalidate flowruns + flowrun(id) + flowrunNodes(id) |
| `mcp_server` | invalidate `qk.mcpServers()` |
| `skill` | invalidate `qk.skills()` |
| `memory` | invalidate `["memories"]` |
| `todo` / `sandbox_env` / `compaction` | 无操作 / invalidate conversation(id) |

---

## 4. queryKeys（qk 工厂）

**文件**：`frontend/src/shared/api/queryKeys.ts`

集中管理所有 TanStack Query key，消除散落 magic string。

| 工厂 | key shape | 对应 REST 资源 |
|---|---|---|
| `qk.health()` | `["health"]` | `GET /api/v1/health` |
| `qk.users()` | `["users"]` | `GET /api/v1/users` |
| `qk.conversations()` | `["conversations"]` | `GET /api/v1/conversations` |
| `qk.conversation(id)` | `["conv", id]` | `GET /api/v1/conversations/{id}` |
| `qk.messages(convId)` | `["conv-messages", convId]` | `GET /api/v1/conversations/{id}/messages` |
| `qk.apikeys()` | `["api-keys"]` | `GET /api/v1/api-keys` |
| `qk.providers()` | `["providers"]` | `GET /api/v1/providers` |
| `qk.scenarios()` | `["scenarios"]` | `GET /api/v1/scenarios` |
| `qk.modelConfigs()` | `["model-configs"]` | `GET /api/v1/model-configs` |
| `qk.functions()` | `["functions"]` | `GET /api/v1/functions` |
| `qk.function(id)` | `["function", id]` | `GET /api/v1/functions/{id}` |
| `qk.functionVersions(id)` | `["function-versions", id]` | `GET /api/v1/functions/{id}/versions` |
| `qk.handlers()` | `["handlers"]` | `GET /api/v1/handlers` |
| `qk.handler(id)` | `["handler", id]` | `GET /api/v1/handlers/{id}` |
| `qk.handlerVersions(id)` | `["handler-versions", id]` | `GET /api/v1/handlers/{id}/versions` |
| `qk.handlerConfig(id)` | `["handler-config", id]` | `GET /api/v1/handlers/{id}/config` |
| `qk.workflows()` | `["workflows"]` | `GET /api/v1/workflows` |
| `qk.workflow(id)` | `["workflow", id]` | `GET /api/v1/workflows/{id}` |
| `qk.workflowVersions(id)` | `["workflow-versions", id]` | `GET /api/v1/workflows/{id}/versions` |
| `qk.flowruns()` | `["flowruns"]` | `GET /api/v1/flowruns` |
| `qk.flowrun(id)` | `["flowrun", id]` | `GET /api/v1/flowruns/{id}` |
| `qk.flowrunNodes(id)` | `["flowrun-nodes", id]` | `GET /api/v1/flowruns/{id}/nodes` |
| `qk.skills()` | `["skills"]` | `GET /api/v1/skills` |
| `qk.skill(id)` | `["skill", id]` | `GET /api/v1/skills/{id}` |
| `qk.mcpServers()` | `["mcp-servers"]` | `GET /api/v1/mcp-servers` |
| `qk.memories(type?)` | `["memories", type\|"all"]` | `GET /api/v1/memories` |
| `qk.memory(name)` | `["memory", name]` | `GET /api/v1/memories/{name}` |
| `qk.documents()` | `["documents"]` | `GET /api/v1/documents` |
| `qk.document(id)` | `["document", id]` | `GET /api/v1/documents/{id}` |
| `qk.relations(entityId)` | `["relations", entityId]` | `GET /api/v1/relations?entityId=...` |
| `qk.notificationsSnap()` | `["notifications-snapshot"]` | `GET /api/v1/notifications/snapshot` |

---

## 5. toastStore

**文件**：`frontend/src/shared/ui/toastStore.ts`

纯 UI 通知原语，无业务语义。归 `shared/ui` 使 widgets/toaster 可渲染，任意下层可 `pushToast`。

```ts
interface Toast {
  id: string;
  kind?: "success" | "error" | "warn" | "info";
  title?: string;
  desc?: string;
  duration?: number;   // ms；0 = 不自动消失；默认 5000
  undo?: () => void;
}

useToastStore.pushToast(t: Omit<Toast, "id">): string   // 返回 id
useToastStore.dismissToast(id: string): void
```

**渲染**：`widgets/toaster`（`ToastTray` 组件）。

---

## 6. i18n

**配置文件**：`frontend/src/shared/lib/i18n/index.ts`（`react-i18next` 实例）

**词典目录**：`frontend/src/shared/lib/i18n/locales/{zh,en}/<ns>.json`

### namespace 清单

| namespace | 覆盖内容 |
|---|---|
| `common` | 通用动词（保存 / 取消 / 删除 / 确认 …）|
| `errors` | 所有错误码人类文案（对位 errorMap）|
| `conv` | 对话页 / chat 相关文案 |
| `forge` | 锻造页 / 版本 / 迭代相关文案 |
| `execute` | 执行页 / flowrun 相关文案 |
| `library` | 资产库页文案 |
| `dashboard` | 仪表盘文案 |
| `settings` | 设置页文案 |
| `onboarding` | 引导流程文案 |
| `sidebar` | 侧边栏 / 导航文案 |
| `toast` | Toast 通知文案 |
| `misc` | 其余杂项文案 |

### 切换机制

1. `entities/settings/model/settingsStore.ts` 持久化 `lang: "zh" | "en"`（默认由 `navigator.language` 检测）。
2. `app/App.tsx` `useEffect` 监听 `prefs.lang` → `i18n.changeLanguage(prefs.lang)`。
3. 后端请求随带 `Accept-Language` header（由 `httpClient` 从 `getUserId()` 同路线注入 lang）。

**使用规范**（CLAUDE.md 全量中英双语要求）：
- 用户可见文案走 `useTranslation("<ns>")` + `t("key")`，**不硬编码中/英文串**。
- 夹 JSX 元素用 `<Trans>`；日期/数字格式按 `i18n.language` 取 locale。

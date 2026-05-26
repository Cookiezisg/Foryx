# entities/handler — 前端 slice 详细设计

**所属层**：entities（对位后端 domain/handler）
**状态**：✅ 已实现
**职责**：管理 Handler（持久化 Python 类 / 服务）的 CRUD + 版本管理（pending accept/reject）+ 方法调用 + 配置管理。Handler 有独立的 init args 配置状态（configState）和在线实例计数。

**关联文档**：
- [`../frontend-design.md`](../frontend-design.md) — FSD 总规范
- 后端 `service-design-documents/handler.md`

---

## 1. 职责边界

- 列表 / 详情 / 版本历史 / 配置查询
- pending 版本审批（accept / reject）
- 方法调用（call）
- 软删除

不含 AI 迭代（features/forge-iterate）、运行日志（entities/flowrun）。

---

## 2. 类型（`model/types.ts`）

```ts
type EnvStatus = "pending" | "syncing" | "ready" | "failed" | "evicted";
type VersionStatus = "pending" | "accepted" | "rejected";
type ConfigState = "unconfigured" | "partially_configured" | "ready";

interface ArgSpec { name; type; description?; required; default? }
interface InitArgSpec { name; type; description?; required; sensitive; default? }
interface MethodSpec {
  name; description?; args: ArgSpec[]; returnSchema?;
  body: string; streaming: boolean; timeout?
}

interface HandlerVersion {
  id; handlerId; status: VersionStatus; version?;
  imports; initBody; shutdownBody;
  methods: MethodSpec[]; initArgsSchema: InitArgSpec[];
  dependencies; pythonVersion;
  envId; envStatus: EnvStatus; envError; envSyncedAt?; envSyncStage; envSyncDetail;
  changeReason; forgedInConversationId?; createdAt; updatedAt;
}

interface Handler {
  id; userId; name; description; tags; activeVersionId; createdAt; updatedAt;
  // 服务端计算字段
  pending?: HandlerVersion; envStatus?; envError?; envSyncedAt?; envSyncStage?; envSyncDetail?;
  configState?: ConfigState; liveInstances?: number;
}

interface HandlerConfig { configState: ConfigState; config: Record<string,unknown> | null }
interface CallHandlerVars { id: string; method: string; args: Record<string,unknown> }
interface CallHandlerResult { result: unknown }
```

Handler 版本比 Function 多 `initArgsSchema`（init 参数模式）和 `methods`（方法列表），以及 `configState`/`liveInstances` 运行时状态。

---

## 3. API hooks（`api/handler.ts`）

| Hook | 方法 + 端点 | 说明 |
|---|---|---|
| `useHandlers()` | GET `/handlers?limit=200` | 列表 |
| `useHandler(id)` | GET `/handlers/{id}` | 详情 |
| `useHandlerVersions(id)` | GET `/handlers/{id}/versions` | 版本历史 |
| `useHandlerConfig(id)` | GET `/handlers/{id}/config` | init args 配置 |
| `useAcceptHandler()` | POST `/handlers/{id}/pending:accept` | 接受 pending |
| `useRejectHandler()` | POST `/handlers/{id}/pending:reject` | 拒绝 pending |
| `useCallHandler()` | POST `/handlers/{id}:call` body `{method, args}` | 调用方法 |
| `useDeleteHandler()` | DELETE `/handlers/{id}` | invalidate handlers |

Handler 无 revert 和 run（区别于 Function）。call 走 `:call` action 后缀。

---

## 4. 端到端数据流

```
用户调用 handler 方法
  → features/call-handler → useCallHandler().mutate({id, method, args})
      → POST /handlers/{id}:call  {method, args}
      → 返回 {result} → 展示调用结果

用户查看 handler 配置状态
  → useHandlerConfig(id)
      → GET /handlers/{id}/config
      → 返回 {configState, config: {...}}
      → ConfigState 驱动 UI 显示 "需要配置" / "部分配置" / "就绪"
```

---

## 5. 实现清单

| 文件 | 说明 |
|---|---|
| `frontend/src/entities/handler/model/types.ts` | Handler / HandlerVersion / Method* / Config* 类型 |
| `frontend/src/entities/handler/api/handler.ts` | 8 个 hooks |
| `frontend/src/entities/handler/index.ts` | public API |

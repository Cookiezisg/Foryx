# Handler

> Trinity-domain stateful Python class surface, Plan 02 deliverable of the forge_redesign topic.

**Code 位置**:`backend/internal/{domain,app,infra/store,infra/handler,transport/httpapi/handlers}/handler/`

**联动文档**:
- 完整设计 spec: [`adhoc-topic-documents/forge_redesign/03-handler.md`](../adhoc-topic-documents/forge_redesign/03-handler.md)
- 跨域决策(D1-D22): [`adhoc-topic-documents/forge_redesign/00-overview.md`](../adhoc-topic-documents/forge_redesign/00-overview.md)
- D22 调用日志: [`adhoc-topic-documents/forge_redesign/08-executions.md`](../adhoc-topic-documents/forge_redesign/08-executions.md)
- 实施计划: [`adhoc-topic-documents/forge_redesign/plans/02-handler-domain.md`](../adhoc-topic-documents/forge_redesign/plans/02-handler-domain.md)

---

## 1. 定位

Handler 是 trinity 第二条腿 — **有状态 Python class**。一个 Handler = `class HandlerImpl: __init__ + N methods + shutdown` 的封装。跟 Function 区别:
- Function 无状态,每次调用是 fresh subprocess
- Handler 有状态,一个 instance 持续存活,跨多次 method call 共享 `self.*` 状态(如 DB connection、缓存)

跟 MCP 区别(D2):
- MCP 是 user-installed external 工具(npm/pip 装的),协议是 JSON-RPC 2.0
- Handler 是 user-authored 代码,在自己 venv 里跑,协议是自定义 JSON line

---

## 2. 二层模型:Definition + Instance

**Definition**(持久化):`handlers` + `handler_versions` 表。Class code parts(imports/init/methods/shutdown)+ init_args schema + deps + python version。

**Instance**(运行时,不持久化):一个长跑 Python subprocess + 我们的 `handlerinfra.Client`(stdio 行 JSON RPC)。Instance 在 `instanceRegistry` 内存表中按 `(owner, handlerName)` 索引。

---

## 3. Caller-owns Lifetime(D3 + 2026-05-12 用户细化)

| Caller | Lifetime |
|---|---|
| chat 单次 tool_call | **单调用**(spawn → method → destroy 一气呵成)|
| workflow run | run 开始 spawn → 跨节点共用 → run 结束销 |
| test | test 开始 spawn → test 结束销 |
| session(HTTP power-user)| 显式 acquire / release |

**没有 idle GC** — chat 不进 registry,workflow/test/session 用 owner-end 钩子(workflow.run.End / test.End / session.Release)显式调 `Service.registry.DestroyOwner`。Process shutdown 时 `Service.Shutdown(ctx)` 把整个 registry 排空。

---

## 4. Ops 集合(method-level,跟 workflow 节点级 ops 一致)

10 个 op:`set_meta` / `set_imports` / `set_init` / `set_shutdown` / `set_init_args_schema` / `add_method` / `update_method` / `delete_method` / `set_dependencies` / `set_python_version`。

`update_method` 用 **JSON Merge Patch(RFC 7396)** — patch 值覆盖,nil 删除键。其他都是整字段覆盖。

`edit_handler` 接受 `ops=[]` 显式语义 = **强制重建当前 active version 的 env**(D-redo-22)— 不改字段,直接销 env + 重装走 fix loop;给"env 突然坏了想 fix"路径用。

错误映射:per-op apply 错误 → `ErrOpInvalid`(400 HANDLER_OP_INVALID);final 校验错误 → `ErrASTParseError`(422 HANDLER_AST_PARSE_FAILED)。

---

## 5. Python class 契约 — 系统按 ops 拼装

### 5.1 系统拼装模板(`app/handler/rpc.go::AssembleClass`)

```python
# Auto-assembled by Forgify from ops; do not edit by hand.
<Imports>

class HandlerImpl:
    def __init__(self, **init_args):
        <InitBody>            # default: pass

    def shutdown(self):
        <ShutdownBody>        # default: pass

    def <m1.Name>(self, **args):
        <m1.Body>             # streaming = method body uses yield → progress

    # ... per method
```

### 5.2 Driver 模板(`app/handler/rpc.go::DriverScript`)

恒定 ~70 行 Python,每个 handler instance 都跑同一份 driver:`import HandlerImpl` → 读 stdin 一行 init JSON → 实例化 → 进消息循环:
- `{type:"call",id:N,method:"X",args}` → 调 `getattr(handler, X)(**args)` → 若是 generator 每 yield emit `{type:"progress",id:N,data}` 再 return,否则直接 `{type:"return",id:N,data}`
- `{type:"shutdown"}` → 调 `handler.shutdown()` 然后 break
- 异常 → `{type:"error",id:N,error,trace}`
- 私有 method(以 `_` 开头)拒绝调用

---

## 6. 调用流程

```
LLM 调 call_handler({handlerName, method, args})
   ↓
Service.Call(ctx, in):
   1. 按 name/id 找 handler
   2. owner.Kind == "chat" 或 "" → callPerCallTracked(spawn+method+destroy)
   3. 否则 → callViaRegistryTracked(registry.Acquire spawn 或 reuse)
   4. recordCall(detached ctx) 写 D22 handler_calls 行
   ↓
spawnInstance(ctx, h, owner):
   1. GetVersion(active) + LoadConfig(decrypt)
   2. 校验 required init_args 都填了 → ErrConfigIncomplete
   3. syncEnv 若 env_status != ready(in-flight,UI 在等)
   4. AssembleClass + WriteCodeFile(user_handler.py + driver.py)
   5. SpawnLongLived → handlerinfra.New(stdin, stdout) → Init(config)
   6. captureStderr goroutine → 256KB ring + zap log per line
   返 Instance{ID, Client, Kill}
```

---

## 7. Handler Config — init args 加密(D-handler)

整个 init_args JSON blob 经 `cryptodomain.Encryptor`(AES-GCM via infra/crypto)加密,存在 `handlers.config_encrypted` 列。Repo 对 ciphertext 不透明 — 加解密在 Service 层。

**Sensitive 字段**(InitArgSpec.Sensitive=true):
- 写时:UI 密码框,LLM 工具 args description 标 "sensitive,不要 log"
- 存时:整 blob 一起 AES-GCM(不区分 sensitive,简化)
- 读时:`MaskedConfig` 替为 `"********"` 给 GET / list / LLM 工具结果用
- Spawn 时:解密后 inject 给 Python `__init__(**init_args)`,Python 进程内可见明文

**ConfigState**(计算字段,`gorm:"-"`):
- `unconfigured` — 从未填(或全部必填项缺)
- `partially_configured` — 填了部分但还缺必填项
- `ready` — 必填项齐全

`Service.attachComputed` 在 Get/List 时算并填到 `Handler.ConfigState`。

---

## 8. 持久化 — 3 张本域表

### 8.1 `handlers`(主键 `hd_<16hex>`)
软删 / 用户作用域 / partial UNIQUE `(user_id, name) WHERE deleted_at IS NULL`。
字段:`name` / `description` / `tags` JSON / `active_version_id` / `config_encrypted`(AES-GCM 密文,json:"-" 永不返)/ 标准时间戳。
**计算字段**:`Pending` / `EnvStatus / EnvError / EnvSyncedAt / EnvSyncStage / EnvSyncDetail` / `ConfigState` / `LiveInstances`。

### 8.2 `handler_versions`(主键 `hdv_<16hex>`)
`status` CHECK in (pending/accepted/rejected),pending/rejected 时 `version` NULL。
字段:`handler_id` 索引 / `imports` / `init_body` / `shutdown_body` / `methods` JSON / `init_args_schema` JSON / `dependencies` JSON / `python_version` / `env_id` 索引 / 5 个 env 字段 / `change_reason` / 时间戳。AcceptedVersionCap = 50。

### 8.3 `handler_calls`(主键 `hcl_<16hex>`,D22)
软删 / 用户作用域。
**通用 16 字段**(spec/08-executions.md §2):`status`(ok/failed/cancelled/timeout)/ `triggered_by`(chat/workflow/http/test)/ `input` JSON / `output` JSON / `error_code` / `error_message` / `elapsed_ms` / `started_at`(索引 DESC)/ `ended_at` / `conversation_id`(索引)/ `message_id` / `tool_call_id` / `flowrun_id`(索引)/ `flowrun_node_id`。
**Handler 专属 6 字段**:`handler_id`(索引)/ `version_id` / `method`(索引)/ `instance_id` / `owner_kind` / `owner_id`。

---

## 9. HTTP API(17 端点)

| Method | Path | 用途 |
|---|---|---|
| POST   | `/api/v1/handlers`                              | 创建(扁平 definition);**前置 sandbox ping**(D-redo-20)|
| GET    | `/api/v1/handlers`                              | 列表 |
| GET    | `/api/v1/handlers/{id}`                         | 详情(含 pending + env + configState + liveInstances)|
| PATCH  | `/api/v1/handlers/{id}`                         | 改 meta |
| DELETE | `/api/v1/handlers/{id}`                         | 软删 + 级联销毁 instance |
| POST   | `/api/v1/handlers/{id}:call`                    | 调用 method(per-call lifetime)|
| POST   | `/api/v1/handlers/{id}:revert`                  | 回滚版本 |
| GET    | `/api/v1/handlers/{id}/versions`                | 版本分页 |
| GET    | `/api/v1/handlers/{id}/versions/{v}`            | 单版本 |
| GET    | `/api/v1/handlers/{id}/pending`                 | 当前 pending |
| POST   | `/api/v1/handlers/{id}/pending:accept`          | accept(瞬时返,env 不在此装,D-redo-10)|
| POST   | `/api/v1/handlers/{id}/pending:reject`          | reject(销 env + 删行,D-redo-12)|
| GET    | `/api/v1/handlers/{id}/config`                  | masked config + configState |
| POST   | `/api/v1/handlers/{id}/config`                  | merge patch update |
| DELETE | `/api/v1/handlers/{id}/config`                  | 清回 unconfigured |
| GET    | `/api/v1/handlers/{id}/calls`                   | 调用日志列表(D22)|
| GET    | `/api/v1/handler-calls/{callId}`                | 全局调用详情 + hints(D22)|

**已删除**:`:resync` 端点(D-redo-14)— LLM 走 `edit_handler({id, ops:[]})` 触发重装(D-redo-22)。

---

## 10. LLM 工具(10 个)

| 工具 | 用途 |
|---|---|
| `search_handler` | LLM 排序 query → 相关 handler |
| `get_handler` | 完整详情 + maskedConfig + configState |
| `create_handler` | 流式 ops 创建;内部 env-fix loop(maxAttempts=3,D-redo-15);env=ready 时 auto-accept v1 |
| `edit_handler` | 流式 ops 编辑;pending 已存在则 iterate same pending(D-redo-11);`ops=[]` = 强制重建 env(D-redo-22)|
| `revert_handler` | 回滚 active 版本 |
| `delete_handler` | 软删 + instance 级联 |
| `call_handler` | per-call lifetime 调 method,流式 progress |
| `update_handler_config` | merge patch 写 config;不回显明文 |
| `search_handler_calls` | D22 调用日志查询;previews + aggregates |
| `get_handler_call` | D22 单行详情 + machine hints |

---

## 11. Sandbox 集成

`handler.Sandbox` port(6 方法):PythonPath / Sync / SpawnLongLived / WriteCodeFile / Destroy / DestroyEnv。具体实现 `SandboxAdapter`(`sandbox_adapter.go`)桥接 `sandboxapp.Service`。

Owner.Kind = `handler`,Owner.ID = `<handlerID>_<envID>`(envID = Version 行的 `hdenv_<16hex>`,D-redo-8 每版本独立 venv + EnvID 与 versionID 解耦)。文件布局:`<dataDir>/handlers/<hdID>/versions/<vID>/{user_handler.py,driver.py}`。

**Env 装配同步发生在 Service.Create / Edit 内**(D-redo-9);失败由 LLM tool(create_handler / edit_handler)走内部 env-fix loop(maxAttempts=3,主 chat scenario LLM 改 deps)。Service.Create / Edit 前置 sandbox ping;失败返 `ErrSandboxUnavailable`(503)硬拒(D-redo-20)。**已删** `SyncEnvForVersion` 异步入口(D-redo-14);**已删** `env_synced` / `env_failed` notification action(D-redo-7)。

---

## 12. Catalog 集成

`Service.AsCatalogSource()` 返 `handlerCatalogSource{}`,`Name()="handler"`,`Granularity()=PerItem`,`Category="service"`(对比 function 的 "computation")。`ListItems` 在 Description 嵌前 3 个 public method 名 + ConfigState(D9-1 — LLM 知 handler 能做啥 + 是否可直接调)。

---

## 13. 错误码

详见 [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md) §Phase 3。19 个 sentinel + HANDLER_* wire code(NOT_FOUND / NAME_DUPLICATE / METHOD_NOT_FOUND / VERSION_NOT_FOUND / PENDING_NOT_FOUND / INSTANCE_SPAWN_FAILED / INSTANCE_CRASHED / INSTANCE_RPC_TIMEOUT / INSTANCE_NOT_FOUND / NO_ACTIVE_VERSION / ENV_NOT_READY / ENV_FAILED / SANDBOX_UNAVAILABLE / OP_INVALID / AST_PARSE_FAILED / CONFIG_INCOMPLETE / CONFIG_INVALID / CONFIG_DECRYPT_FAILED / CALL_NOT_FOUND)。

**已删除**:`HANDLER_PENDING_CONFLICT`(409)— Edit 改"iterate same pending"后无冲突场景(D-redo-11)。**新增**:`HANDLER_SANDBOX_UNAVAILABLE`(503)— Service.Create / Edit 前置 sandbox ping 失败(D-redo-20)。

---

## 14. 测试覆盖

- 33 单测(domain sentinel + Version/MethodSpec/InitArgSpec + Repository + apply 10 ops + registry caller-owns lifetime + config crypto round-trip + AssembleClass + DriverScript)
- 13 store 集成测试(in-memory SQLite — Handler CRUD + Version flow + Config encryption + cross-user isolation)
- 8 stdio client 测试(init/call/streamcall/shutdown + crash detection + ctx-cancel)
- 4 pipeline 测试(HTTP CRUD lifecycle / Config round-trip / LLM search empty / Call+CallLog sandbox-gated)

---

## 15. 历史

- 2026-05-12 forge_redesign Plan 02 完成:domain + store + stdio client + Service(ops engine + registry + config crypto + class assembly + crud + Sandbox adapter + Call)+ 10 LLM tools + 16 HTTP endpoints + main/harness 装配 + D22 handler_calls + pipeline test。11 commits 直推 main。
- 关键决策(forge_redesign 04-discussion 2026-05-12):lifetime 完全 caller-driven,**没有 idle 计时器**;chat = per-call,workflow/test/session = persistent via registry。stdio JSON-line client 独立写不复用 MCP(协议不同)。Handler→Handler 调用不强制禁止(结构上自然防住)。

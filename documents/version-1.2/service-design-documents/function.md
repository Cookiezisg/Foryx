# Function

> Trinity-domain Python user-tool surface, the Plan 01 deliverable of the forge_redesign topic. Replaces the legacy `forge` domain.

**Code位置**：`backend/internal/{domain,app,infra/store,transport/httpapi/handlers}/function/`

**联动文档**：
- 完整设计 spec：[`adhoc-topic-documents/forge_redesign/02-function.md`](../adhoc-topic-documents/forge_redesign/02-function.md)
- 跨域决策 D1-D22：[`adhoc-topic-documents/forge_redesign/00-overview.md`](../adhoc-topic-documents/forge_redesign/00-overview.md)
- D22 执行日志：[`adhoc-topic-documents/forge_redesign/08-executions.md`](../adhoc-topic-documents/forge_redesign/08-executions.md)
- 实施计划：[`adhoc-topic-documents/forge_redesign/plans/01-function-domain.md`](../adhoc-topic-documents/forge_redesign/plans/01-function-domain.md)

---

## 1. 定位

Function 是 trinity 域中的**纯函数/无状态计算**部分。一个 Function = "一段 Python def + 输入 schema + 输出 schema + deps + python 版本" 的封装。每次调用都是无状态执行;状态需求归 Handler(Plan 02 引入);跨步骤编排归 Workflow(Plan 03 引入)。

形态:
- HTTP-friendly **direct definition**(POST `/functions` 扁平 JSON,curl / UI / script 用)
- LLM-friendly **ops stream**(`create_function` / `edit_function` 工具,单 op emit 1 progress delta — 流式锻造)
- **Env 同步发生在 tool 内部**(D-redo-9):`create_function` / `edit_function` 单次工具调用就闭环 ops apply + 装 env + LLM env-fix loop(详 §2 + [`adhoc-topic-documents/forge_redesign/02-function.md`](../adhoc-topic-documents/forge_redesign/02-function.md) §2.1)

---

## 2. 锻造模型(D3 + D6 + 2026-05-12 redesign)

### 2.1 创建

**HTTP path** — `POST /api/v1/functions`,body 扁平字段(name/description/code/parameters/dependencies/python_version/...)。`Service.CreateDirect` 把扁平 input 翻译为 canonical ops 序列(set_meta → set_code → set_parameters → set_return_schema → set_dependencies → set_python_version)再委托 `Service.Create`。

**LLM path** — `create_function` 工具收 `{ops, changeReason}`,直接走 `Service.Create`。每个 op 应用后 emit 1 个 progress block delta。

**Env 装配** — Service.Create / Edit **前置 sandbox ping**;失败返 `ErrSandboxUnavailable`(503,D-redo-20)硬拒,不建 entity。否则同步装 env(blocking),失败时由调用方(LLM tool)走 env-fix loop。

v1 auto-accept(对齐 forge 的 TE-15):**仅当 env=ready 时** auto-accept;env=failed 时 active_version_id 指向 failed 版本但不进 accepted 序列,LLM 看 tool_result 决定下一步。

### 2.2 编辑 — iterate same pending(D-redo-11)

`Service.Edit` 在 pending 之上 ApplyOps:
- **无 pending** → 在 active 之上 ApplyOps → 持久化为新 pending 行 → 装 env
- **有 pending** → 在 pending 之上 ApplyOps → **重写同 ID pending 行**(不创建新行)→ 销旧 env → 装新 env
- **`ops=[]` 显式语义**(D-redo-22)— 不改字段,直接重建当前 active version 的 env(取代以前用 fake set_meta 的 hack)

`ErrPendingConflict` 已移除 — Edit 不再因"pending 存在"返冲突。

### 2.3 接受 / 拒绝 / 回滚

- `AcceptPending` — pending → 带号 accepted,**纯指针翻转**(env 已在 edit 阶段装好,D-redo-10);应用 `AcceptedVersionCap=50`(超限硬删最旧)
- `RejectPending` — pending → rejected,**销 env + 删 pending Version 行**(D-redo-12)
- `Revert(targetVersion int)` — ActiveVersionID 翻到指定 accepted 版本号;目标 env 已被 sandbox GC evict 时,RunFunction 时 in-flight 重建(D-redo-13)

---

## 3. Ops 集合(6 个)

| Op | 字段 | 用途 |
|---|---|---|
| `set_meta` | `name?`, `description?`, `tags?` | 元数据 |
| `set_code` | `code` | Python 源码 |
| `set_parameters` | `parameters: []ParameterSpec` | 输入 schema(LLM 自报) |
| `set_return_schema` | `returnSchema: object` | 输出 schema |
| `set_dependencies` | `deps: []string`(PEP 508)| 依赖列表 |
| `set_python_version` | `version`(PEP 440)| Python 版本约束 |

LLM 发 `[{op:"set_meta",name:"...",...}, {op:"set_code",code:"..."}, ...]`,`functionapp.ParseOps` 解码为 `[]Op{Type, Raw}`(Raw 是不透明 body,各 op handler 自取字段)。

**校验分层**:
- **Incremental** — 每 op 应用后跑;name 字符集 / parameter 唯一+类型白名单
- **Final** — 全部 ops 应用完跑;必填(name+code) + AST 扫(至少含 top-level def + D7 handler import 黑名单)

错误映射:incremental 失败 → `ErrOpInvalid`(400 FUNCTION_OP_INVALID);final 失败 → `ErrASTParseError`(422 FUNCTION_AST_PARSE_FAILED)。

---

## 4. Python 代码契约(D14)

LLM 在 `set_parameters` 自报输入参数 schema(name/type/required/default/description/enum)。`ParameterSpec.Type` 白名单:`string / number / integer / boolean / object / array`。

Sandbox driver(`sandbox_adapter.go::driverTemplate`)把用户函数包以 stdin → kwargs → stdout JSON shim,所以 user 不用 import json,只写纯函数:

```python
def csv_clean(rows, drop_empty=True):
    """User function — returns JSON-serializable object."""
    return [r for r in rows if not drop_empty or any(r.values())]
```

`extractFuncName` 从代码第一个 `def <name>` 行解析名字(用户 entity Name 不必等于 Python 函数名)。

---

## 5. 持久化

### 5.1 `functions`

主键 `fn_<16hex>`;软删;`user_id` 索引;partial UNIQUE `(user_id, name) WHERE deleted_at IS NULL`(schema_extras)。
字段:`name` / `description` / `tags`(JSON 数组,serializer:json)/ `active_version_id` / `created_at` / `updated_at` / `deleted_at`。
**计算字段**(`gorm:"-"`):`Pending *Version` + `EnvStatus/EnvError/EnvSyncedAt/EnvSyncStage/EnvSyncDetail`(从 active version 镜像)。`attachComputed` 在 GET / list 后填充。

### 5.2 `function_versions`

主键 `fnv_<16hex>`;`status` DB CHECK `IN ('pending','accepted','rejected')`,pending/rejected 时 `version` 为 NULL。
字段:`function_id` 索引 / `code` / `parameters` JSON / `return_schema` JSON / `dependencies` JSON / `python_version` / `env_id` 索引 / `env_status` / `env_error` / `env_synced_at` / `env_sync_stage` / `env_sync_detail` / `change_reason` / `created_at` / `updated_at`。

`env_id` 在 Version 创建时**独立生成**(`fnenv_<16hex>`),跟 version_id 1:1 但**解耦**(D-redo-8) — sandbox 是共享基础设施,handler 用 `hdenv_`、mcp / chat tool 等其他消费者用各自前缀,trinity 不应强迫"EnvID == 我的 entity ID"。`ComputeEnvID(deps, python)` 哈希共享逻辑已删,env_status 字段对当前 version 状态零歧义。AcceptedVersionCap=50/function,超限 HardDeleteOldestAccepted。

### 5.3 `function_executions` (D22)

主键 `fne_<16hex>`;软删;通用 16 字段(spec/08-executions.md §2)+ 函数专属 3 字段(`function_id` 索引 / `version_id` / `python_version`)。每次 `Service.RunFunction` 终态写一行(detached ctx §S9 防 caller cancel 丢日志)。

**Schema 通用 16**:`status`(CHECK ok/failed/cancelled/timeout)/ `triggered_by`(CHECK chat/workflow/http/test)/ `input` JSON / `output` JSON / `error_code` / `error_message` / `elapsed_ms` / `started_at`(索引 DESC)/ `ended_at` / `conversation_id`(索引)/ `message_id` / `tool_call_id` / `flowrun_id`(索引)/ `flowrun_node_id`。

`ComputeAggregates` 一次 SELECT 拿 status 分桶 count + avg,再 1000 行 LIMIT pluck 算 p95(in-memory)。`buildHints` 在 GetExecutionDetail 时算 `outputEmpty` + `significantlySlower`(elapsed > 3× function avg)。

---

## 6. HTTP API(12 端点)

| Method | Path | 用途 |
|---|---|---|
| POST | `/api/v1/functions` | 创建(扁平 definition);**前置 sandbox ping**(D-redo-20)|
| GET | `/api/v1/functions` | 列表(cursor 分页)|
| GET | `/api/v1/functions/{id}` | 详情(含 pending + 计算 env 字段)|
| PATCH | `/api/v1/functions/{id}` | 改 meta(name/description/tags)|
| DELETE | `/api/v1/functions/{id}` | 软删 + D20 级联通知 |
| POST | `/api/v1/functions/{id}:run` | 执行(`{args, version?}`);取消走 caller ctx(HTTP 断连),无 per-call timeout |
| POST | `/api/v1/functions/{id}:revert` | 回滚到 accepted 版本号 |
| GET | `/api/v1/functions/{id}/versions` | 版本分页(`?status=`)|
| GET | `/api/v1/functions/{id}/versions/{v}` | 单版本(int → ByNumber, fnv_* → ById)|
| GET | `/api/v1/functions/{id}/pending` | 当前 pending |
| POST | `/api/v1/functions/{id}/pending:accept` | accept pending(瞬时返,env 不在此装,D-redo-10)|
| POST | `/api/v1/functions/{id}/pending:reject` | reject pending(销 env + 删行,D-redo-12)|
| GET | `/api/v1/functions/{id}/executions` | 执行日志列表(D22)|
| GET | `/api/v1/function-executions/{execId}` | 全局执行详情 + hints(D22)|

**已删除**:`:resync` 端点(D-redo-14)— LLM 走 `edit_function({id, ops:[]})` 触发重装(D-redo-22)。

---

## 7. LLM 工具(9 个)

详见 [`02-function.md §7`](../adhoc-topic-documents/forge_redesign/02-function.md) + [`08-executions.md §7`](../adhoc-topic-documents/forge_redesign/08-executions.md)。

| 工具 | 用途 |
|---|---|
| `search_function` | LLM 排序 query → 相关 function id+score |
| `get_function` | 完整详情含 pending + active env |
| `create_function` | 流式 ops 创建(单 op 1 progress delta);内部 env-fix loop(maxAttempts=3);env=ready 时 auto-accept v1 |
| `edit_function` | 流式 ops 编辑;pending 已存在则 iterate same pending(D-redo-11);`ops=[]` = 强制重建 env(D-redo-22)|
| `revert_function` | 回滚 active 版本指针 |
| `delete_function` | 软删 |
| `run_function` | 执行(env=evicted 时 in-flight 重建)|
| `search_function_executions` | D22 执行日志查询;返 previews + aggregates |
| `get_function_execution` | D22 单行详情 + machine hints(outputEmpty / significantlySlower)|

---

## 8. Sandbox 集成

`function.Sandbox` port(`app/function/function.go`)定义 6 方法(PythonPath/Sync/Run/WriteCodeFile/Destroy/DestroyEnv)。具体实现 `SandboxAdapter`(`sandbox_adapter.go`)桥接到 `sandboxapp.Service`(统一 PluginSandbox v2 + mise embed)。

Owner.Kind=`function`,Owner.ID=`<functionID>_<envID>`(envID = Version 行的 `fnenv_<16hex>`,D-redo-8 每版本独立 venv + EnvID 与 versionID 解耦)。文件布局:`<dataDir>/functions/<fnID>/versions/<vID>/main.py`(adapter 拥有,sandbox v2 不管布局)。

env_status 状态机:`pending → syncing → ready / failed`(evicted 由 sandbox GC 设;`fixing` 表示 LLM env-fix loop 进行中)。Service.Create/Edit 调 sandbox 前 ping;ping 失败返 `ErrSandboxUnavailable`。装 env 失败时由调用方(LLM tool create_function/edit_function)走内部 env-fix loop(maxAttempts=3,主 chat scenario LLM 改 deps);成功翻 ready,失败终态写 `failed` + envError + attemptsUsed。

**已删除**:`SyncEnvForVersion` fire-and-forget 异步路径 + `Resync` 后台入口(D-redo-14);`env_synced` / `env_failed` notification action(D-redo-7)— env 终态信息走 LLM tool_result 返,UI 经 GET 拉取。

---

## 9. Catalog 集成

`Service.AsCatalogSource()` 返 `functionCatalogSource{}`,`Name()="function"`,`Granularity()=PerItem`(generator 可自由分组 "5 个 CSV 处理 function")。`ListItems` 把 Description 空白时退化为 tags 拼接保证 catalog 不空白。

---

## 10. 错误码

详见 [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md) §Phase 3。13 个 sentinel + FUNCTION_* wire code:`FUNCTION_NOT_FOUND` / `_NAME_DUPLICATE` / `_VERSION_NOT_FOUND` / `_PENDING_NOT_FOUND` / `_RUN_FAILED` / `_AST_PARSE_FAILED` / `_OP_INVALID` / `_NO_ACTIVE_VERSION` / `_ENV_NOT_READY` / `_ENV_FAILED` / `_DEPENDENCY_RESOLUTION` / `_SANDBOX_UNAVAILABLE` / `_EXECUTION_NOT_FOUND`。

**已删除**:`FUNCTION_PENDING_CONFLICT`(409)— Edit 改"iterate same pending"后无冲突场景(D-redo-11)。`SANDBOX_UNAVAILABLE`(503)语义扩展:不仅 bootstrap 失败,Service.Create / Edit 前置 ping 失败也走此 sentinel(D-redo-20)。

---

## 11. 测试覆盖

- 24 单测(domain sentinel + store CRUD/pagination/cap-trim/version flow/execution log)
- 8 service 单测(ops engine + validators + D7 blacklist)
- 4 pipeline 测试(`test/function/function_test.go`):HTTP CRUD lifecycle / List 分页 / LLM search 空库 / Run+ExecutionLog(sandbox-gated;host python-build 挂时 t.Skip)
- Cross-domain pipeline(test/catalog / test/integration / test/cross)更新用 NewFunction + FUNCTION_* 验证 catalog source 路径

---

## 12. 历史

- 2026-05-11 forge_redesign Plan 01 完成:domain + store + app + tools + HTTP + main/harness 装配 + 删除 forge + D22 执行日志 + pipeline test。13 commits 直推 main。
- 历史 forge domain(2025-Q4 至 2026-05-10)的所有特性都迁移到 function:版本/pending/sandbox 执行/catalog。删除的特性:测试用例(forge_test_cases)、import/export、generate-test-cases LLM 工具(Plan 03 workflow 引入 batch 调用更通用)。

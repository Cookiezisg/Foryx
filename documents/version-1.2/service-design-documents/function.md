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
- Caller-owns env lifetime(D3):创建/edit/accept 后 SyncEnvForVersion 后台起 goroutine 物化 venv,UI 经 GET 看 envStatus 翻转

---

## 2. 锻造模型(D3 + D6)

### 2.1 创建

**HTTP path** — `POST /api/v1/functions`,body 扁平字段(name/description/code/parameters/dependencies/python_version/...)。`Service.CreateDirect` 把扁平 input 翻译为 canonical ops 序列(set_meta → set_code → set_parameters → set_return_schema → set_dependencies → set_python_version)再委托 `Service.Create`。

**LLM path** — `create_function` 工具收 `{ops, changeReason}`,直接走 `Service.Create`。每个 op 应用后 emit 1 个 progress block delta。

两条路径殊途同归:v1 auto-accept(对齐 forge 的 TE-15 — 首次创建用户已通过传入意图同意,免一次 accept tap)。`SyncEnvForVersion` 后台起,Create 立即返。

### 2.2 编辑

`Service.Edit` 走 pending 流程。在活跃 version 上 ApplyOps 产生新 draft,持久化为 pending。已有 pending 时返 `ErrPendingConflict`(LLM/UI 必须先 accept/reject)。

### 2.3 接受 / 拒绝 / 回滚

- `AcceptPending` — pending → 带号 accepted,翻 ActiveVersionID;应用 `AcceptedVersionCap=50`(超限硬删最旧)
- `RejectPending` — pending → rejected,不动 ActiveVersion
- `Revert(targetVersion int)` — ActiveVersionID 翻到指定 accepted 版本号(env 可能 evicted,RunFunction 时 in-flight sync)

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

EnvID = `ComputeEnvID(deps, pythonVersion)` 的 sha256 hash;相同 deps+python 的多版本共享同 EnvID 进而共享 venv。env_status 仍每版本独立(便于 pending 自带 sync 历史)。AcceptedVersionCap=50/function,超限 HardDeleteOldestAccepted。

### 5.3 `function_executions` (D22)

主键 `fne_<16hex>`;软删;通用 16 字段(spec/08-executions.md §2)+ 函数专属 3 字段(`function_id` 索引 / `version_id` / `python_version`)。每次 `Service.RunFunction` 终态写一行(detached ctx §S9 防 caller cancel 丢日志)。

**Schema 通用 16**:`status`(CHECK ok/failed/cancelled/timeout)/ `triggered_by`(CHECK chat/workflow/http/test)/ `input` JSON / `output` JSON / `error_code` / `error_message` / `elapsed_ms` / `started_at`(索引 DESC)/ `ended_at` / `conversation_id`(索引)/ `message_id` / `tool_call_id` / `flowrun_id`(索引)/ `flowrun_node_id`。

`ComputeAggregates` 一次 SELECT 拿 status 分桶 count + avg,再 1000 行 LIMIT pluck 算 p95(in-memory)。`buildHints` 在 GetExecutionDetail 时算 `outputEmpty` + `significantlySlower`(elapsed > 3× function avg)。

---

## 6. HTTP API(13 端点)

| Method | Path | 用途 |
|---|---|---|
| POST | `/api/v1/functions` | 创建(扁平 definition)|
| GET | `/api/v1/functions` | 列表(cursor 分页)|
| GET | `/api/v1/functions/{id}` | 详情(含 pending + 计算 env 字段)|
| PATCH | `/api/v1/functions/{id}` | 改 meta(name/description/tags)|
| DELETE | `/api/v1/functions/{id}` | 软删 + D20 级联通知 |
| POST | `/api/v1/functions/{id}:run` | 执行(`{args, version?, timeoutMs?}`)|
| POST | `/api/v1/functions/{id}:resync` | 强制重 sync env |
| POST | `/api/v1/functions/{id}:revert` | 回滚到 accepted 版本号 |
| GET | `/api/v1/functions/{id}/versions` | 版本分页(`?status=`)|
| GET | `/api/v1/functions/{id}/versions/{v}` | 单版本(int → ByNumber, fnv_* → ById)|
| GET | `/api/v1/functions/{id}/pending` | 当前 pending |
| POST | `/api/v1/functions/{id}/pending:accept` | accept pending |
| POST | `/api/v1/functions/{id}/pending:reject` | reject pending |
| GET | `/api/v1/functions/{id}/executions` | 执行日志列表(D22)|
| GET | `/api/v1/function-executions/{execId}` | 全局执行详情 + hints(D22)|

---

## 7. LLM 工具(9 个)

详见 [`02-function.md §7`](../adhoc-topic-documents/forge_redesign/02-function.md) + [`08-executions.md §7`](../adhoc-topic-documents/forge_redesign/08-executions.md)。

| 工具 | 用途 |
|---|---|
| `search_function` | LLM 排序 query → 相关 function id+score |
| `get_function` | 完整详情含 pending + active env |
| `create_function` | 流式 ops 创建(单 op 1 progress delta;v1 auto-accept)|
| `edit_function` | 流式 ops 编辑 → pending(已有 pending 时 ErrPendingConflict)|
| `revert_function` | 回滚 active 版本指针 |
| `delete_function` | 软删 |
| `run_function` | 执行(env 未 ready 时 in-flight sync,sync 进度经 progress block 流出)|
| `search_function_executions` | D22 执行日志查询;返 previews + aggregates |
| `get_function_execution` | D22 单行详情 + machine hints(outputEmpty / significantlySlower)|

---

## 8. Sandbox 集成

`function.Sandbox` port(`app/function/function.go`)定义 6 方法(PythonPath/Sync/Run/WriteCodeFile/Destroy/DestroyEnv)。具体实现 `SandboxAdapter`(`sandbox_adapter.go`)桥接到 `sandboxapp.Service`(统一 PluginSandbox v2 + mise embed)。

Owner.Kind=`function`,Owner.ID=`<functionID>_<envID>`(per-function envID buffer 便于快速 revert)。文件布局:`<dataDir>/functions/<fnID>/versions/<vID>/main.py`(adapter 拥有,sandbox v2 不管布局)。

env_status 状态机:`pending → syncing → ready / failed`(evicted 由 sandbox GC 设)。`Service.syncEnvSync` 流式回写 `UpdateVersionEnv(stage, detail)`,终态推 `env_synced` / `env_failed` notifications。

---

## 9. Catalog 集成

`Service.AsCatalogSource()` 返 `functionCatalogSource{}`,`Name()="function"`,`Granularity()=PerItem`(generator 可自由分组 "5 个 CSV 处理 function")。`ListItems` 把 Description 空白时退化为 tags 拼接保证 catalog 不空白。

---

## 10. 错误码

详见 [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md) §Phase 3。14 个 sentinel + FUNCTION_* wire code:`FUNCTION_NOT_FOUND` / `_NAME_DUPLICATE` / `_VERSION_NOT_FOUND` / `_PENDING_NOT_FOUND` / `_PENDING_CONFLICT` / `_RUN_FAILED` / `_AST_PARSE_FAILED` / `_OP_INVALID` / `_NO_ACTIVE_VERSION` / `_ENV_NOT_READY` / `_ENV_FAILED` / `_DEPENDENCY_RESOLUTION` / `_SANDBOX_UNAVAILABLE` / `_EXECUTION_NOT_FOUND`。

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

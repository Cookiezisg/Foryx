---
id: DOC-110
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-06
review-due: 2026-09-01
audience: [human, ai]
---
# Function — 无状态 Python 函数与沙箱执行

> **核心地位**：Function 是 Forgify「四项全能」(Quadrinity) 的第一元——**纯粹的、无状态的逻辑**。每次调用在全新隔离的沙箱进程里跑（`python main.py`，stdin 喂参 / stdout 抓 JSON），跑完即弃。对标 Handler 的常驻进程，Function 是「即用即弃」。

---

## 1. 版本模型：线性历史 + 自由指针（无 accept）

旧设计把「版本号分配」「生效」「pending 草稿」拧在一起（用户必须手动 accept 才转正，体验有断点）。新设计把三个概念**彻底正交**：

| 概念 | 语义 | 谁能动 |
|---|---|---|
| **版本号 `version`** | 写入顺序（单调计数器，只增不改） | 写新版本时 = `max+1` |
| **版本内容** | 不可变快照（append-only） | 永不修改既有版本 |
| **active 指针 `active_version_id`** | 「现在用哪个」 | edit 前移 / revert 自由移动 |

- **create** = 初始化三者（写 v1，立即生效）。
- **edit** = 基于 active 当前版本套 ops → 写 `v(max+1)` → 指针前移。立即生效、无断点。
- **revert(N)** = **只挪指针**到 vN，不产生版本、不删「更新的」版本。版本号轴不动；active 号可能小于某些历史号（前端诚实显示「当前 vN，之后还有 …」）。
- 历史保留供 revert / 审计；超 `VersionCap=50` 裁最老的——**但绝不裁 active 版本**（revert 后它可能很老）。

> **删除的旧机制**：`status`(pending/accepted/rejected) 列、accept/reject 状态机、`GetPending`、`pending:accept`/`:reject` 端点、「至多一个 pending」不变式——全部不存在。

```
版本号轴（单调）          active 指针
create  ──  v1                    → v1
edit    ──  v1  v2                → v2
edit    ──  v1  v2  v3            → v3
revert1 ──  v1  v2  v3            → v1   （v2/v3 原样留着）
edit    ──  v1  v2  v3  v4        → v4   （从 v1 fork，号继续 max+1）
```

---

## 2. 物理模型

### 2.1 `functions`（`fn_`，软删）
`id` · `workspace_id`(orm 自动隔离) · `name`(workspace 内 partial-UNIQUE，软删后释放) · `description` · `tags`(json) · **`active_version_id`**(指针) · 时间戳 · `deleted_at`。

### 2.2 `function_versions`（`fnv_`，append-only + cap 裁剪，无软删）
`id` · `workspace_id` · `function_id` · **`version`**(单调号) · `code` · **`inputs`**(json，`[]schema.Field`，`TEXT NOT NULL DEFAULT '[]'`) · **`outputs`**(json，`[]schema.Field`，同上) · `dependencies`(json) · `python_version` · `env_id`(`fnenv_`，sandbox owner 锚) · `env_status`(pending/syncing/ready/failed) · `env_error` · `env_synced_at` · `change_reason` · `forged_in_conversation_id`(relation 边用) · 时间戳。`UNIQUE(function_id, version)`。

> **I/O 统一**：`inputs`/`outputs` 均为 `[]schema.Field`（`internal/pkg/schema` 共享类型：`{name,type,description}`，无 required/default/enum/嵌套）——全锻造实体共用同一字段形状，精确塑形交运行时 CEL。旧 `parameters []ParameterSpec` + `return_schema map` 及 `ParameterSpec` 类型已删。

### 2.3 `function_executions`（`fne_`，append-only log，**无软删/无硬删** D1）
`id` · `workspace_id` · `function_id` · `version_id` · `status`(ok/failed/cancelled/timeout，CHECK) · **`triggered_by`**(chat/agent/workflow/manual，CHECK) · `input`(json) · `output`(json) · `error_message` · `elapsed_ms` · `started_at` · `ended_at` · conversation/message/tool_call/flowrun 关联 · `created_at`。

**`triggered_by` = 执行体**（「谁在跑」，非「请求怎么来的」）：`chat`(对话里 LLM 调) / `agent`(智能体运行) / `workflow`(工作流节点) / `manual`(编辑器手动测试)。写入方在各自波次接线（agent→M3.4 / workflow→M4 / chat→M5；tool 按 ctx 有无 subagent 自动区分 chat/agent）。

---

## 3. 执行（Run）

`RunFunction`：取 active（或指定）版本 → 确保 env 就绪（未就绪则按需 provision；env 被外部回收 `ErrEnvNotFound` 则按版本快照重建、重试一次）→ `SandboxRunner.Run`（写 `main.py`+driver，spawn `python main.py`，抓 stdout JSON）→ 写一行 `Execution` 审计（best-effort，detached ctx 保留 workspace，使被取消的运行仍落库）。

**env 是按版本物化的 venv**；`SandboxRunner` 只管执行 + 清理（写 main.py / spawn / destroy），env 物化全交 `app/envfix`（见 [DOC-304](envfix.md)）。每版本一个 venv，未用的由 sandbox GC 按时回收，revert 到旧版下次 run 时懒重建。

---

## 4. 锻造（Forge）：ops 累积草稿

create/edit 接收 **ops 数组**累积 `VersionDraft`，逐 op 增量校验 + 终校验后落版本。6 种 op：

```
set_meta            {"op":"set_meta","name":"snake_case","description":"one line","tags":[...]}
set_code            {"op":"set_code","code":"def main(x): ..."}
set_inputs          {"op":"set_inputs","inputs":[{"name","type","description"}]}
set_outputs         {"op":"set_outputs","outputs":[{"name","type","description"}]}
set_dependencies    {"op":"set_dependencies","dependencies":["requests==2.31"]}
set_python_version  {"op":"set_python_version","version":"3.12"}
```

**校验诚实轻量**（非真 AST）：name 形态、字段名唯一 + 类型合法（`schema.ValidateFields`，type ∈ string/number/boolean/object/array）、code 必含顶层 `def`、**D7 禁 import `forgify_handler`**（function 无状态、handler 常驻，不可越界）。`inputs`/`outputs` 均 `[]schema.Field`（无 required/default/enum）。

> 旧文档吹的「AST 预校验 + docstring 提参」从未实现——已删。`kind`(normal/polling)/`polling_interval`/`set_kind`/`set_polling_interval` 已剥离：**polling 触发源是独立概念**，不寄生 function。

---

## 5. env-fix 自愈

依赖装失败时，平台用 **utility 小模型**看 stderr 改 deps 重试（≤3 次），修复后的 deps 回写版本行（号不变）。能力下沉共享包 `app/envfix`（function/handler/未来轮询源共用），见 [DOC-304](envfix.md)。create/edit 工具把每次尝试折进结果（`envFixAttempts`），LLM 看到完整自愈过程。

---

## 6. LLM 工具（9，懒加载）

`search_function`（子串找）· `get_function`（含 active 版代码）· `create_function` · `edit_function`（空 ops = 重建 env）· `revert_function`（按号移指针）· `delete_function` · `run_function`（按 ctx 区分 chat/agent 触发）· `search_function_executions`（分页 + ok/failed 汇总）· `get_function_execution`。

全 S18 五方法接口、danger 由 LLM 逐次自报（工具代码零 danger 逻辑）；进 `Toolset.Lazy`，经 `search_tools` 浮现。

---

## 7. HTTP 端点

`POST /functions`（扁平创建）· `GET /functions`（分页）· `GET|PATCH|DELETE /functions/{id}` · `POST /functions/{id}:run|:revert|:edit` · `GET /functions/{id}/versions`(分页) · `GET /functions/{id}/versions/{version}`(整数号或 version id) · `GET /functions/{id}/executions`(分页+filter) · `GET /function-executions/{execId}`。

> **删**：`/{id}/pending`、`pending:accept`、`pending:reject`（无 accept 状态机）。`:iterate`(AI 编辑) 随 askai 波次 6。

---

## 8. 跨域集成

- **sandbox**：env 物化经 `envfix.Provisioner`（`EnsureEnv`）；执行 + 清理经 `SandboxRunner`（`Spawn`/`Destroy`）。
- **relation**：读时 `Namer.NamesByIDs` hydrate 名字；`create` 边（origin 对话→v1）+ `edit` 边（active 版的对话，≠origin 时）——**4 动词** `KindCreate`/`KindEdit`，每次 active 变更（edit/revert）重算 edit 边。
- **catalog**：`CatalogSource{Name, ListItems}`（名字+描述，无 Granularity/InvokeTool）。
- **mention**：`Resolver` 快照 description + active 版代码。
- **notification**：`function.created/edited/reverted/updated/deleted/env_rebuilt` 经 `Emitter`。

---

## 9. 错误字典

| Sentinel | Wire Code | HTTP |
|---|---|---|
| `ErrNotFound` | `FUNCTION_NOT_FOUND` | 404 |
| `ErrDuplicateName` | `FUNCTION_NAME_DUPLICATE` | 409 |
| `ErrVersionNotFound` | `FUNCTION_VERSION_NOT_FOUND` | 404 |
| `ErrExecutionNotFound` | `FUNCTION_EXECUTION_NOT_FOUND` | 404 |
| `ErrNoActiveVersion` | `FUNCTION_NO_ACTIVE_VERSION` | 422 |
| `ErrEnvNotReady` | `FUNCTION_ENV_NOT_READY` | 422 |
| `ErrOpInvalid` | `FUNCTION_OP_INVALID` | 422 |
| `ErrInvalidCode` | `FUNCTION_INVALID_CODE` | 422 |
| `ErrSandboxUnavailable` | `FUNCTION_SANDBOX_UNAVAILABLE` | 503 |

> 工具失败软返 tool-result 串（不冒泡 HTTP）；上表是 HTTP 端点冒泡的 domain 错误。

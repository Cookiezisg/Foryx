---
id: DOC-111
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-07
review-due: 2026-09-01
audience: [human, ai]
---
# Handler — 有状态 Python 类与常驻进程（MCP 式生命周期）

> **核心地位**：Handler 是 Forgify「四项全能」(Quadrinity) 的第二元——**常驻的、有状态的服务**。是 function 的孪生：同样的锻造 / 版本 / env / 审计，多出三件——**类组装**（`__init__`/methods/`shutdown`）、**加密 config**（init-args）、**常驻进程生命周期**。状态（`self.xxx`）跨调用留存，适合数据库连接、API 会话、缓存。

---

## 1. 生命周期：MCP 式单例常驻（boot / restart / shutdown）

**一个 handler = 一个常驻单例进程**，像一台 MCP server：

| 动词 | 触发 | 行为 |
|---|---|---|
| **boot** | 开局（active 版 env-ready + config 完整的）/ 创建后配齐 / 首次调用兜底 | spawn 一个进程，跑 `__init__(config)` |
| **restart** | edit（新代码）/ 改 config / crash / 手动 `restart_handler` `:restart` | 优雅关旧（跑 `shutdown()`）→ spawn 新（吃最新 config + 代码） |
| **shutdown** | 删除 / 退出软件 | 优雅关闭 |

**所有调用方（chat / agent / workflow）共享这单一实例 + 单一状态**（真有状态）。实例键 = `handlerID`（单例），管理器是扁平 `map[handlerID]*Instance`。

> **删除的旧机制**：旧实现有 `Owner` + `map[Owner]map[name]` 每对话一份实例、两套 call 分叉（chat 即起即杀——使 chat 里 handler 退化成无状态、自我矛盾；workflow 走 per-owner registry）、lazy-spawn / crash 重生 / owner-end 销毁。全塌成 3 个动词。文档曾吹的 **15 分钟 idle 自动 SHUTDOWN / ast 自动扫方法 / Unix Domain Socket / :reconnect** 都**未实现**——已删。

**restart 为何是有状态服务的必需**：crash 重生救不了「进程活着但状态坏了」（DB 连接被对端掐断、session 过期、缓存陈旧）——restart 是常驻进程的「重置按钮」。无状态的 function 永不需要它。

**并发**：单例 + 客户端 mutex 串行化调用（单 stdio 管道），共享实例对并发调用方逐个处理。

---

## 2. 版本模型（方案 A，同 function）

线性 append-only + 自由 `active_version_id` 指针，**无 pending/accept**：create/edit 立即生效写 `v(max+1)`，revert 纯移指针（不删「更新的」版本），edit fork-from-active，50 上限裁最老**但绝不裁 active**。edit / revert 改变 active 后**重启常驻实例**加载新代码。

---

## 3. 物理模型

### `handlers`（`hd_`，软删）
`id` · `workspace_id` · `name` · `description` · `tags` · `active_version_id`(指针) · **`config_encrypted`**(init-args 值，加密存盘) · 时间戳 · `deleted_at`。

### `handler_versions`（`hdv_`，append-only + cap 裁剪）
类快照：`imports` · `init_body` · `shutdown_body` · **`methods []MethodSpec`**(多方法，各带 **inputs/outputs**(均 `[]schema.Field`)/body/streaming) · `init_args_schema []InitArgSpec`(Sensitive→加密) · `dependencies` · `python_version` · `version`(单调号) · env_* · `change_reason` · `forged_in_conversation_id`。`UNIQUE(handler_id, version)`。

> **I/O 统一**：`MethodSpec.Inputs`/`.Outputs` 改用共享 `[]schema.Field`（`internal/pkg/schema`：`{name,type,description}`，无 required/default），取代旧 `args []ArgSpec` + `returnSchema map`（`ArgSpec` 类型已删）。**无列改名**——methods 整体是 JSON blob，仅 blob 内字段名变（`args`→`inputs`、`returnSchema`→`outputs`）。`InitArgSpec`（`__init__` 配置，带 Sensitive/Required/Default）**不变**——它是实例化凭证、非 method I/O，保留自有形状。

### `handler_calls`（`hcl_`，append-only log，无软删 D1）
每次方法调用审计：`method` · `status`(ok/failed/cancelled/timeout) · **`triggered_by`**(chat/agent/workflow/manual) · input/output/error/计时 · `instance_id`(哪个实例服务的) · conversation/flowrun 关联。

实例 `hdi_` + 进程 env `hdenv_`(handler 自 mint 的 sandbox owner id)——均不入业务表（实例在内存，env 在 sandbox 侧）。

---

## 4. 加密 config（init-args）

`__init__` 的一次性参数（密钥 / 端点）经**加密** config 存盘（Sensitive 字段读时掩码 `********`）。`ConfigState`：unconfigured / partially_configured / ready。**config 门控 spawn**——必填 init-args 未齐则不起实例。`update_config`（JSON Merge Patch + 重加密 + **重启实例吃新值**）/ `clear_config`（清 + 停实例）。**代码与凭证分离**：同一套 Version 类，不同 config 实例化。

---

## 5. 锻造（Forge）：ops 累积类草稿 → AssembleClass

ops：`set_meta` / `set_imports` / `set_init` / `set_shutdown` / `set_init_args_schema`(InitArgSpec) / `add_method` / `update_method`(RFC 7396 merge patch) / `delete_method` / `set_dependencies` / `set_python_version`。`add_method` 的 `method` 携 `inputs`(必，`[]schema.Field`) + `outputs`(可选，`[]schema.Field`) + `body` + `streaming`（method 参数不再带 required/default）：`{"op":"add_method","method":{"name":"fetch","inputs":[{"name":"url","type":"string"}],"outputs":[{"name":"body","type":"object"}],"body":"...","streaming":false}}`。`AssembleClass` 拼 `class HandlerImpl`(`__init__(self, ...initArgs)` + `shutdown(self)` + 每 method 一 def)。Python `DriverScript`：stdio 行-JSON RPC（init→ready、call→return/error、generator `yield {"progress":...}`→progress、`_` 私有方法不可调）。env 装失败用 utility LLM 改 deps 重试（≤3，复用 [`app/envfix`](envfix.md)）。

---

## 6. LLM 工具（11，懒加载）

`search_handler` · `get_handler`(含 config/runtime 状态) · `create_handler` · `edit_handler`(空 ops=重建 env+重启) · `revert_handler` · `delete_handler` · **`call_handler`**(method+args) · `update_handler_config` · **`restart_handler`**(「这个坏了帮我重启」) · `search_handler_calls` · `get_handler_call`。全 5 方法、danger LLM 自报。

---

## 7. HTTP 端点

`POST /handlers`(扁平) · `GET /handlers`(分页) · `GET|PATCH|DELETE /handlers/{id}` · `POST /handlers/{id}:call|:restart|:revert|:edit` · `GET /handlers/{id}/versions` · `GET /handlers/{id}/versions/{version}` · `GET|PUT|DELETE /handlers/{id}/config` · `GET /handlers/{id}/calls` · `GET /handler-calls/{callId}`。

> **删**：`/{id}/pending`、`pending:accept`、`pending:reject`（无 accept）。新增 `:restart`。`:iterate` 随 askai 波次 6。

---

## 8. 跨域集成

- **sandbox**：env 物化经 `envfix.Provisioner`；常驻进程经 `SandboxRunner.Spawn`（写 user_handler.py + driver.py + `SpawnLongLived`）。
- **apikey**：config 常引用密钥（用户填，加密存）。
- **relation / catalog / mention**：同 function（4 动词 create/edit 边、`CatalogSource`、`Resolver` 快照类接口）。
- **workflow**：`tool` 节点 `kind=handler` 调方法（triggered_by=workflow）。
- **notification**：`handler.created/edited/reverted/restarted/config_updated/deleted` 等经 `Emitter`。

---

## 9. 错误字典

| Sentinel | Wire Code | HTTP |
|---|---|---|
| `ErrNotFound` | `HANDLER_NOT_FOUND` | 404 |
| `ErrDuplicateName` | `HANDLER_NAME_DUPLICATE` | 409 |
| `ErrVersionNotFound` | `HANDLER_VERSION_NOT_FOUND` | 404 |
| `ErrCallNotFound` | `HANDLER_CALL_NOT_FOUND` | 404 |
| `ErrMethodNotFound` | `HANDLER_METHOD_NOT_FOUND` | 404 |
| `ErrNoActiveVersion` | `HANDLER_NO_ACTIVE_VERSION` | 422 |
| `ErrEnvNotReady` | `HANDLER_ENV_NOT_READY` | 422 |
| `ErrConfigIncomplete` | `HANDLER_CONFIG_INCOMPLETE` | 422 |
| `ErrOpInvalid` | `HANDLER_OP_INVALID` | 422 |
| `ErrInvalidCode` | `HANDLER_INVALID_CODE` | 422 |
| `ErrInstanceSpawnFailed` | `HANDLER_INSTANCE_SPAWN_FAILED` | 502 |
| `ErrInstanceCrashed` | `HANDLER_CRASHED` | 502 |
| `ErrInstanceRPCTimeout` | `HANDLER_RPC_TIMEOUT` | 504 |
| `ErrSandboxUnavailable` | `HANDLER_SANDBOX_UNAVAILABLE` | 503 |
| `ErrConfigDecryptFailed` | `HANDLER_CONFIG_DECRYPT_FAILED` | 500 |

> 工具失败软返 tool-result 串（不冒泡 HTTP）；上表是 HTTP 端点冒泡的 domain 错误。

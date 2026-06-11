---
id: DOC-009
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
audience: [human, ai]
---

# 数据库 —— 表 / ID 前缀登记

> 物理 schema 的单一事实源（表 · 关键列 · 索引/约束 · ID 前缀）。DDL 全文在各 `infra/store/<域>` 的 `Schema`（幂等 `CREATE IF NOT EXISTS`，启动时 `db.Migrate` 单事务应用）。随评审逐域填入；当前已落：function · handler · agent · workflow · flowrun · trigger · control · approval。
> 通则（D 系列）：业务表软删 `deleted_at`；Log 表（executions/calls）**只增不删**（D1）；全表带 `workspace_id`（orm 据 ctx 自动隔离，D2）；name 用 partial-UNIQUE `WHERE deleted_at IS NULL`（软删释放名字）；版本表 `UNIQUE(<entity>_id, version)`。

## 三实体共同形状

每实体三张表：**主表**（身份 + `active_version_id` 指针，软删）· **版本表**（不可变快照，只增，cap 50 裁剪但放过 active）· **执行/调用 Log 表**（终态审计，只增）。Log 表统一溯源列：`conversation_id / message_id / tool_call_id`（chat 路径，ctx 注入）+ `flowrun_id / flowrun_node_id`（workflow 路径，调度器 ctx 注入）；CHECK 约束 `status IN (ok,failed,cancelled,timeout)` + `triggered_by`。

## function

| 表 | 关键列 | 约束/索引 |
|---|---|---|
| `functions` | name · description · tags(json) · active_version_id | partial-UNIQUE(ws,name)；ws+created 游标索引 |
| `function_versions` | version(int) · code · inputs/outputs(json) · dependencies(json) · python_version · **env_id/env_status/env_error/env_synced_at**（env 镜像）· change_reason · forged_in_conversation_id | UNIQUE(function_id,version) |
| `function_executions` | version_id · status · triggered_by(chat/agent/workflow/manual) · input/output(json) · error_message · elapsed_ms · started/ended_at · 溯源 5 列 | CHECK ×2；ws+function / ws+conversation / ws+flowrun 偏索引 |

ID：`fn_` `fnv_` `fne_` · env：`fnenv_`（infra 侧自有前缀）

## handler

| 表 | 关键列 | 约束/索引 |
|---|---|---|
| `handlers` | （同上）+ **config_encrypted**（init-args 值，AES-GCM 加密存盘） | 同上 |
| `handler_versions` | version · **imports / init_body / shutdown_body / methods(json MethodSpec[]) / init_args_schema(json InitArgSpec[])** · dependencies · python_version · env 镜像 4 列 | UNIQUE(handler_id,version) |
| `handler_calls` | method · status · triggered_by(含 agent) · input/output · **instance_id** · 溯源 5 列 | 同款 CHECK + 索引 |

ID：`hd_` `hdv_` `hcl_` · env：`hdenv_` · 实例（内存态，不落库）：`hdi_`

## agent

| 表 | 关键列 | 约束/索引 |
|---|---|---|
| `agents` | （同上，无 config） | 同上 |
| `agent_versions` | version · **prompt · skill(0-1 名) · knowledge(json docIDs) · tools(json ToolRef[]) · inputs/outputs(json) · model_override(json)** · change_reason · forged_in_conversation_id | UNIQUE(agent_id,version) |
| `agent_executions` | model_id（实际跑的模型）· status · triggered_by(chat/workflow/manual，**无 agent**——员工不调员工) · input/output · **transcript(json，完整 block 序列——运行的自包含耐久记录，不入 message_blocks)** · 溯源 5 列 | 同款 |

ID：`ag_` `agv_` `agx_`（agent 无 env——不写代码无 sandbox）

## workflow / control / approval（同构对：主表 + 版本表）

| 表 | 特有列 | 约束 |
|---|---|---|
| `workflows` | active(bool) · **lifecycle_state**(CHECK active/draining/inactive) · **concurrency**(CHECK serial/skip/buffer_one/buffer_all/allow_all，DEFAULT serial) · needs_attention/attention_reason/last_action_by | partial-UNIQUE(ws,name) |
| `workflow_versions` | **graph**(JSON blob：nodes+edges) | UNIQUE(workflow_id,version) |
| `controls` / `control_versions` | versions：inputs(json) · **branches**(json Branch[]：port/when/emit) | 同构 |
| `approvals` / `approval_versions` | versions：inputs · **template**(markdown+{{CEL}}) · allow_reason · timeout("30d"/"2w"/""=永不) · timeout_behavior(reject/approve/fail) | 同构 |

ID：`wf_`/`wfv_` · `ctl_`/`ctlv_` · `apf_`/`apfv_`

## trigger（配置实体 + 两张 Log）

| 表 | 关键列 | 约束/索引 |
|---|---|---|
| `triggers` | kind(CHECK cron/webhook/fsnotify/sensor) · **config**(自由 json map) · outputs(json) | partial-UNIQUE(ws,name) |
| `trigger_activations`（Log） | kind · fired(bool) · return_value/payload(json) · error · detail · firing_count | 按 trigger+created 索引 |
| `trigger_firings`（Log，durable 收件箱） | trigger_id · workflow_id · activation_id · payload(json) · **dedup_key** · status(pending/claimed/started/skipped/superseded/shed) · flowrun_id | **`idx_trf_dedup` UNIQUE(workflow_id,trigger_id,dedup_key)**（D3）+ pending 偏索引 |

ID：`trg_`/`tra_`/`trf_`

## flowrun（两张 Log——引擎的全部状态）

| 表 | 关键列 | 约束/索引 |
|---|---|---|
| `flowruns` | workflow_id · **version_id**(钉死拓扑) · **pinned_refs**(json pin 闭包) · trigger_id/firing_id · status(CHECK running/completed/failed/cancelled) · replay_count · error | running 偏索引（跨 ws boot 恢复）|
| `flowrun_nodes` | flowrun_id · **node_id**(图内名) · **iteration**(循环轮次) · kind · ref · status(CHECK completed/failed/parked) · **result**(json 记忆化) · error | **`idx_frn_once` UNIQUE(flowrun_id,node_id,iteration)**（D3 record-once）+ parked 偏索引（收件箱）|

ID：`fr_`/`frn_`。两张无 deleted_at（D1）；唯一物理删 = `:replay` 清 failed 行（非结果）。

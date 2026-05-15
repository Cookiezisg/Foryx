# FlowRun

> Workflow 执行记录簿域,Plan 05 三条腿之一(flowrun / trigger / scheduler 单向依赖链)。

**Code 位置**:`backend/internal/{domain,infra/store}/flowrun/` + `backend/internal/transport/httpapi/handlers/flowrun.go`

**联动文档**:
- 完整 spec:[`adhoc-topic-documents/forge_redesign/05-execution-plane.md`](../adhoc-topic-documents/forge_redesign/05-execution-plane.md) §5
- D22 execution log schema 模板:[`08-executions.md`](../adhoc-topic-documents/forge_redesign/08-executions.md) §2 + §4.5
- 实施计划:[`plans/05-execution-plane.md`](../adhoc-topic-documents/forge_redesign/plans/05-execution-plane.md)
- Trigger / Scheduler 兄弟域:[`trigger.md`](trigger.md) / [`scheduler.md`](scheduler.md)

---

## 1. 定位

FlowRun 是**一次 workflow execution 的记录**。每个 trigger fire (cron / fsnotify / webhook / manual) → scheduler.StartRun → 一行 FlowRun + 多行 FlowRunNode (per-dispatch)。

- **FlowRun**:整 run (started_at / ended_at / status / output / error / paused_state)
- **FlowRunNode**:per-node dispatch (每节点 dispatch 写一行 D22 终态)

跟 function/handler/mcp/skill execution log 同 schema 模板 (spec 08 §2 通用 16 字段) + 自己 specific 字段。

---

## 2. 实体

### 2.1 FlowRun (`flowruns` 表)

| 字段 | 说明 |
|---|---|
| `id` | `fr_<16hex>` PK |
| `user_id` | 用户作用域 |
| `workflow_id` | 索引;FK → workflows.id |
| `version_id` | 锁起跑时的 Version (防 active 切换影响进行中的 run) |
| `trigger_kind` | CHECK cron/fsnotify/webhook/manual |
| `trigger_input` | JSON;触发输入 |
| `status` | CHECK running/paused/completed/failed/cancelled (5 值,**无 V1 run-level timeout**) |
| `started_at` / `ended_at` / `elapsed_ms` | 时序 |
| `output` | JSON;终态产物 |
| `error_code` / `error_message` | failed 时填 |
| `paused_state` | JSON;approval/wait 节点暂停时持久化 ExecutionContext 快照 |

软删 + 复合索引 `(workflow_id, status, started_at DESC)`。

### 2.2 FlowRunNode (`flowrun_nodes` 表)

通用 16 字段 (spec 08 §2) + flowrun-specific 4 字段 (`flowrun_id` 索引 / `node_id` / `node_type` / `attempts`)。每 dispatch 终态写一行 (cron/manual/webhook 等触发都同样)。

**写入规则**:`scheduler/pause.go::driveLoop` 在每个非 approval 节点 dispatch 完成后调 `recordNode()` 写一行;approval 节点**不**写行(只是 pause gate,resume 后下游节点正常写)。所以一个 3-节点的 trigger → approval → function workflow 完成后 `flowrun_nodes` 表里有 2 行(t1 trigger + f1 function),`approval` 节点不在表里。

**Cross-table linking** (spec 08 §4.5):capability 节点 (function/handler/mcp/skill) dispatch 时**同时写两条** — 一条到 `flowrun_nodes` (workflow 视角) + 一条到对应 entity 表 (function_executions / handler_calls / mcp_calls / skill_executions),经 `flowrun_node_id` 字段交叉引用。

### 2.3 PausedState (JSON,持久化在 `flowruns.paused_state` 列)

```go
type PausedState struct {
    NodeID    string                    // 暂停在哪个节点
    Variables map[string]any            // workflow-level vars
    Outputs   map[string]map[string]any // 已完成节点 outputs (nodeID → port → value)
    Position  []string                  // 当前 ready 节点 IDs
    PausedAt  time.Time                 // 暂停时刻
}
```

Plan 05 §6.1 用此跨进程重启 rehydrate paused run。

---

## 3. Repository (`flowrundomain.Repository`)

11 方法:`Create / Get / List / UpdateStatus / SetPausedState / ClearPausedState / ListPaused / CountRunning / HardDeleteOldest / CreateNode / GetNode / ListNodes`。GORM 实现在 `infra/store/flowrun/`,别名 `flowrunstore`。

---

## 4. 状态机 (Plan 05 §3.3)

```
running ←→ paused (approval/wait 长延时)
   ↓
{ completed | failed | cancelled }
```

- `running` → `paused`:approval/wait 节点写 PausedState + flip status
- `paused` → `running`:ResumeApproval HTTP 唤醒
- `running` → 终态:执行完 / cancelled / 节点 onError=stop 失败

**无 run-level timeout 状态** (V1 §3.3) — 节点 timeout 致 run 终止时 `status=failed + error_code=NODE_TIMEOUT`。

---

## 5. HTTP API (5 端点 + 1 webhook 子树)

| Method | Path | 用途 |
|---|---|---|
| GET    | `/api/v1/flowruns` | 列表 (workflowId/status/triggerKind 过滤) |
| GET    | `/api/v1/flowruns/{id}` | 单 FlowRun |
| GET    | `/api/v1/flowruns/{id}/nodes` | per-node 执行记录 |
| DELETE | `/api/v1/flowruns/{id}` | 取消 (scheduler.Cancel) |
| POST   | `/api/v1/flowruns/{id}/approvals/{nodeId}` | approval 签收 (body: `{decision, reason?}`) |
| POST   | `/api/v1/webhooks/{wfId}/{path}` | webhook trigger 入口 (由 trigger.webhook listener 直接挂 ServeMux) |

`POST /api/v1/workflows/{id}:trigger` 手动触发 + `GET /api/v1/workflows/{id}/triggers` trigger 状态由 **WorkflowHandler** 持 (共享 `:revert` 的 `{idAction}` mux dispatcher,避 mux 冲突),委派 FlowRunHandler.FireManual / TriggerStates 薄 helper。

---

## 6. 保留策略 (§6.7)

`HardDeleteOldest(workflowID, keep)` 按 `created_at DESC + id DESC` 排序保留 `keep` 个,其余物理删。默认 `keep=200`,在 `scheduler.finalizeRun` 后异步调。

---

## 7. 错误码 (6 sentinels)

详 [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md):
- `FLOWRUN_NOT_FOUND` (404)
- `FLOWRUN_NOT_CANCELLABLE` (422) — Cancel/ResumeApproval 时已无 cancel 句柄
- `FLOWRUN_NOT_PAUSED` (422) — ResumeApproval 时 status != paused
- `FLOWRUN_APPROVAL_NODE_NOT_FOUND` (404) — ResumeApproval 时 nodeID 不匹配 PausedState.NodeID
- `FLOWRUN_APPROVAL_DECISION_INVALID` (400) — decision ∉ {approved, rejected}
- `FLOWRUN_NODE_NOT_FOUND` (404) — GetNode 未命中

---

## 8. 测试覆盖

- 11 domain 单测 (status / trigger-kind / node-status 枚举值;retention 常量;sentinel distinct + errors.Is round-trip;PausedState JSON round-trip)
- 12 store 集成测试 (Create+Get / cross-user / List 分页 + filter / UpdateStatus 终态 fields / PausedState round-trip + clear / ListPaused / CountRunning / HardDeleteOldest trim / Node CRUD + ListByFlowrun chronological)
- 7 pipeline 测试 (test/scheduler/scheduler_test.go,见 §scheduler.md)

---

## 9. 历史

- 2026-05-13 Plan 05 完成:Plan 05 三条腿之一,E1 (domain) + E2 (store) 落地。FlowRun + FlowRunNode (16 通用 + 4 flowrun-specific) + PausedState + 6 sentinels + 11 Repository 方法 + 12 store 集成测试。E15 main+harness 装配。

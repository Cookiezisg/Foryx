# Scheduler

> Workflow 执行编排器,Plan 05 三条腿之一。读 active version → 持 FlowRun → DAG dispatch → retry/onError/timeout → pause/resume → cleanup。

> **🔧 限制优化（2026-05-31，limits-optimization）**：节点墙钟超时（`retry.go` `defaultTimeouts` 整表）**已删**——无人值守 workflow 靠 run-level ctx + 「stop run」+ 各 dispatcher 自身 bound；只留显式 `node.Timeout` 覆盖。agent 节点 `maxTurns` 默认/硬顶经 `limits.Current().Workflow` 可配（`0`=默认，**不放飞**无人值守 agent）。详 [`../adhoc-topic-documents/limits-optimization/`](../adhoc-topic-documents/limits-optimization/)。

**Code 位置**:`backend/internal/app/scheduler/`

**联动文档**:
- 完整 spec:[`adhoc-topic-documents/forge_redesign/05-execution-plane.md`](../adhoc-topic-documents/forge_redesign/05-execution-plane.md) §3
- D22 execution log:[`08-executions.md`](../adhoc-topic-documents/forge_redesign/08-executions.md) §4.5
- FlowRun / Trigger 兄弟域:[`flowrun.md`](flowrun.md) / [`trigger.md`](trigger.md)
- 实施计划:[`plans/05-execution-plane.md`](../adhoc-topic-documents/forge_redesign/plans/05-execution-plane.md)

---

## 1. 定位

Scheduler 是 trigger→flowrun 中间编排层。StartRun 是唯一入口:

```
trigger.onFire (cron/fsnotify/webhook) ─┐
HTTP POST /workflows/{id}:trigger      ─┼─→ scheduler.StartRun(workflowID, kind, input) → fr_xxx
LLM trigger_workflow tool              ─┘
```

单向依赖链 trigger → scheduler → flowrun (只写)。

---

## 2. StartRun 7-gate (§3.1)

```
1. RequireUserID(ctx)
2. workflowRead.GetWorkflow → ErrWorkflowNotFound (404)
3. !wf.Enabled              → ErrWorkflowDisabled (422 §6.5)
4. wf.NeedsAttention        → ErrWorkflowNeedsAttention (422)
5. (Concurrency=serial && CountRunning ≥ 1) → ErrConcurrencyLimit (409 §6.3)
6. workflowRead.GetActiveVersion → 透传 workflow.ErrNoActiveVersion
7. flowrun.Create + 注 cancel func + go ExecuteFn (detached ctx + recover →
   finalize failed/INTERNAL_PANIC)
```

返 `runID`,异步推进。

---

## 3. executeRun / driveLoop 主循环 (§3.1)

- `newExecutionContext` 初始化 ExecutionContext:Variables 含 reserved `trigger`(=run.TriggerInput),Outputs/Done/Failed/Attempts/NextPort 全 fresh map
- `buildTopo` 算 in-degree map + downstream edges + nodes-by-id
- `topo.initialReady()` 入度=0 的入口节点 (一般是 trigger 节点)
- 主循环:
  - 检查 `ctx.Done` → 终态 cancelled
  - `dispatchBatch(ready)` 并发 dispatch 一批 — per-node goroutine + `defer recover` panic 兜底
  - 串行 process results:`recordNode` 写 flowrun_nodes 终态 + 按 onError 决定 stop/continue/branch + `topo.advance` 推 next ready
  - 抓 `ErrApprovalRequired` → `pauseRun` 持 PausedState + 翻 status=paused + return (不进 finalize)
- 终态 → `finalizeRun` 写 status + output + ended_at + elapsed_ms + 应用保留策略剪 + publish notification

---

## 4. Dispatcher Router (E6 + E7-E8 13 个)

| Node Type | Dispatcher | 实现亮点 |
|---|---|---|
| `trigger` | TriggerDispatcher | no-op + 透传 run.TriggerInput 到 "out" port |
| `function` | FunctionDispatcher | 调 functionapp.RunFunction (TriggeredBy=workflow);ExecutionResult.OK=false 翻 error |
| `handler` | HandlerDispatcher | 调 handlerapp.Call;Owner{Kind=flowrun, ID=runID} 让 instance 跨节点共享 |
| `mcp` | MCPDispatcher | 调 mcpapp.CallTool;args → json.RawMessage |
| `skill` | SkillDispatcher | 调 skillapp.Activate;fork-mode 子 agent 留 Plan 06 |
| `llm` | LLMDispatcher | LLMCaller interface 解耦;nil caller 测试 OK |
| `http` | HTTPDispatcher | net/http GET/POST + SSRF 守卫(拒 loopback/link-local/private/.local/.internal)+ 10MB body cap + JSON auto-parse |
| `condition` | ConditionDispatcher | 调 workflowapp.Compile/Execute 评估;truthy → NextPort="true" |
| `loop` | LoopDispatcher | V1 minimal: items 数组 → "out" port + count;body subgraph 留 Plan 06 (返 ErrLoopBodyNotSupported) |
| `parallel` | ParallelDispatcher | V1 pass-through (executeRun.dispatchBatch 已并发自然 parallel edges);branches subgraph 留 Plan 06 |
| `approval` | ApprovalDispatcher | 返 ErrApprovalRequired → executeRun 走 pause 路径 |
| `wait` | WaitDispatcher | `duration` ms 或 RFC3339 `until`;time.Timer + ctx.Done cancellable |
| `variable` | VariableDispatcher | set/unset ExecCtx.Variables in-place |

Router map[NodeType]Dispatcher;`Set()` 注册;`Dispatch()` 未注册返 `ErrNoDispatcherForType`。13 dispatcher 在 main.go / harness 装配。

---

## 5. Retry + onError + Timeout (E9 §6.8)

### 5.1 Retry (NodeSpec.Retry)

`withRetry(ctx, node, execCtx, fn)`:
- MaxAttempts ≤ 1 → 单次
- Backoff:`fixed` / `linear (+DelayMs)` / `exponential (×2)`
- ctx-cancel 短路
- **Fatal sentinel 短路**:`ErrApprovalRequired` / `ErrLoopBodyNotSupported` / `ErrParallelBranchNotSupported`(retry 这些只是浪费 budget)

execCtx.Attempts[node.ID] 跟踪 attempt 次数,写 `flowrun_nodes.attempts`。

### 5.2 Per-node Timeout

`nodeTimeoutDuration(node)`:NodeSpec.Timeout (ms) 优先,缺则 per-NodeType 默认:

| NodeType | 默认 |
|---|---|
| function / handler / mcp / http | 30s |
| skill / llm | 60s |
| approval | 7d (§6.9) |
| condition / loop / parallel / wait / variable / trigger | 0 (不 enforce) |

`dispatchWithPolicies`:retry 套 per-attempt `context.WithTimeout`;`ctx.DeadlineExceeded` 翻成 `DispatchOutput.Error`。

### 5.3 onError 策略 (NodeSpec.OnError)

- `stop` (default):run.status=failed + ErrorCode=NODE_FAILED
- `continue`:treat 为 completed,go to advance with NextPort=""
- `branch`:advance with NextPort="error" (downstream `error` port 边走)

---

## 6. Approval / Wait Pause + Resume (E10 §3.5 + §6.1)

### 6.1 Pause path

```
ApprovalDispatcher 返 ErrApprovalRequired (carries prompt context)
     ↓
driveLoop 抓 sentinel + pauseRun
     ↓
repo.SetPausedState(PausedState{NodeID, Variables, Outputs, Position, PausedAt})
+ repo.UpdateStatus → paused (无 ended_at)
+ publish "paused" 通知
+ goroutine return (释 dispatcher)
```

### 6.2 Resume path (`Service.ResumeApproval`)

```
HTTP POST /flowruns/{runID}/approvals/{nodeID} body {decision}
     ↓
ResumeApproval 校验 decision ∈ {approved, rejected} + status=paused + nodeID match
     ↓
load 冻结图 (run.VersionID) + 拷 PausedState + ClearPausedState
     ↓
detached ctx + 注 cancel + 翻 running + publish "resumed"
     ↓
go continueRun(ctx, run, graph, pausedNodeID, decision)
```

continueRun 重建 ExecutionContext:
- Variables / Outputs 从 PausedState 取
- Done 标完已 PausedState.Outputs 里的节点
- Approval 节点视为 done + NextPort=decision
- 重建 topo + 回放每完成节点的 in-degree decrement (跳过 approval 节点) + advance approval 节点拿新 ready
- driveLoop 推剩余 DAG

### 6.3 RehydrateOnBoot (§6.1)

桌面 app 用户合盖 / 进程重启时,approval-paused run 不能丢。`Service.RehydrateOnBoot(ctx, userID)` 在启动后调:
- ctx 经 `reqctxpkg.SetUserID` scope 到指定 user → 扫 `repo.ListPaused`
- 每行预注 no-op cancel func (让 `Service.Cancel` 不返 `ErrNotCancellable`)
- Run 保持 paused — 真 ctx 在 ResumeApproval 时新建

**§20 multi-user**：main.go 启动期遍历所有 user 调 `RehydrateOnBoot(ctx, u.ID)`，让任意 user 的 paused flowrun 都能正确恢复 cancel 句柄。

---

## 6.4 Sub-DAG 执行（§5.1 Loop body 子图）

`runReadyLoop` 是从 `driveLoop` 抽出的 ready-set 主循环，**不调 finalizeRun**，让子图复用：

```go
status, errCode, errMsg, paused := s.runReadyLoop(ctx, run, execCtx, topo, ready)
```

`Service.ExecuteSubDAG(req SubDAGRequest)` 给 LoopDispatcher 每迭代调一次：
- 构造 sub-ExecutionContext（继承 Variables / parent Outputs；隔离 Done / Failed / NextPort；绑 `Loop *workflowapp.LoopContext{Item, Index}`）
- `SubDAGFromBody(map)` 把 `loop.config.body` JSON map decode 为 `*workflowdomain.Graph`
- 调 `runReadyLoop` 跑完
- 收集每节点 outputs 给 LoopDispatcher 聚合

**Approval 节点在 body 中被拒**（V1 不支持迭代中途暂停）— `ExecuteSubDAG` 入口检测 + 返 `ErrSubDAGContainsApproval`。

**记录到 flowrun_nodes**：每迭代的 body 节点 record 时自动带 `parent_loop_node` + `iteration_index` 列（execCtx 携带）。

---

## 6.5 Run-level timeout（§5.7）

`Workflow.TimeoutSec int` >0 时，`StartRun` 改用 `context.WithTimeout(timeoutSec)` 替原 `WithCancel`。`runReadyLoop` 每轮 ready-set 前 `select { case <-ctx.Done(): }`：
- `errors.Is(ctx.Err(), context.DeadlineExceeded)` → status=failed + errorCode=`RUN_TIMEOUT`
- 否则（显式 Cancel）→ status=cancelled

---

## 6.6 Dry-run 模式

`FlowRun.DryRun bool` + `StartRunWithOptions(opts StartRunOptions{DryRun: true})`。`ExecutionContext.DryRun` propagate（含 sub-DAG）。`dispatchWithPolicies` 拦截 9 个 side-effect NodeType（function / handler / mcp / skill / llm / agent / http / approval / wait），返 mock：

```go
{Outputs: {"out": "[DRY RUN: <type>]", "_dryRun": true}}
```

`approval` 自动 `NextPort=approved` 让 DAG 越过审批关。纯逻辑节点（trigger / condition / variable / loop / parallel）正常跑——用户能看见 DAG 实际路径。

HTTP 入口：`POST /api/v1/workflows/{id}:trigger?dryRun=true` 走 `scheduler.StartRunWithOptions` bypass trigger.FireManual。

---

## 7. Cancellation (E5 §3.6 §6.14)

`Service.Cancel(runID)`:cancels map 查 cancel func → 调 → ctx 一路串到 dispatcher → in-flight node 立刻 abort。`releaseCancel` 在 executeRun defer 释。

未知 runID → `ErrNotCancellable`(已终态 / 从未存在)。

cleanup:Handler instance 用 Owner{Kind=flowrun,ID=runID} 注册;run 终态时 main.go / harness 装的 Owner-end 钩子 (Plan 06 wire up) 调 `handlerService.DestroyOwner` 销 instance。V1 简化 — handler instance 死亡跟 run goroutine 退出绑定。

---

## 8. WorkflowReader 接口 (port)

```go
type WorkflowReader interface {
    GetActiveVersion(ctx, workflowID) (*Version, error)
    GetWorkflow(ctx, workflowID) (*Workflow, error)
    ListEnabled(ctx) ([]*Workflow, error)
}
```

Plan 04 workflowapp.Service 满足此接口;scheduler 经 interface 消费,解耦。

---

## 9. 错误码 (4 sentinels)

详 [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md):
- `WORKFLOW_DISABLED` (422) — §6.5
- `WORKFLOW_NEEDS_ATTENTION` (422)
- `FLOWRUN_CONCURRENCY_LIMIT` (409) — §6.3 trigger 容忍 skip
- `WORKFLOW_NOT_FOUND_FOR_TRIGGER` (404) — workflow id 查不到

---

## 10. 测试覆盖 (65 scheduler 单测)

- 10 StartRun gate 测试 (missing-uid / not-found / disabled / needs-attention / concurrency-limit / happy + ExecuteFn 调 / stub finalize / Cancel unknown / Cancel cascades / panic recover)
- 8 executeRun state machine 测试 (empty graph / single node / linear A→B→C / fan-out fan-in / onError=stop → failed / onError=continue 吸收 / 未注册 type → failed / flowrun row 写入)
- 11 capability dispatcher 单测 (trigger 透传 + 5 config-parse 错误路径 + 4 LLM fake caller 路径)
- 15 control dispatcher 单测 (HTTP SSRF + url 必填 + condition truthy/falsy + variable set/unset + wait duration + ctx-cancel + approval sentinel + loop items/body unsupported + parallel pass-through/branches unsupported)
- 13 retry/timeout 测试 (single-attempt / MaxAttempts=3 / success on 2nd / fatal short-circuit / ctx-cancel / 3 backoff strategies / 5 nodeTimeoutDuration cases / dispatchWithPolicies timeout)
- 8 pause/rehydrate 测试 (approval pauses run / invalid decision / not-paused / wrong nodeID / end-to-end approve → finishes / rehydrate registers cancel / rehydrate ignores running-completed)

+ 7 pipeline 测试 (`test/scheduler/scheduler_test.go`,见 §11)

---

## 11. Pipeline 测试 (E2E)

`test/scheduler/scheduler_test.go` 7 场景:
1. HTTP :trigger 创建 fr_xxx (happy path)
2. HTTP :trigger 在 disabled workflow 返 422 WORKFLOW_DISABLED (§6.5)
3. HTTP GET /flowruns/{id} 后取回 run
4. HTTP DELETE /flowruns/{id} 取消 (204 in-flight 或 422 已终态,§6.14)
5. HTTP GET /workflows/{id}/triggers 暴露 trigger states (§6.12)
6. HTTP 第二次 :trigger 在 serial 模式撞并发限制 → 409 (§6.3)
7. Plan 05 BootSmoke (Service 字段非 nil + Cancel unknown 返 sentinel)

§6 其他 hardening item 由单测覆盖 (cron missed/fsnotify fail-soft/webhook secret/node timeout/panic recover/paused rehydrate/retention/cron TZ)。

---

## 12. 历史

- 2026-05-13 Plan 05 完成:9 commits W5-W10 + W14-W15 实施 scheduler 主体。E6 driveLoop 抽 executeRun + continueRun 共享主循环。E10 PausedState 重建 ExecutionContext + 回放 topo in-degree decrement。E15 main+harness 装配 13 dispatcher + RehydrateOnBoot。

---
id: DOC-119
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Scheduler Domain — Durable Interpreter 执行引擎原理

> **核心职责**：Scheduler 是 Forgify 的执行大脑。它负责将静态的 Workflow 图转化为动态的、具备容错能力的执行协程。核心基于 **Durable Interpreter (持久化解释器)** 架构实现。

---

## 1. 核心原理 (Principles)

### 1.1 Agenda-Driven 遍历 (ADR-016)
传统的拓扑排序无法处理复杂的循环（回边）和条件分支。Scheduler 采用了 **Agenda (待办任务栈)** 驱动的结构化遍历算法：
- **执行单元**：`(node, iteration_key)`。
- **循环支持**：通过 `IterationKey` 区分同一节点在不同循环轮次中的执行实例。每经过一条回边，序数递增。
- **动态决策**：解释器在运行时动态计算“下一跳”，而非预先固定顺序。

### 1.2 Copy-Hit 重放算法 (Deterministic Replay)
这是 Scheduler 容错的核心。
- **步骤**：解释器在执行每个 `(node, iter)` 前，先查 Journal。
- **命中 (Hit)**：若 Journal 已有该步骤的成功记录，直接从 Journal 提取结果并推进 Agenda，**跳过物理派发**。
- **瞬移恢复**：系统崩溃后，重启同一 FlowRun。解释器会通过一系列“Copy-Hits”迅速跳过已完成步，精准回到断点位置继续。

### 1.3 Active-Branch Join (A-1 算法)
解决 XOR 分支汇合时的死锁难题。
- **Skip Token**：当 `case` 分支未被选中时，解释器向下游传播 `skipped` 标记。
- **逻辑汇合**：Join 节点只需等待所有的入边全部变为 `active` 或 `skipped`。只要有一条 `active` 入边到达，且其余皆为 `skipped`，即可触发执行。

---

## 2. 物理模型与状态

### 2.1 调度器状态 (`Service`)
```go
type Service struct {
    repo          flowrundomain.Repository
    interpreter   *Interpreter
    runWG         sync.WaitGroup // 跟踪所有在途执行协程
    shutdown      chan struct{}
}
```

### 2.2 执行上下文 (`ExecutionContext`)
解释器持有的运行时内存：
- `Variables`: 当前作用域下的命名变量。
- `Outputs`: 历史上所有节点的输出结果集（用于 CEL 引用 `{{nodes.X.out}}`）。
- `Agenda`: 待激活的节点队列。

---

## 3. 生命周期 (Lifecycle)

### 3.1 启动 (StartRun)
- 验证 Workflow 状态（Enabled? Version Active?）。
- **原子事务**：在一次事务中，`ClaimFiring` (标记信号已领) 并 `CreateFlowRun` (创建实例)。
- **协程派发**：启动独立的 `driveLoop` 协程。

### 3.2 节点派发 (Dispatching)
解释器根据节点类型分发到不同的 **Dispatcher**:
- `tool`: 调用 Function/Handler/MCP。
- `agent`: 运行 ReAct 循环（支持 **Sub-step Replay** 内部子步持久化）。
- `case`: 执行 CEL 表达式，选择 NextPort。

### 3.3 优雅停机 (Graceful Shutdown)
- 调用 `Drain()` 时，Scheduler 停止接收新信号。
- 发送 `CANCEL` 给所有在途 FlowRun。
- 等待协程退出，确保 Journal 终态写回。

---

## 4. 跨域集成 (Interactions)

- **Trigger**：从 `trigger_firings` 表认领原始信号。
- **FlowRun**：读取并维护 `flowrun_events` 日志。
- **Agent/Tool**：作为 Activity 的实际执行方。
- **Notifications**：在每个节点状态变化时发布 **Ephemeral Ticks**。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 备注 |
|---|---|---|---|
| `ErrWorkflowDisabled` | 422 | `WORKFLOW_DISABLED` | 尝试触发下线的工作流。 |
| `ErrConcurrencyLimit`| 409 | `FLOWRUN_CONCURRENCY_LIMIT` | Serial 模式下排队已满。 |
| `ErrSubDAGContainsApproval` | 422 | `SUBDAG_CONTAINS_APPROVAL` | 架构限制：循环体/子图中禁止再等审批。 |
| `ErrNotReplayable` | 422 | `FLOWRUN_NOT_REPLAYABLE` | 状态不符。 |

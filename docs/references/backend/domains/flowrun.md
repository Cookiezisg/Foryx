---
id: DOC-109
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# FlowRun Domain — 执行实例与持久化流水账 (Durable Journal)

> **核心地位**：FlowRun 是 Workflow 的物理执行载体。它不仅记录运行结果，更通过 **Durable Journal** 存储执行轨迹，确保系统崩溃后能实现 100% 确定性的状态恢复与重放。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `FlowRun` 实体
```go
type FlowRun struct {
    ID              string            `gorm:"primaryKey;type:text" json:"id"` // fr_<16hex>
    UserID          string            `gorm:"not null;index:idx_flowruns_user_id;type:text" json:"userId"`
    WorkflowID      string            `gorm:"not null;type:text;index:idx_flowruns_workflow,priority:1" json:"workflowId"`
    VersionID       string            `gorm:"not null;type:text" json:"versionId"`
    
    // 版本闭包快照 (ADR-020): 固化起跑时引用的所有 Function/Handler/Agent 版本
    PinnedCallables map[string]string `gorm:"serializer:json;type:text;default:'{}'" json:"pinnedCallables"`
    
    // 重放周期 (ADR-019): 每次 :replay 时递增，用于在 Journal 中隔离旧的失败记录
    Generation      int               `gorm:"not null;default:0" json:"generation"`
    
    TriggerNodeID   string            `gorm:"type:text;default:''" json:"triggerNodeId"`
    TriggerKind     string            `gorm:"type:text" json:"triggerKind"` // cron|fsnotify|webhook|manual|polling
    TriggerInput    map[string]any    `gorm:"serializer:json;type:text;default:'{}'" json:"triggerInput"`
    
    Status          string            `gorm:"not null;index:idx_flowruns_workflow,priority:2;type:text" json:"status"`
    
    StartedAt       time.Time         `gorm:"not null;index:idx_flowruns_workflow,priority:3" json:"startedAt"`
    EndedAt         *time.Time        `json:"endedAt"`
    ElapsedMs       int64             `gorm:"not null;default:0" json:"elapsedMs"`
    
    Output          any               `gorm:"serializer:json;type:text" json:"output"`
    ErrorCode       string            `gorm:"type:text;default:''" json:"errorCode"`
    ErrorMessage    string            `gorm:"type:text;default:''" json:"errorMessage"`
    
    // 协程挂起快照: 存储当前 Agenda、变量作用域及 Activity 输出
    PausedState     *PausedState      `gorm:"serializer:json;type:text" json:"pausedState"`
    
    DryRun          bool              `gorm:"not null;default:false" json:"dryRun"`
    CreatedAt       time.Time         `json:"createdAt"`
    UpdatedAt       time.Time         `json:"updatedAt"`
    DeletedAt       gorm.DeletedAt    `gorm:"index" json:"-"`
}
```

### 1.2 `FlowRunEvent` (Durable Journal)
这是执行引擎的“唯一真相”，所有的非确定性决策必须记入此表。
```go
type FlowRunEvent struct {
    ID           string `gorm:"primaryKey;type:text" json:"id"` // fre_<16hex>
    FlowrunID    string `gorm:"not null;type:text;uniqueIndex:idx_fre_seq,priority:1" json:"flowrunId"`
    Seq          int64  `gorm:"not null;uniqueIndex:idx_fre_seq,priority:2" json:"seq"`
    
    Type         string `gorm:"not null;type:text" json:"type"` 
    // 枚举: node_started, node_completed, node_failed, branch_taken, 
    // signal_awaited, signal_received, timer_armed, timer_fired, 
    // agent_step_started, agent_step_completed, flowrun_cancelled
    
    NodeID       string `gorm:"type:text;default:''" json:"nodeId"`
    IterationKey int    `gorm:"not null;default:0" json:"iterationKey"` // 循环序数
    Generation   int    `gorm:"not null;default:0" json:"generation"`   // 重放代号
    
    // 幂等去重键 (ADR-018): node|iter|type|gen
    DedupKey     string `gorm:"not null;type:text;default:''" json:"-"`
    
    Result       any    `gorm:"serializer:json;type:text" json:"result,omitempty"`
    CreatedAt    time.Time `json:"createdAt"`
}
```

---

## 2. 核心原理 (Principles)

### 2.1 Record-Once (记录唯一性铁律)
为了解决分布式或崩溃环境下的重执行问题，`flowrun_events` 拥有一个物理 UNIQUE 索引 `idx_fre_record_once`：
- **Activity 事件**（如 `node_completed`, `branch_taken`）：通过 `DedupKey` 强制去重。
- **副作用隔离**：解释器在执行任何可能产生副作用的操作前，先查 Journal。若命中，则直接“拷贝”结果（Copy-hit），不重复派发。

### 2.2 Pinned Callables (闭包版本固化)
- **挑战**：长达数周的工作流运行中，引用的 Function 可能已被用户修改。
- **方案**：在 `StartRun` 瞬间，系统扫描图引用，将所有实体的 `active_version_id` 抓取并存入 `flowruns.pinned_callables`。
- **效果**：执行实例在其生命周期内，即便外界环境翻天覆地，其运行的代码版本永远保持一致。

### 2.3 Generation (重放代号)
- 每次用户调 `:replay`，`Generation` 递增。
- **影子覆盖**：新的执行步会携带更高的 Gen。解释器在“状态重构”时，只选取最高 Gen 的记录，从而优雅地覆盖掉旧的失败记录。

---

## 3. 生命周期 (Lifecycle)

1. **材化 (Materialize)**：`Trigger` 收到信号，写 `trigger_firings` 表。
2. **原子认领 (Atomic Claim)**：`Scheduler` 开启事务，认领 Firing 并创建 `FlowRun` 实例（Status: `running`）。
3. **解释执行 (Interpreting)**：协程启动 `Interpreter.Run()`，按 `Agenda` 驱动遍历。
4. **挂起 (Parking)**：遇到 `Approval` 或 `Timer`，协程退出，记录 `signal_awaited`，状态转为 `awaiting_signal`。
5. **恢复 (Resuming)**：用户调 `:decide` 或 Timer 到期，写入 `signal_received`，重起协程。
6. **终结 (Finalizing)**：图走完或出错，记录 `EndedAt`，计算 `ElapsedMs`。

---

## 4. 跨域集成 (Interactions)

- **Workflow**：读取 `version_id` 对应的冻结图结构。
- **Eventlog**：通过 `Notifications` SSE 实时推流。
- **Sandbox**：执行过程中按需申请隔离的虚拟环境。
- **Relation**：运行结束后，可能产生 `runs_in_conversation` 的动态边。

---

## 5. 错误字典 (Sentinels)

| Sentinel | Wire Code | 物理起因 |
|---|---|---|
| `ErrNotFound` | `FLOWRUN_NOT_FOUND` | fr_ID 错误。 |
| `ErrNotCancellable`| `FLOWRUN_NOT_CANCELLABLE` | 任务已在终态。 |
| `ErrNotReplayable` | `FLOWRUN_NOT_REPLAYABLE` | 只有失败任务能重跑。 |
| `ErrNodeNotFound` | `FLOWRUN_NODE_NOT_FOUND` | 画布尝试拉取不存在的节点明细。 |
| `ErrApprovalDecisionInvalid` | `FLOWRUN_APPROVAL_DECISION_INVALID` | 提交了非 yes/no 的决策。 |

---
id: DOC-125
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Trigger Domain — Durable Firing 信号接入与收件箱

> **核心职责**：Trigger 负责将外部的不确定信号（Cron, Webhook, 文件变动）转化为可靠的、可审计的物理记录——**TriggerFiring**，并将其投入收件箱等待 Scheduler 认领。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `TriggerSchedule` — 监听器配置
```go
type TriggerSchedule struct {
    ID              string         `gorm:"primaryKey;type:text" json:"id"` // ts_<16hex>
    WorkflowID      string         `gorm:"not null;index;type:text" json:"workflowId"`
    TriggerNodeID   string         `gorm:"not null;type:text" json:"triggerNodeId"`
    
    // 类型: cron | fsnotify | webhook | polling
    Kind            string         `gorm:"not null;type:text" json:"kind"`
    
    // 配置负载: 如 cron 表达式 {"cron": "*/5 * * * *"}
    Spec            any            `gorm:"serializer:json;type:text" json:"spec"`
    
    Enabled         bool           `gorm:"not null;default:true" json:"enabled"`
    
    // 连续失败计数 (ADR-022): 超过阈值自动 deactivate
    ConsecutiveFailures int        `gorm:"not null;default:0" json:"consecutiveFailures"`
    
    LastFiredAt     *time.Time     `json:"lastFiredAt"`
}
```

### 1.2 `TriggerFiring` — 信号收件箱 (Durable Inbox)
```go
type TriggerFiring struct {
    ID              string         `gorm:"primaryKey;type:text" json:"id"` // tfi_<16hex>
    WorkflowID      string         `gorm:"not null;index;type:text" json:"workflowId"`
    TriggerNodeID   string         `gorm:"not null;type:text" json:"triggerNodeId"`
    TriggerKind     string         `gorm:"not null;type:text" json:"triggerKind"`
    
    // 材化载荷: Webhook body 或 Cron 时间戳
    Payload         any            `gorm:"serializer:json;type:text" json:"payload"`
    
    // 物理去重键: 防止同一 Cron 刻度重复入库
    DedupKey        string         `gorm:"not null;uniqueIndex:idx_trf_dedup" json:"-"`
    
    // 状态: pending (待认领) | claimed (已在跑) | completed | shed (因并发丢弃) | failed
    Status          string         `gorm:"not null;index;type:text" json:"status"`
    
    FlowrunID       string         `gorm:"type:text" json:"flowrunId"`
    
    EnqueuedAt      time.Time      `gorm:"not null" json:"enqueuedAt"`
    ProcessedAt     *time.Time     `json:"processedAt"`
}
```

---

## 2. 核心原理 (Principles)

### 2.1 Persist-Before-Act (先持久化，后响应)
为了保证信号绝对不丢，Trigger 遵循以下严苛流程：
1. **捕获**：监听器监听到外部事件。
2. **入库**：立即向 `trigger_firings` 插入记录。
3. **响应**：只有在 DB Commit 成功后，才给 Webhook 返回 `202 Accepted` 或 Ack 消息。

### 2.2 Single-Transaction Claim (单事务原子认领 - ADR-021)
Scheduler 并不是通过内存队列消费，而是通过数据库事务：
- **原子动作**：`UPDATE trigger_firings SET status='claimed', flowrun_id=? WHERE id=? AND status='pending'`。
- **防止双花**：利用数据库的隔离性，确保一个 Firing 信号绝对不会触发两个 FlowRun 实例。

### 2.3 Missed-tick Catch-up (重启补漏)
针对 Cron 类型：
- 系统重启后，监视器对比 `LastFiredAt` 与当前墙钟。
- 若错过了原定的刻度，会补发一条 Firing，确保任务不因系统维护而跳过。

---

## 3. 生命周期 (Lifecycle)

1. **注册 (Activation)**：用户调 `:activate`，Trigger 模块启动 goroutine (cron/fsnotify)。
2. **触发 (Firing)**：监听到信号 -> 计算 `DedupKey` -> 执行 `Insert`。
3. **调度 (Scheduling)**：Scheduler 轮询（或由 Trigger 唤醒）执行 **Claim** 动作。
4. **终结 (Outcome)**：FlowRun 结束后，反向回写 Firing 的最终 `status` 为 `completed` 或 `failed`。

---

## 4. 跨域集成 (Interactions)

- **Workflow**：提供图结构定义中的 Trigger 节点配置。
- **Scheduler**：消费信号的唯一方。
- **Function**：`polling` 类型触发器会调用特定的 Forge 函数来获取新数据。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 备注 |
|---|---|---|---|
| `ErrFiringNotPending` | 409 | `TRIGGER_FIRING_NOT_PENDING` | 竞争失败：该信号已被别人抢跑。 |
| `ErrWebhookSecretMismatch` | 401 | `TRIGGER_WEBHOOK_SECRET_MISMATCH`| HMAC 验签失败。 |
| `ErrInvalidCronExpression` | 400 | `TRIGGER_INVALID_CRON` | 表达式语法错。 |
| `ErrPathNotExist` | 422 | `TRIGGER_PATH_NOT_EXIST` | fsnotify 监听路径无效。 |

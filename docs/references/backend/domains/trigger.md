---
id: DOC-015
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
audience: [human, ai]
---

# trigger —— 信号源实体 + durable 收件箱

## 1. 定位

独立的信号源：source 条件满足即 fire（cron 刻度 / webhook / 文件变化 / sensor 探测），把信号**扇出**给所有监听它的 active workflow。trigger 是**配置实体**——无版本模型、无 sandbox/env（Config 是自由 map，加 source 种类不改列）。**故意没有 manual 源**——手动跑是 workflow 自己的能力（`:trigger`），不监听任何东西。

## 2. 心智模型（三层职责切分）

```
infra listener（4 种，只知道"我这个 trigger 做了 X"）
   │ ReportFunc(triggerID, Activity{Fired, Payload, DedupKey…})
   ▼
app onReport（解析 workspace + 监听者；Detached(wsID) ctx）
   │ fanOut：写 1 条 Activation（必写）+ Fired 时每监听 workflow 1 条 Firing
   ▼
durable 收件箱 trigger_firings（pending）……scheduler 每 5s 逐 workspace drain
```

- **Activation**（`tra_`）= "trigger 动了一下"的审计——**触没触发都记**（sensor 每次探测都报，Fired=false 带 ReturnValue/Error/Detail——这让"为什么没触发"可查）；cron/webhook/fsnotify 只在真 fire 时报。
- **Firing**（`trf_`）= **persist-before-act** 的收件箱行：fire 瞬间先落库、早于任何 flowrun。单一 status 枚举即处置结果：pending→claimed（claim 事务内瞬态）→started（终态-ok）；skipped（overlap）/superseded（v2）/shed。
- **引用计数监听**：N 个 active workflow 共享一个 trigger 只跑**一个** listener（0→1 Register 启动、1→0 Unregister 停止；注册表在内存，boot 由 workflow.ReattachActive 重放）。`RefCount/Listening` 是读时算的非列字段。
- **一次性待命**（stage）：`AttachOnce` 标记 once，fanOut 后自动 Detach（可能把 listener 1→0 停掉）。

## 3. 去重（D3：`idx_trf_dedup` = UNIQUE(workflow_id, trigger_id, dedup_key)）

`AppendFiring` 幂等：撞键返已存在行（不丢不重）。**UNIQUE 永久，故 key 必须含时间成分**（裸内容键会永久吞掉之后的合法重复触发）。四源各自的"同一物理事件"标识：

| 源 | DedupKey | 折叠什么 |
|---|---|---|
| cron | trigger + tick 时刻 | 同一刻度的重复材化 |
| webhook | sha256(body)[:8hex] + **分钟桶** | 秒级网络重试；下一分钟起同 payload 照常触发 |
| fsnotify | path + op + **秒桶** | 编辑器一次保存的事件突发 |
| sensor | trigger + probe 时刻（秒） | 一次探测至多一条/工作流 |

## 4. 生命周期 / 行为

- **4 源 config**（`ValidateConfig` 按 kind 分检）：cron=robfig **5 段**表达式（分钟粒度，与分钟桶 dedup 一致；`@every`/秒级不支持，错误消息指路）（`TRIGGER_INVALID_CRON`）；webhook=挂载路径 + 可选 secret（明文比对或 HMAC-SHA256 验签，`TRIGGER_WEBHOOK_SECRET_MISMATCH` 401）；fsnotify=路径 + 事件类型 + 可选 pattern；sensor=周期 invoke fn/hd + CEL 条件（`TRIGGER_INVALID_CEL`/`TRIGGER_INVALID_INTERVAL`/`TRIGGER_SENSOR_TARGET_REQUIRED`）。
- **Edit 热更**：正在监听的 trigger 用新 config 重 Register。
- **`:fire`**（FireManual）：手动催一次——扇给当前监听者（可能 0 个，那就只是一条 0 firing 的 Activation）。
- webhook 异步 fire + recover（handler 不被慢/panic 拖累）、202 立即返回。

## 5. 关键设计决策

listener 永不知道 workflow（扇出是 app 的事）；Activation 与 Firing 分开（观测 vs 待办——名字即语义）；收件箱轮询（5s tick）而非事件驱动——单进程本地的简单正确选择，serial 推迟的 firing 天然在下个 tick 重试；trigger 实体（trg_）与 firing 运行时（trf_）是两回事（对位 approval 的 apf_ vs 运行时 parked 行）。

## 6. 契约（引用）

端点（CRUD + `:fire`/`:iterate` + activations 两查询）→ [api.md](../api.md) · 表（`triggers`/`trigger_activations`/`trigger_firings`——后两张 Log）→ [database.md](../database.md) · 码 `TRIGGER_*` 12+3 → [error-codes.md](../error-codes.md) · ID：`trg_`/`tra_`/`trf_`。（另有 `GET {id}/firings`——收件箱处置面：started/skipped/superseded/shed）

## 7. 跨域集成

被 workflow 经 Binder 端口驱动（Attach/AttachOnce/Detach）；firings 被 scheduler 经 FiringInbox 端口消费（ListPending/ClaimFiring 单事务/MarkOutcome）；sensor listener 经 invoker 端口调 function/handler（bootstrap/sensor.go 适配，TriggeredBy=workflow）；catalog/mention/relation 三适配器同构。

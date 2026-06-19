---
id: DOC-015
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-14
review-due: 2026-09-14
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
- **Firing**（`trf_`）= **persist-before-act** 的收件箱行：fire 瞬间先落库、早于任何 flowrun。单一 status 枚举即处置结果：pending→claimed（claim 事务内瞬态）→started（终态-ok）；skipped（overlap skip）/superseded（overlap buffer_one——丢更早的等待 firing）/shed（资源上限）。
- **引用计数监听**：N 个 active workflow 共享一个 trigger 只跑**一个** listener（0→1 Register 启动、1→0 Unregister 停止；注册表在内存，boot 由 workflow.ReattachActive 重放）。`RefCount/Listening` 是读时算的非列字段；**`LastFiredAt`** 同为读时派生（非列）——List/Get 各行从 activation 日志取最近一条 `fired=true` 的 `created_at`（走 `idx_tra_ws_trigger` 一次 First；单用户触发器少、无 N+1），供行显示「N 前 fire」。**`NextFireAt`**（仅 cron）亦读时派生（非列）——`attachRuntime` 用 `croninfra.NextAfter(expression, now)` 算下次调度触发，供行显示「N 后触发」（非 cron 或 expr 不可解析则 nil）。
- **一次性待命**（stage）：`AttachOnce` 标记 once，fanOut 后自动 Detach（可能把 listener 1→0 停掉）。

## 3. 去重（D3：`idx_trf_dedup` = UNIQUE(workflow_id, trigger_id, dedup_key)）

`AppendFiring` 幂等：撞键返已存在行（不丢不重）。**UNIQUE 永久，故 key 必须含时间成分**（裸内容键会永久吞掉之后的合法重复触发）。四源各自的"同一物理事件"标识：

| 源 | DedupKey | 折叠什么 | Fire Payload（trigger 节点 result、下游按 node id 读） |
|---|---|---|---|
| cron | trigger + tick 时刻 | 同一刻度的重复材化 | `{firedAt}` |
| webhook | sha256(body) 前 8 字节(16 hex) + **分钟桶** | 秒级网络重试；下一分钟起同 payload 照常触发 | `{firedAt, method, path, headers, body(JSON 解析)\|bodyRaw(非 JSON 原串)}`；外部 POST 到 `/api/v1/webhooks/{triggerId}/{config.path}`（config.path 只是子路径） |
| fsnotify | path + op + **秒桶** | 编辑器一次保存的事件突发 | `{firedAt, path, eventKind}`；**eventKind 用配置词汇**（create/modify/delete/rename/chmod 小写、组合事件 `\|` 连，非 fsnotify 原始大写 Op）——`configEventKind` 在交付端归一 |
| sensor | trigger + probe 时刻（秒） | 一次探测至多一条/工作流 | = `config.output` CEL 产出的形状（作者自定义） |

> **sensor = 电平触发（level-triggered，F65）**：dedup key 含 probe 秒戳，故每个轮询周期条件为真都 fire 一条新 firing——**持续坏态会每 poll 反复触发**（非 false→true 边沿一次）。alert-storm 由 listener workflow 的并发策略兜住（默认 `serial` 排队；要单跑设 `skip`/`buffer_one`）。**无内建 edge-trigger/跨 poll 状态**——只想"翻转时触发一次"须在 handler 条件里自存上次状态。create_trigger 工具描述同款记此节奏。

**`outputs` 字段（声明下游可读的 payload 字段）**：cron/webhook/fsnotify 在 create/edit 时由 `triggerdomain.CanonicalOutputs(kind)` **盖上**（= 上表 Fire Payload、**覆盖作者所填、永不与 listener emit 漂移**）；sensor 由作者按 `config.output` 自定义、app 不覆盖。`CanonicalOutputs` 须与 listeners 的 fire payload 同步。

## 4. 生命周期 / 行为

- **4 源 config**（`ValidateConfig` 按 kind 分检）：cron=robfig **5 段**表达式（分钟粒度，与分钟桶 dedup 一致；`@every`/秒级不支持，错误消息指路）（`TRIGGER_INVALID_CRON`）；webhook=挂载路径 + 可选 secret（**明文**：caller 带 `X-Webhook-Secret: <secret>` 头或 `?token=<secret>` 查询；**HMAC**（config `signatureAlgo:"hmac-sha256-hex"`）：caller 带 `X-Hub-Signature-256: sha256=<小写 hex hmac_sha256(rawBody,secret)>` 头、头名可经 config `signatureHeader` 改；不匹配 → 401 纯文本响应，不走标准 envelope 错误码）；fsnotify=路径(必填) + 可选事件类型 + 可选 pattern；sensor=周期 invoke function/handler/mcp（targetKind 三选一；handler/mcp 需 method=方法名/工具名，function 整体即单元）+ CEL 条件（`TRIGGER_INVALID_CEL`/`TRIGGER_INVALID_INTERVAL`/`TRIGGER_SENSOR_TARGET_REQUIRED`）。
- **Edit 热更**：正在监听的 trigger 用新 config 重 Register。
- **`:fire`**（FireManual）：手动催一次——扇给当前监听者（可能 0 个，那就只是一条 0 firing 的 Activation）。
- webhook 异步 fire + recover（handler 不被慢/panic 拖累）、202 立即返回。

## 5. 关键设计决策

listener 永不知道 workflow（扇出是 app 的事）；Activation 与 Firing 分开（观测 vs 待办——名字即语义）；收件箱轮询（5s tick）而非事件驱动——单进程本地的简单正确选择，serial 推迟的 firing 天然在下个 tick 重试；trigger 实体（trg_）与 firing 运行时（trf_）是两回事（对位 approval 的 apf_ vs 运行时 parked 行）。

## 6. 契约（引用）

端点（CRUD + `:fire`/`:iterate` + activations 两查询）→ [api.md](../api.md) · 表（`triggers`/`trigger_activations`/`trigger_firings`——后两张 Log）→ [database.md](../database.md) · 码 `TRIGGER_*` 12+3 → [error-codes.md](../error-codes.md) · ID：`trg_`/`tra_`/`trf_`。（另有 `GET {id}/firings`——收件箱处置面：started/skipped/superseded/shed）

## 7. 跨域集成

被 workflow 经 Binder 端口驱动（Attach/AttachOnce/Detach）；firings 被 scheduler 经 FiringInbox 端口消费（ListPendingFirings/ClaimFiring 单事务/MarkFiringOutcome）；sensor listener 经 invoker 端口调 function/handler/mcp（bootstrap/sensor.go 适配，TriggeredBy=workflow；sensor 出向 `equip` 边按 targetKind 指 function/handler/mcp 实体）；catalog/mention/relation 三适配器同构。

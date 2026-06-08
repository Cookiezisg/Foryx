---
id: DOC-125
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-07
review-due: 2026-09-01
audience: [human, ai]
---
# Trigger Domain — 独立信号源实体

> **核心职责**：Trigger 是一个**独立实体**（地位同 function/handler），把外部不确定信号（定时 / Webhook / 文件变动 / 主动探测）转化为可靠记录，扇给**监听它的 workflow**。它是**配置实体**——无版本、无 sandbox、无 env。

---

## 1. 定位：从「workflow 的节点」到「独立实体」

旧模型里 trigger 是 workflow 图内的一个节点，主键 `(workflowId, nodeId)`，寄生宿主 workflow、目标写死。**新模型 trigger 是一等实体**：有自己的 `trg_` id、name、catalog 条目、relation 节点（第 9 种）、LLM 工具组。

- **引用计数生命周期**：trigger 实体光存在不干活；**有 ≥1 个激活的 workflow 引用它 → 它的 listener 才启动；引用归 0 → 停**。
- **共享**：N 个 workflow 引用同一个 trigger → 系统只跑**一个** listener，fire 一次扇给这 N 个 workflow 各起一个 flowrun。

---

## 2. 物理模型（三张表）

| 表 | 前缀 | 作用 |
|---|---|---|
| `triggers` | `trg_` | 实体本体（name/kind/config），软删 |
| `trigger_firings` | `trf_` | durable 收件箱：fire 后待 scheduler 认领的信号 |
| `trigger_activations` | `tra_` | 动作日志：每次活动一条，**触没触发都记** |

### 2.1 `Trigger`
```go
type Trigger struct {
    ID          string         // trg_<16hex>
    WorkspaceID string
    Name        string         // 全工作区唯一
    Description string
    Kind        string         // cron | webhook | fsnotify | sensor
    Config      map[string]any // source 专属配置（JSON）
    Outputs     []schema.Field // 声明扇给监听 workflow 的 payload 字段（下游读这些）；TEXT NOT NULL DEFAULT '[]'
    // 计算字段（读时由内存监听表填，不落库）：
    RefCount    int            // 监听它的 active workflow 数
    Listening   bool           // listener 是否在跑
}
```

### 2.2 `Firing`（durable 收件箱）
```go
type Firing struct {
    ID           string         // trf_<16hex>
    TriggerID    string
    WorkflowID   string         // 扇出目标
    ActivationID string         // 反指产生它的 activation
    Payload      map[string]any
    DedupKey     string         // 幂等键
    Status       string         // pending → claimed → started → {skipped, superseded, shed}
    FlowrunID    string
}
```
- **Persist-Before-Act**：fire 瞬间先写 Firing，再由 scheduler 认领。
- **D3 幂等**：`idx_trf_dedup` = UNIQUE(workflow_id, trigger_id, dedup_key)，同一次 fire 重复材化（cron 补跑等）按 workflow 去重。
- **单事务 claim（ADR-021）**：scheduler（波次 4）在一个事务内 `pending→claimed + 建 flowrun + 回填 started`，无 claimed-但-无-flowrun 残留态。

### 2.3 `Activation`（动作日志）
```go
type Activation struct {
    ID          string         // tra_<16hex>
    TriggerID   string
    Kind        string
    Fired       bool           // 这次到底触没触发
    ReturnValue map[string]any // sensor 探测返回值（即使没触发也记）
    Payload     map[string]any // 触发了的话 fire 出的内容
    Error       string         // 探测/调用出错
    Detail      string         // 人读说明，如 "condition evaluated false"
    FiringCount int            // 扇出了几条 Firing
}
```
> **这是「为什么没触发」的唯一可查处**：sensor 探测但没 fire 时，`ReturnValue` 记下看到了什么、`Detail` 说明原因（条件 false / 调用失败）。一次 Activation 产 0 条（没触发）或 N 条（扇出）Firing。

---

## 3. 四种 Source

| kind | 怎么知道该触发 | payload | 说明 |
|---|---|---|---|
| `cron` | 到 cron 刻度 | `{firedAt}` | `config.expression`（5 字段）。dedupKey 按刻度（分钟）去重 |
| `webhook` | 外部 HTTP 推到 `/api/v1/webhooks/{triggerId}/{path}` | `{method, headers, body}` | `config.path` + 可选 `secret`（+ `signatureAlgo:"hmac-sha256-hex"` 走 HMAC）|
| `fsnotify` | 监听路径文件增/改/删 | `{path, eventKind}` | `config.path` + 可选 `events[]`/`pattern` |
| `sensor` | 周期调一个 function/handler，CEL 条件满足 | CEL `output` 构造 | 见 §4 |

> manual 不是 trigger source——手动跑 workflow 是 workflow 自己的能力（不监听任何东西）。手动催一个 trigger 立即响用 `:fire`（测试用）。

---

## 4. Sensor：function/handler + CEL

sensor 把旧的「polling」一般化：**周期性调用一个 function 或 handler.method（永远看 active 版本），对返回值求 CEL 条件，满足则 fire**。

`config`：
```json
{
  "targetKind": "function",   // function | handler
  "targetId":   "fn_xxx",
  "method":     "",           // handler 才用
  "intervalSec": 60,          // 必填，最小 5
  "condition":  "payload.count > 0",     // CEL bool，对返回值（= payload）求值
  "output":     "{\"items\": payload.items}"  // CEL，构造 fire payload
}
```
流程：每 `intervalSec` 调 target → 返回值 `rv` → `condition(rv)` → 真则 `fire(output(rv))`。

- **要状态绑 handler**：handler 是常驻进程，自己记游标/session/连接，做「自上次以来的新东西」式增量探测；**不要状态绑 function**（每次干净探当前值）。trigger 自身永远无状态。
- CEL 引擎复用 `pkg/cel`（与 workflow 节点控制同款，只读 `payload`/`ctx`、无 `now()`）。

---

## 5. 生命周期（引用计数）

- app 维护内存表 `triggerId → {workspaceId, kind, 监听的 workflow 集}`。
- `Attach(triggerId, workflowId)`：首个引用（0→1）启动底层 listener；后续只加入扇出集。
- `Detach(...)`：最后一个引用（1→0）停掉 listener。
- 持久真相在 workflow（谁 active + 引用谁）；**boot 时 scheduler 重新 Attach 重建**（波次 4）。

`Start()` 开机启所有 listener（cron 启调度器，push 型 no-op）；`Shutdown()` 退出时停全部。

---

## 6. 信号流

```
listener 响 → ReportFunc(triggerId, Activity{Fired, Payload, ReturnValue, ...})
   → app 查监听的 workflow → 写 1 条 Activation（总是）
   → Fired 时每 workflow 写 1 条 Firing（扇出）
   → (scheduler claim Firing → 建 flowrun，波次 4)
```
payload 最终成 workflow 的 flowrun 初始输入（旧 trigger 节点本就是 no-op 透传 `flowrun.TriggerInput`，搬出图不丢信号）。

> **`Outputs`（声明的 payload 字段）**：`[]schema.Field`（共享 `internal/pkg/schema` 类型，全锻造实体统一 I/O），声明本 trigger fire 时投给监听 workflow 的 payload 字段——下游节点据此知道能读什么（workflow wiring 用）。仅声明、不强制塑形（运行期 payload 由各 source / sensor `output` CEL 实际构造）。**注**：sensor 的 `condition`/`output` CEL 仍读 `payload`/`ctx`（探测返回值），**非** `input`——`input` 根是 control/approval 专属（节点喂给实体的输入）。

---

## 7. HTTP 端点

| 方法 | 路径 | 动作 |
|---|---|---|
| POST | `/api/v1/triggers` | 创建（kind + config + outputs）|
| GET | `/api/v1/triggers` | 列表（分页）|
| GET | `/api/v1/triggers/{id}` | 详情（含 refCount/listening）|
| PATCH | `/api/v1/triggers/{id}` | 改 name/description/config/outputs（kind 不可变；config 立即生效）|
| DELETE | `/api/v1/triggers/{id}` | 软删（停 listener + 清边）|
| POST | `/api/v1/triggers/{id}:fire` | 手动触发一次（202）|
| GET | `/api/v1/triggers/{id}/activations?firedOnly&cursor&limit` | 动作日志（"为什么没触发"）|
| GET | `/api/v1/trigger-activations/{actId}` | 单条 activation |
| (动态) | `/api/v1/webhooks/{triggerId}/{path}` | webhook 入口（由 webhook listener 挂载）|

> `:iterate`（askai AI 编辑）随 askai 波次 6。

---

## 8. LLM 工具组（8 个）

`search_triggers` / `get_trigger` / `create_trigger` / `edit_trigger` / `delete_trigger` / `fire_trigger`（手动催一次）/ `search_activations` / `get_activation`。

---

## 9. 跨域集成

- **catalog**：进（名字 + 描述，按 kind 分组）。
- **relation**：trigger 是第 9 个节点类型（`trg_` 前缀）；sensor 绑定记一条 `trigger → function/handler` 的 `equip` 出边；`workflow → trigger` 监听边由 workflow 在激活时产（波次 4）。
- **scheduler**（波次 4）：消费 Firing（单事务 claim → flowrun）；boot 时按 active workflow 重建 Attach。
- **mention**：不进（配置实体，非内容快照）。

---

## 10. 错误字典

| Sentinel | Wire Code | HTTP | 场景 |
|---|---|---|---|
| `ErrNotFound` | `TRIGGER_NOT_FOUND` | 404 | |
| `ErrDuplicateName` | `TRIGGER_NAME_DUPLICATE` | 409 | |
| `ErrInvalidKind` | `TRIGGER_INVALID_KIND` | 422 | 非 4 种 source |
| `ErrInvalidConfig` | `TRIGGER_INVALID_CONFIG` | 422 | config 结构缺字段 |
| `ErrInvalidCron` | `TRIGGER_INVALID_CRON` | 422 | cron 表达式语法错 |
| `ErrInvalidCEL` | `TRIGGER_INVALID_CEL` | 422 | sensor condition/output CEL 编译失败 |
| `ErrInvalidInterval` | `TRIGGER_INVALID_INTERVAL` | 422 | sensor interval < 5s |
| `ErrSensorTargetRequired` | `TRIGGER_SENSOR_TARGET_REQUIRED` | 422 | sensor 缺 function/handler 目标 |
| `ErrWebhookSecretMismatch` | `TRIGGER_WEBHOOK_SECRET_MISMATCH` | 401 | HMAC/secret 验签失败 |
| `ErrActivationNotFound` | `TRIGGER_ACTIVATION_NOT_FOUND` | 404 | |
| `ErrListenerUnavailable` | `TRIGGER_LISTENER_UNAVAILABLE` | 503 | listener 未就绪 |
| `ErrFiringNotPending` | `TRIGGER_FIRING_NOT_PENDING` | 409 | claim 竞争失败（scheduler 波次 4 消费）|

---
id: DOC-125
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-05-31
review-due: 2026-06-30
audience: [human, ai]
---
# Trigger

> Workflow 触发器域,Plan 05 三条腿之一。监听外部信号 (cron / fsnotify / webhook) 或被手动调 (manual) → 转 scheduler.StartRun。

**Code 位置**:`backend/internal/{domain,infra/trigger,app/trigger}/`

**联动文档**:
- 完整 spec:[`archive/forge-redesign-2026-05/05-execution-plane.md`](../archive/forge-redesign-2026-05/05-execution-plane.md) §2 + §6.6 + §6.10-§6.13
- FlowRun / Scheduler 兄弟域:[`flowrun.md`](flowrun.md) / [`scheduler.md`](scheduler.md)

---

## 1. 定位

Trigger 是 workflow 执行的**信号入口**。监听 4 种触发源,fire 时调 `SchedulerStarter.StartRun(workflowID, kind, input)`。

- **trigger** 不知道 workflow 长啥样,不执行任何节点
- 持有 per-(workflowID, nodeID) listener 注册表,workflow accept/revert/delete 时同步 register/unregister
- **lastFiredAt 持久化**：`TriggerSchedule` DB 表（`trigger_schedules`）存 per-(workflowID, nodeID) 的 `last_fired_at`；`onFire` 后 best-effort 更新；下次进程重启 `RegisterTrigger` 时从 DB 种值到 cron listener 的内存 `lastFire`，实现跨重启补漏刻度（详 §6.2）
- runtime listener state（entries/lastFire map）完全 in-memory；`TriggerSchedule` 是唯一的持久化投影 — 经 HTTP `GET /workflows/{id}/triggers` 暴露 (§6.12)
- **§20 multi-user**：`triggerdomain.Spec.UserID` 字段在 register 时填入 workflow owner；`onFire` 回调用 spec.UserID 构造 detached ctx，确保 User B 的 cron workflow fire 时 scheduler.StartRun 能找到 workflow。**2026-05-24 更新**：Spec.UserID 缺失 = wiring bug → log error + drop trigger（不再有 `DefaultLocalUserID` 兜底，避免静默把别人的 trigger 跑到默认用户名下）。

---

## 2. 4 种 V1 listener (§2)

| Kind | Library | config | 链路 |
|---|---|---|---|
| `cron` | `robfig/cron/v3` (v3.0.1,首次引入) | `expression: "0 */1 * * *"` | tick → onFire → scheduler.StartRun |
| `fsnotify` | `fsnotify/fsnotify` v1.10 (升 direct) | `path / pattern / events: [create,modify,delete]` | match → onFire |
| `webhook` | `net/http` (挂主 ServeMux 子路径) | `path / method / secret? / signatureAlgo? / signatureHeader?` | POST → onFire |
| `manual` | 无 listener | — | HTTP `:trigger` / LLM `trigger_workflow` 直调 |

---

## 3. 生产级要点 (Plan 05 §6)

### §6.2 Cron 漏触发 — missedPolicy=runOnce

`lastFire` per-key 内存表，**启动时从 `TriggerSchedule.last_fired_at`（DB）种值**（跨进程重启补漏刻度）；`RegisterTrigger` 调 `ScheduleStore.GetSchedule` 读 lastFiredAt → 种入 cron listener → Register 检查 `schedule.Next(last) < now` → 立刻 goroutine fire 一次（`runOnce` 默认，不补多次）。`onFire` 后 `ScheduleStore.UpdateLastFiredAt` best-effort 持久化。`runAll` / `skip` 留 V1.5。

### §6.6 Webhook secret

`secret` 字段可空 (不校验) 或非空 (POST 必带 `X-Webhook-Secret` header 或 `?token=` query 匹配)。不匹配返 401。

### §6.10 Cron 时区锁本地

`robfig/cron.New(WithLocation(time.Local))` — 桌面 app 跟用户笔记本时区一致。V1.5 加 per-trigger override。

### §6.11 Fsnotify 路径不存在 fail-soft

Register 时 `os.Stat(path)` 不存在 → 标 state=error + LastError + 返 `ErrPathNotExist`;**不阻塞 workflow 本身存在**。trigger Service 把 spec 仍记录便于 State() 暴露给用户。

### §6.12 Trigger 状态可见

`GET /api/v1/workflows/{id}/triggers` 返每 trigger 的 `{kind, status (active/idle/error), lastFiredAt, nextFireAt (cron only), lastError}`。

### §6.13 Trigger panic recover

每 listener 的 onFire goroutine 包 `defer recover()` (cron/fsnotify/webhook 一致)。panic → log + 标 state=error + 通知用户;不影响其他 listener。

---

## 4. 域结构

### 4.1 `domain/trigger`

4 常量 (KindCron / KindFsnotify / KindWebhook / KindManual) + 3 state (active / idle / error) + Spec (WorkflowID/NodeID/Kind/Config/**LastFiredAt?** — 注册时由 app 层从 TriggerSchedule 种值) + State (含 LastFiredAt/NextFireAt/LastError) + 4 sentinels (ErrPathNotExist / ErrPathConflict / ErrWebhookSecretMismatch / ErrInvalidCronExpression)。

**TriggerSchedule**（`infra/store/trigger`，`trigger_schedules` 表）：持久化 listener 注册状态，主键 `(workflow_id, trigger_node_id)`。关键字段：`last_fired_at DATETIME`（cron 跨重启补漏种子），`kind`, `spec`（JSON）。方法：`UpsertSchedule`（注册时插/更）/ `GetSchedule`（读 lastFiredAt）/ `UpdateLastFiredAt`（每次 fire 后 best-effort 更新）。

**ScheduleStore port**（`app/trigger.ScheduleStore`）：`UpsertSchedule / GetSchedule / UpdateLastFiredAt`——由 `app/trigger.Service.SetScheduleStore` 注入（main.go 后置装配避免循环依赖）。

### 4.2 `infra/trigger/{cron,fsnotify,webhook}`

3 个独立 listener 实现:
- **cron**:robfig/cron 包装 + per-key entries + lastFire 内存（启动由 ScheduleStore 种值 → 跨重启补漏）+ Start/Stop/Register/RegisterWithLastFire/Unregister/State
- **fsnotify**:lazy watcher 创建 + per-key specs + dispatch fan-out + 模式过滤 (glob basename) + 事件 kind 过滤 (Create/Write/Remove/Rename/Chmod)
- **webhook**:`http.ServeMux` 子路径 `/api/v1/webhooks/{wfId}/{path}` + path 冲突拒 + body 10MB cap + JSON auto-parse fallback bodyRaw + secret 校验（§12.4：plain `X-Webhook-Secret` / `?token=` 比对 **或** `signatureAlgo=hmac-sha256-hex` HMAC 验签，`signatureHeader` 缺省 `X-Hub-Signature-256`，自动剥 `sha256=` 前缀）

### 4.3 `app/trigger`

`Service` 整合 4 种,主要 API:
- `New(mux *http.ServeMux, log)` — 构造;cron.Start() 立刻起;webhook 接 mux
- `SetScheduler(SchedulerStarter)` — post-construction 接 scheduler (断 ctor cycle)
- `RegisterTrigger(spec)` / `UnregisterByWorkflow(workflowID)` — 注册 / 撤
- `State(workflowID)` — 返 trigger states
- `FireManual(ctx, workflowID, input)` — HTTP / LLM 手动入口

Service 即使 listener Register 失败也保留 spec (让 State() 暴露给用户,§6.11)。

---

## 5. SchedulerStarter port (断 ctor cycle)

```go
type SchedulerStarter interface {
    StartRun(ctx, workflowID, triggerKind string, input map[string]any) (runID, err)
}
```

main.go / harness 装配顺序:`triggerService := trigger.New(mux, log)` → `schedulerService := scheduler.NewService(repo, workflowReader, notif, log)` → `triggerService.SetScheduler(schedulerService)`。

---

## 6. 错误码 (4 sentinels)

详 [`../references/backend/error-codes.md`](../references/backend/error-codes.md):
- `TRIGGER_PATH_NOT_EXIST` (422) — fsnotify path 不存在
- `TRIGGER_PATH_CONFLICT` (409) — webhook 路径已注册
- `TRIGGER_WEBHOOK_SECRET_MISMATCH` (401) — secret 校验失败 (webhook handler 直返 HTTP 401,不进 errmap;sentinel 仅用于 errors.Is)
- `TRIGGER_INVALID_CRON_EXPRESSION` (400) — cron 表达式无效

---

## 7. 测试覆盖

- 6 domain 单测 (枚举值 / sentinel / Spec+State JSON round-trip)
- 6 cron listener 测试 (invalid expr / 注册-fire / unregister stops / state pre+post / register 替换 existing / panic recover)
- 6 fsnotify listener 测试 (path-not-exist sentinel + state=error / empty path / 创建文件 fire / pattern 过滤 / unregister / pre-register idle)
- 7 webhook listener 测试 (注册+fire / path conflict / empty path / header secret / query token alt / unregister-404 / method 405)
- 6 Service 测试 (manual 无 listener / cron-invalid tracks-spec-but-errors / UnregisterByWorkflow cascades / FireManual forwards / no-scheduler returns sentinel / SetScheduler concurrent-safe)

---

## 8. 历史

- 2026-05-13 Plan 05 完成:E3 (domain) + E4 (infra + Service) 落地。robfig/cron v3.0.1 首次引入;fsnotify v1.10 升 direct。`Listener.Stop()` cron/fsnotify 各自 graceful shutdown。Service 内 `kindForNode` helper 让 onFire 拼正确 triggerKind 传给 scheduler。

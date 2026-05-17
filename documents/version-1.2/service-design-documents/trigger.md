# Trigger

> Workflow 触发器域,Plan 05 三条腿之一。监听外部信号 (cron / fsnotify / webhook) 或被手动调 (manual) → 转 scheduler.StartRun。

**Code 位置**:`backend/internal/{domain,infra/trigger,app/trigger}/`

**联动文档**:
- 完整 spec:[`adhoc-topic-documents/forge_redesign/05-execution-plane.md`](../adhoc-topic-documents/forge_redesign/05-execution-plane.md) §2 + §6.6 + §6.10-§6.13
- FlowRun / Scheduler 兄弟域:[`flowrun.md`](flowrun.md) / [`scheduler.md`](scheduler.md)

---

## 1. 定位

Trigger 是 workflow 执行的**信号入口**。监听 4 种触发源,fire 时调 `SchedulerStarter.StartRun(workflowID, kind, input)`。

- **trigger** 不知道 workflow 长啥样,不执行任何节点
- 持有 per-(workflowID, nodeID) listener 注册表,workflow accept/revert/delete 时同步 register/unregister
- runtime state 完全 in-memory (无 DB 表) — 经 HTTP `GET /workflows/{id}/triggers` 暴露 (§6.12)
- **§20 multi-user**：`triggerdomain.Spec.UserID` 字段在 register 时填入 workflow owner；`onFire` 回调用 spec.UserID 构造 detached ctx，确保 User B 的 cron workflow fire 时 scheduler.StartRun 能找到 workflow（不再用 DefaultLocalUserID 兜底）。

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

`lastFire` per-key 内存表;Register 时若 `schedule.Next(last) < now` → 立刻 goroutine fire 一次 (补;`runOnce` 默认,不补多次)。`runAll` / `skip` 留 V1.5。

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

4 常量 (KindCron / KindFsnotify / KindWebhook / KindManual) + 3 state (active / idle / error) + Spec (WorkflowID/NodeID/Kind/Config) + State (含 LastFiredAt/NextFireAt/LastError) + 4 sentinels (ErrPathNotExist / ErrPathConflict / ErrWebhookSecretMismatch / ErrInvalidCronExpression)。

### 4.2 `infra/trigger/{cron,fsnotify,webhook}`

3 个独立 listener 实现:
- **cron**:robfig/cron 包装 + per-key entries + lastFire 内存 + Start/Stop/Register/Unregister/State
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

详 [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md):
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

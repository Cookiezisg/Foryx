# Audit trace — `transport/httpapi/handlers/sandbox.go` (364 LOC)

Phase A audit fork (B3) — §S3 / §S9 / §S15 / §S16 / §S17.
sandbox v2 endpoints (per `service-design-documents/sandbox.md` §11) — runtime install / env管理 / GC / bootstrap / per-conv reset。

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | `sandbox.go:88-95` | `ListRuntimes`: `rows, err := h.svc.ListRuntimes(r.Context()); if err != nil { responsehttpapi.FromDomainError(...) }` | A.1 / A.5 | OK | 标准 §S6 thin handler。错误经 `FromDomainError` 路径——sandbox sentinel 全已登记 errmap.go:103-112。`r.Context()` 是只读路径正确选择。 | N-A | — | — | — |
| 2 | `sandbox.go:103-116` | `ListEnvs`: `if ownerKind == "" { responsehttpapi.Error(... "OWNER_KIND_REQUIRED" ...) }` then `h.svc.ListEnvs(r.Context(), ownerKind); FromDomainError 兜底` | A.5 / N6 | EDGE | wire code `OWNER_KIND_REQUIRED` 不在 errmap——是 handler-local 字面量，与 `INVALID_REQUEST` 同 pattern (handler 业务 wire code 直注)。**问题**：sandbox.md §11 表格未必登记此 wire code（通常约定一致用 `INVALID_REQUEST` + details map），自创新 wire code 引入 ad-hoc 词汇。EDGE-LOW：合规 §S6（handler 业务校验），但 §N1/wire 词汇一致性可商榷。 | LOW | 客户端需识别新 wire code (`OWNER_KIND_REQUIRED`) 否则 fallback 到通用错误处理 | 改用 `responsehttpapi.Error(w, 400, "INVALID_REQUEST", "ownerKind query parameter is required", nil)` 一致词汇；或加 `errorsdomain.ErrOwnerKindRequired` sentinel + errmap 行（**过度设计——LOW**） | FOUND |
| 3 | `sandbox.go:119-127` | `GetEnv`: `id := r.PathValue("id"); env, err := h.svc.GetEnv(r.Context(), id); FromDomainError` | A.1 / A.5 | OK | sandboxdomain.ErrEnvNotFound 经 errmap → 404。thin handler。 | N-A | — | — | — |
| 4 | `sandbox.go:130-137` | `DiskUsage`: `total, err := h.svc.TotalDiskUsage(r.Context()); FromDomainError` | A.1 | OK | 只读路径，标准。 | N-A | — | — | — |
| 5 | `sandbox.go:141-150` | `BootstrapStatus`: `body := {"ok": h.svc.IsReady(), "miseBin": h.svc.MiseBin()}; if err := h.svc.BootstrapError(); err != nil { body["error"] = err.Error() }` | A.1 / A.4 | OK | `BootstrapError()` 是观察性 getter（返记录的 bootstrap 失败原因），handler 把 err.Error() 直注 wire 是 **故意设计**——bootstrap 失败是 *observable state*，不是 HTTP error（同 site 12 retryBootstrap 注释说明）。**§S3 例外**：err.Error() 入 wire body 是产品语义（UI 显示 banner），不是错误吞。**§S16 不适用**：这不是 wrap，是 wire-shape 字段。 | N-A | — | — | — |
| 6 | `sandbox.go:160-175` | `ListConvEnvs`: `all, err := h.svc.ListEnvs(r.Context(), OwnerKindConversation); ... for _, e := range all { if strings.HasPrefix(e.OwnerID, prefix) { scoped = append(scoped, e) } }` | A.1 | OK | 错误路径 FromDomainError；filter loop 无 IO 无错误吞。`r.Context()` 单路径只读 OK。 | N-A | — | — | — |
| 7 | `sandbox.go:182-200` | `envAction`: `id, action := splitAction(...); if action != "destroy" { ... UNKNOWN_ACTION ... }; env, err := h.svc.GetEnv(...); ... h.svc.Destroy(r.Context(), owner)` | A.1 / A.2 / A.5 | EDGE | **§S9 关注点**：`Destroy` 是 sandbox v2 的**真实拆环境**操作（删 env 目录 + 标 manifest deleted_at）——属"终态写"。**但**：`r.Context()` 在 HTTP 同步操作中等待响应是合理的（用户主动触发，前端 spinner 中），cancel 通常意味着用户主动放弃。**问题点**：若 Destroy 是长跑（rm -rf 大目录），中途 cancel 会留下半 deleted env（manifest 标记完但 disk 未清完，反之亦然）。**判定 EDGE**：sandbox.md §11 未明确 Destroy 的"中途 cancel 一致性保证"——若 `sandboxapp.Service.Destroy` 内部用 detached ctx 兜底（**需查 sandbox-app audit batch**），handler 用 r.Context() OK；若没有，handler 应承担 detached ctx 责任。**B3 fork 不审 app 层，但 flag 给 sandbox-app audit 链**。 | LOW | 用户启动 destroy 期间关 tab → manifest 标记可能完但 mise env files 残留 disk（GC 兜底清，但短期不一致） | (a) 查 sandboxapp.Service.Destroy 实现是否 detached；(b) 若没，handler 用 `reqctxpkg.SetUserID(context.Background(), uid)` 派生 ctx | FOUND |
| 7b | `sandbox.go:185-187` | wire code `UNKNOWN_ACTION` (未登记 errmap) | A.5 | OK | handler-local action dispatch 错误，不走 sentinel chain。同 mock_llm `TRACER_DISABLED` pattern——404 直字面量。`UNKNOWN_ACTION` 出现在 4 个 dispatcher (envAction / runtimeAction / sandboxAction / convEnvKindAction)，路由 routing-table 错误兜底，合规 §S6（不是业务 sentinel）。 | N-A | — | — | — |
| 8 | `sandbox.go:205-217` | `runtimeAction`: 同 envAction 结构 + `h.svc.DeleteRuntime(r.Context(), id)` | A.1 / A.2 | EDGE | 同 site 7：DeleteRuntime 是终态写（删 mise 装的整个 runtime kind）。**判定同 site 7**——sandbox-app audit batch 跟踪。 | LOW | 同 site 7 | 同 site 7 | FOUND |
| 9 | `sandbox.go:225-238` | `sandboxAction`: `switch action { case ":gc": ... case ":retry-bootstrap": ... case "runtimes:install": ... default: UNKNOWN_ACTION }` | A.5 | OK | dispatch table——3 子 action 各自 handler。default 路径返 404 UNKNOWN_ACTION 同 §S6。 | N-A | — | — | — |
| 10 | `sandbox.go:240-256` | `gc`: 解析 `?olderThanDays=N` (`if d, err := strconv.Atoi(v); err == nil && d > 0 { days = d }`) | A.1 / 反校验剧场 | EDGE | `strconv.Atoi` 错和负数/0 都 silently fall back to default 30 days。**§S3**：err 被 silently 吞掉无日志——但这是 **设计原则 #6 反校验剧场**例外：默认值兜底是 dev/UI 友好（前端 dropdown 已限合法范围），后端不必告知"你给的不是数字"。EDGE-LOW：合规 §S6（前端可防），但严格 §S3 要求至少 inline 注释说明"silent fallback by design"。 | LOW | 用户传 `?olderThanDays=abc` 静默用 30 天，可能误以为传成功 | 加 inline 注释：`// silent fallback to 30 days when missing/invalid—frontend constrains range, ad-hoc curl is OK with default` | FOUND |
| 11 | `sandbox.go:247-255` | `gc`: `removed, err := h.svc.GC(r.Context(), time.Duration(days)*24*time.Hour); if err != nil { FromDomainError; return }; Success(... {"removed": removed, "olderThanDays": days})` | A.1 / A.2 | EDGE | GC 是终态写（清旧 env）。**长跑风险类似 site 7/8**——但 GC 是后台维护类操作（用户不主动 watch），中途 cancel 留半完成更可能。**判定 EDGE**：sandbox-app audit 跟踪 GC 内部是否 detached / 是否分批 commit / 是否每 env 独立 tx。 | LOW | GC 中途用户 reload，部分 env 已 destroy 部分未；下次 GC 兜底，短期不一致 | 同 site 7/8 — sandbox-app audit batch 检查 GC 内部 ctx 设计 | FOUND |
| 12 | `sandbox.go:258-273` | `retryBootstrap`: `if err := h.svc.RetryBootstrap(r.Context()); err != nil { Success(... {"ok": false, "error": err.Error()}) }` | A.4 / A.1 | OK | **故意设计**：BootstrapStatus + RetryBootstrap 用 200 + body.error 字段表达失败状态（注释 254-260 已说明：bootstrap 失败是观察性状态非 HTTP error，UI banner 显示）。**§N2 状态码语义**例外——但与 §S3 不冲突（错误**没**被吞，反而入 wire body 暴露给用户）。**§S16 不适用**：err.Error() 进 wire body 是 wire-shape 字段，非 wrap。 | N-A | — | — | — |
| 13 | `sandbox.go:280-298` | `installRuntime`: `if err := decodeJSON(r, &req); err != nil { FromDomainError } ... if req.Kind == "" { Error(... "KIND_REQUIRED" ...) } ... rt, err := h.svc.EnsureRuntime(r.Context(), RuntimeSpec{...}, nil); FromDomainError` | A.1 / A.2 / A.5 | EDGE | `EnsureRuntime` 是 sandbox v2 真实安装路径（mise install + 写 manifest）——**长跑 + 终态写 + 任务描述明示重灾**。"lazy install 是否 silent fail？runtime missing 是否 graceful skip？" → handler 层用 `r.Context()` 把 cancel 透传给 mise install 子进程。**判定 EDGE**：(a) handler 层正确用 r.Context() 让 install 跟随 request 生命周期合理；(b) 但若用户 reload → mise 子进程被 SIGKILL → 半装的 runtime（mise 目录残留 + manifest 未写）。**真正的 detached 责任在 sandboxapp.Service.EnsureRuntime**。 | LOW | 用户触发 install 后关 tab → mise 进程被杀，半装 runtime 在 disk；下次 install 同 spec mise 检测 idempotent OK；但 manifest 状态可能 stuck "installing" | sandbox-app audit batch 检查 EnsureRuntime 内部：mise install 应在 detached ctx 跑 + manifest 终态写双保 | FOUND |
| 13b | `sandbox.go:286-289` | `if req.Kind == "" { responsehttpapi.Error(w, 400, "KIND_REQUIRED", "kind is required", nil); return }` | A.5 | EDGE | 同 site 2：自创 wire code `KIND_REQUIRED` 不在 errmap。同 §6 反校验剧场——前端表单已强制，后端守门只为 ad-hoc curl 友好。EDGE-LOW，词汇一致性问题，非 §S3 violation。 | LOW | 客户端需识别 `KIND_REQUIRED` 作为 4xx 类 | 改用 `INVALID_REQUEST`+details 一致；或加 sandboxdomain.ErrKindRequired sentinel 入 errmap | FOUND |
| 14 | `sandbox.go:303-324` | `convEnvKindAction`: `convID + "_" + kind` (注释 311-314 解释 `_` 选择) | A.1 | OK | owner.ID 拼接是设计选择（注释清晰），无错误吞。`Destroy` 错误经 FromDomainError；同 site 7 EDGE 转移到 sandbox-app audit。 | N-A | — | — | — |
| 15 | `sandbox.go:329-350` | `convEnvsAction`: `all, err := h.svc.ListEnvs(...); ... for _, e := range all { if !strings.HasPrefix(e.OwnerID, prefix) { continue }; ... if err := h.svc.Destroy(r.Context(), owner); err != nil { FromDomainError; return } }` | A.1 / A.2 | EDGE | **关键观察**：循环中第一次 Destroy 失败立即 return — 已 destroy 的 env 已 commit，未 destroy 的留着，**部分成功**返一个 error envelope（`removed` 计数没返）。**§S3 边界**：失败可见（不吞），但不告诉客户端"已 destroy N 个，第 N+1 失败"——客户端无法 idempotent retry（因为再调一次会试图 destroy 已不存在的 env，触发 ErrEnvNotFound）。**EDGE-LOW**：UX 不理想但不违反 §S3 严格定义；reset-all 是用户清理操作，retry 时 ListEnvs 已过滤掉 deleted。 | LOW | 部分失败时客户端不知 "removed N before failure"；retry 会 ListEnvs 重新只看到剩余的 | (a) 累积 errors 用 `errors.Join` 返 multi-error；(b) 或者继续循环 collect 失败 env id 一起返 wire body（牺牲 §N2 200/422 严格性）；(c) 现状 LOW 可接受 | FOUND |
| 16 | `sandbox.go:282` | `if err := decodeJSON(r, &req); err != nil { responsehttpapi.FromDomainError(w, h.log, err); return }` | A.4 / A.5 | OK | 共享 helper（apikey.go:217）已修 §S16 prefix（`handlers.decodeJSON: %w`）；errors.Join → errorsdomain.ErrInvalidRequest 经 errmap 落 400。整链合规。 | N-A | — | — | — |
| 17 | `sandbox.go:358-363` | `splitAction(idAction string) (id, action string) { if i := strings.LastIndexByte(idAction, ':'); i >= 0 { return idAction[:i], idAction[i+1:] }; return "", idAction }` | A.1 | OK | 纯 string parse，无 IO 无 error 路径。无吞错风险。 | N-A | — | — | — |

## Sub-check 模板

A.1 §S3 错误吞没:
  - violations: not present (in strict sense)
  - EDGE-LOW: site 10 `strconv.Atoi` 静默 fallback to default 30 days 缺 inline 注释；site 15 partial-success 失败 mid-loop 不告诉客户端"已 destroy N 个"——UX 缺陷非 §S3 严格违规

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: site 7 (envAction Destroy), site 8 (runtimeAction DeleteRuntime), site 11 (gc GC), site 13 (installRuntime EnsureRuntime), site 14 (convEnvKindAction Destroy), site 15 (convEnvsAction Destroy loop)
  - 各自 ctx 来源: 全部 `r.Context()` 直传 service
  - violations: **N/A at handler layer; EDGE-FLAG 转 sandbox-app audit batch**——任务描述明示"sandbox install 是 long-running，可能起 fire-and-forget goroutine——需 detached ctx"。**handler 层用 r.Context() 是合理的中间层选择**（让用户主动 cancel 能透传），detached ctx 责任在 `sandboxapp.Service` 的 EnsureRuntime/Destroy/DeleteRuntime/GC——它们应在内部 launch detached goroutine 兜终态写（mise process spawn + manifest 落库）。**B3 fork 边界**：handler 0 violation；service 内部待 sandbox-app audit。
  - 备注：sandbox handler 与 chat SendMessage 同模式——handler 透传 r.Context() 给 service，service 自行决定是否 detached。这是 §S9 在 transport 层的标准姿态。

A.3 §S15 ID 生成:
  - ID generation calls: 0
  - violations: N/A — 本文件不生成业务 ID。runtime/env IDs 由 `sandboxapp.Service` 内部 idgenpkg.New(prefix) 生成（推测前缀 `sbr_` runtime / `sbe_` env，**待 sandbox-app audit 验证**）；handler 透传 r.PathValue("id") 用 service 返回值。

A.4 §S16 错误 wrap 格式:
  - violations: not present
  - 备注：handler 不 wrap error（全部经 FromDomainError 透传或 inline `responsehttpapi.Error` 字面量 detail）。site 5/12 把 err.Error() 入 wire body 是 wire-shape 字段，**不是 wrap**——故意暴露给 UI 显示。decodeJSON helper（外部 apikey.go）已合规。

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: 本文件 0（不在 handler 包定义）；消费的是 sandboxdomain 12 个 sentinel
  - 已登记 errmap (10/12):
    * `ErrRuntimeNotSupported` errmap.go:103
    * `ErrRuntimeInstallFailed` errmap.go:104
    * `ErrEnvNotFound` errmap.go:105
    * `ErrEnvCreateFailed` errmap.go:106
    * `ErrDepInstallFailed` errmap.go:107
    * `ErrSpawnFailed` errmap.go:108
    * `ErrSpawnTimeout` errmap.go:109
    * `ErrEnvInUse` errmap.go:110
    * `ErrInvalidOwnerID` errmap.go:111
    * `ErrCmdRequired` errmap.go:112
  - missing (2/12): `ErrDockerNotInstalled` (sandbox.go:206), `ErrDockerDaemonDown` (sandbox.go:213)
  - 备注：`grep -rn "ErrDockerNotInstalled\|ErrDockerDaemonDown" backend/` 显示**0 消费者**——两 sentinel 已声明但 service 层无人 return（Phase 5 docker support 预留）。**§S17 严格说**：未到达 handler 路径的 sentinel 不必登记。**判定 OK**——但**预留风险**：未来 docker runtime kind 接入时若忘加 errmap 行，会触发 "unmapped domain error" 警告（与 §S17 规则中的"reqctxpkg.ErrMissingUserID 跨层使用预先登记"逻辑一致）。**Pre-registration 候选**：建议同步登记 `ErrDockerNotInstalled` (422 SANDBOX_DOCKER_NOT_INSTALLED) + `ErrDockerDaemonDown` (503 SANDBOX_DOCKER_DAEMON_DOWN) 兜底。

## Cross-cutting

1. **Wire code ad-hoc 词汇**：site 2 (`OWNER_KIND_REQUIRED`) / site 13b (`KIND_REQUIRED`) / site 7b/9 (`UNKNOWN_ACTION`) / mock_llm (`TRACER_DISABLED`) 全是 handler-local 字面量 wire code，未入 errmap。**判定**：合规 §S6（handler 业务校验自创 wire code 是允许的），但项目层面应有约定——是统一收编到 errmap 的 sentinel？还是允许 handler 字面量？**建议**：当前项目中两种风格混用（B1/B2 也观察到），可在 `service-contract-documents/error-codes.md` 加一节说明哪些 wire code 是 sentinel-driven、哪些是 handler-local。**LOW 全文件**。

2. **Sandbox 终态写的"transport 层透传 + service 层 detached"二段责任**：handler 用 r.Context() 让用户 cancel 能透传给 service，是合理的 transport 层选择；detached ctx 责任在 sandbox-app service 内部。这是 §S9 的"transport 不预设 detached"标准姿态——B3 边界明确将 6 个终态写候选标 EDGE 转 sandbox-app audit batch。

3. **Pre-registration 候选**：sandboxdomain 已声明 `ErrDockerNotInstalled` / `ErrDockerDaemonDown` 但 0 消费者——Phase 5 docker 接入时若忘加 errmap 行会触发警告。**建议同步登记**——成本极低（2 行 errmap.go），收益是 Phase 5 接入零摩擦。HIGH-ROI suggestion 但**不强制**（当前 0 路径触发，严格 §S17 不要求）。

4. **Long-running operation 的 user cancel 语义**：site 11 (gc) / site 13 (installRuntime) 等是用户主动触发但 mise install 可能数分钟——若用户 reload，r.Context() cancel → mise SIGKILL → 半装。**这是产品决策点**：是允许 user cancel 中断（当前），还是后端用 detached ctx + 必装完？sandbox.md §11 应明确——若不明确，sandbox-app audit batch 应 flag。**B3 不下结论**。

5. **partial-success 错误返回 UX**：site 15 (`convEnvsAction`) 是 batch destroy，第一次失败立即返 error，已成功的 N 个不告诉客户端。**§S3 不算违规**（错误可见），但产品 UX 待提升——可考虑 `errors.Join` 返多 error + wire body 含 `removed: N, failed: [{id, code}, ...]`。**LOW**。

6. **handler 层 §S6 一致性**：本文件 14 endpoint 全部薄 handler——`r.PathValue` / `r.URL.Query().Get` → `h.svc.X(ctx, ...)` → envelope。最干净的 sandbox 端点设计——dispatch 用 splitAction helper + switch 比 mux 更紧凑（注释 56-65 说明设计意图）。**§S5/S6 模范**。

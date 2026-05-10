# Package audit summary: internal/pkg/eventlog

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: 本包是 Emitter helper——dual-write SSE Bridge + chatdomain.Repository 的薄壳。**1 个字面 LOW**（attrs json.Marshal err 静默吞 site 6(a)，工程上几乎不触发）。**4 个 EDGE** 集中在 best-effort dual-write 与 publish 错误粒度——godoc 与 §S21 invariant / Phase 2B "持久化作事实源" 之间存在一致性边界没说清的问题。
- **§S9 detached ctx 终态写**: **MED 隐患** — `StopBlock → FinalizeStop`（site 14）是 block status streaming → terminal 的终态写；当前实现把 ctx 来源责任留给调用方，未在 emit 层主动构造 detached ctx。chat loop / tool framework 在用户取消（r.Context cancel）路径下，FinalizeStop 跟着 cancel = SSE 已发 stop 但 DB 行 status 永留 streaming（前端 history replay 看见永远加载中）。
- **§S15 ID 生成**: 全走 `idgenpkg.New("msg")` / `idgenpkg.New("blk")`，前缀符合 CLAUDE.md §S15 list。0 violation。
- **§S16 错误 wrap 格式**: 本包不上抛错误（emit 全 silent skip + log），仅 1 处 panic（site 17 MustFrom）属 wiring-bug 终止，不是 error path。0 violation。
- **§S17 errmap 单一事实源**: 本包定义 **0 sentinel**——错误源（`eventlogdomain.ErrInvalidEvent` / `ErrSeqTooOld`）在 domain/eventlog 包；都经 `handlers/eventlog.go:107` 直接 `errors.Is` 处理（返 410 SEQ_TOO_OLD 按 §N7），**不走 FromDomainError**——无需登记 errmap。属合法设计。

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| eventlog.go | 486 | 19 | 12 | 4 (3 FIXED + 1 partial) | 0 | 3 (1 WAIVED + 2 doc-边界 carry) |
| **TOTAL** | **486** | **19** | **12** | **4** | **0** | **3** |

## Status (post-fix)

| site | severity | status | commit |
|---|---|---|---|
| site 3 (publish err 粒度) | LOW | FIXED | this batch |
| site 5 (StartMessage 失败仍返 ID) | LOW | FIXED | this batch |
| site 6(a) (attrs json.Marshal silent) | LOW | FIXED | this batch |
| site 6(b) (SaveBlock dual-write best-effort) | — | WAIVED | godoc 契约符合 §S3 例外 |
| site 13(b) (AppendDelta dual-write) | LOW | WAIVED | 已 Warn log；godoc 边界澄清归 Phase 2B↔3 cutover |
| site 14 (StopBlock FinalizeStop 终态) | MED | FIXED | this batch |

> 注: `_test.go` 按 fork 约束跳过；EDGE 列含 5 处 dual-write / publish 错误处理粒度议题（§S3 字面 OK，但 §S21 invariant 暗合）+ 1 处 §S21 cross-block parent 旧 ctx 隐患（行 268-285）

## Severity breakdown

| Severity | Count | Sites |
|---|---|---|
| HIGH | 0 | — |
| MED | 1 | site 14 (StopBlock → FinalizeStop 终态写无 detached ctx) |
| LOW | 3 | site 3 (publish err 粒度过粗——ErrInvalidEvent 被淡化为 Warn), site 5 (StartMessage publish 失败仍返新 msgID = 制造 dangling parentId), site 6(a)+13(b) (best-effort dual-write 两个吞错点合并：attrs 失败 silent + AppendDelta 失败 silent 与 §S21 / Phase 2B 事实源边界不清) |

**Net: 4 violations** (0 HIGH / 1 MED / 3 LOW)

## Cross-cutting

### 1. Phase 2B "持久化作事实源" vs godoc "best-effort dual-write" 一致性裂缝

`pkg/eventlog/eventlog.go` godoc / 行内注释多次称 dual-write 是 "Best-effort: failures log + continue (Bridge already shipped the SSE event; DB miss only affects history replay)"（行 206-211 / 384 / 412）。

但 event-log-protocol.md §1 设计原则 5 说 "持久化与实时分离 — DB 存最终 block 树（给历史用），SSE 推事件流（给实时 UI 用），**两者用同一数据模型，历史回放 = DB 转事件流再 emit 一遍**"。+ §S21 强调 invariant `block.status` 单向 streaming → terminal、deltas append-only。

**不一致**：

- 当前 dual-write best-effort = SSE 是事实源、DB 只是 mirror
- 协议设计 = DB 是事实源、SSE 是 transient view
- 当 dual-write 失败：SSE 与 DB 分歧，replay 拉到 incomplete content / 永留 streaming block

**这是 4 个 violation 的共同根**：
- site 5 (publish 失败但返 msgID) — emitter 假设 SSE 是 source-of-truth
- site 6(a) (attrs marshal 失败 silent) — best-effort 字面 §S3 violation
- site 13(b) (AppendDelta 失败 silent) — replay 缺 delta，与 §S21 append-only 矛盾
- site 14 (FinalizeStop 失败 silent + 无 detached ctx) — 终态卡死，与 §S21 status 单向矛盾

**建议**: 在 Phase 2B 边界明确选边——

(选项 A) 保留 best-effort，但在 godoc / event-log-protocol.md §6 持久化节明文说 "DB 失败时 replay 可能 incomplete；30s buffer 内 SSE 重连可补，window 外丢"，并在 ops runbook 加 alarm（dual-write 失败率 > 0.1% / hr 触发）

(选项 B) 升级 dual-write 为强一致——SSE 发布前先 DB 落，DB 失败则 SSE 也不发——保 invariant 但牺牲流式延迟

### 2. §S9 终态写的 emit-layer 责任边界

CLAUDE.md §S9 example 是 chat 流被 cancel 后写 assistant final message 用 detached ctx。这条惯例当前留在 chatapp.Service 内：service 调用 emitter.StopBlock 之前自己构造 detached。

但 emit 层有 5+ 个调用方（chat loop / subagent host / mcp / forge / skill）——每个都要重写 detached ctx 构造，**心智负担分散且漏一个就 site 14 violation**。

**建议**: 把 detached ctx 构造下沉到 emitter——StopBlock / StopMessage 内部对终态 dual-write 路径用 `reqctxpkg.SetUserID(context.Background(), uid)` 自动 detach。这样调用方不必每次想这件事。需要保留 conversationID 给 events bridge 路由。

或者，若决定保留"调用方负责"约定，至少在 §S18 / event-log-protocol.md §4 显式标注 "终态 emit 应使用 detached ctx，调用方责任"，并在每个调用方位点 grep 是否合规。

### 3. publish err 粒度过粗（site 3）

`em.publish` 把 ctx-cancel / Bridge buffer 阻塞 / `ErrInvalidEvent` 三种语义截然不同的 err 全 Warn 一刀切：

- ctx-cancel 是预期的（用户关 tab / 流被取消）—— Debug 级足够
- Bridge buffer 阻塞是性能问题 —— Warn 合理
- **ErrInvalidEvent 是 producer bug**（domain/eventlog/eventlog.go:286-352 显式说 "caller bug, not recoverable"）—— **应当 Error 级 + caller 应 fail-loud**

当前实现淡化 ErrInvalidEvent 后，producer-bug 触发时 SSE 缺事件链而前端无从查根因，开发期 grep "emit failed" 找不到。

**建议**: 按 err 类型分流（`errors.Is(err, eventlogdomain.ErrInvalidEvent)` → Error；`errors.Is(err, ctx.Err())` → Debug；其他 → Warn），并在 ErrInvalidEvent 路径下打 stack trace 帮助定位 producer 位置。

### 4. site 9 §S21 dangling parentId 静态难证

行 268-278 fallback `parentID = MessageID` 在 ctx 复用 / context.WithValue 链长场景下，可能拿到上一 turn 残留的 parentBlockID（已 stopped block）作为新 block 的父——前端 router 仍能找到 map entry，但**接 child 到 stopped block** 违反 §S21 暗含的 "stopped block 不再接 child"。

属架构-级 invariant，audit §S3-S17 不直接覆盖，但 cross-cutting 提一笔。运行期校验需要 emit 层维护"已 stopped block IDs" set，违反就 Error。考虑在 Phase 3 加。

### 5. From godoc 与实现不一致（site 16）

`From` 的 godoc（行 432-435）说 "missing emitter logs a warning so wiring bugs surface"，但实际 return 路径**没 log**。轻度 doc-fix。

属 doc bug 不进 §S3-S17 violation 表，但顺手在下次 commit 修一下。

## Recommended fix priorities

按 §S20 + §S14 优先级：

1. **MED — site 14 FinalizeStop 终态写**: 实施"选项 1"——把 detached ctx 下沉到 StopBlock 内部，确保终态总能落库。或者明确"调用方责任"并在每个 emit 调用栈位点审计 ctx 是否 detached。不修留下次需要在 progress-record [risk] 标注"用户取消流后 history replay 看到永远加载中的 block"风险。

2. **LOW — site 5 StartMessage 失败仍返 msgID**: 改返 ""（或在 godoc 显式声明 "返 ID 不保证 publish 成功"）。修复成本极低（5 行），不修等于主动制造 dangling parentId。

3. **LOW — site 3 publish err 粒度**: 按 err 类型分流 log level；不修也能跑，但开发期定位 ErrInvalidEvent 触发会卡住。

4. **LOW — site 6(a) attrs json.Marshal silent drop + site 13(b) AppendDelta silent drop**: (a) 加 Warn log；(b) 在 godoc 明确"事实源"边界与 §S21 append-only 的关系。属文档清理 + 1 行代码。

## Out-of-scope notes

1. **Phase 2B 双协议状态**: pkg/eventlog 与 pkg/notifications 镜像相同 pattern (Emitter + With/From/MustFrom)。本审仅看 eventlog；notifications 应另起一份 audit（看 grep 结果该包也定 sentinel ErrInvalidEvent / ErrSeqTooOld）。

2. **MustFrom panic 信息无 ctx 上下文** (site 17): `panic(fmt.Sprintf("eventlog.MustFrom: no emitter in ctx"))` — `fmt.Sprintf` 无 format-arg（staticcheck S1039 会捞），但属 lint 非 §S3-S17 scope。属 cleanup 不进 violation 表。

3. **noopEmitter 的 logging**: 11 个 no-op 方法全静默——missing emitter 时调用方拿到 "" / 静默继续，不 log warning。这是有意设计（godoc 说 "Returning a no-op (vs nil) lets callers always invoke methods without nil-checks"）。但 wiring-bug surface 完全靠 ctx 中没有 emitter 时上游会注意到——若上游用 MustFrom 才会 panic surface。属设计 trade-off，不是 audit issue。

4. **sentinel registration scope**: 本包的 dual-write 错误源 (`chatdomain.Repository.SaveBlock|AppendDelta|FinalizeStop`) 是 GORM 错误（含 ErrRecordNotFound）——这些 sentinel 的 errmap 登记是 chat repo + chat domain 的责任。本 audit 不延伸跨包。

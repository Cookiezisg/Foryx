# eventlog.go — handlers/eventlog (177 LOC)

Audit scope: §S3 / §S9 / §S15 / §S16 / §S17.

## Trace 表

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | eventlog.go:99-103 | `if v := r.Header.Get("Last-Event-ID"); v != "" { if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 { fromSeq = n } }` | A.1 | OK | §S3 例外：解析 Last-Event-ID 失败 = 客户端发送畸形 header → fallback 到 fromSeq=0（"无 replay 直接实时"）= **N7 SSE 协议显式行为**。注释 line 96-97 已说明（"缺失/非法 → 0"）。这里 silent fallback 是协议本身规定，不是吞错。 | — | — | — | — |
| 2 | eventlog.go:105-117 | `ch, cancelSub, err := h.bridge.Subscribe(r.Context(), conversationID, fromSeq); if err != nil { if errors.Is(err, eventlogdomain.ErrSeqTooOld) { responsehttpapi.Error(w, http.StatusGone, "SEQ_TOO_OLD", ..., nil); return } ; responsehttpapi.FromDomainError(w, h.log, err); return }` | A.5 | OK | §N7 协议显式：`ErrSeqTooOld` 应映射 410 + code `SEQ_TOO_OLD`。handler **自翻** 不走 errmap 是 §S17 已知例外（同 B1 notifications.go pattern + B1 _summary §A.5 已确认）：handler 用 `errors.Is` 自己翻译为 410 envelope，不调 FromDomainError 即不触发 "unmapped" 警报——所以 errmap 不登记不算违规。其他 err 路径走 FromDomainError 兜底（`r.Context().Canceled` 已登记 line 228）。 | — | — | — | — |
| 3 | eventlog.go:118 | `defer cancelSub()` | A.1 | OK | cancelSub 是 cleanup 不返 error；fire-and-forget 解订阅没有错误路径，§S3 不约束。 | — | — | — | — |
| 4 | eventlog.go:120-134 | `responsehttpapi.StreamSSE(w, r, nil, ch, func(out io.Writer, env eventlogdomain.Envelope) error { data, err := json.Marshal(env.Event); if err != nil { h.log.Warn("SSE marshal failed", ...); return err }; _, err = fmt.Fprintf(out, ...); return err })` | A.1 | OK | StreamSSE marshal 失败 zap.Warn + 返 err 让 StreamSSE 主循环退出连接。§S3 不吞——log + propagate。`fmt.Fprintf` 错（client disconnect-on-write）由 StreamSSE 主循环按 sse.go 契约 `_ = onEvent(...)` 接住——B1 已确认是 SSE 协议常态，§S3 例外。 | — | — | — | — |
| 5 | eventlog.go:153-158 | `var fromSeq int64; if v := r.URL.Query().Get("from"); v != "" { if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 { fromSeq = n } }` | A.1 | OK | 同 site 1 推理——非法 from query 落到默认 0 是 N7 协议设计；非 silent 吞错。**注意**：此 endpoint 是 refetch 路径，`from` 缺失/非法时正确行为是"从头返回所有事件"——这正是 fromSeq=0 的语义。 | — | — | — | — |
| 6 | eventlog.go:160-164 | `envelopes, err := h.repo.ReplayEventsAfter(r.Context(), conversationID, fromSeq); if err != nil { responsehttpapi.FromDomainError(w, h.log, err); return }` | A.2 / A.4 / A.5 | OK | 纯读路径——历史回放，r.Context() 是 §S9 推荐项（cancel = 客户端不要这次 refetch）。错误透传走 errmap：`convdomain.ErrNotFound` 已登记（errmap.go:58）；ctx canceled 已登记。无 wrap 违规，handler 不自创 sentinel。 | — | — | — | — |
| 7 | eventlog.go:165-173 | `tailSeq := fromSeq; if n := len(envelopes); n > 0 { tailSeq = envelopes[n-1].Seq } ; responsehttpapi.Success(w, http.StatusOK, map[string]any{ "events": envelopes, "tailSeq": tailSeq, "count": len(envelopes) })` | A.3 | OK | 不 mint ID——直接消费 envelopes。tailSeq 是 LLM 不知道的 per-conv seq 计数器（不是 §S15 prefix_<16hex>），是 N7 SSE 协议字段，归 chatstore 内部管理。§S15 N/A。 | — | — | — | — |
| 8 | eventlog.go:177 | `var _ chatdomain.Repository = (chatdomain.Repository)(nil)` | A.1 | OK | 编译期 import-keep 占位（"_ marker to keep chatdomain import live when repo is nil at compile"）。这是 nil interface 类型断言，无错误处理路径——非 §S3 范畴。 | — | — | — | — |

## Sub-checks

A.1 §S3 错误吞没:
  - violations: **not present**
  - 6 处与错误相关的 site（1, 4, 5, 6 的 if-err 路径），全部合规：site 1/5 是 N7 协议显式 fallback（缺失/非法 → 0）；site 4 marshal 错 log+propagate；site 6 错误透传 errmap

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: **none**——本文件全是只读流（Subscribe + StreamSSE）/ 读路径（ReplayEventsAfter）
  - 各自 ctx 来源: r.Context() 全部使用——SSE Subscribe 的 ctx 用 r.Context() 是正确的（cancel = 客户端断连 = 必须停止订阅释放 channel buffer）
  - violations: **N/A: SSE handler 不做终态写**——所有持久化（写 message_blocks 行）由 chatapp 内 stream-writer goroutine 负责（用 detached ctx），事件日志 SSE 仅是观察者

A.3 §S15 ID 生成:
  - ID generation calls: **none**——handler 是 transport pure shell；conversationID 来自 query/path，envelopes 内含 ID 由 chatapp + chatstore 在写入时生成
  - violations: **N/A**

A.4 §S16 错误 wrap 格式:
  - violations: **not present**——所有错误路径都是 raw forward via FromDomainError 或 self-translate 410（site 2）；无 `fmt.Errorf` 调用
  - site 4 的 `return err` 也合规（最内层透传给 StreamSSE 主循环）

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: **0 in eventlog.go itself**
  - eventlog domain sentinels reachable from handler:
    - `eventlogdomain.ErrSeqTooOld` (eventlog.go:277) — **NOT in errmap**，但 handler **自翻 410** 不走 FromDomainError → §S17 例外（同 B1 notifications.go pattern + B1 _summary §A.5 footnote 6 确认）
    - `eventlogdomain.ErrInvalidEvent` (eventlog.go:286) — 该 sentinel 用于 Bridge.Publish 校验，被 chatapp 内 producer 消费，不冒泡到 handler
  - chatdomain sentinels via repo.ReplayEventsAfter: 仅 ctx canceled / ctx deadline，已登记 errmap.go:228-229
  - missing: **all reachable sentinels are either registered (ctx errs) OR self-translated by handler (ErrSeqTooOld) OR not handler-reachable (ErrInvalidEvent)**

## Cross-cutting note

B1 _summary §A.5 已建立先例：notifications.go (`notificationsdomain.ErrSeqTooOld`) 也是 handler 自翻 410，不登记 errmap 合规。本 handler 与之同 pattern，是双 SSE 协议 (§E1) 一致的实现。同 pattern 后续加新 SSE channel (mcp_server / build_done / ...) 时也将沿用。

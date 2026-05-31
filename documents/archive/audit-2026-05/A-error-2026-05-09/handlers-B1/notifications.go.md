# notifications.go — audit trace

**Path**: `backend/internal/transport/httpapi/handlers/notifications.go`
**LOC**: 103
**Role**: `NotificationsHandler` for `GET /api/v1/notifications` — the global SSE notifications stream (per §E1, mirrors eventlog reconnect semantics with Last-Event-ID + replay buffer + 410 SEQ_TOO_OLD on eviction).

## 9-col trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | notifications.go:51-54 | `if log == nil { log = zap.NewNop() }; return &NotificationsHandler{bridge: bridge, log: log.Named("notifications.handler")}` | A.1 | OK | Defensive nil-logger guard. | N-A | — | — | — |
| 2 | notifications.go:67-73 | `var fromSeq int64; if v := r.Header.Get("Last-Event-ID"); v != "" { if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 { fromSeq = n } }` | A.1 | OK | Bad `Last-Event-ID` header silently falls back to fromSeq=0 (= "subscribe from now / no replay"). Per §N7 / §E1 / SSE protocol, malformed Last-Event-ID is treated as "no Last-Event-ID" — that's the correct protocol behavior, not a §S3 violation. Mirrors eventlog handler pattern. | N-A | — | — | — |
| 3 | notifications.go:75-85 | `ch, cancelSub, err := h.bridge.Subscribe(r.Context(), fromSeq); if err != nil { if errors.Is(err, notificationsdomain.ErrSeqTooOld) { responsehttpapi.Error(w, http.StatusGone, "SEQ_TOO_OLD", ..., nil); return }; responsehttpapi.FromDomainError(w, h.log, err); return }` | A.1/A.5 | OK | `ErrSeqTooOld` translated handler-side to 410 envelope per §N7 SSE protocol — does NOT need errmap registration (matches eventlog pattern, see eventlog/_summary.md §A.5 note). Other errors fall through to `FromDomainError` → 500 INTERNAL_ERROR (acceptable since `Subscribe` only documented sentinel is `ErrSeqTooOld`; any other error is unexpected and 500 with "unmapped domain error" log warning is the desired surface). | N-A | — | — | — |
| 4 | notifications.go:86 | `defer cancelSub()` | A.1 | OK | Subscription cleanup on handler return. `cancelSub` returns nothing per the Bridge contract (typical `func()`), so no error to drop. | N-A | — | — | — |
| 5 | notifications.go:88-101 | `responsehttpapi.StreamSSE(w, r, nil, ch, func(out io.Writer, env notificationsdomain.Envelope) error { data, err := json.Marshal(env.Event); if err != nil { h.log.Warn("SSE marshal failed", ...); return err }; _, err = fmt.Fprintf(out, "event: notification\nid: %d\ndata: %s\n\n", env.Seq, data); return err })` | A.1 | OK | (a) json.Marshal err: logged at Warn + returned. StreamSSE explicitly drops onEvent errors (sse.go:35-38, 47-49 godoc — `_ = onEvent(w, item)` is documented as the contract; "wire-write errors generally mean the client disconnected mid-response"). The Warn log captures the failure (producer bug = unmarshalable Envelope). (b) fmt.Fprintf err: also returned and dropped at sse.go:87 — same documented contract; client-disconnect is the dominant cause. SSE wire format conforms to §N7 (event/id/data lines + blank-line terminator). | N-A | — | — | — |

## Sub-checks

**A.1 §S3 错误吞没**:
- violations: not present. The two "discarded" error paths are:
  - sse.go:87 `_ = onEvent(...)` is the explicit documented contract per StreamSSE godoc (sse.go:35-38, 47-49); marshal/wire failures are logged inside onEvent (here site 5 logs Warn). Canonical §S3 exception.
  - site 2 bad-Last-Event-ID fallback to fromSeq=0 is correct §N7 SSE protocol semantics, not silent fallback.

**A.2 §S9 detached ctx 终态写**:
- terminal-state writes identified: none — handler is read-only SSE stream; no DB writes
- 各自 ctx 来源: `r.Context()` (passed to Bridge.Subscribe + drives StreamSSE shutdown loop)
- violations: N/A: streaming read; cancel-on-disconnect is the desired behavior

**A.3 §S15 ID 生成**:
- ID generation calls: none in this file
- violations: N/A: `seq` is bridge-assigned (per-Bridge counter, not §S15 prefix-format business ID); no `<prefix>_<16hex>` to mint here

**A.4 §S16 错误 wrap 格式**:
- violations: not present (no `fmt.Errorf` wrapping; only `errors.Is` for type-discrimination on ErrSeqTooOld)

**A.5 §S17 sentinel 登记 errmap**:
- sentinels defined: none in this file (the package this file imports — `notificationsdomain` — defines `ErrSeqTooOld` at notifications.go:103)
- 已登记 errmap: `notificationsdomain.ErrSeqTooOld` is **NOT** in errmap.go::errTable
- missing: none — per §S17 spec extract "完全包内 / 跨包但只在 service 层消费、handler 层翻译成别的 sentinel 的，不需要登记": handler catches `ErrSeqTooOld` via `errors.Is` and writes 410 directly via `responsehttpapi.Error`. This sentinel never reaches `FromDomainError`. Mirrors the eventlog handler pattern (see pkg-eventlog/_summary.md §A.5 note: "都经 handlers/eventlog.go:107 直接 errors.Is 处理（返 410 SEQ_TOO_OLD 按 §N7），不走 FromDomainError——无需登记 errmap"). Confirmed legitimate.

## Summary

- Sites: 5
- Violations: 0 (0 HIGH / 0 MED / 0 LOW)
- Verdict: textbook clean SSE handler matching the documented eventlog pattern. ErrSeqTooOld handler-side translation to 410 is correct per §N7 + spec extract, no errmap row needed. All discarded error paths are documented exceptions (StreamSSE contract + protocol semantics).

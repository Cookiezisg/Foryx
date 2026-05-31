# Audit: backend/internal/transport/httpapi/response/sse.go

**LOC**: 96 (production); single generic helper `StreamSSE[T]`.

## Purpose

Generic SSE serving helper. Centralises wire boilerplate: 4 standard headers + initial flush + 15s keep-alive ping + ctx-driven shutdown. Each SSE handler describes only how to write per item via `onEvent` callback.

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | sse.go:57-61 | `flusher, ok := w.(http.Flusher); if !ok { Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "streaming not supported", nil); return }` | A.1 | OK | §S3 — flusher missing is a hard pre-condition error, surfaced as 500 envelope. Not silent: caller (operator) sees the explicit message. | — | — | — | — |
| 2 | sse.go:63-69 | `w.Header().Set("Content-Type", "text/event-stream"); w.Header().Set("Cache-Control", "no-cache"); ... w.Header().Set("X-Accel-Buffering", "no")` | A.1 | OK | §S3 — `Header().Set` returns no error. Pure header writes. | — | — | — | — |
| 3 | sse.go:73-76 | `if onPrelude != nil { onPrelude(w); flusher.Flush() }` | A.1 | OK | §S3 — onPrelude signature is `func(io.Writer)` returning nothing. No error to swallow. Caller can log internally if needed. | — | — | — | — |
| 4 | sse.go:81-95 | `for { select { case item, ok := <-items: if !ok { return }; _ = onEvent(w, item); flusher.Flush(); case <-ticker.C: fmt.Fprint(w, ": keep-alive\n\n"); flusher.Flush(); case <-r.Context().Done(): return } }` | A.1 | EDGE | §S3 — `_ = onEvent(w, item)` swallows the per-item write error. **Documented** in godoc lines 35-38 / 47-49: "Errors returned by onEvent are silently ignored — wire-write errors generally mean the client disconnected mid-response, the status code is already sent, and the loop's next iteration / ctx-cancel will tear down cleanly." This is the documented carve-out per §S3 example 5 / spec line 21-26 (cleanup paths where the client-disconnected case is unrecoverable). The contract is also explicit that callers can log inside `onEvent` themselves. **However**, the `_ =` line itself does not have an inline pointer to the godoc; spec ritual recommends a one-line `// _ = err — see godoc, client likely disconnected`. | LOW | None — disconnected client can't see anything. Inline comment would help future readers without scrolling up. | Add inline `// _ = err — see godoc; wire write fails when client disconnected, ctx-cancel will tear down on next iteration.` | FOUND |
| 5 | sse.go:89-91 | `case <-ticker.C: fmt.Fprint(w, ": keep-alive\n\n"); flusher.Flush()` | A.1 | EDGE | §S3 — `fmt.Fprint(w, ...)` returns `(n int, err error)`; both discarded by the bare expression statement. Same client-disconnected rationale as site#4. **No inline comment** explains; the godoc covers `onEvent` errors but not `fmt.Fprint`. Worth a one-liner for parity. | LOW | Same as #4 — disconnected client invisible. Operator sees nothing if keep-alive write fails repeatedly (in practice the next ticker tick or ctx-Done teardown will surface). | Either capture and discard with comment: `_, _ = fmt.Fprint(w, ": keep-alive\n\n") // best-effort; client may have disconnected, ctx-Done will tear down.` Or extract a `writeKeepAlive` helper that encapsulates the rationale. | FOUND |

## Sub-checks

```
A.1 §S3 错误吞没:
  - violations: [site#4, site#5] — onEvent and fmt.Fprint errors discarded; documented in godoc but missing inline pointer (§S3 spec line 12-14 ritual)
A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none (StreamSSE is pure wire transport, no DB / persistent state)
  - 各自 ctx 来源: r.Context() is used for shutdown signal only (line 92); not for any terminal write
  - violations: N/A: helper doesn't perform DB / persistent terminal writes (callers' onEvent / onPrelude could, but that's their responsibility)
A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A: package doesn't generate business IDs
A.4 §S16 错误 wrap 格式:
  - violations: not present (no fmt.Errorf calls)
A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A — INTERNAL_ERROR wire code at line 59 matches errmap.go:260 default; emitted directly because "streaming not supported" is a transport-level pre-condition, not a domain error chain
  - missing: N/A: file defines no Go sentinels
```

## Findings

**2 LOW EDGE**: site#4 (onEvent err) and site#5 (fmt.Fprint err) silently dropped without inline comment. Behavior is correct (client likely disconnected, ctx-cancel will tear down) and godoc covers site#4. Audit recommendation: 1-line comment at each site for §S3 ritual parity with recover.go style. Net functional impact: zero.

# audit: backend/internal/app/skill/polling.go

LOC: 102
Read: full file (lines 1-102)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | polling.go:49-53 | `Start: if err := s.Scan(ctx); err != nil { s.log.Warn("skill initial scan failed (continuing with empty cache)", zap.Error(err)) }` | A.1 | OK | §S3 documented soft-fail with audit log: file header (lines 38-43) explicitly cites the rationale — boot-time I/O hiccup must not take the whole app down; next polling tick (1s) retries. Warn log satisfies §S10 "异步必须打". Compliance literal. | N-A | — | — | — |
| 2 | polling.go:54-62 | `pollCtx, pollCancel := context.WithCancel(ctx); s.stopCancel = pollCancel; ...; go func() { defer close(s.pollDone); s.pollLoop(pollCtx) }()` | A.2 | OK | poll goroutine derives from request ctx but **context-management** is correct — pollCancel is captured for explicit Stop drainage; pollDone signals goroutine exit so caller can wait. Not a §S9 terminal-write concern; this is lifecycle plumbing. | N-A | — | — | — |
| 3 | polling.go:72-81 | `Stop: ...; if s.stopCancel != nil { s.stopCancel() }; if s.pollDone != nil { <-s.pollDone }` | A.1 | OK | idempotent Stop via sync.Once; drains via pollDone channel before returning. | N-A | — | — | — |
| 4 | polling.go:89-101 | `pollLoop: for { select { case <-ctx.Done(): return; case <-ticker.C: if err := s.Scan(ctx); err != nil { s.log.Warn("skill rescan failed", zap.Error(err)) } } }` | A.1 | OK | §S3 documented soft-fail per file header (lines 84-86): rescan errors are logged + the next tick retries. Per-tick log noise is minimized via Scan's fingerprint short-circuit (only changes publish). | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (sites #1, #4 are documented soft-fails with Warn log; spec carve-out for boot/poll best-effort applies)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none (file is lifecycle + retry only; actual Scan side-effects are audited in scan.go)
  - 各自 ctx 来源: request ctx for initial Scan; derived pollCtx for periodic Scan
  - violations: N/A — file does no terminal writes itself

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A

A.4 §S16 错误 wrap 格式:
  - violations: not present (errors only logged via zap, never propagated)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels
